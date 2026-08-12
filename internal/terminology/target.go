package terminology

import (
	"fmt"
	"strings"

	"github.com/shrsv/dbctx/internal/db"
)

// Target notation:
//
//	table                    -> table-level target
//	table.column             -> column-level target
//	table.column:$.path      -> a JSONB path within column (":" separates
//	                            the column from the path since JSONB paths
//	                            themselves contain dots, e.g. "$.a.b")
//
// parseTarget splits a raw target string into its components without
// validating them against the schema (see validateTarget for that).
func parseTarget(raw string) (table, column, path string, err error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return "", "", "", fmt.Errorf("empty target")
	}

	rest := raw
	if i := strings.Index(raw, ":"); i >= 0 {
		rest = raw[:i]
		path = raw[i+1:]
		if path == "" {
			return "", "", "", fmt.Errorf("target %q has an empty JSONB path after ':'", raw)
		}
	}

	parts := strings.SplitN(rest, ".", 2)
	table = parts[0]
	if table == "" {
		return "", "", "", fmt.Errorf("target %q is missing a table name", raw)
	}
	if len(parts) == 2 {
		column = parts[1]
		if column == "" {
			return "", "", "", fmt.Errorf("target %q has an empty column name after '.'", raw)
		}
	}
	if path != "" && column == "" {
		return "", "", "", fmt.Errorf("target %q specifies a JSONB path but no column", raw)
	}
	return table, column, path, nil
}

func formatTarget(table, column, path string) string {
	s := table
	if column != "" {
		s += "." + column
	}
	if path != "" {
		s += ":" + path
	}
	return s
}

// validateTarget confirms table/column/path parsed from a target string
// refer to schema objects that actually exist in store, rejecting anything
// that doesn't — the "target object must exist" / "target field/path must
// exist" requirement. This is the only thing standing between an LLM's
// output and what gets persisted, so it deliberately trusts nothing.
func validateTarget(store *db.Store, table, column, path string) error {
	var tableID int64
	err := store.DB().QueryRow("SELECT id FROM tables WHERE name = ?", table).Scan(&tableID)
	if err != nil {
		return fmt.Errorf("table %q does not exist", table)
	}
	if column == "" {
		return nil
	}

	var columnID int64
	err = store.DB().QueryRow("SELECT id FROM columns WHERE table_id = ? AND name = ?", tableID, column).Scan(&columnID)
	if err != nil {
		return fmt.Errorf("column %q does not exist on table %q", column, table)
	}
	if path == "" {
		return nil
	}

	var count int
	store.DB().QueryRow("SELECT COUNT(*) FROM jsonb_paths WHERE column_id = ? AND path = ?", columnID, path).Scan(&count)
	if count == 0 {
		return fmt.Errorf("JSONB path %q does not exist on %s.%s", path, table, column)
	}
	return nil
}
