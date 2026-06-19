package storage

import (
	"database/sql"
	"fmt"

	_ "modernc.org/sqlite"
)

// OpenInMemorySQLite opens a fresh in-memory sqlite database via the storage
// boundary. Used by the review-query engine to build an ephemeral table that is
// discarded when the handle is closed. The caller owns the returned handle and
// must Close() it; the in-memory database (and all its data) is destroyed once
// the last connection closes.
func OpenInMemorySQLite() (*sql.DB, error) {
	db, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		return nil, fmt.Errorf("failed to open in-memory sqlite database: %w", err)
	}
	if err := db.Ping(); err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("failed to connect in-memory sqlite database: %w", err)
	}
	return db, nil
}

// BulkInsert inserts many rows under a single transaction with a prepared
// statement — far faster than autocommitting each row (matters for large repos).
// query is executed as-is with no restriction on statement type, so callers
// must only pass trusted, internally-constructed SQL (e.g. a fixed INSERT
// template) — never untrusted/user-supplied text.
func BulkInsert(db *sql.DB, query string, rows [][]any) error {
	if db == nil {
		return fmt.Errorf("failed bulk insert: nil database handle")
	}
	tx, err := db.Begin()
	if err != nil {
		return fmt.Errorf("failed to begin transaction: %w", err)
	}
	stmt, err := tx.Prepare(query)
	if err != nil {
		_ = tx.Rollback()
		return fmt.Errorf("failed to prepare insert: %w", err)
	}
	defer func() { _ = stmt.Close() }()

	for _, args := range rows {
		if _, err := stmt.Exec(args...); err != nil {
			_ = tx.Rollback()
			return fmt.Errorf("failed bulk insert exec: %w", err)
		}
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("failed to commit insert transaction: %w", err)
	}
	return nil
}

// QueryRows runs a query and returns the column names plus each row
// stringified (NULL -> ""). Keeping database/sql access inside the storage
// boundary lets callers render results without importing database/sql.
// This package does not itself enforce that query is read-only or a single
// statement — when query may originate from user input (as it does for the
// reviewquery engine), the caller is responsible for validating it first.
func QueryRows(db *sql.DB, query string, args ...any) (columns []string, rows [][]string, err error) {
	if db == nil {
		return nil, nil, fmt.Errorf("failed SQL query: nil database handle")
	}

	result, err := db.Query(query, args...)
	if err != nil {
		return nil, nil, fmt.Errorf("failed SQL query: %w", err)
	}
	defer func() { _ = result.Close() }()

	columns, err = result.Columns()
	if err != nil {
		return nil, nil, fmt.Errorf("failed reading query columns: %w", err)
	}

	for result.Next() {
		raw := make([]sql.NullString, len(columns))
		scanTargets := make([]any, len(columns))
		for i := range raw {
			scanTargets[i] = &raw[i]
		}
		if err := result.Scan(scanTargets...); err != nil {
			return nil, nil, fmt.Errorf("failed scanning query row: %w", err)
		}
		row := make([]string, len(columns))
		for i, ns := range raw {
			if ns.Valid {
				row[i] = ns.String
			}
		}
		rows = append(rows, row)
	}
	if err := result.Err(); err != nil {
		return nil, nil, fmt.Errorf("failed iterating query rows: %w", err)
	}
	return columns, rows, nil
}
