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

#### Gaps in recorded activity

The buffer is in memory, so its history ends two ways: it wraps, or the manager
restarts and begins its sequence again at 1. Either way the manager answers an
unservable cursor with a **resync**, and the console records that as an
`activity.gap` marker at the point it happened — not just as a counter. The
Topology page carries a banner and the Overview's telemetry card names the time
before which nothing is available.

The gap rides on the **stream health** (`stream.lastGap`), which every view
already reads, rather than only on the live event stream. That is deliberate:
the reader who matters is usually the one who opens the console *after*
something went wrong, and a live-only signal is invisible to exactly that
person.

That is the whole reason it exists: a stretch with no recorded hops and a
stretch in which nothing happened look identical, and only one of them means the
system was idle. Edge rates spanning a gap under-report and say so.

#### The `live` chip, and who owns reconnection

The masthead chip reports the browser's own stream to the console. **The SPA
reconnects it, not the browser.** `EventSource` retries a dropped connection by
itself, but on an HTTP error — a 502 while the console pod rolls, a 401 once the
session expires — it sets `readyState` to `CLOSED` and gives up permanently.
Nothing reopened it, so the chip read `stream disconnected` and the graph stayed
frozen until the page was reloaded. The client now reopens with backoff (1s
doubling to 30s), leaves genuine transport blips to the browser, and stops on
unmount.

A chip that only tells the truth after a reload is the same failure this surface
exists to prevent, one layer up: a viewer that cannot tell "the link died" from
"the system is idle".

Everything else is unaffected, because everything else is read from Kubernetes:
conversations, topology, configuration and run history come back complete after
any restart. Only the per-hop timeline has a hole in it — the console persists
nothing and needs no volume, since its config cache rebuilds by list→watch and
its activity index by cursor replay.

### Conversations

Server-side filtering (phase, pipeline, profile, channel, errored, unread,
search), sorting by last activity, and pagination with a total match count — an
event storm makes thousands, and shipping them all so the browser can hide most
is how a viewer becomes an API-server problem. Run history is dropped from list
rows; a result is a whole agent message, and each row carries its read state
instead.

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

### Unread

Conversations whose console thread has activity newer than its watermark are
marked in the list, an **Unread only** switch narrows to them, and the count
rides on the *Conversations* navigation item. Opening a conversation reports its
console thread read; **Mark read** over a selection clears a batch of them.

**Read is per PERSON where the console can tell people apart, and per console
otherwise.** Which install you have depends on one thing — whether a request
arrives with a forwarded identity:

| Authentication | Who a "reader" is | Effect |
|---|---|---|
| A proxy forwards an identity (oauth2-proxy et al.) | each person | your badge is yours; a colleague reading does not clear it |
| The shared UI token | everyone, as one reader | whoever reads clears it for all holders — there is one credential and no person behind it |
| No reader salt projected | everyone, as one reader | per-channel marks, exactly as before this existed |

Reading a conversation in Telegram never clears it here, and reading it here
never clears it there — that part is per channel in every install.

**No identity is stored.** The console hashes the resolved identity with a salt
projected as a channel credential and sends only that opaque key; the manager
stores it verbatim and can neither reverse it nor tell whose it is. So a
conversation records that N people read it and when, and never who. The salt is
generated on install (and added on the first upgrade that finds the Secret
without one) — don't rotate it casually: a new salt orphans every stored key and
silently resets everyone to the channel-wide mark.

**Starting a conversation marks it read for you** — and for nobody else. The
opaque key travels with the chat signal, and the manager stamps that reader's
watermark the moment it creates their thread. Without it a conversation you had
just typed came straight back as unread, before an answer could exist.

The rest of what it does, and why:

- **Only conversations the console holds a thread on.** An *observed* one is
  never unread: the console has no watermark on it and no standing to call it
  new. Same reach boundary as closing, and a batch naming one comes back
  `skipped` with the same fix.
- **The count is computed before any filter**, over every conversation, so
  narrowing the view never moves it — a count that shrank because you filtered
  would let a filter hide a backlog without saying so.
