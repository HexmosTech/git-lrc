package dbctx

import (
	"testing"

	"github.com/shrsv/dbctx/internal/schema"
	"github.com/shrsv/dbctx/internal/testutil"
)

func TestComputeFingerprint_DeterministicAcrossCalls(t *testing.T) {
	ext := testutil.FixtureSchema()
	a := ComputeFingerprint(ext)
	b := ComputeFingerprint(ext)
	if a == "" {
		t.Fatal("fingerprint is empty")
	}
	if a != b {
		t.Fatalf("fingerprint not deterministic: %q != %q", a, b)
	}
}

func TestComputeFingerprint_UnaffectedByMapIterationOrder(t *testing.T) {
	// ext.Columns is a map - build the same logical schema with columns
	// inserted in a different order and confirm the fingerprint still
	// matches, since a Go map's range order is randomized per run.
	ext1 := testutil.FixtureSchema()
	ext2 := &schema.ExtractedSchema{
		Tables:      append([]schema.Table{}, ext1.Tables...),
		Columns:     map[string][]schema.Column{},
		Constraints: ext1.Constraints,
	}
	for oid, cols := range ext1.Columns {
		reversed := make([]schema.Column, len(cols))
		for i, c := range cols {
			reversed[len(cols)-1-i] = c
		}
		ext2.Columns[oid] = reversed
	}
	if ComputeFingerprint(ext1) != ComputeFingerprint(ext2) {
		t.Fatal("fingerprint changed when column order within a table changed")
	}
}

func TestComputeFingerprint_ChangesWhenColumnAdded(t *testing.T) {
	base := testutil.FixtureSchema()
	before := ComputeFingerprint(base)

	changed := testutil.FixtureSchema()
	changed.Columns["100"] = append(changed.Columns["100"], schema.Column{
		TableOID: "100", Name: "new_column", DataType: "text", Nullable: true, Attnum: 99,
	})
	after := ComputeFingerprint(changed)

	if before == after {
		t.Fatal("fingerprint did not change after adding a column")
	}
}

func TestComputeFingerprint_ChangesWhenColumnRetyped(t *testing.T) {
	base := testutil.FixtureSchema()
	before := ComputeFingerprint(base)

	changed := testutil.FixtureSchema()
	changed.Columns["100"][1].DataType = "varchar(255)" // was "text"
	after := ComputeFingerprint(changed)

	if before == after {
		t.Fatal("fingerprint did not change after retyping a column")
	}
}

func TestComputeFingerprint_UnaffectedByConstraintOrRowEstimateChurn(t *testing.T) {
	base := testutil.FixtureSchema()
	before := ComputeFingerprint(base)

	changed := testutil.FixtureSchema()
	changed.Tables[0].RowEstimate = 999999 // row count churn
	changed.Constraints = nil              // constraint churn
	after := ComputeFingerprint(changed)

	if before != after {
		t.Fatal("fingerprint changed from row-estimate/constraint churn alone - it should only track table/column shape")
	}
}

func TestSchemaFingerprint_RoundTrip(t *testing.T) {
	idx := newTestIndex(t)
	ext := testutil.FixtureSchema()
	want := ComputeFingerprint(ext)

	if err := StoreFingerprint(idx.store, want); err != nil {
		t.Fatalf("StoreFingerprint: %v", err)
	}

	got, err := idx.SchemaFingerprint()
	if err != nil {
		t.Fatalf("SchemaFingerprint: %v", err)
	}
	if got != want {
		t.Fatalf("SchemaFingerprint() = %q, want %q", got, want)
	}
}

func TestSchemaFingerprint_EmptyWhenNeverStored(t *testing.T) {
	idx := newTestIndex(t)
	got, err := idx.SchemaFingerprint()
	if err != nil {
		t.Fatalf("SchemaFingerprint: %v", err)
	}
	if got != "" {
		t.Fatalf("SchemaFingerprint() = %q, want empty string for an index with no stored fingerprint", got)
	}
}

func TestSchemaFingerprint_FileBacked(t *testing.T) {
	idx := newTestIndexFile(t)
	ext := testutil.FixtureSchema()
	want := ComputeFingerprint(ext)

	if err := StoreFingerprint(idx.store, want); err != nil {
		t.Fatalf("StoreFingerprint: %v", err)
	}

	got, err := idx.SchemaFingerprint()
	if err != nil {
		t.Fatalf("SchemaFingerprint: %v", err)
	}
	if got != want {
		t.Fatalf("SchemaFingerprint() = %q, want %q", got, want)
	}
}

// LiveFingerprint itself (the live-Postgres path) has no test here: this
// repo's test suite has no live-Postgres integration harness to connect to
// (confirmed - no other test file opens a real PG connection), matching
// the existing convention of testing everything through the SQLite-backed
// fixture path. ComputeFingerprint - the actual hashing logic LiveFingerprint
// delegates to after calling the already-tested schema.Extract - is covered
// above.
