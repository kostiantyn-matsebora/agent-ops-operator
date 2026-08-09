# Proposal: rich-console-ui

**This REPLACES the console shipped by `2026-08-09-visualize-agent-ops`.** The
`console/` module — its Go BFF, its JSON API, and its hand-written SPA — is
removed and rebuilt. Chart values keep the `console.*` prefix so operators see
continuity, but nothing inside is carried over, and Decision 6 of
`visualize-agent-ops` ("no build toolchain, hand-rolled SVG") is explicitly
reversed.

## 1. Context

The console today answers "what is configured, and what did it do" from
Kubernetes CR state alone. The requirements below ask for something different in
kind: a Kiali-grade operations console where the **wiring graph and the
conversation graph are both live** — components lit up as messages actually move
between them — and where an operator can **originate** work rather than only
observe and reply to it.

Two of the six requirements cannot be met by any amount of frontend work,
because the data does not exist anywhere in the system:

- **Real-time traffic between components** has no source. Kiali animates edges
  from Envoy telemetry in Prometheus. agent-ops emits no per-hop record at all;
  CR status tells you a conversation is `inflight`, never that the manager just
  handed run `r-7` to a runtime and got a result 4s later. Deriving motion from
  status transitions produces a graph that is *plausible* rather than *true* —
  the one property an operations console cannot trade away.
- **Starting a conversation** is a write path into an agent that, in this
  install, holds `cluster-admin`. That is a privilege boundary, and a single
  shared bearer token is not an adequate one.

So this change is three things: a new activity-telemetry contract in the
manager, a new console service that consumes it, and a React/PatternFly UI.
Retiring hand-rolled SVG is the smallest part of it.

## 2. Summary

1. **Add an activity-event contract to the manager** — a structured, correlated
   per-hop event for every movement the manager mediates, with an SSE stream and
   a bounded replay buffer. This is the load-bearing change; both graph views
   are downstream of it.
2. **Adapters report their own hops** through the existing contracts, so an edge
   to Telegram reflects delivery rather than intent.
3. **Add a manager introspection surface** — `GET /status` for state that lives
   only in the manager's memory (op queues, runtime slots, cooldowns, leader) and
   `GET /pipelines/{name}/resolved` for authoritative capability resolution.
   The manager exposes only what only the manager knows; CR state is never
   proxied.
4. **Rebuild the console as a BFF + SPA**, replacing `console/` entirely: one Go
   service that fans in CR watches, install facts, and five manager surfaces,
   and serves a built frontend.
5. **Frontend: React 18 + TypeScript + Vite + PatternFly 6 + PatternFly React
   Topology** — Kiali's own stack, so Kiali familiarity is inherited rather than
   imitated. Built in a Docker stage, embedded with `go:embed`.
6. **Overview page** — the installation: versions, manager health, adapters,
   runtimes, capacity, and a "what is broken right now" rollup.
7. **Queues page** — live work and delivery queues: conversations waiting on a
   runtime slot, inputs waiting behind an inflight run, channel ops waiting for
   an adapter, and stuck-item detection. The view that separates "queued" from
   "stalled".
8. **Configuration page** — every `agentops.dev` CR: per-kind lists, detail with
   spec/status/conditions/YAML, resolved capabilities, and cross-reference
   validation (dangling refs, unclaimed sources, unserved channels).
9. **Topology page** — the wiring graph with live traffic animation, edge rates,
   health from reconciler conditions, and a Kiali-style Display panel toggling
   every element class.
10. **Conversations page** — filterable list; detail with transcript + chat
    composer, run timeline, inputs queue, and thread bindings.
11. **Conversation graph + sequence view** — the same live traffic, scoped to one
    conversation, as an animated topology and as a waterfall.
12. **New-conversation action** — the console is **also a `SignalSource`**, so it
    originates the way everything else does: a `chat` signal on a claimed source,
    answered by the Pipeline that claimed it. Origination becomes visible traffic
    on the graph, and self-started conversations are joined automatically.
    Needs one API affordance: an externally-served `SignalAdapter`, so both
    identities live in one pod.
13. **Ship it as its own pod** — a Deployment separate from the manager, with
    its own ServiceAccount and lifecycle, **enabled by default** and disabled
    with one chart value.
14. **Auth sized to what the console can do** — it can instruct a `cluster-admin`
    agent, so the token is the boundary and OIDC is the documented answer for
    any Ingress exposure.
15. **Delete `console/`** and its docs; ship the replacement under the same
    chart values.

## 3. Details

### 3.1 Activity events — the manager contract (new)

Everything the two graph requirements need reduces to one question the system
cannot currently answer: *what moved, from where to where, when, and did it
work?* The manager already mediates every one of those movements; it simply
does not record them.

Add an append-only, in-memory, bounded activity log with a stream:

| Endpoint | Purpose |
|---|---|
| `GET /activity?since=<cursor>&limit=` | replay from a bounded ring buffer |
| `GET /activity/stream` (SSE) | live events, with a `cursor` on each |
| `POST /activity` | adapter-reported hops (bearer, adapter-scoped) |

Event shape — deliberately flat, correlation-first:

