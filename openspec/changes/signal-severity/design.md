## Context

See proposal.md for why this is its own change. What the tree holds today:

- A normalized signal is `{fingerprint, labels, title?, payload?, kind?,
  reader?}`. `severity` is a label key two adapters already fill from
  their own scale — k8s-events from the Event type, alertmanager from the
  rule's `severity` label. `k8s-events/enrich.go` already reserves the
  `severity` label key against pod-label override.
- `SignalProvenance` (`sourceRef`, `labels`) and `ConversationInputSpec`
  (`type`, `payload`, `labels`) carry per-conversation and per-input signal
  facts today. Neither carries a rating.
- The `signal` outbound message (`internal/chat/message.go`) carries
  `pipeline`, `source`, `title`, `labels`, `body`, `inputRef` — structured
  fields a surface renders as it chooses. It carries no severity.
- The console transcript (`platform/console/transcript.go` →
  `platform/console/ui/src/pages/Conversation.tsx`) renders a signal's
  `body` and a foldable `payload`. It draws no labels and nothing else
  structured from the message today.
- `signal-attributes-and-icons` (#253) drafted severity, type and icons
  together, including this same CRD field, the same endpoint validation
  and the same adapter mappings. This change supersedes that draft for
  severity, and #253 is trimmed in the same commit.

## Goals / Non-Goals

**Goals:**

- One severity vocabulary enforced at the one door every signal passes.
- Each shipped adapter mapping its own scale onto it, with no behavior
  change for an adapter or a manifest that sends none.
- The console transcript showing a rating where one exists, unchanged
  where none does.
- Nothing breaking.

**Non-Goals:**

- Routing on severity. `signal-attributes-and-icons` still owns the
  `match` field on Pipeline ref lists. This change does not add it, and a
  signal's severity is not yet anything a Pipeline can filter on.
- Icons of any kind, for severity or otherwise. `resource-icons` stays in
  #253.
- Telling the agent about severity in its prompt. The metadata block is a
  separate, larger prompt change (kind, severity and labels together) that
  stays in #253 until it is itself split or implemented whole.
- Telegram rendering severity. Only the console transcript is in scope,
  per the proposal's explicit "console chat" ask.
- Re-binding a running conversation when a later signal is rated
  differently.

## Decisions

### Severity is a field, not a label

Same reasoning #253 already settled, carried over rather than re-argued:
the set is closed and validated, so it cannot live in the open label map
without making that map two things. `kind` is the existing precedent.

**Rejected:** a label with validation on that one key. It would be the
only validated label, invisible in the shape, and every adapter would
still write the label and read the rule.

### The vocabulary, the storage shape and the adapter mapping table are one capability: `signal-severity`

`signal-attributes-and-icons` named this `signal-severity-and-type`
because severity and the reserved `type` label share nothing but a
proposal section. Splitting them into two capabilities — `signal-severity`
here, `signal-type` left in #253 — means each stands as a complete,
independently archivable contract.

**Rejected:** keeping one capability and letting this change ADD to it
while #253 still claims it. Two pending changes writing requirements
under the same capability path is exactly the drift that the archive
tooling assumes does not happen.

Only one change may own a capability's deltas until it archives.

### `signal-attributes-and-icons` is trimmed in this same commit

Leaving its severity-specific proposal text, design decisions, tasks and
delta spec in place would mean two pending changes specifying the same
field, the same validation and the same three adapters' mappings.

Whoever applies #253 second would find CRD fields, endpoint behavior and
adapter tests already done, with no record saying why.

So this change also edits `openspec/changes/signal-attributes-and-icons/`:

- `proposal.md`: drop the severity bullets from "What Changes", narrow the
  `signal-severity-and-type` capability line to `signal-type`, and drop
  the severity rows from "Impact".
- `design.md`: drop the "Severity is a field, type is a label" and
  "Severity rides on provenance and on each input" decisions (both move
  here), and drop severity's row from the migration plan.
- `tasks.md`: drop 1.3 (the `Severity` enum and its two fields), narrow
  4.1–4.3 to their `type`-only and matcher-only concerns, drop 9.4's
  severity assertion if nothing else in that task needs adapter-built
  binaries, and renumber.
- `specs/signal-severity-and-type/` is renamed to `specs/signal-type/`.
  "Severity is a closed vocabulary", "Severity is kept with the
  conversation and with each input" and "Each shipped adapter maps its
  own scale once" are deleted — this change owns them now. "Type is a
  reserved label key" stays as written. "The agent is told what it is
  handling" also stays as written: it names kind, severity and labels
  together as one prompt change the proposal already deferred, and
  splitting it further is its own decision, not a side effect of this
  one.

**Rejected:** leaving #253 untouched and letting `openspec archive` sort
out the collision later. The archive tooling has no merge step for two
changes claiming one capability — whichever archives second fails
validation naming the conflict, discovered at the worst possible time.

### Severity rides on provenance and on each input

`SignalProvenance.Severity` is written once, with the labels, at creation.
`ConversationInputSpec.Severity` is written per input, beside the labels
it already carries — the same place and the same lifecycle as `Labels`
today, so no new write path is needed, only a new field on an existing
one.

The agent's prompt is explicitly not touched here (Non-Goals), so this
field exists for the provenance record and the console card alone until a
later change reads it into a dispatch template.

### The console reads severity the same way it already reads `labels`

`internal/chat/message.go`'s `SignalMessage` constructor already fills
`Labels` from the input's recorded labels. Severity is filled from the
same input's new field, through the same function, so there is exactly
one place that composes a `signal` message's structured fields.

`platform/console/transcript.go`'s `Message` struct gains `Severity
string `json:"severity,omitempty"`` beside `Payload`, filled from the
inbound op the same way `Payload` already is.

The frontend `Message` interface gains `severity?: string`, and
`Conversation.tsx` draws it where present, tinted per value from the
theme's existing status tokens: `critical` and `error` use `--ao-danger`,
`warning` uses `--ao-warning`, `info` uses `--ao-neutral`. No new token is
defined for this change.

**Rejected:** resolving an icon for severity here. That ladder
(`chat.ResolveIcon`, `IconRef`) does not exist until `resource-icons`
lands in #253, and inventing a one-off icon mechanism for this change
alone would be a second ladder to reconcile with the real one later.

### Adapter mapping is fixed per adapter, configurable only where the sender's scale is open

Unchanged from #253's reasoning. k8s-events and ha map by a constant
table, because their upstream scales are closed.

alertmanager maps through `config.severityMap`, because a Prometheus
rule's `severity` label is whatever the rule author typed, with identity
defaults for the `critical`/`warning`/`info` convention. An unmapped value
is reported on the source's Ready message rather than refusing the alert.

## Risks / Trade-offs

- [Two pending changes briefly both reference severity while this one is
  being written] → resolved by trimming #253 in the same commit as this
  change's artifacts, so no intermediate state is ever committed.
- [An adapter outside this repository still sends `severity: Warning`] →
  the 400 names the vocabulary, and the contract page carries the mapping
  tables as the worked example — same posture #253 already chose.
- [The console card's severity tint drifts from the one `signal-attributes-and-icons`
  later pairs with an icon] → both read the same four-value vocabulary and
  the same theme tokens. Adding an icon beside an existing tint is
  additive, not a redesign.

## Migration Plan

1. Apply the new CRDs before the manager upgrade (`kubectl apply -f
   chart/crds/`), as every CRD change here requires. A manager started
   against old CRDs sees `severity` pruned from both types and behaves as
   before.
2. Upgrade adapters in any order. An old adapter sends no severity and its
   signals are unrated. A new adapter against an old manager has its
   `severity` field pruned by the JSON decoder (unknown fields are
   ignored) and is otherwise unchanged.
3. Upgrade the console. An old console build ignores the new `severity`
   field on the message it already receives.
4. Rollback is the reverse at every step: an old component ignores a field
   it does not know, and nothing it already did depended on severity
   existing.
