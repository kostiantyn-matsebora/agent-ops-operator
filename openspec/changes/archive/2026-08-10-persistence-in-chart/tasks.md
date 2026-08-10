## 1. API surface

- [x] 1.1 Add `WorkspaceVolume` (`pvcRef` / `emptyDir`) and `AgentRuntimeSpec.Workspace` in `api/v1alpha1/agentruntime_types.go`, mirroring `HomeVolume`
- [x] 1.2 Add per-thread delivery markers to `RunStatus` in `api/v1alpha1/conversation_types.go` (delivered channel names, bounded with the existing run bound)
- [x] 1.3 Add bounded cooldown entries (`{fingerprint, at}`) to `SignalSourceStatus` in `api/v1alpha1/signalsource_types.go`
- [x] 1.4 Regenerate deepcopy and CRDs (`controller-gen object` + `crd` into `chart/files/crds/`) and confirm every new field is optional so existing objects stay valid

## 2. Runtime workspace persistence

- [x] 2.1 Add `WorkspacePVC` to the runtime pod builder config in `internal/runtimepod/podspec.go`
- [x] 2.2 Mount `/data/workspace` from the claim with `subPath` = conversation name when set, keeping the mount path unchanged; fall back to `emptyDir` when unset
- [x] 2.3 Resolve `spec.workspace` from the `AgentRuntime` CR in the reconciler path that already resolves `spec.home`
- [x] 2.4 Unit-test the pod spec for all three cases: no claim (emptyDir), claim set (subPath per conversation), and two conversations yielding distinct subPaths at the same mount path

## 3. Derivable reply delivery

- [x] 3.1 Add a stable-id enqueue for run replies (`send:<conversation>:<channel>:<runId>`) in `internal/chat/ops.go`, leaving counter-based ack/notice sends alone; update the `OpQueue` doc comment, which currently claims all sends are fire-and-forget
- [x] 3.2 Make `Router.FanOutSend` use the stable id when fanning a run result, and keep ack/notice fan-out on the existing path
- [x] 3.3 Mark the run/channel pair delivered when a reply op completes, via the op-completion path in `internal/httpapi/server.go`
- [x] 3.4 Add the reconciler backstop in `internal/controller/conversation_controller.go`: for each completed run with a recorded result, enqueue a reply for every bound thread lacking a marker
- [x] 3.5 Implement the upgrade backfill — runs that completed before the controller started are recorded as delivered without enqueueing, so an upgrade never re-posts history
- [x] 3.6 Tests: restart between `/work/done` and op claim re-derives the reply; a delivered run enqueues nothing; a partially delivered fan-out completes only the missing threads; backfill posts nothing

## 4. Durable cooldown

- [x] 4.1 Extend `internal/ingest/grouping.go` so a `Cooldown` can be constructed from recorded entries and can expose its current entries for persistence
- [x] 4.2 In `internal/httpapi/signals.go`, load recorded entries from the `SignalSource` on first use per source and write back only when a fingerprint is newly fresh
- [x] 4.3 Prune entries past their window on write and enforce the bound (oldest evicted first)
- [x] 4.4 Tests: suppression holds across a simulated restart; suppressed signals cause no writes; entries past the window are pruned; exceeding the bound degrades to today's behavior rather than erroring

## 5. Chart

- [x] 5.1 Flip `persistence.enabled` to `true` in `chart/values.yaml` and rewrite the block comment to lead with what the default does and how to opt out
- [x] 5.2 Add the `persistence.workspace` block (disabled by default: `enabled`, `name`, `size`, `storageClassName`, `accessModes`, `volumeName`, `existingClaim`)
- [x] 5.3 Render the workspace claim in `chart/templates/pvc.yaml` under the same `helm.sh/resource-policy: keep` posture
- [x] 5.4 Wire `workspace.pvcRef` in `chart/templates/runtime.yaml` from the parent's values, the way `home.pvcRef` is wired — no claim name restated by the operator
- [x] 5.5 Add an explicit `persistence.enabled: false` to CI/demo values so test installs do not depend on an RWX provisioner
- [x] 5.6 Extend `NOTES.txt` with the Pending-PVC diagnosis and the one-line opt-out
- [x] 5.7 Bump the chart version

## 6. Telemetry honesty

- [x] 6.1 Ensure a cursor from a previous manager process is answered with `resync` rather than an empty list in `internal/httpapi/activity.go`
- [x] 6.2 Render a resync boundary as a visible gap in the console activity timeline (`console/activity.go` + the SPA timeline view)
- [x] 6.3 Test that a post-restart reconnect produces a gap marker and that topology, conversations and configuration still render correctly

## 7. Documentation

- [x] 7.1 Write the restart-resilience matrix — component, state held, declared home, cost of a restart — and place it in `docs/concepts.md`
- [x] 7.2 Document `AgentRuntime.spec.workspace`, the delivery markers, and the cooldown entries in `docs/concepts.md`
- [x] 7.3 Document the `/work/done` delivery semantics and the stable reply op id in `docs/contracts.md`
- [x] 7.4 Document the activity gap behavior in `docs/console.md`
- [x] 7.5 Add the `CHANGELOG.md` entry (newest first): the `persistence.enabled` default flip, the RWX requirement and opt-out, the new workspace block, and the fact that upgrading re-posts nothing
- [x] 7.6 Record the durability model in `CLAUDE.md` as an invariant: filesystem state on a PVC, everything else in the Kubernetes API, the manager mounts nothing, and `close-topic` stays the one non-derivable op

## 8. Verification

- [x] 8.1 `go build ./... && go vet ./...` at the root and in every satellite module
- [x] 8.2 Full test run with `KUBEBUILDER_ASSETS` (unit + envtest)
- [x] 8.3 Add or extend an integration test in `internal/integration/` covering reply re-derivation after a controller restart
- [x] 8.4 Extend `internal/integration/charttemplate_test.go` to pin the persistence defaults (home on, workspace off) and the workspace wiring
- [x] 8.5 `helm template` both default and `persistence.enabled=false` renders, plus a server-side dry-run against the live cluster before any apply
- [x] 8.6 Live smoke: run a task, restart the manager between run completion and delivery, confirm the answer still lands exactly once
