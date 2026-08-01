// Package diffparse parses unified diff text (as produced by `git diff`)
// into files and hunks. It is intentionally self-contained (no dependency on
// git-lrc's internal packages) so the blastradius module can be copied into
// another project wholesale.
package diffparse

import (
	"bufio"
	"bytes"
	"regexp"
	"strconv"
	"strings"
)

// Hunk is one `@@ ... @@` chunk of a unified diff.
type Hunk struct {
	// Header is the full hunk header line, e.g. "@@ -12,6 +12,8 @@ func Foo()".
	Header   string
	OldStart int
	OldLines int
	NewStart int
	NewLines int
	// Content is the hunk body: context/added/removed lines, each still
	// prefixed with its leading ' '/'+'/'-' character, newline-joined.
	Content string
}

// File is one file's worth of hunks from a diff.
type File struct {
	// Path is the file's post-change path (relative, no "a/"/"b/" prefix).
	// For a deleted file this is empty; use OldPath instead.
	Path string
	// OldPath is the file's pre-change path. Empty unless the file was
	// renamed or deleted.
	OldPath   string
	IsNew     bool
	IsDeleted bool
	IsRename  bool
	IsBinary  bool
	Hunks     []Hunk
}

var (
	diffGitRe  = regexp.MustCompile(`^diff --git a/(.*) b/(.*)$`)
	hunkHeadRe = regexp.MustCompile(`^@@ -(\d+)(?:,(\d+))? \+(\d+)(?:,(\d+))? @@.*$`)
	oldPathRe  = regexp.MustCompile(`^--- (?:a/(.*)|/dev/null)\s*$`)
	newPathRe  = regexp.MustCompile(`^\+\+\+ (?:b/(.*)|/dev/null)\s*$`)
)

// Parse parses unified diff bytes (e.g. the output of `git diff`) into a
// slice of File, preserving diff order.
func Parse(diff []byte) ([]File, error) {
	var files []File
	var cur *File
	var curHunk *Hunk
	var hunkBody []string

	flushHunk := func() {
		if cur == nil || curHunk == nil {
			return
		}
		curHunk.Content = strings.Join(hunkBody, "\n")
		cur.Hunks = append(cur.Hunks, *curHunk)
		curHunk = nil
		hunkBody = nil
	}
	flushFile := func() {
		flushHunk()
		if cur != nil {
			files = append(files, *cur)
			cur = nil
		}
	}

	scanner := bufio.NewScanner(bytes.NewReader(diff))
	scanner.Buffer(make([]byte, 0, 64*1024), 10*1024*1024)
	for scanner.Scan() {
		line := scanner.Text()

		if m := diffGitRe.FindStringSubmatch(line); m != nil {
			flushFile()
			cur = &File{Path: m[2], OldPath: m[1]}
			if m[1] != m[2] {
				cur.IsRename = true
			}
			continue
		}
		if cur == nil {
			// Content before the first "diff --git" (e.g. a leading commit
			// message when fed a `git show` style blob) is not a diff body;
			// skip it.
			continue
		}
		switch {
		case strings.HasPrefix(line, "Binary files ") || strings.HasPrefix(line, "GIT binary patch"):
			cur.IsBinary = true
			continue
		case strings.HasPrefix(line, "new file mode"):
			cur.IsNew = true
			continue
		case strings.HasPrefix(line, "deleted file mode"):
			cur.IsDeleted = true
			continue
		case strings.HasPrefix(line, "rename from "):
			cur.IsRename = true
			cur.OldPath = strings.TrimPrefix(line, "rename from ")
			continue
		case strings.HasPrefix(line, "rename to "):
			cur.IsRename = true
			cur.Path = strings.TrimPrefix(line, "rename to ")
			continue
		}
		if m := oldPathRe.FindStringSubmatch(line); m != nil {
			if m[1] != "" {
				cur.OldPath = m[1]
			}
			continue
		}
		if m := newPathRe.FindStringSubmatch(line); m != nil {
			if m[1] != "" {
				cur.Path = m[1]
			} else {
				cur.IsDeleted = true
			}
			continue
		}
		if m := hunkHeadRe.FindStringSubmatch(line); m != nil {
			flushHunk()
			curHunk = &Hunk{
				Header:   line,
				OldStart: atoi(m[1]),
				OldLines: atoiDefault(m[2], 1),
				NewStart: atoi(m[3]),
				NewLines: atoiDefault(m[4], 1),
			}
			continue
		}
		if curHunk != nil {
			hunkBody = append(hunkBody, line)
		}
		// Lines between a file header and its first hunk (e.g. "index
		// abc123..def456 100644") are structurally uninteresting; skipped.
	}
	flushFile()

	if err := scanner.Err(); err != nil {
		return nil, err
	}
	return files, nil
}

func atoi(s string) int {
	n, _ := strconv.Atoi(s)
	return n
}

func atoiDefault(s string, def int) int {
	if s == "" {
		return def
	}
	return atoi(s)
}
