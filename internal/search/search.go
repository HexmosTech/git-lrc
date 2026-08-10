package search

import (
	"sort"
	"strings"

	"github.com/sahilm/fuzzy"
	"github.com/shrsv/dbctx/internal/db"
)

type MatchResult struct {
	TableName string  `json:"table_name"`
	Score     float64 `json:"score"`
	Source    string  `json:"source"`
}

type TableContext struct {
	TableName   string       `json:"table_name"`
	Schema      string       `json:"schema"`
	Columns     []ColumnInfo `json:"columns"`
	PrimaryKey  []string     `json:"primary_key"`
	ForeignKeys []FKInfo      `json:"foreign_keys"`
	IsMatch     bool         `json:"is_match"`
	MatchScore  float64      `json:"match_score"`
}

type ColumnInfo struct {
	Name        string          `json:"name"`
	Type        string          `json:"type"`
	Nullable    bool            `json:"nullable"`
	IsPK        bool            `json:"is_pk"`
	FKTarget    string          `json:"fk_target,omitempty"`
	IsState     bool            `json:"is_state"`
	IsCategoric bool            `json:"is_categoric"`
	Values      []ValueInfo     `json:"values,omitempty"`
	JSONBPaths  []JSONBPathInfo `json:"jsonb_paths,omitempty"`
}

type ValueInfo struct {
	Value     string `json:"value"`
	Frequency int    `json:"frequency"`
}

type JSONBPathInfo struct {
	Path         string `json:"path"`
	InferredType string `json:"inferred_type"`
	SampleValues string `json:"sample_values,omitempty"`
}

type FKInfo struct {
	SrcColumns string `json:"src_columns"`
	RefTable   string `json:"ref_table"`
	DstColumns string `json:"dst_columns"`
}

type SearchResult struct {
	Tables []TableContext `json:"tables"`
	Query  string         `json:"query"`
}

func PopulateFTS(store *db.Store) error {
	if _, err := store.DB().Exec("DELETE FROM search_index"); err != nil {
		return err
	}

	rows, err := store.DB().Query(`
		SELECT t.id, t.name,
		       GROUP_CONCAT(c.name, ' ') as col_names
		FROM tables t
		JOIN columns c ON c.table_id = t.id
		GROUP BY t.id, t.name
	`)
	if err != nil {
		return err
	}
	defer rows.Close()

	for rows.Next() {
		var tableID int
		var tableName, colNames string
		if err := rows.Scan(&tableID, &tableName, &colNames); err != nil {
			continue
		}

		var valueTokens []string
		valRows, _ := store.DB().Query(`
			SELECT DISTINCT value FROM field_values
			WHERE column_id IN (SELECT id FROM columns WHERE table_id = ?)
			LIMIT 100
		`, tableID)
		if valRows != nil {
			for valRows.Next() {
				var v string
				if valRows.Scan(&v) == nil {
					valueTokens = append(valueTokens, v)
				}
			}
			valRows.Close()
		}

		valStr := strings.Join(valueTokens, " ")
		store.DB().Exec(
			"INSERT INTO search_index (table_name, column_names, value_tokens) VALUES (?, ?, ?)",
			tableName, colNames, valStr,
		)
	}
	return rows.Err()
}

func Query(store *db.Store, query string) (*SearchResult, error) {
	result := &SearchResult{Query: query}
	tokens := strings.Fields(strings.ToLower(query))

	// 1. FTS5 search
	ftsScores := ftsSearch(store, tokens)

	// 2. Fuzzy match on table names
	fuzzyScores := fuzzyMatchTables(store, tokens)

	// 3. Value match
	valueScores := valueMatch(store, tokens)

	// Merge scores
	merged := make(map[string]float64)
	for t, s := range ftsScores {
		merged[t] += s * 1.0
	}
	for t, s := range fuzzyScores {
		merged[t] += s * 0.8
	}
	for t, s := range valueScores {
		merged[t] += s * 1.2
	}

	// Sort by score
	type scored struct {
		name  string
		score float64
	}
	var sorted []scored
	for name, score := range merged {
		sorted = append(sorted, scored{name, score})
	}
	sort.Slice(sorted, func(i, j int) bool { return sorted[i].score > sorted[j].score })

	// Expand via FKs
	expanded := make(map[string]bool)
	for _, s := range sorted {
		expanded[s.name] = true
		fkExpand(store, s.name, expanded, 2)
	}

	// Build table contexts
	seen := make(map[string]bool)
	for _, s := range sorted {
		if !expanded[s.name] {
			continue
		}
		if seen[s.name] {
			continue
		}
		seen[s.name] = true
		tc := buildTableContext(store, s.name, s.score)
		result.Tables = append(result.Tables, tc)
	}

	// Add FK-expanded tables that weren't in the original match
	for tbl := range expanded {
		if seen[tbl] {
			continue
		}
		seen[tbl] = true
		tc := buildTableContext(store, tbl, 0)
		result.Tables = append(result.Tables, tc)
	}

	return result, nil
}

