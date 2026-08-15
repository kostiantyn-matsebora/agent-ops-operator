# Concepts

The full CRD reference, and how an agent's capabilities are resolved.

## The kinds

### AgentProfile

**Who the agent is**: git repository (private OK — SSH key / HTTPS PAT via secretRef), agent role file in that repo (`.claude/agents/<name>.md`), credentials as `env[]` with `valueFrom`, prompts, limits. Carries **no capabilities** — tools and MCP servers come from the Pipeline routing it ([below](#capabilities-are-wiring)). Not addressed directly: conversations address the Pipeline that originates them.

### AgentRuntime

**What executes it**: the engine hosting the LLM agent — image, entrypoint, idle TTL, storage volumes, service account (the agent's RBAC). Ships with a claude-code runtime; any image speaking the [work contract](contracts.md#the-work-contract) plugs in. `profile.runtimeRef` → CR named `default` → manager env. The chart renders the `default` one for you ([below](#the-substrate-runtime-and-globalagentopsruntime)).

Two optional volumes, shaped alike (`pvcRef` / `emptyDir`) and defaulted apart:

| | mounts at | chart default | absent means |
|---|---|---|---|
| `spec.home` | `/data/home` | **on** (`persistence.enabled`) | agent session files die with the pod, so a resume finds nothing and answers without prior context |
| `spec.workspace` | `/data/workspace` | **off** (`persistence.workspace.enabled`) | the repository checkout is re-cloned per pod |

The asymmetry is the point: losing session files silently costs conversational
history, whereas losing a checkout costs a re-clone, which is cheap and always
correct — a stale shared checkout is neither. Turn `workspace` on to keep
uncommitted agent work across a pod restart mid-conversation.

Each conversation gets **its own subdirectory** within the workspace claim,
mounted with a `subPath`, so concurrent runtime pods can never observe or modify
each other's tree. The mount path itself does not move — claude-code keys
sessions by working directory — which is why isolation is bought with `subPath`
rather than with a per-conversation path. Both claims want `ReadWriteMany` once
more than one runtime pod runs at a time; nothing reclaims a conversation's
workspace directory after it is deleted.

#### Runtime images are generic

A runtime differentiates by **vendor backend and trust level, and nothing else**.
The reference image carries git and openssh-client — for the repository checkout,
which is a runtime responsibility — plus generic shell utilities. It carries **no
domain tooling**: image `0.5.0` dropped the `kubectl` binary earlier versions
shipped.

That is the same rule capabilities already follow. What an agent may reach is
wiring — `MCPConfig` servers and `MCPToolset` allowlists bound by a Pipeline — so
a Kubernetes CLI in the vendor layer would be the same category error as bundling
an MCP server in it. It also removes a version pin that could skew against
whatever cluster the image happened to run near.

Practically: an agent reaches Kubernetes through the MCP tools its Pipeline binds
(the [k8s bundle](k8s-bundle.md) ships them), and `Bash` no longer implies cluster
access. Binding `agentops-shell` is still useful for the workspace; it just stops
being a second door to the API.

**If you need a CLI in the runtime**, that is a supported path — `spec.image`
exists so the runtime is swappable. Derive one:

```dockerfile
FROM kmatsebora/agentops-runtime-claude:0.5.0
USER root
RUN curl -fsSL -o /usr/local/bin/kubectl https://dl.k8s.io/release/<ver>/bin/linux/amd64/kubectl \
 && chmod 0755 /usr/local/bin/kubectl
USER node
```

and point an `AgentRuntime` at it. The version pin becomes yours to own against
your cluster, which is the right place for it. Note what you are choosing: that
CLI authenticates as the runtime ServiceAccount, so its reach is that SA's RBAC
handed over whole — one wall, where the MCP path has the server's own identity
*and* the toolset allowlist.

### Conversation

One incident/task: chat topic + agent session + an append-only queue of inputs (task/alert/reply/recurrence), executed strictly serially. `kubectl get conversations` shows phase/thread/runtime live. Phases: `Pending` (waiting for a capacity slot — nothing provisioned, see [below](#capacity-how-many-run-at-once)), `Queued` (admitted, work waiting its turn), `Working`, `Idle`. Ends with `/close` in its thread, or `kubectl delete` — both archive the chat threads first. `/exit` is the other thread command and is not an ending: it [releases the runtime](#releasing-a-runtime-by-hand--exit) and keeps the conversation.

`status.runs[]` records each completed run and **who has been told about it**:
`delivered[]` names the bound channels whose thread already carries the reply,
and `deliveryTracked` marks a run recorded by a manager that maintains those
markers. Together they make the reply derivable rather than queue-resident — see
[Restart resilience](#restart-resilience). Both are materialized state; nothing
sets them by hand.

`status.threads[]` records one binding per bound channel, and each binding
carries **how far that channel has read it**: `readAt` is the point in the
conversation's activity the thread has been seen up to, and `readTracked` marks
a binding created after read reporting existed. See
[Read state](#read-state-per-thread) below.

`spec.pipelineRef` names the Pipeline that **originated** the conversation. It is
**provenance, never wiring**: written once at creation and read for exactly two
things — scoping conversation reuse, and attribution in displays. What a
conversation RUNS with keeps coming from the materialized fields beside it
(`profileRef`, `channelRefs`, `toolsets`, `mcpConfigs`), so editing or deleting
the Pipeline still cannot re-wire a conversation already running.

It exists because [sources are shareable](#sharing-a-source): two Pipelines
listing one source both open a conversation per signal, and those conversations
carry the same signature — without the ref, the second Pipeline's next signal
would be absorbed by the first Pipeline's conversation and run under the wrong
profile with the wrong tools. It also replaces attribution-by-inference, which
went blank exactly when two Pipelines wired identically.

Conversations created before this field have no ref, and nothing backfills one.
Such a conversation stays reusable only while exactly one Ready Pipeline serves
its source — the state it was created in — and is left alone once a second joins.

### ConversationInput

Out-of-line payloads (full alert JSON) so Conversation objects stay small in etcd.

### Channel

Chat surface, split in two parts: **implementation-agnostic metadata** (`adapter`, `credentialsSecretRef` — this surface's transport credentials by *name*) and an **opaque `config`** only the serving channel implementation interprets (schema-less by design). It describes WHERE output goes, never how it is sent. Carries **no wiring** — its default profile comes from the Pipeline referencing it.

### ChannelAdapter

**A channel implementation, nothing more**: a container image implementing the adapter contract, plugged in as a CR whose **name is the type key** — Channels select it with `spec.adapter: <adapter name>`, so one adapter per implementation holds by construction. The reconciler deploys and owns the workload (zero-RBAC SA, no SA token, `replicas 1 + Recreate` when `singleton`), injects a derived per-adapter contract token, and projects every served Channel's credential Secret into the pod (kubelet-resolved — nothing reads Secrets through the API). **All configuration lives on the served Channels** (connectivity, credentials, opaque `config`) — the adapter CR carries none. Publishing a new channel type (Slack, Teams, …) = an image + one CR, zero operator or chart changes. `channel-telegram/` is the reference.

### SignalSource

Ingest lane, split like `Channel`: **implementation-agnostic metadata** (`adapter`, `grouping`, `credentialsSecretRef`) and an **opaque `config`** only the serving signal implementation interprets. Carries **no wiring** — a Ready Pipeline must list it (`Wired` condition; sources no Pipeline lists drop signals with an explicit reason). A source is **shareable**: several Pipelines may list one, the `Wired` condition names them all, and a signal opens a conversation on each — see [Sharing a source](#sharing-a-source). Grouping stays manager-side for every type: signature grouping (same problem → same conversation; recurrence resumes the session) and fingerprint cooldown. **Every signal type is adapter-served** — the manager hosts no signal transports.

`status.cooldown[]` records fingerprint suppression for this source —
`{fingerprint, at}` per admitted signal, pruned past the window and bounded. The
manager keeps an in-memory map as the hot path, but this is the record: it is
read on first use per source after a restart, so a restart mid-incident does not
re-open conversations for signals still inside their window. Only an **admitted**
fingerprint writes; a suppressed re-delivery — the high-volume case cooldown
exists for — writes nothing.

**Signal labels** are what `grouping.signatureLabels` selects over, so what an adapter emits determines what can be grouped. The shared vocabulary — `alertgroup`, `alertname`, `namespace`, `severity`, `source` — is joined by kind-specific keys. The Kubernetes events adapter adds `kind`, `name`, **`workload`** (the owning controller, e.g. `Deployment/api`, resolved through owner references) and `node`, plus the involved pod's own labels. Adapter-defined keys are reserved: a pod label named `name` or `workload` is never allowed to overwrite one, because through `signatureLabels` that would silently rewrite grouping. Suppression — deciding whether a signal is emitted at all — is the **adapter's** job and is distinct from grouping; see [k8s-bundle](k8s-bundle.md#event-suppression-eventsadaptersourcerules).

### SignalAdapter

**A signal implementation, nothing more**: the inbound-only sibling of `ChannelAdapter` — an image implementing the [signal contract](contracts.md#the-signal-adapter-contract), plugged in as a CR whose **name is the type key** SignalSources select via `spec.adapter`, with the same posture (owned workload, zero-RBAC SA, singleton, derived name-scoped token, credential projection). Webhook-receiving implementations declare `spec.port` and the reconciler also owns a Service `agentops-signal-<name>` + injects `LISTEN_ADDR` — enabling the adapter is a complete appliance. A new signal kind (PagerDuty, email, k8s events, …) = an image + one CR. `signal-cron/` is the reference.

#### `servedBy` — two identities, one pod

A SignalAdapter declares **exactly one of** `spec.image` (it runs its own
workload) or `spec.servedBy: {kind: ChannelAdapter, name: <adapter>}` (another
adapter's pod already holds the process, and this CR exists only to give it a
signal identity). The API enforces the either/or, so neither a CR declaring both
nor one declaring neither reaches a controller.

When `servedBy` is set:

- the SignalAdapter reconciler creates **no** Deployment, Service or
  ServiceAccount — and deletes any it used to own, because ownerRef GC will not
  (the owner is still there) and a leftover Deployment is exactly the second pod
  this mode exists to prevent;
- it reports `Ready=True` with reason `ServedBy`. An adapter with nothing to
  become available must not read as a fault on every dashboard. A `servedBy`
  naming a workload that does not exist reports `Ready=False/ServingAdapterMissing`
  and names it — otherwise the adapter would sit Ready while nothing held its
  token, and its sources would look `Served` while nothing served them;
- the named ChannelAdapter's reconciler injects `SIGNAL_ADAPTER_NAME` and
  `SIGNAL_ADAPTER_TOKEN` into its pod, derived exactly as always
  (`HMAC(master, "signal-adapter:"+name)`) — stateless, nothing minted or
  stored. Clearing the link removes them.

A single env var is the whole mechanism. The identities share a **process**,
never a **scope**: the two derivation contexts differ, and each surface still
validates only against its own CRD list, so a channel token remains a stranger
to `/signal/*` and vice versa. `SignalSource.status.Served` resolves identically
either way — the serving relationship is what matters, not where the process
lives.

**When to use it.** When one implementation is genuinely both a *surface* and an
*originator*: it carries conversations on threads AND starts them from a general
surface. That is the shape of every chat transport — Telegram needs three pods
today precisely because one adapter could not be both. The alternative, two
adapter CRs with images, means an idle pod whose only job is to make a source
`Served`; this repo already paid for that shape once, when `telegram-router` was
an adapter with a signal-free `SignalSource` purely to carry a credential, which
then sat at `Wired=False`. The difference that makes `servedBy` legitimate is
that an externally-served source **originates real conversations for a Pipeline
that claims it**.

### Pipeline

**The wiring**: N `signalSourceRefs` × M `channelRefs` + one `profileRef`, plus the agent's **capabilities** (`toolsets` / `mcpConfigs`, see [below](#capabilities-are-wiring)) — the only place they are declared. It is reached two ways and no others: a signal posted to a source it LISTS, and a chat command NAMING it on a wired surface — there is no HTTP form that names a Pipeline. Every referenced source's signals become conversations **mirrored on all referenced channels**, and conversations started from any referenced channel are mirrored everywhere too — each channel gets its own thread, the manager fans agent replies and acks out to all of them, and a user message on one surface is relayed to the siblings as attributed text. **Wiring lives ONLY here**: sources route nothing until a Ready Pipeline lists them.

**Sources are shareable, exactly as channels are** — see [Sharing a source](#sharing-a-source). Name a Pipeline for its PURPOSE, never for the channel it answers on: one chat surface carries many jobs. **No chart bundle ships a Pipeline**: wiring spans bundles, so the chart declares it once at the top, under `pipelines:`.

#### Sharing a source

Listing a `SignalSource` means "I watch this", not "I own this". Any number of Ready Pipelines may list one, of any signal kind, and none of them reports a conflict or loses `Ready`. Whether two agents watch one thing is the adopter's decision.

A signal admitted on a source served by N Ready Pipelines opens **N conversations — one per Pipeline**, each with that Pipeline's own profile, channels and capabilities. Two agents investigating one alert from two angles is a configuration, not a fault.

Per-source ingest policy is evaluated **once, before the fan-out**: a fingerprint passes cooldown once and is then delivered to every server, so the first Pipeline cannot spend the window and starve the rest. `receivedTotal` counts signals received; the ingest response reports `queued` (signals) and `conversations` (one per server) separately.

The `Wired` condition names **every** serving Pipeline, and that count is what to read: it is how many conversations one signal will open, and — on a chat source — whether a bare message is unambiguous.

**Chat is the one lane that does not fan out.** A person asked one question on one surface and is owed one answer, and unlike an alert they can say which agent they meant. So an unaddressed message on a channel's general surface routes only while exactly ONE Ready Pipeline serves the chat source; with several it opens nothing and the surface is answered with the Pipelines available and the `/<pipeline> <task>` form. With none, it drops with the `Wired=False` reason as before. An **addressed** message resolves by name and is unaffected by how many Pipelines serve the source — and thread replies never travel this path at all.

That distinction comes from the arriving signal's `kind`, which ingest already holds. Nothing on a `SignalSource` or a `SignalAdapter` declares "this is a chat source", and no reconciler decides it.

### MCPConfig

Reusable MCP server sets bound per wiring (or a hand-written `mcp.json` via `configMapRef`/`secretRef`, which must be bound alone); secret values via `valueFrom` compile to env placeholders — **the manager never reads agent secrets**.

### MCPToolset

A named, reusable **list of tool patterns** (`spec.tools`) — MCP namespaces like `mcp__victorialogs__*` or built-in tool names like `Bash`. It defines no servers (that stays `MCPConfig`'s job) and has no status: the patterns are opaque strings passed to the runtime. Pipelines bind it to grant a route its tools; the chart ships the built-in vocabulary as three risk-split toolsets.

## The substrate: `runtime:` and `global.agentops.runtime.*`

**The parent chart contributes the substrate; bundles contribute domain.** How
agents execute here — image, LLM credential, idle TTL, node placement, home
volume, and the identity whose RBAC is the agent's power — is a release-wide
fact, the same whether a conversation started from a VictoriaMetrics alert, a
cluster Event or a person typing on Telegram. A bundle contributes signal
sources, profiles, tooling and channels, and references what the parent
provides. No subchart renders an `AgentRuntime`, a runtime ServiceAccount or a
credential Secret.

That is why a chart with no bundle enabled — or with only `telegram-bundle` —
is still a working install. It was not, for two chart majors: the runtime lived
in `k8s-bundle`, so a chat-only install rendered nothing that could execute a
conversation, and a bundle install ended up with two runtime ServiceAccounts,
one of them granted everything.

```yaml
runtime:
  enabled: true                 # false = you manage AgentRuntime CRs yourself
  name: default                 # the name a profile with no runtimeRef resolves
  image: kmatsebora/agentops-runtime-claude:0.5.0
  idleTtlMinutes: ""            # empty = follow runtimeIdleTtlMinutes
  nodeSelector: {}
  resources: {}
  homePvcRef: ""                # only for a claim this chart did not create
  credentialsSecret:
    name: agentops-claude
    key: oauthToken
    envName: CLAUDE_CODE_OAUTH_TOKEN
    token: ""                   # supplied = the chart CREATES the Secret

global:
  agentops:
    runtime:
      serviceAccountName: agentops-runtime
      rbacMode: ""              # none | readonly | full
```

- **One runtime, named `default`.** Additional runtimes — a second vendor, a
  higher-trust identity — stay hand-written CRs, which is what the vendor ×
  trust-level model asks for. A profile points at one with `runtimeRef`.
- **`home.pvcRef` is wired, not copied.** With `persistence.enabled` (or
  `persistence.existingClaim`) the rendered `AgentRuntime` takes the chart's own
  claim. `runtime.homePvcRef` exists only for a claim the chart did not create.
- **Idle TTL has one default.** Empty `runtime.idleTtlMinutes` follows the
  release's `runtimeIdleTtlMinutes`, so there is one number unless you
  deliberately want a second. The chart writes the value out rather than
  omitting the field: `AgentRuntime.spec.idleTtlMinutes` carries a CRD default
  of `10`, so an omitted field is stored as `10` and the manager prefers any
  non-zero spec value over its own setting — omitting it looks right in the
  manifest and is wrong in the stored object.
- **The credential is release-managed or yours.** With `token` set the chart
  creates the Secret; empty, the `AgentRuntime` references it by name and the
  post-install notes say so — the kubelet resolves that reference, so an
  unsatisfied one shows up as `CreateContainerConfigError` on the runtime pod
  and nowhere else.
- **Why `global.`.** A subchart can read no parent scope but `global.`, and
  `k8s-bundle`'s MCP server derives its own identity guard and posture from both
  keys. Restating them in the subchart would make an operator maintain agreement
  between two keys describing one fact.

### `rbacMode` — the agent's in-cluster power

| mode | what binds to the runtime SA |
|---|---|
| `none` | nothing |
| `readonly` | the built-in `view` ClusterRole, plus `get`/`list`/`watch` on `nodes` and `namespaces` and `get`/`list` on `metrics.k8s.io` nodes/pods — the cluster-scoped reads `view` omits |
| `full` | `cluster-admin`: unrestricted cluster control for an LLM-driven agent |
| `""` (default) | `readonly` when `global.demo.enabled`, otherwise `none` |

Empty resolving two ways is deliberate. Defaulting to `readonly` would silently
bind cluster `view` to the runtime SA of every existing install on upgrade, for
a chart whose stated posture is least privilege; defaulting to `none` would
break the promise that demo mode is one flag and a working, cluster-reading
agent. `full` is never selected by any default or inferred path.

`rbac.runtime.{clusterRoles,bindClusterRoles,namespaced}` are **additive** to
the mode — targeted grants on top of a canned posture, named
`<runtime-sa>-<entry>`.

The mode is also the source `k8s-bundle`'s MCP server derives from: `full`
yields a write-capable server under a `full` ServiceAccount, everything else a
read-only server under a `readonly` one. Widening the agent therefore widens
the MCP path unless you say otherwise — see
[k8s-bundle](k8s-bundle.md#kubernetes-as-mcp-tools-mcp--mcpservers) for the
override that recovers the separation.

## Capabilities are wiring

An `AgentProfile` is an identity — repository, agent role, prompts, env, limits.
It declares **no tools and no MCP servers**. What an agent may *do* comes from
the `Pipeline` routing its conversation, which is the only place capabilities
live:

```yaml
kind: Pipeline
spec:
  profileRef: {name: ha-engineer}
  mcpConfigs: {refs: [{name: vm-logs}, {name: vm-metrics}]}   # which SERVERS
  toolsets:   {refs: [{name: vm-observability}]}              # which TOOLS
```

Both halves matter and are independent: `mcpConfigs` without matching
`toolsets` entries yields servers the agent has no permission to call, and vice
versa. Refs are applied in order — tool lists concatenate with dedup, MCP
server keys are overlaid with the later ref winning a collision.

### `toolsets.mode` — what the route composes against

The `toolsets` stanza carries a `mode`, and the thing it composes against is
the **agent's own definition**: the `tools:` frontmatter of
`.claude/agents/<agent>.md` in the profile's repository. Not the profile — the
profile carries no capabilities at all.

```yaml
  toolsets:
    mode: merge        # default: the route ADDS to what the agent declares
    refs: [{name: agentops-shell}]
```

| mode | allowlist the runtime passes |
|------|------------------------------|
| `merge` (default) | the agent's declared tools **∪** the route's, deduped, the agent's keeping their position |
| `overwrite` | the route's tools alone — the agent's declaration does not apply here |

`merge` is the default because it is additive: granting a toolset extends the
agent rather than silently stripping what it declared for itself. An agent
definition that declares no `tools:`, or a profile with no repository,
contributes nothing, so `merge` degrades to the route's tools alone.

The composition happens in the **runtime**, not the manager — the runtime is
the only component with the repository checked out. The manager resolves the
route's half and states the mode; `allowedTools` on the work unit is that half,
not the final answer.

`mcpConfigs` has no mode, deliberately. An agent definition has no field that
declares an MCP *server* — servers reach a run only through the compiled
`mcp.json` — so there is nothing on the other side to compose against, and the
two values would do the same thing.

The chart ships the built-in tool vocabulary as `MCPToolset` CRs, split by risk
(`global.builtinToolsets`): `agentops-observe` (`Read`, `Grep`, `Glob`),
`agentops-shell` (`Bash`), `agentops-edit` (`Edit`, `Write`). One profile can
therefore serve a route that observes and a route that executes, with no
profile edit and no cloning.

**A task addresses a SOURCE; the Pipelines listing it answer.** Programmatic
work is an ordinary signal — `POST /signal/inbound {"source":"...","signals":
[{"fingerprint":"...","kind":"task","payload":"..."}]}` — and every Ready Pipeline
listing that source originates a conversation, supplying the profile, the
mirrored channel set, and the capabilities. A source two Pipelines list produces
two conversations, exactly as it does for an alert. There is no endpoint that names a
Pipeline and no profile-addressed form: a caller picking its own wiring is what
this model exists to prevent, and a request naming only a profile would have no
wiring and therefore nothing to grant. A posted task inherits the target
source's `grouping` like any other signal; with no `signatureLabels` declared it
keys on its own fingerprint, so each post is its own conversation.

**Capabilities are declared, never inferred.** A Pipeline that declares no
`toolsets` gives its conversations none — no default, no inheritance, and no
warning, because an agent that may do nothing is a configuration an operator is
entitled to choose. The corollary is that every Pipeline you write declares what
its route may do, including the ones the chart ships.

Two more rules worth knowing:

- **Refs are snapshotted onto the conversation at creation; content is not.**
  Editing a toolset or fixing a server URL reaches conversations already
  running under it (resolution happens per work unit — no pod restart), while
  re-wiring a pipeline affects only new conversations.
- **A hand-written `mcp.json` cannot be composed.** `MCPConfig` supports
  `configMapRef`/`secretRef` as an escape hatch, but such a config is
  exclusive: binding it alongside another fails with a visible condition rather
  than mounting one side and dropping the other.

Conversations that bind `mcpConfigs` compile into their own ConfigMap
(`agentops-mcp-conv-<conversation>`, garbage-collected with the conversation).

## Capacity: how many run at once

A conversation is **active** while a runtime pod exists for it. That is the only
thing the cap counts — `maxActiveConversations` (env `MAX_ACTIVE_CONVERSATIONS`,
default **5**), measured from the live pod list rather than from conversation
status, so a pod stuck unschedulable or a status patch lost to a conflict cannot
invent capacity. A conversation that finished and let its pod exit is `Idle`: it
costs nothing and does not count, which is what makes a small default safe.

`maxRuntimes` / `MAX_RUNTIMES` is the deprecated spelling, honored for one
release when the new key is unset (the manager logs it at startup).

**Over-cap work queues, it is not dropped.** A conversation that cannot be
admitted gets phase `Pending`: no runtime pod, no chat topic, no MCP ConfigMap,
no dispatch. Suppressing the *topic* is the point — it is what stops a thousand
signals becoming a thousand chat threads before anyone has read the first one.
Its inputs, signature label and pipeline wiring snapshot are kept, so signature
grouping and window reuse treat it exactly like an admitted conversation, and
because the Conversation CR *is* the queue, the backlog survives a manager
restart with no extra CRD.

Admission is **FIFO by creation time**: a pending conversation takes a slot only
when one is free and no older waiter needs it. No priority, no fairness classes
between pipelines. A freed slot is filled at once — the reconciler watches
runtime pod deletions — with a periodic requeue as the backstop. On admission
the conversation proceeds normally: topic, MCP, pod, dispatch.

Since a pending conversation has no thread, one notice goes to the originating
channel's general surface when it enters `Pending`, and only then.

Two bounds keep this honest:

- **`maxQueuedConversations`** (`MAX_QUEUED_CONVERSATIONS`, default **50**)
  caps the backlog itself; beyond it `/signal/inbound` declines to create a
  conversation and reports the batch dropped for capacity — chat senders are
  told on the surface they typed on, alert and job origins are logged. Window
  reuse is unaffected: the bound gates new objects, not new inputs.
- **`runtimeIdleTtlMinutes`** (`RUNTIME_IDLE_TTL_M`) defaults to **1**, so a
  finished conversation returns its slot within a minute instead of holding it
  for ten. `AgentRuntime.spec.idleTTLMinutes` still overrides it per runtime.
  The trade is latency, not memory: the session lives in `/data/home` and
  resumes with its context. An idle pod may also be evicted early to admit
  waiting work — that deletes the pod only; the conversation resumes on its
  next input.

### Releasing a runtime by hand — `/exit`

Eviction covers one half of this: something is waiting, so the longest-idle pod
is taken. The half it cannot cover is when **nothing is waiting yet** — nobody is
blocked, so nothing evicts, and the pod holds its slot, its checkout and whatever
its runtime keeps resident until the idle TTL expires. Installs that *raise* that
TTL, to avoid re-cloning a large repository or re-warming a local model on every
message, are exactly the ones where that wait is longest.

`/exit`, sent in a conversation's own thread, deletes that conversation's runtime
pod and nothing else. The conversation, its threads, its inputs, its run history
and its context handle all survive, and the next message admits it again with a
fresh pod — the same outcome an eviction already produces, reachable by a person.

It is the same release in both directions: `/exit` and eviction share one
definition of idle (`dispatch.NeedsWorker` — nothing inflight, nothing queued),
so a conversation releasable by one is releasable by the other.

It refuses rather than forces:

| Situation | What happens |
|---|---|
| Nothing inflight, nothing queued | Pod deleted; the reply says whether the context survives |
| A run in flight | **Refused**, naming the run and offering `/close` |
| Input queued | **Refused** — the pod would be recreated immediately |
| No pod running | Reported as nothing to release; not an error |

The mid-run refusal is not politeness. Deleting a pod mid-run creates the
replacement at once (the inflight run still needs a worker), hands it nothing
from `/work`, and then idles it out the *long* TTL until it is reaped — which
clears the inflight state, makes the input pending again, and **re-runs work that
may already have acted**. Abandoning a run is `/close`'s job, and `/close` does it
safely by ending the conversation so nothing re-dispatches.

The reply states what the release cost: where the runtime can carry context
across a pod loss (`contextStorage` against the configured home volume) it says
the conversation keeps its memory, and where it cannot it warns that the next
message starts fresh. That is the same loss the idle TTL would have caused — said
at the moment somebody chooses it rather than discovered later.

**`/exit` and `/close` are one word apart and not interchangeable**: `/exit`
releases the runtime and keeps the conversation, `/close` ends the conversation
and archives the thread. `/agents` lists both with that difference.

Consequence worth knowing: a Pipeline named after a manager command (`exit`,
`close`, `agents`, `help`, `start`) cannot be reached by that command. The
interception happens before the Pipeline lookup, which is what makes the commands
reliable.

## Restart resilience

Every piece of live state has **one declared home**, chosen by what the state is:

- **The Kubernetes API** — configuration, conversation state (session id,
  threads, inputs, runs, phase), adapter cursors, delivery markers and
  suppression windows. Recovered by reading; survives any restart and any
  rescheduling.
- **A PersistentVolume** — state that genuinely *is* a filesystem: the runtime's
  agent session files, and optionally its repository checkout.
- **Deliberately lossy** — bounded telemetry whose loss costs history, never
  correctness. It is documented as lossy and it reports its gaps.

**The manager mounts no volume.** Its state either derives from CR state or is
telemetry, and a claim would pin it to one node, defeat rescheduling, and create
a second source of truth beside the CRs.

| Component | State it holds | Home | A restart costs |
|---|---|---|---|
| Manager — reconcilers | none (reads CRs and the live pod list) | Kubernetes API | nothing |
| Manager — op queue | outbound channel ops | derived from CR state | nothing: `ensure-topic` re-derives from a missing thread binding, input cards from unposted inputs, run replies from runs without a delivery marker, `close-topic` from a bound thread missing from `status.threadsArchived[]` ([below](#ending-a-conversation)) |
| Manager — ingest cooldown | fingerprint suppression | `SignalSource.status.cooldown[]` (map is a cache) | nothing inside the window |
| Manager — admission | active/pending counts | live pod list + `Conversation` phase | nothing |
| Manager — activity ring | recent per-hop telemetry | **deliberately lossy** | recent history; clients are told to resync rather than handed silence |
| Runtime pod | agent session files, repo checkout | PVC when enabled, else ephemeral | with `persistence.enabled` off, conversational context; with it on, nothing |
| Channel / signal adapters | transport cursors | annotations on `Channel` / `SignalSource` | nothing |
| Console | config cache, activity index | rebuilt by list→watch and cursor replay | nothing authoritative; it mounts no volume by design |
| Housekeeping job | none (scans disk, reads conversations) | the claims it mounts | nothing: it runs to completion on a schedule and every decision is re-made from scratch |

### What reclaims what, and why the manager cannot

The two-stage lifecycle reclaims the **Kubernetes API** half: autoclose ends a
conversation, autodelete removes the object, and owner references take its
`ConversationInput` objects and MCP ConfigMap with it. Both live in the manager,
which already holds `delete` on conversations and needs no disk to do any of it.

The **disk** half cannot live there. The manager mounts no PersistentVolume by
invariant — a claim would pin it to a node, defeat rescheduling and become a
second source of truth beside the CRs — and reclaiming a workspace directory
requires mounting the claim ROOT, which is exactly the reach `subPath` isolation
denies runtime pods. So an opt-in `housekeeping` CronJob does it, under its own
ServiceAccount with **read-only** access to conversations and no agent code in
its image.

It reclaims two things:

- **Orphan workspace directories** — `<claim>/<name>` where no `Conversation` of
  that name exists.
- **Orphan session transcripts** — files whose session id appears in no
  conversation and which are older than a grace period.

The orderings differ, and each is a correctness argument rather than a
precaution. For workspaces the job scans the disk **first** and lists
conversations **second**: a directory is created by the kubelet mounting a
`subPath` for a runtime pod, and that pod exists only for a conversation that
already exists, so the CR always predates its directory — anything visible in
the scan had a CR before it, and a later listing sees it unless it was genuinely
deleted. Reversing the order would let a conversation created in between look
like an orphan.

For transcripts that argument runs backwards: a conversation's context handle is
written by `POST /work/done`, i.e. **after** the file exists, so a transcript
for a run in flight is unreferenced and perfectly alive. Both a later listing
**and** a grace period longer than the longest plausible run are required.

**A closed conversation is protected for free**, and this is the property the
split rests on: it still has a CR, so the same rule that identifies an orphan
retains its workspace and its transcripts. The job's listing is therefore
**phase-blind on purpose** — an "optimisation" that skipped closed conversations
to look only at live ones would reclaim the state of every conversation an
operator was deliberately keeping.

The two halves never coordinate. Autodelete removes the object, the directory
becomes an orphan, and the job reclaims it on its next run. Enabling autodelete
**without** the job reclaims the API half and leaves the disk — correct with
persistence off, a silent leak with it on.

Adding state to a component means adding its row. State that fits no row is a
defect: it is either a cache of a Kubernetes object, derivable from Kubernetes
objects, or declared lossy.

### What carries a conversation's context

Three records exist for one conversation, and conflating them is the first way to
get this wrong:

| Record | Where | Holds | Authoritative for |
|---|---|---|---|
| **Runtime context** | wherever the runtime keeps it — for the reference runtime, session files on `/data/home` | every message, tool call and model response | **the agent's memory** |
| Thread transcript | the chat surface, via bound channels | what a human said and was told | the human-visible history |
| Run history | `Conversation.status.runs[]` | outcome + result, truncated | what the operator knows |

"Continue with full context" is a property of the **first** row only. The others
are summaries, and neither can reconstruct it — which is why a lost context is
never simulated from run history: 2000-character results with no tool outcomes
would produce an agent that believes it remembers and gives a plausible, wrong
account of what it did.

`status.runtimeContextId` is the runtime's opaque handle for that context. The
manager stores it, hands it back on the next unit, and interprets nothing —
`session` is one backend's noun, not agent-ops'. It is **latest-wins**: a run may
legitimately end in a different context than it was asked to continue, and
keeping the first handle would name something that no longer exists, so every
later message would repeat the same failed continuation and one recoverable loss
would become permanent.

**Continuity is promised only where it is possible.** `AgentRuntime.spec.contextStorage`
declares where a runtime keeps context — `volume`, `external`, or `none` — and a
runtime keeping it on a home volume the deployment does not provide can never
continue anything. Such conversations are single-run **by declaration**: they
answer each message fresh and say so, rather than failing every follow-up for a
configuration the operator chose. A context that was promised and then lost is a
different thing, and fails the run.

#### Storage topologies for continuity

Durable context does **not** require distributed storage:

| topology | how | for |
|---|---|---|
| **Shared** | RWX claim, runtime pods anywhere | Longhorn, EBS-backed RWX, NFS |
| **Single-node** | RWO claim — or a node-affine `local` PersistentVolume — plus `runtime.nodeSelector` pinning runtime pods to that node | clusters with no distributed provisioner |

A `local` PV is the answer when there is no dynamic provisioner at all: create the
PV and claim, set `persistence.existingClaim`, and pin the selector. Note the
trap — an RWO claim with runtime pods *unpinned* works until a second
conversation schedules elsewhere, then fails to attach, far from the setting that
caused it.

The runtime pod is deliberately **not** given a host filesystem path for this: it
executes agent code, and reaching the node's filesystem should not follow from
wanting durable context.

### The reply is a fact, not a queue entry

`POST /work/done` records the run result and enqueues the reply — the fast path,
unchanged. It is no longer the *only* path: reconciliation enqueues a `send` for
any completed run whose result is recorded and whose bound thread carries no
delivery marker, and the reply's op id is stable per
conversation×channel×run, so the two dedup against each other. A manager restart
between the result landing in `status.runs[].result` and an adapter claiming the
op therefore re-derives the answer instead of dropping it.

Marking happens when the op **completes**, which is what makes the two directions
safe: an op lost before completion is re-derived, one completed before the
restart is not, and a fan-out interrupted after one of three threads succeeds
completes the other two without repeating the first.

Runs recorded by a manager older than this mechanism carry no
`deliveryTracked`. They are backfilled as delivered without sending anything —
otherwise upgrading would re-post every recent answer to every bound thread.

### Read state, per thread

A conversation records how far each bound channel has **read** it, on the
binding rather than on the conversation:

```yaml
status:
  lastActivity: "2026-08-13T11:04:00Z"
  threads:
    - channel: telegram
      threadId: "4242"
      readTracked: true
      readAt: "2026-08-13T11:04:00Z"   # read there
    - channel: console
      threadId: console-uid-abc
      readTracked: true                # bound, never read -> unread here
```

Where the transport can tell one reader from another, each binding also carries
`readers[]` — a bounded list of `{key, readAt}`:

```yaml
    - channel: console
      threadId: console-uid-abc
      readTracked: true
      readAt: "2026-08-15T09:00:00Z"    # channel-wide: has anyone seen it
      readers:                          # per identity, bounded at 50
        - key: "sha256:9f2a…"           # opaque; the adapter hashes, salted
          readAt: "2026-08-15T11:04:00Z"
```

`readAt` stays the CHANNEL-WIDE mark, and the list is an OVERLAY on it, because
a Telegram topic is read or it is not and there is nobody to attribute that to.
An adapter that reports no reader keeps reporting only the channel-wide mark and
stays fully conformant.

**The key is opaque to the manager.** The reporting adapter computes it — the
console as a salted hash of whoever is signed in — and the manager stores it
exactly as it stores `threadId` and `runtimeContextId`: verbatim, uninterpreted,
never derived here. So a conversation records THAT someone read it and when,
and never WHO: no address reaches etcd, a backup, or the console's YAML tab.
`/channel/read` refuses a key containing `@`, since it cannot otherwise tell a
hash from a plaintext address.

**A reader with no entry inherits the channel-wide mark** — a teammate who
joined today, and equally one the LRU evicted at 50. That is the `readTracked`
backfill argument one level down: the alternative hands somebody who just
arrived a namespace-sized backlog they can act on none of.

**Who started it has seen it.** `spec.originReader{channel,key}` records the
opaque key of whoever originated a chat conversation, and the manager stamps
that reader's watermark the moment it creates their thread — the same moment it
sets `readTracked`. Read exactly once, and per channel, since keys live in the
originating surface's own key space. Without it a conversation you had just
typed came back to you as unread before any answer could exist.

**The grain is the thread, therefore the channel.** A conversation bound to
Telegram and the console has two audiences reading it in two places, and one
shared mark would let a Telegram reader clear the console's. Reading it on one
channel says nothing about any other.

A thread is **unread**, for a given reader, when `status.lastActivity` is after
that reader's watermark, or when the binding carries `readTracked` and there is
no watermark at all. "That reader's watermark" is their `readers[]` entry, and
the channel-wide `readAt` when they have none. A channel with no binding on a
conversation is never unread there: with no thread there is no watermark and no
claim to make.

The watermark is written **only by the manager**, on an adapter's report to
[`POST /channel/read`](contracts.md#post-channelread) — no adapter and no console
writes it to the Kubernetes API. Two rules make it safe to report from a
browser:

- **Monotonic.** A report at or before the stored value is skipped: not written,
  not an error. Without it two browsers racing — one showing a stale page —
  would un-read a thread the other just cleared.
- **Clamped to the manager's clock.** A report ahead of `now` is written as
  `now`. Without it one client with a skewed clock marks all future activity
  read forever, and nothing arriving later is ever new again. It is the same
  clock that stamps `lastActivity`, which is what the watermark is compared
  against.

Reporting is **optional per adapter**. One that never reports leaves its threads
unread, which is inert for every surface that does not render unreadness — today
that is everything but [the console](console.md#unread).

A binding without `readTracked` **predates the mechanism and is treated as
read**. It cannot be told apart from one nobody has read, and no timestamp can
separate them — the same shape as `status.runs[].deliveryTracked`, taking the
same fix for the same reason. Without it the first upgrade presents every
conversation in the namespace as new. The manager sets `readTracked` on every
binding it creates from that point on, for every channel, so the rule stays one
rule.

### Telemetry says where it lost the thread

The activity ring stays bounded, in-memory and lossy. What it does not do is
present a gap as quiet: a cursor it cannot serve — evicted, or issued by a
previous manager process, since the sequence restarts at 1 — is answered with a
resync, and the console renders that as an explicit gap in its timeline.
Conversations, topology and configuration are unaffected: those are read from
Kubernetes.

## Ending a conversation

`/close` in a conversation's thread ends it. The manager intercepts the command
on the reply path (before it could become an input for the agent), posts a
farewell to **every** bound thread, and deletes the `Conversation`. Any sender
who can post in the thread may use it — no surface in this system authorizes
individual senders, and inventing that here would be the only such check.
`/close` is honored mid-run: the runtime pod goes and the farewell says the
in-progress work was abandoned, because an agent that has gone off the rails is
exactly when the command is wanted. On a channel's general surface, where there
is no conversation to end, it answers with usage and creates nothing.

**Closing is not deletion.** A closed conversation reaches phase `Closed` and
stays there: inert, but intact. The transition tears down the runtime pod and
the `agentops-mcp-conv-<name>` ConfigMap, archives every bound thread with one
`close-topic` op, and releases the capacity slot to whatever is waiting — while
the object, its `status.runs[].result`, its `runtimeContextId` and its volume
state are all left alone. That is what makes it **reopenable**, and it is why
closing is cheap enough to use: the old `/close` destroyed the record, so nobody
reached for it.

Exhaustively, `Closed` means: no runtime pod, no MCP ConfigMap, no dispatch and
no work units; no capacity consumed and no place in the pending backlog; absent
from conversation **reuse**, so a signal whose signature matches opens a NEW
conversation; absent from every pipeline; and `status.closedAt` stamped, which
is the origin of the delete clock. A reply typed into a closed thread is
answered with "this conversation is closed" and creates nothing — an input there
would never dispatch.

Deletion is a **second verb** with its own trigger, window and flag. `kubectl
delete conversation` still works, and the `agentops.dev/close-topics` finalizer
still covers it: it enqueues one `close-topic` op per bound thread that is not
already archived and lets go once they complete, or after a bounded 2-minute
grace so an adapter that is down can never wedge a deletion. Deleting a
conversation that was properly closed finds its threads archived and releases
immediately.

`close-topic` used to be the one operation not derivable from CR state. It no
longer is, and the reason it WAS is worth keeping: it was enqueued while the
object was disappearing, so there was nothing left to record against and a
failure had to be terminal or deletion would wedge. Now that the object survives
its close, `status.threadsArchived` records which threads are done, an
unarchived thread is simply an archive still owed, and reconciliation re-derives
it. Only the deleting path keeps the old protection, from the finalizer's grace.

**Deletion tells the threads.** A closed conversation's threads were told it
could be reopened; deleting it makes that false. Every deletion — autodelete, the
console's verb, or `kubectl delete` — therefore enqueues one
`delete-conversation` operation per bound thread before the finalizer releases,
carrying a notice that the conversation and its record are gone and that a new
message starts a new one. It replaces `close-topic` on that path, so a
conversation being deleted receives one operation, not two. Like `close-topic`
before it, the operation is not re-derivable — the object is disappearing — and
the finalizer's 2-minute grace releases regardless, because a deletion must
never be wedged by an adapter that is down.

### Reopening

A reopen sets the phase back to `Idle`, clears `status.closedAt` — which stops
the delete clock — and leaves every materialized ref **exactly as it was**. The
refs are snapshots whose content is re-read at every use, so a reopen that
re-resolved wiring would do the one thing the snapshot rule forbids: let a
Pipeline edit re-wire a conversation that already exists. A reopened
conversation is the same conversation with the same profile and the same
capabilities, or it is a new conversation wearing an old name.

Continuity is restored where it was **promised**. Under
`AgentRuntime.spec.contextStorage: volume` the workspace directory and the
context handle are both still there, so the agent resumes; under `none` it
answers fresh and says so, exactly as a resume already does. If a referenced
`AgentProfile` or `Channel` no longer exists the reopen fails and names the
missing one, rather than producing a conversation that looks alive and can never
dispatch.

Threads are re-established through an ordinary `ensure-topic` carrying the
archived thread id as a **hint**. An adapter whose transport can un-archive
returns the same id and the conversation continues where it left off (Telegram
does); one that cannot ignores the hint and returns a new id, which is already
correct. A reopened conversation may therefore continue in its old thread on one
channel and a fresh thread on another — that is what those two transports can
actually do, and both are recorded in `status.threads[]`.

### The two windows

Both are off by default, and they measure different things from different
origins:

| Setting | Closes/deletes | Measured from |
|---|---|---|
| `retention.autoclose.idleAge` | a **finished** conversation | **last activity** — the most recent run or input |
| `retention.autodelete.closedAge` | a **closed** conversation | `status.closedAt` |

"Finished" means all of: phase `Idle`, no pending inputs, no inflight run, no
runtime pod, and every recorded run delivered to every bound thread. That last
clause is not decoration — a conversation goes `Idle` the moment its result is
recorded, while the reply may still be an unclaimed `send` op, so closing on
`Idle` alone can archive a thread out from under its own answer.

**The autoclose window is idle time, never lifetime.** A conversation created
last week that answered an hour ago is busy, not old. There is deliberately no
"close as soon as the answer is delivered" mode: the person who just received an
answer is the most likely person to reply to it, and a closed conversation sends
their reply to a new conversation with none of the context. The idle window
protects the follow-up question.

**The record argument belongs to the DELETE window.** `status.runs[].result` is
the only place an answer lives in the Kubernetes API — the console projects its
transcript from the CR, and metrics keep aggregates only — so choose
`closedAge` as "how long do I want to be able to read this", not "how long until
it is tidy". A conversation bound to no channel has no transport copy anywhere.

Both timers are self-scheduled requeues rather than sweeps: the reconciler is
already invoked per conversation with the state the decision needs, so a
periodic list would re-read everything to act on almost nothing. Each spreads
its first pass with jitter, because enabling either on an established install
makes everything eligible at once.

**One gesture, not one implementation per surface.** A surface that ends several
conversations at once — the console's [bulk close](console.md#closing-a-batch) —
sends each of them the same `/close` on the thread it holds, so all of the above
happens per conversation, identically. There is deliberately **no remote close
verb**: no HTTP endpoint, no adapter contract operation and no CRD field ends a
conversation, and a surface's reach is therefore exactly the threads it holds. A
conversation it merely observes cannot be closed from it, because
`/channel/inbound` is reply-only and there is nowhere to post the command.
