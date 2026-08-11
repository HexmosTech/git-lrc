package db

import (
	"database/sql"
	"fmt"

	_ "modernc.org/sqlite"
)

type Store struct {
	db *sql.DB
}

// OpenStore opens or creates a SQLite database at the given path.
// If path is empty, an in-memory database is used (no file created).
// In-memory databases are useful for ephemeral indexes or testing.
func OpenStore(path string) (*Store, error) {
	dsn := path
	if dsn == "" {
		dsn = "file::memory:?cache=shared"
	}
	db, err := sql.Open("sqlite", dsn)
	if err != nil {
		return nil, fmt.Errorf("open sqlite: %w", err)
	}
	if path != "" {
		db.Exec("PRAGMA journal_mode=WAL")
		db.Exec("PRAGMA synchronous=NORMAL")
	}
	return &Store{db: db}, nil
}

func (s *Store) Close() error {
	return s.db.Close()
}

func (s *Store) DB() *sql.DB {
	return s.db
}

func (s *Store) InitSchema() error {
	stmts := []string{
		`CREATE TABLE IF NOT EXISTS tables (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			schema TEXT NOT NULL,
			name TEXT NOT NULL,
			row_estimate REAL DEFAULT 0,
			UNIQUE(schema, name)
		)`,
		`CREATE TABLE IF NOT EXISTS columns (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			table_id INTEGER NOT NULL REFERENCES tables(id),
			name TEXT NOT NULL,
			type TEXT NOT NULL,
			nullable BOOLEAN DEFAULT 0,
			position INTEGER NOT NULL,
			UNIQUE(table_id, name)
		)`,
		`CREATE TABLE IF NOT EXISTS primary_keys (
			table_id INTEGER NOT NULL REFERENCES tables(id),
			column_name TEXT NOT NULL,
			PRIMARY KEY(table_id, column_name)
		)`,
		`CREATE TABLE IF NOT EXISTS foreign_keys (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			table_id INTEGER NOT NULL REFERENCES tables(id),
			src_columns TEXT NOT NULL,
			ref_table_id INTEGER NOT NULL REFERENCES tables(id),
			dst_columns TEXT NOT NULL,
			constraint_name TEXT
		)`,
		`CREATE TABLE IF NOT EXISTS indexes_info (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			table_id INTEGER NOT NULL REFERENCES tables(id),
			name TEXT NOT NULL,
			columns TEXT NOT NULL,
			is_unique BOOLEAN DEFAULT 0
		)`,
		`CREATE TABLE IF NOT EXISTS field_stats (
			column_id INTEGER PRIMARY KEY REFERENCES columns(id),
			distinct_count INTEGER,
			null_count INTEGER,
			is_state_like BOOLEAN DEFAULT 0,
			is_categorical BOOLEAN DEFAULT 0
		)`,
		`CREATE TABLE IF NOT EXISTS field_values (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			column_id INTEGER NOT NULL REFERENCES columns(id),
			value TEXT,
			frequency INTEGER DEFAULT 1
		)`,
		`CREATE TABLE IF NOT EXISTS jsonb_paths (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			column_id INTEGER NOT NULL REFERENCES columns(id),
			path TEXT NOT NULL,
			inferred_type TEXT,
			distinct_count INTEGER,
			sample_values TEXT,
			UNIQUE(column_id, path)
		)`,
		`CREATE TABLE IF NOT EXISTS metadata (
			key TEXT PRIMARY KEY,
			value TEXT
		)`,
	}
	for _, stmt := range stmts {
		if _, err := s.db.Exec(stmt); err != nil {
			return fmt.Errorf("exec schema stmt: %w\n%s", err, stmt)
		}
	}
	return nil
}

func (s *Store) InitFTS() error {
	stmts := []string{
		`DROP TABLE IF EXISTS search_index`,
		`CREATE VIRTUAL TABLE search_index USING fts5(
			table_name,
			column_names,
			value_tokens,
			content='',
			tokenize='porter unicode61'
		)`,
	}
	for _, stmt := range stmts {
		if _, err := s.db.Exec(stmt); err != nil {
			return fmt.Errorf("exec fts stmt: %w\n%s", err, stmt)
		}
	}
	return nil
}
