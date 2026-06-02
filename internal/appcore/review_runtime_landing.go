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

	// Nexus-style metric cards for the dashboard hero, built from the same
	// impact figures shown in the side panel.
	statCards := fmt.Sprintf(`
<div class="stat-cards">
  <div class="stat-card sc-blue">
    <div class="stat-card-top"><span class="stat-card-icon"><svg width="18" height="18" fill="none" viewBox="0 0 24 24" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><path d="M9 11l3 3L22 4"/><path d="M21 12v7a2 2 0 0 1-2 2H5a2 2 0 0 1-2-2V5a2 2 0 0 1 2-2h11"/></svg></span><span class="stat-card-label">Reviews completed</span></div>
    <div class="stat-card-value">%s</div>
  </div>
  <div class="stat-card sc-amber">
    <div class="stat-card-top"><span class="stat-card-icon"><svg width="18" height="18" fill="none" viewBox="0 0 24 24" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><circle cx="11" cy="11" r="8"/><line x1="21" y1="21" x2="16.65" y2="16.65"/></svg></span><span class="stat-card-label">Issues found</span></div>
    <div class="stat-card-value">%s</div>
  </div>
  <div class="stat-card sc-green">
    <div class="stat-card-top"><span class="stat-card-icon"><svg width="18" height="18" fill="none" viewBox="0 0 24 24" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><path d="M12 22s8-4 8-10V5l-8-3-8 3v7c0 6 8 10 8 10z"/><path d="M9 12l2 2 4-4"/></svg></span><span class="stat-card-label">Bugs caught pre-prod</span></div>
    <div class="stat-card-value">%s</div>
  </div>
  <div class="stat-card sc-red">
    <div class="stat-card-top"><span class="stat-card-icon"><svg width="18" height="18" fill="none" viewBox="0 0 24 24" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><polygon points="7.86 2 16.14 2 22 7.86 22 16.14 16.14 22 7.86 22 2 16.14 2 7.86 7.86 2"/><line x1="12" y1="8" x2="12" y2="12"/><line x1="12" y1="16" x2="12" y2="16"/></svg></span><span class="stat-card-label">Critical issues</span></div>
    <div class="stat-card-value">%s</div>
  </div>
</div>`, totalReviews, issuesFound, bugsCaught, critical)

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
  <div class="gh-stat"><span class="gh-stat-icon"><svg width="13" height="13" viewBox="0 0 24 24" fill="currentColor"><polygon points="12 2 15.09 8.26 22 9.27 17 14.14 18.18 21.02 12 17.77 5.82 21.02 7 14.14 2 9.27 8.91 8.26 12 2"/></svg></span><span class="gh-stat-val">1048+</span><span class="gh-stat-lbl">stars</span></div>
  <div class="gh-stat"><span class="gh-stat-icon"><svg width="13" height="13" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><circle cx="6" cy="3" r="1.8"/><circle cx="6" cy="15" r="1.8"/><circle cx="18" cy="6" r="1.8"/><path d="M18 7.8v.7a3 3 0 0 1-3 3H9a3 3 0 0 0-3 3v.7"/><line x1="6" y1="4.8" x2="6" y2="13.2"/></svg></span><span class="gh-stat-val">157+</span><span class="gh-stat-lbl">forks</span></div>
  <div class="gh-stat"><span class="gh-stat-icon"><svg width="13" height="13" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><circle cx="12" cy="12" r="9"/><line x1="12" y1="8" x2="12" y2="12"/><line x1="12" y1="16" x2="12.01" y2="16"/></svg></span><span class="gh-stat-val">20+</span><span class="gh-stat-lbl">issues</span></div>
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
  <div class="gh-stat"><span class="gh-stat-icon"><svg width="13" height="13" viewBox="0 0 24 24" fill="currentColor"><polygon points="12 2 15.09 8.26 22 9.27 17 14.14 18.18 21.02 12 17.77 5.82 21.02 7 14.14 2 9.27 8.91 8.26 12 2"/></svg></span><span class="gh-stat-val">%d</span><span class="gh-stat-lbl">stars</span></div>
  <div class="gh-stat"><span class="gh-stat-icon"><svg width="13" height="13" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><circle cx="6" cy="3" r="1.8"/><circle cx="6" cy="15" r="1.8"/><circle cx="18" cy="6" r="1.8"/><path d="M18 7.8v.7a3 3 0 0 1-3 3H9a3 3 0 0 0-3 3v.7"/><line x1="6" y1="4.8" x2="6" y2="13.2"/></svg></span><span class="gh-stat-val">%d</span><span class="gh-stat-lbl">forks</span></div>
  <div class="gh-stat"><span class="gh-stat-icon"><svg width="13" height="13" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><circle cx="12" cy="12" r="9"/><line x1="12" y1="8" x2="12" y2="12"/><line x1="12" y1="16" x2="12.01" y2="16"/></svg></span><span class="gh-stat-val">%d</span><span class="gh-stat-lbl">issues</span></div>
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
<script>(function(){try{var t=localStorage.getItem('lrc-theme');if(t!=='light'&&t!=='dark'){t=(window.matchMedia&&window.matchMedia('(prefers-color-scheme: dark)').matches)?'dark':'light';}document.documentElement.setAttribute('data-theme',t);}catch(e){document.documentElement.setAttribute('data-theme','light');}})();</script>
<link rel="stylesheet" href="/static/styles.css">
<script type="module" src="/static/theme.js"></script>
<style>
/* git-lrc Active Reviews landing — themed via the shared design system
   in styles.css (:root = dark, [data-theme=light] = light). The floating
   sun/moon toggle is mounted by theme.js. */
