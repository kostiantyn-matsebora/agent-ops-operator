## 1. The operation

- [x] 1.1 Add the `delete-conversation` kind to `internal/chat/ops.go` with an enqueue helper carrying the thread id and a typed `notice`, and a stable id per conversation×channel
- [x] 1.2 Compose the notice manager-side: the conversation and its record are gone, and a new message starts a new conversation — no transport dialect
- [x] 1.3 Complete it like `close-topic`: terminal either way, logged rather than written as a condition, since the object carrying one is disappearing

## 2. The deletion path

- [x] 2.1 `finalizeClose` enqueues `delete-conversation` per bound thread INSTEAD OF `close-topic`, whether or not the conversation was closed first
- [x] 2.2 Keep the 2-minute grace and the release-regardless rule — a deletion must never wedge on a down adapter
- [x] 2.3 Tests: a closed conversation's threads get the new op on deletion; a never-closed one gets it and no `close-topic`; the grace still releases

## 3. Telegram

- [x] 3.1 Handle `delete-conversation`: un-archive if closed, post the notice, close again — a closed forum topic refuses `sendMessage`
- [x] 3.2 Do NOT delete the forum topic; the history above the tombstone is the point
- [x] 3.3 Tests against the stub Bot API: the three-call sequence on an archived topic, and post-then-close on a live one

## 4. Console

- [x] 4.1 Handle `delete-conversation`: append the notice and mark the transcript archived, as it already does for `close-topic`
- [x] 4.2 Test that the transcript stays readable for the session and offers no composer

## 5. Documentation

- [x] 5.1 `docs/contracts.md`: the fourth kind, its payload, and that the adapter decides what ending looks like
- [x] 5.2 `docs/concepts.md`: what a deleted conversation's threads are told, next to the two-stage lifecycle
- [x] 5.3 `CLAUDE.md`: the op list gains the kind; note that it REPLACES `close-topic` on the deletion path

## 6. Verification

- [x] 6.1 Build, vet and test every module in the warm container
- [x] 6.2 Build and push `channel-telegram` and `console`; bump the chart image tags
- [x] 6.3 Live: delete a closed conversation and confirm its Telegram topic receives the notice and is left archived with its history
