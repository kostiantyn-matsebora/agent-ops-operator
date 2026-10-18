# Signal correlation id

## Why

Two unrelated Pulse alerts (a node-memory threshold and an unreachable
device) merged into one conversation, `alert-master-3`. Routing is a
manager-computed signature — `grouping.signatureLabels` joined with `/`,
defaulting to `[alertgroup, alertname, namespace]`. That default is
Alertmanager vocabulary.

The pulse adapter emits `resource`/`resourceType`/`node`/`severity`, so every
pulse signal's signature is `//`. All pulse alerts hash identically, and any
pulse alert arriving while another pulse conversation is open is appended to
it as a recurrence.

No component fails. "Are these the same problem?" is a question nobody owns —
the sender already answered it (Alertmanager's `group_by`, Pulse's incident
model, an adapter's batching), and the manager answers it again, wrongly,
with another system's vocabulary.

## What Changes

- **BREAKING** — every normalized signal MUST carry `initiationId`, a
  correlation id minted where aggregation already happens (the sender or the
  adapter), never by the manager. A signal without one is rejected `4xx` at
  the webhook — a loud contract violation instead of a silently merged
  conversation.
- **BREAKING** — `initiationId` replaces `fingerprint`. Cooldown keys on
  `initiationId`, so dedup granularity matches the sender's own
  re-notification policy (Alertmanager `group_interval`, Pulse incident
  re-notify) instead of layering a second one.
- Manager routing becomes pure equality, zero policy. An open conversation
  with `conversationId == initiationId` for the claiming pipeline gets the
  signal appended as a recurrence. Otherwise a new conversation is created,
  adopting `initiationId` as its `conversationId`. Recurrence-after-close
  keeps today's behavior (new conversation, `-N` suffix).
- **BREAKING** — `SignalSource.spec.grouping` is removed from the CRD and
  from every bundle's values. `ingest.Signature`/`SignatureHash`/
  `DefaultSignatureLabels` and the alert-vs-oneshot fallback in
  `httpapi/signals.go` go with it.
- The manager stamps `conversationId` on everything leaving the conversation
  (member spawns, channel topic bindings, runtime context). Member returns
  and telegram/console replies arrive already correlated — the same id on
  the return trip, not a second addressing mode. The channel adapter's
  thread mapping becomes transport translation only (`conversationId` ↔
  topic id). Today's three ad-hoc mappings (signature-hash label,
  `status.threads` lookup, member origin backrefs) collapse into this one id.
- Each shipped adapter mints its id where its aggregation already lives:

  | Adapter | `initiationId` |
  |---|---|
  | alertmanager | the webhook `groupKey` (grouping configured upstream via `group_by`) |
  | pulse | the Pulse alert id (`k8s-node-…::metric-threshold:memory`, `…::provider-incident:<hash>`) |
  | ha / k8s-events | minted in their existing pending/batching logic — they already decide what constitutes one problem |
  | cron | the job id — all ticks, one conversation |
  | chat/task one-shots | a fresh unique id per request (own conversation), or a received `conversationId` propagated verbatim when the human continues a bound thread |

  One rule covers every row: a signal adapter supplies its own id — minted
  where its aggregation happens, or propagated verbatim when a round trip
  hands a manager-issued `conversationId` back. A channel adapter never
  mints any id. It receives the manager-issued `conversationId` and only
  translates it to and from the transport's thread id.

- `docs/contracts/signal-adapter.md` bumps MAJOR to 2.0.0 — a
  1.0.0-conformant adapter stops conforming.

**Invariant this change establishes:** "these N signals are one problem" is
decided wherever the id is minted — Alertmanager config, Pulse's incident
model, adapter batching. The manager never regains grouping policy.

## Capabilities

### New Capabilities

(none — correlation replaces grouping inside the existing capabilities)

### Modified Capabilities

- `signal-adapter-contract`: `initiationId` required on every signal and
  `fingerprint` removed, with a missing id rejected `4xx`. Cooldown keys on
  `initiationId`. The manager's grouping provision (`MGR-1`) becomes pure
  id-equality routing. MAJOR → 2.0.0.
- `signal-source-model`: `spec.grouping` removed — the source carries no
  grouping policy.
- `conversation-naming`: the conversation's identity is the adopted
  `initiationId` (as `conversationId`), replacing the signature and its
  hash label. Recurrence-after-close keeps the `-N` suffix.
- `conversation-provenance`: provenance records `initiationId`, and the
  `agentops.dev/signature-hash` label is replaced by the `conversationId`.
- `chat-signal-origination`: a new chat/task request mints a fresh unique
  `initiationId` (its own conversation by construction), and a continuation
  in a bound thread propagates the received `conversationId` — both
  instances of the one adapter id rule, not a special lane.
- `alertmanager-signal-adapter`: mints `initiationId` from the webhook
  `groupKey`.
- `pulse-signal-adapter`: mints `initiationId` from the Pulse alert id.
- `ha-signal-adapter`: mints `initiationId` in its existing pending logic.
- `k8s-events-signal-adapter`: mints `initiationId` in its existing
  workload-grouping/batching logic.
