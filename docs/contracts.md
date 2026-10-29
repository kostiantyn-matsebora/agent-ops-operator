# Contracts

How the out-of-process pieces talk to the manager: the two adapter contracts,
the runtime work contract, and the manager's own HTTP surface.

## The channel adapter contract

See [`docs/contracts/channel-adapter.md`](contracts/channel-adapter.md).

## The signal adapter contract

See [`docs/contracts/signal-adapter.md`](contracts/signal-adapter.md).

## The work contract

See [`docs/contracts/runtime.md`](contracts/runtime.md).

## The aops MCP server contract

**Shipped as `platform/mcp-aops/`**, rendered only under the chart's
`coordination.enabled` (off by default) — see
[ADR 0002](adr/0002-coordinated-agents.md) for the design this contract
carries.

The interface through which a coordinating agent sees and acts on agent-ops
itself. `platform/mcp-aops/` is a thin client of the manager — MCP over
streamable HTTP, no state of its own, deciding nothing about reach.

```
runtime pod  --MCP, token forwarded verbatim-->  mcp-aops
mcp-aops     --HTTP, same token as Bearer------->  manager /coordinate/*
```

**The server forwards a forged request exactly as it forwards a legitimate
one.** It holds no allowlist and makes no decision either way. The manager
validates the token, resolves the calling conversation and enforces every
bound.

This is deliberate. A server holding the manager's own adapter token and
enforcing reach itself was considered and rejected — that puts a credential
stronger than any caller's inside a component every coordinator can reach.

**It holds no credential of its own.** As shipped, the server authenticates no
call to the manager beyond forwarding whatever the caller presented.

There is nothing here for a stolen server process to abuse beyond what a
caller's own token already reaches. What bounds who can reach the SERVER at
all is the network wall below, never a secret it holds.

### Authentication and reach

| Token context | Injected into | Reaches |
|---|---|---|
| `coordinator:<name>:<conversation>` | a Coordinator's own conversation (root OR a plain member, as `AOPS_MCP_TOKEN`) | the coordinator class, scoped to that ONE CONVERSATION'S OWN SUBTREE, PLUS — when the conversation resolves to a Coordinator — the Coordinator-owner class |
| `channel-reader:<channel>` | a reader with no coordination role | `list_conversations` / `get_conversation` only, projected, no verb |

**The token is per conversation, never per Coordinator.** One Coordinator may
hold several open conversations at once, nested or not, and a token naming
only the Coordinator could not scope to one of them.

**The reach classes are THREE**, each decided by the manager from the
token's context, never by the server: the coordinator class (`invoke`,
`escalate`, `read`, the narrow `close`), the Coordinator-owner class
(`list_open_roots`, the widened `close`), and the channel-reader class.

**A nested caller's reach is its own subtree, never the tree's ultimate
root.** A Coordinator's conversation three levels deep sees its own members
and their descendants, and nothing above it — not its parent, and not a
sibling branch.

**The Coordinator-owner class is resolved by WALKING, not by a field on the
caller.** The manager reads the caller's own `coordinatorRef` first.

- **Empty?** It walks `causedBy` to the UNCAUSED root and reads THAT root's
  `coordinatorRef` instead — the same walk the cycle guard performs.
- **This is what lets a plain member reach it.** One with no `coordinatorRef`
  of its own — the self-heal reaper's own shape — still reaches
  `list_open_roots` and the widened `close`.
- **`invoke` resolves the caller's Coordinator the SAME walk.** A plain
  member may `invoke` a sibling entry of the Coordinator it resolves to,
  never only the coordinating root — the reaper calling `invoke` on the
  domain capability it is re-checking is this walk's own reason to exist.

