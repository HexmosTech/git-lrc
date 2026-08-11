package dbctx

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/shrsv/dbctx/internal/db"
	"github.com/shrsv/dbctx/internal/schema"
	"github.com/shrsv/dbctx/internal/search"
	"github.com/shrsv/dbctx/internal/testutil"
)

// newTestIndex creates an Index backed by a seeded in-memory store.
func newTestIndex(t *testing.T) *Index {
	t.Helper()
	s := testutil.NewTestStore(t, search.PopulateFTS)
	idx := &Index{
		store: s,
		ready: make(chan struct{}),
	}
	close(idx.ready)
	return idx
}

// newTestIndexFile creates an Index backed by a temp .dtx file.
func newTestIndexFile(t *testing.T) *Index {
	t.Helper()

	dir := t.TempDir()
	path := filepath.Join(dir, "test.dtx")

	store, err := db.OpenStore(path)
	if err != nil {
		t.Fatalf("open file store: %v", err)
	}
	t.Cleanup(func() { store.Close() })

	if err := store.InitSchema(); err != nil {
		t.Fatalf("init schema: %v", err)
	}
	if err := store.InitFTS(); err != nil {
		t.Fatalf("init fts: %v", err)
	}
	ext := testutil.FixtureSchema()
	if err := testutil.SeedStore(store, ext); err != nil {
		t.Fatalf("seed store: %v", err)
	}
	if err := search.PopulateFTS(store); err != nil {
		t.Fatalf("populate fts: %v", err)
	}

	idx := &Index{
		store: store,
		path:  path,
		ready: make(chan struct{}),
	}
	close(idx.ready)
	return idx
}

func TestStoreSchema(t *testing.T) {
	store, err := db.OpenStore("")
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	defer store.Close()

	if err := store.InitSchema(); err != nil {
		t.Fatalf("init schema: %v", err)
	}

	ext := testutil.FixtureSchema()
	if err := storeSchema(store, ext); err != nil {
		t.Fatalf("storeSchema: %v", err)
	}

	// Verify tables were inserted
	var count int
	store.DB().QueryRow("SELECT COUNT(*) FROM tables").Scan(&count)
	if count != 4 {
		t.Errorf("tables count = %d, want 4", count)
	}

	// Verify columns
	store.DB().QueryRow("SELECT COUNT(*) FROM columns").Scan(&count)
	if count == 0 {
		t.Error("columns count = 0")
	}

	// Verify PKs
	store.DB().QueryRow("SELECT COUNT(*) FROM primary_keys").Scan(&count)
	if count != 4 {
		t.Errorf("primary_keys count = %d, want 4", count)
	}

	// Verify FKs
	store.DB().QueryRow("SELECT COUNT(*) FROM foreign_keys").Scan(&count)
	if count != 4 {
		t.Errorf("foreign_keys count = %d, want 4", count)
	}
}

func TestIndexQuery(t *testing.T) {
	idx := newTestIndex(t)
	defer idx.Close()

	result, err := idx.Query("reviews")
	if err != nil {
		t.Fatalf("Query: %v", err)
	}

	if len(result.Tables) == 0 {
		t.Fatal("Query returned 0 tables")
	}

	found := false
	for _, tbl := range result.Tables {
		if tbl.TableName == "reviews" && tbl.IsMatch {
			found = true
			break
		}
	}
	if !found {
		t.Error("reviews not found as match")
	}
}

func TestIndexTables(t *testing.T) {
	idx := newTestIndex(t)
	defer idx.Close()

	tables, err := idx.Tables()
	if err != nil {
		t.Fatalf("Tables: %v", err)
	}

	if len(tables) != 4 {
		t.Errorf("len(tables) = %d, want 4", len(tables))
	}

	names := make(map[string]bool)
	for _, tbl := range tables {
		names[tbl.Name] = true
	}
	for _, want := range []string{"reviews", "orgs", "pull_requests", "comments"} {
		if !names[want] {
			t.Errorf("missing table %q", want)
		}
	}
}

