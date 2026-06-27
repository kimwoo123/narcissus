package main

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"sync"
	"time"
)

// Azure DevOps lookups. Every query is scoped to a specific repository whose
// org/project/repo are derived from that repo's git remote (see adoRemote), so
// branches with the same name across repos/projects never cross-match. Results
// are cached briefly so 5s board polls don't hammer the API.
const adoTTL = 20 * time.Second

type pipeline struct {
	ID     int    `json:"id"`
	Number string `json:"number"`
	Status string `json:"status"` // notStarted | inProgress | completed
	Result string `json:"result"` // succeeded | failed | canceled | partiallySucceeded
	URL    string `json:"url"`
}

type pullRequest struct {
	ID    int    `json:"id"`
	Title string `json:"title"`
	URL   string `json:"url"`
}

type adoClient struct {
	auth    string
	mu      sync.Mutex
	idCache map[string]string        // org/project/repo -> repository GUID
	bCache  map[string]adoCacheEntry // org/project/repo/branch -> build
	pCache  map[string]adoCacheEntry // org/project/repo/branch -> active PR
	mCache  map[string]adoCacheEntry // org/project/repo/branch->base -> merged PR
}

type adoCacheEntry struct {
	at     time.Time
	pipe   *pipeline
	pr     *pullRequest
	merged *pullRequest
}

func newADO(cfg Config) *adoClient {
	return &adoClient{
		auth:    "Basic " + base64.StdEncoding.EncodeToString([]byte(":"+cfg.ADOPat)),
		idCache: map[string]string{},
		bCache:  map[string]adoCacheEntry{},
		pCache:  map[string]adoCacheEntry{},
		mCache:  map[string]adoCacheEntry{},
	}
}

func (a *adoClient) get(rawurl string, v any) bool {
	req, err := http.NewRequest("GET", rawurl, nil)
	if err != nil {
		return false
	}
	req.Header.Set("Authorization", a.auth)
	req.Header.Set("Accept", "application/json")
	client := &http.Client{Timeout: 8 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return false
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		io.Copy(io.Discard, resp.Body)
		return false
	}
	return json.NewDecoder(resp.Body).Decode(v) == nil
}

func (a *adoClient) base(org, project string) string {
	return fmt.Sprintf("https://dev.azure.com/%s/%s/_apis",
		url.PathEscape(org), url.PathEscape(project))
}

// repoID resolves a repository's stable GUID, used to scope build/PR queries to
// exactly this repo. Cached forever (repo IDs don't change); only successes are
// cached so a transient failure retries on the next poll.
func (a *adoClient) repoID(org, project, repo string) (string, bool) {
	key := org + "/" + project + "/" + repo
	a.mu.Lock()
	id, ok := a.idCache[key]
	a.mu.Unlock()
	if ok {
		return id, true
	}
	u := fmt.Sprintf("%s/git/repositories/%s?api-version=7.1",
		a.base(org, project), url.PathEscape(repo))
	var resp struct {
		ID string `json:"id"`
	}
	if !a.get(u, &resp) || resp.ID == "" {
		return "", false
	}
	a.mu.Lock()
	a.idCache[key] = resp.ID
	a.mu.Unlock()
	return resp.ID, true
}

func (a *adoClient) prWebURL(org, project, repo string, id int) string {
	return fmt.Sprintf("https://dev.azure.com/%s/%s/_git/%s/pullrequest/%d",
		url.PathEscape(org), url.PathEscape(project), url.PathEscape(repo), id)
}

