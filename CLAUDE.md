# Claude context — agent-ops-operator

Go/controller-runtime Kubernetes operator (see README.md for the product view).
Self-contained modules — no dependencies outside this directory; keep it that
way. Five Go modules: the operator (root), `channel-telegram/` (reference
channel adapter), and `signal-cron/`, `signal-vmalertmanager/`,
`signal-k8s-events/` (signal adapters) — the adapters dependency-free.

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
  only new ones. `POST /task` and `/profile`-command conversations carry none.
- **`Pipeline`** = THE wiring, exclusively: sources[] × channels[] + profile
  + TOOL ACCESS. No other CR carries wiring (SignalSource has no
  profile/channel refs, Channel has no default profile) — unclaimed sources
  DROP signals (`Wired=False` + response reason), unwired channels answer bare
  messages with guidance only. One pipeline per source (older claimant wins),
  channels shareable, Ready pipelines only.
  **Capabilities are wiring, exclusively**: two optional stanzas of ordered
  refs — `spec.toolsets` (→ `MCPToolset`, the allowlist) and `spec.mcpConfigs`
  (→ `MCPConfig`, the MCP servers). NO mode: with nothing profile-side to
  compose against, merge/overwrite would be one behavior wearing two names.
  Refs in order: tools concatenate with dedup, server keys overlay (later wins).
  Content stays in the referenced CRs; Ready validates both ref sets.
  **A Pipeline with NO sources and NO channels is a profile's BASELINE** — what
  it may do when a conversation has no routing pipeline (`POST /task` without
  one, `/<profile>` commands). Exactly one per profile: a second sets
  `BaselineConflict` on BOTH and neither applies (no oldest-wins — guessing
  which baseline was meant is worse than granting nothing). No baseline = an
  unwired profile = no tools, same posture as an unclaimed source.
  `POST /task {"pipeline": X}` carries X's channels AND capabilities.
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
for m in channel-telegram signal-cron signal-vmalertmanager signal-k8s-events; do
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
                         /channel contract; getUpdates poller + Bot API live HERE
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
- **HTTP API is NOT leader-gated** (`NeedLeaderElection()=false`) — webhooks
  must serve during rollouts. **Exactly one getUpdates consumer per bot token,
  ever** — the telegram adapter runs ONE poll loop per distinct token (channels
  sharing a token share it), single-instance via ChannelAdapter `singleton`
  (replicas 1 + Recreate); the manager itself has no poller.
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
  stolen updates) — when migrating from another system, stop its poller before
  setting `pollingEnabled: true` (now in Channel `spec.config`) / enabling the
  telegram adapter.

## After changes

Update README.md when concepts/behavior change; keep commits scoped to this
directory.
