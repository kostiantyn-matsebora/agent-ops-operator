## MODIFIED Requirements

### Requirement: The k8s bundle ships as a self-gated subchart, off by default and on in demo mode
A Helm subchart at `chart/charts/k8s-bundle/` SHALL package the Kubernetes agent experience as three components — events signal source, k8s-engineer profile, and its RBAC. Every bundle template SHALL gate on `enabled OR global.demo.enabled` (self-gating, not a Helm `condition:`), with `k8s-bundle.enabled` defaulting to `false`. The parent chart's demo toggle SHALL move to `global.demo.enabled` (**BREAKING** rename from `demo.enabled`) and `chart/templates/demo.yaml` SHALL be removed — demo mode means exactly "the bundle with its defaults", which includes read-only RBAC. Explicit `k8s-bundle.*` values SHALL still apply when enabled via demo.

#### Scenario: Default install renders nothing from the bundle
- **WHEN** the chart is installed with default values
- **THEN** no SignalAdapter, SignalSource, AgentProfile, AgentRuntime, ServiceAccount, Pipeline, or RBAC object from the bundle is rendered

#### Scenario: Demo mode enables the bundle read-only
- **WHEN** the chart is installed with `global.demo.enabled=true` and nothing else
- **THEN** all three components render with read-only RBAC, an AgentRuntime named `default` with the configured LLM credential env, and the install's Pipeline claiming the bundle's events source answers work posted to that source — the source is what a caller names, never the Pipeline and never the profile

#### Scenario: Bundle without demo
- **WHEN** the chart is installed with `k8s-bundle.enabled=true` and `global.demo.enabled=false`
- **THEN** the same components render — demo mode is an enablement path, not a distinct feature set

### Requirement: Demo values migrate to bundle paths
The pre-bundle demo values SHALL move: `demo.enabled` → `global.demo.enabled`, `demo.runtimeImage` → `k8s-bundle.profile.runtime.image`, `demo.credentialsSecret.*` → `k8s-bundle.profile.runtime.credentialsSecret.*`, `demo.readOnlyRbac` → `k8s-bundle.rbac.mode` (true ≙ `readonly`). The chart major version SHALL be bumped and the README SHALL carry the migration table. Upgrading a demo-enabled release SHALL preserve semantics: the AgentRuntime `default` re-renders equivalently (existing conversations keep resolving their runtime) while demo-named SA/RBAC objects are replaced by bundle-named ones.

#### Scenario: Upgraded demo release keeps working
- **WHEN** a release running chart 1.x with `demo.enabled=true` upgrades and adopts the new values paths
- **THEN** the demo advisor flow works — now reached by posting a `kind: task` signal to the bundle's events source, which the install's Pipeline claims — and old demo-suffixed SA/RBAC objects are removed by the upgrade
