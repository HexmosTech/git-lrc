// Package dbctx compiles a PostgreSQL database into a compact, queryable
// context index for text-to-SQL systems, AI agents, and database-aware
// applications.
//
// dbctx connects to PostgreSQL, extracts schema metadata, analyzes field
// statistics from pg_stats, discovers JSONB structure via sampling, and
// builds a full-text search index — all deterministic, no generative LLM
// or external service required. The result is a [Index] that answers
// natural-language queries about which tables, columns, values, and
// relationships are relevant to a given question.
//
// Two optional layers can augment that retrieval, both off by default in
// terms of resource cost but the first on by default in terms of
// behavior:
//
//   - Semantic retrieval: a small local embedding model (on by default —
//     see [Options.NoSemantic]) recovers paraphrases lexical matching
//     structurally can't (e.g. "buyers" finding a customers table),
//     fused with, never replacing, the deterministic signals. See
//     [Index.Query] and the package README's "Semantic retrieval" section.
//   - Terminology: a user-controlled dictionary mapping domain
//     abbreviations/jargon to exact schema objects, populated only via
//     explicit review — see [Index.TerminologyPrompt] and
//     [Index.ImportTerminology].
//
// The index can be stored on disk as a portable .dtx file (SQLite) or
// kept entirely in memory for ephemeral use. It is safe for concurrent
// access from multiple goroutines.
//
// # Quick start
//
// Build an in-memory index and query it:
//
//	idx, err := dbctx.Build(ctx, "postgres://localhost/mydb", nil)
//	if err != nil {
//	    log.Fatal(err)
//	}
//	defer idx.Close()
//
//	result, err := idx.Query("failed reviews last month")
//	if err != nil {
//	    log.Fatal(err)
//	}
//	fmt.Println(result.Matched().Text())
//
// [ResultSet.Matched] returns every table relevant to answering the
// query — direct hits plus FK-expanded join context, not just tables that
// scored on their own. The [Selection.Text] output is a compact,
// notation-annotated schema
// ready to pass to an LLM or text-to-SQL system:
//
//	--- notation ---
//	PK: primary key           col → table  foreign key
//	...
//
//	reviews  (score: 15.24)
//	  PK: id
//	  org_id → orgs
//	  status character varying(50) [state]
//	    {completed, failed, created, in_progress}
//	  metadata jsonb
//	    $.provider  string  {github, gitlab}
//
// # Persisting the index
//
// Save the index to a .dtx file for later reuse — no PostgreSQL needed:
//
//	idx, err := dbctx.Build(ctx, dsn, &dbctx.Options{Path: "mydb.dtx"})
//	// ...later...
//	idx, err = dbctx.Open("mydb.dtx")
//
// # Non-blocking startup
//
// For applications that need the index available without blocking startup,
// use [BuildAsync]. Queries made before the build completes will block
// automatically:
//
//	idx, ready, err := dbctx.BuildAsync(ctx, dsn, nil)
//	if err != nil {
//	    log.Fatal(err)
//	}
//	defer idx.Close()
//	// Register idx with your application immediately...
//	<-ready // or: <-idx.Ready()
//
// # Selection API
//
// Query results can be filtered and rendered in several ways:
//
//	result, _ := idx.Query("failed reviews")
//	result.Matched().Text()                          // matched + FK-expanded, with legend
//	result.Matched().TextRaw()                       // same, no legend
//	result.ScoredOnly().Text()                       // only tables that scored directly
//	result.Include("reviews", "orgs").Text()         // specific tables
//	result.Matched().Exclude("migrations").Text()    // matched minus exclusions
//
// # Semantic retrieval
//
// By default, [Build] also builds a local embedding-based semantic index
// (downloading the model to a local cache on first use) and [Index.Query]
// fuses it with lexical/fuzzy matching automatically. Set
// [Options.NoSemantic] to skip it:
//
//	idx, err := dbctx.Build(ctx, dsn, &dbctx.Options{NoSemantic: true})
//
// [Index.Query]'s result exposes why a semantic-only match appeared via
// [ResultSet.SemanticHits] — the evidence is never a black box.
//
// # Terminology
//
// Terminology maps domain vocabulary (abbreviations, acronyms, jargon) to
// exact schema objects, as a third retrieval signal fully independent of
// both lexical and semantic matching. dbctx never populates it
// automatically — [Index.TerminologyPrompt] generates a self-contained
// prompt for you to run through an LLM of your choice, and
// [Index.ImportTerminology] (or [Index.ImportTerminologyFile],
// [Index.ImportTerminologyGroups]) validates and loads the reviewed
// result back in:
//
//	prompt, _ := idx.TerminologyPrompt()
//	// ...paste prompt into an LLM, review its output...
//	result, _ := idx.ImportTerminology(llmOutputJSON)
//
// # Use cases
//
// dbctx is designed for any system that needs to understand a PostgreSQL
// database at query time: text-to-SQL generation, natural-language analytics,
// AI agents, database explorers, BI tools, and developer assistants.
// It replaces repeated full-schema dumps with a deterministic, queryable index.
package dbctx

import (
	"context"
	"fmt"
	"io"
	"os"
	"strings"
	"sync"

	"github.com/shrsv/dbctx/internal/analyze"
	"github.com/shrsv/dbctx/internal/db"
	"github.com/shrsv/dbctx/internal/embed"
	"github.com/shrsv/dbctx/internal/report"
	"github.com/shrsv/dbctx/internal/schema"
	"github.com/shrsv/dbctx/internal/search"
	"github.com/shrsv/dbctx/internal/semantic"
	"github.com/shrsv/dbctx/internal/terminology"
)

// Index is a compiled database context index. It provides methods to query
// the database structure, relationships, field semantics, and representative
// values extracted from PostgreSQL.
//
// An Index is safe for concurrent use by multiple goroutines.
// Create one with [Build], [BuildAsync], or [Open].
type Index struct {
	store *db.Store
	path  string
	mu    sync.RWMutex
	ready chan struct{}
	err   error

	// semanticOnce lazily constructs semanticScorer on first Query call
	// (rather than at Build/Open time) so that opening or building a
	// lexical-only index never pays any embedding-model cost, and a
	// semantic-enabled index only loads its model when actually queried.
	semanticOnce   sync.Once
	semanticScorer search.SemanticScorer
}

