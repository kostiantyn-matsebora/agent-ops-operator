## Context

See `proposal.md` — Why. Four constraints shape everything below.

- **Adapters hold no Kubernetes access.** `channel-telegram` cannot read a
  Pipeline. Only the manager can say what is addressable.
- **The adapter contract is pull-only.** Adapters need not be addressable — a
  `ChannelAdapter`'s `spec.port` is optional — so the manager cannot dial one.
  Every "push" must ride a connection the adapter already holds.
- **Adapters are separate, dependency-free Go modules.** `console/` and
  `channel-telegram/` cannot import the operator's `internal/`, so shared
  behaviour has to travel over the wire, not through a package.
- **The manager composes meaning, adapters compose presentation.** No transport
  dialect under `internal/` — which is what forces the vocabulary to be
  published unfiltered.

## Goals / Non-Goals

**Goals**

- One derivation of "what may be typed", consumed by every surface.
- A change reaches an idle adapter without a new connection, a timer, or the
  manager dialling anything.
- Purely additive on the wire: contract version stays `2`.

**Non-Goals**

- Completing anything that lives inside a profile's repository. Only the runtime
  holds the checkout.
- Per-person or per-permission vocabularies. Everyone on a surface sees the
  same list, as they do today.
- Any CRD field, chart change, or new adapter permission.

## Decisions

### D1. Vocabulary is a contract endpoint, not a `/channel/list` field

`GET /channel/vocabulary` returns `{revision, entries[]}`. Entries are one flat
list with a `kind` discriminator (`builtin` | `pipeline`), each carrying `name`,
`description`, `position` (`general` | `thread`), and for a Pipeline the
answering `profile`.

*Why not nest it in `/channel/list`?* That endpoint is per-channel and the
vocabulary is namespace-wide — addressing resolves a Pipeline by name with no
claim check, so every Ready Pipeline is reachable from every wired surface.
Nesting a global fact under each channel would duplicate it N times and invite
someone to make it per-channel later.

*Why one flat list rather than `builtins[]` and `pipelines[]`?* An adapter
filters by properties (`position`, name legality), not by which array a thing
came from. Two arrays would make every consumer concatenate them first.

### D2. The revision is derived, and travels on the ops long-poll

Revision = a hash over the sorted published entries. Nothing is stored, so two
managers agree and a restart changes nothing — it satisfies `state-durability`
as "derivable from Kubernetes objects" without adding a row to that matrix.

It is carried as a response **header** on `GET /channel/ops`, on both the `200`
and the existing `204`. A header rather than a body field because the empty case
is a bodyless `204` today and giving it a body would change what every existing
adapter parses.

*Alternatives considered.* A new **op kind** on the OpQueue: rejected because
every existing op is conversation-shaped, needs a stable dedup id, and carries
retry/reclaim machinery that a cache invalidation does not want. The one thing
it would buy — durable reporting when registration fails — is already served by
the Channel Ready condition, which is where adapters report their own problems.
A **dedicated long-poll** or SSE stream: a second connection per adapter for
something that changes a few times a day.

Convergence is the long-poll wait (~25s idle, immediate under traffic).

### D3. Telegram registers Pipelines too, under a spelling it translates back

Telegram command names are `[a-z0-9_]{1,32}` — no hyphens — and every Pipeline
this project ships is hyphenated. That does **not** mean Pipelines cannot be
completed, only that the string Telegram completes is not the string the manager
publishes.

`channel-telegram` registers `k8s_observe` and maps it back to `k8s-observe`
before anything leaves the adapter. The manager never sees the alternate
spelling; the Pipeline CR is untouched; no other component knows it exists.
Registering only the built-ins would teach a person that Pipelines are not
something you type.

Three local rules, all in the adapter:

- **Reversibility.** If two Pipelines collapse to one spelling, neither is
  registered — an ambiguous completion is worse than one that must be typed.
- **One spelling per surface.** Whatever the menu completes is what the adapter
  prints when it names that Pipeline, so the listing and the menu never show two
  strings for one thing.
- **Overflow.** Telegram caps the list at 100 and names at 32 characters.
  Built-ins first, then Pipelines by name; anything over the cap or too long is
  unregistered and still typable.

*Alternatives rejected:* a derived name in the MANAGER — two names for one
object, and `/pipelines` would print a different string from the menu; a
`Pipeline.spec.commandName` field — same drift plus a CRD change; renaming the
shipped Pipelines — underscores read wrong beside every other Kubernetes object,
and it is a breaking change for a cosmetic gain. Keeping the map inside the
adapter is the only option that leaves one name in the API and still completes.

