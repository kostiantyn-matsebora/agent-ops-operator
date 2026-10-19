# Design — signal correlation id

## Context

See `proposal.md` — Why. The pieces this design replaces:

| Today | Where |
|---|---|
| signature from `labels` × `signatureLabels`, default `[alertgroup, alertname, namespace]` | `internal/ingest/grouping.go` |
| fingerprint cooldown, keyed per occurrence | `internal/ingest/grouping.go` |
| alert-vs-oneshot fallback patching the empty-signature hole | `internal/httpapi/signals.go` (~line 231) |
| `agentops.dev/signature-hash` label as the reuse lookup | `findReusableConversation` |
| three mappings for one identity — signature-hash label, `status.threads` lookup, member backrefs | manager + channel adapters |

The delta specs under `specs/` are the behavior reference. This file records
the decisions and their rejected alternatives.

**The whole contract in six words: signal adapter — propagate or create.
Channel adapter — always propagate.** The manager neither. It only compares.

## Goals / Non-Goals

**Goals:**

- One id, minted sender-side, equality-matched manager-side.
- The failure mode flips from silent wrong merge to loud `4xx`.
- One mapping (`conversationId` ↔ thread id) replaces three.

**Non-Goals:**

- No adapter using the external-system feedback loop ships here (enabled,
  not implemented — see Decisions).
- No conversion webhook, no backfill of legacy conversations.
- No change to lane semantics (`alert`/`job`/`task`/`chat` routing stays).

## Decisions

### Id ownership — one rule, no lanes

Every signal adapter MUST supply its own `initiationId`, one of two ways:

| Way | Means |
|---|---|
| minted | where the sender's aggregation happens — one id per "one problem" as the sender defines it |
| propagated | a round trip handed back a manager-issued `conversationId` — passed on VERBATIM, no re-derivation |

The manager never determines an id and treats every id opaquely — pure
equality, no parsing, no lane branch.

Chat is an instance, not a lane: a fresh human request is its own
aggregation unit (fresh mint), a reply in a bound thread is the propagation
case. Channel adapters mint NOTHING — downstream only, translation only.

**Rejected:** a manager-side default per kind (the bug restated), a
chat-only carve-out, and a separate continuation endpoint (a second
addressing mode is what the single id exists to delete).

**Enabled, not implemented:** the feedback loop. An adapter fronting an
external system that accepts input can carry the stamped `conversationId`
outward, receive the response with the id attached, and propagate it as
`initiationId`.

Equality routing appends it to the same conversation — agent, user and
external system in one interactive loop, with no new addressing mode.

### Equality routing

An OPEN conversation with `conversationId == initiationId`, originated by
the claiming pipeline, receives the signal as a recurrence. Otherwise a new
conversation adopts the id. Recurrence-after-close keeps the `-N` suffix.

**Rejected:** window reuse (`windowDays`). "Open" is the window now — a
sender re-notifying after close gets a fresh conversation on purpose.

### Reject, never synthesize

A signal without `initiationId` is rejected `4xx` at `POST /signal/inbound`,
naming the field. A 1.x adapter during rollout gets the same answer. The
contract's COMPAT section states it.

**Rejected:** synthesize-and-warn — a synthesized id is grouping policy
re-minted in the manager, which the invariant forbids.

### No backfill

`conversationId` applies to conversations created from this version on. A
legacy open conversation is never equality-matched — a recurrence opens a
new conversation beside it. Bounded one-time duplication, never a wrong
merge. `v1alpha1` is provisional, so no conversion webhook.

**Rejected:** backfill from `spec.signal` provenance (a wrong guess merges
strangers) and drain-before-upgrade (an operational demand for no gain).

### Stamping and the mapping collapse

The manager stamps `conversationId` on everything leaving the conversation:
member spawns, channel topic bindings, runtime context. Every `/channel/ops`
operation carries it. Thread mapping becomes transport translation only.

Wire impact is ADDITIVE on the channel side: `contract=` unchanged, the
channel adapter document moves MINOR, an adapter ignoring the field stays
conformant. The signal contract moves MAJOR to 2.0.0.

