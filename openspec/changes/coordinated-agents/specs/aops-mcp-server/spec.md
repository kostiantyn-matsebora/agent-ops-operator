## Purpose
The aops MCP server is the component through which a coordinating agent sees and acts on agent-ops itself, with reach bounded per caller.

## ADDED Requirements

### Requirement: Read tools over the agentops kinds

The server SHALL expose read tools listing and getting Conversations,
Pipelines, Agents, Coordinators, SignalSources and Channels, including a
conversation's tree by root. Reads SHALL be filtered to what the calling
Coordinator lists and what it caused.

#### Scenario: A coordinator sees only its members
- **WHEN** a coordinating agent lists agents
- **THEN** it receives its Coordinator's `agents[]` entries — name and description — and nothing else

### Requirement: Four verbs, all asynchronous

The server SHALL expose `invoke(agent, task)`, `close(conversation, reason)`,
`escalate(message)` and `read(conversation)`. Each SHALL return without
waiting on any agent's work. `invoke` SHALL report created or attached.

#### Scenario: Invoke returns at once
- **WHEN** a coordinating agent calls `invoke`
- **THEN** it receives the member's name within the request, and the result arrives later as an input

### Requirement: Reach is bounded by the manager, per conversation, not by an allowlist

Every verb SHALL carry the calling conversation's token, derived by the
manager with context `coordinator:<name>:<conversation>` and injected into
that conversation's runtime pod. The server SHALL forward it and decide
nothing: the MANAGER validates the token and enforces the Coordinator's
`agents[]` list and the root scope on every verb. An allowlist inside the
runtime pod SHALL NOT be relied on for any bound. The token is per
conversation because one Coordinator may hold several roots at once, and a
token naming only the Coordinator could not scope to one of them.

#### Scenario: A forged name is refused by the manager
- **WHEN** a caller holding root A's token invokes an Agent listed only by another Coordinator
- **THEN** the manager refuses it, regardless of the caller's tool allowlist, and the server has made no decision

#### Scenario: One Coordinator, two roots
- **WHEN** roots A and B of one Coordinator are open and A's token asks to close a member of B
- **THEN** the manager refuses it as out of scope

### Requirement: The server sits behind the component wall

The server SHALL be reachable only from runtime pods and the manager under the
network restriction ADR 0001 established, and SHALL hold no Secret reads and no
credential stronger than the manager's adapter token.

#### Scenario: A stranger pod cannot reach it
- **WHEN** a pod outside the wired set connects to the server
- **THEN** the connection is refused at the network
