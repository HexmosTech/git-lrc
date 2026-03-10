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
&nbsp;
<a href="https://goreportcard.com/report/github.com/HexmosTech/git-lrc" target="_blank" rel="noopener noreferrer"><img alt="Go Report Card" src="https://goreportcard.com/badge/github.com/HexmosTech/git-lrc"></a>
&nbsp;
<a href="https://fazier.com/launches/git-lrc" target="_blank" rel="noopener noreferrer"><img src="https://fazier.com/api/v1/public/badges/embed_image.svg?launch_id=7598&amp;badge_type=daily&amp;theme=light" width="270" alt="Fazier badge" /></a>
</div>

<br />
<br />

---

AI 代理写代码很快，也会_悄悄删逻辑_、改行为、引入 bug，而且不告诉你。你往往要到上线才发现。

**`git-lrc` 解决这个问题。** 它挂在 `git commit` 上，在每次 diff _落地前_ 做审查。约 60 秒完成配置，完全免费。

## 实际效果

> 看 git-lrc 如何发现严重安全问题：泄露的凭据、昂贵的云操作、日志中的敏感信息等

https://github.com/user-attachments/assets/cc4aa598-a7e3-4a1d-998c-9f2ba4b4c66e

## 为什么需要

- 🤖 **AI 代理会悄悄搞坏东西。** 代码被删、逻辑被改、边界情况没了，上线前你很难发现。
- 🔍 **在发布前拦住。** 由 AI 驱动的行内评论会_精确_标出改了啥、哪里看起来有问题。
- 🔁 **养成习惯，代码更好。** 定期审查 → 更少 bug → 更稳的代码 → 团队效果更好。
- 🔗 **为什么用 git？** Git 是通用标准，各种编辑器、IDE、AI 工具都用它，提交是必经步骤，所以_几乎不会漏审_——不管你的技术栈是什么。

## 快速开始

### 安装

**Linux / macOS：**

```bash
curl -fsSL https://hexmos.com/lrc-install.sh | bash
```

**Windows（PowerShell）：**

```powershell
iwr -useb https://hexmos.com/lrc-install.ps1 | iex
```

二进制安装完成，钩子全局配置好，即可使用。

### 配置

```bash
git lrc setup
```

配置过程短视频：

https://github.com/user-attachments/assets/392a4605-6e45-42ad-b2d9-6435312444b5

两步，都会在浏览器中打开：

1. **LiveReview API 密钥** — 使用 Hexmos 登录
2. **免费 Gemini API 密钥** — 在 Google AI Studio 获取

**约 1 分钟，一次配置，整机生效。** 之后，你机器上的_每个 git 仓库_在提交时都会触发审查，无需按仓库单独配置。

## 工作方式

### 方式 A：提交时自动审查

```bash
git add .
git commit -m "add payment validation"
# review launches automatically before the commit goes through
```

### 方式 B：提交前手动审查

```bash
git add .
git lrc review          # run AI review first
# or: git lrc review --vouch   # vouch personally, skip AI
# or: git lrc review --skip    # skip review entirely
git commit -m "add payment validation"
```

无论哪种方式，都会在浏览器中打开 Web 界面。

https://github.com/user-attachments/assets/ae063e39-379f-4815-9954-f0e2ab5b9cde

### 审查界面

- 📄 **类 GitHub diff** — 增删行颜色区分
- 💬 **行内 AI 评论** — 精确到相关行，带严重程度标签
- 📝 **审查摘要** — AI 发现内容的高层概览
- 📁 **暂存文件列表** — 一眼看到所有暂存文件，在文件间跳转
- 📊 **Diff 摘要** — 每个文件增删行数，快速了解改动范围
- 📋 **复制问题** — 一键复制所有 AI 标出的问题，可直接贴回给你的 AI 代理
- 🔄 **逐条浏览问题** — 在评论间逐个跳转，无需滚动
- 📜 **事件日志** — 在一处查看审查事件、迭代和状态变化

https://github.com/user-attachments/assets/b579d7c6-bdf6-458b-b446-006ca41fe47d

### 决策选项

| Action               | What happens                           |
| -------------------- | -------------------------------------- |
| ✅ **Commit**        | Accept and commit the reviewed changes |
| 🚀 **Commit & Push** | Commit and push to remote in one step  |
| ⏭️ **Skip**          | Abort the commit — go fix issues first |

```
📎 Screenshot: Pre-commit bar showing Commit / Commit & Push / Skip buttons
```

## 审查循环

AI 生成代码时的典型流程：

1. 用你的 AI 代理**生成代码**
2. **`git add .` → `git lrc review`** — AI 标出问题
3. **复制问题，反馈给**代理去修
4. **`git add .` → `git lrc review`** — AI 再审一遍
5. 重复直到满意
6. **`git lrc review --vouch`** → **`git commit`** — 你担保并提交

