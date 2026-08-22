## Invariants (do not break)

### THE PARENT CHART OWNS THE SUBSTRATE — BUNDLES CONTRIBUTE DOMAIN

How agents execute — image, LLM credential, idle TTL, node placement, home
volume, and the ONE identity whose RBAC is the agent's power — is release-wide
and lives in `chart/values.yaml` (`runtime:` + `global.agentops.runtime.*`).

- **No subchart renders an `AgentRuntime`, a runtime ServiceAccount or a
  credential Secret.** Bundles ship sources, profiles, tooling and channels, and
  REFERENCE it.
- **Both substrate keys are under `global.`** because a subchart can read no
  other parent scope and k8s-bundle's MCP server derives from them. Restating
  them in a subchart recreates the two-spellings-of-one-fact problem chart 4.0
  removed.
- **Putting the runtime in a bundle is what made a chat-only install unable to
  execute anything**, and made TWO runtime SAs exist, one granted everything.

### The manager reads NO secrets — zero Secret API reads

- **Everything secret-shaped compiles to `valueFrom` / `envFrom` in pod specs.**
  The kubelet resolves it.
- **Transport credentials are declared per Channel** (`credentialsSecretRef`)
  and PROJECTED into adapter pods, never read.
- **The adapter auth token reaches the manager via env** (`ADAPTER_TOKEN`).
- **Per-adapter tokens are DERIVED** — HMAC of master plus adapter name,
  validated by re-derivation. Nothing minted or stored.
- **RBAC grants the manager no `secrets` verbs at all.** Keep it that way.

### The operator grants adapters NO Kubernetes permissions, ever

Dedicated SA, and no RBAC objects created or bound by any reconciler.

- **Default posture is `automountServiceAccountToken: false`.**
- **`SignalAdapter.spec.kubernetesAccess` only mounts the token and injects
  `POD_NAMESPACE`.** What it may DO is granted externally, by chart or user,
  against SA `agentops-signal-<name>` — so an adapter CR can never escalate.
- **Name-is-key makes one adapter per implementation structural.** There is no
  conflict machinery to maintain.

### Strictly serial per conversation

- **ONE inflight unit.** Parallelism is across conversations.
- **Capped by `MAX_ACTIVE_CONVERSATIONS`** (default 5), with idle-runtime
  eviction.
- **`MAX_RUNTIMES` is the deprecated alias**, honored one release.

### THE CAP IS DECIDED BEFORE ANYTHING IS PROVISIONED

**"Active" means POD-BACKED and is counted from the live pod list, never from
status.** A pod stuck unschedulable, or a lost status patch, must not invent
capacity.

**A conversation that cannot be admitted gets phase `Pending`:** no runtime pod,
no MCP ConfigMap and — the point of the phase — **no `ensure-topic`**.
Suppressing the topic is what stops a burst becoming a thousand chat threads.

- **`Queued` keeps its old meaning** — ADMITTED, waiting behind the serial rule
  — and is never used for capacity waiting. Conflating them is the mistake to
  avoid.
- **Admission is FIFO by creation time over a waiting set defined by PODS, not
  phase.** Keying on phase lets a brand-new conversation reconciled first jump
  an older one.
- **The backlog itself is bounded by `MAX_QUEUED_CONVERSATIONS`** (default 50),
  checked in INGEST rather than the reconciler because the point is not to
  create the object at all. It gates CREATION only, so window reuse keeps
  appending to a pending conversation.

### `/close` sets a PHASE — DELETION IS A SECOND VERB

Closing writes phase `Closed` plus `status.closedAt` and tears down the pod, the
MCP ConfigMap and the capacity slot, archiving every bound thread at the
TRANSITION.

The object, its `status.runs[].result`, its `runtimeContextId` and its volume
state all survive, which is what makes REOPEN mean anything.

**Closing used to delete, and that is exactly why nobody closed anything:** the
only tidying tool cost more than the backlog.

**A closed conversation is INERT:**

- No dispatch, no capacity, no place in the FIFO waiting set.
- **Absent from conversation REUSE** — a matching signature opens a NEW
  conversation. This is the rule that makes closing mean anything.
- Absent from every pipeline.
- **A reply typed into a closed thread is ANSWERED** ("closed, reopen it") and
  creates nothing. Appending an input there would be a black hole, and an
  implicit reopen would re-materialise threads on every bound channel because
  someone typed "thanks".

**REOPEN NEVER RE-RESOLVES REFS.** Phase → `Idle`, `closedAt` cleared,
materialized refs left EXACTLY as they are.

