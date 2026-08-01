//go:build integration

package symbols

import (
	"context"
	"testing"

	"github.com/HexmosTech/blastradius/client"
)

// Run with: go test -tags=integration ./symbols/... -run Integration -v
// Requires codebase-memory-mcp on PATH with the "home-shrsv-bin-LiveReview"
// project already indexed.
func TestIntegrationInFileRealProject(t *testing.T) {
	c := client.New("home-shrsv-bin-LiveReview")
	if err := c.Available(); err != nil {
		t.Skip(err)
	}

	syms, err := InFile(context.Background(), c, "network/email/invitation.go")
	if err != nil {
		t.Fatal(err)
	}
	if len(syms) == 0 {
		t.Fatal("expected at least one symbol in network/email/invitation.go")
	}

	found := false
	for _, s := range syms {
		t.Logf("symbol: %+v", s)
		if s.Name == "SendInvitationEmail" && s.StartLine == 36 && s.EndLine == 126 {
			found = true
		}
	}
	if !found {
		t.Fatal("expected to find SendInvitationEmail at lines 36-126")
	}
}
