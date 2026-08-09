# Contracts

How the out-of-process pieces talk to the manager: the two adapter contracts,
the runtime work contract, and the manager's own HTTP surface.

## The channel adapter contract

A channel adapter is a deployment that dials the manager (never the reverse) —
same pattern as runtimes, so NetworkPolicies stay simple and transport
credentials never leave the adapter:

1. Long-poll `GET /channel/ops?adapter=<your-adapter>&wait=25` for outbound
   operations: `ensure-topic` (create a thread for a conversation), `send`
   (post a message; chat HTML subset) and `close-topic` (archive the thread in
   `op.threadId` — its conversation has ended). Delivery is at-least-once —
   dedup by `op.id`.
2. Complete each op with `POST /channel/ops/{id}/done` — `{"threadId":"…"}`
   for `ensure-topic` (an opaque string in your id space), an **empty body**
   for `close-topic`, `{"error":"…"}` on failure (surfaced as a Conversation
   condition and regenerated).

   **`close-topic` is the exception to that last clause, in both halves.** Its
   conversation is being deleted, so a failure is *logged* rather than written
   as a condition — there will be no object left to carry one — and the op is
   never regenerated. An adapter that does not implement the kind may complete
   it with an error or ignore it entirely; the visible consequence is one open
   thread for a conversation that no longer exists, closable by hand. Deletion
   itself never waits longer than a 2-minute grace, so a down adapter cannot
   wedge it. Treat an already-closed thread as success: redelivery is normal.

   While a `close-topic` op is outstanding the deleting Conversation is held by
   the `agentops.dev/close-topics` finalizer, which is what keeps every op
   derivable from CR state across a manager restart.
3. Push user REPLIES with `POST /channel/inbound
   {"channel","threadId","text"}` — `threadId` is REQUIRED. This endpoint
   continues an existing conversation and never starts one; a message in a
   thread the manager does not know is dropped, not adopted. Relay to sibling
   channels and busy-acks happen manager-side in the shared router.
   To ORIGINATE, your transport's general surface belongs to a chat
   `SignalSource`: post `{"kind":"chat","fingerprint":…,"payload":…,"labels":
   {"agentops.dev/channel":…,"agentops.dev/sender":…}}` to `/signal/inbound`
   (see `signal-telegram/`). The Pipeline claiming that source decides who
   answers, and command parsing (`/agents`, `/<pipeline> <task>`) happens
   there.
4. Read your channels + opaque `spec.config` from `GET /channel/channels?adapter=`,
   persist cursors (e.g. poll offsets) via `GET/PUT /channel/state/{channel}/{key}`,
   report config problems via `POST /channel/channels/{name}/status`.

**Credentials** are declared per Channel (`spec.credentialsSecretRef`, a Secret
name) and projected into the adapter pod by the ChannelAdapter reconciler as
env vars — every key `K` of the Secret appears as `<credentialEnvPrefix>K`,
with the prefix advertised per channel in the `GET /channel/channels` listing
(e.g. key `botToken` → `AGENTOPS_CRED_HOME_OPS_botToken`). The kubelet resolves
the values; neither the manager nor any reconciler ever reads a Secret through
the API. Several Channels with different Secrets = several bots/workspaces
through one adapter process.

**Auth**: all `/channel/*` calls carry `Authorization: Bearer <token>`. A
ChannelAdapter-managed workload gets a per-adapter token derived from the
master key (`HMAC(ADAPTER_TOKEN, adapter name)`, validated statelessly by
re-derivation) and **scoped to its name** (the type key Channels select) —
cross-key calls get 403.
The bare master token (chart-provisioned into the manager as env) keeps full
scope, so hand-deployed adapters work unchanged. No Kubernetes API access
needed — the reference adapter [`channel-telegram/`](../channel-telegram/) is
dependency-free Go.

**Discovering what `config` needs.** An adapter CR may optionally declare
`spec.configSchema` (a JSON Schema for the `config` of the Channels/SignalSources
it serves) and `spec.credentialKeys` (the Secret keys it expects). Because the
declaration lives on the CR spec, `kubectl get channeladapter telegram -o yaml`
answers "what do I write?" before the adapter pod has ever started — no
registration step, and adapter binaries play no part. The reconciler
compile-checks a declared schema and reports `SchemaValid` on the adapter CR;
served Channels/SignalSources then carry `ConfigValid` (`SchemaValidated` /
`SchemaViolation` naming the offending fields).

Both are **advisory**: a violation never blocks serving, projection, or
ingestion — the adapter's own Ready report stays authoritative, because a
CR-declared schema can drift from the running image. Declaring nothing keeps
behavior exactly as before, and no `ConfigValid` appears. Authoring rule: bump
the schema in the same diff as `image`.

A `Channel` whose adapter nothing serves (no in-process provider, no Ready
`ChannelAdapter`, no adapter-reported readiness) carries a `Served=False`
condition — typos fail visibly instead of queueing ops forever.