- **Refs are snapshots whose content is re-read at every use**, so re-resolving
  would let a Pipeline edit re-wire an existing conversation.
- **A missing profile or channel FAILS the reopen naming it**, never partially.
- **Threads come back through an ordinary `ensure-topic` carrying
  `previousThreadId` as a HINT.** An adapter that can un-archive returns the same
  id, one that cannot returns a new one and is already correct.
- **`status.reopens` exists so each reopen's ensure-topic op id is distinct.**
  The ids are stable per conversation × channel, so without it the re-establish
  dedups against the original topic creation and never reaches the adapter.

**`close-topic` IS NOW DERIVABLE.** It was the exception only because it was
enqueued while the object was disappearing, leaving nothing to record against.

**`status.threadsArchived[]` marks the done threads**, so an unarchived one is
an archive still owed. Do not re-add the "one non-derivable op" clause.

**The `agentops.dev/close-topics` finalizer survives for the ONE path where the
object really does go away** — a direct `kubectl delete` of a conversation
nobody closed — with its 2-minute grace so a down adapter can never wedge a
deletion.

`/close` is intercepted on the REPLY path before the text could become an input,
and answers with usage on a general surface.

**Delete and reopen are MANAGER VERBS whose reach is the BINDING**
(`spec.channelRefs`, read off the conversation, never off the request), and
delete REFUSES anything not already `Closed`.

**That is what the retired "no remote close verb exists" rule protected** —
*you may only end a conversation you are PART of*, with a live thread as the
proof. A closed conversation has none, so the binding is the next-strongest.

### `/exit` RELEASES THE RUNTIME — `/close` ENDS THE CONVERSATION

**One word apart and not interchangeable.** `/exit` deletes the runtime POD and
nothing else: object, threads, inputs, runs and `runtimeContextId` all survive,
and the next input admits it again with a fresh pod.

**It exists for the half eviction cannot serve.** Eviction only runs when
something is WAITING, so with nothing waiting an idle pod holds its slot, its
checkout and whatever the runtime keeps resident until the idle TTL.

That wait is longest on exactly the installs that RAISE that TTL, for a big
checkout or a warm local model.

**`dispatch.NeedsWorker` is THE ONE definition of idle**, shared by the command
and the eviction path.

The controller's private `needsWorker` is gone, and restating it either side is
the regression to avoid — the two disagreeing surfaces as a bug report about the
cap, far from both.

**REFUSED MID-RUN, on correctness grounds, not politeness.** An inflight run
still needs a worker, so the replacement pod:

1. Is created AT ONCE.
2. Gets nothing from `/work`.
3. Idles out the LONG TTL and is reaped as `Succeeded`.
4. Clears `Inflight`, makes the input pending again and **RE-RUNS work that may
   already have acted**.

**`/close` owns abandonment, and owns it safely.** Queued input is refused too,
merely because the pod would come straight back.

**What the release COSTS is computed, never guessed.**
`ResolveFor(...).ContinuityPossible()` — the same call dispatch uses — decides
whether the reply promises the context or warns it starts fresh.

**A Pipeline named after a manager command** (`pipelines`, `agents`, `exit`, `close`, `help`,
`start`) **is unreachable by that command.** Interception precedes the Pipeline
lookup, which is what makes the commands reliable.

### A RUNTIME POD THAT NEVER STARTS IS REAPED, NEVER EXEMPTED FROM THE CAP

Reaping used to handle `Succeeded` and `Failed` only.

`Pending` is COUNTED as active — correctly, since a stuck pod must not invent
capacity — but nothing bounded how long it could sit there, so five pods behind
a corrupt filesystem held an entire install for fifteen hours on 2026-08-20.

- **The fix is a start DEADLINE after which the pod stops existing**, which
  frees the slot through the DELETE watch that already promotes the FIFO-first
  waiter.
- **Un-counting it instead is the invent-capacity mistake** and would provision
  past the cap against resources the cluster has not released.
- **The condition carries the KUBELET'S OWN REASON, verbatim.** A message
  reading only "deadline exceeded" reproduces the real failure — fifteen hours
  in which nothing said what was wrong — with a timer attached.
- **Classification comes from POD STATUS alone.**
  `PodReadyToStartContainers` is the discriminator: false exactly while a volume
  will not attach, true before image pulling begins. So the manager needs no
  event-read RBAC for it.
- **A conversation inside its start-failure BACKOFF is skipped by the admission
  waiting set.** Leaving it at the FIFO head reproduces the outage one layer up:
  the oldest conversation cannot start, and everything behind it waits on a slot
  nobody will take.

