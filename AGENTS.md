# Rule 1: git-lrc build command

Build and install lrc locally with:

make build-local && lrc hooks install

When building the lrc command, always use the binary name "lrc"

A primary rule in programming is that the name must match function or meaning. If the name says one thing and the function or meaning is another - it must be treated as a major bug and treated with highest priority. Always fix naming bugs immediately when discovered.

When doing refactors or improvements - don't do fallback implementations unless explicitly asked for. Having fallbacks creates very confusing program behavior.

When you want to run a test - always run the most specific test you want to run. Don't run all or other irrelevant tests.

When making proposals - think of how you can accomplish goals in an incremental way rather than suggesting sweeping refactors.


# Rule 2: Code organization and status docs

Enforces the rule that db, file operations go into storage folder/module with specific function/struct names in logical files. Then network operations go into network folder/module again with specific function/struct names in logical files.

The other code should call these storage/network functions/structs and use the results from there.

This should be enforced and not violated.

Secondly, the status docs #file:storage_status.md   and #file:network_status.md  be also simultaneously updated to keep with the new changes.

Finally the line numbers should be kept upto date in these status docs, and violation should be checked via #file:check-status-doc-links.sh 

Stick strictly to my language and specifications here. Don't change it and generalize it the way you want. Stay with my guidance as is.


# Rule 3: UI iconography guidance

For any UI icon work in git-lrc, follow `docs/ui-iconography.md` as the source of truth.

- Use the shared icon registry in `internal/staticserve/static/components/icons.js`.
- Do not introduce new emoji or Unicode action icons.
- Do not force vendor logos onto action buttons just because the label names a vendor.
- If a new icon decision is needed, update `docs/ui-iconography.md` together with the code.


# Rule 4: Syncing to LiveReview

LiveReview (sibling repo, typically at `../LiveReview`) hosts a web-based review-details
page that ports capabilities built here — starting with the review UI
(`internal/staticserve/static/`) and the blast-radius scoring engine (`blastradius/`).
See `/home/shrsv/.claude/plans/piped-imagining-sky.md` for the design, and
`LiveReview/AGENTS.md`'s "Porting from git-lrc" section for the full convention.

When changing `blastradius/` or the review UI components, check whether LiveReview has a
ported copy that cites the changed file (LiveReview files carry a
`// Ported from git-lrc:<path>#L<start>-L<end>` header) — if so, the port may need a
follow-up update there too. This repo has no obligation to keep those in sync itself;
it's just worth flagging in the PR/commit description when a change touches ported code.

Locally-computed artifacts (like `blastradius.Report`) reach LiveReview via a generic
upload channel: `POST {api_url}/api/v1/diff-review/{review_id}/artifacts/{artifact_type}`,
fire-and-forget from the CLI, using the same API key + `review_id` already obtained from
the initial review submission (`internal/reviewapi/helpers.go`). Never let an artifact
upload failure affect the review itself — log and move on, matching how blast-radius
scoring already treats its own failures as non-fatal.


# Rule 5: Never scan and release at the same time

Scanning and releasing are separate, serialized phases, in this repo and in LiveReview.

- Run one scanner at a time, each to completion, before starting any release work. No
  `make bump`, tag, binary upload, or release-note edit while a scan is still running.
- A real scanner finding blocks the release until it is fixed or explicitly triaged as a
  documented false positive.
- Finish one repository's scan-then-release cycle before starting the other repository's.
  Never scan or release both repos in parallel.
- One release at a time: bump, tag, upload binaries, then publish notes.
- **Release from `main`, never from a feature branch.** Merge the feature branch into
  `main` first (fast-forward when possible), push `main`, and only then bump/tag/upload
  from `main`. A tag on a feature branch publishes code that `main` does not contain.
  `scripts/lrc_build.py` enforces this and refuses to bump or release off `main`
  (`ALLOW_RELEASE_FROM_BRANCH=1` is the deliberate, documented override).

If a scanner is running, the only correct next action is to wait for its result.
See `CLAUDE.md` ("Release sequencing is mandatory") for the concrete sequence.
