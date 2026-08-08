# Claude context — agent-ops-operator

Go/controller-runtime Kubernetes operator (see README.md for the product view).
Self-contained modules — no dependencies outside this directory; keep it that
way. Seven Go modules: the operator (root), `channel-telegram/` (reference
channel adapter), `telegram-router/` (the single getUpdates consumer), and
`signal-cron/`, `signal-vmalertmanager/`, `signal-k8s-events/`,
`signal-telegram/` (signal adapters) — the adapters dependency-free.

## Terminology (binding)

- **Agent runtime**, never "worker": CRD `AgentRuntime`, SA `agentops-runtime`,
  env `RUNTIME_*`, pkg `runtimepod`, pods `agentops-conv-<conversation>`.
- `AgentProfile` = who the agent is — identity ONLY (repo, role, prompts, env,
  limits). It carries NO capabilities: no `allowedTools`, no `mcp`. What an
  agent MAY DO comes exclusively from the Pipeline routing it.
  `AgentRuntime` = what executes it;
  `Conversation` = session + serial input queue + one thread PER bound channel
  (`spec.channelRefs[]` / `status.threads[]{channel,threadId}`).
  `Conversation.spec.toolsets`/`.mcpConfigs` mirror the originating Pipeline's
  bindings — MATERIALIZED state like `profileRef`/`channelRefs`, never hand-set,
  no `pipelineRef` exists. REFS are snapshotted, CONTENT is not: every use
  re-reads the CRs, so edits heal running conversations while re-wiring affects
  only new ones. EVERY origination now has a Pipeline to mirror: signals from
  the claiming one, `POST /task` from the one it names, and a `/<pipeline>
  <task>` chat command from the one it addresses. Nothing creates a
  Conversation without wiring behind it.
- **`Pipeline`** = THE wiring, exclusively: sources[] × channels[] + profile
  + TOOL ACCESS. No other CR carries wiring (SignalSource has no
  profile/channel refs, Channel has no default profile) — unclaimed sources
  DROP signals (`Wired=False` + response reason; for a CHAT source the reason
  also goes back to the surface the person typed on, because they are waiting).
  Channels originate NOTHING, so there is no "unwired channel" behavior to
  define: an unclaimed chat source is the unwired case. One pipeline per source
  (older claimant wins), channels shareable, Ready pipelines only.
  **Capabilities are wiring, exclusively**: two optional stanzas of ordered
  refs — `spec.toolsets` (→ `MCPToolset`, the allowlist) and `spec.mcpConfigs`
  (→ `MCPConfig`, the MCP servers). NO mode: with nothing profile-side to
  compose against, merge/overwrite would be one behavior wearing two names.
  Refs in order: tools concatenate with dedup, server keys overlay (later wins).
  Content stays in the referenced CRs; Ready validates both ref sets.
  **ADDRESSABLE**: `POST /task {"pipeline": X, "task": ...}` names a Pipeline and
  takes its profile, channels, and capabilities from it. There is no
  profile-addressed form and no per-profile default — a Pipeline declaring no
  bindings grants nothing, and that is a configuration, not a defect to warn
  about. Every Pipeline the CHART ships must therefore declare its own tools;
  forgetting that is what made every signal-driven conversation toolless once.
  Consequence: runtimes are generic — one `AgentRuntime` per vendor × trust
  level (the SA stays runtime-level on purpose; a Pipeline choosing an SA
  would make pipeline-edit rights a privilege escalation).