### ONE STORAGE BREAKER, TWO EDGES

`internal/storagebreaker` treats unavailability as an OUTAGE before a LOSS, and
it is fed BOTH by runs that report an unreachable context AND by pods that
cannot be provisioned for a storage reason.

- **It lived in `httpapi` watching only the first**, which is why it never fired
  for the incident it was written for: no pod started, so no run existed to file
  a report.
- **A SECOND breaker would be worse than none** — two judgements about whether
  storage is down, disagreeing at the worst moment.
- **Only STORAGE-attributable provisioning failures count.** An unschedulable
  pod or an unpullable image opening a storage breaker would hold every
  conversation in the install for a reason that has nothing to do with storage.
- **While open:** admit nothing, hold in `Pending` with a reason that says
  STORAGE rather than queue, and re-test with ONE canary.
- **The provisioning edge cannot close its own breaker** — no pod means no run
  means no success to report — which is the whole reason `ProbeDue` exists.

### CONDITION TAINTS ARE NOT DRAINS

`node.kubernetes.io/not-ready`, `unreachable` and the pressure taints are
applied by Kubernetes from node CONDITIONS.

- **Reading them as a drain releases runtime pods during a transient NotReady**,
  and during a partition across many nodes at once — precisely when acting on a
  stale view is least affordable.
- **Only `spec.unschedulable` and taints outside that set** mean a node is being
  taken down deliberately.
- **Drain awareness is OFF by default and gated on `rbac.drainAware`**, because
  seeing a cordon means reading NODES and every other permission this manager
  holds is namespaced.
- **It shrinks the corruption window, it does not close it.** The storage
  provider picks where a shared volume is served independently of where runtime
  pods run.

### THE RECLAIMING JOB'S LISTING IS PHASE-BLIND, ON PURPOSE

`housekeeping/` removes workspace directories and session transcripts with no
`Conversation` behind them.

**A CLOSED conversation still HAS a CR**, so its state is protected by the same
rule that identifies an orphan.

The job needs no phase knowledge at all, and an "only look at live ones"
optimisation would reclaim the state of every conversation an operator was
keeping.

**Two orderings, each a correctness argument:**

| Target | Order | Because |
|---|---|---|
| Workspaces | scan the disk FIRST, list SECOND | the CR always predates its directory — the pod that creates it exists only for a conversation that already exists |
| Transcripts | the OPPOSITE, plus a grace period | the context handle is written AFTER the file exists |

**It runs under its own SA** — mounting the claim ROOT is the reach `subPath`
isolation denies agents — and the render fails if that SA equals the runtime's.

### FILESYSTEM STATE GOES ON A PVC — EVERYTHING ELSE IN THE KUBERNETES API

**And THE MANAGER MOUNTS NOTHING.** A claim would pin it to one node, defeat
rescheduling, and be a second source of truth beside the CRs — which is the
failure mode this rule exists to name.

Manager state is therefore always one of three things:

1. A cache of a Kubernetes object.
2. DERIVABLE from Kubernetes objects.
3. Declared lossy telemetry.

**State fitting none of the three is a defect.** The matrix in
`docs/concepts.md` is where its row goes.

Consequences that were each a real loss:

- **The reply is a FACT, not a queue entry** — `status.runs[].delivered[]` per
  bound thread plus `.deliveryTracked`, a stable op id
  `send:<conversation>:<channel>:<runId>`, and a reconciler backstop. Otherwise
  `/work/done` enqueueing into an in-memory queue means a restart drops an
  answer already durable in `status.runs[].result`.
  - **Marking happens on op COMPLETION.** Mark on enqueue and a lost op is never
    re-derived.
- **A run with no `deliveryTracked` is BACKFILLED as delivered, never sent.** It
  predates the mechanism, and no timestamp can tell it from a run lost to a
  restart, since both completed before the current process started. Without it,
  upgrading re-posts every recent answer to every bound thread.
- **A person's WORDS are a fact too** — `status.runs[].inputs[]`. Never declared
  lossy and lost anyway, because the only copy lived in a queue built to be
  pruned. Own invariant below.
- **Cooldown lives on `SignalSource.status.cooldown[]`**, written only when a
  fingerprint is ADMITTED. A suppressed re-delivery must stay free, or the
  high-volume case cooldown exists for becomes a write storm.
- **`close-topic` is DERIVABLE now** — from a bound thread missing from
  `status.threadsArchived[]`. It stopped being the exception when closing
  stopped deleting: the object survives, so there is something to record
  against.
