# git-lrc

**Check AI-Generated Code With Git Hooks**

AI agents write code fast. They also *silently remove logic*, change behavior, and introduce bugs -- without telling you. You often find out in production.

**`git-lrc` fixes this.** It hooks into `git commit` and reviews every diff *before* it lands. 60-second setup. Completely free.

## Why

- 🤖 **AI agents silently break things.** Code removed. Logic changed. Edge cases gone. You won't notice until production.
- 🔍 **Catch it before it ships.** AI-powered inline comments show you *exactly* what changed and what looks wrong.
- 🔁 **Build a habit, ship better code.** Regular review → fewer bugs → more robust code → better results in your team.
- 🔗 **Why git?** Git is universal. Every editor, every IDE, every AI toolkit uses it. Committing is mandatory. So there's *almost no chance of missing a review* — regardless of your stack.

## Get Started

### Install

**Linux / macOS:**

```bash
curl -fsSL https://hexmos.com/lrc-install.sh | sudo bash
```

**Windows (PowerShell):**

```powershell
iwr -useb https://hexmos.com/lrc-install.ps1 | iex
```

Binary installed. Hooks set up globally. Done.

### Setup

```bash
git lrc setup
```

Two steps, both open in your browser:

1. **LiveReview API key** — sign in with Hexmos
2. **Free Gemini API key** — grab one from Google AI Studio

**~1 minute. One-time setup, machine-wide.** After this, *every git repo* on your machine triggers review on commit. No per-repo config needed.

## How It Works

### Option A: Review on commit (automatic)

```bash
git add .
git commit -m "add payment validation"
# review launches automatically before the commit goes through
```

### Option B: Review before commit (manual)

```bash
git add .
git lrc review          # run AI review first
# or: git lrc review --vouch   # vouch personally, skip AI
# or: git lrc review --skip    # skip review entirely
git commit -m "add payment validation"
```

Either way, a web UI opens in your browser.

```
📎 GIF: git add + git commit triggering review, browser opening
```

### The Review UI

- 📄 **GitHub-style diff** — color-coded additions/deletions
- 💬 **Inline AI comments** — at the exact lines that matter, with severity badges
- 📝 **Review summary** — high-level overview of what the AI found
- 📁 **File sidebar** — jump between files, see comment counts

```
📎 GIF: Web UI with diff view, inline comments, file sidebar, summary
```

### The Decision

| Action | What happens |
|--------|-------------|
| ✅ **Commit** | Accept and commit the reviewed changes |
| 🚀 **Commit & Push** | Commit and push to remote in one step |
| ⏭️ **Skip** | Abort the commit — go fix issues first |

```
📎 Screenshot: Pre-commit bar showing Commit / Commit & Push / Skip buttons
```

## The Review Cycle

Typical workflow with AI-generated code:

1. **Generate code** with your AI agent
2. **`git add .` → `git lrc review`** — AI flags issues
3. **Copy issues, feed them back** to your agent to fix
4. **`git add .` → `git lrc review`** — AI reviews again
5. Repeat until satisfied
6. **`git lrc review --vouch`** → **`git commit`** — you vouch and commit

Each `git lrc review` is an **iteration**. The tool tracks how many iterations you did and what percentage of the diff was AI-reviewed (**coverage**).

### Vouch

Once you've iterated enough and you're satisfied with the code:

```bash
git lrc review --vouch
```

This says: *"I've reviewed this — through AI iterations or personally — and I take responsibility."* No AI review runs, but coverage stats from prior iterations are recorded.

### Skip

Just want to commit without review or responsibility attestation?

```bash
git lrc review --skip
```

No AI review. No personal attestation. The git log will record `skipped`.

## Git Log Tracking

Every commit gets a **review status line** appended to its git log message:

```
LiveReview Pre-Commit Check: ran (iter:3, coverage:85%)
```
```
LiveReview Pre-Commit Check: vouched (iter:2, coverage:50%)
```
```
LiveReview Pre-Commit Check: skipped
```

