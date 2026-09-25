## Why

Nothing routes on how urgent a signal is, and nothing draws what kind of
object it reached. A signal carries a lane (`kind`) and an open label map,
and nothing else a route or a person can rely on:

- Severity is a label key each adapter fills from its own scale: `Warning`
  from Kubernetes, `ERROR` from Home Assistant, whatever the Prometheus rule
  says.
- The agent never sees a label at all.
- Every conversation opens threads on every channel its Pipeline names.

So an adopter cannot page one channel on critical alerts and keep warnings in
the console, and an agent investigating an alert does not know how urgent it
was rated.

The console draws nine built-in icons and one kind carries `spec.icon`, so
the topology and the cards read as text where a glance should do.

## What Changes

- **Severity is a closed vocabulary on the signal.** `severity` becomes a
  field of the normalized signal beside `kind`, with four values:
  `critical`, `error`, `warning`, `info`. Absent means unrated. The inbound
  endpoint refuses any other value with a 400 naming the vocabulary, so
  every adapter maps its own scale once and nothing else passes.
  - k8s-events maps `Warning` to `warning` and `Normal` to `info`.
  - ha maps `CRITICAL` to `critical`, `ERROR` to `error`, `WARNING` to
    `warning`, the rest to `info`.
  - alertmanager maps the rule's `severity` label through a per-source
    `config.severityMap`, identity by default for `critical`, `warning` and
    `info`, and reports an unmappable value on the source's Ready condition
    while sending the alert unrated.
  - cron sends none.
  - The severity is kept on the conversation's signal provenance and on each
    input, and the signal card carries it.
- **`type` is a reserved label key.** Open values, adopter-defined, passed
  through by adapters that forward labels, and guarded in k8s-events so a pod
  label named `type` never overwrites it.
- **Routing matchers on both Pipeline ref lists.** `signalSourceRefs[]` and
  `channelRefs[]` gain an optional `match`, a list of Kubernetes
  `LabelSelectorRequirement` with the operator restricted to `In` and `NotIn`.
  Expressions evaluate over one map: the signal's labels with `severity` and
  `kind` projected in as keys.
  - On a source claim, a non-matching signal does not open a conversation on
    that Pipeline. Two Pipelines whose matchers both fit still both open one,
    since sources are shareable and there is no tiebreak.
  - On a channel binding, a conversation snapshots only the channels whose
    matcher fits its originating signal. Delivery code is untouched.
  - Absent matches everything. **Not breaking.**
- **The agent is told the metadata.** The investigate and task templates gain
  a block above the payload naming the kind, the severity and the labels, so
  adapters stop copying severity into payloads by hand.
- **An icon on every adopter-authored kind.** `spec.icon` generalises from
  Pipeline to SignalSource, Channel, AgentProfile, AgentRuntime,
  SignalAdapter, ChannelAdapter, MCPToolset and MCPConfig, with one shared
  type and one doc comment. Conversation and ConversationInput get none, since
  they are materialised state and inherit their Pipeline's.
  - Resolution is a ladder: the instance's icon, then the kind's built-in
    `aops:` icon, then nothing. The built-in set gains one entry per kind.
  - Severity values get built-in icons. Type values get icons from chart
    values (`global.agentops.signalTypes[]`), published by the manager onto
    the signal card.
  - Every CR the chart ships declares an `aops:` icon, asserted by a chart
    render test. Adopter CRs may omit it and get the kind fallback.
  - The console's topology, cards and lists and Telegram's menu and cards
    draw them.

## Capabilities

### New Capabilities

- `signal-severity-and-type`: the severity vocabulary, its enforcement at
  ingest, per-adapter mapping, the reserved `type` label, where both are
  stored, and what the agent's prompt carries.
- `signal-routing-matchers`: `match` on source claims and channel bindings,
  the operators, the map they evaluate over, and how absent and empty read.
- `resource-icons`: `spec.icon` on every adopter-authored kind, the four
  forms, the fallback ladder, the built-in per-kind and per-severity icons,
  chart-declared type icons, and the chart's obligation to declare one on
  every CR it ships. The Pipeline icon had no spec of its own until now.

### Modified Capabilities

- `signal-adapter-contract`: the inbound signal shape gains `severity`, and
  the endpoint refuses a value outside the vocabulary.
- `pipeline-model`: the wiring requirement's ref lists carry an optional
  matcher, and the fan-out requirement states that matching decides which
  Pipelines open and which channels bind.
- `adapter-rendered-messages`: the `signal` message carries `severity`,
  `type` and resolved icon references for the source, the pipeline, the
  severity and the type.
- `k8s-events-signal-adapter`: severity is sent as the field, mapped from the
  Event type, and `type` joins the reserved label keys.
- `ha-signal-adapter`: the log level maps to the severity field.
- `alertmanager-signal-adapter`: the rule's severity label maps through the
  source's `severityMap`, with the unmappable case reported.
- `console-adapter`: the transcript's signal card draws severity, type and
  icons.
- `console-topology`: graph nodes and inventory rows draw the resolved icon.
- `telegram-channel-adapter`: the signal card leads with the severity emoji
  and the menu uses the icon ladder.

## Impact

**Manager (`platform/manager/`).**

| Area | Change |
|---|---|
| `api/v1alpha1/` | `Icon` on nine kinds (eight new, Pipeline moved onto the shared type), `match` on both Pipeline ref lists with CEL on the operator, `severity` on `SignalProvenance` and `ConversationInputSpec`, regenerated deepcopy and CRDs |
| `internal/httpapi/signals.go` | `severity` on the normalized signal, vocabulary validation, matcher evaluation over the claiming Pipelines and the channel snapshot |
| `internal/chat/pipelines.go`, `message.go` | matching helper, the signal message's new fields, icon resolution |
| `internal/dispatch/` | the metadata block in both templates |
| `cmd/manager/main.go` | reads the type icon table from the chart |

**Adapters.** `signals/k8s-events`, `signals/ha`, `signals/alertmanager` map
severity. `channels/telegram` renders it. `platform/console` draws icons and
severity in the transcript, topology and inventory, and its built-in icon set
grows.

**Chart.** Every rendered agentops CR declares an icon. `global.agentops.signalTypes`
is new. A render test asserts the declaration.

**Reference docs made untrue and updated:**

| Document | Because |
|---|---|
| `docs/concepts.md` | the signal label vocabulary table gains `type`, severity moves from a label to a field, the Pipeline section gains matchers, the icon rule covers every kind |
| `docs/contracts.md` | the inbound signal shape and the `signal` message shape |
| `docs/cr-reference.md` and every generated block | new fields on nine CRDs, regenerated |
| `docs/configuration.md` | `global.agentops.signalTypes` |
| `docs/integrations/kubernetes.md`, `home-assistant.md`, `prometheus.md` | each adapter's severity mapping, the prometheus source's `severityMap` |
| `docs/console.md` | what the card and topology draw |
| `docs/CHANGELOG.md` | the new fields, no breaking change |
| `docs/guides/pipeline.md`, `signal-adapter.md` | matchers in the worked Pipeline, severity in an adapter's emission |

**Adopter site made untrue and updated:**

| Page | Because |
|---|---|
| `docs/index.md` | the "What you write" Pipeline shows a matcher, icons in the console strip |
| `docs/introduction.md` | routing by severity is a thing the model does now |
| `docs/getting-started.md` | the first Pipeline mentions where the icon comes from |
| `docs/console-guide.md` | severity and icons in the views |
| `README.md` | the annotated Pipeline gains a `match` line, within budget |
