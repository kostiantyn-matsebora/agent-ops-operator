# The channel adapter contract

**A channel adapter dials the manager, never the reverse** — the same pattern as
runtimes. NetworkPolicies stay simple and transport credentials never leave the
adapter.

1. **Long-poll for outbound operations.**
   `GET /channel/ops?adapter=<your-adapter>&contract=2&wait=25`

   | op | Does |
   |---|---|
   | `ensure-topic` | create a thread for a conversation |
   | `send` | post a message |
   | `close-topic` | archive the thread in `op.threadId` — its conversation has ended |
   | `delete-conversation` | tell the thread in `op.threadId` its conversation is gone for good |

   **Delivery is at-least-once.** Dedup by `op.id`.

   **`contract` is required.** An absent or outdated version is refused with 400
   naming the current one. Version 1 carried rendered `text`, and an adapter
   still reading that field would post empty messages and look healthy doing it,
   so the handshake fails at the door rather than downstream.

   **The block grammar did NOT move it.** No field was added and none changed
   meaning — a body that was markdown is now markdown plus a grammar, read by
   the component that already read the markdown. See [The body
   grammar](#the-body-grammar).

   **A claimed op also carries `reclaimAfterSeconds`** — how long the manager
   leaves the claim with you before returning the op to the queue. It is
   additive and optional: ignore it and nothing changes. Honour it if you absorb
   transport backpressure in-process (below), because finishing after it expires
   means a second claimant posts the same message again.

   **A `503` means this replica cannot safely answer the poll.** It is not the
   manager's current leader, and only the leader claims an op (multiple manager
   pods share one `/channel/ops`).

   **The claim is recorded on the Conversation** as `status.threads[].claim`
   (`holder`, `claimedAt`), written only by the leader. A claim older than
   `claimStalenessSeconds` is cleared and the op dispatched again. A claim held
   by a former leader is cleared at once.

   **Retry at once**, exactly as you already do on an empty `204`, and never
   surface it as an error. The claim may well exist, this replica simply cannot
   see it.

   **Every response carries `X-Agentops-Vocabulary-Revision`** — the delivered
   op, the empty `204` and the `503` alike. See [What may be
   typed](#what-may-be-typed) below.

2. **Complete each op** with `POST /channel/ops/{id}/done`.

   | op | Body |
   |---|---|
   | `ensure-topic` | `{"threadId":"…"}` — an opaque string in your id space |
   | `close-topic` | **empty** |   | any, on failure | `{"error":"…"}` — surfaced as a Conversation condition and regenerated |

   **`close-topic` is the exception to that last clause, in both halves.** Its
   conversation is being deleted, so a failure is *logged* rather than written
   as a condition — there will be no object left to carry one — and the op is
   never regenerated.

   An adapter that does not implement the kind may complete it with an error or
   ignore it entirely. The visible consequence is one open thread for a
   conversation that no longer exists, closable by hand.

   **Deletion itself never waits longer than a 2-minute grace**, so a down
   adapter cannot wedge it. Treat an already-closed thread as success —
   redelivery is normal.

   While a `close-topic` op is outstanding the deleting Conversation is held by
   the `agentops.dev/close-topics` finalizer, which is what keeps every op
   derivable from CR state across a manager restart.

3. **Push user REPLIES** with `POST /channel/inbound`, body
   `{"channel","threadId","text"}`.

   - **`threadId` is REQUIRED.** This endpoint continues an existing
     conversation and never starts one. A message in a thread the manager does
     not know is dropped, not adopted.
   - **Delivery to the other bound surfaces and busy-acks happen
     manager-side.** A reply you push is delivered back to you only if your
     adapter declares `echoesOwnMessages: false`.
   - **Refused when the origin surface names the thread's own target
     conversation.** A message that would feed a conversation its own output
     back as input is the shape a self-feeding loop takes, and the manager
     refuses it with a reason rather than dispatching it.

   **To ORIGINATE**, your transport's general surface belongs to a chat
   `SignalSource`. Post to `/signal/inbound` (see `signals/telegram/`):

   ```json
   {"kind":"chat","fingerprint":"…","payload":"…",
    "labels":{"agentops.dev/channel":"…","agentops.dev/sender":"…",
              "agentops.dev/message":"…"}}
   ```

   The Pipeline claiming that source decides who answers, and command parsing
   (`/pipelines`, `/<pipeline> <task>`) happens there.

   `agentops.dev/message` is optional. It is your transport's own handle for the
   message that arrived. The manager stores it and hands it back on any reply,
   so a reply can say which message it answers.

4. **Read what may be typed** from `GET /channel/vocabulary`, if you offer a
   command menu or a typeahead.

5. **Read your channels** and their opaque `spec.config` from
   `GET /channel/channels?adapter=`.
   - Persist cursors — poll offsets and the like — via
     `GET/PUT /channel/state/{channel}/{key}`.
   - Report config problems via `POST /channel/channels/{name}/status`.

6. **Optionally act on a conversation your channel is bound to**, all with
   `{"channel":"…"}`:
   - `POST /channel/conversations/{name}/reopen`
   - `POST /channel/conversations/{name}/delete`
   - `POST /channel/conversations/{name}/reset-context`

7. **Optionally report how far your threads have been read** —
   `POST /channel/read` ([below](#post-channelread)).

### POST /channel/read

**Optional.** An adapter that never calls it stays fully conformant. Its threads
simply carry no watermark, which is inert for every surface that does not render
unreadness.

```jsonc
POST /channel/read
{"channel":"console",
 "reads":[{"threadId":"console-uid-abc","readAt":"2026-08-13T10:12:04Z",
           "reader":"sha256:9f2a…"}]}

200
{"results":[{"threadId":"console-uid-abc","outcome":"marked"}],
 "marked":1,"skipped":0,"failed":0}
```

The manager resolves each `threadId` to its conversation and writes the
watermark to `status.threads[].readAt`.

**The manager owns the write.** The adapter reports and issues no Kubernetes
write of its own, exactly as with `POST /channel/channels/{name}/status`.

**`threadId` rather than a conversation name, on purpose.** An adapter knows
thread ids, `/channel/inbound` already addresses a thread, and the mapping from
one to the other is the manager's to own.

Naming conversations here would hand adapters a Kubernetes identifier they have
no other reason to hold.

**`reader` is OPTIONAL and OPAQUE** — the adapter's own key for whoever read the
thread. The manager records that reader's watermark rather than the channel-wide
one.

Omit it and the behaviour is exactly what it was before the field existed. That
is what every adapter but the console does, since a Telegram topic is read or it
is not and there is nobody to attribute it to.

**Never send an identity in it.** The manager stores the value verbatim and
cannot tell a hash from an address, so a conversation would end up recording who
read it.

Compute a SALTED hash — an unsalted digest of a known address is confirmable by
anyone holding the CR. A key containing `@` is refused with 400.

| Rule | Behaviour |
|---|---|
| Auth and scope | The same adapter token and channel scope as every other `/channel/*` route — 401 unauthenticated, 403 for a channel served by another adapter |
| Bound | At most **50** entries, enforced by the manager. More is 400 and nothing is written. An empty list is 400 |
| Monotonic | A watermark at or before the stored one is `skipped` — no write, no error. Per READER when one is named, so one reader is never skipped because another is ahead |
| Bounded readers | At most 50 readers per binding, oldest watermark evicted first. An evicted reader falls back to the channel-wide mark |
| Clamped | A watermark ahead of the manager's clock is written as its `now` |
| Per entry | One `marked` / `skipped` / `failed` outcome per requested thread, with a reason for anything not marked, plus totals |
| Mixed batch | Still 200, and one bad entry never stops the rest — an unknown thread is `failed` and its neighbours are still marked |
| Rewind | `"rewind":true` sets the NAMED reader's own watermark to `readAt` even where that is earlier than what is stored — "mark unread". Still clamped to the manager's clock. Refused with no `reader`: there is no per-reader entry to rewind, and the channel-wide mark is never rewound, which would un-read the thread for every other reader too |

Entries resolving to the same conversation are grouped into ONE status patch,
and a report that would not advance the watermark writes nothing at all. So
re-opening a quiet conversation costs no API write.

A rewind is the one exception: it always writes, since moving the watermark
backward is its whole purpose.

See [Read state](../concepts.md#read-state-per-thread) for the field and the
backfill rule.

### Reach for `reopen`, `delete` and `reset-context`

**Reach for those verbs is the BINDING**, read from the conversation's
`spec.channelRefs` and never taken from the request. A surface may act on a
conversation whose bindings name its channel, and no other.

That is narrower than it sounds, and it is the amendment the archived-thread
case forces on "no remote close verb exists".

That rule protected a property rather than a syntax — *you may only end a
conversation you are part of*.

Holding a live thread was how membership was PROVEN, because posting `/close` on
a thread is only possible for a surface that has one.

A closed conversation has no thread, so that proof is unavailable and the
binding that put the thread there is the next-strongest.

**`delete` additionally refuses anything not already `Closed`**, with a reason
naming the missing step.

**It never closes first.** One call doing the irreversible thing to a
conversation that was still working, behind a confirmation that named only the
delete, is exactly what the refusal prevents.

Closing itself is still `/close` on a thread. There is no close endpoint.

### Backpressure is yours to absorb

**A transport that says "slow down" — HTTP 429, a `retry_after`, a token-bucket
rejection — is not an operation failure.**

- **Absorb it inside your claim window.** Wait the interval the transport
  stated, never a backoff you computed instead of one it gave you, and retry the
  same call.
- **Report the op failed only when the condition is terminal**, or when riding
  it out would overrun the claim.

The manager's recovery path is for operations that genuinely could not be
performed. Reporting a retryable condition as a failure makes that path
load-bearing for something you could have waited out, and every re-derivation
arrives into the same backpressure.

**Budget your total in-process wait strictly below `reclaimAfterSeconds`**, and
pace your own sending so bursts are spread rather than rejected.

**Pace before you CLAIM, not before you send.** Work you cannot yet deliver
should stay queued in the manager, where it is still derivable from CR state and
survives your restart.

### What may be typed

`GET /channel/vocabulary` returns everything a person may type on a chat
surface, and the revision identifying it.

```json
{"revision":"3f2a91c…",
 "entries":[
   {"kind":"builtin","name":"pipelines","description":"…","position":"general"},
   {"kind":"builtin","name":"exit","description":"…","position":"thread"},
   {"kind":"pipeline","name":"k8s-observe","description":"k8s-engineer",
    "position":"general","profile":"k8s-engineer"}]}
```

It exists because **an adapter holds no Kubernetes access**. You cannot read a
Pipeline, so the manager is the only thing that can tell you what is
addressable.

| field | what it is |
|---|---|
| `kind` | `builtin` for a manager command, `pipeline` for an addressable Pipeline |
| `position` | `general` starts a conversation, `thread` acts on one |
| `description` | menu text — for a Pipeline, the profile answering for it |

**The two positions take disjoint sets.** Addressing a Pipeline works only on a
general surface. `/exit` and `/close` work only inside a thread.

Offer only what applies where the person is typing, **if your transport can
express the difference**. If it cannot, offer the union — the manager answers a
command used in the wrong position with usage, and that is the correction.

**The list is UNFILTERED, and deciding what you can express is your job.**

Telegram command names admit no hyphen, so `channel-telegram` registers
`k8s_observe` and translates it back before anything leaves the adapter.

That rule lives in that adapter. Nothing here knows it, and no Pipeline was
renamed for it.

**Learn about changes from the long-poll, not a timer.** Every
`GET /channel/ops` response carries `X-Agentops-Vocabulary-Revision`, on the
`200` and the `204` both. A revision different from your last fetch means
refetch.

The manager cannot dial you — your port is optional and this contract is
pull-only — so the news rides the connection you are already blocked in.

**All of it is additive.** Ignore the endpoint and the header and you behave
exactly as an adapter built before either existed. The contract version stays
`2`.

### Commands the manager answers itself

These are handled before any Pipeline lookup. An adapter forwards them as
ordinary text and does not implement them.

| Command | Where | What it does |
|---|---|---|
| `/pipelines`, `/help`, `/start` | general surface | lists Ready pipelines and their profiles |
| `/<pipeline> <task>` | general surface | originates a conversation ([above](#the-channel-adapter-contract)) |
| `/close` | a conversation's thread | ends the conversation, archives the thread |
| `/exit` | a conversation's thread | releases the runtime pod, keeps the conversation |

- **`/agents` still works** and is never printed, offered or registered. It was
  the old name for `/pipelines`.
- **The addressed form is one segment.** Text after the Pipeline name is the
  task, colons included.

- **Both thread commands are intercepted on the REPLY path**, before the text
  could become an input.
- **Both answer with usage** when typed on a general surface where there is no
  conversation to act on.
- **`/exit` refuses while a run is inflight or input is queued** — see
  [concepts](../concepts.md#releasing-a-runtime-by-hand--exit).
- **A Pipeline whose name collides with any of these is not reachable by that
  command.**

### The manager composes meaning, the adapter composes presentation

**An op carries STRUCTURE, never rendered text.** There is no `op.text` and no
`op.title`.

`send` carries `op.message`, one of four kinds:

| kind | fields | what it is |
|---|---|---|
| `signal` | `pipeline?`, `source`, `title`, `labels{}`, `body`, `inputRef` | the event that opened or advanced the conversation, as it arrived |
| `answer` | `body`, `status` | agent output, reported through `/work/done` |
| `relay` | `origin`, `sender?`, `body` | a user message, from any surface but this one — including THIS surface's own users when the adapter declares `echoesOwnMessages: false` |
| `notice` | `level` (`info`/`warn`), `body` | the manager on its own behalf: acks, listings, refusals |

**Any kind MAY also carry `choices` and `inReplyTo`.** Both are optional and
both are structured, not prose.

| field | what it is | what you do |
|---|---|---|
| `choices[]` | offered actions: `label`, `command` | render controls if your transport has them. Otherwise the body already names each one, so nothing is lost |
| `inReplyTo` | your OWN handle for the message this answers, as you supplied it in `agentops.dev/message` | reply to that message, or ignore it |
| `expectsReply` | this message ASKS the reader for something | open a reply box if you can, or do nothing — the prose says what to send |

`inReplyTo` is **opaque** to the manager. It stores and returns the string
unaltered and never parses it, exactly as it treats a thread id.

**`expectsReply` exists because a command menu SENDS on tap.** Somebody picking
`/k8s-ops` from a transport's own list posts it bare, with nowhere to type the
task — so the manager asks for one instead of answering with usage.

The answer finds its way back through the reply chain, and nothing is stored:

```
  the reply          →  the manager's question  →  the bare command
                            (inReplyTo)              (the Pipeline)
```

An adapter that cannot open a reply box is still conformant. The prose names the
Pipeline and says what to send.

Together they are what lets a control offered on somebody's own words carry
those words forward. Post the offer as a reply to their message and your
transport still holds the text, so a selection needs no state on either side.

**Never drop `choices`.** They are the reader's only account of what is on
offer. A transport without controls renders them as a list.

### The body grammar

**A free-text body is markdown in the named subset, PLUS a block grammar.** You
read both. That is how this contract has always worked for prose: it names a
language, and each surface renders what it can.

**The manager does not parse either one.** It hands you what the agent printed,
character for character.

#### The grammar

A tag is recognized only when **all three** hold:

1. it stands alone on its own line, at the start of that line
2. it forms a well-formed open/close pair
3. it is not inside a fenced code block

```
<title>
Pod is looping
</title>

<root-cause>
OOM at 512Mi.
</root-cause>

<details>
Everything a reader only wants if they ask.
</details>
```

| Tag | Is |
|---|---|
| `<title>` | at most one, rendered FIRST wherever it appeared, a single line |
| `<details>` | **THE FOLD** — present it collapsed, expandable by the reader |
| anything else | a section the AGENT named, above the fold, in written order |

#### Rules your parser must follow

- **Anything failing the three conditions is LITERAL TEXT.** Agent output is
  full of `<` — `if x < y`, `Deployment<T>`, shell redirects. None of it is a
  tag.
- **Parsing is TOTAL.** No recognized tag yields one block holding the whole
  text, which renders exactly as prose does today. That is the entire
  backward-compatibility story.
- **An unpaired opening tag closes at end of output.** Never discard the region
  — losing an agent's words to a grammar slip is the worst failure available.
- **A tag inside an open region is literal.** The model is flat.
- **The section vocabulary is OPEN.** Every agent names its own sections for its
  own job — `root-cause`, `changed`, `what-i-checked`. Render a label
  generically. **An adapter carrying a list of section names is wrong**, and
  will be wrong again the next time somebody writes a profile.
- **Only the fold is closed**, which is all you need to know where to collapse.
- **Never reorder named sections** and never shorten anything. Which part is the
  summary is the agent's judgement, already made by what it put in `<details>`.

#### Which bodies carry it

| kind | Parse it? |
|---|---|
| `answer` | **yes** — agent output by definition |
| `notice` | **yes** — a failed run that explained itself leaves as one, and that explanation is the longest thing an agent produces. Manager-composed prose parses to one block, same result |
| `relay` | **no** — somebody's typed words. Parsing them consumes characters a person deliberately wrote |
| `signal` | **no** — not prose at all, see below |

**A `signal` is a CARD, not a body.** Its structured fields *are* the message:
title, source, pipeline, labels, payload.

Its payload is a machine document or somebody's typed text, never this grammar.

**That is the one place you need a second renderer.** Keeping the two apart is
what lets a card show a source and a label table while an answer shows a title
and a fold.

#### If you do not implement it

You render the tags literally, which is ugly.

That is prevented upstream. `AgentProfile.spec.outputFormat` is **required**,
and a profile declaring `none` emits no tags at all — so an install serves the
grammar only to surfaces that understand it.

**The compatibility boundary is the profile, not the wire.**

**`ensure-topic` carries `op.topic`, a descriptor** — `conversation`,
`pipeline?`, `source?`, `title`, `labels{}`, `kind` — and YOU name the thread
from it.

The manager sends no baked title because it cannot know your limits. Telegram
forum topics cap at 128 characters and take no markup. A web chat has neither
constraint.

The descriptor and the first card in the thread carry the same facts, so an
adapter can choose not to repeat itself.

**`pipeline` may be EMPTY on both.** It is inferred from the conversation's
materialized bindings and left blank when several Ready Pipelines could claim
it. Blank means "not determinable", so render it as absent rather than inventing
a name.

**`delete-conversation` carries the target thread id and a `notice`**, and
reports that the conversation is **gone for good** — not that a thread should be
closed.

What ending means for your transport is YOUR decision: post the tombstone,
archive the thread, rename it, delete it.

It is named for the conversation because that is what ended. `ensure-topic` and
`close-topic` instruct you about a thread, this one informs you about a
lifecycle event.

**It REPLACES `close-topic` on the deletion path** — a conversation being
deleted gets one or the other, never both — so you never have to work out
whether a pair means one ending or two.

- **Do not silently ignore it.** An adapter that cannot act should complete it
  with an error.
- **Not implementing the kind at all is still correct.** The operation is
  reported failed and the deletion proceeds.

`channel-telegram` un-archives the topic, posts the notice and closes it again,
because a closed forum topic refuses `sendMessage` and an open one would invite
replies into a conversation that no longer exists.

It does not delete the topic: the history above the tombstone is what a person
scrolls back to.

**`previousThreadId` is an optional hint**, present only when a closed
conversation is being reopened: the thread this conversation used before its
topics were archived. **Ignoring it is a valid implementation.**

| Your transport | Do |
|---|---|
| can un-archive | honour the hint and return that same id — the conversation continues where it left off, with its history above the new messages |
| cannot | open a fresh thread and return the new id. The manager records whatever comes back |

**That asymmetry is why this is a hint on an existing operation rather than a
`reopen-topic` kind.** Most transports have no un-archive, so most
implementations of a new kind would be a second name for `ensure-topic`.

Whether un-archiving is possible is transport knowledge, and the manager holds
none.

**Prose fields are markdown**, in one deliberately small subset:

```
**bold**   *italic*   `inline code`   ```fenced code```   [text](url)
```

Anything outside that subset is UNDEFINED. You may render it, escape it or strip
it, and no caller may depend on which. The subset is small because the
alternative to naming it is every adapter inventing its own.

**Rendering, escaping, splitting and truncation are yours, entirely.** The
manager guarantees nothing about how a message looks or how long it may be.

`channels/telegram/render.go` is the reference: HTML composition, entity
escaping, 4096-character splitting, 128-character topic names. An adapter that
cannot be bothered may concatenate the fields and send them plain — the contract
asks only that presentation be the adapter's call.

### Credentials

**Declared per Channel** (`spec.credentialsSecretRef`, a Secret name) and
projected into the adapter pod by the ChannelAdapter reconciler as env vars.

- Every key `K` of the Secret appears as `<credentialEnvPrefix>K`.
- The prefix is advertised per channel in the `GET /channel/channels` listing.
  Key `botToken` becomes `AGENTOPS_CRED_HOME_OPS_botToken`.
- **The kubelet resolves the values.** Neither the manager nor any reconciler
  ever reads a Secret through the API.

Several Channels with different Secrets means several bots or workspaces through
one adapter process.

### Auth

**All `/channel/*` calls carry `Authorization: Bearer <token>`.**

| Token | Scope |
|---|---|
| Per-adapter, derived from the master key (`HMAC(ADAPTER_TOKEN, adapter name)`, validated statelessly by re-derivation) | **its own name** — the type key Channels select. Cross-key calls get 403 |
| The bare master token, chart-provisioned into the manager as env | full, so hand-deployed adapters work unchanged |

**No Kubernetes API access is needed.** The reference adapter
[`channels/telegram/`](../../channels/telegram/) is dependency-free Go.

### Discovering what `config` needs

An adapter CR may optionally declare:

- **`spec.configSchema`** — a JSON Schema for the `config` of the Channels and
  SignalSources it serves.
- **`spec.credentialKeys`** — the Secret keys it expects.

**The declaration lives on the CR spec**, so
`kubectl get channeladapter telegram -o yaml` answers "what do I write?" before
the adapter pod has ever started. No registration step, and adapter binaries
play no part.

The reconciler compile-checks a declared schema and reports `SchemaValid` on the
adapter CR. Served Channels and SignalSources then carry `ConfigValid`, with
`SchemaValidated` or `SchemaViolation` naming the offending fields.

**Both are advisory.** A violation never blocks serving, projection, or
ingestion — the adapter's own Ready report stays authoritative, because a
CR-declared schema can drift from the running image.

Declaring nothing keeps behavior exactly as before, and no `ConfigValid`
appears.

**Authoring rule:** bump the schema in the same diff as `image`.

**A `Channel` whose adapter nothing serves** — no in-process provider, no Ready
`ChannelAdapter`, no adapter-reported readiness — carries a `Served=False`
condition. Typos fail visibly instead of queueing ops forever.

### A thread opens with the event that caused it

**Delivery is decided per DESTINATION.** When an input is appended and a thread
binding exists, the manager delivers it to every bound channel EXCEPT the
surface it entered on, because that surface displayed it as it was typed.

An event nobody typed entered on no surface, so every channel receives it.

**What arrives depends on who said it:**

| Said by | Arrives as |
|---|---|
| an event | a `signal` message — so an alert thread reads as the event, then the work, then the answer |
| a person | a `relay`, with `origin` and `sender` |

**Whether the origin surface displayed it is a fact about YOUR transport, so you
declare it:** `ChannelAdapter.spec.echoesOwnMessages`, default true. A viewer
that renders only what it is sent sets it false and receives its own users'
messages like any other destination.

Three more rules:

- **An input predating provenance is delivered nowhere.**
- **Op ids are stable per conversation × input × channel**, so reconcile-driven
  re-enqueues dedup.
- **Delivery runs parallel to dispatch.** It neither gates nor is gated by the
  run.

### The operator delivers, always

**An agent's printed answer is its whole deliverable.** The runtime reports it
via `/work/done` and the manager posts it to every bound thread through the
serving adapters.

**Agents never send chat messages themselves**, so no prompt carries transport
instructions and no runtime holds a channel's credentials. The surface is the
adapter's business alone.

- **A conversation dispatches once at least one of its topics exists**, so one
  broken channel never deadlocks it.
- **Channel implementations must never re-ingest their own outbound posts as
  inbound.** Relayed messages would loop otherwise.

That last rule is load-bearing in one more place now: one adapter may serve
several surfaces of one conversation, so a message can be delivered toward the
transport it entered through.

### Conversation context travels as an OPAQUE HANDLE

A work unit carries `runtimeContextId` — the runtime's own identifier for
everything this conversation has accumulated.

The obligation is one sentence: **continue the context this handle names, or
report that you could not.**

**Where that context lives is entirely yours**: session files on a mounted
volume, a thread id at a vendor API, rows in a database. The manager stores the
handle, hands it back, and interprets nothing.

`--resume` is one runtime's implementation of this and `session` is that
runtime's word for it. Neither appears in the contract.

`POST /work/done` reports back:

| field | meaning |
|---|---|
| `runtimeContextId` | the handle for the context this run leaves behind. **Latest-wins** — it replaces the stored one, including on a FAILED run, because a crash after a context was established should not strand it |
| `continuity` | `continued`, `new`, or `unavailable` |
| `continuityReason` | free text, recorded verbatim, when a context could not be reached |

**`continuity` exists because the manager cannot infer it.** It sends a handle
and receives a handle, and when they differ it has no way to tell an agent that
branched deliberately from one that was forced to start over.

- **Absent means no claim.** A runtime that omits it keeps today's behaviour,
  because an addition to the contract must not make an existing runtime look
  broken.
- **A runtime that cannot continue anything conforms by always reporting
  `new`.**

**Reporting `unavailable` means the context was LOST.**

**Do not answer without it.** A conversation without its context is a new one
wearing the same name and thread, and an agent asked to undo something it has no
memory of will guess.

Fail the run and say why — never with an empty result, which is what made
answering-anyway look like the kinder option.

**Before declaring it, distinguish a store that says GONE from one that did not
ANSWER.** A shared filesystem can stall for seconds, and ending a conversation
over that would be a correctness bug.

`resumeSessionId` and `sessionId` are the retired spellings of the handle, sent
and accepted for one release so a runtime image can be upgraded independently of
the manager. Read `runtimeContextId`.

### Delivery is a recorded fact, not a queue entry

**`/work/done` writes the run into `Conversation.status.runs[]` and enqueues the
reply**, exactly as before. It is no longer the only path that can produce it.

The reply op id is **stable per conversation × channel × run**:

```
send:<conversation>:<channel>:<runId>
```

- **Completing it appends the channel to `status.runs[].delivered[]`.**
- **Reconciliation enqueues a reply** for any completed run whose bound thread
  is missing from that list.

So a manager restart between `/work/done` and an adapter claiming the op
re-derives the answer rather than losing it. A fan-out interrupted after one of
three threads completes the other two without repeating the first.

**Adapters need change nothing.** The id is a better dedup key than the counter
it replaces, and delivery stays at-least-once.

**Ack and notice sends keep counter-based ids** (`send:<counter>`) and stay
fire-and-forget. Saying the same thing twice is correct when a user asked twice,
and neither derives from CR state.

