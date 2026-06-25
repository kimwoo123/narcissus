package main

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"sync"
	"time"
)

// session is the distilled state of one Claude Code session jsonl file.
type session struct {
	ID        string
	Cwd       string
	Branch    string
	Title     string // aiTitle — "what this session was doing"
	Prompt    string // last user prompt
	Version   string
	LastRole  string // user | assistant — role of the latest main-chain entry
	LastTS    time.Time
}

type rawEntry struct {
	Type        string          `json:"type"`
	AiTitle     string          `json:"aiTitle"`
	LastPrompt  string          `json:"lastPrompt"`
	Timestamp   string          `json:"timestamp"`
	Cwd         string          `json:"cwd"`
	GitBranch   string          `json:"gitBranch"`
	Version     string          `json:"version"`
	IsSidechain bool            `json:"isSidechain"`
	Message     json.RawMessage `json:"message"`
}

// parse cache keyed by file path; reparses only when mtime changes.
var (
	sessCacheMu sync.Mutex
	sessCache   = map[string]sessCacheEntry{}
)

type sessCacheEntry struct {
	mtime time.Time
	size  int64
	sess  *session
}

// loadSessions scans CLAUDE_HOME/projects/*/*.jsonl and returns one distilled
// session per file that has been active within MaxAgeHours.
func loadSessions(cfg Config) []*session {
	root := filepath.Join(cfg.ClaudeHome, "projects")
	dirs, err := os.ReadDir(root)
	if err != nil {
		return nil
	}
	cutoff := time.Now().Add(-time.Duration(cfg.MaxAgeHours) * time.Hour)

	var out []*session
	for _, d := range dirs {
		if !d.IsDir() {
			continue
		}
		files, err := os.ReadDir(filepath.Join(root, d.Name()))
		if err != nil {
			continue
		}
		for _, f := range files {
			if f.IsDir() || filepath.Ext(f.Name()) != ".jsonl" {
				continue
			}
			path := filepath.Join(root, d.Name(), f.Name())
			s := parseSessionCached(path)
			if s == nil || s.LastTS.Before(cutoff) || s.Cwd == "" {
				continue
			}
			out = append(out, s)
		}
	}
	return out
}

func parseSessionCached(path string) *session {
	info, err := os.Stat(path)
	if err != nil {
		return nil
	}
	sessCacheMu.Lock()
	if e, ok := sessCache[path]; ok && e.mtime.Equal(info.ModTime()) && e.size == info.Size() {
		sessCacheMu.Unlock()
		return e.sess
	}
	sessCacheMu.Unlock()

	s := parseSession(path)
	sessCacheMu.Lock()
	sessCache[path] = sessCacheEntry{mtime: info.ModTime(), size: info.Size(), sess: s}
	sessCacheMu.Unlock()
	return s
}

func parseSession(path string) *session {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil
	}
	s := &session{ID: trimExt(filepath.Base(path))}
	for _, line := range bytes.Split(data, []byte{'\n'}) {
		line = bytes.TrimSpace(line)
		if len(line) == 0 {
			continue
		}
		var e rawEntry
		if json.Unmarshal(line, &e) != nil {
			continue
		}
		switch e.Type {
		case "ai-title":
			if e.AiTitle != "" {
				s.Title = e.AiTitle
			}
		case "last-prompt":
			if e.LastPrompt != "" {
				s.Prompt = e.LastPrompt
			}
		case "user", "assistant":
			if e.IsSidechain {
				continue // subagent traffic, not the user-facing turn
			}
			ts := parseTS(e.Timestamp)
			if ts.IsZero() || !ts.After(s.LastTS) {
				continue
			}
			s.LastTS = ts
			s.LastRole = e.Type
			if e.Cwd != "" {
				s.Cwd = e.Cwd
			}
			if e.GitBranch != "" {
				s.Branch = e.GitBranch
			}
			if e.Version != "" {
				s.Version = e.Version
			}
		}
	}
	if s.LastTS.IsZero() {
		return nil
	}
	return s
}

func parseTS(s string) time.Time {
	if s == "" {
		return time.Time{}
	}
	t, err := time.Parse(time.RFC3339Nano, s)
	if err != nil {
		return time.Time{}
	}
	return t
}

func trimExt(name string) string {
	return name[:len(name)-len(filepath.Ext(name))]
}
