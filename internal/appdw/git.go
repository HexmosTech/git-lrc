package appdw

import (
	"archive/tar"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

// materializeRef returns a directory containing the given ref's tree: the
// working directory itself when ref resolves to the current HEAD, or a temp
// directory populated from `git archive` otherwise (so a branch/tag/commit
// that isn't checked out still documents its own content). The returned
// cleanup func removes the temp directory when work was done.
func materializeRef(repoPath, ref string) (string, func(), error) {
	ref = strings.TrimSpace(ref)
	if ref == "" {
		ref = "HEAD"
	}

	if head, err := exec.Command("git", "-C", repoPath, "rev-parse", "HEAD").Output(); err == nil {
		if refHash, err := exec.Command("git", "-C", repoPath, "rev-parse", ref+"^{commit}").Output(); err == nil {
			if strings.TrimSpace(string(head)) == strings.TrimSpace(string(refHash)) {
				return repoPath, func() {}, nil
			}
		}
	}

	tmp, err := os.MkdirTemp("", "lrc-dw-")
	if err != nil {
		return "", func() {}, err
	}
	cleanup := func() { _ = os.RemoveAll(tmp) }

	cmd := exec.Command("git", "-C", repoPath, "archive", ref)
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		cleanup()
		return "", func() {}, err
	}
	if err := cmd.Start(); err != nil {
		cleanup()
		return "", func() {}, err
	}

	tr := tar.NewReader(stdout)
	for {
		hdr, err := tr.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			_ = cmd.Wait()
			cleanup()
			return "", func() {}, err
		}
		target := filepath.Join(tmp, filepath.Clean(hdr.Name))
		if !strings.HasPrefix(target, tmp+string(os.PathSeparator)) {
			continue // guard against path traversal
		}
		switch hdr.Typeflag {
		case tar.TypeDir:
			_ = os.MkdirAll(target, 0o755)
		case tar.TypeReg:
			_ = os.MkdirAll(filepath.Dir(target), 0o755)
			f, err := os.Create(target)
			if err != nil {
				continue
			}
			_, _ = io.Copy(f, tr)
			_ = f.Close()
		}
	}
	if err := cmd.Wait(); err != nil {
		cleanup()
		return "", func() {}, err
	}
	return tmp, cleanup, nil
}

// gitRefInfo is the resolved ref context for the current repository.
type gitRefInfo struct {
	RepoPath string // absolute repo root
	Current  string // current branch name, or "HEAD" when detached
	TreeHash string // git tree hash of the current HEAD
}

// resolveRepo finds the git repo root from the current working directory.
func resolveRepo() (string, error) {
	out, err := exec.Command("git", "rev-parse", "--show-toplevel").Output()
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(string(out)), nil
}

// currentRef returns the checked-out branch name, or "HEAD" when detached.
func currentRef(repoPath string) string {
	out, err := exec.Command("git", "-C", repoPath, "rev-parse", "--abbrev-ref", "HEAD").Output()
	if err != nil {
		return "HEAD"
	}
	ref := strings.TrimSpace(string(out))
	if ref == "" || ref == "HEAD" {
		return "HEAD"
	}
	return ref
}

// treeHash returns the git tree hash for a ref ("" when unresolved).
func treeHash(repoPath, ref string) string {
	spec := ref
	if spec == "" || spec == "HEAD" {
		spec = "HEAD"
	}
	out, err := exec.Command("git", "-C", repoPath, "rev-parse", spec+"^{tree}").Output()
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(out))
}

// refsResult holds the branch/tag/commit browser data.
type refsResult struct {
	Branches []string `json:"branches"`
	Tags     []string `json:"tags"`
	Commits  []commit `json:"commits"`
	Current  string   `json:"current"`
}

type commit struct {
	Hash    string `json:"hash"`
	Subject string `json:"subject"`
}

// listRefs enumerates local branches, tags, and recent commits.
func listRefs(repoPath string, current string) refsResult {
	res := refsResult{Current: current, Branches: []string{}, Tags: []string{}, Commits: []commit{}}

	if out, err := exec.Command("git", "-C", repoPath, "for-each-ref", "--format=%(refname:short)", "refs/heads").Output(); err == nil {
		for _, l := range strings.Split(strings.TrimSpace(string(out)), "\n") {
			if l != "" {
				res.Branches = append(res.Branches, l)
			}
		}
	}
	if out, err := exec.Command("git", "-C", repoPath, "for-each-ref", "--format=%(refname:short)", "refs/tags").Output(); err == nil {
		for _, l := range strings.Split(strings.TrimSpace(string(out)), "\n") {
			if l != "" {
				res.Tags = append(res.Tags, l)
			}
		}
	}
	if out, err := exec.Command("git", "-C", repoPath, "log", "--oneline", "-30").Output(); err == nil {
		for _, l := range strings.Split(strings.TrimSpace(string(out)), "\n") {
			if l == "" {
				continue
			}
			hash, subject, _ := strings.Cut(l, " ")
			res.Commits = append(res.Commits, commit{Hash: hash, Subject: subject})
		}
	}
	return res
}