// Options configures how a database context index is built.
type Options struct {
	// Path is the file path for the .dtx file. If empty, an in-memory
	// SQLite database is used (no file created). In-memory indexes are
	// faster but must be rebuilt on each process start.
	Path string

	// Schemas is a comma-separated list of PostgreSQL schemas to extract.
	// Defaults to "public" if empty.
	Schemas string

	// MaxConns is the maximum number of concurrent PostgreSQL connections
	// in the connection pool. Higher values allow more parallel JSONB
	// analysis. Defaults to 4 if zero.
	MaxConns int

	// Logger receives progress messages during build. If nil, os.Stderr is used.
	Logger io.Writer

	// NoSemantic disables building the optional local embedding-based
	// semantic index. By default (false), Build downloads (if not already
	// cached — see internal/embed) and runs a small local embedding model
	// to add a semantic retrieval signal alongside dbctx's existing
	// lexical/fuzzy matching, improving recall for paraphrased queries
	// (e.g. "buyers" finding a "customers" table). This never replaces
	// lexical matching, only augments it — see [Index.Query].
	//
	// If the model or its inference runtime can't be obtained or loaded
	// (offline, unsupported platform, etc.), Build logs a warning and
	// continues with a lexical-only index rather than failing — semantic
	// indexing is always best-effort.
	NoSemantic bool
}

// Build connects to PostgreSQL and builds a complete database context index.
//
// It extracts schema, analyzes field statistics from pg_stats, discovers JSONB
// structure via sampling, and builds a full-text search index. The resulting
// index is ready for queries immediately upon return.
//
// If opts is nil or opts.Path is empty, the index is stored in memory.
// Pass opts.Path to persist the index as a .dtx file on disk.
//
// The caller must call Close on the returned Index when done.
func Build(ctx context.Context, dsn string, opts *Options) (*Index, error) {
	if opts == nil {
		opts = &Options{}
	}
	log := opts.Logger
	if log == nil {
		log = os.Stderr
	}

	// Connect to PostgreSQL
	fmt.Fprintf(log, "Connecting to PostgreSQL...\n")
	maxConns := opts.MaxConns
	if maxConns == 0 {
		maxConns = 4
	}
	pg, err := db.ConnectWithMaxConns(ctx, dsn, int32(maxConns))
	if err != nil {
		return nil, fmt.Errorf("pg connect: %w", err)
	}
	defer pg.Close(ctx)

	// Open SQLite store
	storePath := opts.Path
	if storePath != "" {
		fmt.Fprintf(log, "Creating %s...\n", storePath)
	} else {
		fmt.Fprintf(log, "Creating in-memory index...\n")
	}
	store, err := db.OpenStore(storePath)
	if err != nil {
		return nil, fmt.Errorf("open store: %w", err)
	}
	if err := store.InitSchema(); err != nil {
		store.Close()
		return nil, fmt.Errorf("init schema: %w", err)
	}

	schemas := opts.Schemas
	if schemas == "" {
		schemas = "public"
	}

	// Phase 1: Schema extraction
	fmt.Fprintf(log, "1/4 Extracting schema (schemas=%s)...\n", schemas)
	ext, err := schema.Extract(ctx, pg, schemas)
	if err != nil {
		store.Close()
		return nil, fmt.Errorf("schema extraction: %w", err)
	}
	if err := storeSchema(store, ext); err != nil {
		store.Close()
		return nil, fmt.Errorf("store schema: %w", err)
	}
	fmt.Fprintf(log, "  %d tables, %d constraints\n", len(ext.Tables), len(ext.Constraints))

	// Phase 2: Field analysis
	fmt.Fprintf(log, "2/4 Analyzing fields...\n")
	if err := analyze.AnalyzeFields(ctx, pg, ext, store, schemas); err != nil {
		store.Close()
		return nil, fmt.Errorf("field analysis: %w", err)
	}

	// Phase 3: JSONB analysis
	fmt.Fprintf(log, "3/4 Analyzing JSONB fields...\n")
	if err := analyze.AnalyzeJSONB(ctx, pg, ext, store); err != nil {
		store.Close()
		return nil, fmt.Errorf("jsonb analysis: %w", err)
	}

	// Phase 4: Build search index
	fmt.Fprintf(log, "4/4 Building search index...\n")
	if err := store.InitFTS(); err != nil {
		store.Close()
		return nil, fmt.Errorf("init fts: %w", err)
	}
	if err := search.PopulateFTS(store); err != nil {
		store.Close()
		return nil, fmt.Errorf("populate fts: %w", err)
	}

	if !opts.NoSemantic {
		buildSemanticIndex(store, log)
	}

	fmt.Fprintf(log, "Done — index ready\n")

	idx := &Index{
		store: store,
		path:  storePath,
		ready: make(chan struct{}),
	}
	close(idx.ready)
	return idx, nil
}

// buildSemanticIndex runs the optional semantic embedding phase, logging
// progress in the same style as the other build phases. Any failure here
// (model download failed, unsupported platform, onnxruntime unavailable)
// is logged as a warning and swallowed — the resulting index is still a
// perfectly usable lexical-only index, matching Options.NoSemantic's
// documented best-effort behavior.
func buildSemanticIndex(store *db.Store, log io.Writer) {
	fmt.Fprintf(log, "5/5 Building semantic index...\n")
	emb, err := semantic.NewDefaultEmbedder(progressLogger(log))
	if err != nil {
		fmt.Fprintf(log, "  semantic indexing unavailable (%v); continuing with lexical-only index\n", err)
		return
	}
	stats, err := semantic.BuildIndex(store, emb, log)
	if err != nil {
		fmt.Fprintf(log, "  semantic indexing failed (%v); continuing with lexical-only index\n", err)
		return
	}
	fmt.Fprintf(log, "  %d objects (%d embedded, %d reused, %d removed)\n", stats.Total, stats.Embedded, stats.Reused, stats.Removed)
}

// progressLogger adapts an io.Writer into an embed.ProgressFunc that prints
// one line per asset the first time it starts downloading, rather than
// flooding the log with a line per chunk.
func progressLogger(log io.Writer) embed.ProgressFunc {
	seen := make(map[string]bool)
	return func(label string, downloaded, total int64) {
		if seen[label] {
			return
		}
		seen[label] = true
		if total > 0 {
			fmt.Fprintf(log, "  downloading %s (%.1f MB)...\n", label, float64(total)/(1024*1024))
		} else {
			fmt.Fprintf(log, "  downloading %s...\n", label)
		}
	}
}