- **Telemetry is the declared-lossy class and must REPORT its gaps.** A cursor
  from a previous process is `>= next` in the new one's sequence, so answering
  it with an empty list reads as "nothing happened" — the case eviction alone
  does not catch.

### CONVERSATIONS ORIGINATE ONLY FROM SERVED SIGNAL SOURCES

**A channel CARRIES conversations, it never starts one.**

- **`/channel/inbound` is reply-only** — `threadId` REQUIRED, unknown threads
  dropped, no adoption.
- **A message on a chat's general surface arrives as a `kind: chat` signal from
  a chat `SignalSource`**, so who answers is DECLARED by the Pipelines listing
  it: ALL of them for any other kind, and for a bare chat message only when
  there is exactly one.
- **There is no channel default profile and no `PipelineForChannel`.** Channels
  are shareable on purpose, so "which pipeline answers for this channel" has no
  defensible answer, and the oldest-Ready tiebreak that used to supply one is
  gone.
- **`PipelineForSource` is gone too**, replaced by the plural
  `PipelinesForSource`. A caller wanting ONE answer must now say what it does
  with several.

**The chat lane:**

- Task inputs, never `job` — that resumes sessions.
- Cooldown OFF by default.
- NO signature grouping unless `signatureLabels` is set. Chat keys on the
  fingerprint, and the default alert labels would hash every message alike into
  one conversation.
- **Commands whose whole result is a reply** (`/pipelines`, unknown pipeline, usage
  error) emit a send op and create nothing.
- **A chat signal MUST carry `agentops.dev/channel`.** `/signal/inbound` refuses
  one it could not answer.

### HTTP API is NOT leader-gated

`NeedLeaderElection()=false` — webhooks must serve during rollouts.

**Exactly one getUpdates consumer per bot token, ever.** That consumer is
`telegram-router`: ONE poll loop per Deployment and ONE Deployment per token
(replicas 1 + Recreate, chart-owned).

**Neither adapter polls, and the manager has no poller.** Adding a poll loop
back to `channel-telegram` is the mistake that produces 409s and stolen
updates.

### Channel ops are at-least-once

**`spec.config` is opaque to the operator.** Never parse channel-type config
manager-side — adapters validate their own and report via the Channel Ready
condition.

**The manager never *interprets* config, but it MAY apply an adapter-declared
`configSchema` mechanically** (`internal/configschema`, the one place config
content is touched): advisory-only `ConfigValid`, no type knowledge, adapter
stays authoritative.

### THE MANAGER COMPOSES MEANING, ADAPTERS COMPOSE PRESENTATION

**No transport dialect anywhere in `internal/`** — no `<b>`, no `&lt;`, no
`parse_mode`.

**An op carries a TYPED message** (`signal` | `answer` | `relay` | `notice`,
prose in a named markdown subset) **or a TOPIC DESCRIPTOR, never rendered
text.** There is no `op.text` and no `op.title`.

- **Escaping, length limits, splitting and topic naming belong to the component
  that knows them.** Telegram caps messages at 4096 and topics at 128, nothing
  else does, and a manager-side fix would be one transport's limits imposed on
  all of them.
- **In-process providers are held to the same contract.** They are a second
  renderer, not an exemption.
- **`/channel/ops` REFUSES an adapter that does not declare `contract=`**,
  because one reading the retired `text` field would post empty messages and
  look healthy doing it.
- **`router.go` used to open with "transport-neutral" and then emit Telegram
  HTML.** That is the habit this invariant names.
- **It binds the AGENT too.** `dispatch/templates/format.md` tells it to write
  the same markdown subset, because an adapter escapes what it is given — the
  first version of this change left format.md on HTML and every agent answer
  reached Telegram with its tags showing.

### A thread opens with the event that caused it

**DELIVERY IS DECIDED PER DESTINATION.** Every input is delivered to every bound
channel EXCEPT the surface it entered on, because that surface displayed it as
it was typed.

**ONE rule, ONE implementation** (`chat.DeliverInputs`), called from two places:

1. **The reconciler** — the backstop that makes it derivable.
2. **The router**, the moment an input is appended — the fast path that keeps a
   thread in the order things happened.

Exactly as a run's reply is.

**"Already seen" is a fact about a SURFACE, never about a message.** The
origin-KIND rule (`InputItem.PostToChannels`: `signal` posts, `channel` does
not, `kind: chat` does not) and its stated chat exception are DELETED.

**They asked the question once, per MESSAGE**, and so withheld a person's words
from channels that had never shown them — a console transcript beginning at the
agent's answer was that bug. Re-adding either, in any layer, is the
regression.

