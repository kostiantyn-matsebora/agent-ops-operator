# Design: rich-console-ui

## Context

`2026-08-09-visualize-agent-ops` shipped a console that answers *what is
configured* and *what did it do*, from CR state alone, in a hand-written SPA. The
requirements driving this change ask for something different in kind: a
Kiali-grade console where the wiring graph and the conversation graph are both
**live** — components lit by messages actually moving between them — and where an
operator can **originate** work, not only observe and reply.

Four of the six requirements are frontend work. Two are not:

- **Live traffic has no data source.** Kiali animates from Envoy telemetry in
  Prometheus. agent-ops records nothing per-hop. CR status says a conversation is
  `inflight`; it never says the manager handed run `r-7` to a runtime and got a
  result 4s later.
- **Origination is a wiring question, not a UI button.** A conversation may only
  start from a claimed `SignalSource`. Anything else is a side door.

Constraints that bind this design:

- Conversations originate ONLY from claimed signal sources; a channel carries,
  never starts. `/channel/inbound` is reply-only.
- agent-ops' own health is STATUS, not SIGNAL — no observing component may emit
  a signal about agent-ops machinery (the no-signal-loops invariant).
- The manager reads no Secrets and creates no RBAC for adapters.
- One adapter per implementation; the CR name is the routing key.
- A `SignalSource` is claimed by exactly ONE Pipeline; a `Channel` is shareable.
- Strictly serial per conversation; channel ops are at-least-once.

## Goals / Non-Goals

**Goals**

- Replace `console/` wholesale — module, API, and UI.
- A live wiring graph and a live per-conversation graph, both driven by real
  recorded hops rather than inference.
- Conversation origination from the console, through the sanctioned lane.
- An installation overview that answers "is this install healthy" on one page.
- Ship as its own pod, enabled by default, disabled by one value.

**Non-Goals**

- Not a general Kubernetes dashboard: `agentops.dev` CRs, plus the Deployments
  and pods that constitute the install itself.
- No CR editing. Pipelines are the wiring and the wiring is GitOps-managed; a
  console that edits them competes with helmfile.
- No time-series *storage*. The activity log is a bounded in-memory ring; the
  console persists nothing. It MAY read an existing metrics backend when one is
  configured (optional, degrades cleanly to buffer-length windows), but it never
  becomes one.
- No multi-cluster, no multi-namespace.
- Not a tracing system — the event shape is span-like on purpose, but exporting
  to OTel is a later, additive change.

## Decisions

### 1. Activity events become a first-class manager contract

The manager mediates every hop and records none. Add an append-only, bounded,
in-memory activity log with `GET /activity` (replay by cursor),
`GET /activity/stream` (SSE), and `POST /activity` (adapter-reported hops,
adapter-token authenticated). Events name graph nodes in `from`/`to`, so an
event renders directly as motion along an edge the topology already draws.

*Alternative — derive motion from CR watch deltas.* No new contract, but the
console would animate `pipeline → channel` because a conversation went
`inflight`, not because anything was sent; and since a console sees only its own
adapter's ops, every Telegram edge would be fabricated. Rejected: a graph that
can lie is worse than no graph, and an operations console's whole value is that
it cannot disagree with the cluster.

*Alternative — OpenTelemetry spans to Tempo/Jaeger with a traces tab.* The right
long-term answer for cross-process latency forensics, and Kiali does exactly
this. Rejected for now: it makes a tracing backend a hard dependency of watching
your own graph move, for a system whose scope is one namespace and one manager
process. `runId` is a trace id in all but name, so an exporter is additive.

**Telemetry is not signal.** These events go to a stream, never to
`/signal/inbound`. The no-signal-loops invariant is the reason, and it holds by
construction here: emitting an activity event is not itself a hop, so there is
nothing to recurse on.

### 2. The console originates through a SignalSource, not `POST /task`

The console declares a `SignalSource` served by its own `SignalAdapter`
identity. "Start a conversation" posts a `kind: chat` signal to
`/signal/inbound` carrying `agentops.dev/channel` and `agentops.dev/sender`; the
Pipeline claiming that source answers. This is the lane a Telegram general-surface
message already travels.

Four consequences, each of which `/task` cannot supply:

1. The origination invariant holds literally, with no side door to defend.
2. **Who answers is declared, not chosen by the caller.** `/task` takes a
   pipeline name in the request body — the caller picks the agent. A claimed
   source means the wiring decides, which is the rule's actual intent.
