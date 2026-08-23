# Validate `Pipeline.spec.runtimeRef` on Ready

## Why

**A Pipeline naming an `AgentRuntime` that does not exist reports `Ready=True`.**

`pipeline-model` states the opposite, and carries a scenario for it:

> `spec.runtimeRef` IS validated, because the manager already reads
> `AgentRuntime` and a dangling runtime ref is the same class of dangling
> reference every other stanza reports.

`internal/controller/pipeline_controller.go` validates `signalSourceRefs`,
`channelRefs`, `profileRef`, `toolsets.refs` and `mcpConfigs.refs`. It does not
validate `runtimeRef`.

**Found by the `truthful-specs` audit**, which reads every published requirement
against the code. Its rule for this case is explicit: a requirement that is true
and desirable and NOT satisfied is a DEFECT, raised as its own change — never
resolved by weakening the requirement, which converts a bug into a decision
nobody made. So the spec was left as written and this change exists.

## What Changes

- The Pipeline reconciler resolves `spec.runtimeRef` and reports a missing
  `AgentRuntime` on `Ready=False`, in the same `unresolved references:` message
  every other dangling ref already uses.
- The reconciler watches `AgentRuntime`, so creating the missing runtime
  converges the condition without an edit.

## Impact

- **Behaviour change, and it can turn a Ready Pipeline unready on upgrade.** That
  is the point — such a Pipeline is already broken, and today the failure
  surfaces only when a pod is built, far from the object that caused it.
- **`spec.serviceAccountName` is still NOT validated**, and that asymmetry stays.
  Checking an account exists needs a `serviceaccounts` read the manager holds no
  RBAC for, and granting a permission to produce a warning is a worse trade than
  a failure that is already loud, local and named: the pod is refused at
  admission naming the account. The manager ALREADY reads `AgentRuntime`, so no
  such trade applies here.
- **Affected spec**: none. `pipeline-model` already describes the behaviour this
  change implements, which is why the change carries no delta.

## Out of scope

- The console's equivalent cross-check. `configapi.go:findings()` checks
  AgentProfile `runtimeRef` and not a Pipeline's; it derives findings
  independently and can be brought in line separately.
