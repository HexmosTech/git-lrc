package appcore

import (
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"strings"

	"github.com/HexmosTech/git-lrc/gitops"
	"github.com/HexmosTech/git-lrc/internal/reviewapi"
)

// resolveReviewRepositoryPath returns the absolute worktree root when available.
// Bare repositories or other setups without a worktree root should surface an
// error so callers can omit the local-only UI affordance and fall back cleanly.
func resolveReviewRepositoryPath(resolveRepoRoot func() (string, error)) (string, error) {
	repoRootPath, err := resolveRepoRoot()
	if err != nil {
		return "", err
	}

	repoRootPath = strings.TrimSpace(repoRootPath)
	if repoRootPath == "" {
		return "", fmt.Errorf("repository root path is empty")
	}

	if !filepath.IsAbs(repoRootPath) {
		repoRootPath, err = filepath.Abs(repoRootPath)
		if err != nil {
			return "", fmt.Errorf("failed to normalize repository root path: %w", err)
		}
	}

	return filepath.Clean(repoRootPath), nil
}

func resolveRuntimeRepositoryPath() (string, error) {
	return resolveReviewRepositoryPath(reviewapi.ResolveRepoRoot)
}

func resolveReviewRepositoryName(explicitRepoName, repositoryPath string, getwd func() (string, error)) (string, error) {
	repoName := strings.TrimSpace(explicitRepoName)
	if repoName != "" {
		return repoName, nil
	}

	if repositoryPath != "" {
		return filepath.Base(repositoryPath), nil
	}

	currentDir, err := getwd()
	if err != nil {
		return "", fmt.Errorf("failed to get current directory: %w", err)
	}

	if !filepath.IsAbs(currentDir) {
		currentDir, err = filepath.Abs(currentDir)
		if err != nil {
			return "", fmt.Errorf("failed to normalize current directory: %w", err)
		}
	}

	return filepath.Base(filepath.Clean(currentDir)), nil
}

func resolveRuntimeRepositoryName(explicitRepoName, repositoryPath string) (string, error) {
	return resolveReviewRepositoryName(explicitRepoName, repositoryPath, os.Getwd)
}

// resolveRepoRemoteURL returns the git remote URL the review came from, used
// only to resolve the review to a connected LiveReview repository row for
// cross-repo blast-radius matching. It is best-effort: a missing remote (or a
// non-git directory) simply disables cross-repo enrichment for this run.
func resolveRepoRemoteURL(explicit, repoRoot string) string {
	if v := strings.TrimSpace(explicit); v != "" {
		return sanitizeRemoteRepoURL(v)
	}
	if strings.TrimSpace(repoRoot) == "" {
		return ""
	}
	for _, key := range []string{"remote.origin.url", "remote.upstream.url"} {
		if v, err := gitops.ReadGitConfig(repoRoot, key); err == nil {
			if v = strings.TrimSpace(v); v != "" {
				return sanitizeRemoteRepoURL(v)
			}
		}
	}
	return ""
}

// sanitizeRemoteRepoURL removes embedded credentials (e.g. a token in an
// https://user:token@host/... remote) before this value is sent to
// LiveReview and stored in review metadata. scp-like git@host:path remotes
// carry no secret and are left alone.
func sanitizeRemoteRepoURL(raw string) string {
	u, err := url.Parse(raw)
	if err != nil || u.User == nil {
		return raw
	}
	u.User = nil
	return u.String()
}
