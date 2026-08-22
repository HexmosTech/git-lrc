package report

import (
	"fmt"
	"io"
	"strings"

	"github.com/shrsv/dbctx/internal/db"
	"github.com/shrsv/dbctx/internal/search"
)

func ReportAll(w io.Writer, store *db.Store) error {
	fmt.Fprintln(w, "=== DATABASE CONTEXT REPORT ===")
	fmt.Fprintln(w)

	// Tables summary
	tableCount := 0
	store.DB().QueryRow("SELECT COUNT(*) FROM tables").Scan(&tableCount)
	fmt.Fprintf(w, "Tables: %d\n\n", tableCount)

	// Schema with compact format (like llm-schema.py)
	reportSchema(w, store)

	// State-like fields
	reportStateFields(w, store)

	// Categorical fields
	reportCategoricalFields(w, store)

	// JSONB intelligence
	reportJSONB(w, store)

	// Relationship graph
	reportRelationships(w, store)

	// Stats summary
	reportStats(w, store)

	return nil
}

func reportSchema(w io.Writer, store *db.Store) {
	fmt.Fprintln(w, "--- SCHEMA ---")
	fmt.Fprintln(w)

	rows, err := store.DB().Query(`
		SELECT t.name,
		       GROUP_CONCAT(
		           c.name || ' ' || c.type ||
		           CASE WHEN pk.column_name IS NOT NULL THEN ' ^' ELSE '' END ||
		           CASE WHEN c.nullable = 1 THEN ' ?' ELSE '' END ||
		           CASE WHEN fk.src_columns IS NOT NULL THEN ' >' || COALESCE(rt.name, '') || COALESCE('.' || fk.dst_columns, '') ELSE '' END,
		           ', '
		       ) as columns
		FROM tables t
		JOIN columns c ON c.table_id = t.id
		LEFT JOIN primary_keys pk ON pk.table_id = c.table_id AND pk.column_name = c.name
		LEFT JOIN (
			SELECT fk.table_id, fk.src_columns, fk.ref_table_id, fk.dst_columns
			FROM foreign_keys fk
		) fk ON fk.table_id = c.table_id AND fk.src_columns = c.name
		LEFT JOIN tables rt ON rt.id = fk.ref_table_id
		GROUP BY t.name
		ORDER BY t.name
	`)
	if err != nil {
		fmt.Fprintf(w, "  (error: %v)\n", err)
		return
	}
	defer rows.Close()

	for rows.Next() {
		var name, cols string
		if rows.Scan(&name, &cols) == nil {
			fmt.Fprintf(w, "  %s: %s\n\n", name, cols)
		}
	}
}

func reportStateFields(w io.Writer, store *db.Store) {
	rows, err := store.DB().Query(`
		SELECT t.name, c.name, c.type
		FROM field_stats fs
		JOIN columns c ON c.id = fs.column_id
		JOIN tables t ON t.id = c.table_id
		WHERE fs.is_state_like = 1
		ORDER BY t.name, c.name
	`)
	if err != nil {
		return
	}
	defer rows.Close()

	first := true
	for rows.Next() {
		var tbl, col, typ string
		if rows.Scan(&tbl, &col, &typ) != nil {
			continue
		}
		if first {
			fmt.Fprintln(w, "--- STATE-LIKE FIELDS ---")
			fmt.Fprintln(w)
			first = false
		}
		fmt.Fprintf(w, "  %s.%s (%s)\n", tbl, col, typ)
		// Print values
		valRows, _ := store.DB().Query(`
			SELECT value, frequency FROM field_values fv
			JOIN columns c ON c.id = fv.column_id
			JOIN tables t ON t.id = c.table_id
			WHERE t.name = ? AND c.name = ?
			ORDER BY frequency DESC
		`, tbl, col)
		if valRows != nil {
			for valRows.Next() {
				var val string
				var freq int
				if valRows.Scan(&val, &freq) == nil {
					fmt.Fprintf(w, "    %s (%d)\n", val, freq)
				}
			}
			valRows.Close()
		}
		fmt.Fprintln(w)
	}
}

