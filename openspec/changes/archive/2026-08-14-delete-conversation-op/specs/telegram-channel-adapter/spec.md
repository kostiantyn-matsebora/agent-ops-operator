## ADDED Requirements

### Requirement: The adapter marks a deleted conversation's topic and closes it again
On `delete-conversation` the Telegram adapter SHALL un-archive the forum topic
if it is closed, post the notice into it, and close it again.

All three steps are required by the transport: a closed forum topic REFUSES
`sendMessage`, and leaving it open after the notice would invite replies into a
conversation that no longer exists — replies the manager drops, because the
thread maps to nothing.

The adapter SHALL NOT delete the forum topic. The transcript above the tombstone
is what a person scrolls back to after an incident, and an archived topic already
refuses replies without destroying it.

#### Scenario: A deleted conversation's archived topic still receives its notice
- **WHEN** `delete-conversation` arrives for a topic that was archived by an earlier close
- **THEN** the topic is un-archived, the notice is posted, and the topic is closed again

#### Scenario: A live topic is marked and archived
- **WHEN** `delete-conversation` arrives for a topic that is still open
- **THEN** the notice is posted and the topic is closed

#### Scenario: The history survives
- **WHEN** a conversation is deleted
- **THEN** its forum topic and every message in it remain readable
