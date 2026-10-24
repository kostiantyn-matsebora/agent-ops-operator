## MODIFIED Requirements

### Requirement: The profile component ships the k8s-engineer identity chain
When active, the `profile` component SHALL render exactly one object: the `k8s-engineer` `AgentProfile` (values-configurable name, `maxTurns`, no repository, and **no capabilities** — no `allowedTools`, no `mcp`). It SHALL render no `AgentRuntime`, no ServiceAccount, and no credential Secret; the profile executes on the parent chart's runtime.

Because the profile has NO repository, no agent definition file can be resolved for it. The component SHALL therefore support an inline role (`systemPrompt`) and ship a sensible default, so the shipped agent is not personality-free: a conversation started by a cluster event would otherwise arrive with no instructions at all.

**The inline role SHALL state the install's pod-execution posture, rendered from the same value the RBAC reads.** `global.agentops.runtimeDefaults.allowPodExecution` gates every write that produces or enters a pod on the route account and on the MCP server's role; the profile is the third wall on the same value. When the gate is off, the rendered role SHALL carry a paragraph telling the agent that this install withholds pod execution — creating or entering a pod, and editing the pod template of a Deployment, StatefulSet, DaemonSet, ReplicaSet, Job or CronJob — and that such a request is to be declined with that reason, naming what remains possible and suggesting the operator makes the edit, rather than attempted. When the gate is on, the paragraph SHALL be absent. The paragraph's text SHALL be a bundle value, appended after `systemPrompt` whether that prompt is the shipped one or the operator's own, because the posture is the chart's fact and not the prompt author's. There SHALL be one profile, never a second one per posture: a hand-kept copy drifts from the gate the first time it is flipped.

The toolset wall is a different wall, and the role already covers it: a tool absent from the allowlist is reported as unavailable. The posture paragraph exists because the pod-execution gate is invisible from the tool list — the MCP server advertises the workload-patch tool whatever the gate says, and the refusal arrives from the API server one hop later.

`profile.runtimeRef` SHALL remain, naming a runtime other than `default` — a different-vendor runtime the install declared. Left empty, the profile emits no `runtimeRef` and falls back to `default`, whose existence the render-time default-runtime guard is what guarantees.

#### Scenario: Profile executes under the release's runtime SA
- **WHEN** the bundle renders with defaults and a task reaches `k8s-engineer`
- **THEN** the conversation's runtime pod runs under the account its route names, or the release's default where it names none, and holds exactly what that account was granted

#### Scenario: The profile component renders one object
- **WHEN** the bundle renders with `profile.enabled=true`
- **THEN** the component's output is the `AgentProfile` alone, and no `AgentRuntime`, ServiceAccount, or Secret carries a bundle label

#### Scenario: Pointing at a different runtime
- **WHEN** `profile.runtimeRef` names an existing AgentRuntime
- **THEN** the `AgentProfile` renders with that `runtimeRef` and the parent's `default` runtime is left unused by this profile

#### Scenario: Fallback needs no wiring
- **WHEN** `profile.runtimeRef` is empty and a runtime answers to `default`
- **THEN** the profile emits no `runtimeRef` and resolves the parent's `default` runtime

#### Scenario: The profile stays free of capabilities
- **WHEN** the bundle renders with the MCP component active
- **THEN** the `MCPConfig` is referenced by whichever Pipeline routes the conversation — the bundle's own or the install's — and the AgentProfile itself declares no `mcp` block and no tools, so profiles stay reusable across differently-tooled routes

#### Scenario: The demo reaches a Pipeline through the source it claims
- **WHEN** a `kind: task` signal is posted to a source claimed by the bundle's own wiring or by the install's Pipeline
- **THEN** the work unit carries that Pipeline's tools and the rendered AgentProfile declares none — the route is the one that claimed the source, and it is reached by posting to the source, never by naming the Pipeline or the profile

#### Scenario: An observe-only agent
- **WHEN** the claiming Pipeline binds the observation toolset without the shell or mutating ones
- **THEN** the agent reads the cluster but changes nothing, because the allowlist is the whole grant

#### Scenario: The repo-less agent still has a role
- **WHEN** the bundle renders with defaults
- **THEN** the AgentProfile carries an inline role describing the agent's job and how to act on a cluster, which the runtime appends to its system prompt

#### Scenario: The withheld posture is stated to the agent
- **WHEN** the bundle renders with `allowPodExecution` false (the default)
- **THEN** the AgentProfile's inline role ends with the posture paragraph: pod execution is withheld, a pod-template edit is declined with that reason, what remains possible is named, and the operator is pointed at making the edit

#### Scenario: The agent declines instead of failing
- **WHEN** `allowPodExecution` is false and a person asks the acting route to add a `nodeSelector` to a Deployment
- **THEN** the agent does not call a workload-patch tool, and its answer says the install withholds pod-template edits, what it could still do, and that the operator can make the change — never an RBAC refusal read back from the API server

#### Scenario: The paragraph follows the gate
- **WHEN** the bundle renders with `allowPodExecution` true
- **THEN** the inline role carries no posture paragraph, and the RBAC it describes is granted on the same render

#### Scenario: An operator's own prompt keeps the posture
- **WHEN** `profile.systemPrompt` is overridden and `allowPodExecution` is false
- **THEN** the posture paragraph is appended after the operator's text, unchanged
