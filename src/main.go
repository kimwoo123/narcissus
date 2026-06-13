// Claude JSONL 세션 뷰어 (Go 포팅).
//
// ~/.claude/projects 아래의 세션 JSONL 파일을 사이드바에서 고르고
// 메인 화면에서 대화 내용을 읽어보는 로컬 웹앱.
// fsnotify(커널 파일 이벤트) + SSE로 변경을 실시간 반영한다.
//
// 빌드:  go build -o jsonl-viewer
// 실행:  ./jsonl-viewer   (브라우저가 자동으로 열림, http://localhost:8765)
//
// 파일 구성:
//   main.go   — 진입점: 전역 설정, HTTP 라우팅, 서버 기동
//   parse.go  — JSONL 파싱: 라인 읽기, 제목 추출, 블록 펼치기
//   api.go    — API 응답 조립: 목록/세션 렌더링, 경로 검증
//   labels.go — 사용자 라벨 저장소
//   watch.go  — 변경 감지: fsnotify 감시 + SSE 허브
package main

import (
	_ "embed"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"time"
)

//go:embed index.html
var indexHTML []byte

const port = 8765

var (
	projectsDir string
	labelsFile  string
)

func writeJSON(w http.ResponseWriter, v any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	json.NewEncoder(w).Encode(v)
}

func main() {
	home, err := os.UserHomeDir()
	if err != nil {
		log.Fatal(err)
	}
	projectsDir = filepath.Join(home, ".claude", "projects")
	labelsFile = filepath.Join(home, ".claude", "jsonl_viewer_labels.json")

	if err := startWatcher(); err != nil {
		log.Fatal(err)
	}

	http.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/" {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.Write(indexHTML)
	})
	http.HandleFunc("/api/projects", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, listProjects())
	})
	http.HandleFunc("/api/sessions", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, listSessions(r.URL.Query().Get("project")))
	})
	http.HandleFunc("/api/session", func(w http.ResponseWriter, r *http.Request) {
		q := r.URL.Query()
		writeJSON(w, renderSession(q.Get("project"), q.Get("file")))
	})
	http.HandleFunc("/api/label", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, "POST only", http.StatusMethodNotAllowed)
			return
		}
		var body struct{ Project, File, Label string }
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			http.Error(w, "bad request", http.StatusBadRequest)
			return
		}
		writeJSON(w, map[string]bool{"ok": setLabel(body.Project, body.File, body.Label)})
	})
	http.HandleFunc("/api/events", sseHandler)

	url := fmt.Sprintf("http://localhost:%d", port)
	fmt.Printf("Claude JSONL 뷰어 실행 중 → %s  (종료: Ctrl+C)\n", url)
	go func() {
		time.Sleep(800 * time.Millisecond)
		exec.Command("open", url).Start()
	}()
	log.Fatal(http.ListenAndServe(fmt.Sprintf("127.0.0.1:%d", port), nil))
}
