## Why

Deleting a conversation currently leaves its Telegram forum topic behind: the
adapter un-archives it, posts a tombstone and closes it again, so the topic
survives with its whole transcript above a line saying the conversation is gone.

That is the right default — the history is what a person scrolls back to after
an incident — but it is not the only reasonable answer. A busy group accumulates
one archived topic per conversation forever, and a group used for
high-volume alerting ends up with a topic list nobody can navigate, full of
threads whose conversations were deliberately reclaimed. An operator who deletes
a conversation to reclaim it may well mean "and take the thread with it".

The trade is real in both directions, which is exactly why it should be a
choice rather than a default: keeping the topic costs clutter, deleting it costs
the transcript, and only the person running the group knows which they want.

## What Changes

- **A new opt-in on the Telegram surface**: when enabled, a
  `delete-conversation` operation DELETES the forum topic instead of marking and
  archiving it.
- **OFF by default.** The shipped behaviour is unchanged: un-archive, post the
  tombstone, close. Nobody loses a transcript by upgrading.
- **It is CHANNEL configuration, not adapter configuration.** The flag lives in
  the opaque `Channel.spec.config` the serving adapter already interprets —
  `ChannelAdapter` carries implementation only, never configuration, and two
  Telegram surfaces in one install may reasonably differ.
- **The chart surfaces it** as one value on the telegram bundle's surface block,
  rendered into that Channel's config.
- **The adapter's `configSchema` declares it**, so a typo is reported on the
  Channel's `ConfigValid` condition rather than silently ignored.
- **A failure to delete is reported, not swallowed.** `deleteForumTopic` needs
  the bot to hold `can_delete_messages`; without it the operation completes with
  an error and the deletion proceeds after the finalizer's grace, as it already
  does for any adapter that cannot act.

Not in scope: changing what `close-topic` does (a closed conversation is
reopenable, so its topic must survive), any behaviour for other transports, and
the manager, which learns nothing about this — it sends the same operation
either way.

## Capabilities

### New Capabilities

None.

### Modified Capabilities

- `telegram-channel-adapter`: the opt-in, where it is configured, what it does
  to the topic, and what happens when the bot lacks the right to do it.

## Impact

- `channel-telegram/` — config parsing for the new key, the branch in the
  `delete-conversation` handler, and a `deleteForumTopic` call; new image.
- `chart/charts/telegram-bundle/` — one value on the surface block, rendered
  into the Channel config, plus the key in the shipped `configSchema`.
- `docs/telegram-bundle.md` — the option and its trade-off.
- No manager change, no CRD change, no new RBAC. The manager sends
  `delete-conversation` exactly as before; what it means on this transport is
  the adapter's decision, which is the contract this feature exercises rather
  than bends.
