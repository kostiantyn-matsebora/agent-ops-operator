## MODIFIED Requirements

### Requirement: An orphan directory is identified by ordering, not by a timeout
Workspace directories SHALL be reclaimed only when no conversation with that `ID`
exists AND no conversation's record names it as the directory it mounts,
determined by scanning directory entries BEFORE listing conversations and
deleting only entries absent from the later listing.

This ordering is what makes the decision sound: a directory is created by the
kubelet mounting a subPath for a runtime pod, and that pod exists only for a
conversation created earlier, so a conversation always predates its directory. A
listing taken after the scan therefore contains every conversation whose directory
was seen, and an absence is a deletion rather than a race.

A workspace directory is named by the conversation's `ID` (`runtime-workspace-persistence`), so a conversation created later under a reused name does not keep a deleted conversation's directory alive.

A directory a member shares (`member-workspace-sharing`) is named by its owner's `ID` and mounted by every member that shares it. It SHALL be live while any conversation, in any phase, either carries that `ID` or records it as its workspace owner. The owner's object being deleted SHALL NOT make it an orphan while such a member exists. The listing SHALL remain phase-blind, so a closed member keeps the directory live, and the directory is reclaimed in the first run after the last such conversation is gone.

The reverse order SHALL NOT be used, because a conversation created between the
listing and the scan would appear to be an orphan.

#### Scenario: A conversation created mid-run is not reclaimed
- **WHEN** a conversation is created after the directory scan begins and its directory appears before the run ends
- **THEN** the conversation appears in the listing taken after the scan and its directory is left alone

#### Scenario: A deleted conversation's directory is reclaimed
- **WHEN** a directory exists for a conversation that no longer exists and no conversation records it as its workspace owner
- **THEN** the directory and its contents are removed

#### Scenario: A reused name does not shield an orphan
- **WHEN** a conversation was deleted, a new one was created under the same name, and the first one's directory remains
- **THEN** the directory is reclaimed, because no conversation carries its `ID`

#### Scenario: A deleted owner's directory survives while a member mounts it
- **WHEN** the owner conversation was deleted and a sharing member, open or closed, still records its `ID` as workspace owner
- **THEN** a reclaiming run leaves the directory alone

#### Scenario: The last member's removal frees the directory
- **WHEN** the owner and every member that shared its directory are gone
- **THEN** the next reclaiming run removes the directory