| Verb | Bound |
|---|---|
| `invoke` | the `agents[]` list of the Coordinator the caller RESOLVES to (its own `coordinatorRef`, or failing that, its uncaused root's) |
| `escalate` | the caller itself — never a member reached through it |
| `read` | the calling conversation's own subtree, at any depth — never the tree's ultimate root when the caller is nested |
| `close` | the caller itself, a conversation it directly caused — OR, when the caller RESOLVES to a Coordinator, any open UNCAUSED root of that SAME Coordinator other than the caller's own ancestor root. Never a member of any kind, and never a different Coordinator's root |
| `list_open_roots` | the calling conversation's own Coordinator's open UNCAUSED roots only, excluding the caller's own root |
| `list_conversations`, `get_conversation` | for a `channel-reader:<channel>` token, the projection of that Channel's conversations and no verb |

A refusal reaches the caller as an error naming the bound that refused it.

| Refused | By |
|---|---|
| Invoking an AgentCapability outside the caller's resolved Coordinator's `agents[]` list | manager |
| `invoke`, `close` or `list_open_roots` for a caller that resolves to no Coordinator at all — an ordinary Pipeline-addressed conversation | manager |
| Acting on a conversation outside the caller's own subtree | manager |
| `close` past one hop — a directly caused member only, never a deeper descendant reached through it, even within the caller's own subtree | manager |
| `close` or `list_open_roots` reaching a member, however shallow, even one within the caller's own Coordinator's tree | manager |
| `close` or `list_open_roots` reaching a different Coordinator's root | manager |
| `close` reaching the caller's own ancestor root through the widened bound | manager |
| Any verb, from a `channel-reader` token | manager |
| An `invoke` that would repeat a Coordinator already in the caller's ancestor chain | manager (cycle guard, below) |

An allowlist inside the runtime pod is never relied on for any of these.

### The tool list

| Tool | Kind | Returns |
|---|---|---|
| `list_agents()` | read | the caller's Coordinator's `agents[]` entries — name and description only |
| `list_conversations()` | read | conversations in the caller's scope, each with `brief` |
| `get_conversation(name)` | read | one conversation's detail, in scope |
| `get_tree()` | read | the caller's own subtree, walked from itself downward |
| `invoke(agent, task)` | move | the member's name at once — `created` or `attached` |
| `close(conversation, reason)` | move | acknowledgement |
| `escalate(message)` | move | acknowledgement |
| `read(conversation)` | read | a conversation's own record, in scope |
| `list_open_roots()` | read | the caller's own Coordinator's OTHER open, uncaused roots — name, title, brief, phase, plus `members` (each root's direct `agents[]` entry names) |

**A move ends a coordinating turn, a read does not.** Every tool description
states which it is.

- An accepted move marks the caller's inflight run. A refused verb marks
  nothing.
- A root run that made no move and answered no person is handed back once,
  then escalated.
- See [Every root turn ends in a
  move](concepts.md#every-root-turn-ends-in-a-move).

**All nine complete within the request.** None waits on another agent's
work — a `list_conversations` call never blocks on a member still running.

`list_open_roots` is the Coordinator-owner reach class's own verb. It is
refused outright for a caller that resolves to no Coordinator, and it never
returns a transcript or a run — the same bounded shape `list_conversations`
already returns, plus `members`.

### Read tools

The five list and get tools — `list_agents`, `list_conversations`,
`get_conversation`, `get_tree` and `list_open_roots` — cover Conversations and the caller's own
Coordinator's `agents[]` entries, filtered to what the calling Coordinator
lists and what it caused.

`list_open_roots` is the exception to "what it caused". It lists the
Coordinator's other open, uncaused roots, for a caller that resolves to a
Coordinator.

Pipelines, AgentCapabilities, Coordinators, SignalSources and Channels have no
reader here.

`list_conversations` carries `brief` beside name, title, phase and pipeline,
so a caller deciding WHICH conversation it means never has to `read` one
first:

```json
{
  "conversations": [
    {
      "name": "agentops-conv-a1b2c3",
      "title": "ingress 502s in web",
      "brief": "Checking the web namespace ingress and its Service after 502s were reported.",
      "phase": "Active",
      "pipeline": "incident-coordinator"
    }
  ]
}
```

`get_tree` walks from the CALLING conversation downward, one hop at a time —
never toward the tree's ultimate root when the caller is itself nested:

```json
{
  "name": "agentops-conv-sub-1",
  "brief": "Diagnosing the Service side of the ingress incident.",
  "members": [
    {"name": "agentops-conv-sub-1-a", "entry": "endpoint-check", "phase": "Closed", "members": []}
  ]
}
```

The parent that invoked this Coordinator, and any sibling branch, sit outside
this response — refused if named directly, not merely omitted.

### The verbs, all asynchronous

Every verb returns without waiting on any agent's work. A result, when there
is one, arrives later as an ordinary input.

**`invoke(agent, task)`.** Starts, or attaches to, a member conversation
running the named `agents[]` entry with `task` as its first input.

Reports which happened — a live member carrying the same `conversationId` is
ATTACHED to, never duplicated. Refused when the target is outside
`agents[]`, when the caller's own `maxAgents` is exhausted, or when it would
cycle the Coordinator graph (below).

**`close(conversation, reason)`.** Ends the caller itself, or a conversation
it directly caused, stamping `reason` as `closeReason`. Required from a
coordinator — refused with none. Refused outright for a deeper descendant
reached through an intermediate member.

A caller that RESOLVES to a Coordinator (above) may also close any OTHER
open, uncaused root of that SAME Coordinator — never its own ancestor root,
never a member, and never a different Coordinator's root. This is the
Coordinator-owner class's own widening of this verb.

**`list_open_roots()`.** The Coordinator-owner class's own verb. Lists the
caller's own Coordinator's other open, uncaused roots, each with its direct
members' `agents[]` entry names. Refused for a caller that resolves to no
Coordinator.

**`escalate(message)`, on the tree's UNCAUSED conversation** (no
`causedBy`): binds the Coordinator's escalation channels, snapshotted at
creation, and opens a thread on each with `message` as the first post.

**`escalate(message)`, on a NESTED conversation** (one carrying `causedBy`):
opens no thread. Closes that conversation with `message` as its
`closeReason` and its result, landing on its own parent as an ordinary
member-result input.

The parent's agent then decides whether to handle it or call `escalate`
again — bubbling one hop at a time until a call reaches the uncaused root.

**`read(conversation)`.** Scoped exactly as the read tools are.

### The cycle guard

Nesting carries no depth limit — a tree runs as deep as each level's own
budget allows. A CYCLE is a different failure from depth: Coordinator A
invoking B invoking A never terminates, however small each step.

On every `invoke`, the manager collects the calling conversation's OWN
`coordinatorRef`, then walks its `causedBy` chain to the uncaused root
collecting each ancestor's. A target that resolves to (or is wired as) a
Coordinator already in that list is refused, naming the repeated Coordinator:

```json
{"error": "cycle: coordinator \"A\" already appears in this conversation's ancestor chain"}
```

**A `maxDepth` field was considered and rejected.** It bounds a symptom — a
long chain — rather than the actual failure, a chain that repeats, and would
refuse a legitimately deep but acyclic tree an operator asked for.

### The channel-reader projection

A second reach class exists for a caller that is not a coordinator at all —
first user: the voice lane's analyzer, which must pick the conversation a
spoken reply belongs to without reading any conversation's transcript.

```jsonc
// token context: channel-reader:voice-desk
{
  "conversations": [
    {
      "name": "agentops-conv-x9",
      "title": "kitchen light schedule",
      "brief": "Adjusting the evening lighting automation.",
      "phase": "Active",
      "pipeline": "home-assistant"
    }
  ]
}
// no run, no input, no tree -- only conversations with a thread on "voice-desk"
```

Every verb is refused for this token. The bound is the CHANNEL, because "a
conversation you could mean" is one that has a thread where you are
speaking — the same bound the channel-close remote verb uses for who may
end a conversation.

The server learns nothing about channels. It forwards this token exactly as
it forwards a coordinator's, and the manager alone decides the projection.

### Every token derivation context, catalogued

One HMAC family — `HMAC-SHA256(ADAPTER_TOKEN, context)`, base64url — one
context string per surface, validated by RE-DERIVATION rather than storage.
Nothing is minted or persisted anywhere.

| Context | Identifies |
|---|---|
| `adapter:<name>` | a `ChannelAdapter`'s own `/channel/*` token |
| `signal-adapter:<name>` | a `SignalAdapter`'s own `/signal/*` token |
| `coordinator:<name>:<conversation>` | one coordinated conversation's own `/coordinate/*` token |
| `channel-reader:<channel>` | the channel-reader projection token, above |

A distinct context per surface is what keeps a `ChannelAdapter` and a
`SignalAdapter` sharing a name — or a `Channel` and a `Coordinator` sharing
one — from ever sharing a token.

### Placement

| | |
|---|---|
| Reachable from | runtime pods only, under the [ADR 0001](adr/0001-bound-component-reach.md) network wall — the manager never calls it, since the server is the manager's caller |
| Holds | no credential of its own and no Secret reads |
| Bound to a Coordinator via | `MCPConfig`, rendered by the chart, exposed through `global.builtinToolsets.agentops-coordinate` |
| Component path | `platform/mcp-aops/` — standard-library Go, the shared Dockerfile recipe |

## The activity contract

**Per-hop telemetry: one structured event for every movement the manager
mediates.**

It exists because nothing else records motion. CR status says a conversation is
`inflight`, never that the manager handed run `r-7` to a runtime and got a
result 4s later, so a graph derived from status animates what it *infers* rather
than what happened.

| Endpoint | Purpose |
|---|---|
| `GET /activity?since=<cursor>&limit=<n>` | bounded replay from the ring buffer |
| `GET /activity/stream` | SSE, each event carrying its cursor |
| `POST /activity` | adapter-reported hops (delivery confirmation) |

**All three use the adapter bearer scheme** and accept **either** a channel- or
a signal-adapter derived token, or the master token. The console holds both
identities and would otherwise have to pick one arbitrarily.

Event shape:

```json
{
  "cursor":       "0000000000012345",
  "ts":           "2026-08-09T11:04:22.117Z",
  "kind":         "run.completed",
  "from":         {"kind": "runtime",  "name": "default"},
  "to":           {"kind": "pipeline", "name": "k8s-ops"},
  "status":       "ok",
  "conversation": "chat-abc12",
  "pipeline":     "k8s-ops",
  "runId":        "r-7",
  "opId":         "send:9",
  "inputId":      "in-mfx1",
  "latencyMs":    4218,
  "code":         "succeeded",
  "detail":       "succeeded (exit 0)",
  "data":         {"model": "claude-sonnet-5", "tokensIn": "18422"},
  "adapter":      "telegram"
}
```

**`from` and `to` name nodes the way the topology graph names them**, so an
event renders directly as motion along an edge that already exists. No inference
in the consumer.

Node kinds: `signal-adapter`, `signal-source`, `pipeline`, `conversation`,
`profile`, `runtime`, `channel`, `channel-adapter`, `toolset`, `mcp-config`,
`manager`, plus four that name what the runtime reaches:

| Node kind | Named by |
|---|---|
| `runtime-image` | the image the conversation's pod runs, resolved as the pod builder resolves it. It is the `from` of every call the runtime reports, because the harness inside the image made the call |
| `model` | the model, as the runtime reported it |
| `mcp-server` | the server's key in the bound MCPConfigs |
| `external` | a system outside the install, as an adapter CR's `spec.externals[]` names it |

**A runtime that no longer resolves to an image** makes its calls leave from
its `runtime` node instead. A hop from a coarser node is honest, and one from
an image nobody ran is not.

| Kind | From → To | Emitted when |
|---|---|---|
| `signal.received` | signal-adapter → signal-source | `POST /signal/inbound` accepted |
| `signal.claimed` | signal-source → pipeline | a Ready Pipeline claims the source |
| `signal.dropped` | signal-source → ∅ | unclaimed source, or the pending backlog is full |
| `conversation.created` | pipeline → conversation | Conversation CR created |
| `input.queued` | source/channel → conversation | an input was appended |
| `run.dispatched` | pipeline → runtime | `GET /work` handed a unit out |
| `run.completed` | runtime → pipeline | `POST /work/done` |
| `channel.op.enqueued` | conversation → channel | `ensure-topic` / `send` / `close-topic` queued |
| `channel.op.completed` | channel-adapter → manager | op acked, or failed with a reason |
| `channel.inbound` | channel-adapter → conversation | a user reply entered the router |
| `context.restored` | runtime → conversation | durable context copied into a starting pod |
| `context.checkpoint` | conversation → runtime | live context copied to the volume |
| `context.skipped` | conversation → runtime | a checkpoint ran and found nothing changed |
| `context.failed` | conversation → runtime | a restore or checkpoint failed |
| `model.call` | runtime-image → model | one per `turns[]` entry a runtime reported with its result |
| `tool.call` | runtime-image → mcp-server, or ∅ for a built-in tool | one per `toolCalls[]` entry. `latencyMs` is the call's `durationMs` |

**Both call hops carry the run's `conversation`, `pipeline` and `runId`**, and
`detail` names the model or the tool. A turn with no `model` records no `to`.

**`data` is a bounded map of facts that exist nowhere else**, keys and values
both strings. It is never content — an input's text, a run's result and an
op's message have durable homes and are joined from there.

| Hop | `data` keys, each present only when known |
|---|---|
| `model.call` | `model`, `tokensIn`, `tokensOut`, `cacheReadTokens`, `cacheCreationTokens`, `stopReason` |
| `tool.call` | `tool`, `server`, `resultBytes` |
| `context.checkpoint`, `context.skipped`, `context.restored` | `bytes`, `files`, `quiesced` |

- **At most 16 keys**, and a value or key at most 200 bytes.
- **Over the bound it is TRUNCATED, never refused.** Telemetry never fails the
  request that produced it. Keys past the sixteenth are dropped in sorted
  order, so one map always keeps the same keys, and a long value is cut at a
  UTF-8 boundary.
- **`POST /activity` carries no `data`.** Only the manager writes it.

**Three properties are load-bearing:**

- **Bounded and lossy by design.** A fixed-size in-memory ring
  (`ACTIVITY_BUFFER`, default 10000), evicting oldest-first, never persisted and
  never written to any Kubernetes object. The durable record stays
  `Conversation.status.runs[]`. Emission never blocks the operation it records:
  a full buffer drops, and a slow subscriber is marked lagged rather than waited
  on.
- **A gap is always explicit.** A cursor older than the buffer's oldest answers
  `"resync": true` (SSE: an `event: resync` frame), and the client is expected
  to re-read snapshots. A silent short list would be indistinguishable from
  continuity.
- **Telemetry is not signal.** These events go to this log, never to
  `/signal/inbound`, and nothing converts one into the other. agent-ops' own
  health is STATUS, not SIGNAL — routing an error event about a broken runtime
  pod back through ingest is the loop `signals/k8s-events/selfexclude.go` exists
  to break, and keeping the surfaces apart makes it structural here.

**Attribution.** `pipeline` is present when it is knowable and EMPTY when it is
not.

A Conversation records no `pipelineRef`, so attribution is inferred from the
bindings it materialized and is left blank when two pipelines wire identically.
Empty means "not attributable", never "none".

**Adapter-reported hops are optional.** An adapter that reports nothing still
appears on the graph through manager-side events. It simply never confirms
delivery, and the edge reads "sent, unconfirmed" rather than claiming success.

`POST /activity` takes:

```json
{"kind":"…","from":{},"to":{},"status":"…","conversation":"…","opId":"…",
 "latencyMs":0,"detail":"…"}
```

Every field but `kind` is optional. The reporting adapter is taken from the
TOKEN, so an adapter naming another in `adapter` is refused with 403.

## Manager introspection

**The manager exposes only what only the manager knows.**

Anything that lives in a CR is read from the API server, with the API server's
own RBAC, and is never proxied. A manager that mirrors CRs becomes a second
Kubernetes API with a second auth scope and its own staleness.

Both endpoints below are read-only and use the same bearer scheme as
`/activity`.

**`GET /status`** — manager-internal state that exists in no Kubernetes object:

- **Build version** and the leader-election lease holder.
- **Runtime slots in use** against `MAX_ACTIVE_CONVERSATIONS`, counted from the
  live POD list — the same definition the admission gate uses, so the two cannot
  disagree — plus how many conversations are waiting for one.
- **Per-adapter op queue depth**, split into **queued** (nothing is claiming)
  and **claimed but uncompleted** (an adapter is wedged mid-delivery). Those are
  two failure modes that look identical from outside, and each carries the id
  and age of the oldest.
- **Active cooldowns per source**, because a suppressed lane looks exactly like
  an idle one on a graph.

**`GET /pipelines/{name}/resolved`** — the authoritative capability resolution:

- `allowedTools` — the wiring's half, composed through the same function
  dispatch uses.
- `toolsMode`, `toolsets`, `mcpConfigs`, `mcpServers`, and the resolving
  runtime.
- `unresolved` for refs that resolve to nothing.

404 for an unknown pipeline. **An empty allowlist is reported as empty**, never
as a fallback.

**A consumer must not recompute this.** A second implementation of composition
would eventually disagree with the one that runs.

**The split between the two surfaces and `:9090/metrics` is deliberate.**
Metrics answer *how deep, how old, how many*. `/status` answers *which one*. Ids
never become metric labels — see below.

## Metrics

The same aggregates are exposed in the Prometheus exposition format on the
manager's existing `:9090/metrics`, registered into the controller-runtime
registry already serving that port.

**Nothing new is listened on**, so alerting never depends on anyone having a
browser open.

**One instrumentation pass.** Counters and histograms are driven by the activity
event stream — the metric set is an `activity.Observer` — so an event and its
metric observation cannot occur independently. Gauges are levels, sampled at
scrape time from the same in-memory state `/status` reports.

| Metric | Type | Labels |
|---|---|---|
| `agentops_signals_received_total` | counter | `source`, `adapter`, `status` |
| `agentops_signals_dropped_total` | counter | `source`, `reason` |
| `agentops_conversations_created_total` | counter | `pipeline` |
| `agentops_runs_total` | counter | `pipeline`, `status` |
| `agentops_run_duration_seconds` | histogram | `pipeline` |
| `agentops_model_calls_total` | counter | `pipeline`, `status` |
| `agentops_model_call_tokens` | histogram | `pipeline`, `direction` (`in`, `out`, `cache-read`) |
| `agentops_tool_calls_total` | counter | `pipeline`, `target` (`builtin`, `mcp`), `status` |
| `agentops_tool_call_duration_seconds` | histogram | `pipeline`, `target` |
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

**The cardinality rule is binding.**

- **Labels carry only values bounded by CR count** — `pipeline`, `adapter`,
  `source`, `channel`, `kind`, `status`, `reason`.
- **They are read from an event's structured fields** (node names, `code`),
  never from `detail`, which may carry an `initiationId` or an error message.
- **A conversation, run or op id as a label would grow series without limit.**
  Those identify the specific stuck item and stay in `/status`.
- **Nor is a model or a tool name a label.** Both are reported by a runtime
  rather than declared by a CR, so neither is bounded by CR count. They stay in
  the events, and the call metrics key on `target` instead.

## The control-plane contract

**One named, additive HTTP surface for operational configuration** —
`/control-plane/*`, a package and route prefix inside the manager, not a new
component: same process, same container, same ServiceAccount.

Guarded by its own dedicated bearer token, `CONTROL_PLANE_TOKEN`.

- **A single flat shared secret**, checked constant-time, the same shape
  `ADAPTER_TOKEN` and the coordinator token already use.
- **A distinct credential from both.** A caller holding one must not reach
  the other's surface.
- **An unconfigured token refuses every request with 503**, never falling
  open — the same rule `/channel/*` and `/signal/*` follow.

| Route | Guards |
|---|---|
| `PUT /control-plane/probes/{name}` | create-or-update the named `ControlPlaneProbe` by name |
| `GET /control-plane/probes/{name}` | read it back, 404 on an unknown name |

A request for a kind this surface does not yet handle is 404 and touches no
object — never a partial create, read or modify of a different kind.

`ControlPlaneProbe` (`docs/concepts.md`) is the disposable placeholder this
surface was proven against.

| Role | On a kind behind `/control-plane/*` |
|---|---|
| The manager's own Role | full CRUD |
| Any other component's Role | read-only, or nothing |

This pattern applies to whatever resident lands here next.

## HTTP API

| Endpoint | Purpose |
|---|---|
| `GET /work`, `POST /work/done` | runtime-facing dispatch (see contract) |
| `POST /work/context` | context-sync sidecar reports one restore or checkpoint |
| `GET/POST /channel/*` | adapter-facing channel contract (bearer token, see adapter contract) |
| `GET/POST/PUT /signal/*` | adapter-facing signal contract (bearer token, see signal adapter contract) |
| `GET/POST /activity*` | per-hop telemetry (bearer token, see activity contract) |
| `POST /coordinate/*` | the aops MCP server's verbs and read tools (per-conversation or channel-reader bearer token, see aops MCP server contract) |
| `PUT/GET /control-plane/*` | operational-configuration surface (dedicated bearer token, see control-plane contract) |
| `GET /status`, `GET /pipelines/{name}/resolved` | manager introspection (bearer token) |
| `GET /healthz` | liveness |
| `:9090/metrics` | controller-runtime metrics + the `agentops_*` set above |

**There is no programmatic origination endpoint.** To start a conversation from
a script, post a `kind: task` signal to a `SignalSource` a Ready Pipeline
claims:

```sh
curl -s -X POST http://agentops-manager.<ns>.svc:8080/signal/inbound \
  -H "Authorization: Bearer $ADAPTER_TOKEN" -H 'Content-Type: application/json' \
  -d '{"source":"<source>","signals":[{"initiationId":"ci-'"$BUILD_ID"'",
       "kind":"task","payload":"why is the api pod crashlooping?"}]}'
```

**The manager publishes, then consumes.** Every admitted signal becomes its
own `Signal` CR before the response is written, and the response
(`{"queued": n}`) acknowledges that storage, never routing. A separate
consumer on the leader then decides the claimants, cooldown and capacity and
routes it. A `5xx` means nothing of the failing signal was stored: re-send
with the same ids. An optional per-message `signalId` deduplicates the
re-send.

The `conversations` field is withdrawn, and every routing outcome lives on
the `Signal`. See [Concepts](concepts.md#signal) for the kind and
[the adapter specification](contracts/signal-adapter.md), version 2.3.0, for
the contract.
