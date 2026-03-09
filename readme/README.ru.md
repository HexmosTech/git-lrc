<div align="center">

[🇬🇧 English](../README.md)

</div>

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

ИИ-агенты пишут код быстро. Они также _тихо убирают логику_, меняют поведение и вносят баги — не предупреждая. Часто вы узнаёте об этом только в продакшене.

**`git-lrc` это исправляет.** Он подключается к `git commit` и проверяет каждый diff _до_ того, как он попадёт в репозиторий. Настройка за 60 секунд. Полностью бесплатно.

## В действии

> Смотрите, как git-lrc находит серьёзные проблемы безопасности: утёкшие учётные данные,
> дорогие облачные операции и чувствительные данные в логах

https://github.com/user-attachments/assets/cc4aa598-a7e3-4a1d-998c-9f2ba4b4c66e

## Зачем

- 🤖 **ИИ-агенты тихо ломают код.** Логика удалена. Поведение изменено. Граничные случаи пропали. Заметите только в продакшене.
- 🔍 **Поймайте до выката.** Инлайн-комментарии с ИИ показывают _точно_, что изменилось и что выглядит подозрительно.
- 🔁 **Привычка → меньше багов.** Регулярный ревью → меньше багов → более устойчивый код → лучшие результаты в команде.
- 🔗 **Почему git?** Git универсален. Любой редактор, любая IDE, любой ИИ-тулчейн его использует. Коммит обязателен. Поэтому _почти невозможно пропустить ревью_ — независимо от стека.

## Начало работы

### Установка

**Linux / macOS:**

```bash
curl -fsSL https://hexmos.com/lrc-install.sh | bash
```

**Windows (PowerShell):**

```powershell
iwr -useb https://hexmos.com/lrc-install.ps1 | iex
```

Бинарник установлен. Хуки настроены глобально. Готово.

### Настройка

```bash
git lrc setup
```

Короткое видео о настройке:

https://github.com/user-attachments/assets/392a4605-6e45-42ad-b2d9-6435312444b5

Два шага, оба открываются в браузере:

1. **Ключ API LiveReview** — войдите через Hexmos
2. **Бесплатный ключ API Gemini** — получите в Google AI Studio

**~1 минута. Один раз на машину.** После этого _каждый git-репозиторий_ на вашей машине запускает ревью при коммите. Отдельная настройка на репо не нужна.

## Как это работает

### Вариант А: Ревью при коммите (автоматически)

```bash
git add .
git commit -m "add payment validation"
# review launches automatically before the commit goes through
```

### Вариант Б: Ревью перед коммитом (вручную)

```bash
git add .
git lrc review          # run AI review first
# or: git lrc review --vouch   # vouch personally, skip AI
# or: git lrc review --skip    # skip review entirely
git commit -m "add payment validation"
```

В обоих случаях в браузере откроется веб-интерфейс.

https://github.com/user-attachments/assets/ae063e39-379f-4815-9954-f0e2ab5b9cde

### Интерфейс ревью

- 📄 **Diff в стиле GitHub** — добавления/удаления с подсветкой
- 💬 **Инлайн-комментарии ИИ** — на нужных строках, с метками серьёзности
- 📝 **Итог ревью** — обзор того, что нашёл ИИ
- 📁 **Список файлов в stage** — все staged-файлы сразу, переход между ними
- 📊 **Сводка по diff** — добавлено/удалено строк по файлам для быстрой оценки объёма изменений
- 📋 **Копировать замечания** — один клик скопирует все замечания ИИ, готово для вставки обратно в агента
- 🔄 **Переход по замечаниям** — листать комментарии по одному без прокрутки
- 📜 **Журнал событий** — события ревью, итерации и смены статуса в одном месте

https://github.com/user-attachments/assets/b579d7c6-bdf6-458b-b446-006ca41fe47d

### Решение

| Action               | What happens                           |
| -------------------- | -------------------------------------- |
| ✅ **Commit**        | Accept and commit the reviewed changes |
| 🚀 **Commit & Push** | Commit and push to remote in one step  |
| ⏭️ **Skip**          | Abort the commit — go fix issues first |

```
📎 Screenshot: Pre-commit bar showing Commit / Commit & Push / Skip buttons
```

## Цикл ревью

Типичный сценарий с кодом, сгенерированным ИИ:

1. **Генерируете код** своим ИИ-агентом
2. **`git add .` → `git lrc review`** — ИИ помечает замечания
3. **Копируете замечания, отдаёте** агенту на исправление
4. **`git add .` → `git lrc review`** — ИИ снова ревьюит
5. Повторяете до удовлетворения
6. **`git lrc review --vouch`** → **`git commit`** — вы подтверждаете и коммитите

Каждый `git lrc review` — одна **итерация**. Инструмент считает, сколько итераций было и какой процент diff прошёл ревью ИИ (**coverage**).

### Vouch

Когда итераций достаточно и код вас устраивает:

```bash
git lrc review --vouch
```

Это значит: _«Я проверил это — через итерации ИИ или сам — и несу ответственность.»_ Ревью ИИ не запускается, но статистика coverage по прошлым итерациям записывается.

### Skip

Хотите просто закоммитить без ревью и без заявления об ответственности?