- `cron-signal-adapter`: `initiationId` is the job id — all ticks correlate
  to one conversation without relying on the empty-signature collapse.
- `k8s-bundle`: the one bundle whose spec normatively required grouping
  values — the rendered source drops them, one-conversation-per-workload
  now comes from the adapter's id.
- `channel-adapter-contract`: every `/channel/ops` operation carries the
  `conversationId` (additive, wire `contract` unchanged, document MINOR),
  and a channel adapter never mints a conversation id — thread mapping is
  transport translation only (`conversationId` ↔ thread id).
- `console-origination`: the console's human-origination path gets the same
  two cases as chat — fresh mint, or verbatim propagation of the stamped
  binding id.
- `conversation-capacity`: the Pending lookup keys on `conversationId`
  instead of the signature label.
- `conversation-housekeeping`: a closed conversation keeps its
  `conversationId` in the record but is excluded from equality routing.
- `state-durability`: suppression windows that survive a manager restart
  key on `initiationId`.
- `pipeline-model`: cooldown stays evaluated once above the fan-out, and
  equality routing is scoped per pipeline — legacy conversations are never
  matched or backfilled.
- `contract-conformance-suite`: the signal adapter conformance set keys on
  the correlation id (mint + determinism items).
- `telegram-ingest-router`: a replayed update batch converges because a
  replayed update yields the same `initiationId`.
- `conversation-opens-with-its-input`: rationale re-grounded on "same
  `initiationId`" instead of "same signature".

Dropped grouping values in `prometheus-bundle`, `pulse-bundle` and
`ha-bundle` are implementation only — their published specs never name
grouping at requirement level. `telegram-channel-adapter` is bound by the
channel contract's new rules and carries no mapping requirement of its own.

## Impact

### Code consequences (named here, implemented by the change)

| Where | Consequence |
|---|---|
| `platform/manager/internal/ingest/grouping.go` | `Signature`/`SignatureHash`/`DefaultSignatureLabels` removed, `Cooldown` re-keyed to `initiationId` |
| `platform/manager/internal/httpapi/signals.go` | the alert-vs-oneshot fallback (~line 231) removed, routing by `conversationId` equality, `4xx` on a missing `initiationId` |
| `platform/manager/api/v1alpha1/` + `chart/crds/` | `SignalSource.spec.grouping` dropped (`cooldownHours` promoted top-level), `Conversation.spec.signature` replaced by `conversationId`, the `agentops.dev/signature-hash` label removed (lookup becomes a field index), CRD regen |
| `internal/chat/`, `internal/controller/` | thread binding and member-origin backrefs keyed by `conversationId` |
| `signals/*` (alertmanager, ha, k8s-events, cron, telegram) and the pulse adapter | each mints `initiationId` as the table above states |
| `channels/telegram`, `platform/console` | thread mapping as transport translation only |
| bundle charts under `chart/charts/` | `grouping` values removed |
| dispatch/ingest test fixtures | pin these semantics — they change deliberately with this proposal |

### Migration decision (settled in design.md)

- Backfill `conversationId` on open conversations from `spec.signal`
  provenance, or drain open conversations and apply to new ones only.
- Wire compatibility: how the manager treats a 1.x adapter during rollout
  (reject vs synthesize-and-warn), per the contract's COMPAT section.

### Reference docs made untrue

- `docs/contracts/signal-adapter.md` — MAJOR 2.0.0, requirement IDs
  (`API-n`/`ADP-n`/`MGR-n`) and the changelog table. Edited only with the
  user's explicit approval, per `signal-rules.md`.
- `docs/contracts/channel-adapter.md` — thread-mapping and ops-queue impact.
- `docs/contracts.md` — `/signal/inbound` request shape and rejection.
- `docs/concepts.md` — grouping/signature vocabulary replaced by the
  correlation id.
- `docs/cr-reference.md` and every generated block — re-run
  `docs-generate.py` after the CRD change.
- `docs/guides/signal-adapter.md` — authoring guidance mints an id, no
  longer configures `signatureLabels`.
- `docs/integrations/prometheus.md`, `pulse.md`, `home-assistant.md`,
  `kubernetes.md` — where each integration's id comes from.
- `docs/CHANGELOG.md` — breaking change plus upgrade steps, newest first.

### Adopter site made untrue

- `docs/index.md` (landing) — names grouping vocabulary.
- `docs/configuration.md` — bundle grouping values removed.
- `docs/installation.md` — upgrade note for the CRD change
  (`kubectl apply -f chart/crds/`, or fields silently prune).
- `docs/getting-started.md` / `docs/introduction.md` — only if either names
  grouping. Verify and record the verdict.

### Retired vocabulary

`signature`, `signatureLabels`, `signature-hash` and `fingerprint` (signal
sense) enter `retired-vocabulary.md` and `.github/retired-vocabulary.json`
in the same change.
