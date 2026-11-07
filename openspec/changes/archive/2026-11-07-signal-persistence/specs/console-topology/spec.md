# console-topology (delta)

## MODIFIED Requirements

### Requirement: Configuration state from read-only Kubernetes watches
The console SHALL build its configuration state exclusively from list/watch of `agentops.dev/v1alpha1` resources, in its own namespace, using its own ServiceAccount:

- The watched kinds are AgentProfile, AgentRuntime, Channel, ChannelAdapter, Conversation, MCPConfig, MCPToolset, Pipeline, Signal, SignalAdapter and SignalSource.
- The console SHALL write none of them, and SHALL read no Secret and no non-agentops resource.
- Watches SHALL resume by resourceVersion and relist on 410 Gone, so the cache converges after disconnects.

#### Scenario: State reflects a CR change without polling
- **WHEN** a Pipeline's `channels[]` is edited with kubectl
- **THEN** the console's topology updates from the watch event without any console restart or manual refresh

#### Scenario: Watch expiry recovers
- **WHEN** the API server returns 410 Gone for a stale watch
- **THEN** the console relists that kind, replaces its cache, and resumes watching without serving an error to browsers

#### Scenario: Signals arrive by watch like every other kind
- **WHEN** a Signal's phase moves from Pending to Routed
- **THEN** the console reflects it from the watch event, with no polling and no write path to the kind

### Requirement: CR inventory views
The console SHALL provide per-kind inventory views and a per-object detail view:

- The inventory lists each agentops CR with its key spec fields, conditions, and age.
- The detail view shows the full object, spec and status.
- Opaque `config` blocks SHALL be displayed verbatim, without interpretation.
- The Signal inventory SHALL list phase beside the key fields.
- A Signal's detail view SHALL show its per-claimant routing state.

#### Scenario: Condition drill-down
- **WHEN** a user opens a Channel showing `Ready=False`
- **THEN** the detail view shows the condition's reason and message as reported by the serving adapter

#### Scenario: Signal inventory shows lifecycle at a glance
- **WHEN** a user opens the Signal inventory while one signal is PartiallyRouted
- **THEN** the list shows that signal's phase beside its source and age, without opening the detail view
