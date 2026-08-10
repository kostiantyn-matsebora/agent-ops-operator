## MODIFIED Requirements

### Requirement: Unwired sources are visible and drop signals loudly
A SignalSource SHALL carry a `Wired` condition: True when at least one Ready `Pipeline` serves it, False otherwise. The condition SHALL name the serving Pipelines — one for a source with a single server, all of them for a chat source several Pipelines serve — so that "who answers here" is readable from the object. Signals arriving for an unwired source SHALL NOT create conversations; the ingest/inbound response SHALL state the reason explicitly.

For a chat source, the NUMBER of serving Ready Pipelines is what decides bare-message behaviour: one makes a bare message unambiguous and routable, several make it ambiguous and answerable with the choices. An operator SHALL be able to read that number from the condition rather than by listing Pipelines and matching refs by hand.

#### Scenario: Unwired source reports itself
- **WHEN** a SignalSource exists that no Ready Pipeline references
- **THEN** its status shows `Wired=False`

#### Scenario: Signals for an unwired source are dropped with a reason
- **WHEN** a signal arrives for an unwired source
- **THEN** no conversation or input is created and the response carries queued 0 with an explicit not-wired reason

#### Scenario: Claim flips the condition
- **WHEN** a Ready Pipeline adds the source to its `signalSourceRefs`
- **THEN** the source's `Wired` condition becomes True naming that pipeline and subsequent signals route

#### Scenario: A chat surface served by several pipelines names them all
- **WHEN** two Ready Pipelines list one chat source
- **THEN** the source reports `Wired=True` naming both, which is also what tells an operator that bare messages there will be refused as ambiguous
