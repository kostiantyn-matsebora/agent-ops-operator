## Context

See `proposal.md — Why`. The current state, in three facts that shape everything
below:

- `chat.Message.Body` is one opaque markdown string. `internal/chat/message.go`
  carries typed fields around it but nothing inside it.
- `channels/telegram/render.go` converts the markdown subset to HTML and splits at
  4096 characters, preferring a paragraph break. `platform/console/ui/src/components/Text.tsx`
  does the opposite: it prints the string verbatim and **strips every tag**, on a
  deliberate no-`dangerouslySetInnerHTML` rule.
- `/channel/ops` negotiates `?contract=`, so a version bump HAS a home — and
  this change turns out not to need one. See D5.
- **Prose on the wire is already a grammar adapters read.** The contract names a
  markdown subset and each adapter renders it. Nothing parses markdown centrally,
  and that is the precedent block tags follow.

Constraint from `CLAUDE.md`, load-bearing throughout: **the manager composes
meaning, adapters compose presentation.** No transport dialect under `internal/`.

## Goals / Non-Goals

**Goals**

- One GRAMMAR, named by the contract. Each adapter reads it and renders to what
  its transport has.
- An OPEN section vocabulary with a CLOSED fold, so agents differ and adapters
  do not.
- No adapter, prompt or profile has to change for existing behaviour to survive.
  Nothing emits a tag until a profile opts in.

**Non-Goals**

- Making agents terse. This makes verbosity foldable, not absent.
- Actionable blocks (`<choice>` → a real button). See Open Questions.
- Restructuring signal payloads. Opt-in only, and no shipped adapter opts in
  here.

## Decisions

### D1 — The ADAPTERS parse, not the manager

**Chosen:** the manager passes an agent's reported text through UNTOUCHED, and
each channel adapter parses the grammar and renders it to its own capability.

**Alternative, and what this change first built:** the manager parses and the
message carries `blocks[]`.

**Why the reversal — three arguments, and the first is the one that settles it:**

**MARKDOWN IS ALREADY AN ADAPTER-PARSED GRAMMAR IN THIS CONTRACT.** The manager
does not parse markdown into a tree and ship it. It states that prose is
markdown in a named subset, and every adapter renders that to what it has.
Block tags are an extension of the same grammar and belong in the same place. A
parsed structure on the wire was the inconsistency, not the plain text.

**NOTHING MANAGER-SIDE CONSUMES BLOCKS.** The manager parsed, put the result on
the wire, and every consumer was a renderer. It routed nothing on them, decided
nothing from them, recorded nothing of them. That is presentation, which this
architecture assigns to the adapter.

**Manager-parsing broke the READ PATH, and could not not break it.** Blocks are
derivable and so are not persisted — but the only component allowed to derive
them ran on the WRITE path. A conversation reopened after a console restart
rebuilt its transcript from `status.runs[].result`, got the raw text, and
rendered it flat. Every deploy flattened all history. The fixes available were a
new endpoint to re-derive, or storing the same characters in etcd twice.

With the adapter parsing, that entire problem does not exist: the tags ARE
`status.runs[].result`, so a rehydrated transcript parses exactly like a live
message.

**Cost, accepted:** two parsers, Go for Telegram and TypeScript for the console,
which can drift. That is the price. It buys back a wire version, a downgrade
path for the previous one, and the read-path hole.

**What the original D1 argued, and why it does not hold:**

- *"An adapter deciding what `<details>` MEANS is meaning-composition."* The
  grammar is a SERIALIZATION of meaning the agent already composed. Reading it
  is no more meaning-composition than reading `**bold**` is.
