//go:build integration

package blastradius

import (
	"context"
	"os"
	"testing"

	"github.com/HexmosTech/blastradius/client"
)

// Run with: go test -tags=integration . -run Integration -v
func TestIntegrationScoreDiffRealFixture(t *testing.T) {
	c := client.New("home-shrsv-bin-LiveReview")
	if err := c.Available(); err != nil {
		t.Skip(err)
	}

	diff, err := os.ReadFile("internal/testfixtures/sample-core.diff")
	if err != nil {
		t.Fatal(err)
	}

	report, err := ScoreDiff(context.Background(), diff, "home-shrsv-bin-LiveReview")
	if err != nil {
		t.Fatal(err)
	}
	if len(report.Files) == 0 {
		t.Fatal("expected at least one file in report")
	}
	for _, f := range report.Files {
		for _, h := range f.Hunks {
			t.Logf("%s %s blast=%.2f/%.1f priority=%.2f/%.1f combined=%.1f symbols=%d",
				f.Path, h.Header, h.BlastRadiusRaw, h.BlastRadiusNorm, h.ReviewPriorityRaw, h.ReviewPriorityNorm, h.Combined, len(h.Symbols))
			for _, s := range h.Symbols {
				t.Logf("    %s (%s, %s) blast=%.2f priority=%.2f direct=%d transitive=%d",
					s.Name, s.Label, s.Method, s.BlastRadiusRaw, s.ReviewPriorityRaw, s.DirectCount, s.TransitiveCount)
			}
		}
	}
}