// BuildAsync starts building the index in a background goroutine and returns
// immediately. The returned channel is closed when the build completes.
//
// This is useful for non-blocking application startup. The returned Index
// can be registered with your application immediately. Any calls to [Index.Query],
// [Index.Tables], or other methods will block until the build completes.
//
// Example:
//
//	idx, ready, err := dbctx.BuildAsync(ctx, dsn, nil)
//	if err != nil {
//	    log.Fatal(err)
//	}
//	defer idx.Close()
//
//	// Register idx with your app immediately...
//
//	// Wait for readiness:
//	<-ready
//
// If the build fails, [Index.Err] returns the error and Query/Tables/etc
// will return that error.
func BuildAsync(ctx context.Context, dsn string, opts *Options) (*Index, <-chan struct{}, error) {
	if opts == nil {
		opts = &Options{}
	}

	idx := &Index{
		path:  opts.Path,
		ready: make(chan struct{}),
	}

	go func() {
		defer close(idx.ready)

		log := opts.Logger
		if log == nil {
			log = os.Stderr
		}

		maxConns := opts.MaxConns
		if maxConns == 0 {
			maxConns = 4
		}
		pg, err := db.ConnectWithMaxConns(ctx, dsn, int32(maxConns))
		if err != nil {
			idx.err = fmt.Errorf("pg connect: %w", err)
			return
		}
		defer pg.Close(ctx)

		storePath := opts.Path
		if storePath != "" {
			fmt.Fprintf(log, "Creating %s...\n", storePath)
		} else {
			fmt.Fprintf(log, "Creating in-memory index...\n")
		}
		store, err := db.OpenStore(storePath)
		if err != nil {
			idx.err = fmt.Errorf("open store: %w", err)
			return
		}
		if err := store.InitSchema(); err != nil {
			store.Close()
			idx.err = fmt.Errorf("init schema: %w", err)
			return
		}

		schemas := opts.Schemas
		if schemas == "" {
			schemas = "public"
		}

		fmt.Fprintf(log, "1/4 Extracting schema (schemas=%s)...\n", schemas)
		ext, err := schema.Extract(ctx, pg, schemas)
		if err != nil {
			store.Close()
			idx.err = fmt.Errorf("schema extraction: %w", err)
			return
		}
		if err := storeSchema(store, ext); err != nil {
			store.Close()
			idx.err = fmt.Errorf("store schema: %w", err)
			return
		}
		fmt.Fprintf(log, "  %d tables, %d constraints\n", len(ext.Tables), len(ext.Constraints))

		fmt.Fprintf(log, "2/4 Analyzing fields...\n")
		if err := analyze.AnalyzeFields(ctx, pg, ext, store, schemas); err != nil {
			store.Close()
			idx.err = fmt.Errorf("field analysis: %w", err)
			return
		}

		fmt.Fprintf(log, "3/4 Analyzing JSONB fields...\n")
		if err := analyze.AnalyzeJSONB(ctx, pg, ext, store); err != nil {
			store.Close()
			idx.err = fmt.Errorf("jsonb analysis: %w", err)
			return
		}

		fmt.Fprintf(log, "4/4 Building search index...\n")
		if err := store.InitFTS(); err != nil {
			store.Close()
			idx.err = fmt.Errorf("init fts: %w", err)
			return
		}
		if err := search.PopulateFTS(store); err != nil {
			store.Close()
			idx.err = fmt.Errorf("populate fts: %w", err)
			return
		}

		if !opts.NoSemantic {
			buildSemanticIndex(store, log)
		}

		idx.mu.Lock()
		idx.store = store
		idx.mu.Unlock()

		fmt.Fprintf(log, "Done — index ready\n")
	}()

	return idx, idx.ready, nil
}

// Open opens an existing .dtx file for querying. The file must exist and
// contain a valid dbctx index created by [Build] or the `dbctx build` CLI.
//
// The caller must call Close on the returned Index when done.
func Open(path string) (*Index, error) {
	store, err := db.OpenStore(path)
	if err != nil {
		return nil, fmt.Errorf("open %s: %w", path, err)
	}
	idx := &Index{
		store: store,
		path:  path,
		ready: make(chan struct{}),
	}
	close(idx.ready)
	return idx, nil
}

// Close releases all resources held by the index, including the underlying
// SQLite database connection. After Close, no other methods may be called.
func (idx *Index) Close() error {
	<-idx.ready
	idx.mu.RLock()
	defer idx.mu.RUnlock()
	if idx.store != nil {
		return idx.store.Close()
	}
	return nil
}

// Ready returns a channel that is closed when the index is ready for queries.
// For synchronous builds created with [Build], the channel is already closed.
// For async builds created with [BuildAsync], the channel closes when the
// background build completes.
func (idx *Index) Ready() <-chan struct{} {
	return idx.ready
}

// Err returns the build error if an async build failed. Returns nil if
// the build succeeded or is still in progress. Check [Index.Ready] first
// to know when the build is done.
func (idx *Index) Err() error {
	idx.mu.RLock()
	defer idx.mu.RUnlock()
	return idx.err
}

// Query searches the index for tables matching the given natural language
// query. It combines full-text search, fuzzy table name matching, value
// matching, and foreign-key expansion to find relevant tables and their
// context.
//
// If the index was created with [BuildAsync] and the build is still in
// progress, Query blocks until the build completes.
//
// Returns a [ResultSet] that can be filtered and converted to compact text:
//
//	result, _ := idx.Query("failed reviews last month")
//	text := result.Matched().Text()          // matched + FK-expanded tables
//	text := result.ScoredOnly().Text()       // only tables that scored directly
//	text := result.Include("reviews").Text() // specific tables
func (idx *Index) Query(query string) (*ResultSet, error) {
	<-idx.ready
	if idx.err != nil {
		return nil, idx.err
	}
	idx.mu.RLock()
	defer idx.mu.RUnlock()

	raw, err := search.QueryHybrid(idx.store, query, idx.semantic())
	if err != nil {
		return nil, err
	}
	return newResultSet(raw), nil
}

// semantic lazily opens the semantic scorer for this index on first use.
// If the store has no semantic index, or the embedding backend can't be
// loaded (offline, unsupported platform, etc.), this returns nil once and
// every subsequent Query call cheaply reuses that nil result rather than
// retrying — semantic search degrades to "not available" the same way a
// missing index does, never to a per-query error.
func (idx *Index) semantic() search.SemanticScorer {
	idx.semanticOnce.Do(func() {
		scorer, err := semantic.OpenScorer(idx.store, progressLogger(os.Stderr))
		if err != nil {
			fmt.Fprintf(os.Stderr, "dbctx: semantic search unavailable (%v); using lexical search only\n", err)
			return
		}
		idx.semanticScorer = scorer
	})
	return idx.semanticScorer
}

