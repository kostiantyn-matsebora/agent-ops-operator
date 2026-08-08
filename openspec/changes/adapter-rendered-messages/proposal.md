# Adapters render; the manager sends semantic messages — and conversations open with their signal

## Why

The manager claims transport neutrality and does not have it. `internal/chat/router.go`
opens with *"Transport-neutral inbound routing"* and then emits Telegram's
dialect:

```go
r.Ops.EnqueueSend(ctx, ch, nil, "🤖 <b>Agents</b>: "+...+
    "\nUsage: /&lt;agent&gt; &lt;task&gt; — each call gets its own topic.")
```

`<b>`, `&lt;`, and the contract's own wording — *"post a message; chat HTML
subset"* — are one transport's format sitting in the neutral core. Every other
surface has to un-parse it, and every presentation decision (length, escaping,
markup) is made once, centrally, by the component that knows the least about
where the message is going.

That leak stays cheap only while messages are short. Two things make it
expensive:

- **A conversation never shows the event that started it.** An alert thread is
  a topic title, then silence, then the agent's interpretation. The raw signal
  lives in a `ConversationInput` CR the human would have to `kubectl get` to
  read. If the agent hangs or dies, the thread never says what happened.
- **Posting the signal is impossible under the current contract.**
  `channel-telegram/telegram.go:75` sends `text` straight to `sendMessage` with
  no chunking and no length check — Telegram caps at 4096 — and a payload
  containing `<`, `>`, or `&` breaks HTML parsing. Any manager-side fix would
  be the manager guessing at one transport's limits on behalf of all of them.

Both are the same fault. Fixing presentation ownership is the prerequisite for
fixing the missing context, so they are one change.

## What Changes

- **`send` ops carry a typed message, not a pre-rendered string — BREAKING.**
  Four kinds cover everything the manager ever says:

  ```
  signal   {pipeline, source, title, labels{}, body, inputRef}
                                                the event, as it arrived, and where from
  answer   {body, status}                       agent output via /work/done
  relay    {origin, sender, body}               a user message mirrored to sibling channels
  notice   {level, body}                        acks, guidance, errors, listings
  ```

  Free-text fields are **markdown** — a transport-neutral lingua franca;
  structured fields stay typed so an adapter can render a label table, a
  collapsible, or an attachment.
- **`ensure-topic` carries a topic descriptor**, not a baked title:
  `{conversation, pipeline, source, title, labels{}, kind}`. The adapter names
  the topic from its own template and enforces its own limits (Telegram forum
  topics cap at 128 characters) — so a topic can be named `[vm-alerts]
  HighMemoryUsage` without the manager deciding that format.
- **Conversations record their provenance, because today they cannot.** No
  `pipelineRef` exists anywhere in `api/v1alpha1`, and the originating source
  name survives only by accident — `signals.go:351` sets `InputItem.JobName`
  to the source name for `kind: job` and drops it for alerts. A card cannot
  name what the conversation never recorded, so:
  - `ConversationSpec` gains `pipelineRef` — the route that originated it,
    snapshotted like `profileRef` and `channelRefs`;
  - `InputItem` gains a typed `origin` (`{kind: signal|api|channel, name}`),
    generalizing the accidental `JobName` into provenance that every input
    carries.

  This also makes the posting rule computable from the input itself rather than
  inferred from its type.
- **Rendering, escaping, chunking, and truncation move to adapters, entirely.**
  The manager stops guaranteeing anything about how a message looks. Telegram
  renders HTML and splits at 4096; a web chat renders a card; each does what its
  surface can.
- **Conversations open with the input that started them.** The rule is general:
  **post every input the channel did not originate.**

  | input | origin | posted |
  |---|---|---|
  | `alert`, `job`, `recurrence` | signal source | yes |
  | `task` via `POST /task` | API | yes — today such a topic appears with no explanation |
  | `chat`, `reply` | the channel itself | no — echo; `relayToSiblings` already mirrors those |

  The card is enqueued once the thread binding exists, in parallel with
  dispatch — the human reads the event while the agent is still working.
- **Op ids for input cards are stable** (`input:<conversation>:<inputID>:<channel>`),
  so reconcile-driven re-enqueues dedup exactly as `ensure-topic` already does.
- **Adapters declare the contract version they speak.** An adapter built against
  the string-valued contract fails loudly rather than silently posting empty
  messages — the same posture as the retired `?type=` parameter returning 400.

Not in scope: changing what the agent writes (`format.md`'s six templates
still govern the agent's own output, which becomes an `answer` body); a fetch
endpoint for out-of-line payloads (the payload is inlined, see design);
progress or lifecycle events beyond inputs; any change to grouping, cooldown, or
dispatch.

## Capabilities

### New Capabilities
- `adapter-rendered-messages`: the typed outbound message contract, the topic
  descriptor, markdown as the neutral text format, and the relocation of
  escaping/length/presentation to adapters.
- `conversation-opens-with-its-input`: which inputs are posted to bound
  channels, when, and with what dedup — so a thread reads as the event followed
  by the work.

### Modified Capabilities
- `channel-adapter-contract`: outbound ops carry a kind-specific *structured*
  payload instead of message text and a title string.
- `multi-channel-conversations`: fan-out delivers one semantic message per bound
  channel, each rendered independently by its adapter — the same answer can look
  different on Telegram and on the web.
- `telegram-channel-adapter`: the reference adapter gains the renderer — HTML
  composition, 4096-char splitting, topic-name templating, and the option to
  attach oversized payloads as documents.

## Impact

- **`api/v1alpha1/conversation_types.go`**: `ConversationSpec.PipelineRef` and
  `InputItem.Origin`; `JobName` folds into `Origin`. Regenerates deepcopy and
  the Conversation CRD.
- **`internal/chat/ops.go`**: `Op.Text`/`Op.Title` become structured message and
  topic-descriptor fields; `EnqueueSend` splits into per-kind constructors. FIFO,
  dedup, claim/complete, and `ReclaimAfter` are untouched.
- **`internal/chat/router.go`**: every hand-written HTML string becomes a
  `notice`; `relayToSiblings` emits `relay` instead of composing
  `💬 <b>origin</b>: …`.
- **`internal/httpapi/server.go:327`**: `/work/done` fan-out emits `answer`.
- **`internal/httpapi/signals.go`**: enqueues the `signal` card after the thread
  binding lands.
- **`internal/chat/provider.go`**: in-process registry providers render too —
  they consume the same ops.
- **`channel-telegram/`**: gains a renderer package; `Send` grows chunking and
  escaping; `CreateTopic` renders its name from the descriptor.
- **Docs**: the contract description in README/`docs/` stops saying "chat HTML
  subset"; `CLAUDE.md` gains "the manager composes meaning, adapters compose
  presentation" as an invariant.
- **Migration**: manager and adapters upgrade together. Third-party adapters
  break by design and fail loudly on the version check.
- **Interacts with `chat-signal-origination`**: chat messages become signals
  there, and this change's "don't post what the channel originated" rule is what
  keeps them from echoing. Land the rule with whichever ships second.
- **Overlaps `pipeline-addressed-conversations`** (in flight): it also makes a
  conversation address its Pipeline. Whichever lands first should add
  `ConversationSpec.PipelineRef`; the other consumes it. Adding it twice, or
  under two names, is the failure to avoid.
