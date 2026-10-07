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

func TestSanitizeMermaidSource(t *testing.T) {
	cases := []struct {
		name string
		in   string
		want string
	}{
		{
			name: "backslash-escaped quote in quoted label",
			in:   `C["Call Greet(\"world\")"]`,
			want: `C["Call Greet(&quot;world&quot;)"]`,
		},
		{
			name: "raw quote in unquoted label",
			in:   `A[Input: name = "world"]`,
			want: `A["Input: name = &quot;world&quot;"]`,
		},
		{
			name: "already-quoted label is untouched",
			in:   `B["main()"]`,
			want: `B["main()"]`,
		},
		{
			name: "edge label with escaped quote",
			in:   `A -->|"calls Greet(\"world\")"| B`,
			want: `A -->|"calls Greet(&quot;world&quot;)"| B`,
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := sanitizeMermaidSource(c.in); got != c.want {
				t.Errorf("sanitizeMermaidSource(%q) = %q, want %q", c.in, got, c.want)
			}
		})
	}
}

func TestSanitizeMermaidBlocksOnlyTouchesMermaidFences(t *testing.T) {
	in := "Note: keep \\\"this\\\" backslash-quote.\n\n```mermaid\ngraph TD\n    A[Input: name = \"world\"]\n```\n\nAfter `\"x\"` stays."
	got := sanitizeMermaidBlocks(in)

	if !strings.Contains(got, `A["Input: name = &quot;world&quot;"]`) {
		t.Errorf("mermaid block not sanitized:\n%s", got)
	}
	// Non-mermaid content must remain untouched.
	if !strings.Contains(got, `keep \"this\" backslash-quote`) {
		t.Errorf("non-mermaid backslash-quote was altered:\n%s", got)
	}
	if !strings.Contains(got, "After `\"x\"` stays") {
		t.Errorf("non-mermaid inline quote was altered:\n%s", got)
	}
}
