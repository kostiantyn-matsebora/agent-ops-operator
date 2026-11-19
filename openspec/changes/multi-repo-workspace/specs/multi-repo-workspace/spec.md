## Purpose

An agent works from one repository or from several. This capability is the repository list, how it resolves and is snapshotted, the workspace layout it produces, the agent file location, and the validation that refuses an inconsistent declaration before a pod exists.

## ADDED Requirements

### Requirement: A repository list is declared on a profile and on a capability

`AgentProfile.spec` and `AgentCapability.spec` SHALL each accept `repositories[]`. Each entry SHALL carry:

| Field | Meaning | Rule |
|---|---|---|
| `name` | the entry's identity in the list | required, a DNS-label, unique in the list, never containing `/` or `..` |
| `url` | the repository, in any form git understands | required |
| `ref` | branch, tag or commit to check out | default `main` |
| `auth` | `{type: ssh\|https, secretRef}` | optional, the shape `AgentProfile.spec.repository.auth` has today |
| `keepGit` | keep the `.git` directory after the clone | default `true` |

`AgentProfile.spec.repository` SHALL remain, DEPRECATED, as shorthand for a one-entry list. When both `repository` and `repositories` are set on one profile, the CRD SHALL refuse the object. The `name` of a one-entry list made from `repository` is inert.

```yaml
repositories:
  - name: app
    url: git@example.com:team/app.git
    ref: main
    auth: {type: ssh, secretRef: {name: app-deploy-key}}
  - name: infra
    url: https://example.com/team/infra.git
    ref: release-2
    keepGit: false
    auth: {type: https, secretRef: {name: infra-token}}
```

#### Scenario: A duplicate or unsafe name is refused by the API server

- **WHEN** a profile or capability lists two entries named `app`, or an entry named `a/b` or `..`
- **THEN** the API server rejects the object with the CEL message naming the rule

#### Scenario: The deprecated field still works

- **WHEN** a profile declares only `repository`
- **THEN** it resolves as a one-entry list with `keepGit` true
- **AND** a conversation behaves as it did before this change, apart from the workspace path

#### Scenario: Both spellings on one profile

- **WHEN** a profile declares `repository` and `repositories`
- **THEN** the API server rejects the object

### Requirement: A capability's list replaces the profile's list

The list a conversation uses SHALL be resolved as follows. A capability that declares a non-empty `repositories` (or an `agentFile`) REPLACES the profile's list, never merges with it. A capability that declares none leaves the profile's list in force. A Pipeline or Coordinator SHALL reach a list only through its inline `AgentCapabilitySpec` or through `agentRef` to an `AgentCapability`. Neither kind SHALL gain a field of its own for it.

When the resolved list is empty, the conversation SHALL have no checkout and its pod SHALL have no init container for one.

#### Scenario: The capability replaces

- **WHEN** a profile lists `app` and `infra`, and the wired capability lists `docs`
- **THEN** the conversation's list is exactly `docs`

#### Scenario: A silent capability leaves the profile's list

- **WHEN** the wired capability declares no `repositories`
- **THEN** the conversation uses the profile's list

#### Scenario: No list anywhere

- **WHEN** neither the profile nor the capability declares a repository
- **THEN** the pod has no clone init container and `/workspace` is an empty directory

#### Scenario: Two spellings, one result

- **WHEN** one Pipeline declares a list inline and another reaches the same list through an `AgentCapability`
- **THEN** conversations from either carry the same snapshot

### Requirement: The resolved list is snapshotted onto the Conversation

At conversation creation the manager SHALL write the resolved list, the resolved `agentFile` and the resolved `workingDir` onto the `Conversation`. Every later pod build, including the rebuild after a pod restart and the build after a reopen, SHALL read that snapshot and SHALL NOT read the live profile, capability or Pipeline for them. A secret reference travels in the snapshot as a reference, never as content.

#### Scenario: A later edit does not re-wire a running conversation

- **WHEN** a profile's `repositories` is edited after a conversation was created
- **THEN** the conversation's next pod build uses the list it was created with

#### Scenario: A reopened conversation keeps its snapshot

- **WHEN** a closed conversation is reopened after the profile changed
- **THEN** its pod is built from the original snapshot and no reference is resolved again

### Requirement: The workspace layout follows the number of repositories

The workspace SHALL be mounted at `/workspace` for every runtime. The layout SHALL be:

| Repositories | Where each lands | Agent starts in |
|---|---|---|
| none | `/workspace` is empty | `/workspace` |
| exactly one | the clone IS `/workspace` | `/workspace` |
| two or more | `/workspace/<name>/` | `/workspace`, or `/workspace/<workingDir>` |

The `name` of a single repository SHALL be inert. No `/workspace/<name>` alias SHALL exist for it. A prompt written for one repository therefore does not carry over to a list of many, and the documentation SHALL say so.

