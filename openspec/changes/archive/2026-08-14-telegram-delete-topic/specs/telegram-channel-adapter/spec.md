## MODIFIED Requirements

### Requirement: The adapter marks a deleted conversation's topic and closes it again
On `delete-conversation` the Telegram adapter SHALL, BY DEFAULT, un-archive the
forum topic if it is closed, post the notice into it, and close it again.

All three steps are required by the transport: a closed forum topic REFUSES
`sendMessage`, and leaving it open after the notice would invite replies into a
conversation that no longer exists — replies the manager drops, because the
thread maps to nothing.

By default the adapter SHALL NOT delete the forum topic. The transcript above
the tombstone is what a person scrolls back to after an incident, and an
archived topic already refuses replies without destroying it. A surface MAY opt
out of that default (below).

#### Scenario: A deleted conversation's archived topic still receives its notice
- **WHEN** `delete-conversation` arrives for a topic that was archived by an earlier close
- **THEN** the topic is un-archived, the notice is posted, and the topic is closed again

#### Scenario: A live topic is marked and archived
- **WHEN** `delete-conversation` arrives for a topic that is still open
- **THEN** the notice is posted and the topic is closed

#### Scenario: The history survives by default
- **WHEN** a conversation is deleted on a surface that has not opted in
- **THEN** its forum topic and every message in it remain readable

## ADDED Requirements

### Requirement: A surface may opt into deleting the topic with the conversation
The adapter SHALL read an optional boolean `deleteTopicOnConversationDelete`
from the served `Channel`'s opaque `spec.config`, defaulting to FALSE when
absent. When it is true, `delete-conversation` SHALL delete the forum topic
instead of un-archiving, posting the notice and closing it.

The setting SHALL live on the CHANNEL, never on the `ChannelAdapter`. The
adapter CR carries implementation and workload knobs only, and whether a
group's threads should survive their conversations is a property of that group —
two surfaces served by one adapter may reasonably differ.

Deleting SHALL REPLACE the notice rather than follow it. The tombstone exists so
that a person returning to the thread understands why it stopped; a thread about
to disappear has nobody to tell, and posting first would write a message into a
topic being removed by the next call.

A failed deletion SHALL be reported as an operation failure and SHALL NOT fall
back to marking and archiving. `deleteForumTopic` requires the bot to hold
`can_delete_messages`, and a silent fallback would make the setting mean "delete
the topic, or maybe not" — leaving an operator who enabled it to keep a group
tidy with a growing list of archived topics and no signal that anything was
wrong. Conversation deletion itself SHALL still proceed, bounded by the
finalizer's existing grace.

The chart-shipped `ChannelAdapter.configSchema` SHALL declare the key, so a
misspelling is reported on the Channel's `ConfigValid` condition. A boolean
whose absence is indistinguishable from `false` is exactly the mistake that
declaration exists to catch.

After a successful deletion the adapter SHALL post ONE line to the chat's
GENERAL surface, naming the conversation. Deleting the topic destroys the only
place that conversation was visible, and its `Conversation` object is gone too —
so without this a thread simply vanishes and NOTHING anywhere records that
agent-ops did it. A reader would reasonably conclude somebody deleted it by hand.

The note SHALL be posted AFTER the deletion, never before: announcing a removal
that then failed is worse than silence. A failure to post the note SHALL be
logged and SHALL NOT fail the operation, because the topic is already gone and
retrying the operation would ask for a deletion that already succeeded.

The chart SHALL expose the setting as one value on the Telegram bundle's surface
block, rendered into that Channel's config, and SHALL default it off.

#### Scenario: An opted-in surface loses the topic with the conversation
- **WHEN** `delete-conversation` arrives for a channel whose config sets `deleteTopicOnConversationDelete: true`
- **THEN** the forum topic is deleted, no tombstone is posted into it, and one line naming the conversation appears on the general surface

#### Scenario: The group can tell agent-ops did it
- **WHEN** a topic is deleted with its conversation
- **THEN** the general surface carries a line naming the conversation, so the removal is not indistinguishable from someone deleting the topic by hand

#### Scenario: A failed note does not fail the operation
- **WHEN** the topic is deleted but the general-surface note cannot be posted
- **THEN** the operation still succeeds and the failure is logged, because the deletion already happened

#### Scenario: The default is unchanged
- **WHEN** the key is absent from a channel's config
- **THEN** the adapter marks and archives the topic exactly as before, and no transcript is lost by upgrading

#### Scenario: A missing permission is reported rather than hidden
- **WHEN** the bot lacks `can_delete_messages` and the topic cannot be deleted
- **THEN** the operation is completed with an error, the adapter does not archive the topic instead, and the conversation is still deleted once the grace expires

#### Scenario: Two surfaces may differ
- **WHEN** one Telegram channel opts in and another served by the same adapter does not
- **THEN** each behaves according to its own config

#### Scenario: A misspelled key is reported
- **WHEN** a Channel's config carries a near-miss of the key
- **THEN** the Channel's `ConfigValid` condition reports it, rather than the setting silently reading as false
