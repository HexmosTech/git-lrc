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

AI-agentit kirjoittavat koodia nopeasti. Ne myös _poistavat logiikan hiljaa_, muuttavat käyttäytymistä ja tuovat bugeja — kertomatta sinulle. Usein huomaat vasta tuotannossa.

**`git-lrc` korjaa tämän.** Se kytkeytyy `git commit`-iin ja tarkistaa jokaisen diffin _ennen_ kuin se menee läpi. 60 sekunnin asennus. Täysin ilmainen.

## Katso käytännössä

> Katso git-lrcin tunnistavan vakavia turvallisuusongelmia kuten vuotaneet tunnukset, kalliit pilvi-
> operaatiot ja arkaluonteinen materiaali lokiviestissä

https://github.com/user-attachments/assets/cc4aa598-a7e3-4a1d-998c-9f2ba4b4c66e

## Miksi

- 🤖 **AI-agentit rikkovat asioita hiljaa.** Koodia poistettu. Logiikka muuttunut. Reunatapaukset poissa. Huomaat vasta tuotannossa.
- 🔍 **Tartu siihen ennen julkaisua.** AI-pohjaiset rivikommentit näyttävät _tarkalleen_ mitä muuttui ja mikä näyttää vialliselta.
- 🔁 **Rakenna tapa, julkaise parempaa koodia.** Säännöllinen tarkastus → vähemmän bugeja → vankempi koodi → paremmat tulokset tiimissäsi.
- 🔗 **Miksi git?** Git on yleismaailmallinen. Jokainen editori, jokainen IDE, jokainen AI-työkalu käyttää sitä. Commit on pakollinen. Joten _lähes mahdotonta ohittaa tarkastusta_ — pinosta riippumatta.

## Aloita

### Asennus

**Linux / macOS:**

```bash
curl -fsSL https://hexmos.com/lrc-install.sh | sudo bash
```

**Windows (PowerShell):**

```powershell
iwr -useb https://hexmos.com/lrc-install.ps1 | iex
```

Binaari asennettu. Hookit asetettu globaalisti. Valmista.

### Asetukset

```bash
git lrc setup
```

Tässä pikavideo asetuksen toiminnasta:

https://github.com/user-attachments/assets/392a4605-6e45-42ad-b2d9-6435312444b5

Kaksi vaihetta, molemmat avautuvat selaimessasi:

1. **LiveReview API-avain** — kirjaudu Hexmoksella
2. **Ilmainen Gemini API-avain** — hae sellainen Google AI Studiosta

**~1 minuutti. Kerta-asetus, koneen laajuinen.** Tämän jälkeen _jokainen git-repo_ koneellasi laukaisee tarkastuksen commitissa. Ei per-repo-määritystä tarvita.

## Miten se toimii

### Vaihtoehto A: Tarkastus commitissa (automaattinen)

```bash
git add .
git commit -m "add payment validation"
# review launches automatically before the commit goes through
```

### Vaihtoehto B: Tarkastus ennen committia (manuaalinen)

```bash
git add .
git lrc review          # run AI review first
# or: git lrc review --vouch   # vouch personally, skip AI
# or: git lrc review --skip    # skip review entirely
git commit -m "add payment validation"
```

Kummassakin tapauksessa web-käyttöliittymä avautuu selaimessasi.

https://github.com/user-attachments/assets/ae063e39-379f-4815-9954-f0e2ab5b9cde

### Tarkastuskäyttöliittymä

