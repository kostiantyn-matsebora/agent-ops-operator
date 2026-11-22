## MODIFIED Requirements

### Requirement: Concurrent runtime pods never share one checkout
When a workspace claim is used, each conversation SHALL receive its own
subdirectory within the claim, mounted so that no two runtime pods can observe
or modify each other's checkout. A shared working tree across concurrent agents
SHALL NOT be possible by configuration, with exactly one exception: a member
whose `Coordinator` entry declares `workspace: shared` mounts its workspace
owner's subdirectory (`member-workspace-sharing`).

The exception SHALL be declared on the Coordinator entry and nowhere else. No
chart value, `Pipeline` field, runtime setting or equality of two claim names
SHALL make two conversations share a subdirectory. The claim ROOT SHALL remain
unreachable to agent code, shared or not.

#### Scenario: Two conversations, two trees
- **WHEN** two conversations run concurrently against the same workspace claim
- **THEN** each pod sees only its own checkout and neither can read or write the other's

#### Scenario: Restart mid-conversation keeps the tree
- **WHEN** a runtime pod for a conversation is deleted and recreated while the conversation is still open
- **THEN** the new pod sees the same checkout, including work not yet committed

#### Scenario: Two Coordinators' members on one claim stay separate
- **WHEN** two members of different roots resolve the same workspace claim and neither entry declares `workspace: shared`
- **THEN** each sees only its own subdirectory

#### Scenario: A declared share is the only share
- **WHEN** a member's entry declares `workspace: shared`
- **THEN** it and its owner see the same subdirectory, and no third conversation does

#### Scenario: The claim root is still unreachable
- **WHEN** a member shares its owner's tree
- **THEN** its pod mounts only the owner's subdirectory, never the claim root
