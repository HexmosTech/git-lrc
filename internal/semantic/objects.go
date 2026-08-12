package semantic

import (
	"crypto/sha256"
	"encoding/hex"
	"regexp"
	"strings"

	"github.com/shrsv/dbctx/internal/db"
)

// candidate is a schema-derived object eligible for embedding. It carries
// enough identity (kind + table/column/jsonb_path id) to be matched against
// previously-persisted semantic_objects rows for incremental rebuilds, and
// enough text to be embedded and later shown back as a match "reason".
type candidate struct {
	Kind        string
	TableID     int64
	TableName   string
	ColumnID    int64 // 0 if not applicable
	JSONBPathID int64 // 0 if not applicable
	Text        string
}

// hash returns a content hash of the candidate's embeddable text, used to
// detect whether a previously-embedded object actually changed (and so
// needs re-embedding) versus being byte-identical to what's already
// persisted.
func (c candidate) hash() string {
	sum := sha256.Sum256([]byte(c.Text))
	return hex.EncodeToString(sum[:])
}

// identity is the stable key used to match a candidate against an existing
// semantic_objects row across rebuilds, independent of its current text.
type identity struct {
	Kind        string
	TableID     int64
	ColumnID    int64
	JSONBPathID int64
}

func (c candidate) identity() identity {
	return identity{c.Kind, c.TableID, c.ColumnID, c.JSONBPathID}
}

// maxJSONBObjectsPerTable bounds how many JSONB path objects get embedded
// per table, so a handful of wide/deeply-nested JSONB columns can't blow
// up the semantic corpus with low-value noise.
const maxJSONBObjectsPerTable = 40

// maxValuesInTableText bounds how many representative state/categorical
// values get folded into a table's own text blurb.
const maxValuesInTableText = 25

// maxValuesInColumnText bounds representative values folded into a single
// column's text blurb.
const maxValuesInColumnText = 15

// collectCandidates derives the full set of embeddable objects from a
// store's existing schema/field/JSONB data — no PostgreSQL access needed,
// since everything used here was already extracted during build. This is
// what keeps semantic indexing usable as its own phase (or standalone
// re-run) independent of schema extraction.
func collectCandidates(store *db.Store) ([]candidate, error) {
	tables, err := loadTables(store)
	if err != nil {
		return nil, err
	}

	var out []candidate
	for _, t := range tables {
		cols, err := loadColumns(store, t.id)
		if err != nil {
			return nil, err
		}
		fkTargets, err := loadFKTargets(store, t.id)
		if err != nil {
			return nil, err
		}
		tableValues, err := loadTableValues(store, t.id, maxValuesInTableText)
		if err != nil {
			return nil, err
		}

		out = append(out, candidate{
			Kind:      KindTable,
			TableID:   t.id,
			TableName: t.name,
			Text:      buildTableText(t.name, cols, fkTargets, tableValues),
		})

		for _, c := range cols {
			if !c.isStateLike && !c.isCategorical && !c.isFK {
				continue
			}
			values, err := loadColumnValues(store, c.id, maxValuesInColumnText)
			if err != nil {
				return nil, err
			}
			out = append(out, candidate{
				Kind:      KindColumn,
				TableID:   t.id,
				TableName: t.name,
				ColumnID:  c.id,
				Text:      buildColumnText(t.name, c, values),
			})
		}

		jsonbCols := make([]int64, 0)
		for _, c := range cols {
			if c.dataType == "jsonb" || c.dataType == "json" {
				jsonbCols = append(jsonbCols, c.id)
			}
		}
		for _, colID := range jsonbCols {
			paths, err := loadJSONBPaths(store, colID, maxJSONBObjectsPerTable)
			if err != nil {
				return nil, err
			}
			colName := ""
			for _, c := range cols {
				if c.id == colID {
					colName = c.name
				}
			}
			for _, p := range paths {
				out = append(out, candidate{
					Kind:        KindJSONBPath,
					TableID:     t.id,
					TableName:   t.name,
					JSONBPathID: p.id,
					Text:        buildJSONBPathText(t.name, colName, p),
				})
			}
		}
	}
	return out, nil
}

// --- text construction ---

var identSplitter = regexp.MustCompile(`[_\.\[\]]+`)

// spacedWords turns a snake_case/dotted/JSON-path-ish identifier into
// space-separated words (e.g. "pull_requests" -> "pull requests",
// "$.repository.name" -> "repository name"), giving the embedding model
// more natural-language surface to match against than the raw identifier
// alone.
func spacedWords(ident string) string {
	words := identSplitter.Split(ident, -1)
	var kept []string
	for _, w := range words {
		if w != "" && w != "$" {
			kept = append(kept, w)
		}
	}
	return strings.Join(kept, " ")
}

func buildTableText(name string, cols []columnRow, fkTargets []string, values []string) string {
	var b strings.Builder
	b.WriteString(name)
	b.WriteByte('\n')
	if spaced := spacedWords(name); spaced != name {
		b.WriteString(spaced)
		b.WriteByte('\n')
	}
	colNames := make([]string, 0, len(cols))
	for _, c := range cols {
		colNames = append(colNames, c.name)
	}
	if len(colNames) > 0 {
		b.WriteString("columns: " + strings.Join(colNames, " "))
		b.WriteByte('\n')
	}
	if len(fkTargets) > 0 {
		b.WriteString("related: " + strings.Join(fkTargets, " "))
		b.WriteByte('\n')
	}
	if len(values) > 0 {
		b.WriteString("values: " + strings.Join(values, " "))
		b.WriteByte('\n')
	}
	return b.String()
}

func buildColumnText(tableName string, c columnRow, values []string) string {
	var b strings.Builder
	b.WriteString(tableName + "." + c.name)
	b.WriteByte('\n')
	b.WriteString(spacedWords(c.name))
	b.WriteByte('\n')
	b.WriteString("table: " + tableName)
	b.WriteByte('\n')
	if c.fkTarget != "" {
		b.WriteString("references: " + c.fkTarget)
		b.WriteByte('\n')
	}
	if len(values) > 0 {
		b.WriteString("values: " + strings.Join(values, " "))
		b.WriteByte('\n')
	}
	return b.String()
}

func buildJSONBPathText(tableName, colName string, p jsonbPathRow) string {
	var b strings.Builder
	full := tableName + "." + colName + p.path
	b.WriteString(full)
	b.WriteByte('\n')
	b.WriteString(spacedWords(p.path))
	b.WriteByte('\n')
	if p.inferredType != "" {
		b.WriteString(p.inferredType)
		b.WriteByte('\n')
	}
	if sampleVals := parseSampleValues(p.sampleValues); len(sampleVals) > 0 {
		b.WriteString("values: " + strings.Join(sampleVals, " "))
		b.WriteByte('\n')
	}
	return b.String()
}

// parseSampleValues strips the "(freq)" suffixes from the
// "val1(500), val2(200)" format jsonb_paths.sample_values is stored in,
// leaving just the value tokens — frequencies aren't semantically useful
// text for embedding.
func parseSampleValues(s string) []string {
	if s == "" {
		return nil
	}
	parts := strings.Split(s, ",")
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		p = strings.TrimSpace(p)
		if i := strings.LastIndex(p, "("); i > 0 {
			p = p[:i]
		}
		p = strings.TrimSpace(p)
		if p != "" {
			out = append(out, p)
		}
	}
	return out
}
