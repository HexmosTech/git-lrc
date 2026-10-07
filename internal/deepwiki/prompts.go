package deepwiki

import "fmt"

// languageNames maps a language code to the human name used in prompts.
// Ported from DeepWiki-Open's prompts.py.
var languageNames = map[string]string{
	"en":    "English",
	"ja":    "Japanese (日本語)",
	"zh":    "Mandarin Chinese (中文)",
	"zh-tw": "Traditional Chinese (繁體中文)",
	"es":    "Spanish (Español)",
	"kr":    "Korean (한국어)",
	"vi":    "Vietnamese (Tiếng Việt)",
	"pt-br": "Brazilian Portuguese (Português Brasileiro)",
	"fr":    "Français (French)",
	"ru":    "Русский (Russian)",
}

// LanguageName returns the display name for a language code, defaulting to
// English.
func LanguageName(language string) string {
	if name, ok := languageNames[language]; ok {
		return name
	}
	return "English"
}

// buildPagePrompt returns the page-generation prompt. fileLinks is the
// pre-built markdown list of "- [path](url)" lines; fileContents is the
// inlined content of those files (RAG retrieval is replaced with direct
// inlining in this port).
func buildPagePrompt(title, fileLinks, fileContents, language string) string {
	return fmt.Sprintf(`You are an expert technical writer and software architect.
Your task is to generate a comprehensive and accurate technical wiki page in Markdown format about a specific feature, system, or module within a given software project.

You will be given:
1. The "[WIKI_PAGE_TOPIC]" for the page you need to create.
2. A list of "[RELEVANT_SOURCE_FILES]" from the project that you MUST use as the sole basis for the content. The full content of these files is provided below in "[FILE_CONTENTS]".

CRITICAL STARTING INSTRUCTION:
The very first thing on the page MUST be a "<details>" block listing ALL the "[RELEVANT_SOURCE_FILES]" you used to generate the content. Do not provide any acknowledgements, disclaimers, apologies, or any other preface before the "<details>" block. JUST START with the "<details>" block.
Format the block EXACTLY like the following template, reproducing it verbatim (do not add line numbers, do not convert the links to plain text, do not add any other text):
<details>
<summary>Relevant source files</summary>

The following files were used as context for generating this wiki page:

%[1]s
</details>

Immediately after the "<details>" block, the main title of the page should be a H1 Markdown heading: "# %[2]s".

The "[WIKI_PAGE_TOPIC]" is: %[2]s

Based ONLY on the content of the "[RELEVANT_SOURCE_FILES]" provided in "[FILE_CONTENTS]":

1. **Introduction:** Start with a concise introduction (1-2 paragraphs) explaining the purpose, scope, and high-level overview of "%[2]s" within the context of the overall project. If relevant, and if information is available in the provided files, link to other potential wiki pages using the format "[Link Text](#page-anchor-or-id)".

2. **Detailed Sections:** Break down "%[2]s" into logical sections using H2 (##) and H3 (###) Markdown headings. For each section:
    * Explain the architecture, components, data flow, or logic relevant to the section's focus, as evidenced in the source files.
    * Identify key functions, classes, data structures, API endpoints, or configuration elements pertinent to that section.

3. **Mermaid Diagrams:**
    * EXTENSIVELY use Mermaid diagrams (e.g. "flowchart TD", "sequenceDiagram", "classDiagram", "erDiagram", "graph TD") to visually represent architectures, flows, relationships, and schemas found in the source files.
    * Ensure diagrams are accurate and directly derived from information in the "[RELEVANT_SOURCE_FILES]".
    * CRITICAL: All diagrams MUST follow strict vertical orientation:
       - Use "graph TD" (top-down) directive for flow diagrams
       - NEVER use "graph LR" (left-right)
     * NEVER escape double quotes with a backslash inside labels or edge text (do NOT write C["Call Greet(\"world\")"]). Use HTML entities instead (e.g. C["Call Greet(&quot;world&quot;)"]) or rephrase to avoid quotes.

4. **Tables:**
    * Use Markdown tables to summarize information such as key features or components and their descriptions, API endpoint parameters, configuration options, and data model fields.

5. **Code Snippets (ENTIRELY OPTIONAL):**
    * Include short, relevant code snippets directly from the "[RELEVANT_SOURCE_FILES]" to illustrate key implementation details, data structures, or configurations.
    * Ensure snippets are well-formatted within Markdown code blocks with appropriate language identifiers.

6. **Source Citations (EXTREMELY IMPORTANT):**
    * For EVERY piece of significant information, explanation, diagram, table entry, or code snippet, you MUST cite the specific source file(s) and relevant line numbers from which the information was derived.
    * Place citations at the end of the paragraph, under the diagram/table, or after the code snippet.
    * Use the EXACT format below, and ALWAYS use the FULL repository-relative path exactly as it appears in the "Relevant source files" list above — NEVER a bare filename:
        * Range: "Sources: [src/full/path/file.ext:start_line-end_line]()"
        * Single line: "Sources: [src/full/path/file.ext:line_number]()"
    * The word "Sources:" MUST be placed BEFORE the opening bracket, never inside it.
    * Leave the parentheses "()" EMPTY — they are resolved into real links automatically. Do not put a URL inside them.

7. **Technical Accuracy:** All information must be derived SOLELY from the "[RELEVANT_SOURCE_FILES]". Do not infer, invent, or use external knowledge about similar systems or common practices unless it's directly supported by the provided code.

8. **Clarity and Conciseness:** Use clear, professional, and concise technical language suitable for other developers working on or learning about the project.

IMPORTANT: Generate the content in %[3]s language.

Here is the "[RELEVANT_SOURCE_FILES]" list:
%[1]s

Here is the "[FILE_CONTENTS]":
%[4]s

Remember:
- Ground every claim in the provided source files.
- Prioritize accuracy and direct representation of the code's functionality and structure.
- Structure the document logically for easy understanding by other developers.
`, fileLinks, title, LanguageName(language), fileContents)
}

