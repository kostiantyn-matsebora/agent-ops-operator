## ADDED Requirements

### Requirement: A Coordinator may be paused independently of Ready

A `Coordinator` SHALL carry a mutable `spec.paused` (bool, default
`false`) and a status condition `Paused`, with the shared shape
`operational-pause-controls` defines.

Pausing a Coordinator SHALL NOT change its `Ready` computation. A
Coordinator that is both `Ready=True` and `Paused=True` reports a
composition that is correctly wired and deliberately stopped — two
different facts.

#### Scenario: Pausing a Ready coordinator leaves Ready true
- **WHEN** a `Ready=True` Coordinator is paused
- **THEN** `Ready` stays `True` and `Paused` becomes `True`

#### Scenario: Resuming clears the condition
- **WHEN** a paused Coordinator's `spec.paused` is set back to `false`
- **THEN** its `Paused` condition becomes `False`

### Requirement: A paused Coordinator claims nothing and refuses direct addressing

A paused Coordinator SHALL be excluded from `PipelinesForSource`'s
fan-out, exactly as a non-Ready one already is. It opens no root
conversation when a signal is admitted on a source it would otherwise
claim.

Where a paused Coordinator was the ONLY claimant of a SignalSource, that
source's own `Wired` condition SHALL become `False`, naming that its only
claimant is paused (`signal-source-model`).

**Direct addressing is the subtle case.** `/<name> <task>` resolves a
Coordinator by the same plain lookup BY NAME a Pipeline is resolved by —
no `Ready` check is performed today, and that is unchanged.

This change adds an explicit `Paused` check to that SAME lookup. A chat
command addressing a paused Coordinator by name SHALL be refused, naming
the Coordinator and that it is paused — even though the lookup still
performs no `Ready` check at all.

Pausing a Coordinator SHALL NOT affect any conversation it has already
opened. An open root and its members keep running on their own snapshot.

This is the same shape an edit to the Coordinator's other fields already
takes — a running incident is untouched (`coordinator-model`'s "Limits and
escalation channels are snapshotted" requirement).

#### Scenario: A paused coordinator is excluded from fan-out
- **WHEN** a signal is admitted on a source listed only by a paused Coordinator
- **THEN** no conversation is created for it, and the source reports
  `Wired=False` naming the paused claimant

#### Scenario: A paused coordinator sharing a source with an active pipeline still fans out to the pipeline
- **WHEN** a source is listed by one paused Coordinator and one unpaused
  Ready Pipeline
- **THEN** a conversation opens for the Pipeline only

#### Scenario: Direct addressing refuses a paused coordinator
- **WHEN** a chat command addresses a Coordinator by name whose
  `spec.paused` is `true`
- **THEN** the command is refused, naming the Coordinator and that it is
  paused, rather than opening a root conversation

#### Scenario: Addressing still performs no Ready check
- **WHEN** a chat command addresses a Coordinator that is `Ready=False` but
  `Paused=False`
- **THEN** the command is NOT refused for that reason — only `Paused` is
  newly checked

#### Scenario: An open incident is unaffected by pausing
- **WHEN** a Coordinator with an open root conversation and live members is
  paused
- **THEN** that root and its members keep running unchanged, and only a
  NEW claim or address is refused

#### Scenario: The listing omits a paused coordinator
- **WHEN** a user sends `/pipelines` on a surface served by one paused
  Coordinator and one active Pipeline
- **THEN** only the active Pipeline is named as addressable
