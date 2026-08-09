# signal-self-exclusion Specification

## Purpose
TBD - created by archiving change smart-k8s-events. Update Purpose after archive.
## Requirements
### Requirement: agent-ops never ingests its own machinery as a signal
A signal adapter that observes cluster state SHALL NOT emit a signal for an object that agent-ops itself created. This is an invariant, not a configurable filter: it is the signal-lane twin of the no-relay-loops rule for channels — the system must never re-ingest its own output as input.

The rationale is structural, not aesthetic. A Conversation whose runtime pod cannot start emits a Warning event on that pod; without this rule the event becomes a signal, the signal opens a Conversation, that Conversation creates another runtime pod under a NEW name, and the cycle repeats without bound. Fingerprint cooldown does not break it (the pod name is fresh each turn), workload grouping does not break it (the owner is the Conversation CR, fresh each turn), and the dwell liveness re-check does not break it (the pod genuinely is still broken). `MAX_RUNTIMES` caps concurrent pods but not Conversation creation, so the runaway fills etcd while the pod pool thrashes.

agent-ops' own health SHALL be reported as status — conditions on the owning CR — never as a signal.

#### Scenario: A failing runtime pod produces no signal
- **WHEN** a runtime pod `agentops-conv-<name>` cannot start (unschedulable, quota exceeded, image pull failure, or OOM) and Kubernetes emits Warning events against it
- **THEN** the adapter emits no signal for those events, and no Conversation is created as a result

#### Scenario: The loop cannot be re-opened by configuration
- **WHEN** a source's configuration contains no exclusion for agent-ops objects, or explicitly attempts to include them
- **THEN** events for agent-ops-owned objects are still dropped, because the rule is not expressed as a deny-list entry and no configuration path can disable it

### Requirement: Self-exclusion has three independent mechanisms
The adapter SHALL detect its own machinery by all three of the following, independently, so that no single cold cache or configuration edit re-opens the cycle:

1. **Owner/label rule** — the involved object carries `app.kubernetes.io/name` in the agentops family (`agentops-runtime`, `agentops-manager`, and adapter workload names), OR its owner reference chain reaches a `Conversation`.
2. **Name-prefix rule** — the involved object's name begins with `agentops-conv-`, `agentops-adapter-`, or `agentops-signal-`. This mechanism SHALL require no read of the involved object, so that it holds before any object cache is warm.
3. **Release-namespace exclusion** — events in the operator's own namespace are dropped by default.

Mechanisms 1 and 2 SHALL NOT be configurable. Mechanism 3 SHALL be overridable, for installations that co-locate their own workloads with the operator.

#### Scenario: Exclusion holds during adapter startup
- **WHEN** the adapter has just started, its object cache is not yet populated, and a Warning event arrives for `agentops-conv-abc123`
- **THEN** the name-prefix rule drops the event without reading the involved object

#### Scenario: A renamed agent-ops object is still excluded
- **WHEN** an agent-ops-owned object does not match any known name prefix but carries an agentops `app.kubernetes.io/name` label or is owned by a Conversation
- **THEN** the owner/label rule drops the event

#### Scenario: Co-located workloads can be observed
- **WHEN** an installation runs its own application in the release namespace and disables release-namespace exclusion
- **THEN** events for that application produce signals, while events for agent-ops' own objects in the same namespace are still dropped by mechanisms 1 and 2