```json
{
  "cursor":       "0000000000012345",
  "ts":           "2026-08-09T11:04:22.117Z",
  "kind":         "run.completed",
  "from":         {"kind": "runtime",  "name": "k8s-engineer"},
  "to":           {"kind": "pipeline", "name": "k8s-ops"},
  "conversation": "task-abc12",
  "pipeline":     "k8s-ops",
  "runId":        "r-7",
  "status":       "ok",
  "latencyMs":    4218,
  "detail":       "exit 0"
}
```

Event kinds, one per real hop:

| Kind | From → To | Emitted when |
|---|---|---|
| `signal.received` | signal-adapter → manager | `POST /signal/inbound` accepted |
| `signal.claimed` | signal-source → pipeline | a Pipeline claims the source |
| `signal.dropped` | signal-source → ∅ | unclaimed source; carries the `Wired=False` reason |
| `conversation.created` | pipeline → conversation | Conversation CR created |
| `input.queued` | * → conversation | input appended (task, chat, alert) |
| `run.dispatched` | pipeline → runtime | `GET /work` handed out |
| `run.completed` | runtime → pipeline | `POST /work/done` |
| `channel.op.enqueued` | pipeline → channel | `ensure-topic` / `send` queued |
| `channel.op.completed` | channel-adapter → manager | op acked, or failed with reason |
| `channel.inbound` | channel-adapter → pipeline | user reply entered the router |

`from`/`to` name **graph nodes**, so an event is directly renderable as motion
along an edge the topology already draws. No frontend inference, no guessing.

Three properties are load-bearing:

- **Bounded and lossy by design.** A ring buffer (default 10k events, ~15 min
  under load), never persisted, never in etcd. The durable record stays
  `status.runs[]`. A console that reconnects after an hour gets a `resync`
  and re-reads CR state — the same discipline the watch cache uses.
- **Telemetry is not signal.** These events are emitted to a stream, never
  through `/signal/inbound`. The no-signal-loops invariant is why: agent-ops'
  own machinery must never wake an agent. Emitting a hop event *about* a hop
  event is impossible by construction — activity emission is not itself a hop.
- **Zero secret reads, zero new RBAC.** The manager holds these in memory and
  serves them under the existing bearer scheme.

*Alternative considered — OpenTelemetry spans to Tempo/Jaeger, with a Kiali-style
traces tab.* Richer, and the right long-term answer if agent-ops ever needs
cross-process latency forensics. Rejected for now: it makes a tracing backend a
hard dependency of seeing your own graph move, and the console's needs are one
namespace and one process. The event shape above is deliberately span-like
(`runId` is a trace id in all but name), so an OTel exporter is additive later.

*Alternative considered — derive motion from CR watch deltas.* No new manager
contract, but the console would animate `pipeline → channel` because a
conversation went `inflight`, not because anything was sent. It cannot see other
adapters' ops at all, so every Telegram edge would be fabricated. Rejected: a
graph that can lie is worse than no graph.

### 3.2 Adapter-reported hops

The manager knows an op was *enqueued*; only the adapter knows it was
*delivered*. Adapters `POST /activity` with `channel.op.completed` and their own
latency, authenticated with the adapter token they already hold. This is
opt-in — an adapter that reports nothing still shows manager-side hops, just
without delivery confirmation, and the UI renders that edge as "sent,
unconfirmed" rather than pretending.

### 3.3 The console service (BFF)

**The stack, stated once so it is not inferred from anywhere else:**

| Half | Language / stack | Why |
|---|---|---|
| **Backend (BFF)** | **Go**, a new module replacing `console/` | Same as every other adapter and the manager: it talks to the manager's HTTP API and does raw Kubernetes list/watch, shipping as one distroless binary. Nothing about this half changes language. |
| **Frontend (SPA)** | **React 18 + TypeScript + Vite + PatternFly 6 + PatternFly React Topology** | Kiali's own stack — see §3.4. |

The frontend is built in a Docker stage and embedded into the Go binary with
`go:embed`, so the deployable artifact stays exactly what it is today: **one Go
image serving one port**. npm exists at build time only, inside `console/`.
No other module, and not the manager, gains a build step or a JavaScript
dependency.

The Go service fans in four sources the browser must never touch directly:

| Source | Where from | Why the BFF owns it |
|---|---|---|
| CR state, all `agentops.dev` kinds | Kubernetes API (list/watch) | RBAC belongs to a ServiceAccount, not a browser token |
| Install facts (Deployments, pods, images) | Kubernetes API | needs a read grant the browser cannot hold |
| Activity events | manager `GET /activity/stream` | one upstream connection multiplexed to N browsers |
| Manager runtime state | manager `GET /status` | in-memory only — exists nowhere else (§3.3.1) |
| Resolved capabilities | manager `GET /pipelines/{name}/resolved` | authoritative; recomputing it invites disagreement (§3.3.1) |
| Reply path | manager `POST /channel/inbound` | write, authorized per user |
| Origination path | manager `POST /signal/inbound` | write, authorized per user |

So the console talks to the manager over **five** surfaces, not two: the channel
and signal contracts it holds adapter identities for, plus activity, status and
resolution. Only CR and install state come from the API server directly.

#### 3.3.1 What the console asks the manager for — and what it must not

`visualize-agent-ops` rejected a manager-side `/console/state` API on the
grounds that the manager would become a CR-snapshot proxy — a new surface and a
new auth scope duplicating what the API server already serves with proper RBAC.
That rejection stands, and it draws the line precisely:

