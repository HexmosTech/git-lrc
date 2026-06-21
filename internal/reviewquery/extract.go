package reviewquery

import (
	"fmt"
	"log"
	"os/exec"
	"regexp"
	"strconv"
	"strings"
	"time"
)

// trailerPrefix is the marker the commit-msg hook writes into each commit.
// See hooks/commit-msg.sh and internal/appcore/attestation_flow.go.
const trailerPrefix = "LiveReview Pre-Commit Check:"

// trailerDetailRe pulls the optional "(iter:N, coverage:M%)" suffix.
var trailerDetailRe = regexp.MustCompile(`iter:(\d+),\s*coverage:(\d+)%`)

// field/record separators chosen so they never appear in commit text.
const (
	fieldSep  = "\x1f"
	recordSep = "\x1e"
)

// parseTrailer extracts the outcome and optional metrics from a single
// commit-message line. Pure function — unit-testable without git.
func parseTrailer(line string) (action string, iter int, covPct int, ok bool) {
	line = strings.TrimSpace(line)
	if !strings.HasPrefix(line, trailerPrefix) {
		return "", 0, 0, false
	}
	rest := strings.TrimSpace(strings.TrimPrefix(line, trailerPrefix))

	switch {
	case strings.HasPrefix(rest, "ran"):
		action = "reviewed"
	case strings.HasPrefix(rest, "vouched"):
		action = "vouched"
	case strings.HasPrefix(rest, "skipped"):
		action = "skipped"
	default:
		return "", 0, 0, false
	}

	if m := trailerDetailRe.FindStringSubmatch(rest); m != nil {
		iter, _ = strconv.Atoi(m[1])
		covPct, _ = strconv.Atoi(m[2])
	}
	return action, iter, covPct, true
}

// parseRecord turns one git-log record (fields joined by fieldSep) into a
// ReviewRecord. Pure function. Returns ok=false if the record is malformed.
func parseRecord(raw, branch string) (ReviewRecord, bool) {
	parts := strings.Split(raw, fieldSep)
	if len(parts) < 7 {
		return ReviewRecord{}, false
	}
	rec := ReviewRecord{
		Hash:      strings.TrimSpace(parts[0]),
		ShortHash: strings.TrimSpace(parts[1]),
		Author:    parts[2],
		Email:     parts[3],
		Subject:   parts[5],
		Branch:    branch,
		Action:    "none",
	}
	if t, err := time.Parse(time.RFC3339, strings.TrimSpace(parts[4])); err == nil {
		rec.Date = t
	} else {
		log.Printf("reviewquery: failed to parse commit date %q for %s: %v", parts[4], rec.ShortHash, err)
	}
	body := parts[6]
	for line := range strings.SplitSeq(body, "\n") {
		if action, iter, cov, ok := parseTrailer(line); ok {
			rec.Action = action
			rec.Iterations = iter
			rec.CoveragePct = cov
			break
		}
	}
	return rec, true
}

// currentBranch returns the branch git log will run against (best-effort).
func currentBranch() string {
	out, err := exec.Command("git", "rev-parse", "--abbrev-ref", "HEAD").Output()
	if err != nil {
		log.Printf("reviewquery: failed to determine current branch: %v", err)
		return ""
	}
	return strings.TrimSpace(string(out))
}

// Extract runs `git log` (scoped by filter) and returns one record per commit.
func Extract(f Filter) ([]ReviewRecord, error) {
	format := strings.Join([]string{"%H", "%h", "%an", "%ae", "%aI", "%s", "%B"}, fieldSep) + recordSep
	args := []string{"log", "--pretty=format:" + format}

	if f.Author != "" {
		args = append(args, "--author="+f.Author)
	}
	if f.From != "" {
		args = append(args, "--since="+f.From)
	}
	if f.To != "" {
		args = append(args, "--until="+f.To)
	}
	if f.Range != "" {
		// --end-of-options stops git from treating f.Range as an option if it
		// happens to start with '-' (e.g. a crafted --range value); the rest of
		// the args up to "--" are already our own well-formed --flag=value pairs.
		args = append(args, "--end-of-options", f.Range)
	}
	if f.PathPrefix != "" {
		args = append(args, "--", f.PathPrefix)
	}

	out, err := exec.Command("git", args...).Output()
	if err != nil {
		return nil, fmt.Errorf("failed to read git log (are you inside a git repo?): %w", err)
	}

	branch := currentBranch()
	rawRecords := strings.Split(string(out), recordSep)
	records := make([]ReviewRecord, 0, len(rawRecords))
	for _, raw := range rawRecords {
		if strings.TrimSpace(raw) == "" {
			continue
		}
		rec, ok := parseRecord(raw, branch)
		if !ok {
			continue
		}
		if f.Action != "" && rec.Action != f.Action {
			continue
		}
		records = append(records, rec)
	}
	return records, nil
}
