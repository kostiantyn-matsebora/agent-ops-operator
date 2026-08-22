## Context

See `proposal.md — Why`. The current state, in three facts that shape everything
below:

- `chat.Message.Body` is one opaque markdown string. `internal/chat/message.go`
  carries typed fields around it but nothing inside it.
- `channels/telegram/render.go` converts the markdown subset to HTML and splits at
  4096 characters, preferring a paragraph break. `platform/console/ui/src/components/Text.tsx`
  does the opposite: it prints the string verbatim and **strips every tag**, on a
  deliberate no-`dangerouslySetInnerHTML` rule.
- `/channel/ops` already negotiates `?contract=`, so a version bump has a home
  and a graceful path for adapters that have not upgraded.

Constraint from `CLAUDE.md`, load-bearing throughout: **the manager composes
meaning, adapters compose presentation.** No transport dialect under `internal/`.

## Goals / Non-Goals

**Goals**

- One parser, in the manager. Adapters receive structure and render it.
- An OPEN section vocabulary with a CLOSED fold, so agents differ and adapters
  do not.
- No adapter, prompt or profile has to change for existing behaviour to survive.

**Non-Goals**

- Making agents terse. This makes verbosity foldable, not absent.
- Actionable blocks (`<choice>` → a real button). See Open Questions.
- Restructuring signal payloads. Opt-in only, and no shipped adapter opts in
  here.

## Decisions

### D1 — The manager parses, not the adapters

**Chosen:** agent-reported bodies are parsed manager-side and `Message` carries
`blocks[]`. WHERE that parse is called is D9.

**Alternative:** each adapter parses tags out of `body`.

**Why:** an adapter deciding what `<details>` MEANS is meaning-composition, which
the architecture assigns to the manager. Practically, it is also the escaping
argument: `render.go` escapes `&<>` on every interpolated value because *an alert
payload containing `<` once broke Telegram parsing and the message with the
incident in it was the one that failed*. Agent output contains `<` constantly.
With N parsers, every adapter must independently get "recognize this grammar,
escape everything else" right. With one, it is one place to be correct and one
place to fix.

The console makes the same argument from the other side: it renders blocks as
React elements, so it gets rich output while KEEPING its rule that no string is
handed to the DOM. Markdown-in-a-string would force a parser plus a sanitizer
into the console.

### D2 — Open vocabulary, closed fold

**Chosen:** two reserved tags (`<title>`, `<details>`); everything else is an
agent-named section rendered in written order.

**Alternative:** a fixed tier set (`title` / `summary` / `action` / `details`)
known to manager and adapters.

**Why:** every agent has a different job. An `alert-investigator` wants *root
cause / evidence / fix*; a `ha-operator` wants *what I checked / what I changed*.
A fixed set makes every agent contort into an investigation report. The adapter
never needed the names — it needs to know where the fold is, and that is one
reserved tag.

**Cost, accepted:** the manager cannot REORDER sections, because with open names
it cannot tell which one is the conclusion. Order is the agent's, which is the
profile's. The guarantee weakens from "these sections in this order" to
"above the fold is bounded" — see D4.

### D3 — Grammar recognition rules

A tag is recognized only when it is (a) alone on its own line at line start,
(b) part of a well-formed open/close pair, and (c) outside fenced and inline
code. Everything else is literal.

**Why all three:** an open vocabulary means any `<word>` could be a tag, so `if
x < y`, `Deployment<T>` and a shell redirect must all be safe. Condition (a)
alone handles mid-line `<`. Condition (b) handles an agent that opens a tag it
never closes — such a region is closed at end of output rather than discarded,
because losing an agent's words to a grammar slip is the worst available
failure. Condition (c) is why code samples about this very grammar survive.

**Inline tags are excluded by design.** Inline formatting stays the existing
markdown subset. Inline tags satisfy none of (a)-(c), are where models produce
the most malformed output, and would reopen the escaping problem the contract
closed.

### D4 — The manager bounds the fold; it never truncates

Above-the-fold content is capped. Overflow is **demoted into `<details>`**.

**Why demote rather than refuse:** refusing means a round trip and an agent that
may fail the same way twice, with the user waiting. Demotion always produces a
readable message and loses nothing.

**Tension with an existing requirement, resolved explicitly:**
`adapter-rendered-messages` says the manager SHALL NOT truncate bodies and SHALL
NOT declare a maximum message size. Demotion does neither — nothing is removed
and no transport limit is asserted. Which part is the summary is a question about
meaning. The delta spec states this so a later reader does not read it as a
violation and "fix" it.

### D5 — Contract 3 serves contract 2, rather than refusing it

`Message` gains `blocks[]`; `body` stays populated as the flattened equivalent.
An adapter declaring `contract=2` gets `body` and no `blocks[]`.

**Why not refuse, as v1 was refused:** v1 was refused because it had no field to
render — it would have posted empty messages and looked healthy. v2 has `body`
and renders a complete, correct message. Refusing a conforming adapter to force
an upgrade would be a self-inflicted outage on every install that has a
third-party adapter.

The invariant that makes this safe: **`body` is never empty when `blocks[]` is
populated.** Flattening is total.

### D6 — The profile flag gates the prompt, never the parse

`AgentProfile` gains a flag. Set: `format.md` is injected. Unset: nothing is
injected.

