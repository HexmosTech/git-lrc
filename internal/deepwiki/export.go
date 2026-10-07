package deepwiki

import (
	"fmt"
	"strings"
)

// Markdown renders the wiki as a single Markdown document with a table of
// contents and each page — the programmatic --output markdown path.
func (w *Wiki) Markdown() string {
	var b strings.Builder

	repoName := w.Ref
	if w.Structure.Title != "" {
		repoName = w.Structure.Title
	}
	fmt.Fprintf(&b, "# %s\n\n", repoName)
	if w.Structure.Description != "" {
		fmt.Fprintf(&b, "%s\n\n", w.Structure.Description)
	}

	b.WriteString("## Table of Contents\n\n")
	for _, p := range w.Structure.Pages {
		fmt.Fprintf(&b, "- [%s](#%s)\n", p.Title, p.ID)
	}
	b.WriteString("\n")

	for _, p := range w.Structure.Pages {
		fmt.Fprintf(&b, "<a id='%s'></a>\n\n## %s\n\n", p.ID, p.Title)

		if len(p.RelatedPages) > 0 {
			var related []string
			for _, rid := range p.RelatedPages {
				if title := pageTitle(w.Structure.Pages, rid); title != "" {
					related = append(related, fmt.Sprintf("[%s](#%s)", title, rid))
				}
			}
			if len(related) > 0 {
				fmt.Fprintf(&b, "### Related Pages\n\nRelated topics: %s\n\n", strings.Join(related, ", "))
			}
		}

		b.WriteString(p.Content)
		b.WriteString("\n\n---\n\n")
	}

	return b.String()
}

func pageTitle(pages []WikiPage, id string) string {
	for _, p := range pages {
		if p.ID == id {
			return p.Title
		}
	}
	return ""
}
