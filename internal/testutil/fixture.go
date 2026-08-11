// Package testutil provides shared test helpers for dbctx tests.
package testutil

import (
	"strings"
	"testing"

	"github.com/shrsv/dbctx/internal/db"
	"github.com/shrsv/dbctx/internal/schema"
)

// FixtureSchema returns a hand-crafted ExtractedSchema with 4 tables
// covering every notation feature: PKs, FKs, state fields, categorical
// fields, nullable columns, JSONB, and representative values.
func FixtureSchema() *schema.ExtractedSchema {
	return &schema.ExtractedSchema{
		Tables: []schema.Table{
			{Schema: "public", Name: "orgs", OID: "100", RowEstimate: 500},
			{Schema: "public", Name: "pull_requests", OID: "101", RowEstimate: 5000},
			{Schema: "public", Name: "reviews", OID: "102", RowEstimate: 12000},
			{Schema: "public", Name: "comments", OID: "103", RowEstimate: 30000},
		},
		Columns: map[string][]schema.Column{
			"100": {
				{TableOID: "100", Name: "id", DataType: "integer", Nullable: false, Attnum: 1},
				{TableOID: "100", Name: "name", DataType: "text", Nullable: false, Attnum: 2},
				{TableOID: "100", Name: "plan", DataType: "character varying(20)", Nullable: false, Attnum: 3},
				{TableOID: "100", Name: "tier", DataType: "character varying(20)", Nullable: false, Attnum: 4},
			},
			"101": {
				{TableOID: "101", Name: "id", DataType: "integer", Nullable: false, Attnum: 1},
				{TableOID: "101", Name: "org_id", DataType: "integer", Nullable: false, Attnum: 2},
				{TableOID: "101", Name: "title", DataType: "text", Nullable: false, Attnum: 3},
				{TableOID: "101", Name: "number", DataType: "integer", Nullable: false, Attnum: 4},
			},
			"102": {
				{TableOID: "102", Name: "id", DataType: "integer", Nullable: false, Attnum: 1},
				{TableOID: "102", Name: "org_id", DataType: "integer", Nullable: false, Attnum: 2},
				{TableOID: "102", Name: "pr_id", DataType: "integer", Nullable: false, Attnum: 3},
				{TableOID: "102", Name: "status", DataType: "character varying(50)", Nullable: false, Attnum: 4},
				{TableOID: "102", Name: "priority", DataType: "integer", Nullable: false, Attnum: 5},
				{TableOID: "102", Name: "metadata", DataType: "jsonb", Nullable: false, Attnum: 6},
				{TableOID: "102", Name: "body", DataType: "text", Nullable: true, Attnum: 7},
			},
			"103": {
				{TableOID: "103", Name: "id", DataType: "integer", Nullable: false, Attnum: 1},
				{TableOID: "103", Name: "review_id", DataType: "integer", Nullable: false, Attnum: 2},
				{TableOID: "103", Name: "author", DataType: "text", Nullable: false, Attnum: 3},
				{TableOID: "103", Name: "body", DataType: "text", Nullable: true, Attnum: 4},
			},
		},
		Constraints: []schema.Constraint{
			{Kind: "PK", TableOID: "100", ConName: "orgs_pkey", SrcColumns: "id"},
			{Kind: "PK", TableOID: "101", ConName: "pull_requests_pkey", SrcColumns: "id"},
			{Kind: "PK", TableOID: "102", ConName: "reviews_pkey", SrcColumns: "id"},
			{Kind: "PK", TableOID: "103", ConName: "comments_pkey", SrcColumns: "id"},
			{Kind: "FK", TableOID: "101", ConName: "pr_org_fk", RefSchema: "public", RefTable: "orgs", SrcColumns: "org_id", DstColumns: "id"},
			{Kind: "FK", TableOID: "102", ConName: "review_org_fk", RefSchema: "public", RefTable: "orgs", SrcColumns: "org_id", DstColumns: "id"},
			{Kind: "FK", TableOID: "102", ConName: "review_pr_fk", RefSchema: "public", RefTable: "pull_requests", SrcColumns: "pr_id", DstColumns: "id"},
			{Kind: "FK", TableOID: "103", ConName: "comment_review_fk", RefSchema: "public", RefTable: "reviews", SrcColumns: "review_id", DstColumns: "id"},
		},
	}
}

