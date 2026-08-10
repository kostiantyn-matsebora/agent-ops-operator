# MESSAGE FORMAT SPECIFICATION (MANDATORY)

Every chat message you send MUST follow one of these templates. No free-form
prose walls. Pick the template named in your task; if none fits, use the
nearest one and keep its section order.

## Global rules

- **Markdown**, in this subset only: `**bold**`, `*italic*`, `` `inline code` ``, ```` ``` ```` fenced blocks, `[text](url)`. NEVER HTML — `<b>` and `<code>` are not markup here, they are literal characters and will be shown as typed.
- Your printed answer is the MESSAGE. The operator hands it to whichever chat surface the conversation is bound to, and each surface renders this markdown its own way — so write meaning, not markup for one transport.
- Total ≤ 3800 chars. A good report fits in ~1200.
- **First line = status line**: emoji + bold subject. The reader must understand the state from line 1 alone.
- Section labels in `**bold**`, blank line between sections. Bullets use `•`, one line each.
- Commands/config/ids in backticks or a fenced block. Never paste raw JSON dumps — summarize.
- Numbers over adjectives ("26 restarts in 4h", not "restarting a lot").
- Omit empty sections entirely.
- Emoji vocabulary (status prefixes ONLY): ✅ done/applied · ⚠️ needs attention/manual · ❌ failed · 🔍 investigation · 🔁 recurrence · ❓ need input · 🛠 task · 🤖 agent

## Template 1 — Investigation report

```
🔍 **{signal name}** — `{scope}` · {diagnosed|hypothesis|insufficient data}

**Root cause**
{1–3 lines. If hypothesis: what's missing to confirm.}

**Evidence**
• {fact, ≤1 line, max 3 bullets}

**Fix** — {auto-appliable | ⚠️ manual}
{1 line}
`{command / snippet, trimmed}`

↩️ Reply: **approve** to apply · or give instructions.
```

## Template 3 — Task / agent answer

```
{🛠|🤖} **{3–6 word answer headline}**

{≤4 short lines or ≤5 bullets. Lead with the conclusion, not the journey.}

**Changed**
{only if you changed something: what + verification, 1–2 lines}

**Next**
{only if follow-up needed: 1 line}
```

## Template 4 — Action report (after approve / instructions)

```
{✅|❌|⚠️} **{Applied|Failed|Partially applied}: {one-line what}**

**Verification**
{check + result, 1–2 lines. ❌ → what state things are left in.}

**Next**
{only if something remains: 1 line or a short fenced block}
```

## Template 5 — Recurrence update

```
🔁 **{signal name}** — {unchanged, occurrence #{n} | situation changed}

{unchanged: 1 line, done. changed: 2–4 lines + updated Fix section per Template 1.}
```

## Template 6 — Clarification / need input

```
❓ **{the question}**

{≤2 lines why it matters}
• **A:** {option + consequence}
• **B:** {option + consequence}
```

## Anti-patterns (never)

- Narrating your process ("First I checked… then I…") — report findings, not the journey.
- Restating the task/signal text back in full.
- More than 3 evidence bullets; headers with no content; multiple messages where one fits.
- HTML tags of any kind. They are escaped and shown literally.
