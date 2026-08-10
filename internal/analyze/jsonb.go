package analyze

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"sort"
	"strings"
	"time"

	"github.com/shrsv/dbctx/internal/db"
	"github.com/shrsv/dbctx/internal/schema"
)

type JSONBPath struct {
	ColumnID      int
	Path          string
	InferredType  string
	DistinctCount int
	SampleValues  string
}

func walkJSON(val interface{}, prefix string, paths map[string]map[string]int) {
	switch v := val.(type) {
	case map[string]interface{}:
		for key, child := range v {
			path := prefix + "." + key
			inferred := inferType(child)
			if _, ok := paths[path]; !ok {
				paths[path] = make(map[string]int)
			}
			paths[path][inferred]++
			walkJSON(child, path, paths)
		}
	case []interface{}:
		for _, item := range v {
			walkJSON(item, prefix+"[]", paths)
		}
	}
}

func inferType(val interface{}) string {
	switch val.(type) {
	case nil:
		return "null"
	case bool:
		return "boolean"
	case float64:
		return "number"
	case string:
		return "string"
	case []interface{}:
		return "array"
	case map[string]interface{}:
		return "object"
	default:
		return "unknown"
	}
}

// sampleQuery returns a SQL expression that samples rows efficiently.
// For small tables (< threshold), it just does LIMIT.
// For large tables, it uses TABLESAMPLE BERNOULLI to avoid full scans.
// Always caps at `limit` rows and filters out oversized JSONB values.
func sampleQuery(colExpr, tblName, whereClause string, rowEstimate float64, limit int) string {
	// Cap individual value size to 10KB to avoid transferring massive JSONB blobs
	sizeFilter := fmt.Sprintf("octet_length(%s::text) < 10240", colExpr)
	if whereClause == "" {
		whereClause = "WHERE " + sizeFilter
	} else {
		whereClause += " AND " + sizeFilter
	}

	if rowEstimate < 5000 || rowEstimate == 0 {
		return fmt.Sprintf(`SELECT %s FROM %s %s LIMIT %d`, colExpr, tblName, whereClause, limit)
	}
	pct := 1.0
	if rowEstimate > 100000 {
		pct = 0.05
	} else if rowEstimate > 50000 {
		pct = 0.1
	} else if rowEstimate > 10000 {
		pct = 0.5
	}
	return fmt.Sprintf(
		`SELECT %s FROM %s TABLESAMPLE BERNOULLI(%.2f) %s LIMIT %d`,
		colExpr, tblName, pct, whereClause, limit,
	)
}

func AnalyzeJSONB(ctx context.Context, pg *db.PG, ext *schema.ExtractedSchema, store *db.Store) error {
	for ti, table := range ext.Tables {
		cols := ext.Columns[table.OID]
		hasJSONB := false
		for _, col := range cols {
			dt := strings.ToLower(col.DataType)
			if dt == "jsonb" || dt == "json" {
				hasJSONB = true
				break
			}
		}
		if !hasJSONB {
			continue
		}

		if (ti+1)%20 == 0 {
			fmt.Fprintf(os.Stderr, "  JSONB: table %d/%d: %s\n", ti+1, len(ext.Tables), table.Name)
		}

		tblName := schema.QuoteIdent(table.Name)
		if table.Schema != "public" {
			tblName = schema.QuoteIdent(table.Schema) + "." + tblName
		}

		tableStart := time.Now()
		jsonbCols := 0

		for _, col := range cols {
			dt := strings.ToLower(col.DataType)
			if dt != "jsonb" && dt != "json" {
				continue
			}

			var columnID int
			err := store.DB().QueryRow(
				"SELECT id FROM columns WHERE table_id = (SELECT id FROM tables WHERE schema = ? AND name = ?) AND name = ?",
				table.Schema, table.Name, col.Name,
			).Scan(&columnID)
			if err != nil {
				continue
			}

			colName := schema.QuoteIdent(col.Name)
			whereClause := fmt.Sprintf("WHERE %s IS NOT NULL", colName)

			q := sampleQuery(colName, tblName, whereClause, table.RowEstimate, 50)
			rows, err := pg.Conn.Query(ctx, q)
			if err != nil {
				continue
			}

			pathTypes := make(map[string]map[string]int)
			pathValues := make(map[string]map[string]int)

			for rows.Next() {
				var raw []byte
				if err := rows.Scan(&raw); err != nil {
					continue
				}
				var parsed interface{}
				if err := json.Unmarshal(raw, &parsed); err != nil {
					continue
				}
				walkJSON(parsed, "$", pathTypes)
				collectLeafValues(parsed, "$", pathValues)
			}
			rows.Close()

			paths := make([]string, 0, len(pathTypes))
			for p := range pathTypes {
				paths = append(paths, p)
			}
			sort.Strings(paths)

			for _, path := range paths {
				types := pathTypes[path]
				dominantType := "mixed"
				maxCount := 0
				for t, c := range types {
					if c > maxCount {
						maxCount = c
						dominantType = t
					}
				}

				sampleVals := ""
				if vals, ok := pathValues[path]; ok && len(vals) > 0 && len(vals) <= 20 {
					type valFreq struct {
						Val  string
						Freq int
					}
					var vfs []valFreq
					for v, f := range vals {
						vfs = append(vfs, valFreq{v, f})
					}
					sort.Slice(vfs, func(i, j int) bool { return vfs[i].Freq > vfs[j].Freq })
					var parts []string
					for _, vf := range vfs {
						parts = append(parts, fmt.Sprintf("%s(%d)", vf.Val, vf.Freq))
					}
					sampleVals = strings.Join(parts, ", ")
				}

				store.DB().Exec(
					`INSERT OR REPLACE INTO jsonb_paths (column_id, path, inferred_type, distinct_count, sample_values)
					 VALUES (?, ?, ?, ?, ?)`,
					columnID, path, dominantType, len(types), sampleVals,
				)
			}
			jsonbCols++
		}
		if elapsed := time.Since(tableStart); elapsed > 2*time.Second {
			fmt.Fprintf(os.Stderr, "  JSONB: %s (%d cols, %s, ~%d rows)\n",
				table.Name, jsonbCols, elapsed.Round(time.Millisecond), int(table.RowEstimate))
		}
	}
	return nil
}

func collectLeafValues(val interface{}, prefix string, pathValues map[string]map[string]int) {
	switch v := val.(type) {
	case map[string]interface{}:
		for key, child := range v {
			path := prefix + "." + key
			switch child.(type) {
			case string, bool, float64:
				str := fmt.Sprintf("%v", child)
				if len(str) <= 100 {
					if _, ok := pathValues[path]; !ok {
						pathValues[path] = make(map[string]int)
					}
					pathValues[path][str]++
				}
			default:
				collectLeafValues(child, path, pathValues)
			}
		}
	case []interface{}:
		for _, item := range v {
			collectLeafValues(item, prefix+"[]", pathValues)
		}
	}
}
