// Package terminology implements dbctx's optional, user-controlled
// terminology layer: a mapping from domain vocabulary (abbreviations,
// acronyms, business jargon, natural-language descriptions) that a human
// might type into a query, to the exact dbctx schema object it refers to.
//
// This is deliberately independent from both the deterministic lexical
// index and the optional semantic embedding index (see internal/semantic).
// Embeddings are good at general paraphrase/synonym recall; they are not
// reliable for domain-specific abbreviations a model has never seen used
// the way a particular organization uses them (e.g. "LOC" -> "lines of
// code" -> metrics.loc). Terminology closes that gap, but only with
// mappings a human has actually approved — dbctx never invents them.
//
// The intended workflow is:
//
//  1. GeneratePrompt renders the full schema plus instructions into a
//     self-contained prompt.
//  2. The user pastes that into an LLM of their choice (Claude, GPT,
//     Gemini, ...) and works through it interactively, accepting or
//     rejecting proposed mappings.
//  3. The LLM's output — a JSON document in the format this package
//     defines — is fed to Import, which validates every entry against the
//     actual schema before persisting anything.
package terminology

// Entry is one persisted (alias -> target) mapping, as stored in the
// `terminology` table and returned by List for inspection.
type Entry struct {
	ID           int64  `json:"id"`
	Term         string `json:"term"`
	Alias        string `json:"alias"`
	TargetTable  string `json:"target_table"`
	TargetColumn string `json:"target_column,omitempty"`
	TargetPath   string `json:"target_path,omitempty"`
	Source       string `json:"source"`
	ImportedAt   string `json:"imported_at,omitempty"`
}

// Target renders the entry's target back into the "table",
// "table.column", or "table.column:$.path" notation Import accepts —
// useful for round-tripping or displaying an entry compactly.
func (e Entry) Target() string {
	return formatTarget(e.TargetTable, e.TargetColumn, e.TargetPath)
}

// TermGroup is the unit of input Import expects: one term with all of its
// human-language aliases and the exact schema objects it refers to. This
// mirrors the format GeneratePrompt asks the external LLM to produce.
//
//	{
//	  "term": "loc",
//	  "aliases": ["line of code", "lines of code", "source lines of code"],
//	  "targets": ["metrics.loc"]
//	}
type TermGroup struct {
	Term    string   `json:"term"`
	Aliases []string `json:"aliases"`
	Targets []string `json:"targets"`
}

// RejectedEntry records one (term, alias, target) combination Import
// refused to persist, and why — so a bad import is inspectable rather
// than silently dropping data.
type RejectedEntry struct {
	Term   string `json:"term"`
	Alias  string `json:"alias"`
	Target string `json:"target"`
	Reason string `json:"reason"`
}

// ImportResult summarizes what Import did.
type ImportResult struct {
	Accepted int             `json:"accepted"`
	Rejected []RejectedEntry `json:"rejected,omitempty"`
}
