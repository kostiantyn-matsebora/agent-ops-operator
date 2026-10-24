# conversation-opens-with-its-input — delta

## RENAMED Requirements

- FROM: `### Requirement: Inputs record where they came from; the pipeline is inferred`
- TO: `### Requirement: Inputs record where they came from and the pipeline is inferred`

## MODIFIED Requirements

### Requirement: Inputs record where they came from and the pipeline is inferred

Every input SHALL record its own origin as `{kind: signal|channel|member, name}` —
materialized state, snapshotted like `profileRef` and `channelRefs` and never set
by hand.

It SHALL replace the job-only `jobName` field, so the originating source
is recorded for every input kind rather than for jobs alone. It SHALL be optional
so inputs created before this change remain valid.

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