> **The manager exposes only what only the manager knows.** Anything that lives
> in a CR is read from the API server and never proxied.

Two classes of fact fall on the manager's side of that line, and both are
currently unreachable by any client:

**Manager runtime state — `GET /status`.** The `OpQueue` is "in-memory by
design"; pending and claimed channel ops exist in no Kubernetes object. Neither
does the runtime slot count against `MAX_RUNTIMES`, nor cooldown state, nor
which manager replica holds the lease. That makes ordinary operational
questions unanswerable today: *is the op queue backed up? is an adapter claiming
ops and not completing them? are we at the runtime ceiling, so conversations are
queueing rather than running?* An operations console that cannot answer those is
decorative. `/status` returns build info, leader identity, runtime slots in use
against the ceiling, per-adapter op queue depth with the oldest queued and
oldest claimed-but-uncompleted op, and cooldown state.

#### 3.3.2 The same facts as standard Prometheus metrics

`/status` is a console API. Alerting must not depend on anyone having a browser
open, so the aggregates are **also** exposed in the Prometheus exposition format
on the manager's existing `:9090/metrics`, registered into the controller-runtime
registry that already serves that port. Nothing new is listened on.

**One emission point, two outputs.** The activity emitter (§3.1) is where every
hop is already observed, so it is also where metrics are incremented: emit once,
fan out to the ring buffer for the console and to the registry for Prometheus.
Two instrumentation passes over the same call sites would drift.

Conventions are the standard ones, not house style: `agentops_` namespace,
`_total` on counters, base units with `_seconds` suffixes, `HELP` and `TYPE` on
every series, OpenMetrics-compatible output.

| Metric | Type | Labels |
|---|---|---|
| `agentops_signals_received_total` | counter | `source`, `adapter`, `status` |
| `agentops_signals_dropped_total` | counter | `source`, `reason` |
| `agentops_conversations_created_total` | counter | `pipeline` |
| `agentops_runs_total` | counter | `pipeline`, `status` |
| `agentops_run_duration_seconds` | histogram | `pipeline` |
| `agentops_channel_ops_total` | counter | `adapter`, `kind`, `status` |
| `agentops_channel_op_latency_seconds` | histogram | `adapter`, `kind` |
| `agentops_channel_ops_queued` | gauge | `adapter` |
| `agentops_channel_ops_claimed` | gauge | `adapter` |
| `agentops_channel_op_oldest_queued_age_seconds` | gauge | `adapter` |
| `agentops_channel_op_oldest_claimed_age_seconds` | gauge | `adapter` |
| `agentops_runtime_slots_in_use` | gauge | — |
| `agentops_runtime_slots_max` | gauge | — |
| `agentops_conversations_inflight` | gauge | `pipeline` |
| `agentops_cooldowns_active` | gauge | `source` |

**The cardinality rule is binding.** Labels may carry only values bounded by CR
count — `pipeline`, `adapter`, `source`, `channel`, `kind`, `status`, `reason`.
A conversation id, run id or op id as a label would grow series without limit;
those identify the *specific* stuck item and stay in `/status`, which is exactly
the split: metrics answer "how deep, how old, how many", `/status` answers
"which one".

The chart ships an optional `VMServiceScrape` (and a `ServiceMonitor`
equivalent) plus example alert rules for the two conditions that matter —
ops queued with nothing claiming them, and runtime slots pinned at the ceiling
with waiters. Both off by default, since neither CRD is guaranteed present.

#### 3.3.3 Three consumers of one instrumentation pass

Standardizing on the Prometheus protocol means the same emission serves three
audiences, and the third one is worth calling out because it removes a
limitation this design otherwise accepts:

1. **The console, live** — the activity stream and `/status`, exact and
   per-item, over the ring buffer's window.
2. **Prometheus / VictoriaMetrics, Grafana, Alertmanager** — dashboards,
   long-retention trends and alerting that work with no console running at all.
   This is agent-ops becoming a normal citizen of an existing monitoring stack
   rather than a system you can only observe through its own UI.
3. **The console, historically** — *optionally* configured with the query URL of
   the metrics backend (`console.metrics.url`), the console reads its own series
   back and renders time ranges far beyond the ring buffer: throughput per
   pipeline, run-duration percentiles, queue depth over a week. This is
   precisely how Kiali works — live detail from one source, rates and history
   from Prometheus.

That splits the two window classes honestly, and it is the same split Kiali
makes:

| Window | Source | Gives |
|---|---|---|
| Live and recent | activity stream + `/status` | exact per-item detail — this op, this run, this conversation |
| Historical | metrics backend | aggregates only — rates, percentiles, depths; no per-item identity |

**Optional, and degrading cleanly.** With no metrics URL configured the console
is fully functional and simply offers no windows past the buffer, saying so
rather than rendering an empty chart. The console never requires a monitoring
stack to work — it just gets better when one is present.

**Resolved capabilities — `GET /pipelines/{name}/resolved`.** Capability
resolution is manager logic: toolset composition modes, the agent definition's
declared tools, MCP configs, and the runtime's allowlist combine into what an
agent may actually do. The Configuration page's most valuable question — *what
can this agent actually reach?* — is exactly that resolution. The console must
not recompute it: a second implementation of composition would eventually
disagree with the one that runs, and the console's whole claim is that it cannot
disagree with the system. So it asks, and renders the answer verbatim.

