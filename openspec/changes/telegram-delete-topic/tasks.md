## 1. The adapter

- [x] 1.1 Add `deleteTopicOnConversationDelete` (optional bool, default false) to the adapter's `channelConfig` parsing in `channel-telegram/`
- [x] 1.2 Add `DeleteTopic` to the Bot API client — `deleteForumTopic`, tolerating an already-gone topic the way `CloseTopic` tolerates an already-closed one, since ops are at-least-once
- [x] 1.3 Branch the `delete-conversation` handler on the flag: delete the topic INSTEAD OF un-archive → post → close
- [x] 1.4 Report a failed deletion as an operation error; do NOT fall back to marking and archiving
- [x] 1.5 NOT PLANNED, found live: add the periodic `refreshLoop` this adapter never had. `refreshChannels` ran once at startup and again only for a channel it had never seen, so editing an EXISTING Channel's config reached the adapter only on a pod restart — the console adapter has had this loop all along

## 2. Tests

- [x] 2.1 Opted-in surface: one `deleteForumTopic` call, and no `sendMessage`, `reopenForumTopic` or `closeForumTopic`
- [x] 2.2 Default surface: the existing three-call sequence, unchanged — this is the regression that matters
- [x] 2.3 A `deleteForumTopic` failure is returned as an op error and archives nothing
- [x] 2.4 Two channels, one opted in and one not, each behave by their own config

## 3. Chart

- [x] 3.1 Add `surface.deleteTopicOnDelete` (default false) to `chart/charts/telegram-bundle/values.yaml`, with a ONE-LINE comment naming the trade
- [x] 3.2 Render it into the Channel's `config` in `templates/surface.yaml`, omitted when false so the config stays clean
- [x] 3.3 Declare the key in the chart-shipped `ChannelAdapter.configSchema`
- [x] 3.4 Chart render tests: absent by default; present when set; the schema declares it

## 4. Documentation

- [x] 4.1 `docs/telegram-bundle.md`: the option, that it is per-surface, that it destroys the transcript, and that the bot needs `can_delete_messages`
- [x] 4.2 `CHANGELOG.md`: a short entry — opt-in, off by default, nothing changes on upgrade

## 5. Verification

- [x] 5.1 Build, vet and test every module in the warm container
- [x] 5.2 Build and push `channel-telegram`; bump the chart image tag and versions
- [x] 5.3 Live: with the flag ON, delete a conversation and confirm its topic is gone; with it OFF, confirm the tombstone behaviour still holds
