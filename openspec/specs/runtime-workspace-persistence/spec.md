# runtime-workspace-persistence Specification

## Purpose
TBD - created by archiving change persistence-in-chart. Update Purpose after archive.
## Requirements
### Requirement: Agent session persistence is the shipped default
The chart SHALL provision the runtime home volume by default, so that agent
session files survive a runtime pod restart out of the box. An operator SHALL be
able to opt out explicitly for clusters without a suitable provisioner, and the
opt-out SHALL be documented alongside the symptom it avoids.

Turning persistence off SHALL NOT be silent at the moment it costs something. A
run whose session files are gone SHALL FAIL with an explicit reason rather than
answering without context, and that reason SHALL reach both the thread, for the
person waiting on it, AND the `Conversation`, for anyone asking later why the
agent does not remember. A warning that scrolls past in a chat surface is not a
record, and the runtime pod that emitted it has usually exited by the time the
question is asked.

#### Scenario: Fresh install persists sessions
- **WHEN** the chart is installed with no persistence values supplied
- **THEN** a claim is provisioned and the rendered `AgentRuntime` carries `home.pvcRef` naming it

#### Scenario: Cluster without a suitable provisioner
- **WHEN** an operator sets `persistence.enabled=false`
- **THEN** the install succeeds, runtime pods use ephemeral storage, and the documentation states that sessions die with each pod

#### Scenario: Lost session is explained, not hidden
- **WHEN** a run is asked to continue a context whose session files no longer exist
- **THEN** the run fails with an explicit reason instead of answering without context, the thread is told what happened and that a new conversation is the remedy, and the conversation records that it can no longer be continued

### Requirement: AgentRuntime can declare a persistent workspace volume
`AgentRuntime.spec` SHALL accept a `workspace` volume declaration alongside
`home`, referencing a PersistentVolumeClaim. When declared, the runtime pod
SHALL mount the repository checkout path from that claim; when absent, the
checkout path SHALL remain ephemeral.

The checkout path SHALL NOT move — claude-code sessions are keyed by working
directory, and relocating it breaks session resume.

#### Scenario: Workspace claim is mounted
- **WHEN** an `AgentRuntime` declares a workspace claim
- **THEN** runtime pods built from it mount the checkout path from that claim at the unchanged path

#### Scenario: Absent declaration stays ephemeral
- **WHEN** an `AgentRuntime` declares no workspace volume
- **THEN** runtime pods use ephemeral storage for the checkout, as before

### Requirement: Concurrent runtime pods never share one checkout
When a workspace claim is used, each conversation SHALL receive its own
subdirectory within the claim, mounted so that no two runtime pods can observe
or modify each other's checkout. A shared working tree across concurrent agents
SHALL NOT be possible by configuration.

#### Scenario: Two conversations, two trees
- **WHEN** two conversations run concurrently against the same workspace claim
- **THEN** each pod sees only its own checkout and neither can read or write the other's

#### Scenario: Restart mid-conversation keeps the tree
- **WHEN** a runtime pod for a conversation is deleted and recreated while the conversation is still open
- **THEN** the new pod sees the same checkout, including work not yet committed

### Requirement: Workspace persistence is opt-in and provisioned by the chart
The chart SHALL expose a workspace persistence block that is disabled by
default and, when enabled, provisions a claim and wires it into the rendered
`AgentRuntime` without the operator restating the claim name. The default SHALL
be off: a fresh checkout is cheap and always correct, whereas a stale shared
checkout is neither.

The chart SHALL support pointing at an existing claim instead of provisioning
one.

#### Scenario: Disabled by default
- **WHEN** the chart is installed with no workspace values supplied
- **THEN** no workspace claim is rendered and the `AgentRuntime` declares no workspace volume

#### Scenario: Enabling needs one value, not two
- **WHEN** an operator enables workspace persistence
- **THEN** the claim is provisioned and the rendered `AgentRuntime` references it with no runtime-side claim name set

#### Scenario: Existing claim is honored
- **WHEN** an operator names an existing claim for the workspace
- **THEN** the chart provisions nothing and the `AgentRuntime` references the named claim