Both are read-only, both are adapter-token authenticated, and neither proxies a
CR.

**The console holds both adapter identities, for the same reason Telegram does.**
A `ChannelAdapter` + `Channel` because chatting inside a conversation means
holding a thread binding, and only a channel gets one (requirement 4). A
`SignalAdapter` + `SignalSource` because starting a conversation means emitting
a signal from a claimed source (requirement 6). That is the split the
architecture already settled on for chat transports — the general surface
originates, topics continue — and the console is a chat transport that happens
to render a graph. §3.10 covers the origination half.

**It is its own component — one Deployment, one pod, one ServiceAccount, its own
image and release cadence.** Nothing about the console lives inside the manager:
a UI crash, an OOM from a large graph, or a console rollout must not touch
dispatch. The `ChannelAdapter` reconciler owns the Deployment and (because
`port` is set) the Service; the `SignalAdapter` is externally served (§3.10) and
owns no workload, so two identities still mean **one pod**. The chart renders
CRs and RBAC rather than workload plumbing — the same shape every other adapter
has.

`console.enabled` (**default `true`**) is the switch. Setting it `false` removes
the CR, and with it the Deployment, the pod and the Service; Channels naming
`adapter: console` report `Served=False`, and conversations keep their other
threads. Nothing else in the install changes. The console runs `replicas: 1`
with `Recreate`: it holds the channel op loop and every browser's SSE
connection, so a second replica would split both.

Browser-facing API — one snapshot endpoint per page, one multiplexed stream:

```
GET  /api/overview                    installation facts + health rollup
GET  /api/config/{kind}[/{name}]      CR inventory and detail (+ YAML)
GET  /api/topology                    nodes, edges, health, live rates
GET  /api/conversations               filtered, paginated
GET  /api/conversations/{name}        detail: runs, inputs, threads, transcript
GET  /api/conversations/{name}/graph  per-conversation nodes + edges + events
POST /api/conversations               start one (pipeline, task, agent?, title?)
POST /api/conversations/{name}/messages   chat in scope
GET  /api/stream                      SSE: CR deltas, activity events, transcript
```

**Snapshots are authoritative; the stream carries deltas plus a monotonic
cursor.** A browser that misses events re-fetches and converges. Reconnect and
first-connect are the same code path.

### 3.4 Frontend stack

**React 18 + TypeScript + Vite + PatternFly 6 + PatternFly React Topology.**

The requirement is explicitly "based on Kiali UI", and this is Kiali's stack.
PatternFly supplies the shell, tables, drawers, and empty/error states that
otherwise cost weeks; PF React Topology supplies node/edge rendering, layouts,
grouping, and — critically — **edge animation with rate-driven speed**, which is
exactly the Kiali traffic idiom, rather than something to reimplement in raw SVG.

- State: TanStack Query for snapshots, one SSE multiplexer into a Zustand store
  for live events. Query invalidation is driven by stream cursors.
- Graph: PF React Topology. *Alternative: Cytoscape.js* — better exotic layouts,
  but a second visual language next to PatternFly and no PF theming.
- Charts (run duration, throughput): PF Charts (Victory).
- Tests: Vitest + React Testing Library; Playwright for one graph smoke test.

**Angular was considered and is not recommended.** It is a capable choice, but
PatternFly is React-first and Kiali is React — Angular means rebuilding the
design system that supplies the requested look, and reimplementing the topology
component that supplies the requested behavior. The Kiali-parity requirement is
what decides this, not a general preference.

**Build:** multi-stage Dockerfile — `node:22` builds `ui/dist`, the Go stage
embeds it with `go:embed all:ui/dist`. A `dev` build tag serves from disk for
local work, and `make ui` produces `dist` for a plain `go build`. The
dependency-free rule that governs adapter modules is deliberately not applied
here: the console is a frontend, npm stays inside its module and its image, and
no other module or the manager gains a build step.

### 3.5 Page 1 — Overview (requirement: installation details)

The install at a glance, and what is wrong with it:

- **Versions** — chart version, `appVersion`, manager image and digest, each
  adapter image, each `AgentRuntime` image. Sourced from Deployment pod specs
  and CR specs.
- **Manager** — replicas ready, leader identity, HTTP surface reachable, uptime,
  `MAX_RUNTIMES` and current runtime pods against it.
- **Adapters** — every `ChannelAdapter`/`SignalAdapter`: ready, image, port,
  `servedChannels`, singleton, whether it reports activity.
- **Capacity** — active conversations, queued inputs, runtime pods in use.
- **Problems** — every non-`True` condition across every kind, newest first,
  each linking to the object. This is the page's real job: the answer to "is my
  install healthy" should not require reading four other pages.

*New grant required:* namespaced read-only `get/list/watch` on `deployments` and
`pods`. Versions and manager health are not in any CR. This widens the console
SA beyond `agentops.dev` — called out explicitly because the previous design
deliberately avoided pod inspection. The narrower alternative is a manager
`GET /version` endpoint plus per-adapter self-report, which avoids the grant but
covers less (no image digests, no restart counts, no pod-level failure reasons).
**Recommendation: take the grant**; it is read-only, namespaced, and an
operations console that cannot see a CrashLoopBackOff is not one.