### Lookup mechanics — field index, no label

`initiationId` values are not label-safe (an Alertmanager `groupKey`
carries `{}`, quotes, spaces). The raw value never appears in a label or an
object name.

Equality routing reads an informer FIELD INDEX on `spec.conversationId`.
The `agentops.dev/signature-hash` label is removed without a successor —
every manager-side consumer moves to the index, and the console reads the
spec field directly.

**Rejected:** a digest label as the lookup (a second derived identity, the
exact shape this change deletes).

### Reopen keeps the at-most-one-open invariant

Equality routing is deterministic only while at most ONE open conversation
per (pipeline, `conversationId`) exists. Reopen is the one verb that could
break that — reopening a closed conversation while a `-N` successor with
the same id is open.

Reopen SHALL be refused while another open conversation carries the same
`conversationId` for that pipeline, with a reason naming the successor.

**Rejected:** newest-wins tiebreak — it silently splits one problem's
history across two conversations, which is the bug this change exists to
kill, in mirror image.

### Cooldown — one knob, re-keyed

Cooldown keys on `initiationId`, configured by `spec.cooldownHours`
promoted to the top of `SignalSource.spec` (default 6, chat keeps 0).
Inside the window a repeat is a re-send and drops. Outside it, a repeat is
a re-notification and routes by equality. Durability on the SignalSource
is unchanged.

**Rejected:** keeping `spec.grouping` with only `cooldownHours` inside — a
retired name wrapping one field.

### CRD field changes

| Field | Fate |
|---|---|
| `SignalSource.spec.grouping` (`signatureLabels`, `windowDays`, `cooldownHours`) | removed |
| `SignalSource.spec.cooldownHours` | added top-level, default 6, the one surviving knob |
| `Conversation.spec.signature` | replaced by `spec.conversationId`, written once, opaque |
| `agentops.dev/signature-hash` label | removed, no successor — lookup is a field index |

### Per-adapter id, where each one's aggregation already lives

| Adapter | `initiationId` |
|---|---|
| alertmanager | webhook `groupKey` verbatim — hash of sorted `groupLabels` when absent |
| pulse | the Pulse alert id, stable across retries and firing/resolved |
| ha | minted in the pending/dwell logic, keyed on condition identity |
| k8s-events | `<source>@<namespace>/<workload>`, owner-reference resolution kept |
| cron | the source's job identity — all ticks, one conversation |
| chat/console origination | fresh mint per request, or the stamped binding id propagated on continuation |

The k8s-events shape is FIXED — the old configurable object-level
granularity went with `signatureLabels`, stated in that delta's REMOVED
migration.

## Risks / Trade-offs

- [A 1.x third-party adapter breaks at upgrade] → that is the MAJOR bump
  working as intended. The `4xx` names the field, the CHANGELOG carries the
  upgrade step, every shipped adapter conforms in this same change.
- [Legacy open conversations duplicate once] → accepted, bounded, and
  visibly better than a wrong merge. They age out through normal close.
- [One knob couples dedup and recurrence] → a repeat inside `cooldownHours`
  drops even while its conversation is open. A source firing more often
  than 6h sizes its own `cooldownHours` — the bundles carry the values.
- [The open `channel-adapter-specification` change also deltas
  `channel-adapter-contract`] → whichever archives second rebases its
  delta. Tracked as a task.
- [CRD fields silently prune on upgrade] → `kubectl apply -f chart/crds/`
  in the upgrade note, per the standing gotcha.

## Migration Plan

1. Land the contract 2.0.0 text, manager routing, CRD regen and every
   shipped adapter in one change (the conformance rule demands it).
2. Upgrade order on an install: CRDs first (`kubectl apply -f chart/crds/`),
   then the release. Old conversations keep working — delivery without a
   `conversationId` is specified on the channel contract.
3. Rollback is the previous chart plus previous images. New-format
   conversations then carry an unused `conversationId` field, which the old
   manager ignores. Signals flow again under signature grouping.

## Open Questions

None. The reopen tiebreak, label mechanics, 1.x treatment and backfill —
each settled above.
