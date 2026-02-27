<div align="center">

<img width="60" alt="git-lrc logo" src="https://hexmos.com/freedevtools/public/lr_logo.svg" />

<strong style="font-size:2em; display:block; margin:0.67em 0;">git-lrc</strong>


<strong style="font-size:1.5em; display:block; margin:0.67em 0;">Free, Unlimited AI Code Reviews That Run on Commit</strong>

<br />

</div>

<br />

<div align="center">
<a href="https://www.producthunt.com/products/git-lrc?embed=true&amp;utm_source=badge-top-post-badge&amp;utm_medium=badge&amp;utm_campaign=badge-git-lrc" target="_blank" rel="noopener noreferrer"><img alt="git-lrc - Free, unlimited AI code reviews that run on commit | Product Hunt" width="250" height="54" src="https://api.producthunt.com/widgets/embed-image/v1/top-post-badge.svg?post_id=1079262&amp;theme=light&amp;period=daily&amp;t=1771749170868"></a>
</div>

<br />
<br />

---

AI-agenter skriver kode hurtigt. De _fjerner også logik stille_, ændrer adfærd og introducerer fejl — uden at fortælle dig det. Du opdager det ofte først i produktion.

**`git-lrc` løser det.** Det kobles på `git commit` og gennemgår hver diff _før_ den lander. 60 sekunders opsætning. Helt gratis.

## Se det i aktion

> Se git-lrc fange alvorlige sikkerhedsproblemer som lækkede credentials, dyre cloud-
> operationer og følsomt materiale i log-udtalelser

https://github.com/user-attachments/assets/cc4aa598-a7e3-4a1d-998c-9f2ba4b4c66e

## Hvorfor

- 🤖 **AI-agenter ødelægger ting stille.** Kode fjernet. Logik ændret. Edge cases væk. Du opdager det først i produktion.
- 🔍 **Fang det før det ships.** AI-drevne inline-kommentarer viser _præcis_ hvad der ændredes og hvad der ser forkert ud.
- 🔁 **Byg en vane, ship bedre kode.** Regelmæssig review → færre bugs → mere robust kode → bedre resultater i dit team.
- 🔗 **Hvorfor git?** Git er universelt. Hver editor, hvert IDE, hvert AI-værktøj bruger det. At committe er obligatorisk. Så der er _næsten ingen chance for at gå glip af en review_ — uanset din stack.

## Kom i gang

### Installation

**Linux / macOS:**

```bash
curl -fsSL https://hexmos.com/lrc-install.sh | sudo bash
```

**Windows (PowerShell):**

```powershell
iwr -useb https://hexmos.com/lrc-install.ps1 | iex
```

Binær installeret. Hooks sat globalt. Færdig.

### Opsætning

```bash
git lrc setup
```

Her er en kort video af, hvordan opsætningen fungerer:

https://github.com/user-attachments/assets/392a4605-6e45-42ad-b2d9-6435312444b5

To trin, begge åbnes i din browser:

1. **LiveReview API-nøgle** — log ind med Hexmos
2. **Gratis Gemini API-nøgle** — hent én fra Google AI Studio

**~1 minut. Engangsopsætning, maskinbred.** Derefter udløser _hvert git-repo_ på din maskine review ved commit. Ingen per-repo-konfiguration nødvendig.

## Sådan virker det

### Mulighed A: Review ved commit (automatisk)

```bash
git add .
git commit -m "add payment validation"
# review launches automatically before the commit goes through
```

### Mulighed B: Review før commit (manuel)

```bash
git add .
git lrc review          # run AI review first
# or: git lrc review --vouch   # vouch personally, skip AI
# or: git lrc review --skip    # skip review entirely
git commit -m "add payment validation"
```

Uanset hvad åbnes et web-UI i din browser.

https://github.com/user-attachments/assets/ae063e39-379f-4815-9954-f0e2ab5b9cde

### Review-UI’et

