# Concepts

The full CRD reference, and how an agent's capabilities are resolved.

## The kinds

### AgentProfile

**Who the agent is**: git repository (private OK — SSH key / HTTPS PAT via secretRef), agent role file in that repo (`.claude/agents/<name>.md`), credentials as `env[]` with `valueFrom`, prompts, limits. Carries **no capabilities** — tools and MCP servers come from the Pipeline routing it ([below](#capabilities-are-wiring)). Not addressed directly: conversations address the Pipeline that originates them.

### AgentRuntime

**What executes it**: the engine hosting the LLM agent — image, entrypoint, idle TTL, home volume (session persistence), service account (the agent's RBAC). Ships with a claude-code runtime; any image speaking the [work contract](contracts.md#the-work-contract) plugs in. `profile.runtimeRef` → CR named `default` → manager env.

### Conversation

One incident/task: chat topic + agent session + an append-only queue of inputs (task/alert/reply/recurrence), executed strictly serially. `kubectl get conversations` shows phase/thread/runtime live. Phases: `Pending` (waiting for a capacity slot — nothing provisioned, see [below](#capacity-how-many-run-at-once)), `Queued` (admitted, work waiting its turn), `Working`, `Idle`. Ends with `/close` in its thread, or `kubectl delete` — both archive the chat threads first.

### ConversationInput

Out-of-line payloads (full alert JSON) so Conversation objects stay small in etcd.

### Channel

Chat surface, split in two parts: **implementation-agnostic metadata** (`adapter`, `credentialsSecretRef` — this surface's transport credentials by *name*) and an **opaque `config`** only the serving channel implementation interprets (schema-less by design). It describes WHERE output goes, never how it is sent. Carries **no wiring** — its default profile comes from the Pipeline referencing it.

### ChannelAdapter

**A channel implementation, nothing more**: a container image implementing the adapter contract, plugged in as a CR whose **name is the type key** — Channels select it with `spec.adapter: <adapter name>`, so one adapter per implementation holds by construction. The reconciler deploys and owns the workload (zero-RBAC SA, no SA token, `replicas 1 + Recreate` when `singleton`), injects a derived per-adapter contract token, and projects every served Channel's credential Secret into the pod (kubelet-resolved — nothing reads Secrets through the API). **All configuration lives on the served Channels** (connectivity, credentials, opaque `config`) — the adapter CR carries none. Publishing a new channel type (Slack, Teams, …) = an image + one CR, zero operator or chart changes. `channel-telegram/` is the reference.

### SignalSource

Ingest lane, split like `Channel`: **implementation-agnostic metadata** (`adapter`, `grouping`, `credentialsSecretRef`) and an **opaque `config`** only the serving signal implementation interprets. Carries **no wiring** — a Pipeline must claim it (`Wired` condition; unclaimed sources drop signals with an explicit reason). Grouping stays manager-side for every type: signature grouping (same problem → same conversation; recurrence resumes the session) and fingerprint cooldown. **Every signal type is adapter-served** — the manager hosts no signal transports.

**Signal labels** are what `grouping.signatureLabels` selects over, so what an adapter emits determines what can be grouped. The shared vocabulary — `alertgroup`, `alertname`, `namespace`, `severity`, `source` — is joined by kind-specific keys. The Kubernetes events adapter adds `kind`, `name`, **`workload`** (the owning controller, e.g. `Deployment/api`, resolved through owner references) and `node`, plus the involved pod's own labels. Adapter-defined keys are reserved: a pod label named `name` or `workload` is never allowed to overwrite one, because through `signatureLabels` that would silently rewrite grouping. Suppression — deciding whether a signal is emitted at all — is the **adapter's** job and is distinct from grouping; see [k8s-bundle](k8s-bundle.md#event-suppression-eventsadaptersourcerules).

### SignalAdapter

**A signal implementation, nothing more**: the inbound-only sibling of `ChannelAdapter` — an image implementing the [signal contract](contracts.md#the-signal-adapter-contract), plugged in as a CR whose **name is the type key** SignalSources select via `spec.adapter`, with the same posture (owned workload, zero-RBAC SA, singleton, derived name-scoped token, credential projection). Webhook-receiving implementations declare `spec.port` and the reconciler also owns a Service `agentops-signal-<name>` + injects `LISTEN_ADDR` — enabling the adapter is a complete appliance. A new signal kind (PagerDuty, email, k8s events, …) = an image + one CR. `signal-cron/` is the reference.

### Pipeline

**The wiring**: N `signalSourceRefs` × M `channelRefs` + one `profileRef`, plus the agent's **capabilities** (`toolsets` / `mcpConfigs`, see [below](#capabilities-are-wiring)) — the only place they are declared. It is ADDRESSABLE by name — `POST /task` names a Pipeline, not a profile. Every referenced source's signals become conversations **mirrored on all referenced channels**, and conversations started from any referenced channel are mirrored everywhere too — each channel gets its own thread, the manager fans agent replies and acks out to all of them, and a user message on one surface is relayed to the siblings as attributed text. **Wiring lives ONLY here**: sources route nothing until claimed. One pipeline per source (older claimant wins); channels are shareable — one chat surface carries many jobs, so name a Pipeline for its PURPOSE, never for the channel it answers on. **No chart bundle ships a Pipeline**: wiring spans bundles, so the chart declares it once at the top, under `pipelines:`.

### MCPConfig

Reusable MCP server sets bound per wiring (or a hand-written `mcp.json` via `configMapRef`/`secretRef`, which must be bound alone); secret values via `valueFrom` compile to env placeholders — **the manager never reads agent secrets**.

### MCPToolset

A named, reusable **list of tool patterns** (`spec.tools`) — MCP namespaces like `mcp__victorialogs__*` or built-in tool names like `Bash`. It defines no servers (that stays `MCPConfig`'s job) and has no status: the patterns are opaque strings passed to the runtime. Pipelines bind it to grant a route its tools; the chart ships the built-in vocabulary as three risk-split toolsets.

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

**A task addresses a Pipeline.** `POST /task {"pipeline":"...","task":"..."}` —
the Pipeline originates the conversation, so it supplies the profile, the
mirrored channel set, and the capabilities. There is no profile-addressed form:
a request that named only a profile would have no wiring, and therefore nothing
to grant. The chart ships an addressable Pipeline per agent so the demo has one
to name.

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

Deletion does the rest through machinery that already exists: owner references
GC the runtime pod and the `agentops-mcp-conv-<name>` ConfigMap, and the freed
slot goes to whatever is waiting. Chat threads are archived first, by the
`agentops.dev/close-topics` finalizer: it enqueues one `close-topic` op per
bound thread and lets go once they complete, or after a bounded 2-minute grace
so an adapter that is down can never wedge a deletion. `kubectl delete
conversation` takes the same path.
