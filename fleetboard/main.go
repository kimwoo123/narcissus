package main

import (
	"embed"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
)

//go:embed web/index.html
var webFS embed.FS

func main() {
	cfg := loadConfig()
	var ado *adoClient
	if cfg.adoConfigured() {
		ado = newADO(cfg)
	}

	mux := http.NewServeMux()

	mux.HandleFunc("/api/state", func(w http.ResponseWriter, r *http.Request) {
		state := build(cfg, ado)
		w.Header().Set("Content-Type", "application/json")
		enc := json.NewEncoder(w)
		enc.SetIndent("", "  ")
		enc.Encode(state)
	})

	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/" {
			http.NotFound(w, r)
			return
		}
		page, err := webFS.ReadFile("web/index.html")
		if err != nil {
			http.Error(w, err.Error(), 500)
			return
		}
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.Write(page)
	})

	addr := "127.0.0.1:" + cfg.Port
	fmt.Printf("FleetBoard → http://%s\n", addr)
	fmt.Printf("  CLAUDE_HOME = %s\n", cfg.ClaudeHome)
	if cfg.adoConfigured() {
		fmt.Printf("  Azure DevOps = %s/%s\n", cfg.ADOOrg, cfg.ADOProject)
	} else {
		fmt.Printf("  Azure DevOps = (not configured; set ADO_ORG / ADO_PROJECT / ADO_PAT)\n")
	}
	log.Fatal(http.ListenAndServe(addr, mux))
}