// Tables returns a summary of all tables in the index. Each entry includes
// the table name, schema, row estimate, column count, and FK count.
//
// Blocks until the index is ready if an async build is in progress.
func (idx *Index) Tables() ([]TableSummary, error) {
	<-idx.ready
	if idx.err != nil {
		return nil, idx.err
	}
	idx.mu.RLock()
	defer idx.mu.RUnlock()

	rows, err := idx.store.DB().Query(`
		SELECT t.id, t.schema, t.name, t.row_estimate,
		       (SELECT COUNT(*) FROM columns c WHERE c.table_id = t.id) as col_count,
		       (SELECT COUNT(*) FROM foreign_keys fk WHERE fk.table_id = t.id) as fk_count
		FROM tables t
		ORDER BY t.name
	`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var tables []TableSummary
	for rows.Next() {
		var t TableSummary
		if rows.Scan(&t.ID, &t.Schema, &t.Name, &t.RowEstimate, &t.ColCount, &t.FKCount) == nil {
			tables = append(tables, t)
		}
	}
	return tables, rows.Err()
}

// TableDetail returns detailed information about a specific table, including
// columns with types, PK/FK tags, value distributions, JSONB paths, and
// foreign key relationships.
//
// Returns nil and no error if the table is not found.
// Blocks until the index is ready if an async build is in progress.
func (idx *Index) TableDetail(name string) (*TableDetail, error) {
	<-idx.ready
	if idx.err != nil {
		return nil, idx.err
	}
	idx.mu.RLock()
	defer idx.mu.RUnlock()

	var td TableDetail
	err := idx.store.DB().QueryRow(`
		SELECT t.id, t.schema, t.name, t.row_estimate,
		       (SELECT COUNT(*) FROM columns c WHERE c.table_id = t.id) as col_count,
		       (SELECT COUNT(*) FROM foreign_keys fk WHERE fk.table_id = t.id) as fk_count
		FROM tables t WHERE t.name = ?
	`, name).Scan(&td.ID, &td.Schema, &td.Name, &td.RowEstimate, &td.ColCount, &td.FKCount)
	if err != nil {
		return nil, err
	}

	// Primary keys
	pkRows, _ := idx.store.DB().Query(`SELECT column_name FROM primary_keys WHERE table_id = ?`, td.ID)
	if pkRows != nil {
		for pkRows.Next() {
			var col string
			if pkRows.Scan(&col) == nil {
				td.PrimaryKey = append(td.PrimaryKey, col)
			}
		}
		pkRows.Close()
	}

	// Foreign keys
	fkRows, _ := idx.store.DB().Query(`
		SELECT fk.src_columns, rt.name, fk.dst_columns
		FROM foreign_keys fk
		JOIN tables rt ON rt.id = fk.ref_table_id
		WHERE fk.table_id = ?
	`, td.ID)
	if fkRows != nil {
		for fkRows.Next() {
			var fk FKInfo
			if fkRows.Scan(&fk.SrcColumns, &fk.RefTable, &fk.DstColumns) == nil {
				td.ForeignKeys = append(td.ForeignKeys, fk)
			}
		}
		fkRows.Close()
	}

	// Columns with stats
	colRows, _ := idx.store.DB().Query(`
		SELECT c.name, c.type, c.nullable,
		       CASE WHEN pk.column_name IS NOT NULL THEN 1 ELSE 0 END,
		       COALESCE(fs.distinct_count, 0),
		       COALESCE(fs.is_state_like, 0),
		       COALESCE(fs.is_categorical, 0)
		FROM columns c
		LEFT JOIN primary_keys pk ON pk.table_id = c.table_id AND pk.column_name = c.name
		LEFT JOIN field_stats fs ON fs.column_id = c.id
		WHERE c.table_id = ?
		ORDER BY c.position
	`, td.ID)
	if colRows != nil {
		for colRows.Next() {
			var cd ColumnDetail
			var isPK, isState, isCat int
			if colRows.Scan(&cd.Name, &cd.Type, &cd.Nullable, &isPK, &cd.Distinct, &isState, &isCat) == nil {
				cd.IsPK = isPK == 1
				cd.IsState = isState == 1
				cd.IsCategoric = isCat == 1
				td.Columns = append(td.Columns, cd)
			}
		}
		colRows.Close()
	}

	// FK targets per column
	fkByCol := make(map[string]string)
	for _, fk := range td.ForeignKeys {
		for _, col := range strings.Split(fk.SrcColumns, ",") {
			dst := fk.RefTable
			if fk.DstColumns != "id" {
				dst += "." + fk.DstColumns
			}
			fkByCol[col] = dst
		}
	}
	for i := range td.Columns {
		if target, ok := fkByCol[td.Columns[i].Name]; ok {
			td.Columns[i].FKTarget = target
		}
	}

	// Values for state/categorical columns
	for i := range td.Columns {
		if td.Columns[i].IsState || td.Columns[i].IsCategoric {
			valRows, _ := idx.store.DB().Query(`
				SELECT value, frequency FROM field_values fv
				JOIN columns c ON c.id = fv.column_id
				WHERE c.table_id = ? AND c.name = ?
				ORDER BY frequency DESC
			`, td.ID, td.Columns[i].Name)
			if valRows != nil {
				for valRows.Next() {
					var v ValueInfo
					if valRows.Scan(&v.Value, &v.Frequency) == nil {
						td.Columns[i].Values = append(td.Columns[i].Values, v)
					}
				}
				valRows.Close()
			}
		}
	}

	// JSONB paths
	for i := range td.Columns {
		if td.Columns[i].Type == "jsonb" || td.Columns[i].Type == "json" {
			jRows, _ := idx.store.DB().Query(`
				SELECT jp.path, jp.inferred_type, jp.sample_values
				FROM jsonb_paths jp
				JOIN columns c ON c.id = jp.column_id
				WHERE c.table_id = ? AND c.name = ?
				ORDER BY jp.path
			`, td.ID, td.Columns[i].Name)
			if jRows != nil {
				for jRows.Next() {
					var jp JSONBPathInfo
					if jRows.Scan(&jp.Path, &jp.InferredType, &jp.SampleValues) == nil {
						td.Columns[i].JSONBPaths = append(td.Columns[i].JSONBPaths, jp)
					}
				}
				jRows.Close()
			}
		}
	}

	return &td, nil
}

// Stats returns summary statistics about the index, including counts of
// tables, columns, foreign keys, state fields, categorical fields, JSONB
// paths, and field values.
//
// Blocks until the index is ready if an async build is in progress.
func (idx *Index) Stats() (*Stats, error) {
	<-idx.ready
	if idx.err != nil {
		return nil, idx.err
	}
	idx.mu.RLock()
	defer idx.mu.RUnlock()

	var s Stats
	idx.store.DB().QueryRow("SELECT COUNT(*) FROM tables").Scan(&s.Tables)
	idx.store.DB().QueryRow("SELECT COUNT(*) FROM columns").Scan(&s.Columns)
	idx.store.DB().QueryRow("SELECT COUNT(*) FROM foreign_keys").Scan(&s.ForeignKeys)
	idx.store.DB().QueryRow("SELECT COUNT(*) FROM field_stats WHERE is_state_like = 1").Scan(&s.StateFields)
	idx.store.DB().QueryRow("SELECT COUNT(*) FROM field_stats WHERE is_categorical = 1").Scan(&s.CategoricalFields)
	idx.store.DB().QueryRow("SELECT COUNT(*) FROM jsonb_paths").Scan(&s.JSONBPaths)
	idx.store.DB().QueryRow("SELECT COUNT(*) FROM field_values").Scan(&s.FieldValues)
	return &s, nil
}

// Report writes a human-readable report of the entire index to w.
// The report includes schema, state fields, categorical fields, JSONB
// structure, relationships, and summary statistics.
//
// Blocks until the index is ready if an async build is in progress.
func (idx *Index) Report(w io.Writer) error {
	<-idx.ready
	if idx.err != nil {
		return idx.err
	}
	idx.mu.RLock()
	defer idx.mu.RUnlock()
	return report.ReportAll(w, idx.store)
}

// TerminologyPrompt generates a self-contained prompt that can be pasted
// into a large external LLM (Claude, GPT, Gemini, or similar) to
// interactively derive a terminology dictionary for this database — a
// mapping from domain vocabulary (abbreviations, acronyms, business
// jargon) to the exact schema objects it refers to. dbctx never calls an
// LLM itself; this only produces text for the caller to use however they
// like (print it, copy it, pipe it into their own LLM integration).
//
// The prompt embeds the complete schema this Index already knows —
// tables, columns, relationships, state/categorical values, and JSONB
// structure — so it is usable on its own without additional context.
//
// See [Index.ImportTerminology] to load the LLM's resulting output back
// into the index.
func (idx *Index) TerminologyPrompt() (string, error) {
	<-idx.ready
	if idx.err != nil {
		return "", idx.err
	}
	idx.mu.RLock()
	defer idx.mu.RUnlock()
	return terminology.GeneratePrompt(idx.store)
}

// ImportTerminology validates and persists a terminology dictionary —
// typically produced by working through the prompt from
// [Index.TerminologyPrompt] with an external LLM — into this index.
//
// data must be a JSON array of term groups:
//
//	[{"term": "loc", "aliases": ["lines of code"], "targets": ["metrics.loc"]}]
//
// data is just bytes: a JSON string literal works fine as
// []byte(jsonString), a value read from a file, an HTTP request body, or
// anything else that ends up as a []byte — there's no separate
// string-typed variant of this method because none is needed. If you
// already have a file on disk, [Index.ImportTerminologyFile] saves the
// os.ReadFile boilerplate; if you already have Go values instead of JSON
// text (e.g. built programmatically), use [Index.ImportTerminologyGroups]
// to skip the JSON round-trip entirely.
//
// Every alias/target pair is validated against the actual schema before
// being persisted; entries that don't resolve to a real table, column, or
// JSONB path are rejected individually (reported in the result) rather
// than failing the whole import. Terminology is purely additive to
// retrieval — see the package documentation — and is never required.
func (idx *Index) ImportTerminology(data []byte) (*TerminologyImportResult, error) {
	<-idx.ready
	if idx.err != nil {
		return nil, idx.err
	}
	idx.mu.RLock()
	defer idx.mu.RUnlock()

	result, err := terminology.Import(idx.store, data)
	if err != nil {
		return nil, err
	}
	return convertImportResult(result), nil
}

// ImportTerminologyFile reads path and passes its contents to
// [Index.ImportTerminology] — the same JSON format, just read from disk
// for convenience instead of requiring the caller to os.ReadFile it
// first. This is what `dbctx terminology import` uses internally.
func (idx *Index) ImportTerminologyFile(path string) (*TerminologyImportResult, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read %s: %w", path, err)
	}
	return idx.ImportTerminology(data)
}

