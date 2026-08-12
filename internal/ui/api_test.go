package ui

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/shrsv/dbctx/internal/db"
	"github.com/shrsv/dbctx/internal/search"
	"github.com/shrsv/dbctx/internal/testutil"
)

func newTestAPI(t *testing.T) (*API, *db.Store) {
	store := testutil.NewTestStore(t, search.PopulateFTS)
	return NewAPI(store), store
}

func TestHandleStats(t *testing.T) {
	api, _ := newTestAPI(t)
	mux := http.NewServeMux()
	api.Register(mux)

	req := httptest.NewRequest("GET", "/api/stats", nil)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	if w.Code != 200 {
		t.Fatalf("status = %d, want 200", w.Code)
	}

	var stats map[string]int
	if err := json.NewDecoder(w.Body).Decode(&stats); err != nil {
		t.Fatalf("decode: %v", err)
	}

	if stats["tables"] != 4 {
		t.Errorf("tables = %d, want 4", stats["tables"])
	}
	if stats["columns"] == 0 {
		t.Error("columns = 0, want > 0")
	}
	if stats["foreign_keys"] == 0 {
		t.Error("foreign_keys = 0, want > 0")
	}
	if stats["state_fields"] == 0 {
		t.Error("state_fields = 0, want > 0")
	}
}

func TestHandleTables(t *testing.T) {
	api, _ := newTestAPI(t)
	mux := http.NewServeMux()
	api.Register(mux)

	req := httptest.NewRequest("GET", "/api/tables", nil)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	if w.Code != 200 {
		t.Fatalf("status = %d, want 200", w.Code)
	}

	var tables []tableInfo
	if err := json.NewDecoder(w.Body).Decode(&tables); err != nil {
		t.Fatalf("decode: %v", err)
	}

	if len(tables) != 4 {
		t.Errorf("len(tables) = %d, want 4", len(tables))
	}

	names := make(map[string]bool)
	for _, t := range tables {
		names[t.Name] = true
	}
	for _, want := range []string{"reviews", "orgs", "pull_requests", "comments"} {
		if !names[want] {
			t.Errorf("missing table %q", want)
		}
	}
}

func TestHandleTableDetail(t *testing.T) {
	api, _ := newTestAPI(t)
	mux := http.NewServeMux()
	api.Register(mux)

	req := httptest.NewRequest("GET", "/api/tables/reviews", nil)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	if w.Code != 200 {
		t.Fatalf("status = %d, want 200", w.Code)
	}

	var td tableDetail
	if err := json.NewDecoder(w.Body).Decode(&td); err != nil {
		t.Fatalf("decode: %v", err)
	}

	if td.Name != "reviews" {
		t.Errorf("name = %q, want reviews", td.Name)
	}
	if len(td.PrimaryKey) == 0 {
		t.Error("primary_key is empty")
	}
	if len(td.ForeignKeys) == 0 {
		t.Error("foreign_keys is empty")
	}
	if len(td.Columns) == 0 {
		t.Error("columns is empty")
	}

	// Check state field
	for _, col := range td.Columns {
		if col.Name == "status" {
			if !col.IsState {
				t.Error("status should be state")
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
	}
}

func TestHandleTableDetail_NotFound(t *testing.T) {
	api, _ := newTestAPI(t)
	mux := http.NewServeMux()
	api.Register(mux)

	req := httptest.NewRequest("GET", "/api/tables/nonexistent", nil)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	if w.Code != 404 {
		t.Errorf("status = %d, want 404", w.Code)
	}
}

func TestHandleQuery(t *testing.T) {
	api, _ := newTestAPI(t)
	mux := http.NewServeMux()
	api.Register(mux)

	req := httptest.NewRequest("GET", "/api/query?q=reviews", nil)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	if w.Code != 200 {
		t.Fatalf("status = %d, want 200", w.Code)
	}

	var result queryResult
	if err := json.NewDecoder(w.Body).Decode(&result); err != nil {
		t.Fatalf("decode: %v", err)
	}

	if result.Query != "reviews" {
		t.Errorf("query = %q, want reviews", result.Query)
	}
	if len(result.Tables) == 0 {
		t.Error("tables is empty")
	}
}

// TestHandleQuery_UsesTerminology is a regression test: handleQuery used
// to call search.Query (lexical-only) directly, silently ignoring any
// imported terminology (and semantic search) even though the CLI and
// library API both used the hybrid path. A table findable only via a
// terminology alias — no lexical or literal-name overlap with the query
// at all — must show up through `dbctx ui`'s /api/query the same way it
// does through `dbctx query`.
func TestHandleQuery_UsesTerminology(t *testing.T) {
	api, store := newTestAPI(t)
	if err := store.InitTerminologySchema(); err != nil {
		t.Fatalf("InitTerminologySchema: %v", err)
	}
	// "widget count" shares no lexical overlap with "orgs" at all.
	if _, err := store.DB().Exec(
		`INSERT INTO terminology (term, alias, target_table, source) VALUES ('org', 'widget count', 'orgs', 'user')`,
	); err != nil {
		t.Fatalf("insert terminology: %v", err)
	}

	mux := http.NewServeMux()
	api.Register(mux)

	req := httptest.NewRequest("GET", "/api/query?q=widget+count", nil)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	if w.Code != 200 {
		t.Fatalf("status = %d, want 200", w.Code)
	}
	var result queryResult
	if err := json.NewDecoder(w.Body).Decode(&result); err != nil {
		t.Fatalf("decode: %v", err)
	}

	found := false
	for _, tbl := range result.Tables {
		if tbl.TableName == "orgs" && tbl.IsMatch {
			found = true
		}
	}
	if !found {
		t.Errorf("expected terminology-only match 'orgs' to appear via /api/query, got %+v", result.Tables)
	}
}

func TestHandleQuery_MissingParam(t *testing.T) {
	api, _ := newTestAPI(t)
	mux := http.NewServeMux()
	api.Register(mux)

	req := httptest.NewRequest("GET", "/api/query", nil)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	if w.Code != 400 {
		t.Errorf("status = %d, want 400", w.Code)
	}
}

func TestHandleTableDetail_ColumnStructure(t *testing.T) {
	api, _ := newTestAPI(t)
	mux := http.NewServeMux()
	api.Register(mux)

	req := httptest.NewRequest("GET", "/api/tables/reviews", nil)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	var td tableDetail
	json.NewDecoder(w.Body).Decode(&td)

	// Check FK target is set on org_id
	for _, col := range td.Columns {
		if col.Name == "org_id" {
			if col.FKTarget == "" {
				t.Error("org_id should have FKTarget")
			}
			if !strings.Contains(col.FKTarget, "orgs") {
				t.Errorf("org_id FKTarget = %q, should reference orgs", col.FKTarget)
			}
		}
		if col.Name == "body" {
			if !col.Nullable {
				t.Error("body should be nullable")
			}
		}
	}
}
