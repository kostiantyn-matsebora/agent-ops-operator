## ADDED Requirements

### Requirement: A Pipeline may be paused independently of Ready

A `Pipeline` SHALL carry a mutable `spec.paused` (bool, default `false`)
and a status condition `Paused`, with the shared shape
`operational-pause-controls` defines.

Pausing a Pipeline SHALL NOT change its `Ready` computation. A Pipeline
that is both `Ready=True` and `Paused=True` reports a route that is
correctly wired and deliberately stopped — two different facts.

#### Scenario: Pausing a Ready pipeline leaves Ready true
- **WHEN** a `Ready=True` Pipeline is paused
- **THEN** `Ready` stays `True` and `Paused` becomes `True`

#### Scenario: Resuming clears the condition
- **WHEN** a paused Pipeline's `spec.paused` is set back to `false`
- **THEN** its `Paused` condition becomes `False`

### Requirement: A paused Pipeline claims nothing and refuses direct addressing

A paused Pipeline SHALL be excluded from `PipelinesForSource`'s fan-out,
exactly as a non-Ready one already is. It contributes no conversation when
a signal is admitted on a source it would otherwise claim.

Where a paused Pipeline was the ONLY claimant of a SignalSource, that
source's own `Wired` condition SHALL become `False`, naming that its only
claimant is paused (`signal-source-model`).

**Direct addressing is the subtle case.** `/<pipeline> <task>` resolves a
Pipeline by a plain lookup BY NAME. `pipeline-model`'s own "Pipeline-only
resolution" requirement already states this performs no `Ready` check.

This change adds an explicit `Paused` check to that SAME lookup. A chat
command addressing a paused Pipeline by name SHALL be refused, naming the
Pipeline and that it is paused — even though the lookup still performs no
`Ready` check at all.

#### Scenario: A paused pipeline is excluded from fan-out
- **WHEN** a signal is admitted on a source listed only by a paused Pipeline
- **THEN** no conversation is created for that Pipeline, and the source
  reports `Wired=False` naming the paused claimant

#### Scenario: A paused pipeline sharing a source with an active one still fans out to the active one
- **WHEN** a source is listed by one paused and one unpaused Ready Pipeline
- **THEN** a conversation opens for the unpaused Pipeline only

#### Scenario: Direct addressing refuses a paused pipeline
- **WHEN** a chat command addresses a Pipeline by name whose `spec.paused`
  is `true`
- **THEN** the command is refused, naming the Pipeline and that it is
  paused, rather than opening a conversation

#### Scenario: Addressing still performs no Ready check
- **WHEN** a chat command addresses a Pipeline that is `Ready=False` but
  `Paused=False`
- **THEN** the command is NOT refused for that reason — addressing bypasses
  `Ready` exactly as it did before this change, and only `Paused` is newly
  checked

#### Scenario: The listing omits a paused pipeline
- **WHEN** a user sends `/pipelines` on a surface served by one paused and
  one active Pipeline
- **THEN** only the active Pipeline is named as addressable
