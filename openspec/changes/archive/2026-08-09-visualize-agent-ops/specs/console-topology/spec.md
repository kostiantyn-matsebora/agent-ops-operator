# console-topology

## ADDED Requirements

### Requirement: Configuration state from read-only Kubernetes watches
The console SHALL build its configuration state exclusively from list/watch of `agentops.dev/v1alpha1` resources (AgentProfile, AgentRuntime, Channel, ChannelAdapter, Conversation, Pipeline, SignalAdapter, SignalSource) in its own namespace using its own ServiceAccount, with no writes to any of them and no reads of Secrets or any non-agentops resource. Watches SHALL resume by resourceVersion and relist on 410 Gone so the cache converges after disconnects.

#### Scenario: State reflects a CR change without polling
- **WHEN** a Pipeline's `channels[]` is edited with kubectl
- **THEN** the console's topology updates from the watch event without any console restart or manual refresh

#### Scenario: Watch expiry recovers
- **WHEN** the API server returns 410 Gone for a stale watch
- **THEN** the console relists that kind, replaces its cache, and resumes watching without serving an error to browsers

### Requirement: Pipeline topology graph
The console SHALL render the wiring as a graph: SignalSource, Pipeline, AgentProfile, Channel, and adapter nodes, with edges derived solely from CR spec (`Pipeline.spec.sources[]`, `Pipeline.spec.channels[]`, `Pipeline.spec.profileRef`, and `spec.adapter` references to serving adapter CRs). Node health SHALL be derived solely from the conditions the reconcilers maintain (Ready, Served, Wired) — the console SHALL NOT compute health the cluster does not assert.

#### Scenario: Healthy pipeline renders connected
- **WHEN** a Ready Pipeline wires a Served SignalSource to a profile and two Served channels
- **THEN** the graph shows the source, pipeline, profile, and both channels connected, with healthy status coloring

#### Scenario: Unclaimed source is visibly dropped
- **WHEN** a SignalSource is claimed by no Pipeline (`Wired=False`)
- **THEN** it renders as a disconnected node carrying the Wired condition's reason, making the signal-dropping state visible

#### Scenario: Unserved adapter reference is diagnosable
- **WHEN** a Channel names `spec.adapter: slak` and no such ChannelAdapter exists
- **THEN** the channel node shows `Served=False` with the condition reason, and no edge to an adapter node is drawn

### Requirement: CR inventory views
The console SHALL provide per-kind inventory views listing each agentops CR with its key spec fields, conditions, and age, and a detail view showing the full object (spec and status). Opaque `config` blocks SHALL be displayed verbatim without interpretation.

#### Scenario: Condition drill-down
- **WHEN** a user opens a Channel showing `Ready=False`
- **THEN** the detail view shows the condition's reason and message as reported by the serving adapter
