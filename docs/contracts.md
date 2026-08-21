# Contracts

How the out-of-process pieces talk to the manager: the two adapter contracts,
the runtime work contract, and the manager's own HTTP surface.

## The channel adapter contract

A channel adapter is a deployment that dials the manager (never the reverse) —
same pattern as runtimes, so NetworkPolicies stay simple and transport
credentials never leave the adapter:

1. Long-poll `GET /channel/ops?adapter=<your-adapter>&contract=2&wait=25` for
   outbound operations: `ensure-topic` (create a thread for a conversation),
   `send` (post a message) and `close-topic` (archive the thread in
   `op.threadId` — its conversation has ended). Delivery is at-least-once —
   dedup by `op.id`.

   **`contract` is required**, and an absent or outdated version is refused with
   400 naming the current one. Version 1 carried rendered `text`; an adapter
   still reading that field would post empty messages and look healthy doing it,
   so the handshake fails at the door rather than downstream.
   A claimed op also carries **`reclaimAfterSeconds`** — how long the manager
   leaves the claim with you before returning the op to the queue. Additive and
   optional: ignore it and nothing changes. Honour it if you absorb transport
   backpressure in-process (below), because finishing after it expires means a
   second claimant posts the same message again.
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
5. Optionally act on a conversation your channel is bound to:
   `POST /channel/conversations/{name}/reopen`,
   `POST /channel/conversations/{name}/delete` and
   `POST /channel/conversations/{name}/reset-context`, all `{"channel":"…"}`.
