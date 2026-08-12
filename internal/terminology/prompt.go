package terminology

import (
	"bytes"
	"fmt"

	"github.com/shrsv/dbctx/internal/db"
	"github.com/shrsv/dbctx/internal/report"
)

// GeneratePrompt renders a self-contained prompt that a user can paste into
// a large external LLM (Claude, GPT, Gemini, ...) to interactively derive
// a terminology dictionary for store's schema. dbctx never calls an LLM
// itself — this only produces text.
//
// The prompt embeds the full schema using report.ReportAll, the most
// complete compact schema/context renderer dbctx already has (tables,
// columns, relationships, state/categorical fields with observed values,
// and JSONB structure) — deliberately reused rather than inventing a
// second schema serialization just for this.
func GeneratePrompt(store *db.Store) (string, error) {
	var schemaBuf bytes.Buffer
	if err := report.ReportAll(&schemaBuf, store); err != nil {
		return "", fmt.Errorf("render schema: %w", err)
	}
	return fmt.Sprintf(promptTemplate, schemaBuf.String()), nil
}

const promptTemplate = `You are helping build a TERMINOLOGY DICTIONARY for a database
natural-language retrieval system called dbctx.

dbctx already retrieves relevant tables/columns for a user's question using
deterministic lexical matching and an optional local embedding model. Both
of those are good at typos, synonyms, and general paraphrases. Neither is
reliable for domain-specific vocabulary that has no lexical or semantic
resemblance to the underlying schema: internal abbreviations, acronyms,
business/industry terms, and internal jargon that a generic model has never
seen used the way THIS organization uses it. That gap is what this
terminology dictionary is for.

====================================================================
AUTHORITATIVE SCHEMA
====================================================================
The database schema below — extracted directly from the database, not
inferred — is the ONLY ground truth for this task. It includes tables,
columns, types, primary/foreign keys, state-like and categorical fields
with their OBSERVED values, and JSONB path structure with sample values.

%s
====================================================================
YOUR TASK
====================================================================
Identify terms a human at this organization might type into a search box
when they mean one of the schema objects above, where that connection
would NOT be obvious to plain keyword or synonym matching. For each one,
produce a mapping from the human term (and its natural variants) to the
exact schema object(s) it refers to.

Pay particular attention to:
  - abbreviations and acronyms (e.g. "LOC" -> lines of code, "MRR" ->
    monthly recurring revenue, "CAC" -> customer acquisition cost)
  - internal engineering or business jargon specific to this schema
  - singular/plural and other surface variants of the same concept
  - multi-word natural-language phrases describing a concept
    (e.g. "how many lines of code" referring to a "loc" column)
  - common alternate names for the same underlying concept

====================================================================
RULES — READ CAREFULLY
====================================================================
1. DO NOT INVENT. Only propose a mapping when there is a genuine,
   plausible relationship between the human term and an ACTUAL object in
   the schema above. If you are not looking at a real table/column/JSONB
   path from the schema section, do not propose it.

2. This is NOT an invitation to add ordinary synonyms dbctx's existing
   lexical or semantic search would already catch on its own (e.g. don't
   bother mapping "orders" -> "orders", or generic word substitutions with
   no organization-specific meaning). Terminology exists for the cases
   those signals structurally cannot cover — abbreviations, acronyms, and
   jargon with no lexical or semantic resemblance to the schema object.

3. If there is genuine ambiguity — a term could plausibly map to more than
   one object, or you are not confident a term is actually used at this
   organization — ASK THE USER rather than guessing. For example:

     "I found a column 'loc'. Could this mean 'lines of code'? Is that
     terminology your team actually uses?"

     User: "Yes."

     "Should 'source lines of code' map to the same column, or is that a
     different metric at your organization?"

     User: "No, that's different — we don't track that separately."

   Only include a mapping in your final output once the user has
   confirmed it, or you are highly confident based on unambiguous
   evidence in the schema (e.g. a column literally named 'mrr' with an
   an business context field).

4. Every mapping must resolve to an EXACT object from the schema above,
   written in this notation:
     - "table"                    for a table-level concept
     - "table.column"             for a specific column
     - "table.column:$.json.path" for a JSONB path within a column
       (note the ':' before the path — JSONB paths themselves contain
       dots, so ':' disambiguates the column boundary)

5. Work interactively. Propose your findings, ask clarifying questions
   where needed, and only finalize the output after the user has had a
   chance to confirm or correct each non-obvious mapping. The resulting
   dictionary represents USER-APPROVED domain knowledge, not something
   dbctx or you inferred unilaterally — accuracy matters more than
   coverage.

====================================================================
OUTPUT FORMAT
====================================================================
Once mappings are confirmed, produce a JSON array like this (and ONLY
this — no other schema objects, no explanatory prose mixed into the
JSON):

[
  {
    "term": "loc",
    "aliases": ["line of code", "lines of code", "source lines of code"],
    "targets": ["metrics.loc"]
  },
  {
    "term": "mrr",
    "aliases": ["monthly recurring revenue", "recurring monthly revenue"],
    "targets": ["subscriptions.mrr_cents"]
  }
]

This output is meant to be saved to a file and loaded with:

    dbctx terminology import mydb.dtx terminology.json

which will validate every target against the real schema before accepting
anything, so it is safe to include a mapping you are reasonably (not
100%%) confident about — invalid targets are rejected automatically rather
than corrupting the database context.

Begin by proposing an initial set of candidate terms from the schema
above, and ask me about any that are ambiguous or that you are not
confident are actually used here, before producing the final JSON.
`
