## Repository map

**Self-contained modules.** No dependencies outside this directory. Keep it that
way.

| Count | What |
|---|---|
| **Twelve Go modules** | the operator (root) plus eleven submodules, every submodule dependency-free |
| **Thirteen images** | those twelve plus `runtime-claude/`, which has no `go.mod` |

| The eleven submodules | Are |
|---|---|
| `channel-telegram/`, `console/` | channel adapters — the reference one, and the viewer that is also an adapter |
| `signal-cron/`, `signal-alertmanager/`, `signal-k8s-events/`, `signal-ha/`, `signal-telegram/` | signal adapters |
| `telegram-router/` | neither — the single getUpdates consumer |
| `context-sync/`, `egress-proxy/`, `housekeeping/` | run BESIDE a conversation, serving no CR |

**Count from the repo, not from this sentence.** It was wrong twice, and
`egress-proxy/` was missing from the context entirely.

### The operator

| Path | Holds |
|---|---|
| `api/v1alpha1/` | CRD types plus generated deepcopy. CRD YAML in `chart/files/crds/` |
| `cmd/manager/main.go` | wiring: reconciler, httpapi, chat registry/ops/router, env config |
| `internal/ingest/` | signature grouping, fingerprint cooldown |
| `internal/runtimepod/` | runtime pod builder (AgentRuntime CR over bootstrap Config) |
| `internal/addressing/` | `/<pipeline> <task>` parsing — ONE segment. The `:<agent>` override is deleted: a Pipeline names one profile and a profile names one agent, so the agent comes from the wiring, and a sender picking their own reached past it |
| `internal/integration/` | envtest suite — real API server, fake chat, no kubelet |
| `config/samples/` | example CRs, the only `config/` content. Deployment-specific config belongs with the deployment, never in this module |

**`internal/controller/`** — the Conversation reconciler, plus the adapter and
wiring reconcilers:

- **Topic op enqueue** (async).
- **MCP ConfigMap**, always conversation-owned `agentops-mcp-conv-<name>`.
  Profiles declare no MCP, so there is nothing to collide over.
- **The ADMISSION GATE** — conversation cap → `Pending`, FIFO promotion driven
  by a runtime-pod DELETE watch, idle eviction.
- **The close-topics finalizer**, ownerRef GC, input pruning.
- **ChannelAdapter + SignalAdapter reconcilers** on shared workload machinery.
  `adapterworkload.go` holds ownership, credential projection and the
  type-conflict guard.
- **Channel + SignalSource reconcilers** — the `Served` condition.
- **Pipeline reconciler** — wiring validation ONLY. No source-conflict guard,
  because sources are shareable.

**`internal/httpapi/`** — `/work` long-poll dispatch, `/work/done`, and the
`/channel/*` + `/signal/*` adapter contracts (bearer auth via `ADAPTER_TOKEN`
env).

- **The pending-backlog bound lives here**, in `signals.go`.
- **NO origination endpoint.** `POST /task` is deleted.
- **The signature fallback in `signals.go` splits on LANE.** Do not collapse it
  into one rule.
  - **`alert` / `job`** keep `ingest.DefaultSignatureLabels`, which
    prometheus-bundle and signal-cron depend on.
  - **`task` / `chat`** key on the fingerprint.

**`internal/chat/`** — the channel-type-agnostic core:

| Piece | Is |
|---|---|
| Provider + Registry | in-process built-ins |
| OpQueue | outbound ops, at-least-once: `ensure-topic` \| `send` \| `close-topic` \| `delete-conversation` |
| Router | transport-neutral inbound, `/close` intercepted on the reply path |

**`delete-conversation` REPLACES `close-topic` on the deletion path.** It
reports a FACT rather than a thread instruction, and is named for the
conversation because that is what ended.

**`internal/dispatch/`** — input → work-unit resolution, plus the built-in lane
templates:

- **`templates/format.md` is the mandatory message format spec.** The agent
  writes the SAME markdown subset the outbound contract carries, never HTML —
  adapters escape tags, so a `<b>` reaches chat as literal characters.
- **`EffectiveAllowedTools` is the bound toolsets, per unit.** No profile base,
  no mode.

**`internal/mcpcompile/`** — bound MCPConfigs → `mcp.json` plus `valueFrom` env.

- **ONE entry** — `Compile` over an ordered list.
- **A raw hand-written `mcp.json` is EXCLUSIVE.** Bound with others is an
  error.

### `runtime-claude/`

The reference `AgentRuntime` — Node plus claude-code, implementing the `/work`
contract.

- **GENERIC BY CONSTRUCTION** — git and openssh for the checkout it owns, plus
  generic shell utilities, and NO domain tooling. kubectl was dropped in image
  0.5.0.
- **A CLI here is the same category error as bundling an MCP server.** What an
  agent may reach is WIRING.
- **Need one? Derive an image** and point `AgentRuntime.spec.image` at it
  (README). That is why the field exists.

### The Telegram trio

**`channel-telegram/`** — the reference channel adapter, implementing the
`/channel` contract. Bot API sending lives HERE.

**It does NOT poll.** It receives topic updates pushed by the router
(`POST /updates`, `ChannelAdapter spec.port`) and persists the router's offset
(`GET/PUT /offset` → state API).

**`telegram-router/`** — the ONLY getUpdates consumer.

- **It classifies each update on `is_topic_message` and forwards it VERBATIM.**
  No topic → `signal-telegram` (origination). Topic → `channel-telegram`
  (continuation).
