package deepwiki

import (
	"strings"
	"testing"
)

func TestPostProcessWikiContentRebuildsDetails(t *testing.T) {
	content := "Some intro.\n\n<details>\n<summary>Relevant source files</summary>\nold\n</details>\n\nBody."
	got := postProcessWikiContent(content, []string{"a.go", "b/c.py"})

	if !strings.Contains(got, "<summary>Relevant source files</summary>") {
		t.Errorf("missing details block:\n%s", got)
	}
	if !strings.Contains(got, "`a.go`") || !strings.Contains(got, "`b/c.py`") {
		t.Errorf("details block missing file paths:\n%s", got)
	}
	if strings.Contains(got, ">old<") {
		t.Errorf("old details content not replaced:\n%s", got)
	}
}

func TestPostProcessWikiContentResolvesCitations(t *testing.T) {
	content := "See Sources: [internal/core.go:5-10]() for details."
	got := postProcessWikiContent(content, []string{"internal/core.go"})

	if !strings.Contains(got, "`internal/core.go:5-10`") {
		t.Errorf("citation not normalized:\n%s", got)
	}
	if strings.Contains(got, "[]()") {
		t.Errorf("empty citation parens remain:\n%s", got)
	}
}

func TestPostProcessWikiContentBasenameLookup(t *testing.T) {
	content := "Sources: [core.go:3]()"
	got := postProcessWikiContent(content, []string{"internal/core.go"})

	if !strings.Contains(got, "`internal/core.go:3`") {
		t.Errorf("basename citation not resolved:\n%s", got)
	}
}

func TestStripMarkdownFences(t *testing.T) {
	in := "```markdown\n# Hello\n```"
	got := stripMarkdownFences(in)
	if strings.TrimSpace(got) != "# Hello" {
		t.Errorf("stripMarkdownFences = %q", got)
	}
}
