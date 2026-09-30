## Purpose
The aops MCP server is the component through which a coordinating agent sees and acts on agent-ops itself, with reach bounded per caller.

This change depends on `coordinated-agents` landing first. The
`aops-mcp-server` spec modified here is defined there and is not yet
archived.

## MODIFIED Requirements

### Requirement: Four verbs, all asynchronous

The server SHALL expose `invoke(agent, task)`, `close(conversation, reason)`,
`escalate(message)`, `read(conversation)` and `list_open_roots()`.
`list_conversations` and `get_conversation` are the channel-reader
projection defined by `coordinated-agents`, not coordination verbs. Each SHALL
return without waiting on any agent's work. `invoke` SHALL report created or
attached.

The heading keeps its name because a MODIFIED requirement matches the
pending base spec (`coordinated-agents`) by name. The verbs are now FIVE.

`list_open_roots` is the Coordinator-owner reach class's own verb, defined
in full by `coordinator-owner-reach`. It is listed here only so this file's
verb count and table stay accurate.

#### Scenario: Invoke returns at once
- **WHEN** a coordinating agent calls `invoke`
- **THEN** it receives the member's name within the request, and the result arrives later as an input

### Requirement: Reach is bounded by the manager, per conversation, not by an allowlist

Every verb SHALL carry the calling conversation's token, derived by the
manager with context `coordinator:<name>:<conversation>` and injected into
that conversation's runtime pod. The server SHALL forward it and decide
nothing.

The MANAGER validates the token and enforces a bound per verb:

| Verb | Bound |
|---|---|
| `invoke` | the Coordinator's `agents[]` list |
| `escalate` | the caller itself — it takes no conversation argument and acts only on the calling conversation, never a member reached through it |
| `read` | the calling conversation's own subtree, at any depth — never the tree's ultimate root when the caller is nested |
| `close` | the caller itself, a conversation it directly caused, per `conversation-close`'s rule, OR — when the caller RESOLVES to a Coordinator (`coordinator-owner-reach`'s walk) — any open UNCAUSED root of that SAME Coordinator other than the caller's own ancestor, never a deeper descendant reached through an intermediate member and never a member of any kind |
| `list_conversations`, `get_conversation` | for a `channel-reader:<channel>` token, the projection of that Channel's conversations and no verb, per `coordinated-agents` |
| `list_open_roots` | the calling conversation's own Coordinator's open UNCAUSED roots only — see `coordinator-owner-reach` |

An allowlist inside the runtime pod SHALL NOT be relied on for any bound.

The token is per conversation because one Coordinator may hold several open
conversations at once, nested or not, and a token naming only the Coordinator
could not scope to one of them.

The `close` widening applies ONLY to a caller that RESOLVES to a Coordinator
by `coordinator-owner-reach`'s walk — its own `coordinatorRef`, or failing
that, the uncaused root its `causedBy` chain leads to.

An ordinary Pipeline-addressed caller, or a member with no Coordinator
anywhere in its chain, keeps the unwidened bound: itself, or a conversation
it directly caused.

#### Scenario: A forged name is refused by the manager
- **WHEN** a caller holding root A's token invokes an AgentCapability listed only by another Coordinator
- **THEN** the manager refuses it, regardless of the caller's tool allowlist, and the server has made no decision

#### Scenario: One Coordinator, two roots
- **WHEN** roots A and B of one Coordinator are open and A's token asks to close a member of B
- **THEN** the manager refuses it as out of scope

#### Scenario: Close cannot reach past a direct member
- **WHEN** a caller asks to close its own member's member — a conversation it did not directly cause
- **THEN** the manager refuses it, even though the target is within the caller's own subtree

#### Scenario: A Coordinator-owner may close a sibling root

- **WHEN** a member caller that resolves to Coordinator `incident-coord`
  (via its own `coordinatorRef`, or its uncaused root's) asks to close a
  DIFFERENT open root that also carries `coordinatorRef: incident-coord`,
  one it did not directly cause
- **THEN** the manager permits it, because the widened bound covers any open
  uncaused root of the caller's own Coordinator, other than its own ancestor

#### Scenario: A Pipeline-addressed caller keeps the narrow bound

- **WHEN** a caller that resolves to no Coordinator asks to close a
  conversation it did not itself cause
- **THEN** the manager refuses it — the widened bound never applies to a
  caller that is not itself acting for a Coordinator

### Requirement: The server sits behind the component wall

The server SHALL be reachable only from runtime pods under the
network restriction ADR 0001 established, and SHALL hold no Secret reads and no
credential stronger than the manager's adapter token. The manager never calls
it, since the server is the manager's caller.

The reach classes it forwards are THREE: the coordinator class (`invoke`,
`escalate`, `read`, the narrow `close`), the Coordinator-owner class
(`list_open_roots`, the widened `close`), and the channel-reader class —
each decided by the manager from the token's context, never by the server.

#### Scenario: A stranger pod cannot reach it
- **WHEN** a pod outside the wired set connects to the server
- **THEN** the connection is refused at the network
