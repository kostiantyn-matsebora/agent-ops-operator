## 1. API types and snapshot — api-architect

- [ ] 1.1 Add optional `workspace` (`own` default, `shared`) and `repoContext` (`load` default, `omit`) to `CoordinatorAgentEntry` in `platform/manager/api/v1alpha1/`, with enums and doc comments, then regenerate deepcopy and `chart/crds/` with the two `controller-gen` commands from `build-test.md` and verify the generated diff is only those fields
- [ ] 1.2 Add the owner field (`WorkspaceOwnerID`, name settled in 1.3) and `RepoContext` to `state.ConversationRecord` and the kube and NATS mappings, and verify an absent value round-trips as `own` and `load` through both backends with the state conformance suite
- [ ] 1.3 Expose the owner field on the `/state/conversations` response in `internal/httpapi/state.go` and verify a conversation without sharing reads as its own owner

## 2. Owner resolution, readiness and snapshot — backend-developer

- [ ] 2.1 Resolve the workspace owner in `createMember` (`internal/chat/coordinate.go`) by following `causedBy` to the first conversation that does not share, and snapshot the owner's claim and `subPath` onto the member instead of calling `SnapshotFor` for the claim, verified by a unit test for a one-hop, a nested-sharing and an `own` member
- [ ] 2.2 Snapshot `workspace` and `repoContext` from the entry onto the member record at creation, and verify an attach to a live member keeps the recorded values after the entry changes
- [ ] 2.3 Refuse `shared` at Coordinator readiness when no workspace claim resolves, naming the entry in the condition, and refuse it again at `invoke` when the owner has no claim (nested Coordinators), verified by envtest cases for both refusals
- [ ] 2.4 Confirm `invoke` and the result path are untouched by running the existing coordination-loop tests unchanged

## 3. Pod builder — backend-developer

- [ ] 3.1 In `internal/runtimepod/podspec.go`, mount the owner's `subPath` for a sharing member and never the claim root, and verify with a pod-spec unit test that the volume source, mount path and `subPath` match the owner's
- [ ] 3.2 Give a sharing member no workspace init container, no git credential and no `REPO_URL` clone inputs, and verify the rendered pod carries none of them (after `multi-repo-workspace`, this is the absence of the `workspace-sync` init container)
- [ ] 3.3 Verify the member keeps its own context volume, so two members sharing one tree do not share context

## 4. repoContext on the work unit — backend-developer

- [ ] 4.1 Add `repoContext` to the work unit in `internal/dispatch`, sent only for `omit`, and verify a golden-fixture test pins an absent field for `load` and the field for `omit`
- [ ] 4.2 Verify the dispatch fixtures for every existing unit are unchanged

## 5. Runtimes honour repoContext — backend-developer

- [ ] 5.1 Verify, before implementing, whether `claude-cli` can skip `CLAUDE.md` and `.claude/` per invocation, and whether the Copilot SDK can skip repository instructions, skills and custom agents. Record the answer in `design.md` Open Questions as resolved or as a runtime that fails `omit` under `RT-4.1`
- [ ] 5.2 `runtimes/claude`: pass `settingSources: []` for `omit` and confirm no other path loads repository context, verified by `node --test` asserting the SDK options for both values
- [ ] 5.3 `runtimes/pi`: skip `pi-code` and its workspace approval for an `omit` unit, keep `PI_CODE_ENABLED` as a ceiling, verified by a `picode` test for the four combinations of env and unit
- [ ] 5.4 `runtimes/claude-cli` and `runtimes/copilot`: honour `omit` where 5.1 found a mechanism, otherwise fail the run naming `repoContext` (`RT-4.1`), verified by a test of each branch
- [ ] 5.5 `runtimes/ollama`: add a test pinning that no repository context is loaded, since it conforms by construction

## 6. Housekeeping liveness — backend-developer

- [ ] 6.1 In `platform/housekeeping`, treat a directory as live while any conversation in any phase carries its `ID` or records it as its owner, keeping the scan-then-list order, and verify a test of owner-deleted-member-live leaves the directory in place
- [ ] 6.2 Verify closing an owner leaves a sharing member's tree readable, with a test asserting `/close` removes only the pod, ConfigMap and slot

## 7. Chart — deployment-engineer

- [ ] 7.1 Verify the regenerated CRDs are the ones the chart ships, run the chart render tests, and confirm a Coordinator with the two new fields renders and an existing one renders unchanged

## 8. The runtime contract edit — the session, behind approval

- [ ] 8.1 Ask the user for explicit approval to edit `docs/contracts/runtime.md`, and make no edit before it is given
- [ ] 8.2 After approval, add `RT-4` and `RT-4.1`, the `repoContext` work-unit field, the MINOR version bump with a changelog row and the two checklist items, and move the claimed version on every runtime page under `docs/runtimes/`
- [ ] 8.3 Bring all five runtimes into conformance in this same change and verify with the runtime conformance suite, since the change is not archivable before that

## 9. Unit tests

- [ ] 9.1 Run `go build ./... && go vet ./... && go test ./...` in `platform/manager` with `KUBEBUILDER_ASSETS` set so the envtest suite runs, and in `platform/housekeeping`
- [ ] 9.2 Run `go test -tags conformance -count=1 -v ./test/conformance/` in `platform/manager`
- [ ] 9.3 Run `node --test` in `runtimes/claude`, `runtimes/claude-cli`, `runtimes/pi` and `runtimes/copilot`, and `go test ./...` in `runtimes/ollama`
- [ ] 9.4 Run the chart render tests and `python3 .github/scripts/docs-generate.py --check`
- [ ] 9.5 Run `python3 .github/scripts/publication-guard.py --counts` and `python3 .github/scripts/retired-vocabulary-guard.py --counts`, and record only the verdicts

## 10. System tests

- [ ] 10.1 Add a lane in `platform/manager/test/system/` where a Coordinator member with `workspace: shared` sees a file the owner wrote and the owner sees the member's edit, on an RWX workspace claim
- [ ] 10.2 Extend that lane to show a member restart keeps the shared tree, the owner closing does not remove it, and a Coordinator asking to share with no workspace claim is `Ready=False`
- [ ] 10.3 Add a lane showing a member with `repoContext: omit` runs without the checkout's context in the default `pi` runtime
- [ ] 10.4 Run the system pack against this working copy's tree, not the default branch's, and record the result

## 11. Documentation

### Reference docs

- [ ] 11.1 `docs/concepts.md`: the `agents[]` entry fields, the workspace volume section (the one-tree-per-conversation statement now has an exception), and the workspace owner term
- [ ] 11.2 `docs/contracts.md`: the owner field on `/state/conversations` and the work-unit field
- [ ] 11.3 `docs/crds/coordinator.md`: re-run `python3 .github/scripts/docs-generate.py`, and write the hand prose for when to choose `shared` and `omit`
- [ ] 11.4 `docs/security.md`: the shared filesystem as a path around ServiceAccount isolation, the control and the open risk, then re-run `threat-model.py` if a trust boundary moved
- [ ] 11.5 `docs/CHANGELOG.md`, `docs/gotchas.md` and `.claude/rules/invariants.md` and `.claude/rules/terminology.md` where they say a conversation owns its workspace

### Adopter site

- [ ] 11.6 The Coordinator guide under `docs/guides/`: how to split work into disjoint slices for a shared tree and when to omit repository context
- [ ] 11.7 The runtime pages under `docs/runtimes/`: what each runtime does for `repoContext: omit`
- [ ] 11.8 `docs/installation.md` and `docs/configuration.md`: the workspace claim's access mode requirement for sharing, and `persistence.workspace`