6. Optionally report how far your threads have been **read**:
   `POST /channel/read` ([below](#post-channelread)).

### POST /channel/read

**Optional.** An adapter that never calls it stays fully conformant; its threads
simply carry no watermark, which is inert for every surface that does not render
unreadness.

```jsonc
POST /channel/read
{"channel":"console",
 "reads":[{"threadId":"console-uid-abc","readAt":"2026-08-13T10:12:04Z",
           "reader":"sha256:9f2a…"}]}

200
{"results":[{"threadId":"console-uid-abc","outcome":"marked"}],
 "marked":1,"skipped":0,"failed":0}
```

The manager resolves each `threadId` to its conversation and writes the
watermark to `status.threads[].readAt`. **The manager owns the write** — the
adapter reports and issues no Kubernetes write of its own, exactly as with
`POST /channel/channels/{name}/status`.

`threadId` rather than a conversation name on purpose: an adapter knows thread
ids, `/channel/inbound` already addresses a thread, and the mapping from one to
the other is the manager's to own. Naming conversations here would hand adapters
a Kubernetes identifier they have no other reason to hold.

`reader` is OPTIONAL and OPAQUE: the adapter's own key for whoever read the
thread, and the manager records that reader's watermark rather than the
channel-wide one. Omit it and the behaviour is exactly what it was before the
field existed — which is what every adapter but the console does, since a
Telegram topic is read or it is not and there is nobody to attribute it to.

**Never send an identity in it.** The manager stores the value verbatim and
cannot tell a hash from an address, so a conversation would end up recording who
read it. Compute a SALTED hash — an unsalted digest of a known address is
confirmable by anyone holding the CR. A key containing `@` is refused with 400.

| Rule | Behaviour |
|---|---|
| Auth and scope | The same adapter token and channel scope as every other `/channel/*` route — 401 unauthenticated, 403 for a channel served by another adapter |
| Bound | At most **50** entries, enforced by the manager; more is 400 and nothing is written. An empty list is 400 |
| Monotonic | A watermark at or before the stored one is `skipped` — no write, no error. Per READER when one is named, so one reader is never skipped because another is ahead |
| Bounded readers | At most 50 readers per binding, oldest watermark evicted first; an evicted reader falls back to the channel-wide mark |
| Clamped | A watermark ahead of the manager's clock is written as its `now` |
| Per entry | One `marked` / `skipped` / `failed` outcome per requested thread, with a reason for anything not marked, plus totals |
| Mixed batch | Still 200, and one bad entry never stops the rest — an unknown thread is `failed` and its neighbours are still marked |

Entries resolving to the same conversation are grouped into ONE status patch,
and a report that would not advance the watermark writes nothing at all — so
re-opening a quiet conversation costs no API write. See
[Read state](concepts.md#read-state-per-thread) for the field and the backfill
rule.

**Reach for those two verbs is the BINDING**, read from the conversation's
`spec.channelRefs` and never taken from the request: a surface may act on a
conversation whose bindings name its channel, and no other. That is narrower
than it sounds, and it is the amendment the archived-thread case forces on "no
remote close verb exists". That rule protected a property rather than a syntax —
*you may only end a conversation you are part of* — and holding a live thread
was how membership was PROVEN, because posting `/close` on a thread is only
possible for a surface that has one. A closed conversation has no thread, so
that proof is unavailable and the binding that put the thread there is the
next-strongest.

`delete` additionally refuses anything not already `Closed`, with a reason
naming the missing step. It never closes first: one call doing the irreversible
thing to a conversation that was still working, behind a confirmation that named
only the delete, is exactly what the refusal prevents. Closing itself is still
`/close` on a thread — there is no close endpoint.

### Backpressure is yours to absorb

A transport that says "slow down" — HTTP 429, a `retry_after`, a token-bucket
rejection — is **not** an operation failure. Absorb it inside your claim window:
wait the interval the transport stated (never a backoff you computed instead of
one it gave you) and retry the same call. Report the op failed only when the
condition is terminal, or when riding it out would overrun the claim.

The manager's recovery path is for operations that genuinely could not be
performed. Reporting a retryable condition as a failure makes that path
load-bearing for something you could have waited out, and every re-derivation
arrives into the same backpressure.

Budget your total in-process wait **strictly below** `reclaimAfterSeconds`, and
pace your own sending so bursts are spread rather than rejected. Pace before you
CLAIM, not before you send: work you cannot yet deliver should stay queued in
the manager, where it is still derivable from CR state and survives your restart.

**Commands the manager answers itself**, before any Pipeline lookup — an adapter
forwards them as ordinary text and does not implement them:

| Command | Where | What it does |
|---|---|---|
| `/agents`, `/help`, `/start` | general surface | lists Ready pipelines and their profiles |
| `/<pipeline> <task>` | general surface | originates a conversation ([above](#the-channel-adapter-contract)) |
| `/close` | a conversation's thread | ends the conversation, archives the thread |
| `/exit` | a conversation's thread | releases the runtime pod, keeps the conversation |

Both thread commands are intercepted on the REPLY path, before the text could
become an input, and both answer with usage when typed on a general surface where
there is no conversation to act on. `/exit` refuses while a run is inflight or
input is queued; see [concepts](concepts.md#releasing-a-runtime-by-hand--exit).
A Pipeline whose name collides with any of these is not reachable by that
command.

### The manager composes meaning; the adapter composes presentation

An op carries STRUCTURE, never rendered text. There is no `op.text` and no
`op.title`.

`send` carries `op.message`, one of four kinds:

| kind | fields | what it is |
|---|---|---|
| `signal` | `pipeline?`, `source`, `title`, `labels{}`, `body`, `inputRef` | the event that opened or advanced the conversation, as it arrived |
| `answer` | `body`, `status` | agent output, reported through `/work/done` |
| `relay` | `origin`, `sender?`, `body` | a user message from a sibling channel |
| `notice` | `level` (`info`/`warn`), `body` | the manager on its own behalf: acks, listings, refusals |

`ensure-topic` carries `op.topic`, a descriptor — `conversation`, `pipeline?`,
`source?`, `title`, `labels{}`, `kind` — and YOU name the thread from it. The
manager sends no baked title because it cannot know your limits: Telegram forum
topics cap at 128 characters and take no markup; a web chat has neither
constraint. The descriptor and the first card in the thread carry the same
facts, so an adapter can choose not to repeat itself.

`pipeline` may be EMPTY on both. It is inferred from the conversation's
materialized bindings and left blank when several Ready Pipelines could claim
it — blank means "not determinable", so render it as absent rather than
inventing a name.

`delete-conversation` carries the target thread id and a `notice`, and reports
that the conversation is **gone for good** — not that a thread should be closed.
What ending means for your transport is YOUR decision: post the tombstone,
archive the thread, rename it, delete it. It is named for the conversation
because that is what ended; `ensure-topic` and `close-topic` instruct you about
a thread, this one informs you about a lifecycle event.

It REPLACES `close-topic` on the deletion path — a conversation being deleted
gets one or the other, never both — so you never have to work out whether a pair
means one ending or two. Do not silently ignore it: an adapter that cannot act
should complete it with an error. Not implementing the kind at all is still
correct; the operation is reported failed and the deletion proceeds.

`channel-telegram` un-archives the topic, posts the notice and closes it again,
because a closed forum topic refuses `sendMessage` and an open one would invite
replies into a conversation that no longer exists. It does not delete the topic:
the history above the tombstone is what a person scrolls back to.

`previousThreadId` is an optional **hint**, present only when a closed
conversation is being reopened: the thread this conversation used before its
topics were archived. **Ignoring it is a valid implementation.** If your
transport can un-archive, honour it and return that same id — the conversation
then continues where it left off, with its history above the new messages. If it
cannot, open a fresh thread and return the new id; the manager records whatever
comes back. That asymmetry is why this is a hint on an existing operation rather
than a `reopen-topic` kind: most transports have no un-archive, so most
implementations of a new kind would be a second name for `ensure-topic`. Whether
un-archiving is possible is transport knowledge, and the manager holds none.

**Prose fields are markdown**, in one deliberately small subset:

```
**bold**   *italic*   `inline code`   ```fenced code```   [text](url)
```

Anything outside that subset is UNDEFINED — you may render it, escape it or
strip it, and no caller may depend on which. The subset is small because the
alternative to naming it is every adapter inventing its own.

**Rendering, escaping, splitting and truncation are yours, entirely.** The
manager guarantees nothing about how a message looks or how long it may be.
`channel-telegram/render.go` is the reference: HTML composition, entity
escaping, 4096-character splitting, 128-character topic names. An adapter that
cannot be bothered may concatenate the fields and send them plain — the contract
asks only that presentation be the adapter's call.

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

**A thread opens with the event that caused it.** When an input is appended and
a thread binding exists, the manager posts it as a `signal` message — so an
alert thread reads as the event, then the work, then the answer, and a run that
hangs or dies still leaves the thread saying what happened. The rule is "post
what a human has not already seen": inputs with a `signal` origin are posted,
`chat` signals and channel-originated inputs are not (the person typed those,
and siblings get them as a `relay`), and inputs predating provenance post
nothing at all. Card op ids are stable per conversation×input×channel, so
reconcile-driven re-enqueues dedup. Posting runs parallel to dispatch — it
neither gates nor is gated by the run.

**The operator delivers, always.** An agent's printed answer is its whole
deliverable: the runtime reports it via `/work/done` and the manager posts it
to every bound thread through the serving adapters. Agents never send chat
messages themselves, so no prompt carries transport instructions and no
runtime holds a channel's credentials — the surface is the adapter's business
alone. A conversation dispatches once at least one of
its topics exists (one broken channel never deadlocks it), and channel
implementations must never re-ingest their own outbound posts as inbound
(relayed messages would loop otherwise).

**Conversation context travels as an OPAQUE HANDLE.** A work unit carries
`runtimeContextId` — the runtime's own identifier for everything this
conversation has accumulated — and the obligation is one sentence: **continue the
context this handle names, or report that you could not.**

Where that context lives is entirely yours: session files on a mounted volume, a
thread id at a vendor API, rows in a database. The manager stores the handle,
hands it back, and interprets nothing. `--resume` is one runtime's implementation
of this and `session` is that runtime's word for it; neither appears in the
contract.

`POST /work/done` reports back:

| field | meaning |
|---|---|
| `runtimeContextId` | the handle for the context this run leaves behind. **Latest-wins** — it replaces the stored one, including on a FAILED run, because a crash after a context was established should not strand it |
| `continuity` | `continued`, `new`, or `unavailable` |
| `continuityReason` | free text, recorded verbatim, when a context could not be reached |

`continuity` exists because the manager cannot infer it: it sends a handle and
receives a handle, and when they differ it has no way to tell an agent that
branched deliberately from one that was forced to start over. **Absent means no
claim** — a runtime that omits it keeps today's behaviour, because an addition to
the contract must not make an existing runtime look broken. A runtime that cannot
continue anything conforms by always reporting `new`.

Reporting `unavailable` means the context was LOST. Do not answer without it: a
conversation without its context is a new one wearing the same name and thread,
and an agent asked to undo something it has no memory of will guess. Fail the run
and say why — never with an empty result, which is what made answering-anyway
look like the kinder option. Before declaring it, distinguish a store that says
GONE from one that did not ANSWER: a shared filesystem can stall for seconds, and
ending a conversation over that would be a correctness bug.

`resumeSessionId` / `sessionId` are the retired spellings of the handle, sent and
accepted for one release so a runtime image can be upgraded independently of the
manager. Read `runtimeContextId`.

**...and delivery is a recorded fact, not a queue entry.** `/work/done` writes
the run into `Conversation.status.runs[]` and enqueues the reply, exactly as
before — but it is no longer the only path that can produce it. The reply op id
is **stable per conversation×channel×run**:

```
send:<conversation>:<channel>:<runId>
```

Completing it appends the channel to `status.runs[].delivered[]`, and
reconciliation enqueues a reply for any completed run whose bound thread is
missing from that list. So a manager restart between `/work/done` and an adapter
claiming the op re-derives the answer rather than losing it, and a fan-out
interrupted after one of three threads completes the other two without repeating
the first. Adapters need change nothing: the id is a better dedup key than the
counter it replaces, and delivery stays at-least-once.

Ack and notice sends keep counter-based ids (`send:<counter>`) and stay
fire-and-forget — saying the same thing twice is correct when a user asked
twice, and neither derives from CR state.


## The signal adapter contract

Signals are one-directional, so this is the channel contract minus the ops
queue — an adapter normalizes its transport into signals and the manager does
the rest (**adapters normalize, the manager groups**):

1. Read your sources + opaque `spec.config` from `GET /signal/sources?adapter=`
   (entries carry `credentialEnvPrefix` exactly like the channel listing).
2. Push normalized signals: `POST /signal/inbound {"source", "signals":
   [{"fingerprint", "labels", "title"?, "payload", "kind":
   "alert"|"job"|"task"|"chat"}]}`.
   The manager applies the source's `grouping` policy: fingerprint cooldown
   (at-least-once delivery is safe — re-sends collapse), signature from
   `labels` × `signatureLabels`, window reuse, recurrence-on-session.
3. Persist cursors via `GET/PUT /signal/state/{source}/{key}`, report config
   problems via `POST /signal/sources/{name}/status`.

**This endpoint is the only way work originates from outside a chat surface.**
There is no route that names a `Pipeline`: a caller posts to a `SignalSource`,
and the Ready Pipeline claiming that source decides which agent answers, on
which channels, with which capabilities.

The `kind` selects the lane, and the lane decides two things — which prompt
renders, and how a later signal is keyed when the source declares no
`signatureLabels`:

| `kind` | Lane | Subject | Keying with no `signatureLabels` |
|---|---|---|---|
| `alert` (default) | read-only investigation | a problem that recurs | default labels `alertgroup`/`alertname`/`namespace` — group and resume |
| `job` | task-lane prompt, `jobName` = source | a job that recurs | same default labels — successive ticks fold into one conversation |
| `task` | task-lane prompt, no `jobName` | a caller asking once | the signal's own fingerprint — each post is its own conversation |
| `chat` | task-lane prompt, no `jobName` | a person asking once | the signal's own fingerprint — each message is its own conversation |

`alert` and `job` are recurring-subject lanes: the second signal is more news
about the same thing, so it folds into the open conversation and resumes the
session. `task` and `chat` are one-shot lanes: the second signal is a second
request, and the default labels are alert vocabulary neither carries, so keying
on them would hash every request to one empty signature.

A `chat` signal MUST carry the label `agentops.dev/channel` naming the surface
it arrived on, or it is refused — its reply has nowhere to go. A `task` signal
must not: replies go to the claiming Pipeline's `channelRefs`.

**A posted task inherits the target source's `grouping`.** The lane rule above
is a FALLBACK, not an override — a source that declares `signatureLabels` groups
by them in every lane, `task` included, so two tasks sharing those label values
land in one conversation (and two tasks carrying none of them share the empty
signature). Post to a source whose grouping is what you want; operators who need
an isolated ask lane create their own source against an adapter they run.

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

**`$CONTROL_URL` is not always the manager.** When a runtime declares
`contextSync`, the manager points it at a sidecar in the same pod, which
forwards every request to the real manager.

Nothing in the contract changes, and a runtime needs no awareness of this.

That is the point. Observing the contract a runtime already implements is what
lets context be checkpointed at work boundaries without every image
reimplementing it. Two moments are acted on, both invisible to the runtime.

| moment | what happens |
|---|---|
| before the first `GET /work` answers | durable context is restored into the pod-local home |
| before `POST /work/done` reaches the manager | the context is checkpointed |

That ordering is a guarantee, not an implementation detail. The manager records
the context handle from the completion report, so checkpointing afterwards could
leave a recorded handle whose context was never persisted. The next run would
then fail a continuation that should have worked.

Reference implementation: [`runtime-claude/`](../runtime-claude/) (Node.js + claude-code, ~200 lines).
The same bring-your-own pattern applies to chat transports — see the channel
adapter contract above and [`channel-telegram/`](../channel-telegram/).
The sidecar is [`context-sync/`](../context-sync/).


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
| `context.restored` | runtime → conversation | durable context copied into a starting pod |
| `context.checkpoint` | conversation → runtime | live context copied to the volume |
| `context.skipped` | conversation → runtime | a checkpoint ran and found nothing changed |
| `context.failed` | conversation → runtime | a restore or checkpoint failed |

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
| `GET /work`, `POST /work/done` | runtime-facing dispatch (see contract) |
| `POST /work/context` | context-sync sidecar reports one restore or checkpoint |
| `GET/POST /channel/*` | adapter-facing channel contract (bearer token; see adapter contract) |
| `GET/POST/PUT /signal/*` | adapter-facing signal contract (bearer token; see signal adapter contract) |
| `GET/POST /activity*` | per-hop telemetry (bearer token; see activity contract) |
| `GET /status`, `GET /pipelines/{name}/resolved` | manager introspection (bearer token) |
| `GET /healthz` | liveness |
| `:9090/metrics` | controller-runtime metrics + the `agentops_*` set above |

**There is no programmatic origination endpoint.** To start a conversation from
a script, post a `kind: task` signal to a `SignalSource` a Ready Pipeline claims:

```sh
curl -s -X POST http://agentops-manager.<ns>.svc:8080/signal/inbound \
  -H "Authorization: Bearer $ADAPTER_TOKEN" -H 'Content-Type: application/json' \
  -d '{"source":"<source>","signals":[{"fingerprint":"ci-'"$BUILD_ID"'",
       "kind":"task","payload":"why is the api pod crashlooping?"}]}'
```
