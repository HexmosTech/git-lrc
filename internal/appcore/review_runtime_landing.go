package appcore

import (
	"encoding/json"
	"fmt"
	"html"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

type githubRepoStats struct {
	Name        string `json:"name"`
	Description string `json:"description"`
	Stars       int    `json:"stargazers_count"`
	Forks       int    `json:"forks_count"`
	Watchers    int    `json:"subscribers_count"`
	OpenIssues  int    `json:"open_issues_count"`
	Language    string `json:"language"`
	HTMLURL     string `json:"html_url"`
}

func fetchGitHubStats() *githubRepoStats {
	client := &http.Client{Timeout: 3 * time.Second}
	resp, err := client.Get("https://api.github.com/repos/HexmosTech/git-lrc")
	if err != nil {
		return nil
	}
	defer resp.Body.Close()
	var stats githubRepoStats
	if json.NewDecoder(resp.Body).Decode(&stats) != nil {
		return nil
	}
	return &stats
}

type reviewRegistryEntry struct {
	ReviewID     string    `json:"review_id"`
	FriendlyName string    `json:"friendly_name"`
	Repository   string    `json:"repository"`
	Port         int       `json:"port"`
	PID          int       `json:"pid"`
	StartedAt    time.Time `json:"started_at"`
}

func reviewRegistryDir() string {
	return filepath.Join(os.TempDir(), ".lrc-reviews")
}

// registerActiveReview writes a registry entry to /tmp/.lrc-reviews/<port>.json so the
// listing page can discover live review processes across ports. Returns a cleanup func that
// removes the file on exit. On any write failure the review still works; listing just won't
// show this entry.
func registerActiveReview(port int, reviewID, friendlyName, repository string, startedAt time.Time) func() {
	dir := reviewRegistryDir()
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return func() {}
	}
	entry := reviewRegistryEntry{
		ReviewID:     reviewID,
		FriendlyName: friendlyName,
		Repository:   repository,
		Port:         port,
		PID:          os.Getpid(),
		StartedAt:    startedAt,
	}
	data, err := json.Marshal(entry)
	if err != nil {
		return func() {}
	}
	path := filepath.Join(dir, fmt.Sprintf("%d.json", port))
	if err := os.WriteFile(path, data, 0o600); err != nil {
		return func() {}
	}
	return func() { _ = os.Remove(path) }
}

// readActiveReviews scans /tmp/.lrc-reviews/ and returns entries for live processes.
// PID liveness is checked via kill(pid, 0) — Unix-only; stale files are cleaned up.
// All errors are treated as "skip this entry" — the listing is best-effort.
func readActiveReviews() []reviewRegistryEntry {
	dir := reviewRegistryDir()
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil
	}
	var result []reviewRegistryEntry
	for _, e := range entries {
		if e.IsDir() || filepath.Ext(e.Name()) != ".json" {
			continue
		}
		data, err := os.ReadFile(filepath.Join(dir, e.Name()))
		if err != nil {
			continue
		}
		var entry reviewRegistryEntry
		if json.Unmarshal(data, &entry) != nil {
			continue
		}
		if !isProcessAlive(entry.PID) {
			_ = os.Remove(filepath.Join(dir, e.Name()))
			continue
		}
		result = append(result, entry)
	}
	return result
}

type listingImpactStats struct {
	TotalReviews int `json:"total_reviews"`
	IssuesFound  int `json:"issues_found"`
	BugsCaught   int `json:"bugs_caught"`
	Critical     int `json:"critical"`
	Errors       int `json:"errors"`
	Warnings     int `json:"warnings"`
	Info         int `json:"info"`
}

func fetchListingImpactStats(apiURL, apiKey string) *listingImpactStats {
	client := &http.Client{Timeout: 3 * time.Second}
	req, err := http.NewRequest("GET", strings.TrimRight(apiURL, "/")+"/api/v1/feedback/impact-stats", nil)
	if err != nil {
		return nil
	}
	req.Header.Set("X-API-Key", apiKey)
	resp, err := client.Do(req)
	if err != nil {
		return nil
	}
	defer resp.Body.Close()
	var stats listingImpactStats
	if json.NewDecoder(resp.Body).Decode(&stats) != nil {
		return nil
	}
	return &stats
}