**The operator delivers, always.** An agent's printed answer is its whole
deliverable: the runtime reports it via `/work/done` and the manager posts it
to every bound thread through the serving adapters. Agents never send chat
messages themselves, so no prompt carries transport instructions and no
runtime holds a channel's credentials — the surface is the adapter's business
alone. A conversation dispatches once at least one of
its topics exists (one broken channel never deadlocks it), and channel
implementations must never re-ingest their own outbound posts as inbound
(relayed messages would loop otherwise).


## The signal adapter contract

Signals are one-directional, so this is the channel contract minus the ops
queue — an adapter normalizes its transport into signals and the manager does
the rest (**adapters normalize, the manager groups**):

1. Read your sources + opaque `spec.config` from `GET /signal/sources?adapter=`
   (entries carry `credentialEnvPrefix` exactly like the channel listing).
2. Push normalized signals: `POST /signal/inbound {"source", "signals":
   [{"fingerprint", "labels", "title"?, "payload", "kind": "alert"|"job"}]}`.
   The manager applies the source's `grouping` policy: fingerprint cooldown
   (at-least-once delivery is safe — re-sends collapse), signature from
   `labels` × `signatureLabels`, window reuse, recurrence-on-session.
   `kind: job` takes the task-lane prompt instead of the read-only
   investigation lane.
3. Persist cursors via `GET/PUT /signal/state/{source}/{key}`, report config
   problems via `POST /signal/sources/{name}/status`.

Auth mirrors channels: master token or a per-`SignalAdapter` derived token
(distinct derivation context — channel and signal adapters sharing a name
never share a token), scoped to the adapter's name. A `SignalSource`
whose adapter nothing serves carries `Served=False`.

Reference implementation: [`signal-cron/`](../signal-cron/) — replaces the old
roadmap `cron` sub-struct: `config: {schedule, input, title?}` fires job-lane
signals with `<source>@<tick>` fingerprints (restart-safe via the state API);
the grouping window turns a recurring job into one conversation whose later
runs resume the agent session.


## The work contract

An `AgentRuntime` image must:

1. Long-poll `GET $CONTROL_URL/work?convo=$CONVO_ID&pod=$POD_NAME&wait=25`
2. Execute the returned unit — `promptText` (rendered) or `promptFile`+`promptVars`
   (relative to the checked-out repo at `/data/workspace`) with `resumeSessionId`
   when continuing — streaming progress to **stdout**
3. `POST $CONTROL_URL/work/done {convo, runId, status, sessionId, result}`
4. Exit `0` after `RUNTIME_IDLE_TTL_M` minutes without work

