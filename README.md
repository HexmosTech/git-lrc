<div align="left">

| [🇩🇰 Dansk](readme/README.da.md) | [🇪🇸 Español](readme/README.es.md) | [🇮🇷 Farsi](readme/README.fa.md) | [🇫🇮 Suomi](readme/README.fi.md) | [🇯🇵 日本語](readme/README.ja.md) | [🇳🇴 Norsk](readme/README.nn.md) | [🇵🇹 Português](readme/README.pt.md) | [🇷🇺 Русский](readme/README.ru.md) | [🇦🇱 Shqip](readme/README.sq.md) | [🇨🇳 中文](readme/README.zh.md) |

<br />
<br />

<img width="60" alt="git-lrc logo" src="https://hexmos.com/freedevtools/public/lr_logo.svg" />

<br />
<h1>git-lrc</h1>

<h2>AI Micro Code Reviews That Run on Commit</h2>

<br />
<br />



<a href="https://www.producthunt.com/products/git-lrc?embed=true&amp;utm_source=badge-top-post-badge&amp;utm_medium=badge&amp;utm_campaign=badge-git-lrc" target="_blank" rel="noopener noreferrer"><img alt="git-lrc - Free, unlimited AI code reviews that run on commit | Product Hunt" width="200" src="https://api.producthunt.com/widgets/embed-image/v1/top-post-badge.svg?post_id=1079262&amp;theme=light&amp;period=daily&amp;t=1771749170868"></a>
&nbsp;

<br />
<a href="https://discord.gg/sGdnKwB3qq" target="_blank">
  <img alt="Discord Community" src="https://img.shields.io/badge/Discord-Community-5865F2?logo=discord&labelColor=white">
</a> <a href="https://goreportcard.com/report/github.com/HexmosTech/git-lrc" target="_blank" rel="noopener noreferrer"><img alt="Go Report Card" src="https://goreportcard.com/badge/github.com/HexmosTech/git-lrc"></a>&nbsp;<a href="https://github.com/HexmosTech/git-lrc/actions/workflows/gitleaks.yml" target="_blank" rel="noopener noreferrer"><img alt="gitleaks.yml" title="gitleaks.yml: Secret scanning workflow" src="https://github.com/HexmosTech/git-lrc/actions/workflows/gitleaks.yml/badge.svg"></a>&nbsp;<a href="https://github.com/HexmosTech/git-lrc/actions/workflows/osv-scanner.yml" target="_blank" rel="noopener noreferrer"><img alt="osv-scanner.yml" title="osv-scanner.yml: Dependency vulnerability scan" src="https://github.com/HexmosTech/git-lrc/actions/workflows/osv-scanner.yml/badge.svg"></a>&nbsp;<a href="https://github.com/HexmosTech/git-lrc/actions/workflows/govulncheck.yml" target="_blank" rel="noopener noreferrer"><img alt="govulncheck.yml" title="govulncheck.yml: Go vulnerability check" src="https://github.com/HexmosTech/git-lrc/actions/workflows/govulncheck.yml/badge.svg"></a>&nbsp;<a href="https://github.com/HexmosTech/git-lrc/actions/workflows/semgrep.yml" target="_blank" rel="noopener noreferrer"><img alt="semgrep.yml" title="semgrep.yml: Static analysis security scan" src="https://github.com/HexmosTech/git-lrc/actions/workflows/semgrep.yml/badge.svg"></a>&nbsp;<img alt="dependabot-enabled" title="dependabot-enabled: Automated dependency updates are enabled" src="./gfx/dependabot-enabled.svg">
</div>

<br />
<br />

---

AI agents write code fast. They also _silently remove logic_, change behavior, and introduce bugs -- without telling you. You often find out in production.

**`git-lrc` fixes this.** It hooks into `git commit` and reviews every diff _before_ it lands. 60-second setup. Completely free.

## See It In Action

> See git-lrc catch serious security issues such as leaked credentials, expensive cloud
> operations, and sensitive material in log statements

https://github.com/user-attachments/assets/cc4aa598-a7e3-4a1d-998c-9f2ba4b4c66e

## Why

