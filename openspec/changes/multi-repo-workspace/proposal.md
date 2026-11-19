## Why

An agent can run from exactly one repository today. `AgentProfile.spec.repository` holds one `{url, ref, auth}`, every runtime clones it into `/data/workspace`, and `spec.agent` names a definition that must live in that same repo.

That breaks down when work spans repositories, for example an application repo plus an infrastructure repo, or an agent definition kept apart from the code it works on.

The clone is also copied into every runtime. Five runtimes now carry their own clone routine, with the git credential inside the agent container. It is the wrong place for both reasons.

## What Changes

- **Many repositories.** `repositories[]` on `AgentProfile` and on `AgentCapability`. Each entry has `name`, `url`, `ref`, `auth` and `keepGit` (default `true`).
  - A capability's list replaces the profile's. Lists never merge.
  - A Pipeline or Coordinator reaches a list through its inline capability or a `capabilityRef`, so neither kind gains a field.
  - `repository` stays as deprecated shorthand for a one-entry list.
  - The resolved list is snapshotted onto the Conversation at creation, like the persistence claim names.
- **Workspace layout.**
  - One repo is the workspace: the clone is `/workspace`.
  - Two or more repos each land in `/workspace/<name>/`.
  - The agent starts in `/workspace`, or in a repo when `workingDir: <repo name>` is set (two or more repos only).
- **Agent definition by location.** `agentFile: {repo, path}` names the definition file. `repo` is optional with one repo. It is exclusive with `agent`, and `prompt` and `replyPrompt` resolve against the same repo by default.
- **Cloning leaves the runtime.** An init container in a new `platform/workspace-sync` component clones every repo before the runtime starts.
  - It is the only holder of git credentials, and the token never lands in `.git/config`.
  - A failed clone is a pod-start failure, reported through the existing start-deadline condition.
  - `keepGit: false` removes `.git` after the clone.
- **BREAKING: the path moves.** The workspace mount moves from `/data/workspace` to `/workspace`, so the stored transcript key changes. The `REPO_URL`, `REPO_REF` and `GIT_*` environment variables leave the runtime contract, and runtimes stop cloning. Delete conversations before upgrading.
- **Validation.** A `Ready` condition on Pipeline and Coordinator names a dangling `agentFile.repo` or `workingDir`.

Not in scope:
- Conversation identity and the workspace `subPath`. That is `conversation-identity`.
- Merging repo lists by name.
- Pushing credentials or write policy per repo beyond `keepGit`.

## Capabilities

### New Capabilities
- `multi-repo-workspace`: the repository list, its resolution and snapshot, the workspace layout, `workingDir`, `agentFile` and the validation condition.
- `workspace-clone-init`: the init container that clones, its credential handling, its idempotent re-run and `keepGit`.

### Modified Capabilities
- `runtime-contract`: the runtime no longer clones. It covers `claude`, `claude-cli` and `pi`, which have no capability spec of their own. It receives a populated `/workspace`, a start directory and a resolved agent file path.
- `runtime-workspace-persistence`: the mount path is `/workspace`.
- `profile-is-identity`: a profile declares repositories and an agent file location.
- `agent-capability-model`: a capability may declare repositories and replaces the profile's list.
- `agent-definition-tools`: the definition is read from the resolved agent file.
- `copilot-agent-runtime`: no clone, new workspace path.
- `ollama-agent-runtime`: no clone, new workspace path.

## Impact

- **Manager**: `api/v1alpha1` (AgentProfile, AgentCapability, Conversation snapshot, CRDs and deepcopy regenerated), `internal/runtimepod` (init container, mount, env), `internal/dispatch` capability resolution, Pipeline and Coordinator readiness.
- **New component**: `platform/workspace-sync`, with its own image published under the existing component rules.
- **Runtimes**: `runtimes/claude`, `claude-cli`, `copilot`, `ollama`, `pi`. Each drops its clone and reads `WORKSPACE` at `/workspace`.
- **Depends on** `agent-definition-adopted-by-runtime`, which makes the runtime read the definition. This change gives it an explicit location.
- **Chart**: runtime bundles, the new image, a major version because the path and contract change.
- **Reference docs**: `docs/concepts.md`, `docs/contracts/runtime.md`, `docs/contracts.md`, `docs/crds/agent-profile.md`, `docs/crds/agent-capability.md`, `docs/security.md` (credential no longer in the agent container), `docs/CHANGELOG.md`, `docs/gotchas.md`.
- **Adopter site**: `docs/guides/agent-runtime.md`, `docs/guides/agent-profile.md`, `docs/installation.md`, `docs/configuration.md`, the runtime pages under `docs/runtimes/`, and `docs/getting-started.md` where it shows a repository.
- **Rules**: `.claude/rules/terminology.md` (the `/data/workspace` row) and `.claude/rules/invariants.md`.
