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

Los agentes de IA escriben código rápido. También _eliminan lógica en silencio_, cambian el comportamiento e introducen bugs — sin avisarte. A menudo te enteras en producción.

**`git-lrc` lo soluciona.** Se engancha a `git commit` y revisa cada diff _antes_ de que se registre. Configuración en 60 segundos. Completamente gratis.

## Verlo en acción

> Mira cómo git-lrc detecta problemas serios de seguridad como credenciales filtradas, operaciones
> cloud costosas y material sensible en logs

https://github.com/user-attachments/assets/cc4aa598-a7e3-4a1d-998c-9f2ba4b4c66e

## Por qué

- 🤖 **Los agentes de IA rompen cosas en silencio.** Código eliminado. Lógica cambiada. Casos límite perdidos. No te das cuenta hasta producción.
- 🔍 **Cógelo antes de hacer ship.** Los comentarios inline con IA muestran _exactamente_ qué cambió y qué parece mal.
- 🔁 **Crea el hábito, haz ship de mejor código.** Revisión regular → menos bugs → código más robusto → mejores resultados en tu equipo.
- 🔗 **¿Por qué git?** Git es universal. Cualquier editor, cualquier IDE, cualquier toolkit de IA lo usa. Hacer commit es obligatorio. Así que _casi no hay forma de saltarse una revisión_ — sin importar tu stack.

## Empezar

### Instalación

**Linux / macOS:**

```bash
curl -fsSL https://hexmos.com/lrc-install.sh | sudo bash
```

**Windows (PowerShell):**

```powershell
iwr -useb https://hexmos.com/lrc-install.ps1 | iex
```

Binario instalado. Hooks configurados globalmente. Listo.

### Configuración

```bash
git lrc setup
```

Un vídeo rápido de cómo funciona la configuración:

https://github.com/user-attachments/assets/392a4605-6e45-42ad-b2d9-6435312444b5

Dos pasos, ambos se abren en el navegador:

1. **Clave API de LiveReview** — inicia sesión con Hexmos
2. **Clave API gratuita de Gemini** — consigue una en Google AI Studio

**~1 minuto. Configuración única, para toda la máquina.** Después de esto, _cada repo git_ en tu máquina lanza la revisión en cada commit. No hace falta config por repo.

## Cómo funciona

### Opción A: Revisión en el commit (automática)

```bash
git add .
git commit -m "add payment validation"
# review launches automatically before the commit goes through
```

### Opción B: Revisión antes del commit (manual)

```bash
git add .
git lrc review          # run AI review first
# or: git lrc review --vouch   # vouch personally, skip AI
# or: git lrc review --skip    # skip review entirely
git commit -m "add payment validation"
```

En ambos casos se abre una interfaz web en el navegador.

https://github.com/user-attachments/assets/ae063e39-379f-4815-9954-f0e2ab5b9cde

### La interfaz de revisión

- 📄 **Diff estilo GitHub** — adiciones/eliminaciones con color
- 💬 **Comentarios inline de IA** — en las líneas exactas que importan, con badges de severidad
- 📝 **Resumen de la revisión** — visión general de lo que encontró la IA
- 📁 **Lista de archivos en stage** — ve todos los archivos en stage de un vistazo, salta entre ellos
- 📊 **Resumen del diff** — líneas añadidas/eliminadas por archivo para una idea rápida del alcance del cambio
- 📋 **Copiar issues** — un clic para copiar todos los issues marcados por la IA, listos para pegar de vuelta en tu agente de IA
- 🔄 **Recorrer issues** — navegar entre comentarios uno a uno sin scroll
- 📜 **Registro de eventos** — sigue eventos de revisión, iteraciones y cambios de estado en un solo sitio

https://github.com/user-attachments/assets/b579d7c6-bdf6-458b-b446-006ca41fe47d

### La decisión

| Action               | What happens                           |
| -------------------- | -------------------------------------- |
| ✅ **Commit**        | Accept and commit the reviewed changes |
| 🚀 **Commit & Push** | Commit and push to remote in one step  |
| ⏭️ **Skip**          | Abort the commit — go fix issues first |

```
📎 Screenshot: Pre-commit bar showing Commit / Commit & Push / Skip buttons
```

## El ciclo de revisión

Flujo típico con código generado por IA:

1. **Genera código** con tu agente de IA
2. **`git add .` → `git lrc review`** — la IA marca issues
3. **Copia los issues, devuélveselos** al agente para que los corrija
4. **`git add .` → `git lrc review`** — la IA revisa de nuevo
5. Repite hasta quedar satisfecho
6. **`git lrc review --vouch`** → **`git commit`** — tú avalas y haces commit

Cada `git lrc review` es una **iteración**. La herramienta registra cuántas iteraciones hiciste y qué porcentaje del diff fue revisado por la IA (**coverage**).

### Vouch

Cuando hayas iterado lo suficiente y estés satisfecho con el código:

```bash
git lrc review --vouch
```

