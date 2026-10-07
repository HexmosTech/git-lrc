package deepwiki

import "testing"

func TestParseWikiStructureStrict(t *testing.T) {
	input := `Here is the structure:

<wiki_structure>
  <title>Demo Wiki</title>
  <description>A demo repository.</description>
  <pages>
    <page id="page-1">
      <title>Overview</title>
      <importance>high</importance>
      <relevant_files>
        <file_path>README.md</file_path>
        <file_path>main.go</file_path>
      </relevant_files>
      <related_pages>
        <related>page-2</related>
      </related_pages>
    </page>
    <page id="page-2">
      <title>Architecture</title>
      <importance>medium</importance>
      <relevant_files>
        <file_path>internal/core.go</file_path>
      </relevant_files>
    </page>
  </pages>
</wiki_structure>`

	st, err := parseWikiStructure(input, false)
	if err != nil {
		t.Fatalf("parseWikiStructure() error = %v", err)
	}
	if st.Title != "Demo Wiki" {
		t.Errorf("title = %q, want %q", st.Title, "Demo Wiki")
	}
	if len(st.Pages) != 2 {
		t.Fatalf("len(pages) = %d, want 2", len(st.Pages))
	}
	if st.Pages[0].Importance != "high" {
		t.Errorf("page-1 importance = %q, want high", st.Pages[0].Importance)
	}
	if len(st.Pages[0].FilePaths) != 2 || st.Pages[0].FilePaths[0] != "README.md" {
		t.Errorf("page-1 filePaths = %v", st.Pages[0].FilePaths)
	}
	if len(st.Pages[0].RelatedPages) != 1 || st.Pages[0].RelatedPages[0] != "page-2" {
		t.Errorf("page-1 relatedPages = %v", st.Pages[0].RelatedPages)
	}
}

func TestParseWikiStructureFencedAndTruncated(t *testing.T) {
	// Simulate a model that wraps output in fences and drops the closing tag.
	input := "```xml\n<wiki_structure>\n<title>T</title>\n<pages>\n<page id=\"p1\"><title>One</title></page>\n<page id=\"p2\"><title>Two</title></page>\n"
	st, err := parseWikiStructure(input, false)
	if err != nil {
		t.Fatalf("parseWikiStructure() error = %v", err)
	}
	if len(st.Pages) != 2 {
		t.Fatalf("len(pages) = %d, want 2", len(st.Pages))
	}
}

func TestParseWikiStructureNoStructure(t *testing.T) {
	if _, err := parseWikiStructure("no xml here at all", false); err == nil {
		t.Fatal("expected error for missing wiki_structure")
	}
}

func TestParseWikiStructureComprehensive(t *testing.T) {
	input := `<wiki_structure>
  <title>T</title>
  <sections>
    <section id="section-1"><title>Core</title>
      <pages><page_ref>page-1</page_ref></pages>
    </section>
  </sections>
  <pages>
    <page id="page-1"><title>One</title></page>
  </pages>
</wiki_structure>`
	st, err := parseWikiStructure(input, true)
	if err != nil {
		t.Fatalf("parseWikiStructure() error = %v", err)
	}
	if len(st.Sections) != 1 || st.Sections[0].Pages[0] != "page-1" {
		t.Errorf("sections = %+v", st.Sections)
	}
	if len(st.RootSections) != 1 || st.RootSections[0] != "section-1" {
		t.Errorf("rootSections = %v", st.RootSections)
	}
}
