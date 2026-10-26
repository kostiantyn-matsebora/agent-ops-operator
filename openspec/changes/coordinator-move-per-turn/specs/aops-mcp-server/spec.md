## ADDED Requirements

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