Esto dice: _"He revisado esto — por iteraciones de IA o en persona — y asumo la responsabilidad."_ No se ejecuta revisión de IA, pero se registran las estadísticas de coverage de iteraciones anteriores.

### Skip

¿Solo quieres hacer commit sin revisión ni attestation de responsabilidad?

```bash
git lrc review --skip
```

Sin revisión de IA. Sin attestation personal. El git log registrará `skipped`.

## Seguimiento en Git Log

Cada commit recibe una **línea de estado de revisión** añadida a su mensaje de git log:

```
LiveReview Pre-Commit Check: ran (iter:3, coverage:85%)
```

```
LiveReview Pre-Commit Check: vouched (iter:2, coverage:50%)
```

```
LiveReview Pre-Commit Check: skipped
```

- **`iter`** — número de ciclos de revisión antes del commit. `iter:3` = tres rondas de revisión → corrección → revisión.
- **`coverage`** — porcentaje del diff final ya revisado por la IA en iteraciones anteriores. `coverage:85%` = solo el 15% del código no está revisado.

Tu equipo ve _exactamente_ qué commits fueron revisados, vouched o skipped — directamente en `git log`.

## FAQ

### ¿Review vs Vouch vs Skip?

|                       | **Review**                  | **Vouch**                       | **Skip**                  |
| --------------------- | --------------------------- | ------------------------------- | ------------------------- |
| AI reviews the diff?  | ✅ Yes                      | ❌ No                           | ❌ No                     |
| Takes responsibility? | ✅ Yes                      | ✅ Yes, explicitly              | ⚠️ No                     |
| Tracks iterations?    | ✅ Yes                      | ✅ Records prior coverage       | ❌ No                     |
| Git log message       | `ran (iter:N, coverage:X%)` | `vouched (iter:N, coverage:X%)` | `skipped`                 |
| When to use           | Each review cycle           | Done iterating, ready to commit | Not reviewing this commit |

**Review** es el valor por defecto. La IA analiza tu diff en stage y da feedback inline. Cada revisión es una iteración en el ciclo cambio–revisión.

**Vouch** significa que _asumes explícitamente la responsabilidad_ de este commit. Típicamente usado tras varias iteraciones de revisión — has ido y venido, corregido issues y estás satisfecho. La IA no se ejecuta de nuevo, pero se registran tus iteraciones y estadísticas de coverage anteriores.

**Skip** significa que no estás revisando este commit. Quizá es trivial, quizá no es crítico — la razón es tuya. El git log simplemente registra `skipped`.

### ¿Cómo es gratis?

`git-lrc` usa la **API Gemini de Google** para las revisiones con IA. Gemini ofrece un tier gratuito generoso. Tú traes tu propia clave API — no hay facturación intermediaria. El servicio en la nube LiveReview que coordina las revisiones es gratis para desarrolladores individuales.

### ¿Qué datos se envían?

Solo se analiza el **diff en stage**. No se sube contexto completo del repositorio y los diffs no se almacenan tras la revisión.

### ¿Puedo desactivarlo para un repo concreto?

```bash
git lrc hooks disable   # disable for current repo
git lrc hooks enable    # re-enable later
```

### ¿Puedo revisar un commit anterior?

```bash
git lrc review --commit HEAD       # review the last commit
git lrc review --commit HEAD~3..HEAD  # review a range
```

## Referencia rápida

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

> **Consejo:** `git lrc <command>` y `lrc <command>` son intercambiables.

## Es gratis. Compártelo.

`git-lrc` es **completamente gratis.** Sin tarjeta. Sin trial. Sin trampa.

Si te ayuda — **compártelo con tus amigos desarrolladores.** Cuanta más gente revise código generado por IA, menos bugs llegarán a producción.

⭐ **[Dale una estrella a este repo](https://github.com/HexmosTech/git-lrc)** para que otros lo descubran.

## Licencia

`git-lrc` se distribuye bajo una variante modificada de la **Sustainable Use License (SUL)**.

> [!NOTE]
>
> **Qué significa esto:**
>
> - ✅ **Source Available** — El código fuente completo está disponible para self-hosting
> - ✅ **Business Use Allowed** — Usa LiveReview en tus operaciones internas
> - ✅ **Modifications Allowed** — Personaliza para tu propio uso
> - ❌ **No Resale** — No se puede revender ni ofrecer como servicio competidor
> - ❌ **No Redistribution** — No se pueden redistribuir versiones modificadas comercialmente
>
> Esta licencia asegura que LiveReview siga siendo sostenible y te da acceso completo para self-host y personalizar según necesites.

Para términos detallados, ejemplos de usos permitidos y prohibidos y definiciones, consulta el [LICENSE.md](LICENSE.md) completo.

---

## Para equipos: LiveReview

> ¿Usas `git-lrc` en solitario? Genial. ¿Construyes con un equipo? Echa un vistazo a **[LiveReview](https://hexmos.com/livereview)** — el conjunto completo para revisión de código con IA a nivel de equipo, con dashboards, políticas a nivel de organización y analytics de revisión. Todo lo que hace `git-lrc`, más coordinación de equipo.
