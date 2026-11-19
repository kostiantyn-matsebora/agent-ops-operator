## 1. API types and snapshot — api-architect

- [ ] 1.1 Add `Repositories`, `AgentFile` and `WorkingDir` to `AgentProfileSpec` and `AgentCapabilitySpec` with the entry fields `name`, `url`, `ref`, `auth`, `keepGit`. Verify the CRDs regenerate (`controller-gen object` and `crd`) and `chart/crds/` shows the fields.
- [ ] 1.2 Mark `AgentProfileSpec.Repository` deprecated and treat it as a one-entry list. Verify with a unit test that `repository` and `repositories` together are refused by CEL.
- [ ] 1.3 Add the CEL rules: unique and safe repo names, `agent` exclusive with `agentFile`, and no `..` or leading `/` in a path. Verify with envtest cases for each refusal.
- [ ] 1.4 Add the resolved list, `agentFile` and `workingDir` to the Conversation record and its CRD mapping in both state backends. Verify the conformance suite round-trips them.

## 2. Resolution, snapshot and readiness — backend-developer

- [ ] 2.1 Resolve the list in `dispatch.ResolveCapability`: the capability's list replaces the profile's, `repository` counts as one entry, both empty means no checkout. Verify with table tests for every precedence case.
- [ ] 2.2 Snapshot the resolved list at conversation creation next to `SnapshotFor`, and make `runtimepod.Build` read the snapshot instead of the live profile for repositories. Verify a profile edit after creation changes nothing for a rebuilt pod, and a reopen reuses the snapshot.
- [ ] 2.3 Report a dangling `agentFile.repo` or prompt qualifier, a `workingDir` naming an unknown repo, a `workingDir` with fewer than two repos, and a missing `agentFile.repo` with two or more repos as a `Ready` condition on `Pipeline` and `Coordinator`. Verify each cause in the envtest suite with its reason string.

## 3. The workspace-sync component — backend-developer

- [ ] 3.1 Create `platform/workspace-sync` as its own module and image, discovered by `.github/components.sh` as `agentops-workspace-sync`. Verify `components.sh images` and `modules` list it and the multi-arch build is wired.
- [ ] 3.2 Implement the manifest read and the layout: one repo is the workspace root, two or more clone to `<name>/`. Verify with tests against local bare repositories for the zero, one and many cases.
- [ ] 3.3 Implement credentials without persisting them: `GIT_ASKPASS` or a per-command header for https, a 0600 key file outside `/workspace` with `GIT_SSH_COMMAND` for ssh. Verify no file under the workspace and no git config contains the token or key after a clone.
- [ ] 3.4 Implement the re-run rule: completion marker per repo written last, a completed clone is never touched, a partial one is removed and cloned again, `.agentops/` excluded in git, `keepGit: false` removes `.git`. Verify with tests that kill a clone midway and re-run, and that uncommitted edits survive a re-run.
- [ ] 3.5 Verify the agent file exists, is regular and stays inside its repository after symlinks. Verify with tests for a missing file, a symlink escape and a directory.
- [ ] 3.6 Write the termination message naming the repository and the cause and never an env value. Verify with a test that plants a token in the environment and a failing clone.

## 4. Pod builder and contract — backend-developer

- [ ] 4.1 Add the init container with the manifest env var and per-repo `valueFrom` credentials on it only, and mount the workspace at `/workspace`. Verify a pod-spec test asserts no git credential and no `REPO_*` or `GIT_*` variable on the agent container.
- [ ] 4.2 Set `WORKSPACE` always and `START_DIR` when `workingDir` is set, and pass `agentFile` as an absolute path in the work unit. Verify with pod-spec and dispatch tests.
- [ ] 4.3 Resolve `prompt` and `replyPrompt` against the `agentFile` repo with the `<repo>:<path>` qualifier. Verify with tests for a plain path, a qualified path and a name with a colon.
- [ ] 4.4 Add the unmet-clone failure to the start-deadline condition path. Verify an unreachable repository leaves the pod in `Init:Error` and the condition carries the kubelet reason.

## 5. Runtimes stop cloning — backend-developer

