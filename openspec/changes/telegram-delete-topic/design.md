## Context

`delete-conversation` reports a fact — this conversation is gone — and leaves
the thread consequence to the adapter. That was the whole argument for naming it
after the conversation rather than the topic, and this change is the first time
one adapter offers two answers to the same operation.

Three constraints shape it:

- **`ChannelAdapter` carries no configuration.** It is implementation and
  workload knobs only; everything an operator sets lives on the served
  `Channel`, in an opaque `config` the serving adapter alone interprets. A flag
  on the adapter CR would also force every Telegram surface in an install to
  agree, which they need not.
- **`deleteForumTopic` requires `can_delete_messages`.** A bot added to a group
  without that right can create and close topics but not delete them, so the
  failure is ordinary and must be reported rather than assumed away.
- **The manager must learn nothing.** It already sends one operation; a second
  kind, or a flag on the op, would put transport policy back in `internal/`.

## Goals / Non-Goals

**Goals:**

- An operator can choose, per surface, whether deleting a conversation takes its
  topic with it.
- The default is unchanged, and upgrading destroys no transcript.
- A misconfiguration is visible before it matters.
- The manager stays unaware.

**Non-Goals:**

- Changing `close-topic`. A closed conversation can be reopened, so its topic
  must survive; only DELETION is final enough to justify removing it.
- Offering the same switch on other transports. Each adapter decides what
  ending means for it, and inventing a cross-transport "deletion style" would be
  the manager holding transport policy by another route.
- Recovering a deleted topic. There is no undo, which is the point of the flag
  being opt-in.

## Decisions

### D1: The flag is Channel config, named for the behaviour

`Channel.spec.config.deleteTopicOnConversationDelete: true` (default absent =
false). The chart exposes it as `telegram-bundle.surface.deleteTopicOnDelete`
and renders it into that Channel's config.

*Why config and not the adapter CR:* the adapter is implementation. Config on it
would be the one place this project keeps free of it, and it would force two
surfaces served by the same adapter to share a policy that is a property of the
GROUP — a noisy alerting supergroup and a small ops chat can reasonably differ.

*Why named for the behaviour rather than the mode:* `deleteTopic: true` reads as
"delete the topic" with no object; the key says which event it applies to, so a
reader who finds it on a Channel knows it does nothing until a conversation is
deleted.

### D2: Deleting REPLACES the tombstone, and the record moves to the general surface

With the flag on, the adapter calls `deleteForumTopic` and posts NOTHING INTO THE
TOPIC — then posts one line naming the conversation to the chat's GENERAL
surface.

*Why no tombstone in the topic:* it exists so a person who returns to the thread
understands why it stopped. If the thread is about to disappear there is nobody
to tell, and posting first would write a message into a topic the next call
removes.

*Why a line on the general surface:* because otherwise there is no evidence
ANYWHERE. The topic is gone and the `Conversation` is gone, so a reader sees a
thread that existed yesterday and does not now, and would reasonably conclude a
person deleted it by hand. This was drafted as an open question leaning the
other way, on noise grounds; that weighed the wrong cost — one line per deleted
conversation is far cheaper than an unexplained disappearance.

*Ordering:* after the deletion, never before. Announcing a removal that then
failed is worse than silence. And a failed note does not fail the operation: the
topic is already gone, so a retry would ask for a deletion that already
succeeded.

### D3: A missing permission is an operation failure, not a fallback

If `deleteForumTopic` fails — most likely `can_delete_messages` — the adapter
completes the operation with the error. It does NOT silently fall back to
marking and archiving.

*Why:* a fallback would make the setting mean "delete the topic, or maybe not",
and an operator who enabled it to keep the group tidy would find a growing list
of archived topics with no signal that anything was wrong. The failure is
already handled well downstream: it is logged, and the finalizer's grace lets
the conversation delete regardless, so a wrong permission costs a leftover topic
and a log line rather than a stuck object.

*Alternative rejected:* try to delete, and mark-and-archive on failure. Softer,
and it hides exactly the misconfiguration worth seeing.

### D4: The declared schema carries the key

The chart-shipped `ChannelAdapter.configSchema` gains the property, so a
misspelling surfaces on the Channel's `ConfigValid` condition. The schema is
advisory — the adapter remains authoritative — but this is precisely the class
of mistake it exists to catch: a boolean whose absence is indistinguishable from
`false`, on a feature nobody notices is off until a topic they expected to
disappear is still there.

### D5: The manager is untouched

No new operation kind, no field on the op, no manager-side condition. The
adapter answers `delete-conversation` differently depending on its own channel's
config, which is the contract working as designed rather than being extended.

That is worth stating because the obvious shortcut — a `deleteTopic` boolean on
the operation, set from a chart value read by the manager — would have been
fewer lines and would have put a Telegram concept in `internal/`.

## Risks / Trade-offs

- **An operator enables it and loses a transcript they wanted** → off by
  default, named for what it does, and the values comment states the trade
  where the setting is.
- **The bot lacks `can_delete_messages`** → reported as an operation failure and
  visible in the adapter log; deletion of the conversation still proceeds.
- **A topic vanishes with no explanation for anyone reading it** → one line on
  the general surface names the conversation, so the removal is attributable to
  agent-ops rather than looking like someone deleting a thread by hand.
- **Two surfaces configured differently confuse an operator** → that is the
  reason it is per-Channel; the alternative is one policy for surfaces with
  different purposes.

## Migration Plan

1. Ship the adapter image; the flag is absent everywhere, so behaviour is
   unchanged.
2. An operator opting in sets one value, confirms the bot holds
   `can_delete_messages`, and deletes one conversation to watch the topic go.
3. Rollback is the same value set back to false; topics deleted while it was on
   do not come back.

## Open Questions

**Resolved during implementation:** *should the general surface get a one-line
note when a topic is deleted?* **Yes.** The draft leaned no, on the grounds that
it puts noise on the surface people type on. That weighed the wrong cost. With
the topic deleted AND the `Conversation` gone, there is no record anywhere that
agent-ops removed anything — a reader sees a thread that was there yesterday and
is not there now, and would reasonably conclude a person deleted it. One line
naming the conversation is the only evidence the group can have, and it is
cheaper than the confusion it prevents. Posted after the deletion, and a failure
to post it does not fail the operation.
