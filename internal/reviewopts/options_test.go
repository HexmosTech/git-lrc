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
		if opts.BlockingReviewTimeout != DefaultBlockingReviewTimeout {
			t.Fatalf("BlockingReviewTimeout = %v, want %v", opts.BlockingReviewTimeout, DefaultBlockingReviewTimeout)
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

	t.Run("rejects non-positive blocking timeout", func(t *testing.T) {
		ctx := newOptionsTestContext(t, []string{"--blocking-review", "--blocking-review-timeout", "0s"})

		_, err := BuildFromContext(ctx, false)
		if err == nil || err.Error() != "--blocking-review-timeout must be greater than zero" {
			t.Fatalf("BuildFromContext() error = %v, want blocking timeout validation", err)
		}
	})
}

func TestBuildFromContextBlastRadius(t *testing.T) {
	t.Run("works without a project name (auto-derived at review time)", func(t *testing.T) {
		ctx := newOptionsTestContext(t, []string{"--blast-radius"})

		opts, err := BuildFromContext(ctx, false)
		if err != nil {
			t.Fatalf("BuildFromContext() error = %v", err)
		}
		if !opts.BlastRadius || opts.BlastRadiusProject != "" {
			t.Fatalf("opts = %+v, want BlastRadius=true with empty project", opts)
		}
	})

	t.Run("accepts project name", func(t *testing.T) {
		ctx := newOptionsTestContext(t, []string{"--blast-radius", "--blast-radius-project", "my-project"})

		opts, err := BuildFromContext(ctx, false)
		if err != nil {
			t.Fatalf("BuildFromContext() error = %v", err)
		}
		if !opts.BlastRadius || opts.BlastRadiusProject != "my-project" {
			t.Fatalf("opts = %+v, want BlastRadius=true, BlastRadiusProject=my-project", opts)
		}
	})

	t.Run("sort-by-blast-radius implies blast-radius", func(t *testing.T) {
		ctx := newOptionsTestContext(t, []string{"--sort-by-blast-radius", "--blast-radius-project", "my-project"})

		opts, err := BuildFromContext(ctx, false)
		if err != nil {
			t.Fatalf("BuildFromContext() error = %v", err)
		}
		if !opts.BlastRadius || !opts.SortByBlastRadius {
			t.Fatalf("opts = %+v, want BlastRadius=true and SortByBlastRadius=true", opts)
		}
	})

	// The real CLI default for --blast-radius is true (set in main.go's flag
	// definition); this harness's raw flagset defaults it to false, which
	// verifies BuildFromContext mirrors the flag rather than force-enabling.
	t.Run("respects a disabled flag", func(t *testing.T) {
		ctx := newOptionsTestContext(t, nil)

		opts, err := BuildFromContext(ctx, false)
		if err != nil {
			t.Fatalf("BuildFromContext() error = %v", err)
		}
		if opts.BlastRadius || opts.SortByBlastRadius {
			t.Fatalf("opts = %+v, want blast-radius mirroring the (false) flag", opts)
		}
	})
}

func newOptionsTestContext(t *testing.T, args []string) *cli.Context {
	t.Helper()

	set := flag.NewFlagSet("reviewopts-test", flag.ContinueOnError)
	for _, boolName := range []string{"staged", "serve", "verbose", "precommit", "blocking-review", "skip", "force", "vouch", "blast-radius", "sort-by-blast-radius"} {
		set.Bool(boolName, false, "")
	}
	for _, stringName := range []string{"repo-name", "range", "commit", "diff-file", "api-url", "api-key", "output", "save-html", "save-json", "save-text", "diff-source", "blast-radius-project"} {
		set.String(stringName, "", "")
	}
	set.Duration("blocking-review-timeout", DefaultBlockingReviewTimeout, "")
	set.Int("port", 8000, "")

	if err := set.Parse(args); err != nil {
		t.Fatalf("Parse() error = %v", err)
	}

	return cli.NewContext(cli.NewApp(), set, nil)
}
