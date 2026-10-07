package deepwiki

import (
	"encoding/xml"
	"errors"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
)

var (
	errNoWikiStructure = errors.New("no valid <wiki_structure> XML found in response")
	errNoPages         = errors.New("wiki structure contained no pages")
)

// defaultExcludedDirs mirrors DeepWiki-Open's repo.json file_filters.
var defaultExcludedDirs = []string{
	".venv", "venv", "env", "virtualenv", "node_modules", "bower_components",
	"jspm_packages", ".git", ".svn", ".hg", ".bzr", "vendor", "__pycache__",
	".pytest_cache", ".mypy_cache", ".ruff_cache", ".coverage", "dist", "build",
	"out", "target", "bin", "obj", "docs", "_docs", "site-docs", "_site",
	".idea", ".vscode", ".vs", ".eclipse", ".settings", "logs", "log", "tmp",
	"temp",
}

// defaultExcludedFiles and suffix list mirror repo.json (lock/config/build
// artifacts are never documentation-relevant).
var defaultExcludedFiles = []string{
	"yarn.lock", "pnpm-lock.yaml", "npm-shrinkwrap.json", "poetry.lock",
	"Pipfile.lock", "requirements.txt.lock", "Cargo.lock", "composer.lock",
	".DS_Store", "Thumbs.db", "desktop.ini", ".gitignore", ".gitattributes",
	".gitmodules", ".gitlab-ci.yml", ".prettierrc", ".eslintrc",
	".eslintignore", ".stylelintrc", ".editorconfig", ".jshintrc", ".pylintrc",
	".flake8", "mypy.ini", "pyproject.toml", "tsconfig.json",
	"webpack.config.js", "babel.config.js", "rollup.config.js",
	"jest.config.js", "karma.conf.js", "vite.config.js", "next.config.js",
}

var defaultExcludedFileSuffixes = []string{
	".lock", ".min.js", ".min.css", ".bundle.js", ".bundle.css", ".map", ".gz",
	".zip", ".tar", ".tgz", ".rar", ".7z", ".iso", ".dmg", ".img", ".msix",
	".appx", ".ipa", ".deb", ".rpm", ".msi", ".exe", ".dll", ".so", ".dylib",
	".o", ".obj", ".jar", ".war", ".ear", ".class", ".pyc", ".pyd", ".pyo",
	".a", ".lib", ".lo", ".la", ".slo", ".dSYM",
}

var codeExtensions = []string{
	".py", ".js", ".ts", ".java", ".cpp", ".c", ".h", ".hpp", ".go", ".rs",
	".jsx", ".tsx", ".html", ".css", ".php", ".swift", ".cs",
}

var docExtensions = []string{".md", ".txt", ".rst", ".json", ".yaml", ".yml"}

var allowedExtensions = append(append([]string{}, codeExtensions...), docExtensions...)

func containsString(haystack []string, needle string) bool {
	for _, h := range haystack {
		if h == needle {
			return true
		}
	}
	return false
}

func hasSuffixIn(list []string, s string) bool {
	for _, suffix := range list {
		if strings.HasSuffix(s, suffix) {
			return true
		}
	}
	return false
}

// readRepoFileTree walks a repo directory and returns (sorted repo-relative
// file list, README text). Ported from DeepWiki-Open's structure.py.
func readRepoFileTree(root string) ([]string, string, error) {
	var files []string

	err := filepath.Walk(root, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return nil // skip unreadable entries
		}
		if info.IsDir() {
			if path != root && containsString(defaultExcludedDirs, info.Name()) {
				return filepath.SkipDir
			}
			return nil
		}

		rel, err := filepath.Rel(root, path)
		if err != nil {
			return nil
		}
		rel = filepath.ToSlash(rel)

		name := info.Name()
		if containsString(defaultExcludedFiles, name) {
			return nil
		}
		if hasSuffixIn(defaultExcludedFileSuffixes, name) {
			return nil
		}
		ext := strings.ToLower(filepath.Ext(name))
		if !containsString(allowedExtensions, ext) {
			return nil
		}
		files = append(files, rel)
		return nil
	})
	if err != nil {
		return nil, "", err
	}

	sort.Slice(files, func(i, j int) bool { return len(files[i]) < len(files[j]) })

	readme := ""
	for _, f := range files {
		base := strings.TrimSuffix(filepath.Base(f), filepath.Ext(filepath.Base(f)))
		if strings.HasSuffix(strings.ToLower(base), "readme") {
			if b, err := os.ReadFile(filepath.Join(root, f)); err == nil {
				readme = string(b)
			}
			break
		}
	}

	return files, readme, nil
}