- 🤖 **AI agents silently break things.** Code removed. Logic changed. Edge cases gone. You won't notice until production.
- 🔍 **Catch it before it ships.** AI-powered inline comments show you _exactly_ what changed and what looks wrong.
- 🔁 **Build a habit, ship better code.** Regular review → fewer bugs → more robust code → better results in your team.
- 🔗 **Why git?** Git is universal. Every editor, every IDE, every AI toolkit uses it. Committing is mandatory. So there's _almost no chance of missing a review_ — regardless of your stack.

## Get Started

### Install

#### Via IPM (Recommended):
```bash
# Linux/macOS
curl -L https://hexmos.com/ipm-install | bash && ipm i HexmosTech/git-lrc

# Windows
iwr https://hexmos.com/ipm-install-ps | iex; ipm i HexmosTech/git-lrc
```

#### Alternative (direct install):

**Linux / macOS:**

```bash
curl -fsSL https://hexmos.com/lrc-install.sh | bash
```

**Windows (PowerShell):**

```powershell
iwr -useb https://hexmos.com/lrc-install.ps1 | iex
```

<details>
<summary><strong>GitHub Codespaces</strong></summary>

```bash
curl -fsSL https://git.new/lrc-install | bash
```

</details>

Binary installed. Hooks set up globally. Done.

### Setup

```bash
git lrc setup
```

Here's a quick video of how setup works:

https://github.com/user-attachments/assets/392a4605-6e45-42ad-b2d9-6435312444b5

Two steps, both open in your browser:

1. **LiveReview API key** — sign in with Hexmos
2. **Free Gemini API key** — grab one from Google AI Studio

**~1 minute. One-time setup, machine-wide.** After this, _every git repo_ on your machine triggers review on commit. No per-repo config needed.


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

https://github.com/user-attachments/assets/ae063e39-379f-4815-9954-f0e2ab5b9cde

### The Review UI

- 📄 **GitHub-style diff** — color-coded additions/deletions
- 💬 **Inline AI comments** — at the exact lines that matter, with severity badges
- 📝 **Review summary** — high-level overview of what the AI found
- 📁 **Staged file list** — see all staged files at a glance, jump between them
- 📊 **Diff summary** — lines added/removed per file for a quick sense of change scope
- 📋 **Copy issues** — one click to copy all AI-flagged issues, ready to paste back into your AI agent
- 🔄 **Cycle through issues** — navigate between comments one by one without scrolling
- 📜 **Event log** — track review events, iterations, and status changes in one place

https://github.com/user-attachments/assets/b579d7c6-bdf6-458b-b446-006ca41fe47d

### The Decision

| Action               | What happens                           |
| -------------------- | -------------------------------------- |
| ✅ **Commit**        | Accept and commit the reviewed changes |
| 🚀 **Commit & Push** | Commit and push to remote in one step  |
| ⏭️ **Skip**          | Abort the commit — go fix issues first |

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

This says: _"I've reviewed this — through AI iterations or personally — and I take responsibility."_ No AI review runs, but coverage stats from prior iterations are recorded.

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

Your team sees _exactly_ which commits were reviewed, vouched, or skipped — right in `git log`.



## Bring Your Own AI Connector (BYOK)

In addition to the default Gemini setup, you can bring your own API keys for:

- OpenAI
- Claude
- DeepSeek
- OpenRouter

Use:

```bash
lrc ui
```

From the UI, you can:

- Re-authenticate your account
- Add or update AI connectors
- Reorder connectors to set priority

By default, the **first connector in the list** is used for reviews.

![LRC UI connector management preview](./gfx/git-lrc-ui.png)

## Security You Can Trust

- Security is treated as a core product requirement in git-lrc.
- We document reporting channels, response commitments, and operational safeguards clearly.
- Automated security checks and SBOM workflows support transparent verification.
- For complete details, see [SECURITY.md](SECURITY.md).

## FAQ

### Review vs Vouch vs Skip?

|                       | **Review**                  | **Vouch**                       | **Skip**                  |
| --------------------- | --------------------------- | ------------------------------- | ------------------------- |
| AI reviews the diff?  | ✅ Yes                      | ❌ No                           | ❌ No                     |
| Takes responsibility? | ✅ Yes                      | ✅ Yes, explicitly              | ⚠️ No                     |
| Tracks iterations?    | ✅ Yes                      | ✅ Records prior coverage       | ❌ No                     |
| Git log message       | `ran (iter:N, coverage:X%)` | `vouched (iter:N, coverage:X%)` | `skipped`                 |
| When to use           | Each review cycle           | Done iterating, ready to commit | Not reviewing this commit |

