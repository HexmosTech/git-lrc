package reviewquery

import (
	"fmt"
	"log"
	"strings"

	"github.com/HexmosTech/git-lrc/storage"
)

// QueryResult is a generic tabular result: column headers + stringified rows.
// Invariant: every row in Rows has exactly len(Columns) cells. storage.QueryRows
// guarantees this when building a QueryResult; preserve it in any other
// construction path (formatters rely on it).
type QueryResult struct {
	Columns []string
	Rows    [][]string
}

const createTableSQL = `
CREATE TABLE review_log (
    hash         TEXT,
    short_hash   TEXT,
    author       TEXT,
    email        TEXT,
    date         TEXT,
    branch       TEXT,
    subject      TEXT,
    action       TEXT,
    iterations   INTEGER,
    coverage     INTEGER
);`

const insertSQL = `
INSERT INTO review_log
    (hash, short_hash, author, email, date, branch, subject, action, iterations, coverage)
VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?);`

// Run extracts the review history (scoped by filter), loads it into an in-memory
// SQLite table named review_log, and runs sqlText against it. sqlText must
// already be resolved (alias -> SQL) by the caller, but may otherwise be raw
// text typed by the user (an unresolved alias name is treated as ad-hoc SQL) —
// RunOnRecords enforces that it's a single read-only statement before running it.
func Run(f Filter, sqlText string) (QueryResult, error) {
	records, err := Extract(f)
	if err != nil {
		return QueryResult{}, err
	}
	return RunOnRecords(records, sqlText)
}

// validateReadOnlySQL guards the in-memory review_log database against
// anything but a single read-only SELECT/WITH statement. sqlText can come
// straight from a user's shell (unresolved alias -> raw positional args) or
// from a saved alias, so this rejects stacked statements (which could smuggle
// a DROP/ATTACH/PRAGMA in after a semicolon) and any non-SELECT statement type.
func validateReadOnlySQL(sqlText string) error {
	stmt := strings.TrimSpace(sqlText)
	if stmt == "" {
		return fmt.Errorf("query is empty")
	}
	stmt = strings.TrimSpace(strings.TrimSuffix(stmt, ";"))
	if strings.Contains(stmt, ";") {
		return fmt.Errorf("only a single SQL statement is allowed")
	}
	upper := strings.ToUpper(stmt)
	if !strings.HasPrefix(upper, "SELECT") && !strings.HasPrefix(upper, "WITH") {
		return fmt.Errorf("only read-only SELECT queries are allowed")
	}
	return nil
}

// RunOnRecords loads records into an in-memory review_log table and runs sqlText
// against it. Split out from Run so it can be tested/benchmarked without git.
func RunOnRecords(records []ReviewRecord, sqlText string) (QueryResult, error) {
	if err := validateReadOnlySQL(sqlText); err != nil {
		return QueryResult{}, err
	}

	db, err := storage.OpenInMemorySQLite()
	if err != nil {
		return QueryResult{}, err
	}
	defer func() {
		if cerr := db.Close(); cerr != nil {
			log.Printf("reviewquery: failed to close in-memory sqlite db: %v", cerr)
		}
	}()

	if _, err := storage.ExecSQL(db, createTableSQL); err != nil {
		return QueryResult{}, fmt.Errorf("failed to create review_log table: %w", err)
	}

	rows := make([][]any, 0, len(records))
	for _, r := range records {
		date := ""
		if !r.Date.IsZero() {
			date = r.Date.UTC().Format("2006-01-02T15:04:05Z")
		}
		rows = append(rows, []any{
			r.Hash, r.ShortHash, r.Author, r.Email, date,
			r.Branch, r.Subject, r.Action, r.Iterations, r.CoveragePct,
		})
	}
	if err := storage.BulkInsert(db, insertSQL, rows); err != nil {
		return QueryResult{}, err
	}

	columns, outRows, err := storage.QueryRows(db, sqlText)
	if err != nil {
		return QueryResult{}, err
	}
	return QueryResult{Columns: columns, Rows: outRows}, nil
}
