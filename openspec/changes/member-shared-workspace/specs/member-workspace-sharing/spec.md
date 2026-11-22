## ADDED Requirements

### Requirement: An agents entry declares whether its member shares the workspace

An `agents[]` entry of a `Coordinator` SHALL accept a `workspace` field with two values:

| Value | Meaning |
|---|---|
| `own` | the member gets its own workspace directory, as every conversation does today |
| `shared` | the member mounts the workspace directory of its workspace owner |

An absent field SHALL read as `own`. Silence SHALL grant nothing: no Coordinator, member or chart value makes a member share a tree unless its entry says `shared`. Sharing SHALL NOT be inferred from two conversations resolving the same claim.

The field SHALL be valid on an entry naming a `capabilityRef` and on one naming a `coordinatorRef`. On a `coordinatorRef` entry the invoked Coordinator's root is the member that shares.

```yaml
spec:
  agents:
    - name: backend
      description: Edits the service code
      capabilityRef: backend-dev
      workspace: shared
    - name: reviewer
      description: Reads and comments, never edits
      capabilityRef: reviewer
```

#### Scenario: The default is own
- **WHEN** an entry omits `workspace`
- **THEN** its members mount their own workspace directory, exactly as before this change

#### Scenario: A value outside the enum is refused
- **WHEN** an entry carries `workspace: both`
- **THEN** the API server rejects the manifest

#### Scenario: A nested Coordinator entry may share
- **WHEN** an entry names a `coordinatorRef` and declares `workspace: shared`
- **THEN** the invoked Coordinator's root mounts the invoker's workspace owner's directory

### Requirement: The workspace owner is the first non-sharing conversation along causedBy

The workspace owner of a member that shares SHALL be found by following its `causedBy` chain to the first conversation that does not itself share. The member SHALL mount that owner's claim and subdirectory.

A member of a sharing member that also shares SHALL therefore resolve to the same owner as its parent, never to its parent. A member that does not share SHALL be its own owner, and so SHALL its own members' owner when they share.

The owner SHALL be resolved when the member is created and recorded on the member's conversation. It SHALL NOT be resolved again. A later edit to the `Coordinator`, to any capability or to the owner's own record SHALL NOT move an existing member to another tree.

#### Scenario: A member shares its parent's tree
- **WHEN** a root invokes an entry declaring `workspace: shared`
- **THEN** the member mounts the root's claim and subdirectory, and files either writes are visible to the other

#### Scenario: A chain of sharing members has one owner
- **WHEN** a root invokes a sharing member and that member invokes a further entry declaring `workspace: shared`
- **THEN** the second member mounts the root's directory, not the first member's

#### Scenario: A non-sharing member in the middle starts a new tree
- **WHEN** a root invokes a member with `workspace: own`, and that member invokes an entry declaring `workspace: shared`
- **THEN** the second member mounts the first member's directory, and the root's directory is not mounted

#### Scenario: A re-wire does not move a running member
- **WHEN** a `Coordinator` entry is edited from `shared` to `own` while a member created under it is open
- **THEN** the member keeps mounting the owner's directory

### Requirement: A shared workspace forces the owner's claim and needs one to exist

A member that shares SHALL mount the workspace claim of its owner. The claim its own capability would have resolved SHALL NOT be consulted for the workspace, and the two claim names need not match. The member's context volume SHALL remain the one its own capability resolves.

A workspace that resolves to no claim is an ephemeral per-pod volume and cannot be shared. Therefore:

- A `Coordinator` with an entry declaring `workspace: shared` SHALL be `Ready=False` when the workspace claim its own root would resolve is absent, naming the entry. A Coordinator that is not Ready claims nothing.
- An `invoke` that would create a sharing member whose owner has no workspace claim SHALL be refused, naming the entry and the reason, and SHALL create no conversation. This covers an owner reached through a nested Coordinator.

#### Scenario: Shared on an ephemeral workspace is refused at readiness
- **WHEN** a Coordinator lists an entry with `workspace: shared` and its root would resolve no workspace claim
- **THEN** `Ready` is False with the entry's name and the missing claim in its message, and its sources are not claimed

#### Scenario: Claims that differ do not matter
- **WHEN** a member's capability names workspace claim `B` and its owner mounts claim `A`
- **THEN** the member mounts `A`, and no refusal or warning is raised for the mismatch

#### Scenario: An invoke on an unshareable owner is refused
- **WHEN** a sharing entry is invoked and the owner has no workspace claim
- **THEN** the invoke is refused with a reason naming the entry and the missing claim, and no conversation is created

### Requirement: A member on a shared tree never clones into it and never resets it

A member that shares SHALL start on the tree its owner populated. The manager SHALL NOT add a workspace-sync init container to it and SHALL NOT project any git credential from the member's own capability. The member's own `repositories` SHALL NOT be cloned into the shared tree.

No component SHALL run `git reset --hard`, `git clean` or any rewrite of the tree on a member's behalf. A completed clone is never touched, as `workspace-sync` specifies for every conversation.

#### Scenario: A sharing member gets no init container
- **WHEN** a pod is built for a member whose entry declares `workspace: shared`
- **THEN** the pod has no workspace-sync init container and carries no repository credential

#### Scenario: The parent's uncommitted edits survive a member's start
- **WHEN** the owner has uncommitted edits and a sharing member's pod starts or restarts
- **THEN** those edits are present and unchanged afterwards

### Requirement: The shared tree has no write coordination

The system SHALL NOT lock, merge or serialise writes to a shared tree. Two conversations that write the same path at the same time produce whatever the filesystem produces.

Dividing the work into disjoint slices SHALL be the invoking agent's responsibility. The documentation SHALL say so next to the field.

#### Scenario: No lock exists
- **WHEN** two members sharing a tree run concurrently
- **THEN** neither is delayed or refused because of the other's writes

### Requirement: A shared directory lives while any conversation mounts it

A shared workspace directory SHALL be live while ANY conversation that mounts it exists, whatever its phase. Closing the owner SHALL NOT remove, empty or detach the directory a member still mounts. Closing the owner releases its pod and ConfigMap explicitly, as it does for any conversation, and nothing else.

Deleting the owner's object SHALL NOT cause the directory's reclamation while another conversation's record names it (`conversation-housekeeping`).

#### Scenario: Closing the owner leaves the member's tree
- **WHEN** the owner is closed while a sharing member is open
- **THEN** the member's pod continues to see the same files

#### Scenario: A member outlives its owner's object
- **WHEN** the owner conversation is deleted and a member that mounts its directory still exists
- **THEN** the directory remains until the last such conversation is gone

### Requirement: Sharing is visible on the conversation

A conversation that mounts another's directory SHALL record, in its own record and readable through the state read surface, which conversation owns the directory it mounts. A conversation that owns its directory SHALL record no owner, and an absent owner SHALL read as itself.

#### Scenario: The owner is readable
- **WHEN** a sharing member is listed through the state read surface
- **THEN** its record names the identity of the conversation whose directory it mounts
