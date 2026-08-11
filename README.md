# dbctx

### Compile a PostgreSQL database into compact, queryable context.

**dbctx** builds a `.dtx` database context index from PostgreSQL: tables, relationships, field semantics, representative values, state-like fields, JSONB structure, and query-relevant metadata — without using an LLM.

```text
"What was our revenue from enterprise customers last quarter?"
        │
        ▼
   dbctx query
        │
        ▼
  relevant tables + compact schema + field context
        │
        ▼
  your text-to-SQL system
```

---

**Jump to:**

| I want to... | Go to |
|---|---|
| Understand what dbctx does and why it exists | [What is this?](#what-is-this) |
| See the output format | [What does the output look like?](#what-does-the-output-look-like) |
| Use it from the command line | [CLI](#cli) |
| Use it as a Go library | [Library](#library) |
| Browse the database in a web UI | [Web UI](#web-ui) |
| Understand how it works under the hood | [How it works](#how-it-works) |
| See what dbctx understands about a database | [What dbctx understands](#what-dbctx-understands) |
| Build a text-to-SQL system with it | [Integrating with your app](#integrating-with-your-app) |
| See the API reference | [API reference](#api-reference) |
| Contribute or extend it | [Contributing](#contributing) |

---

## What is this?

dbctx is a **database context compiler**. It reads your PostgreSQL database and produces a compact, queryable index that tells downstream systems — LLMs, text-to-SQL engines, analytics tools, AI agents — what the database actually contains.

A conventional schema dump gives you:

```sql
reviews.status TEXT
reviews.metadata JSONB
```

dbctx gives you:

```text
reviews.status  text  [state]
  {pending, running, completed, failed}

reviews.metadata  jsonb
  $.provider  string  {github, gitlab, bitbucket}
  $.repository.name  string
  $.automated  boolean
```

**No LLM required.** It uses PostgreSQL statistics, heuristics, and relationship analysis.

---

## What does the output look like?

Given a query like `"failed reviews last month"`, dbctx returns:

```text
reviews  (score: 15.24)
  PK: id
  org_id → orgs
  pull_request_id → pull_requests
  status character varying(50) [state]
    {completed, failed, created, in_progress}
  trigger_type character varying(50) [state]
    {cli_diff, manual, mcp}
  provider character varying(100) [cat]
    {cli, github, unknown, gitlab, gitea}
  metadata jsonb
    $.ai_summary_title  string
    $.preloaded_changes  array

orgs  (score: 3.12)
  PK: id
  name text
  plan text [state]
    {free, pro, enterprise}
```

This text is ready to paste into an LLM prompt or pass to a SQL generator.

---

## CLI

### Install

```bash
go install github.com/shrsv/dbctx/cmd/dbctx@latest
```

Or build from source:

```bash
git clone https://github.com/shrsv/dbctx.git
cd dbctx
make install
```

### Build an index

```bash
dbctx build postgres://user:pass@localhost/mydb --output mydb.dtx
```

Takes ~20 seconds for a 60-table database.

### Query

```bash
dbctx query mydb.dtx "How many failed GitHub reviews last month?"
```

![dbctx CLI query output](media/dbctx-3-query.png)

### Report

Dump everything dbctx extracted:

```bash
dbctx report mydb.dtx
```

---

## Library

dbctx is a Go library. The core use case: **query → select tables → get compact text for your LLM prompt.**

### Install

```bash
go get github.com/shrsv/dbctx
```

### Quick start

```go
import "github.com/shrsv/dbctx"

// Build index (in-memory, no file)
idx, _ := dbctx.Build(ctx, "postgres://localhost/mydb", nil)
defer idx.Close()

// Query
result, _ := idx.Query("failed reviews last month")

// Get compact schema for matched tables — ready for an LLM prompt
fmt.Println(result.Matched().Text())
```

### Selection API

```go
result.Matched().Text()                          // matched tables only
result.All().Text()                              // all tables including FK-expanded
result.Include("reviews", "orgs").Text()         // specific tables
result.Matched().Exclude("migrations").Text()    // matched minus one
result.Matched().Include("audit_log").Text()     // matched plus extras
result.Matched().Tables()                        // []TableContext for custom logic
```

### Persist to disk

```go
// Build and save
idx, _ := dbctx.Build(ctx, dsn, &dbctx.Options{Path: "mydb.dtx"})

// Open later (no PostgreSQL needed)
idx, _ := dbctx.Open("mydb.dtx")
```

### In-memory mode

When `Options.Path` is empty (or opts is nil), the index lives in memory. No files created. Faster to build, must be rebuilt each startup.

```go
idx, _ := dbctx.Build(ctx, dsn, nil) // in-memory
```

### Non-blocking startup

Start the build in a background goroutine. Queries made before completion block automatically:

```go
idx, ready, _ := dbctx.BuildAsync(ctx, dsn, nil)
defer idx.Close()

// Register idx with your app immediately...
// Queries block automatically until ready:
http.HandleFunc("/query", func(w http.ResponseWriter, r *http.Request) {
    result, _ := idx.Query(r.URL.Query().Get("q"))
    w.Write([]byte(result.Matched().Text()))
})

<-ready // or: select { case <-idx.Ready(): ... default: ... }
```

---

## Web UI

```bash
dbctx ui mydb.dtx
```

Opens a local web explorer (default port 7777). Styled after VS Code, embedded in the binary.

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

Features: collapsible result cards, select/copy tables, click-through FK navigation, JSONB path trees.

---

## How it works

```text
PostgreSQL
     │
     ▼
┌─────────────────────────────────────────┐
│  1. Schema extraction    (~1s)          │  tables, columns, types, PKs, FKs
│  2. Field analysis       (~2s)          │  pg_stats → categorical/state detection
│  3. JSONB analysis       (~18s)         │  TABLESAMPLE → paths, types, values
│  4. Search index         (~1s)          │  FTS5 + fuzzy matching
└─────────────────────────────────────────┘
     │
     ▼
  .dtx file (SQLite)  or  in-memory index
     │
     ▼
  query → fuzzy match + FTS + FK expansion → ranked tables → compact text
```

**No LLM. No API keys. No vector database.** Deterministic, reproducible, ship as a file.

---

## What dbctx understands

### State and categorical fields

Many databases have implicit enums stored as `TEXT`:

```text
reviews.status → {pending, running, completed, failed}
organizations.plan → {free, pro, enterprise}
deployments.trigger_type → {cli_diff, manual, mcp}
```

dbctx detects these from field names, cardinality, and `pg_stats` distributions — zero table scans.

### JSONB structure

```text
reviews.metadata JSONB

  $.provider          string    {github, gitlab, bitbucket}
  $.repository.name   string
  $.repository.owner  string
  $.labels[]          array
  $.labels[].name     string
  $.automated         boolean
```

Sampled via `TABLESAMPLE BERNOULLI`, capped at 10KB per value.

### Relationships

Foreign keys form a graph. Query `reviews` → auto-expand to `repositories` → `organizations`.

```text
reviews.repository_id → repositories.id
repositories.organization_id → organizations.id
```

---

## Integrating with your app

The typical text-to-SQL integration:

```go
// At startup (non-blocking)
idx, ready, _ := dbctx.BuildAsync(ctx, dsn, nil)

// At query time
func handleQuestion(question string) string {
    result, err := idx.Query(question)
    if err != nil {
        return "Error: " + err.Error()
    }

    // Get compact context for matched tables
    context := result.Matched().Text()

    // Pass to your LLM / SQL generator
    return generateSQL(question, context)
}
```

The `Text()` output is designed to be directly usable in LLM system prompts. It includes:
- Table names with relevance scores
- Primary keys and foreign key relationships
- Column types with semantic flags (`[state]`, `[cat]`, `^` for PK, `?` for nullable)
- Representative values for state/categorical fields
- JSONB paths with inferred types and sample values

---

## API reference

Full godoc: [pkg.go.dev/github.com/shrsv/dbctx](https://pkg.go.dev/github.com/shrsv/dbctx)

Or locally: `go doc github.com/shrsv/dbctx` or `make doc` (serves on localhost:6060)

### Key types

| Type | Description |
|---|---|
| `Index` | Compiled database context. Create with `Build`, `BuildAsync`, or `Open`. |
| `ResultSet` | Query result. Methods: `Matched()`, `All()`, `Include()`, `TableMap()`. |
| `Selection` | Filtered table set. Chain `Include()`/`Exclude()`, render with `Text()`. |
| `TableContext` | A table with columns, PK, FKs, values, JSONB paths, match score. |
| `Options` | Build config: `Path`, `Schemas`, `Logger`. |
| `Stats` | Summary counts: tables, columns, FKs, state fields, JSONB paths. |

### Key functions

| Function | Description |
|---|---|
| `Build(ctx, dsn, opts)` | Build index synchronously. Returns ready-to-query `*Index`. |
| `BuildAsync(ctx, dsn, opts)` | Build in background. Returns `*Index` + readiness channel. |
| `Open(path)` | Open existing `.dtx` file. No PostgreSQL needed. |
| `Index.Query(text)` | Search index. Returns `*ResultSet` for selection. |
| `Index.Tables()` | List all tables with summary info. |
| `Index.TableDetail(name)` | Full detail for one table. |
| `Index.Stats()` | Summary statistics. |
| `Index.Report(w)` | Dump human-readable report. |

---

## How dbctx is different

| Approach | dbctx | Dump schema to LLM | Vector embeddings |
|---|---|---|---|
| Requires LLM at build time | No | No | Yes |
| Discovers field values | Yes | No | Partial |
| Understands JSONB | Yes | No | No |
| Detects state fields | Yes | No | No |
| Compact output | Yes | Noisy | Noisy |
| Deterministic/reproducible | Yes | Yes | No |
| Works offline | Yes | Yes | No |
| Scales to large databases | Yes | No | Partial |

For small databases, dumping the whole schema to an LLM works fine. dbctx becomes valuable when you have 50+ tables, JSONB columns, or need reproducible context.

---

## Project status

* [x] PostgreSQL schema extraction
* [x] primary/foreign-key relationships
* [x] field statistics from `pg_stats`
* [x] categorical/state detection
* [x] representative values
* [x] JSONB structural inference
* [x] `.dtx` file format (SQLite)
* [x] fuzzy table/field/value retrieval
* [x] foreign-key expansion
* [x] compact context export (library)
* [x] web UI explorer
* [x] Go library API
* [ ] incremental updates
* [ ] stable `.dtx` specification

---

## Contributing

The most interesting parts of dbctx are the heuristics and the `.dtx` format itself.

Contributions welcome around:

* PostgreSQL introspection
* efficient incremental indexing
* JSONB structural inference
* categorical/state detection
* compact representations
* retrieval algorithms
* `.dtx` format design

---

## License

[License to be decided]