- **It holds no channel config, persists nothing, needs no RBAC.**
- **NOT AN ADAPTER.** It emits no signals, so it has no `SignalAdapter` CR and
  no served CR.
  - **The telegram-bundle chart owns its Deployment**, injecting
    `SIGNAL_TARGET` / `CHANNEL_TARGET` / the bot token as env.
  - **It never contacts the manager.**
- **One Deployment per bot token makes the single-consumer rule structural.** A
  missing env var exits at startup.

**`signal-telegram/`** — the chat ORIGINATION adapter.

It normalizes general-surface updates and posts `/signal/inbound`:

```
{kind: chat, fingerprint: tg-<update_id>,
 labels: agentops.dev/channel + /sender}
```

**It holds NO credentials** — it never contacts Telegram.

### The other signal adapters

**`signal-cron/`** — the reference signal adapter, implementing the `/signal`
contract. Five-field cron parser plus scheduler.

**`signal-alertmanager/`** — webhook-receiving signal adapter. Hosts `/webhook/{source}` for Alertmanager-format posts, and the
prometheus-bundle subchart ships it.

- **The pod label `agentops.dev/signal-adapter` is a CHART CONTRACT**, pinned by
  integration test.
- **RENAMED from `signal-vmalertmanager` in chart 5.24.0.** It reads the
  STANDARD Alertmanager payload, which vanilla Alertmanager and VictoriaMetrics
  both send, so the vendor name described one sender rather than the component.
- **The published image was renamed with it.** The old one is left in place for
  installs pinned to an older chart, never deleted.

**THE LINE FOR VM NAMING, everywhere in this repo: rename what names OUR
component, KEEP what names a VictoriaMetrics API OBJECT.**

- `register.go` stays full of `VMAlertmanagerConfig` and
  `operator.victoriametrics.com` because it WRITES that object, and vanilla
  Alertmanager has no object to write — its config is a file.
- `metrics.yaml` keeps `VMServiceScrape` / `VMRule` and the
  `metrics.vmServiceScrape` value on the same grounds.
- **Renaming either would name a thing that does not exist.**

**`signal-ha/`** — Home Assistant log signal adapter.

- **Reads that instance's WebSocket API** over a hand-written RFC 6455 client:
  `system_log_event`, with `system_log/list` for backfill and for the dwell
  re-check's evidence.
- **NO Kubernetes client at all.** `kubernetesAccess: false`, credential
  projected per SOURCE.
- **Same `rules`/`route` vocabulary as `signal-k8s-events`**, minus the time
  axis.
- **The fingerprint keys on LOGGER + SOURCE LOCATION** — Home Assistant's own
  dedup identity — never on the occurrence.
- **Verification ladder:** config-entry state, then recurrence.
- **Its loop breaker is the agent SURFACE** (`mcp_server`, `api`,
  `websocket_api`), because a failed agent call is logged there and reporting it
  would wake the agent that made it.

**`signal-k8s-events/`** — cluster Events signal adapter.

- **In-cluster API over `net/http`**, no client-go: SA token re-read, list+watch
  per namespace scope, 410 relist.
- **Needs `kubernetesAccess: true`.** The CHART binds its events RBAC — the
  operator grants adapters nothing.
- **The fingerprint keys on the involved OBJECT + reason**, never the Event
  object. Kubernetes recreates those per recurrence.

### Beside a conversation

**`housekeeping/`** — the disk half of conversation retention.

- **A CronJob, not a daemon.** It scans the claim ROOTS for workspace
  directories and session transcripts no Conversation backs, and removes them.
- **READ-ONLY on the API** — both etcd-side stages are the manager's — with its
  OWN SA and no agent code in the image.
- **Its own workload because THE MANAGER MOUNTS NOTHING** — see the invariant.
- **Every run is bounded** (`maxDeletions`) and `dryRun`-able.
- **The listing is PHASE-BLIND** — see the invariant.
- **Named `agentops-housekeeping`** so `signal-k8s-events`' prefix
  self-exclusion catches it. A CronJob fails on a SCHEDULE.

**`egress-proxy/`** — the tool-access wall INSIDE the runtime pod.

- **ONE binary, two subcommands:** `install-redirect` (privileged init
  container, writes the redirect rules) and `proxy` (serves the redirected
  connections). One binary because the two must agree exactly on the port and
  the uid, and a second image is a second place for that agreement to rot.
- **It exists because `--allowedTools` configures a COOPERATING agent.** One
  with a shell can open a socket to a bound MCP server and call anything that
  server registers. This is the wall for an agent that does not cooperate.
- **It terminates no TLS, inspects no non-MCP byte and holds no Kubernetes
  credential.** Other traffic is copied through untouched.
- **Opt-in via `runtime.egressMediation.enabled`** — see
  `docs/adr/0001-bound-component-reach.md`.

**`context-sync/`** — the context sidecar. Semantics in the terminology entry;
what lives HERE is the implementation:

- **Atomic generations plus a `current` symlink.** Copies are labelled quiesced
  or best-effort.

### `console/`

**The agent-ops console** — a ChannelAdapter that is ALSO the viewer.

- **Config from read-only list/watch of the eight agentops kinds**, in-cluster
  API over `net/http`, the same technique as `signal-k8s-events`.
- **Conversation traffic from the ordinary `/channel/*` contract.**
- **Embedded SPA via `go:embed`** — no npm at runtime.
- **NO write path to the Kubernetes API exists in the module.** The only write
  anywhere is `POST /channel/inbound`.
- **Needs `kubernetesAccess: true`** for identity, plus the CHART's read-only
  Role against SA `agentops-adapter-console`.
- **Conversations carry no `pipelineRef`**, so pipeline attribution is INFERRED
  from the materialized bindings and left blank when ambiguous. Never guessed.
