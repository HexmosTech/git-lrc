# dbctx

### Compile a PostgreSQL database into compact, queryable context.

**dbctx** builds a `.dtx` database context index from PostgreSQL: tables, relationships, field semantics, representative values, state-like fields, JSONB structure, and query-relevant metadata — without using an LLM.

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
    └── retrieval index
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
| Use it from the command line | [CLI quick start](#1-build-an-index) |
| Browse the database in a web UI | [Web UI](#3-explore-in-the-ui) |
| Use it as a Go library in my app | [Library Usage](#library-usage) |
| Query with natural language and get compact schema | [Querying the context](#querying-the-context) |
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

*(screenshot coming soon)*

---

### 2. Query from the CLI

```bash
dbctx query mydb.dtx "How many failed GitHub reviews last month?"
```

The query finds relevant tables, surfaces JSONB structure, and highlights state-like fields — all in compact text output.

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

    // Get compact schema for matched tables only — ready for an LLM prompt
    fmt.Println(result.Matched().Text())         // includes notation legend

    // Or refine the selection
    fmt.Println(result.All().Text())                             // all tables
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
result.Matched().Text()                          // matched tables only (score > 0)
result.All().Text()                              // all tables including FK-expanded
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

# dbctx does not use an LLM

This is a deliberate design decision.

dbctx uses deterministic database introspection, statistics, heuristics, indexing, and relationship analysis.

It does **not** require:

* an OpenAI API key
* an embedding model
* an inference server
* an LLM
* a vector database

The database context should be something you can build locally, inspect, diff, cache, ship, and reproduce.

For example:

```text
same database state
        +
same dbctx version
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
table / field / value matching
    │
    ▼
candidate tables
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
┌───────────────────────────────────────────┐
│                  dbctx                    │
│                                           │
│  PostgreSQL introspection                 │
│          │                                │
│          ▼                                │
│  Schema graph                             │
│          │                                │
│          ├── Field analysis               │
│          ├── Value analysis               │
│          ├── JSONB analysis               │
│          └── Relationship analysis        │
│                    │                      │
│                    ▼                      │
│              .dtx database context        │
│                    │                      │
│          ┌─────────┴─────────┐            │
│          ▼                   ▼            │
│      retrieval             export        │
│          │                   │            │
│          ▼                   ▼            │
│   candidate context     compact format   │
└───────────────────────────────────────────┘
```

The core implementation is intended to be a **single binary**.

No database server.

No separate indexing service.

No model runtime.

No external vector database.

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

### 1. No LLM required

The database context should be derived from observable facts and deterministic heuristics.

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

Engineers should be able to understand why a particular table or field appeared in a context result.

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
* a vector database
* a replacement for PostgreSQL's system catalog
* an attempt to infer arbitrary business logic

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

[License to be decided]
