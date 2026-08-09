# Proposal: task-is-a-signal

## Why

`POST /task` is a second origination doorway that exists for no reason the first
one cannot serve. The architecture states that **conversations originate only
from claimed signal sources**; `/task` is the standing exception, and it is
already being renounced piecemeal — `rich-console-ui` specifies that the console
"SHALL NOT use `POST /task`" and originates a `chat` signal instead.

The duplication is not only conceptual. `httpapi.handleTask`
(`internal/httpapi/server.go:359-411`) and `chat.Router.CreateTaskConversation`
(`internal/chat/router.go:157-183`) are two implementations of *build a task
Conversation from a Pipeline*, and they have already drifted: different title
formats, and only one supports `channel`/`title`. A third builder,
`routeSignalGroup`, does the same job on the signal path — the one that also
applies cooldown, grouping, window reuse and source bookkeeping. `/task` skips
all of it.

The surface is also visibly unmaintained: `docs/contracts.md:148` still
documents `{"profile","task",...}`, a form the handler has rejected with 400
since Pipelines became the addressable unit.

Removing it collapses three origination paths to two (signal, chat command) and
three conversation builders to two.

## What Changes

- **BREAKING: `POST /task` is removed.** The route, `handleTask`, and `taskReq`
  are deleted. Programmatic origination becomes an ordinary signal:
  `POST /signal/inbound` naming a `SignalSource` that a Ready Pipeline claims.
- **New signal kind `task`.** Alongside `alert` and `job`, it takes the task
  lane (`InputTask`) with no `jobName` and no recurrence-on-session — precisely
  the semantics `/task` had, and precisely what `chat` already does minus the
  chat-surface requirement. It carries no `agentops.dev/channel` label, because
  replies go to the claiming Pipeline's `channelRefs`.
- **Per-fingerprint keying generalized off the chat lane.** Today
  `signals.go:136` keys on the fingerprint only for `kind: chat`. That rule
  becomes: when a source declares no `signatureLabels`, key on the fingerprint
  **unless the kind is `alert`**. Each `task`/`job`/`chat` call therefore gets
  its own conversation, while the alert lane keeps its
  `alertgroup/alertname/namespace` default — which `vm-bundle` depends on, since
  it ships `grouping: {}`.
- **The chart ships no manual-ask SignalSource.** `spec.adapter` is required and
  immutable, and a source no adapter serves would report `Served=False` forever
  on a healthy object. Callers post to a source that already exists and is
  already served.
- **Dropped without replacement**: the `agent` override and the `channel`
  add-a-surface field. Per-call agent selection remains available through the
  chat form `/<pipeline>:<agent>`; a caller choosing an extra channel was a
  caller choosing wiring, which the Pipeline model reserves to the Pipeline.
- **Docs follow the ownership table**: endpoint row and addressability prose in
  `docs/contracts.md` / `docs/concepts.md`, the curl in `README.md`, the "Ask an
  agent" line in `chart/templates/NOTES.txt`, comment references in
  `chart/values.yaml`, `chart/charts/k8s-bundle/templates/profile.yaml`,
  `config/samples/samples.yaml`, `api/v1alpha1/conversation_types.go`, and a
  migration entry in `CHANGELOG.md`.

Explicitly **out of scope**: unifying `Router.CreateTaskConversation` with
`routeSignalGroup`. The chat-command path short-circuits grouping deliberately
(a command is answered, not accumulated), and merging them is a separate
argument. Authentication is also out of scope — `/work` and `/work/done` are
unauthenticated by the same posture, and the adapter token is a scoping
mechanism, not a boundary.

## Capabilities

### New Capabilities

None. This change removes a surface and generalizes two existing rules; every
behavior it leaves behind is already owned by an existing spec.

### Modified Capabilities

- `signal-adapter-contract`: the `kind` enum gains `task`, and the endpoint's
  stated grouping behavior gains the fingerprint-keying rule.
- `signal-source-model`: the manager-side grouping requirement states when a
  source with no `signatureLabels` keys on the fingerprint instead of the
  default alert labels.
- `chat-signal-origination`: chat's "no signature grouping unless configured"
  stops being chat-specific — it is the general non-alert rule, and chat
  inherits it.
- `pipeline-model`: "A Pipeline is addressable by name" no longer means the task
  API. A Pipeline is reached by claiming a source; the scenario is restated in
  those terms.
- `mcp-toolset-model`: the scenario asserting `POST /task` carries a Pipeline's
  toolsets and mcpConfigs is restated against the signal path.
- `builtin-toolset-catalog`: the scenario in which an addressed Pipeline governs
  what a task may do drops its `POST /task` clause.
- `k8s-bundle`: the two demo scenarios describing the bundle's Pipeline as
  "askable via `POST /task`" are restated against the bundle's own served
  source.

## Impact

**Code** — `internal/httpapi/server.go` (route, handler, request type, package
doc), `internal/httpapi/signals.go` (kind constant, validation, keying rule,
input-lane switch), `internal/ingest/grouping.go` (no change expected; the
fallback stays the alert default and is simply reached less often).

**Tests** — `internal/integration/tooling_test.go:270-298` posts to `/task`
three times and must move to `/signal/inbound`. Dispatch/ingest fixtures pin
grouping semantics, so the keying change is a deliberate fixture edit, not an
incidental one.

**Charts and samples** — no rendered object changes. `k8s-bundle` and
`vm-bundle` keep their sources and grouping exactly as they are; only comments
and NOTES text move.

**Callers** — anything scripting `POST /task` breaks at upgrade. The migration
is a different URL, a bearer token, and a source name in place of a pipeline
name; `CHANGELOG.md` carries the before/after.

**Not affected** — the manager still reads no Secrets, still creates no RBAC,
and gains no new Kubernetes permissions. Adapter contracts are unchanged apart
from one added `kind` value.
