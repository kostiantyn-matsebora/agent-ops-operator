## Why

A signal carries no sense of how urgent it is beyond whatever each adapter
stuffs into its own labels. `signals/k8s-events` and `signals/ha` each write
an ad hoc `severity` label from their own scale.

Routing or display logic has no single field to read urgency from, and no
closed vocabulary to depend on.

This change gives the signal contract one optional, validated `severity`
field, nothing more. It does not ask any shipped adapter to populate it.

The vocabulary borrows from the two conventions already in play —
Alertmanager's `critical`/`warning`/`info`, Home Assistant's
four-and-a-debug scale — ready for a later change to wire a sender into.

## What Changes

- **A closed, optional severity vocabulary on the normalized signal**:
  `critical`, `error`, `warning`, `info`. Absent means unrated — today's
  only state, unchanged.
- **`POST /signal/inbound` validates it when present.** An unknown value is
  refused with a 400 naming the four accepted values. A batch with no
  `severity` field at all behaves exactly as it does today.
- **Severity rides with the conversation's signal provenance and with each
  input**, the same place and lifecycle as `labels` today — written once at
  creation, recorded per-input on a later recurrence, never rewriting the
  provenance.
- **The outbound `signal` message carries it through** as a plain optional
  field, next to `labels`, for any adapter that can draw it.
- **The console transcript's signal card draws it where present**, tinted
  from the theme's existing status tokens (`critical`/`error` →
  `--ao-danger`, `warning` → `--ao-warning`, `info` → `--ao-neutral`). No new
  token is added. A card whose message carries no severity renders exactly
  as it does today.
- **Explicitly not changed**: `signals/k8s-events`, `signals/ha`, and
  `signals/alertmanager` keep writing their own `severity` **label** exactly
  as they do today. None of the three populates the new field. The field
  ships with no shipped sender — proven only by a hand-sent
  `curl POST /signal/inbound` carrying `severity: critical`, not by adapter
  code. Mapping a real adapter onto the field is deliberately left to a
  later change.
- **Not breaking.** An adapter or a manifest written before this change
  behaves identically — absent severity is, and remains, the only state it
  ever produced.

## Capabilities

### New Capabilities

- `signal-severity`: the severity vocabulary itself, where it is validated,
  and where it is stored on the conversation's provenance and on each input.

### Modified Capabilities

- `signal-adapter-contract`: the inbound signal shape gains an optional
  `severity` field, and the endpoint refuses a value outside the vocabulary.
- `adapter-rendered-messages`: the `signal` outbound message gains an
  optional `severity` field.
- `console-adapter`: the transcript's signal card draws `severity` where the
  message carries it.

## Impact

**Manager (`platform/manager/`).**

| Area | Change |
|---|---|
| `api/v1alpha1/` | `Severity` enum type, optional field on `SignalProvenance` and on `ConversationInputSpec`, regenerated deepcopy and CRDs |
| `internal/httpapi/signals.go` | optional `Severity` on `NormalizedSignal`, vocabulary validation ahead of any routing |
| `internal/chat/message.go` | optional `Severity` on the `signal` message, filled from the provenance/input when present |

**Console.** `platform/console/transcript.go` carries `Severity` through to
the frontend `Message`. `platform/console/ui/src/pages/Conversation.tsx`
draws it on the signal card when present.

**Not touched.** `signals/k8s-events`, `signals/ha`, `signals/alertmanager`,
`signals/cron`, `signals/pulse`, `signals/telegram`, `channels/telegram` —
none of these adapters changes behavior.

**Reference docs made untrue and updated:**

| Document | Because |
|---|---|
| `docs/concepts.md` | the normalized signal shape gains an optional `severity` field and its vocabulary |
| `docs/contracts.md` | the inbound signal shape and the `signal` message shape both gain `severity` |
| `docs/cr-reference.md` and its generated blocks | `Severity` on two CRD-backed types |
| `docs/console.md` | what the signal card now draws when present |
| `docs/CHANGELOG.md` | the new optional field, no breaking change |
| `docs/guides/signal-adapter.md` | a note that severity is a contract field no shipped adapter sets yet |

**Adopter site made untrue and updated:**

| Page | Because |
|---|---|
| `docs/console-guide.md` | a severity mark can appear in the transcript view |
