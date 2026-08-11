package db

import (
	"os"
	"path/filepath"
	"testing"
)

func TestOpenStoreMemory(t *testing.T) {
	store, err := OpenStore("")
	if err != nil {
		t.Fatalf("OpenStore(\"\") error: %v", err)
	}
	defer store.Close()

	if store.DB() == nil {
		t.Error("DB() returned nil")
	}

	// Verify it's usable
	var one int
	if err := store.DB().QueryRow("SELECT 1").Scan(&one); err != nil {
		t.Errorf("SELECT 1 failed: %v", err)
	}
	if one != 1 {
		t.Errorf("SELECT 1 = %d, want 1", one)
	}
}

func TestOpenStoreFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "test.dtx")

	store, err := OpenStore(path)
	if err != nil {
		t.Fatalf("OpenStore(%q) error: %v", path, err)
	}
	store.Close()

	if _, err := os.Stat(path); os.IsNotExist(err) {
		t.Errorf("file %s was not created", path)
	}
}

func TestOpenStoreFileReopen(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "test.dtx")

	// Create and write
	store, err := OpenStore(path)
	if err != nil {
		t.Fatalf("first open: %v", err)
	}
	store.DB().Exec("CREATE TABLE test (id INTEGER PRIMARY KEY, name TEXT)")
	store.DB().Exec("INSERT INTO test (name) VALUES (?)", "hello")
	store.Close()

	// Reopen and read
	store2, err := OpenStore(path)
	if err != nil {
		t.Fatalf("reopen: %v", err)
	}
	defer store2.Close()

	var name string
	if err := store2.DB().QueryRow("SELECT name FROM test WHERE id = 1").Scan(&name); err != nil {
		t.Errorf("read after reopen: %v", err)
	}
	if name != "hello" {
		t.Errorf("name = %q, want %q", name, "hello")
	}
}

func TestInitSchema(t *testing.T) {
	store, err := OpenStore("")
	if err != nil {
		t.Fatalf("OpenStore: %v", err)
	}
	defer store.Close()

	if err := store.InitSchema(); err != nil {
		t.Fatalf("InitSchema: %v", err)
	}

	// Verify all tables exist
	expected := []string{
		"tables", "columns", "primary_keys", "foreign_keys",
		"indexes_info", "field_stats", "field_values", "jsonb_paths", "metadata",
	}
	for _, name := range expected {
		var count int
		err := store.DB().QueryRow(
			"SELECT COUNT(*) FROM sqlite_master WHERE type='table' AND name=?",
			name,
		).Scan(&count)
		if err != nil {
			t.Errorf("check table %s: %v", name, err)
		} else if count != 1 {
			t.Errorf("table %s not found", name)
		}
	}
}

func TestInitSchema_Idempotent(t *testing.T) {
	store, err := OpenStore("")
	if err != nil {
		t.Fatalf("OpenStore: %v", err)
	}
	defer store.Close()

	if err := store.InitSchema(); err != nil {
		t.Fatalf("first InitSchema: %v", err)
	}
	if err := store.InitSchema(); err != nil {
		t.Fatalf("second InitSchema (should be idempotent): %v", err)
	}
}

func TestInitFTS(t *testing.T) {
	store, err := OpenStore("")
	if err != nil {
		t.Fatalf("OpenStore: %v", err)
	}
	defer store.Close()

	if err := store.InitSchema(); err != nil {
		t.Fatalf("InitSchema: %v", err)
	}
	if err := store.InitFTS(); err != nil {
		t.Fatalf("InitFTS: %v", err)
	}

	// Verify FTS table exists
	var count int
	err = store.DB().QueryRow(
		"SELECT COUNT(*) FROM sqlite_master WHERE type='table' AND name='search_index'",
	).Scan(&count)
	if err != nil {
		t.Errorf("check search_index: %v", err)
	} else if count != 1 {
		t.Error("search_index table not found")
	}
}
