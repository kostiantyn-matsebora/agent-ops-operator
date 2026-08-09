# The agent-ops console

A browser view of the whole install — what is configured, what is running, what
is moving between components right now — that is **also a channel** (so you can
reply to an agent from the screen you are watching it on) and **also a signal
source** (so you can start one).

It ships as its own pod, **enabled by default**, and is disabled with one value.

```yaml
console:
  enabled: false   # the whole opt-out
```

## What it is, structurally

The console holds **two adapter identities in one pod**:

| Identity | CR | What it buys |
|---|---|---|
| Channel | `ChannelAdapter/console` + `Channel/console` | conversations bound to it get a console thread, so the transcript IS the channel rather than a rendering of one |
| Signal | `SignalAdapter/console` (`servedBy` the ChannelAdapter) + `SignalSource/console` | "start a conversation" travels the sanctioned lane — a chat signal from a claimed source |

The SignalAdapter is **externally served**: it declares
`servedBy: {kind: ChannelAdapter, name: console}`, owns no Deployment, Service
or ServiceAccount, and the ChannelAdapter reconciler injects
`SIGNAL_ADAPTER_TOKEN` into the same pod. Two identities, **one** Deployment.
See [concepts](concepts.md#servedby--two-identities-one-pod).

It fans in five sources the browser never touches directly:

| Source | From | Why the console owns it |
|---|---|---|
| CR state, every `agentops.dev` kind | Kubernetes list/watch | RBAC belongs to a ServiceAccount, not a browser token |
| Install facts — Deployments, pods, images, digests, restarts | Kubernetes list/watch | needs a read grant the browser cannot hold |
| Per-hop activity | manager `GET /activity/stream` | one upstream connection multiplexed to N browsers |
| Runtime state — op queues, slots, cooldowns, leader | manager `GET /status` | in-memory only; exists nowhere else |
| Resolved capabilities | manager `GET /pipelines/{name}/resolved` | authoritative; recomputing it invites disagreement |

Plus two write paths, both through the manager: `POST /channel/inbound` (reply)
and `POST /signal/inbound` (originate).

## Pages

### Overview

Versions and image digests, manager health and leader, adapters, runtimes,
capacity — and the section that is the page's actual job: **every non-`True`
condition across every kind**, plus pod-level failure, newest first, each linking
to the object.

Each problem is labelled by where it came from: `reported` (a condition a
reconciler wrote), `pod` (kubelet-reported workload state), or `console check`
(the console's own cross-reference). Those carry different authority and are
never presented as the same thing.

### Queues

The view that separates **queued** from **stalled**. Two queues, kept apart
because they fail differently:

- **Work queue** — conversations waiting for a runtime slot, and inputs waiting
  behind an inflight run (dispatch is strictly serial per conversation).
- **Delivery queue** — channel ops waiting for an adapter to claim them, and ops
  claimed but never completed. This exists in **no Kubernetes object**; without
  the manager's `/status` it is reported unavailable rather than rendered empty.

Every row carries an age, and stuck items are flagged with the cause, because
each cause has a different fix:

| Flag | Reading |
|---|---|
| `nothing claiming` | ops queued, no adapter polling — its pod is down |
| `adapter wedged` | claimed and never completed — stuck mid-delivery |
| `at runtime ceiling` | capacity-bound, **not broken** — raise `maxActiveConversations` or shed load |
| `runtime hung` | inflight far beyond typical — the runtime is hung, not queued |

Cooldowns are shown here too: a suppressed signal lane looks exactly like an idle
one on a graph, and this is where that distinction belongs.

### Configuration

Per-kind inventory with kind-specific columns → detail with conditions, the full
spec, YAML, and inbound references ("used by these 2 pipelines" — the *is it safe
to delete this* answer).

A Pipeline's detail additionally shows its **resolved capabilities**: the composed
tool allowlist, the composition mode, effective toolsets, MCP configs and servers,
and the resolving runtime — fetched from the manager and rendered verbatim. The
console does not recompute composition; a second implementation would eventually
disagree with the one that runs.

Beyond rendering, the page validates **across** objects: dangling refs, sources no
pipeline claims, channels whose adapter is absent, profiles whose runtime does not
exist, `configSchema` violations. Findings are advisory and marked as the
console's own.

**Read-only.** CR editing is out of scope: Pipelines are the wiring, the wiring is
GitOps-managed, and a console that edits them competes with helmfile. There is no
write path to the Kubernetes API anywhere in the module.

### Topology

The wiring graph with live traffic.

- **Nodes: all nine kinds** — SignalSource, SignalAdapter, Pipeline, AgentProfile,
  AgentRuntime, Channel, ChannelAdapter, MCPToolset, MCPConfig. Not just the
  spine: "what can this agent actually reach" is a question about exactly those
  last two.
- **Edges:** `feeds`, `answers`, `posts`, `served-by`, `uses`.
- **Health comes from reconciler conditions only** (`Ready`, `Served`, `Wired`).
  The console asserts no health of its own, so the graph cannot disagree with
  `kubectl`.
- **Traffic animates from recorded hops**, not from status transitions. Edge dash
  speed scales with event rate; error events mark the edge; an edge the manager
  enqueued but no adapter confirmed renders as **sent, unconfirmed** rather than
  as success — adapter reporting is optional, and an adapter that reports nothing
  must not look like one that delivered.
- An edge with no events is **visibly idle**, which is a different statement from
  absent.
- Unclaimed sources render **detached** with their `Wired=False` reason; dangling
  refs render as broken edges to placeholder nodes.

#### The Display panel

Per-class show/hide (sources, channels, adapters, profiles, runtimes, toolsets,
MCP configs, runtime pods, conversations), traffic animation on/off, idle
elements shown/hidden, edge labels (none / rate / latency), and the time window.
Selections persist across navigation and reload.

**Hiding is presentation only.** A hidden class still counts toward the graph's
health summary and the overview's problem rollup, and the panel says when hidden
classes contain failures. A filter that could conceal a broken component without
saying so is the one way this view could mislead, so the counts are computed
before filtering and never move because you hid something.

The capability layer (toolsets, MCP configs, runtime pods) starts folded away:
it is what you bring forward when debugging reach, not what you want between you
and the wiring on first look.

#### Time windows

Windows are bounded by the manager's in-memory activity buffer (~15 minutes under
load). Longer ranges come from a metrics backend when one is configured
(`console.metrics.url`), rendered as clearly labelled **aggregates** — rates,
percentiles, depths, no per-item identity. With none configured, long windows are
reported unavailable rather than drawn empty: an empty chart claims "there was no
traffic", which the console cannot honestly say about a window it never had data
for.

### Conversations

Server-side filtering (phase, pipeline, profile, channel, errored, search),
sorting by last activity, and pagination with a total match count — an event storm
makes thousands, and shipping them all so the browser can hide most is how a
viewer becomes an API-server problem. Run history is dropped from list rows; a
result is a whole agent message.

Detail is tabbed:

- **Transcript** with a composer. Conversations the console started are joined
  automatically. For ones another source started, the composer is live when the
  console channel holds a thread; when it does not, the tab explains why and shows
  the exact patch. The console never edits a Pipeline — showing the edit is the
  answer.
- **Runs** — `status.runs[]` with status, exit code, result, plus the bindings the
  conversation materialized and its runtime pod.
- **Graph** — every element this conversation involved, **built from what the
  Conversation recorded, not from the Pipeline's current spec**. A Conversation
  snapshots the bindings it materialized, so after a re-wire this graph still
  shows the capabilities that run actually had, and says the current wiring
  differs. Reading the live Pipeline would silently rewrite history, and the
  forensic value of this view is precisely that it does not.
- **Sequence** — a waterfall over the same hops, in time order, with per-hop
  latency. This is where "why did that take 40 seconds" gets answered, and it is
  the view a graph cannot replace.
- **YAML** — the Conversation object.

## Starting a conversation

**The console originates the way everything else does: by emitting a chat signal
from a claimed `SignalSource`.** Not through `POST /task`.

Four things fall out of that:

1. The origination invariant holds literally — no side door to defend.
2. **Who answers is declared, not chosen by the caller.** With `/task` the caller
   names the pipeline. With a claimed source, the wiring decides, which is the
   rule's actual point. The console cannot reach an agent no wiring points at.
3. **Origination becomes visible traffic.** A `/task` conversation materializes
   from nowhere, leaving a hole in the graph exactly where the operator acted. A
   console source is a node with an edge: pressing "start" lights up the graph the
   console is showing.
4. **Self-started conversations join themselves** — the claiming pipeline's
   channel set includes the console Channel, so the result already has a console
   thread. No pipeline edit, no copy-paste patch.

**It requires a claim.** The chart ships the `SignalSource` and **no Pipeline** —
wiring names a profile, sources and channels from different bundles, so only the
installer sees all of it. Until a Ready Pipeline claims the console source it
sits at `Wired=False`, the picker is unavailable, and the UI shows that reason
with the patch:

```sh
kubectl patch pipeline <name> --type=json \
  -p '[{"op":"add","path":"/spec/signalSourceRefs/-","value":{"name":"console"}}]'
```

A source is claimed by exactly ONE Pipeline, so one source means one destination.
To originate to several agents, declare several sources (`console-k8s`,
`console-ha`), each claimed by its own pipeline. This is a feature: *what you can
start is what is wired.* The picker is a rendering of the topology, not a
free-text field that can name something no wiring supports.

## Trust boundary

Requirements the console meets make it a control plane, not a viewer: it can
instruct an agent that, in a `rbacMode: full` install, holds `cluster-admin`.

- **The token is the boundary**, generated per install. Whoever holds it can read
  every CR the console's ServiceAccount can read — including conversation
  payloads: alert bodies, agent results — and can instruct any joined agent.
- **An unconfigured token authorizes nobody**, and is indistinguishable from a
  wrong one. "No token set" must never read as "no authentication required".
- **`ClusterIP`, no Ingress by default.** Reaching it means a port-forward or a
  deliberate decision.
- **OIDC is the answer for Ingress exposure**, via forward-auth from a proxy that
  has already authenticated the user (oauth2-proxy is the usual one). When a
  trusted identity header is present the console records it; when it is absent it
  records `token`.
- **Every write is logged with the resolved identity** — who started what, and who
  said what to an agent.
- **`console.write.enabled: false`** makes it a strict viewer: the composer and the
  new-conversation action disappear, and both endpoints refuse server-side. The UI
  hiding a button is presentation; the server refuses regardless.

What it **cannot** do: write anything to the Kubernetes API. Its Role carries no
write verb, and no write path exists in the module.

## RBAC

Namespaced, read-only, bound to the ServiceAccount the ChannelAdapter reconciler
creates (`agentops-adapter-console`). The operator grants adapters nothing — this
Role is the chart's grant.

| Group | Resources | Verbs |
|---|---|---|
| `agentops.dev` | all ten kinds | get, list, watch |
| `apps` | deployments | get, list, watch |
| (core) | pods | get, list, watch |

The pod/deployment grant is a deliberate widening past `agentops.dev`: image
digests, restart counts and pod failure reasons exist in no CR, and an operations
console that cannot see a CrashLoopBackOff is not one.

## Values

| Value | Default | Meaning |
|---|---|---|
| `console.enabled` | `true` | the whole component |
| `console.name` | `console` | ChannelAdapter/SignalAdapter CR name, and therefore the workload, SA and Service |
| `console.channelName` | `""` (= `name`) | Channel CR pipelines reference |
| `console.signalSourceName` | `""` (= `name`) | SignalSource it originates from |
| `console.write.enabled` | `true` | gates both write paths |
| `console.metrics.url` | `""` | optional Prometheus/VictoriaMetrics query endpoint for long windows |
| `console.image.*` | | image, bumped per release |
| `console.port` | `8080` | browser-facing port; the reconciler owns the Service |
| `console.auth.existingSecret` / `.uiToken` | `""` | supply or pin the browser token instead of generating one |
| `console.ingress.*` | disabled | see the trust boundary above before enabling |

## Building it

The frontend is React 18 + TypeScript + Vite + PatternFly 6 — Kiali's stack, so
Kiali familiarity is inherited rather than imitated. It is built in a Docker stage
and embedded into the Go binary with `go:embed all:ui/dist`, so the deployable
artifact is what every other adapter is: **one Go image serving one port**, with
nothing fetched at runtime.

npm exists at build time only, inside `console/` and its image. No other module,
and not the manager, gains a build step.

```sh
cd console
make ui          # build the SPA into ui/dist (what go:embed needs)
make build       # the binary, SPA embedded
make dev-build   # -tags dev: serves ui/dist (or $UI_DIR) from disk instead
make test        # Go tests + the frontend suite
```

For UI work, run `npm run dev` in `console/ui` (it proxies `/api` to
`localhost:8080`) against a console started with the `dev` tag or port-forwarded
from the cluster.