// ImportTerminologyGroups validates and persists terminology supplied as
// Go values rather than JSON text — for callers building a dictionary
// programmatically (from their own data source, a different format, code
// generation, ...) who would otherwise have to marshal it to JSON just to
// call [Index.ImportTerminology]. Validation and persistence behavior are
// identical either way; this only changes how the input arrives.
func (idx *Index) ImportTerminologyGroups(groups []TerminologyGroup) (*TerminologyImportResult, error) {
	<-idx.ready
	if idx.err != nil {
		return nil, idx.err
	}
	idx.mu.RLock()
	defer idx.mu.RUnlock()

	internal := make([]terminology.TermGroup, len(groups))
	for i, g := range groups {
		internal[i] = terminology.TermGroup{Term: g.Term, Aliases: g.Aliases, Targets: g.Targets}
	}
	result, err := terminology.ImportGroups(idx.store, internal)
	if err != nil {
		return nil, err
	}
	return convertImportResult(result), nil
}

// Terminology returns every currently-imported terminology entry, for
// inspection — so the user-supplied mappings influencing retrieval are
// never a black box. Returns an empty slice if none have been imported.
func (idx *Index) Terminology() ([]TerminologyEntry, error) {
	<-idx.ready
	if idx.err != nil {
		return nil, idx.err
	}
	idx.mu.RLock()
	defer idx.mu.RUnlock()

	entries, err := terminology.List(idx.store)
	if err != nil {
		return nil, err
	}
	out := make([]TerminologyEntry, 0, len(entries))
	for _, e := range entries {
		out = append(out, TerminologyEntry{
			Term:         e.Term,
			Alias:        e.Alias,
			TargetTable:  e.TargetTable,
			TargetColumn: e.TargetColumn,
			TargetPath:   e.TargetPath,
			Source:       e.Source,
			ImportedAt:   e.ImportedAt,
		})
	}
	return out, nil
}

func convertImportResult(r *terminology.ImportResult) *TerminologyImportResult {
	out := &TerminologyImportResult{Accepted: r.Accepted}
	for _, rej := range r.Rejected {
		out.Rejected = append(out.Rejected, RejectedTerminology{
			Term: rej.Term, Alias: rej.Alias, Target: rej.Target, Reason: rej.Reason,
		})
	}
	return out
}

// --- Types ---

// ResultSet holds the results of a query and provides methods to select
// subsets of matched tables and render them as compact text.
//
// The typical flow is:
//
//	result, _ := idx.Query("failed reviews")
//	text := result.Matched().Text()  // compact schema: matched + FK-expanded
type ResultSet struct {
	// Query is the original query string.
	Query string `json:"query"`
	// Tables contains all tables in the result, including both directly
	// matched tables (score > 0) and FK-expanded tables (score = 0).
	Tables []TableContext `json:"tables"`
	// SemanticHits lists the evidence the optional semantic retrieval
	// signal contributed to this result, if semantic search was available
	// and ran. Empty when semantic search is disabled/unavailable, or when
	// it ran but found nothing above the other tables already found
	// lexically. This exists so a result's ranking is inspectable — why a
	// table appeared even without a lexical match — not just a black-box
	// score. See the design principle: "Engineers should be able to
	// understand why a particular table or field appeared."
	SemanticHits []SemanticHit `json:"semantic_hits,omitempty"`
	// Timing breaks down how long each phase of the query took, so
	// latency is inspectable the same way scoring is.
	Timing QueryTiming `json:"timing"`
}

