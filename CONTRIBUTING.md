## Contributing to git-lrc

## TL;DR

- Start with a Discussion if the work is not already agreed and clearly scoped.
- Do not open direct PRs for unscoped work. The preferred flow is Discussion -> Issue -> PR.
- Build locally with `make build-local && lrc hooks install`.
- Run the most specific test that proves your change.
- Keep storage and file operations in `storage/`, and keep network operations in `network/`.
- If your change touches UI, a GIF or video walkthrough is required in the PR. This is a hard requirement.
- If your change affects `storage/` or `network/`, update the matching status doc and run `make check-status-doc`.
- Private security reports should go through GitHub Security Advisories, not public issues.
- git-lrc runs locally by default. Code leaves the machine when you submit reviews or run setup or update flows that call remote APIs.
- Read [SECURITY.md](./SECURITY.md) if your contribution touches security, data flow, storage, network behavior, or disclosure handling.
- By contributing, you agree to the Contributor License Agreement.

The ideas behind git-lrc matter, but so does the way changes enter the project.

This guide is here to help you contribute in a way that is clear, scoped, and easy to review.

## Security At A Glance

git-lrc has explicit security and disclosure guidance. Please use it.

- If you are reporting a vulnerability, use the private reporting flow in [SECURITY.md](./SECURITY.md) and open a GitHub Security Advisory instead of a public issue.
- git-lrc runs locally as a CLI by default. Review, setup, and update flows can call remote APIs; see [SECURITY.md](./SECURITY.md) for the exact runtime and data-flow details.
- If your change affects storage, network behavior, credentials, review payloads, or disclosure handling, check [SECURITY.md](./SECURITY.md) before opening or updating the PR.

## Start With the Right Forum

There are several valid ways to contribute, but they serve different purposes:

- Use [GitHub Discussions](https://github.com/HexmosTech/git-lrc/discussions) for ideas, problem framing, design discussion, and early proposals.
- Use [GitHub Issues](https://github.com/HexmosTech/git-lrc/issues) for concrete, agreed, scoped work.
- Use pull requests to implement an agreed issue.

If the work is still fuzzy, start in Discussions.

## When In Doubt, Start With a Discussion

At git-lrc, we treat writing as a tool for clarifying thought.

It is usually a mistake to begin implementation before the shape of the work and the reason for doing it are clearly understood.

If you cannot explain what you plan to do, what problem it solves, and why that approach is the right one, the work is probably not ready yet.

Work backwards from the problem and from the user experience. That usually leads to better scoping and better implementation.

## Preferred Contribution Flow

We do **not** recommend directly raising PRs.

The preferred path is:

1. Open a Discussion and describe the problem, the user impact, and the proposed direction.
2. Refine the scope until the work is concrete.
3. Promote the agreed work into an [Issue](https://github.com/HexmosTech/git-lrc/issues).
4. Open a PR that fulfills that issue.

This keeps the project focused and avoids PRs that arrive before the problem has been agreed on.

## Getting Started Locally

Use the Go version declared in [go.mod](./go.mod).

Build and install git-lrc locally with:

```bash
make build-local && lrc hooks install
```

Important conventions:

- The binary name must stay `lrc`.
- If you build or script around the CLI, keep that naming consistent.

Useful sanity checks after building:

```bash
lrc version
lrc hooks status
```

## Testing Your Change

Run the most specific test that proves your change.

Prefer focused validation such as:

```bash
make test-pkg PKG=./path/to/package
```

Use broader test runs only when the scope actually justifies them.

The main confidence lanes are now split explicitly:

```bash
make test-go
make test-simulator
make test-hooks-worktree
make test-hooks-claude
make test-js
make testall
```

Use the cheapest lane that proves the behavior you changed:

- `make test-simulator` for decision flow, phase gating, message resolution, and race behavior.
- `make test-hooks-worktree` or `make test-hooks-claude` for real temp-repo, hook, worktree, and Claude wrapper regressions.
- `make test-js` for deterministic UI state-model regressions.
- `make test-go` when the touched slice is broader or package-scoped.

If a bug escaped once, add a regression test in the cheapest relevant lane before moving on.

For end-to-end testing without real AI calls, use fake review mode:

```bash
make build-local-test
WAIT=30s make run-fake-review
```

For UI work, use the live development loop:

```bash
make dev-ui
```

That flow lets you edit files under `internal/staticserve/static/` and refresh the browser without rebuilding the binary for each UI change.

## Code Organization Expectations

git-lrc has explicit architectural boundaries. Please follow them.

- Database and file operations belong in `storage/`.
- Network operations belong in `network/`.
- Other packages should call those `storage/` and `network/` functions rather than reimplementing the behavior elsewhere.

If your change affects those responsibilities, update the status documents alongside the code:

- [storage/storage_status.md](./storage/storage_status.md)
- [network/network_status.md](./network/network_status.md)

Then verify the line references with:

```bash
make check-status-doc
```

## Quality Bar

Please keep these project rules in mind when proposing or implementing changes:

- A name must match its function or meaning. If a name says one thing and the code does another, treat that as a major bug.
- Do not add fallback implementations unless they are explicitly asked for. Fallbacks create confusing behavior and make the system harder to reason about.
- Prefer incremental changes over sweeping refactors. Make the smallest coherent change that solves the agreed problem.
- Keep the why visible in the Discussion, Issue, and PR so reviewers can evaluate the change on purpose, not just code shape.

## PR Expectations

A good pull request in git-lrc is:

- Based on an agreed issue.
- Narrow enough to review without guessing at hidden scope.
- Clear about what changed and why.
- Explicit about what you tested.

If your PR changes behavior, call that out directly.

If your PR touches UI, you must include a GIF or video walkthrough of the working change.

## UI Changes Require a GIF or Video

When your work changes the user interface, a GIF or video walkthrough is required.

The walkthrough should make it easy for a reviewer to see:

- what changed,
- how the interaction behaves,
- and that the change was actually exercised.

PRs that change UI without this walkthrough are not complete.

AI-assisted programming is fine, but UI changes still need to be tested and demonstrated clearly.

## Contributor License Agreement

If you contribute to git-lrc, you agree that you have read and accepted the terms in the [Contributor License Agreement](./CONTRIBUTOR_LICENSE_AGREEMENT.md).
