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

AI-agentar skriv kode raskt. Dei _fjernar også logikk stille_, endrar oppførsel og introduserer buggar — utan å fortelje deg. Du oppdagar det ofte først i produksjon.

**`git-lrc` løyser det.** Det koplar seg til `git commit` og gjennomgår kvar diff _før_ han landar. 60 sekund oppsett. Heilt gratis.

## Sjå det i praksis

> Sjå git-lrc fange alvorlege sikkerheitsproblem som lekte credentials, dyre sky-
> operasjonar og sensitivt materiale i loggmeldingar

https://github.com/user-attachments/assets/cc4aa598-a7e3-4a1d-998c-9f2ba4b4c66e

## Kvifor

- 🤖 **AI-agentar øydelegg ting stille.** Kode fjerna. Logikk endra. Edge cases borte. Du merkar det først i produksjon.
- 🔍 **Fang det før det shipar.** AI-drevne inline-kommentarar viser _nøyaktig_ kva som endra seg og kva som ser gale ut.
- 🔁 **Bygg ein vane, ship betre kode.** Regelmessig review → færre buggar → meir robust kode → betre resultat i teamet ditt.
- 🔗 **Kvifor git?** Git er universelt. Kvar editor, kvar IDE, kvar AI-verktøy brukar det. Å committe er obligatorisk. Så det er _nesten ingen sjanse for å misse ein review_ — uansett stack.

## Kom i gang

### Installasjon

**Linux / macOS:**

```bash
curl -fsSL https://hexmos.com/lrc-install.sh | sudo bash
```

**Windows (PowerShell):**

```powershell
iwr -useb https://hexmos.com/lrc-install.ps1 | iex
```

Binær installert. Hooks satt globalt. Ferdig.

### Oppsett

```bash
git lrc setup
```

Her er ein kort video av korleis oppsettet fungerer:

https://github.com/user-attachments/assets/392a4605-6e45-42ad-b2d9-6435312444b5

To steg, begge opnar i nettlesaren din:

1. **LiveReview API-nøkkel** — logg inn med Hexmos
2. **Gratis Gemini API-nøkkel** — hent ein frå Google AI Studio

**~1 minutt. Ein gangs oppsett, maskinvid.** Etterpå utløyser _kvar git-repo_ på maskina di review ved commit. Ingen per-repo-oppsett nødvendig.

## Korleis det fungerer

### Val A: Review ved commit (automatisk)

```bash
git add .
git commit -m "add payment validation"
# review launches automatically before the commit goes through
```

### Val B: Review før commit (manuell)

```bash
git add .
git lrc review          # run AI review first
# or: git lrc review --vouch   # vouch personally, skip AI
# or: git lrc review --skip    # skip review entirely
git commit -m "add payment validation"
```

Uansett opnar eit web-UI i nettlesaren din.

https://github.com/user-attachments/assets/ae063e39-379f-4815-9954-f0e2ab5b9cde

### Review-UIet

- 📄 **GitHub-style diff** — fargekoda tillegg/slettingar
- 💬 **Inline AI-kommentarar** — på dei nøyaktige linjene som matter, med severity-merke
- 📝 **Review-samandrag** — oversyn på høgt nivå av kva AI fann
- 📁 **Staged fil-liste** — sjå alle staged filer med eitt blikk, hopp mellom dei
- 📊 **Diff-samandrag** — linjer lagt til/fjerna per fil for rask kjensle av endringsomfang
- 📋 **Kopier issue** — eitt klikk for å kopiere alle AI-flagga issue, klare til å limast tilbake i AI-agenten din
- 🔄 **Sykl gjennom issue** — naviger mellom kommentarar eine om gangen utan å scrolle
- 📜 **Händlingslogg** — spor review-hendingar, iterasjonar og statusendringar på eitt staden

https://github.com/user-attachments/assets/b579d7c6-bdf6-458b-b446-006ca41fe47d

### Avgjerda

| Action               | What happens                           |
| -------------------- | -------------------------------------- |
| ✅ **Commit**        | Accept and commit the reviewed changes |
| 🚀 **Commit & Push** | Commit and push to remote in one step  |
| ⏭️ **Skip**          | Abort the commit — go fix issues first |

```
📎 Screenshot: Pre-commit bar showing Commit / Commit & Push / Skip buttons
```

## Review-syklusen

Typisk arbeidsflyt med AI-generert kode:

1. **Generer kode** med AI-agenten din
2. **`git add .` → `git lrc review`** — AI flaggar issue
3. **Kopier issue, gje dei tilbake** til agenten din for retting
4. **`git add .` → `git lrc review`** — AI reviewer igjen
5. Gjenta til du er nøgd
6. **`git lrc review --vouch`** → **`git commit`** — du voucher og committar

Kvar `git lrc review` er éin **iterasjon**. Verktøyet spor kor mange iterasjonar du gjorde og kor stor del av diffen som vart AI-reviewa (**coverage**).

### Vouch

Når du har iterert nok og er nøgd med koden:

```bash
git lrc review --vouch
```

Det seier: _“Eg har gjennomgått dette — via AI-iterasjonar eller personleg — og tek ansvar.”_ Ingen AI-review køyrer, men coverage-statistikk frå tidlegare iterasjonar vert registrert.

