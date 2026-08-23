# telegram-channel-adapter

## Purpose

The reference Telegram channel adapter: an external, dependency-free process serving type=telegram via the adapter contract.

## Requirements

### Requirement: Telegram runs as an external reference adapter, not in the manager
The manager SHALL contain no Telegram-specific code (no poller, no Bot API client, no bot-token reads) **and no Telegram-specific presentation**: HTML composition, entity escaping, the 4096-character message limit, and forum-topic naming limits SHALL live in the adapter alone. A reference adapter in `channels/telegram/` (own binary and image, precedent `runtimes/claude/`) SHALL serve Channels with `adapter: telegram`, consuming the channel adapter contract: offset persistence, approver filtering by Telegram user id, topic creation via `createForumTopic`, and message sends with HTML parse mode and general-topic fallback.

The adapter SHALL render each typed outbound message for Telegram: composing HTML from markdown bodies, escaping payload content so markup-bearing text cannot break parsing, splitting or truncating messages that exceed the transport limit, and deriving forum-topic names from the `ensure-topic` descriptor within Telegram's own naming limit. It MAY present an oversized `signal` payload as a document rather than text. Routing-visible behavior (commands, adoption, default profile, busy-acks) SHALL be whatever the shared router implements — the adapter adds no routing rules of its own, so a command naming a Pipeline reaches Telegram users through exactly the same path as any other surface.

#### Scenario: End-to-end Telegram flow through the adapter
- **WHEN** a Telegram user sends `/agents` to a bot whose adapter serves a `adapter: telegram` Channel
- **THEN** the listing of addressable Pipelines arrives in Telegram, produced by the shared router as a `notice` message and rendered by the adapter

#### Scenario: Manager has no Telegram surface
- **WHEN** the manager runs with no Telegram adapter deployed
- **THEN** it performs no Telegram API calls and no bot-token secret reads, and non-Telegram channels work normally

#### Scenario: Approver filtering stays enforced
- **WHEN** a Telegram update arrives from a user id not in the channel's configured approvers
- **THEN** the adapter drops it without posting to `/channel/inbound`

#### Scenario: Oversized message is split, not failed
- **WHEN** a message renders longer than Telegram's per-message limit
- **THEN** the adapter splits or truncates it and the operation completes successfully

#### Scenario: Markup in a payload does not break the post
- **WHEN** a signal payload contains `<`, `>`, or `&`
- **THEN** the adapter escapes it and the message posts with the content intact

#### Scenario: Topic name comes from the descriptor
- **WHEN** an `ensure-topic` descriptor would render a name longer than Telegram allows
- **THEN** the adapter shortens it and creates the topic

### Requirement: Adapter owns its credentials and config parsing
The adapter SHALL resolve each served channel's bot token from the projected credential environment advertised by the contract's `credentialEnvPrefix` (env `<prefix>botToken`), falling back to the `TELEGRAM_BOT_TOKEN` environment variable for channels without `credentialsSecretRef` (hand-deployed back-compat). It SHALL parse its channel settings (chat id, approvers, polling enablement, feed thread) from `spec.config` of the Channels it serves, reporting config errors — including a missing token from both sources — on the Channel's status condition rather than crashing.

#### Scenario: Per-channel token resolved from projection
- **WHEN** a served Channel's listing entry maps `botToken` to a projected env var
- **THEN** the adapter uses that token for this channel's polling, topic creation, and sends

#### Scenario: Fallback token preserved
- **WHEN** a served Channel has no `credentialsSecretRef` and `TELEGRAM_BOT_TOKEN` is set
- **THEN** the adapter serves it with the fallback token exactly as before

#### Scenario: Invalid config is surfaced on the Channel
- **WHEN** a `adapter: telegram` Channel's `config` lacks a required field (e.g. chat id) or no token is resolvable
- **THEN** the adapter sets a not-ready condition with the reason on that Channel and continues serving other Channels

### Requirement: Single getUpdates consumer preserved
Exactly one getUpdates consumer per **bot token** SHALL hold at all times. The polling loop SHALL live in the telegram ROUTER component, not in the channel adapter: the router SHALL run one loop per distinct token, its workload SHALL run single-instance (replicas 1, Recreate), and the channel adapter SHALL NOT poll at all — it receives topic messages forwarded by the router and continues posting them to `/channel/inbound`. The documented migration SHALL sequence shutdown of the previously-polling adapter before the router starts, so two consumers of one token are never live simultaneously.

#### Scenario: Two bots poll independently, once each
- **WHEN** the router serves two channels with distinct projected tokens
- **THEN** it runs exactly two getUpdates loops — one per token — inside the single pod

