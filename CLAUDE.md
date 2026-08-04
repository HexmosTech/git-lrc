# git-lrc release process

This is the canonical, verified sequence for cutting a release. All commands
run from the repo root via the top-level `Makefile`.

## Prerequisites

- Clean working tree (`git status` shows nothing to commit) — `make bump`
  and `scripts/lrc_build.py` both refuse to run otherwise.
- `.env` with `B2_KEY_ID`, `B2_APP_KEY`, `B2_BUCKET_NAME`, `B2_BUCKET_ID` (see
  `.env.example`) — required for `make release`'s binary upload. If missing,
  `make download-secrets` pulls them from the GitHub repo variables via `gh`
  (requires `gh` to be authenticated with access to repo variables). This
  writes real production B2 credentials to a local file and the resulting
  `make release` publishes binaries that live users' `lrc self-update` may
  pull — confirm with the human before running it if credentials aren't
  already present.
- `gh` authenticated against `HexmosTech/git-lrc` for `make release-gh`.

## Sequence

1. **Land the code change first.** Stage, review (or skip), commit, and push
   *before* touching the version:
   ```
   git add <files>
   git lrc review --skip   # or run a real review; skip only for low-risk/docs changes
   git commit -m "..."
   git push
   ```

2. **Bump the version** — `make bump`. This is interactive:
   prompts for bump type (`patch`/`minor`/`major`), then confirms
   `vOLD → vNEW`. It updates `main.go`'s `appVersion`, commits that change
   itself (`git add main.go`, `git lrc review --skip`, `git commit`), and
   creates + pushes an **annotated tag** (`git tag -a vNEW`, `git push origin
   vNEW`).
   - Important: `make bump` pushes the **tag**, not the branch. The bump
     commit itself stays local until a later `git push origin main`.
   - Non-interactively: `printf '<patch|minor|major>\ny\n' | make bump`.

3. **Build and upload binaries** — `make release`. Builds all 5 platforms
   (linux/darwin × amd64/arm64, windows/amd64) and uploads each to Backblaze
   B2 under `lrc/<version>/<platform>/`, plus a `latest.json` manifest that
   `lrc self-update` reads. This step takes a while (uploads block on B2) —
   expect it to run past a 2-minute foreground timeout; let it run in the
   background and check back rather than re-invoking it.
   - Afterward it prompts (interactively) to scaffold release notes: creates
     `docs/releases/<version>.md` from `docs/releases/_template.md` and an
     empty `docs/releases/img/<version>/` folder (with its own README
     explaining the `IMG:path` reference syntax). If the scaffold already
     exists for this version, it skips straight to printing next-step
     reminders instead of failing.
   - Non-interactively: `printf 'y\n' | make release` (or pass `y` up front
     if scripting; the prompt only fires when the scaffold doesn't exist
     yet).

4. **Fill in the release notes** at `docs/releases/<version>.md`. The
   scaffold's `## Summary`, `## Changes`, `## Breaking Changes`, and
   `## Known Issues` sections are placeholders — replace them with real,
   specific content (see any past `docs/releases/vX.Y.Z.md` for tone: short
   bullet points, one per user-facing change, naming the actual
   files/mechanisms touched).
   - **Delete the leading HTML comment block** the scaffold inserts (the
     `<!-- Release images for this version belong in: ... -->` block right
     under the `Date:` line) once you've either used it or confirmed there
     are no images for this release — it's scaffolding instructions, not
     part of the published notes, and `make release-notes-check` doesn't
     require it either way.
   - If you do have screenshots/GIFs/video for this release, drop them in
     `docs/releases/img/<version>/` and reference with
     `![alt](IMG:path/to/file.png)`; for a video you can't embed inline,
     leave a `<!-- VIDEO:demo.mp4 -->` HTML-comment reminder for a human
     follow-up — but don't leave the *auto-generated* instructional comment
     block itself in the published file.

5. **Commit and push the release notes**:
   ```
   git add docs/releases/<version>.md docs/releases/img/<version>/
   git lrc review --skip
   git commit -m "docs: add release notes for <version>"
   git push origin main
   ```
   This is also the point to push the version-bump commit from step 2 if it
   hasn't gone up yet — `git push origin main` sends both in one go.

6. **Validate and publish** the GitHub release:
   ```
   make release-notes-check VERSION=<version>   # optional standalone check
   make release-gh VERSION=<version>
   ```
   `release-gh` runs `release-preflight` first (re-checks release notes
   headings + `check-status-doc`), then publishes the GitHub release itself
   from the markdown notes (no binary assets attached — those already live
   on B2 from step 3). `VERSION` is optional; `scripts/release_gh.py` can
   auto-infer it (from the latest tag) if omitted.

7. **Verify**: `gh release view <version> --repo HexmosTech/git-lrc`.

## Notes

- `make release-internal` (channel=internal) is a separate, fixed-pseudo-version
  path for internal builds that don't participate in self-update — different
  flow, not part of the above.
- If a step needs to be rerun (e.g. release notes rejected by
  `release-notes-check`), re-running `make release-notes-init VERSION=...`
  will refuse if the scaffold already exists — edit the existing file
  directly instead.
