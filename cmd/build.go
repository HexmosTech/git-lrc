package cmd

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"os"
	"strings"
	"time"

	"github.com/joho/godotenv"
	"github.com/shrsv/dbctx"
	"github.com/shrsv/dbctx/internal/analyze"
	"github.com/shrsv/dbctx/internal/db"
	"github.com/shrsv/dbctx/internal/embed"
	"github.com/shrsv/dbctx/internal/schema"
	"github.com/shrsv/dbctx/internal/semantic"
	"github.com/spf13/cobra"
)

var buildCmd = &cobra.Command{
	Use:   "build [dsn]",
	Short: "Extract schema and build .dtx context index",
	Long:  "Connects to PostgreSQL, extracts schema, analyzes fields/values/JSONB, and stores everything in a .dtx SQLite file.",
	Args:  cobra.MaximumNArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		output, _ := cmd.Flags().GetString("output")
		schemas, _ := cmd.Flags().GetString("schemas")
		overwrite, _ := cmd.Flags().GetBool("overwrite")
		noSemantic, _ := cmd.Flags().GetBool("no-semantic")

		// Get DSN
		dsn := ""
		if len(args) > 0 {
			dsn = args[0]
		}
		if dsn == "" {
			godotenv.Load(".env.prod")
			dsn = os.Getenv("DATABASE_URL")
		}
		if dsn == "" {
			return fmt.Errorf("database URL required: pass as argument or set DATABASE_URL in .env.prod")
		}

		if output == "" {
			output = "dbctx.dtx"
		}

		// Handle existing file
		if _, err := os.Stat(output); err == nil {
			if overwrite {
				if err := os.Remove(output); err != nil {
					return fmt.Errorf("remove existing %s: %w", output, err)
				}
			} else {
				// Check if stdin is a terminal
				if !isTerminal() {
					return fmt.Errorf("%s already exists; use --overwrite to replace it", output)
				}
				fmt.Fprintf(os.Stderr, "%s already exists. Overwrite? [y/N] ", output)
				reader := bufio.NewReader(os.Stdin)
				answer, _ := reader.ReadString('\n')
				answer = strings.TrimSpace(strings.ToLower(answer))
				if answer != "y" && answer != "yes" {
					fmt.Fprintln(os.Stderr, "Aborted.")
					return nil
				}
				if err := os.Remove(output); err != nil {
					return fmt.Errorf("remove existing %s: %w", output, err)
				}
			}
		}

		ctx := context.Background()
		totalStart := time.Now()
		var phaseStart time.Time

		// Connect to PostgreSQL (pool with 4 connections for parallel analysis)
		fmt.Fprintf(os.Stderr, "Connecting to PostgreSQL...\n")
		pg, err := db.ConnectWithMaxConns(ctx, dsn, 4)
		if err != nil {
			return err
		}
		defer pg.Close(ctx)

		// Open/create SQLite store
		fmt.Fprintf(os.Stderr, "Creating %s...\n", output)
		store, err := db.OpenStore(output)
		if err != nil {
			return err
		}
		defer store.Close()

		if err := store.InitSchema(); err != nil {
			return err
		}

		// Phase 1: Schema extraction
		phaseStart = time.Now()
		fmt.Fprintf(os.Stderr, "1/4 Extracting schema (schemas=%s)...\n", schemas)
		ext, err := schema.Extract(ctx, pg, schemas)
		if err != nil {
			return fmt.Errorf("schema extraction: %w", err)
		}
		if err := storeSchema(store, ext); err != nil {
			return fmt.Errorf("store schema: %w", err)
		}
		if err := dbctx.StoreFingerprint(store, dbctx.ComputeFingerprint(ext)); err != nil {
			return err
		}
		fmt.Fprintf(os.Stderr, "  %d tables, %d constraints (%s)\n",
			len(ext.Tables), len(ext.Constraints), time.Since(phaseStart).Round(time.Millisecond))

		// Phase 2: Field analysis
		phaseStart = time.Now()
		fmt.Fprintf(os.Stderr, "2/4 Analyzing fields...\n")
		if err := analyze.AnalyzeFields(ctx, pg, ext, store, schemas); err != nil {
			return fmt.Errorf("field analysis: %w", err)
		}
		fmt.Fprintf(os.Stderr, "  done (%s)\n", time.Since(phaseStart).Round(time.Millisecond))

		// Phase 3: JSONB analysis
		phaseStart = time.Now()
		fmt.Fprintf(os.Stderr, "3/4 Analyzing JSONB fields...\n")
		if err := analyze.AnalyzeJSONB(ctx, pg, ext, store); err != nil {
			return fmt.Errorf("jsonb analysis: %w", err)
		}
		fmt.Fprintf(os.Stderr, "  done (%s)\n", time.Since(phaseStart).Round(time.Millisecond))
		fmt.Fprintf(os.Stderr, "  done (%s)\n", time.Since(phaseStart).Round(time.Millisecond))

		// Phase 4: Build FTS index
		phaseStart = time.Now()
		fmt.Fprintf(os.Stderr, "4/4 Building search index...\n")
		if err := store.InitFTS(); err != nil {
			return fmt.Errorf("init fts: %w", err)
		}
		if err := populateFTS(store); err != nil {
			return fmt.Errorf("populate fts: %w", err)
		}
		fmt.Fprintf(os.Stderr, "  done (%s)\n", time.Since(phaseStart).Round(time.Millisecond))

		// Phase 5: Semantic index (optional, on by default)
		if !noSemantic {
			phaseStart = time.Now()
			fmt.Fprintf(os.Stderr, "5/5 Building semantic index...\n")
			buildSemanticIndex(store, os.Stderr)
			fmt.Fprintf(os.Stderr, "  done (%s)\n", time.Since(phaseStart).Round(time.Millisecond))
		}

		fmt.Fprintf(os.Stderr, "\nDone in %s — %s\n", time.Since(totalStart).Round(time.Millisecond), output)

		return nil
	},
}