// SeedStore populates an already-initialized SQLite store with schema data
// from an ExtractedSchema, plus field stats, field values, and JSONB paths.
// This replicates the storeSchema + AnalyzeFields + AnalyzeJSONB pipeline
// without requiring PostgreSQL.
func SeedStore(store *db.Store, ext *schema.ExtractedSchema) error {
	// --- Replicate storeSchema ---
	tx, err := store.DB().Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	tableIDByOID := make(map[string]int64)
	for _, t := range ext.Tables {
		res, err := tx.Exec(
			"INSERT INTO tables (schema, name, row_estimate) VALUES (?, ?, ?)",
			t.Schema, t.Name, t.RowEstimate,
		)
		if err != nil {
			return err
		}
		id, _ := res.LastInsertId()
		tableIDByOID[t.OID] = id
	}

	columnIDByTableCol := make(map[string]int64)
	for tableOID, cols := range ext.Columns {
		tid, ok := tableIDByOID[tableOID]
		if !ok {
			continue
		}
		for _, c := range cols {
			res, err := tx.Exec(
				"INSERT INTO columns (table_id, name, type, nullable, position) VALUES (?, ?, ?, ?, ?)",
				tid, c.Name, c.DataType, c.Nullable, c.Attnum,
			)
			if err != nil {
				return err
			}
			cid, _ := res.LastInsertId()
			columnIDByTableCol[tableOID+":"+c.Name] = cid
		}
	}

	findOID := func(schemaName, name string) string {
		for _, t := range ext.Tables {
			if t.Schema == schemaName && t.Name == name {
				return t.OID
			}
		}
		return ""
	}

	for _, c := range ext.Constraints {
		tid, ok := tableIDByOID[c.TableOID]
		if !ok {
			continue
		}
		switch c.Kind {
		case "PK":
			for _, col := range strings.Split(c.SrcColumns, ",") {
				tx.Exec("INSERT OR IGNORE INTO primary_keys (table_id, column_name) VALUES (?, ?)", tid, strings.TrimSpace(col))
			}
		case "UQ":
			tx.Exec("INSERT INTO indexes_info (table_id, name, columns, is_unique) VALUES (?, ?, ?, 1)",
				tid, c.ConName, c.SrcColumns)
		case "FK":
			refOID := findOID(c.RefSchema, c.RefTable)
			refTid, ok := tableIDByOID[refOID]
			if !ok {
				continue
			}
			tx.Exec(
				"INSERT INTO foreign_keys (table_id, src_columns, ref_table_id, dst_columns, constraint_name) VALUES (?, ?, ?, ?, ?)",
				tid, c.SrcColumns, refTid, c.DstColumns, c.ConName,
			)
		}
	}

	if err := tx.Commit(); err != nil {
		return err
	}

	// --- Seed field_stats and field_values ---
	seedFieldData(store, columnIDByTableCol, ext)

	// --- Seed jsonb_paths ---
	seedJSONBPaths(store, columnIDByTableCol)

	return nil
}

// seedFieldData inserts field_stats and field_values rows for the fixture.
func seedFieldData(store *db.Store, colIDs map[string]int64, ext *schema.ExtractedSchema) {
	type fieldSeed struct {
		tableOID   string
		colName    string
		distinct   int
		nullCount  int
		isState    bool
		isCat      bool
		values     []string // representative values
	}

	seeds := []fieldSeed{
		// orgs
		{"100", "plan", 4, 0, true, true, []string{"free", "pro", "enterprise", "custom"}},
		{"100", "tier", 15, 0, false, true, []string{"starter", "growth", "scale", "enterprise"}},
		// pull_requests
		{"101", "number", 0, 0, false, false, nil},
		// reviews
		{"102", "status", 5, 0, true, true, []string{"created", "in_progress", "completed", "failed", "cancelled"}},
		{"102", "priority", 4, 0, false, true, []string{"1", "2", "3", "4"}},
	}

	for _, s := range seeds {
		cid, ok := colIDs[s.tableOID+":"+s.colName]
		if !ok {
			continue
		}
		store.DB().Exec(
			`INSERT OR REPLACE INTO field_stats (column_id, distinct_count, null_count, is_state_like, is_categorical)
			 VALUES (?, ?, ?, ?, ?)`,
			cid, s.distinct, s.nullCount, s.isState, s.isCat,
		)
		for _, v := range s.values {
			store.DB().Exec(
				`INSERT INTO field_values (column_id, value, frequency) VALUES (?, ?, ?)`,
				cid, v, 1,
			)
		}
	}
}

// seedJSONBPaths inserts JSONB path metadata for the reviews.metadata column.
func seedJSONBPaths(store *db.Store, colIDs map[string]int64) {
	cid, ok := colIDs["102:metadata"]
	if !ok {
		return
	}

	paths := []struct {
		path         string
		inferredType string
		sampleValues string
	}{
		{"$.provider", "string", "github(500), gitlab(200)"},
		{"$.provider.id", "string", "abc(500), def(200)"},
		{"$.score", "number", "85(300), 92(250), 78(100)"},
		{"$.tags", "array", ""},
		{"$.review_url", "string", "https://example.com/review/1(700)"},
	}

	for _, p := range paths {
		store.DB().Exec(
			`INSERT OR REPLACE INTO jsonb_paths (column_id, path, inferred_type, distinct_count, sample_values)
			 VALUES (?, ?, ?, ?, ?)`,
			cid, p.path, p.inferredType, 1, p.sampleValues,
		)
	}
}

// NewSeedStore creates an in-memory SQLite store seeded with the fixture
// schema, field data, and JSONB paths. Does NOT build the FTS index —
// call search.PopulateFTS separately if needed (avoids import cycle for
// search package tests).
func NewSeedStore(t *testing.T) *db.Store {
	t.Helper()

	store, err := db.OpenStore("")
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	t.Cleanup(func() { store.Close() })

	if err := store.InitSchema(); err != nil {
		t.Fatalf("init schema: %v", err)
	}

	ext := FixtureSchema()
	if err := SeedStore(store, ext); err != nil {
		t.Fatalf("seed store: %v", err)
	}

	return store
}

// NewTestStore creates an in-memory SQLite store seeded with the fixture
// schema, field data, JSONB paths, AND the FTS index. Use this for tests
// that need full-text search. For tests in the search package itself,
// use NewSeedStore + search.PopulateFTS to avoid import cycles.
//
// This function accepts a populateFTS function to avoid importing the
// search package at the testutil level (which would create an import cycle
// when search tests import testutil).
func NewTestStore(t *testing.T, populateFTS func(*db.Store) error) *db.Store {
	t.Helper()

	store := NewSeedStore(t)

	if err := store.InitFTS(); err != nil {
		t.Fatalf("init fts: %v", err)
	}

	if err := populateFTS(store); err != nil {
		t.Fatalf("populate fts: %v", err)
	}

	return store
}
