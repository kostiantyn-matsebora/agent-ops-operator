## MODIFIED Requirements

### Requirement: One pipeline per source
Exclusivity SHALL apply to sources that cannot be addressed. A NON-CHAT
`SignalSource` SHALL be claimed by at most one Pipeline: when a second Pipeline
references an already-claimed source, the newer Pipeline SHALL report
`SourceConflict=True` naming the contested source and the older claim SHALL keep
routing. An alert or job has no prefix to disambiguate it, so "who investigates
this" must have exactly one answer.

A CHAT `SignalSource` SHALL NOT be exclusive. Any number of Ready Pipelines MAY
list one, and doing so SHALL NOT produce a conflict and SHALL NOT affect
`Ready`. Listing a chat source means "I serve this surface" — it makes the
surface wired and this Pipeline addressable from it — not "I own this inbox".
Which Pipeline answers a message on such a surface is decided by the message:
addressed messages route by name, and bare messages route only when exactly one
Pipeline serves the source.

Channels MAY be referenced by multiple Pipelines; a conversation's binding set
comes from the pipeline that originates it.

#### Scenario: Second claim on a non-chat source is refused
- **WHEN** two Pipelines reference the same alert SignalSource
- **THEN** the newer reports `SourceConflict=True` and the source's signals keep
  routing per the older Pipeline

#### Scenario: Several pipelines serve one chat surface
- **WHEN** two Ready Pipelines both list the same chat SignalSource
- **THEN** neither reports a conflict, both stay `Ready=True`, and both appear in
  the surface's listing of available agents

#### Scenario: Channel shared by two pipelines stays valid
- **WHEN** two Ready Pipelines both reference channel `web`
- **THEN** neither reports a conflict, and each pipeline's sources produce
  conversations bound per their own pipeline