func ftsSearch(store *db.Store, tokens []string) map[string]float64 {
	scores := make(map[string]float64)
	query := strings.Join(tokens, " OR ")
	if query == "" {
		return scores
	}
	rows, err := store.DB().Query(`
		SELECT table_name, rank FROM search_index
		WHERE search_index MATCH ?
		ORDER BY rank LIMIT 20
	`, query)
	if err != nil {
		return scores
	}
	defer rows.Close()
	for rows.Next() {
		var name string
		var rank float64
		if rows.Scan(&name, &rank) == nil {
			scores[name] = 1.0 / (1.0 + (-rank))
		}
	}
	return scores
}

func fuzzyMatchTables(store *db.Store, tokens []string) map[string]float64 {
	scores := make(map[string]float64)
	rows, err := store.DB().Query("SELECT name FROM tables")
	if err != nil {
		return scores
	}
	defer rows.Close()
	var names []string
	for rows.Next() {
		var n string
		if rows.Scan(&n) == nil {
			names = append(names, n)
		}
	}
	for _, token := range tokens {
		matches := fuzzy.Find(token, names)
		for _, m := range matches {
			scores[m.Str] += float64(m.Score) / 100.0
		}
	}
	return scores
}

func valueMatch(store *db.Store, tokens []string) map[string]float64 {
	scores := make(map[string]float64)
	for _, token := range tokens {
		rows, err := store.DB().Query(`
			SELECT DISTINCT t.name
			FROM field_values fv
			JOIN columns c ON c.id = fv.column_id
			JOIN tables t ON t.id = c.table_id
			WHERE LOWER(fv.value) LIKE '%' || LOWER(?) || '%'
			LIMIT 20
		`, token)
		if err != nil {
			continue
		}
		for rows.Next() {
			var name string
			if rows.Scan(&name) == nil {
				scores[name] += 0.5
			}
		}
		rows.Close()
	}
	return scores
}

func fkExpand(store *db.Store, tableName string, expanded map[string]bool, depth int) {
	if depth <= 0 {
		return
	}
	rows, err := store.DB().Query(`
		SELECT rt.name FROM foreign_keys fk
		JOIN tables st ON st.id = fk.table_id
		JOIN tables rt ON rt.id = fk.ref_table_id
		WHERE st.name = ?
		UNION
		SELECT st.name FROM foreign_keys fk
		JOIN tables st ON st.id = fk.table_id
		JOIN tables rt ON rt.id = fk.ref_table_id
		WHERE rt.name = ?
	`, tableName, tableName)
	if err != nil {
		return
	}
	defer rows.Close()
	for rows.Next() {
		var related string
		if rows.Scan(&related) == nil && !expanded[related] {
			expanded[related] = true
			fkExpand(store, related, expanded, depth-1)
		}
	}
}

