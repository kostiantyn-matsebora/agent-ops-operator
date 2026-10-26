## MODIFIED Requirements

### Requirement: Inputs record where they came from and the pipeline is inferred

Every input SHALL record its own origin as
`{kind: signal|channel|member|manager, name}`.

- Materialized state, snapshotted like `profileRef` and `channelRefs`, never
  set by hand.
- It replaces the job-only `jobName` field, so the originating source is
  recorded for every input kind rather than for jobs alone.
- It is optional, so inputs created before this change remain valid.

The fourth kind, `manager`, is an input the manager appended under a rule of
its own. Its `name` is the rule that wrote it, `move-per-turn` today (`conversation-provenance`).

It entered on no surface. The per-destination delivery rule below therefore
delivers it to every bound channel, attributed to the manager, with no
clause of its own.

**The originating Pipeline IS recorded**, on `Conversation.spec.pipelineRef` —
this requirement once said no such field should exist, and `pipeline-model` now
owns the rule that replaced it: the ref is PROVENANCE, written once at creation
and never read to resolve wiring.

It exists because sources are SHAREABLE: two Pipelines fanning out from one
source open conversations adopting the same `initiationId`, and without the ref
the second Pipeline's next signal would land on the first's conversation under
the wrong profile.

A message SHALL name the pipeline by READING that ref. For a conversation
predating it, the pipeline SHALL be inferred from the materialized bindings and
OMITTED when the inference is ambiguous. A message SHALL NOT name a pipeline that
is a guess.

#### Scenario: An alert input names its source

- **WHEN** an alert from source `cluster-alerts` is routed by pipeline `prod-oncall`
- **THEN** the input records `{kind: signal, name: cluster-alerts}` and the
  rendered message names `prod-oncall`, read from the conversation's recorded
  origin

#### Scenario: An ambiguous route is left blank, never guessed

- **WHEN** a conversation predating `pipelineRef` has bindings matching more than
  one Ready pipeline
- **THEN** the message renders without a pipeline name and is otherwise complete

#### Scenario: A manager-origin input is delivered everywhere

- **WHEN** the manager appends an input `{kind: manager, name: move-per-turn}`
  to a conversation bound to two channels
- **THEN** both channels receive it, attributed to the manager, because it
  entered on no surface
