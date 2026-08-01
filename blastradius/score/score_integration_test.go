//go:build integration

package score

import (
	"context"
	"testing"

	"github.com/HexmosTech/blastradius/client"
)

// Run with: go test -tags=integration ./score/... -run Integration -v
func TestIntegrationFanInRealProject(t *testing.T) {
	c := client.New("home-shrsv-bin-LiveReview")
	if err := c.Available(); err != nil {
		t.Skip(err)
	}

	targets := []string{
		"home-shrsv-bin-LiveReview.network.email.SendInvitationEmail",
		"home-shrsv-bin-LiveReview.network.email.SendInvitationEmailSMTP",
		"home-shrsv-bin-LiveReview.network.email.getParseAppID",
	}
	got, err := FanIn(context.Background(), c, targets, Defaults())
	if err != nil {
		t.Fatal(err)
	}
	for _, qn := range targets {
		s := got[qn]
		t.Logf("%s: raw=%.3f callers=%d", qn, s.Raw, len(s.Callers))
		for _, caller := range s.Callers {
			t.Logf("   <- %s (depth %d, weight %.3f)", caller.QualifiedName, caller.Depth, caller.Weight)
		}
	}
	// SendInvitationEmail is called directly by the API handler and
	// transitively reaches CreateUser/CreateUserInOrg/CreateUserInAnyOrg -
	// it should score meaningfully higher than the tiny unexported helper
	// getParseAppID, which we already know has a single direct caller.
	if got["home-shrsv-bin-LiveReview.network.email.SendInvitationEmail"].Raw <=
		got["home-shrsv-bin-LiveReview.network.email.getParseAppID"].Raw {
		t.Fatal("expected SendInvitationEmail to outscore getParseAppID")
	}
}
