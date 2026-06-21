package appcore

import (
	"fmt"
	"strings"

	"github.com/HexmosTech/git-lrc/internal/lrcrules"
	"github.com/urfave/cli/v2"
)

// RunConfigInit scaffolds .lrc/ under the repository root, creating only
// the files that don't already exist.
func RunConfigInit(c *cli.Context) error {
	repoRootPath, err := resolveRuntimeRepositoryPath()
	if err != nil {
		return fmt.Errorf("failed to resolve repository root: %w", err)
	}

	created, err := lrcrules.Init(repoRootPath)
	if err != nil {
		return fmt.Errorf("failed to scaffold .lrc/: %w", err)
	}

	if len(created) == 0 {
		fmt.Println(".lrc/ already exists; nothing to create.")
		return nil
	}

	fmt.Println("Created:")
	for _, path := range created {
		fmt.Printf("  %s\n", path)
	}
	return nil
}

// RunConfigCheck validates .lrc/ structure and the rules bundle entirely
// offline, exiting non-zero if any error-level issue is found.
func RunConfigCheck(c *cli.Context) error {
	repoRootPath, err := resolveRuntimeRepositoryPath()
	if err != nil {
		return fmt.Errorf("failed to resolve repository root: %w", err)
	}

	lrcDir, ok, err := lrcrules.Load(repoRootPath)
	if err != nil {
		return err
	}
	if !ok {
		return fmt.Errorf(".lrc/ not found; run 'lrc config init' to scaffold it")
	}

	var issues []lrcrules.Issue
	issues = append(issues, lrcrules.ValidateStructure(lrcDir)...)
	issues = append(issues, lrcrules.CheckIgnoreSyntax(lrcDir)...)

	_, charCount, bundleIssues := lrcrules.BuildRulesBundle(lrcDir)
	issues = append(issues, bundleIssues...)

	errorCount := 0
	for _, issue := range issues {
		fmt.Printf("[%s] %s: %s\n", strings.ToUpper(issue.Level), issue.Path, issue.Message)
		if issue.Level == "error" {
			errorCount++
		}
	}

	if len(issues) == 0 {
		fmt.Println("OK: .lrc/ looks good.")
	}
	fmt.Printf("rules bundle: %d/%d chars\n", charCount, lrcrules.CharLimit)

	if errorCount > 0 {
		return cli.Exit(fmt.Sprintf("found %d error(s) in .lrc/ configuration", errorCount), 1)
	}
	return nil
}

// RunConfigPreview prints the exact rules bundle LiveReview will assemble
// from .lrc/rules/*.md, computed entirely offline.
func RunConfigPreview(c *cli.Context) error {
	repoRootPath, err := resolveRuntimeRepositoryPath()
	if err != nil {
		return fmt.Errorf("failed to resolve repository root: %w", err)
	}

	lrcDir, ok, err := lrcrules.Load(repoRootPath)
	if err != nil {
		return err
	}
	if !ok {
		return fmt.Errorf(".lrc/ not found; run 'lrc config init' to scaffold it")
	}

	text, charCount, issues := lrcrules.BuildRulesBundle(lrcDir)
	if text == "" {
		fmt.Println("(no rules to send — rules/ is empty or missing)")
	} else {
		fmt.Println(text)
	}

	fmt.Println()
	fmt.Printf("%d/%d chars\n", charCount, lrcrules.CharLimit)
	for _, issue := range issues {
		fmt.Printf("[%s] %s: %s\n", strings.ToUpper(issue.Level), issue.Path, issue.Message)
	}
	return nil
}