const conciseStructureFormat = `<wiki_structure>
  <title>[Overall title for the wiki]</title>
  <description>[Brief description of the repository]</description>
  <pages>
    <page id="page-1">
      <title>[Page title]</title>
      <description>[Brief description of what this page will cover]</description>
      <importance>high|medium|low</importance>
      <relevant_files>
        <file_path>[Path to a relevant file]</file_path>
      </relevant_files>
      <related_pages>
        <related>page-2</related>
      </related_pages>
    </page>
  </pages>
</wiki_structure>`

const comprehensiveStructureFormat = `<wiki_structure>
  <title>[Overall title for the wiki]</title>
  <description>[Brief description of the repository]</description>
  <sections>
    <section id="section-1">
      <title>[Section title]</title>
      <pages>
        <page_ref>page-1</page_ref>
      </pages>
      <subsections>
        <section_ref>section-2</section_ref>
      </subsections>
    </section>
  </sections>
  <pages>
    <page id="page-1">
      <title>[Page title]</title>
      <description>[Brief description of what this page will cover]</description>
      <importance>high|medium|low</importance>
      <relevant_files>
        <file_path>[Path to a relevant file]</file_path>
      </relevant_files>
      <related_pages>
        <related>page-2</related>
      </related_pages>
      <parent_section>section-1</parent_section>
    </page>
  </pages>
</wiki_structure>`

// buildStructurePrompt returns the structure-determination prompt.
func buildStructurePrompt(owner, repo, fileTree, readme string, comprehensive bool, language string) string {
	structureFormat := conciseStructureFormat
	pageCount := "4-6"
	kind := "concise"
	if comprehensive {
		structureFormat = comprehensiveStructureFormat
		pageCount = "8-12"
		kind = "comprehensive"
	}
	return fmt.Sprintf(`Analyze this repository %[1]s/%[2]s and create a wiki structure for it.

1. The complete file tree of the project:
<file_tree>
%[3]s
</file_tree>

2. The README file of the project:
<readme>
%[4]s
</readme>

I want to create a wiki for this repository. Determine the most logical structure for a wiki based on the repository's content.

IMPORTANT: The wiki content will be generated in %[5]s language.

When designing the wiki structure, include pages that would benefit from visual diagrams, such as:
- Architecture overviews
- Data flow descriptions
- Component relationships
- Process workflows
- State machines
- Class hierarchies

%[6]s

IMPORTANT FORMATTING INSTRUCTIONS:
- Return ONLY the valid XML structure specified above
- DO NOT wrap the XML in markdown code blocks (no %[7]s%[7]s%[7]s or %[7]s%[7]sxml)
- DO NOT include any explanation text before or after the XML
- Ensure the XML is properly formatted and valid
- Start directly with <wiki_structure> and end with </wiki_structure>

IMPORTANT:
1. Create %[8]s pages that would make a %[9]s wiki for this repository
2. Each page should focus on a specific aspect of the codebase (e.g., architecture, key features, setup)
3. The relevant_files should be actual files from the repository that would be used to generate that page
4. Return ONLY valid XML with the structure specified above, with no markdown code block delimiters`,
		owner, repo, fileTree, readme, LanguageName(language),
		structureFormat, "`", pageCount, kind)
}