### 3.5.1 Page 2 — Queues and capacity

Its own view, because the question it answers is asked under pressure and has
two completely different answers that look identical from outside: **an agent
has not replied — is it queued, or is it stuck?**

Two queues, deliberately kept separate:

- **Work queue** — conversations waiting for a runtime slot against
  `MAX_RUNTIMES`, and inputs waiting behind an inflight run (dispatch is
  strictly serial per conversation, so a busy conversation queues its own
  messages). Sources: `/status` for the ceiling and slots in use, CR status for
  per-conversation queued inputs (`spec.inputs[]` against
  `status.processedInputIDs[]`).
- **Delivery queue** — channel ops waiting for an adapter to claim them, and ops
  claimed but never completed. Source: `/status`, because `OpQueue` is in-memory
  and appears in no Kubernetes object.

Each row carries an age, and age is what separates the two failure modes:

| Symptom | Reading |
|---|---|
| Queue deep, ages short and turning over | Healthy backlog — the system is busy |
| Slots at ceiling, oldest wait growing | Capacity-bound; raise `MAX_RUNTIMES` or shed load |
| Ops queued, none claimed | An adapter is down or not polling |
| Ops claimed, never completed, age climbing | An adapter is wedged mid-delivery |
| Conversation inflight far beyond typical run time | The runtime is hung, not queued |

The view SHALL flag stuck items rather than leaving the operator to compare
timestamps, and each row links to the conversation, adapter or pipeline it
concerns. Cooldowns are shown here too — a suppressed signal lane looks exactly
like an idle one on a graph, and this is where that distinction belongs.

Live updates: the BFF polls `/status` on a short interval and pushes deltas over
the stream browsers already hold, so the queue view needs no new browser
connection. Per-conversation queueing comes from the CR watch and needs no poll
at all.

### 3.6 Page 3 — Configuration (requirement: custom resources)

Per-kind inventory → detail. Each kind gets a purpose-built list (a Pipeline row
shows profile/sources/channels; a Channel row shows adapter and served state),
and each detail view shows conditions, the full spec, the raw YAML, and inbound
references ("used by these 2 pipelines").

A Pipeline's detail additionally shows its **resolved capabilities** — the
composed tool allowlist, effective toolsets, MCP servers and runtime — fetched
from `GET /pipelines/{name}/resolved` and rendered verbatim. This is the "what
can this agent actually reach" answer, and it is asked for rather than
recomputed on purpose (§3.3.1).

Beyond rendering, the page **validates across objects** — the checks that
currently require assembling seven kinds by hand: refs that resolve to nothing,
`SignalSource`s no pipeline claims (they silently drop signals), `Channel`s
whose adapter is absent, adapter `configSchema` violations, and pipelines whose
profile has no runtime. Findings are advisory, sourced from reconciler
conditions where one exists, and clearly marked when they are the console's own
cross-reference rather than a reported condition.

Read-only. CR editing is out of scope: Pipelines are the wiring, the wiring is
GitOps-managed here, and a console that edits them competes with helmfile.

### 3.7 Page 4 — Topology (requirement: pipeline graph + live traffic)

The Kiali graph, for agent-ops.

**Nodes — every CRD involved, not just the wiring spine:** SignalSource,
SignalAdapter, Pipeline, AgentProfile, AgentRuntime, Channel, ChannelAdapter,
MCPToolset and MCPConfig. **Edges:** `feeds`, `answers`, `posts`, `served-by`,
`uses`.

- **A Kiali-style Display panel toggles element classes.** Sources, channels,
  adapters, profiles, runtimes, toolsets, MCP configs and runtime pods each show
  or hide independently, so the graph reads as the wiring spine when you want
  orientation and as the full capability picture when you are debugging what an
  agent can actually reach. The panel also carries traffic animation on/off,
  idle nodes and edges shown/hidden, and edge labels (none, rate, latency).
  Selections persist across navigation and reload.
- **Hiding is presentation only.** A hidden class still counts toward the
  graph's health summary and the overview's problem rollup, and the panel says
  when hidden elements include failures — a filter that can conceal a broken
  component without saying so is the one way this view could mislead.

- **Health colors come from reconciler conditions only** (`Ready`, `Served`,
  `Wired`). The console asserts no health of its own, so the graph cannot
  disagree with `kubectl`.
- **Traffic animates from activity events.** Edge dash speed scales with event
  rate; error events flash the edge red and badge it with the reason. An edge
  with no events is visibly idle rather than absent.
- **The console's own source is on the graph.** Its `SignalSource` node sits
  beside the cluster-event and alert sources, wired to whichever Pipeline claimed
  it — so "start a conversation" is an edge the operator can see before pressing
  it, and traffic on it afterwards.
- **Time window** — live, 5m, 1h, driven by the replay buffer.
- **Side panel** on select: the node's conditions, its recent events, its
  conversations.
- Unclaimed sources render detached with their `Wired=False` reason; dangling
  refs render as broken edges to placeholder nodes.

### 3.8 Page 5 — Conversations (requirement: list, detail, chat)

List: filter by phase, pipeline, profile, channel, age, errored; sort by last
activity; server-side paginated (an event storm makes thousands).

Detail, tabbed:

- **Transcript** — messages with a composer. Conversations the console started
  are joined automatically (§3.10). For ones another source started, the
  composer is live when the console channel holds a thread; when it does not,
  the tab explains why and shows the exact patch to add the console channel to
  that pipeline.
- **Runs** — timeline of `status.runs[]`: status, exit code, duration, result,
  and the inputs each run consumed.
- **Inputs** — the queue, including what is not yet processed.
- **Graph** and **Sequence** — §3.9.
- **YAML** — the Conversation object.

### 3.9 Page 6 — Conversation graph + sequence (requirement: per-conversation traffic)

Same events, filtered to one `conversation`, rendered two ways:

- **Graph** — every element this conversation involved: its originating source
  and adapter, the pipeline, the AgentProfile, that profile's AgentRuntime, the
  runtime pod, each bound Channel with its adapter, and every MCPToolset and
  MCPConfig it materialized. The same Display panel applies, so the toolset
  layer can be folded away or brought forward. Live, animated identically to
  the topology view, so the idiom transfers.

  **Built from what the Conversation recorded, not from the Pipeline's current
  spec.** A Conversation snapshots the bindings it materialized, so if the
  pipeline has since been re-wired, this graph shows the capabilities that
  conversation actually ran with — and says the current wiring differs. That is
  the question a per-conversation graph exists to answer: *what could this agent
  reach when it did that?*
- **Sequence** — a waterfall over the same events: signal in, conversation
  created, run dispatched, run completed, ops sent, replies in — with
  per-hop latency. This is where "why did that take 40 seconds" gets answered,
  and it is the view a graph cannot replace.

### 3.10 Start a conversation — the console is a SignalSource (requirement 6)

**The console originates conversations the way everything else does: by emitting
a signal from a claimed `SignalSource`.** Not through `POST /task`.

`POST /api/conversations {source, task, agent?}` → the console posts a
`kind: chat` signal to `/signal/inbound` naming its own SignalSource, carrying
`agentops.dev/channel: <console channel>` and `agentops.dev/sender: <identity>`.
The Pipeline claiming that source answers it. This is the same lane a Telegram
message on the general surface travels.

Four things fall out of this that `/task` cannot give:

1. **The invariant holds literally.** "Conversations originate only from claimed
   signal sources" stops being a rule with a side door. `/task` is a second
   origination mechanism that bypasses the wiring model; not using it means not
   having to defend it.
2. **Who answers is declared, not chosen by the caller.** With `/task` the
   console names the pipeline in the request — the caller picks the agent. With
   a SignalSource, the Pipeline that claimed it decides, which is the invariant's
   actual point. The console cannot reach an agent no wiring points at.
3. **Origination becomes visible traffic** — and this is what ties requirement 6
   back to requirements 3 and 5. A `/task` conversation materializes from
   nowhere: the graph has a hole exactly where the operator acted. A console
   SignalSource is a real node with a real edge, so pressing "start" lights up
   the same graph the console is showing, from a node the operator can point at.
4. **Self-started conversations are joined automatically.** The router appends
   the originating channel to the conversation's `channelRefs`, so a conversation
   started from the console already has a console thread — no pipeline edit, no
   copy-paste patch. Requirements 4 and 6 stop being two features and become one
   path. (Joining a pipeline remains necessary only to *observe and reply to*
   conversations other sources started.)

And the failure mode is diagnosable with machinery that already exists: an
unclaimed console source sits at `Wired=False`, which is exactly "this console
cannot start conversations yet, because no Pipeline claims it" — rendered in the
UI, from a condition a reconciler wrote, with the patch that fixes it.

#### One source per target, and that is the picker

A `SignalSource` is claimed by exactly one Pipeline. So one console source means
one destination — the UI's "New conversation" picker lists **the console
SignalSources that are `Wired=True`**, each labeled with its claiming pipeline
and that pipeline's profile. To originate to several agents, declare several
sources (`console-k8s`, `console-ha`), each claimed by its own pipeline.

This is a feature, not a workaround: *what you can start is what is wired.* The
picker is a rendering of the topology rather than a free-text pipeline field
that can name something no wiring supports. The chart renders one source
(`console`) by default; a fresh install has one destination as soon as some
Pipeline claims it, and the UI says so when none does.

#### The API affordance this needs

The console pod must hold two identities: `ChannelAdapter/console` (carry and
chat) and `SignalAdapter/console` (originate). Today each reconciler owns a
workload and `SignalAdapter.spec.image` is required, so declaring both would
produce **two Deployments** — and one of them would be an idle pod existing only
to make a source `Served`. That is precisely the shape this repo already paid
for once: telegram-router "used to be an adapter with a signal-free SignalSource
purely to carry that credential — which then sat at `Wired=False`."

So add an explicit externally-served mode:

- **`SignalAdapter.spec.image` becomes optional**, alongside a new
  **`spec.servedBy: {kind: ChannelAdapter, name: console}`**. When `servedBy` is
  set the SignalAdapter reconciler creates **no** Deployment, Service or
  ServiceAccount, and reports `Ready` with reason `ServedBy`.
- **The ChannelAdapter reconciler injects `SIGNAL_ADAPTER_TOKEN`** into the pod
  of any adapter a SignalAdapter names — derived exactly as today
  (`HMAC(master, "signal-adapter:"+name)`), stateless, nothing minted or stored.
  A single env var is the whole mechanism; both surfaces keep validating only
  against their own CRD list, so the two identities stay separate.