// latestBuild returns the most recent pipeline run for a branch in this repo.
func (a *adoClient) latestBuild(org, project, repo, branch string) *pipeline {
	if branch == "" {
		return nil
	}
	key := org + "/" + project + "/" + repo + "/" + branch
	a.mu.Lock()
	if e, ok := a.bCache[key]; ok && time.Since(e.at) < adoTTL {
		a.mu.Unlock()
		return e.pipe
	}
	a.mu.Unlock()

	var p *pipeline
	if id, ok := a.repoID(org, project, repo); ok {
		u := fmt.Sprintf("%s/build/builds?api-version=7.1&$top=1&queryOrder=queueTimeDescending&repositoryId=%s&repositoryType=TfsGit&branchName=%s",
			a.base(org, project), url.QueryEscape(id), url.QueryEscape("refs/heads/"+branch))
		var resp struct {
			Value []struct {
				ID          int    `json:"id"`
				BuildNumber string `json:"buildNumber"`
				Status      string `json:"status"`
				Result      string `json:"result"`
				Links       struct {
					Web struct {
						Href string `json:"href"`
					} `json:"web"`
				} `json:"_links"`
			} `json:"value"`
		}
		if a.get(u, &resp) && len(resp.Value) > 0 {
			b := resp.Value[0]
			p = &pipeline{ID: b.ID, Number: b.BuildNumber, Status: b.Status, Result: b.Result, URL: b.Links.Web.Href}
		}
	}
	a.mu.Lock()
	a.bCache[key] = adoCacheEntry{at: time.Now(), pipe: p}
	a.mu.Unlock()
	return p
}

// activePR returns the active pull request whose source branch matches, in this repo.
func (a *adoClient) activePR(org, project, repo, branch string) *pullRequest {
	if branch == "" {
		return nil
	}
	key := org + "/" + project + "/" + repo + "/" + branch
	a.mu.Lock()
	if e, ok := a.pCache[key]; ok && time.Since(e.at) < adoTTL {
		a.mu.Unlock()
		return e.pr
	}
	a.mu.Unlock()

	var pr *pullRequest
	if id, ok := a.repoID(org, project, repo); ok {
		u := fmt.Sprintf("%s/git/repositories/%s/pullrequests?api-version=7.1&$top=1&searchCriteria.status=active&searchCriteria.sourceRefName=%s",
			a.base(org, project), url.PathEscape(id), url.QueryEscape("refs/heads/"+branch))
		var resp struct {
			Value []struct {
				ID    int    `json:"pullRequestId"`
				Title string `json:"title"`
			} `json:"value"`
		}
		if a.get(u, &resp) && len(resp.Value) > 0 {
			v := resp.Value[0]
			pr = &pullRequest{ID: v.ID, Title: v.Title, URL: a.prWebURL(org, project, repo, v.ID)}
		}
	}
	a.mu.Lock()
	a.pCache[key] = adoCacheEntry{at: time.Now(), pr: pr}
	a.mu.Unlock()
	return pr
}

// mergedPR returns the most recent completed PR from branch into base in this
// repo. A non-nil result means this branch's work was merged into base
// (squash·rebase·merge 무관 — PR 완료가 곧 머지).
func (a *adoClient) mergedPR(org, project, repo, branch, base string) *pullRequest {
	if branch == "" {
		return nil
	}
	key := org + "/" + project + "/" + repo + "/" + branch + "->" + base
	a.mu.Lock()
	if e, ok := a.mCache[key]; ok && time.Since(e.at) < adoTTL {
		a.mu.Unlock()
		return e.merged
	}
	a.mu.Unlock()

	var pr *pullRequest
	if id, ok := a.repoID(org, project, repo); ok {
		u := fmt.Sprintf("%s/git/repositories/%s/pullrequests?api-version=7.1&$top=1&searchCriteria.status=completed&searchCriteria.sourceRefName=%s&searchCriteria.targetRefName=%s",
			a.base(org, project), url.PathEscape(id), url.QueryEscape("refs/heads/"+branch), url.QueryEscape("refs/heads/"+base))
		var resp struct {
			Value []struct {
				ID    int    `json:"pullRequestId"`
				Title string `json:"title"`
			} `json:"value"`
		}
		if a.get(u, &resp) && len(resp.Value) > 0 {
			v := resp.Value[0]
			pr = &pullRequest{ID: v.ID, Title: v.Title, URL: a.prWebURL(org, project, repo, v.ID)}
		}
	}
	a.mu.Lock()
	a.mCache[key] = adoCacheEntry{at: time.Now(), merged: pr}
	a.mu.Unlock()
	return pr
}
