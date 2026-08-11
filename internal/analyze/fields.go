package analyze

import (
	"context"
	"fmt"
	"os"
	"strings"
	"unicode"

	"github.com/shrsv/dbctx/internal/db"
	"github.com/shrsv/dbctx/internal/schema"
)

var stateFieldNames = map[string]bool{
	"status": true, "state": true, "stage": true, "phase": true,
	"type": true, "kind": true, "role": true, "category": true,
	"mode": true, "level": true, "priority": true, "severity": true,
	"action": true, "event": true, "result": true, "outcome": true,
	"disposition": true, "resolution": true,
}

func isStateLikeName(name string) bool {
	lower := strings.ToLower(name)
	if stateFieldNames[lower] {
		return true
	}
	for _, suffix := range []string{"_status", "_state", "_type", "_kind", "_stage", "_role", "_category", "_mode"} {
		if strings.HasSuffix(lower, suffix) {
			return true
		}
	}
	return false
}

func isTextHeavy(val string) bool {
	if len(val) > 100 {
		return true
	}
	spaceCount := 0
	for _, r := range val {
		if unicode.IsSpace(r) {
			spaceCount++
		}
	}
	return spaceCount > 3
}

type pgStatRow struct {
	TableName        string
	ColumnName       string
	NDistinct        float64
	NullFrac         float64
	MostCommonVals   string
	MostCommonFreqs  string
}

func getStatsFromPG(ctx context.Context, pg *db.PG, schemas string) ([]pgStatRow, error) {
	rows, err := pg.Conn.Query(ctx, `
		SELECT tablename, attname, n_distinct, null_frac,
		       COALESCE(most_common_vals::text, ''),
		       COALESCE(most_common_freqs::text, '')
		FROM pg_stats
		WHERE schemaname = ANY(string_to_array(
			COALESCE(NULLIF(current_setting('app.dbctx_schemas', true), ''), 'public'),
			','
		))
		ORDER BY tablename, attname
	`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var stats []pgStatRow
	for rows.Next() {
		var s pgStatRow
		if err := rows.Scan(&s.TableName, &s.ColumnName, &s.NDistinct, &s.NullFrac, &s.MostCommonVals, &s.MostCommonFreqs); err != nil {
			continue
		}
		stats = append(stats, s)
	}
	return stats, rows.Err()
}

func parsePGArray(s string) []string {
	s = strings.TrimSpace(s)
	if s == "" || s == "{}" {
		return nil
	}
	if s[0] == '{' {
		s = s[1:]
	}
	if len(s) > 0 && s[len(s)-1] == '}' {
		s = s[:len(s)-1]
	}
	if s == "" {
		return nil
	}
	var result []string
	var current strings.Builder
	inQuote := false
	for i := 0; i < len(s); i++ {
		ch := s[i]
		if ch == '"' {
			inQuote = !inQuote
			continue
		}
		if ch == ',' && !inQuote {
			result = append(result, current.String())
			current.Reset()
			continue
		}
		current.WriteByte(ch)
	}
	if current.Len() > 0 {
		result = append(result, current.String())
	}
	return result
}

func parseFloats(s string) []float64 {
	s = strings.TrimSpace(s)
	if s == "" || s == "{}" {
		return nil
	}
	s = s[1 : len(s)-1]
	var result []float64
	for _, part := range strings.Split(s, ",") {
		var f float64
		fmt.Sscanf(strings.TrimSpace(part), "%f", &f)
		result = append(result, f)
	}
	return result
}

func AnalyzeFields(ctx context.Context, pg *db.PG, ext *schema.ExtractedSchema, store *db.Store, schemas string) error {
	fmt.Fprintf(os.Stderr, "  Reading pg_stats...\n")
	stats, err := getStatsFromPG(ctx, pg, schemas)
	if err != nil {
		return fmt.Errorf("get pg_stats: %w", err)
	}

	statsMap := make(map[string]*pgStatRow)
	for i := range stats {
		key := stats[i].TableName + "." + stats[i].ColumnName
		statsMap[key] = &stats[i]
	}

	fmt.Fprintf(os.Stderr, "  Processing %d tables...\n", len(ext.Tables))

	tx, err := store.DB().Begin()
	if err != nil {
		return fmt.Errorf("begin tx: %w", err)
	}
	defer tx.Rollback()

	for _, table := range ext.Tables {
		cols := ext.Columns[table.OID]
		if len(cols) == 0 {
			continue
		}

		for _, col := range cols {
			var columnID int
			err := tx.QueryRow(
				"SELECT id FROM columns WHERE table_id = (SELECT id FROM tables WHERE schema = ? AND name = ?) AND name = ?",
				table.Schema, table.Name, col.Name,
			).Scan(&columnID)
			if err != nil {
				continue
			}

			statsKey := table.Name + "." + col.Name
			st, hasStats := statsMap[statsKey]

			distinct := 0
			nullFrac := 0.0
			if hasStats {
				if st.NDistinct < 0 {
					distinct = int(-st.NDistinct * 1000)
				} else {
					distinct = int(st.NDistinct)
				}
				nullFrac = st.NullFrac
			}

			isState := isStateLikeName(col.Name)
			isCategorical := distinct > 0 && distinct <= 50

			_, err = tx.Exec(
				`INSERT INTO field_stats (column_id, distinct_count, null_count, is_state_like, is_categorical)
				 VALUES (?, ?, ?, ?, ?)`,
				columnID, distinct, int(nullFrac*100), isState && distinct <= 20, isCategorical,
			)
			if err != nil {
				return fmt.Errorf("insert field_stats: %w", err)
			}

			if hasStats && st.MostCommonVals != "" {
				vals := parsePGArray(st.MostCommonVals)
				freqs := parseFloats(st.MostCommonFreqs)
				for i, val := range vals {
					if isTextHeavy(val) || val == "" {
						continue
					}
					freq := 0
					if i < len(freqs) {
						freq = int(freqs[i] * 1000)
					}
					tx.Exec(
						`INSERT OR IGNORE INTO field_values (column_id, value, frequency) VALUES (?, ?, ?)`,
						columnID, val, freq,
					)
				}
			}
		}
	}
	return tx.Commit()
}