Parsing is unconditional either way. That decoupling is what makes the flag
harmless: a profile that declines the shared spec and teaches its own agent the
grammar still gets folded output, and a profile that declines everything gets
one block, which is today's rendering.

This also resolves the earlier idea of a chart-level syntax switch. Two prompt
dialects plus two parser modes can be misconfigured into a state where the model
emits tags the parser is not looking for — reproducing the literal-tags-in-chat
failure as a config option. One tolerant parser plus a prompt that varies per
profile has no such state.

### D7 — `format.md` keeps the grammar, sheds the templates

What stays: the block grammar, the fold rule, the inline markdown subset, the
length budget, the emoji/status conventions, the anti-patterns.

What goes: the five mandatory report templates. They become the DEFAULT section
set that the flag injects — a starting point a profile overrides, not a form
every agent must fit.

The file itself is written in the new grammar, so the spec demonstrates what it
specifies.

### D8 — Signal parsing is opt-in on the inbound signal

Default raw. An adapter emitting structured bodies declares it per signal.

**Why not a `SignalSource` or `SignalAdapter` field:** the same reasoning the
bare-chat lane uses — the arriving signal carries what is true about itself, and
a declaration on a CR is one more thing an adapter author can get wrong while the
body says otherwise. Per-signal also lets one adapter emit both kinds.

**Why default raw:** a chat signal's body is a person's typed words. Someone
asking why `<details>` will not render in their docs must see their own
characters.

### D9 — The parse follows agent-reported TEXT, not the message KIND

**Chosen:** a run's reply carries blocks whether it leaves as an `answer` or as a
`notice`.

`chat.RunReplyMessage` sends a run that FAILED but explained itself as
`Warn(body)` — a `notice` — deliberately, so the reader gets the reason instead
of "run failed". That explanation is agent output, and it is the longest thing a
failed investigation produces. Leaving `notice` blockless would exempt the one
case the fold serves best.

`notice` therefore gains `blocks[]`, populated only where its body is
agent-reported. A manager-composed notice — a `/pipelines` listing, a refusal, a
usage error — is the manager's own text and carries none.

**Where the parse is CALLED follows from the same helper.** `RunReplyMessage`
composes from the RECORDED RUN ALONE, so that `/work/done` and the reconciler
backstop produce the same message from the same facts. The parse is a pure
function called there, never a step on the `/work/done` path — parsing at the
report point would leave the backstop's re-derived reply blockless, which is the
"a re-derived reply would differ from the one it replaces" failure that helper
exists to prevent.

**Blocks are therefore DERIVABLE from `status.runs[].result` and are not
persisted.** Storing them would add a fourth kind of manager state to a matrix
that admits three.

## Risks / Trade-offs

- **Agents ignore the grammar** → parsing is total; no tags yields one block,
  which renders exactly as today. The floor is current behaviour.
- **A model emits malformed tags** → recognition rules D3 make malformed input
  literal text rather than lost text. Unclosed regions close at end of output.
- **Section-name sprawl across profiles** → accepted. Adapters render labels
  generically, so sprawl costs consistency between agents, not correctness. The
  default section set from D7 gives most profiles a common shape for free.
- **Open vocabulary loses the ordering guarantee** → mitigated by D4's cap plus
  the profile prompt naming sections in order. Stated in the spec so the weaker
  promise is not mistaken for the stronger one.
- **Telegram's expandable blockquote needs a recent Bot API** → verify the
  minimum version during implementation; fall back to a plain quote plus a
  visible marker if unavailable, which degrades to today's readability rather
  than failing.
- **A third-party adapter never upgrades** → D5. It stays on contract 2
  indefinitely and renders correct messages.

## Migration Plan

1. **Wire first, prompts last.** Ship `blocks[]`, the parser, contract 3 and the
   flattening while no prompt emits tags. Every message has one block; nothing
   changes on any surface. This is a no-op release that can be verified in
   production.
2. **Adapters.** Telegram and console render blocks. Still a no-op — there is
   one block to render.
3. **Prompt.** Rewrite `format.md` in the grammar and add the profile flag,
   default OFF. Nothing changes until a profile opts in.
4. **Opt in.** Turn the flag on for the chart-shipped profiles
   (`k8s-engineer`, `alert-investigator`, `ha-user`, `ha-operator`) and observe.
5. **Signals.** Nothing here. No shipped adapter declares a tagged body.

**Rollback:** at any step, clearing the profile flag returns every agent to
untagged prose, which parses to one block and renders as it does today. No data
migration, no CR rewrite.

## Open Questions

- **The above-the-fold cap value.** Needs a real number, and the right way to
  choose one is to look at recent answers rather than guess. Does not change the
  specs, the approach or the tasks.
- **A `<choice>` block tag.** Stated as "the contract gains offered actions",
  which was WRONG: `Message.Choices` and `Message.InReplyTo` already exist and
  the main spec already requires both to stay structured and opaque. What is
  actually missing is a way for the AGENT to populate `choices` — a reserved tag
  the parser lifts into the existing field.

  The inbound half is the real cost and is unchanged: Telegram callbacks arrive
  through `gateway-telegram`, which classifies on `is_topic_message`, and a
  `callback_query` carries that field one level down, so the router needs a new
  rule. Scoped OUT as its own change. Nothing here forecloses it.