// QueryTiming records how long each phase of a query took, in
// milliseconds. SemanticRan distinguishes "semantic search ran and took
// SemanticMs" from "semantic search did not run at all" (SemanticMs left
// at zero either way).
type QueryTiming struct {
	LexicalMs   float64 `json:"lexical_ms"`
	SemanticMs  float64 `json:"semantic_ms"`
	SemanticRan bool    `json:"semantic_ran"`
	ExpandMs    float64 `json:"expand_ms"`
	TotalMs     float64 `json:"total_ms"`
}

// SemanticHit is one piece of evidence the semantic retrieval signal
// contributed: the best-matching embedded schema object for a table and
// its similarity to the query.
type SemanticHit struct {
	TableName string  `json:"table_name"`
	Kind      string  `json:"kind"`
	Text      string  `json:"text"`
	Score     float64 `json:"score"`
}

// Matched returns a [Selection] containing every table relevant to
// competently answering the query: tables that scored a direct hit from
// the retrieval signals (fuzzy, FTS, value, terminology, semantic), plus
// tables pulled in via foreign-key expansion so join context isn't
// silently dropped. This is the recommended default for feeding an LLM
// or text-to-SQL system — see [ResultSet.ScoredOnly] for just the
// directly-scored subset, with FK context excluded.
func (rs *ResultSet) Matched() *Selection {
	names := make([]string, 0, len(rs.Tables))
	for _, t := range rs.Tables {
		names = append(names, t.TableName)
	}
	return &Selection{rs: rs, names: names}
}

// ScoredOnly returns a [Selection] containing only tables that scored a
// direct match (MatchScore > 0) from the retrieval signals, excluding
// tables pulled in purely via foreign-key expansion. Use this for the
// narrow "what literally matched" view — e.g. inspecting retrieval
// quality — rather than [ResultSet.Matched]'s full join-ready context.
func (rs *ResultSet) ScoredOnly() *Selection {
	names := make([]string, 0)
	for _, t := range rs.Tables {
		if t.MatchScore > 0 {
			names = append(names, t.TableName)
		}
	}
	return &Selection{rs: rs, names: names}
}

// Include returns a [Selection] containing only the named tables.
// Tables not found in the result set are silently ignored.
func (rs *ResultSet) Include(names ...string) *Selection {
	valid := make(map[string]bool)
	for _, t := range rs.Tables {
		valid[t.TableName] = true
	}
	filtered := make([]string, 0, len(names))
	for _, n := range names {
		if valid[n] {
			filtered = append(filtered, n)
		}
	}
	return &Selection{rs: rs, names: filtered}
}

// TableMap returns a map of table name to [TableContext] for quick lookup.
func (rs *ResultSet) TableMap() map[string]TableContext {
	m := make(map[string]TableContext, len(rs.Tables))
	for _, t := range rs.Tables {
		m[t.TableName] = t
	}
	return m
}

// Selection represents a subset of tables from a [ResultSet]. It provides
// methods to refine the selection and render it as compact text suitable
// for passing to an LLM or text-to-SQL system.
type Selection struct {
	rs    *ResultSet
	names []string
}

// Include adds the named tables to the selection. Tables not in the
// result set are silently ignored.
func (s *Selection) Include(names ...string) *Selection {
	valid := make(map[string]bool)
	for _, t := range s.rs.Tables {
		valid[t.TableName] = true
	}
	existing := make(map[string]bool, len(s.names))
	for _, n := range s.names {
		existing[n] = true
	}
	for _, n := range names {
		if valid[n] && !existing[n] {
			s.names = append(s.names, n)
			existing[n] = true
		}
	}
	return s
}

// Exclude removes the named tables from the selection.
func (s *Selection) Exclude(names ...string) *Selection {
	exclude := make(map[string]bool, len(names))
	for _, n := range names {
		exclude[n] = true
	}
	filtered := s.names[:0]
	for _, n := range s.names {
		if !exclude[n] {
			filtered = append(filtered, n)
		}
	}
	s.names = filtered
	return s
}

// Tables returns the [TableContext] objects in this selection, in the
// same order they appear in the original result set.
func (s *Selection) Tables() []TableContext {
	tableMap := s.rs.TableMap()
	result := make([]TableContext, 0, len(s.names))
	for _, name := range s.names {
		if t, ok := tableMap[name]; ok {
			result = append(result, t)
		}
	}
	return result
}

// Len returns the number of tables in the selection.
func (s *Selection) Len() int {
	return len(s.names)
}

// Text renders the selected tables as compact, human-readable text with
// a notation legend at the top. The legend explains every symbol and
// annotation used in the output so that an LLM (or human) can interpret
// the schema without external documentation.
//
// Use [Selection.TextRaw] to omit the legend.
func (s *Selection) Text() string {
	return legend() + "\n" + s.TextRaw()
}

// TextRaw renders the selected tables as compact, human-readable text
// without the notation legend. Use this when the caller already knows
// the notation, or when token budget is tight and the legend would be
// wasted context.
//
// The output includes table names, scores, primary keys, foreign keys,
// columns with type/flags, state/categorical values, and JSONB paths.
func (s *Selection) TextRaw() string {
	tableMap := s.rs.TableMap()
	var buf strings.Builder
	for i, name := range s.names {
		t, ok := tableMap[name]
		if !ok {
			continue
		}
		if i > 0 {
			buf.WriteString("\n\n")
		}
		writeTableText(&buf, t)
	}
	return buf.String()
}

// TableContext represents a table in a query result with its relevance score
// and full context (columns, values, relationships, JSONB paths).
type TableContext struct {
	TableName   string       `json:"table_name"`
	Schema      string       `json:"schema"`
	Columns     []ColumnInfo `json:"columns"`
	PrimaryKey  []string     `json:"primary_key"`
	ForeignKeys []FKInfo     `json:"foreign_keys"`
	IsMatch     bool         `json:"is_match"`
	MatchScore  float64      `json:"match_score"`
	// Score documents exactly how MatchScore was computed, signal by
	// signal (FTS, fuzzy, value, terminology, and — if it ran — semantic).
	// Nil for tables that were only pulled in via foreign-key expansion.
	Score *ScoreBreakdown `json:"score,omitempty"`
}

// ScoreBreakdown is the "show your work" behind TableContext.MatchScore:
// every lexical signal's raw score, its fixed weight, and the resulting
// weighted contribution, plus the semantic signal's contribution if one
// ran. See [search.ScoreBreakdown] (internal/search) for the full formula
// documentation this mirrors.
type ScoreBreakdown struct {
	FTS          SignalContribution    `json:"fts"`
	Fuzzy        SignalContribution    `json:"fuzzy"`
	Value        SignalContribution    `json:"value"`
	Terminology  SignalContribution    `json:"terminology"`
	LexicalTotal float64               `json:"lexical_total"`
	Semantic     *SemanticContribution `json:"semantic,omitempty"`
	FinalScore   float64               `json:"final_score"`
}

