//go:build integration

package dbctx

import (
	"context"
	"os"
	"strings"
	"testing"
)

// TestIntegration_Build runs the full Build pipeline against a real
// PostgreSQL database. Requires DATABASE_URL environment variable.
//
// Run with: make test-integration
func TestIntegration_Build(t *testing.T) {
	dsn := os.Getenv("DATABASE_URL")
	if dsn == "" {
		t.Skip("DATABASE_URL not set, skipping integration test")
	}

	ctx := context.Background()
	idx, err := Build(ctx, dsn, nil)
	if err != nil {
		t.Fatalf("Build: %v", err)
	}
	defer idx.Close()

	// Verify tables were extracted
	tables, err := idx.Tables()
	if err != nil {
		t.Fatalf("Tables: %v", err)
	}
	if len(tables) == 0 {
		t.Fatal("Build extracted 0 tables")
	}
	t.Logf("Extracted %d tables", len(tables))

	// Verify stats
	stats, err := idx.Stats()
	if err != nil {
		t.Fatalf("Stats: %v", err)
	}
	if stats.Tables == 0 {
		t.Error("Stats.Tables = 0")
	}
	if stats.Columns == 0 {
		t.Error("Stats.Columns = 0")
	}
	t.Logf("Stats: tables=%d columns=%d fks=%d state=%d cat=%d jsonb=%d",
		stats.Tables, stats.Columns, stats.ForeignKeys,
		stats.StateFields, stats.CategoricalFields, stats.JSONBPaths)

	// Verify query works
	result, err := idx.Query("id")
	if err != nil {
		t.Fatalf("Query: %v", err)
	}
	if len(result.Tables) == 0 {
		t.Error("Query('id') returned 0 tables")
	}

	// Verify report works
	var buf strings.Builder
	if err := idx.Report(&buf); err != nil {
		t.Fatalf("Report: %v", err)
	}
	if buf.Len() == 0 {
		t.Error("Report produced empty output")
	}

	// Verify text rendering with legend
	text := result.Matched().Text()
	if len(text) == 0 {
		t.Error("Text() produced empty output")
	}
}

func TestIntegration_BuildWithSchemas(t *testing.T) {
	dsn := os.Getenv("DATABASE_URL")
	if dsn == "" {
		t.Skip("DATABASE_URL not set, skipping integration test")
	}

	ctx := context.Background()
	idx, err := Build(ctx, dsn, &Options{Schemas: "public"})
	if err != nil {
		t.Fatalf("Build with schemas: %v", err)
	}
	defer idx.Close()

	tables, err := idx.Tables()
	if err != nil {
		t.Fatalf("Tables: %v", err)
	}
	if len(tables) == 0 {
		t.Error("Build with schemas extracted 0 tables")
	}
}

func TestIntegration_BuildToFile(t *testing.T) {
	dsn := os.Getenv("DATABASE_URL")
	if dsn == "" {
		t.Skip("DATABASE_URL not set, skipping integration test")
	}

	path := t.TempDir() + "/test.dtx"

	ctx := context.Background()
	idx, err := Build(ctx, dsn, &Options{Path: path})
	if err != nil {
		t.Fatalf("Build: %v", err)
	}
	idx.Close()

	// Re-open and verify
	idx2, err := Open(path)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer idx2.Close()

	tables, err := idx2.Tables()
	if err != nil {
		t.Fatalf("Tables: %v", err)
	}
	if len(tables) == 0 {
		t.Error("Re-opened index has 0 tables")
	}
}