- **`MCPToolset`** = a pure LIST of tool patterns (`spec.tools`), no servers,
  no status — patterns are opaque, passed through like `allowedTools`. Servers
  live ONLY in `MCPConfig`. Manager RBAC on it is read-only.
  Bound from `Pipeline.spec.toolsets` ONLY — capabilities are wiring, never
  profile fields. The chart ships the built-in vocabulary risk-split under
  `global.builtinToolsets` (`agentops-observe` / `-shell` / `-edit`); `global.`
  because subcharts read no other parent scope.
  `POST /task {"pipeline": X}` carries X's bindings — channels AND tooling both;
  naming a pipeline asks for its wiring, not half of it.
  Multi-channel conversations: manager fans replies/acks to every bound
  thread, relays user messages to sibling channels as attributed text, and
  dispatches once ≥1 thread binding exists.
  **The OPERATOR delivers** — agent output is posted to every bound thread by
  the manager via the adapters, for single- and multi-channel alike. Agents
  never post to a transport (no delivery mode on Channel), so prompts carry no
  transport steps and runtimes hold no channel credentials.
- **Channel adapter** = out-of-process channel-type implementation consuming
  `/channel/*` (ops long-poll + inbound push). `Channel.spec` = type-agnostic
  metadata (`adapter`, `credentialsSecretRef` — NO wiring, NO delivery mode)
  + opaque `config` that only the serving adapter interprets.
  `status.threadId` is an opaque STRING.
- **`ChannelAdapter` CR** = pure implementation (`image` + workload knobs,
  NEVER configuration or credentials — no `type`, no `env`). Interface
  METADATA is allowed and encouraged: optional `configSchema` (JSON Schema for
  the served CRs' `config`) + `credentialKeys` (docs only — the manager reads
  no Secrets). No config VALUES, connectivity, or credentials. Its reconciler
  owns the adapter Deployment. Credentials are per-surface on
  `Channel.credentialsSecretRef`, projected into the adapter pod as `envFrom`
  with prefix `AGENTOPS_CRED_<CHANNEL>_` (kubelet-resolved; the contract's
  channel listing advertises `credentialEnvPrefix`).
