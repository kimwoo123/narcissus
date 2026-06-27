package main

import (
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

// Config holds all runtime settings. Everything is driven by environment
// variables so the same binary runs unchanged on any local machine.
type Config struct {
	Port        string // FLEETBOARD_PORT (default 7777)
	ClaudeHome  string // CLAUDE_HOME (default ~/.claude)
	MaxAgeHours int    // FLEETBOARD_MAX_AGE_HOURS: hide sessions idle longer than this (default 168 = 7d)
	RunningSec  int    // FLEETBOARD_RUNNING_SEC: activity newer than this counts as "running" (default 45)
	WaitingMin  int    // FLEETBOARD_WAITING_MIN: assistant-ended session newer than this is "waiting"; older (but within WaitingHrs) is "recent" (default 30)
	WaitingHrs  int    // FLEETBOARD_WAITING_HOURS: assistant-ended session older than this is "idle", not "recent" (default 24)
	ExtraRepos  []string

	// ADO org/project/repo는 각 repo의 git origin 리모트에서 자동 판별한다(adoRemote).
	// 설정으로는 인증용 PAT만 받는다. PAT는 org 범위라 여러 프로젝트를 함께 조회한다.
	ADOPat     string // ADO_PAT
	BaseBranch string // FLEETBOARD_BASE_BRANCH: 머지 대상 브랜치 (default main)
}

func env(key, def string) string {
	if v := strings.TrimSpace(os.Getenv(key)); v != "" {
		return v
	}
	return def
}

func envInt(key string, def int) int {
	if v := strings.TrimSpace(os.Getenv(key)); v != "" {
		if n, err := strconv.Atoi(v); err == nil {
			return n
		}
	}
	return def
}

func loadConfig() Config {
	home, _ := os.UserHomeDir()
	c := Config{
		Port:        env("FLEETBOARD_PORT", "7777"),
		ClaudeHome:  env("CLAUDE_HOME", filepath.Join(home, ".claude")),
		MaxAgeHours: envInt("FLEETBOARD_MAX_AGE_HOURS", 168),
		RunningSec:  envInt("FLEETBOARD_RUNNING_SEC", 45),
		WaitingMin:  envInt("FLEETBOARD_WAITING_MIN", 30),
		WaitingHrs:  envInt("FLEETBOARD_WAITING_HOURS", 24),
		ADOPat:      env("ADO_PAT", ""),
		BaseBranch:  env("FLEETBOARD_BASE_BRANCH", "main"),
	}
	// FLEETBOARD_REPOS: extra repo paths (comma or colon separated) to show
	// even when no Claude session currently targets them.
	if raw := strings.TrimSpace(os.Getenv("FLEETBOARD_REPOS")); raw != "" {
		for _, p := range strings.FieldsFunc(raw, func(r rune) bool { return r == ',' || r == ':' }) {
			if p = strings.TrimSpace(p); p != "" {
				c.ExtraRepos = append(c.ExtraRepos, p)
			}
		}
	}
	return c
}

func (c Config) adoConfigured() bool {
	return c.ADOPat != ""
}
