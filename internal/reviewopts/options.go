package reviewopts

import (
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/HexmosTech/git-lrc/storage"
	"github.com/urfave/cli/v2"
)

const (
	DefaultAPIURL                = "http://localhost:8888"
	DefaultPollInterval          = 2 * time.Second
	DefaultTimeout               = 5 * time.Minute
	DefaultBlockingReviewTimeout = 20 * time.Minute
	DefaultOutputFormat          = "pretty"
)

type Options struct {
	RepoName              string
	DiffSource            string
	RangeVal              string
	CommitVal             string
	DiffFile              string
	APIURL                string
	APIKey                string
	PollInterval          time.Duration
	Timeout               time.Duration
	Output                string
	SaveBundle            string
	SaveJSON              string
	SaveText              string
	SaveHTML              string
	Serve                 bool
	Port                  int
	Verbose               bool
	Precommit             bool
	BlockingReview        bool
	BlockingReviewTimeout time.Duration
	Skip                  bool
	Force                 bool
	Vouch                 bool
	InitialMsg            string
	Terminal              bool
}

func BuildFromContext(c *cli.Context, includeDebug bool) (Options, error) {
	initialMsg := ""
	if msgFile := os.Getenv("LRC_INITIAL_MESSAGE_FILE"); msgFile != "" {
		if data, err := storage.ReadInitialMessageFile(msgFile); err == nil {
			initialMsg = strings.TrimRight(string(data), "\r\n")
		}
	} else {
		initialMsg = strings.TrimRight(os.Getenv("LRC_INITIAL_MESSAGE"), "\r\n")
	}

	opts := Options{
		RepoName:              c.String("repo-name"),
		RangeVal:              c.String("range"),
		CommitVal:             c.String("commit"),
		DiffFile:              c.String("diff-file"),
		APIURL:                c.String("api-url"),
		APIKey:                c.String("api-key"),
		Output:                c.String("output"),
		SaveHTML:              c.String("save-html"),
		Serve:                 c.Bool("serve"),
		Port:                  c.Int("port"),
		Verbose:               c.Bool("verbose"),
		Precommit:             c.Bool("precommit"),
		BlockingReview:        c.Bool("blocking-review"),
		BlockingReviewTimeout: c.Duration("blocking-review-timeout"),
		Skip:                  c.Bool("skip"),
		Force:                 c.Bool("force"),
		Vouch:                 c.Bool("vouch"),
		SaveJSON:              c.String("save-json"),
		SaveText:              c.String("save-text"),
		InitialMsg:            initialMsg,
		Terminal:              c.Bool("terminal"),
	}

	if opts.Skip || opts.Vouch {
		opts.Precommit = false
	}
	if opts.Skip && opts.Vouch {
		return Options{}, fmt.Errorf("cannot use --skip and --vouch together")
	}
	if opts.BlockingReview {
		if opts.Precommit {
			return Options{}, fmt.Errorf("cannot use --blocking-review and --precommit together")
		}
		if opts.BlockingReviewTimeout <= 0 {
			return Options{}, fmt.Errorf("--blocking-review-timeout must be greater than zero")
		}
		if opts.Skip {
			return Options{}, fmt.Errorf("cannot use --blocking-review and --skip together")
		}
		if opts.Vouch {
			return Options{}, fmt.Errorf("cannot use --blocking-review and --vouch together")
		}
	}

	staged := c.Bool("staged")
	diffSource := c.String("diff-source")

	if opts.DiffFile != "" {
		diffSource = "file"
	} else if opts.CommitVal != "" {
		if opts.BlockingReview {
			return Options{}, fmt.Errorf("cannot use --blocking-review with --commit reviews")
		}
		diffSource = "commit"
		opts.Precommit = false
		opts.Skip = false
		if !c.IsSet("serve") && !c.IsSet("save-html") && !opts.Terminal {
			opts.Serve = true
		}
	} else if opts.RangeVal != "" {
		diffSource = "range"
	} else if staged {
		diffSource = "staged"
	}

	if diffSource == "" {
		diffSource = "staged"
	}

	opts.DiffSource = diffSource
	if opts.BlockingReview {
		opts.Serve = true
	}

	if includeDebug {
		opts.PollInterval = c.Duration("poll-interval")
		opts.Timeout = c.Duration("timeout")
		opts.SaveBundle = c.String("save-bundle")
	} else {
		opts.PollInterval = DefaultPollInterval
		opts.Timeout = DefaultTimeout
	}

	if opts.APIURL == "" {
		opts.APIURL = DefaultAPIURL
	}

	if opts.Output == "" {
		opts.Output = DefaultOutputFormat
	}

	// If terminal mode is explicitly enabled, ensure serve is disabled
	if opts.Terminal {
		opts.Serve = false
	}

	return opts, nil
}

func ApplyDefaultHTMLServe(opts *Options) (string, error) {
	if opts.SaveHTML != "" || opts.Output != DefaultOutputFormat {
		return opts.SaveHTML, nil
	}

	if opts.Serve {
		tmpFile, err := storage.CreateTempReviewHTMLFile()
		if err != nil {
			return "", fmt.Errorf("failed to create temporary HTML file: %w", err)
		}

		if err := tmpFile.Close(); err != nil {
			return "", fmt.Errorf("failed to prepare temporary HTML file: %w", err)
		}

		opts.SaveHTML = tmpFile.Name()
		return opts.SaveHTML, nil
	}

	opts.SaveHTML = "review_output.html"
	return opts.SaveHTML, nil
}
