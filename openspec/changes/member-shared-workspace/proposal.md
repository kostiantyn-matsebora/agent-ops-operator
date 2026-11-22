## Why

**A member cannot work on the tree its parent works on.**

- Every conversation mounts its own `subPath` of the workspace claim.
- A member that edits code edits a private copy the parent never sees.
- Work that edits one checkout together cannot be delegated. A development session with a backend role and a testing role is the case.

**A member inherits whatever the checkout carries for an agent.**

- That means `CLAUDE.md`, rules, skills and agent definitions.
- Both default runtimes load them. `pi` does through `pi-code`, and `claude` through the SDK's default `settingSources`.
- A member then holds context it did not need, which costs tokens and biases its work.
- The only control today is the chart-wide `pi.piCode`.

## What Changes

- **A shared workspace, opt-in per `agents[]` entry.** An entry may declare `workspace: shared`. The member then mounts the workspace owner's claim and `subPath` instead of resolving its own. The owner is found by following `causedBy` to the first conversation that does not itself share.
  - The default stays `own`. Nothing changes for an existing Coordinator.
  - Sharing needs a workspace claim. A Coordinator whose members would share an `emptyDir` is refused at readiness, because there is nothing to share.
  - The shared tree has no write coordination. The parent splits work into disjoint slices, as it does for Claude Code subagents.
- **A completed tree is never reset.** A member that shares a tree starts on a populated checkout and must not run `git reset --hard`. This depends on `multi-repo-workspace`, whose `workspace-sync` init container never touches a completed clone.
- **Per-member control of repository context.** An entry may declare `repoContext: omit`. The runtime then loads none of the checkout's `CLAUDE.md`, rules, skills, hooks or agent definitions into that member's context. The member still receives its own capability's definition and prompt.
  - The signal reaches the runtime on the work unit, so the setting is per conversation and not per pod image.
  - **BREAKING for `docs/contracts/runtime.md`.** A new work-unit field and a runtime obligation. The specification is not edited without explicit approval, and every shipped runtime is brought into conformance in the same change.
- **The shared-tree rule is amended.** `runtime-workspace-persistence` says a shared working tree across concurrent agents SHALL NOT be possible by configuration. That becomes: not possible except by a Coordinator entry that declares it.
- **Lifecycle.** Housekeeping treats a shared `subPath` as live while any conversation that mounts it is open, and closing the owner does not delete a tree a member still uses.

Not in scope:
- A synchronous result path for `invoke`. A member's result already returns to its parent as an input, and an attach to a live member already reuses its context.
- Write locking or merging on the shared tree.
- Read scoping inside the tree. `workingDir` belongs to `multi-repo-workspace`.

## Capabilities

### New Capabilities
- `member-workspace-sharing`: the `workspace` entry field, owner resolution along `causedBy`, the claim requirement, the never-reset rule on a shared tree and the lifecycle of a shared `subPath`.
- `member-repo-context`: the `repoContext` entry field, its delivery on the work unit and the runtime obligation not to load checkout context when it is `omit`.

### Modified Capabilities
- `coordinator-model`: an `agents[]` entry carries `workspace` and `repoContext`, and readiness names an entry that asks to share without a workspace claim.
- `runtime-workspace-persistence`: the requirement that concurrent pods never share a checkout gains the one declared exception.
- `runtime-contract`: the work unit carries `repoContext` and a runtime honours it. Covers `claude` and `claude-cli`.
- `pi-agent-runtime`: `pi-code` is not loaded for a member whose `repoContext` is `omit`.
- `copilot-agent-runtime`: honours `repoContext`.
- `ollama-agent-runtime`: honours `repoContext`.
- `conversation-housekeeping`: a shared `subPath` is live while any mounting conversation is open.

## Impact

Sequencing: apply after `multi-repo-workspace` (the never-reset rule and the init container) and `conversation-identity` (the `subPath` moves from the name to the identity).

- **Manager**: `api/v1alpha1` (`CoordinatorAgentEntry`, CRD and deepcopy regenerated), `internal/chat/coordinate.go` (`createMember` resolves the owner's claim and `subPath`), `internal/runtimepod/podspec.go` (the mount), `internal/dispatch` (the work unit field), Coordinator readiness.
- **Runtimes**: `runtimes/claude` (`settingSources: []`), `claude-cli`, `pi` (`piCodeLoaderOptions`), `copilot`, `ollama`.
- **Housekeeping**: `platform/housekeeping` liveness of a shared `subPath`.
- **Security**: a shared filesystem is a path around ServiceAccount isolation, since a low-privilege member can write files a higher-privilege parent later executes.
- **Reference docs**: `docs/concepts.md` (the `agents[]` entry, the workspace volume section), `docs/contracts/runtime.md` (after approval), `docs/contracts.md`, `docs/crds/coordinator.md`, `docs/security.md`, `docs/CHANGELOG.md`, `docs/gotchas.md`.
- **Adopter site**: the Coordinator guide under `docs/guides/`, `docs/runtimes/` pages for each runtime's `repoContext` behaviour, `docs/installation.md` where it states the workspace claim's access mode, `docs/configuration.md` for `persistence.workspace`.
- **Rules**: `.claude/rules/invariants.md` and `.claude/rules/terminology.md` where they say a conversation owns its workspace.
