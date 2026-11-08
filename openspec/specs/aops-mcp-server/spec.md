# aops-mcp-server

## Purpose
The aops MCP server is the component through which a coordinating agent sees and acts on agent-ops itself, with reach bounded per caller.

## Requirements

### Requirement: Read tools scoped to the caller's own subtree

The server SHALL expose read tools listing and getting Conversations and the
calling Coordinator's own `agents[]` entries, plus `get_tree` for walking a
conversation's own subtree. Pipelines, AgentCapabilities, Coordinators,
SignalSources and Channels have no reader here.

The tree read walks from the CALLING conversation downward — its own
descendants, at any depth, never its ancestors or their other branches.
Reads SHALL be filtered to what the calling Coordinator lists and to the
calling conversation's own subtree, at any depth.

#### Scenario: A coordinator sees only its members
- **WHEN** a coordinating agent lists agents
- **THEN** it receives its Coordinator's `agents[]` entries — name and description — and nothing else

#### Scenario: A nested coordinator's tree excludes its ancestors
- **WHEN** a nested Coordinator's conversation calls `get_tree`
- **THEN** it receives its own members and their descendants, and nothing from its parent or siblings

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
nothing.

The MANAGER validates the token and enforces a bound per verb:

| Verb | Bound |
|---|---|
| `invoke` | the Coordinator's `agents[]` list |
| `escalate` | the caller itself — it takes no conversation argument and acts only on the calling conversation, never a member reached through it |
| `read` | the calling conversation's own subtree, at any depth — never the tree's ultimate root when the caller is nested |
| `close` | the caller itself, or a conversation it directly caused, per `conversation-close`'s rule — never a deeper descendant reached through an intermediate member |

An allowlist inside the runtime pod SHALL NOT be relied on for any bound.

The token is per conversation because one Coordinator may hold several open
conversations at once, nested or not, and a token naming only the Coordinator
could not scope to one of them.

#### Scenario: A forged name is refused by the manager
- **WHEN** a caller holding root A's token invokes an AgentCapability listed only by another Coordinator
- **THEN** the manager refuses it, regardless of the caller's tool allowlist, and the server has made no decision

#### Scenario: One Coordinator, two roots
- **WHEN** roots A and B of one Coordinator are open and A's token asks to close a member of B
- **THEN** the manager refuses it as out of scope

#### Scenario: Close cannot reach past a direct member
- **WHEN** a caller asks to close its own member's member — a conversation it did not directly cause
- **THEN** the manager refuses it, even though the target is within the caller's own subtree

### Requirement: A channel-reader token reaches a projection and no verb

A token derived with context `channel-reader:<channel>` SHALL reach, through
`list_conversations` and `get_conversation`, only the projection `{name,
title, brief, phase, pipeline}` of conversations bound to that Channel. Every
verb SHALL be refused for it, and no run, input or tree SHALL be returned.

The MANAGER SHALL decide this from the token context. The server SHALL
forward the token and decide nothing, exactly as for a coordinator's.

#### Scenario: A reader picks a conversation without reading one
- **WHEN** a caller holding `channel-reader:voice-desk` lists conversations
- **THEN** it receives name, title, brief, phase and pipeline for each conversation with a thread on `voice-desk`, and nothing for any other

#### Scenario: A reader cannot act
- **WHEN** a caller holding a channel-reader token calls `invoke`, `close`, `escalate` or `read`
- **THEN** the manager refuses it, and the server has made no decision

### Requirement: The server sits behind the component wall

The server SHALL be reachable only from runtime pods under the
network restriction ADR 0001 established, and SHALL hold no Secret reads and no
credential stronger than the manager's adapter token. The manager never calls
it, since the server is the manager's caller.

#### Scenario: A stranger pod cannot reach it
- **WHEN** a pod outside the wired set connects to the server
- **THEN** the connection is refused at the network

### Requirement: The tool descriptions state the turn rule

The descriptions the server registers for its tools SHALL state the turn
rule. A coordinating conversation's every turn ends in one of four ways:

- `invoke`
- `escalate`
- `close`
- an answer to the person whose message the turn answered

The descriptions SHALL say which tools are reads and which are moves:

| Tool | Kind |
|---|---|
| `invoke`, `escalate`, `close` | a move |
| `list_agents`, `read`, `get_tree`, `list_conversations`, `get_conversation`, `list_open_roots` | a read, not a move |

The descriptions SHALL also say what the manager does with a turn that ends
without a move. The first such turn is handed back once as an input. A second
consecutive one is escalated.

The rule lives in the descriptions so the model reads it where it reads its
tools. The corrective input then names something the model has seen.

Reach and bounds are unchanged. A description states the rule and grants
nothing. The manager still validates the token and enforces every bound,
and the server still decides nothing.

#### Scenario: A client lists the tools

- **WHEN** a client lists the server's tools
- **THEN** the description of each of `invoke`, `escalate` and `close` states
  the turn rule and names itself a move

#### Scenario: A read describes itself as a read

- **WHEN** a client reads the description of `list_agents`, `read`,
  `get_tree`, `list_conversations`, `get_conversation` or `list_open_roots`
- **THEN** it states that the tool is a read and ends no turn

#### Scenario: The descriptions name the consequence

- **WHEN** a client reads the description of a move
- **THEN** it states that a turn ending without a move is handed back once as
  an input, and escalated on a second consecutive miss

#### Scenario: A description grants no reach

- **WHEN** a caller reads a description and then calls a verb outside its
  bound
- **THEN** the manager refuses it exactly as before this change

### Requirement: An accepted move is recorded on the caller's inflight run

When the caller is a conversation with an inflight run, an accepted `invoke`,
`escalate` or `close` SHALL record on that inflight run that the run made a
move.

The manager's own `/coordinate/*` handlers are the verbs. The evidence comes
from the manager and never from a runtime's report.

| Call | Records a move |
|---|---|
| `invoke` that creates a member | yes |
| `invoke` that attaches to a live member | yes |
| `escalate`, including on an already-escalated root | yes |
| `close` | yes |
| a refused verb | no |
| any read | no |

A refused verb records nothing, whatever refused it:

- out of scope
- unknown agent
- a cycle
- a spent budget
- a missing reason

The record SHALL live only on the inflight run. It SHALL be consumed when the
run completes and SHALL survive nowhere else.

#### Scenario: An invoke records a move

- **WHEN** a conversation with an inflight run calls `invoke` and a member is
  created
- **THEN** the inflight run records that it made a move

#### Scenario: An attached invoke records a move

- **WHEN** a conversation with an inflight run calls `invoke` and the call
  attaches to a live member
- **THEN** the inflight run records that it made a move

#### Scenario: A refused invoke records nothing

- **WHEN** a conversation with an inflight run calls `invoke` on an agent
  outside its Coordinator's `agents[]`
- **THEN** the manager refuses it, and the inflight run records no move

#### Scenario: A read records nothing

- **WHEN** a conversation with an inflight run calls `list_agents`
- **THEN** the inflight run records no move

#### Scenario: The record ends with the run

- **WHEN** a run that recorded a move completes
- **THEN** the record is consumed, and the next run starts with none
