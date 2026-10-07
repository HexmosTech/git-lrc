package deepwiki

import (
	"regexp"
	"strings"
)

// Content post-processing for the generated markdown. git-lrc is local-first
// (no hosted blob URLs), so citations are normalized to inline code spans
// rather than web links — the same intent as DeepWiki-Open's content.py, but
// with local rendering in mind.

var detailsRe = regexp.MustCompile(`(?is)<details>\s*<summary>\s*Relevant source files\s*</summary>.*?</details>`)

// stripMarkdownFences removes a leading ```markdown fence and a trailing ```.
func stripMarkdownFences(content string) string {
	content = regexp.MustCompile(`(?i)^` + "```markdown" + `\s*`).ReplaceAllString(content, "")
	content = regexp.MustCompile("```\\s*$").ReplaceAllString(content, "")
	return content
}

// buildDetailsBlock renders the "Relevant source files" block from known
// file paths as a code-span list (no broken local links).
func buildDetailsBlock(filePaths []string) string {
	var b strings.Builder
	b.WriteString("<details>\n<summary>Relevant source files</summary>\n\n")
	b.WriteString("The following files were used as context for generating this wiki page:\n\n")
	for _, p := range filePaths {
		b.WriteString("- `" + p + "`\n")
	}
	b.WriteString("</details>")
	return b.String()
}

var (
	// genericRe matches `[path.ext]()`, `[path.ext:line]()`, `[path.ext:line-end]()`.
	genericRe = regexp.MustCompile(`\[([^\[\]\s()]+?\.[A-Za-z0-9]+)(?::(\d+)(?:-(\d+))?)?\]\(\)`)
	// prefixedRe matches `[Sources: path.ext:line]()`.
	prefixedRe = regexp.MustCompile(`(?i)\[(Sources?|Source):\s*([^\[\]\s():]+?)(?::(\d+)(?:-(\d+))?)?\]\(\)`)
	// strayParensRe matches a redundant `()` immediately after a completed link.
	strayParensRe = regexp.MustCompile(`(\]\([^)\s]+\))\(\)`)
)

func codeSpan(path, start, end string) string {
	switch {
	case end != "":
		return "`" + path + ":" + start + "-" + end + "`"
	case start != "":
		return "`" + path + ":" + start + "`"
	default:
		return "`" + path + "`"
	}
}

// postProcessWikiContent rebuilds the <details> block and normalizes the
// empty-parenthesis citation forms the model emits into plain inline code
// spans (local repos have no web URL to link to).
func postProcessWikiContent(content string, filePaths []string) string {
	processed := content

	if len(filePaths) > 0 {
		details := buildDetailsBlock(filePaths)
		if detailsRe.MatchString(processed) {
			processed = detailsRe.ReplaceAllString(processed, details)
		} else {
			processed = details + "\n\n" + processed
		}
	}

	// Resolve citations against the known filePaths (longest first).
	byBasename := map[string]string{}
	for _, p := range filePaths {
		base := p[strings.LastIndex(p, "/")+1:]
		if _, ok := byBasename[base]; !ok {
			byBasename[base] = p
		}
	}

	if len(filePaths) > 0 {
		sorted := make([]string, len(filePaths))
		copy(sorted, filePaths)
		// sort by length descending for alternation
		for i := 0; i < len(sorted); i++ {
			for j := i + 1; j < len(sorted); j++ {
				if len(sorted[j]) > len(sorted[i]) {
					sorted[i], sorted[j] = sorted[j], sorted[i]
				}
			}
		}
		alternation := ""
		for i, p := range sorted {
			if i > 0 {
				alternation += "|"
			}
			alternation += regexp.QuoteMeta(p)
		}
		knownRe := regexp.MustCompile(`\[(` + alternation + `)(?::(\d+)(?:-(\d+))?)?\]\(\)`)
		processed = knownRe.ReplaceAllStringFunc(processed, func(m string) string {
			sm := knownRe.FindStringSubmatch(m)
			if len(sm) < 2 {
				return m
			}
			return codeSpan(sm[1], sm[2], sm[3])
		})
	}

	// Resolve remaining file-path-looking empty citations, with basename lookup.
	processed = genericRe.ReplaceAllStringFunc(processed, func(m string) string {
		sm := genericRe.FindStringSubmatch(m)
		if len(sm) < 2 {
			return m
		}
		path := sm[1]
		if !strings.Contains(path, "/") {
			if full, ok := byBasename[path]; ok {
				path = full
			}
		}
		return codeSpan(path, sm[2], sm[3])
	})

	// Resolve `[Sources: barename:line]()` via basename lookup.
	if len(filePaths) > 0 {
		processed = prefixedRe.ReplaceAllStringFunc(processed, func(m string) string {
			sm := prefixedRe.FindStringSubmatch(m)
			if len(sm) < 3 {
				return m
			}
			prefix, token := sm[1], sm[2]
			fullPath := token
			if !strings.Contains(token, "/") {
				if p, ok := byBasename[token]; ok {
					fullPath = p
				} else {
					return m
				}
			}
			return prefix + ": " + codeSpan(fullPath, sm[3], sm[4])
		})
	}

	// Strip a redundant empty `()` after a completed link.
	processed = strayParensRe.ReplaceAllString(processed, "$1")

	return processed
}