每次 `git lrc review` 算一次**迭代**。工具会记录你做了几次迭代，以及 diff 中有多少比例被 AI 审过（**覆盖率**）。

### Vouch（担保）

迭代足够多、对代码满意后：

```bash
git lrc review --vouch
```

表示：_“我已审查过（通过 AI 迭代或亲自）并对此负责。”_ 不再跑 AI 审查，但会记录之前迭代的覆盖率等统计。

### Skip（跳过）

只想提交、不做审查也不做责任声明？

```bash
git lrc review --skip
```

不跑 AI 审查，不做个人声明，git 日志会记录 `skipped`。

## Git 日志追踪

每次提交都会在 git 日志消息后追加一条**审查状态**：

```
LiveReview Pre-Commit Check: ran (iter:3, coverage:85%)
```

```
LiveReview Pre-Commit Check: vouched (iter:2, coverage:50%)
```

```
LiveReview Pre-Commit Check: skipped
```

- **`iter`** — 提交前的审查轮数。`iter:3` 表示三轮：审查 → 修改 → 再审查。
- **`coverage`** — 最终 diff 中已在之前迭代中被 AI 审过的比例。`coverage:85%` 表示只有 15% 的代码未审。

团队可以在 `git log` 里_清楚看到_哪些提交是审查过、担保过还是跳过的。

## 常见问题

### Review / Vouch / Skip 区别？

|                       | **Review**                  | **Vouch**                       | **Skip**                  |
| --------------------- | --------------------------- | ------------------------------- | ------------------------- |
| AI reviews the diff?  | ✅ Yes                      | ❌ No                           | ❌ No                     |
| Takes responsibility? | ✅ Yes                      | ✅ Yes, explicitly              | ⚠️ No                     |
| Tracks iterations?    | ✅ Yes                      | ✅ Records prior coverage       | ❌ No                     |
| Git log message       | `ran (iter:N, coverage:X%)` | `vouched (iter:N, coverage:X%)` | `skipped`                 |
| When to use           | Each review cycle           | Done iterating, ready to commit | Not reviewing this commit |

**Review** 是默认。AI 分析你的暂存 diff 并给出行内反馈，每次审查算一次变更–审查循环中的迭代。

**Vouch** 表示你_明确为本次提交负责_，通常在多轮审查迭代之后使用——你已经来回修过、满意了。AI 不再运行，但会记录之前的迭代和覆盖率。

**Skip** 表示不审查这次提交，可能因为无关紧要或非关键，原因自定，git 日志只记 `skipped`。

### 为什么免费？

`git-lrc` 使用 **Google 的 Gemini API** 做 AI 审查。Gemini 有 generous 免费额度，你自带 API 密钥，没有中间计费。负责协调审查的 LiveReview 云服务对个人开发者免费。

### 会上传什么数据？

只分析**暂存 diff**，不会上传完整仓库上下文，审查后也不会保存 diff。

### 能对某个仓库关闭吗？

```bash
git lrc hooks disable   # disable for current repo
git lrc hooks enable    # re-enable later
```

### 能审查以前的提交吗？

```bash
git lrc review --commit HEAD       # review the last commit
git lrc review --commit HEAD~3..HEAD  # review a range
```

## 命令速查

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

> **提示：** `git lrc <command>` 与 `lrc <command>` 可互换使用。

## 完全免费，欢迎分享

`git-lrc` **完全免费**，无需信用卡、无试用限制、无套路。

若对你有用，**可以分享给身边的开发者。** 越多人审查 AI 生成的代码，进生产的 bug 就越少。

⭐ **[给本仓库点 Star](https://github.com/HexmosTech/git-lrc)** 让更多人发现它。

## 社区

在 **git-lrc Discord 社区**中讨论可能性、改进、功能并获取贡献帮助：[discord.gg/sGdnKwB3qq](https://discord.gg/sGdnKwB3qq)。

## 许可证

`git-lrc` 在 **Sustainable Use License (SUL)** 的修改版本下分发。

> [!NOTE]
>
> **含义简述：**
>
> - ✅ **Source Available** — 完整源码可自托管
> - ✅ **Business Use Allowed** — 可将 LiveReview 用于内部业务
> - ✅ **Modifications Allowed** — 可自行修改使用
> - ❌ **No Resale** — 不得转售或作为竞争服务提供
> - ❌ **No Redistribution** — 不得将修改版以商业方式再分发
>
> 该许可在保证 LiveReview 可持续的同时，允许你自托管和按需定制。

详细条款、允许与禁止用途示例及定义见 [LICENSE.md](LICENSE.md)。

---

## 团队版：LiveReview

> 一个人用 `git-lrc`？没问题。和团队一起开发？可了解 **[LiveReview](https://hexmos.com/livereview)** —— 面向全团队的 AI 代码审查套件，含仪表盘、组织级策略和审查分析。具备 `git-lrc` 的全部能力，并支持团队协作。
