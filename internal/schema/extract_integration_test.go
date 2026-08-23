//go:build integration

package schema

import (
	"context"
	"os"
	"testing"

	"github.com/shrsv/dbctx/internal/db"
)

// Guards against tables with a PK/UQ/FK constraint getting RowEstimate=0. Run with: make test-integration
func TestIntegration_Extract_RowEstimateOnConstrainedTable(t *testing.T) {
	dsn := os.Getenv("DATABASE_URL")
	if dsn == "" {
		t.Skip("DATABASE_URL not set, skipping integration test")
	}

	ctx := context.Background()
	pg, err := db.Connect(ctx, dsn)
	if err != nil {
		t.Fatalf("connect: %v", err)
	}
	defer pg.Close(ctx)

	const tbl = "dbctx_test_row_estimate"
	pg.Conn.Exec(ctx, "DROP TABLE IF EXISTS "+tbl)
	defer pg.Conn.Exec(ctx, "DROP TABLE IF EXISTS "+tbl)

	// A primary key is enough to trigger the bug: it produces a 'PK' row
	// in the constraint block, which used to sort ('P' < 'T') before the
	// 'TABLE' row that carries the real row_estimate.
	if _, err := pg.Conn.Exec(ctx, "CREATE TABLE "+tbl+" (id serial primary key, payload jsonb)"); err != nil {
		t.Fatalf("create table: %v", err)
	}
	for i := 0; i < 200; i++ {
		if _, err := pg.Conn.Exec(ctx, "INSERT INTO "+tbl+" (payload) VALUES ('{}')"); err != nil {
			t.Fatalf("insert: %v", err)
		}
	}
	if _, err := pg.Conn.Exec(ctx, "ANALYZE "+tbl); err != nil {
		t.Fatalf("analyze: %v", err)
	}

	ext, err := Extract(ctx, pg, "public")
	if err != nil {
		t.Fatalf("Extract: %v", err)
	}

	var found *Table
	for i := range ext.Tables {
		if ext.Tables[i].Name == tbl {
			found = &ext.Tables[i]
			break
		}
	}
	if found == nil {
		t.Fatalf("table %q not found in extracted schema", tbl)
	}
	if found.RowEstimate <= 0 {
		t.Errorf("RowEstimate = %v, want > 0 (table has a PK constraint and 200 analyzed rows)", found.RowEstimate)
	}
}
