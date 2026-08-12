//go:build integration

package dbctx

import (
	"context"
	"fmt"
	"os"
	"testing"
	"time"

	"github.com/shrsv/dbctx/internal/analyze"
	"github.com/shrsv/dbctx/internal/db"
	"github.com/shrsv/dbctx/internal/schema"
	"github.com/shrsv/dbctx/internal/search"
)

// TestIntegration_PerfBuild measures end-to-end build time and reports
// per-phase timings. Run with:
//
//	make test-integration
func TestIntegration_PerfBuild(t *testing.T) {
	dsn := os.Getenv("DATABASE_URL")
	if dsn == "" {
		t.Skip("DATABASE_URL not set")
	}

	ctx := context.Background()

	// Time the full build
	start := time.Now()
	idx, err := Build(ctx, dsn, nil)
	if err != nil {
		t.Fatalf("Build: %v", err)
	}
	defer idx.Close()
	buildDuration := time.Since(start)

	stats, _ := idx.Stats()
	tables, _ := idx.Tables()

	t.Logf("=== Full Build ===")
	t.Logf("Duration:  %v", buildDuration)
	t.Logf("Tables:    %d", len(tables))
	t.Logf("Columns:   %d", stats.Columns)
	t.Logf("FKs:       %d", stats.ForeignKeys)
	t.Logf("State:     %d", stats.StateFields)
	t.Logf("Cat:       %d", stats.CategoricalFields)
	t.Logf("JSONB:     %d", stats.JSONBPaths)

	// Time query
	queryStart := time.Now()
	result, err := idx.Query("id")
	if err != nil {
		t.Fatalf("Query: %v", err)
	}
	queryDuration := time.Since(queryStart)
	t.Logf("=== Query('id') ===")
	t.Logf("Duration:  %v", queryDuration)
	t.Logf("Matched:   %d tables", result.Matched().Len())

	// Time text rendering
	textStart := time.Now()
	text := result.Matched().Text()
	textDuration := time.Since(textStart)
	t.Logf("=== Text() render ===")
	t.Logf("Duration:  %v", textDuration)
	t.Logf("Output:    %d chars", len(text))
}

// TestIntegration_PerfPhases measures each build phase individually.
func TestIntegration_PerfPhases(t *testing.T) {
	dsn := os.Getenv("DATABASE_URL")
	if dsn == "" {
		t.Skip("DATABASE_URL not set")
	}

	ctx := context.Background()
	schemas := "public"

	// Connect
	connStart := time.Now()
	pg, err := db.Connect(ctx, dsn)
	if err != nil {
		t.Fatalf("Connect: %v", err)
	}
	defer pg.Close(ctx)
	connDur := time.Since(connStart)
	t.Logf("Connect:   %v", connDur)

	// Schema extraction
	schemaStart := time.Now()
	ext, err := schema.Extract(ctx, pg, schemas)
	if err != nil {
		t.Fatalf("Extract: %v", err)
	}
	schemaDur := time.Since(schemaStart)
	t.Logf("Schema:    %v (%d tables, %d constraints)", schemaDur, len(ext.Tables), len(ext.Constraints))

	// Open in-memory store
	store, err := db.OpenStore("")
	if err != nil {
		t.Fatalf("OpenStore: %v", err)
	}
	defer store.Close()
	store.InitSchema()

	// Store schema
	storeStart := time.Now()
	if err := storeSchema(store, ext); err != nil {
		t.Fatalf("storeSchema: %v", err)
	}
	storeDur := time.Since(storeStart)
	t.Logf("Store:     %v", storeDur)

	// Field analysis
	fieldStart := time.Now()
	if err := analyze.AnalyzeFields(ctx, pg, ext, store, schemas); err != nil {
		t.Fatalf("AnalyzeFields: %v", err)
	}
	fieldDur := time.Since(fieldStart)
	t.Logf("Fields:    %v", fieldDur)

	// JSONB analysis
	jsonbStart := time.Now()
	if err := analyze.AnalyzeJSONB(ctx, pg, ext, store); err != nil {
		t.Fatalf("AnalyzeJSONB: %v", err)
	}
	jsonbDur := time.Since(jsonbStart)
	t.Logf("JSONB:     %v", jsonbDur)

	// FTS
	ftsStart := time.Now()
	store.InitFTS()
	search.PopulateFTS(store)
	ftsDur := time.Since(ftsStart)
	t.Logf("FTS:       %v", ftsDur)

	total := connDur + schemaDur + storeDur + fieldDur + jsonbDur + ftsDur
	t.Logf("=== Total: %v ===", total)

	// Report phase breakdown
	t.Logf("--- Phase breakdown ---")
	phases := []struct {
		name string
		dur  time.Duration
	}{
		{"Connect", connDur},
		{"Schema", schemaDur},
		{"Store", storeDur},
		{"Fields", fieldDur},
		{"JSONB", jsonbDur},
		{"FTS", ftsDur},
	}
	for _, p := range phases {
		pct := float64(p.dur) / float64(total) * 100
		t.Logf("  %-10s %8v  (%5.1f%%)", p.name, p.dur, pct)
	}
}

// TestIntegration_PerfQueryTypes measures different query patterns.
func TestIntegration_PerfQueryTypes(t *testing.T) {
	dsn := os.Getenv("DATABASE_URL")
	if dsn == "" {
		t.Skip("DATABASE_URL not set")
	}

	ctx := context.Background()
	idx, err := Build(ctx, dsn, nil)
	if err != nil {
		t.Fatalf("Build: %v", err)
	}
	defer idx.Close()

	queries := []string{
		"id",
		"reviews",
		"failed reviews last month",
		"revews",          // fuzzy
		"nonexistent_xyz", // no match
	}

	for _, q := range queries {
		start := time.Now()
		result, err := idx.Query(q)
		dur := time.Since(start)

		if err != nil {
			t.Errorf("Query(%q): %v", q, err)
			continue
		}

		matched := result.Matched().Len()
		scoredOnly := result.ScoredOnly().Len()

		// Time text rendering
		textStart := time.Now()
		text := result.Matched().Text()
		textDur := time.Since(textStart)

		fmt.Printf("  %-25q  query=%8v  matched=%d  scored=%d  text=%8v  chars=%d\n",
			q, dur, matched, scoredOnly, textDur, len(text))
	}
}