- [ ] 5.1 Remove the clone, `REPO_URL`, `REPO_REF` and `GIT_*` handling from `runtimes/claude`, `claude-cli` and `copilot`, read `WORKSPACE` and `START_DIR`, and use `agentFile`. Verify `node --test` passes in each, with the git-clone tests replaced by tests that nothing is cloned.
- [ ] 5.2 Do the same in `runtimes/pi`. Verify `node --test`.
- [ ] 5.3 Do the same in `runtimes/ollama`. Verify `go test ./...`.
- [ ] 5.4 Update `test/stubruntime` and the conformance suite for the new contract and drop RT-1 and MGR-5. Verify `go test -tags conformance ./test/conformance/`.

## 6. Chart and release wiring — deployment-engineer

- [ ] 6.1 Add the `workspace-sync` image to the chart values and the manager's configuration, and move the runtime bundles to the new tags. Verify the chart render tests pass and `helm template` shows the init image.
- [ ] 6.2 Bump the chart major version and the image tags consistently. Verify `python3 .github/scripts/docs-generate.py --check` and the release-docs hook agree.
- [ ] 6.3 Add the retired-vocabulary entries named in the design. Verify `python3 .github/scripts/retired-vocabulary-guard.py` passes.

## 7. Unit tests

- [ ] 7.1 Run `go build ./... && go vet ./... && go test ./...` in `platform/manager` with `KUBEBUILDER_ASSETS` set, and `go test -tags conformance -count=1 ./test/conformance/`.
- [ ] 7.2 Run the same Go gates in `platform/workspace-sync` and `runtimes/ollama`, and `node --test` in `runtimes/claude`, `claude-cli`, `copilot` and `pi`.
- [ ] 7.3 Run the chart render tests and the script suite touched by the release wiring.

## 8. System tests

- [ ] 8.1 Add a lane to `platform/manager/test/system/` that runs a one-repo and a two-repo conversation, asserts the layout at `/workspace` and `/workspace/<name>`, and asserts the agent container holds no git credential. Verify with `go test -tags system -count=1 -timeout 45m -v ./test/system/` against this working copy's tree, or dispatch `system-smoke.yml` on this branch if k3d is unavailable.
- [ ] 8.2 Extend the lane to a failed clone: an unreachable repository reaps the pod with the kubelet reason, and a restart with `persistence.workspace` on keeps uncommitted edits.

## 9. Documentation

### 9.1 Reference docs

- [ ] 9.1.1 Update `docs/contracts/runtime.md` (RT-1 and MGR-5 gone, `/workspace`, `WORKSPACE`, `START_DIR`, `agentFile`) and `docs/contracts.md`.
- [ ] 9.1.2 Update `docs/concepts.md` for the repository list, resolution, layout and the path move, and `docs/crds/agent-profile.md` and `docs/crds/agent-capability.md` through their markers.
- [ ] 9.1.3 Update `docs/security.md` (the credential is no longer in the agent container), `docs/gotchas.md`, and `docs/CHANGELOG.md` with the breaking change and upgrade steps.
- [ ] 9.1.4 Update `.claude/rules/terminology.md` and `.claude/rules/invariants.md` where they state `/data/workspace` never moves.
- [ ] 9.1.5 Re-run `python3 .github/scripts/docs-generate.py` and commit the generated blocks.

### 9.2 Adopter site

- [ ] 9.2.1 Update `docs/guides/agent-from-a-repository.md`, `docs/guides/agent-profile.md` and `docs/guides/agent-runtime.md`, including the note that a one-repo prompt does not carry to two repos.
- [ ] 9.2.2 Update `docs/runtimes/claude.md`, `claude-cli.md`, `copilot.md`, `ollama.md` and `pi.md`.
- [ ] 9.2.3 Update `docs/installation.md` and `docs/configuration.md` for the new image and the upgrade steps, and `docs/getting-started.md` and the landing page where they show a repository.
- [ ] 9.2.4 Run `python3 .claude/scripts/rules_compliance.py` over the touched pages, `python3 .github/scripts/publication-guard.py --counts` and `python3 .github/scripts/retired-vocabulary-guard.py --counts`.
