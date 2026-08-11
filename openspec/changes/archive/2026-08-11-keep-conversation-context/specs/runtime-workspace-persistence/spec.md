## MODIFIED Requirements

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
