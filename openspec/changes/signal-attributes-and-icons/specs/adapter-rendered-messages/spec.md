## ADDED Requirements

### Requirement: A signal message carries its rating, its type and its icons
A `signal` message SHALL carry, beside `labels`, the structured fields `severity`, `type` and `icons`.

- `severity` is the vocabulary value, or absent for an unrated signal.
- `type` is the value of the reserved `type` label, or absent.
- `icons` is a map of resolved icon references, keyed `pipeline`, `source`, `severity` and `type`, each present only where the ladder resolved one.
- Every field is structured. The manager states what was rated and how it is marked, and SHALL NOT state how a surface draws it or whether it can.
- A surface that cannot draw a reference SHALL omit it. An unrated or untyped signal SHALL render exactly as today.

#### Scenario: A rated, typed card
- **WHEN** a `critical` alert labelled `type: security` opens a conversation on a Pipeline with an icon, from a source with none
- **THEN** the `signal` message carries `severity: critical`, `type: security`, and icons for the pipeline, the source's kind fallback, the severity, and the type if declared

#### Scenario: An unrated card is unchanged
- **WHEN** a `kind: task` signal with no severity and no type opens a conversation
- **THEN** the `signal` message carries no `severity`, no `type`, and only the pipeline and source icons
