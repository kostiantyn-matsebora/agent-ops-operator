# Claude context — agent-ops-operator

Go/controller-runtime Kubernetes operator (see README.md for the product view).
Self-contained module — no dependencies outside this directory; keep it that way.

## Terminology (binding)

- **Agent runtime**, never "worker": CRD `AgentRuntime`, SA `agentops-runtime`,
  env `RUNTIME_*`, pkg `runtimepod`, pods `agentops-conv-<conversation>`.
- `AgentProfile` = who the agent is; `AgentRuntime` = what executes it;
  `Conversation` = topic + session + serial input queue.
- API group `agentops.dev/v1alpha1` (provisional; rename possible pre-1.0).

## Build / test

```sh
go build ./... && go vet ./...
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
# then update the image refs (chart values for the manager, AgentRuntime CRs for
# runtimes), helm upgrade, and verify with a live task:
#   POST /task {"profile":"stub","task":"..."}   (stub runtime = no LLM cost)
```

## Map

```
api/v1alpha1/            CRD types (+ generated deepcopy); CRD YAML in chart/files/crds/
cmd/manager/main.go      wiring: reconciler, httpapi, telegram poller, env config
internal/
  controller/            Conversation reconciler: topic, MCP ConfigMap, runtime-pod
                         pool (cap + idle eviction), ownerRef GC, input pruning
  httpapi/               /work long-poll dispatch, /work/done, /task, /ingest/alertmanager
  chat/                  Provider interface, Telegram impl, polling loop (leader-only)
  dispatch/              input → work-unit resolution + built-in lane templates
                         (templates/format.md = mandatory message format spec)
  ingest/                signature grouping, fingerprint cooldown
  mcpcompile/            tri-form MCP → mcp.json + valueFrom env
  runtimepod/            runtime pod builder (AgentRuntime CR over bootstrap Config)
  addressing/            /<profile>[:<agent>] parsing
  integration/           envtest suite (real API server, fake chat, no kubelet)
runtime-claude/          reference AgentRuntime (Node + claude-code) — /work contract
chart/                   Helm chart: manager Deployment/RBAC/Service + CRDs as gated
                         templates (crds.enabled, crds.keep -> helm.sh/resource-policy:
                         keep so uninstall never cascade-deletes CRs); CRD source of
                         truth = chart/files/crds/ (controller-gen output)
config/samples/          example CRs (the only config/ content — deployment-specific
                         config belongs with the deployment, never in this module)
```

## Invariants (do not break)

- **The manager never reads agent secrets.** Everything secret-shaped compiles
  to `valueFrom` in the runtime pod spec (the kubelet resolves it). The only
  secret the manager reads is the Channel bot token — via `GetAPIReader()`
  (uncached GET; the cached client would demand list/watch RBAC on all secrets).
- **Strictly serial per conversation** (one inflight unit); parallelism is
  across conversations, capped by `MAX_RUNTIMES` with idle-runtime eviction.
- **HTTP API is NOT leader-gated** (`NeedLeaderElection()=false`) — webhooks
  must serve during rollouts. The **chat poller IS leader-only** (default
  gating) — exactly one getUpdates consumer per bot token, ever.
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
  setting `pollingEnabled: true`.

## After changes

Update README.md when concepts/behavior change; keep commits scoped to this
directory.