func init() {
	buildCmd.Flags().StringP("output", "o", "", "Output .dtx file path (default: dbctx.dtx)")
	buildCmd.Flags().StringP("schemas", "s", "public", "Comma-separated PostgreSQL schemas to extract")
	buildCmd.Flags().Bool("overwrite", false, "Overwrite existing .dtx file without prompting")
	buildCmd.Flags().Bool("no-semantic", false, "Skip building the local embedding-based semantic index (lexical/fuzzy search only)")
	rootCmd.AddCommand(buildCmd)
}

// buildSemanticIndex runs the optional local embedding phase, downloading
// the model/runtime to the local cache on first use. Failures are logged
// as warnings and swallowed — a build must still succeed with a
// lexical-only index if semantic indexing can't be completed (offline,
// unsupported platform, etc.).
func buildSemanticIndex(store *db.Store, log io.Writer) {
	emb, err := semantic.NewDefaultEmbedder(progressLogger(log))
	if err != nil {
		fmt.Fprintf(log, "  semantic indexing unavailable (%v); continuing with lexical-only index\n", err)
		return
	}
	stats, err := semantic.BuildIndex(store, emb, log)
	if err != nil {
		fmt.Fprintf(log, "  semantic indexing failed (%v); continuing with lexical-only index\n", err)
		return
	}
	fmt.Fprintf(log, "  %d objects (%d embedded, %d reused, %d removed)\n", stats.Total, stats.Embedded, stats.Reused, stats.Removed)
}

// progressLogger adapts an io.Writer into an embed.ProgressFunc that logs
// one line per asset the first time it starts downloading.
func progressLogger(log io.Writer) embed.ProgressFunc {
	seen := make(map[string]bool)
	return func(label string, downloaded, total int64) {
		if seen[label] {
			return
		}
		seen[label] = true
		if total > 0 {
			fmt.Fprintf(log, "  downloading %s (%.1f MB)...\n", label, float64(total)/(1024*1024))
		} else {
			fmt.Fprintf(log, "  downloading %s...\n", label)
		}
	}
}

