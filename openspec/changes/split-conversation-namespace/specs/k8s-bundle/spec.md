## ADDED Requirements

### Requirement: Route identities follow the split, everything else stays in the release namespace
The bundle SHALL render each shipped route's ServiceAccount in the
conversations namespace, and every binding the route declares SHALL name it
there.

Every other object the bundle renders SHALL stay in the release namespace:
profiles, Pipelines, capabilities, toolsets, MCP configs, the SignalSource, the
SignalAdapter, the events adapter account `agentops-signal-<name>` and the MCP server.

The MCP server's NetworkPolicy SHALL admit runtime pods from the conversations
namespace.

#### Scenario: Route accounts render beside the pods
- **WHEN** the bundle renders its observe and admin routes in a split release
- **THEN** both route ServiceAccounts are in the conversations namespace and every ClusterRoleBinding subject names them there

#### Scenario: Wiring renders in the release namespace
- **WHEN** the bundle renders in a split release
- **THEN** its profiles, Pipelines, capabilities, toolsets, MCP config, SignalSource, SignalAdapter, events adapter account and MCP server are in the release namespace

#### Scenario: The MCP server stays reachable under policy
- **WHEN** the bundle renders split with NetworkPolicies enabled
- **THEN** the MCP server's policy admits runtime pods from the conversations namespace, and an agent using the k8s toolset reaches it