### D4. Registration is per chat scope, and only when the adapted list changes

`setMyCommands` with `BotCommandScopeChat` per served channel, rather than
`BotCommandScopeDefault` once, so the bot does not claim a command vocabulary in
chats it does not serve.

The adapter compares the **adapted** list against what it last registered. A
Pipeline going Ready therefore changes the revision, causes a refetch, and makes
no Telegram call at all — which matters because registration is rate-limited and
Pipeline edits are the common change.

### D5. Choices are a structured message field; buttons are inline, never chat-wide

`Message` gains `choices[]`, each `{label, command}` — beside `labels`, same
category: the manager says what is on offer, not how it looks.

Telegram renders them as an **inline keyboard on the message**. A reply keyboard
was rejected outright: it is shown to *every member* of a group and replaces
their composer, which is unacceptable on a shared operations chat.

Inline keyboards mean `callback_query`, which means the router change (D7).

`callback_data` is capped at 64 bytes and carries the Pipeline name. A name that
does not fit is rendered as text instead of a control — an adapter-local
degradation, consistent with `choices` degrading to a list everywhere else.

### D6. One tap re-sends the original message, statelessly, via `inReplyTo`

The ambiguity refusal is the moment worth optimising: the person has already
typed their task. Tapping a Pipeline must send *that* text, not open a dialogue
asking for it again.

The mechanism: the refusal is posted **as a reply** to the person's message, so
`callback_query.message.reply_to_message.text` hands the original back at tap
time. Nothing is retained between the offer and the selection — the adapter can
restart in between and the tap still works.

For the adapter to reply to the right message, the manager must carry an opaque
handle. So:

- `signal-telegram` adds one label beside `agentops.dev/channel` and
  `agentops.dev/sender`, carrying the transport's message handle as an opaque
  string.
- `Message` gains `inReplyTo`, an **opaque** transport handle — the same
  category as `threadId`, which the manager already stores and never interprets.
  "This answers that message" is meaning, not presentation.
- `channel-telegram` uses it as `reply_to_message_id`.

*Alternatives considered.* Putting the original text in `callback_data`: 64
bytes. A two-step `force_reply` prompt: makes the person type the task twice,
which is the cost this exists to remove. An adapter-side buffer of recent
messages: state, in a component whose whole design is to hold none.

### D7. The router acknowledges taps; classification does not fork

`allowed_updates` gains `callback_query`. The existing binary rule survives
unchanged — `is_topic_message` is read from `callback_query.message` instead of
`message`, so a tap on the general surface is an origination and a tap in a
topic is a continuation, by the same rule.

The **router** sends `answerCallbackQuery`, unconditionally and with no content,
before forwarding. Two reasons: the client spins until something answers, and
the router is the only component that always holds the token —
`signal-telegram` deliberately holds no credentials, and giving it one to stop a
spinner would undo the reason it has none. An unconditional, content-free ack is
stream hygiene, like the offset the router already delegates.

*Alternative rejected:* fanning one update to both adapters so the channel
adapter can ack. It makes classification non-binary and gives two components a
share of one update.

### D8. The console consumes the same endpoint it could have derived locally

The console *can* read Pipelines — it watches them for its topology and config
views. It will nonetheless fetch `/channel/vocabulary` over the channel contract
it already speaks, and re-serve it to its SPA.

*Why not derive locally?* The spec requires the console typeahead and the
Telegram menu never to disagree, and `console/` cannot import the manager's
derivation — it is a separate module by design. Two independent derivations of
one fact is exactly the drift this change exists to remove. The side benefit is
that the console proves the endpoint on a second, non-Telegram adapter.

`/api/agents` becomes a projection of the vocabulary rather than a second walk
over the Pipeline cache.

### D9. Position is honoured where a transport can express it, and not otherwise

The console has two composers and offers a different list in each. Telegram's
finest scope is `BotCommandScopeChat` — there is no per-topic scope — so it
registers the union and relies on the existing usage warnings for a command used
in the wrong place. Those warnings are already written and already good; the
menu offers, the warnings correct.

### D10. The listing command is renamed, and the old name goes quiet

`/pipelines` replaces `/agents`. `/agents` keeps working — it is published in
installs already and a dead command is a bad way to learn about a rename — but
it is never registered with a transport, never offered in a typeahead, and never
printed in a reply. It joins `exit`, `close`, `help` and `start` in the reserved
set a Pipeline cannot shadow, and so does `pipelines`.