// normalizeImportance coerces an importance string to high|medium|low.
func normalizeImportance(value string) string {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "high", "medium", "low":
		return strings.ToLower(strings.TrimSpace(value))
	default:
		return "medium"
	}
}

// XML schema for the strict parse. Matches DeepWiki-Open's structure format.
type xmlWikiStructure struct {
	XMLName     xml.Name     `xml:"wiki_structure"`
	Title       string       `xml:"title"`
	Description string       `xml:"description"`
	Sections    []xmlSection `xml:"sections>section"`
	Pages       []xmlPage    `xml:"pages>page"`
}

type xmlSection struct {
	ID          string   `xml:"id,attr"`
	Title       string   `xml:"title"`
	Pages       []string `xml:"pages>page_ref"`
	Subsections []string `xml:"subsections>section_ref"`
}

type xmlPage struct {
	ID           string   `xml:"id,attr"`
	Title        string   `xml:"title"`
	Description  string   `xml:"description"`
	Importance   string   `xml:"importance"`
	FilePaths    []string `xml:"relevant_files>file_path"`
	RelatedPages []string `xml:"related_pages>related"`
}

// parseWikiStructure parses the LLM's XML response into a WikiStructure.
// Robust against markdown fences, control chars, bare '&', and truncated
// responses, with a regex fallback — a port of structure.py.
func parseWikiStructure(text string, comprehensive bool) (*WikiStructure, error) {
	text = regexp.MustCompile(`(?i)^` + "```" + `(?:xml)?\s*`).ReplaceAllString(strings.TrimSpace(text), "")
	text = regexp.MustCompile("```\\s*$").ReplaceAllString(text, "")

	xmlText := ""
	if m := regexp.MustCompile(`(?s)<wiki_structure>.*?</wiki_structure>`).FindString(text); m != "" {
		xmlText = m
	} else {
		openMatch := regexp.MustCompile(`(?s)<wiki_structure>.*`).FindString(text)
		if openMatch == "" {
			return nil, errNoWikiStructure
		}
		xmlText = openMatch + "\n</wiki_structure>"
	}

	xmlText = regexp.MustCompile(`[\x00-\x08\x0B\x0C\x0E-\x1F\x7F]`).ReplaceAllString(xmlText, "")
	xmlText = escapeBareAmpersands(xmlText)

	if st, err := parseWikiStructureStrict(xmlText, comprehensive); err == nil && len(st.Pages) > 0 {
		return st, nil
	}

	st := parseWikiStructureRegex(xmlText, comprehensive)
	if len(st.Pages) == 0 {
		return nil, errNoPages
	}
	if st.ID == "" {
		st.ID = "wiki"
	}
	return st, nil
}

func parseWikiStructureStrict(xmlText string, comprehensive bool) (*WikiStructure, error) {
	var x xmlWikiStructure
	if err := xml.Unmarshal([]byte(xmlText), &x); err != nil {
		return nil, err
	}

	st := &WikiStructure{ID: "wiki"}
	st.Title = strings.TrimSpace(x.Title)
	st.Description = strings.TrimSpace(x.Description)

	for i, p := range x.Pages {
		id := strings.TrimSpace(p.ID)
		if id == "" {
			id = "page-" + itoa(i+1)
		}
		st.Pages = append(st.Pages, WikiPage{
			ID:           id,
			Title:        strings.TrimSpace(p.Title),
			FilePaths:    trimAll(p.FilePaths),
			Importance:   normalizeImportance(p.Importance),
			RelatedPages: trimAll(p.RelatedPages),
		})
	}

	if comprehensive {
		referenced := map[string]bool{}
		for i, s := range x.Sections {
			id := strings.TrimSpace(s.ID)
			if id == "" {
				id = "section-" + itoa(i+1)
			}
			subs := trimAll(s.Subsections)
			st.Sections = append(st.Sections, WikiSection{
				ID:          id,
				Title:       strings.TrimSpace(s.Title),
				Pages:       trimAll(s.Pages),
				Subsections: subs,
			})
			for _, sub := range subs {
				referenced[sub] = true
			}
		}
		for _, s := range st.Sections {
			if !referenced[s.ID] {
				st.RootSections = append(st.RootSections, s.ID)
			}
		}
	}

	return st, nil
}

