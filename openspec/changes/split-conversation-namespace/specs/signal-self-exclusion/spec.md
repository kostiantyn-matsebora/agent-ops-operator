## RENAMED Requirements

- FROM: `### Requirement: Self-exclusion has three independent mechanisms`
- TO: `### Requirement: Self-exclusion has four independent mechanisms`

## MODIFIED Requirements

### Requirement: Self-exclusion has four independent mechanisms
The adapter SHALL detect its own machinery by all four of the following, independently, so that no single cold cache, configuration edit or namespace layout re-opens the cycle:

1. **Owner/label rule** — the involved object carries `app.kubernetes.io/name` in the agentops family (`agentops-runtime`, `agentops-manager`, and adapter workload names), OR its owner reference chain reaches a `Conversation`.
2. **Name-prefix rule** — the involved object's name begins with `agentops-conv-`, `agentops-adapter-`, `agentops-signal-`, or `agentops-housekeeping-`. This mechanism SHALL require no read of the involved object, so that it holds before any object cache is warm.
3. **Release-namespace exclusion** — events in the operator's own namespace, read from `POD_NAMESPACE`, are dropped by default.
4. **Conversations-namespace exclusion** — events in the namespace `CONVERSATION_NAMESPACE` names are dropped whenever it is set and differs from `POD_NAMESPACE`. Like mechanism 2 it SHALL require no read of the involved object. Where the two are equal or it is unset, mechanisms 1 to 3 govern alone.

Mechanisms 1, 2 and 4 SHALL NOT be configurable. Mechanism 3 SHALL be overridable, for installations that co-locate their own workloads with the operator.

Mechanism 4 is not overridable because the conversations namespace holds only agent-ops machinery by construction. Many objects there, such as the claim `agentops-context`, carry none of mechanism 2's prefixes.

Every agent-ops workload SHALL carry one of the prefixes mechanism 2 lists.

A maintenance workload is the case most likely to be forgotten and the worst to forget: it fails on a schedule, so an unexcluded one starts a conversation about agent-ops' own upkeep on every failure.

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

#### Scenario: An unprefixed object in the conversations namespace is excluded
- **WHEN** the release runs split and a Warning event arrives for the claim `agentops-context` in the conversations namespace before any object cache is warm
- **THEN** the conversations-namespace rule drops the event without reading the claim

#### Scenario: Disabling release-namespace exclusion does not reach the conversations namespace
- **WHEN** the release runs split and a source turns release-namespace exclusion off
- **THEN** every event in the conversations namespace is still dropped by mechanism 4

#### Scenario: The single-namespace layout keeps the override
- **WHEN** `CONVERSATION_NAMESPACE` equals `POD_NAMESPACE` and a source turns release-namespace exclusion off
- **THEN** mechanism 4 does not apply, and a co-located application's events produce signals
