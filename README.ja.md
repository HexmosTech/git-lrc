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

AIエージェントはコードを速く書きます。その一方で、_こっそりロジックを削除_したり、挙動を変えたり、バグを仕込んだりします。本番で初めて気づくことも多いです。

**`git-lrc` がそれを解消します。** `git commit` にフックし、すべての diff を _コミット前に_ レビューします。約60秒でセットアップ。完全無料。

## 実際の様子

> git-lrc が、漏れた認証情報・高額なクラウド操作・ログ内の機密情報といった深刻なセキュリティ問題を検出する様子

https://github.com/user-attachments/assets/cc4aa598-a7e3-4a1d-998c-9f2ba4b4c66e

## なぜ使うか

- 🤖 **AIエージェントは静かに破壊する。** コードが消える。ロジックが変わる。エッジケースがなくなる。本番まで気づかない。
- 🔍 **出荷前にキャッチ。** AIによるインラインコメントで、_何が変わったか・何がおかしいか_ を正確に示します。
- 🔁 **習慣にして、より良いコードを出荷。** 定期的なレビュー → バグ減 → 堅牢なコード → チームの成果向上。
- 🔗 **なぜ git？** Git は普遍。あらゆるエディタ・IDE・AIツールが使う。コミットは必須。だから _レビュー漏れはほぼない_ — スタックに依存しません。

## はじめに

### インストール

**Linux / macOS:**

```bash
curl -fsSL https://hexmos.com/lrc-install.sh | sudo bash
```

**Windows (PowerShell):**

```powershell
iwr -useb https://hexmos.com/lrc-install.ps1 | iex
```

バイナリが入り、フックがグローバルに設定されます。完了。

### セットアップ

```bash
git lrc setup
```

セットアップの流れの短い動画：

https://github.com/user-attachments/assets/392a4605-6e45-42ad-b2d9-6435312444b5

ブラウザで開く2ステップです：

1. **LiveReview API キー** — Hexmos でサインイン
2. **無料 Gemini API キー** — Google AI Studio で取得

**約1分。1回のセットアップでマシン全体に有効。** 以降、そのマシン上の _すべての git リポジトリ_ でコミット時にレビューが走ります。リポジトリごとの設定は不要です。

## 動き方

### 方法A: コミット時にレビュー（自動）

```bash
git add .
git commit -m "add payment validation"
# review launches automatically before the commit goes through
```

### 方法B: コミット前にレビュー（手動）

```bash
git add .
git lrc review          # run AI review first
# or: git lrc review --vouch   # vouch personally, skip AI
# or: git lrc review --skip    # skip review entirely
git commit -m "add payment validation"
```

いずれにせよ、ブラウザで Web UI が開きます。

https://github.com/user-attachments/assets/ae063e39-379f-4815-9954-f0e2ab5b9cde

### レビューUI

- 📄 **GitHub風 diff** — 追加/削除を色分け
- 💬 **インラインAIコメント** — 該当行に正確に、重要度バッジ付き
- 📝 **レビューサマリ** — AI が検出した内容の概要
- 📁 **ステージ済みファイル一覧** — 一覧で確認、ファイル間をジャンプ
- 📊 **diff サマリ** — ファイルごとの追加/削除行で変更規模を把握
- 📋 **問題のコピー** — AI が指摘した問題をワンクリックでコピー、AIエージェントに貼り戻し可能
- 🔄 **問題を順に表示** — スクロールせずにコメント間を移動
- 📜 **イベントログ** — レビューイベント・反復・状態変更を一箇所で追跡

https://github.com/user-attachments/assets/b579d7c6-bdf6-458b-b446-006ca41fe47d

### 判断

| Action               | What happens                           |
| -------------------- | -------------------------------------- |
| ✅ **Commit**        | Accept and commit the reviewed changes |
| 🚀 **Commit & Push** | Commit and push to remote in one step  |
| ⏭️ **Skip**          | Abort the commit — go fix issues first |

```
📎 Screenshot: Pre-commit bar showing Commit / Commit & Push / Skip buttons
```

## レビューサイクル

AI生成コードでの典型的な流れ：

1. AIエージェントで**コードを生成**
2. **`git add .` → `git lrc review`** — AI が問題を指摘
3. **問題をコピーして**エージェントに渡して修正
4. **`git add .` → `git lrc review`** — AI が再レビュー
5. 満足するまで繰り返し
6. **`git lrc review --vouch`** → **`git commit`** — 自分で保証してコミット

各 `git lrc review` が 1 **イテレーション**です。ツールはイテレーション回数と、diff の何割が AI レビューされたか（**カバレッジ**）を記録します。

### Vouch

十分イテレしてコードに満足したら：

```bash
git lrc review --vouch
```

