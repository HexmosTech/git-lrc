package search

import (
	"testing"

	"github.com/shrsv/dbctx/internal/db"
	"github.com/shrsv/dbctx/internal/testutil"
)

func seedTestStore(t *testing.T) *db.Store {
	t.Helper()
	store := testutil.NewSeedStore(t)
	if err := store.InitFTS(); err != nil {
		t.Fatalf("init fts: %v", err)
	}
	if err := PopulateFTS(store); err != nil {
		t.Fatalf("populate fts: %v", err)
	}
	return store
}

func TestPopulateFTS(t *testing.T) {
	store := seedTestStore(t)

	// Verify FTS rows were created
	var count int
	err := store.DB().QueryRow("SELECT COUNT(*) FROM search_index").Scan(&count)
	if err != nil {
		t.Fatalf("count search_index: %v", err)
	}
	if count == 0 {
		t.Error("search_index has 0 rows after PopulateFTS")
	}
	// We have 4 tables in the fixture
	if count != 4 {
		t.Errorf("search_index has %d rows, want 4", count)
	}
}

func TestQuery_ExactTableName(t *testing.T) {
	store := seedTestStore(t)

	result, err := Query(store, "reviews")
	if err != nil {
		t.Fatalf("Query: %v", err)
	}
	if len(result.Tables) == 0 {
		t.Fatal("Query('reviews') returned 0 tables")
	}

	found := false
	for _, tbl := range result.Tables {
		if tbl.TableName == "reviews" && tbl.IsMatch {
			found = true
			break
		}
	}
	if !found {
		t.Error("reviews not found as a match in results")
	}
}

func TestQuery_FuzzyMatch(t *testing.T) {
	store := seedTestStore(t)

	// "revew" is a typo for "reviews"
	result, err := Query(store, "revew")
	if err != nil {
		t.Fatalf("Query: %v", err)
	}

	found := false
	for _, tbl := range result.Tables {
		if tbl.TableName == "reviews" {
			found = true
			break
		}
	}
	if !found {
		t.Error("fuzzy match did not find 'reviews' for query 'revew'")
	}
}

func TestQuery_ValueMatch(t *testing.T) {
	store := seedTestStore(t)

	// "completed" is a field_value for reviews.status
	result, err := Query(store, "completed")
	if err != nil {
		t.Fatalf("Query: %v", err)
	}

	found := false
	for _, tbl := range result.Tables {
		if tbl.TableName == "reviews" {
			found = true
			break
		}
	}
	if !found {
		t.Error("value match did not find 'reviews' for query 'completed'")
	}
}

func TestQuery_FKExpansion(t *testing.T) {
	store := seedTestStore(t)

	// Querying "orgs" should also surface reviews and pull_requests via FK
	result, err := Query(store, "orgs")
	if err != nil {
		t.Fatalf("Query: %v", err)
	}

	tableNames := make(map[string]bool)
	for _, tbl := range result.Tables {
		tableNames[tbl.TableName] = true
	}

	if !tableNames["orgs"] {
		t.Error("orgs not in results")
	}
	// FK expansion should bring in tables that reference orgs
	if !tableNames["reviews"] {
		t.Error("reviews not in results (FK expansion from orgs)")
	}
	if !tableNames["pull_requests"] {
		t.Error("pull_requests not in results (FK expansion from orgs)")
	}
}

func TestQuery_NoMatch(t *testing.T) {
	store := seedTestStore(t)

	result, err := Query(store, "xyznonexistent")
	if err != nil {
		t.Fatalf("Query: %v", err)
	}

	// Should have 0 or only FK-expanded tables
	for _, tbl := range result.Tables {
		if tbl.IsMatch {
			t.Errorf("unexpected match: %s (score=%.2f)", tbl.TableName, tbl.MatchScore)
		}
	}
}

func TestQuery_MultipleTerms(t *testing.T) {
	store := seedTestStore(t)

	result, err := Query(store, "failed reviews")
	if err != nil {
		t.Fatalf("Query: %v", err)
	}
	if len(result.Tables) == 0 {
		t.Fatal("Query('failed reviews') returned 0 tables")
	}

	// "reviews" should match on both "reviews" (name) and "failed" (value)
	found := false
	for _, tbl := range result.Tables {
		if tbl.TableName == "reviews" && tbl.IsMatch {
			found = true
			break
		}
	}
	if !found {
		t.Error("reviews not found as match for 'failed reviews'")
	}
}

func TestQuery_TableContextStructure(t *testing.T) {
	store := seedTestStore(t)

	result, err := Query(store, "reviews")
	if err != nil {
		t.Fatalf("Query: %v", err)
	}

	var reviews TableContext
	for _, tbl := range result.Tables {
		if tbl.TableName == "reviews" {
			reviews = tbl
			break
		}
	}

	if reviews.PrimaryKey == nil {
		t.Error("reviews.PrimaryKey is nil")
	} else if len(reviews.PrimaryKey) != 1 || reviews.PrimaryKey[0] != "id" {
		t.Errorf("reviews.PrimaryKey = %v, want [id]", reviews.PrimaryKey)
	}

	if len(reviews.ForeignKeys) == 0 {
		t.Error("reviews.ForeignKeys is empty")
	}

	// Check that state field has values
	for _, col := range reviews.Columns {
		if col.Name == "status" {
			if !col.IsState {
				t.Error("status column should be marked as state")
			}
			if len(col.Values) == 0 {
				t.Error("status column has no values")
			}
		}
		if col.Name == "metadata" {
			if len(col.JSONBPaths) == 0 {
				t.Error("metadata column has no JSONB paths")
			}
		}
		if col.Name == "body" {
			if !col.Nullable {
				t.Error("body column should be nullable")
			}
		}
	}
}

func TestQuery_ScoreOrdering(t *testing.T) {
	store := seedTestStore(t)

	result, err := Query(store, "reviews")
	if err != nil {
		t.Fatalf("Query: %v", err)
	}

	// First matched result should have a positive score
	for _, tbl := range result.Tables {
		if tbl.IsMatch {
			if tbl.MatchScore <= 0 {
				t.Errorf("first match %s has score %.2f, want > 0", tbl.TableName, tbl.MatchScore)
			}
			break
		}
	}
}
