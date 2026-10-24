## ADDED Requirements

### Requirement: The analyzer is inside the wall and reaches two things

The analyzer SHALL be reachable only from the adapters wired to call it, and
SHALL be allowed out only to the aops MCP server — as a holder of a
`channel-reader:<channel>` token, the projection-only reach class that
server's wall admits (`coordinated-agents`, aops-mcp-server › A channel-reader
token reaches a projection and no verb) — and to its configured
language-model endpoint. It SHALL hold no Kubernetes access and no Secret.

#### Scenario: A stranger pod cannot ask it anything
- **WHEN** a pod outside the wired set posts an utterance
- **THEN** the connection is refused at the network

#### Scenario: It cannot reach the manager
- **WHEN** the analyzer attempts a connection to the manager's adapter contracts
- **THEN** the network refuses it, so "delivers nothing" is enforced rather than trusted
