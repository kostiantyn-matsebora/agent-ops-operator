## MODIFIED Requirements

### Requirement: Configuration state from read-only Kubernetes watches
The console SHALL build its configuration state exclusively from list/watch of `agentops.dev/v1alpha1` resources (AgentProfile, AgentRuntime, Channel, ChannelAdapter, MCPConfig, MCPToolset, Pipeline, SignalAdapter, SignalSource) in its own namespace using its own ServiceAccount, with no writes to any of them and no reads of Secrets or any non-agentops resource. It SHALL hold no grant on Conversations, ConversationInputs or Signals: it builds their state from the manager's state read surface, and its Kubernetes ServiceAccount SHALL NOT be able to read them. Watches SHALL resume by resourceVersion and relist on 410 Gone so the cache converges after disconnects.

#### Scenario: State reflects a CR change without polling
- **WHEN** a Pipeline's `channels[]` is edited with kubectl
- **THEN** the console's topology updates from the watch event without any console restart or manual refresh

#### Scenario: The console's grant excludes state
- **WHEN** the console's ServiceAccount is checked for `get`, `list` or `watch` on conversations, conversationinputs and signals
- **THEN** each is denied, and the console still shows every conversation and signal

#### Scenario: Watch expiry recovers
- **WHEN** the API server returns 410 Gone for a stale watch
- **THEN** the console relists that kind, replaces its cache, and resumes watching without serving an error to browsers
