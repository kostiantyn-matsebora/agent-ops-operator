# Message line breaks

## Why

**One answer, two shapes.** `internal/dispatch/templates/format.md` tells every
agent to put a section label on its own line and the content on the next:

```
**Root cause**
{1–3 lines}
```

Telegram renders that as written — `channels/telegram/render.go` returns every
non-table line untouched, line for line. The console renders it as one run-on
paragraph, because `react-markdown` follows CommonMark, where a single newline
inside a paragraph is a space.

**Neither renderer is wrong, because the contract never said.** The subset names
what may be written — bold, italic, inline code, fenced blocks, links — and says
nothing about what a newline MEANS. Two surfaces read the same silence
differently, and the reader of one of them gets a wall of text.

It surfaced on the published site: the console screenshots showed
`**Root cause**` run together with the sentence under it, in exactly the shape
`format.md` exists to prevent.

## What Changes

- **The contract states it: a NEWLINE IS A LINE BREAK**, on every surface. It is
  the only reading that matches what agents are told to write, and the one
  transport that already ships is already doing it.

- **The console honours it.** `remark-breaks` in the transcript renderer, so a
  message written to a template renders as the template draws it.

- **`format.md` gains the corollary: DO NOT HARD-WRAP PROSE.** A newline the
  agent types is one the reader sees, so wrapping a sentence at 80 columns is
  now a formatting decision rather than source tidiness. The surface wraps.

- **The screenshot fixture stops hard-wrapping its answer**, and the site's
  published screenshots and recording are regenerated from it.

## Capabilities

### New Capabilities

*(none — the subset already has a home)*

### Modified Capabilities

- `adapter-rendered-messages`: the markdown subset gains the meaning of a
  newline, binding on every renderer — external adapters and in-process
  providers alike — so two surfaces cannot render one message in two shapes.

## Impact

**The console.** `platform/console/ui/src/components/Markdown.tsx` plus one dependency
(`remark-breaks`) and its tests. A rendering change means a **console image
release**, its tag in the chart values, and a `docs/CHANGELOG.md` entry.

**The contract.** `internal/dispatch/templates/format.md` (what an agent is told
to write) and `docs/contracts.md` (what the subset is).

**The site.** `platform/console/ui/screenshots/fixture.ts` unwraps its answer, and the
twelve screenshots plus the landing recording are regenerated — both are build
output, so this is a command, not an edit.

**Not affected.** No CRD, no manager code, no stored data: nothing is stored
rendered, so old transcripts simply re-render correctly.

**`channel-telegram` changes not at all** — it already does this. It gains a test
that says so, because the whole point is that the two surfaces stop drifting.