*,*::before,*::after{box-sizing:border-box;margin:0;padding:0}
body{background:var(--ambient,var(--bg-primary));color:var(--text-secondary);font-family:var(--font-sans,-apple-system,BlinkMacSystemFont,"Segoe UI",system-ui,sans-serif);font-size:13px;min-height:100vh;-webkit-font-smoothing:antialiased;}
.titlebar{background:var(--surface-glass);backdrop-filter:blur(12px) saturate(140%%);-webkit-backdrop-filter:blur(12px) saturate(140%%);border-bottom:1px solid var(--border-subtle);height:46px;display:flex;align-items:center;padding:0 18px;gap:8px;user-select:none;position:fixed;top:0;left:0;right:0;z-index:10;box-shadow:var(--shadow-sm);}
.titlebar-dot{color:var(--text-dim);}.titlebar-text{color:var(--text-secondary);font-size:13px;font-weight:600;}
.activity{position:fixed;top:46px;left:0;bottom:26px;width:54px;background:var(--bg-secondary);border-right:1px solid var(--border-subtle);display:flex;flex-direction:column;align-items:center;padding-top:10px;gap:4px;z-index:9;}
.act-btn{width:42px;height:42px;display:flex;align-items:center;justify-content:center;color:var(--text-muted);cursor:pointer;border:none;background:none;border-radius:var(--radius-sm);transition:all .15s;}
.act-btn:hover{color:var(--text-primary);background:var(--bg-hover);}
.act-btn.open{color:var(--accent-blue);background:rgba(10,111,209,0.12);box-shadow:inset 2px 0 0 var(--accent-blue);}
.side-panel{position:fixed;top:46px;left:54px;bottom:26px;width:300px;background:var(--bg-secondary);border-right:1px solid var(--border-subtle);display:none;flex-direction:column;z-index:8;overflow:hidden;box-shadow:var(--shadow-md);}
.side-panel.visible{display:flex;}
.panel-hdr{padding:12px 14px 12px 16px;font-size:11px;font-weight:700;letter-spacing:.08em;text-transform:uppercase;color:var(--text-muted);border-bottom:1px solid var(--border-subtle);flex-shrink:0;display:flex;align-items:center;justify-content:space-between;}
.panel-collapse{display:inline-flex;align-items:center;justify-content:center;width:26px;height:26px;border-radius:var(--radius-sm);border:1px solid var(--border-subtle);background:var(--bg-tertiary);color:var(--text-muted);cursor:pointer;transition:all .15s;}
.panel-collapse:hover{color:var(--text-primary);border-color:var(--accent-blue);background:var(--bg-hover);}
.panel-body{padding:14px;flex:1;overflow-y:auto;display:flex;flex-direction:column;gap:10px;}
.stat-row{display:flex;justify-content:space-between;align-items:baseline;padding:7px 0;border-bottom:1px solid var(--border-subtle);}
.stat-row:last-child{border-bottom:none;}
.stat-label{color:var(--text-muted);font-size:12px;}.stat-val{color:var(--accent-blue-light);font-size:13px;font-family:ui-monospace,SFMono-Regular,monospace;font-weight:700;}
.share-textarea{width:100%%;height:200px;background:var(--bg-tertiary);border:1px solid var(--border-medium);border-radius:var(--radius-sm);color:var(--text-secondary);font-size:11px;line-height:1.6;padding:10px;resize:vertical;outline:none;}
.share-textarea:focus{border-color:var(--accent-blue);box-shadow:0 0 0 3px rgba(10,111,209,0.18);}
.copy-btn{width:100%%;padding:9px 0;background:var(--accent-gradient);border:none;border-radius:var(--radius-sm);color:#fff;font-size:12px;font-weight:600;cursor:pointer;display:flex;align-items:center;justify-content:center;gap:6px;transition:filter .15s;flex-shrink:0;box-shadow:var(--shadow-sm);}
.copy-btn:hover{filter:brightness(1.06);}.copy-btn.copied{background:var(--accent-green);}.copy-btn.failed{background:var(--accent-red);}
.gh-name{display:flex;align-items:center;gap:7px;}
.gh-name svg{color:var(--text-primary)!important;}
.gh-link{color:var(--accent-blue-light);text-decoration:none;font-size:14px;font-weight:700;}.gh-link:hover{text-decoration:underline;}
.gh-desc{color:var(--text-muted);font-size:12px;line-height:1.5;}
.gh-stats{display:flex;gap:16px;}
.gh-stat{display:flex;align-items:center;gap:4px;}
.gh-stat-icon{font-size:13px;color:var(--accent-yellow);}
.gh-stat-val{color:var(--text-primary);font-size:13px;font-weight:700;}
.gh-stat-lbl{color:var(--text-dim);font-size:11px;}
.gh-lang{display:flex;align-items:center;gap:5px;color:var(--text-muted);font-size:11px;}
.lang-dot{width:10px;height:10px;border-radius:50%%;background:#00add8;flex-shrink:0;}
.star-btn{display:flex;align-items:center;justify-content:center;gap:6px;padding:9px 0;background:var(--accent-green);border:none;border-radius:var(--radius-sm);color:#fff;font-size:12px;font-weight:600;cursor:pointer;text-decoration:none;transition:filter .15s;width:100%%;box-shadow:var(--shadow-sm);}
.star-btn:hover{filter:brightness(1.08);}
.gh-cta{color:var(--text-muted);font-size:12px;line-height:1.6;}
.gh-cta a{color:var(--accent-blue-light);text-decoration:none;}.gh-cta a:hover{text-decoration:underline;}
.main{margin-left:54px;margin-top:46px;padding:28px 32px;overflow-y:auto;height:calc(100vh - 72px);transition:margin-left .15s;}
.main.shifted{margin-left:354px;}
.page-hero{margin-bottom:22px;}
.page-hero h1{font-size:23px;font-weight:800;letter-spacing:-0.5px;color:var(--text-primary);margin-bottom:4px;}
.page-hero p{color:var(--text-muted);font-size:13px;}
.stat-cards{display:grid;grid-template-columns:repeat(auto-fit,minmax(200px,1fr));gap:14px;margin-bottom:28px;}
.stat-card{background:var(--surface-card);border:1px solid var(--border-subtle);border-radius:var(--radius-lg);padding:18px;box-shadow:var(--shadow-md),var(--ring-inset-top);transition:transform .2s,box-shadow .2s;}
.stat-card:hover{transform:translateY(-2px);box-shadow:var(--shadow-lg);}
.stat-card-top{display:flex;align-items:center;gap:9px;margin-bottom:13px;}
.stat-card-icon{width:36px;height:36px;border-radius:var(--radius-sm);display:inline-flex;align-items:center;justify-content:center;flex-shrink:0;}
.stat-card-label{font-size:12px;font-weight:600;color:var(--text-muted);}
.stat-card-value{font-size:30px;font-weight:800;letter-spacing:-0.6px;color:var(--text-primary);font-variant-numeric:tabular-nums;line-height:1;}
.sc-blue .stat-card-icon{background:rgba(10,111,209,0.14);color:var(--accent-blue);}
.sc-green .stat-card-icon{background:rgba(34,197,94,0.16);color:var(--accent-green);}
.sc-red .stat-card-icon{background:rgba(239,68,68,0.15);color:var(--accent-red);}
.sc-amber .stat-card-icon{background:rgba(245,158,11,0.18);color:var(--accent-yellow);}
.sec-hdr{font-size:12px;font-weight:700;letter-spacing:.06em;text-transform:uppercase;color:var(--text-muted);margin-bottom:14px;display:flex;align-items:center;gap:8px;}
.badge{background:var(--accent-blue);color:#fff;font-size:10px;font-weight:700;padding:2px 8px;border-radius:var(--radius-pill);letter-spacing:0;text-transform:none;}
table{width:100%%;border-collapse:separate;border-spacing:0 8px;}
th{text-align:left;font-size:11px;font-weight:600;letter-spacing:.05em;text-transform:uppercase;color:var(--text-dim);padding:0 14px 4px;}
.trow{cursor:pointer;transition:transform .12s,box-shadow .15s;}
.trow td{background:var(--surface-card);border-top:1px solid var(--border-subtle);border-bottom:1px solid var(--border-subtle);box-shadow:var(--shadow-xs);}
.trow td:first-child{border-left:1px solid var(--border-subtle);border-top-left-radius:var(--radius-md);border-bottom-left-radius:var(--radius-md);}
.trow td:last-child{border-right:1px solid var(--border-subtle);border-top-right-radius:var(--radius-md);border-bottom-right-radius:var(--radius-md);}
.trow:hover td{border-color:var(--accent-blue);}
td{padding:13px 14px;vertical-align:middle;}
.td-name{display:flex;align-items:center;gap:8px;white-space:nowrap;}
.td-name svg{color:var(--accent-blue)!important;}
.name-link{color:var(--text-primary);text-decoration:none;font-size:13px;font-weight:600;}.name-link:hover{color:var(--accent-blue-light);}
.td-repo{color:var(--accent-teal,#0e7c66);font-size:12px;white-space:nowrap;font-family:ui-monospace,monospace;}
.td-id,.td-port,.td-started{color:var(--text-muted);font-size:12px;white-space:nowrap;}
.statusbar{position:fixed;bottom:0;left:0;right:0;height:26px;background:var(--accent-gradient);display:flex;align-items:center;padding:0 12px;gap:14px;font-size:11px;color:#fff;}
.lrc-theme-toggle{left:66px!important;bottom:36px!important;}
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
  <button class="act-btn open" id="statsBtn" title="Impact stats" onclick="toggle('stats')">
    <svg width="17" height="17" fill="none" viewBox="0 0 24 24" stroke="currentColor" stroke-width="1.8"><line x1="18" y1="20" x2="18" y2="10"/><line x1="12" y1="20" x2="12" y2="4"/><line x1="6" y1="20" x2="6" y2="14"/></svg>
  </button>
  <button class="act-btn" id="shareBtn" title="Share impact" onclick="toggle('share')">
    <svg width="17" height="17" fill="none" viewBox="0 0 24 24" stroke="currentColor" stroke-width="1.8"><circle cx="18" cy="5" r="3"/><circle cx="6" cy="12" r="3"/><circle cx="18" cy="19" r="3"/><line x1="8.59" y1="13.51" x2="15.42" y2="17.49"/><line x1="15.41" y1="6.51" x2="8.59" y2="10.49"/></svg>
  </button>
  <button class="act-btn" id="infoBtn" title="About git-lrc" onclick="toggle('info')">
    <svg width="17" height="17" fill="none" viewBox="0 0 24 24" stroke="currentColor" stroke-width="1.8"><circle cx="12" cy="12" r="10"/><line x1="12" y1="8" x2="12" y2="8" stroke-linecap="round" stroke-width="2.5"/><line x1="12" y1="12" x2="12" y2="16" stroke-linecap="round"/></svg>
  </button>
</div>

<div class="side-panel visible" id="statsPanel">
  <div class="panel-hdr">Impact Stats<button class="panel-collapse" onclick="toggle('stats')" title="Collapse panel" aria-label="Collapse panel"><svg width="15" height="15" fill="none" viewBox="0 0 24 24" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><polyline points="15 18 9 12 15 6"/></svg></button></div>
  <div class="panel-body">%s</div>
</div>

<div class="side-panel" id="sharePanel">
  <div class="panel-hdr">Share Impact<button class="panel-collapse" onclick="toggle('share')" title="Collapse panel" aria-label="Collapse panel"><svg width="15" height="15" fill="none" viewBox="0 0 24 24" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><polyline points="15 18 9 12 15 6"/></svg></button></div>
  <div class="panel-body">
    <textarea class="share-textarea" id="shareText">%s</textarea>
    <button class="copy-btn" id="copyBtn" onclick="copyShare()">
      <svg width="13" height="13" fill="none" viewBox="0 0 24 24" stroke="currentColor" stroke-width="2"><rect x="9" y="9" width="13" height="13" rx="2"/><path d="M5 15H4a2 2 0 01-2-2V4a2 2 0 012-2h9a2 2 0 012 2v1"/></svg>
      Copy message
    </button>
  </div>
</div>

<div class="side-panel" id="infoPanel">
  <div class="panel-hdr">About git-lrc<button class="panel-collapse" onclick="toggle('info')" title="Collapse panel" aria-label="Collapse panel"><svg width="15" height="15" fill="none" viewBox="0 0 24 24" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><polyline points="15 18 9 12 15 6"/></svg></button></div>
  <div class="panel-body">
    <p class="gh-cta">Is it being useful? If you haven't starred, <a href="https://github.com/HexmosTech/git-lrc" target="_blank">star us on GitHub</a> — it really helps!</p>
    %s
  </div>
</div>

<div class="main shifted" id="main">
  <div class="page-hero">
    <h1>Dashboard</h1>
    <p>Your AI code-review impact and live review sessions.</p>
  </div>
  %s
  <div class="sec-hdr">Active Reviews <span class="badge">%d</span></div>
  <table>
    <thead><tr><th>Name</th><th>Repository</th><th>ID</th><th>Port</th><th>Started</th></tr></thead>
    <tbody>%s</tbody>
  </table>
</div>
<div class="statusbar"><a href="https://hexmos.com/livereview/" target="_blank" style="color:#fff;text-decoration:none;">LiveReview</a><span>git-lrc</span></div>
<script>
const COPY_SVG = '<svg width="13" height="13" fill="none" viewBox="0 0 24 24" stroke="currentColor" stroke-width="2"><rect x="9" y="9" width="13" height="13" rx="2"/><path d="M5 15H4a2 2 0 01-2-2V4a2 2 0 012-2h9a2 2 0 012 2v1"/></svg> Copy message';
let openPanel = 'stats';
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
</html>`, statsRows, shareText, ghPanel, statCards, len(reviews), tableRows)

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	fmt.Fprint(w, page)
}
