# Tasks — validate Pipeline.spec.runtimeRef

## 1. The check

- [ ] 1.1 In `internal/controller/pipeline_controller.go`, resolve
      `p.Spec.RuntimeRef` alongside the other refs and append
      `agentruntime/<name>` to `missing` when the `AgentRuntime` does not exist.
      An unset ref resolves nothing and is not a miss — it means "the runtime
      named `default`", which the resolution chain handles
- [ ] 1.2 Add `AgentRuntime` to the reconciler's `Watches`, so applying the
      missing runtime converges `Ready` without anyone editing the Pipeline

## 2. Prove it

- [ ] 2.1 Unit or envtest coverage for the three cases: no `runtimeRef` stays
      Ready, a `runtimeRef` naming an existing `AgentRuntime` stays Ready, and
      one naming a missing runtime reports `Ready=False` naming it
- [ ] 2.2 Coverage that `spec.serviceAccountName` naming nothing is STILL Ready.
      The asymmetry is deliberate, and an untested deliberate asymmetry is
      indistinguishable from an oversight
- [ ] 2.3 `go build ./... && go vet ./... && go test ./...` in `platform/manager`

## 3. Documentation

- [ ] 3.1 **Reference docs** — `docs/concepts.md`: state that a Pipeline's
      `runtimeRef` is validated on `Ready` and its `serviceAccountName` is not,
      with the reason for the split
- [ ] 3.2 **The adopter site** — `docs/guides/agent-runtime.md` names what a
      Pipeline sees when it selects a runtime; say that a missing one is
      reported on the Pipeline rather than discovered at pod creation
- [ ] 3.3 **`docs/CHANGELOG.md`** — newest first: a Pipeline that was Ready with
      a dangling `runtimeRef` becomes `Ready=False` on upgrade, and what to do
      about it
