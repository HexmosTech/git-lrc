package appcore

import (
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/HexmosTech/git-lrc/internal/reviewapi"
	"github.com/HexmosTech/git-lrc/internal/reviewmodel"
	"github.com/HexmosTech/git-lrc/internal/reviewquery"
	"github.com/HexmosTech/git-lrc/network"
	"github.com/urfave/cli/v2"
)

// RunCoverage implements `git-lrc coverage <ref> [ref...]`: merges local
// git-log-trailer evidence (already embedded in this repo's commit history
// by git-lrc's own pre-commit flow — internal/reviewquery, no network call)
// with LiveReview's backend-stored review records (POST
// /api/v1/review-coverage, which covers PR/MR, API, MCP, and --commit/
// --range CLI reviews) into one {commits, reports} object. No verdict is
// computed here — the caller (typically a CI/CD pipeline) applies its own
// pass/fail policy over the raw reports.
func RunCoverage(c *cli.Context) error {
	refs := c.Args().Slice()
	if len(refs) == 0 {
		return fmt.Errorf("usage: git-lrc coverage <ref> [ref...]  (a commit SHA, or a range like a..b)")
	}
	verbose := c.Bool("verbose")

	var reports []reviewmodel.ReviewCoverageReport

	gitLogReports, err := collectGitLogReports(refs)
	if err != nil && verbose {
		fmt.Fprintf(os.Stderr, "warning: local git-log lookup failed: %v\n", err)
	}
	reports = append(reports, gitLogReports...)

	dbReports, err := collectDatabaseReports(c, refs, verbose)
	if err != nil {
		fmt.Fprintf(os.Stderr, "warning: LiveReview backend lookup failed: %v\n", err)
	}
	reports = append(reports, dbReports...)

	result := reviewmodel.ReviewCoverageResponse{Commits: refs, Reports: reports}

	if c.Bool("json") {
		enc := json.NewEncoder(os.Stdout)
		enc.SetIndent("", "  ")
		return enc.Encode(result)
	}
	printCoverageReports(result)
	return nil
}

// collectGitLogReports produces the "git_log" half of the coverage object:
// every commit-msg trailer already embedded in this repo's history for the
// given refs, parsed straight out of `git log` (internal/reviewquery) with
// no network call. A range ref is expanded via reviewquery.Extract (real
// git ancestry, since this runs where the repo actually is); a bare commit
// ref is looked up directly via reviewquery.ExtractOne.
func collectGitLogReports(refs []string) ([]reviewmodel.ReviewCoverageReport, error) {
	var out []reviewmodel.ReviewCoverageReport
	var firstErr error
	for _, ref := range refs {
		if strings.Contains(ref, "..") {
			records, err := reviewquery.Extract(reviewquery.Filter{Range: ref})
			if err != nil {
				if firstErr == nil {
					firstErr = err
				}
				continue
			}
			for _, rec := range records {
				if rec.Action == "none" {
					continue
				}
				out = append(out, gitLogReportFromRecord(rec.Hash, rec))
			}
			continue
		}

		rec, ok, err := reviewquery.ExtractOne(ref)
		if err != nil {
			if firstErr == nil {
				firstErr = err
			}
			continue
		}
		if !ok || rec.Action == "none" {
			continue
		}
		out = append(out, gitLogReportFromRecord(ref, rec))
	}
	return out, firstErr
}

func gitLogReportFromRecord(ref string, rec reviewquery.ReviewRecord) reviewmodel.ReviewCoverageReport {
	report := reviewmodel.ReviewCoverageReport{
		Ref:         ref,
		Source:      "git_log",
		Action:      rec.Action,
		Iterations:  rec.Iterations,
		CoveragePct: rec.CoveragePct,
	}
	if !rec.Date.IsZero() {
		report.CommitDate = rec.Date.Format(time.RFC3339)
	}
	return report
}

