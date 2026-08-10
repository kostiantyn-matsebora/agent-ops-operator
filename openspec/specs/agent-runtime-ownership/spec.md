# agent-runtime-ownership

## Purpose

Who owns the agent execution substrate in the Helm chart: the parent chart contributes the runtime, the one identity whose RBAC is the agent's power, the LLM credential and the home volume, while bundles contribute domain — signal sources, profiles, tooling and channels — and reference what the parent provides.
## Requirements
### Requirement: The main chart owns the default agent runtime
The parent chart SHALL render the default `AgentRuntime` from a top-level `runtime:` component, enabled by default. The component SHALL own everything describing how agents execute — image, LLM credential reference, idle TTL, `nodeSelector`, resources, and the home volume — and SHALL render the runtime named `runtime.name` (default `default`), which is the name a profile with no `runtimeRef` falls back to.

No subchart SHALL render an `AgentRuntime`, a runtime ServiceAccount, or a runtime credential Secret. Bundles contribute domain — signal sources, profiles, tooling, channels — and reference the substrate the parent provides. A bundle MAY name a different runtime through its profile's `runtimeRef`, but SHALL NOT create one.

`runtime.enabled: false` SHALL render nothing from the component, for installs that manage `AgentRuntime` CRs themselves.

#### Scenario: A default install can execute a conversation
- **WHEN** the chart is installed with default values and no bundle enabled
- **THEN** an `AgentRuntime` named `default` renders, so any profile reaching the manager resolves a runtime — and no bundle objects render

#### Scenario: A bundle install renders exactly one runtime
- **WHEN** the chart is installed with any combination of bundles enabled
- **THEN** exactly one `AgentRuntime` and one runtime ServiceAccount render, both from the parent

#### Scenario: Chat-only install works
- **WHEN** only `telegram-bundle.enabled=true` is set
- **THEN** the install renders a runtime and can execute conversations started from chat, without enabling a Kubernetes bundle for its substrate

#### Scenario: Bring your own runtime
- **WHEN** `runtime.enabled=false`
- **THEN** no `AgentRuntime`, credential Secret, or runtime object renders from the component, and profiles resolve whichever runtimes the operator applied

### Requirement: One identity carries the agent's power, declared under global
The runtime ServiceAccount SHALL be `global.agentops.runtime.serviceAccountName` (default `agentops-runtime`) — the same SA the parent already creates and the manager already defaults runtime pods onto. Exactly one runtime ServiceAccount SHALL exist per release.

The agent's in-cluster power SHALL be declared by a single value, `global.agentops.runtime.rbacMode`, with modes `none`, `readonly` and `full`. `readonly` SHALL bind the built-in `view` ClusterRole plus a ClusterRole granting `get`/`list`/`watch` on `nodes` and `namespaces` and `get`/`list` on `metrics.k8s.io` nodes/pods. `full` SHALL bind `cluster-admin` and SHALL be documented as unrestricted cluster control for an LLM-driven agent. `none` SHALL bind nothing.

Both keys live under `global.` because subcharts can read no other parent scope and SHALL derive their own posture from the effective mode rather than restating it. The existing `rbac.runtime.{clusterRoles,bindClusterRoles,namespaced}` values SHALL keep working and SHALL be additive to the mode.

#### Scenario: One knob, one binding set
- **WHEN** `global.agentops.runtime.rbacMode=full` is set
- **THEN** a `cluster-admin` ClusterRoleBinding for the runtime SA renders, and no other values need to be set anywhere for the install to be internally consistent

#### Scenario: Targeted grants compose with the mode
- **WHEN** `rbacMode=readonly` and `rbac.runtime.clusterRoles` names an extra role
- **THEN** both the read-only objects and the extra ClusterRole/binding render against the same SA

#### Scenario: No second runtime identity
- **WHEN** any bundle is enabled
- **THEN** no bundle-named runtime ServiceAccount (such as `agentops-runtime-k8s`) renders, and all runtime RBAC targets the global SA