### Skip

Vil du berre committe utan review eller ansvarsattestasjon?

```bash
git lrc review --skip
```

Ingen AI-review. Ingen personleg attestasjon. Git-loggen vil registrere `skipped`.

## Git log-sporing

Kvar commit får ei **review-statuslinje** lagt til git log-meldinga si:

```
LiveReview Pre-Commit Check: ran (iter:3, coverage:85%)
```

```
LiveReview Pre-Commit Check: vouched (iter:2, coverage:50%)
```

```
LiveReview Pre-Commit Check: skipped
```

- **`iter`** — talet på review-syklar før commit. `iter:3` = tre runder review → fix → review.
- **`coverage`** — prosentdel av den endelege diffen allereie AI-reviewa i tidlegare iterasjonar. `coverage:85%` = berre 15 % av koden er ugjennomgått.

Teamet ditt ser _nøyaktig_ kva for commitar som vart reviewa, voucha eller hoppa over — rett i `git log`.

## FAQ

### Review vs Vouch vs Skip?

|                       | **Review**                  | **Vouch**                       | **Skip**                  |
| --------------------- | --------------------------- | ------------------------------- | ------------------------- |
| AI reviews the diff?  | ✅ Yes                      | ❌ No                           | ❌ No                     |
| Takes responsibility? | ✅ Yes                      | ✅ Yes, explicitly              | ⚠️ No                     |
| Tracks iterations?    | ✅ Yes                      | ✅ Records prior coverage       | ❌ No                     |
| Git log message       | `ran (iter:N, coverage:X%)` | `vouched (iter:N, coverage:X%)` | `skipped`                 |
| When to use           | Each review cycle           | Done iterating, ready to commit | Not reviewing this commit |

**Review** er standard. AI analyserer den stagede diffen din og gjev inline-tilbakemelding. Kvar review er éin iterasjon i endring–review-syklusen.

**Vouch** tyder at du _eksplisitt tek ansvar_ for denne commiten. Typisk brukt etter fleire review-iterasjonar — du har gått fram og tilbake, retta issue og er no nøgd. AI køyrer ikkje igjen, men tidlegare iterasjons- og coverage-statistikk vert registrert.

**Skip** tyder at du ikkje reviewer denne commiten. Kanskje han er triviell, kanskje ikkje kritisk — årsaka er din. Git-loggen registrerer berre `skipped`.

### Korleis er dette gratis?

`git-lrc` brukar **Googles Gemini API** til AI-reviewar. Gemini tilbyr eit raust gratisnivå. Du tek med din eiga API-nøkkel — det er ingen mellommann-fakturering. LiveReview skytenesta som koordinerer reviewar er gratis for individuelle utviklarar.

### Kva data vert sendt?

Berre den **stagede diffen** vert analysert. Ingen full repo-kontekst vert lasta opp, og diffar vert ikkje lagra etter review.

### Kan eg slå det av for eit bestemt repo?

```bash
git lrc hooks disable   # disable for current repo
git lrc hooks enable    # re-enable later
```

### Kan eg reviewe ein eldre commit?

```bash
git lrc review --commit HEAD       # review the last commit
git lrc review --commit HEAD~3..HEAD  # review a range
```

## Snarreferanse

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

> **Tips:** `git lrc <command>` og `lrc <command>` er utskiftbare.

## Det er gratis. Del det.

`git-lrc` er **heilt gratis.** Ingen kredittkort. Ingen prøveperiode. Ingen hake.

Viss det hjelper deg — **del det med utviklarvenene dine.** Jo fleire som reviewer AI-generert kode, jo færre buggar når til produksjon.

⭐ **[Gi denne repo-en ei stjerne](https://github.com/HexmosTech/git-lrc)** for å hjelpe andre å oppdage han.

## Lisens

`git-lrc` vert distribuert under ei modifisert variant av **Sustainable Use License (SUL)**.

> [!NOTE]
>
> **Det tyder:**
>
> - ✅ **Source Available** — Full kjeldekode er tilgjengeleg for sjølvhosting
> - ✅ **Business Use Allowed** — Bruk LiveReview for interne forretningsoperasjonar
> - ✅ **Modifications Allowed** — Tilpass for eige bruk
> - ❌ **No Resale** — Kan ikkje videreseljast eller tilbodast som konkurrerande teneste
> - ❌ **No Redistribution** — Modifiserte versjonar kan ikkje distribuerast kommersielt
>
> Denne lisensen sikrar at LiveReview vert vedvarande samtidig som du får full tilgang til å sjølvhoste og tilpasse etter behov.

For detaljerte vilkår, døme på tillatne og forbodne bruk og definisjonar, sjå heile
[LICENSE.md](LICENSE.md).

---

## For team: LiveReview

> Brukar du `git-lrc` solo? Bra. Byggjer du med eit team? Sjekk **[LiveReview](https://hexmos.com/livereview)** — heile pakka for teamvide AI-kodereview med dashbord, org-nivå-policyar og review-analytikk. Alt `git-lrc` gjer, pluss teamkoordinering.
