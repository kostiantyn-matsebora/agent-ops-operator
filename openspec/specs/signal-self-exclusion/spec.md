# signal-self-exclusion Specification

## Purpose

The loop breaker: an observing signal adapter never emits a signal about
agent-ops' own machinery. It is the signal-lane twin of the no-relay-loops rule
for channels, and it is an INVARIANT rather than a filter — a failing runtime pod
that became a signal would open a conversation that creates another failing
runtime pod, forever, and nothing downstream catches that.

It also fixes what "own machinery" means: three independent mechanisms, only the
coarsest of which an installation may turn off.

## Requirements

### Requirement: agent-ops never ingests its own machinery as a signal
A signal adapter that observes cluster state SHALL NOT emit a signal for an object that agent-ops itself created. This is an invariant, not a configurable filter: it is the signal-lane twin of the no-relay-loops rule for channels — the system must never re-ingest its own output as input.

The rationale is structural, not aesthetic. A Conversation whose runtime pod cannot start emits a Warning event on that pod; without this rule the event becomes a signal, the signal opens a Conversation, that Conversation creates another runtime pod under a NEW name, and the cycle repeats without bound. Fingerprint cooldown does not break it (the pod name is fresh each turn), workload grouping does not break it (the owner is the Conversation CR, fresh each turn), and the dwell liveness re-check does not break it (the pod genuinely is still broken). The conversation cap bounds concurrent pods and `MAX_QUEUED_CONVERSATIONS` throttles the backlog, but neither BREAKS the cycle — the runaway still fills etcd, only more slowly.

agent-ops' own health SHALL be reported as status — conditions on the owning CR — never as a signal.

#### Scenario: A failing runtime pod produces no signal
- **WHEN** a runtime pod `agentops-conv-<name>` cannot start (unschedulable, quota exceeded, image pull failure, or OOM) and Kubernetes emits Warning events against it
- **THEN** the adapter emits no signal for those events, and no Conversation is created as a result

#### Scenario: The loop cannot be re-opened by configuration
- **WHEN** a source's configuration contains no exclusion for agent-ops objects, or explicitly attempts to include them
- **THEN** events for agent-ops-owned objects are still dropped, because the rule is not expressed as a deny-list entry and no configuration path can disable it

### Requirement: Self-exclusion has four independent mechanisms
The adapter SHALL detect its own machinery by all four of the following, independently, so that no single cold cache, configuration edit or namespace layout re-opens the cycle:

1. **Owner/label rule** — the involved object carries `app.kubernetes.io/name` in the agentops family (`agentops-runtime`, `agentops-manager`, and adapter workload names), OR its owner reference chain reaches a `Conversation`.
2. **Name-prefix rule** — the involved object's name begins with `agentops-conv-`, `agentops-mcp-conv-`, `agentops-adapter-`, `agentops-signal-`, `agentops-housekeeping-`, or `agentops-context-probe-`. This mechanism SHALL require no read of the involved object, so that it holds before any object cache is warm.
3. **Release-namespace exclusion** — events in the operator's own namespace, read from `POD_NAMESPACE`, are dropped by default.
4. **Conversations-namespace exclusion** — events in the namespace `CONVERSATION_NAMESPACE` names are dropped whenever it is set and differs from `POD_NAMESPACE`. Like mechanism 2 it SHALL require no read of the involved object. Where the two are equal or it is unset, mechanisms 1 to 3 govern alone.

Mechanisms 1, 2 and 4 SHALL NOT be configurable. Mechanism 3 SHALL be overridable, for installations that co-locate their own workloads with the operator.

Mechanism 4 is not overridable because the conversations namespace holds only agent-ops machinery by construction. Many objects there, such as the claim `agentops-context`, carry none of mechanism 2's prefixes.

Every agent-ops workload SHALL be caught by mechanism 1 or 2, whatever namespace it runs in. Mechanisms 3 and 4 are a second line, never the only one.

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
