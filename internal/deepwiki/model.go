// Package deepwiki ports the DeepWiki-Open wiki-generation pipeline into
// git-lrc: it walks a local repository, asks an LLM (via a Completer) for a
// wiki structure and then for each page's markdown, and post-processes the
// citations. It is provider-agnostic — the LLM itself is supplied by the
// network package (see network/deepwiki_llm.go).
package deepwiki

// TaskStatus tracks the coarse phases of a generation run, mirroring
// DeepWiki-Open's task state machine so the UI/CLI can surface progress.
type TaskStatus string

const (
	TaskPending             TaskStatus = "pending"
	TaskDeterminingStructure TaskStatus = "determining_structure"
	TaskGenerating          TaskStatus = "generating"
	TaskCompleted           TaskStatus = "completed"
	TaskFailed              TaskStatus = "failed"
)

// WikiPage is one generated page. JSON tags match DeepWiki-Open's wire format.
type WikiPage struct {
	ID           string   `json:"id"`
	Title        string   `json:"title"`
	Content      string   `json:"content"`
	FilePaths    []string `json:"filePaths"`
	Importance   string   `json:"importance"`
	RelatedPages []string `json:"relatedPages"`
}

// WikiSection groups pages (comprehensive mode only).
type WikiSection struct {
	ID          string   `json:"id"`
	Title       string   `json:"title"`
	Pages       []string `json:"pages"`
	Subsections []string `json:"subsections,omitempty"`
}

// WikiStructure is the parsed <wiki_structure> response. After generation,
// each page's Content field is filled with the page markdown.
type WikiStructure struct {
	ID           string        `json:"id"`
	Title        string        `json:"title"`
	Description  string        `json:"description"`
	Pages        []WikiPage    `json:"pages"`
	Sections     []WikiSection `json:"sections,omitempty"`
	RootSections []string      `json:"rootSections,omitempty"`
}

// Wiki is the persisted artifact for one repo at one ref.
type Wiki struct {
	Structure   WikiStructure `json:"wiki_structure"`
	Ref         string        `json:"ref"`
	TreeHash    string        `json:"tree_hash,omitempty"`
	GeneratedAt int64         `json:"generated_at"` // Unix milliseconds
	Provider    string        `json:"provider,omitempty"`
	Model       string        `json:"model,omitempty"`
}
