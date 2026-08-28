# Contracts

How the out-of-process pieces talk to the manager: the two adapter contracts,
the runtime work contract, and the manager's own HTTP surface.

## The channel adapter contract

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

   **Every response carries `X-Agentops-Vocabulary-Revision`** — the delivered
   op and the empty `204` alike. See [What may be
   typed](#what-may-be-typed) below.

2. **Complete each op** with `POST /channel/ops/{id}/done`.

   | op | Body |
   |---|---|
   | `ensure-topic` | `{"threadId":"…"}` — an opaque string in your id space |
   | `close-topic` | **empty** |
   | any, on failure | `{"error":"…"}` — surfaced as a Conversation condition and regenerated |

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

Entries resolving to the same conversation are grouped into ONE status patch,
and a report that would not advance the watermark writes nothing at all. So
re-opening a quiet conversation costs no API write.

See [Read state](concepts.md#read-state-per-thread) for the field and the
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
  [concepts](concepts.md#releasing-a-runtime-by-hand--exit).
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
[`channels/telegram/`](../channels/telegram/) is dependency-free Go.

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

## The signal adapter contract

**Signals are one-directional**, so this is the channel contract minus the ops
queue. An adapter normalizes its transport into signals and the manager does the
rest: **adapters normalize, the manager groups.**

1. **Read your sources** and their opaque `spec.config` from
   `GET /signal/sources?adapter=`. Entries carry `credentialEnvPrefix` exactly
   like the channel listing.
2. **Push normalized signals** to `POST /signal/inbound`:

   ```json
   {"source":"…","signals":[{"fingerprint":"…","labels":{},"title":"…",
    "payload":"…","kind":"alert"}]}
   ```

   `kind` is one of `alert`, `job`, `task`, `chat`. `title` is optional.

   **A signal payload is never parsed as prose.** It reaches every adapter
   exactly as you sent it, and the adapter renders a CARD from your structured
   fields. The block grammar applies to AGENT output only — see [The body
   grammar](#the-body-grammar).

   The manager applies the source's `grouping` policy: fingerprint cooldown
   (at-least-once delivery is safe, re-sends collapse), signature from `labels`
   × `signatureLabels`, window reuse, and recurrence-on-session.
3. **Persist cursors** via `GET/PUT /signal/state/{source}/{key}`, and report
   config problems via `POST /signal/sources/{name}/status`.

**This endpoint is the only way work originates from outside a chat surface.**

There is no route that names a `Pipeline`. A caller posts to a `SignalSource`,
and the Ready Pipeline claiming that source decides which agent answers, on
which channels, with which capabilities.

**The `kind` selects the lane**, and the lane decides two things — which prompt
renders, and how a later signal is keyed when the source declares no
`signatureLabels`:

| `kind` | Lane | Subject | Keying with no `signatureLabels` |
|---|---|---|---|
| `alert` (default) | read-only investigation | a problem that recurs | default labels `alertgroup`/`alertname`/`namespace` — group and resume |
| `job` | task-lane prompt, `jobName` = source | a job that recurs | same default labels — successive ticks fold into one conversation |
| `task` | task-lane prompt, no `jobName` | a caller asking once | the signal's own fingerprint — each post is its own conversation |
| `chat` | task-lane prompt, no `jobName` | a person asking once | the signal's own fingerprint — each message is its own conversation |

- **`alert` and `job` are recurring-subject lanes.** The second signal is more
  news about the same thing, so it folds into the open conversation and resumes
  the session.
- **`task` and `chat` are one-shot lanes.** The second signal is a second
  request, and the default labels are alert vocabulary neither carries, so
  keying on them would hash every request to one empty signature.

**A `chat` signal MUST carry the label `agentops.dev/channel`** naming the
surface it arrived on, or it is refused — its reply has nowhere to go.

**A `task` signal must not.** Replies go to the claiming Pipeline's
`channelRefs`.

**A posted task inherits the target source's `grouping`.** The lane rule above
is a FALLBACK, not an override.

A source that declares `signatureLabels` groups by them in every lane, `task`
included, so two tasks sharing those label values land in one conversation. Two
tasks carrying none of them share the empty signature.

Post to a source whose grouping is what you want. Operators who need an isolated
ask lane create their own source against an adapter they run.

**Auth mirrors channels**: master token, or a per-`SignalAdapter` derived token
scoped to the adapter's name. The derivation context is distinct, so channel and
signal adapters sharing a name never share a token.

A `SignalSource` whose adapter nothing serves carries `Served=False`.

**Reference implementation:** [`signals/cron/`](../signals/cron/), which replaced
the old roadmap `cron` sub-struct.

`config: {schedule, input, title?}` fires job-lane signals with
`<source>@<tick>` fingerprints, restart-safe via the state API. The grouping
window turns a recurring job into one conversation whose later runs resume the
agent session.

**How an adapter proves it:** the contract conformance suite,
`platform/manager/test/conformance/`, runs every adapter's BUILT BINARY against
a fake manager and asserts the obligations above.

For a channel adapter:

- the long-poll and the `contract=` declaration
- typed-message rendering
- ack-once under a redelivered op id
- inbound push with a `threadId`
- listing and status
- no relay loop

For a signal adapter:

- normalized emission, the bearer, and source scoping
- a rejected post that is retried or surfaced rather than dropped
- for a chat-originating adapter, the channel label on every signal

A new adapter joins by being listed there. Nothing in its own source changes.
See [Testing](testing.md).

## The work contract

An `AgentRuntime` image must:

1. **Long-poll** `GET $CONTROL_URL/work?convo=$CONVO_ID&pod=$POD_NAME&wait=25`
2. **Execute the returned unit** — `promptText` (rendered), or
   `promptFile` + `promptVars` relative to the checked-out repo at
   `/data/workspace`, with `runtimeContextId` when continuing — streaming
   progress to **stdout**
3. **Report** the outcome:

   ```
   POST $CONTROL_URL/work/done {convo, runId, status, sessionId, result}
   ```

4. **Exit `0`** after `RUNTIME_IDLE_TTL_M` minutes without work

**`allowedTools` is the route's half of the allowlist, not the whole of it.**

The unit also carries `toolsMode` (`merge` | `overwrite`) and `agent`. A runtime
holding the repository is expected to read the agent's definition, take its
`tools:` frontmatter as the agent's own declaration, and compose the two.
WHERE that definition lives is the runtime's fact, not the contract's:
`runtime-claude` and `runtime-ollama` read `.claude/agents/<agent>.md`,
`runtime-copilot` reads `.github/agents/<agent>.agent.md`, and another backend
may read somewhere else.

| `toolsMode` | Result |
|---|---|
| `merge` | the union, the agent's keeping position |
| `overwrite` | `allowedTools` alone |

A runtime that cannot see a repository can use `allowedTools` as-is. That is
what `merge` degrades to.

**An empty allowlist means empty.** Substituting a tool nobody declared is a
grant the operator did not write down.

`runtime-claude` passes the composed list verbatim, even when it is empty, and
runs with `--permission-mode dontAsk` so an unlisted tool is denied outright. In
a pod there is nobody to answer a permission prompt, so prompting would hang the
run until its idle TTL.

`runtime-ollama` is the second implementation, and the one that had to build
everything the CLI provides for the first: the agent loop, tool dispatch, the
transcript and the handle. It composes the same two halves, applies the gate
ONCE before the request — only allowed tools are advertised — and logs every
allowlist entry it cannot provide. Building it needed no change to this
contract, which is what makes the contract vendor-neutral rather than a
description of one CLI.

`runtime-copilot` is the third, and the first whose vendor owns its own tool
vocabulary. It translates the composed allowlist at the point of use — into
Copilot's availability filters and a per-invocation permission callback — and
what it cannot translate it withholds and logs, never passes through. Three
obligations it made visible bind every runtime:

| Obligation | Because |
|---|---|
| **an absent or unreadable definition contributes NOTHING**, whatever the vendor's own default | Copilot reads a definition with no `tools:` as "every tool". The runtime passes the composed list explicitly, empty included, so that reading never applies |
| **a pattern the runtime cannot honour is reported, never guessed at** | passing it through would hand the vendor a string it reads as some other tool. Dropping it silently would widen or narrow a route with no record |
| **what a narrowing pattern such as `Bash(kubectl:*)` does is the RUNTIME's fact** | `runtime-ollama` has no per-invocation hook and grants nothing on it. `runtime-copilot` has one and enforces the scope. Each says so on its own page |

The context id is the runtime's to mint there — the SDK accepts a caller-chosen
one — which is the first backend to exercise `runtimeContextId` as a handle the
runtime chose rather than one scraped from a vendor's output.

**`$CONTROL_URL` is not always the manager.** When a runtime declares
`contextSync`, the manager points it at a sidecar in the same pod, which
forwards every request to the real manager.

Nothing in the contract changes, and a runtime needs no awareness of this.

**That is the point.** Observing the contract a runtime already implements is
what lets context be checkpointed at work boundaries without every image
reimplementing it. Two moments are acted on, both invisible to the runtime.

| moment | what happens |
|---|---|
| before the first `GET /work` answers | durable context is restored into the pod-local home |
| before `POST /work/done` reaches the manager | the context is checkpointed |

**That ordering is a guarantee, not an implementation detail.** The manager
records the context handle from the completion report, so checkpointing
afterwards could leave a recorded handle whose context was never persisted. The
next run would then fail a continuation that should have worked.

### A tool call the model cannot FORM

A model writes its tool arguments as text, and that text is not always valid
JSON. When it is not, the call is **discarded before anything runs** — no MCP
server sees it, no allowlist refuses it, no request leaves the pod.

That failure is invisible from outside. The run looks busy, and the model tends
to write the same broken call again, because nothing tells it which character
was wrong.

`runtime-claude` therefore counts them:

| Situation | What happens |
|---|---|
| a call whose arguments do not parse | logged, and counted |
| the run ANSWERS having made some | the answer carries a line saying how many never ran, and which tool |
| the SAME tool, the SAME arguments, `RUNTIME_UNPARSED_REPEAT_LIMIT` times in a row (default 5) | the run is ended and reported **failed**, naming the tool and quoting what was written |

**Consecutive and identical, both deliberately.** A model that varies its
arguments is trying something, and one whose next call parses has recovered.
The failure worth ending is the loop that cannot end, because nothing about it
changes.

**A run that recovers still says so.** Recovery usually means ABANDONING the
tool, not fixing the call — twice out of twice observed, the model then answered
from what the session already held and the run was reported a success. The
notice is appended to the answer, never substituted for it, because the answer
itself does not mention it.

**Failing is the point of the breaker.** Without one, a spin ends the same way:
a successful-looking run, answered from memory, presented as current.

**A schema with no declared type is the usual cause.** A parameter typed
`string` gets quoted, and one whose schema says nothing gets a bare word. Set
the limit to `0` to disable the breaker without disabling the counting.

**Reference implementations:**

- [`runtimes/claude/`](../runtimes/claude/) — Node.js + claude-code, ~200 lines.
- [`channels/telegram/`](../channels/telegram/) — the same bring-your-own pattern
  for chat transports, against the channel adapter contract above.
- [`platform/context-sync/`](../platform/context-sync/) — the sidecar.

## The activity contract

**Per-hop telemetry: one structured event for every movement the manager
mediates.**

It exists because nothing else records motion. CR status says a conversation is
`inflight`, never that the manager handed run `r-7` to a runtime and got a
result 4s later, so a graph derived from status animates what it *infers* rather
than what happened.

| Endpoint | Purpose |
|---|---|
| `GET /activity?since=<cursor>&limit=<n>` | bounded replay from the ring buffer |
| `GET /activity/stream` | SSE, each event carrying its cursor |
| `POST /activity` | adapter-reported hops (delivery confirmation) |

**All three use the adapter bearer scheme** and accept **either** a channel- or
a signal-adapter derived token, or the master token. The console holds both
identities and would otherwise have to pick one arbitrarily.

Event shape:

```json
{
  "cursor":       "0000000000012345",
  "ts":           "2026-08-09T11:04:22.117Z",
  "kind":         "run.completed",
  "from":         {"kind": "runtime",  "name": "default"},
  "to":           {"kind": "pipeline", "name": "k8s-ops"},
  "status":       "ok",
  "conversation": "chat-abc12",
  "pipeline":     "k8s-ops",
  "runId":        "r-7",
  "opId":         "send:9",
  "inputId":      "in-mfx1",
  "latencyMs":    4218,
  "code":         "succeeded",
  "detail":       "succeeded (exit 0)",
  "adapter":      "telegram"
}
```

**`from` and `to` name nodes the way the topology graph names them**, so an
event renders directly as motion along an edge that already exists. No inference
in the consumer.

Node kinds: `signal-adapter`, `signal-source`, `pipeline`, `conversation`,
`profile`, `runtime`, `channel`, `channel-adapter`, `toolset`, `mcp-config`,
`manager`.

| Kind | From → To | Emitted when |
|---|---|---|
| `signal.received` | signal-adapter → signal-source | `POST /signal/inbound` accepted |
| `signal.claimed` | signal-source → pipeline | a Ready Pipeline claims the source |
| `signal.dropped` | signal-source → ∅ | unclaimed source, or the pending backlog is full |
| `conversation.created` | pipeline → conversation | Conversation CR created |
| `input.queued` | source/channel → conversation | an input was appended |
| `run.dispatched` | pipeline → runtime | `GET /work` handed a unit out |
| `run.completed` | runtime → pipeline | `POST /work/done` |
| `channel.op.enqueued` | conversation → channel | `ensure-topic` / `send` / `close-topic` queued |
| `channel.op.completed` | channel-adapter → manager | op acked, or failed with a reason |
| `channel.inbound` | channel-adapter → conversation | a user reply entered the router |
| `context.restored` | runtime → conversation | durable context copied into a starting pod |
| `context.checkpoint` | conversation → runtime | live context copied to the volume |
| `context.skipped` | conversation → runtime | a checkpoint ran and found nothing changed |
| `context.failed` | conversation → runtime | a restore or checkpoint failed |

**Three properties are load-bearing:**

- **Bounded and lossy by design.** A fixed-size in-memory ring
  (`ACTIVITY_BUFFER`, default 10000), evicting oldest-first, never persisted and
  never written to any Kubernetes object. The durable record stays
  `Conversation.status.runs[]`. Emission never blocks the operation it records:
  a full buffer drops, and a slow subscriber is marked lagged rather than waited
  on.
- **A gap is always explicit.** A cursor older than the buffer's oldest answers
  `"resync": true` (SSE: an `event: resync` frame), and the client is expected
  to re-read snapshots. A silent short list would be indistinguishable from
  continuity.
- **Telemetry is not signal.** These events go to this log, never to
  `/signal/inbound`, and nothing converts one into the other. agent-ops' own
  health is STATUS, not SIGNAL — routing an error event about a broken runtime
  pod back through ingest is the loop `signals/k8s-events/selfexclude.go` exists
  to break, and keeping the surfaces apart makes it structural here.

**Attribution.** `pipeline` is present when it is knowable and EMPTY when it is
not.

A Conversation records no `pipelineRef`, so attribution is inferred from the
bindings it materialized and is left blank when two pipelines wire identically.
Empty means "not attributable", never "none".

**Adapter-reported hops are optional.** An adapter that reports nothing still
appears on the graph through manager-side events. It simply never confirms
delivery, and the edge reads "sent, unconfirmed" rather than claiming success.

`POST /activity` takes:

```json
{"kind":"…","from":{},"to":{},"status":"…","conversation":"…","opId":"…",
 "latencyMs":0,"detail":"…"}
```

Every field but `kind` is optional. The reporting adapter is taken from the
TOKEN, so an adapter naming another in `adapter` is refused with 403.

## Manager introspection

**The manager exposes only what only the manager knows.**

Anything that lives in a CR is read from the API server, with the API server's
own RBAC, and is never proxied. A manager that mirrors CRs becomes a second
Kubernetes API with a second auth scope and its own staleness.

Both endpoints below are read-only and use the same bearer scheme as
`/activity`.

**`GET /status`** — manager-internal state that exists in no Kubernetes object:

- **Build version** and the leader-election lease holder.
- **Runtime slots in use** against `MAX_ACTIVE_CONVERSATIONS`, counted from the
  live POD list — the same definition the admission gate uses, so the two cannot
  disagree — plus how many conversations are waiting for one.
- **Per-adapter op queue depth**, split into **queued** (nothing is claiming)
  and **claimed but uncompleted** (an adapter is wedged mid-delivery). Those are
  two failure modes that look identical from outside, and each carries the id
  and age of the oldest.
- **Active cooldowns per source**, because a suppressed lane looks exactly like
  an idle one on a graph.

**`GET /pipelines/{name}/resolved`** — the authoritative capability resolution:

- `allowedTools` — the wiring's half, composed through the same function
  dispatch uses.
- `toolsMode`, `toolsets`, `mcpConfigs`, `mcpServers`, and the resolving
  runtime.
- `unresolved` for refs that resolve to nothing.

404 for an unknown pipeline. **An empty allowlist is reported as empty**, never
as a fallback.

**A consumer must not recompute this.** A second implementation of composition
would eventually disagree with the one that runs.

**The split between the two surfaces and `:9090/metrics` is deliberate.**
Metrics answer *how deep, how old, how many*. `/status` answers *which one*. Ids
never become metric labels — see below.

## Metrics

The same aggregates are exposed in the Prometheus exposition format on the
manager's existing `:9090/metrics`, registered into the controller-runtime
registry already serving that port.

**Nothing new is listened on**, so alerting never depends on anyone having a
browser open.

**One instrumentation pass.** Counters and histograms are driven by the activity
event stream — the metric set is an `activity.Observer` — so an event and its
metric observation cannot occur independently. Gauges are levels, sampled at
scrape time from the same in-memory state `/status` reports.

| Metric | Type | Labels |
|---|---|---|
| `agentops_signals_received_total` | counter | `source`, `adapter`, `status` |
| `agentops_signals_dropped_total` | counter | `source`, `reason` |
| `agentops_conversations_created_total` | counter | `pipeline` |
| `agentops_runs_total` | counter | `pipeline`, `status` |
| `agentops_run_duration_seconds` | histogram | `pipeline` |
| `agentops_channel_ops_total` | counter | `adapter`, `kind`, `status` |
| `agentops_channel_op_latency_seconds` | histogram | `adapter`, `kind` |
| `agentops_channel_ops_queued` | gauge | `adapter` |
| `agentops_channel_ops_claimed` | gauge | `adapter` |
| `agentops_channel_op_oldest_queued_age_seconds` | gauge | `adapter` |
| `agentops_channel_op_oldest_claimed_age_seconds` | gauge | `adapter` |
| `agentops_runtime_slots_in_use` | gauge | — |
| `agentops_runtime_slots_max` | gauge | — |
| `agentops_conversations_inflight` | gauge | `pipeline` |
| `agentops_cooldowns_active` | gauge | `source` |

**The cardinality rule is binding.**

- **Labels carry only values bounded by CR count** — `pipeline`, `adapter`,
  `source`, `channel`, `kind`, `status`, `reason`.
- **They are read from an event's structured fields** (node names, `code`),
  never from `detail`, which may carry a fingerprint or an error message.
- **A conversation, run or op id as a label would grow series without limit.**
  Those identify the specific stuck item and stay in `/status`.

## HTTP API

| Endpoint | Purpose |
|---|---|
| `GET /work`, `POST /work/done` | runtime-facing dispatch (see contract) |
| `POST /work/context` | context-sync sidecar reports one restore or checkpoint |
| `GET/POST /channel/*` | adapter-facing channel contract (bearer token, see adapter contract) |
| `GET/POST/PUT /signal/*` | adapter-facing signal contract (bearer token, see signal adapter contract) |
| `GET/POST /activity*` | per-hop telemetry (bearer token, see activity contract) |
| `GET /status`, `GET /pipelines/{name}/resolved` | manager introspection (bearer token) |
| `GET /healthz` | liveness |
| `:9090/metrics` | controller-runtime metrics + the `agentops_*` set above |

**There is no programmatic origination endpoint.** To start a conversation from
a script, post a `kind: task` signal to a `SignalSource` a Ready Pipeline
claims:

```sh
curl -s -X POST http://agentops-manager.<ns>.svc:8080/signal/inbound \
  -H "Authorization: Bearer $ADAPTER_TOKEN" -H 'Content-Type: application/json' \
  -d '{"source":"<source>","signals":[{"fingerprint":"ci-'"$BUILD_ID"'",
       "kind":"task","payload":"why is the api pod crashlooping?"}]}'
```