func reportCategoricalFields(w io.Writer, store *db.Store) {
	rows, err := store.DB().Query(`
		SELECT t.name, c.name, c.type, fs.distinct_count
		FROM field_stats fs
		JOIN columns c ON c.id = fs.column_id
		JOIN tables t ON t.id = c.table_id
		WHERE fs.is_categorical = 1 AND fs.is_state_like = 0
		ORDER BY t.name, c.name
	`)
	if err != nil {
		return
	}
	defer rows.Close()

	first := true
	for rows.Next() {
		var tbl, col, typ string
		var distinct int
		if rows.Scan(&tbl, &col, &typ, &distinct) != nil {
			continue
		}
		if first {
			fmt.Fprintln(w, "--- CATEGORICAL FIELDS ---")
			fmt.Fprintln(w)
			first = false
		}
		fmt.Fprintf(w, "  %s.%s (%s) — %d distinct\n", tbl, col, typ, distinct)
		valRows, _ := store.DB().Query(`
			SELECT value, frequency FROM field_values fv
			JOIN columns c ON c.id = fv.column_id
			JOIN tables t ON t.id = c.table_id
			WHERE t.name = ? AND c.name = ?
			ORDER BY frequency DESC
			LIMIT 10
		`, tbl, col)
		if valRows != nil {
			for valRows.Next() {
				var val string
				var freq int
				if valRows.Scan(&val, &freq) == nil {
					fmt.Fprintf(w, "    %s (%d)\n", val, freq)
				}
			}
			valRows.Close()
		}
		fmt.Fprintln(w)
	}
}

func reportJSONB(w io.Writer, store *db.Store) {
	rows, err := store.DB().Query(`
		SELECT t.name, c.name, jp.path, jp.inferred_type, jp.sample_values
		FROM jsonb_paths jp
		JOIN columns c ON c.id = jp.column_id
		JOIN tables t ON t.id = c.table_id
		ORDER BY t.name, c.name, jp.path
	`)
	if err != nil {
		return
	}
	defer rows.Close()

	first := true
	for rows.Next() {
		var tbl, col, path, typ, samples string
		if rows.Scan(&tbl, &col, &path, &typ, &samples) != nil {
			continue
		}
		if first {
			fmt.Fprintln(w, "--- JSONB STRUCTURE ---")
			fmt.Fprintln(w)
			first = false
		}
		fmt.Fprintf(w, "  %s.%s  %s  %s\n", tbl, col, path, typ)
		if samples != "" {
			fmt.Fprintf(w, "    values: %s\n", samples)
		}
	}
	fmt.Fprintln(w)
}

func reportRelationships(w io.Writer, store *db.Store) {
	rows, err := store.DB().Query(`
		SELECT st.name, fk.src_columns, rt.name, fk.dst_columns
		FROM foreign_keys fk
		JOIN tables st ON st.id = fk.table_id
		JOIN tables rt ON rt.id = fk.ref_table_id
		ORDER BY st.name
	`)
	if err != nil {
		return
	}
	defer rows.Close()

	first := true
	for rows.Next() {
		var src, srcCols, dst, dstCols string
		if rows.Scan(&src, &srcCols, &dst, &dstCols) != nil {
			continue
		}
		if first {
			fmt.Fprintln(w, "--- RELATIONSHIPS ---")
			fmt.Fprintln(w)
			first = false
		}
		dstSuffix := ""
		if dstCols != "id" {
			dstSuffix = "." + dstCols
		}
		fmt.Fprintf(w, "  %s.%s → %s%s\n", src, srcCols, dst, dstSuffix)
	}
	fmt.Fprintln(w)
}