The difference from the telegram-router mistake is the one that matters: that
source was signal-free and could never be meaningfully claimed. This one
originates real conversations and a Pipeline claims it for real work.

This generalizes past the console. A chat transport is inherently both a
surface and an originator — Telegram is that today across *three* pods
(`signal-telegram`, `channel-telegram`, `telegram-router`) precisely because one
adapter cannot be both. This affordance is the prerequisite for ever collapsing
that, which is a good sign it is the right shape rather than a console hook.

#### `POST /task` — not used here, and being removed elsewhere

This design does not call `/task`, so nothing here blocks on it. The sibling
change **`task-is-a-signal`** removes the route outright, citing this
proposal's refusal to use it. Two findings that motivated that change, recorded
here because they were surfaced from this side:

1. **`POST /task` is unauthenticated** — `internal/httpapi/server.go:78`, no
   `adapterAuth`, unlike every `/channel/*` and `/signal/*` route. Anything with
   cluster-network reach can start a conversation on a `cluster-admin` agent.
   Moot once the route is deleted; worth fixing first if that change stalls.
2. **`docs/contracts.md:148` documents `{"profile",…}`**; the implementation
   requires `{"pipeline",…}` and 400s otherwise. Owned by `task-is-a-signal`,
   which deletes the row rather than correcting it.

If both changes land, origination collapses to exactly one model: a claimed
signal source, whether the signal came from a cluster event, an alert, a chat
message, or this console.

### 3.11 Auth

Requirements 4 and 6 make the console a control plane, not a viewer: it can
instruct an agent that, in this install, holds `cluster-admin`.

Since the console is on by default, its features are on by default too — a
default-enabled component whose headline capabilities are default-disabled is
just a confusing way to ship nothing. The posture that makes that defensible:

- **The token is the boundary, and it is generated per install.** Whoever holds
  it can read every CR the SA can read and instruct any joined agent. This is
  the posture the system already has — the Telegram bundle's `approvers` is
  optional, so today anyone in the group may talk to the agent. The console is
  not a new exposure class; it is the same one with a browser.
- **`ClusterIP` by default, `console.ingress.enabled: false`.** Reaching it
  means a port-forward or a deliberate decision.
- **OIDC is the documented answer for Ingress exposure**, via forward-auth from
  a proxy that has already authenticated the user — oauth2-proxy against Vault
  OIDC is what this cluster already runs in front of comparable UIs. When a
  trusted identity header is present the console records it; when it is absent
  it records `token`.
- **`console.write.enabled`** exists for installs that want a strict viewer
  (default `true`). Turning it off hides the composer and the new-conversation
  action and rejects both endpoints server-side.
- Every write is logged with the resolved identity: who started what, and who
  said what to an agent.

### 3.12 Removal

`console/` is deleted: module, image, `docs/console.md`, and its chart
templates' internals. Chart values keep the `console.*` prefix and their
meanings where they still apply (`name`, `channelName`, `image`, `port`, `auth`,
`ingress`, `resources`), and gain `console.write.enabled` plus
`console.signalSourceName` (default `console`) for the origination source.
Anyone on the old console changes an image tag and gets the new one; no CR
surgery, no re-wiring. Pipelines already listing the console channel keep
working — they gain origination by adding the console source to
`signalSourceRefs[]`.

**`console.enabled` flips from `false` to `true`.** That is a breaking chart
change in the sense that matters — an upgrade starts a pod that was not running
before, on a component that reads every CR in the namespace — so it belongs in
`CHANGELOG.md` with the opt-out spelled out, and the chart major bumps.

### 3.13 Capabilities and impact

**New capabilities**

- `activity-telemetry` — the manager's per-hop event contract: ring buffer,
  replay, SSE stream, adapter-reported hops.
- `manager-introspection` — `GET /status` and `GET /pipelines/{name}/resolved`,
  plus the boundary rule that the manager exposes only what only it knows.
- `console-origination` — the console as a claimed SignalSource; the picker,
  auto-join, and the gating.
- `console-application` — the BFF: fan-in, snapshot/stream model, overview and
  configuration surfaces, auth.

**Modified capabilities**

- `signal-adapter-lifecycle` — optional `image`, `servedBy`, and
  `SIGNAL_ADAPTER_TOKEN` injection into the serving pod.
- `console-adapter` — origination identity alongside the channel identity;
  self-started conversations arrive joined.
- `console-topology` — all nine node kinds, event-driven traffic, the Display
  panel, the per-conversation graph and sequence.
- `console-live-runs` — filterable server-side listing, full conversation
  detail, composer semantics.
- `console-deployment` — own single-replica pod, default enabled, widened
  read-only RBAC, new values.

**Impact**

- `internal/activity/` (new), `internal/httpapi/server.go` (activity routes),
  emission call sites in ingest, dispatch and the channel op pipeline.
- `api/v1alpha1/signaladapter_types.go` + regenerated CRDs;
  `internal/controller/signaladapter_controller.go` and
  `channeladapter_controller.go`.
- `console/` replaced wholesale: Go BFF plus a `ui/` React application and a
  multi-stage image build.
