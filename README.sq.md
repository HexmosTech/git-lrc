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

Agjentët e AI shkruajnë kodin shpejt. Ata gjithashtu _heqin logjikën në heshtje_, ndryshojnë sjelljen dhe sjellin gabime — pa ju thënë. Shpesh e zbuloni në prodhim.

**`git-lrc` e rregullon këtë.** Lidhet me `git commit` dhe rishikon çdo ndryshim _para_ se të hyjë. Konfigurim 60 sekondash. Plotësisht falas.

## Shikojeni Në Veprim

> Shikoni git-lrc duke kapur probleme serioze sigurie si kredencialë të zbuluar, operacione të shtrenjta në cloud
> dhe material të ndjeshëm në deklaratat e log

https://github.com/user-attachments/assets/cc4aa598-a7e3-4a1d-998c-9f2ba4b4c66e

## Pse

- 🤖 **Agjentët e AI thyejnë gjëra në heshtje.** Kod i hequr. Logjikë e ndryshuar. Raste skajore zhdukur. Nuk do ta vini re deri në prodhim.
- 🔍 **Kapeni para se të dërgohet.** Komente të brendshme me AI ju tregojnë _saktësisht_ çfarë ndryshoi dhe çfarë duket gabim.
- 🔁 **Ndërtoni një zakon, dërgojeni kod më të mirë.** Rishikim i rregullt → më pak gabime → kod më i qëndrueshëm → rezultate më të mira në ekipin tuaj.
- 🔗 **Pse git?** Git është universal. Çdo redaktor, çdo IDE, çdo mjet AI e përdor. Commit-i është i detyrueshëm. Pra ka _pothuajse zero mundësi të humbni një rishikim_ — pavarësisht nga steki juaj.

## Filloni

### Instalimi

**Linux / macOS:**

```bash
curl -fsSL https://hexmos.com/lrc-install.sh | sudo bash
```

**Windows (PowerShell):**

```powershell
iwr -useb https://hexmos.com/lrc-install.ps1 | iex
```

Binar i instaluar. Hooks të konfiguruar globalisht. Gati.

### Konfigurimi

```bash
git lrc setup
```

Këtu është një video e shkurtër se si funksionon konfigurimi:

https://github.com/user-attachments/assets/392a4605-6e45-42ad-b2d9-6435312444b5

Dy hapa, të dyja hapen në shfletuesin tuaj:

1. **Çelësi API LiveReview** — identifikohu me Hexmos
2. **Çelësi falas Gemini API** — merrni një nga Google AI Studio

**~1 minutë. Konfigurim një herë, për të gjithë makinën.** Pas kësaj, _çdo repo git_ në makinën tuaj nxit rishikim në commit. Nuk nevojitet konfigurim për repo.

## Si Funksionon

### Opsioni A: Rishikim në commit (automatik)

```bash
git add .
git commit -m "add payment validation"
# review launches automatically before the commit goes through
```

### Opsioni B: Rishikim para commit (manual)

```bash
git add .
git lrc review          # run AI review first
# or: git lrc review --vouch   # vouch personally, skip AI
# or: git lrc review --skip    # skip review entirely
git commit -m "add payment validation"
```

Sido që të jetë, një ndërfaqe web hapet në shfletuesin tuaj.

https://github.com/user-attachments/assets/ae063e39-379f-4815-9954-f0e2ab5b9cde

### Ndërfaqja e Rishikimit

- 📄 **Diff në stilin GitHub** — shtesa/fshirje me ngjyra
- 💬 **Komente AI të brendshme** — në rreshtat e saktë që kanë rëndësi, me badge severiteti
- 📝 **Përmbledhje rishikimi** — pamje e përgjithshme e asaj që gjeti AI
- 📁 **Lista e skedarëve të përgatitur** — shikoni të gjithë skedarët e përgatitur menjëherë, kërceni ndërmjet tyre
- 📊 **Përmbledhje diff** — rreshta të shtuar/hequr për skedar për një kuptim të shpejtë të hapësirës së ndryshimit
- 📋 **Kopjoni problemet** — një klik për të kopjuar të gjitha problemet e shënuara nga AI, gati për t'u ngjitur përsëri te agjenti juaj AI
- 🔄 **Kaloni ndërmjet problemeve** — lundroni ndërmjet komenteve një nga një pa scroll
- 📜 **Regjistri i ngjarjeve** — ndiqni ngjarjet e rishikimit, përsëritjet dhe ndryshimet e statusit në një vend

