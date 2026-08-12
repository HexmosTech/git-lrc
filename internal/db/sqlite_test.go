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

func TestInitSemanticSchema(t *testing.T) {
	store, err := OpenStore("")
	if err != nil {
		t.Fatalf("OpenStore: %v", err)
	}
	defer store.Close()

	if err := store.InitSchema(); err != nil {
		t.Fatalf("InitSchema: %v", err)
	}
	if err := store.InitSemanticSchema(); err != nil {
		t.Fatalf("InitSemanticSchema: %v", err)
	}

	var count int
	err = store.DB().QueryRow(
		"SELECT COUNT(*) FROM sqlite_master WHERE type='table' AND name='semantic_objects'",
	).Scan(&count)
	if err != nil {
		t.Errorf("check semantic_objects: %v", err)
	} else if count != 1 {
		t.Error("semantic_objects table not found")
	}
}

func TestInitSemanticSchema_Idempotent(t *testing.T) {
	store, err := OpenStore("")
	if err != nil {
		t.Fatalf("OpenStore: %v", err)
	}
	defer store.Close()

	if err := store.InitSchema(); err != nil {
		t.Fatalf("InitSchema: %v", err)
	}
	if err := store.InitSemanticSchema(); err != nil {
		t.Fatalf("first InitSemanticSchema: %v", err)
	}
	if err := store.InitSemanticSchema(); err != nil {
		t.Fatalf("second InitSemanticSchema (should be idempotent): %v", err)
	}
}

func TestInitTerminologySchema(t *testing.T) {
	store, err := OpenStore("")
	if err != nil {
		t.Fatalf("OpenStore: %v", err)
	}
	defer store.Close()

	if err := store.InitSchema(); err != nil {
		t.Fatalf("InitSchema: %v", err)
	}
	if err := store.InitTerminologySchema(); err != nil {
		t.Fatalf("InitTerminologySchema: %v", err)
	}

	var count int
	err = store.DB().QueryRow(
		"SELECT COUNT(*) FROM sqlite_master WHERE type='table' AND name='terminology'",
	).Scan(&count)
	if err != nil {
		t.Errorf("check terminology: %v", err)
	} else if count != 1 {
		t.Error("terminology table not found")
	}
}

// TestSemanticSchema_AddableToOldStore verifies the core .dtx backward
// compatibility guarantee: a store built with only the original InitSchema
// (i.e. a .dtx file predating semantic/terminology support) can still be
// opened and used for lexical queries, and can optionally gain the new
// tables later without disturbing existing data.
func TestSemanticSchema_AddableToOldStore(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "old.dtx")

	store, err := OpenStore(path)
	if err != nil {
		t.Fatalf("OpenStore: %v", err)
	}
	if err := store.InitSchema(); err != nil {
		t.Fatalf("InitSchema (old-format build): %v", err)
	}
	store.DB().Exec("INSERT INTO tables (schema, name, row_estimate) VALUES ('public', 'orders', 100)")
	store.Close()

	// Reopen as if this were an old .dtx file being used by a newer dbctx.
	reopened, err := OpenStore(path)
	if err != nil {
		t.Fatalf("reopen: %v", err)
	}
	defer reopened.Close()

	var name string
	if err := reopened.DB().QueryRow("SELECT name FROM tables WHERE name='orders'").Scan(&name); err != nil {
		t.Fatalf("old data unreadable after reopen: %v", err)
	}

	// Semantic/terminology tables must not exist yet on this old file.
	var count int
	reopened.DB().QueryRow("SELECT COUNT(*) FROM sqlite_master WHERE type='table' AND name='semantic_objects'").Scan(&count)
	if count != 0 {
		t.Error("semantic_objects should not exist on an old-format store until explicitly added")
	}

	// Adding it later must not disturb existing data.
	if err := reopened.InitSemanticSchema(); err != nil {
		t.Fatalf("InitSemanticSchema on old store: %v", err)
	}
	if err := reopened.InitTerminologySchema(); err != nil {
		t.Fatalf("InitTerminologySchema on old store: %v", err)
	}
	if err := reopened.DB().QueryRow("SELECT name FROM tables WHERE name='orders'").Scan(&name); err != nil {
		t.Fatalf("old data unreadable after adding new schema: %v", err)
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
