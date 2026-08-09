# Tasks: visualize-agent-ops

## 1. ChannelAdapter parity fields (API + controller)

- [x] 1.1 Add `kubernetesAccess *bool` and `port *int32` to `ChannelAdapterSpec` in `api/v1alpha1/channeladapter_types.go`, mirroring the SignalAdapter field comments; regen deepcopy + CRDs (controller-gen, output to `chart/files/crds/`)
- [x] 1.2 Extend the shared machinery in `internal/controller/adapterworkload.go` so kubernetesAccess (SA token mount + `POD_NAMESPACE`) and port (`LISTEN_ADDR` + owned Service `agentops-adapter-<name>`) apply on the channel side as they do for SignalAdapter; keep RBAC-creation at zero
- [x] 1.3 Unit + envtest coverage: default posture unchanged (no token mount, no Service), kubernetesAccess mounts identity only, port creates/deletes the owned Service; verify manager RBAC needs no new verbs beyond Services already granted
- [x] 1.4 `go build ./... && go vet ./...` and full envtest suite green

## 2. Console module scaffold + Kubernetes watch cache

- [x] 2.1 Create self-contained `console/` Go module (own `go.mod`, no external deps) with main wiring: env config (`MANAGER_URL`, `ADAPTER_NAME`, `ADAPTER_TOKEN`, `LISTEN_ADDR`, `POD_NAMESPACE`), HTTP server, graceful shutdown
- [x] 2.2 Implement raw Kubernetes REST client (in-cluster token/CA from the mounted SA, same technique as `signal-vmalertmanager` self-registration) with list + streaming watch for the eight `agentops.dev/v1alpha1` kinds
- [x] 2.3 Implement the in-memory cache: per-kind store keyed by name, resourceVersion resume, relist-on-410, event fan-out to subscribers; test against recorded list/watch JSON fixtures (no cluster needed)
- [x] 2.4 Derive the topology model from the cache: nodes (sources, pipelines, profiles, channels, adapters), edges from Pipeline spec + `spec.adapter` refs, health from Ready/Served/Wired conditions; table-driven tests covering unclaimed source, unserved adapter, healthy pipeline

## 3. Console as channel adapter

- [x] 3.1 Implement the `/channel/*` client loop: long-poll `ops?adapter=console`, complete `ensure-topic` with deterministic thread id `console-<conversation-UID>`, dedupe ops by id
- [x] 3.2 Implement per-thread bounded transcript ring buffer fed by `send` ops (results, acks, attributed relays), with relay attribution preserved for rendering
- [x] 3.3 Implement inbound send: UI message → `POST /channel/inbound` with thread id, local pending state confirmed by the returning ack/relay op; enforce the no-relay-loop rule (received ops are never re-posted inbound) with a test pinning it
- [x] 3.4 Adapter-loop tests against a fake manager HTTP server: op dedupe, ensure-topic determinism across restart, pending-confirm flow, no-relay-loop

## 4. Console HTTP API + UI

- [x] 4.1 Browser API: token login (constant-time compare, cookie session), JSON snapshot endpoints (topology, per-kind inventory, CR detail, conversation list with joined/observed flag, runs history), 401 on everything unauthenticated
- [x] 4.2 SSE stream endpoint multiplexing watch deltas, activity updates, and transcript appends; snapshot+resubscribe semantics on reconnect (no event-loss correctness dependency)
- [x] 4.3 Embedded SPA via `go:embed` (hand-written HTML/JS/CSS, no toolchain): topology graph as SVG with condition coloring and live activity badges, CR inventory/detail views, runs view, transcript view with send box for joined conversations, join-instructions panel for unjoined pipelines
- [x] 4.4 UI reads the token from its channel's projected credentials (`credentialEnvPrefix` from the contract's channel listing + `uiToken` key); document the key in the console ChannelAdapter's `credentialKeys`

## 5. Image + chart bundle

- [x] 5.1 `console/Dockerfile` (distroless, linux/amd64) and build entry in the images list; bump-tag discipline noted
- [x] 5.2 Chart: `console.*` values gating (default false) — ChannelAdapter CR (`singleton`, `kubernetesAccess: true`, `port`, `configSchema`/`credentialKeys`), console Channel CR with `credentialsSecretRef`, UI token Secret, read-only Role/RoleBinding for SA `agentops-adapter-console` (get/list/watch on agentops.dev only), optional Ingress
- [x] 5.3 Chart template tests / `helm template` assertions: nothing rendered when disabled; no chart-owned Deployment/Service for the console when enabled; RBAC contains no write verbs and no Secret access
- [x] 5.4 NOTES.txt: how to reach the console, how to join it to a Pipeline (`channels[]` edit)

## 6. Integration + docs

- [x] 6.1 envtest integration: ChannelAdapter `console` reconciles to a workload with token mount + Service; console Channel goes Served; conversation on a console-wired Pipeline gets a console thread binding via the ops flow (fake adapter completing ensure-topic)
- [x] 6.2 Live verification per CLAUDE.md: deploy with stub runtime, wire the console channel into a test Pipeline, confirm topology renders, a `POST /task` run appears live, and a message typed in the UI round-trips through the router
- [x] 6.3 Update README.md (console concept, trust boundary: UI token ⇒ sees all agentops CRs) and CLAUDE.md map/terminology for `console/` and the ChannelAdapter parity fields
- [x] 6.4 Record the `add-web-chat-channel` supersession decision on that change (re-scope or withdraw note in its proposal), per design Decision 7
