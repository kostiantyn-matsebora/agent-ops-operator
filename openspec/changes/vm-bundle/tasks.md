## 1. API constant (carried from merged change)

- [ ] 1.1 Add `SourceVMAlertmanager = "vmAlertmanagerWebhook"` constant to `api/v1alpha1/signalsource_types.go` (doc comment: adapter-served, standard Alertmanager webhook format; built-in `alertmanagerWebhook` unchanged)

## 2. signal-vmalertmanager adapter module (carried from merged change)

- [ ] 2.1 Scaffold `signal-vmalertmanager/` module (go.mod with no requires, Dockerfile mirroring `signal-cron/`, distroless nonroot) with `manager.go` contract client (Sources/Inbound/Status, bearer auth) from the signal-cron pattern
- [ ] 2.2 Implement the source registry: 15s poll of `GET /signal/sources?type=vmAlertmanagerWebhook`, tracking name → `credentialEnvPrefix`; report `Ready` via status API for served sources
- [ ] 2.3 Implement the webhook server: `LISTEN_ADDR` (default `:8080`), `POST /webhook/{source}` with 404 for unknown sources, 1 MiB body cap, opt-in bearer auth (constant-time compare against projected `AGENTOPS_CRED_<SOURCE>_TOKEN` when the source advertises a credential prefix; 401 on mismatch)
- [ ] 2.4 Implement normalization: firing-only filter (report `queued: 0` reason otherwise), verbatim fingerprint with sorted-label-hash fallback, raw labels, `"🔍 " + alertname — namespace` title, per-alert JSON payload (labels/annotations/startsAt/generatorURL), no kind; push to `/signal/inbound`
- [ ] 2.5 Unit tests with `httptest` fakes for both sides: source routing/404, firing filter, fingerprint fallback stability, bearer auth accept/reject/anonymous, inbound push body shape, source-list refresh

## 3. vm-bundle subchart

- [ ] 3.1 Scaffold `chart/charts/vm-bundle/` (Chart.yaml, values.yaml with defaults from design D1, `_helpers.tpl` for the enabled gate and resolved names)
- [ ] 3.2 Alertmanager component template: SignalAdapter CR (`type: vmAlertmanagerWebhook`, singleton), Service `agentops-signal-<name>` (selector `agentops.dev/signal-adapter: <name>`, numeric targetPort 8080, values port), optional default SignalSource gated on `defaultSource.enabled` with `fail` on empty `profileRef`
- [ ] 3.3 MCP component templates: MCPConfig `vm-logs` (server key `victorialogs`) and `vm-metrics` (server key `victoriametrics`), `type: sse`, values URL with `fail` on empty-when-enabled, headers passthrough incl. `valueFrom` secret refs
- [ ] 3.4 Pin the pod-label contract: assert in `internal/integration/signaladapter_test.go` (comment + explicit label assertions) that `agentops.dev/signal-adapter` is chart-consumed and must not be renamed (skip if already pinned by the k8s-bundle change)
- [ ] 3.5 Parent chart: add a commented `vm-bundle:` override block to `chart/values.yaml`; bump chart version (minor over whatever `k8s-bundle` left; 1.3.0 if landing first)

## 4. Verification, samples, docs

- [ ] 4.1 `helm template` matrix: defaults render nothing; `global.demo.enabled=true` alone renders nothing from this bundle; enabled renders adapter + Service + both MCPConfigs; each component individually disabled; enabled MCP component with empty url fails naming the value; defaultSource without profileRef fails
- [ ] 4.2 Add samples to `config/samples/samples.yaml`: a `vmAlertmanagerWebhook` SignalSource (with and without `credentialsSecretRef`) and a profile snippet showing `mcp.configRefs: [vm-logs, vm-metrics]` + `mcp__victorialogs__*`/`mcp__victoriametrics__*` allowlist entries
- [ ] 4.3 Update README.md (VM bundle section: components and flags, VMAlertmanager `webhook_configs` snippet with the Service URL, webhook auth setup, MCP wiring worked example, migration from built-in `alertmanagerWebhook`) and CLAUDE.md (module map, build commands with `agentops-signal-vmalertmanager` image, terminology)
- [ ] 4.4 Full verification: `go build ./... && go vet ./...` in all modules (root, channel-telegram, signal-cron, signal-vmalertmanager), envtest suite with `KUBEBUILDER_ASSETS`, confirm built-in `/ingest/alertmanager` tests untouched and passing, `helm lint` + template smoke of the matrix
