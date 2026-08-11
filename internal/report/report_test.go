package report

import (
	"bytes"
	"strings"
	"testing"

	"github.com/shrsv/dbctx/internal/db"
	"github.com/shrsv/dbctx/internal/search"
	"github.com/shrsv/dbctx/internal/testutil"
)

func newTestStore(t *testing.T) *db.Store {
	return testutil.NewTestStore(t, search.PopulateFTS)
}

func TestReportAll(t *testing.T) {
	store := newTestStore(t)

	var buf bytes.Buffer
	err := ReportAll(&buf, store)
	if err != nil {
		t.Fatalf("ReportAll: %v", err)
	}

	output := buf.String()

	if len(output) == 0 {
		t.Fatal("ReportAll produced empty output")
	}

	sections := []string{
		"DATABASE CONTEXT REPORT",
		"SCHEMA",
		"STATE-LIKE FIELDS",
		"CATEGORICAL FIELDS",
		"JSONB STRUCTURE",
		"RELATIONSHIPS",
		"STATS",
	}

	for _, section := range sections {
		if !strings.Contains(output, section) {
			t.Errorf("output missing section %q", section)
		}
	}
}

func TestReportAll_ContainsTableNames(t *testing.T) {
	store := newTestStore(t)

	var buf bytes.Buffer
	ReportAll(&buf, store)
	output := buf.String()

	expectedTables := []string{"reviews", "orgs", "pull_requests", "comments"}
	for _, name := range expectedTables {
		if !strings.Contains(output, name) {
			t.Errorf("output missing table %q", name)
		}
	}
}

func TestReportAll_ContainsStateValues(t *testing.T) {
	store := newTestStore(t)

	var buf bytes.Buffer
	ReportAll(&buf, store)
	output := buf.String()

	expectedValues := []string{"created", "in_progress", "completed", "failed"}
	for _, val := range expectedValues {
		if !strings.Contains(output, val) {
			t.Errorf("output missing state value %q", val)
		}
	}
}

func TestReportAll_ContainsJSONBPaths(t *testing.T) {
	store := newTestStore(t)

	var buf bytes.Buffer
	ReportAll(&buf, store)
	output := buf.String()

	if !strings.Contains(output, "$.provider") {
		t.Error("output missing JSONB path $.provider")
	}
	if !strings.Contains(output, "$.score") {
		t.Error("output missing JSONB path $.score")
	}
}

func TestReportAll_ContainsRelationships(t *testing.T) {
	store := newTestStore(t)

	var buf bytes.Buffer
	ReportAll(&buf, store)
	output := buf.String()

	if !strings.Contains(output, "org_id") {
		t.Error("output missing FK column org_id")
	}
	if !strings.Contains(output, "review_id") {
		t.Error("output missing FK column review_id")
	}
}

func TestReportAll_ContainsStats(t *testing.T) {
	store := newTestStore(t)

	var buf bytes.Buffer
	ReportAll(&buf, store)
	output := buf.String()

	if !strings.Contains(output, "Tables:") {
		t.Error("output missing Tables: stat")
	}
	if !strings.Contains(output, "Columns:") {
		t.Error("output missing Columns: stat")
	}
	if !strings.Contains(output, "Foreign keys:") {
		t.Error("output missing Foreign keys: stat")
	}
}

func TestFormatQueryResult(t *testing.T) {
	store := newTestStore(t)

	result, err := search.Query(store, "reviews")
	if err != nil {
		t.Fatalf("search.Query: %v", err)
	}

	var buf bytes.Buffer
	FormatQueryResult(&buf, result)
	output := buf.String()

	if len(output) == 0 {
		t.Fatal("FormatQueryResult produced empty output")
	}
	if !strings.Contains(output, "reviews") {
		t.Error("output missing 'reviews'")
	}
	if !strings.Contains(output, "Query:") {
		t.Error("output missing 'Query:' header")
	}
	if !strings.Contains(output, "TABLES (ranked)") {
		t.Error("output missing 'TABLES (ranked)' header")
	}
}

func TestFormatQueryResult_ContainsStructure(t *testing.T) {
	store := newTestStore(t)

	result, err := search.Query(store, "reviews")
	if err != nil {
		t.Fatalf("search.Query: %v", err)
	}

	var buf bytes.Buffer
	FormatQueryResult(&buf, result)
	output := buf.String()

	if !strings.Contains(output, "PK:") {
		t.Error("output missing PK info")
	}
	if !strings.Contains(output, "COLUMNS:") {
		t.Error("output missing COLUMNS header")
	}
}
