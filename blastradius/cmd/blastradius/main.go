// Command blastradius scores a unified diff's hunks by how "important" the
// symbols they touch are, using a codebase-memory-mcp knowledge graph.
//
// Usage:
//
//	git diff | blastradius score --project <name> --out report.json
//	blastradius score --diff mychange.diff --project <name>
package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"os"

	"github.com/HexmosTech/blastradius"
	"github.com/HexmosTech/blastradius/score"
)

func main() {
	if len(os.Args) < 2 {
		usage()
		os.Exit(2)
	}
	switch os.Args[1] {
	case "score":
		if err := runScore(os.Args[2:]); err != nil {
			fmt.Fprintln(os.Stderr, "blastradius:", err)
			os.Exit(1)
		}
	case "-h", "--help", "help":
		usage()
	default:
		fmt.Fprintf(os.Stderr, "blastradius: unknown command %q\n\n", os.Args[1])
		usage()
		os.Exit(2)
	}
}

func usage() {
	fmt.Fprint(os.Stderr, `blastradius - blast-radius + review-priority scoring for diff hunks

Usage:
  blastradius score --project <name> [--diff <file>] [--out <file>] [--max-depth N] [--decay F] [--blast-weight F] [--priority-weight F]

If --diff is omitted or "-", the diff is read from stdin.
If --out is omitted or "-", the JSON report is written to stdout.
`)
}

func runScore(args []string) error {
	fs := flag.NewFlagSet("score", flag.ExitOnError)
	project := fs.String("project", "", "codebase-memory-mcp project name (required, see `codebase-memory-mcp cli list_projects`)")
	diffPath := fs.String("diff", "-", "path to a unified diff file, or - for stdin")
	outPath := fs.String("out", "-", "path to write the JSON report, or - for stdout")
	maxDepth := fs.Int("max-depth", score.Defaults().MaxDepth, "max CALLS hops to traverse when computing caller fan-in")
	decay := fs.Float64("decay", score.Defaults().Decay, "per-extra-hop weight decay for indirect callers (0,1]")
	blastWeight := fs.Float64("blast-weight", blastradius.DefaultWeights().BlastRadius, "weight of BlastRadius in the Combined score")
	priorityWeight := fs.Float64("priority-weight", blastradius.DefaultWeights().ReviewPriority, "weight of ReviewPriority in the Combined score")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if *project == "" {
		return fmt.Errorf("--project is required")
	}

	var diffBytes []byte
	var err error
	if *diffPath == "" || *diffPath == "-" {
		diffBytes, err = io.ReadAll(os.Stdin)
	} else {
		diffBytes, err = os.ReadFile(*diffPath)
	}
	if err != nil {
		return fmt.Errorf("reading diff: %w", err)
	}
	if len(diffBytes) == 0 {
		return fmt.Errorf("empty diff input")
	}

	opts := blastradius.Options{
		Score:   score.Config{MaxDepth: *maxDepth, Decay: *decay},
		Weights: blastradius.Weights{BlastRadius: *blastWeight, ReviewPriority: *priorityWeight},
	}
	report, err := blastradius.ScoreDiff(context.Background(), diffBytes, *project, opts)
	if err != nil {
		return err
	}

	out := os.Stdout
	if *outPath != "" && *outPath != "-" {
		f, err := os.Create(*outPath)
		if err != nil {
			return fmt.Errorf("creating %s: %w", *outPath, err)
		}
		defer f.Close()
		out = f
	}
	return writeJSON(out, report)
}

func writeJSON(w io.Writer, v any) error {
	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	return enc.Encode(v)
}