`workingDir` SHALL be an optional field beside `repositories`, naming an entry. It SHALL be valid only when the resolved list has two or more entries.

```
one repo:           /workspace/.git  /workspace/src  ...
two repos:          /workspace/app/.git  /workspace/infra/.git
workingDir: infra:  agent cwd = /workspace/infra
```

#### Scenario: One repository is the workspace

- **WHEN** a conversation resolves exactly one repository named `app`
- **THEN** its files are directly under `/workspace` and `/workspace/app` does not exist

#### Scenario: Two repositories side by side

- **WHEN** a conversation resolves `app` and `infra`
- **THEN** `/workspace/app` and `/workspace/infra` each hold a clone and the agent starts in `/workspace`

#### Scenario: workingDir selects the start directory

- **WHEN** a conversation resolves two repositories and `workingDir: infra`
- **THEN** the runtime is told to start in `/workspace/infra`

### Requirement: An agent file is named by location

`AgentProfile.spec` and `AgentCapability.spec` SHALL accept `agentFile: {repo?, path}`. It SHALL be mutually exclusive with `agent`, which stays as the name form for the conventional single-repository case. The rules SHALL be:

| Field | Rule |
|---|---|
| `repo` | optional when exactly one repository resolves, required when two or more |
| `path` | relative to the repository root, never absolute, never containing `..`, never escaping the repository through a symlink |

`agentFile` SHALL travel with the repository list. A capability that replaces the list also replaces `agentFile`. The manager SHALL resolve the file's absolute path under `/workspace` and hand it to the runtime. Each runtime SHALL decide how to apply it. The definition's body and tools are read by the runtime as `agent-definition-adopted-by-runtime` specifies, and this capability states only where the file is.

`prompt` and `replyPrompt` SHALL resolve against the agent file's repository, or the sole repository when there is no `agentFile`. A value MAY carry a repository qualifier in the form `<repo>:<path>` to name another entry.

```yaml
agentFile: {repo: infra, path: agents/reviewer.md}
prompt: app:prompts/review.md
```

#### Scenario: Both spellings are refused

- **WHEN** a profile sets `agent` and `agentFile`
- **THEN** the API server rejects the object

#### Scenario: The repo is optional with one repository

- **WHEN** one repository resolves and `agentFile` omits `repo`
- **THEN** the path resolves under `/workspace`

#### Scenario: A path that escapes is refused

- **WHEN** `agentFile.path` is `../other/agent.md` or `/etc/passwd`
- **THEN** the API server rejects the object
- **AND** a path that passes admission yet resolves outside the repository through a symlink is refused at pod start, naming the path

#### Scenario: A capability replaces the file with the list

- **WHEN** a profile declares `agentFile` against `app` and the wired capability replaces the list with `docs` and declares no `agentFile`
- **THEN** the conversation carries no `agentFile`

### Requirement: An inconsistent declaration is refused at resolution

The `Ready` condition on `Pipeline` and `Coordinator`, the two holders of a resolved capability, SHALL be False and name the cause when the resolved declaration holds any of:

| Cause | Reason names |
|---|---|
| `agentFile.repo` or a prompt qualifier names a repository not in the resolved list | the repo and the list |
| `workingDir` names a repository not in the resolved list | the repo and the list |
| `workingDir` is set with fewer than two repositories | the count |
| `agentFile.repo` is missing with two or more repositories | the list |
| two entries share a name (a list assembled across objects) | the name |

CEL on the CRDs SHALL cover what one object can express. No conversation SHALL be created from a holder that is not Ready for these causes, and the failure SHALL NOT be left to surface at pod start.

#### Scenario: A dangling working directory

- **WHEN** a Pipeline's capability resolves to `app` only and a `workingDir` is set
- **THEN** the Pipeline is not Ready and the reason says `workingDir` needs two or more repositories

#### Scenario: A capability replaces the list under a profile's agentFile

- **WHEN** the profile's `agentFile.repo` is `app` and the wired capability replaces the list with `docs` and `infra`, keeping no file
- **THEN** the holder is Ready, because the file went with the replaced list

#### Scenario: A missing repo with many

- **WHEN** two repositories resolve and `agentFile` omits `repo`
- **THEN** the holder is not Ready and the reason says `repo` is required

### Requirement: A conversation's workspace layout never changes

A conversation's repositories, agent file and working directory SHALL NOT change after creation. The one to many layout change SHALL therefore never happen to a running conversation. Conversation identity and the workspace `subPath` belong to `conversation-identity` and are not specified here.

#### Scenario: Changing the layout needs a new conversation

- **WHEN** an operator edits a profile from one repository to two
- **THEN** only conversations created afterwards use the two-repository layout
