## Context

Three facts shape this.

**A closed forum topic refuses posts.** Telegram returns `TOPIC_CLOSED` for a
`sendMessage` into an archived topic. By the time a conversation is deleted it
has usually been `Closed` for a while, so its topics are already archived — a
plain `send` cannot reach them.

**The manager holds no transport knowledge.** It cannot know that Telegram needs
an un-archive first, that another transport deletes threads outright, and that a
third has no notion of a thread at all. That is the standing rule, and it is why
this is an operation the adapter interprets rather than a sequence the manager
orchestrates.

**Deletion is the one path where the object disappears.** Nothing can be
recorded against it, so the operation is not re-derivable — the same position
`close-topic` was in before closing stopped deleting.

## Goals / Non-Goals

**Goals:**

- Every bound surface is told, in its own transport's terms, that a conversation
  ended for good.
- The thread stops promising a reopen that is no longer possible.
- No transport dialect in `internal/`.
- An adapter that has not implemented the kind still deletes cleanly.

**Non-Goals:**

- Changing what deletion does to the object, its inputs or its volume state.
- Guaranteeing delivery. A best-effort notice bounded by the finalizer's grace
  is the correct trade: a deletion that waits forever on a down adapter is the
  failure mode the grace exists to prevent.
- Deciding for the adapter whether the thread is deleted, archived, renamed or
  left open.

## Decisions

### D1: A new operation kind, not a `send` followed by a `close-topic`

`delete-conversation` carries the target thread id and a typed message, and the
adapter decides what to do with both.

*Why not send + close-topic:* the manager would have to know that the send must
be preceded by an un-archive on Telegram and not on a web chat, which is exactly
the knowledge the contract keeps out of it. It would also be two operations for
one fact, with no way to express "if you cannot post, at least mark the thread
ended".

*Why not reuse `close-topic`:* it means something else. A closed conversation's
thread is archived AND reopenable; a deleted conversation's is neither. Reusing
the kind would make the two indistinguishable to every adapter, and the whole
point of the two-stage lifecycle is that they are different.

*Cost accepted:* a fourth kind is a fourth thing an adapter may implement. The
contract already requires tolerating unknown kinds, so not implementing it is a
degraded-but-correct adapter, not a broken one.

### D2: Named for the CONVERSATION, not the topic

`ensure-topic` and `close-topic` are topic-scoped: they act on a thread and say
so. This operation reports that the CONVERSATION is gone; what happens to the
thread is the adapter's conclusion, not the instruction.

Naming it `delete-topic` would have said the opposite — that the manager wants a
thread deleted — and an adapter whose transport keeps the thread (posting a
tombstone instead) would then be disobeying its name while doing the right
thing. The domain's noun is `Conversation`; the operation is about one ending.

### D3: It REPLACES close-topic on the deletion path

The finalizer enqueues `delete-conversation` per bound thread and waits on it
under the existing 2-minute grace. It no longer enqueues `close-topic`.

*Why:* a conversation deleted without ever being closed would otherwise receive
both, and an adapter would have to guess whether the pair meant one ending or
two. One lifecycle event, one operation.

A conversation deleted AFTER a proper close has already had `close-topic`
delivered and recorded in `status.threadsArchived[]`; it now also gets
`delete-conversation`, which is the correction: the thread was told "archived,
reopenable", and this is what makes that stop being the last word.

### D4: The message is composed by the manager, rendered by the adapter

The operation carries a `notice` message in the same markdown subset every other
message uses: the conversation is gone, its record with it, and a new message
starts a new conversation. The adapter escapes, splits and places it.

An adapter MAY ignore the message and express the ending structurally instead —
deleting the thread, renaming it — and that is a legitimate implementation. What
it may not do is nothing at all silently; the operation should be completed with
an error if it cannot act.

### D5: Telegram un-archives, posts, re-archives

`channel-telegram` reopens the forum topic if it is closed, posts the notice,
then closes it again. Three calls where one would do, because a closed topic
refuses the post and leaving it open would invite replies into a conversation
that no longer exists.

*Alternative rejected:* `deleteForumTopic`. It removes the history with the
conversation, and the history is the thing a person scrolls back to after an
incident. An archived topic with a tombstone keeps it and still refuses replies.

### D6: Best effort, bounded by the grace that already exists

The operation is not re-derivable — the object is going away, and there is
nowhere to record a marker. The finalizer waits for completion or 2 minutes,
then releases and the deletion proceeds regardless.

*Why that is right:* the alternative is a deletion that can be wedged by a down
adapter, which is the exact failure the grace was introduced to prevent. A
missing tombstone is a cosmetic loss; a conversation that cannot be deleted is
an operational one.

## Risks / Trade-offs

- **A fourth operation kind to implement** → tolerating unknown kinds is already
  contractual; the two reference adapters implement it and the rest degrade to
  today's behaviour, which is silence.
- **Three Bot API calls per deleted thread** → deletion is rare compared with
  sends, and the adapter's rate limiting already paces it.
- **The notice can be lost** → bounded by the same grace as `close-topic`, and
  the loss is a missing message rather than a stuck object.
- **An adapter posts into a topic it then closes, racing a person mid-reply** →
  Telegram refuses their post, which is the correct outcome: the conversation is
  gone either way.

## Migration Plan

1. Land the manager change; adapters that do not implement the kind report it
   failed and deletion proceeds after the grace, exactly as before.
2. Ship `channel-telegram` and `console` images implementing it.
3. Document the kind in `docs/contracts.md` and the deletion behaviour in
   `docs/concepts.md`.

Rollback: revert the manager image. `close-topic` returns to the deletion path
and threads go back to being told nothing.

## Open Questions

- Should the notice name WHY it was deleted (autodelete window vs. an operator's
  action)? Leaning yes for the automatic case, as the farewell already names its
  window — deferred until the wording is exercised on a real surface.