https://github.com/user-attachments/assets/b579d7c6-bdf6-458b-b446-006ca41fe47d

### Vendimi

| Action               | What happens                           |
| -------------------- | -------------------------------------- |
| ✅ **Commit**        | Accept and commit the reviewed changes |
| 🚀 **Commit & Push** | Commit and push to remote in one step  |
| ⏭️ **Skip**          | Abort the commit — go fix issues first |

```
📎 Screenshot: Pre-commit bar showing Commit / Commit & Push / Skip buttons
```

## Cikli i Rishikimit

Rrjedha tipike me kod të gjeneruar nga AI:

1. **Gjeneroni kod** me agjentin tuaj AI
2. **`git add .` → `git lrc review`** — AI shënon problemet
3. **Kopjoni problemet, jepini përsëri** agjentin tuaj për t'i rregulluar
4. **`git add .` → `git lrc review`** — AI rishikon përsëri
5. Përsëriteni deri sa të jeni të kënaqur
6. **`git lrc review --vouch`** → **`git commit`** — ju garantoni dhe bëni commit

Çdo `git lrc review` është një **përsëritje**. Mjeti ndjek sa përsëritje keni bërë dhe çfarë përqindje të diff-it u rishikua nga AI (**mbulimi**).

### Vouch

Pasi të keni përsëritur mjaft dhe jeni të kënaqur me kodin:

```bash
git lrc review --vouch
```

Kjo thotë: _"E kam rishikuar këtë — përmes përsëritjeve AI ose personalisht — dhe marr përgjegjësi."_ Nuk ekzekutohet rishikim AI, por statistikat e mbulimit nga përsëritjet e mëparshme regjistrohen.

### Skip

Thjesht doni të bëni commit pa rishikim ose dëshmi përgjegjësie?

```bash
git lrc review --skip
```

Pa rishikim AI. Pa dëshmi personale. Regjistri git do të regjistrojë `skipped`.

## Ndalimi i Git Log

Çdo commit merr një **rresht statusi rishikimi** të shtuar në mesazhin e git log:

```
LiveReview Pre-Commit Check: ran (iter:3, coverage:85%)
```

```
LiveReview Pre-Commit Check: vouched (iter:2, coverage:50%)
```

```
LiveReview Pre-Commit Check: skipped
```

- **`iter`** — numri i cikleve të rishikimit para commit-it. `iter:3` = tre raunde rishikim → rregullim → rishikim.
- **`coverage`** — përqindja e diff-it përfundimtar tashmë i rishikuar nga AI në përsëritjet e mëparshme. `coverage:85%` = vetëm 15% e kodit nuk është e rishikuar.

Ekipi juaj sheh _saktësisht_ cilët commit u rishikuan, u garantuan ose u kaluan — drejt në `git log`.

## FAQ

### Review vs Vouch vs Skip?

|                       | **Review**                  | **Vouch**                       | **Skip**                  |
| --------------------- | --------------------------- | ------------------------------- | ------------------------- |
| AI reviews the diff?  | ✅ Yes                      | ❌ No                           | ❌ No                     |
| Takes responsibility? | ✅ Yes                      | ✅ Yes, explicitly              | ⚠️ No                     |
| Tracks iterations?    | ✅ Yes                      | ✅ Records prior coverage       | ❌ No                     |
| Git log message       | `ran (iter:N, coverage:X%)` | `vouched (iter:N, coverage:X%)` | `skipped`                 |
| When to use           | Each review cycle           | Done iterating, ready to commit | Not reviewing this commit |

