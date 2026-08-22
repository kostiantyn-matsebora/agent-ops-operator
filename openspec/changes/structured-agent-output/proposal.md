## Why

An agent's answer reaches chat as one opaque prose blob, and both surfaces fail
it differently. The console renders it as literal text — `PlainText` prints
`**bold**` with the asterisks showing and `plain()` strips every tag — while
Telegram renders the markdown subset properly. Neither can fold a long tail,
because nothing on the wire says which part is the conclusion and which is the
kilobyte behind it.

The prompt already asks for brevity and conclusion-first ordering
(`internal/dispatch/templates/format.md`) and agents do not reliably comply. A
prompt cannot make a structural promise. Naming the parts on the wire can.

## What Changes

- **A block grammar for agent output.** Two RESERVED block tags — `<title>`
  (first, one line) and `<details>` (THE FOLD) — plus arbitrary agent-named
  sections rendered in written order above the fold. The vocabulary is OPEN and
  the fold is CLOSED: adapters learn where to fold, never what any agent's
  sections mean.
- **The manager parses ONCE.** `/work/done` bodies are parsed manager-side into
  typed blocks. Adapters render blocks; no adapter parses the grammar. A second
  parser in an adapter is the regression this shape exists to prevent.
- **Outbound contract v3.** `Message` gains `blocks[]` beside `body`. Adapters
  negotiating `contract=2` on `/channel/ops` receive `body` flattened to today's
  markdown, so an un-upgraded adapter keeps working unchanged.
- **The manager bounds the fold.** Above-the-fold length is capped; overflow is
  DEMOTED into `<details>`, never dropped. Telegram splits at BLOCK boundaries,
  so the first chunk is always the above-fold part.
- **`AgentProfile` gains an output-format flag.** When set, the shared format
  spec is injected into the prompt. When unset, nothing is injected and the
  profile's own prompt owns formatting entirely.
- **`format.md` is rewritten in the tag grammar.** Its five report templates
  (investigation / task answer / action report / recurrence / clarification) are
  REMOVED as mandatory forms — each agent's sections come from its profile. What
  remains is the grammar, the fold rule, the inline markdown subset, and a
  default section set for agents whose profile says nothing.
- **The parser is always on and always tolerant.** No tags at all yields ONE
  block. That is the whole backward-compatibility story, and it means the flag
  gates the PROMPT, never the parse.
- **Signal bodies are parsed only on OPT-IN.** A chat signal's body is a
  person's typed words, so parsing it by default would consume text somebody
  deliberately typed. An adapter emitting structured signals declares it on the
  inbound signal; absent that declaration the payload is raw, exactly as today.
- **Console renders blocks as elements.** `<details>` becomes a real disclosure
  element. The app's no-`dangerouslySetInnerHTML` rule is preserved — blocks are
  already elements, so the console needs neither a markdown parser nor a
  sanitizer.

**BREAKING** — `format.md`'s templates are removed. A profile relying on the
built-in investigation/action templates keeps them only by enabling the flag,
which now injects the default section set instead.

## Capabilities

### New Capabilities
- `structured-agent-output`: the block grammar, its parsing rules, the fold, the
  manager-side caps, and the `AgentProfile` flag that injects the format spec.

### Modified Capabilities
- `adapter-rendered-messages`: outbound messages carry `blocks[]`; contract
  version rises to 3 with a flattened `body` served to `contract=2` adapters.
- `profile-is-identity`: a profile MAY declare that the shared output-format
  spec is injected into its prompt — identity, never capability.
- `telegram-channel-adapter`: renders blocks, folds `<details>` into an
  expandable blockquote, splits at block boundaries.
- `signal-adapter-contract`: an inbound signal MAY declare that its body is
  tagged; absent the declaration the body is raw and unparsed.
- `console-application`: renders blocks as elements with a real disclosure
  control, replacing literal-markdown display of agent output.

## Impact

- `internal/chat/message.go` — `Message.Blocks`, `ContractVersion` 2 → 3, block
  types, flattening for v2 adapters.
- New manager-side parser package (grammar + normalization + caps).
- `internal/httpapi` — `/channel/ops` contract negotiation; `/work/done` parse
  point.
- `internal/dispatch/templates/format.md` — rewritten; prompt assembly gated on
  the profile flag.
- `api/v1alpha1` — `AgentProfile` flag field; deepcopy + CRD regeneration.
- `channel-telegram/render.go` — block rendering, expandable blockquote,
  block-boundary splitting.
- `console/ui` — block components; `Text.tsx` retains its plain-text rule for
  everything else.
- Docs: `docs/concepts.md` (the flag), `docs/contracts.md` (contract v3 and the
  grammar), `CHANGELOG.md` (template removal).

## Open questions

- **`<choice>` blocks.** Turning `Reply: **approve**` into a real button is the
  highest-value block, but it drags Telegram callback routing through
  `telegram-router`, which classifies on `is_topic_message`. Scoped OUT here.
