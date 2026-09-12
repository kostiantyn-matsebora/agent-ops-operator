## Why

On the reference install, `k8s-operate` (the kubernetes bundle's admin route,
binding the `k8s-engineer` `AgentProfile`) was asked to add a `nodeSelector` to
a Deployment. It called the patch tool, the API server refused it, and the
agent reported "the MCP server works but the service account lacks
permission". The refusal was correct —
`global.agentops.runtimeDefaults.allowPodExecution` defaults off and gates
every write to a pod template — but the agent learned the posture only by
hitting it, and the person read a permissions fault instead of a decision the
install made on purpose.

The profile's prompt already tells the agent "if a tool is not in your
allowlist, say so plainly". That covers the toolset wall and not this one: the
MCP server advertises the workload-patch tool whatever the gate says, and RBAC
refuses it one hop later, where the agent cannot see it coming.

## What Changes

- The `kubernetes` bundle's `AgentProfile` system prompt gains a **posture
  paragraph rendered from the same value the RBAC reads**. When
  `allowPodExecution` is false, the profile tells the agent that this install
  withholds pod execution — no creating or entering pods, no edits to the pod
  template of a Deployment, StatefulSet, DaemonSet, ReplicaSet, Job or CronJob —
  so it does not attempt the change, states that the install withholds it,
  names what it can still do, and suggests the operator makes the edit. When
  the gate is on, the paragraph is absent.
- **One profile, not two.** The prompt becomes the third wall that moves with
  the one value, beside the route account's rules and the MCP server's role. A
  second hand-kept profile would drift from the gate the first time someone
  flipped it.
- The paragraph's text is a bundle value (`profile.podExecutionWithheldPrompt`),
  appended after `profile.systemPrompt` whether that prompt is the shipped one
  or the operator's own — the fact is the chart's, not the prompt author's.
- Chart render tests pin both states: the paragraph present with the gate off,
  absent with it on, and appended to an operator-supplied `systemPrompt`.
- No CRD, manager, runtime or RBAC change. Nothing is granted or withheld that
  was not already.

## Capabilities

### New Capabilities

(none)

### Modified Capabilities

- `k8s-bundle`: the profile component's inline role SHALL state the install's
  pod-execution posture, rendered from `allowPodExecution`, so the agent
  declines a pod-template edit with the reason instead of attempting it.

## Impact

**Code**

- `chart/charts/kubernetes/templates/profile.yaml` — reads
  `agentops.runtimePodExecutionAllowed` (the parent helper the RBAC already
  uses) and appends the paragraph when it answers false.
- `chart/charts/kubernetes/values.yaml` — the new `profile.podExecutionWithheldPrompt`
  value with its default text and the comment saying why it exists and why it
  is not a second profile.
- `chart/charts/kubernetes/Chart.yaml` — version bump.
- `platform/manager/internal/integration/charttemplate_test.go` — render
  assertions for both gate states and the operator-prompt case.

**Reference docs**

- `docs/configuration.md` — the `allowPodExecution` section gains the third
  wall: what the agent is told, and the value that holds the text.
- `docs/integrations/kubernetes.md` — the `allowPodExecution` callout says the
  agent knows the posture and declines rather than fails; the bundle's values
  reference lists the new key.
- `docs/CHANGELOG.md` — an Unreleased entry.
- `docs/cr-reference.md` and the generated blocks — unaffected (no CRD or
  chart-object change), confirmed by running `docs-generate.py --check`.

**Adopter site**

- The landing page, `introduction.md`, `getting-started.md`,
  `installation.md` and `docs/guides/*` make no claim about how the agent
  answers a withheld action, so none becomes untrue. Checked, not assumed;
  the documentation task records the check.

**Rules**

- `.claude/rules/wiring.md`, "BOTH WALLS MOVE TOGETHER" — becomes three walls,
  naming the prompt.