func buildTableContext(store *db.Store, tableName string, score float64) TableContext {
	tc := TableContext{TableName: tableName, MatchScore: score, IsMatch: score > 0}

	// Get schema
	store.DB().QueryRow("SELECT COALESCE(schema, 'public') FROM tables WHERE name = ?", tableName).Scan(&tc.Schema)

	// Get PK columns
	pkRows, _ := store.DB().Query(`
		SELECT column_name FROM primary_keys
		WHERE table_id = (SELECT id FROM tables WHERE name = ?)
	`, tableName)
	if pkRows != nil {
		for pkRows.Next() {
			var col string
			if pkRows.Scan(&col) == nil {
				tc.PrimaryKey = append(tc.PrimaryKey, col)
			}
		}
		pkRows.Close()
	}

	// Get FK info
	fkRows, _ := store.DB().Query(`
		SELECT fk.src_columns, rt.name, fk.dst_columns
		FROM foreign_keys fk
		JOIN tables st ON st.id = fk.table_id
		JOIN tables rt ON rt.id = fk.ref_table_id
		WHERE st.name = ?
	`, tableName)
	if fkRows != nil {
		for fkRows.Next() {
			var fk FKInfo
			if fkRows.Scan(&fk.SrcColumns, &fk.RefTable, &fk.DstColumns) == nil {
				tc.ForeignKeys = append(tc.ForeignKeys, fk)
			}
		}
		fkRows.Close()
	}

	// Get columns with metadata
	colRows, _ := store.DB().Query(`
		SELECT c.name, c.type, c.nullable,
		       CASE WHEN pk.column_name IS NOT NULL THEN 1 ELSE 0 END as is_pk,
		       fs.is_state_like, fs.is_categorical
		FROM columns c
		LEFT JOIN primary_keys pk ON pk.table_id = c.table_id AND pk.column_name = c.name
		LEFT JOIN field_stats fs ON fs.column_id = c.id
		WHERE c.table_id = (SELECT id FROM tables WHERE name = ?)
		ORDER BY c.position
	`, tableName)
	if colRows != nil {
		for colRows.Next() {
			var ci ColumnInfo
			var isPK int
			var isState, isCategoric *bool
			if colRows.Scan(&ci.Name, &ci.Type, &ci.Nullable, &isPK, &isState, &isCategoric) == nil {
				ci.IsPK = isPK == 1
				if isState != nil {
					ci.IsState = *isState
				}
				if isCategoric != nil {
					ci.IsCategoric = *isCategoric
				}
				// Find FK target for this column
				for _, fk := range tc.ForeignKeys {
					if strings.Contains(fk.SrcColumns, ci.Name) {
						dstSuffix := ""
						if fk.DstColumns != "id" {
							dstSuffix = "." + fk.DstColumns
						}
						ci.FKTarget = fk.RefTable + dstSuffix
						break
					}
				}
				tc.Columns = append(tc.Columns, ci)
			}
		}
		colRows.Close()
	}

	// Get values for state/categorical columns
	for i := range tc.Columns {
		if tc.Columns[i].IsState || tc.Columns[i].IsCategoric {
			valRows, _ := store.DB().Query(`
				SELECT fv.value, fv.frequency FROM field_values fv
				JOIN columns c ON c.id = fv.column_id
				JOIN tables t ON t.id = c.table_id
				WHERE t.name = ? AND c.name = ?
				ORDER BY fv.frequency DESC
			`, tableName, tc.Columns[i].Name)
			if valRows != nil {
				for valRows.Next() {
					var v ValueInfo
					if valRows.Scan(&v.Value, &v.Frequency) == nil {
						tc.Columns[i].Values = append(tc.Columns[i].Values, v)
					}
				}
				valRows.Close()
			}
		}
	}

	// Get JSONB paths
	for i := range tc.Columns {
		if tc.Columns[i].Type == "jsonb" || tc.Columns[i].Type == "json" {
			jRows, _ := store.DB().Query(`
				SELECT jp.path, jp.inferred_type, jp.sample_values
				FROM jsonb_paths jp
				JOIN columns c ON c.id = jp.column_id
				JOIN tables t ON t.id = c.table_id
				WHERE t.name = ? AND c.name = ?
				ORDER BY jp.path
			`, tableName, tc.Columns[i].Name)
			if jRows != nil {
				for jRows.Next() {
					var jp JSONBPathInfo
					if jRows.Scan(&jp.Path, &jp.InferredType, &jp.SampleValues) == nil {
						tc.Columns[i].JSONBPaths = append(tc.Columns[i].JSONBPaths, jp)
					}
				}
				jRows.Close()
			}
		}
	}

	return tc
}
