## Why

An agent's answer reaches chat as one opaque prose blob, and both surfaces fail
it differently. The console renders it as literal text — `PlainText` prints
`**bold**` with the asterisks showing and `plain()` strips every tag — while
Telegram renders the markdown subset properly. Neither can fold a long tail,
because nothing on the wire says which part is the conclusion and which is the
kilobyte behind it.

The prompt already asks for brevity and conclusion-first ordering
(`internal/dispatch/templates/format.md`) and agents do not reliably comply. A
prompt cannot make a structural promise. Naming the parts IN THE OUTPUT can —
and the contract already works this way for markdown, which the manager passes
through and every adapter renders to what it has.

## What Changes

- **A block grammar for agent output.** Two RESERVED block tags — `<title>`
  (first, one line) and `<details>` (THE FOLD) — plus arbitrary agent-named
  sections rendered in written order above the fold. The vocabulary is OPEN and
  the fold is CLOSED: adapters learn where to fold, never what any agent's
  sections mean.
- **THE ADAPTERS PARSE.** The grammar joins the markdown subset as part of the
  documented body contract, and each adapter reads it and renders to its own
  capability — an expandable blockquote on Telegram, a disclosure control in the
  console. The manager passes an agent's text through untouched.
- **NO WIRE CHANGE.** `Message` gains no field and the contract version does not
  move. A parsed structure on the wire would be a second representation of text
  the message already carries.
- **A failed run's explanation is structured too.** It leaves as a `notice`
  rather than an `answer`, so an adapter parses AGENT-REPORTED text rather than
  keying on the message kind — otherwise the longest thing a failed
  investigation produces would be the one case that cannot fold.
- **A `signal` is NEVER parsed.** It carries structured fields — title, source,
  labels, payload — and the adapter renders a CARD from them. That is the one
  place an adapter needs a second renderer, and both already have one.
- **Nothing bounds the fold.** Length is the prompt's business. Telegram splits
  at BLOCK boundaries so a split message still opens with the conclusion.
- **`AgentProfile` gains an output-format flag.** When set, the shared format
  spec is injected into the prompt. When unset, nothing is injected and the
  profile's own prompt owns formatting entirely.
- **`format.md` is rewritten in the tag grammar.** Its five report templates
  (investigation / task answer / action report / recurrence / clarification) are
  REMOVED as mandatory forms — each agent's sections come from its profile. What
  remains is the grammar, the fold rule, the inline markdown subset, and a
  default section set for agents whose profile says nothing.
- **Parsing is always tolerant.** No tags at all yields ONE block, which renders
  as agent output renders today. The flag gates the PROMPT, never the parse, so
  an agent that emits tags without it is still folded.
- **HISTORY RENDERS LIKE LIVE OUTPUT.** The tags are in `status.runs[].result`,
  so a viewer rebuilding a transcript from the record parses the same characters
  a live message carried. Nothing is derivable-but-underived.
- **Console renders blocks as elements.** `<details>` becomes a real disclosure
  element. The app's no-`dangerouslySetInnerHTML` rule is preserved — blocks are
  already elements, so the console needs neither a markdown parser nor a
  sanitizer.

**BREAKING** — `format.md`'s templates are removed. A profile relying on the
built-in investigation/action templates keeps them only by enabling the flag,
which now injects the default section set instead.

## Capabilities

### New Capabilities
- `structured-agent-output`: the block grammar, its recognition rules and the
  fold. Who gets the format specification injected is stated ONCE, in
  `profile-is-identity`.

### Modified Capabilities
- `adapter-rendered-messages`: a free-text body is markdown in the named subset
  PLUS the block grammar, and the adapter reads both. The contract version does
  not move.
- `profile-is-identity`: a profile MAY declare that the shared output-format
  spec is injected into its prompt — identity, never capability.
- `telegram-channel-adapter`: parses the grammar, folds `<details>` into an
  expandable blockquote, splits at block boundaries.
- `console-application`: parses the grammar and renders it as elements with a
  real disclosure control, live and rehydrated alike.

## Impact

**The manager barely moves. The surfaces do the work.**

- `internal/chat/message.go` — NOTHING. No `Blocks` field, no version bump, no
  flattening. `RunReplyMessage` keeps composing from the recorded run alone and
  now passes the agent's text through as it was written.
- `internal/dispatch/templates/format.md` — rewritten in the grammar; prompt
  assembly gated on the profile flag and injected via the work unit's system
  prompt, so BOTH prompt lanes get it (a profile with its own `prompt` file
  sends the runtime a path, which nothing manager-side can append to).
- `api/v1alpha1` — `AgentProfile.spec.sharedOutputFormat`; deepcopy + CRD
  regeneration.
- `channels/telegram/` — a Go parser for the grammar, plus rendering: expandable
  blockquote for the fold, block-boundary splitting, markdown lists as spaced
  bullets, language-tagged `<pre><code>`.
- `platform/console/ui` — a TypeScript parser, plus block components with a real
  disclosure control. `Text.tsx` keeps its plain-text rule and no
  `dangerouslySetInnerHTML` is introduced — blocks are elements, and syntax
  highlighting goes through a rehype plugin that emits a tree rather than a
  string.
- `platform/console` — the transcript renders a rehydrated message exactly as a
  live one, because both parse the same characters.
- Docs: `docs/concepts.md` (the flag), `docs/contracts.md` (the grammar as part
  of the body contract, and the signal card's structured fields),
  `CHANGELOG.md` (template removal).
- `.claude/rules/invariants.md` — "THE MANAGER COMPOSES MEANING" binds the agent
  to the markdown subset through `format.md`. That line now also covers the
  block grammar, and must say the manager does not parse it.

## Open questions

- ~~The above-the-fold cap value.~~ **RESOLVED: there is no cap** — see design
  D4. Any value cuts markdown structure in half, which was observed on a live
  install. Brevity is the prompt's job.
- **A `<choice>` block tag.** The outbound contract ALREADY carries `choices` as
  a structured field on every message kind, so the open question is narrower than
  "add offered actions": it is whether the parser lifts a `<choice>` tag out of
  the agent's own text into that existing field. Scoped OUT here, and nothing in
  this change forecloses it.
