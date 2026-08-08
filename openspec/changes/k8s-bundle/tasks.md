> **No API or controller work.** The merged change's `automountServiceAccountToken`
> opt-in already shipped as `SignalAdapterSpec.kubernetesAccess` (threading, envtest
> coverage and all), and its `SourceK8sEvents` constant is dropped rather than
> deferred — `spec.type` no longer exists and the adapter CR's name is the routing
> key. See design D4.

## 1. signal-k8s-events adapter module (carried from merged change)

- [x] 1.1 Scaffold `signal-k8s-events/` module (go.mod with no requires, Dockerfile mirroring `signal-cron/`, distroless nonroot) with `manager.go` contract client (Sources/Inbound/State/Status, bearer auth) from the signal-cron pattern
- [x] 1.2 Implement the dependency-free in-cluster client (`kube.go`): token file read with periodic re-read, CA pool, base URL from `KUBERNETES_SERVICE_HOST/PORT`; Events list and streaming watch decode over `net/http`
- [x] 1.3 Implement config parsing/validation: `severities` (default `["Warning"]`, reject unknown types), `namespaces`, `includeReasons`/`excludeReasons`; invalid config → `Ready=False` via status API, other sources keep serving
- [x] 1.4 Implement the watch loop: one list+watch per namespace scope, fan-out to matching sources, client-side severity/reason filtering, relist on 410/stream error
- [x] 1.5 Implement normalization + cursor: `kind: alert`, fingerprint `<source>@<ns>/<kind>/<name>/<reason>`, labels (`alertgroup: k8s-events`, `alertname`, `namespace`, `kind`, `name`, `severity`, `source`), title/payload; persist max `lastTimestamp` cursor via state API and skip ≤ cursor on startup list
- [x] 1.6 Unit tests with `httptest` fakes for both APIs: severity/reason filtering, fingerprint stability, cursor skip-on-restart, 410 relist recovery, invalid-config status reporting

## 2. k8s-bundle subchart

- [x] 2.1 Scaffold `chart/charts/k8s-bundle/` (Chart.yaml, values.yaml with the component defaults from design D2, `_helpers.tpl` for the `enabled OR global.demo.enabled` gate and resolved names)
- [x] 2.2 Events component template: SignalAdapter CR (default name `k8s-events`, `kubernetesAccess: true`, singleton, optional `configSchema`), events RBAC (ClusterRole/Role + binding to `agentops-signal-<name>` per `rbac.clusterWide`/`rbac.create`), and — gated on `source.create` — the SignalSource (`adapter:` naming the CR) TOGETHER WITH the Pipeline claiming it, `profileRef` resolution + render-time `fail` when unresolvable. Mirror vm-bundle's `defaultSource`; never render the source without its Pipeline (unclaimed = `Wired=False`, signals dropped)
- [x] 2.3 Profile component template: AgentProfile `k8s-engineer` (values-configurable), runtime SA `agentops-runtime-k8s`, AgentRuntime (default name `default`, image + credentialsSecret env via `valueFrom`, `serviceAccountName`) gated on `runtime.create`; `runtimeRef` passthrough for bring-your-own-runtime
- [x] 2.4 RBAC component template: `mode: readonly` → `view` binding + nodes/namespaces/metrics ClusterRole + binding (verbatim from demo.yaml); `mode: full` → cluster-admin binding; nothing when `rbac.enabled=false`
- [x] 2.5 Parent chart rewiring: add `global.demo.enabled: false` and a commented `k8s-bundle:` override block to `chart/values.yaml`, delete `chart/templates/demo.yaml`, remove the old `demo.*` values, bump chart version to 2.0.0

## 3. Verification, samples, docs

- [x] 3.1 `helm template` matrix: defaults render no bundle objects; `global.demo.enabled=true` renders all three components read-only with runtime `default`; `k8s-bundle.enabled=true` equivalent; each component individually disabled; `rbac.mode=full` renders exactly the cluster-admin binding; `eventsAdapter.rbac.clusterWide=false` renders namespaced RBAC; a rendered SignalSource is ALWAYS accompanied by its Pipeline; source without resolvable profile fails render with a clear message
- [x] 3.2 Verify manager RBAC in `chart/templates/rbac.yaml` is untouched (no roles/rolebindings verbs, no events list/watch)
- [x] 3.3 Add a k8s-events SignalSource + Pipeline example to `config/samples/samples.yaml`
- [x] 3.4 Update README.md (k8s bundle concept + component flags, demo section rewrite on bundle terms incl. events-cost note and full-mode warning, values migration table) and CLAUDE.md (module map, build commands with `agentops-signal-k8s-events` image, chart notes)
- [x] 3.5 Full verification: `go build ./... && go vet ./...` in all modules, envtest suite with `KUBEBUILDER_ASSETS`, `helm lint` + template smoke of the matrix
