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

عامل‌های هوش مصنوعی سریع کد می‌نویسند. آن‌ها همچنین _بی‌سر و صدا منطق را حذف می‌کنند_، رفتار را عوض می‌کنند و باگ معرفی می‌کنند — بدون اینکه به شما بگویند. اغلب در محیط تولید متوجه می‌شوید.

**`git-lrc` این را درست می‌کند.** به `git commit` وصل می‌شود و هر diff را _قبل از_ اعمال بررسی می‌کند. راه‌اندازی ۶۰ ثانیه‌ای. کاملاً رایگان.

## در عمل ببینید

> ببینید git-lrc چطور مسائل امنیتی جدی مثل credentialهای نشت‌کرده، عملیات ابری پرهزینه
> و مطالب حساس در لاگ را تشخیص می‌دهد

https://github.com/user-attachments/assets/cc4aa598-a7e3-4a1d-998c-9f2ba4b4c66e

## چرا

- 🤖 **عامل‌های هوش مصنوعی بی‌سر و صدا خراب می‌کنند.** کد حذف شده. منطق عوض شده. موارد لبه از بین رفته. تا محیط تولید متوجه نمی‌شوید.
- 🔍 **قبل از ship گرفتن بگیرید.** کامنت‌های خطی مبتنی بر هوش مصنوعی _دقیقاً_ نشان می‌دهند چه چیزی عوض شده و چه چیزی اشتباه به نظر می‌رسد.
- 🔁 **عادت بسازید، کد بهتر ship کنید.** بررسی منظم → باگ کمتر → کد مقاوم‌تر → نتیجه بهتر در تیم.
- 🔗 **چرا git؟** Git همه‌گیر است. هر ادیتور، هر IDE، هر ابزار هوش مصنوعی از آن استفاده می‌کند. commit اجباری است. پس _تقریباً هیچ شانسی برای از دست دادن review نیست_ — مستقل از استک شما.

## شروع کنید

### نصب

**لینوکس / مک:**

```bash
curl -fsSL https://hexmos.com/lrc-install.sh | bash
```

**ویندوز (PowerShell):**

```powershell
iwr -useb https://hexmos.com/lrc-install.ps1 | iex
```

باینری نصب شد. هوک‌ها به‌صورت سراسری تنظیم شد. تمام.

### راه‌اندازی

```bash
git lrc setup
```

یک ویدیوی کوتاه از نحوهٔ راه‌اندازی:

https://github.com/user-attachments/assets/392a4605-6e45-42ad-b2d9-6435312444b5

دو مرحله، هر دو در مرورگر باز می‌شوند:

1. **کلید API لایو ریویو** — با Hexmos وارد شوید
2. **کلید رایگان API جمینی** — یکی از Google AI Studio بگیرید

**حدود ۱ دقیقه. راه‌اندازی یک‌بار، برای کل ماشین.** بعد از این، _هر ریپوی git_ روی ماشین شما با commit بررسی را اجرا می‌کند. نیازی به تنظیم per-repo نیست.

## چطور کار می‌کند

### گزینه الف: بررسی روی commit (خودکار)

```bash
git add .
git commit -m "add payment validation"
# review launches automatically before the commit goes through
```

### گزینه ب: بررسی قبل از commit (دستی)

```bash
git add .
git lrc review          # run AI review first
# or: git lrc review --vouch   # vouch personally, skip AI
# or: git lrc review --skip    # skip review entirely
git commit -m "add payment validation"
```

در هر صورت، یک رابط وب در مرورگر باز می‌شود.

https://github.com/user-attachments/assets/ae063e39-379f-4815-9954-f0e2ab5b9cde

### رابط بررسی

- 📄 **diff سبک گیت‌هاب** — اضافه/حذف با رنگ
- 💬 **کامنت‌های خطی هوش مصنوعی** — دقیقاً روی خطوط مهم، با نشان شدت
- 📝 **خلاصه بررسی** — نمای کلی از آنچه هوش مصنوعی پیدا کرد
- 📁 **لیست فایل‌های staged** — همهٔ فایل‌های staged را یک‌جا ببینید، بین آن‌ها جابه‌جا شوید
- 📊 **خلاصه diff** — خطوط اضافه/حذف‌شده به‌ازای هر فایل برای حس سریع دامنهٔ تغییر
- 📋 **کپی مسائل** — یک کلیک برای کپی همهٔ مسائل علامت‌خوردهٔ هوش مصنوعی، آماده برای برگرداندن به عامل هوش مصنوعی شما
- 🔄 **چرخیدن بین مسائل** — بین کامنت‌ها یکی‌یکی بدون اسکرول حرکت کنید
- 📜 **لاگ رویداد** — رویدادهای بررسی، تکرارها و تغییر وضعیت را در یک جا دنبال کنید