func TestIndexTableDetail(t *testing.T) {
	idx := newTestIndex(t)
	defer idx.Close()

	td, err := idx.TableDetail("reviews")
	if err != nil {
		t.Fatalf("TableDetail: %v", err)
	}
	if td == nil {
		t.Fatal("TableDetail returned nil")
	}

	if td.Name != "reviews" {
		t.Errorf("name = %q, want reviews", td.Name)
	}
	if len(td.PrimaryKey) == 0 {
		t.Error("primary key is empty")
	}
	if td.PrimaryKey[0] != "id" {
		t.Errorf("primary key[0] = %q, want id", td.PrimaryKey[0])
	}
	if len(td.ForeignKeys) == 0 {
		t.Error("foreign keys is empty")
	}
	if len(td.Columns) == 0 {
		t.Error("columns is empty")
	}

	// Check state field
	for _, col := range td.Columns {
		if col.Name == "status" {
			if !col.IsState {
				t.Error("status should be state-like")
			}
			if len(col.Values) == 0 {
				t.Error("status has no values")
			}
		}
		if col.Name == "metadata" {
			if len(col.JSONBPaths) == 0 {
				t.Error("metadata has no JSONB paths")
			}
		}
		if col.Name == "body" {
			if !col.Nullable {
				t.Error("body should be nullable")
			}
		}
		if col.Name == "org_id" {
			if col.FKTarget == "" {
				t.Error("org_id should have FKTarget")
			}
		}
	}
}

func TestIndexTableDetail_NotFound(t *testing.T) {
	idx := newTestIndex(t)
	defer idx.Close()

	_, err := idx.TableDetail("nonexistent")
	if err == nil {
		t.Error("expected error for nonexistent table")
	}
}

func TestIndexStats(t *testing.T) {
	idx := newTestIndex(t)
	defer idx.Close()

	stats, err := idx.Stats()
	if err != nil {
		t.Fatalf("Stats: %v", err)
	}

	if stats.Tables != 4 {
		t.Errorf("Tables = %d, want 4", stats.Tables)
	}
	if stats.Columns == 0 {
		t.Error("Columns = 0")
	}
	if stats.ForeignKeys != 4 {
		t.Errorf("ForeignKeys = %d, want 4", stats.ForeignKeys)
	}
	if stats.StateFields == 0 {
		t.Error("StateFields = 0")
	}
}

func TestIndexReport(t *testing.T) {
	idx := newTestIndex(t)
	defer idx.Close()

	var buf bytes.Buffer
	if err := idx.Report(&buf); err != nil {
		t.Fatalf("Report: %v", err)
	}

	output := buf.String()
	if len(output) == 0 {
		t.Error("Report produced empty output")
	}
	if !strings.Contains(output, "DATABASE CONTEXT REPORT") {
		t.Error("output missing report header")
	}
}

// --- Selection / Text tests ---

func TestSelectionText_IncludesLegend(t *testing.T) {
	idx := newTestIndex(t)
	defer idx.Close()

	result, err := idx.Query("reviews")
	if err != nil {
		t.Fatalf("Query: %v", err)
	}

	text := result.Matched().Text()

	if !strings.Contains(text, "--- notation ---") {
		t.Error("Text() missing legend")
	}
	if !strings.Contains(text, "PK:") {
		t.Error("Text() legend missing PK:")
	}
	if !strings.Contains(text, "→") {
		t.Error("Text() legend missing →")
	}
	if !strings.Contains(text, "[state]") {
		t.Error("Text() legend missing [state]")
	}
	if !strings.Contains(text, "[cat]") {
		t.Error("Text() legend missing [cat]")
	}
	if !strings.Contains(text, "$.path") {
		t.Error("Text() legend missing $.path")
	}
	if !strings.Contains(text, "reviews") {
		t.Error("Text() missing table 'reviews'")
	}
}

func TestSelectionTextRaw_NoLegend(t *testing.T) {
	idx := newTestIndex(t)
	defer idx.Close()

	result, err := idx.Query("reviews")
	if err != nil {
		t.Fatalf("Query: %v", err)
	}

	raw := result.Matched().TextRaw()

	if strings.Contains(raw, "--- notation ---") {
		t.Error("TextRaw() should NOT contain legend")
	}
	if !strings.Contains(raw, "reviews") {
		t.Error("TextRaw() missing table 'reviews'")
	}
}

func TestSelectionTextRaw_ContainsTableData(t *testing.T) {
	idx := newTestIndex(t)
	defer idx.Close()

	result, err := idx.Query("reviews")
	if err != nil {
		t.Fatalf("Query: %v", err)
	}

	raw := result.Matched().TextRaw()

	if !strings.Contains(raw, "PK: id") {
		t.Error("TextRaw() missing PK: id")
	}
	if !strings.Contains(raw, "org_id") {
		t.Error("TextRaw() missing org_id")
	}
	if !strings.Contains(raw, "[state]") {
		t.Error("TextRaw() missing [state]")
	}
	if !strings.Contains(raw, "$.provider") {
		t.Error("TextRaw() missing $.provider")
	}
}

