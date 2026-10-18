# conversation-housekeeping — delta

## MODIFIED Requirements

### Requirement: A closed conversation is inert but intact
A closed conversation SHALL have no runtime pod and no compiled MCP ConfigMap,
SHALL receive no dispatch, and SHALL consume neither active capacity nor a place
in the pending backlog.

It SHALL take part in no pipeline and SHALL be excluded from id-equality
routing: a signal whose `initiationId` equals a closed conversation's
`conversationId` SHALL open a new conversation rather than landing on the
closed one. A closed conversation is not somewhere work can land.

Everything that makes it a record SHALL be retained: its spec, its materialized
profile, channel, toolset and MCP references, its `conversationId`, its runtime
context handle, its recorded runs and their results, and its state on any
volume.

The moment of closing SHALL be recorded on the conversation, because it is the
origin of the delete window.

#### Scenario: A closed conversation runs nothing
- **WHEN** a conversation is closed
- **THEN** its runtime pod and MCP ConfigMap are gone, no work unit is dispatched for it, and the capacity it held admits a waiting conversation

#### Scenario: A matching signal does not land on a closed conversation
- **WHEN** a signal arrives whose `initiationId` equals a closed conversation's `conversationId`
- **THEN** a new conversation is opened adopting that id, and the closed one is untouched

#### Scenario: The record survives the close
- **WHEN** an operator views a closed conversation
- **THEN** its recorded runs, their results and its materialized wiring are all still readable

#### Scenario: Volume state survives the close
- **WHEN** a conversation with a persisted workspace is closed
- **THEN** its workspace directory and its session transcripts remain on their claims
