// Package dbctx provides a PostgreSQL database context compiler and index.
//
// dbctx connects to PostgreSQL, extracts schema, analyzes fields, values,
// and JSONB structure, and stores everything in a queryable SQLite index.
// The index can be stored on disk as a .dtx file or kept entirely in memory.
//
// # Building an index
//
// The simplest way to create an index is [Build], which connects to PostgreSQL,
// extracts everything, and returns a ready-to-query index:
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
//	for _, t := range result.Tables {
//	    fmt.Printf("%s (score: %.2f)\n", t.TableName, t.MatchScore)
//	}
//
// To save the index to a .dtx file for later reuse:
//
//	idx, err := dbctx.Build(ctx, dsn, &dbctx.Options{Path: "mydb.dtx"})
//
// To open an existing .dtx file:
//
//	idx, err := dbctx.Open("mydb.dtx")
//
// # Non-blocking startup
//
// For applications that need database context available at startup without
// blocking, use [BuildAsync]. It starts the build in a background goroutine
// and returns immediately. Queries made before the build completes will
// block until the index is ready:
//
//	idx, ready, err := dbctx.BuildAsync(ctx, dsn, nil)
//	if err != nil {
//	    log.Fatal(err)
//	}
//	defer idx.Close()
//
//	// idx is available immediately for configuration
//	// Register it with your application, set up handlers, etc.
//
//	go func() {
//	    <-ready
//	    log.Println("dbctx index ready")
//	}()
//
//	// Queries automatically wait for the index to be ready:
//	result, err := idx.Query("some query") // blocks if build still running
//
// For a non-blocking check instead of waiting:
//
//	select {
//	case <-idx.Ready():
//	    // index is ready, query now
//	default:
//	    // index still building, use fallback
//	}
//
// # In-memory index
//
// When Options.Path is empty (or nil Options), the index is stored in memory
// using SQLite's in-memory mode. No files are created. This is useful for
// ephemeral indexes, testing, or when the .dtx file is not needed:
//
//	idx, _ := dbctx.Build(ctx, dsn, nil) // in-memory
//
// In-memory indexes are faster to build (no disk I/O) but must be rebuilt
// each time the process starts.
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
	"github.com/shrsv/dbctx/internal/report"
	"github.com/shrsv/dbctx/internal/schema"
	"github.com/shrsv/dbctx/internal/search"
	"golang.org/x/sync/errgroup"
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

	// Phase 2+3: Field analysis + JSONB analysis (concurrent)
	fmt.Fprintf(log, "2/4 Analyzing fields + JSONB (concurrent)...\n")
	g, gctx := errgroup.WithContext(ctx)
	g.Go(func() error {
		return analyze.AnalyzeFields(gctx, pg, ext, store, schemas)
	})
	g.Go(func() error {
		return analyze.AnalyzeJSONB(gctx, pg, ext, store)
	})
	if err := g.Wait(); err != nil {
		store.Close()
		return nil, fmt.Errorf("analysis: %w", err)
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

	fmt.Fprintf(log, "Done — index ready\n")

	idx := &Index{
		store: store,
		path:  storePath,
		ready: make(chan struct{}),
	}
	close(idx.ready)
	return idx, nil
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

		fmt.Fprintf(log, "2/4 Analyzing fields + JSONB (concurrent)...\n")
		g, gctx := errgroup.WithContext(ctx)
		g.Go(func() error {
			return analyze.AnalyzeFields(gctx, pg, ext, store, schemas)
		})
		g.Go(func() error {
			return analyze.AnalyzeJSONB(gctx, pg, ext, store)
		})
		if err := g.Wait(); err != nil {
			store.Close()
			idx.err = fmt.Errorf("analysis: %w", err)
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
//	text := result.Matched().Text()         // only matched tables
//	text := result.All().Text()             // all tables including FK-expanded
//	text := result.Include("reviews").Text() // specific tables
func (idx *Index) Query(query string) (*ResultSet, error) {
	<-idx.ready
	if idx.err != nil {
		return nil, idx.err
	}
	idx.mu.RLock()
	defer idx.mu.RUnlock()

	raw, err := search.Query(idx.store, query)
	if err != nil {
		return nil, err
	}
	return newResultSet(raw), nil
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

// --- Types ---

// ResultSet holds the results of a query and provides methods to select
// subsets of matched tables and render them as compact text.
//
// The typical flow is:
//
//	result, _ := idx.Query("failed reviews")
//	text := result.Matched().Text()  // compact schema of matched tables only
type ResultSet struct {
	// Query is the original query string.
	Query string `json:"query"`
	// Tables contains all tables in the result, including both directly
	// matched tables (score > 0) and FK-expanded tables (score = 0).
	Tables []TableContext `json:"tables"`
}

// Matched returns a [Selection] containing only tables with a match score > 0.
// These are the tables most relevant to the query.
func (rs *ResultSet) Matched() *Selection {
	names := make([]string, 0)
	for _, t := range rs.Tables {
		if t.MatchScore > 0 {
			names = append(names, t.TableName)
		}
	}
	return &Selection{rs: rs, names: names}
}

// All returns a [Selection] containing all tables in the result set,
// including FK-expanded tables that were not directly matched.
func (rs *ResultSet) All() *Selection {
	names := make([]string, 0, len(rs.Tables))
	for _, t := range rs.Tables {
		names = append(names, t.TableName)
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
	ForeignKeys []FKInfo      `json:"foreign_keys"`
	IsMatch     bool         `json:"is_match"`
	MatchScore  float64      `json:"match_score"`
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
	return r
}

func convertTableContext(t search.TableContext) TableContext {
	tc := TableContext{
		TableName:  t.TableName,
		Schema:     t.Schema,
		IsMatch:    t.IsMatch,
		MatchScore: t.MatchScore,
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
	return `--- notation ---
PK: primary key           col → table  foreign key
^  is primary key         ?  nullable   >target  FK target
[state] state-like categorical (< 100 distinct values)
[cat]   categorical field
{a, b, c}  representative values (from pg_stats)
$.path  type  {samples}  JSONB path with inferred type
(score: X.XX)  relevance score from query matching
`
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

		fmt.Fprintf(buf, "  %s %s", c.Name, c.Type)
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
