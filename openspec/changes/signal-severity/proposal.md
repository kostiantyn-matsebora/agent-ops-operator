## Why

`signal-attributes-and-icons` (#253, still pending) bundles three unrelated
capabilities into one change: severity, routing matchers and icons.

That bundling makes the smallest useful piece wait on the largest. Knowing
how urgent a signal was rated is small, next to a CRD field on nine kinds,
a chart-wide icon obligation with its render test, and a label-selector
matcher on both Pipeline ref lists.

Severity stands on its own. An adopter who wants their k8s-events and Home
Assistant signals rated, and that rating visible in the console transcript,
should not also need matchers or icons to land first.

This change extracts severity into its own change: the field, its
enforcement at ingest, each shipped adapter's mapping, and the console
transcript showing it.

Matchers and icons stay in `signal-attributes-and-icons`, which is updated
in the same commit to drop what this change now owns.

## What Changes

- **Severity is a closed vocabulary on the normalized signal**, distinct
  from its labels: `critical`, `error`, `warning`, `info`. Absent means
  unrated. `POST /signal/inbound` refuses any other value with a 400 naming
  the vocabulary, creating nothing from that batch.
- **Severity rides on the conversation's signal provenance and on each
  input.** The provenance records how the conversation was opened. A later
  recurrence rated differently is recorded on its own input, never
  rewriting the provenance.
- **Each shipped adapter maps its own scale once, as a field instead of a
  label:**
  - `signals/k8s-events` maps the Event type, `Warning` to `warning` and
    `Normal` to `info`, and stops copying it into the payload.
  - `signals/ha` maps the log record's level — `CRITICAL`, `ERROR`,
    `WARNING` to their namesakes, `INFO`/`DEBUG` to `info`.
  - `signals/alertmanager` maps the alert's `severity` label through a new
    per-source `config.severityMap`, identity by default for `critical`,
    `warning` and `info`, and reports an unmappable value on the source's
    Ready condition while sending the alert unrated.
  - `signals/cron` sends none — a tick has no scale to map.
- **The console's transcript shows it.** The `signal` outbound message
  gains a `severity` field, and the console's signal card leads with it,
  tinted from the theme's existing severity tokens. A signal with no
  severity renders exactly as it does today.
- **Not breaking.** A manifest or an adapter written before this change
  behaves as it did: absent severity is unrated, which is today's only
  state.

**Deliberately out of scope**, staying in `signal-attributes-and-icons`:
routing matchers on Pipeline ref lists, `spec.icon` on any kind, the
reserved `type` label, and Telegram's rendering. That change is updated to
remove the severity work this one now does, so neither change specifies the
same field twice.

## Capabilities

### New Capabilities

- `signal-severity`: the severity vocabulary, its enforcement at the one
  inbound door, where it is stored on the conversation and its inputs, and
  the fixed mapping table each shipped adapter applies.

### Modified Capabilities

- `signal-adapter-contract`: the inbound signal shape gains `severity`, and
  the endpoint refuses a value outside the vocabulary.
- `k8s-events-signal-adapter`: severity is sent as the field, mapped from
  the Event type, and dropped from the payload.
- `ha-signal-adapter`: the log record's level maps to the severity field.
- `alertmanager-signal-adapter`: the alert's `severity` label maps through
  the source's new `severityMap`, with the unmappable case reported on
  Ready.
- `adapter-rendered-messages`: the `signal` message carries `severity`.
- `console-adapter`: the transcript's signal card draws it.

## Impact

**Manager (`platform/manager/`).**

| Area | Change |
|---|---|
| `api/v1alpha1/` | `Severity` enum type, field on `SignalProvenance` and on `ConversationInputSpec`, regenerated deepcopy and CRDs |
| `internal/httpapi/signals.go` | `Severity` on `NormalizedSignal`, vocabulary validation ahead of any routing |
| `internal/chat/message.go` | `Severity` on the `signal` message, filled from the provenance/input |

**Adapters.** `signals/k8s-events`, `signals/ha`, `signals/alertmanager` map
their own scale onto the field and stop writing it into the payload where
they did. `signals/cron` is untouched.

**Console.** `platform/console/transcript.go` carries `Severity` through to
the frontend `Message`, and `platform/console/ui/src/pages/Conversation.tsx`
draws it on the signal card.

**`signal-attributes-and-icons` (#253).** Its proposal, design, tasks and
delta specs are trimmed to drop every severity-specific line — the
`signal-severity-and-type` capability there is narrowed to type alone — so
the two changes do not both own the same requirement.

**Reference docs made untrue and updated:**

| Document | Because |
|---|---|
| `docs/concepts.md` | severity moves from a per-adapter label to a validated field, with its vocabulary |
| `docs/contracts.md` | the inbound signal shape and the `signal` message shape both gain `severity` |
| `docs/cr-reference.md` and its generated blocks | `Severity` on two CRD-backed types |
| `docs/integrations/kubernetes.md`, `home-assistant.md`, `prometheus.md` | each adapter's mapping table, and the prometheus source's `severityMap` |
| `docs/console.md` | what the signal card now draws |
| `docs/CHANGELOG.md` | the new field, no breaking change |
| `docs/guides/signal-adapter.md` | severity in the worked emission |

**Adopter site made untrue and updated:**

| Page | Because |
|---|---|
| `docs/console-guide.md` | severity marks in the transcript view |
| `docs/introduction.md` | one sentence that a signal can be rated |