3. **Origination is visible traffic.** A `/task` conversation materializes from
   nowhere, leaving a hole in the graph exactly where the operator acted. A
   console source is a node with an edge: pressing "start" lights up the graph
   the console is rendering.
4. **Self-started conversations join themselves.** The router appends the
   originating channel, so a console-started conversation already has a console
   thread — no pipeline edit. Requirements 4 and 6 become one path.

The failure mode is diagnosable with existing machinery: an unclaimed console
source sits at `Wired=False`, which reads exactly as "this console cannot start
conversations yet."

*Consequence accepted:* one source is claimed by one Pipeline, so one source
means one destination. Originating to several agents means declaring several
sources (`console-k8s`, `console-ha`). The "New conversation" picker lists the
console sources that are `Wired=True`, labeled by claiming pipeline — so what
you can start is what is wired, rather than a free-text field that can name
something no wiring supports.

### 3. Externally-served SignalAdapter — two identities, one pod

The console needs `ChannelAdapter/console` (carry, chat) and
`SignalAdapter/console` (originate). Both reconcilers own workloads and
`SignalAdapter.spec.image` is required, so declaring both yields **two
Deployments**, one of which exists only to make a source `Served`. That is the
shape this repo already paid for: telegram-router "used to be an adapter with a
signal-free SignalSource purely to carry that credential — which then sat at
`Wired=False`."

So: `spec.image` becomes optional, and `spec.servedBy: {kind, name}` is added.
When set, the SignalAdapter reconciler creates no Deployment, Service or
ServiceAccount and reports `Ready=True/ServedBy`; the ChannelAdapter reconciler
injects `SIGNAL_ADAPTER_TOKEN` into the named adapter's pod, derived exactly as
today (`HMAC(master, "signal-adapter:"+name)`). Derivation contexts are already
separated in `internal/chat/token.go`, so the identities stay distinct and each
surface still validates only against its own CRD list.

The difference from the telegram-router mistake is the one that matters: that
source was signal-free and unclaimable. This one originates real conversations
for a Pipeline that claims it.

*This generalizes.* A chat transport is inherently surface **and** originator;
Telegram needs three pods today precisely because one adapter cannot be both.
`servedBy` is the prerequisite for ever collapsing that — evidence the shape is
right rather than a console-specific hook.

### 4. React + PatternFly + PatternFly React Topology — frontend only

**The backend stays Go.** This decision is about the browser half and nothing
else. The console's service remains a Go module talking to the manager's HTTP
API (five surfaces — see Decision 6) and doing raw Kubernetes list/watch,
shipping as one distroless binary with the built frontend embedded via
`go:embed`. The deployable artifact is unchanged in kind: one Go image, one
port.

The requirement is explicitly Kiali-like, and this is Kiali's stack. PatternFly
supplies the shell, tables, drawers and empty/error states; PF React Topology
supplies nodes, edges, layouts, grouping and rate-driven edge animation — the
Kiali traffic idiom itself, rather than something to rebuild in raw SVG.

