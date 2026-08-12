package testutil

import (
	"fmt"
	"testing"

	"github.com/shrsv/dbctx/internal/db"
)

// domainTableNames gives NewLargeStore's generated tables realistic names
// (rather than table_0, table_1, ...) so benchmark/recall queries against
// them exercise the same kind of lexical/semantic surface a real SaaS
// database would. Recycled with a numeric suffix if more tables are
// requested than there are names.
var domainTableNames = []string{
	"users", "accounts", "organizations", "teams", "memberships",
	"customers", "orders", "order_items", "products", "categories",
	"payments", "invoices", "refunds", "subscriptions", "plans",
	"coupons", "carts", "cart_items", "wishlists", "reviews",
	"repositories", "pull_requests", "commits", "branches", "deployments",
	"comments", "notifications", "sessions", "audit_logs", "webhooks",
	"tags", "articles", "media_assets", "tickets", "ticket_messages",
	"shipments", "addresses", "warehouses", "inventory", "suppliers",
	"employees", "departments", "projects", "tasks", "milestones",
	"integrations", "api_keys", "roles", "permissions", "feature_flags",
}

// stateValueSets provides plausible observed values for state-like columns,
// varied by column name so different tables don't all look identical.
var stateValueSets = map[string][]string{
	"status":   {"pending", "active", "completed", "failed", "cancelled"},
	"state":    {"open", "closed", "archived"},
	"role":     {"admin", "member", "viewer"},
	"priority": {"low", "medium", "high", "urgent"},
	"provider": {"github", "gitlab", "bitbucket"},
	"plan":     {"free", "pro", "enterprise"},
}

// NewLargeStore builds an in-memory store with a procedurally generated
// schema of numTables tables — realistic table names, ~10 columns each,
// a connected foreign-key graph, state/categorical columns with observed
// values, and a JSONB "metadata" column with sample paths on roughly a
// third of tables — then populates field stats/values, JSONB paths, and
// the FTS index. This exists because benchmarking semantic/hybrid
// retrieval against the 4-table hand-written fixture used elsewhere
// wouldn't say much about performance at a realistic schema size (see
// README "Performance" — dbctx's own benchmarks target 50+ table
// databases).
func NewLargeStore(tb testing.TB, numTables int) *db.Store {
	tb.Helper()
	return NewLargeStoreAt(tb, numTables, "")
}