**Review** is the default. AI analyzes your staged diff and gives inline feedback. Each review is one iteration in the change–review cycle.

**Vouch** means you're _explicitly taking responsibility_ for this commit. Typically used after multiple review iterations — you've gone back and forth, fixed issues, and are now satisfied. The AI doesn't run again, but your prior iteration and coverage stats are recorded.

**Skip** means you're not reviewing this particular commit. Maybe it's trivial, maybe it's not critical — the reason is yours. The git log simply records `skipped`.

### How is this free?

`git-lrc` uses **Google's Gemini API** by default for AI reviews, and also supports BYOK connectors (OpenAI, Claude, DeepSeek, OpenRouter) — see [Bring Your Own AI Connector (BYOK)](#bring-your-own-ai-connector-byok). Gemini offers a generous free tier. You bring your own API key(s) — there's no middleman billing. The LiveReview cloud service that coordinates reviews is free for individual developers.

### Which AI providers can I use with BYOK?

You can connect Gemini (default), OpenAI, Claude, DeepSeek, and OpenRouter.

Manage connectors from:

```bash
lrc ui
```

### How do I choose which connector is used for review?

Open:

```bash
lrc ui
```

Then reorder your connector list. The first connector is used by default when a review runs.

### Can I re-authenticate or change connectors later?

Yes. Run `lrc ui` anytime to re-authenticate, add/remove connectors, and update connector priority.

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

| Command                                 | Description                                              |
| --------------------------------------- | -------------------------------------------------------- |
| `lrc setup`                             | Guided onboarding and initial auth/config                |
| `lrc ui`                                | Open local UI to re-auth, manage BYOK connectors, priority |
| `lrc` or `lrc review`                   | Run a review with sensible defaults                      |
| `lrc review --staged`                   | Review staged changes only                               |
| `lrc review --commit HEAD`              | Review a specific commit                                 |
| `lrc review --commit HEAD~3..HEAD`      | Review a commit range                                    |
| `lrc review --range HEAD~1..HEAD`       | Review a git diff range (working/staged override)        |
| `lrc review --vouch`                    | Vouch — skip AI, take personal responsibility            |
| `lrc review --skip`                     | Skip review for this commit                              |
| `lrc hooks install`                     | Install global hook dispatcher                           |
| `lrc hooks uninstall`                   | Remove global hook dispatcher and managed scripts        |
| `lrc hooks enable`                      | Enable hooks for current repo                            |
| `lrc hooks disable`                     | Disable hooks for current repo                           |
| `lrc hooks status`                      | Show hook status for current repo                        |
| `lrc self-update`                       | Update to latest version                                 |
| `lrc version`                           | Show version info                                        |

> **Tip:** `git lrc <command>` and `lrc <command>` are interchangeable.

## It's Free. Share it with your friends and colleagues.

`git-lrc` is **completely free.** No credit card. No trial. No catch.

If it helps you — **share it with your developer friends.** The more people review AI-generated code, the fewer bugs make it to production.

⭐ **[Star this repo](https://github.com/HexmosTech/git-lrc)** to help others discover it.

## Community

Pick the right place based on what you need:

- **Discord**: [discord.gg/sGdnKwB3qq](https://discord.gg/sGdnKwB3qq) - Best for joining the community, asking general Q&A, and having quick back-and-forth with the team.
- **GitHub Discussions**: [github.com/HexmosTech/git-lrc/discussions](https://github.com/HexmosTech/git-lrc/discussions) - For in-depth idea proposals, scoping, design discussions, modification proposals, and constructive criticism.
- **GitHub Issues**: [github.com/HexmosTech/git-lrc/issues](https://github.com/HexmosTech/git-lrc/issues) - For concrete, scoped tasks such as bugs, focused feature requests, and actionable implementation work.

## License

`git-lrc` is distributed under a modified variant of **Sustainable Use License (SUL)**.

> [!NOTE]
>
> **What this means:**
>
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
Test PR by Vinanthi