https://github.com/user-attachments/assets/b579d7c6-bdf6-458b-b446-006ca41fe47d

### تصمیم

| Action               | What happens                           |
| -------------------- | -------------------------------------- |
| ✅ **Commit**        | Accept and commit the reviewed changes |
| 🚀 **Commit & Push** | Commit and push to remote in one step  |
| ⏭️ **Skip**          | Abort the commit — go fix issues first |

```
📎 Screenshot: Pre-commit bar showing Commit / Commit & Push / Skip buttons
```

## چرخهٔ بررسی

گردش کار معمول با کد تولیدشدهٔ هوش مصنوعی:

1. **کد تولید کنید** با عامل هوش مصنوعی
2. **`git add .` → `git lrc review`** — هوش مصنوعی مسائل را علامت می‌زند
3. **مسائل را کپی کنید، به عامل برگردانید** تا اصلاح کند
4. **`git add .` → `git lrc review`** — هوش مصنوعی دوباره بررسی می‌کند
5. تا رضایت تکرار کنید
6. **`git lrc review --vouch`** → **`git commit`** — شما ضمانت می‌کنید و commit می‌کنید

هر `git lrc review` یک **تکرار** است. ابزار تعداد تکرارها و درصد diffای که توسط هوش مصنوعی بررسی شده (**coverage**) را نگه می‌دارد.

### Vouch

وقتی به‌قدر کافی تکرار کردید و از کد راضی هستید:

```bash
git lrc review --vouch
```

یعنی: _«این را بررسی کردم — با تکرارهای هوش مصنوعی یا شخصاً — و مسئولیت می‌پذیرم.»_ بررسی هوش مصنوعی اجرا نمی‌شود، ولی آمار coverage از تکرارهای قبلی ثبت می‌شود.

### Skip

فقط می‌خواهید بدون بررسی یا attestation مسئولیت commit کنید؟

```bash
git lrc review --skip
```

بدون بررسی هوش مصنوعی. بدون attestation شخصی. git log مقدار `skipped` را ثبت می‌کند.

## ردیابی Git Log

هر commit یک **خط وضعیت بررسی** به پیام git log خود اضافه می‌کند:

```
LiveReview Pre-Commit Check: ran (iter:3, coverage:85%)
```

```
LiveReview Pre-Commit Check: vouched (iter:2, coverage:50%)
```

```
LiveReview Pre-Commit Check: skipped
```

- **`iter`** — تعداد چرخه‌های بررسی قبل از commit. `iter:3` = سه دور بررسی → اصلاح → بررسی.
- **`coverage`** — درصد diff نهایی که قبلاً در تکرارهای قبل توسط هوش مصنوعی بررسی شده. `coverage:85%` = فقط ۱۵٪ کد بررسی‌نشده است.

تیم شما _دقیقاً_ می‌بیند کدام commitها بررسی شدند، vouched شدند یا skip شدند — در خود `git log`.

## سوالات متداول

### Review در مقابل Vouch در مقابل Skip؟

|                       | **Review**                  | **Vouch**                       | **Skip**                  |
| --------------------- | --------------------------- | ------------------------------- | ------------------------- |
| AI reviews the diff?  | ✅ Yes                      | ❌ No                           | ❌ No                     |
| Takes responsibility? | ✅ Yes                      | ✅ Yes, explicitly              | ⚠️ No                     |
| Tracks iterations?    | ✅ Yes                      | ✅ Records prior coverage       | ❌ No                     |
| Git log message       | `ran (iter:N, coverage:X%)` | `vouched (iter:N, coverage:X%)` | `skipped`                 |
| When to use           | Each review cycle           | Done iterating, ready to commit | Not reviewing this commit |

**Review** پیش‌فرض است. هوش مصنوعی diff staged شما را تحلیل و بازخورد خطی می‌دهد. هر بررسی یک تکرار در چرخهٔ تغییر–بررسی است.