This **reverses Decision 6 of `visualize-agent-ops`** ("no build toolchain,
hand-rolled SVG"), deliberately. That decision was correct for a graph of tens
of nodes with no animation; it is not correct for two live graph views, a
waterfall, and a filterable table of thousands of conversations. npm stays
inside `console/` and its image: no other module and not the manager gains a
build step, so the dependency-free rule that governs adapter modules is
narrowed, not broken.

*Alternative — Angular.* Capable, but PatternFly is React-first and Kiali is
React. Choosing Angular means rebuilding the design system that supplies the
requested look and reimplementing the topology component that supplies the
requested behavior. The Kiali-parity requirement decides this.

*Alternative — Cytoscape.js for the graph.* Better exotic layouts; a second
visual language beside PatternFly and no PF theming. Kept as the fallback if PF
Topology cannot express the conversation graph.

### 5. Both graphs carry every involved CRD, with class-level display toggles

Nodes are not limited to the wiring spine: MCPToolsets, MCPConfigs, AgentRuntimes
and adapters are on the graph too, because "what can this agent actually reach"
is a question about exactly those objects. A Kiali-style Display panel toggles
each class independently, along with traffic animation, idle elements, and edge
labels (none, rate, latency), with selections persisted.

Two rules keep this from becoming a way to be misled:

- **Hiding is presentation only.** Health summaries and the overview problem
  rollup count hidden elements, and the panel says when hidden classes contain
  failures. A filter that can silently conceal a broken component is worse than
  no filter.
- **The conversation graph is built from the Conversation's own recorded
  bindings**, never from the Pipeline's current spec. Conversations snapshot what
  they materialized, so after a re-wire the graph still shows the capabilities
  that run actually had, and reports that current wiring differs. Reading the
  live Pipeline instead would silently rewrite history — and the forensic value
  of a per-conversation graph is precisely that it does not.

### 6. The manager exposes only what only the manager knows

`visualize-agent-ops` rejected a manager-side `/console/state` API: the manager
would become a CR-snapshot proxy, duplicating what the API server already serves
under proper RBAC. That holds — and it draws the boundary rather than closing
the question. CR state comes from the API server, always. Manager-internal state
comes from the manager, because nothing else has it.

Two classes fall on the manager's side and are unreachable today:

- **Runtime state (`GET /status`).** `OpQueue` is in-memory by design: pending
  and claimed ops exist in no object. Neither do runtime slots against
  `MAX_RUNTIMES`, cooldowns, or lease holder. So "is the op queue backed up",
  "is an adapter claiming ops without completing them", "are we at the ceiling
  so conversations queue rather than run" are unanswerable — and they are
  ordinary operational questions.
- **Resolved capabilities (`GET /pipelines/{name}/resolved`).** Composition
  modes, declared tools, MCP configs and runtime allowlists resolve into what an
  agent may actually do. The console MUST NOT recompute this: a second
  implementation would eventually disagree with the one that runs, and the
  console's entire claim is that it cannot disagree with the system.

*Alternative for the first — scrape `:9090/metrics`.* Partly right, and the
counters land there anyway (Decision 6b) so alerting on a stuck queue does not
require the console to be open. But a metric label is the wrong place for
identifying detail (which op, which conversation, which adapter is stalling), so
the split is: aggregates to metrics, identities to `/status`.

### 6b. Standard Prometheus metrics, emitted once, consumed three ways

The same aggregates go out in the Prometheus exposition format on the manager's
existing `:9090`, registered into the controller-runtime registry already
serving it — standard conventions (`agentops_` prefix, `_total` counters, base
units, HELP/TYPE, OpenMetrics), not house style.

**One emission point.** Metrics are incremented at the same call sites that emit
activity events. Two instrumentation passes over the same hops would drift, and
a console whose stream disagrees with its own charts is worse than either alone.

**Cardinality is a hard rule**: labels carry only CR-bounded values (`pipeline`,
`adapter`, `source`, `channel`, `kind`, `status`, `reason`). Conversation, run
and op ids would grow series without bound — they identify the specific stuck
item and live in `/status`. Metrics answer *how deep, how old, how many*;
`/status` answers *which one*.

This buys three consumers from one pass: the console live; an existing
Prometheus/Grafana/Alertmanager stack with no console involved; and — optionally
— the console reading its own series *back* from that stack to render windows
far longer than the ring buffer, which is exactly Kiali's model (live detail
from one source, rates and history from Prometheus).

*Consequence worth stating:* this is the escape from the ring buffer's ~15
minute horizon without the console storing anything. Where a metrics backend is
configured the console offers historical aggregates; where none is, it offers
buffer-length windows and says so. It never requires a monitoring stack, and
never becomes one.

*Alternative for the second — resolve in the console.* Rejected above; it is the
one place the console would start asserting something the cluster did not say.

### 7. BFF owns fan-in; snapshots authoritative, stream carries cursors

One Go service fans in four sources the browser must never touch: CR list/watch,
the activity stream, install facts, and the manager's channel/signal contracts.
Per-page snapshot endpoints plus one multiplexed SSE stream. Every stream event
carries a monotonic cursor; the browser re-fetches what it is showing. A missed
event costs one stale second, not a wrong screen, and reconnect-after-sleep is
the same code path as first connect.

### 8. Enabled by default, in its own pod

`console.enabled` defaults to `true`. The console is its own Deployment, pod,
ServiceAccount, image and release cadence — a UI OOM from a large graph must not
touch dispatch. `replicas: 1` + `Recreate`: it holds the channel op loop and
every browser's SSE connection, so a second replica splits both.

