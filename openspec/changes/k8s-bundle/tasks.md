## 1. API and workload opt-in (carried from merged change)

- [ ] 1.1 Add `AutomountServiceAccountToken *bool` (default false, optional) to `SignalAdapterSpec` in `api/v1alpha1/signaladapter_types.go` and `SourceK8sEvents = "k8sEvents"` constant to `api/v1alpha1/signalsource_types.go`
- [ ] 1.2 Regenerate deepcopy and CRD YAML (`controller-gen object` + `crd` into `chart/files/crds/`)
- [ ] 1.3 Thread an automount option through `internal/controller/adapterworkload.go` and set it from the CR in `signaladapter_controller.go`; the ChannelAdapter path never sets it
- [ ] 1.4 Extend `internal/integration/signaladapter_test.go`: opt-in renders `automountServiceAccountToken: true`, omitted keeps `false`; confirm ChannelAdapter rendering tests still pass unmodified

## 2. signal-k8s-events adapter module (carried from merged change)

- [ ] 2.1 Scaffold `signal-k8s-events/` module (go.mod with no requires, Dockerfile mirroring `signal-cron/`, distroless nonroot) with `manager.go` contract client (Sources/Inbound/State/Status, bearer auth) from the signal-cron pattern
- [ ] 2.2 Implement the dependency-free in-cluster client (`kube.go`): token file read with periodic re-read, CA pool, base URL from `KUBERNETES_SERVICE_HOST/PORT`; Events list and streaming watch decode over `net/http`
- [ ] 2.3 Implement config parsing/validation: `severities` (default `["Warning"]`, reject unknown types), `namespaces`, `includeReasons`/`excludeReasons`; invalid config → `Ready=False` via status API, other sources keep serving
- [ ] 2.4 Implement the watch loop: one list+watch per namespace scope, fan-out to matching sources, client-side severity/reason filtering, relist on 410/stream error
- [ ] 2.5 Implement normalization + cursor: `kind: alert`, fingerprint `<source>@<ns>/<kind>/<name>/<reason>`, labels (`alertgroup`, `alertname`, `namespace`, `kind`, `name`, `severity`, `source`), title/payload; persist max `lastTimestamp` cursor via state API and skip ≤ cursor on startup list
- [ ] 2.6 Unit tests with `httptest` fakes for both APIs: severity/reason filtering, fingerprint stability, cursor skip-on-restart, 410 relist recovery, invalid-config status reporting

## 3. k8s-bundle subchart

- [ ] 3.1 Scaffold `chart/charts/k8s-bundle/` (Chart.yaml, values.yaml with the component defaults from design D2, `_helpers.tpl` for the `enabled OR global.demo.enabled` gate and resolved names)
- [ ] 3.2 Events component template: SignalAdapter CR (`type: k8sEvents`, `automountServiceAccountToken: true`, singleton), events RBAC (ClusterRole/Role + binding to `agentops-signal-<name>` per `rbac.clusterWide`/`rbac.create`), SignalSource gated on `source.create` with `profileRef` resolution + render-time `fail` when unresolvable
- [ ] 3.3 Profile component template: AgentProfile `k8s-engineer` (values-configurable), runtime SA `agentops-runtime-k8s`, AgentRuntime (default name `default`, image + credentialsSecret env via `valueFrom`, `serviceAccountName`) gated on `runtime.create`; `runtimeRef` passthrough for bring-your-own-runtime
- [ ] 3.4 RBAC component template: `mode: readonly` → `view` binding + nodes/namespaces/metrics ClusterRole + binding (verbatim from demo.yaml); `mode: full` → cluster-admin binding; nothing when `rbac.enabled=false`
- [ ] 3.5 Parent chart rewiring: add `global.demo.enabled: false` and a commented `k8s-bundle:` override block to `chart/values.yaml`, delete `chart/templates/demo.yaml`, remove the old `demo.*` values, bump chart version to 2.0.0

## 4. Verification, samples, docs

- [ ] 4.1 `helm template` matrix: defaults render no bundle objects; `global.demo.enabled=true` renders all three components read-only with runtime `default`; `k8s-bundle.enabled=true` equivalent; each component individually disabled; `rbac.mode=full` renders exactly the cluster-admin binding; `eventsAdapter.rbac.clusterWide=false` renders namespaced RBAC; source without resolvable profile fails render with a clear message
- [ ] 4.2 Verify manager RBAC in `chart/templates/rbac.yaml` is untouched (no roles/rolebindings verbs, no events list/watch)
- [ ] 4.3 Add a `k8sEvents` SignalSource example to `config/samples/samples.yaml`
- [ ] 4.4 Update README.md (k8s bundle concept + component flags, demo section rewrite on bundle terms incl. events-cost note and full-mode warning, values migration table) and CLAUDE.md (module map, build commands with `agentops-signal-k8s-events` image, chart notes)
- [ ] 4.5 Full verification: `go build ./... && go vet ./...` in all modules, envtest suite with `KUBEBUILDER_ASSETS`, `helm lint` + template smoke of the matrix