- **Whether the origin surface displayed it is TRANSPORT knowledge**, declared
  by the implementation: `ChannelAdapter.spec.echoesOwnMessages`, default TRUE,
  and FALSE on a viewer that renders only what it is sent — which is why the
  console receives its own users' messages. An unreadable channel or adapter
  answers TRUE, the conservative half.
- **The SURFACE itself is resolved in one place for both lanes**
  (`InputItem.OriginSurface`): a channel origin names its channel, a chat signal
  carries `agentops.dev/channel` in its labels.
- **What ARRIVES depends on who said it.** An event is a `signal` card, a
  person's words are a `relay` keeping `origin` and `sender` structured.
- **An ABSENT origin is delivered nowhere**, so upgrading cannot spray history
  into every open thread.
- **Op ids stay stable per conversation × input × channel**, or every reconcile
  reposts everything.
- **A card names its pipeline from `chat.PipelineForConversation`**, which READS
  `spec.pipelineRef` and falls back to binding-matching only for conversations
  predating it, omitting the name when even that is ambiguous.

### A CONVERSATION'S MESSAGES ARE KUBERNETES-API STATE

`status.runs[].inputs[]` holds what each run was asked — text (capped at
`MaxRecordedInputText`, marked `truncated` beyond it), arrival time, origin
surface, sender — beside what it answered.

**THE QUEUE AND THE RECORD ARE DIFFERENT THINGS.** `spec.inputs[]` is a work
queue and `pruneProcessed` keeps emptying it, which is what stops answered work
running twice.

**Pruning must never be the only copy of what a person said.** It was, so a
conversation kept the answers and dropped the questions and a viewer could
rebuild half a thread.

**The ORDERING is the guarantee.** The record is written by the SAME status
write that marks the inputs processed (`handleWorkDone`), therefore strictly
before anything may prune them. Recording in a second pass would let a crash in
between destroy the message permanently.

**A viewer's buffer is a CACHE of that record, never its only copy.** The
console workaround that watched the queue and matched text is deleted, not kept
as a fallback.

### No relay loops

**Channel implementations — adapters AND in-process providers — must never
re-ingest their own outbound posts as inbound.** Cross-channel relay depends on
it.

**LOAD-BEARING IN ONE MORE PLACE now:** one adapter may serve several surfaces
of one conversation, so a message can be delivered TOWARD the transport it
entered through, and an implementation that echoed its own outbound posts would
loop rather than merely duplicate.

### No signal loops

The same rule one lane over: **an observing signal adapter must NEVER emit a
signal about agent-ops' own machinery.**

The cycle: a runtime pod that cannot start emits a Warning event, that event
becomes a signal, the signal opens a Conversation, the Conversation creates
another runtime pod under a NEW name, forever.

**Nothing downstream catches it:**

- The fingerprint is fresh (new pod name).
- The workload is fresh (owner is the Conversation CR).
- Even a correct liveness re-check passes it, because the pod really is broken.
- `MAX_ACTIVE_CONVERSATIONS` caps pods and `MAX_QUEUED_CONVERSATIONS` caps the
  backlog, but neither stops the LOOP. It just fills etcd more slowly.

`signal-k8s-events/selfexclude.go` implements THREE independent mechanisms:

1. **Name prefix** — needs no API read, so it holds with a cold cache.
2. **Owner/label.**
3. **Own-namespace.**

**Only the third is configurable.** A deny-list is editable, and an editable
loop breaker is not one. A nil excluder still applies mechanism 1 on purpose.

**agent-ops' own health is STATUS, not SIGNAL.** The reconciler already holds
the failure. Routing it back through ingest to wake an agent is the
architectural error, not merely a noisy one.

### Runtime pods

- **ownerRef → Conversation**, for GC.
- **Repo checkout at `/data/workspace`.** claude-code sessions are keyed by cwd,
  so moving this path breaks session resume.
- **`/data/workspace` and `/data/home` are mount points.** Clear contents, never
  rmdir.

### Dispatch and ingest semantics are pinned by test fixtures

Change behavior by changing tests deliberately, not incidentally.

### The signal adapters' rule vocabulary is its own rule

**`rules`, `route`, dwell, inhibition and the time axis are in
`.claude/rules/signal-rules.md`**, which loads with the adapters and the chart
values that render their defaults.

- **`for:` is Prometheus and `group_wait` is Alertmanager**, and they are NOT
  the same thing. That is the whole of it at this level.
- **Event grouping is by WORKLOAD** — `[namespace, workload]` through owner
  references, never by parsing a pod name.