*Why now, rather than as its own tidy-up?* This change is what makes the word
permanent. `setMyCommands` carves it into the composer of every install, and a
typeahead repeats it every time somebody types `/`. Publishing the wrong word
into a menu is much harder to walk back than changing it before the menu exists.

*Why it is the wrong word.* "Agent" already names a definition in
`.claude/agents/` inside the profile's repo. The listing lists Pipelines. The
current reply manages to use "agent" for the Pipeline and "role" for the agent
in a single sentence, which is the tell.

### D11. The addressed form loses its second segment

`addressing.Parse` stops capturing `:<agent>`, `Command.Agent` goes, and
`dispatch` reads the agent from the profile alone.

A Pipeline names one profile and a profile names one agent, so the agent is
already fully determined by the wiring — exactly as the toolsets and the MCP
servers are. The override let whoever typed it pick a different one, which is
the same shape as a caller naming its own Pipeline over HTTP. `pipeline-model`
already forbids that in so many words.

`InputItem.agent` is **deprecated, not deleted**: nothing writes it, and
`dispatch` keeps reading it for one release so inputs already queued when the
manager restarts still dispatch to the agent they were parsed with. Same posture
as the retired `sessionId` dual-read. Removing the field is a later change.

Text after the Pipeline name is simply the task. `/k8s-observe check:the pods`
is a task containing a colon, which it always should have been.

## Risks / Trade-offs

| Risk | Mitigation |
|---|---|
| **Telegram's menu shows `/exit` and `/close` in the main chat, where they do nothing.** No per-topic scope exists. | The existing usage replies (`router.go`) already answer both by name and say where they belong. Registered descriptions say "inside a conversation's thread". |
| **`/agents` muscle memory** in installs that already use it. | It keeps working, unchanged and forever-ish. It is simply never offered, registered or printed, so nobody learns it from us again. |
| **Two spellings for one Pipeline on Telegram** — the menu completes `k8s_observe`, the CR says `k8s-observe`. | The adapter prints one spelling everywhere it names a Pipeline on that transport, and the real name keeps working. The alternate never reaches a signal, a conversation or a record. |
| **A collision leaves two Pipelines uncompleted** with no obvious cause. | The adapter reports it on the Channel Ready condition, where adapters already report their own problems. |
| **`inReplyTo` is a transport handle in a semantic message** — the shape the message contract exists to prevent. | It is opaque and never interpreted, exactly as `threadId` and `previousThreadId` already are. Pinned by a test asserting no `internal/` code parses it. |
| **`setMyCommands` rate limits** if re-registration is chatty. | Compare the FILTERED list (D4): Pipeline churn, the common case, causes zero calls. |
| **Revision churn** if the hash includes anything volatile. | Hash only `(kind, name, description, position)` sorted — no timestamps, no resource versions, no conditions beyond Ready. Pinned by a test that mutates an unrelated Pipeline field and asserts the revision is unchanged. |
| **An adapter refetches on a revision it cannot act on** (Pipeline-only change). | One cheap GET, then no transport call. Acceptable; the alternative is a per-adapter filtered revision, which puts transport knowledge in the manager. |
| **A stale offer**: a tap on an old message whose original is gone. | The adapter answers with the addressed form to type and delivers nothing — specified as a scenario. |
| **Console screenshot drift**: the site's console images are build output. | `npm run screenshots` is a task, not a follow-up. |

## Migration Plan

Nothing to migrate. Every wire addition is optional and the contract version is
unchanged, so the rollout order is free:

1. Manager first. Older adapters ignore the header and the endpoint; behaviour
   is byte-for-byte what it is today.
2. Adapters in any order. An un-upgraded `channel-telegram` beside an upgraded
   manager registers nothing and renders `choices` not at all — which is the
   pre-change experience, not a broken one.
3. `telegram-router` last. Until it widens `allowed_updates`, buttons render and
   taps do nothing visible — so ship the router with or before the adapter that
   starts drawing them.

Rollback is a manager downgrade: the header stops appearing, adapters keep their
last-registered command list until they restart, and nothing errors.

## Open Questions

- **Does the listing keep its prose once its Pipelines are also controls?**
  Keeping both duplicates the information on Telegram; dropping the prose leaves
  a bare control grid on a transport without them. Leaning toward keeping the
  prose and letting `choices` add controls beside it — decidable during
  implementation without touching the specs.
- **Description text for the built-ins.** They are user-visible strings in the
  Telegram menu and want editing against the real UI, not the spec.
- **How long `/agents` stays.** It costs one line, so there is no pressure to
  pick a removal release now.