Because the component is on by default, its features are on by default:
`console.write.enabled` defaults to `true`. A default-enabled component whose
headline capabilities are default-disabled is a confusing way to ship nothing.
The token is the boundary, `ClusterIP` and no Ingress are the defaults, and OIDC
via forward-auth is the documented answer for exposure. This is not a new
exposure class — the Telegram bundle's `approvers` is optional, so anyone in the
group can already instruct the agent.

### 9. Install facts require a pod/deployment read grant

Versions, image digests, restart counts and manager health exist in no CR. The
console SA gains namespaced read-only `get/list/watch` on `deployments` and
`pods` — a deliberate widening past `agentops.dev`, which the previous design
avoided.

*Alternative — a manager `GET /version` plus adapter self-report.* Avoids the
grant, covers less: no digests, no restart counts, no pod-level failure reasons.
Taken the grant instead: an operations console that cannot see a
CrashLoopBackOff is not one.

## Risks / Trade-offs

- [An npm build lands in a repo whose modules are deliberately dependency-free]
  → confined to `console/` and its image; multi-stage Dockerfile, `dev` build tag
  serving from disk, `make ui` for a plain `go build`. No other module changes.
- [Ring buffer memory under an event storm] → fixed-size ring, oldest evicted,
  emitters never block; size is a value, and §Open Questions flags that 10k is
  an unmeasured guess on a cluster that has produced ~2000 conversations in a
  bad hour.
- [Two identities in one pod widen its blast radius — it can both read every CR
  and originate conversations] → the write path is `console.write.enabled` and an
  authenticated identity; the token boundary is documented; the SignalSource must
  still be claimed by a Pipeline before anything can be started at all.
- [Default-enabled starts a pod that reads every CR in the namespace on upgrade]
  → chart major bump, CHANGELOG entry with the opt-out spelled out, and a
  verification step that `console.enabled=false` is clean.
- [PF Topology at scale — hundreds of conversations or a busy namespace] → the
  wiring graph is bounded by CR count (tens); the conversation graph is bounded
  by one conversation's components. Conversation *lists* are paginated
  server-side, never graphed in aggregate.
- [Adapter-reported hops are opt-in, so some edges show intent rather than
  delivery] → rendered explicitly as "sent, unconfirmed" rather than as success.
- [Two live graph views plus a waterfall is a large frontend for one maintainer]
  → phased migration (below); the wiring graph ships before the conversation
  graph, and each phase is independently useful.

## Migration Plan

1. **Manager: activity contract.** Emit events, ring buffer, `/activity`,
   `/activity/stream`, `POST /activity`. No consumer yet; independently useful
   (`curl` becomes a live trace of the system).
2. **API: `servedBy` + optional `image` on SignalAdapter**, plus
   `SIGNAL_ADAPTER_TOKEN` injection. Independently useful; no behavior change for
   existing adapters (both optional).
3. **New console module**: BFF + API + React SPA. Built and shipped as a new
   image tag under the existing `console.*` values.
4. **Delete `console/`** and `docs/console.md`; write `docs/console.md` fresh.
5. **Chart**: `console.enabled` default `true`, new `console.write.*` and
   `console.signalSourceName`, RBAC widening, default `SignalSource`. Chart major
   bump + CHANGELOG migration entry.
6. **Rollback**: `console.enabled=false` removes the CRs and with them the pod
   and Service. Channels naming `adapter: console` report `Served=False`;
   conversations keep their other threads. The manager's activity contract and
   the `servedBy` field are additive and stay.

## Open Questions

1. ~~**Does `POST /task` still earn its place?**~~ Answered by the sibling change
   `task-is-a-signal`, which removes it and cites this design's refusal to use it.
   Neither change blocks the other: the console never calls `/task`.
2. **Should Telegram collapse to one pod** now that `servedBy` exists? Only
   `telegram-router` must stay separate (one poller per token).
3. **Ring buffer sizing** — measure an event storm on a real cluster.
4. **`run.tool` events** would show which MCP call consumed the time, but require
   instrumenting `runtime-claude` — a separate contract and change.
5. **`console.joinAllPipelines`** — inherited. With origination auto-joining what
   the console starts, this now matters only for observing other sources' work,
   which weakens the case for it further.
6. **Multi-namespace** — the event stream and both graphs assume one namespace.
   Worth settling before `split-conversation-namespace` lands.
