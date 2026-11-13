---
title: ChannelAdapter specification
permalink: /contracts/channel-adapter/
description: >-
  The versioned, normative specification of what a ChannelAdapter owes the
  manager, and what the manager owes it — for building a channel adapter
  agent-ops does not ship.

next:
  eyebrow: Reference
  title: Every ChannelAdapter field
  body: >-
    The kind's own spec page — a commented example and every field, in the
    sidebar's CRDs group.
  url: /agent-ops-operator/crds/channel-adapter/
---

**Version 2.0.0**

This specification gives everything necessary to build and maintain a
`ChannelAdapter` — a component agent-ops does not ship, that carries a
conversation onto a chat surface (Telegram, a web console, Slack, anything
with a reply box).

## Conventions

MUST / MUST NOT / SHOULD / MAY follow RFC 2119. Every requirement carries a
stable ID, prefixed by the section it belongs to:

| Prefix | Section |
|---|---|
| `OPS-n` | [GET /channel/ops](#get-channelops) / [the acknowledgement](#acknowledging-an-operation) / [POST /channel/ops/{id}/done](#post-channelopsiddone) / [GET /channel/ops/stream](#get-channelopsstream) |
| `IN-n` | [POST /channel/inbound](#post-channelinbound) |
| `READ-n` | [POST /channel/read](#post-channelread) |
| `MGR-n` | [Manager's provisions](#managers-provisions) |
| `ADA-n` | [Adapter's obligations](#adapters-obligations) |
| `SEC-n` | [Auth](#auth) |
| `COMPAT-n` | [Compatibility](#compatibility) |

> Every `MUST`/`MUST NOT` on this page is required for conformance.
> Everything gathered in [Optional capabilities](#optional-capabilities) is
> not — an adapter implementing none of it is still fully conformant.
{: .ao-callout}

## GET /channel/ops

Long-polls for this adapter's next outbound operation, **without taking it**.

`GET $MANAGER_URL/channel/ops?adapter=<name>&contract=2&wait=25`

| Param | Required | Meaning |
|---|---|---|
| `adapter` | yes | This `ChannelAdapter` CR's own name — the routing key. |
| `contract` | yes | The outbound message contract version this adapter speaks. |
| `wait` | no | Seconds to block. Clamped to 30 server-side. |

| Status | Body |
|---|---|
| `200` | One [operation](#the-operation-envelope), below. |
| `204` | No work yet. Not an error. |
| `400` | `adapter` or `contract` missing or outdated — the message names the version to build against. |
| `401` | no or wrong bearer token. |
| `403` | the token's scope names a different adapter. |

**A read changes nothing.** The operation stays in the manager's queue until
the adapter [acknowledges](#acknowledging-an-operation) it, so an adapter that dies
between the read and the acknowledgement is simply offered the same operation
again. Any manager replica answers.

**Every response carries `X-Agentops-Vocabulary-Revision`** — the `200`
that delivers an op and the `204` alike. See
[GET /channel/vocabulary](#get-channelvocabulary).

| ID | Requirement |
|---|---|
| `OPS-18` | An adapter MUST acknowledge an operation before acting on it. It MUST NOT deliver an operation it has not acknowledged. |
| `OPS-19` | An adapter MUST tolerate being offered an operation it already acknowledged or finished, and MUST treat it as opaque by `id`. |

### The operation envelope

| Field | Type | Required | Description |
|---|---|---|---|
| `id` | string | yes | Dedup key. Delivery is at-least-once — tolerate a repeated id. |
| `channel` | string | yes | The `Channel` CR this op targets. |
| `conversation` | string | no | The `Conversation` this op belongs to. Empty for a manager notice bound to no conversation. |
| `conversationId` | string | no | The conversation's manager-issued correlation id, carried beside `conversation`. OPAQUE to the adapter — carried, compared, never parsed. Absent on an operation for a conversation created before this field existed. |
| `kind` | string | yes | One of [the typed operations](#typed-operations). |
| `threadId` | string | no | The send target. Absent means the channel's general surface. |
| `topic` | object | no | A [topic descriptor](#the-topic-descriptor). Set only on `ensure-topic`. |
| `message` | object | no | The [typed message](#the-typed-message) to post. Set only on `send`. |

### Typed operations

| Kind | Does | Completed with |
|---|---|---|
| `ensure-topic` | create a thread for a conversation | `{"threadId":"…"}` |
| `send` | post `op.message` | an empty body |
| `close-topic` | archive the thread named in `op.threadId` — its conversation ended | an empty body, or a logged failure (never a condition, see `OPS-8`) |
| `delete-conversation` | tell the thread in `op.threadId` its conversation is gone for good | an empty body |

| ID | Requirement |
|---|---|
| `OPS-1` | An adapter MUST long-poll this endpoint while able to accept ops. `204` is not an error. |
| `OPS-2` | A `503` from a replica or a proxy in front of it MUST be treated exactly like a `204` — retried at once, on a fresh connection, never surfaced as an error. |
| `OPS-3` | An adapter not implementing a kind MAY complete it with an `error`, or ignore it. |
| `OPS-9` | Every operation additionally carries `conversationId`, beside the channel and conversation names it already carries. The field is ADDITIVE — an adapter that ignores it MUST remain fully conformant, declaring the same `contract` version. |

### Delivery operation ids are published shapes

The manager mints stable ids for the two `send` operations that deliver
conversation content:

| Delivery | Id shape |
|---|---|
| an agent's reply, per run | `send:<conversation>:<channel>:<runId>` |
| a person's input, fanned out | `input:<conversation>:<input>:<channel>` |

| ID | Requirement |
|---|---|
| `OPS-10` | Each published shape MUST stay stable across a repeated offer and manager restarts — it is what lets an adapter recognise an op it already delivered. |
| `OPS-11` | A consumer MAY parse a published shape. The conversation and channel segments are Kubernetes names, which cannot contain the separator, so the one remaining segment is recovered by position, whatever it contains. |
| `ADA-15` | An adapter MUST treat an id matching no published shape as opaque — completing the operation by that id verbatim and inferring nothing from its content. An unpublished id can change without breaking anyone. |

### Acknowledging an operation

**The ack is the hand-over.** From it the operation is the adapter's: the
manager never sends it again, and its delivery TTL starts.

`POST $MANAGER_URL/channel/ops/{id}/ack?adapter=<name>`

```
POST /channel/ops/send:conv-1:tg-ops:run-7/ack?adapter=telegram
200  {"ok": true, "deliveryTtlSeconds": 600, "consumed": false}
```

| Status | Body |
|---|---|
| `200` | `{"ok": true, "deliveryTtlSeconds": <seconds>, "consumed": <bool>}`. |
| `400` | `adapter` missing, or an unknown query parameter, refused by name. |
| `401`, `403` | as for the read. |
| `404` | the operation is gone: consumed, expired, or never enqueued here. A repeated acknowledgement of a finished op reads as this and is harmless. |
| `409` | the operation exists but is not being offered: behind an unfinished operation of its conversation, or finished. |

| Reply field | Meaning |
|---|---|
| `deliveryTtlSeconds` | how long the adapter has to [report](#post-channelopsiddone) the outcome. `0` when `consumed` is true |
| `consumed` | `true` when the manager consumed the op at the acknowledgement and nothing is to be reported. Only a [receipt adapter](#the-receipt-only-adapter) sees it |

The delivery TTL is declared per kind, so every adapter gets the same one for
the same kind of operation:

| Kind | Default delivery TTL |
|---|---|
| a reply, an input card | 10 minutes |
| an ack or a notice | 2 minutes |
| `ensure-topic`, `close-topic` | 5 minutes |
| `delete-conversation` | 5 minutes |

| ID | Requirement |
|---|---|
| `OPS-4` | An adapter absorbing transport backpressure in-process MUST budget the total wait, retry and timeout of every call ONE operation makes strictly below `deliveryTtlSeconds`, leaving room to report. An adapter overrunning it is failed by the manager while it may still be delivering, and a failed operation is not offered again. |
| `OPS-5` | An acknowledgement is idempotent and answers the same on any replica. An adapter MAY repeat it after a lost reply. |
| `OPS-20` | The manager MUST NOT send an acknowledged operation again, however long the adapter takes inside its delivery TTL. |
| `OPS-21` | An acknowledged operation with no report when its delivery TTL passes MUST be failed by the manager, and a failure MUST be final: the operation is never redelivered. |
| `OPS-22` | What is still owed after a failure is derived again from the conversation, with the same stable id, only after a cool-off. A failed operation never comes back by being offered again. |

## POST /channel/ops/{id}/done

Reports the outcome of an acknowledged op, **done or failed**.

`POST $MANAGER_URL/channel/ops/{id}/done`

| Status | Body |
|---|---|
| `200` | `{"ok": true}`. A duplicate report is legal and always `200`. |
| `400` | malformed JSON. |

An empty body reports success, as `close-topic` and `send` do.

```
POST /channel/ops/topic:conv-1:7c1e:tg-ops/done   {"threadId": "1187"}     # done, with its result
POST /channel/ops/send:conv-1:tg-ops:run-7/done   {}                       # done
POST /channel/ops/send:conv-1:tg-ops:run-7/done   {"error": "chat not found"}   # failed
```

### The report

| Field | Type | Required | Description |
|---|---|---|---|
| `threadId` | string | no | Required for `ensure-topic` — the opaque thread id the adapter minted, in its own id space. |
| `error` | string | no | Set when the delivery failed. The op is FAILED, final, never redelivered. |

| ID | Requirement |
|---|---|
| `OPS-6` | An `ensure-topic` reported with no `error` and no `threadId` MUST be treated by the manager as a failure. |
| `OPS-7` | `close-topic`'s failure MUST be logged rather than written as a condition — its conversation may be deleted, so no object will survive to carry one. |
| `OPS-8` | `delete-conversation`'s failure is TERMINAL: logged, never a condition. Deletion MUST NOT wait longer than a 2-minute grace, so a down adapter cannot wedge it. An already-closed thread is treated as success. |
| `OPS-23` | An adapter MUST report every operation it acknowledged, done or failed, before its delivery TTL passes. It SHOULD retry the report a few times, since a lost report is failed by the manager at the TTL. |

### Backpressure is the adapter's to absorb

A transport that says "slow down" — HTTP 429, a `retry_after`, a
token-bucket rejection — is RETRYABLE backpressure, not an operation
failure. A TERMINAL failure — a missing thread, a rejected credential, a
reason retrying cannot fix — is reported at once, never ridden out.

| ID | Requirement |
|---|---|
| `ADA-3` | An adapter MUST absorb a retryable backpressure signal inside its delivery TTL, waiting the interval the transport stated rather than a backoff it computed instead. |
| `ADA-3.1` | An adapter MUST report an op failed only when the condition is terminal, or when riding it out would overrun the delivery TTL. |
| `ADA-3.2` | The retry budget is PER OPERATION, never per transport call. An operation making several calls — a reopen plus a create, one send per chunk, a fallback — MUST charge every call's wait, retry and own timeout against the one budget. |
| `ADA-3.3` | A failure is final. Reporting a retryable condition as an operation failure gives up the delivery for a condition the adapter could have ridden out. An adapter MUST NOT treat it as the normal path. |
| `ADA-4` | An adapter MUST pace its own sending so bursts are spread rather than rejected, and MUST gate pacing before it READS an op, never before it sends and never between the acknowledgement and the send. A delivery waiting on a pacer spends its delivery TTL. |

## GET /channel/ops/stream

**Optional, and for a receipt adapter only.** The same operations as
[GET /channel/ops](#get-channelops), pushed over one server-sent-events
connection. It is available to an adapter the manager is configured to treat as
[receipt-only](#the-receipt-only-adapter), the console by default. Any other
adapter is answered `404` and reads the long-poll.

`GET $MANAGER_URL/channel/ops/stream?adapter=<name>&contract=2`

| Param | Required | Meaning |
|---|---|---|
| `adapter` | yes | This `ChannelAdapter` CR's own name — the routing key. |
| `contract` | yes | The outbound message contract version this adapter speaks. |

Any other parameter is refused by name. There is no resume point.

| Status | Body |
|---|---|
| `200` | `text/event-stream`, below. |
| `400` | as for the long-poll. |
| `401`, `403` | as for the long-poll. |
| `404` | this adapter is not a receipt adapter. Read `GET /channel/ops`. |

The response sets `X-Accel-Buffering: no`, and carries
`X-Agentops-Vocabulary-Revision` as the long-poll does.

### The stream's events

| Event | `id:` | `data:` |
|---|---|---|
| `op` | the op's `id` | an [operation](#the-operation-envelope), byte for byte what the long-poll returns |
| `vocabulary` | none | `{"revision": "<revision>"}`, sent when the revision changes after the connection opened |

A comment line, `: keep-alive`, is sent when the stream is otherwise idle.

| ID | Requirement |
|---|---|
| `OPS-12` | The manager MUST push each unacknowledged operation once per connection, in queue order, and MUST change nothing by pushing it. |
| `OPS-13` | After a reconnect the manager MUST push again exactly the operations never acknowledged. There is no cursor, no sequence and no `Last-Event-ID`. |
| `OPS-14` | A consumer MUST acknowledge each operation with [`POST /channel/ops/{id}/ack`](#acknowledging-an-operation). A pushed operation that is not acknowledged stays unacknowledged. |
| `OPS-15` | Any manager replica MUST serve the stream, and an acknowledgement reaching a replica other than the one that pushed the operation MUST still take it. |
| `OPS-16` | A consumer MUST tolerate being pushed an operation it already rendered, and MUST render one `id` once. |

A consumer SHOULD treat a stream silent for several keep-alive intervals as
dead and reconnect with backoff.

### The receipt-only adapter

**The console is the one declared exception to the delivery TTL, by
configuration.** The manager's `CHANNEL_RECEIPT_ADAPTERS` (default `console`,
chart value `commandBus.channelReceiptAdapters`) names the adapters whose
acknowledgement is only their receipt of the operation from the manager.

For such an adapter the manager performs the delivery side effects at the
acknowledgement, deletes the operation, and answers `consumed: true` with
`deliveryTtlSeconds: 0`. The adapter reports nothing. It also does not wait for
its own onward push to a browser, which stays fire-and-forget.

| ID | Requirement |
|---|---|
| `OPS-24` | A receipt adapter MUST acknowledge each operation on receipt and MUST NOT call `/done`. |
| `OPS-25` | The manager assigns a receipt adapter's thread id when it enqueues `ensure-topic` (`console-<conversation uid>`), so the adapter uses the id it is given. No result travels back. |
| `OPS-26` | The mechanism is generic. A new adapter needs no manager change: it gets the full contract above, and becomes receipt-only by being named in `CHANNEL_RECEIPT_ADAPTERS`. |

## POST /channel/inbound

Pushes a user reply into an existing conversation thread.

`POST $MANAGER_URL/channel/inbound`

### The inbound request

| Field | Type | Required | Description |
|---|---|---|---|
| `channel` | string | yes | The `Channel` CR the message arrived on. |
| `threadId` | string | yes | The thread this message continues. |
| `text` | string | yes | The message, as the person typed it. |
| `sender` | string | no | The transport-side identity, when the adapter can supply one. |

| ID | Requirement |
|---|---|
| `IN-1` | The manager MUST refuse a request with no `threadId` — this endpoint continues an existing conversation and never starts one. A general-surface message is an ORIGINATION, posted instead as a chat signal to `POST /signal/inbound` from a chat `SignalSource` a Ready Pipeline claims. |
| `IN-2` | A message naming a thread the manager does not know MUST be dropped, not adopted. |
| `IN-3` | The manager MUST refuse a message whose origin surface names the thread's own target conversation — the shape a self-feeding loop takes. |
| `IN-4` | A reply on a bound thread is correlated by that binding's `conversationId`, not minted by the adapter — see `ADA-14`. |
| `ADA-5` | An adapter MUST NOT re-ingest its own outbound post as inbound. Delivery to the other bound surfaces, and busy-acks, happen manager-side — a reply the adapter pushes is delivered back to it only when it declares `echoesOwnMessages: false`. |
| `ADA-14` | A channel adapter MUST NOT create, derive, or alter any conversation id. The manager-issued `conversationId` reaches it already attached to the binding and its operations. The adapter's only id work is transport translation: `conversationId` on one side, the thread id on the other. Origination stays on the signal lane — no `/channel/*` route accepts a new conversation. |

## GET /channel/channels

Lists the Channels this adapter serves, with their opaque config.

`GET $MANAGER_URL/channel/channels?adapter=<name>`

### The channel-listing entry

| Field | Type | Required | Description |
|---|---|---|---|
| `name` | string | yes | The `Channel` CR's own name. |
| `config` | object | no | The Channel's `spec.config`, opaque to the manager — never parsed manager-side. |
| `credentialEnvPrefix` | string | no | Where this channel's projected credentials live in the adapter's own environment. Secret key `K` appears as env `<prefix>K`. |

### Two more utility calls

| Call | Does |
|---|---|
| `GET`/`PUT /channel/state/{channel}/{key}` | Persists adapter-side cursor state — poll offsets and the like. |
| `POST /channel/channels/{name}/status` | Reports a config problem: `{"ready","reason"?,"message"?}`. |

> A `Channel` nothing serves — no in-process provider, no Ready
> `ChannelAdapter`, no adapter-reported readiness — carries a
> `Served=False` condition. Typos fail visibly instead of queueing ops
> forever.
{: .ao-callout}

## POST /channel/read

**Optional** — see [Optional capabilities](#optional-capabilities).

```jsonc
POST /channel/read
{"channel":"console",
 "reads":[{"threadId":"console-uid-abc","readAt":"2026-08-13T10:12:04Z",
           "reader":"sha256:9f2a…"}]}

200
{"results":[{"threadId":"console-uid-abc","outcome":"marked"}],
 "marked":1,"skipped":0,"failed":0}
```

### A read-report entry

| Field | Type | Required | Description |
|---|---|---|---|
| `threadId` | string | yes | Which thread was read. The manager resolves it to a conversation. |
| `readAt` | string | yes | An RFC3339(-Nano) timestamp, opaque to the manager. |
| `reader` | string | no | OPAQUE key for whoever read the thread. Absent means the CHANNEL-WIDE mark. |
| `rewind` | boolean | no | "Mark unread" — sets the named reader's own watermark to `readAt` even where that is earlier than stored. Refused with no `reader`. |

### A read-report outcome

| Field | Type | Required | Description |
|---|---|---|---|
| `threadId` | string | yes | Echoes the entry's own `threadId`. |
| `reader` | string | no | Echoes the entry's own `reader`. |
| `outcome` | string | yes | `marked`, `skipped`, or `failed` — see [Enums](#enums). |
| `reason` | string | no | Set for anything not `marked`. |

| ID | Requirement |
|---|---|
| `READ-1` | A batch MUST hold at least one entry and at most 50. An empty or oversized batch is refused whole, with nothing written. |
| `READ-2` | A watermark at or before the one already stored for its reader (or the channel-wide mark, when no reader is named) MUST be `skipped`, not written and not an error. |
| `READ-3` | A watermark ahead of the manager's own clock MUST be clamped to that clock. |
| `READ-4` | A mixed batch MUST still answer `200`, with one `marked`/`skipped`/`failed` outcome per entry — one bad entry MUST NOT stop the rest. |
| `READ-5` | `reader` MUST be an OPAQUE token, never an identity. A key containing `@` MUST be refused with `400`. |
| `READ-6` | At most 50 readers per thread binding are kept, oldest watermark evicted first — never the entry that triggered the eviction. An evicted reader falls back to the channel-wide mark. |

**The manager owns the write.** The adapter reports and issues no
Kubernetes write of its own, exactly as with
`POST /channel/channels/{name}/status`. See
[Read state](../concepts.md#read-state-per-thread) for the field and the
backfill rule.

## Acting on a bound conversation

All three take `{"channel":"…"}` in the body:

| Verb | Does |
|---|---|
| `POST /channel/conversations/{name}/reopen` | returns a `Closed` conversation to `Idle` |
| `POST /channel/conversations/{name}/delete` | removes an already-`Closed` conversation for good |
| `POST /channel/conversations/{name}/reset-context` | clears the conversation's `runtimeContextId` |

| ID | Requirement |
|---|---|
| `MGR-7` | Reach for all three MUST be the BINDING, read from the conversation's `spec.channelRefs` and never taken from the request — a surface may act only on a conversation whose bindings name its channel. |
| `MGR-8` | `delete` MUST additionally refuse anything not already `Closed`, naming the missing step, and MUST NOT close the conversation itself first. |

## GET /channel/vocabulary

Returns everything a person may type on a chat surface, and the revision
identifying it.

```json
{"revision":"3f2a91c…",
 "entries":[
   {"kind":"builtin","name":"pipelines","description":"…","position":"general"},
   {"kind":"pipeline","name":"k8s-observe","description":"k8s-engineer",
    "position":"general","profile":"k8s-engineer"}]}
```

| Field | Type | Required | Description |
|---|---|---|---|
| `revision` | string | yes | Changes whenever the entries change. Compare against the long-poll's header to know when to refetch. |
| `entries[].kind` | string | yes | `builtin` for a manager command, `pipeline` or `coordinator` for an addressable route. |
| `entries[].name` | string | yes | What to type. |
| `entries[].description` | string | yes | Menu text. |
| `entries[].position` | string | yes | `general` starts a conversation, `thread` acts on one. The two positions take disjoint sets. |
| `entries[].icon` | string | no | How this entry is recognised in a list. Published as declared. |
| `entries[].profile` | string | no | The profile answering for a `pipeline`/`coordinator` entry. Empty on a `builtin`. |

| ID | Requirement |
|---|---|
| `MGR-9` | The response MUST be UNFILTERED — which entries a transport can express is the adapter's own knowledge, never the manager's. |
| `ADA-6` | An adapter offering a command menu MUST decide for itself what it can express, and MUST learn of a change from the long-poll's `X-Agentops-Vocabulary-Revision` header, never a timer — the manager cannot dial an otherwise-idle adapter any other way. |
| `ADA-6.1` | An adapter whose transport rejects a name's own characters (a hyphen, say) MAY translate it on the way out and back, as long as nothing else in the system learns the substitute name. |

**Commands the manager answers itself** (`/pipelines`, `/help`, `/start`,
`/<pipeline> <task>`, `/close`, `/exit`) are handled before any Pipeline
lookup. An adapter forwards them as ordinary text and implements none of
them — see [concepts](../concepts.md#releasing-a-runtime-by-hand--exit).

## The typed message

**An op carries STRUCTURE, never rendered text.** There is no `op.text` and
no `op.title` — the manager composes meaning, the adapter composes
presentation.

### Message fields

| Field | Type | Required | Description |
|---|---|---|---|
| `kind` | string | yes | `signal`, `answer`, `relay`, or `notice` — see [Enums](#enums). |
| `body` | string | no | Prose, in [the markdown subset](#prose-fields-are-markdown). Present on every kind. |
| `pipeline` | string | no | *(signal)* The route that originated the conversation. Empty when inference is ambiguous — render as absent, never guess a name. |
| `source` | string | no | *(signal)* The `SignalSource` the input came from. |
| `title` | string | no | *(signal)* The conversation title. |
| `labels` | map of string to string | no | *(signal)* The signal's grouping labels. |
| `severity` | string | no | *(signal)* One of `critical`, `error`, `warning`, `info` — see [Enums](#enums). Absent means the originating signal carried none. ADDITIVE: an adapter ignoring it stays fully conformant. |
| `inputRef` | string | no | *(signal)* The `ConversationInput` holding the full payload. |
| `originKind` | string | no | *(signal)* `signal`, `channel`, or `member` — purely additive, for an adapter that distinguishes a coordination member's result from an ordinary signal. |
| `choices` | array of [Choice](#a-message-choice) | no | *(any kind)* Offered actions. Never dropped — they are the reader's only account of what is on offer. |
| `expectsReply` | boolean | no | *(any kind)* This message ASKS the reader for something, because a transport's own command menu sends on tap with nowhere to type the task. |
| `inReplyTo` | string | no | *(any kind)* The transport's OWN handle for the message this one answers, as supplied in the original inbound push. OPAQUE — stored and returned unaltered. |
| `origin` | string | no | *(relay)* The channel the message was typed on. |
| `sender` | string | no | *(relay)* The transport-side identity, when supplied. |
| `status` | string | no | *(answer)* The run outcome — see [Enums](#enums). |
| `level` | string | no | *(notice)* `info` or `warn` — see [Enums](#enums). |

### A message choice

| Field | Type | Required | Description |
|---|---|---|---|
| `label` | string | yes | Names the action in the reader's own terms. |
| `command` | string | yes | The addressed text the choice stands for, so a transport with no controls can print something the reader can type. |

### The topic descriptor

| Field | Type | Required | Description |
|---|---|---|---|
| `conversation` | string | yes | The conversation this thread belongs to. |
| `pipeline` | string | no | Inferred, may be empty — see `Message.pipeline` above. |
| `source` | string | no | The `SignalSource` the input came from. |
| `title` | string | no | A suggested name. The adapter enforces its own limits (Telegram forum topics cap at 128 characters, take no markup). |
| `labels` | map of string to string | no | The signal's grouping labels. |
| `kind` | string | no | The originating signal kind (`alert`, `job`, `task`, `chat`), so an adapter can prefix or icon a topic by lane. |
| `previousThreadId` | string | no | Set only on a REOPEN: the thread this conversation used before its topics were archived. |

| ID | Requirement |
|---|---|
| `ADA-7` | An adapter whose transport can un-archive a thread MUST honour `previousThreadId` and return that same id, so the conversation continues where it left off. One whose transport cannot MUST open a fresh thread and return the new id — ignoring the hint entirely is a valid implementation. |
| `ADA-8` | An adapter MUST render `choices` as controls where its transport has them, and as a plain list where it does not — but MUST NOT drop them. |
| `ADA-9` | An adapter that opens a reply box MUST do so on `expectsReply`, and one that cannot is still conformant — the body prose names what to send. |

### Prose fields are markdown

Prose fields are markdown, in one deliberately small subset:

```
**bold**   *italic*   `inline code`   ```fenced code```   [text](url)
```

Anything outside that subset is UNDEFINED — an adapter may render it, escape
it, or strip it, and no caller may depend on which.

| ID | Requirement |
|---|---|
| `ADA-10` | Rendering, escaping, splitting and truncation are the adapter's ENTIRELY. The manager guarantees nothing about how a message looks or how long it may be. |

## The body grammar

A free-text `answer`/`notice` body is markdown in the subset above, plus a
block grammar owned by the `structured-agent-output` capability — this page
states only WHERE it applies, never restates the rules.

| ID | Requirement |
|---|---|
| `ADA-11` | An adapter MUST parse the grammar on an `answer`/`notice` body, and MUST NOT parse one on a `relay` or a `signal` — a signal is a CARD, rendered by a dedicated renderer, never this grammar. |
| `ADA-11.1` | An adapter not implementing the grammar MAY leave its tags rendering literally — prevented upstream since `AgentProfile.spec.outputFormat` is required, and a profile declaring `none` emits none. |
| `ADA-12` | Parsing MUST be TOTAL. A body with no recognized tag yields one block holding the whole text. |
| `ADA-13` | The manager MUST NOT parse, rewrite, or shorten the body. The section vocabulary is OPEN. |

## Credentials

Declared per `Channel` (`spec.credentialsSecretRef`, a Secret name) and
projected into the adapter pod by the `ChannelAdapter` reconciler as env
vars: Secret key `K` appears as `<credentialEnvPrefix>K`, advertised per
channel in [the channel listing](#get-channelchannels).

| ID | Requirement |
|---|---|
| `MGR-10` | The manager and every reconciler MUST NOT read a Secret through the Kubernetes API. The kubelet resolves the values. |

## Auth

All `/channel/*` calls carry `Authorization: Bearer <token>`.

| Token | Scope |
|---|---|
| Per-adapter, `HMAC(ADAPTER_TOKEN, "adapter:"+name)`, base64url, validated by RE-DERIVATION against the `ChannelAdapter` list | the adapter's own CR name — the type key `Channel`s select. A cross-scope call gets `403` |
| The bare master token (`ADAPTER_TOKEN`, chart-provisioned into the manager) | full scope, so a hand-deployed adapter works unchanged |

| ID | Requirement |
|---|---|
| `SEC-1` | A per-adapter token MUST be derived, never minted or stored — the manager validates a presented token by re-deriving it against every `ChannelAdapter`, stateless across restarts. |
| `SEC-2` | The manager MUST reject an unauthenticated or wrongly-scoped call with `401`/`403` naming which. |
| `SEC-3` | No reconciler MUST grant a channel adapter any Kubernetes permission of its own. `spec.serviceAccountName` is a reference the operator never creates, validates or binds. |

### Discovering what `config` needs

| Field | On `ChannelAdapter` | Is |
|---|---|---|
| `spec.configSchema` | optional | A JSON Schema for the `config` of the `Channel`s/`SignalSource`s it serves. |
| `spec.credentialKeys` | optional | The Secret keys it expects. |

| ID | Requirement |
|---|---|
| `MGR-11` | Both fields are interface METADATA, advisory only — readable the moment the CR is applied, and MUST NOT block serving, projection, or ingestion. |

## Adapter's obligations

| ID | Obligation |
|---|---|
| `ADA-1` | **Consume the `/channel/*` endpoints it needs** — read, acknowledge and report operations, inbound replies, channel listing, cursor state, channel status. |
| `ADA-2` | **Parse the body grammar** — the markdown subset plus the block grammar, under the shared `structured-agent-output` recognition rules. |
| `OPS-18`/`OPS-23` | **Acknowledge an operation before acting on it, and report every one acknowledged, done or failed, before its delivery TTL passes** — see [Acknowledging an operation](#acknowledging-an-operation). |
| `ADA-3`/`ADA-3.1`/`ADA-3.2`/`ADA-3.3`/`ADA-4` | **Absorb retryable backpressure inside the delivery TTL, PER OPERATION never per call**, and **own its outbound pacing** — see [Backpressure](#backpressure-is-the-adapters-to-absorb). |
| `ADA-5` | **Never re-ingest its own outbound post as inbound.** |
| `ADA-6` | **Declare `echoesOwnMessages` truthfully**, and decide for itself what its transport can express from the UNFILTERED vocabulary. |
| `ADA-14` | **Never create, derive, or alter a `conversationId`** — translate it to and from the transport's own thread id, and nothing more. |
| `ADA-15` | **Treat an id matching no published shape as opaque** — see [Delivery operation ids](#delivery-operation-ids-are-published-shapes). |

### Optional capabilities

| Capability | An adapter may | Trigger |
|---|---|---|
| Read reporting | call [`POST /channel/read`](#post-channelread) and track per-reader watermarks | its surface renders unreadness |
| Config/credential metadata | declare [`spec.configSchema`/`spec.credentialKeys`](#discovering-what-config-needs) | the CR wants its interface machine-checked |
| Thread reopening | honour `previousThreadId` and resume the archived thread (`ADA-7`) | its transport can un-archive a thread |
| Name translation | translate a vocabulary name on the way out and back (`ADA-6.1`) | its transport rejects the name's own characters |
| Reset-context | expose [`reset-context`](#acting-on-a-bound-conversation) as a user action | its surface offers that control |
| Op stream | consume [`GET /channel/ops/stream`](#get-channelopsstream) in place of the long-poll | it is a receipt adapter and wants ops pushed |

An adapter implementing none of these stays fully conformant — see
[Conformance](#conformance).

## Manager's provisions

| ID | Provision |
|---|---|
| `MGR-1` | **The ops long-poll** — `GET /channel/ops?adapter=<name>&contract=<version>&wait=<seconds>`, a read that changes nothing, served by any replica. |
| `MGR-2` | **The hand-over and its delivery TTL** — `POST /channel/ops/{id}/ack` hands an op over and starts its delivery TTL. The manager never sends an acknowledged op again, and fails one with no report at its TTL. A failure is final. |
| `MGR-3` | **Typed operations** — `send`, `ensure-topic`, `close-topic`, `delete-conversation`, each a structured payload, never rendered display text. |
| `MGR-4` | **The vocabulary revision** — every long-poll response carries the current revision, and `GET /channel/vocabulary` serves the entries to the same authentication. |
| `MGR-5` | **Derived-token auth** — the master token plus per-adapter tokens derived as `HMAC(masterKey, "adapter:"+name)`, validated by re-derivation, scoped to the adapter's own name. |
| `MGR-6` | **Delivery recording** — completions land on the Conversation: the thread id in `status.threads[]`, the archive in `status.threadsArchived[]`, read watermarks on the thread bindings. |
| `MGR-6.1` | **Delivery exclusion** — an input MUST be delivered to every bound channel except the surface it entered on, unless that surface declares `ChannelAdapter.spec.echoesOwnMessages: false`. |
| `MGR-6.2` | **Idempotent reply ids** — a reply op's id MUST be stable per conversation × channel × run (`send:<conversation>:<channel>:<runId>`), so a manager restart between a run's completion and its delivery re-derives the answer, appending the channel to `status.runs[].delivered[]`. |
| `MGR-12` | **`conversationId` on every op** — each operation delivered on `/channel/ops` additionally carries the conversation's correlation id, ADDITIVE and opaque to the adapter, absent only on a conversation created before the field existed. |
| `MGR-13` | **The op stream** — `GET /channel/ops/stream`, the long-poll's ops pushed to a receipt adapter in queue order, on any replica (`OPS-12` to `OPS-16`). OPTIONAL: an adapter that never calls it is unaffected. |
| `MGR-14` | **The receipt-only exception** — adapters named by `CHANNEL_RECEIPT_ADAPTERS` acknowledge on receipt, with no delivery TTL and no report (`OPS-24` to `OPS-26`). |

## Enums

| Enum | Values | Reading |
|---|---|---|
| `kind` (op) | `ensure-topic`, `send`, `close-topic`, `delete-conversation` | — |
| `kind` (message) | `signal`, `answer`, `relay`, `notice` | An adapter that does not recognise one SHOULD render the body rather than drop the message. |
| `entries[].kind` | `builtin`, `pipeline`, `coordinator` | `builtin` is a manager command, the other two an addressable route. |
| `level` | `info`, `warn` | Two-valued on purpose — an error needing durable recording surfaces as a Conversation condition, not as chat. |
| `status` | `succeeded`, any other string | Reported on an `answer`, from the run's own outcome. |
| `outcome` (read report) | `marked`, `skipped`, `failed` | — |
| `originKind` | `signal`, `channel`, `member` | — |
| `severity` | `critical`, `error`, `warning`, `info` | *(signal)* Absent means the originating signal carried none — see [signal adapter `API-6`](signal-adapter.md#post-signalinbound). |

## Compatibility

| ID | Requirement |
|---|---|
| `COMPAT-1` | An adapter MUST ignore an operation field it does not know. |
| `COMPAT-2` | The manager MUST ignore a field it does not know in a completion, an inbound request, or a read report. |
| `COMPAT-3` | A published section anchor on this page is a stable name. Renaming one is MAJOR. |

| Version part | Moves when |
|---|---|
| MAJOR | an adapter conformant with the previous version stops conforming |
| MINOR | a new optional capability or field is added |
| PATCH | a clarification that changes no obligation |

**The document version is NOT the wire contract version.** The
`contract=<version>` parameter an adapter declares on the long-poll keeps
its own integer series, unrelated to this document's semver:

| Document version | Wire `contract` value |
|---|---|
| 2.0.0 | `2` |
| 1.3.0 | `2` |
| 1.2.0 | `2` |
| 1.1.0 | `2` |
| 1.0.0 | `2` |

A document MINOR bump — an additive field, a new optional endpoint — MUST
NOT move the wire `contract` value. Only a change that an adapter speaking
the current wire version could no longer parse safely moves it, and that
change is itself document-MAJOR.

## Conformance

An adapter implementing every requirement on this page except
[Optional capabilities](#optional-capabilities) is fully conformant — no
optional capability is required.

`severity` on the typed message is purely additive (`COMPAT-2`).

| Adapter | Treats `severity` as |
|---|---|
| `channels/telegram` | an unknown field, ignored |
| `platform/console` | a presentation choice local to its own signal card |

Both stay fully conformant with this version unchanged.

## Changelog

| Version | Date | Changed |
|---|---|---|
| 2.0.0 | 2026-11-13 | **BREAKING.** An outbound operation is taken in three steps: a read that changes nothing, an acknowledgement (`POST /channel/ops/{id}/ack`) that hands it over and starts a per-kind delivery TTL, and a report (`/done`) that is done or failed. The manager never resends an acknowledged op, fails one with no report at its TTL, and a failure is final (`OPS-18` to `OPS-23`, `MGR-2`). `reclaimAfterSeconds` and the claim window are removed. An adapter that never acknowledges is offered the same op forever, so adapter images upgrade together with the manager. Added an optional push stream, `GET /channel/ops/stream`, and the receipt-only exception for the console (`OPS-12` to `OPS-16`, `OPS-24` to `OPS-26`, `MGR-13`, `MGR-14`). The wire `contract` value is unchanged. |
| 1.3.0 | 2026-10-21 | The retry budget is explicitly PER OPERATION, never per transport call — every call an operation makes charges one total (`ADA-3.2`, `ADA-3.3`, `OPS-4`). Published the two delivery operation id shapes, `send:<conversation>:<channel>:<runId>` and `input:<conversation>:<input>:<channel>` (`OPS-10`, `OPS-11`, `ADA-15`) — any other id stays opaque. ADDITIVE: the wire `contract` value is unchanged, and an adapter ignoring the published shapes stays fully conformant. |
| 1.2.0 | 2026-10-21 | The typed message's `signal` kind may carry an optional `severity` — one of `critical`, `error`, `warning`, `info`, mirroring the signal adapter contract's own field. ADDITIVE: the wire `contract` value is unchanged, and an adapter ignoring the field stays fully conformant. |
| 1.1.0 | 2026-10-18 | Every `/channel/ops` operation additionally carries the conversation's `conversationId` (`OPS-9`, `MGR-12`), opaque to the adapter. A channel adapter MUST NOT mint one (`ADA-14`, `IN-4`) — it only translates it to and from the transport's thread id. ADDITIVE: the wire `contract` value is unchanged, and an adapter ignoring the field stays fully conformant. |
| 1.0.0 | 2026-10-17 | Initial release. |
