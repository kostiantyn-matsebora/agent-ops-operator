## MODIFIED Requirements

### Requirement: Self-exclusion has three independent mechanisms
The adapter SHALL detect its own machinery by all three of the following, independently, so that no single cold cache or configuration edit re-opens the cycle:

1. **Owner/label rule** — the involved object carries `app.kubernetes.io/name` in the agentops family (`agentops-runtime`, `agentops-manager`, and adapter workload names), OR its owner reference chain reaches a `Conversation`.
2. **Name-prefix rule** — the involved object's name begins with `agentops-conv-`, `agentops-adapter-`, `agentops-signal-`, or `agentops-housekeeping-`. This mechanism SHALL require no read of the involved object, so that it holds before any object cache is warm.
3. **Release-namespace exclusion** — events in the operator's own namespace are dropped by default.

Mechanisms 1 and 2 SHALL NOT be configurable. Mechanism 3 SHALL be overridable, for installations that co-locate their own workloads with the operator.

Every agent-ops workload SHALL carry one of the prefixes mechanism 2 lists. A maintenance workload is the case most likely to be forgotten and the worst to forget: it fails on a schedule, so an unexcluded one wakes an agent about agent-ops' own upkeep on every failure.

#### Scenario: Exclusion holds during adapter startup
- **WHEN** the adapter has just started, its object cache is not yet populated, and a Warning event arrives for `agentops-conv-abc123`
- **THEN** the name-prefix rule drops the event without reading the involved object

#### Scenario: A renamed agent-ops object is still excluded
- **WHEN** an agent-ops-owned object does not match any known name prefix but carries an agentops `app.kubernetes.io/name` label or is owned by a Conversation
- **THEN** the owner/label rule drops the event

#### Scenario: Co-located workloads can be observed
- **WHEN** an installation runs its own application in the release namespace and disables release-namespace exclusion
- **THEN** events for that application produce signals, while events for agent-ops' own objects in the same namespace are still dropped by mechanisms 1 and 2

#### Scenario: A failing housekeeping run is status, not signal
- **WHEN** a housekeeping Job pod named `agentops-housekeeping-<suffix>` fails and emits a Warning event
- **THEN** the name-prefix rule drops it, so agent-ops' own maintenance never opens a conversation about itself