#### Scenario: The channel adapter never polls
- **WHEN** the split stack is running
- **THEN** the channel adapter makes no getUpdates call, and its only Telegram API calls are sends and topic creation

#### Scenario: Upgrade never double-polls
- **WHEN** an install migrates from the polling channel adapter to the router-fronted stack following the documented steps
- **THEN** at no point do two getUpdates consumers use the same bot token

#### Scenario: Offset survives the split
- **WHEN** the migration completes
- **THEN** the router resumes from the offset the previous adapter persisted, rather than re-reading old updates

### Requirement: Chart deploys the adapter opt-in
The chart SHALL ship the Telegram adapter as a `ChannelAdapter` CR (gated on `telegramAdapter.enabled`, default **false**) instead of a bespoke Deployment template; the chart SHALL contain no channel-type-specific workload templates. The `ChannelAdapter` reconciler owns the workload, auth injection, and credential projection; the chart's remaining role is the CRD and the gated CR.

#### Scenario: Disabled by default
- **WHEN** the chart renders with default values
- **THEN** no Telegram adapter resources are produced

#### Scenario: Enabled renders only a CR
- **WHEN** `telegramAdapter.enabled=true`
- **THEN** the rendered output contains a `ChannelAdapter` for `adapter: telegram` and no Deployment for it (the reconciler creates the workload)

### Requirement: Chart-shipped ChannelAdapter declares the telegram config schema
The chart's gated telegram `ChannelAdapter` CR SHALL declare the config contract on its spec: a JSON Schema for `spec.config` declaring `chatId` (string, required), `feedThreadId` (integer), `approvers` (array of integers), and `deleteTopicOnConversationDelete` (boolean), plus `credentialKeys` documenting `botToken` (not required — the `TELEGRAM_BOT_TOKEN` fallback exists). The declaration SHALL live beside the `image` reference in the same template so an image bump and its schema update travel in one diff. The adapter binary SHALL be unchanged — it plays no role in the declaration.

#### Scenario: Declaration matches the parser
- **WHEN** the chart renders with `telegramAdapter.enabled=true`
- **THEN** the `ChannelAdapter` for telegram declares exactly the fields the adapter's config parser accepts, with `chatId` required

#### Scenario: Misconfigured channel flagged before the adapter sees it
- **WHEN** a `type: telegram` Channel is created with `config: {}` while the declaring ChannelAdapter exists
- **THEN** the Channel gains `ConfigValid=False` naming `chatId` from the manager, in addition to whatever Ready condition the adapter later reports

#### Scenario: Adapter binary unchanged
- **WHEN** the telegram adapter image runs against a ChannelAdapter with or without the declaration
- **THEN** its behavior is identical — it neither reads nor publishes any schema

### Requirement: Adapter closes forum topics on close-topic operations
The Telegram channel adapter SHALL serve the `close-topic` operation by closing
the corresponding forum topic through the Bot API (`closeForumTopic`) for the
chat the Channel's config names, then completing the operation with an empty
result. A Bot API failure SHALL be reported as the operation's error and SHALL
NOT be retried by the adapter — the manager treats a failed `close-topic` as
terminal.

#### Scenario: Forum topic is closed
- **WHEN** the adapter receives a `close-topic` operation for thread id `9876`
- **THEN** it calls `closeForumTopic` for that message thread and completes the operation with an empty result

#### Scenario: Bot API failure is reported, not retried
- **WHEN** `closeForumTopic` returns an error
- **THEN** the adapter completes the operation with that error and does not retry it

#### Scenario: Duplicate close is tolerated
- **WHEN** the same `close-topic` operation id is delivered twice
- **THEN** the adapter completes it without treating an already-closed topic as a failure

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

### Requirement: The adapter re-reads its channels periodically
The Telegram adapter SHALL refresh its channel configuration from the manager on
a recurring interval, not only at startup and on first sight of an unknown
channel. `Channel.spec.config` is live configuration an operator edits, and an
adapter that reads it once cannot see an edit until its pod restarts — which
made enabling topic deletion on a running surface do nothing at all, with no
error anywhere to explain why.

#### Scenario: A config edit reaches a running adapter
- **WHEN** an operator edits an existing Channel's `spec.config` while the adapter is running
- **THEN** the adapter picks the change up within one refresh interval, without a restart

### Requirement: Adapter paces its Bot API calls
The Telegram adapter SHALL pace every outbound Bot API call against two
independent budgets: a global per-bot rate and a per-chat rate, the per-chat
budget being the tighter of the two for a group. Pacing SHALL cover
`createForumTopic` as well as `sendMessage`, because a burst of new
conversations spends the same budget as a burst of replies — an incident in
which every topic was created but most stayed empty is the shape this
requirement exists to prevent.

