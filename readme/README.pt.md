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

Agentes de IA escrevem código rápido. Também _removem lógica em silêncio_, mudam comportamento e introduzem bugs — sem te avisar. Muitas vezes só descobres em produção.

**O `git-lrc` resolve isto.** Liga-se ao `git commit` e revê cada diff _antes_ de entrar. Configuração em 60 segundos. Totalmente gratuito.

## Ver em ação

> Vê o git-lrc a detetar problemas sérios de segurança como credenciais expostas, operações
> cloud dispendiosas e material sensível em logs

https://github.com/user-attachments/assets/cc4aa598-a7e3-4a1d-998c-9f2ba4b4c66e

## Porquê

- 🤖 **Agentes de IA quebram coisas em silêncio.** Código removido. Lógica alterada. Casos extremos perdidos. Só reparas em produção.
- 🔍 **Apanha antes de fazer ship.** Comentários inline com IA mostram _exatamente_ o que mudou e o que parece errado.
- 🔁 **Cria o hábito, faz ship de melhor código.** Revisão regular → menos bugs → código mais robusto → melhores resultados na tua equipa.
- 🔗 **Porquê git?** Git é universal. Qualquer editor, qualquer IDE, qualquer toolkit de IA usa-o. Fazer commit é obrigatório. Por isso _quase não há hipótese de falhar uma revisão_ — independentemente do teu stack.

## Começar

### Instalação

**Linux / macOS:**

```bash
curl -fsSL https://hexmos.com/lrc-install.sh | bash
```

**Windows (PowerShell):**

```powershell
iwr -useb https://hexmos.com/lrc-install.ps1 | iex
```

Binário instalado. Hooks configurados globalmente. Feito.

### Configuração

```bash
git lrc setup
```

Aqui está um vídeo rápido de como funciona a configuração:

https://github.com/user-attachments/assets/392a4605-6e45-42ad-b2d9-6435312444b5

Dois passos, ambos abrem no teu browser:

1. **Chave API LiveReview** — inicia sessão com Hexmos
2. **Chave API Gemini gratuita** — obtém uma no Google AI Studio

**~1 minuto. Configuração única, para toda a máquina.** Depois disto, _cada repo git_ na tua máquina dispara revisão no commit. Não é preciso config por repo.

## Como funciona

### Opção A: Revisão no commit (automática)

```bash
git add .
git commit -m "add payment validation"
# review launches automatically before the commit goes through
```

### Opção B: Revisão antes do commit (manual)

```bash
git add .
git lrc review          # run AI review first
# or: git lrc review --vouch   # vouch personally, skip AI
# or: git lrc review --skip    # skip review entirely
git commit -m "add payment validation"
```

De qualquer forma, abre uma UI web no browser.

https://github.com/user-attachments/assets/ae063e39-379f-4815-9954-f0e2ab5b9cde

### A UI de revisão

- 📄 **Diff estilo GitHub** — adições/remoções com cores
- 💬 **Comentários inline da IA** — nas linhas exatas que importam, com badges de severidade
- 📝 **Resumo da revisão** — visão geral do que a IA encontrou
- 📁 **Lista de ficheiros em stage** — vê todos os ficheiros em stage de relance, salta entre eles
- 📊 **Resumo do diff** — linhas adicionadas/removidas por ficheiro para uma ideia rápida do âmbito da alteração
- 📋 **Copiar issues** — um clique para copiar todos os issues assinalados pela IA, prontos para colar de volta no teu agente de IA
- 🔄 **Percorrer issues** — navegar entre comentários um a um sem scroll
- 📜 **Registo de eventos** — acompanhar eventos de revisão, iterações e mudanças de estado num só sítio

https://github.com/user-attachments/assets/b579d7c6-bdf6-458b-b446-006ca41fe47d

### A decisão

| Action               | What happens                           |
| -------------------- | -------------------------------------- |
| ✅ **Commit**        | Accept and commit the reviewed changes |
| 🚀 **Commit & Push** | Commit and push to remote in one step  |
| ⏭️ **Skip**          | Abort the commit — go fix issues first |

```
📎 Screenshot: Pre-commit bar showing Commit / Commit & Push / Skip buttons
```

## O ciclo de revisão

Fluxo típico com código gerado por IA:

1. **Gera código** com o teu agente de IA
2. **`git add .` → `git lrc review`** — a IA assinala issues
3. **Copia os issues, devolve-os** ao agente para corrigir
4. **`git add .` → `git lrc review`** — a IA revê de novo
5. Repete até ficares satisfeito
6. **`git lrc review --vouch`** → **`git commit`** — tu garantes e fazes commit

Cada `git lrc review` é uma **iteração**. A ferramenta regista quantas iterações fizeste e que percentagem do diff foi revista pela IA (**coverage**).

### Vouch

Quando já iteraste o suficiente e estás satisfeito com o código:

```bash
git lrc review --vouch
```