// SignalContribution is one lexical signal's raw score, its fixed weight,
// and the resulting weighted contribution (Raw * Weight).
type SignalContribution struct {
	Raw          float64 `json:"raw"`
	Weight       float64 `json:"weight"`
	Contribution float64 `json:"contribution"`
}

// SemanticContribution documents how the optional semantic signal
// contributed to a table's final score: contribution = Weight *
// Normalized * Scale. Cosine is the best-matching embedded object's raw
// similarity to the query; Normalized is that score after query-relative
// min-max normalization; Scale is the strongest lexical score found
// anywhere in the query (or 1.0 if lexical found nothing at all).
type SemanticContribution struct {
	Cosine       float64 `json:"cosine"`
	Normalized   float64 `json:"normalized"`
	Weight       float64 `json:"weight"`
	Scale        float64 `json:"scale"`
	Contribution float64 `json:"contribution"`
	EvidenceKind string  `json:"evidence_kind"`
	EvidenceText string  `json:"evidence_text"`
}

// ColumnInfo describes a column in a query result, including its type,
// flags (PK, nullable, state, categorical), representative values, and
// JSONB paths if applicable.
type ColumnInfo struct {
	Name        string          `json:"name"`
	Type        string          `json:"type"`
	Nullable    bool            `json:"nullable"`
	IsPK        bool            `json:"is_pk"`
	FKTarget    string          `json:"fk_target,omitempty"`
	IsState     bool            `json:"is_state"`
	IsCategoric bool            `json:"is_categoric"`
	Values      []ValueInfo     `json:"values,omitempty"`
	JSONBPaths  []JSONBPathInfo `json:"jsonb_paths,omitempty"`
}

// ValueInfo represents a representative value for a field, with its
// frequency (as permille, 0-1000).
type ValueInfo struct {
	Value     string `json:"value"`
	Frequency int    `json:"frequency"`
}

// JSONBPathInfo describes a path within a JSONB column, including its
// inferred type and sample values.
type JSONBPathInfo struct {
	Path         string `json:"path"`
	InferredType string `json:"inferred_type"`
	SampleValues string `json:"sample_values,omitempty"`
}

// FKInfo describes a foreign key relationship between tables.
type FKInfo struct {
	SrcColumns string `json:"src_columns"`
	RefTable   string `json:"ref_table"`
	DstColumns string `json:"dst_columns"`
}

// TableSummary is a lightweight table descriptor returned by [Index.Tables].
type TableSummary struct {
	ID          int     `json:"id"`
	Schema      string  `json:"schema"`
	Name        string  `json:"name"`
	RowEstimate float64 `json:"row_estimate"`
	ColCount    int     `json:"columns"`
	FKCount     int     `json:"fk_count"`
}

// TableDetail contains complete information about a table, including
// columns with types, flags, values, JSONB paths, and all relationships.
type TableDetail struct {
	TableSummary
	PrimaryKey  []string       `json:"primary_key"`
	ForeignKeys []FKInfo       `json:"foreign_keys"`
	Columns     []ColumnDetail `json:"columns"`
}

// ColumnDetail describes a column in a table detail response. It includes
// distinct count, state/categorical flags, representative values, and
// JSONB paths.
type ColumnDetail struct {
	Name        string          `json:"name"`
	Type        string          `json:"type"`
	Nullable    bool            `json:"nullable"`
	IsPK        bool            `json:"is_pk"`
	FKTarget    string          `json:"fk_target,omitempty"`
	Distinct    int             `json:"distinct"`
	IsState     bool            `json:"is_state"`
	IsCategoric bool            `json:"is_categoric"`
	Values      []ValueInfo     `json:"values,omitempty"`
	JSONBPaths  []JSONBPathInfo `json:"jsonb_paths,omitempty"`
}

// TerminologyGroup is one term with all of its human-language aliases and
// the exact schema objects it refers to — the unit of input
// [Index.ImportTerminologyGroups] accepts, and the Go-value equivalent of
// one entry in the JSON array [Index.ImportTerminology] parses:
//
//	dbctx.TerminologyGroup{
//	    Term:    "loc",
//	    Aliases: []string{"lines of code", "source lines of code"},
//	    Targets: []string{"metrics.loc"},
//	}
//
// Targets use dbctx's "table" / "table.column" / "table.column:$.json.path"
// notation (see [Index.TerminologyPrompt]'s generated instructions) and
// are validated against the actual schema on import — see
// [Index.ImportTerminologyGroups].
type TerminologyGroup struct {
	Term    string   `json:"term"`
	Aliases []string `json:"aliases"`
	Targets []string `json:"targets"`
}

// TerminologyEntry is one user-approved (alias -> schema object) mapping,
// as returned by [Index.Terminology].
type TerminologyEntry struct {
	Term         string `json:"term"`
	Alias        string `json:"alias"`
	TargetTable  string `json:"target_table"`
	TargetColumn string `json:"target_column,omitempty"`
	TargetPath   string `json:"target_path,omitempty"`
	Source       string `json:"source"`
	ImportedAt   string `json:"imported_at,omitempty"`
}

// TerminologyImportResult summarizes an [Index.ImportTerminology] call.
type TerminologyImportResult struct {
	Accepted int                   `json:"accepted"`
	Rejected []RejectedTerminology `json:"rejected,omitempty"`
}

// RejectedTerminology records one terminology mapping ImportTerminology
// refused to persist, and why (e.g. the target doesn't resolve to a real
// schema object) — so a partially-rejected import is inspectable rather
// than silently dropping entries.
type RejectedTerminology struct {
	Term   string `json:"term"`
	Alias  string `json:"alias"`
	Target string `json:"target"`
	Reason string `json:"reason"`
}

// Stats contains summary statistics about a database context index.
type Stats struct {
	Tables            int `json:"tables"`
	Columns           int `json:"columns"`
	ForeignKeys       int `json:"foreign_keys"`
	StateFields       int `json:"state_fields"`
	CategoricalFields int `json:"categorical_fields"`
	JSONBPaths        int `json:"jsonb_paths"`
	FieldValues       int `json:"field_values"`
}

// --- internal helpers ---

func newResultSet(raw *search.SearchResult) *ResultSet {
	if raw == nil {
		return nil
	}
	r := &ResultSet{Query: raw.Query}
	for _, t := range raw.Tables {
		r.Tables = append(r.Tables, convertTableContext(t))
	}
	for _, h := range raw.SemanticHits {
		r.SemanticHits = append(r.SemanticHits, SemanticHit{
			TableName: h.TableName,
			Kind:      h.Kind,
			Text:      h.Text,
			Score:     h.Score,
		})
	}
	r.Timing = QueryTiming{
		LexicalMs:   raw.Timing.LexicalMs,
		SemanticMs:  raw.Timing.SemanticMs,
		SemanticRan: raw.Timing.SemanticRan,
		ExpandMs:    raw.Timing.ExpandMs,
		TotalMs:     raw.Timing.TotalMs,
	}
	return r
}

