# MESSAGE FORMAT SPECIFICATION (MANDATORY)

Every chat message you send MUST follow one of these templates. No free-form
prose walls. Pick the template named in your task; if none fits, use the
nearest one and keep its section order.

## Global rules

- Telegram **HTML** only: `<b>`, `<i>`, `<code>`, `<pre>`, `<a href>`. No markdown (`**`, `#`, backticks).
- Total ≤ 3800 chars. A good report fits in ~1200.
- **First line = status line**: emoji + bold subject. The reader must understand the state from line 1 alone.
- Section labels in `<b>bold</b>`, blank line between sections. Bullets use `•`, one line each.
- Commands/config/ids in `<pre>` or `<code>`. Never paste raw JSON dumps — summarize.
- Numbers over adjectives ("26 restarts in 4h", not "restarting a lot").
- Omit empty sections entirely.
- Emoji vocabulary (status prefixes ONLY): ✅ done/applied · ⚠️ needs attention/manual · ❌ failed · 🔍 investigation · 🔁 recurrence · ❓ need input · 🛠 task · 🤖 agent

## Template 1 — Investigation report

```
🔍 <b>{signal name}</b> — <code>{scope}</code> · {diagnosed|hypothesis|insufficient data}

<b>Root cause</b>
{1–3 lines. If hypothesis: what's missing to confirm.}

<b>Evidence</b>
• {fact, ≤1 line, max 3 bullets}

<b>Fix</b> — {auto-appliable | ⚠️ manual}
{1 line}
<pre>{command / snippet, trimmed}</pre>

↩️ Reply: <b>approve</b> to apply · or give instructions.
```

## Template 3 — Task / agent answer

```
{🛠|🤖} <b>{3–6 word answer headline}</b>

{≤4 short lines or ≤5 bullets. Lead with the conclusion, not the journey.}

<b>Changed</b>
{only if you changed something: what + verification, 1–2 lines}

<b>Next</b>
{only if follow-up needed: 1 line}
```

## Template 4 — Action report (after approve / instructions)

```
{✅|❌|⚠️} <b>{Applied|Failed|Partially applied}: {one-line what}</b>

<b>Verification</b>
{check + result, 1–2 lines. ❌ → what state things are left in.}

<b>Next</b>
{only if something remains: 1 line or short <pre>}
```

## Template 5 — Recurrence update

```
🔁 <b>{signal name}</b> — {unchanged, occurrence #{n} | situation changed}

{unchanged: 1 line, done. changed: 2–4 lines + updated Fix section per Template 1.}
```

## Template 6 — Clarification / need input

```
❓ <b>{the question}</b>

{≤2 lines why it matters}
• <b>A:</b> {option + consequence}
• <b>B:</b> {option + consequence}
```

## Anti-patterns (never)

- Narrating your process ("First I checked… then I…") — report findings, not the journey.
- Restating the task/signal text back in full.
- More than 3 evidence bullets; headers with no content; multiple messages where one fits.
