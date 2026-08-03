package main

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"time"

	"github.com/HexmosTech/blastradius"
)

func main() {
	if len(os.Args) < 3 {
		fmt.Fprintln(os.Stderr, "usage: genfixture <repo-path> <project-name> [git-range]")
		os.Exit(1)
	}
	repoPath := os.Args[1]
	projName := os.Args[2]
	gitRange := "HEAD~3...HEAD"
	if len(os.Args) > 3 {
		gitRange = os.Args[3]
	}

	diffBytes, err := gitDiff(repoPath, gitRange)
	if err != nil {
		fmt.Fprintf(os.Stderr, "git diff: %v\n", err)
		os.Exit(1)
	}
	if len(diffBytes) == 0 {
		fmt.Fprintln(os.Stderr, "empty diff — no changes in range")
		os.Exit(1)
	}
	fmt.Fprintf(os.Stderr, "diff: %d bytes, scoring with project %s...\n", len(diffBytes), projName)

	report, err := blastradius.ScoreDiff(context.Background(), diffBytes, projName)
	if err != nil {
		fmt.Fprintf(os.Stderr, "score diff: %v\n", err)
		os.Exit(1)
	}

	payload := map[string]interface{}{
		"status":      "ready",
		"generatedAt": time.Now().UTC().Format(time.RFC3339Nano),
		"report":      report,
	}
	out, err := json.MarshalIndent(payload, "", "  ")
	if err != nil {
		fmt.Fprintf(os.Stderr, "marshal: %v\n", err)
		os.Exit(1)
	}
	fmt.Print(string(out))
}

func gitDiff(repoPath, gitRange string) ([]byte, error) {
	cmd := exec.Command("git", "-C", repoPath, "diff", "--unified=3", gitRange)
	var stdout, stderr strings.Builder
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		return nil, fmt.Errorf("%w: %s", err, stderr.String())
	}
	return []byte(stdout.String()), nil
}
