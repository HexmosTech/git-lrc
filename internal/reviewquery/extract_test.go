package reviewquery

import (
	"strings"
	"testing"
)

func TestParseTrailer(t *testing.T) {
	cases := []struct {
		name   string
		in     string
		action string
		iter   int
		cov    int
		ok     bool
	}{
		{"ran plain", "LiveReview Pre-Commit Check: ran", "reviewed", 0, 0, true},
		{"ran with metrics", "LiveReview Pre-Commit Check: ran (iter:3, coverage:82%)", "reviewed", 3, 82, true},
		{"vouched", "LiveReview Pre-Commit Check: vouched (iter:1, coverage:100%)", "vouched", 1, 100, true},
		{"skipped", "LiveReview Pre-Commit Check: skipped", "skipped", 0, 0, true},
		{"skipped manually", "LiveReview Pre-Commit Check: skipped manually", "skipped", 0, 0, true},
		{"indented", "    LiveReview Pre-Commit Check: ran", "reviewed", 0, 0, true},
		{"unrelated", "Fix the login bug", "", 0, 0, false},
		{"empty", "", "", 0, 0, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			action, iter, cov, ok := parseTrailer(tc.in)
			if ok != tc.ok || action != tc.action || iter != tc.iter || cov != tc.cov {
				t.Errorf("parseTrailer(%q) = (%q,%d,%d,%v); want (%q,%d,%d,%v)",
					tc.in, action, iter, cov, ok, tc.action, tc.iter, tc.cov, tc.ok)
			}
		})
	}
}

func TestParseRecord(t *testing.T) {
	// fields: hash, short, author, email, dateISO, subject, body
	raw := strings.Join([]string{
		"abc123def456",
		"abc123d",
		"Jane Dev",
		"jane@example.com",
		"2026-06-17T10:30:00Z",
		"Add the thing",
		"Add the thing\n\nLiveReview Pre-Commit Check: ran (iter:2, coverage:75%)",
	}, fieldSep)

	rec, ok := parseRecord(raw, "main")
	if !ok {
		t.Fatal("parseRecord returned ok=false for a valid record")
	}
	if rec.Hash != "abc123def456" || rec.ShortHash != "abc123d" {
		t.Errorf("hash fields wrong: %+v", rec)
	}
	if rec.Author != "Jane Dev" || rec.Branch != "main" {
		t.Errorf("author/branch wrong: %+v", rec)
	}
	if rec.Action != "reviewed" || rec.Iterations != 2 || rec.CoveragePct != 75 {
		t.Errorf("trailer parse wrong: action=%q iter=%d cov=%d", rec.Action, rec.Iterations, rec.CoveragePct)
	}
	if rec.Date.Year() != 2026 || rec.Date.Month() != 6 {
		t.Errorf("date parse wrong: %v", rec.Date)
	}
}

func TestParseRecordNoTrailer(t *testing.T) {
	raw := strings.Join([]string{
		"h", "h", "A", "a@b.c", "2026-06-17T10:30:00Z", "subject", "body with no trailer",
	}, fieldSep)
	rec, ok := parseRecord(raw, "main")
	if !ok {
		t.Fatal("expected ok=true")
	}
	if rec.Action != "none" {
		t.Errorf("expected action=none, got %q", rec.Action)
	}
}

func TestParseRecordMalformed(t *testing.T) {
	if _, ok := parseRecord("too\x1ffew\x1ffields", "main"); ok {
		t.Error("expected ok=false for malformed record")
	}
}