意味：_「AIのイテレーションか自分で確認し、責任を取る。」_ AI レビューは走りませんが、それまでのイテレーションのカバレッジは記録されます。

### Skip

レビューや責任表明なしでコミットしたいだけなら：

```bash
git lrc review --skip
```

AI レビューなし。個人の保証なし。git ログには `skipped` と記録されます。

## Git ログでの追跡

各コミットの git ログメッセージに **レビュー状態行** が付きます：

```
LiveReview Pre-Commit Check: ran (iter:3, coverage:85%)
```

```
LiveReview Pre-Commit Check: vouched (iter:2, coverage:50%)
```

```
LiveReview Pre-Commit Check: skipped
```

- **`iter`** — コミット前のレビューサイクル数。`iter:3` = レビュー → 修正 → レビュー を3回。
- **`coverage`** — 最終 diff のうち、過去のイテレーションで既に AI レビューされた割合。`coverage:85%` = 未レビューは 15% のみ。

チームは `git log` で、どのコミットがレビュー済み・vouch 済み・スキップか _正確に_ 分かります。

## よくある質問

### Review / Vouch / Skip の違いは？

|                       | **Review**                  | **Vouch**                       | **Skip**                  |
| --------------------- | --------------------------- | ------------------------------- | ------------------------- |
| AI reviews the diff?  | ✅ Yes                      | ❌ No                           | ❌ No                     |
| Takes responsibility? | ✅ Yes                      | ✅ Yes, explicitly              | ⚠️ No                     |
| Tracks iterations?    | ✅ Yes                      | ✅ Records prior coverage       | ❌ No                     |
| Git log message       | `ran (iter:N, coverage:X%)` | `vouched (iter:N, coverage:X%)` | `skipped`                 |
| When to use           | Each review cycle           | Done iterating, ready to commit | Not reviewing this commit |

**Review** がデフォルト。AI がステージ済み diff を解析しインラインフィードバックを返します。1回のレビュー = 変更–レビューサイクルの1イテレーション。

**Vouch** はこのコミットに _明示的に責任を取る_ 意思表示。複数回レビューして満足した後に使うのが典型。AI は再実行されませんが、それまでのイテレーション・カバレッジが記録されます。

**Skip** はこのコミットはレビューしない、という意味。 trivial な変更や重要でない変更など、理由は自由。git ログには `skipped` とだけ記録されます。

### なぜ無料なのか？

`git-lrc` は AI レビューに **Google の Gemini API** を使います。Gemini は無料枠が広いです。自分の API キーを持ち込むだけなので、中間課金はありません。レビューを調整する LiveReview クラウドサービスは個人開発者向けに無料です。

### 送信されるデータは？

**ステージ済み diff** だけが解析されます。リポジトリ全体のコンテキストはアップロードされず、レビュー後も diff は保存されません。

### 特定リポジトリで無効にできる？

```bash
git lrc hooks disable   # disable for current repo
git lrc hooks enable    # re-enable later
```

### 過去のコミットをレビューできる？

```bash
git lrc review --commit HEAD       # review the last commit
git lrc review --commit HEAD~3..HEAD  # review a range
```

## クイックリファレンス

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

> **ヒント:** `git lrc <command>` と `lrc <command>` は同じように使えます。

## 無料です。シェアしてください。

`git-lrc` は **完全無料** です。クレジットカード不要。トライアル制限なし。縛りなし。

役に立ったら **開発者仲間にシェア** してください。AI 生成コードをレビューする人が増えれば、本番に乗るバグは減ります。

⭐ **[このリポジトリをスター](https://github.com/HexmosTech/git-lrc)** して、他の人にも見つけてもらいましょう。

## ライセンス

`git-lrc` は **Sustainable Use License (SUL)** の改変版の下で配布されています。

> [!NOTE]
>
> **意味するところ:**
>
> - ✅ **Source Available** — セルフホスト用の完全なソースコードが利用可能
> - ✅ **Business Use Allowed** — LiveReview を社内業務で利用可能
> - ✅ **Modifications Allowed** — 自用にカスタマイズ可能
> - ❌ **No Resale** — 転売や競合サービスとしての提供は不可
> - ❌ **No Redistribution** — 改変版の商用再配布は不可
>
> このライセンスにより、LiveReview を持続可能にしつつ、セルフホストとニーズに合わせたカスタマイズが可能です。

詳細な条項・許可・禁止用途の例・定義は [LICENSE.md](LICENSE.md) を参照してください。

---

## チーム向け: LiveReview

> `git-lrc` を一人で使う？それで十分。チームで開発する？**[LiveReview](https://hexmos.com/livereview)** をどうぞ — チーム全体の AI コードレビュー用スイート。ダッシュボード、組織ポリシー、レビュー分析付き。`git-lrc` の機能に加え、チーム連携が可能です。