The budgets SHALL be constants of the adapter, not manager-side configuration
and not `Channel.spec.config` fields. They describe Telegram, which the adapter
is the only component that knows.

Pacing SHALL NOT be implemented by holding claimed operations in an adapter-side
queue. The adapter SHALL instead defer requesting its next operation until its
budget allows, leaving unclaimed work in the manager's queue where it stays
derivable from CR state.

#### Scenario: Concurrent conversations do not exhaust the budget
- **WHEN** dozens of conversations open within a few minutes and each requires a topic and a reply
- **THEN** the adapter spreads its calls within the per-chat and global budgets and every topic receives its card and its answer

#### Scenario: Pacing holds no operations
- **WHEN** the adapter is pacing and its budget is momentarily exhausted
- **THEN** it delays its next long-poll rather than claiming operations it cannot yet send, and an adapter restart at that moment loses nothing

#### Scenario: Budgets are not configurable
- **WHEN** an operator inspects the Channel or ChannelAdapter CRs
- **THEN** no rate-limit field is present, because the limits are Telegram's and belong to the implementation

### Requirement: Adapter honors Telegram's retry_after
On a `429 Too Many Requests`, the adapter SHALL read `parameters.retry_after`
from the Bot API response, wait that many seconds, and retry the same call. It
SHALL NOT report the operation as failed while retries remain within budget, and
SHALL NOT substitute its own backoff for a stated `retry_after`.

The adapter's total wait for one operation SHALL be bounded well below the
manager's claim reclaim interval of five minutes. When that bound is reached the
adapter SHALL report the operation as failed and let the manager re-derive it.

A `429` on `createForumTopic` SHALL be retried on the same terms as one on
`sendMessage`; both are ordinary backpressure, not errors.

#### Scenario: 429 on send is retried transparently
- **WHEN** `sendMessage` returns `429` with `retry_after: 30`
- **THEN** the adapter waits 30 seconds, retries, and reports the operation complete on success

#### Scenario: 429 on topic creation is retried transparently
- **WHEN** `createForumTopic` returns `429` with a stated `retry_after`
- **THEN** the adapter waits and retries rather than reporting a failed `ensure-topic`

#### Scenario: Retry budget bounded below reclaim
- **WHEN** repeated `429` responses would push the adapter's total wait for one operation toward five minutes
- **THEN** the adapter stops retrying and reports failure while the claim is still valid, so the manager re-derives the operation instead of a second claimant duplicating it

#### Scenario: A retried call posts exactly once
- **WHEN** the adapter retries a `sendMessage` after a `429`
- **THEN** exactly one message appears in the thread, because the rejected call posted nothing

### Requirement: The adapter registers the vocabulary Telegram can express
The adapter SHALL register commands from the manager's vocabulary with Telegram,
scoped to each chat it serves, so Telegram renders its own command control in
the composer and completes what a person types. That control is the entry
point: it appears without a message being posted and does not scroll away.

Both kinds SHALL be registered — the built-in commands and the addressable
Pipelines — because a person is choosing between them in one composer and a menu
listing only half of what may be typed teaches that the other half does not
exist.

The adapter SHALL filter and adapt the vocabulary against Telegram's own rules
for command names, description length and list size. Those rules SHALL live in
the adapter. The manager SHALL publish names unchanged, and an entry Telegram
cannot express SHALL be adapted or omitted locally — never renamed in the
manager, never carried back as a constraint on the vocabulary, and never
resolved by renaming the Pipeline itself.

#### Scenario: Both kinds become Telegram commands
- **WHEN** the adapter reads the vocabulary
- **THEN** it registers the built-in commands and the Ready Pipelines for each
  served chat, and Telegram completes both as a person types

#### Scenario: Telegram's rules stay in the adapter
- **WHEN** the manager's vocabulary is inspected
- **THEN** it carries no Telegram naming rule, length limit or ordering
  constraint

#### Scenario: The Pipeline is never renamed to fit
- **WHEN** a Pipeline's name cannot be registered verbatim
- **THEN** the Pipeline CR is unchanged, the manager's published name is
  unchanged, and only the adapter's presentation differs

### Requirement: A transport-local spelling is translated back on receipt
Where a Pipeline's name cannot be a command name on the transport, the adapter
MAY register a transport-local spelling of it, and SHALL translate that spelling
back to the Pipeline's real name before anything leaves the adapter.

The mapping SHALL be confined to the adapter. No component outside it — and
nothing the manager stores, publishes or records — SHALL see the alternate
spelling.

The adapter SHALL present the same spelling everywhere it names that Pipeline to
a person on that transport, so what the menu completes and what a listing prints
are the same string.