- *"The escaping argument: with N parsers every adapter must get `recognize this
  grammar, escape everything else` right."* Real for Telegram, which composes
  HTML — and it already carries exactly that risk for markdown, with the same
  mitigation. Moot for the console, where React escapes everything and no string
  reaches the DOM as markup.

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
"above the fold is WHAT THE AGENT PUT THERE" — which is the honest promise, and
the one D4 explains cannot be strengthened by counting characters.

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

### D4 — NOTHING BOUNDS THE FOLD. Brevity is the prompt's job

**Chosen:** the manager imposes no length budget on above-the-fold content.

**Alternative, and what this change first built:** cap the above-fold length and
DEMOTE the overflow into `<details>`.

**Why it was removed, after one afternoon on a live install:**

A length budget is a GUESS AT IMPORTANCE, and with an OPEN section vocabulary
there is nothing better to guess with. Two failures, both observed:

- **It buried `<fix>`** — the one section a reader needs — because the section
  was last in written order when the budget ran out. Order is not importance.
- **It cut a markdown TABLE in half.** The header and one row stayed above the
  fold, and the other ten landed in the fold as raw `|` pipes with nothing to
  head them. A list was split from its own items the same way.

Trimming text at a line boundary is not a safe operation on markdown: a table
needs its header, a list its siblings, a fence its closing ticks. Structure is
what this change exists to preserve, and the cap destroyed it to save length.

**IMPORTANCE IS ALREADY DECLARED** — by the agent, when it chooses what goes
inside `<details>`. That is the one signal here carrying meaning, and the cap
second-guessed it with arithmetic.

**So brevity belongs to the PROMPT.** `format.md` can ask for short sections,
bulleted facts and no prose above the fold. It cannot be enforced by cutting the
result up afterwards, and a verbose agent now produces a long message. That is
the accepted cost, and it is smaller than a mangled one.

### D5 — No wire change at all

**Chosen:** `Message` gains no field, and the outbound contract version does not
move. The body carries the grammar, exactly as it already carries markdown.

**What this change first built:** `blocks[]` beside `body`, contract 2 → 3, and
a downgrade path serving a flattened `body` to adapters declaring 2.

**Why none of it is needed:** all three existed only to carry a parse the
manager should not have been doing. With the adapter parsing there is no second
representation to version, nothing to downgrade, and no adapter to refuse.

**Compatibility is the PROFILE FLAG, and it is enough.**
`AgentProfile.spec.sharedOutputFormat` defaults OFF, so no agent emits a tag
until an install turns it on — and an install turns it on when its adapters
understand them. An adapter that never learns the grammar simply serves profiles
that never opt in, and sees the prose it always saw.

**The failure this avoids** is an adapter posting literal `<title>` into a chat.
A contract version could also express that, but only if the manager could serve
the old shape — which means parsing, which is the thing being removed.

### D6 — `outputFormat` is a REQUIRED enum, not a flag with a default

**Chosen:** `AgentProfile.spec.outputFormat`, required, `blocks | none`.

**Alternatives, both rejected:**

| | Rejected because |
|---|---|
| default `false` (built first) | the feature is opt-in per profile, so realistically nobody switches it on and the work ships unused |
| default `true` | every existing profile changes behaviour silently on upgrade, and an adapter with no parser starts showing literal `<title>` |

**A DEFAULT IS THE WRONG SHAPE HERE BECAUSE BOTH DEFAULTS ARE WRONG.** Off means
unformatted output unless the profile author wrote their own format into the
prompt. On means output shaped by something the author never asked for. The
honest resolution is to refuse to guess: the author declares the contract.

**An ENUM, not a required bool.** A mandatory boolean reads as a toggle somebody
forgot to give a default; an enum reads as "declare your output contract", which
is what this is. It also leaves room for a third format without another field.

**REQUIRED BREAKS `kubectl apply` ON EVERY EXISTING PROFILE, and that is
accepted.** Stored objects stay readable and the manager keeps working — only
writes fail, loudly, with the valid values named. Chart-managed profiles are
re-applied with the field by the same upgrade. The blast radius is hand-written
profiles, once.

**It is the compatibility boundary too.** Nothing on the wire signals the
grammar (D5), so an adapter with no parser is protected by profiles declaring
`none`. Making the declaration mandatory is what stops that protection being
accidental.

**IT GATES THE PROMPT, NEVER THE PARSE.** An adapter parses whatever it is
given either way, so a profile declaring `none` whose agent emits tags anyway is
still rendered as blocks. Decoupling them is what keeps this safe: a switch that
moved the parser too could be configured into a state where the model emits tags
nothing is looking for.

**What it does NOT cover:** the operator's unconditional prompt content stays
unconditional. `deliverySection` — "your printed answer IS the deliverable" — is
a fact about the system rather than a preference, and is injected whatever this
field says. The line between them is exactly that: mechanism is not declared,
STYLE is.

### D7 — `format.md` keeps the grammar, sheds the templates

What stays: the block grammar, the fold rule, the inline markdown subset, the
length budget, the emoji/status conventions, the anti-patterns.

What goes: the five mandatory report templates. They become the DEFAULT section
set that the flag injects — a starting point a profile overrides, not a form
every agent must fit.

The file itself is written in the new grammar, so the spec demonstrates what it
specifies.

### D8 — A signal is never parsed. It is a CARD

**Chosen:** the grammar applies to AGENT-REPORTED text only. A `signal` message
carries STRUCTURED FIELDS — title, source, pipeline, labels, payload — and the
adapter renders a card from them.

**What this change first built:** a `structuredBody` declaration on the inbound
signal, opting its payload into the grammar.

**Why it is deleted:** it answered a question nobody asked. A signal body is a
machine document or a person's typed words, never prose written in this grammar,
and no shipped adapter would ever have set it.

**This is the one place an adapter needs a SECOND renderer**, and both already
have one: fields become a card, a body becomes parsed prose. Keeping the two
apart is what lets the card show a source and a label table while an answer
shows a title and a fold.

### D9 — The manager passes agent text through UNTOUCHED

**Chosen:** whatever the agent printed reaches the adapter as it was written.
The manager neither parses it, rewrites it, nor decides which message kind is
eligible.

This replaces a decision about WHERE the manager parsed. With no manager-side
parse there is no such place, and the question dissolves.

**Two consequences worth stating, because both were built the other way:**

- **`chat.RunReplyMessage` composes from the RECORDED RUN ALONE**, and that is
  unchanged and still load-bearing: `/work/done` and the reconciler backstop
  must produce the same message from the same facts. What changed is that the
  message it produces carries the agent's text verbatim.
- **A failed run that explained itself still leaves as a `notice`**, and its
  body is still agent text. An adapter parses it exactly as it parses an
  `answer` — the grammar follows the TEXT, never the message kind, and that
  rule now lives in the adapter rather than in the manager.

**Nothing is derivable-but-underived.** The tags are in
`status.runs[].result`, so a viewer rebuilding a transcript from the record
parses the same characters a live message carried.

## Risks / Trade-offs

- **Agents ignore the grammar** → parsing is total; no tags yields one block,
  which renders exactly as today. The floor is current behaviour.
- **A model emits malformed tags** → recognition rules D3 make malformed input
  literal text rather than lost text. Unclosed regions close at end of output.
- **Section-name sprawl across profiles** → accepted. Adapters render labels
  generically, so sprawl costs consistency between agents, not correctness. The
  default section set from D7 gives most profiles a common shape for free.
- **Open vocabulary loses the ordering guarantee** → the profile prompt names
  the sections it wants and in what order, and `format.md` supplies a default
  set. NOT mitigated by a cap: D4 is the record of trying that and finding it
  cut tables in half. Stated in the spec so the weaker promise is not mistaken
  for the stronger one.
- **Telegram's expandable blockquote needs Bot API 7.2** → there is no version
  probe, so the send itself is the probe: refused for the quote tag, the adapter
  latches the feature off and re-renders with a plain quote plus a visible
  marker. Degrades to today's readability rather than failing.
- **A third-party adapter never learns the grammar** → D5. It serves profiles
  that never set the flag and sees the prose it always saw. Turn the flag on for
  a profile whose conversations reach such an adapter and its readers see
  literal tags — that is the ONE compatibility hazard, and it is per-install and
  per-profile rather than global.
- **TWO PARSERS DRIFT** → accepted, and the cost of D1. The grammar is small and
  its recognition rules (D3) are stated once in the spec, which is what both
  implementations are written against. A conformance case list shared between
  the two test suites is the cheapest mitigation if they start to diverge.

## Migration Plan

**Nothing on the wire changes, so the order is: teach the surfaces, then the
agent.**

1. **Adapters learn the grammar.** Telegram and the console each parse and
   render blocks. No prompt emits a tag yet, so every message has none and
   nothing changes on any surface. Verifiable in production as a no-op.
2. **Prompt.** `format.md` is rewritten in the grammar, and `AgentProfile` gains
   the flag, default OFF. Still nothing changes until a profile opts in.
3. **Opt in, one profile at a time**, and read the result on every surface that
   profile's conversations reach.
4. **Signals.** Nothing, ever. A signal is a card (D8).

**Rollback:** clear the profile flag. The agent returns to untagged prose, which
every adapter already renders. No data migration, no CR rewrite, no adapter
rollback — the surfaces keep their parsers and simply meet nothing to parse.

## Open Questions

- ~~The above-the-fold cap value.~~ **ANSWERED BY DELETING THE CAP** — see D4.
  The number was guessed at 1200, tightened to 600 against a real answer, and
  then removed entirely once it was clear that any value cuts markdown
  structure in half. There is no number that makes trimming safe.
- **A `<choice>` block tag.** Stated as "the contract gains offered actions",
  which was WRONG: `Message.Choices` and `Message.InReplyTo` already exist and
  the main spec already requires both to stay structured and opaque. What is
  actually missing is a way for the AGENT to populate `choices` — a reserved tag
  the parser lifts into the existing field.

  The inbound half is the real cost and is unchanged: Telegram callbacks arrive
  through `gateway-telegram`, which classifies on `is_topic_message`, and a
  `callback_query` carries that field one level down, so the router needs a new
  rule. Scoped OUT as its own change. Nothing here forecloses it.