// serveReviewListing renders the VS Code-styled active-reviews page at GET /.
// It is called from two mutually exclusive code paths:
//   - progressive review path (inside runReviewWithOptions HTTP server)
//   - non-progressive path (inside serveHTMLInteractive HTTP server)
//
// Both paths have already called registerActiveReview for their own process, so there is
// no double-registration: these code paths never run in the same process simultaneously.
func serveReviewListing(w http.ResponseWriter, cfg Config) {
	// Fetch GitHub stats and impact stats concurrently to cap latency at 3s, not 6s.
	var ghStats *githubRepoStats
	var impact *listingImpactStats
	var wg sync.WaitGroup
	wg.Add(2)
	go func() { defer wg.Done(); ghStats = fetchGitHubStats() }()
	go func() { defer wg.Done(); impact = fetchListingImpactStats(cfg.APIURL, cfg.APIKey) }()
	wg.Wait()

	reviews := readActiveReviews()

	iv := func(n int) string { return fmt.Sprintf("%d", n) }
	totalReviews, issuesFound, bugsCaught, critical, errsCount, warnings := "—", "—", "—", "—", "—", "—"
	if impact != nil {
		totalReviews = iv(impact.TotalReviews)
		issuesFound = iv(impact.IssuesFound)
		bugsCaught = iv(impact.BugsCaught)
		critical = iv(impact.Critical)
		errsCount = iv(impact.Errors)
		warnings = iv(impact.Warnings)
	}

	shareText := fmt.Sprintf(`🚀 Shipping with confidence — here's my code review impact since Jan 2025:

✅ %s reviews completed
🐛 %s bugs caught before production
🔍 %s total issues found
🔴 %s critical issues found
🟠 %s errors caught
🟡 %s warnings flagged

Using git-lrc to AI-review every commit before it lands.

⭐ Star it if you find it useful: https://github.com/HexmosTech/git-lrc

#CodeReview #DevOps #SoftwareEngineering #AI`, totalReviews, bugsCaught, issuesFound, critical, errsCount, warnings)

	statsRows := fmt.Sprintf(`
<div class="stat-row"><span class="stat-label">Reviews completed</span><span class="stat-val">%s</span></div>
<div class="stat-row"><span class="stat-label">Issues found</span><span class="stat-val">%s</span></div>
<div class="stat-row"><span class="stat-label">Bugs caught pre-prod</span><span class="stat-val">%s</span></div>
<div class="stat-row"><span class="stat-label">Critical</span><span class="stat-val">%s</span></div>
<div class="stat-row"><span class="stat-label">Errors</span><span class="stat-val">%s</span></div>
<div class="stat-row"><span class="stat-label">Warnings</span><span class="stat-val">%s</span></div>`,
		totalReviews, issuesFound, bugsCaught, critical, errsCount, warnings)

	tableRows := ""
	if len(reviews) == 0 {
		tableRows = `<tr><td colspan="5" style="text-align:center;padding:32px 0;color:#6a6a6a;font-style:italic;">No active reviews</td></tr>`
	} else {
		for _, rv := range reviews {
			name := html.EscapeString(rv.FriendlyName)
			if name == "" {
				name = "Review #" + html.EscapeString(rv.ReviewID)
			}
			repo := html.EscapeString(rv.Repository)
			if repo == "" {
				repo = "—"
			}
			elapsed := "—"
			if !rv.StartedAt.IsZero() {
				d := time.Since(rv.StartedAt)
				if d < time.Minute {
					elapsed = fmt.Sprintf("%.0fs ago", d.Seconds())
				} else {
					elapsed = fmt.Sprintf("%.0fm ago", d.Minutes())
				}
			}
			href := fmt.Sprintf("http://localhost:%d/?r=%s", rv.Port, url.QueryEscape(rv.ReviewID))
			// html.EscapeString is applied to href in both the onclick JS string and the href
			// attribute so that any character that could break out of the attribute is neutralised.
			safeHref := html.EscapeString(href)
			tableRows += fmt.Sprintf(`
<tr class="trow" onclick="location.href='%s'">
  <td class="td-name">
    <svg class="td-icon" width="13" height="13" fill="none" viewBox="0 0 24 24" stroke="#569cd6" stroke-width="1.8"><path stroke-linecap="round" stroke-linejoin="round" d="M9 5H7a2 2 0 00-2 2v12a2 2 0 002 2h10a2 2 0 002-2V7a2 2 0 00-2-2h-2M9 5a2 2 0 002 2h2a2 2 0 002-2M9 5a2 2 0 012-2h2a2 2 0 012 2"/></svg>
    <a href="%s" class="name-link">%s</a>
  </td>
  <td class="td-repo">%s</td>
  <td class="td-id">%s</td>
  <td class="td-port">%d</td>
  <td class="td-started">%s</td>
</tr>`, safeHref, safeHref, name, repo, html.EscapeString(rv.ReviewID), rv.Port, elapsed)
		}
	}

	const ghFallbackPanel = `
<div class="gh-name">
  <svg width="14" height="14" fill="currentColor" viewBox="0 0 24 24" style="flex-shrink:0;color:#cccccc;"><path d="M12 .297c-6.63 0-12 5.373-12 12 0 5.303 3.438 9.8 8.205 11.385.6.113.82-.258.82-.577 0-.285-.01-1.04-.015-2.04-3.338.724-4.042-1.61-4.042-1.61C4.422 18.07 3.633 17.7 3.633 17.7c-1.087-.744.084-.729.084-.729 1.205.084 1.838 1.236 1.838 1.236 1.07 1.835 2.809 1.305 3.495.998.108-.776.417-1.305.76-1.605-2.665-.3-5.466-1.332-5.466-5.93 0-1.31.465-2.38 1.235-3.22-.135-.303-.54-1.523.105-3.176 0 0 1.005-.322 3.3 1.23.96-.267 1.98-.399 3-.405 1.02.006 2.04.138 3 .405 2.28-1.552 3.285-1.23 3.285-1.23.645 1.653.24 2.873.12 3.176.765.84 1.23 1.91 1.23 3.22 0 4.61-2.805 5.625-5.475 5.92.42.36.81 1.096.81 2.22 0 1.606-.015 2.896-.015 3.286 0 .315.21.69.825.57C20.565 22.092 24 17.592 24 12.297c0-6.627-5.373-12-12-12"/></svg>
  <a href="https://github.com/HexmosTech/git-lrc" target="_blank" class="gh-link">git-lrc</a>
</div>
<p class="gh-desc">AI-powered code review in your git commit flow.</p>
<div class="gh-stats">
  <div class="gh-stat"><span class="gh-stat-icon">★</span><span class="gh-stat-val">1048+</span><span class="gh-stat-lbl">stars</span></div>
  <div class="gh-stat"><span class="gh-stat-icon">⑂</span><span class="gh-stat-val">157+</span><span class="gh-stat-lbl">forks</span></div>
  <div class="gh-stat"><span class="gh-stat-icon">◎</span><span class="gh-stat-val">20+</span><span class="gh-stat-lbl">issues</span></div>
</div>
<div class="gh-lang"><span class="lang-dot"></span>Go</div>
<a href="https://github.com/HexmosTech/git-lrc" target="_blank" class="star-btn">
  <svg width="13" height="13" fill="none" viewBox="0 0 24 24" stroke="currentColor" stroke-width="2"><polygon points="12 2 15.09 8.26 22 9.27 17 14.14 18.18 21.02 12 17.77 5.82 21.02 7 14.14 2 9.27 8.91 8.26 12 2"/></svg>
  Star on GitHub
</a>`

	ghPanel := ghFallbackPanel
	if ghStats != nil {
		desc := ghStats.Description
		if desc == "" {
			desc = "AI-powered code review in your git commit flow."
		}
		ghPanel = fmt.Sprintf(`
<div class="gh-name">
  <svg width="14" height="14" fill="currentColor" viewBox="0 0 24 24" style="flex-shrink:0;color:#cccccc;"><path d="M12 .297c-6.63 0-12 5.373-12 12 0 5.303 3.438 9.8 8.205 11.385.6.113.82-.258.82-.577 0-.285-.01-1.04-.015-2.04-3.338.724-4.042-1.61-4.042-1.61C4.422 18.07 3.633 17.7 3.633 17.7c-1.087-.744.084-.729.084-.729 1.205.084 1.838 1.236 1.838 1.236 1.07 1.835 2.809 1.305 3.495.998.108-.776.417-1.305.76-1.605-2.665-.3-5.466-1.332-5.466-5.93 0-1.31.465-2.38 1.235-3.22-.135-.303-.54-1.523.105-3.176 0 0 1.005-.322 3.3 1.23.96-.267 1.98-.399 3-.405 1.02.006 2.04.138 3 .405 2.28-1.552 3.285-1.23 3.285-1.23.645 1.653.24 2.873.12 3.176.765.84 1.23 1.91 1.23 3.22 0 4.61-2.805 5.625-5.475 5.92.42.36.81 1.096.81 2.22 0 1.606-.015 2.896-.015 3.286 0 .315.21.69.825.57C20.565 22.092 24 17.592 24 12.297c0-6.627-5.373-12-12-12"/></svg>
  <a href="%s" target="_blank" class="gh-link">%s</a>
</div>
<p class="gh-desc">%s</p>
<div class="gh-stats">
  <div class="gh-stat"><span class="gh-stat-icon">★</span><span class="gh-stat-val">%d</span><span class="gh-stat-lbl">stars</span></div>
  <div class="gh-stat"><span class="gh-stat-icon">⑂</span><span class="gh-stat-val">%d</span><span class="gh-stat-lbl">forks</span></div>
  <div class="gh-stat"><span class="gh-stat-icon">◎</span><span class="gh-stat-val">%d</span><span class="gh-stat-lbl">issues</span></div>
</div>
<div class="gh-lang"><span class="lang-dot"></span>%s</div>
<a href="%s" target="_blank" class="star-btn">
  <svg width="13" height="13" fill="none" viewBox="0 0 24 24" stroke="currentColor" stroke-width="2"><polygon points="12 2 15.09 8.26 22 9.27 17 14.14 18.18 21.02 12 17.77 5.82 21.02 7 14.14 2 9.27 8.91 8.26 12 2"/></svg>
  Star on GitHub
</a>`,
			html.EscapeString(ghStats.HTMLURL), html.EscapeString(ghStats.Name),
			html.EscapeString(desc),
			ghStats.Stars, ghStats.Forks, ghStats.OpenIssues,
			html.EscapeString(ghStats.Language),
			html.EscapeString(ghStats.HTMLURL))
	}

	page := fmt.Sprintf(`<!DOCTYPE html>
<html lang="en">
<head>
<meta charset="UTF-8">
<meta name="viewport" content="width=device-width,initial-scale=1">
<title>LiveReview — Active Reviews</title>
<style>
*,*::before,*::after{box-sizing:border-box;margin:0;padding:0}
body{background:#1e1e1e;color:#cccccc;font-family:"Segoe UI",system-ui,-apple-system,sans-serif;font-size:13px;min-height:100vh;}
.titlebar{background:#323233;border-bottom:1px solid #111;height:35px;display:flex;align-items:center;padding:0 14px;gap:8px;user-select:none;position:fixed;top:0;left:0;right:0;z-index:10;}
.titlebar-dot{color:#555;}.titlebar-text{color:#cccccc;font-size:12px;}
.activity{position:fixed;top:35px;left:0;bottom:22px;width:48px;background:#333333;border-right:1px solid #252526;display:flex;flex-direction:column;align-items:center;padding-top:6px;gap:2px;z-index:9;}
.act-btn{width:40px;height:40px;display:flex;align-items:center;justify-content:center;color:#858585;cursor:pointer;border:none;background:none;border-radius:4px;}
.act-btn:hover{color:#cccccc;}
.act-btn.open{color:#cccccc;box-shadow:inset 2px 0 0 #569cd6;}
.side-panel{position:fixed;top:35px;left:48px;bottom:22px;width:280px;background:#252526;border-right:1px solid #1a1a1a;display:none;flex-direction:column;z-index:8;overflow:hidden;}
.side-panel.visible{display:flex;}
.panel-hdr{padding:9px 12px 8px;font-size:11px;font-weight:700;letter-spacing:.08em;text-transform:uppercase;color:#bbbbbb;border-bottom:1px solid #1a1a1a;flex-shrink:0;}
.panel-body{padding:12px;flex:1;overflow-y:auto;display:flex;flex-direction:column;gap:10px;}
.stat-row{display:flex;justify-content:space-between;align-items:baseline;padding:4px 0;border-bottom:1px solid #2d2d2d;}
.stat-row:last-child{border-bottom:none;}
.stat-label{color:#9a9a9a;font-size:12px;}.stat-val{color:#9cdcfe;font-size:12px;font-family:monospace;font-weight:600;}
.share-textarea{width:100%%;height:200px;background:#1e1e1e;border:1px solid #3c3c3c;border-radius:3px;color:#cccccc;font-size:11px;font-family:"Segoe UI",system-ui,sans-serif;line-height:1.6;padding:10px;resize:vertical;outline:none;}
.share-textarea:focus{border-color:#569cd6;}
.copy-btn{width:100%%;padding:6px 0;background:#0e639c;border:none;border-radius:3px;color:#fff;font-size:12px;cursor:pointer;display:flex;align-items:center;justify-content:center;gap:6px;transition:background .15s;flex-shrink:0;}
.copy-btn:hover{background:#1177bb;}.copy-btn.copied{background:#16825d;}.copy-btn.failed{background:#c62828;}
.gh-name{display:flex;align-items:center;gap:7px;}
.gh-link{color:#9cdcfe;text-decoration:none;font-size:13px;font-weight:600;}.gh-link:hover{text-decoration:underline;}
.gh-desc{color:#9a9a9a;font-size:11px;line-height:1.5;}
.gh-stats{display:flex;gap:14px;}
.gh-stat{display:flex;align-items:center;gap:4px;}
.gh-stat-icon{font-size:13px;color:#e3b341;}
.gh-stat-val{color:#cccccc;font-size:12px;font-weight:600;}
.gh-stat-lbl{color:#6a6a6a;font-size:11px;}
.gh-lang{display:flex;align-items:center;gap:5px;color:#9a9a9a;font-size:11px;}
.lang-dot{width:10px;height:10px;border-radius:50%%;background:#00add8;flex-shrink:0;}
.star-btn{display:flex;align-items:center;justify-content:center;gap:6px;padding:7px 0;background:#238636;border:none;border-radius:3px;color:#fff;font-size:12px;cursor:pointer;text-decoration:none;transition:background .15s;width:100%%;}
.star-btn:hover{background:#2ea043;}
.gh-cta{color:#9a9a9a;font-size:12px;line-height:1.6;}
.gh-cta a{color:#569cd6;text-decoration:none;}.gh-cta a:hover{text-decoration:underline;}
.main{margin-left:48px;margin-top:35px;padding:24px 28px;overflow-y:auto;height:calc(100vh - 57px);transition:margin-left .15s;}
.main.shifted{margin-left:328px;}
.sec-hdr{font-size:11px;font-weight:700;letter-spacing:.08em;text-transform:uppercase;color:#bbbbbb;margin-bottom:12px;display:flex;align-items:center;gap:7px;}
.badge{background:#0e639c;color:#fff;font-size:10px;font-weight:700;padding:1px 7px;border-radius:10px;letter-spacing:0;text-transform:none;}
table{width:100%%;border-collapse:collapse;}
thead tr{border-bottom:1px solid #3c3c3c;}
th{text-align:left;font-size:11px;font-weight:600;letter-spacing:.05em;text-transform:uppercase;color:#858585;padding:0 12px 8px;}
th:first-child{padding-left:4px;}
.trow{cursor:pointer;border-bottom:1px solid #2a2a2a;transition:background .1s;}
.trow:hover{background:#2a2d2e;}.trow:last-child{border-bottom:none;}
td{padding:10px 12px;vertical-align:middle;}td:first-child{padding-left:4px;}
.td-name{display:flex;align-items:center;gap:7px;white-space:nowrap;}
.name-link{color:#9cdcfe;text-decoration:none;font-size:13px;}.name-link:hover{text-decoration:underline;}
.td-repo{color:#ce9178;font-size:12px;white-space:nowrap;}
.td-id,.td-port,.td-started{color:#9a9a9a;font-size:12px;white-space:nowrap;}
.statusbar{position:fixed;bottom:0;left:0;right:0;height:22px;background:#007acc;display:flex;align-items:center;padding:0 10px;gap:14px;font-size:11px;color:#fff;}
</style>
</head>
<body>
<div class="titlebar">
  <svg width="13" height="13" fill="none" viewBox="0 0 24 24" stroke="#569cd6" stroke-width="1.8"><path stroke-linecap="round" stroke-linejoin="round" d="M9 5H7a2 2 0 00-2 2v12a2 2 0 002 2h10a2 2 0 002-2V7a2 2 0 00-2-2h-2M9 5a2 2 0 002 2h2a2 2 0 002-2M9 5a2 2 0 012-2h2a2 2 0 012 2"/></svg>
  <span class="titlebar-text">LiveReview</span>
  <span class="titlebar-dot">—</span>
  <span class="titlebar-text">Active Reviews</span>
</div>
<div class="activity">
  <button class="act-btn" id="statsBtn" title="Impact stats" onclick="toggle('stats')">
    <svg width="17" height="17" fill="none" viewBox="0 0 24 24" stroke="currentColor" stroke-width="1.8"><line x1="18" y1="20" x2="18" y2="10"/><line x1="12" y1="20" x2="12" y2="4"/><line x1="6" y1="20" x2="6" y2="14"/></svg>
  </button>
  <button class="act-btn" id="shareBtn" title="Share impact" onclick="toggle('share')">
    <svg width="17" height="17" fill="none" viewBox="0 0 24 24" stroke="currentColor" stroke-width="1.8"><circle cx="18" cy="5" r="3"/><circle cx="6" cy="12" r="3"/><circle cx="18" cy="19" r="3"/><line x1="8.59" y1="13.51" x2="15.42" y2="17.49"/><line x1="15.41" y1="6.51" x2="8.59" y2="10.49"/></svg>
  </button>
  <button class="act-btn" id="infoBtn" title="About git-lrc" onclick="toggle('info')">
    <svg width="17" height="17" fill="none" viewBox="0 0 24 24" stroke="currentColor" stroke-width="1.8"><circle cx="12" cy="12" r="10"/><line x1="12" y1="8" x2="12" y2="8" stroke-linecap="round" stroke-width="2.5"/><line x1="12" y1="12" x2="12" y2="16" stroke-linecap="round"/></svg>
  </button>
</div>

<div class="side-panel" id="statsPanel">
  <div class="panel-hdr">Impact Stats</div>
  <div class="panel-body">%s</div>
</div>

<div class="side-panel" id="sharePanel">
  <div class="panel-hdr">Share Impact</div>
  <div class="panel-body">
    <textarea class="share-textarea" id="shareText">%s</textarea>
    <button class="copy-btn" id="copyBtn" onclick="copyShare()">
      <svg width="13" height="13" fill="none" viewBox="0 0 24 24" stroke="currentColor" stroke-width="2"><rect x="9" y="9" width="13" height="13" rx="2"/><path d="M5 15H4a2 2 0 01-2-2V4a2 2 0 012-2h9a2 2 0 012 2v1"/></svg>
      Copy message
    </button>
  </div>
</div>

<div class="side-panel" id="infoPanel">
  <div class="panel-hdr">About git-lrc</div>
  <div class="panel-body">
    <p class="gh-cta">Is it being useful? If you haven't starred, <a href="https://github.com/HexmosTech/git-lrc" target="_blank">star us on GitHub</a> — it really helps!</p>
    %s
  </div>
</div>

<div class="main" id="main">
  <div class="sec-hdr">Active <span class="badge">%d</span></div>
  <table>
    <thead><tr><th>Name</th><th>Repository</th><th>ID</th><th>Port</th><th>Started</th></tr></thead>
    <tbody>%s</tbody>
  </table>
</div>
<div class="statusbar"><a href="https://hexmos.com/livereview/" target="_blank" style="color:#fff;text-decoration:none;">LiveReview</a><span>git-lrc</span></div>
<script>
const COPY_SVG = '<svg width="13" height="13" fill="none" viewBox="0 0 24 24" stroke="currentColor" stroke-width="2"><rect x="9" y="9" width="13" height="13" rx="2"/><path d="M5 15H4a2 2 0 01-2-2V4a2 2 0 012-2h9a2 2 0 012 2v1"/></svg> Copy message';
let openPanel = null;
const PANELS = ['stats','share','info'];
function toggle(which) {
  const main = document.getElementById('main');
  if (openPanel === which) {
    document.getElementById(which+'Panel').classList.remove('visible');
    document.getElementById(which+'Btn').classList.remove('open');
    main.classList.remove('shifted');
    openPanel = null;
    return;
  }
  PANELS.forEach(p => {
    document.getElementById(p+'Panel').classList.remove('visible');
    document.getElementById(p+'Btn').classList.remove('open');
  });
  document.getElementById(which+'Panel').classList.add('visible');
  document.getElementById(which+'Btn').classList.add('open');
  main.classList.add('shifted');
  openPanel = which;
}
function copyShare() {
  const btn = document.getElementById('copyBtn');
  const text = document.getElementById('shareText').value;
  navigator.clipboard.writeText(text).then(() => {
    btn.classList.add('copied');
    btn.innerHTML = '<svg width="13" height="13" fill="none" viewBox="0 0 24 24" stroke="currentColor" stroke-width="2"><polyline points="20 6 9 17 4 12"/></svg> Copied!';
    setTimeout(() => {
      btn.classList.remove('copied');
      btn.innerHTML = COPY_SVG;
    }, 2000);
  }).catch(() => {
    btn.classList.add('failed');
    btn.textContent = 'Copy failed — select text manually';
    setTimeout(() => {
      btn.classList.remove('failed');
      btn.innerHTML = COPY_SVG;
    }, 2500);
  });
}
</script>
</body>
</html>`, statsRows, shareText, ghPanel, len(reviews), tableRows)

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	fmt.Fprint(w, page)
}
