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
| `Channel` | Chat surface, split in two parts: **type-agnostic metadata** (`type`, default profile, delivery hints) and an **opaque `config`** only the serving channel implementation interprets (schema-less by design). Channel types are served by out-of-process **channel adapters** — the operator ships `channel-telegram/` as the reference; bring your own for Slack/Teams/… with zero operator changes. |
| `SignalSource` | Ingest lane: `alertmanagerWebhook` (implemented) / `cron` / `k8sEvents` (roadmap) with signature grouping (same problem → same conversation; recurrence resumes the session) and fingerprint cooldown. |
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

All `/channel/*` calls carry `Authorization: Bearer $ADAPTER_TOKEN` (the chart
provisions the shared token Secret into the manager and every adapter). No
Kubernetes API access needed — the reference adapter
[`channel-telegram/`](channel-telegram/) is dependency-free Go.

Delivery of agent answers is channel-metadata-driven: by default the agent's
printed answer is the deliverable (captured via `/work/done`); a channel may
set `spec.delivery.mode: agent` plus `agentInstructions` to have the agent
post to the chat surface itself (the Telegram sample does this).

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

Point Alertmanager at `http://agentops-manager.<ns>.svc:8080/ingest/alertmanager/<signalsource-name>`
(`continue: true` route). Helm chart, docs site, and public repo extraction are on the roadmap (Phase D).

## HTTP API

| Endpoint | Purpose |
|---|---|
| `POST /task` `{"profile","task","agent"?,"channel"?}` | start a conversation programmatically |
| `POST /ingest/alertmanager/{source}` | Alertmanager webhook |
| `GET /work`, `POST /work/done` | runtime-facing dispatch (see contract) |
| `GET/POST /channel/*` | adapter-facing channel contract (bearer token; see adapter contract) |
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

   The bot token secretRef moves out of the CR entirely — it becomes
   `telegramAdapter.botTokenSecret` in chart values (mounted into the adapter;
   the manager reads no Secrets at all anymore).
3. **Upgrade**: `helm upgrade … --set telegramAdapter.enabled=true`. The new
   CRD applies, the manager restarts without Telegram code, the adapter starts
   as the sole getUpdates consumer (replicas 1, Recreate).
4. `status.threadId` is now a **string** (existing numeric ids remain valid as
   decimal strings) — update anything that parsed it as a number.

Rollback = reverse order: disable the adapter, restore the previous chart
version and Channel CR shape.

## Development

See [CLAUDE.md](CLAUDE.md) for build/test workflow. `go test ./...` covers unit
semantics (grouping, cooldown, dispatch, addressing, MCP compilation) and
envtest integration (real API server: lifecycle, alert routing, runtime selection).

## Status

`v1alpha1` — young but running in production for its author. Roadmap: approve
buttons (inline keyboards), cron + k8s Events signal sources, custom metrics,
Helm chart. License TBD.