- **`SignalAdapter` CR / signal adapter** = same pattern for ingest, but
  inbound-only (no ops queue): adapters push normalized signals
  (`fingerprint`, `labels`, `title?`, `payload`, `kind: alert|job`) to
  `/signal/inbound`; grouping/cooldown/recurrence stay MANAGER-side from
  `SignalSource.spec.grouping` — adapters normalize, the manager groups.
  Workload names `agentops-signal-<name>`; token derivation context
  `signal-adapter:<name>` (never interchangeable with channel tokens).
  There are NO built-in signal types — the manager hosts no signal
  transports; every type needs a serving adapter. `SignalAdapter.spec.port`
  (implementation property): when set, the reconciler owns the Service
  `agentops-signal-<name>` and injects `LISTEN_ADDR` — charts ship NO
  adapter connectivity. `spec.kubernetesAccess`: mounts the SA token +
  `POD_NAMESPACE` for implementations that self-register with their SENDER
  (push-model senders hold the "where to push" binding; the adapter writes it
  from `SignalSource.spec.config.register`, degrading to instructions in the
  Ready condition when it can't).
- On both adapter kinds, the CR NAME is the ROUTING KEY:
  `Channel`/`SignalSource.spec.adapter` names the serving adapter — a
  REFERENCE, not an attribute: that adapter's implementation defines the schema
  of the sibling `config` (drives the contract listing `?adapter=`, the
  injected `ADAPTER_NAME`, credential projection, token scope, Served) — one
  adapter per implementation by construction; duplicate adapters for one
  implementation are impossible. Adapter CRs carry no configuration —
  connectivity/creds/config live ONLY on Channel/SignalSource.
- API group `agentops.dev/v1alpha1` (provisional; rename possible pre-1.0).

## Build / test

```sh
go build ./... && go vet ./...
for m in channel-telegram telegram-router signal-telegram signal-cron \
         signal-vmalertmanager signal-k8s-events; do
  (cd $m && go build ./... && go vet ./... && go test ./...)
done
# regen after editing api/v1alpha1/ (deepcopy + CRDs):
go run sigs.k8s.io/controller-tools/cmd/controller-gen@v0.16.5 object paths=./api/...
go run sigs.k8s.io/controller-tools/cmd/controller-gen@v0.16.5 crd paths=./api/... output:crd:artifacts:config=chart/files/crds
# full tests (unit + envtest against a real API server):
KUBEBUILDER_ASSETS=$(go run sigs.k8s.io/controller-runtime/tools/setup-envtest@release-0.19 use 1.31.x --bin-dir ~/.envtest -p path) go test ./...
```

Images (bump the tag on every change — never overwrite a pushed tag):

```sh
docker build --platform linux/amd64 -t <registry>/agentops-manager:<tag> .
docker build --platform linux/amd64 -t <registry>/agentops-runtime-claude:<tag> ./runtime-claude/
docker build --platform linux/amd64 -t <registry>/agentops-channel-telegram:<tag> ./channel-telegram/
docker build --platform linux/amd64 -t <registry>/agentops-telegram-router:<tag> ./telegram-router/
docker build --platform linux/amd64 -t <registry>/agentops-signal-telegram:<tag> ./signal-telegram/
docker build --platform linux/amd64 -t <registry>/agentops-signal-cron:<tag> ./signal-cron/
docker build --platform linux/amd64 -t <registry>/agentops-signal-vmalertmanager:<tag> ./signal-vmalertmanager/
docker build --platform linux/amd64 -t <registry>/agentops-signal-k8s-events:<tag> ./signal-k8s-events/
# then update the image refs (chart values for the manager, AgentRuntime CRs for
# runtimes), helm upgrade, and verify with a live task:
#   POST /task {"profile":"stub","task":"..."}   (stub runtime = no LLM cost)
```

## Map

```
api/v1alpha1/            CRD types (+ generated deepcopy); CRD YAML in chart/files/crds/
cmd/manager/main.go      wiring: reconciler, httpapi, chat registry/ops/router, env config
internal/
  controller/            Conversation reconciler: topic op enqueue (async), MCP
                         ConfigMap (always conversation-owned
                         agentops-mcp-conv-<name>; profiles declare no MCP, so
                         there is nothing to collide over), runtime-pod pool (cap + idle eviction),
                         ownerRef GC, input pruning; ChannelAdapter +
                         SignalAdapter reconcilers on shared workload machinery
                         (adapterworkload.go: ownership, credential projection,
                         type-conflict guard); Channel + SignalSource
                         reconcilers (Served condition); Pipeline reconciler
                         (wiring validation, source-conflict guard)
  httpapi/               /work long-poll dispatch, /work/done, /task,
                         /channel/* + /signal/* adapter contracts
                         (bearer auth via ADAPTER_TOKEN env)
  chat/                  channel-type-agnostic core: Provider+Registry
                         (in-process built-ins), OpQueue (outbound ops,
                         at-least-once), Router (transport-neutral inbound)
  dispatch/              input → work-unit resolution + built-in lane templates
                         (templates/format.md = mandatory message format spec);
                         EffectiveAllowedTools = the bound toolsets, per unit
                         (no profile base, no mode)
  ingest/                signature grouping, fingerprint cooldown
  mcpcompile/            bound MCPConfigs → mcp.json + valueFrom env; ONE entry
                         (Compile over an ordered list). A raw hand-written
                         mcp.json is EXCLUSIVE — bound with others = error
  runtimepod/            runtime pod builder (AgentRuntime CR over bootstrap Config)
  addressing/            /<profile>[:<agent>] parsing
  integration/           envtest suite (real API server, fake chat, no kubelet)
runtime-claude/          reference AgentRuntime (Node + claude-code) — /work contract
channel-telegram/        reference channel adapter (own module, no deps) —
                         /channel contract; Bot API sending lives HERE. Does
                         NOT poll: receives topic updates pushed by the router
                         (POST /updates, ChannelAdapter spec.port) and persists
                         the router's offset (GET/PUT /offset -> state API)
telegram-router/         the ONLY getUpdates consumer (own module, no deps) —
                         classifies each update on is_topic_message and
                         forwards it VERBATIM: no topic -> signal-telegram
                         (origination), topic -> channel-telegram
                         (continuation). Holds no channel config, persists
                         nothing, needs no RBAC. Reads ONE thing: its own
                         SignalSource, for forwarding targets + the shared bot
                         Secret's env prefix
signal-telegram/         chat ORIGINATION adapter (own module, no deps) —
                         normalizes general-surface updates to
                         {kind: chat, fingerprint: tg-<update_id>, labels:
                         agentops.dev/channel + /sender} and posts
                         /signal/inbound. Holds NO credentials — it never
                         contacts Telegram
signal-cron/             reference signal adapter (own module, no deps) —
                         /signal contract; five-field cron parser + scheduler
signal-vmalertmanager/   webhook-receiving signal adapter (own module, no
                         deps) — hosts /webhook/{source} for Alertmanager-
                         format posts; vm-bundle subchart ships it + Service
                         (pod label agentops.dev/signal-adapter is a CHART
                         CONTRACT, pinned by integration test)
signal-k8s-events/       cluster Events signal adapter (own module, no deps) —
                         in-cluster API over net/http (no client-go): SA token
                         re-read, list+watch per namespace scope, 410 relist.
                         Needs kubernetesAccess: true; the CHART binds its
                         events RBAC (the operator grants adapters nothing).
                         Fingerprint keys on involved OBJECT+reason, never the
                         Event object — k8s recreates those per recurrence
chart/charts/k8s-bundle/ subchart: cluster Events lane (adapter + RBAC +
                         SignalSource AND its claiming Pipeline — never one
                         without the other), k8s-engineer profile + runtime +
                         SA, and that SA's RBAC (readonly | full=cluster-admin).
                         Self-gated on `enabled OR global.demo.enabled`; demo
                         mode IS this bundle (chart/templates/demo.yaml is gone)
chart/charts/telegram-bundle/
                         subchart: the three-component Telegram stack (router +
                         signal adapter + channel adapter) as adapter CRs, and
                         — under `surface.enabled` — the Channel, the chat
                         SignalSource, and the router's credential source.
                         surface.enabled makes the unguessable fields REQUIRED:
                         missing chatId, missing credential, or BOTH credential
                         forms at once FAIL the render. Credentials either way:
                         `credentials.existingSecret` OR `credentials.botToken`
                         (bundle creates the Secret). One Secret serves both —
                         the Channel sends with it, the router's source polls.
                         Ships NO Pipeline on purpose (wiring drags in a profile
                         + runtime + creds): the sources sit unclaimed until the
                         installer wires them, so NOTES.txt prints the exact
                         Pipeline to apply
chart/                   Helm chart: manager Deployment/RBAC/Service + CRDs as gated
                         templates (crds.enabled, crds.keep -> helm.sh/resource-policy:
                         keep so uninstall never cascade-deletes CRs); CRD source of
                         truth = chart/files/crds/ (controller-gen output)
config/samples/          example CRs (the only config/ content — deployment-specific
                         config belongs with the deployment, never in this module)
```

## Invariants (do not break)

- **The manager reads NO secrets — zero Secret API reads.** Everything
  secret-shaped compiles to `valueFrom`/`envFrom` in pod specs (the kubelet
  resolves it); transport credentials are declared per Channel
  (`credentialsSecretRef`) and PROJECTED into adapter pods, never read; the
  adapter auth token reaches the manager via env (`ADAPTER_TOKEN`), and
  per-adapter tokens are DERIVED (HMAC of master + adapter name, validated by
  re-derivation — nothing minted or stored). RBAC grants the manager no
  `secrets` verbs at all — keep it that way.
- **The operator grants adapters NO Kubernetes permissions, ever**: dedicated
  SA, no RBAC objects created or bound by any reconciler. Default posture is
  `automountServiceAccountToken: false`; `SignalAdapter.spec.kubernetesAccess`
  only mounts the token + injects `POD_NAMESPACE` — what it may DO is granted
  externally (chart/user) against SA `agentops-signal-<name>`, so an adapter
  CR can never escalate. Name-is-key makes one adapter per implementation
  structural — there is no conflict machinery to maintain.
- **Strictly serial per conversation** (one inflight unit); parallelism is
  across conversations, capped by `MAX_RUNTIMES` with idle-runtime eviction.
- **CONVERSATIONS ORIGINATE ONLY FROM CLAIMED SIGNAL SOURCES.** A channel
  CARRIES conversations; it never starts one. `/channel/inbound` is
  reply-only — `threadId` REQUIRED, unknown threads dropped, no adoption. A
  message on a chat's general surface arrives as a `kind: chat` signal from a
  chat `SignalSource`, so who answers is DECLARED by the Pipeline claiming it.
  There is no channel default profile and no `PipelineForChannel` — channels
  are shareable on purpose, so "which pipeline answers for this channel" has no
  defensible answer, and the oldest-Ready tiebreak that used to supply one is
  gone. Chat lane: task inputs (never `job` — that resumes sessions), cooldown
  OFF by default, and NO signature grouping unless `signatureLabels` is set
  (chat keys on the fingerprint; the default alert labels would hash every
  message alike into one conversation). Commands whose whole result is a reply
  (`/agents`, unknown agent, usage error) emit a send op and create nothing.
  A chat signal MUST carry `agentops.dev/channel` — `/signal/inbound` refuses
  one it could not answer.
- **HTTP API is NOT leader-gated** (`NeedLeaderElection()=false`) — webhooks
  must serve during rollouts. **Exactly one getUpdates consumer per bot token,
  ever** — that consumer is `telegram-router`, ONE poll loop per distinct token,
  single-instance via SignalAdapter `singleton` (replicas 1 + Recreate).
  Neither adapter polls and the manager has no poller. Adding a poll loop back
  to `channel-telegram` is the mistake that produces 409s and stolen updates.
- **Channel ops are at-least-once.** `spec.config` is opaque to the operator —
  never parse channel-type config manager-side; adapters validate their own
  and report via the Channel Ready condition. The manager never *interprets*
  config, but it MAY apply an adapter-declared `configSchema` mechanically
  (`internal/configschema`, the one place config content is touched):
  advisory-only `ConfigValid`, no type knowledge, adapter stays authoritative.
- **No relay loops**: channel implementations (adapters AND in-process
  providers) must never re-ingest their own outbound posts as inbound —
  cross-channel relay depends on it.
- Runtime pods: ownerRef → Conversation (GC); repo checkout at
  **`/data/workspace`** (claude-code sessions are keyed by cwd — moving this
  path breaks session resume); `/data/workspace` and `/data/home` are mount
  points — **clear contents, never rmdir**.
- Dispatch/ingest semantics are pinned by test fixtures — change behavior by
  changing tests deliberately, not incidentally.

## Gotchas (paid for in debugging)

- RBAC `resources:` are lowercase plurals — a blanket rename once produced
  `AgentRuntimes` and silently broke the informer (forbidden loops in the log,
  reconciler does nothing).
- SSH deploy keys in Secrets must be LF-only with a trailing newline — CRLF or
  flattened-to-one-line keys fail with `error in libcrypto`. Prefer building
  the Secret from base64 rather than shell `--from-literal` interpolation.
- envtest needs `KUBEBUILDER_ASSETS`; `kubectl auth can-i` misparses the
  `pods/eviction` slash form — use `--subresource=eviction`.
- Never run two getUpdates consumers against one Telegram bot token (409s and
  stolen updates) — when migrating from another system, or from the old
  single-container adapter, stop its poller and CONFIRM none remains before
  starting `telegram-router`. `Channel.spec.config.pollingEnabled` is gone;
  ingest is on when the router runs.
- The router's bot Secret is the SAME one the Channel uses (it polls the bot
  the channel sends as), reached by projection from the router's own
  `SignalSource` — adapter CRs carry no credentials, so a credential-bearing
  served CR is the only path. Claim that source in a Pipeline or it reports
  `Wired=False` forever despite working fine.

## After changes

Update README.md when concepts/behavior change; keep commits scoped to this
directory.