### Requirement: Unset RBAC mode grants nothing except in demo mode
`global.agentops.runtime.rbacMode` SHALL default to empty. Empty SHALL resolve to `readonly` when `global.demo.enabled` is true, and to `none` otherwise, so the chart keeps its "grants NOTHING unless configured" posture for ordinary installs while demo mode stays a one-flag working install. `full` SHALL never be selected by any default or inferred path.

The resolution SHALL be computed in one place and be readable by subcharts, so the parent's bindings and any subchart deriving from the mode always agree.

#### Scenario: Upgrade does not widen an existing install
- **WHEN** a release that never set an RBAC value upgrades to this chart without demo mode
- **THEN** the runtime SA holds no bindings, exactly as before

#### Scenario: Demo mode stays one flag
- **WHEN** the chart is installed with `global.demo.enabled=true` and nothing else
- **THEN** the runtime SA holds the read-only bindings and the demo agent can inspect the cluster

#### Scenario: Full is never implicit
- **WHEN** any combination of defaults, demo mode, and bundle enablement is rendered without an explicit `full`
- **THEN** no `cluster-admin` binding renders

### Requirement: The credential and home volume are wired, not restated
When `runtime.credentialsSecret.token` is supplied the component SHALL create that Secret, so the credential is release-managed; when it is empty the `AgentRuntime` SHALL reference the named Secret without creating it, and the post-install notes SHALL warn that the reference is unsatisfied. The credential SHALL reach the runtime as env via `valueFrom` — the manager SHALL read no Secrets.

The rendered `AgentRuntime` SHALL take `home.pvcRef` from the parent's own `persistence` block (its claim name, or `existingClaim`) with an explicit `runtime.homePvcRef` override for a claim the chart did not create. It SHALL take an optional `workspace.pvcRef` from the parent's workspace persistence block the same way. No operator SHALL have to copy a claim name between values blocks, for either volume.

Home persistence SHALL be enabled by default and workspace persistence SHALL be disabled by default. The asymmetry is deliberate: losing session files silently costs conversational history, whereas losing a checkout costs a re-clone.

`runtime.idleTtlMinutes` SHALL default to empty and SHALL then follow the release's `runtimeIdleTtlMinutes`, so there is one number unless an operator deliberately wants two. The chart SHALL WRITE the resolved value into the rendered CR rather than omitting the field: `AgentRuntime.spec.idleTtlMinutes` carries a CRD default, so an omitted field is not unset — the API server stores the CRD default and the manager prefers any non-zero spec value over its own configured TTL, which makes an omitted field render a correct-looking manifest and a wrong stored object.

#### Scenario: Credential comes back with the release
- **WHEN** `runtime.credentialsSecret.token` is set from a secret store
- **THEN** the Secret renders with the release and the runtime references it by name only

#### Scenario: Unsatisfied reference is announced, not silent
- **WHEN** `runtime.enabled=true` and no token is supplied
- **THEN** the install succeeds and the notes state that runtime pods will reach `CreateContainerConfigError` until the named Secret exists, because the kubelet resolves the reference and nothing else reports it

#### Scenario: Persistence needs no second declaration
- **WHEN** `persistence.enabled=true`
- **THEN** the rendered `AgentRuntime` carries `home.pvcRef` naming the chart's claim, with no runtime-side value set

#### Scenario: Sessions persist without being asked for
- **WHEN** the chart is installed with no persistence values supplied
- **THEN** the home claim is provisioned and the rendered `AgentRuntime` references it

#### Scenario: Workspace is wired the same way when enabled
- **WHEN** workspace persistence is enabled
- **THEN** the rendered `AgentRuntime` carries `workspace.pvcRef` naming the chart's workspace claim, with no runtime-side value set

#### Scenario: Idle TTL has one default
- **WHEN** `runtime.idleTtlMinutes` is left empty
- **THEN** the rendered `AgentRuntime` carries the release's `runtimeIdleTtlMinutes`, and runtime pods use that number rather than the CRD's default

