# Claude context — agent-ops-operator

Go/controller-runtime Kubernetes operator (see README.md for the product view).
Self-contained modules — no dependencies outside this directory; keep it that
way. Three Go modules: the operator (root), `channel-telegram/` (reference
channel adapter), and `signal-cron/` (reference signal adapter) — the adapters
dependency-free.

## Terminology (binding)

- **Agent runtime**, never "worker": CRD `AgentRuntime`, SA `agentops-runtime`,
  env `RUNTIME_*`, pkg `runtimepod`, pods `agentops-conv-<conversation>`.
- `AgentProfile` = who the agent is; `AgentRuntime` = what executes it;
  `Conversation` = topic + session + serial input queue.
- **Channel adapter** = out-of-process channel-type implementation consuming
  `/channel/*` (ops long-poll + inbound push). `Channel.spec` = type-agnostic
  metadata (`type`, `defaultProfileRef`, `delivery`, `credentialsSecretRef`)
  + opaque `config` that only the serving adapter interprets.
  `status.threadId` is an opaque STRING.
- **`ChannelAdapter` CR** = pure implementation (`type` + `image`, never
  credentials); its reconciler owns the adapter Deployment. Credentials are
  per-surface on `Channel.credentialsSecretRef`, projected into the adapter
  pod as `envFrom` with prefix `AGENTOPS_CRED_<CHANNEL>_` (kubelet-resolved;
  the contract's channel listing advertises `credentialEnvPrefix`).
- **`SignalAdapter` CR / signal adapter** = same pattern for ingest, but
  inbound-only (no ops queue): adapters push normalized signals
  (`fingerprint`, `labels`, `title?`, `payload`, `kind: alert|job`) to
  `/signal/inbound`; grouping/cooldown/recurrence stay MANAGER-side from
  `SignalSource.spec.grouping` — adapters normalize, the manager groups.
  Workload names `agentops-signal-<name>`; token derivation context
  `signal-adapter:<name>` (never interchangeable with channel tokens).
  `alertmanagerWebhook` is the one built-in signal type.
- API group `agentops.dev/v1alpha1` (provisional; rename possible pre-1.0).

## Build / test

```sh
go build ./... && go vet ./...
(cd channel-telegram && go build ./... && go vet ./...)
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
                         ConfigMap, runtime-pod pool (cap + idle eviction),
                         ownerRef GC, input pruning; ChannelAdapter +
                         SignalAdapter reconcilers on shared workload machinery
                         (adapterworkload.go: ownership, credential projection,
                         type-conflict guard); Channel + SignalSource
                         reconcilers (Served condition)
  httpapi/               /work long-poll dispatch, /work/done, /task,
                         /ingest/alertmanager, /channel/* adapter contract
                         (bearer auth via ADAPTER_TOKEN env)
  chat/                  channel-type-agnostic core: Provider+Registry
                         (in-process built-ins), OpQueue (outbound ops,
                         at-least-once), Router (transport-neutral inbound)
  dispatch/              input → work-unit resolution + built-in lane templates
                         (templates/format.md = mandatory message format spec)
  ingest/                signature grouping, fingerprint cooldown
  mcpcompile/            tri-form MCP → mcp.json + valueFrom env
  runtimepod/            runtime pod builder (AgentRuntime CR over bootstrap Config)
  addressing/            /<profile>[:<agent>] parsing
  integration/           envtest suite (real API server, fake chat, no kubelet)
runtime-claude/          reference AgentRuntime (Node + claude-code) — /work contract
channel-telegram/        reference channel adapter (own module, no deps) —
                         /channel contract; getUpdates poller + Bot API live HERE
signal-cron/             reference signal adapter (own module, no deps) —
                         /signal contract; five-field cron parser + scheduler
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
- **Adapter pods have zero ambient authority**: dedicated SA with no RBAC,
  `automountServiceAccountToken: false`. One ACTIVE ChannelAdapter per type
  (newer claimant gets `TypeConflict`, is not deployed).
- **Strictly serial per conversation** (one inflight unit); parallelism is
  across conversations, capped by `MAX_RUNTIMES` with idle-runtime eviction.
- **HTTP API is NOT leader-gated** (`NeedLeaderElection()=false`) — webhooks
  must serve during rollouts. **Exactly one getUpdates consumer per bot token,
  ever** — the telegram adapter runs ONE poll loop per distinct token (channels
  sharing a token share it), single-instance via ChannelAdapter `singleton`
  (replicas 1 + Recreate); the manager itself has no poller.
- **Channel ops are at-least-once.** `spec.config` is opaque to the operator —
  never parse channel-type config manager-side; adapters validate their own
  and report via the Channel Ready condition.
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