- 📄 **GitHub-tyylinen diff** — värikoodatut lisäykset/poistot
- 💬 **Rivikommentit AI:lta** — tarkasti oikeilla riveillä, vakavuusmerkinnöillä
- 📝 **Tarkastuksen yhteenveto** — yleiskuva siitä mitä AI löysi
- 📁 **Staged-tiedostolista** — näe kaikki staged-tiedostot yhdellä silmäyksellä, hyppää niiden välillä
- 📊 **Diff-yhteenveto** — lisätyt/poistetut rivit per tiedosto nopeaan muutoksen laajuuden tuntemukseen
- 📋 **Kopioi ongelmat** — yhdellä klikkauksella kopioi kaikki AI:n liputtamat ongelmat, valmiina liitettäväksi takaisin AI-agenttiisi
- 🔄 **Selaile ongelmia** — navigoi kommenttien välillä yksi kerrallaan ilman scrollausta
- 📜 **Tapahtumaloki** — seuraa tarkastustapahtumia, iteraatioita ja tilan muutoksia yhdessä paikassa

https://github.com/user-attachments/assets/b579d7c6-bdf6-458b-b446-006ca41fe47d

### Päätös

| Action               | What happens                           |
| -------------------- | -------------------------------------- |
| ✅ **Commit**        | Accept and commit the reviewed changes |
| 🚀 **Commit & Push** | Commit and push to remote in one step  |
| ⏭️ **Skip**          | Abort the commit — go fix issues first |

```
📎 Screenshot: Pre-commit bar showing Commit / Commit & Push / Skip buttons
```

## Tarkastussykli

Tyypillinen työnkulku AI-generoidulla koodilla:

1. **Generoi koodi** AI-agentillasi
2. **`git add .` → `git lrc review`** — AI liputtaa ongelmat
3. **Kopioi ongelmat, palauta ne** agentillesi korjattavaksi
4. **`git add .` → `git lrc review`** — AI tarkastaa uudelleen
5. Toista kunnes tyytyväinen
6. **`git lrc review --vouch`** → **`git commit`** — vahvistat ja commitoit

Jokainen `git lrc review` on yksi **iteraatio**. Työkalu seuraa kuinka monta iteraatiota teit ja kuinka suuri osa diffistä AI tarkasti (**coverage**).

### Vouch

Kun olet iteroinut tarpeeksi ja olet tyytyväinen koodiin:

```bash
git lrc review --vouch
```

Tämä tarkoittaa: _"Olen tarkastanut tämän — AI-iteraatioilla tai itse — ja otan vastuun."_ AI-tarkastusta ei ajeta, mutta aiempien iteraatioiden coverage-tilastot tallennetaan.

### Skip

Haluatko vain commitoida ilman tarkastusta tai vastuunottoa?

```bash
git lrc review --skip
```

Ei AI-tarkastusta. Ei henkilökohtaista vahvistusta. Git-loki tallentaa `skipped`.

## Git-lokin seuranta

Jokainen commit saa **tarkastustilarivin** liitettynä git-lokiviestiinsä:

```
LiveReview Pre-Commit Check: ran (iter:3, coverage:85%)
```

```
LiveReview Pre-Commit Check: vouched (iter:2, coverage:50%)
```

```
LiveReview Pre-Commit Check: skipped
```

- **`iter`** — tarkastussyklisten määrä ennen committia. `iter:3` = kolme kierrosta tarkastus → korjaus → tarkastus.
- **`coverage`** — osuus lopullisesta diffistä, jonka AI jo tarkasti aiemmissa iteraatioissa. `coverage:85%` = vain 15 % koodista on tarkastamatta.

Tiimisi näkee _tarkalleen_ mitkä commitit tarkastettiin, vahvistettiin tai ohitettiin — suoraan `git log`issa.

## FAQ

### Review vs Vouch vs Skip?

|                       | **Review**                  | **Vouch**                       | **Skip**                  |
| --------------------- | --------------------------- | ------------------------------- | ------------------------- |
| AI reviews the diff?  | ✅ Yes                      | ❌ No                           | ❌ No                     |
| Takes responsibility? | ✅ Yes                      | ✅ Yes, explicitly              | ⚠️ No                     |
| Tracks iterations?    | ✅ Yes                      | ✅ Records prior coverage       | ❌ No                     |
| Git log message       | `ran (iter:N, coverage:X%)` | `vouched (iter:N, coverage:X%)` | `skipped`                 |
| When to use           | Each review cycle           | Done iterating, ready to commit | Not reviewing this commit |