- **`iter`** — number of review cycles before committing. `iter:3` = three rounds of review → fix → review.
- **`coverage`** — percentage of the final diff already AI-reviewed in prior iterations. `coverage:85%` = only 15% of the code is unreviewed.

Your team sees *exactly* which commits were reviewed, vouched, or skipped — right in `git log`.

## FAQ

### Review vs Vouch vs Skip?

| | **Review** | **Vouch** | **Skip** |
|---|---|---|---|
| AI reviews the diff? | ✅ Yes | ❌ No | ❌ No |
| Takes responsibility? | ✅ Yes | ✅ Yes, explicitly | ⚠️ No |
| Tracks iterations? | ✅ Yes | ✅ Records prior coverage | ❌ No |
| Git log message | `ran (iter:N, coverage:X%)` | `vouched (iter:N, coverage:X%)` | `skipped` |
| When to use | Each review cycle | Done iterating, ready to commit | Not reviewing this commit |

**Review** is the default. AI analyzes your staged diff and gives inline feedback. Each review is one iteration in the change–review cycle.

**Vouch** means you're *explicitly taking responsibility* for this commit. Typically used after multiple review iterations — you've gone back and forth, fixed issues, and are now satisfied. The AI doesn't run again, but your prior iteration and coverage stats are recorded.

**Skip** means you're not reviewing this particular commit. Maybe it's trivial, maybe it's not critical — the reason is yours. The git log simply records `skipped`.

### How is this free?

`git-lrc` uses **Google's Gemini API** for AI reviews. Gemini offers a generous free tier. You bring your own API key — there's no middleman billing. The LiveReview cloud service that coordinates reviews is free for individual developers.

### What data is sent?

Only the **staged diff** is analyzed. No full repository context is uploaded, and diffs are not stored after review.

### Can I disable it for a specific repo?

```bash
git lrc hooks disable   # disable for current repo
git lrc hooks enable    # re-enable later
```

### Can I review an older commit?

```bash
git lrc review --commit HEAD       # review the last commit
git lrc review --commit HEAD~3..HEAD  # review a range
```

## Quick Reference

| Command | Description |
|---------|-------------|
| `lrc` or `lrc review` | Review staged changes |
| `lrc review --vouch` | Vouch — skip AI, take personal responsibility |
| `lrc review --skip` | Skip review for this commit |
| `lrc review --commit HEAD` | Review an already-committed change |
| `lrc hooks disable` | Disable hooks for current repo |
| `lrc hooks enable` | Re-enable hooks for current repo |
| `lrc hooks status` | Show hook status |
| `lrc self-update` | Update to latest version |
| `lrc version` | Show version info |

> **Tip:** `git lrc <command>` and `lrc <command>` are interchangeable.

## It's Free. Share It.

`git-lrc` is **completely free.** No credit card. No trial. No catch.

If it helps you — **share it with your developer friends.** The more people review AI-generated code, the fewer bugs make it to production.

⭐ **[Star this repo](https://github.com/HexmosTech/git-lrc)** to help others discover it.


## License

`git-lrc` is distributed under a modified variant of **Sustainable Use License (SUL)**.

> [!NOTE]
>
> **What this means:**
> - ✅ **Source Available** — Full source code is available for self-hosting
> - ✅ **Business Use Allowed** — Use LiveReview for your internal business operations
> - ✅ **Modifications Allowed** — Customize for your own use
> - ❌ **No Resale** — Cannot be resold or offered as a competing service
> - ❌ **No Redistribution** — Cannot redistribute modified versions commercially
>
> This license ensures LiveReview remains sustainable while giving you full access to self-host and customize for your needs.

For detailed terms, examples of permitted and prohibited uses, and definitions, see the full
[LICENSE.md](LICENSE.md).

---

## For Teams: LiveReview

> Using `git-lrc` solo? Great. Building with a team? Check out **[LiveReview](https://hexmos.com/livereview)** — the full suite for team-wide AI code review, with dashboards, org-level policies, and review analytics. Everything `git-lrc` does, plus team coordination.
