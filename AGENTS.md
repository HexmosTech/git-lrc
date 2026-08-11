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