**Review** është parazgjedhja. AI analizon diff-in tuaj të përgatitur dhe jep reagime të brendshme. Çdo rishikim është një përsëritje në ciklin ndryshim–rishikim.

**Vouch** do të thotë që _marrni qartë përgjegjësi_ për këtë commit. Zakonisht përdoret pas shumë përsëritjeve rishikimi — keni shkuar e ardhur, keni rregulluar problemet dhe tani jeni të kënaqur. AI nuk ekzekutohet përsëri, por statistikat tuaja të mëparshme të përsëritjes dhe mbulimit regjistrohen.

**Skip** do të thotë që nuk po rishikoni këtë commit të veçantë. Ndoshta është i parëndësishëm, ndoshta nuk është kritik — arsyeja është e juaja. Regjistri git thjesht regjistron `skipped`.

### Si është falas?

`git-lrc` përdor **Google's Gemini API** për rishikime AI. Gemini ofron një nivel falas bujar. Ju sillni çelësin tuaj API — nuk ka faturim ndërmjetës. Shërbimi LiveReview në cloud që koordinon rishikimet është falas për zhvillues individualë.

### Çfarë të dhënash dërgohen?

Vetëm **diff-i i përgatitur** analizohet. Nuk ngarkohet kontekst i plotë i depozitës dhe diff-et nuk ruhen pas rishikimit.

### A mund ta çaktivizoj për një repo të caktuar?

```bash
git lrc hooks disable   # disable for current repo
git lrc hooks enable    # re-enable later
```

### A mund të rishikoj një commit më të vjetër?

```bash
git lrc review --commit HEAD       # review the last commit
git lrc review --commit HEAD~3..HEAD  # review a range
```

## Referencë e Shpejtë

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

> **Këshillë:** `git lrc <command>` dhe `lrc <command>` janë të këmbyeshme.

## Është Falas. Ndajeni.

`git-lrc` është **plotësisht falas.** Pa kartë krediti. Pa provë. Pa kurth.

Nëse ju ndihmon — **ndajeni me miqtë tuaj zhvillues.** Sa më shumë njerëz rishikojnë kod të gjeneruar nga AI, aq më pak gabime arrijnë në prodhim.

⭐ **[Dukeju një yll këtij repo](https://github.com/HexmosTech/git-lrc)** për të ndihmuar të tjerët ta zbulojnë.

## Licensa

`git-lrc` shpërndahet sipas një varianti të modifikuar të **Sustainable Use License (SUL)**.

> [!NOTE]
>
> **Çfarë do të thotë kjo:**
>
> - ✅ **Source Available** — Kodi burimor i plotë është i disponueshëm për vetë-hosting
> - ✅ **Business Use Allowed** — Përdorni LiveReview për operacionet e brendshme të biznesit
> - ✅ **Modifications Allowed** — Përshtatni për përdorimin tuaj
> - ❌ **No Resale** — Nuk mund të rishitet ose të ofrohet si shërbim konkurrues
> - ❌ **No Redistribution** — Nuk mund të rishpërndahen versionet e modifikuara komercialisht
>
> Kjo licensë siguron që LiveReview mbetet i qëndrueshëm duke ju dhënë akses të plotë për vetë-hosting dhe përshtatje sipas nevojave tuaja.

Për kushte të hollësishme, shembuj përdorimesh të lejuara dhe të ndaluara dhe përkufizime, shihni
[LICENSE.md](LICENSE.md).

---

## Për Ekipet: LiveReview

> Duke përdorur `git-lrc` vetëm? Mirë. Po ndërtoni me një ekip? Shikoni **[LiveReview](https://hexmos.com/livereview)** — paketa e plotë për rishikim AI të kodit në të gjithë ekipin, me panele, politika në nivel organizate dhe analitikë rishikimi. Gjithçka që bën `git-lrc`, plus koordinim ekipi.