Isto diz: _"Reví isto — por iterações da IA ou pessoalmente — e assumo a responsabilidade."_ Não corre revisão da IA, mas as estatísticas de coverage de iterações anteriores ficam registadas.

### Skip

Queres só fazer commit sem revisão nem attestation de responsabilidade?

```bash
git lrc review --skip
```

Sem revisão da IA. Sem attestation pessoal. O git log regista `skipped`.

## Registo no Git Log

Cada commit recebe uma **linha de estado da revisão** anexada à mensagem do git log:

```
LiveReview Pre-Commit Check: ran (iter:3, coverage:85%)
```

```
LiveReview Pre-Commit Check: vouched (iter:2, coverage:50%)
```

```
LiveReview Pre-Commit Check: skipped
```

- **`iter`** — número de ciclos de revisão antes do commit. `iter:3` = três rondas de revisão → correção → revisão.
- **`coverage`** — percentagem do diff final já revista pela IA em iterações anteriores. `coverage:85%` = só 15% do código não foi revisto.

A tua equipa vê _exatamente_ que commits foram revistos, vouched ou skipped — diretamente no `git log`.

## FAQ

### Review vs Vouch vs Skip?

|                       | **Review**                  | **Vouch**                       | **Skip**                  |
| --------------------- | --------------------------- | ------------------------------- | ------------------------- |
| AI reviews the diff?  | ✅ Yes                      | ❌ No                           | ❌ No                     |
| Takes responsibility? | ✅ Yes                      | ✅ Yes, explicitly              | ⚠️ No                     |
| Tracks iterations?    | ✅ Yes                      | ✅ Records prior coverage       | ❌ No                     |
| Git log message       | `ran (iter:N, coverage:X%)` | `vouched (iter:N, coverage:X%)` | `skipped`                 |
| When to use           | Each review cycle           | Done iterating, ready to commit | Not reviewing this commit |

**Review** é o padrão. A IA analisa o teu diff em stage e dá feedback inline. Cada revisão é uma iteração no ciclo alteração–revisão.

**Vouch** significa que estás _explicitamente a assumir responsabilidade_ por este commit. Tipicamente usado após várias iterações de revisão — já foste e vieste, corrigiste issues e estás satisfeito. A IA não corre de novo, mas as tuas iterações e estatísticas de coverage anteriores ficam registadas.

**Skip** significa que não estás a rever este commit. Talvez seja trivial, talvez não seja crítico — a razão é tua. O git log regista apenas `skipped`.

### Como é que isto é gratuito?

O `git-lrc` usa a **API Gemini da Google** para revisões com IA. O Gemini tem um tier gratuito generoso. Trazes a tua própria chave API — não há faturação intermediária. O serviço cloud LiveReview que coordena as revisões é gratuito para programadores individuais.

### Que dados são enviados?

Só o **diff em stage** é analisado. Não é enviado contexto completo do repositório e os diffs não são guardados após a revisão.

### Posso desativar para um repo específico?

```bash
git lrc hooks disable   # disable for current repo
git lrc hooks enable    # re-enable later
```

### Posso rever um commit mais antigo?

```bash
git lrc review --commit HEAD       # review the last commit
git lrc review --commit HEAD~3..HEAD  # review a range
```

## Referência rápida

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

> **Dica:** `git lrc <command>` e `lrc <command>` são intercambiáveis.

## É gratuito. Partilha.

O `git-lrc` é **totalmente gratuito.** Sem cartão de crédito. Sem trial. Sem truques.

Se te ajudar — **partilha com os teus amigos programadores.** Quanto mais pessoas reverem código gerado por IA, menos bugs chegam a produção.

⭐ **[Dá uma estrela a este repo](https://github.com/HexmosTech/git-lrc)** para ajudar outros a descobri-lo.

## Licença

O `git-lrc` é distribuído sob uma variante modificada da **Sustainable Use License (SUL)**.

> [!NOTE]
>
> **O que isto significa:**
>
> - ✅ **Source Available** — Código fonte completo disponível para self-hosting
> - ✅ **Business Use Allowed** — Usa o LiveReview nas tuas operações internas
> - ✅ **Modifications Allowed** — Personaliza para uso próprio
> - ❌ **No Resale** — Não pode ser revendido ou oferecido como serviço concorrente
> - ❌ **No Redistribution** — Versões modificadas não podem ser redistribuídas comercialmente
>
> Esta licença garante que o LiveReview se mantém sustentável e dá-te acesso total para self-host e personalizar conforme precisares.

Para termos detalhados, exemplos de usos permitidos e proibidos e definições, consulta o [LICENSE.md](LICENSE.md) completo.

---

## Para equipas: LiveReview

> A usar o `git-lrc` sozinho? Ótimo. A construir com uma equipa? Vê o **[LiveReview](https://hexmos.com/livereview)** — o conjunto completo para revisão de código com IA à escala da equipa, com dashboards, políticas ao nível da organização e analytics de revisão. Tudo o que o `git-lrc` faz, mais coordenação de equipa.