The mapping SHALL be INJECTIVE BY CONSTRUCTION rather than by detection. Only
characters that cannot occur in a Kubernetes object name may be introduced, so
the reverse is a pure function of the string and two Pipelines can never share
one spelling. An entry the mapping cannot render injectively SHALL simply not be
registered — it stays typable, and nothing is refused, reported or conditioned
on it.

#### Scenario: A hyphenated pipeline autocompletes
- **WHEN** a Pipeline whose name Telegram rejects as a command is published
- **THEN** the adapter registers a spelling Telegram accepts, and completing it
  starts a conversation on that Pipeline

#### Scenario: The alternate spelling never escapes the adapter
- **WHEN** a conversation is started through the completed form
- **THEN** the signal, the conversation and every record of it name the
  Pipeline exactly as the manager published it

#### Scenario: The listing and the menu agree
- **WHEN** the adapter posts a listing naming Pipelines
- **THEN** each is named in the same spelling the menu completes

#### Scenario: Two Pipelines can never share a spelling
- **WHEN** any two Pipelines are published
- **THEN** their transport-local spellings differ, because the mapping only
  introduces characters a Kubernetes name cannot contain

#### Scenario: The real name still works
- **WHEN** a person types the Pipeline's real name as a command
- **THEN** it is routed as before, whether or not a spelling was registered

### Requirement: The adapter re-registers only when its own view changes
The adapter SHALL refetch the vocabulary when the revision it observes differs
from the one it last fetched, and SHALL call Telegram only when the ADAPTED
result differs from what it last registered.

Registration is rate-limited by the transport, so a vocabulary change that
produces an identical registered list SHALL produce no transport call.

#### Scenario: An inconsequential change causes no registration
- **WHEN** the vocabulary changes in a way that does not alter the adapted list
- **THEN** the adapter refetches and makes no Telegram registration call

#### Scenario: Startup registers once
- **WHEN** the adapter starts and reads the vocabulary
- **THEN** it registers once per served chat and does not repeat while the
  adapted list is unchanged

### Requirement: The adapter parses the grammar and folds the detail

The adapter SHALL parse the block grammar out of an agent-reported body and
render it: the title first, named sections labelled and in order, and the folded
region using the transport's own collapsed presentation.

It parses the body itself. Nothing upstream has, and the tags reach it exactly as
the agent wrote them.

Chunking SHALL prefer BLOCK boundaries. Because the above-the-fold content is
bounded by the manager, the FIRST chunk of a split message SHALL contain the
title and the named sections — a reader who sees only one message still sees the
conclusion.

A body with no recognized tag SHALL render exactly as today. That is the whole
backward-compatibility story on this surface: untagged prose parses to one block
and looks unchanged.

A `signal` SHALL NOT be parsed. Its structured fields render as a CARD, and its
payload is a machine document — quoted, and folded once tall enough to dominate
the thread.

#### Scenario: The detail arrives collapsed

- **WHEN** an answer carries a folded region
- **THEN** it is posted collapsed and the reader expands it in place

#### Scenario: The conclusion leads the first chunk

- **WHEN** a long answer must be split across several messages
- **THEN** the first message carries the title and the named sections, and the
  fold's content follows

#### Scenario: An unstructured message is unchanged

- **WHEN** a notice or a relay arrives with prose carrying no tags
- **THEN** it renders as it does today

#### Scenario: A signal card is not prose

- **WHEN** a signal arrives whose payload happens to contain a tag-shaped line
- **THEN** the card renders from the signal's fields and the payload is quoted
  verbatim, with nothing folded by the grammar

### Requirement: The adapter renders choices as inline controls
The adapter SHALL render a message's `choices` as controls attached to that
message, not as controls attached to the chat. A chat-wide control is shown to
every member of a shared surface and replaces their own composer, which is not
acceptable on an operations chat several people read.

Selecting a choice offered in answer to a message somebody wrote SHALL send that
original message to the chosen Pipeline. The adapter SHALL recover the original
from the transport's own reply linkage rather than from state held for the
purpose, so nothing is retained between the offer and the selection.

#### Scenario: Choices attach to the message
- **WHEN** a message carrying choices is posted
- **THEN** the controls appear on that message and no member's composer is
  altered

#### Scenario: One selection sends the original message
- **WHEN** a person selects a Pipeline offered in answer to their ambiguous
  message
- **THEN** that message is delivered to the chosen Pipeline without being
  retyped

#### Scenario: Nothing is held between offer and selection
- **WHEN** the adapter restarts between posting the offer and a person selecting
  from it
- **THEN** the selection still works, because the original is recovered from the
  transport

#### Scenario: An expired offer refuses rather than misfires
- **WHEN** the original message can no longer be recovered from the transport
- **THEN** the person is told to send the addressed form, and nothing is
  delivered to any Pipeline