func reportStats(w io.Writer, store *db.Store) {
	fmt.Fprintln(w, "--- STATS ---")
	fmt.Fprintln(w)

	var tables, columns, fks, stateFields, categoricalFields, jsonbPaths int
	store.DB().QueryRow("SELECT COUNT(*) FROM tables").Scan(&tables)
	store.DB().QueryRow("SELECT COUNT(*) FROM columns").Scan(&columns)
	store.DB().QueryRow("SELECT COUNT(*) FROM foreign_keys").Scan(&fks)
	store.DB().QueryRow("SELECT COUNT(*) FROM field_stats WHERE is_state_like = 1").Scan(&stateFields)
	store.DB().QueryRow("SELECT COUNT(*) FROM field_stats WHERE is_categorical = 1").Scan(&categoricalFields)
	store.DB().QueryRow("SELECT COUNT(*) FROM jsonb_paths").Scan(&jsonbPaths)

	fmt.Fprintf(w, "  Tables:            %d\n", tables)
	fmt.Fprintf(w, "  Columns:           %d\n", columns)
	fmt.Fprintf(w, "  Foreign keys:      %d\n", fks)
	fmt.Fprintf(w, "  State-like fields: %d\n", stateFields)
	fmt.Fprintf(w, "  Categorical fields:%d\n", categoricalFields)
	fmt.Fprintf(w, "  JSONB paths:       %d\n", jsonbPaths)
	fmt.Fprintln(w)
}

func FormatQueryResult(w io.Writer, result *search.SearchResult) {
	fmt.Fprint(w, search.Legend())
	fmt.Fprintln(w)

	fmt.Fprintf(w, "Query: %s\n", result.Query)

	t := result.Timing
	if t.SemanticRan {
		fmt.Fprintf(w, "Time: %.2fms total  (lexical %.2fms, semantic %.2fms, expand %.2fms)\n\n",
			t.TotalMs, t.LexicalMs, t.SemanticMs, t.ExpandMs)
	} else {
		fmt.Fprintf(w, "Time: %.2fms total  (lexical %.2fms, expand %.2fms)\n\n",
			t.TotalMs, t.LexicalMs, t.ExpandMs)
	}

	fmt.Fprintln(w, "TABLES (ranked)")
	for _, t := range result.Tables {
		scoreStr := ""
		if t.IsMatch {
			scoreStr = fmt.Sprintf("  score: %.2f", t.MatchScore)
		}
		fmt.Fprintf(w, "\n  %s%s\n", t.TableName, scoreStr)

		if len(t.PrimaryKey) > 0 {
			fmt.Fprintf(w, "    PK: %s\n", strings.Join(t.PrimaryKey, ", "))
		}

		for _, fk := range t.ForeignKeys {
			dstSuffix := ""
			if fk.DstColumns != "id" {
				dstSuffix = "." + fk.DstColumns
			}
			fmt.Fprintf(w, "    %s → %s%s\n", fk.SrcColumns, fk.RefTable, dstSuffix)
		}

		fmt.Fprintln(w, "    COLUMNS:")
		for _, c := range t.Columns {
			flags := ""
			if c.IsPK {
				flags += " ^"
			}
			if c.Nullable {
				flags += " ?"
			}
			if c.FKTarget != "" {
				flags += " >" + c.FKTarget
			}
			if c.IsState {
				flags += " [state]"
			}
			if c.IsCategoric {
				flags += " [cat]"
			}
			fmt.Fprintf(w, "      %s %s%s\n", c.Name, search.ShortenTypeParam(c.Type), flags)

			if len(c.Values) > 0 {
				var vals []string
				for _, v := range c.Values {
					vals = append(vals, v.Value)
				}
				fmt.Fprintf(w, "        values: {%s}\n", strings.Join(vals, ", "))
			}

			if len(c.JSONBPaths) > 0 {
				for _, jp := range c.JSONBPaths {
					fmt.Fprintf(w, "        %s  %s", jp.Path, jp.InferredType)
					if jp.SampleValues != "" {
						fmt.Fprintf(w, "  {%s}", jp.SampleValues)
					}
					fmt.Fprintln(w)
				}
			}
		}
	}
	fmt.Fprintln(w)

	if len(result.SemanticHits) > 0 {
		fmt.Fprintln(w, "SEMANTIC SIGNAL (evidence for tables lexical search alone might have missed)")
		for _, h := range result.SemanticHits {
			fmt.Fprintf(w, "\n  %s  (%s, similarity: %.2f)\n", h.TableName, h.Kind, h.Score)
			for _, line := range strings.Split(strings.TrimSpace(h.Text), "\n") {
				fmt.Fprintf(w, "    %s\n", line)
			}
		}
		fmt.Fprintln(w)
	}
}
