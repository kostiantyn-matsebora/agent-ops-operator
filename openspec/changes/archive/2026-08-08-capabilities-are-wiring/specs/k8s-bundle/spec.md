# k8s-bundle — delta

## MODIFIED Requirements

### Requirement: The profile component ships the k8s-engineer identity chain
When active, the `profile` component SHALL render: the `k8s-engineer` AgentProfile (values-configurable name, `maxTurns` 40, no repository — and NO capabilities, since AgentProfiles declare none); a dedicated runtime ServiceAccount (default `agentops-runtime-k8s`); and, when `runtime.create` is on, an `AgentRuntime` (name defaulting to `default`, values-configured image and LLM credential Secret ref projected as env via `valueFrom` — the manager reads no Secrets) whose `serviceAccountName` is that SA. `runtime.create: false` SHALL support operators wiring the profile to an existing runtime via `runtimeRef` values.

When `profile.baseline.create` is on, the component SHALL also render that profile's **capability-only Pipeline** — no sources, no channels — binding the chart's built-in toolsets, with `baseline.grantShell` controlling whether execution is among them. Without it a bare `POST /task` would reach an agent with no tools at all, which is exactly what the documented five-minute demo does.

#### Scenario: Profile executes under the bundle SA
- **WHEN** the bundle renders with defaults and a task addresses `k8s-engineer`
- **THEN** the conversation's runtime pod runs under the bundle's ServiceAccount, so the agent's in-cluster power is exactly what the bundle's RBAC component granted

#### Scenario: Bring-your-own runtime
- **WHEN** `profile.runtime.create=false` and `profile.runtimeRef` names an existing AgentRuntime
- **THEN** the AgentProfile renders with that `runtimeRef` and no AgentRuntime or SA is created by the bundle

#### Scenario: The demo stays usable without any routing wiring
- **WHEN** the bundle renders with defaults and a task is posted to `POST /task` with no pipeline named
- **THEN** the baseline Pipeline supplies the agent's tools, and the rendered AgentProfile declares none

#### Scenario: An observe-only agent
- **WHEN** `profile.baseline.grantShell=false`
- **THEN** the baseline binds the observation toolset only, so the agent reads the cluster but runs nothing through the pipeline-less paths
