## Why

A Conversation's object name is pure `metadata.generateName` output — a fixed
prefix (`alert-`, `job-`, `chat-`, `task-`, `member-`) plus Kubernetes' own
random 5-character suffix. A human reads `member-h6jv7` or `alert-2qj2j` and
learns nothing.

That token is everywhere: `kubectl get conversations`, the derived pod name
(`agentops-conv-member-h6jv7`), the console's secondary name line, the YAML
tab heading, and every fallback where `spec.title` is empty or not yet
rendered.

`spec.title` already exists and carries a readable label in most cases. But
the object's actual NAME — the thing an operator types, greps, and reads in
`kubectl` output — carries no content of its own.

## What Changes

- Every conversation-creation site builds its object `Name` directly — never
  a random `generateName` suffix — as `<kind>-<words>`, where `<words>` is a
  short chain of significant words (not a single truncated token) drawn from
  context already computed at that call site:

  | Creation path | Word source | Example |
  |---|---|---|
  | Root conversation from a signal (`alert`, `job`, `chat`, `task` kind) | the same title text already derived for `spec.title` | `alert-node-down-ns` |
  | Task conversation opened from a channel command | the addressed pipeline name | `task-deploy-notes` |
  | Member conversation created by a Coordinator `invoke` | the invoked `agents[]` entry name | `member-researcher` |

- Word-chain extraction splits on punctuation and `CamelCase` boundaries
  (so an alert named `NodeDown` reads as `node-down`), drops a small fixed
  stopword list (`the`, `a`, `of`, `on`, …), keeps the first three
  remaining words, and caps the total length well inside Kubernetes' name
  and label-value limits — dropping a whole trailing word rather than
  cutting one in half.

- **The one existing gap that could leave a conversation with no words at
  all is closed, not routed around.** `titleForGroup`'s fallback to the
  claiming `SignalSource`'s own name — already used for `alert`/`job` when a
  signal carries no title — is extended to `chat`/`task` too. A
  caption-less sticker, voice note, or photo on a chat surface now slugs
  from the source's name (e.g. `chat-ops-room`) instead of producing empty
  text. A `SignalSource` name is a required Kubernetes field, so after this
  change every creation path always has at least one word to build from.

- **Collisions are resolved by a lookup, never by guessing and never by a
  random suffix.** Each conversation carries an immutable label naming its
  own base name (`agentops.dev/name-base: <kind>-<words>`). Creating a new
  conversation lists existing conversations sharing that label, picks the
  next unused numeric suffix, and creates with that name
  (`alert-node-down-ns-2`, `-3`, …) in one list-then-create step — not
  by retrying guesses one at a time. A create conflict from a genuine race
  between two concurrent creators retries that same lookup a small, bounded
  number of times.

- `spec.title` itself is untouched: already populated at creation for every
  path above, and remains the primary human-facing label everywhere a UI
  already prefers it. This change makes the k8s object NAME readable too —
  a second, independent improvement, not a replacement for `spec.title`.

**Out of scope**: adding a `kubectl` printer column for `spec.title`,
renaming already-created conversations, and changing the derived
pod/PVC/ConfigMap name schemes (`agentops-conv-<name>`, etc.) beyond
accommodating the new, still-bounded conversation name length.

## Capabilities

### New Capabilities
- `conversation-naming`: how a Conversation's object `Name` is built as a deterministic word-chain, how collisions are resolved without any random suffix, and the guarantee that every creation path has words to build from.

### Modified Capabilities
(none — no existing spec currently constrains `metadata.generateName` or `metadata.name` content, so there is no delta to an existing capability)

## Impact

- **Code**: `platform/manager/internal/httpapi/signals.go` (`createConversationForGroup`, `titleForGroup`), `platform/manager/internal/chat/router.go` (`CreateTaskConversation`), `platform/manager/internal/chat/coordinate.go` (`createMember`), plus a new word-chain and name-allocation helper (package decision for `design.md`).
- **No CRD field change.** The name-allocation label is ordinary `metadata.labels` content, available on any object today. No new Conversation spec field, no CRD regeneration.
- **No HTTP contract change.** `/signal/inbound`'s request/response shapes are unchanged. This only changes what the manager does internally with inputs it already receives.
- **RBAC**: the manager already has `list`/`watch` on Conversations for its own reconciler, so the new label-based list needs no new permission.
- **Docs**: `docs/concepts.md` gets one short note under the Conversation section describing the word-chain naming convention and the `agentops.dev/name-base` label. No adopter-site page (landing page, Introduction, Getting started, Installation, guides) makes any claim this contradicts — none of them describe conversation naming today — so no adopter-facing page needs an update. `CHANGELOG.md` gets an entry, since object names an operator may have scripted against change shape (non-breaking, but worth a line).
