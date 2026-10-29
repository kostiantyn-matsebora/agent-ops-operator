## ADDED Requirements

### Requirement: A manager-origin input records the manager's own hand

An input's origin kind SHALL admit a fourth value, `manager`, beside `signal`,
`channel` and `member`. It marks an input the manager itself appended to a
conversation.

| Origin | Is |
|---|---|
| `signal` | arrived through `/signal/inbound` on a SignalSource |
| `channel` | typed into a channel by a person |
| `member` | crossed a `causedBy` edge between a coordinated conversation and its parent |
| `manager` | appended by the manager under a rule of its own |

Today the only manager-origin input is the corrective input a Coordinator
root receives after a successful turn that made no move. `Name` on such an
origin SHALL name the rule that produced it, `move-per-turn`.

```yaml
status:
  runs:
    - inputs:
        - origin:
            kind: manager
            name: move-per-turn
```

A manager-origin input SHALL be recorded on the durable `status.runs[].inputs[]`
like every other origin, so the record shows who wrote it and a later rule
can see the earlier one.

It SHALL be delivered to the conversation's bound threads by the existing
per-destination rule, attributed to the manager. It entered on no surface,
so every bound channel receives it, and a person watching sees the
hand-back.

The corrective input SHALL be of type `reply`, so the run it dispatches
resumes the conversation's stored context. A `task` input never carries the
context handle, and the turn must see what the agent concluded.

It SHALL never be treated as a person's message. A turn that answers only a
manager-origin input with prose is still a turn without a move.

#### Scenario: The corrective input is recorded

- **WHEN** a Coordinator root's successful run made no move and no input of
  the run came from a person
- **THEN** the manager appends one input with origin `{kind: manager, name:
  move-per-turn}`, and the next run's `status.runs[].inputs[]` carries it

#### Scenario: The corrective input continues the context
- **WHEN** the manager appends the corrective input to a root that has a stored runtime context
- **THEN** its type is `reply` and the next work unit carries that context handle

#### Scenario: A root with no stored context
- **WHEN** the manager appends the corrective input to a root that has no stored runtime context
- **THEN** the input is still of type `reply`, and the next work unit has no context handle to resume, so it is framed as a fresh turn around the input's text

#### Scenario: A manager-origin input reaches every bound channel

- **WHEN** a manager-origin input is appended to a conversation bound to
  channels
- **THEN** every bound channel receives it, attributed to the manager, by
  the same rule a member's result is delivered by

#### Scenario: A manager-origin input is not a person

- **WHEN** a Coordinator root's next run answers the corrective input with
  prose and calls no `invoke`, `escalate` or `close`
- **THEN** that turn counts as a second consecutive turn without a move

#### Scenario: Older origins are unchanged

- **WHEN** an input arrives through a signal, a channel or a `causedBy` edge
- **THEN** its origin kind is `signal`, `channel` or `member`, exactly as
  before
