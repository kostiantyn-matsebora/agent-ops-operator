## MODIFIED Requirements

### Requirement: Unwired sources are visible and drop signals loudly
A SignalSource SHALL carry a `Wired` condition: True when at least one Ready `Pipeline` lists it, False otherwise. The condition SHALL name ALL the Pipelines serving it, not the first — a source several Pipelines watch fans its signals out to every one of them, so "who answers here" is only readable if the condition says all of them. Signals arriving for an unwired source SHALL NOT create conversations; the ingest/inbound response SHALL state the reason explicitly.

The NUMBER of serving Ready Pipelines is what an operator needs to predict behaviour, and it means two different things by lane: for any source it is the number of conversations one signal will produce, and for a CHAT source it additionally decides bare-message behaviour — one makes a bare message unambiguous and routable, several make it ambiguous and answerable with the choices. An operator SHALL be able to read that number from the condition rather than by listing Pipelines and matching refs by hand.

#### Scenario: Unwired source reports itself
- **WHEN** a SignalSource exists that no Ready Pipeline references
- **THEN** its status shows `Wired=False`

#### Scenario: Signals for an unwired source are dropped with a reason
- **WHEN** a signal arrives for an unwired source
- **THEN** no conversation or input is created and the response carries queued 0 with an explicit not-wired reason

#### Scenario: Claim flips the condition
- **WHEN** a Ready Pipeline adds the source to its `signalSourceRefs`
- **THEN** the source's `Wired` condition becomes True naming that pipeline and subsequent signals route

#### Scenario: A source served by several pipelines names them all
- **WHEN** two Ready Pipelines list one source
- **THEN** the source reports `Wired=True` naming both, which is also what tells an operator that one signal there will open two conversations

#### Scenario: A chat surface served by several pipelines names them all
- **WHEN** two Ready Pipelines list one chat source
- **THEN** the source reports `Wired=True` naming both, which is also what tells an operator that bare messages there will be refused as ambiguous