- `chart/`: console CR templates, RBAC, values, chart major bump, CHANGELOG
  migration entry.
- `docs/console.md` rewritten; `docs/contracts.md` gains the activity contract
  and loses a stale `/task` row; `docs/concepts.md` gains `servedBy`.
- Security surface: one read grant widened (`deployments`, `pods`), one write
  path added (origination), both documented as the trust boundary.

## 4. Verification

**The telemetry contract**

- Unit: every emitter produces exactly one event per hop, with correlation ids
  populated; ring buffer evicts oldest and never blocks an emitter.
- envtest: drive one conversation end to end (signal → dispatch → done → send)
  and assert the event sequence, its `from`/`to` pairs, and that latencies are
  consistent with the timestamps.
- Invariant test: no activity emission reaches `/signal/inbound`; a synthetic
  storm of events creates zero Conversations.
- Load: 10k events in ≤ 30s stays flat in memory and drops oldest first.

**The console**

- Contract tests per endpoint against a fixture cache; snapshot/stream
  convergence test — apply N deltas with the stream disconnected, reconnect, and
  assert the rendered state equals a cold fetch.
- Graph correctness: a fixture namespace with a dangling ref, an unclaimed
  source, and an unserved channel renders one broken edge, one detached node,
  and one `Served=False` node — and matches what `kubectl` reports for each.
- Playwright: load the topology, drive three synthetic events, assert the
  animated edges are the ones the events name.

**Origination and the externally-served adapter**

- envtest: a `SignalAdapter` with `servedBy` creates **no** Deployment, Service
  or ServiceAccount, and reports `Ready=True/ServedBy`; removing `servedBy` and
  supplying `image` produces a workload again, so the modes are reversible.
- envtest: the ChannelAdapter pod named by a `servedBy` SignalAdapter carries
  `SIGNAL_ADAPTER_TOKEN`, its value re-derives to the signal-adapter token, and
  the channel and signal tokens are not equal.
- Contract: a `chat` signal from the console source with `agentops.dev/channel`
  set creates a Conversation whose `channelRefs` already include the console
  channel — the auto-join claim, asserted rather than assumed.
- Negative: with no Pipeline claiming the console source, origination is refused,
  the source reports `Wired=False`, and the UI surfaces that reason instead of a
  generic error.
- Negative: an unauthenticated `POST /api/conversations`, and one with writes
  disabled, are both rejected server-side.

**Acceptance, per requirement**

| # | Requirement | Passes when |
|---|---|---|
| 1 | Installation details | Overview names every image and version in the namespace, and lists every non-`True` condition |
| 1b | Queue state | Work and delivery queues are visible with ages; an unclaimed op, a wedged adapter and slot exhaustion are each flagged with the right cause |
| 2 | Configuration | Every `agentops.dev` kind is listable and inspectable, YAML matches `kubectl get -o yaml` |
| 3 | Pipeline graph + live traffic | Wiring matches the CRs; an event moves a visible edge within 1s |
| 4 | Conversation list / detail / chat | A message sent from the UI appears in the agent's inputs and its reply returns to the transcript |
| 5 | Conversation traffic | Graph and sequence show only that conversation's hops, with per-hop latency |
| 6 | Start conversation | Picking a wired console source + task emits a `chat` signal, the claiming Pipeline answers, and the console is joined to the result without any pipeline edit |

**Manual, on this cluster**

Deploy at defaults (console on), join `k8s-ops` to the console channel, then
start a conversation from the UI against the `stub` runtime (no LLM cost) and
watch it traverse the topology graph, land in the conversation graph, and answer
in the transcript.

Then set `console.enabled=false` and confirm the opt-out is clean: pod, Service
and Deployment gone, `Served=False` on the console Channel, every other pipeline
still delivering, and no Conversation losing a thread it depends on.

## 5. Open questions

1. ~~**Does `POST /task` still earn its place?**~~ **Answered elsewhere** — the
   sibling change `task-is-a-signal` removes it, citing this proposal's refusal
   to use it as precedent. No dependency runs in either direction for
   implementation: this change never calls `/task`, so it lands with or without
   that one. Two notes: the unauthenticated-route finding in §3.10 becomes moot
   once the route is gone, and the stale `{"profile",…}` doc row is that
   change's to fix — dropped from this change's task list to avoid a collision.
2. **Should Telegram collapse to one pod?** The `servedBy` affordance makes
   `signal-telegram` + `channel-telegram` mergeable, leaving `telegram-router` as
   the only separate piece (it must stay one poller per token). Not in scope
   here; worth deciding before another transport is written to the three-pod
   pattern.
3. **Ring buffer size and window** — 10k events is a guess. Sizing should come
   from an event-storm measurement on this cluster, where a bad hour produced
   ~2000 conversations.
4. **Does the sequence view need `run.tool` events?** Seeing which MCP call took
   the time is the natural next question, but it requires instrumenting
   `runtime-claude`, which is a separate contract and a separate change.
5. **Whether `console.joinAllPipelines` should exist** — inherited open question.
   A chart mutating user Pipelines remains a questionable pattern; the UI showing
   the exact patch is the current answer and may be enough.
6. **Multi-namespace** — out of scope here, but the event stream and the graph
   both assume one namespace. Worth knowing before `split-conversation-namespace`
   lands, since that change makes conversations live elsewhere.