**`allowedTools` is the route's half of the allowlist, not the whole of it.**
The unit also carries `toolsMode` (`merge` | `overwrite`) and `agent`. A runtime
holding the repository is expected to read `.claude/agents/<agent>.md`, take its
`tools:` frontmatter as the agent's own declaration, and compose the two:
`merge` unions them (the agent's keeping position), `overwrite` passes
`allowedTools` alone. A runtime that cannot see a repository can use
`allowedTools` as-is — that is what `merge` degrades to.

**An empty allowlist means empty.** Substituting a tool nobody declared is a
grant the operator did not write down. `runtime-claude` passes the composed
list verbatim, even when it is empty, and runs with `--permission-mode dontAsk`
so an unlisted tool is denied outright: in a pod there is nobody to answer a
permission prompt, so prompting would hang the run until its idle TTL.

Reference implementation: [`runtime-claude/`](../runtime-claude/) (Node.js + claude-code, ~200 lines).
The same bring-your-own pattern applies to chat transports — see the channel
adapter contract above and [`channel-telegram/`](../channel-telegram/).


## The activity contract

Per-hop telemetry: one structured event for every movement the manager
mediates. It exists because nothing else records motion — CR status says a
conversation is `inflight`, never that the manager handed run `r-7` to a runtime
and got a result 4s later, so a graph derived from status animates what it
*infers* rather than what happened.

| Endpoint | Purpose |
|---|---|
| `GET /activity?since=<cursor>&limit=<n>` | bounded replay from the ring buffer |
| `GET /activity/stream` | SSE; each event carries its cursor |
| `POST /activity` | adapter-reported hops (delivery confirmation) |

All three use the adapter bearer scheme and accept **either** a channel- or a
signal-adapter derived token (or the master token) — the console holds both
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
  "adapter":      "telegram"
}
```

`from` and `to` name nodes the way the topology graph names them, so an event
renders directly as motion along an edge that already exists — no inference in
the consumer. Node kinds: `signal-adapter`, `signal-source`, `pipeline`,
`conversation`, `profile`, `runtime`, `channel`, `channel-adapter`, `toolset`,
`mcp-config`, `manager`.

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

Three properties are load-bearing:

- **Bounded and lossy by design.** A fixed-size in-memory ring (`ACTIVITY_BUFFER`,
  default 10000), evicting oldest-first, never persisted and never written to
  any Kubernetes object. The durable record stays `Conversation.status.runs[]`.
  Emission never blocks the operation it records: a full buffer drops, a slow
  subscriber is marked lagged rather than waited on.
- **A gap is always explicit.** A cursor older than the buffer's oldest answers
  `"resync": true` (SSE: an `event: resync` frame), and the client is expected
  to re-read snapshots. A silent short list would be indistinguishable from
  continuity.
- **Telemetry is not signal.** These events go to this log, never to
  `/signal/inbound`, and nothing converts one into the other. agent-ops' own
  health is STATUS, not SIGNAL — routing an error event about a broken runtime
  pod back through ingest is the loop `signal-k8s-events/selfexclude.go` exists
  to break, and keeping the surfaces apart makes it structural here.

**Attribution.** `pipeline` is present when it is knowable and EMPTY when it is
not: a Conversation records no `pipelineRef`, so attribution is inferred from
the bindings it materialized and is left blank when two pipelines wire
identically. Empty means "not attributable", never "none".

**Adapter-reported hops are optional.** An adapter that reports nothing still
appears on the graph through manager-side events; it simply never confirms
delivery, and the edge reads "sent, unconfirmed" rather than claiming success.
`POST /activity` takes `{kind, from?, to?, status?, conversation?, opId?,
latencyMs?, detail?}`; the reporting adapter is taken from the TOKEN, so an
adapter naming another in `adapter` is refused with 403.


## Manager introspection

**The manager exposes only what only the manager knows.** Anything that lives in
a CR is read from the API server, with the API server's own RBAC, and is never
proxied — a manager that mirrors CRs becomes a second Kubernetes API with a
second auth scope and its own staleness. Both endpoints below are read-only and
use the same bearer scheme as `/activity`.

`GET /status` — manager-internal state that exists in no Kubernetes object:

- build version, the leader-election lease holder;
- runtime slots in use against `MAX_ACTIVE_CONVERSATIONS`, counted from the live
  POD list (the same definition the admission gate uses, so the two cannot
  disagree), plus how many conversations are waiting for one;
- per-adapter op queue depth, split into **queued** (nothing is claiming) and
  **claimed but uncompleted** (an adapter is wedged mid-delivery) — two failure
  modes that look identical from outside — each with the id and age of the
  oldest;
- active cooldowns per source, because a suppressed lane looks exactly like an
  idle one on a graph.

`GET /pipelines/{name}/resolved` — the authoritative capability resolution:
`allowedTools` (the wiring's half, composed through the same function dispatch
uses), `toolsMode`, `toolsets`, `mcpConfigs`, `mcpServers` and the resolving
runtime, plus `unresolved` for refs that resolve to nothing. 404 for an unknown
pipeline; **an empty allowlist is reported as empty**, never as a fallback. A
consumer must not recompute this: a second implementation of composition would
eventually disagree with the one that runs.

The split between the two surfaces and `:9090/metrics` is deliberate — metrics
answer *how deep, how old, how many*; `/status` answers *which one*. Ids never
become metric labels (see below).


## Metrics

The same aggregates are exposed in the Prometheus exposition format on the
manager's existing `:9090/metrics`, registered into the controller-runtime
registry already serving that port. **Nothing new is listened on**, and alerting
therefore never depends on anyone having a browser open.

**One instrumentation pass.** Counters and histograms are driven by the activity
event stream (the metric set is an `activity.Observer`), so an event and its
metric observation cannot occur independently. Gauges are levels, sampled at
scrape time from the same in-memory state `/status` reports.

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

**The cardinality rule is binding.** Labels carry only values bounded by CR
count — `pipeline`, `adapter`, `source`, `channel`, `kind`, `status`, `reason` —
and are read from an event's structured fields (node names, `code`), never from
`detail`, which may carry a fingerprint or an error message. A conversation, run
or op id as a label would grow series without limit; those identify the specific
stuck item and stay in `/status`.


## HTTP API

| Endpoint | Purpose |
|---|---|
| `POST /task` `{"profile","task","agent"?,"channel"?}` | start a conversation programmatically |
| `GET /work`, `POST /work/done` | runtime-facing dispatch (see contract) |
| `GET/POST /channel/*` | adapter-facing channel contract (bearer token; see adapter contract) |
| `GET/POST/PUT /signal/*` | adapter-facing signal contract (bearer token; see signal adapter contract) |
| `GET/POST /activity*` | per-hop telemetry (bearer token; see activity contract) |
| `GET /status`, `GET /pipelines/{name}/resolved` | manager introspection (bearer token) |
| `GET /healthz` | liveness |
| `:9090/metrics` | controller-runtime metrics + the `agentops_*` set above |