func convertTableContext(t search.TableContext) TableContext {
	tc := TableContext{
		TableName:  t.TableName,
		Schema:     t.Schema,
		IsMatch:    t.IsMatch,
		MatchScore: t.MatchScore,
		Score:      convertScoreBreakdown(t.Score),
	}
	tc.PrimaryKey = append(tc.PrimaryKey, t.PrimaryKey...)
	for _, fk := range t.ForeignKeys {
		tc.ForeignKeys = append(tc.ForeignKeys, FKInfo{
			SrcColumns: fk.SrcColumns,
			RefTable:   fk.RefTable,
			DstColumns: fk.DstColumns,
		})
	}
	for _, c := range t.Columns {
		tc.Columns = append(tc.Columns, convertColumnInfo(c))
	}
	return tc
}

func convertScoreBreakdown(bd *search.ScoreBreakdown) *ScoreBreakdown {
	if bd == nil {
		return nil
	}
	convertSignal := func(s search.SignalContribution) SignalContribution {
		return SignalContribution{Raw: s.Raw, Weight: s.Weight, Contribution: s.Contribution}
	}
	out := &ScoreBreakdown{
		FTS:          convertSignal(bd.FTS),
		Fuzzy:        convertSignal(bd.Fuzzy),
		Value:        convertSignal(bd.Value),
		Terminology:  convertSignal(bd.Terminology),
		LexicalTotal: bd.LexicalTotal,
		FinalScore:   bd.FinalScore,
	}
	if bd.Semantic != nil {
		out.Semantic = &SemanticContribution{
			Cosine:       bd.Semantic.Cosine,
			Normalized:   bd.Semantic.Normalized,
			Weight:       bd.Semantic.Weight,
			Scale:        bd.Semantic.Scale,
			Contribution: bd.Semantic.Contribution,
			EvidenceKind: bd.Semantic.EvidenceKind,
			EvidenceText: bd.Semantic.EvidenceText,
		}
	}
	return out
}

func convertColumnInfo(c search.ColumnInfo) ColumnInfo {
	ci := ColumnInfo{
		Name:        c.Name,
		Type:        c.Type,
		Nullable:    c.Nullable,
		IsPK:        c.IsPK,
		FKTarget:    c.FKTarget,
		IsState:     c.IsState,
		IsCategoric: c.IsCategoric,
	}
	for _, v := range c.Values {
		ci.Values = append(ci.Values, ValueInfo{Value: v.Value, Frequency: v.Frequency})
	}
	for _, jp := range c.JSONBPaths {
		ci.JSONBPaths = append(ci.JSONBPaths, JSONBPathInfo{
			Path:         jp.Path,
			InferredType: jp.InferredType,
			SampleValues: jp.SampleValues,
		})
	}
	return ci
}

// legend returns the notation guide prepended to Text() output. It explains
// every symbol and annotation so that an LLM (or human reader) can interpret
// the compact schema without external documentation.
func legend() string {
	return search.Legend()
}

// writeTableText writes a single table's compact text representation to buf.
func writeTableText(buf *strings.Builder, t TableContext) {
	// Header: table name + score
	buf.WriteString(t.TableName)
	if t.MatchScore > 0 {
		fmt.Fprintf(buf, "  (score: %.2f)", t.MatchScore)
	}
	buf.WriteByte('\n')

	// Primary key
	if len(t.PrimaryKey) > 0 {
		fmt.Fprintf(buf, "  PK: %s\n", strings.Join(t.PrimaryKey, ", "))
	}

	// Foreign keys
	for _, fk := range t.ForeignKeys {
		dst := fk.RefTable
		if fk.DstColumns != "id" {
			dst += "." + fk.DstColumns
		}
		fmt.Fprintf(buf, "  %s → %s\n", fk.SrcColumns, dst)
	}

	// Columns
	for _, c := range t.Columns {
		var flags []string
		if c.IsPK {
			flags = append(flags, "^")
		}
		if c.Nullable {
			flags = append(flags, "?")
		}
		if c.FKTarget != "" {
			flags = append(flags, ">"+c.FKTarget)
		}
		if c.IsState {
			flags = append(flags, "[state]")
		}
		if c.IsCategoric && !c.IsState {
			flags = append(flags, "[cat]")
		}

		fmt.Fprintf(buf, "  %s %s", c.Name, search.ShortenTypeParam(c.Type))
		if len(flags) > 0 {
			buf.WriteString(" " + strings.Join(flags, " "))
		}
		buf.WriteByte('\n')

		// Representative values
		if len(c.Values) > 0 {
			vals := make([]string, 0, len(c.Values))
			for _, v := range c.Values {
				vals = append(vals, v.Value)
			}
			fmt.Fprintf(buf, "    {%s}\n", strings.Join(vals, ", "))
		}

		// JSONB paths
		for _, jp := range c.JSONBPaths {
			fmt.Fprintf(buf, "    %s  %s", jp.Path, jp.InferredType)
			if jp.SampleValues != "" {
				fmt.Fprintf(buf, "  {%s}", jp.SampleValues)
			}
			buf.WriteByte('\n')
		}
	}
}

// storeSchema writes the extracted PostgreSQL schema into the SQLite store.
func storeSchema(store *db.Store, ext *schema.ExtractedSchema) error {
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
			return fmt.Errorf("insert table %s: %w", t.Name, err)
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
				return fmt.Errorf("insert column %s: %w", c.Name, err)
			}
			cid, _ := res.LastInsertId()
			columnIDByTableCol[tableOID+":"+c.Name] = cid
		}
	}

	for _, c := range ext.Constraints {
		tid, ok := tableIDByOID[c.TableOID]
		if !ok {
			continue
		}
		switch c.Kind {
		case "PK":
			for _, col := range splitCSV(c.SrcColumns) {
				tx.Exec("INSERT OR IGNORE INTO primary_keys (table_id, column_name) VALUES (?, ?)", tid, col)
			}
		case "UQ":
			tx.Exec("INSERT INTO indexes_info (table_id, name, columns, is_unique) VALUES (?, ?, ?, 1)",
				tid, c.ConName, c.SrcColumns)
		case "FK":
			refOID := findOIDByTable(ext, c.RefSchema, c.RefTable)
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

	return tx.Commit()
}

func findOIDByTable(ext *schema.ExtractedSchema, schemaName, name string) string {
	for _, t := range ext.Tables {
		if t.Schema == schemaName && t.Name == name {
			return t.OID
		}
	}
	return ""
}

func splitCSV(s string) []string {
	var parts []string
	for _, p := range strings.Split(s, ",") {
		parts = append(parts, strings.TrimSpace(p))
	}
	return parts
}
