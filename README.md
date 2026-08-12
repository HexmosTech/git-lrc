# dbctx

[![Go Reference](https://pkg.go.dev/badge/github.com/shrsv/dbctx.svg)](https://pkg.go.dev/github.com/shrsv/dbctx)
[![License: MIT](https://img.shields.io/badge/License-MIT-blue.svg)](LICENSE)

### Compile a PostgreSQL database into compact, queryable context.

**dbctx** is a Go library and CLI tool that compiles a PostgreSQL database into a portable, queryable context index (`.dtx` file). It extracts schema, relationships, field semantics, representative values, JSONB structure, and builds a full-text search index — all from deterministic introspection, statistics, and heuristics, with no generative LLM and no external services required for the core index. It also supports an optional local semantic embedding signal and an optional, user-controlled terminology dictionary — both additive, both off-by-default-cost, described below.

Use it to give text-to-SQL systems, AI agents, and database-aware applications a compact, relevant slice of your database schema at query time, instead of dumping the entire `information_schema` into every prompt.

**Key features:**

- **Natural-language query** — find relevant tables, columns, and relationships from a text question
- **Semantic retrieval (optional, on by default)** — a local embedding model (BGE-small-en-v1.5, ~33M params, runs on CPU) recovers paraphrases lexical matching structurally can't, e.g. `"buyers"` → `customers`, `"purchases"` → `orders` — fused with, never replacing, exact/fuzzy lexical matching
- **Terminology dictionary (optional, user-controlled)** — map domain abbreviations/jargon (`"MRR"`, `"LOC"`) to exact schema objects via a generated LLM prompt + reviewed import; independent of both the lexical and semantic signals
- **JSONB intelligence** — discover paths, types, and representative values inside JSONB columns
- **State/categorical detection** — identify implicit enums (status, plan, role) with their values
- **FK expansion** — automatically include related tables through foreign-key graph traversal
- **Compact text output** — LLM-ready schema notation with notation legend, optimized for token budget
- **Portable .dtx format** — SQLite-based, ship/cache/version/inspect it like any file
- **No generative LLM required** — the core index and all retrieval logic are deterministic; the optional embedding model is a small local encoder, not a chat/completion model, and terminology generation happens outside dbctx entirely (you paste a prompt into whatever LLM you already use)
- **Go library API** — embed directly in your Go application, no subprocess needed
- **In-memory mode** — ephemeral indexes for testing or ephemeral workloads
- **Web UI** — built-in browser-based database explorer

It is designed to be the missing layer between a real database and systems that need to understand it:

```text
PostgreSQL
    │
    ▼
  dbctx
    │
    ▼
 database.dtx
    │
    ├── schema
    ├── relationships
    ├── field intelligence
    ├── representative values
    ├── JSONB structure
    ├── retrieval index (lexical)
    ├── semantic index (optional)
    └── terminology (optional, user-supplied)
    │
    ▼
Text query
    │
    ▼
Relevant tables + compact schema + field context
    │
    ▼
Text-to-SQL / visualization / analytics system
```

---

**Jump to:**

| I want to... | Go to |
|---|---|
| Understand what dbctx does and why it exists | [Why does this need to exist?](#why-does-this-need-to-exist) |
| See a quick demo | [Quick look](#quick-look) |
| See how fast it is | [Performance](#performance) |
| Use it from the command line | [CLI quick start](#1-build-an-index) |
| Browse the database in a web UI | [Web UI](#3-explore-in-the-ui) |
| Use it as a Go library in my app | [Library Usage](#library-usage) |
| Query with natural language and get compact schema | [Querying the context](#querying-the-context) |
| Understand semantic (embedding-based) retrieval | [Semantic retrieval](#semantic-retrieval) |
| Map domain jargon/abbreviations to schema objects | [Terminology](#terminology) |
| Understand the `.dtx` file format | [The `.dtx` format](#the-dtx-format) |
| See what dbctx understands about a database | [What dbctx understands](#what-dbctx-understands) |
| Understand how it works under the hood | [Architecture](#architecture) / [The retrieval model](#the-retrieval-model) |
| See intended use cases (text-to-SQL, agents, etc.) | [Intended use cases](#intended-use-cases) |
| Understand the design decisions | [Design principles](#design-principles) |
| See the project roadmap | [Project status](#project-status) / [Roadmap](#roadmap) |
| Contribute or extend it | [Contributing](#contributing) |

---

## Quick look

### 1. Build an index

```bash
dbctx build postgres://user:pass@localhost/mydb --output mydb.dtx
```

By default this also builds a local semantic (embedding) index — downloading the model to a local cache on first use (see [Semantic retrieval](#semantic-retrieval)). Skip it with `--no-semantic` if you only want the deterministic lexical index:

```bash
dbctx build postgres://user:pass@localhost/mydb --output mydb.dtx --no-semantic
```

*(screenshot coming soon)*

---

### 2. Query from the CLI

```bash
dbctx query mydb.dtx "How many failed GitHub reviews last month?"
```

The query finds relevant tables, surfaces JSONB structure, and highlights state-like fields — all in compact text output. By default this includes tables pulled in via foreign-key expansion alongside direct hits, since that's the join context an LLM needs to actually answer the query; pass `--matched-only` to restrict output to tables that scored a direct match.

![dbctx CLI query output](media/dbctx-3-query.png)

---

### 3. Explore in the UI

```bash
dbctx ui mydb.dtx
```

A local web interface for browsing everything dbctx extracted from your database.

<table>
<tr>
<td align="center"><b>Overview</b><br><img src="media/dbctx-1-general-stats.png" width="420"></td>
<td align="center"><b>Table details</b><br><img src="media/dbctx-4-table-details.png" width="420"></td>
</tr>
<tr>
<td align="center"><b>JSONB expansion</b><br><img src="media/dbctx-6-jsonb-expansion.png" width="420"></td>
<td align="center"><b>State &amp; categorical values</b><br><img src="media/dbctx-7-state-values.png" width="420"></td>
</tr>
<tr>
<td align="center" colspan="2"><b>Query interface</b><br><img src="media/dbctx-5-query-ui.png" width="860"></td>
</tr>
</table>

---

## Performance

Real-world numbers against a production PostgreSQL database with **60 tables, 758 columns, 97 foreign keys**, and **677 JSONB paths**.

### Full build

```text
Phase              Duration     Share
──────────────────────────────────────────
Connect              0.1ms      0.0%
Schema               2.5s      20.2%
Store                11ms      0.1%
Fields               3.2s      26.2%
JSONB                6.5s      53.1%  (4 workers, connection pool)
FTS                  49ms      0.4%
──────────────────────────────────────────
Total               ~12s          100%
```

JSONB analysis uses a connection pool (`pgxpool`, 4 connections) and a worker pool (4 goroutines) for parallel PostgreSQL queries. SQLite writes are batched in transactions.

The `.dtx` file is **448 KB** for this database.

### Query performance

```text
Query                          Duration    Matched    Text render
──────────────────────────────────────────────────────────────────
"id"                              138ms     11 tables      270µs
"reviews"                          76ms      7 tables       47µs
"failed reviews last month"       105ms     11 tables      615µs
"revews" (fuzzy)                   81ms      6 tables       90µs
"nonexistent_xyz" (no match)        2ms      0 tables        2µs
```

### Library benchmarks (in-memory, 4-table fixture, 3-run average)

```text
BenchmarkQuery_Short         ~816 µs/op     38 KB/op
BenchmarkQuery_Medium        ~854 µs/op     41 KB/op
BenchmarkQuery_Fuzzy         ~660 µs/op     38 KB/op
BenchmarkMatchedText         ~4.4 µs/op    3.4 KB/op
BenchmarkMatchedTextRaw      ~3.9 µs/op    2.4 KB/op
BenchmarkAllText             ~7.8 µs/op    5.9 KB/op
BenchmarkReport              ~378 µs/op     14 KB/op
BenchmarkTables               ~28 µs/op    1.7 KB/op
BenchmarkTableDetail         ~147 µs/op     11 KB/op
BenchmarkStats                ~33 µs/op    3.2 KB/op
```

### Semantic retrieval benchmarks

Measured on a synthetic 50-table schema (`internal/testutil.NewLargeStore`) — dbctx's own retrieval design targets 50+ table databases, so the 4-table fixture above isn't representative of semantic/hybrid overhead at realistic scale. Retrieval-side numbers (build, score, query fusion) use a deterministic fake embedder to isolate this package's own cost from model inference; model-inference numbers are measured separately against the real ONNX backend. All numbers are CPU-only, single machine, no GPU.

```text
Retrieval overhead (internal/semantic, 50 tables, ~90 embedded objects)
─────────────────────────────────────────────────────────────────────
BenchmarkQuery_Large_LexicalOnly                    ~7.0 ms/op    321 KB/op
BenchmarkQuery_Large_Hybrid                         ~7.6 ms/op    425 KB/op   (+9% over lexical-only)
BenchmarkScorer_Score_Large (semantic score only)   ~0.32 ms/op    96 KB/op
BenchmarkBuildIndex_Large (full rebuild)            ~16.5 ms/op   728 KB/op
BenchmarkBuildIndex_Large_Incremental (no changes)  ~11.6 ms/op   585 KB/op   (diff-only, no re-embedding)
BenchmarkOpenAndQuery_WithSemanticIndex_FileBacked  ~12.1 ms/op   431 KB/op   (file-backed .dtx reopen + query)

Real BGE-small-en-v1.5 model inference (internal/embed, onnxruntime, CPU)
─────────────────────────────────────────────────────────────────────
BenchmarkOnnxEmbedder_ColdInit (session load, 133 MB model)   ~240 ms/op
BenchmarkOnnxEmbedder_EmbedQuery (1 text)                       ~16 ms/op
BenchmarkOnnxEmbedder_EmbedPassages_Single (1 text)             ~17 ms/op
BenchmarkOnnxEmbedder_EmbedPassages_Batch16 (16 texts)          ~44 ms/op   (~2.7 ms/text — batching matters)

End-to-end with the real model, 50 tables / ~90 embedded objects
─────────────────────────────────────────────────────────────────────
BenchmarkBuildIndex_Large_RealModel      ~1.36 s/op    (full semantic build; embedder already warm)
BenchmarkQuery_Large_RealModel_Hybrid    ~36-43 ms/op  (embedder already warm)
```

Takeaways:

* **Hybrid query overhead over lexical-only is small when the embedder is already warm** (~9% in the fake-embedder isolation benchmark) — the brute-force cosine scan itself is cheap. With the **real** model warm, a hybrid query costs ~36-43ms at 50-table scale vs. ~7ms lexical-only — the difference is almost entirely the ~16ms `EmbedQuery` call plus request/allocation overhead, not the cosine scan.
* **Model session load (~240ms) is the real one-time cost**, paid once per **process**, lazily on first semantic query. A long-running library process (or the `dbctx ui` server) pays this once and every later query is fast. **A `dbctx query` CLI invocation is a fresh process each time**, so it pays the ~240ms load on every single call — wall-clock for one CLI query against a small `.dtx` measured ~400ms total, dominated by that cold load, not by search itself. If you're issuing many CLI queries in a loop, prefer the library API (or a long-lived process) over shelling out repeatedly.
* **Build-time embedding cost is the real number to budget for**: ~1.36s for a 50-table schema (~90 embedded objects) with the model already warm — scales roughly linearly with how many table/column/JSONB-path objects end up embedded, not total table count. `dbctx build` logs the embedded/reused/removed counts so you can see this per-database.
* **Incremental rebuilds skip re-embedding entirely** when nothing changed — the ~11.6ms "incremental" cost (fake-embedder benchmark) is schema diffing (re-deriving candidate text and hashing it), not model inference. Re-running a build against an unchanged schema costs essentially nothing extra for the semantic phase.
* Batch embedding during a build is meaningfully more efficient per-object than one-at-a-time (~2.7ms/text batched vs. ~17ms/text unbatched) — `internal/embed` batches automatically.

Key takeaways:

* **Full build** completes in **~12 seconds** for a real 60-table database *(lexical index only — see below for the added cost of `--semantic`, on by default)*
* **Query + text rendering** completes in **~100ms** — fast enough for interactive use *(lexical-only; add the embedder's one-time load if semantic search is enabled — see below)*
* **Text rendering** itself is sub-millisecond — the FTS query dominates latency
* **Fuzzy search** adds negligible overhead over exact match
* The resulting `.dtx` is **448 KB** — small enough to ship, cache, or embed *(a semantic-enabled `.dtx` is larger — each embedded object stores a 384×4-byte vector, so ~1.5KB per table/column/JSONB-path object embedded, on top of the base file)*

---

# Library Usage

dbctx is a Go library that can be imported directly into your application. This is the intended integration path for text-to-SQL systems, AI agents, analytics tools, and database-aware applications.

## Install

```bash
go get github.com/shrsv/dbctx
```

## Basic usage

```go
package main

import (
    "context"
    "fmt"
    "log"

    "github.com/shrsv/dbctx"
)

func main() {
    ctx := context.Background()

    // Build an in-memory index (no file created)
    idx, err := dbctx.Build(ctx, "postgres://localhost/mydb", nil)
    if err != nil {
        log.Fatal(err)
    }
    defer idx.Close()

    // Query with natural language
    result, err := idx.Query("failed reviews last month")
    if err != nil {
        log.Fatal(err)
    }

    // Get compact schema for everything the query needs — matched tables
    // plus FK-expanded join context — ready for an LLM prompt
    fmt.Println(result.Matched().Text())         // includes notation legend

    // Or refine the selection
    fmt.Println(result.ScoredOnly().Text())                      // only tables that scored directly
    fmt.Println(result.Include("reviews", "orgs").Text())        // specific tables
    fmt.Println(result.Matched().Exclude("migrations").Text())   // matched minus one

    // Use TextRaw() to omit the legend (tighter token budget)
    fmt.Println(result.Matched().TextRaw())
}
```

The `Text()` output is a compact, LLM-ready representation with a notation legend:

```text
--- notation ---
PK: primary key           col → table  foreign key
^  is primary key         ?  nullable   >target  FK target
[state] state-like categorical (< 100 distinct values)
[cat]   categorical field
{a, b, c}  representative values (from pg_stats)
$.path  type  {samples}  JSONB path with inferred type
(score: X.XX)  relevance score from query matching

reviews  (score: 15.24)
  PK: id
  org_id → orgs
  pull_request_id → pull_requests
  status character varying(50) [state]
    {completed, failed, created, in_progress}
  metadata jsonb
    $.provider  string  {github, gitlab}
  created_at timestamp with time zone

orgs  (score: 3.12)
  PK: id
  name text
  plan text [state]
    {free, pro, enterprise}
```

## Persist to a .dtx file

```go
// Build and save to disk
idx, err := dbctx.Build(ctx, dsn, &dbctx.Options{
    Path:    "mydb.dtx",
    Schemas: "public,app",
})

// Later: open the existing file (read-only, no PostgreSQL needed)
idx, err := dbctx.Open("mydb.dtx")
```

By default, `Build` also constructs a local semantic embedding index (downloading the model to a local cache on first use — see [Semantic retrieval](#semantic-retrieval)). Set `Options.NoSemantic` to skip it if you only want the deterministic lexical index, or if you want to avoid the model download/CGO dependency entirely:

```go
idx, err := dbctx.Build(ctx, dsn, &dbctx.Options{
    Path:       "mydb.dtx",
    NoSemantic: true, // lexical/fuzzy retrieval only, no embedding model
})
```

If semantic indexing is enabled but the model or its runtime can't be obtained (offline, unsupported platform), `Build` logs a warning and continues with a lexical-only index rather than failing — it's always best-effort. `idx.Query` degrades the same way at query time if a semantic index exists on disk but the model can't be loaded when the index is later opened.

## In-memory mode

When `Options.Path` is empty (or opts is nil), the index lives in memory only. No files are created. This is useful for:

* ephemeral indexes rebuilt on each startup
* testing
* environments where file I/O is undesirable

```go
idx, _ := dbctx.Build(ctx, dsn, nil) // in-memory, no .dtx file
```

In-memory indexes are faster to build (no disk I/O) but must be rebuilt each time the process starts.

## Non-blocking startup

For applications that need database context available at startup without blocking the main thread, use `BuildAsync`. It starts the build in a background goroutine and returns immediately. Queries made before the build completes will block until the index is ready.

This pattern is useful for binary startup where you want to begin serving requests immediately while the index builds in the background:

```go
func main() {
    ctx := context.Background()

    // Start building in background — returns immediately
    idx, ready, err := dbctx.BuildAsync(ctx, dsn, nil)
    if err != nil {
        log.Fatal(err)
    }
    defer idx.Close()

    // Register idx with your application server, handlers, etc.
    // The index is safe to pass around even before the build completes.

    // Start serving HTTP immediately
    http.HandleFunc("/query", func(w http.ResponseWriter, r *http.Request) {
        // This call blocks automatically if the index isn't ready yet
        result, err := idx.Query(r.URL.Query().Get("q"))
        if err != nil {
            http.Error(w, err.Error(), 500)
            return
        }
        // Return compact text of matched tables
        w.Header().Set("Content-Type", "text/plain")
        w.Write([]byte(result.Matched().Text()))
    })

    // Log when the index is ready
    go func() {
        <-ready
        log.Println("dbctx index is ready")
    }()

    log.Println("server starting on :8080 (index building in background)")
    http.ListenAndServe(":8080", nil)
}
```

For a non-blocking readiness check instead of waiting:

```go
select {
case <-idx.Ready():
    // index is ready, serve with full context
    result, _ := idx.Query(query)
default:
    // index still building, return a fallback response
    w.Write([]byte("database context is loading, please retry"))
}
```

If the background build fails, `idx.Err()` returns the error and all query methods will return it.

## Available methods

```go
// Query — returns a ResultSet for selection and text rendering
result, _ := idx.Query("failed reviews last month")

// ResultSet — select tables and render (includes notation legend)
result.Matched().Text()                          // matched + FK-expanded tables (default: everything needed)
result.ScoredOnly().Text()                       // only tables that scored a direct match
result.Include("reviews", "orgs").Text()         // specific tables by name
result.Matched().Exclude("migrations").Text()    // matched minus exclusions
result.Matched().Include("extra").Text()         // matched plus extras
result.Matched().TextRaw()                       // same as Text() but without the legend
result.Matched().Tables()                        // get []TableContext for custom logic
result.Matched().Len()                           // count of selected tables
result.TableMap()                                // map[string]TableContext for lookup

// Tables — list all tables with summary info
tables, _ := idx.Tables()

// TableDetail — full column/relationship/value detail for one table
detail, _ := idx.TableDetail("reviews")

// Stats — summary counts (tables, columns, FKs, state fields, etc.)
stats, _ := idx.Stats()

// Report — dump human-readable report to a writer
idx.Report(os.Stdout)

// Ready — channel that closes when the index is ready
<-idx.Ready()

// Err — returns build error (for async builds)
if err := idx.Err(); err != nil { ... }

// TerminologyPrompt — generate a self-contained prompt for an external LLM
// to derive a terminology dictionary from this schema (see Terminology below)
prompt, _ := idx.TerminologyPrompt()

// ImportTerminology — three ways to supply the same JSON-shaped data,
// pick whichever fits how the data actually arrives in your program:
result, _ := idx.ImportTerminology(jsonBytes)              // []byte (a JSON string works too: []byte(s))
result, _ := idx.ImportTerminologyFile("terminology.json") // read from disk
result, _ := idx.ImportTerminologyGroups([]dbctx.TerminologyGroup{ // Go values, no JSON round-trip
    {Term: "loc", Aliases: []string{"lines of code"}, Targets: []string{"metrics.loc"}},
})

// Terminology — inspect what's currently imported
entries, _ := idx.Terminology()

// Close — release resources
idx.Close()
```

## Full API reference

See the [pkg.go.dev documentation](https://pkg.go.dev/github.com/shrsv/dbctx) or run `go doc github.com/shrsv/dbctx` locally.

---

## Why does this need to exist?

When building a system that lets users ask questions about a database in natural language, the first problem is usually presented as:

> "How do I generate SQL from a user's question?"

That is often the wrong first problem.

The harder problem is:

> **How do I efficiently tell the model what this database actually contains?**

A real PostgreSQL database isn't just:

```sql
users(id, name, email)
orders(id, user_id, status, created_at)
```

It contains information that is critical for understanding queries but is absent from a conventional schema dump.

You quickly run into questions like:

### How do I get a highly compressed schema?

Given a database with hundreds of tables and thousands of fields, how do I give a downstream system only the relevant 10–20 tables without dumping the entire `information_schema` into a prompt?

### How do I discover the possible states of a field?

If I have:

```sql
reviews.status TEXT
```

how do I discover that the meaningful values are:

```text
pending
running
completed
failed
```

without manually documenting every field?

### How do I understand JSONB?

If I have:

```sql
reviews.metadata JSONB
```

how do I discover that it actually contains:

```text
provider
repository.name
repository.owner
severity
automated
```

and that `provider` is usually one of:

```text
github
gitlab
bitbucket
```

### How do I find the right tables for a question?

Given:

> "How many failed GitHub reviews did we have last month?"

how do I identify that the relevant tables are probably:

```text
reviews
repositories
```

rather than sending the entire database schema to an LLM?

### How do I expand the result intelligently?

If a query matches `reviews`, how do I automatically include related tables through foreign keys?

### And how do I do all of this without an LLM?

That is the purpose of **dbctx**.

---

# What is dbctx?

`dbctx` is a **database context compiler and index**.

It connects to PostgreSQL and builds a persistent `.dtx` file containing a compact representation of the database that is useful for downstream systems.

It captures both **structural facts** and **derived observations**.

```text
Structural
────────────────────────────
tables
columns
types
primary keys
foreign keys
indexes
relationships

Derived
────────────────────────────
categorical fields
state-like fields
representative values
value frequencies
JSONB paths
JSONB types
JSONB representative values
field characteristics

Retrieval
────────────────────────────
table matching
field matching
value matching
foreign-key expansion
relevant-context extraction
semantic (embedding) matching   [optional]
terminology matching            [optional, user-supplied]
```

The result is not SQL.

It is **context from which SQL can be generated reliably and cheaply**.

---

# The `.dtx` format

The most important artifact produced by dbctx is the `.dtx` file.

`.dtx` stands for **DB Context**.

Instead of treating the database context as an ephemeral prompt assembled every time a query arrives, dbctx makes it a persistent artifact:

```text
production.dtx
```

Conceptually:

```text
PostgreSQL
     │
     │ introspection + observation
     ▼
production.dtx
```

Then:

```text
production.dtx + user query
              │
              ▼
       relevant context
              │
              ▼
        SQL generator
```

This separation is intentional.

The database can be scanned and analyzed once. Query-time systems can then retrieve only the information they need.

### Compatibility

`.dtx` is a SQLite file, and semantic/terminology support was added as new tables (`semantic_objects`, `terminology`), not a format break:

* A `.dtx` file built before semantic/terminology support existed opens and queries fine on a newer dbctx — it simply has no semantic index and no terminology, and retrieval falls back to exactly the lexical behavior it always had.
* A `.dtx` built with `--no-semantic` behaves the same way.
* `dbctx terminology import` can add terminology to a `.dtx` file that predates the feature — it creates the table on demand rather than requiring a rebuild.
* Persisted embeddings record the exact model identity and dimensionality they were built with; an incompatible or mismatched embedder is rejected cleanly (falls back to lexical-only) rather than producing meaningless cosine similarities.

---

# Example

Suppose PostgreSQL contains:

```sql
reviews (
    id,
    repository_id,
    status,
    created_at,
    metadata JSONB
)

repositories (
    id,
    organization_id,
    provider
)

organizations (
    id,
    name,
    plan
)
```

A conventional schema extractor might give you:

```text
reviews.metadata JSONB
```

That's technically correct but not particularly useful.

dbctx can derive a richer representation:

```text
reviews
  id                uuid       PK
  repository_id     uuid       → repositories.id
  status            text       state
  created_at        timestamptz
  metadata          jsonb

reviews.status
  values:
    pending
    running
    completed
    failed

reviews.metadata
  provider          string
    values: github, gitlab, bitbucket

  severity          string
    values: low, medium, high, critical

  repository.name   string
  repository.owner  string
  automated         boolean

repositories
  id                uuid       PK
  organization_id   uuid       → organizations.id
  provider          text

organizations
  id                uuid       PK
  name              text
  plan              text
    values: free, pro, enterprise
```

This is much closer to what an AI system actually needs to understand the database.

---

# Querying the context

Now give dbctx a textual query:

```text
How many failed GitHub reviews did we have last month?
```

dbctx can identify candidate tables through fuzzy matching and database structure:

```text
reviews          score: high
repositories     score: high
organizations    score: low
```

Then foreign-key expansion gives:

```text
reviews
  └── repositories
        └── organizations
```

The resulting context might contain only:

```text
reviews(
  id,
  repository_id → repositories.id,
  status,
  created_at,
  metadata
)

reviews.status
  {pending, running, completed, failed}

reviews.metadata.provider
  {github, gitlab, bitbucket}

repositories(
  id,
  organization_id → organizations.id,
  provider
)

repositories.provider
  {github, gitlab, bitbucket}
```

That compact context can then be passed to whatever generates SQL.

---

# Semantic retrieval

Lexical/fuzzy matching is precise but has a hard ceiling: it can only find what shares vocabulary (or near-vocabulary, via typo tolerance) with your schema. It cannot find `customers` from the query `"buyers"` — there's no lexical relationship between those two strings at all.

dbctx addresses this with an **optional, local, embedding-based retrieval signal** — never a replacement for lexical/fuzzy matching, an additional signal fused into the same ranking:

```text
query
  │
  ├── lexical/fuzzy retrieval (FTS, fuzzy match, value match, terminology)
  │
  └── semantic retrieval (embedding cosine similarity)
          │
          ▼
      weighted fusion
          │
          ▼
   existing ranking / FK expansion / compact context
```

It's on by default (`dbctx build`), backed by **BGE-small-en-v1.5** (384-dim, ~33M parameters) running locally via the ONNX Runtime — no API key, no external inference server, no vector database. Skip it with `--no-semantic` (or `Options.NoSemantic` in the library) if you only want the original deterministic index.

### What gets embedded

Not raw table names — dbctx builds a compact, natural-language-ish text blurb per schema object from information it already extracted, and embeds that:

* **Tables**: name, column names, related tables (via FK), a sample of observed state/categorical values
* **Meaningful columns**: state-like, categorical, or foreign-key columns only (not every column — this keeps the embedded corpus small and low-noise). A column named `total` with no state/categorical/FK signal is still covered by its table's text, just not embedded on its own.
* **JSONB paths**: only paths with actual observed sample values (e.g. `reviews.metadata.provider` with `{github, gitlab, bitbucket}`), capped per table

At query time, dbctx embeds the query and scores it against every embedded object with **brute-force cosine similarity** — no ANN index. dbctx's expected corpus size (one database's worth of tables/columns/JSONB paths) makes this the right tradeoff: it's simpler to reason about, has no index-quality tuning surface, and is fast enough in practice (see [benchmarks](#semantic-retrieval-benchmarks)) that adding HNSW or similar would be solving a problem dbctx doesn't have.

### How the score is fused

Exact identifiers stay powerful. Querying `"orders"` should strongly favor the `orders` table even if some other table is semantically related — semantic retrieval exists to recover **recall** where lexical search has none, not to outrank a direct name match. dbctx uses weighted, normalized fusion (not reciprocal rank fusion — see `internal/search.FuseScores` for the reasoning) that scales the semantic contribution by the strongest lexical score already found:

```text
final = lexical
      + semantic_weight * semantic_score(0..1) * strongest_lexical_score_in_this_query
```

If lexical search found nothing at all for a query (e.g. `"buyers"` against a schema with only `customers`), the scale falls back to a flat 1.0 — enough for a purely-semantic match to surface, just never enough to bury a real lexical match when one exists.

Query results are inspectable, not a black box: `ResultSet.SemanticHits` (library) / the `SEMANTIC SIGNAL` section (`dbctx query` output) show exactly which embedded object and similarity score contributed to each table that lexical search alone wouldn't have surfaced.

### Model & distribution

* **Model**: [BAAI/bge-small-en-v1.5](https://huggingface.co/BAAI/bge-small-en-v1.5) via its ONNX export, CLS-token pooling + L2 normalization, BGE's documented query-instruction prefix for retrieval
* **Runtime**: [onnxruntime](https://onnxruntime.ai/) via CGO (dynamically loaded, not statically linked)
* **Tokenizer**: a pure-Go WordPiece implementation (`internal/embed`), no CGO, matching bert-base-uncased's vocabulary
* **Distribution**: nothing is embedded in the dbctx binary. The model (~133MB) and the platform onnxruntime shared library (~10-80MB depending on platform) are downloaded once to `~/.dbctx` (same fixed location on Linux, macOS, and Windows — override with `DBCTX_CACHE_DIR`) on first semantic build or query — never during ordinary lexical-only operation. Both are pinned by exact version and verified by SHA-256 on download.

This is the one place dbctx isn't CGO-free — `onnxruntime_go` requires CGO to compile (though it dlopens the actual runtime library at runtime, so no link-time dependency). It was a deliberate tradeoff for numerical correctness and maintenance burden over a from-scratch pure-Go transformer implementation; see the design note in `internal/embed`.

### Storage & compatibility

Embeddings live in a `.dtx` file exactly like everything else — new SQLite tables (`semantic_objects`, plus terminology's own tables), added additively. Opening an older `.dtx` file that predates semantic support works unchanged; it simply has no semantic index, and dbctx falls back to lexical-only automatically. Vectors are stored as raw little-endian `float32` BLOBs (not JSON) alongside a recorded model identity and dimensionality, so a mismatched or incompatible embedder is rejected cleanly rather than corrupting cosine similarity silently. Rebuilding is incremental: unchanged schema objects are never re-embedded (matched by a content hash of their derived text), and objects for now-dropped tables/columns are pruned.

---

# Terminology

Semantic embeddings are good at general paraphrases. They are not reliable for domain-specific abbreviations and jargon a generic model has never seen used the way *your* organization uses them — `LOC` for "lines of code", `MRR` for "monthly recurring revenue", an internal nickname for a metric. That gap is what dbctx's **terminology** layer is for.

Terminology is a third, fully independent retrieval signal — separate from both lexical and semantic matching, and never populated automatically:

```text
                     .dtx
                      │
                      ▼
             terminology prompt
                      │
                      ▼
          large external LLM + you
                      │
                      ▼
            terminology.json
                      │
                      ▼
             terminology import
                      │
                      ▼
                     .dtx
```

dbctx never calls an LLM itself. Instead:

```bash
# 1. Generate a self-contained prompt (schema + instructions) to stdout
dbctx terminology prompt mydb.dtx > terminology-prompt.txt

# 2. Paste it into Claude/GPT/Gemini/whatever you use. Work through any
#    clarifying questions it asks about ambiguous terms, then save its
#    final JSON output.

# 3. Import it — every mapping is validated against the real schema;
#    invalid entries are rejected individually, not the whole batch.
dbctx terminology import mydb.dtx terminology.json

# Inspect what's currently imported
dbctx terminology list mydb.dtx
```

The prompt embeds the complete schema (reusing dbctx's existing full-detail report renderer — no second schema format), and instructs the model to: distinguish genuine domain terminology from ordinary synonyms lexical/semantic search already handle; identify abbreviations, acronyms, business terms, and internal jargon; ask you rather than guess when a mapping is ambiguous; and map every accepted term back to an *exact* schema object using dbctx's `table` / `table.column` / `table.column:$.json.path` notation.

The output format:

```json
[
  {
    "term": "loc",
    "aliases": ["line of code", "lines of code", "source lines of code"],
    "targets": ["metrics.loc"]
  }
]
```

Terminology is metadata used only by retrieval — importing a large dictionary does not bloat the compact schema output (`Text()`/`TextRaw()`) that gets sent to an LLM downstream; token budget stays exactly what it was without terminology.

---

# dbctx does not require a generative LLM

This is a deliberate design decision, and remains true even with semantic retrieval available.

The core index — schema, relationships, field statistics, JSONB structure, categorical/state detection, lexical retrieval — is built entirely from deterministic database introspection, statistics, and heuristics. It does **not** require:

* an OpenAI (or any) API key
* an inference server
* a generative/chat LLM
* a vector database (dbctx's optional embeddings use brute-force cosine similarity over its own small schema-object corpus, not a general-purpose vector database)

The **optional** semantic layer adds a small local encoder model (33M parameters, not a generative model) that runs entirely on your machine via the ONNX Runtime — no network calls at query time, no API key, ever. The **optional** terminology layer explicitly *does* involve an LLM, but that call happens entirely outside dbctx: you paste a generated prompt into whichever model you already use, and only reviewed, human-approved output ever comes back in.

The database context should be something you can build locally, inspect, diff, cache, ship, and reproduce.

For example:

```text
same database state
        +
same dbctx version
        +
same embedding model version (if semantic indexing is used)
        =
same context index
```

This makes the system substantially easier to reason about than an LLM-generated database description.

---

# What dbctx understands

## Tables and columns

dbctx extracts the PostgreSQL structure:

```text
table
column
PostgreSQL type
nullable
default
primary key
foreign key
indexes
```

It preserves the relationships between objects rather than flattening everything into text.

---

## Relationships

Foreign keys form a database graph:

```text
users
  │
  ├── organizations
  │      │
  │      └── subscriptions
  │
  └── reviews
         │
         └── repositories
```

This graph is useful both for retrieval and for generating useful context.

A textual match does not have to discover every relevant table independently.

If:

```text
reviews → repositories
```

and `reviews` is strongly matched, the related repository table can be expanded automatically.

---

# State and categorical fields

Many real-world databases contain implicit enums:

```text
status
state
stage
phase
type
kind
role
category
mode
```

even when the PostgreSQL type is merely:

```sql
TEXT
```

dbctx can use heuristics involving field names, cardinality, data types, value distributions, and observed values to identify likely categorical or state-like fields.

For example:

```text
deployment.status

state-like: true

values:
  pending
  building
  deployed
  failed
  cancelled
```

This information is particularly valuable for questions involving:

```text
failed
active
pending
cancelled
enterprise
premium
github
mobile
production
```

because those concepts often exist only as data values rather than schema declarations.

---

# JSONB intelligence

JSONB is one of the biggest reasons dbctx exists.

A conventional schema sees:

```text
metadata JSONB
```

dbctx attempts to understand what is actually inside it.

For example:

```text
metadata JSONB

$.provider
    string
    values: github, gitlab, bitbucket

$.repository
    object

$.repository.name
    string

$.repository.owner
    string

$.labels
    array

$.labels[].name
    string

$.automated
    boolean
```

The representation can include observations such as:

```text
path: $.provider
type: string
cardinality: 3
representative_values:
    github
    gitlab
    bitbucket
```

This gives downstream systems useful knowledge without requiring raw JSON documents to be inserted into every prompt.

---

# Representative values

dbctx is not intended to store a copy of your database.

Instead, it maintains compact observations about fields.

For a categorical field:

```text
status

distinct: 5

representative:
    pending
    running
    completed
    failed
    cancelled
```

For a high-cardinality field:

```text
email

type: text
distinct: ~1.2M
representative:
    alice@example.com
    bob@example.com
    ...
```

The exact observation strategy can vary by field type.

The goal is always:

> **retain enough information to understand the field without turning the context index into a copy of the database.**

---

# Incremental updates

The `.dtx` file is designed to be incrementally updated.

A database context should not need to be rebuilt from scratch every time the database changes.

Conceptually:

```text
database
   │
   ├── schema changed?
   │
   ├── values changed?
   │
   ├── JSONB structure changed?
   │
   └── statistics changed?
   │
   ▼
incremental update
   │
   ▼
database.dtx
```

This is particularly important for large production databases where:

* schemas evolve
* new enum-like values appear
* JSONB structures evolve
* tables grow continuously
* new relationships are added

The `.dtx` artifact retains the accumulated context and updates the pieces that need refreshing.

---

# The retrieval model

dbctx treats database understanding as a retrieval problem.

A query follows roughly this path:

```text
text query
    │
    ▼
lexical/fuzzy matching + optional semantic + optional terminology
    │
    ▼
candidate tables (fused score)
    │
    ▼
foreign-key expansion
    │
    ▼
relevant fields
    │
    ▼
state + categorical information
    │
    ▼
JSONB structure
    │
    ▼
compressed database context
```

Semantic and terminology, when present, are additional signals fused into the same per-table score before this expansion step — see [Semantic retrieval](#semantic-retrieval) and [Terminology](#terminology). The rest of the pipeline is unchanged.

This is intentionally separate from SQL generation.

dbctx answers:

> **"What part of this database does this question appear to be about?"**

A downstream system answers:

> **"What SQL should I write against it?"**

That separation is one of the central design principles of the project.

---

# Why not just send the whole schema to an LLM?

You can.

For small databases, it often works.

It becomes increasingly unattractive as databases grow.

Imagine:

```text
500 tables
6,000 columns
1,500 foreign keys
hundreds of JSONB fields
thousands of categorical values
```

Dumping all of that into every request is expensive and noisy.

More importantly, the model has to perform database retrieval and SQL generation simultaneously.

dbctx moves the first problem into a deterministic index:

```text
Database understanding
        ↓
     dbctx
        ↓
Relevant context
        ↓
    LLM / SQL
```

The downstream model gets a much smaller and more relevant representation.

---

# Why a file format?

Because database context is useful outside a single running process.

A `.dtx` file can potentially be:

* generated in CI
* cached locally
* checked into a repository
* versioned
* diffed
* inspected
* generated during deployment
* shared between services
* used by multiple AI applications
* regenerated incrementally

For example:

```text
schema.sql
database.dtx
```

can become part of an application's development and deployment artifacts.

The database itself remains the source of truth.

The `.dtx` file is its **compiled context representation**.

---

# Architecture

dbctx is deliberately small.

```text
┌────────────────────────────────────────────────────┐
│                       dbctx                        │
│                                                     │
│  PostgreSQL introspection                          │
│          │                                         │
│          ▼                                         │
│  Schema graph                                      │
│          │                                         │
│          ├── Field analysis                        │
│          ├── Value analysis                        │
│          └── JSONB analysis                        │
│                    │                                │
│                    ▼                                │
│              .dtx database context                  │
│                    │                                │
│      ┌─────────────┼─────────────┐                  │
│      ▼             ▼             ▼                  │
│   lexical       embeddings   terminology            │
│   index          (optional)   (optional,             │
│      │             │           user-supplied)        │
│      └─────────────┼─────────────┘                  │
│                    ▼                                │
│              query ranking                          │
│                    │                                │
│                    ▼                                │
│         FK expansion / compact context               │
└────────────────────────────────────────────────────┘
```

The core implementation is intended to be a **single binary**.

No database server.

No separate indexing service.

No external vector database.

Semantic retrieval is the one place this loosens slightly: it runs a local ONNX Runtime session in-process (dynamically loaded, not a subprocess or separate service) to produce embeddings. It's optional, it's still local, and its dependency (the model + runtime library) is downloaded to a cache — not embedded in the binary and not required unless you actually use semantic indexing/search. See [Semantic retrieval](#semantic-retrieval).

---

# Intended use cases

dbctx is intended to simplify building systems such as:

### Text → SQL

```text
"What was our revenue from enterprise customers last quarter?"
```

→ relevant tables + relationships + field context

→ SQL generation

---

### Text → Visualization

```text
"Show weekly failed deployments for the last six months."
```

→ relevant tables + state fields + time fields

→ SQL

→ chart

---

### Natural-language analytics

```text
"Which customers haven't used the product in 30 days?"
```

→ database context

→ SQL

→ answer

---

### AI agents

Agents frequently need to discover the structure of an application's database before performing an operation.

Instead of repeatedly introspecting PostgreSQL:

```text
agent
  ↓
dbctx
  ↓
relevant database context
```

---

### Database-aware developer tools

The same context can power:

* database explorers
* query assistants
* analytics interfaces
* admin panels
* debugging tools
* reporting systems
* BI applications

---

# Design principles

### 1. No generative LLM required

The core database context should be derived from observable facts and deterministic heuristics. Optional local embeddings augment retrieval without replacing this — they're a small encoder, not a generative model, and never required. Terminology explicitly does involve an LLM, but only outside dbctx, on your terms, with output you review before it's ever loaded back in.

### 2. Compact over exhaustive

The objective isn't to reproduce the database.

It is to preserve the information necessary to understand it.

### 3. Incremental by design

A growing database should not require a complete rebuild of its context.

### 4. PostgreSQL first

PostgreSQL has an exceptionally rich system catalog and strong type/relationship information.

dbctx starts there.

### 5. The format is a first-class artifact

The `.dtx` format should be useful independently of the binary that produces it.

### 6. Retrieval before generation

Finding the relevant database context is a separate problem from generating SQL.

### 7. Inspectable and reproducible

Engineers should be able to understand why a particular table or field appeared in a context result — including *why a semantic match appeared*: which embedded object and similarity score contributed, not just a fused number.

### 8. Semantic and terminology are additive signals, not replacements

Exact identifiers stay powerful, deterministic retrieval stays the foundation, and both optional layers exist purely to improve recall where the deterministic layer structurally can't reach — never to override it.

---

# Example workflow

Build an index:

```bash
dbctx build postgres://user:password@localhost/myapp \
    --output myapp.dtx
```

Update it:

```bash
dbctx update postgres://user:password@localhost/myapp \
    --index myapp.dtx
```

Query it:

```bash
dbctx query myapp.dtx \
    "How many failed GitHub reviews did we have last month?"
```

Potential output:

```text
TABLES

reviews              0.97
repositories         0.91

RELATIONSHIPS

reviews.repository_id
    → repositories.id

FIELDS

reviews.status
    state
    {pending, running, completed, failed}

reviews.created_at
    timestamptz

reviews.metadata
    jsonb

JSONB PATHS

reviews.metadata.provider
    string
    {github, gitlab, bitbucket}
```

A downstream application can then construct whatever prompt or query representation it wants.

---

# Web UI

dbctx includes a built-in web explorer for browsing the database context interactively.

```bash
dbctx ui myapp.dtx
```

This starts a local web server and opens the explorer in your browser.

The UI provides:

* **Overview** — summary statistics at a glance (tables, columns, relationships, state fields, JSONB paths)
* **Tables** — full table list with column counts, FK counts, and row estimates; click any table to explore
* **Table detail** — columns with types, PK/FK tags, nullable flags, distinct counts; expandable value lists for state-like and categorical fields; JSONB path trees; clickable FK relationships for navigation
* **Query** — natural language search against the context index; results ranked by relevance with collapsible detail sections for columns, values, relationships, and JSONB paths

The UI is styled after VS Code and is embedded in the binary itself — no external dependencies or build steps required.

```text
┌──────────────────────────────────────────────────┐
│  dbctx — Database Context Explorer               │
├──────────────────────────────────────────────────┤
│  Overview  │  Tables  │  Table  │  Query         │
├─────────┬────────────────────────────────────────┤
│ sidebar │  content area                          │
│         │                                        │
│ tables  │  stats / table detail / query results  │
│ list    │                                        │
│         │  • collapsible sections                │
│         │  • expandable value lists              │
│         │  • clickable FK navigation             │
│         │  • JSONB path trees                    │
└─────────┴────────────────────────────────────────┘
```


# What dbctx is not

dbctx is **not**:

* a text-to-SQL model
* a SQL execution engine
* a BI platform
* an LLM wrapper
* a general-purpose vector database (its optional embedding index is brute-force cosine similarity over its own small schema-object corpus — no ANN, no external service, not something meant to hold arbitrary vectors for other applications)
* a replacement for PostgreSQL's system catalog
* an attempt to infer arbitrary business logic
* a system that invents terminology on its own — its optional terminology dictionary only ever contains mappings a human reviewed and approved

It is the layer underneath those systems.

```text
             ┌──────────────────┐
             │  Visualization   │
             ├──────────────────┤
             │   Text → SQL     │
             ├──────────────────┤
             │      Agents      │
             └────────┬─────────┘
                      │
                 compact context
                      │
                ┌─────▼─────┐
                │   dbctx    │
                └─────┬─────┘
                      │
                 PostgreSQL
```

---

# Project status

dbctx is currently being developed.

The initial focus is:

* [x] PostgreSQL schema extraction
* [x] table and column graph
* [x] primary/foreign-key relationships
* [x] field statistics
* [x] categorical/state detection
* [x] representative values
* [x] JSONB structural inference
* [x] `.dtx` file format
* [ ] incremental updates
* [x] fuzzy table/field/value retrieval
* [x] foreign-key expansion
* [x] compact context export
* [ ] stable `.dtx` specification
* [x] web UI explorer
* [x] optional local semantic (embedding-based) retrieval
* [x] optional, user-controlled terminology dictionary

The ambition is to keep the core small enough that the entire system can remain understandable.

---

# Roadmap

### Phase 1 — Database understanding

Build the PostgreSQL introspection layer.

```text
tables
columns
types
PKs
FKs
indexes
```

### Phase 2 — Data understanding

Add deterministic field analysis:

```text
cardinality
distributions
representative values
categorical detection
state detection
```

### Phase 3 — JSONB

Build structural inference for JSONB:

```text
paths
types
arrays
objects
cardinality
representative values
```

### Phase 4 — `.dtx`

Define a stable, versioned database context format.

### Phase 5 — Retrieval

Implement:

```text
fuzzy matching
field matching
value matching
FK expansion
context ranking
```

### Phase 6 — Integration

Make it easy for applications to consume dbctx output for:

```text
text → SQL
text → charts
text → analytics
AI agents
database assistants
```

### Phase 7 — Semantic retrieval & terminology (done)

Add recall for paraphrased/domain-specific queries without giving up dbctx's deterministic, LLM-free core:

```text
local embedding model (optional, on by default)
brute-force cosine similarity retrieval signal
weighted fusion with lexical/fuzzy scoring
user-controlled terminology dictionary (optional)
terminology prompt generator + validated import
```

---

# The bigger idea

SQL generation is only one part of making databases accessible to natural language.

The system first needs to know:

```text
What tables exist?
What do they represent?
How are they related?
Which fields matter?
What values can those fields take?
What is hidden inside JSONB?
Which tables are relevant to this question?
```

Only then does SQL generation become interesting.

dbctx is an attempt to make that database understanding:

**deterministic, compact, incremental, portable, and reusable.**

```text
              PostgreSQL
                   │
                   ▼
             ┌──────────┐
             │  dbctx   │
             └────┬─────┘
                  │
               .dtx
                  │
        ┌─────────┼─────────┐
        ▼         ▼         ▼
      SQL       Charts    Agents
      │          │          │
      └──────────┴──────────┘
                 │
          Database-aware apps
```

**Build the context once. Use it everywhere.**

---

## Contributing

The most interesting parts of dbctx are likely to be the heuristics and the `.dtx` format itself.

Contributions around:

* PostgreSQL introspection
* efficient incremental indexing
* JSONB structural inference
* categorical/state detection
* compact representations
* retrieval algorithms
* `.dtx` format design

are especially welcome.

---

## License

MIT License. See [LICENSE](LICENSE) for details.

---

# See More

Your team's attention is limited. Spend review effort where **business risk is highest** — not spread evenly across every diff.

If dbctx is about giving AI systems compact, relevant context on your *database*, [**LiveReview**](https://hexmos.com/livereview) does the analogous thing for your *code changes*: instead of reviewing every diff with equal effort, it scores each change by blast radius — how far its impact reaches through your call graph — so review attention goes where it actually matters.

[![LiveReview — Blast-Radius Aware AI Code Review for Business-Critical Systems](media/livereview-banner.png)](https://hexmos.com/livereview)