// collectDatabaseReports produces the "database" half of the coverage
// object by calling LiveReview's backend. Range refs are expanded to
// individual commit SHAs via `git rev-list` (git-lrc has the repo; the
// backend never does ancestry expansion itself) in addition to sending the
// literal range expression, so the query matches whichever identifier form
// a prior review submission used.
func collectDatabaseReports(c *cli.Context, refs []string, verbose bool) ([]reviewmodel.ReviewCoverageReport, error) {
	config, err := loadConfigValues(c.String("api-key"), c.String("api-url"), verbose)
	if err != nil {
		return nil, err
	}
	if strings.TrimSpace(config.APIKey) == "" {
		return nil, fmt.Errorf("no API key configured (run `git-lrc setup`, or set LRC_API_KEY / --api-key)")
	}
	return fetchDatabaseReports(config.APIURL, config.APIKey, refs)
}

// fetchDatabaseReports expands any range refs to individual commit SHAs via
// `git rev-list` (best-effort; the literal range ref is still queried even
// if expansion fails) and calls LiveReview's /api/v1/review-coverage with
// the combined set. Split out from collectDatabaseReports so it's testable
// against a mock server without needing a *cli.Context.
func fetchDatabaseReports(apiURL, apiKey string, refs []string) ([]reviewmodel.ReviewCoverageReport, error) {
	querySet := make(map[string]bool, len(refs))
	for _, ref := range refs {
		querySet[ref] = true
		if !strings.Contains(ref, "..") {
			continue
		}
		out, err := reviewapi.RunGitCommand("rev-list", ref)
		if err != nil {
			continue // best-effort expansion; the literal range ref is still queried
		}
		for _, sha := range strings.Split(strings.TrimSpace(string(out)), "\n") {
			if sha = strings.TrimSpace(sha); sha != "" {
				querySet[sha] = true
			}
		}
	}
	commits := make([]string, 0, len(querySet))
	for ref := range querySet {
		commits = append(commits, ref)
	}

	client := network.NewReviewAPIClient(30 * time.Second)
	resp, err := network.ReviewCoverage(client, apiURL, reviewmodel.ReviewCoverageRequest{Commits: commits}, apiKey)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode != http.StatusOK {
		return nil, &reviewmodel.APIError{StatusCode: resp.StatusCode, Body: string(resp.Body)}
	}

	var parsed reviewmodel.ReviewCoverageResponse
	if err := json.Unmarshal(resp.Body, &parsed); err != nil {
		return nil, fmt.Errorf("failed to parse review-coverage response: %w", err)
	}
	for i := range parsed.Reports {
		parsed.Reports[i].Source = "database"
	}
	return parsed.Reports, nil
}

func printCoverageReports(result reviewmodel.ReviewCoverageResponse) {
	fmt.Printf("Coverage for %d ref(s):\n", len(result.Commits))
	if len(result.Reports) == 0 {
		fmt.Println("  No reports found -- neither local git log nor LiveReview's backend has evidence for these refs.")
		return
	}
	for _, r := range result.Reports {
		if r.Source == "git_log" {
			fmt.Printf("  %-12s [git_log]  %-8s iter:%d coverage:%d%%%s\n",
				shortRef(r.Ref), r.Action, r.Iterations, r.CoveragePct, dateSuffix(r.CommitDate))
			continue
		}
		fmt.Printf("  %-12s [database] trigger=%-9s status=%-9s%s%s\n",
			shortRef(r.Ref), r.TriggerType, r.Status, userSuffix(r.UserEmail), dateSuffix(r.CompletedAt))
	}
}

func shortRef(ref string) string {
	if len(ref) > 12 && !strings.Contains(ref, "..") {
		return ref[:12]
	}
	return ref
}

func dateSuffix(v string) string {
	if v == "" {
		return ""
	}
	return " at " + v
}

func userSuffix(v string) string {
	if v == "" {
		return ""
	}
	return " user=" + v
}
