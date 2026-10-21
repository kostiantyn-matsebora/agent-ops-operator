## Context

See `proposal.md` for the motivation. What the tree holds today:

- `SignalProvenance` (`conversation_types.go`) and `ConversationInputSpec`
  (`conversationinput_types.go`) already carry `labels` per-conversation and
  per-input. Neither carries a rating.
- `signals/k8s-events` and `signals/ha` each already write their own ad hoc
  `severity` **label** from their own scale. Nothing validates it, and
  nothing else in the manager reads it as anything but an opaque string.
- `internal/httpapi/signals.go`'s `NormalizedSignal` and `internal/chat
  /message.go`'s `SignalMessage` carry `labels` but no severity field.
- The console transcript draws a signal's `body` and its `labels`. It draws
  nothing else structured from the message today.

## Goals / Non-Goals

**Goals:**

- One optional, validated severity vocabulary at the one door every signal
  passes.
- No shipped adapter changes, no breaking change for any adapter or
  manifest that sends none.
- The console transcript shows a rating where one exists, unchanged where
  none does.

**Non-Goals:**

- Mapping any shipped adapter onto the field. `signals/k8s-events`,
  `signals/ha`, and `signals/alertmanager` keep writing their own `severity`
  label exactly as today — a later change maps a real sender onto this
  field.
- Routing, icons, a reserved `type` label, Telegram rendering, or an
  agent-prompt metadata block. None of those is this change's concern.
- Reconciling this with the other, unrelated open changes in this repo that
  also touch the word "severity" — out of scope by instruction, not an
  oversight.

## Decisions

### Severity is a field, not a label

A label is free-form per-adapter text with no shared meaning across
sources. `k8s-events` and `ha` already each write their own `severity`
label on their own scale — exactly the ambiguity this change solves rather
than extends.

A field gets a closed vocabulary the manager validates at the door, and a
shape every reader can depend on without parsing an adapter-specific
string.

Keeping the label alongside it, unaffected, is what makes this additive
rather than a migration.

### Severity rides on both provenance and per-input, like `labels` already does

`SignalProvenance` is the conversation's own memory of how it started,
written once and never touched again. Rewriting it on a later recurrence
would make the record lie about what opened the thread.

`ConversationInputSpec` is the per-occurrence half of each input, already
carrying that input's own `labels` for the same reason.

A later signal on the same conversation can disagree with the first
one — a `warning` that escalates to `critical`. That fact belongs to the
input that observed it, not retroactively to the provenance.

Giving severity the same two-stop placement adds one field to an existing
pattern instead of inventing a new one.

### The console reads severity exactly where it already reads labels

`message.go`'s `SignalMessage` composes the `signal` kind's structured
fields once, in one place under `internal/chat`. Every adapter, console
included, renders whatever structured fields show up without the manager
caring how.

Adding `severity` next to `labels` in that one struct gives the console's
signal-card renderer one more field to draw, with no new composition path.

The console's own spec delta is additive (`## ADDED Requirements`) rather
than a change to the fan-out or transcript requirements that already
exist. Drawing severity is a presentation decision local to the card, not
a change to how messages arrive.

### No new theme token

`--ao-danger`/`--ao-warning`/`--ao-neutral` already mean "bad/notable
/informational" elsewhere in the console. Mapping `critical` and `error`
onto the same danger token, rather than inventing a fifth shade, keeps
severity legible against everything else already tinted that way.

### Rejected: validating severity as a special-cased label

A validated label would be the only validated key in an otherwise opaque
map, invisible in the signal's shape, and every adapter would still write
a label and read a rule. `kind` already set the precedent that a closed,
meaningful value is a field.

## Risks / Trade-offs

- **The field ships with no shipped sender, so nothing exercises it in
  practice.** → Accepted. Proof is a hand-sent `curl POST /signal/inbound`
  carrying `severity: critical`, per the existing smoke-test pattern. A
  later change maps a real adapter onto it.
- **A reader expecting `labels["severity"]` (from `k8s-events`/`ha`) and
  the new `severity` field to mean the same thing.** → The field and the
  pre-existing label are unrelated by design in this change. Reference
  docs name both explicitly so the distinction is visible, not implied.
- **An adapter outside this repository sends `severity: Warning`.** → The
  400 names the four accepted values. The contract page documents the
  vocabulary as the worked example.

## Migration Plan

1. Apply the new CRDs before the manager upgrade (`kubectl apply -f
   chart/crds/`), as every CRD change requires. A manager started against
   old CRDs sees `severity` pruned from both types and behaves as before.
2. Upgrade the manager. No adapter needs to change — none sends the field,
   so none is upgraded as part of this change.
3. Upgrade the console. An old console build ignores the new `severity`
   field on the message it already receives.
4. Rollback is the reverse at every step: an old component ignores a field
   it does not know, and nothing already running depended on severity
   existing.
