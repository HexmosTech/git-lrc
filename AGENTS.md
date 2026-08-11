# AGENTS.md — Guidelines for AI Agents

## Build & Test

```bash
make all             # build + run all tests (single command, use this)
make build           # build CLI binary only
make test            # run all tests only
make test-short      # skip slow tests
make test-cover      # run with coverage report
make test-integration  # run integration tests (requires DATABASE_URL)
make install         # install to ~/go/bin
```

All tests use in-memory SQLite. No PostgreSQL or external services required for `make test`.

## Writing tests

- Use table-driven tests (`[]struct{name string; ...}` with `t.Run`)
- Use `internal/testutil.NewSeedStore(t)` for tests that need a seeded database without FTS
- Use `internal/testutil.NewTestStore(t, search.PopulateFTS)` for tests that need FTS/search
- For search package tests, use `testutil.NewSeedStore(t)` + `PopulateFTS(store)` directly (avoids import cycle)
- Use `t.TempDir()` for tests that need a file path
- No external test libraries — use stdlib `testing` only
- Each test should be independent and not rely on test execution order
- Name tests: `TestFunctionName_Scenario` (e.g., `TestQuery_FuzzyMatch`)

### Test fixture

The test fixture (`internal/testutil/fixture.go`) provides 4 tables covering all features:

| Table | Features |
|-------|----------|
| reviews | PK, FK→orgs, FK→pull_requests, [state] status, [cat] priority, jsonb metadata, nullable body |
| orgs | PK, [state] plan, [cat] tier |
| pull_requests | PK, FK→orgs, integer columns |
| comments | PK, FK→reviews, nullable body |

To add new test scenarios, extend `FixtureSchema()` or create additional seed data.

### What to test

- Pure functions: table-driven input/output tests
- Store operations: seed with `testutil.NewSeedStore(t)`, verify reads
- HTTP handlers: use `net/http/httptest` with seeded store
- Text rendering: verify output contains expected substrings
- Benchmarks: use `*testing.B` with `testutil.NewTestStore(b, search.PopulateFTS)`

### What NOT to test

- Don't test against live PostgreSQL in unit tests (use `make test-integration` for those)
- Don't test CLI cobra wiring (test the underlying functions instead)
- Don't test third-party library behavior

## Integration tests

Integration tests are behind a `//go:build integration` tag. They require:

```bash
export DATABASE_URL="postgres://user:pass@host/db"
make test-integration
```

These test the full `Build()` pipeline against real PostgreSQL. They are not run in CI by default.

## Performance tests

- `dbctx_bench_test.go` — benchmarks for Query, Text(), Report, Tables, TableDetail, Stats (uses in-memory fixture)
- `dbctx_perf_test.go` — integration tests that time each build phase and query types against real PostgreSQL

Run benchmarks: `go test -bench=Benchmark -benchmem -run=^$ .`
Run perf tests: `source .env.prod && DATABASE_URL="$DATABASE_URL" go test -tags integration -run='TestIntegration_Perf' -v .`

## Publishing to pkg.go.dev

### Best practices checklist

Before tagging a release, verify:

1. **`go.mod` exists** — module path is `github.com/shrsv/dbctx`
2. **LICENSE file exists** — MIT at repo root (pkg.go.dev checks redistributability)
3. **Package doc comment** — first sentence is indexed by pkg.go.dev search; must be concise and keyword-rich
4. **Example test functions** — render as "Examples" section on pkg.go.dev (`Example()`, `ExampleOpen()`, `ExampleIndex_TableDetail()`, etc. in `example_test.go`)
5. **All tests pass** — `make test`
6. **`go vet` clean** — `go vet ./...`
7. **Tagged version** — use semver (`v0.1.0`, `v0.2.0`, etc.)

### Release procedure

```bash
# 1. Run all checks
make release-check

# 2. Tag and push
make tag V=v0.1.0

# 3. Verify on pkg.go.dev (takes a few minutes to index)
# Visit: https://pkg.go.dev/github.com/shrsv/dbctx@v0.1.0
# Click "Request" if not yet indexed
```

### Versioning rules

- Follow semver: `vMAJOR.MINOR.PATCH`
- **v0.x.x** = experimental (current state); breaking changes allowed in minor bumps
- **v1.0.0+** = stable; breaking changes require major version bump
- Always tag from a clean working tree
- Write a brief release note for each tag (what changed, what's new)

### Godoc conventions

- Package doc comment: first sentence = one-line summary (indexed for search)
- All exported types and functions must have doc comments
- Example functions in `example_test.go` render on pkg.go.dev — add them for all public APIs
- Use `[TypeName]` bracket syntax for cross-references in doc comments (renders as links)
- Use `# Heading` in doc comments for godoc section headers
