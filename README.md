# Agent Ops Operator

A Kubernetes operator for **agents you can address**: monitoring signals and
direct chat tasks become **Conversations** — each pinned to its own chat topic,
executed by an isolated per-conversation agent pod, resumable across restarts,
approvable from your phone.

> Working name. API group `agentops.dev/v1alpha1` (provisional pre-1.0).

```
  Alertmanager ─┐                                        ┌─▶ Telegram topic per
  cron          ├─▶ SignalSource ─┐                      │   conversation
  k8s events ───┘                 │                      │   (reply = continue,
                                  ▼                      │    approve = act)
  /task API ────────▶        Conversation CR ◀───────────┤
  /<agent> <task> ──▶       (queue of inputs)            │
  in chat                         │                      │
                                  ▼                      │
                          manager (this operator)        │
                          dispatches work units          │
                                  │ long-poll /work      │
                                  ▼                      │
                       agent runtime pod (per convo) ────┘
                       runs the agent CLI, streams
                       transcript to pod logs
```

## Concepts (CRDs)

| Kind | What it defines |
|---|---|
| `AgentProfile` | **Who the agent is**: git repository (private OK — SSH key / HTTPS PAT via secretRef), agent role file in that repo (`.claude/agents/<name>.md`), MCP servers (tri-form: inline / `MCPConfig` refs / raw ConfigMap), credentials as `env[]` with `valueFrom`, tool allowlist, limits. Addressable in chat: `/<profile> <task>`, `/<profile>:<agent>` to pick a role in the repo. |
| `AgentRuntime` | **What executes it**: the engine hosting the LLM agent — image, entrypoint, idle TTL, home volume (session persistence), service account (the agent's RBAC). Ships with a claude-code runtime; any image speaking the [work contract](#the-work-contract) plugs in. `profile.runtimeRef` → CR named `default` → manager env. |
| `Conversation` | One incident/task: chat topic + agent session + an append-only queue of inputs (task/alert/reply/recurrence), executed strictly serially. `kubectl get conversations` shows phase/thread/runtime live. |
| `ConversationInput` | Out-of-line payloads (full alert JSON) so Conversation objects stay small in etcd. |
| `Channel` | Chat surface, split in two parts: **type-agnostic metadata** (`type`, `credentialsSecretRef` — this surface's transport credentials by *name*) and an **opaque `config`** only the serving channel implementation interprets (schema-less by design). It describes WHERE output goes, never how it is sent. Carries **no wiring** — its default profile comes from the Pipeline referencing it. |
| `ChannelAdapter` | **A channel implementation, nothing more**: a container image implementing the adapter contract, plugged in as a CR whose **name is the type key** — Channels select it with `spec.type: <adapter name>`, so one adapter per implementation holds by construction. The reconciler deploys and owns the workload (zero-RBAC SA, no SA token, `replicas 1 + Recreate` when `singleton`), injects a derived per-adapter contract token, and projects every served Channel's credential Secret into the pod (kubelet-resolved — nothing reads Secrets through the API). **All configuration lives on the served Channels** (connectivity, credentials, opaque `config`) — the adapter CR carries none. Publishing a new channel type (Slack, Teams, …) = an image + one CR, zero operator or chart changes. `channel-telegram/` is the reference. |
| `SignalSource` | Ingest lane, split like `Channel`: **type-agnostic metadata** (open `type`, `grouping`, `credentialsSecretRef`) and an **opaque `config`** only the serving signal implementation interprets. Carries **no wiring** — a Pipeline must claim it (`Wired` condition; unclaimed sources drop signals with an explicit reason). Grouping stays manager-side for every type: signature grouping (same problem → same conversation; recurrence resumes the session) and fingerprint cooldown. **Every signal type is adapter-served** — the manager hosts no signal transports. |
| `SignalAdapter` | **A signal implementation, nothing more**: the inbound-only sibling of `ChannelAdapter` — an image implementing the [signal contract](#the-signal-adapter-contract), plugged in as a CR whose **name is the type key** SignalSources select via `spec.type`, with the same posture (owned workload, zero-RBAC SA, singleton, derived name-scoped token, credential projection). Webhook-receiving implementations declare `spec.port` and the reconciler also owns a Service `agentops-signal-<name>` + injects `LISTEN_ADDR` — enabling the adapter is a complete appliance. A new signal kind (PagerDuty, email, k8s events, …) = an image + one CR. `signal-cron/` is the reference. |
| `Pipeline` | **The wiring**: N `signalSourceRefs` × M `channelRefs` + one `profileRef`. Every referenced source's signals become conversations **mirrored on all referenced channels**, and conversations started from any referenced channel are mirrored everywhere too — each channel gets its own thread, the manager fans agent replies and acks out to all of them, and a user message on one surface is relayed to the siblings as attributed text. **Wiring lives ONLY here**: sources route nothing until claimed, channels get their default profile from their oldest Ready pipeline (`/profile` commands work everywhere regardless). One pipeline per source (older claimant wins); channels are shareable. |
| `MCPConfig` | Reusable MCP server sets, shareable across profiles; secret values via `valueFrom` compile to env placeholders — **the manager never reads agent secrets**. |

## Behaviors that matter

- **Conversation = topic = session.** Replying in a topic resumes the same agent
  session; new problems get new topics; the same alert signature within its
  window reuses the existing conversation instead of spamming duplicates.
- **Per-conversation pods, on demand.** A runtime pod spawns when work arrives,
  stays warm across turns, exits after its idle TTL, respawns with full context
  (sessions live on a shared volume). `kubectl logs agentops-conv-<name> -f`
  streams the live agent transcript. Pool cap with idle-eviction.
- **Least privilege by construction.** The manager holds no cluster powers
  beyond its own CRDs + pod lifecycle in its namespace, and never touches agent
  credentials (all `valueFrom`, resolved by the kubelet). Agent powers are
  exactly the runtime service account's RBAC.
- **Structured chat.** Built-in lane prompts embed a six-template message format
  spec (investigation / answer / action report / recurrence / clarification) —
  no stream-of-consciousness walls. Profiles with custom prompts control their own format.

## The channel adapter contract

A channel adapter is a deployment that dials the manager (never the reverse) —
same pattern as runtimes, so NetworkPolicies stay simple and transport
credentials never leave the adapter:

1. Long-poll `GET /channel/ops?type=<your-type>&wait=25` for outbound
   operations: `ensure-topic` (create a thread for a conversation) and `send`
   (post a message; chat HTML subset). Delivery is at-least-once — dedup by
   `op.id`.
2. Complete each op with `POST /channel/ops/{id}/done` — `{"threadId":"…"}`
   for `ensure-topic` (an opaque string in your id space), `{"error":"…"}` on
   failure (surfaced as a Conversation condition and regenerated).
3. Push user messages with `POST /channel/inbound
   {"channel","threadId"?,"text"}` — command parsing, conversation
   creation/adoption, busy-acks all happen manager-side in the shared router.
4. Read your channels + opaque `spec.config` from `GET /channel/channels?type=`,
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
needed — the reference adapter [`channel-telegram/`](channel-telegram/) is
dependency-free Go.

A `Channel` whose type nothing serves (no in-process provider, no Ready
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

1. Read your sources + opaque `spec.config` from `GET /signal/sources?type=`
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
whose type nothing serves carries `Served=False`.

Reference implementation: [`signal-cron/`](signal-cron/) — replaces the old
roadmap `cron` sub-struct: `config: {schedule, input, title?}` fires job-lane
signals with `<source>@<tick>` fingerprints (restart-safe via the state API);
the grouping window turns a recurring job into one conversation whose later
runs resume the agent session.

## VictoriaMetrics bundle (subchart)

`chart/charts/vm-bundle/` packages the VM experience — **off by default,
never enabled by demo mode** (it consumes your VictoriaMetrics endpoints).
Components (individually toggleable once `vm-bundle.enabled=true`):

- **`alertmanager`** — the Alertmanager-webhook ingestion path: a
  `SignalAdapter` CR named `vm-alertmanager` (reference adapter
  `signal-vmalertmanager/`, accepts the standard Alertmanager webhook format)
  with `port: 8080` — the reconciler owns both the workload and the webhook
  Service; the chart ships no connectivity. Sources select it with
  `spec.type: vm-alertmanager`.

  With `registration.enabled=true` (plus the target
  `registration.vmalertmanager: {name, namespace}`) the **adapter configures
  the sender itself**: it writes a `VMAlertmanagerConfig
  agentops-<source>` — webhook receiver pointing at its own endpoint, route
  with `continue: true` so existing receivers keep their alerts — and the
  bundle renders the least-privilege Role/RoleBinding that makes it possible.
  The routing decision lives entirely in the source's `register` block
  (`matchers`, `groupWait`, `groupInterval`, `repeatInterval`, `maxAlerts`,
  `sendResolved`), so it can **replace** a hand-written receiver rather than
  sit beside one. Two things decide whether the replacement actually
  receives anything, and both live on the sender: vm-operator appends these
  routes *after* the ones in your base config, so an earlier route matching
  the same alerts needs `continue: true` or it terminates matching first;
  and it scopes them to their own namespace unless the VMAlertmanager sets
  `spec.disableNamespaceMatcher`.

  Registration failure never unserves the source: the webhook stays live and
  the source's Ready condition names the cause plus the manual step, retried
  every 15s so granting the permission heals it without a restart.

  Without registration, point VMAlertmanager at it yourself:

  ```yaml
  receivers:
    - name: agentops
      webhook_configs:
        - url: http://agentops-signal-vm-alertmanager.<ns>.svc:8080/webhook/<source>
          # recommended: create the source with credentialsSecretRef (Secret
          # key TOKEN) and configure the same bearer token here:
          # http_config: {authorization: {credentials: <token>}}
  ```

  `defaultSource.enabled=true` + `profileRef` renders a turnkey
  SignalSource **and the Pipeline claiming it** (wiring is pipeline-only —
  unclaimed sources drop signals with `Wired=False`). Migration from the
  built-in `alertmanagerWebhook` is per-source: create a new source with the
  new type, claim it in a pipeline, repoint `webhook_configs`, retire the old
  source — both paths can run in parallel during cutover.

- **`mcp.vmlogs` / `mcp.vmmetrics`** — `MCPConfig` CRs (`vm-logs`/`vm-metrics`)
  with fixed server keys `victorialogs`/`victoriametrics` (the keys ARE the
  tool namespaces). URLs point at your MCP servers; `headers` pass through
  with `valueFrom` secret refs for authenticated endpoints.

- **`mcpServers`** (off by default) — optionally deploy the MCP server
  workloads themselves (upstream `ghcr.io/victoriametrics/mcp-victorialogs`
  / `mcp-victoriametrics` images in SSE mode; pin the tags in production).
  Each needs its `backend` (the VictoriaLogs/VictoriaMetrics instance URL);
  with the workloads deployed, empty `mcp.*.url` values default onto the
  deployed Services automatically.

The bundle ships **no profile** — `defaultSource.profileRef` names your own
alert-handling profile, and giving it the MCP tools is one edit on that
profile (the one manual wiring step):

  ```yaml
  spec:
    allowedTools: "Read,Grep,Glob,Bash,mcp__victorialogs__*,mcp__victoriametrics__*"
    mcp:
      configRefs:
        - name: vm-logs
        - name: vm-metrics
  ```

## The work contract

An `AgentRuntime` image must:

1. Long-poll `GET $CONTROL_URL/work?convo=$CONVO_ID&pod=$POD_NAME&wait=25`
2. Execute the returned unit — `promptText` (rendered) or `promptFile`+`promptVars`
   (relative to the checked-out repo at `/data/workspace`) with `resumeSessionId`
   when continuing — streaming progress to **stdout**
3. `POST $CONTROL_URL/work/done {convo, runId, status, sessionId, result}`
4. Exit `0` after `RUNTIME_IDLE_TTL_M` minutes without work

Reference implementation: [`runtime-claude/`](runtime-claude/) (Node.js + claude-code, ~200 lines).
The same bring-your-own pattern applies to chat transports — see the channel
adapter contract above and [`channel-telegram/`](channel-telegram/).

## Try it in five minutes (demo advisor)

A project-agnostic, **read-only k8s-engineer** agent — no chat, no repository,
no MCP setup. One credential, one flag:

```sh
kubectl -n agent-ops create secret generic agentops-claude \
  --from-literal=oauthToken=$(claude setup-token)   # or an Anthropic API key
helm install agent-ops ./chart -n agent-ops --create-namespace --set demo.enabled=true

# ask something
kubectl -n agent-ops run q --rm -i --image=curlimages/curl --restart=Never -- \
  curl -s -X POST http://agentops-manager.agent-ops.svc:8080/task \
  -H 'Content-Type: application/json' \
  -d '{"profile":"k8s-engineer","task":"any pods crashlooping? what should I look at first?"}'

kubectl -n agent-ops get conversations                  # watch it work
kubectl -n agent-ops logs -f agentops-conv-<name>       # live agent transcript
kubectl -n agent-ops get conversation <name> -o jsonpath='{.status.runs[0].result}'
```

The demo binds only the built-in `view` ClusterRole (+ node/namespace reads) to
the runtime SA — a pure advisor. Everything is gated by `demo.enabled` (default
`false`) and removable by flipping it off. Graduating to the real thing =
adding your own `AgentProfile`s (repos, MCP, chat) and declaring agent powers
under `rbac.runtime` values.

## Install (current state)

```sh
helm install agent-ops ./chart -n agent-ops --create-namespace \
  --set persistence.enabled=true   # agent session continuity (PVC, RWX recommended;
                                   # off = sessions are ephemeral per runtime pod)
# CRDs install/upgrade with the release (crds.enabled=true) and carry
# helm.sh/resource-policy: keep (crds.keep=true) — uninstall never deletes your CRs;
# the session PVC is kept on uninstall too.
# Then your site config: secrets, AgentRuntime "default", profiles, Channel, SignalSource
# (see config/samples/ for example CRs)
```

For alert ingestion, enable the [VictoriaMetrics bundle](#victoriametrics-bundle-subchart)
and point any Alertmanager-compatible sender at the adapter's webhook Service
(`continue: true` route). Helm chart, docs site, and public repo extraction are on the roadmap (Phase D).

## HTTP API

| Endpoint | Purpose |
|---|---|
| `POST /task` `{"profile","task","agent"?,"channel"?}` | start a conversation programmatically |
| `GET /work`, `POST /work/done` | runtime-facing dispatch (see contract) |
| `GET/POST /channel/*` | adapter-facing channel contract (bearer token; see adapter contract) |
| `GET/POST/PUT /signal/*` | adapter-facing signal contract (bearer token; see signal adapter contract) |
| `GET /healthz` | liveness |
| `:9090/metrics` | controller-runtime metrics |

## Migrating to chart 1.0 (extensible channels) — BREAKING

Chart 1.0 restructures the `Channel` CRD and moves Telegram out of the manager
into the `channel-telegram` adapter. For a live install:

1. **Stop the old manager** (`kubectl -n <ns> scale deploy agentops-manager
   --replicas=0`) — this stops the in-process poller, freeing the bot token's
   single getUpdates slot.
2. **Migrate Channel CRs** from the typed sub-struct to metadata + config:

   ```yaml
   # before                              # after
   spec:                                 spec:
     telegram:                             type: telegram
       botTokenSecretRef: {…}              defaultProfileRef: {name: …}
       chatId: "-100…"                     config:
       approvers: [1, 2]                     chatId: "-100…"
       pollingEnabled: true                  approvers: [1, 2]
     defaultProfileRef: {name: …}            pollingEnabled: true
   ```

   The bot token secretRef moves out of the CR entirely (the manager reads no
   Secrets at all anymore) — since chart 1.1 it returns as
   `spec.credentialsSecretRef` on the Channel (see below).
3. **Upgrade**: `helm upgrade … --set telegramAdapter.enabled=true`. The new
   CRD applies, the manager restarts without Telegram code, the adapter starts
   as the sole getUpdates consumer (replicas 1, Recreate).
4. `status.threadId` is now a **string** (existing numeric ids remain valid as
   decimal strings) — update anything that parsed it as a number.

Rollback = reverse order: disable the adapter, restore the previous chart
version and Channel CR shape.

## Migrating to chart 1.8 (adapters can configure their senders)

Additive. `SignalAdapter.spec.kubernetesAccess` (default false) mounts the
adapter's ServiceAccount token and injects `POD_NAMESPACE` — **the operator
still grants adapters no RBAC whatsoever**; permissions come from the chart
or you, bound against the deterministic SA `agentops-signal-<name>`. The
vm-bundle uses it for `alertmanager.registration`, which replaces the manual
VMAlertmanager repoint. Existing adapters are untouched.

## Migrating to chart 1.7 (adapters are pure implementation) — BREAKING

`ChannelAdapter`/`SignalAdapter` lose `spec.type` and `spec.env` — the CR
**name** is now the type key (`Channel`/`SignalSource.spec.type` names the
serving adapter), and adapter CRs carry no configuration at all.
`SignalAdapter` gains `spec.port`: when set, the reconciler owns the Service
`agentops-signal-<name>` and injects `LISTEN_ADDR` (the vm-bundle chart no
longer ships one). To upgrade: rename adapter CRs (or re-type
channels/sources) so names and `spec.type` values match — the shipped
`telegram`, `cron`, and `vm-alertmanager` names already do; `spec.type` on
sources/channels is immutable, so a source whose type key changes (e.g.
`vmAlertmanagerWebhook` → `vm-alertmanager`) is deleted and recreated, then
re-claimed by its Pipeline.

## Migrating to chart 1.6 (built-in alertmanager removed) — BREAKING

The manager's `POST /ingest/alertmanager/{source}` endpoint and the built-in
`alertmanagerWebhook` type are gone — the `signal-vmalertmanager` adapter
(vm-bundle) accepts the identical webhook format and is the only Alertmanager
path. BEFORE upgrading: repoint senders to the adapter Service
(`/webhook/{source}`, see the VM bundle section) and move sources to the
adapter's type key claimed by a Pipeline; sender retries plus
fingerprint cooldown make the switchover itself lossless. After upgrading,
retire the old `alertmanagerWebhook` sources.

## Migrating to chart 1.4 (pipeline-only wiring) — BREAKING

`SignalSource.spec.channelRef`/`profileRef` and `Channel.spec.defaultProfileRef`
are removed — wiring exists only on `Pipeline`. Upgrade sequence (order
matters so alert routing never gaps):

1. **Apply a Pipeline first**, claiming every live source with the intended
   profile and channels (the old manager ignores it; the new one requires it).
2. Upgrade to chart 1.4 / manager 0.7. Unclaimed sources now show
   `Wired=False` and drop signals with an explicit response reason; bare
   messages on channels outside any pipeline get usage guidance.
3. Re-apply your CR manifests without the removed fields.

## Migrating to chart 1.3 (Pipelines, multi-channel conversations) — BREAKING

`Conversation.spec.channelRef`/`status.threadId` became `spec.channelRefs[]` /
`status.threads[]` (`{channel, threadId}` per bound channel). Upgrading is
behavior-neutral (no Pipeline CRs = single-channel flows unchanged), but
existing chat-bound conversations lose their binding fields; to keep replying
in their existing topics with session continuity, patch each one:

```sh
kubectl -n <ns> patch conversation <name> --type=merge \
  -p '{"spec":{"channelRefs":[{"name":"<channel>"}]}}'
kubectl -n <ns> patch conversation <name> --subresource=status --type=merge \
  -p '{"status":{"threads":[{"channel":"<channel>","threadId":"<thread>"}]}}'
```

(Unmigrated topics still work — replying triggers re-adoption as a fresh
conversation without the old session.) Mirroring is opt-in per Pipeline CR;
deleting the Pipeline reverts new conversations to source-level routing.

## Migrating to chart 1.1 (ChannelAdapter CR)

Chart 1.1 replaces the chart's Telegram adapter Deployment with a
`ChannelAdapter` CR — the reconciler owns the workload. For a live install
running the chart-1.0 adapter:

1. **Upgrade with the adapter disabled** (`telegramAdapter.enabled=false`, the
   default): helm removes the old Deployment — the bot token's single
   getUpdates slot is free, and the new CRD + manager (with the ChannelAdapter
   reconciler) are in place.
2. **Move the bot token to the Channel**: add
   `spec.credentialsSecretRef: {name: <your bot-token secret>}` (Secret key
   `botToken`) to each telegram Channel. `TELEGRAM_BOT_TOKEN` env remains a
   fallback for hand-deployed adapters only.
3. **Enable the adapter** (`--set telegramAdapter.enabled=true`, or apply your
   own `ChannelAdapter` CR): the reconciler deploys the workload with the
   projected credentials as the sole getUpdates consumer.

Rollback: delete the `ChannelAdapter` CR (the reconciler's Deployment is
GC'd), then redeploy chart 1.0.x whose template restores the old wiring.

## Development

See [CLAUDE.md](CLAUDE.md) for build/test workflow. `go test ./...` covers unit
semantics (grouping, cooldown, dispatch, addressing, MCP compilation) and
envtest integration (real API server: lifecycle, alert routing, runtime selection).

## Status

`v1alpha1` — young but running in production for its author. Roadmap: approve
buttons (inline keyboards), cron + k8s Events signal sources, custom metrics,
Helm chart. License TBD.