func trimAll(in []string) []string {
	out := make([]string, 0, len(in))
	for _, s := range in {
		if t := strings.TrimSpace(s); t != "" {
			out = append(out, t)
		}
	}
	return out
}

var (
	pageBlockRegex   = regexp.MustCompile(`(?s)<page\b.*?</page>`)
	pageIDRegex      = regexp.MustCompile(`(?s)<page\s+id="([^"]+)"`)
	pageIDRegexQuote = regexp.MustCompile(`(?s)<page\s+id='([^']+)'`)
)

func tagTextRegex(tag string) *regexp.Regexp {
	return regexp.MustCompile(`(?s)<` + tag + `>(.*?)</` + tag + `>`)
}

func firstTagText(block, tag string) string {
	m := tagTextRegex(tag).FindStringSubmatch(block)
	if len(m) > 1 {
		return strings.TrimSpace(m[1])
	}
	return ""
}

func allTagText(block, tag string) []string {
	var out []string
	for _, m := range tagTextRegex(tag).FindAllStringSubmatch(block, -1) {
		if len(m) > 1 && strings.TrimSpace(m[1]) != "" {
			out = append(out, strings.TrimSpace(m[1]))
		}
	}
	return out
}

// parseWikiStructureRegex recovers complete <page>/<section> blocks when
// strict XML parsing fails (e.g. a truncated response).
func parseWikiStructureRegex(xmlText string, comprehensive bool) *WikiStructure {
	st := &WikiStructure{}
	st.Title = firstTagText(xmlText, "title")
	st.Description = firstTagText(xmlText, "description")

	for i, block := range pageBlockRegex.FindAllString(xmlText, -1) {
		id := ""
		if m := pageIDRegex.FindStringSubmatch(block); len(m) > 1 {
			id = m[1]
		} else if m := pageIDRegexQuote.FindStringSubmatch(block); len(m) > 1 {
			id = m[1]
		}
		if id == "" {
			id = "page-" + itoa(i+1)
		}
		st.Pages = append(st.Pages, WikiPage{
			ID:           id,
			Title:        firstTagText(block, "title"),
			FilePaths:    allTagText(block, "file_path"),
			Importance:   normalizeImportance(firstTagText(block, "importance")),
			RelatedPages: allTagText(block, "related"),
		})
	}

	if comprehensive {
		st.Sections, st.RootSections = parseSectionsRegex(xmlText)
	}
	return st
}

var (
	sectionBlockRegex = regexp.MustCompile(`(?s)<section\b.*?</section>`)
	sectionIDRegex    = regexp.MustCompile(`(?s)<section\s+id="([^"]+)"`)
)

func parseSectionsRegex(xmlText string) ([]WikiSection, []string) {
	var sections []WikiSection
	referenced := map[string]bool{}

	for i, block := range sectionBlockRegex.FindAllString(xmlText, -1) {
		id := ""
		if m := sectionIDRegex.FindStringSubmatch(block); len(m) > 1 {
			id = m[1]
		}
		if id == "" {
			id = "section-" + itoa(i+1)
		}
		subs := allTagText(block, "section_ref")
		sections = append(sections, WikiSection{
			ID:          id,
			Title:       firstTagText(block, "title"),
			Pages:       allTagText(block, "page_ref"),
			Subsections: subs,
		})
		for _, s := range subs {
			referenced[s] = true
		}
	}

	var roots []string
	for _, s := range sections {
		if !referenced[s.ID] {
			roots = append(roots, s.ID)
		}
	}
	return sections, roots
}

func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	var digits []byte
	for n > 0 {
		digits = append([]byte{byte('0' + n%10)}, digits...)
		n /= 10
	}
	return string(digits)
}

// entityRe matches a well-formed XML entity so escapeBareAmpersands can leave
// it untouched while escaping every other '&' (RE2 has no lookahead).
var entityRe = regexp.MustCompile(`&(amp|lt|gt|quot|apos|#\d+|#x[0-9a-fA-F]+);`)

// escapeBareAmpersands escapes '&' characters that are not already part of a
// valid XML entity, so the strict encoding/xml parse succeeds.
func escapeBareAmpersands(s string) string {
	var b strings.Builder
	last := 0
	for _, loc := range entityRe.FindAllStringIndex(s, -1) {
		b.WriteString(strings.ReplaceAll(s[last:loc[0]], "&", "&amp;"))
		b.WriteString(s[loc[0]:loc[1]])
		last = loc[1]
	}
	b.WriteString(strings.ReplaceAll(s[last:], "&", "&amp;"))
	return b.String()
}