**Vouch** یعنی شما _صریحاً مسئولیت_ این commit را می‌گیرید. معمولاً بعد از چند تکرار بررسی — رفت‌وآمد کردید، مسائل را اصلاح کردید و الان راضی‌اید. هوش مصنوعی دوباره اجرا نمی‌شود، ولی آمار تکرار و coverage قبلی ثبت می‌شود.

**Skip** یعنی این commit را بررسی نمی‌کنید. شاید پیش‌پاافتاده است، شاید حیاتی نیست — دلیل با شماست. git log فقط `skipped` را ثبت می‌کند.

### چطور رایگانه؟

`git-lrc` از **API جمینی گوگل** برای بررسی‌های هوش مصنوعی استفاده می‌کند. جمینی سطح رایگان سخاوتمندانه دارد. شما کلید API خودتان را می‌آورید — صورتحساب واسطه نیست. سرویس ابری LiveReview که بررسی‌ها را هماهنگ می‌کند برای توسعه‌دهندگان انفرادی رایگان است.

### چه داده‌ای فرستاده می‌شود؟

فقط **diff staged** تحلیل می‌شود. هیچ زمینهٔ کامل ریپو آپلود نمی‌شود و بعد از بررسی diffها ذخیره نمی‌شوند.

### می‌توانم برای یک ریپوی خاص غیرفعالش کنم؟

```bash
git lrc hooks disable   # disable for current repo
git lrc hooks enable    # re-enable later
```

### می‌توانم یک commit قدیمی‌تر را بررسی کنم؟

```bash
git lrc review --commit HEAD       # review the last commit
git lrc review --commit HEAD~3..HEAD  # review a range
```

## مرجع سریع

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

> **نکته:** `git lrc <command>` و `lrc <command>` قابل تعویض هستند.

## رایگانه. به اشتراک بگذارید.

`git-lrc` **کاملاً رایگان** است. بدون کارت اعتباری. بدون trial. بدون تله.

اگر به دردتان خورد — **با دوستان توسعه‌دهنده به اشتراک بگذارید.** هرچه بیشتر کد تولیدشدهٔ هوش مصنوعی بررسی شود، باگ کمتری به تولید می‌رسد.

⭐ **[به این ریپو ستاره بدهید](https://github.com/HexmosTech/git-lrc)** تا دیگران هم پیدا کنند.

## جامعه

در **جامعهٔ دیسکورد git-lrc** دربارهٔ امکان‌ها، بهبودها، قابلیت‌ها گفتگو کنید و برای مشارکت کمک بگیرید: [discord.gg/sGdnKwB3qq](https://discord.gg/sGdnKwB3qq).

## مجوز

`git-lrc` تحت گونهٔ تغییریافتهٔ **Sustainable Use License (SUL)** توزیع می‌شود.

> [!NOTE]
>
> **یعنی چه:**
>
> - ✅ **Source Available** — کد منبع کامل برای self-hosting در دسترس است
> - ✅ **Business Use Allowed** — از LiveReview برای عملیات داخلی کسب‌وکار استفاده کنید
> - ✅ **Modifications Allowed** — برای استفادهٔ خود سفارشی کنید
> - ❌ **No Resale** — قابل فروش مجدد یا عرضه به‌عنوان سرویس رقیب نیست
> - ❌ **No Redistribution** — نسخه‌های تغییر یافته به‌صورت تجاری قابل توزیع مجدد نیستند
>
> این مجوز تضمین می‌کند LiveReview پایدار بماند و در عین حال دسترسی کامل برای self-host و سفارشی‌سازی را دارید.

برای شرایط دقیق، نمونهٔ استفاده‌های مجاز و ممنوع و تعاریف، [LICENSE.md](LICENSE.md) کامل را ببینید.

---

## برای تیم‌ها: LiveReview

> تنها از `git-lrc` استفاده می‌کنید؟ عالی. با تیم می‌سازید؟ **[LiveReview](https://hexmos.com/livereview)** را ببینید — مجموعهٔ کامل برای بررسی کد هوش مصنوعی در سطح تیم، با داشبورد، سیاست‌های سطح سازمان و تحلیل بررسی. هرچه `git-lrc` انجام می‌دهد، به‌علاوهٔ هماهنگی تیم.