func isTerminal() bool {
	fi, err := os.Stdin.Stat()
	if err != nil {
		return false
	}
	return fi.Mode()&os.ModeCharDevice != 0
}

func storeSchema(store *db.Store, ext *schema.ExtractedSchema) error {
	tx, err := store.DB().Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	// Store tables
	tableIDByOID := make(map[string]int64)
	for _, t := range ext.Tables {
		res, err := tx.Exec(
			"INSERT INTO tables (schema, name, row_estimate) VALUES (?, ?, ?)",
			t.Schema, t.Name, t.RowEstimate,
		)
		if err != nil {
			return fmt.Errorf("insert table %s: %w", t.Name, err)
		}
		id, _ := res.LastInsertId()
		tableIDByOID[t.OID] = id
	}

	// Store columns
	columnIDByTableCol := make(map[string]int64) // "tableOID:colName" -> id
	for tableOID, cols := range ext.Columns {
		tid, ok := tableIDByOID[tableOID]
		if !ok {
			continue
		}
		for _, c := range cols {
			res, err := tx.Exec(
				"INSERT INTO columns (table_id, name, type, nullable, position) VALUES (?, ?, ?, ?, ?)",
				tid, c.Name, c.DataType, c.Nullable, c.Attnum,
			)
			if err != nil {
				return fmt.Errorf("insert column %s: %w", c.Name, err)
			}
			cid, _ := res.LastInsertId()
			columnIDByTableCol[tableOID+":"+c.Name] = cid
		}
	}

	// Store constraints
	for _, c := range ext.Constraints {
		tid, ok := tableIDByOID[c.TableOID]
		if !ok {
			continue
		}
		switch c.Kind {
		case "PK":
			for _, col := range splitCSV(c.SrcColumns) {
				tx.Exec("INSERT OR IGNORE INTO primary_keys (table_id, column_name) VALUES (?, ?)", tid, col)
			}
		case "UQ":
			// Store as index for now
			tx.Exec("INSERT INTO indexes_info (table_id, name, columns, is_unique) VALUES (?, ?, ?, 1)",
				tid, c.ConName, c.SrcColumns)
		case "FK":
			refOID := findOIDByTable(ext, c.RefSchema, c.RefTable)
			refTid, ok := tableIDByOID[refOID]
			if !ok {
				continue
			}
			tx.Exec(
				"INSERT INTO foreign_keys (table_id, src_columns, ref_table_id, dst_columns, constraint_name) VALUES (?, ?, ?, ?, ?)",
				tid, c.SrcColumns, refTid, c.DstColumns, c.ConName,
			)
		}
	}

	return tx.Commit()
}

func findOIDByTable(ext *schema.ExtractedSchema, schema, name string) string {
	for _, t := range ext.Tables {
		if t.Schema == schema && t.Name == name {
			return t.OID
		}
	}
	return ""
}

func splitCSV(s string) []string {
	var parts []string
	for _, p := range splitComma(s) {
		parts = append(parts, trimSpace(p))
	}
	return parts
}

func splitComma(s string) []string {
	var result []string
	start := 0
	for i := 0; i < len(s); i++ {
		if s[i] == ',' {
			result = append(result, s[start:i])
			start = i + 1
		}
	}
	result = append(result, s[start:])
	return result
}

func trimSpace(s string) string {
	start, end := 0, len(s)
	for start < end && s[start] == ' ' {
		start++
	}
	for end > start && s[end-1] == ' ' {
		end--
	}
	return s[start:end]
}

func populateFTS(store *db.Store) error {
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

		valStr := ""
		if len(valueTokens) > 0 {
			valStr = joinStrings(valueTokens, " ")
		}
		store.DB().Exec(
			"INSERT INTO search_index (table_name, column_names, value_tokens) VALUES (?, ?, ?)",
			tableName, colNames, valStr,
		)
	}
	return rows.Err()
}

func joinStrings(strs []string, sep string) string {
	if len(strs) == 0 {
		return ""
	}
	result := strs[0]
	for _, s := range strs[1:] {
		result += sep + s
	}
	return result
}