```bash
git lrc review --skip
```

Без ревью ИИ. Без личного подтверждения. В git log попадёт `skipped`.

## Отслеживание в git log

К сообщению каждого коммита добавляется **строка статуса ревью**:

```
LiveReview Pre-Commit Check: ran (iter:3, coverage:85%)
```

```
LiveReview Pre-Commit Check: vouched (iter:2, coverage:50%)
```

```
LiveReview Pre-Commit Check: skipped
```

- **`iter`** — число циклов ревью до коммита. `iter:3` = три раза: ревью → правки → ревью.
- **`coverage`** — доля итогового diff, уже проверенная ИИ в прошлых итерациях. `coverage:85%` = не проверено только 15% кода.

Команда видит _точно_, какие коммиты были отревьюены, vouched или пропущены — прямо в `git log`.

## FAQ

### Review vs Vouch vs Skip?

|                       | **Review**                  | **Vouch**                       | **Skip**                  |
| --------------------- | --------------------------- | ------------------------------- | ------------------------- |
| AI reviews the diff?  | ✅ Yes                      | ❌ No                           | ❌ No                     |
| Takes responsibility? | ✅ Yes                      | ✅ Yes, explicitly              | ⚠️ No                     |
| Tracks iterations?    | ✅ Yes                      | ✅ Records prior coverage       | ❌ No                     |
| Git log message       | `ran (iter:N, coverage:X%)` | `vouched (iter:N, coverage:X%)` | `skipped`                 |
| When to use           | Each review cycle           | Done iterating, ready to commit | Not reviewing this commit |

**Review** — по умолчанию. ИИ анализирует ваш staged diff и даёт инлайн-комментарии. Каждый ревью — одна итерация в цикле правки–ревью.

**Vouch** — вы _явно принимаете ответственность_ за этот коммит. Обычно после нескольких итераций ревью: вы поправили замечания и довольны. ИИ больше не запускается, но предыдущие итерации и coverage записываются.

**Skip** — этот коммит вы не ревьюите. Может, он мелкий, может, не критичный — причина на вас. В git log просто будет `skipped`.

### Почему это бесплатно?

`git-lrc` использует **Google Gemini API** для ревью с ИИ. У Gemini щедрый бесплатный уровень. Вы подставляете свой API-ключ — без посреднической оплаты. Облачный сервис LiveReview, который координирует ревью, бесплатен для отдельных разработчиков.

### Какие данные отправляются?

Анализируется только **staged diff**. Полный контекст репозитория не загружается, после ревью diff не хранятся.

### Можно отключить для конкретного репо?

```bash
git lrc hooks disable   # disable for current repo
git lrc hooks enable    # re-enable later
```

### Можно отревьюить старый коммит?

```bash
git lrc review --commit HEAD       # review the last commit
git lrc review --commit HEAD~3..HEAD  # review a range
```

## Краткая справка

| Command                    | Description                                   |
| -------------------------- | --------------------------------------------- |
| `lrc` or `lrc review`      | Review staged changes                         |
| `lrc review --vouch`       | Vouch — skip AI, take personal responsibility |
| `lrc review --skip`        | Skip review for this commit                   |
| `lrc review --commit HEAD` | Review an already-committed change             |
| `lrc hooks disable`        | Disable hooks for current repo                |
| `lrc hooks enable`         | Re-enable hooks for current repo              |
| `lrc hooks status`         | Show hook status                              |
| `lrc self-update`          | Update to latest version                      |
| `lrc version`              | Show version info                             |

> **Подсказка:** `git lrc <command>` и `lrc <command>` взаимозаменяемы.

## Бесплатно. Делитесь.

`git-lrc` **полностью бесплатен.** Без карты. Без триала. Без подвоха.

Если он вам полезен — **поделитесь с коллегами.** Чем больше людей ревьюит код от ИИ, тем меньше багов уходит в прод.

⭐ **[Поставьте звезду репозиторию](https://github.com/HexmosTech/git-lrc)** — так его легче найти.

## Лицензия

`git-lrc` распространяется под модифицированным вариантом **Sustainable Use License (SUL)**.

> [!NOTE]
>
> **Что это значит:**
>
> - ✅ **Source Available** — Исходный код доступен для самостоятельного хостинга
> - ✅ **Business Use Allowed** — Можно использовать LiveReview во внутренних процессах
> - ✅ **Modifications Allowed** — Можно изменять под свои нужды
> - ❌ **No Resale** — Нельзя перепродавать или предлагать как конкурирующий сервис
> - ❌ **No Redistribution** — Нельзя коммерчески распространять изменённые версии
>
> Лицензия сохраняет устойчивость LiveReview и даёт полный доступ к самохостингу и доработке.

Подробные условия, примеры разрешённого и запрещённого использования и определения — в [LICENSE.md](LICENSE.md).

---

## Для команд: LiveReview

> Используете `git-lrc` в одиночку? Отлично. Работаете в команде? Посмотрите **[LiveReview](https://hexmos.com/livereview)** — полный набор для командного ИИ-ревью кода: дашборды, политики на уровне организации, аналитика ревью. Всё то же, что делает `git-lrc`, плюс координация команды.
