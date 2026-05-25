package reviewopts

import (
	"flag"
	"testing"

	"github.com/urfave/cli/v2"
)

func TestBuildFromContextBlockingReview(t *testing.T) {
	t.Run("enables serve automatically", func(t *testing.T) {
		ctx := newOptionsTestContext(t, []string{"--blocking-review"})

		opts, err := BuildFromContext(ctx, false)
		if err != nil {
			t.Fatalf("BuildFromContext() error = %v", err)
		}
		if !opts.BlockingReview {
			t.Fatalf("BlockingReview = false, want true")
		}
		if !opts.Serve {
			t.Fatalf("Serve = false, want true")
		}
	})

	t.Run("rejects precommit", func(t *testing.T) {
		ctx := newOptionsTestContext(t, []string{"--blocking-review", "--precommit"})

		_, err := BuildFromContext(ctx, false)
		if err == nil || err.Error() != "cannot use --blocking-review and --precommit together" {
			t.Fatalf("BuildFromContext() error = %v, want blocking-review/precommit conflict", err)
		}
	})

	t.Run("rejects commit review", func(t *testing.T) {
		ctx := newOptionsTestContext(t, []string{"--blocking-review", "--commit", "HEAD"})

		_, err := BuildFromContext(ctx, false)
		if err == nil || err.Error() != "cannot use --blocking-review with --commit reviews" {
			t.Fatalf("BuildFromContext() error = %v, want blocking-review/commit conflict", err)
		}
	})
}

func newOptionsTestContext(t *testing.T, args []string) *cli.Context {
	t.Helper()

	set := flag.NewFlagSet("reviewopts-test", flag.ContinueOnError)
	for _, boolName := range []string{"staged", "serve", "verbose", "precommit", "blocking-review", "skip", "force", "vouch"} {
		set.Bool(boolName, false, "")
	}
	for _, stringName := range []string{"repo-name", "range", "commit", "diff-file", "api-url", "api-key", "output", "save-html", "save-json", "save-text", "diff-source"} {
		set.String(stringName, "", "")
	}
	set.Int("port", 8000, "")

	if err := set.Parse(args); err != nil {
		t.Fatalf("Parse() error = %v", err)
	}

	return cli.NewContext(cli.NewApp(), set, nil)
}