**Review** on oletus. AI analysoi staged-diffisi ja antaa rivikommentteja. Jokainen tarkastus on yksi iteraatio muutos–tarkastus-syklyssä.

**Vouch** tarkoittaa että _otat nimenomaan vastuun_ tästä commitista. Tyypillisesti usean tarkastusiteraation jälkeen — olet käynyt edestakaisin, korjannut ongelmat ja olet nyt tyytyväinen. AI ei aja uudelleen, mutta aiemmat iteraatio- ja coverage-tilastot tallennetaan.

**Skip** tarkoittaa ettei tarkasteta tätä commitia. Ehkä se on triviaali, ehkä ei kriittinen — syy on sinun. Git-loki tallentaa vain `skipped`.

### Miten tämä on ilmaista?

`git-lrc` käyttää **Googlen Gemini APIa** AI-tarkastuksiin. Gemini tarjoaa runsaan ilmaisen tason. Tuot oman API-avaimesi — ei välikäden laskutusta. Tarkastuksia koordinoiva LiveReview-pilvipalvelu on ilmainen yksittäisille kehittäjille.

### Mitä tietoja lähetetään?

Vain **staged diff** analysoidaan. Koko repositorion kontekstia ei lähetetä, eikä diffejä tallenneta tarkastuksen jälkeen.

### Voinko poistaa sen käytöstä tietylle repolle?

```bash
git lrc hooks disable   # disable for current repo
git lrc hooks enable    # re-enable later
```

### Voinko tarkastaa vanhemman commitin?

```bash
git lrc review --commit HEAD       # review the last commit
git lrc review --commit HEAD~3..HEAD  # review a range
```

## Pikaviite

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

> **Vinkki:** `git lrc <command>` ja `lrc <command>` ovat vaihtokelpoisia.

## Se on ilmainen. Jaa se.

`git-lrc` on **täysin ilmainen.** Ei luottokorttia. Ei kokeilua. Ei koukkuja.

Jos se auttaa sinua — **jaa kehittäjäystävillesi.** Mitä enemmän ihmisiä tarkastaa AI-generoitua koodia, sitä vähemmän bugeja päätyy tuotantoon.

⭐ **[Anna tälle repolle tähden](https://github.com/HexmosTech/git-lrc)** auttaaksesi muita löytämään sen.

## Lisenssi

`git-lrc` on jaettu **Sustainable Use License (SUL)** -lisenssin muokatun version alaisena.

> [!NOTE]
>
> **Mitä tämä tarkoittaa:**
>
> - ✅ **Source Available** — Täysi lähdekoodi on saatavilla omalle isännöinnille
> - ✅ **Business Use Allowed** — Käytä LiveReviewia sisäisiin liiketoimintatoimiin
> - ✅ **Modifications Allowed** — Mukauta omaan käyttöön
> - ❌ **No Resale** — Ei saa myydä eteenpäin tai tarjota kilpailevana palveluna
> - ❌ **No Redistribution** — Muokattuja versioita ei saa jakaa kaupallisesti
>
> Tämä lisenssi varmistaa että LiveReview pysyy kestävässä käytössä ja antaa sinulle täyden mahdollisuuden hostata ja mukauttaa tarpeidesi mukaan.

Yksityiskohtaiset ehdot, sallittujen ja kiellettyjen käyttötapojen esimerkit sekä määritelmät: [LICENSE.md](LICENSE.md).

---

## Tiimeille: LiveReview

> Käytätkö `git-lrc`:ää soolona? Hienoa. Rakennatko tiimin kanssa? Tutustu **[LiveReview](https://hexmos.com/livereview)** — koko paketti tiimin laajuiseen AI-kooditarkastukseen, dashbordeineen, org-tason käytäntöineen ja tarkastusanalytiikalla. Kaikki mitä `git-lrc` tekee, plus tiimikoordinointi.