- **The browser never invents a timestamp.** A read is reported with the
  watermark the server read off the conversation's own state, and nothing is
  sent when it would not advance. The manager clamps and enforces monotonicity
  on top of that, so a stale page cannot un-read a thread.
- **The batch is bounded at 50**, the list page size, enforced by the server —
  the selection is over the rows on screen, and there is no "mark everything
  matching the filter", for the reason there is no close-everything.
- **It is authenticated and attributed, but NOT behind the write gate.**
  `console.write.enabled` makes the console a strict viewer by removing its
  ability to instruct an agent or start work; a read watermark does neither. A
  read-only console still selects rows and marks them read — one that could show
  a backlog and never clear it would be broken in the way the unread mark exists
  to fix.
- **The mark only moves forward.** There is no "mark as unread": a watermark
  that could go backwards is a different mechanism, and reading a conversation
  again is what makes it read.

Threads bound before this existed are treated as **read** — see the backfill
rule in [concepts](concepts.md#read-state-per-thread). Right after the upgrade
the list is therefore quiet, and fills as new activity arrives.

### Closing a batch

Select rows and press **Close selected**. The confirmation names the count, how
many of them are working, and that the conversation stays and **can be
reopened** — closing is reversible.

**It is `/close`, fanned out.** Each selected conversation is sent the literal
text `/close` on its own console thread, exactly as a person typing it would.
The manager intercepts it on the reply path, posts the farewell to every bound
thread, archives them and moves the conversation to phase `Closed` — inert, but
intact. The runtime pod and the MCP ConfigMap go, the freed slot admits a
waiting conversation, and the conversation's recorded answers, context handle
and workspace all stay. There is exactly one implementation of closing, so a
batch cannot drift from a typed close.

What that costs, and why each cost is the right one:

- **Reach is the threads the console holds.** A conversation the console merely
  *observes* has no console thread and so nowhere to post; it comes back
  `skipped` with the fix — add the console channel to that conversation's
  pipeline `channels[]`. This is the joined/observed distinction made visible,
  not a permission gap.
- **Working conversations are left alone** unless the confirmation's
  *include working — abandons in-progress runs* toggle is turned on. The phase is
  read server-side from the conversation's own state; nothing the browser asserts
  decides it.
- **The result is per conversation** — closed / skipped / failed with a reason
  for anything not closed. A mixed batch is a successful request; "12 of 15
  closed" is the honest summary and a single verdict cannot carry the reasons.
- **The batch is bounded at 50**, the list page size, enforced by the server. The
  selection is over the rows on screen: there is no "close everything matching
  the filter", because a mis-set filter would then close far more than was ever
  visible.
- **A closed conversation shows as `Closed`, not as an absence.** It stays in the
  list, keeps its answers, costs no runtime pod and no capacity, and offers
  **Reopen**. A conversation held by its finalizer on its way out shows as
  `closing` instead and cannot be re-selected.
- **It is a write**: authentication, the install-wide write gate
  (`console.write.enabled`) and a forwarded identity all apply, and each close is
  logged against the identity that ordered it. A read-only console renders no
  close action, and refuses the request if one is made anyway.

### Reopening one

A `Closed` row offers **Reopen**. It goes back to `Idle` with its wiring, its
recorded runs and its context handle exactly as they were — the materialized
refs are not re-resolved, so a Pipeline edit made in the meantime does not leak
into a conversation that already exists. Under `contextStorage: volume` the
agent resumes with its workspace; under `none` it answers fresh and says so.

Threads come back through an ordinary `ensure-topic` carrying the archived
thread id as a hint: Telegram un-archives and you continue in the same topic, an
adapter whose transport cannot opens a fresh one. A reopen whose profile or
channel no longer exists fails and **names the missing object** rather than
producing a conversation that looks alive and can never dispatch.

**There is no bulk reopen, deliberately.** Reopening re-materialises threads on
every bound channel, so a batch of them would announce itself on surfaces nobody
is watching. It is a decision about one conversation.

### Deleting a batch

Select closed rows and press **Delete selected**. This is the irreversible half
of the lifecycle, and the confirmation says what goes: **the recorded answers**,
which are the only durable copy of what the agent said, and **the workspace on
disk**.

- **Only `Closed` conversations are candidates.** A live one named in a batch
  anyway comes back `skipped` with *close it first* — never closed on the way
  through. One call doing the irreversible thing to a conversation that was
  still working, behind a confirmation that named only the delete, is exactly
  what the two-step prevents.
- **Everything else mirrors closing**: the same 50-name bound enforced
  server-side, the same explicit selection over rows on screen, the same
  per-item outcomes (`deleted` / `skipped` / `failed`) with reasons, and the
  same write gate, identity and logging.

**Delete and reopen are manager verbs the console calls**, not Kubernetes
writes — the console still has no write path to the API. Their reach is the
**binding**: a surface may act on a conversation whose `spec.channelRefs` names
its channel, read from the conversation and never taken from the request. That
is the amendment the archived-thread case forces on "no remote close verb
exists": holding a live thread was how membership was proven, and a closed
conversation has none, so the binding that put the thread there stands in.

Disk is **not** reclaimed by deleting. The conversation's workspace directory
and session transcripts become orphans, and the opt-in housekeeping CronJob
removes them on its next run — see
[concepts](concepts.md#what-reclaims-what-and-why-the-manager-cannot).

## Starting a conversation

**The console originates the way everything else does: by emitting a chat signal
from a claimed `SignalSource`.** There is no pipeline-addressed endpoint to use
instead — the manager exposes none, and this is why it needs none.

Four things fall out of that:

1. The origination invariant holds literally — no side door to defend.
2. **Who answers is declared, not chosen by the caller.** Naming a pipeline in a
   request body would let the caller pick the agent. With a wired source, the
   wiring decides, which is the rule's actual point. The console cannot reach an
   agent no wiring points at — addressing one by name reaches only what is
   already declared Ready.
3. **Origination becomes visible traffic.** A conversation conjured by a direct
   call materializes from nowhere, leaving a hole in the graph exactly where the
   operator acted. A console source is a node with an edge: pressing "start"
   lights up the graph the console is showing.
4. **Self-started conversations join themselves** — the claiming pipeline's
   channel set includes the console Channel, so the result already has a console
   thread. No pipeline edit, no copy-paste patch.

**It requires a claim** — which a bundle that ships a route now makes for you.
Where the k8s bundle renders its route, that route claims the console source and
binds the console as a channel, so a turnkey install can start a conversation in
the console with no wiring step. The names come from `global.agentops.console`,
and the render fails if they disagree with the console's own
(see [the k8s bundle](k8s-bundle.md)).

Nothing else claims it for you. Wiring names a profile, sources and channels from
different bundles, so only the installer sees all of it. Until a Ready Pipeline
claims the console source it sits at `Wired=False`, the picker is unavailable,
and the UI shows that reason with the patch:

```sh
kubectl patch pipeline <name> --type=json \
  -p '[{"op":"add","path":"/spec/signalSourceRefs/-","value":{"name":"console"}}]'
```

A source is **shareable**: several Pipelines may list the console source, and all
of them stay Ready and addressable. What you can start is still exactly what is
wired — the picker is a rendering of the topology, not a free-text field that can
name something no wiring supports.

### Addressing an agent

With ONE Pipeline serving the console source, an unaddressed task goes to it and
nothing needs saying. With SEVERAL, an unaddressed task is **refused** rather
than handed to an arbitrary one, and the way to reach a specific agent is to
address it: `/<pipeline> <task>`.

The composer offers them. Typing `/` at the start of the task lists the Ready
Pipelines with their answering profile, narrows as you keep typing, and inserts
`/<name> ` with the cursor after it. Arrow keys move, Enter or Tab picks, Escape
dismisses without sending; a surface with nothing addressable shows no menu
rather than an empty one.

The listing is served by `GET /api/agents` from the Pipelines the console already
list/watches — **no new RBAC, no manager endpoint, no CRD field.** It is
Ready-filtered for the same reason `/agents` is: an unready Pipeline names wiring
that does not resolve, so offering it would invite a request nothing can serve.
The two must never disagree, and tests on both sides pin that they do not.

The listing is advisory — the console's cache can lag a moment, and an addressed
message to a Pipeline that has since gone is already answered with "unknown
agent". `/agents` remains the fallback wherever a client cannot present a menu.

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
- **Exposing it takes two things, not one.** The token is sent as a request
  header on every call, so an Ingress without TLS puts it on the wire in clear
  text, and TLS alone still leaves a single shared secret as the only gate.
  - **TLS** — name a certificate (`console.ingress.tls.secretName`) or let
    cert-manager issue one (`console.ingress.tls.clusterIssuer`, which also
    derives the Secret name). The chart cannot see what sits in front of it, so
    an install terminating TLS upstream at a load balancer or in a mesh is
    correct and the post-install warning can be ignored — but it warns, because
    the alternative is exposing the token silently.
  - **A forward-auth proxy** — oauth2-proxy against your OIDC provider is the
    usual one. When a trusted identity header is present the console records it;
    when it is absent it records `token`, so an install without a proxy has an
    audit trail that names nobody. Once one is in place you can stop
    authenticating twice — see below.
- **Root of a hostname only.** The SPA is embedded at build time with an
  absolute asset base, so it emits `/assets/...` URLs and cannot be served under
  a sub-path — that configuration routes correctly and then renders a blank
  page. `console.ingress.path` exists and is validated rather than quietly
  accepted: a non-root value fails the render. Give the console its own hostname
  or subdomain.
- **Every write is logged with the resolved identity** — who started what, and who
  said what to an agent.
- **`console.write.enabled: false`** makes it a strict viewer: the composer and the
  new-conversation action disappear, and both endpoints refuse server-side. The UI
  hiding a button is presentation; the server refuses regardless.

What it **cannot** do: write anything to the Kubernetes API. Its Role carries no
write verb, and no write path exists in the module.

### Where the UI token comes from

Three sources, in this order — the first one available wins:

| Source | Set | Rendered |
|---|---|---|
| Configured value | `console.auth.uiToken` | on install **and every upgrade** |
| Existing Secret of that name | — | install only, adopted rather than replaced |
| Generated | — | install only, 40 random characters |

A fourth option sits outside the order: `console.auth.existingSecret` supplies the
whole Secret, and the chart creates none.

**A redeploy does not sign anyone out.** When the token was generated, the Secret
is rendered on install only and carries `helm.sh/resource-policy: keep`, so an
upgrade neither regenerates it nor reports it as changed — including on a renderer
with no cluster (`helm template` piped to apply, CI, a GitOps controller, a
client-side dry run), where a cluster `lookup` returns nothing and the old
template therefore minted a fresh token. Signing every browser out is a
consequence an operator asks for, never one a deploy causes.

**Rotating is a values edit**: set `console.auth.uiToken` and upgrade. It wins
over the existing Secret, so it takes effect on an install that already has a
token — it used to be checked last, which made it a silent no-op exactly there.

The trade for stability: once generated, the Secret is no longer part of the
release. Deleting it by hand leaves the console pod unable to start (the Channel
projects it with `envFrom`), and the way back is `console.auth.uiToken`, not
another upgrade. The same trade the CRDs and the persistence claims already make.

The adapter master token behaves identically via `adapterAuth.token` /
`adapterAuth.existingSecret` — with a wider blast radius, since every per-adapter
token is an HMAC of it: changing it 401s every adapter until its pod restarts.

### Letting something else authenticate

With a proxy already in front, the console's own token is a second sign-in that
identifies nobody. Two values retire it — and they must be set together, because
the render fails otherwise:

```yaml
console:
  auth:
    enabled: false
    externalAuthenticator: oauth2-proxy   # cloudflare-access, envoy-ext-authz, …
```

The name is not verified — the chart cannot see what sits in front of it. It is
**recorded**, so "what protects this console?" is answerable from `helm get
values`, from the post-install notes and from review, rather than from memory.

With ingress-nginx and an oauth2-proxy deployed alongside, that is four
annotations on the console Ingress (`console.ingress.annotations`):

```yaml
nginx.ingress.kubernetes.io/auth-url: http://<proxy>.<ns>.svc.cluster.local:8091/oauth2/auth
nginx.ingress.kubernetes.io/auth-signin: https://$host/oauth2/start?rd=$escaped_request_uri
nginx.ingress.kubernetes.io/auth-cache-duration: "200 202 401 5m"
nginx.ingress.kubernetes.io/auth-response-headers: X-Auth-Request-User,X-Auth-Request-Email,X-Auth-Request-Preferred-Username,X-Forwarded-User,X-Forwarded-Email,X-Forwarded-Preferred-Username
```

The last one does **two** jobs, and the second is the one worth understanding:
nginx sets each listed header from the auth subrequest, which also means a
client-supplied copy of it never reaches the console — a header oauth2-proxy
does not send is set empty and therefore dropped. List **all six** the console
trusts, not just the ones your proxy populates. Listing only
`X-Auth-Request-Email` would leave `X-Forwarded-Preferred-Username` passing
straight through from the client, and the console prefers that one first.

The proxy also needs `set_xauthrequest = true`, or its `/oauth2/auth` response
carries no identity for nginx to copy and the console stays read-only.

What this mode requires of the proxy:

- **It must be the only route to the Service.** A port-forward, or any pod in
  the namespace, reaches `agentops-adapter-console:8080` directly and is served
  without question. In a shared namespace, restrict that with a NetworkPolicy.
- **It must STRIP client-supplied identity headers.** The console *believes*
  `X-Forwarded-Email` and its five siblings; it cannot distinguish a header the
  proxy set from one a client sent, since both arrive on the same connection. A
  proxy that passes them through lets a caller choose their own identity. This
  is a requirement of every forward-auth deployment, not a peculiarity of this
  one — the console does not reimplement the check weakly.
- **It should FORWARD one.** Without an identity the console is **read-only**:
  reads are served, writes are refused, and the masthead shows `unknown` with
  the reason. There is no `token` fallback in this mode — no token was proven,
  and a write log naming one asserts something untrue. Every write is recorded
  against a person or it does not happen.

What does **not** change:

- **An empty `uiToken` still authorizes nobody.** "No credential configured" and
  "no credential required" stay independent settings; the entire hazard is one
  being read as the other. A `false` with no authenticator named is half a
  declaration and leaves the console closed — in the chart, which refuses to
  render, and in the pod, which ignores it.
- **The token Secret is still created.** The console Channel projects it with
  `envFrom`, so a missing Secret is a pod that will not start. Re-enabling
  authentication is one value (`console.auth.enabled=true`) and the same token.
- **Authorization is still coarse.** Whoever is in sees everything the
  ServiceAccount can read. Per-user RBAC is not in scope.

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
| `console.auth.enabled` | `true` | whether the console authenticates browsers itself; `false` requires the next value |
| `console.auth.externalAuthenticator` | `""` | **required** when `auth.enabled: false` — names what authenticates instead (see the trust boundary) |
| `console.ingress.enabled` | `false` | see the trust boundary above before enabling |
| `console.ingress.host` | `""` | **required** when enabled — a hostname cannot be guessed |
| `console.ingress.extraHosts` | `[]` | additional hostnames serving the same console; each gets a rule and is covered by the derived certificate |
| `console.ingress.className` | `""` | `ingressClassName` |
| `console.ingress.annotations` | `{}` | merged with the cert-manager annotation when `tls.clusterIssuer` is set |
| `console.ingress.labels` | `{}` | merged onto the Ingress alongside the chart's own |
| `console.ingress.path` | `/` | root only — a non-root value fails the render (see the trust boundary) |
| `console.ingress.pathType` | `Prefix` | `Prefix`, `ImplementationSpecific` or `Exact`; all work at the root |
| `console.ingress.tls.secretName` | `""` | existing certificate Secret; `tls[].hosts` is derived from `host` + `extraHosts` |
| `console.ingress.tls.clusterIssuer` | `""` | cert-manager issuer: adds the annotation and derives `secretName` when unset |
| `console.ingress.tls.existing` | `[]` | raw `tls[]` entries, used verbatim and taking precedence over the derived form |

`console.ingress.tls` also still accepts the pre-6.x **list** form (raw Ingress
`tls` entries), rendered verbatim so existing values files and
`helm upgrade --reuse-values` keep working. The map form above is its
replacement — it derives the hostnames, so a rule host and a certificate host
cannot drift apart.

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