func TestSelectionInclude(t *testing.T) {
	idx := newTestIndex(t)
	defer idx.Close()

	result, err := idx.Query("reviews")
	if err != nil {
		t.Fatalf("Query: %v", err)
	}

	text := result.Matched().Include("orgs").TextRaw()

	if !strings.Contains(text, "reviews") {
		t.Error("missing reviews")
	}
	if !strings.Contains(text, "orgs") {
		t.Error("missing orgs after Include")
	}
}

func TestSelectionExclude(t *testing.T) {
	idx := newTestIndex(t)
	defer idx.Close()

	result, err := idx.Query("reviews")
	if err != nil {
		t.Fatalf("Query: %v", err)
	}

	text := result.Matched().Exclude("reviews").TextRaw()

	if strings.Contains(text, "reviews") {
		t.Error("reviews should be excluded")
	}
}

func TestSelectionLen(t *testing.T) {
	idx := newTestIndex(t)
	defer idx.Close()

	result, err := idx.Query("reviews")
	if err != nil {
		t.Fatalf("Query: %v", err)
	}

	matched := result.Matched()
	if matched.Len() == 0 {
		t.Error("Matched().Len() = 0")
	}

	all := result.All()
	if all.Len() < matched.Len() {
		t.Error("All().Len() should be >= Matched().Len()")
	}
}

func TestSelectionTables(t *testing.T) {
	idx := newTestIndex(t)
	defer idx.Close()

	result, err := idx.Query("reviews")
	if err != nil {
		t.Fatalf("Query: %v", err)
	}

	tables := result.Matched().Tables()
	if len(tables) == 0 {
		t.Error("Tables() returned empty slice")
	}

	for _, tbl := range tables {
		if tbl.TableName == "" {
			t.Error("table has empty name")
		}
	}
}

func TestResultSetTableMap(t *testing.T) {
	idx := newTestIndex(t)
	defer idx.Close()

	result, err := idx.Query("reviews")
	if err != nil {
		t.Fatalf("Query: %v", err)
	}

	tableMap := result.TableMap()
	if len(tableMap) == 0 {
		t.Error("TableMap() returned empty map")
	}

	if _, ok := tableMap["reviews"]; !ok {
		t.Error("TableMap() missing 'reviews'")
	}
}

func TestOpen(t *testing.T) {
	idx := newTestIndexFile(t)
	defer idx.Close()

	// Verify the file exists
	if _, err := os.Stat(idx.path); os.IsNotExist(err) {
		t.Errorf("file %s does not exist", idx.path)
	}

	// Open it in a new Index
	idx2, err := Open(idx.path)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer idx2.Close()

	// Open doesn't close the ready channel (Build does that),
	// so we need to close it manually for the test.
	close(idx2.ready)

	// Should be able to query
	tables, err := idx2.Tables()
	if err != nil {
		t.Fatalf("Tables: %v", err)
	}
	if len(tables) != 4 {
		t.Errorf("len(tables) = %d, want 4", len(tables))
	}
}

func TestSplitCSV(t *testing.T) {
	tests := []struct {
		input string
		want  int
	}{
		{"a,b,c", 3},
		{"a, b, c", 3},
		{"single", 1},
		{"", 1}, // strings.Split returns [""]
	}

	for _, tt := range tests {
		got := splitCSV(tt.input)
		if len(got) != tt.want {
			t.Errorf("splitCSV(%q) = %v (len %d), want len %d", tt.input, got, len(got), tt.want)
		}
	}
}

func TestFindOIDByTable(t *testing.T) {
	ext := &schema.ExtractedSchema{
		Tables: []schema.Table{
			{Schema: "public", Name: "users", OID: "42"},
			{Schema: "public", Name: "posts", OID: "43"},
		},
	}

	if got := findOIDByTable(ext, "public", "users"); got != "42" {
		t.Errorf("findOIDByTable(public, users) = %q, want 42", got)
	}
	if got := findOIDByTable(ext, "public", "nonexistent"); got != "" {
		t.Errorf("findOIDByTable(public, nonexistent) = %q, want empty", got)
	}
}

func TestLegend(t *testing.T) {
	l := legend()
	if !strings.Contains(l, "PK:") {
		t.Error("legend missing PK:")
	}
	if !strings.Contains(l, "→") {
		t.Error("legend missing →")
	}
	if !strings.Contains(l, "[state]") {
		t.Error("legend missing [state]")
	}
	if !strings.Contains(l, "[cat]") {
		t.Error("legend missing [cat]")
	}
	if !strings.Contains(l, "$.path") {
		t.Error("legend missing $.path")
	}
	if !strings.Contains(l, "(score:") {
		t.Error("legend missing (score:")
	}
}
