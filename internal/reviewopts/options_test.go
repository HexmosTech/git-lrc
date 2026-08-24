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

func TestBuildFromContextNoServe(t *testing.T) {
	t.Run("range without no-serve auto-enables serve", func(t *testing.T) {
		ctx := newOptionsTestContext(t, []string{"--range", "main...feature"})

		opts, err := BuildFromContext(ctx, false)
		if err != nil {
			t.Fatalf("BuildFromContext() error = %v", err)
		}
		if !opts.Serve {
			t.Fatalf("Serve = false, want true (range auto-enables serve)")
		}
		if opts.DiffSource != "range" {
			t.Fatalf("DiffSource = %q, want %q", opts.DiffSource, "range")
		}
	})

	t.Run("range with no-serve suppresses serve", func(t *testing.T) {
		ctx := newOptionsTestContext(t, []string{"--range", "main...feature", "--no-serve"})

		opts, err := BuildFromContext(ctx, false)
		if err != nil {
			t.Fatalf("BuildFromContext() error = %v", err)
		}
		if opts.Serve {
			t.Fatalf("Serve = true, want false (--no-serve should suppress)")
		}
		if opts.DiffSource != "range" {
			t.Fatalf("DiffSource = %q, want %q", opts.DiffSource, "range")
		}
	})

	t.Run("commit without no-serve auto-enables serve", func(t *testing.T) {
		ctx := newOptionsTestContext(t, []string{"--commit", "HEAD"})

		opts, err := BuildFromContext(ctx, false)
		if err != nil {
			t.Fatalf("BuildFromContext() error = %v", err)
		}
		if !opts.Serve {
			t.Fatalf("Serve = false, want true (commit auto-enables serve)")
		}
		if opts.DiffSource != "commit" {
			t.Fatalf("DiffSource = %q, want %q", opts.DiffSource, "commit")
		}
	})

	t.Run("commit with no-serve suppresses serve", func(t *testing.T) {
		ctx := newOptionsTestContext(t, []string{"--commit", "HEAD", "--no-serve"})

		opts, err := BuildFromContext(ctx, false)
		if err != nil {
			t.Fatalf("BuildFromContext() error = %v", err)
		}
		if opts.Serve {
			t.Fatalf("Serve = true, want false (--no-serve should suppress)")
		}
		if opts.DiffSource != "commit" {
			t.Fatalf("DiffSource = %q, want %q", opts.DiffSource, "commit")
		}
	})

	t.Run("staged with no-serve suppresses serve", func(t *testing.T) {
		ctx := newOptionsTestContext(t, []string{"--staged", "--no-serve"})

		opts, err := BuildFromContext(ctx, false)
		if err != nil {
			t.Fatalf("BuildFromContext() error = %v", err)
		}
		if opts.Serve {
			t.Fatalf("Serve = true, want false (--no-serve should suppress)")
		}
		if opts.DiffSource != "staged" {
			t.Fatalf("DiffSource = %q, want %q", opts.DiffSource, "staged")
		}
	})

	t.Run("rejects no-serve with serve", func(t *testing.T) {
		ctx := newOptionsTestContext(t, []string{"--no-serve", "--serve"})

		_, err := BuildFromContext(ctx, false)
		if err == nil || err.Error() != "cannot use --no-serve and --serve together" {
			t.Fatalf("BuildFromContext() error = %v, want no-serve/serve conflict", err)
		}
	})

	t.Run("rejects no-serve with blocking-review", func(t *testing.T) {
		ctx := newOptionsTestContext(t, []string{"--no-serve", "--blocking-review"})

		_, err := BuildFromContext(ctx, false)
		if err == nil || err.Error() != "cannot use --no-serve and --blocking-review together" {
			t.Fatalf("BuildFromContext() error = %v, want no-serve/blocking-review conflict", err)
		}
	})

	t.Run("range with no-serve and output json", func(t *testing.T) {
		ctx := newOptionsTestContext(t, []string{"--range", "main...feature", "--no-serve", "--output", "json"})

		opts, err := BuildFromContext(ctx, false)
		if err != nil {
			t.Fatalf("BuildFromContext() error = %v", err)
		}
		if opts.Serve {
			t.Fatalf("Serve = true, want false")
		}
		if opts.Output != "json" {
			t.Fatalf("Output = %q, want %q", opts.Output, "json")
		}
		if opts.NoServe != true {
			t.Fatalf("NoServe = false, want true")
		}
	})
}

func newOptionsTestContext(t *testing.T, args []string) *cli.Context {
	t.Helper()

	set := flag.NewFlagSet("reviewopts-test", flag.ContinueOnError)
	for _, boolName := range []string{"staged", "serve", "no-serve", "verbose", "precommit", "blocking-review", "skip", "force", "vouch", "blast-radius", "sort-by-blast-radius"} {
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
