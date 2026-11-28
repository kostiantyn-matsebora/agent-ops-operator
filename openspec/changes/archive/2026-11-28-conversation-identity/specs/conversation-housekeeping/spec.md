## MODIFIED Requirements

### Requirement: A closed conversation is inert but intact
A closed conversation SHALL have no runtime pod and no compiled MCP ConfigMap,
SHALL receive no dispatch, and SHALL consume neither active capacity nor a place
in the pending backlog.

It SHALL take part in no pipeline and SHALL be excluded from id-equality
routing: a signal whose `initiationId` equals a closed conversation's
`correlationId` SHALL open a new conversation rather than landing on the
closed one. A closed conversation is not somewhere work can land.

Everything that makes it a record SHALL be retained: its spec, its materialized
profile, channel, toolset and MCP references, its `correlationId`, its runtime
context handle, its recorded runs and their results, and its state on any
volume. The moment of closing SHALL be recorded on the conversation, because it
is the origin of the delete window.

#### Scenario: A closed conversation runs nothing
- **WHEN** a conversation is closed
- **THEN** its runtime pod and MCP ConfigMap are gone, no work unit is dispatched for it, and the capacity it held admits a waiting conversation

#### Scenario: A matching signal does not land on a closed conversation
- **WHEN** a signal arrives whose `initiationId` equals a closed conversation's `correlationId`
- **THEN** a new conversation is opened adopting that id, and the closed one is untouched

#### Scenario: The record survives the close
- **WHEN** an operator views a closed conversation
- **THEN** its recorded runs, their results and its materialized wiring are all still readable

#### Scenario: Volume state survives the close
- **WHEN** a conversation with a persisted workspace is closed
- **THEN** its workspace directory and its session transcripts remain on their claims

### Requirement: An orphan directory is identified by ordering, not by a timeout
Workspace directories SHALL be reclaimed only when no conversation with that `ID`
exists, determined by scanning directory entries BEFORE listing conversations and
deleting only entries absent from the later listing.

This ordering is what makes the decision sound: a directory is created by the
kubelet mounting a subPath for a runtime pod, and that pod exists only for a
conversation created earlier, so a conversation always predates its directory. A
listing taken after the scan therefore contains every conversation whose directory
was seen, and an absence is a deletion rather than a race.

A workspace directory is named by the conversation's `ID` (`runtime-workspace-persistence`), so a conversation created later under a reused name does not keep a deleted conversation's directory alive.

The reverse order SHALL NOT be used, because a conversation created between the
listing and the scan would appear to be an orphan.

#### Scenario: A conversation created mid-run is not reclaimed
- **WHEN** a conversation is created after the directory scan begins and its directory appears before the run ends
- **THEN** the conversation appears in the listing taken after the scan and its directory is left alone

#### Scenario: A deleted conversation's directory is reclaimed
- **WHEN** a directory exists for a conversation that no longer exists
- **THEN** the directory and its contents are removed

#### Scenario: A reused name does not shield an orphan
- **WHEN** a conversation was deleted, a new one was created under the same name, and the first one's directory remains
- **THEN** the directory is reclaimed, because no conversation carries its `ID`