- 📄 **GitHub-style diff** — farvekodede tilføjelser/sletninger
- 💬 **Inline AI-kommentarer** — på de præcise linjer der betyder noget, med severity-badges
- 📝 **Review-opsummering** — overordnet overblik over hvad AI fandt
- 📁 **Staged fil-liste** — se alle staged filer med et blik, spring mellem dem
- 📊 **Diff-opsummering** — linjer tilføjet/fjernet per fil for hurtig fornemmelse af ændringsomfang
- 📋 **Kopier issues** — ét klik for at kopiere alle AI-flagrede issues, klar til at indsætte tilbage i din AI-agent
- 🔄 **Gennemgå issues** — naviger mellem kommentarer én ad gangen uden at scrolle
- 📜 **Eventlog** — spor review-events, iterationer og statusændringer ét sted

https://github.com/user-attachments/assets/b579d7c6-bdf6-458b-b446-006ca41fe47d

### Beslutningen

| Action               | What happens                           |
| -------------------- | -------------------------------------- |
| ✅ **Commit**        | Accept and commit the reviewed changes |
| 🚀 **Commit & Push** | Commit and push to remote in one step  |
| ⏭️ **Skip**          | Abort the commit — go fix issues first |

```
📎 Screenshot: Pre-commit bar showing Commit / Commit & Push / Skip buttons
```

## Review-cyklussen

Typisk workflow med AI-genereret kode:

1. **Generer kode** med din AI-agent
2. **`git add .` → `git lrc review`** — AI flagger issues
3. **Kopier issues, giv dem tilbage** til din agent til rettelse
4. **`git add .` → `git lrc review`** — AI reviewer igen
5. Gentag indtil tilfreds
6. **`git lrc review --vouch`** → **`git commit`** — du voucher og committer

Hver `git lrc review` er en **iteration**. Værktøjet tracker hvor mange iterationer du lavede og hvor stor en del af diff’en der blev AI-reviewet (**coverage**).

### Vouch

Når du har itereret nok og er tilfreds med koden:

```bash
git lrc review --vouch
```

Det betyder: _“Jeg har gennemgået dette — via AI-iterationer eller personligt — og tager ansvar.”_ Ingen AI-review kører, men coverage-statistik fra tidligere iterationer registreres.

### Skip

Vil du bare committe uden review eller ansvarserklæring?

```bash
git lrc review --skip
```

Ingen AI-review. Ingen personlig attestation. Git-loggen vil registrere `skipped`.

## Git Log-sporing

Hver commit får en **review-statuslinje** tilføjet sin git-log-besked:

```
LiveReview Pre-Commit Check: ran (iter:3, coverage:85%)
```

```
LiveReview Pre-Commit Check: vouched (iter:2, coverage:50%)
```

```
LiveReview Pre-Commit Check: skipped
```

- **`iter`** — antal review-cykler før commit. `iter:3` = tre runder review → fix → review.
- **`coverage`** — procentdel af den endelige diff allerede AI-reviewet i tidligere iterationer. `coverage:85%` = kun 15 % af koden er ugennemgået.

Dit team ser _præcis_ hvilke commits der blev reviewet, vouchet eller sprunget over — direkte i `git log`.

## FAQ

### Review vs Vouch vs Skip?

|                       | **Review**                  | **Vouch**                       | **Skip**                  |
| --------------------- | --------------------------- | ------------------------------- | ------------------------- |
| AI reviews the diff?  | ✅ Yes                      | ❌ No                           | ❌ No                     |
| Takes responsibility? | ✅ Yes                      | ✅ Yes, explicitly              | ⚠️ No                     |
| Tracks iterations?    | ✅ Yes                      | ✅ Records prior coverage       | ❌ No                     |
| Git log message       | `ran (iter:N, coverage:X%)` | `vouched (iter:N, coverage:X%)` | `skipped`                 |
| When to use           | Each review cycle           | Done iterating, ready to commit | Not reviewing this commit |

