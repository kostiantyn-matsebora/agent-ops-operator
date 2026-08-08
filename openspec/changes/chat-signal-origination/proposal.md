# Conversations originate only from signal sources; chat ingest splits behind a router

## Why

A Conversation can be born two ways today, through two unrelated mechanisms. A
signal goes through `/signal/inbound` → claim check → fingerprint cooldown →
signature grouping → window reuse → typed input lane, with `Wired=False` and
`receivedTotal` making it observable. A chat message goes through
`/channel/inbound` → `Router.HandleMessage` → straight to `Create`, with none
of that.

The seam shows in the code. `PipelineForChannel` picks the **oldest Ready
pipeline** referencing a channel to decide who answers a bare message — a
creation-timestamp tiebreak. Forty lines away, `CapabilityPipelineForProfile`
refuses to do the same thing, with the comment *"whichever was created first is
not a defensible answer."* That tiebreak exists only because a channel can
originate a conversation without anything having claimed it for that purpose,
while channels are deliberately shareable across pipelines.

The product has one workflow — **a signal arrives from a source, then the
conversation is carried on channels**. Chat origination is the single exception
to it. Removing the exception makes the model uniform instead of patching the
tiebreak.

## What Changes

- **Origination becomes exclusively a signal-source concern — BREAKING.** A
  message on a channel's general surface (no topic) is a *signal*, normalized
  and routed by the same core that routes alerts and cron jobs. The Pipeline
  names the chat `SignalSource` explicitly, so who answers is declared rather
  than inferred from creation timestamps.
- **Channel responsibility is unchanged otherwise.** Topic messages, replies,
  acks, sibling relay, thread bindings, mirroring, and delivery all stay exactly
  as they are. The Channel is where the conversation lives; it just no longer
  starts one.
- **Telegram ingest splits into three components** behind a router, because
  Telegram serves exactly one update stream per bot token (a second concurrent
  `getUpdates` gets `409`, and confirming an offset destructively consumes
  updates for every reader):

  ```
  Telegram getUpdates ─▶ telegram-router ─┬─ no topic  ─▶ signal-telegram  ─▶ /signal/inbound
                          (only component │              (SignalAdapter)
                           that knows      └─ topic     ─▶ channel-telegram ─▶ /channel/inbound
                           IsTopicMessage)                (ChannelAdapter, also runs the ops loop)
  ```

  The classification is local and needs no manager state — `IsTopicMessage` is
  already on the update and already drives `threadID` in
  `channel-telegram/main.go:352`.
- **No transport knowledge enters the manager.** It keeps seeing two generic
  contracts, exactly as it does for cron and Alertmanager. The telegram-specific
  origination-vs-continuation rule lives in the router.
- **`channel-telegram` stops polling** and instead receives forwarded topic
  messages; it keeps its ops loop, `createForumTopic`, `sendMessage`, chatId
  matching, approver filtering, and its `/channel/inbound` posting. Its
  `getUpdates` loop and offset handling move behind the router.
- **New module `signal-telegram/`** (dependency-free, mirroring `signal-cron/`):
  receives forwarded general-surface messages, normalizes them to signals, and
  pushes `/signal/inbound`.
- **New module `telegram-router/`**: owns the single `getUpdates` loop per token,
  classifies, forwards raw updates in-cluster. It holds no channel config —
  downstream adapters keep doing chatId matching and approver filtering — and
  delegates offset persistence rather than owning state.
- **`adoptThread` is removed.** A message in a topic the manager does not know
  is no longer an origination; origination happens only on the general surface.
- **`PipelineForChannel` is deleted**, and with it the oldest-Ready tiebreak.
- **Chat sources default to `cooldownHours: 0`** — fingerprint cooldown
  swallowing a human who repeats a request would be a bug, not dedup.
- **Chart**: the whole telegram stack renders under the existing
  `telegramAdapter.enabled` flag (default false) — the `ChannelAdapter`, the new
  `SignalAdapter`, the router's CR, and a sample Pipeline pairing the chat
  `SignalSource` with its `Channel`. Per the existing chart requirement, these
  are adapter CRs, not channel-type workload templates.

Accepted cost, chosen deliberately: three containers where there was one. The
gain is that the model has one origination path and each process has one job.

Not in scope: `POST /task` (a third origination path, with its own open question
in `capabilities-are-wiring`); porting other channel types; any change to the
ops queue, delivery, mirroring, or the reply path.

## Capabilities

### New Capabilities
- `chat-signal-origination`: general-surface chat messages are signals — the
  normalized chat signal shape, the claiming Pipeline as the declared answerer,
  chat-appropriate grouping defaults, and command responses that produce a reply
  without a conversation.
- `telegram-ingest-router`: the single-consumer split — one poller, local
  classification by topic presence, in-cluster forwarding, offset delegation,
  and the chart wiring of all three components under one flag.

### Modified Capabilities
- `pipeline-model`: a channel no longer supplies a default profile for bare
  messages; the Pipeline claiming the chat source does. The oldest-Ready inbound
  tiebreak is removed.
- `channel-adapter-contract`: `/channel/inbound` becomes reply-only — thread id
  required; the origination branch (command parsing, default-profile
  conversation creation) moves to the signal path.
- `telegram-channel-adapter`: the adapter no longer polls; the single-consumer
  requirement moves to the router and is restated across the three components.

## Impact

- **New modules** `telegram-router/` and `signal-telegram/`, each with a
  Dockerfile and image, mirroring the existing dependency-free adapter modules.
- **`channel-telegram/`**: `pollManager`, `pollToken`, `tokenGroup` removed;
  `dispatch` becomes an HTTP handler for forwarded updates; `GetUpdates` leaves
  `telegram.go`; needs a listen port to receive from the router.
- **`internal/chat/router.go`**: the three origination paths
  (bare text, `/<profile>`, `adoptThread`) and `defaultPipeline` are removed;
  `convByThread`, `appendInput`, `FanOutSend`, `relayToSiblings` stay untouched.
- **`internal/chat/pipelines.go`**: `PipelineForChannel` deleted.
- **`internal/httpapi/signals.go`**: chat signal kind, command handling that
  emits ops instead of conversations, and the originating-surface labels.
- **`api/v1alpha1`**: `ChannelAdapter` likely needs `spec.port` parity with
  `SignalAdapter` so the reconciler can own the Service the router posts to.
- **Chart**: `telegramAdapter.enabled` now renders three adapter CRs; new
  images; a sample Pipeline claiming the chat source.
- **Migration**: existing installs add a chat `SignalSource` claimed by their
  Pipeline, and must carry the `getUpdates` offset annotation across to whatever
  owns it after the split. Old adapter must stop before the router starts — the
  single-consumer rule spans the cutover.
- **Interacts with `pipeline-addressed-conversations`** (applied): the
  capability-only Pipeline this once had to reason about is gone. Conversations
  address the Pipeline that originates them, `POST /task` names one, and
  capabilities are declared per Pipeline with no default. So chat addressing is
  this change's to define — `/<pipeline>` rather than `/<profile>` — and the
  chat SignalSource's claiming Pipeline supplies its conversations' capabilities
  like any other route.
