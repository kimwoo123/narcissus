// API 응답 조립: 프로젝트/세션 목록, 세션 대화 렌더링, 경로 검증.
package main

import (
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

// sessionPath는 project/file이 PROJECTS_DIR 바로 아래의 실제 파일일 때만 경로를 준다.
func sessionPath(project, file string) (string, bool) {
	if !validName(project) || !validName(file) {
		return "", false
	}
	p := filepath.Join(projectsDir, project, file)
	if fi, err := os.Stat(p); err != nil || fi.IsDir() {
		return "", false
	}
	return p, true
}

func validName(s string) bool {
	return s != "" && s != "." && s != ".." && !strings.ContainsAny(s, `/\`)
}

type projectInfo struct {
	ID    string `json:"id"`
	Label string `json:"label"`
}

func listProjects() []projectInfo {
	entries, err := os.ReadDir(projectsDir)
	if err != nil {
		return []projectInfo{}
	}
	type pe struct {
		name string
		mt   time.Time
	}
	var ps []pe
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		info, err := e.Info()
		if err != nil {
			continue
		}
		ps = append(ps, pe{e.Name(), info.ModTime()})
	}
	sort.Slice(ps, func(i, j int) bool { return ps[i].mt.After(ps[j].mt) })

	home, _ := os.UserHomeDir()
	user := filepath.Base(home)
	out := make([]projectInfo, 0, len(ps))
	for _, p := range ps {
		// 앞쪽 '-Users-<me>-' 류 접두어를 정리해 표시
		label := strings.ReplaceAll(p.name, "-Users-"+user+"-", "")
		label = strings.ReplaceAll(label, "-Users-"+user, "")
		if label == "" {
			label = p.name
		}
		out = append(out, projectInfo{p.name, label})
	}
	return out
}

type sessionInfo struct {
	File  string  `json:"file"`
	Title string  `json:"title"`
	Label *string `json:"label"`
	Mtime string  `json:"mtime"`
}

func listSessions(project string) []sessionInfo {
	out := []sessionInfo{}
	if !validName(project) {
		return out
	}
	dir := filepath.Join(projectsDir, project)
	entries, err := os.ReadDir(dir)
	if err != nil {
		return out
	}
	type fe struct {
		name string
		mt   time.Time
	}
	var fs []fe
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".jsonl") {
			continue
		}
		info, err := e.Info()
		if err != nil {
			continue
		}
		fs = append(fs, fe{e.Name(), info.ModTime()})
	}
	sort.Slice(fs, func(i, j int) bool { return fs[i].mt.After(fs[j].mt) })

	labels := loadLabels()
	for _, f := range fs {
		lines := loadSession(filepath.Join(dir, f.name))
		out = append(out, sessionInfo{
			File:  f.name,
			Title: sessionTitle(lines),
			Label: labelPtr(labels, project+"/"+f.name),
			Mtime: f.mt.Format("01-02 15:04"),
		})
	}
	return out
}

type message struct {
	Role   string  `json:"role"`
	Time   string  `json:"time"`
	Blocks []block `json:"blocks"`
}

type sessionResp struct {
	Title    string    `json:"title"`
	Label    *string   `json:"label"`
	Messages []message `json:"messages"`
}

func renderSession(project, file string) sessionResp {
	resp := sessionResp{Messages: []message{}}
	path, ok := sessionPath(project, file)
	if !ok {
		return resp
	}
	lines := loadSession(path)

	// 단일 패스: 라인마다 iterBlocks를 '한 번만' 호출해 블록을 추출하고
	// (병목 3: 이전엔 두 번 호출), 동시에 tool_use_id -> 결과 매핑을 모은다.
	type parsedMsg struct {
		role, ts string
		blocks   []block
	}
	var msgs []parsedMsg
	results := map[string]string{}
	for _, o := range lines {
		bs := iterBlocks(o)
		for _, b := range bs {
			if b.Kind == "result" && b.ref != "" {
				results[b.ref] = strings.TrimSpace(b.Text)
			}
		}
		if t := str(o["type"]); t == "user" || t == "assistant" {
			msgs = append(msgs, parsedMsg{t, str(o["timestamp"]), bs})
		}
	}

	// 추출된 블록을 조립한다 (여기선 JSON 파싱 없음 — 결과 붙이고 비우기만).
	for _, pm := range msgs {
		var blocks []block
		for _, b := range pm.blocks {
			if b.Kind == "text" {
				b.Text = stripWrappers(b.Text)
			} else {
				b.Text = strings.TrimSpace(b.Text)
			}
			if b.Kind == "result" {
				if _, paired := results[b.ref]; paired {
					continue // 결과는 대응하는 도구 호출 쪽에 붙여서 보여준다
				}
			}
			if b.Text == "" {
				continue
			}
			if b.Kind == "tool" {
				if r, paired := results[b.ref]; paired {
					b.Result = &r // 호출+결과를 한 묶음으로
				}
			}
			// Claude의 일반 텍스트만 마크다운 → HTML (내 메시지·thinking·도구는 원문 유지)
			if b.Kind == "text" && pm.role == "assistant" {
				b.HTML = renderMarkdown(b.Text)
			}
			blocks = append(blocks, b)
		}
		if len(blocks) > 0 {
			resp.Messages = append(resp.Messages, message{
				Role:   pm.role,
				Time:   fmtTime(pm.ts),
				Blocks: blocks,
			})
		}
	}
	resp.Title = sessionTitle(lines)
	resp.Label = labelPtr(loadLabels(), project+"/"+file)
	return resp
}
