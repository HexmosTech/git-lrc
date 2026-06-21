package storage

import (
	"database/sql"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	_ "modernc.org/sqlite"
)

const defaultSQLiteBusyTimeoutMS = 5000

func sqliteBusyTimeoutMS() int {
	raw := strings.TrimSpace(os.Getenv("LRC_SQLITE_BUSY_TIMEOUT_MS"))
	if raw == "" {
		return defaultSQLiteBusyTimeoutMS
	}
	value, err := strconv.Atoi(raw)
	if err != nil || value <= 0 {
		return defaultSQLiteBusyTimeoutMS
	}
	return value
}

// WriteFileAtomically atomically replaces a target file with provided content.
func WriteFileAtomically(path string, data []byte, mode os.FileMode) error {
	if strings.TrimSpace(path) == "" {
		return fmt.Errorf("failed to atomically write file: empty target path")
	}

	targetDir := filepath.Dir(path)
	if targetDir == "" {
		targetDir = "."
	}
	if err := os.MkdirAll(targetDir, 0700); err != nil {
		return fmt.Errorf("failed to create target directory %s: %w", targetDir, err)
	}

	tmpFile, err := os.CreateTemp(targetDir, ".lrc-config-*.tmp")
	if err != nil {
		return fmt.Errorf("failed to create temporary file: %w", err)
	}
	tmpPath := tmpFile.Name()

	if _, err := tmpFile.Write(data); err != nil {
		_ = tmpFile.Close()
		_ = os.Remove(tmpPath)
		return fmt.Errorf("failed to write temporary file: %w", err)
	}

	if err := tmpFile.Sync(); err != nil {
		_ = tmpFile.Close()
		_ = os.Remove(tmpPath)
		return fmt.Errorf("failed to flush temporary file: %w", err)
	}

	if err := tmpFile.Close(); err != nil {
		_ = os.Remove(tmpPath)
		return fmt.Errorf("failed to finalize temporary file: %w", err)
	}

	if err := os.Chmod(tmpPath, mode); err != nil {
		_ = os.Remove(tmpPath)
		return fmt.Errorf("failed to set permissions on temporary file: %w", err)
	}

	if err := os.Rename(tmpPath, path); err != nil {
		_ = os.Remove(tmpPath)
		return fmt.Errorf("failed to atomically replace %s: %w", path, err)
	}

	return nil
}

// ReadFile reads bytes from disk via the storage boundary.
func ReadFile(path string) ([]byte, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("failed to read file %s: %w", path, err)
	}
	return data, nil
}

// WriteFile writes bytes to disk via the storage boundary.
func WriteFile(path string, data []byte, mode os.FileMode) error {
	if err := os.WriteFile(path, data, mode); err != nil {
		return fmt.Errorf("failed to write file %s: %w", path, err)
	}
	return nil
}

// MkdirAll creates directories via the storage boundary.
func MkdirAll(path string, perm os.FileMode) error {
	if err := os.MkdirAll(path, perm); err != nil {
		return fmt.Errorf("failed to create directory %s: %w", path, err)
	}
	return nil
}

// Remove deletes a file via the storage boundary.
func Remove(path string) error {
	if err := os.Remove(path); err != nil {
		return fmt.Errorf("failed to remove file %s: %w", path, err)
	}
	return nil
}

// RemoveAll recursively deletes a path via the storage boundary.
func RemoveAll(path string) error {
	if err := os.RemoveAll(path); err != nil {
		return fmt.Errorf("failed to remove path %s: %w", path, err)
	}
	return nil
}

// Rename moves a file via the storage boundary.
func Rename(oldPath, newPath string) error {
	if err := os.Rename(oldPath, newPath); err != nil {
		return fmt.Errorf("failed to rename %s to %s: %w", oldPath, newPath, err)
	}
	return nil
}

// Chmod updates file permissions via the storage boundary.
func Chmod(path string, mode os.FileMode) error {
	if err := os.Chmod(path, mode); err != nil {
		return fmt.Errorf("failed to chmod %s: %w", path, err)
	}
	return nil
}

// CreateTemp creates a temporary file via the storage boundary.
func CreateTemp(dir, pattern string) (*os.File, error) {
	file, err := os.CreateTemp(dir, pattern)
	if err != nil {
		return nil, fmt.Errorf("failed to create temporary file: %w", err)
	}
	return file, nil
}

// OpenFile opens a file via the storage boundary.
func OpenFile(path string, flag int, perm os.FileMode) (*os.File, error) {
	file, err := os.OpenFile(path, flag, perm)
	if err != nil {
		return nil, fmt.Errorf("failed to open file %s: %w", path, err)
	}
	return file, nil
}

// OpenSQLite opens a sqlite database via the storage boundary.
func OpenSQLite(dbPath string) (*sql.DB, error) {
	dsn := fmt.Sprintf("%s?_journal_mode=WAL&_busy_timeout=%d", dbPath, sqliteBusyTimeoutMS())
	db, err := sql.Open("sqlite", dsn)
	if err != nil {
		return nil, fmt.Errorf("failed to open sqlite database: %w", err)
	}

	if err := db.Ping(); err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("failed to connect sqlite database: %w", err)
	}

	return db, nil
}

// ExecSQL executes a SQL statement via the storage boundary.
func ExecSQL(db *sql.DB, query string, args ...any) (sql.Result, error) {
	if db == nil {
		return nil, fmt.Errorf("failed SQL exec: nil database handle")
	}

	result, err := db.Exec(query, args...)
	if err != nil {
		trimmedQuery := ""
		for _, line := range strings.Split(query, "\n") {
			line = strings.TrimSpace(line)
			if line != "" {
				if trimmedQuery != "" {
					trimmedQuery += " "
				}
				trimmedQuery += line
			}
		}
		queryRunes := []rune(trimmedQuery)
		if len(queryRunes) > 240 {
			trimmedQuery = string(queryRunes[:240]) + "..."
		}
		return nil, fmt.Errorf("failed SQL exec (%s): %w", trimmedQuery, err)
	}
	return result, nil
}
