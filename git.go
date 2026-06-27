package main

import (
	"net/url"
	"os/exec"
	"regexp"
	"strconv"
	"strings"
)

type worktree struct {
	Path   string
	Branch string
	Dirty  int
	Ahead  int
	Behind int
	Clean  bool
}

func runGit(dir string, args ...string) (string, bool) {
	cmd := exec.Command("git", append([]string{"-C", dir, "-c", "color.ui=false"}, args...)...)
	out, err := cmd.Output()
	if err != nil {
		return "", false
	}
	return string(out), true
}

// commonDir returns the shared .git dir for a path, used to group worktrees of
// the same repo together. ok=false means the path is not inside a git repo.
func commonDir(dir string) (string, bool) {
	out, ok := runGit(dir, "rev-parse", "--path-format=absolute", "--git-common-dir")
	if !ok {
		return "", false
	}
	return strings.TrimSpace(out), true
}

// listWorktrees enumerates every worktree of the repo that `dir` belongs to.
func listWorktrees(dir string) []worktree {
	out, ok := runGit(dir, "worktree", "list", "--porcelain")
	if !ok {
		return nil
	}
	var wts []worktree
	var cur worktree
	flush := func() {
		if cur.Path != "" {
			cur.Clean = cur.Dirty == 0
			wts = append(wts, cur)
		}
		cur = worktree{}
	}
	for _, line := range strings.Split(out, "\n") {
		switch {
		case strings.HasPrefix(line, "worktree "):
			flush()
			cur.Path = strings.TrimPrefix(line, "worktree ")
		case strings.HasPrefix(line, "branch "):
			cur.Branch = shortBranch(strings.TrimPrefix(line, "branch "))
		case line == "detached":
			cur.Branch = "(detached)"
		}
	}
	flush()
	for i := range wts {
		fillStatus(&wts[i])
	}
	return wts
}

var aheadRe = regexp.MustCompile(`ahead (\d+)`)
var behindRe = regexp.MustCompile(`behind (\d+)`)

func fillStatus(w *worktree) {
	out, ok := runGit(w.Path, "status", "-sb", "--porcelain=v1")
	if !ok {
		return
	}
	for i, line := range strings.Split(strings.TrimRight(out, "\n"), "\n") {
		if i == 0 {
			if m := aheadRe.FindStringSubmatch(line); m != nil {
				w.Ahead, _ = strconv.Atoi(m[1])
			}
			if m := behindRe.FindStringSubmatch(line); m != nil {
				w.Behind, _ = strconv.Atoi(m[1])
			}
			continue
		}
		if strings.TrimSpace(line) != "" {
			w.Dirty++
		}
	}
	w.Clean = w.Dirty == 0
}

func shortBranch(ref string) string {
	return strings.TrimPrefix(strings.TrimSpace(ref), "refs/heads/")
}

// adoRemote reads dir's origin remote and parses it into ADO org/project/repo.
// ok=false means the repo is not on Azure DevOps (GitHub 등) → ADO 조회 생략.
func adoRemote(dir string) (org, project, repo string, ok bool) {
	out, gitOK := runGit(dir, "config", "--get", "remote.origin.url")
	if !gitOK {
		return "", "", "", false
	}
	return parseADOURL(out)
}

// parseADOURL extracts Azure DevOps org/project/repo from a git remote URL.
// ok=false for non-ADO remotes. Supported forms:
//
//	git@ssh.dev.azure.com:v3/<org>/<project>/<repo>
//	https://[user@]dev.azure.com/<org>/<project>/_git/<repo>
//	https://<org>.visualstudio.com/[Collection/]<project>/_git/<repo>
//	<user>@vs-ssh.visualstudio.com:v3/<org>/<project>/<repo>
func parseADOURL(raw string) (org, project, repo string, ok bool) {
	u := strings.TrimSuffix(strings.TrimSpace(raw), "/")
	u = strings.TrimSuffix(u, ".git")
	if !strings.Contains(u, "dev.azure.com") && !strings.Contains(u, ".visualstudio.com") {
		return "", "", "", false
	}
	dec := func(s string) string {
		if d, err := url.PathUnescape(s); err == nil {
			return d
		}
		return s
	}
	// SSH v3 form (ssh.dev.azure.com / vs-ssh.visualstudio.com): .../v3/<org>/<project>/<repo>
	if i := strings.Index(u, ":v3/"); i >= 0 {
		p := strings.Split(u[i+len(":v3/"):], "/")
		if len(p) == 3 && p[0] != "" && p[1] != "" && p[2] != "" {
			return dec(p[0]), dec(p[1]), dec(p[2]), true
		}
		return "", "", "", false
	}
	// HTTPS dev.azure.com: dev.azure.com/<org>/<project>/_git/<repo>
	if i := strings.Index(u, "dev.azure.com/"); i >= 0 {
		p := strings.Split(u[i+len("dev.azure.com/"):], "/")
		if len(p) >= 4 && p[2] == "_git" {
			return dec(p[0]), dec(p[1]), dec(p[3]), true
		}
		return "", "", "", false
	}
	// HTTPS visualstudio.com: <org>.visualstudio.com/[Collection/]<project>/_git/<repo>
	if i := strings.Index(u, ".visualstudio.com/"); i >= 0 {
		host := u[:i]
		if j := strings.LastIndex(host, "/"); j >= 0 {
			host = host[j+1:]
		}
		if j := strings.LastIndex(host, "@"); j >= 0 {
			host = host[j+1:]
		}
		p := strings.Split(u[i+len(".visualstudio.com/"):], "/")
		for k := 0; k+1 < len(p); k++ {
			if p[k] == "_git" && k >= 1 {
				return dec(host), dec(p[k-1]), dec(p[k+1]), true
			}
		}
	}
	return "", "", "", false
}
