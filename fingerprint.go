package dbctx

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"sort"
	"strconv"
	"strings"

	"github.com/shrsv/dbctx/internal/db"
	"github.com/shrsv/dbctx/internal/schema"
)

// schemaFingerprintKey is the metadata table key a build's fingerprint is
// stored under (see StoreFingerprint / SchemaFingerprint).
const schemaFingerprintKey = "schema_fingerprint"

// ComputeFingerprint hashes the table/column *shape* of an extracted
// schema - (schema, table, column, data type, nullable) tuples, sorted for
// determinism - into a short hex digest. Deliberately excludes constraints,
// indexes, and row estimates: those can churn (a new index, a changing row
// count) without making dbctx's own retrieval or schema text wrong, so
// counting them as drift would force unnecessary rebuilds. Only a change
// that would actually make an already-built .dtx describe the database
// incorrectly - an added/dropped/retyped column or table - changes this
// value.
func ComputeFingerprint(ext *schema.ExtractedSchema) string {
	type tuple struct {
		schemaName, table, column, dataType string
		nullable                            bool
	}
	tableByOID := make(map[string]schema.Table, len(ext.Tables))
	for _, t := range ext.Tables {
		tableByOID[t.OID] = t
	}

	tuples := make([]tuple, 0, len(ext.Columns))
	for oid, cols := range ext.Columns {
		t, ok := tableByOID[oid]
		if !ok {
			continue
		}
		for _, c := range cols {
			tuples = append(tuples, tuple{t.Schema, t.Name, c.Name, c.DataType, c.Nullable})
		}
	}
	// Tables with no columns (unlikely, but not impossible for a view-like
	// relation) would otherwise vanish from the fingerprint entirely -
	// still worth counting as part of the shape.
	for _, t := range ext.Tables {
		if len(ext.Columns[t.OID]) == 0 {
			tuples = append(tuples, tuple{t.Schema, t.Name, "", "", false})
		}
	}

	sort.Slice(tuples, func(i, j int) bool {
		a, b := tuples[i], tuples[j]
		if a.schemaName != b.schemaName {
			return a.schemaName < b.schemaName
		}
		if a.table != b.table {
			return a.table < b.table
		}
		return a.column < b.column
	})

	var sb strings.Builder
	for _, t := range tuples {
		sb.WriteString(t.schemaName)
		sb.WriteByte('\x00')
		sb.WriteString(t.table)
		sb.WriteByte('\x00')
		sb.WriteString(t.column)
		sb.WriteByte('\x00')
		sb.WriteString(t.dataType)
		sb.WriteByte('\x00')
		sb.WriteString(strconv.FormatBool(t.nullable))
		sb.WriteByte('\n')
	}

	sum := sha256.Sum256([]byte(sb.String()))
	return hex.EncodeToString(sum[:])
}

// StoreFingerprint persists a fingerprint into the store's metadata table.
// Called by Build/BuildAsync right after schema.Extract, reusing that same
// result rather than re-querying.
func StoreFingerprint(store *db.Store, fingerprint string) error {
	_, err := store.DB().Exec(
		"INSERT OR REPLACE INTO metadata (key, value) VALUES (?, ?)",
		schemaFingerprintKey, fingerprint,
	)
	if err != nil {
		return fmt.Errorf("store schema fingerprint: %w", err)
	}
	return nil
}

// SchemaFingerprint returns the schema fingerprint stored in this index at
// build time - see [LiveFingerprint] for the counterpart that recomputes
// one fresh against a live database, and compare the two to detect
// whether a .dtx has gone stale relative to the schema it describes.
// Returns "" (no error) if the index predates this feature and was never
// given a fingerprint.
//
// Blocks until the index is ready if an async build is in progress.
func (idx *Index) SchemaFingerprint() (string, error) {
	<-idx.ready
	if idx.err != nil {
		return "", idx.err
	}
	idx.mu.RLock()
	defer idx.mu.RUnlock()

	var fp string
	err := idx.store.DB().QueryRow(
		"SELECT value FROM metadata WHERE key = ?", schemaFingerprintKey,
	).Scan(&fp)
	if err != nil {
		// No row yet (older .dtx, or a build that failed before storing
		// one) reads as an empty fingerprint, not an error - it's the
		// caller's job to decide what "no fingerprint on record" means for
		// a staleness check (e.g. LiveReview treats it as unverifiable).
		return "", nil
	}
	return fp, nil
}

// LiveFingerprint computes a schema fingerprint fresh against a live
// PostgreSQL database, without opening or writing a .dtx file - the
// read-only counterpart to the fingerprint [Build]/[BuildAsync] store
// automatically. Compare its result against an already-built index's
// [Index.SchemaFingerprint] to detect schema drift cheaply: this only runs
// the same lightweight schema-extraction query Build's own first phase
// does, not a full rebuild (no field statistics, no JSONB sampling, no
// semantic embedding).
//
// opts may be nil; only Schemas and MaxConns are consulted (Path, Logger,
// and NoSemantic don't apply here - nothing is written or embedded).
func LiveFingerprint(ctx context.Context, dsn string, opts *Options) (string, error) {
	if opts == nil {
		opts = &Options{}
	}
	maxConns := opts.MaxConns
	if maxConns == 0 {
		maxConns = 4
	}
	pg, err := db.ConnectWithMaxConns(ctx, dsn, int32(maxConns))
	if err != nil {
		return "", fmt.Errorf("pg connect: %w", err)
	}
	defer pg.Close(ctx)

	schemas := opts.Schemas
	if schemas == "" {
		schemas = "public"
	}

	ext, err := schema.Extract(ctx, pg, schemas)
	if err != nil {
		return "", fmt.Errorf("schema extraction: %w", err)
	}
	return ComputeFingerprint(ext), nil
}