**Review** er standard. AI analyserer din staged diff og giver inline-feedback. Hver review er én iteration i ændring–review-cyklussen.

**Vouch** betyder at du _eksplicit tager ansvar_ for denne commit. Typisk brugt efter flere review-iterationer — du har gået frem og tilbage, rettet issues og er nu tilfreds. AI kører ikke igen, men dine tidligere iterations- og coverage-statistikker registreres.

**Skip** betyder at du ikke reviewer denne konkrete commit. Måske er den triviel, måske er den ikke kritisk — årsagen er din. Git-loggen registrerer blot `skipped`.

### Hvordan er det gratis?

`git-lrc` bruger **Googles Gemini API** til AI-reviews. Gemini tilbyder en generøs gratis tier. Du medbringer din egen API-nøgle — der er ingen mellemmand-fakturering. LiveReview cloud-tjenesten der koordinerer reviews er gratis for individuelle udviklere.

### Hvilke data sendes?

Kun den **staged diff** analyseres. Ingen fuld repository-kontekst uploades, og diffs gemmes ikke efter review.

### Kan jeg deaktivere det for et bestemt repo?

```bash
git lrc hooks disable   # disable for current repo
git lrc hooks enable    # re-enable later
```

### Kan jeg reviewe en ældre commit?

```bash
git lrc review --commit HEAD       # review the last commit
git lrc review --commit HEAD~3..HEAD  # review a range
```

## Hurtig reference

| Command                    | Description                                   |
| -------------------------- | --------------------------------------------- |
| `lrc` or `lrc review`      | Review staged changes                         |
| `lrc review --vouch`       | Vouch — skip AI, take personal responsibility |
| `lrc review --skip`        | Skip review for this commit                   |
| `lrc review --commit HEAD` | Review an already-committed change            |
| `lrc hooks disable`        | Disable hooks for current repo                |
| `lrc hooks enable`         | Re-enable hooks for current repo              |
| `lrc hooks status`         | Show hook status                              |
| `lrc self-update`          | Update to latest version                      |
| `lrc version`              | Show version info                             |

> **Tip:** `git lrc <command>` og `lrc <command>` er udskiftelige.

## Det er gratis. Del det.

`git-lrc` er **helt gratis.** Ingen kreditkort. Ingen prøveperiode. Ingen fangst.

Hvis det hjælper dig — **del det med dine udviklervenner.** Jo flere der reviewer AI-genereret kode, jo færre bugs når frem til produktion.

⭐ **[Giv denne repo en stjerne](https://github.com/HexmosTech/git-lrc)** så andre kan opdage den.

## Licens

`git-lrc` distribueres under en modificeret variant af **Sustainable Use License (SUL)**.

> [!NOTE]
>
> **Det betyder:**
>
> - ✅ **Source Available** — Fuld kildekode er tilgængelig til self-hosting
> - ✅ **Business Use Allowed** — Brug LiveReview til dine interne forretningsoperationer
> - ✅ **Modifications Allowed** — Tilpas til eget brug
> - ❌ **No Resale** — Må ikke videresælges eller tilbydes som konkurrerende service
> - ❌ **No Redistribution** — Må ikke redistribueres modificerede versioner kommercielt
>
> Licensen sikrer at LiveReview forbliver bæredygtig samtidig med at du får fuld adgang til at self-host og tilpasse efter behov.

For detaljerede vilkår, eksempler på tilladte og forbudte brug og definitioner, se den fulde
[LICENSE.md](LICENSE.md).

---

## For teams: LiveReview

> Bruger du `git-lrc` solo? Fint. Bygger du med et team? Tjek **[LiveReview](https://hexmos.com/livereview)** — det fulde sæt til teambred AI-code review med dashboards, org-niveau-politikker og review-analytics. Alt hvad `git-lrc` gør, plus teamkoordination.
