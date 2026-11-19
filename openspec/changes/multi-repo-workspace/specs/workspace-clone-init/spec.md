## Purpose

The workspace is populated before the agent starts, by an init container in its own component, so no runtime holds a git credential or carries a clone routine. This capability is that init container, its credential handling, its re-run rule and its failure reporting.

## ADDED Requirements

### Requirement: An init container clones every repository before the runtime starts

The manager SHALL add an init container to a runtime pod whose resolved repository list is non-empty. The container SHALL run the image of the `platform/workspace-sync` component, published as `agentops-workspace-sync` and built for `linux/amd64` and `linux/arm64`. It SHALL mount the workspace at `/workspace`, clone each entry per the layout in `multi-repo-workspace`, and exit before the agent container starts. The list SHALL reach it as one JSON manifest in a single environment variable. Each entry SHALL carry its `name`, `url`, `ref`, `keepGit` and the names of the environment variables holding its credential.

#### Scenario: Two repositories are ready before the agent starts

- **WHEN** a pod is built for a conversation resolving `app` and `infra`
- **THEN** the init container exits 0 with both clones in place before the runtime container starts

#### Scenario: No list, no init container

- **WHEN** the resolved list is empty
- **THEN** the pod spec has no workspace-sync container

### Requirement: Credentials reach the init container only and never persist

The manager SHALL read no secret. Each repository's `auth.secretRef` SHALL compile to `valueFrom` environment on the init container alone, with per-repository variable names. The agent container, and every sidecar, SHALL hold no git credential.

The init container SHALL NOT write a credential into `.git/config`, a remote URL, the workspace or any file under it. An https token SHALL be supplied per command through `GIT_ASKPASS` or a credential helper, or an `http.extraHeader`. An ssh key SHALL be written to a file outside the workspace and used through `GIT_SSH_COMMAND`. The remote `origin` SHALL hold the URL as declared.

#### Scenario: The token is not in origin

- **WHEN** an https clone with a token completes
- **THEN** `git remote get-url origin` prints the declared URL and no file under `/workspace` contains the token

#### Scenario: The agent container has no credential

- **WHEN** a pod is built for a repository with `auth`
- **THEN** no environment variable and no mounted secret for it exists on any container except the init container

#### Scenario: Two repositories, two keys

- **WHEN** `app` uses one ssh key and `infra` another
- **THEN** each clone authenticates with its own key and neither key is readable by the other repository's command

### Requirement: keepGit controls whether history stays

When `keepGit` is true the clone SHALL keep its `.git` directory. When false the init container SHALL clone with depth 1 and delete `.git`.

#### Scenario: keepGit false leaves no history

- **WHEN** an entry has `keepGit: false`
- **THEN** its directory holds the files at `ref` and no `.git`

### Requirement: The init container is idempotent under a restart

The init container SHALL record completion per repository, as a marker under `/workspace/.agentops/sync/<name>.done` written after the clone succeeds. On a re-run in the same conversation, which holds a surviving workspace only when `persistence.workspace` is on, it SHALL act per repository:

| State found | Action |
|---|---|
| directory empty or absent | clone |
| marker present | leave the repository untouched, whatever `keepGit` says |
| no marker (a partial clone) | remove that repository's contents and clone again |

For a single repository the clone is `/workspace` itself, so removal SHALL spare `/workspace/.agentops`. When `.git` is kept, `.agentops/` SHALL be added to `.git/info/exclude`.

A completed clone SHALL NOT be reset, cleaned or fetched into, because the agent owns the tree after the first clone and uncommitted work must survive a pod restart.

#### Scenario: Uncommitted work survives a restart

- **WHEN** a pod restarts and its repository has a marker and uncommitted edits
- **THEN** the edits are present when the runtime starts

#### Scenario: A killed clone is redone

- **WHEN** a pod died mid-clone and the repository has no marker
- **THEN** the next run removes that repository's contents and clones again

#### Scenario: A neighbour is not disturbed

- **WHEN** `infra` is partial and `app` has a marker
- **THEN** only `infra` is removed and cloned

### Requirement: The path of an agent file is verified before the runtime starts

When the manifest names an agent file, the init container SHALL verify after cloning that the path exists, is a regular file, and resolves inside its repository once symlinks are followed. A failure SHALL fail the container with a message naming the repository and path.

#### Scenario: A symlink points outside the repository

- **WHEN** the agent file is a symlink to a target outside its repository
- **THEN** the init container exits non-zero naming the path

### Requirement: A failed clone is a pod-start failure reported verbatim

A clone, authentication, agent file or manifest failure SHALL exit the init container non-zero with a message naming the repository and the cause, and no credential. The pod SHALL then be treated as one that never starts. The existing start deadline SHALL reap it and free its admission slot, and the condition on the `Conversation` SHALL carry the kubelet's reason verbatim.

#### Scenario: A bad credential

- **WHEN** a clone is refused for authentication
- **THEN** the init container fails, the pod never starts, and at the deadline the conversation's condition names the init container's failure reason, and the pod is reaped

#### Scenario: A message names no secret

- **WHEN** an https clone fails with a token set
- **THEN** the container's log and termination message contain the repository name and cause and not the token