// NewLargeStoreAt is [NewLargeStore] backed by a .dtx file at path instead
// of an in-memory database, for benchmarks that need to measure file-backed
// load/query cost (e.g. reopening a .dtx with a populated semantic index).
func NewLargeStoreAt(tb testing.TB, numTables int, path string) *db.Store {
	tb.Helper()

	store, err := db.OpenStore(path)
	if err != nil {
		tb.Fatalf("OpenStore: %v", err)
	}
	tb.Cleanup(func() { store.Close() })
	if err := store.InitSchema(); err != nil {
		tb.Fatalf("InitSchema: %v", err)
	}
	if err := store.InitFTS(); err != nil {
		tb.Fatalf("InitFTS: %v", err)
	}

	exec := func(q string, args ...any) {
		tb.Helper()
		if _, err := store.DB().Exec(q, args...); err != nil {
			tb.Fatalf("exec %q %v: %v", q, args, err)
		}
	}

	tableID := func(i int) int64 { return int64(i + 1) }
	tableName := func(i int) string {
		base := domainTableNames[i%len(domainTableNames)]
		if i >= len(domainTableNames) {
			return fmt.Sprintf("%s_%d", base, i/len(domainTableNames))
		}
		return base
	}

	commonCols := []string{"id", "name", "description", "created_at", "updated_at"}
	stateCols := []string{"status", "state", "role", "priority", "provider", "plan"}

	for i := 0; i < numTables; i++ {
		tid := tableID(i)
		name := tableName(i)
		exec(`INSERT INTO tables (id, schema, name, row_estimate) VALUES (?, 'public', ?, ?)`, tid, name, 1000+i*137)

		pos := 1
		exec(`INSERT INTO columns (table_id, name, type, nullable, position) VALUES (?, 'id', 'integer', 0, ?)`, tid, pos)
		exec(`INSERT INTO primary_keys (table_id, column_name) VALUES (?, 'id')`, tid)
		pos++

		for _, c := range commonCols[1:] {
			typ := "text"
			if c == "created_at" || c == "updated_at" {
				typ = "timestamptz"
			}
			exec(`INSERT INTO columns (table_id, name, type, nullable, position) VALUES (?, ?, ?, 0, ?)`, tid, c, typ, pos)
			pos++
		}

		// One FK to a nearby earlier table, forming a connected FK graph
		// (a shallow tree rather than one long chain, so FK expansion
		// benchmarks see realistic branching) similar in spirit to a real
		// app schema.
		if i > 0 {
			refIdx := (i - 1) / 3
			refTid := tableID(refIdx)
			fkCol := tableName(refIdx) + "_id"
			// strip a trailing 's' for a slightly more natural FK column name
			if len(fkCol) > 4 && fkCol[len(fkCol)-4] == 's' {
				fkCol = fkCol[:len(fkCol)-4] + fkCol[len(fkCol)-3:]
			}
			exec(`INSERT INTO columns (table_id, name, type, nullable, position) VALUES (?, ?, 'integer', 0, ?)`, tid, fkCol, pos)
			exec(`INSERT INTO foreign_keys (table_id, src_columns, ref_table_id, dst_columns, constraint_name) VALUES (?, ?, ?, 'id', ?)`,
				tid, fkCol, refTid, fmt.Sprintf("%s_%s_fk", name, fkCol))
			pos++
		}

		// A state-like column (varies by table so not every table has
		// identical values).
		stateCol := stateCols[i%len(stateCols)]
		exec(`INSERT INTO columns (table_id, name, type, nullable, position) VALUES (?, ?, 'text', 0, ?)`, tid, stateCol, pos)
		var stateColID int64
		store.DB().QueryRow(`SELECT id FROM columns WHERE table_id = ? AND name = ?`, tid, stateCol).Scan(&stateColID)
		values := stateValueSets[stateCol]
		exec(`INSERT INTO field_stats (column_id, distinct_count, null_count, is_state_like, is_categorical) VALUES (?, ?, 0, 1, 1)`,
			stateColID, len(values))
		for _, v := range values {
			exec(`INSERT INTO field_values (column_id, value, frequency) VALUES (?, ?, 100)`, stateColID, v)
		}
		pos++

		// Roughly a third of tables get a JSONB metadata column with a
		// couple of sample paths, exercising the jsonb_path semantic
		// object path.
		if i%3 == 0 {
			exec(`INSERT INTO columns (table_id, name, type, nullable, position) VALUES (?, 'metadata', 'jsonb', 1, ?)`, tid, pos)
			var metaColID int64
			store.DB().QueryRow(`SELECT id FROM columns WHERE table_id = ? AND name = 'metadata'`, tid).Scan(&metaColID)
			exec(`INSERT INTO jsonb_paths (column_id, path, inferred_type, distinct_count, sample_values) VALUES (?, '$.source', 'string', 2, 'web(500), mobile(300)')`, metaColID)
			exec(`INSERT INTO jsonb_paths (column_id, path, inferred_type, distinct_count, sample_values) VALUES (?, '$.version', 'string', 3, '1(100), 2(200)')`, metaColID)
		}
	}

	// FTS is column/value-driven (see internal/search.PopulateFTS), so
	// reuse it here too rather than duplicating its SQL.
	if _, err := store.DB().Exec("DELETE FROM search_index"); err != nil {
		tb.Fatalf("clear search_index: %v", err)
	}
	rows, err := store.DB().Query(`
		SELECT t.id, t.name, GROUP_CONCAT(c.name, ' ') FROM tables t
		JOIN columns c ON c.table_id = t.id
		GROUP BY t.id, t.name
	`)
	if err != nil {
		tb.Fatalf("query for fts: %v", err)
	}
	defer rows.Close()
	for rows.Next() {
		var id int64
		var name, cols string
		if rows.Scan(&id, &name, &cols) != nil {
			continue
		}
		exec(`INSERT INTO search_index (table_name, column_names, value_tokens) VALUES (?, ?, '')`, name, cols)
	}

	return store
}

// OpenExistingStore opens an already-built .dtx file at path, registering
// tb.Cleanup to close it. Used by benchmarks that need to measure
// file-backed reopen cost separately from the initial build (see
// NewLargeStoreAt).
func OpenExistingStore(tb testing.TB, path string) *db.Store {
	tb.Helper()
	store, err := db.OpenStore(path)
	if err != nil {
		tb.Fatalf("OpenStore(%q): %v", path, err)
	}
	return store
}
