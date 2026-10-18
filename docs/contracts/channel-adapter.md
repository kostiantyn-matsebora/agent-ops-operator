---
title: ChannelAdapter specification
permalink: /contracts/channel-adapter/
description: >-
  The versioned, normative specification of what a ChannelAdapter owes the
  manager, and what the manager owes it — for building a channel adapter
  agent-ops does not ship.

next:
  eyebrow: Reference
  title: Every field of every kind
  body: >-
    The generated custom resource reference, and the contracts the adapter and
    runtime kinds serve.
  url: https://github.com/kostiantyn-matsebora/agent-ops-operator/blob/master/docs/cr-reference.md
---

**Version 1.0.0**

This specification gives everything necessary to build and maintain a
`ChannelAdapter` — a component agent-ops does not ship, that carries a
conversation onto a chat surface (Telegram, a web console, Slack, anything
with a reply box).

## Conventions

MUST / MUST NOT / SHOULD / MAY follow RFC 2119. Every requirement carries a
stable ID, prefixed by the section it belongs to:

| Prefix | Section |
|---|---|
| `OPS-n` | [GET /channel/ops](#get-channelops) / [POST /channel/ops/{id}/done](#post-channelopsiddone) |
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

Long-polls for this adapter's next outbound operation.

`GET $MANAGER_URL/channel/ops?adapter=<name>&contract=2&wait=25`

| Param | Required | Meaning |
|---|---|---|
| `adapter` | yes | This `ChannelAdapter` CR's own name — the routing key. |
| `contract` | yes | The outbound message contract version this adapter speaks. |
| `wait` | no | Seconds to block. Clamped to 30 server-side. |

| Status | Body |
|---|---|
| `200` | A [claimed op](#the-claim-window), below. |
| `204` | No work yet. Not an error. |
| `400` | `adapter` or `contract` missing or outdated — the message names the version to build against. |
| `401` | no or wrong bearer token. |
| `403` | the token's scope names a different adapter. |
| `503` | this replica is not the current leader, and the leader could not be reached. |

**Every response carries `X-Agentops-Vocabulary-Revision`** — the `200`
that delivers an op, the `204` and the `503` alike. See
[GET /channel/vocabulary](#get-channelvocabulary).

### The operation envelope

| Field | Type | Required | Description |
|---|---|---|---|
| `id` | string | yes | Dedup key. Delivery is at-least-once — tolerate a repeated id. |
| `channel` | string | yes | The `Channel` CR this op targets. |
| `conversation` | string | no | The `Conversation` this op belongs to. Empty for a manager notice bound to no conversation. |
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
| `OPS-2` | A `503` MUST be treated exactly like a `204` — retried at once, on a fresh connection, never surfaced as an error. |
| `OPS-3` | An adapter not implementing a kind MAY complete it with an `error`, or ignore it. |

### The claim window

It embeds every field of [the operation envelope](#the-operation-envelope),
plus:

| Field | Type | Required | Description |
|---|---|---|---|
| `reclaimAfterSeconds` | integer | yes | How long the manager leaves this claim with the adapter before returning the op to the queue. |

| ID | Requirement |
|---|---|
| `OPS-4` | An adapter absorbing transport backpressure in-process MUST budget its total wait strictly below `reclaimAfterSeconds`, or a second claimant may post the same op again. |
| `OPS-5` | `reclaimAfterSeconds` is ADDITIVE and optional to act on. An adapter that ignores it is fully conformant, falling back to its own conservative default. |

**The claim itself is recorded on the Conversation**, as
`status.threads[].claim` (`holder`, `claimedAt`), written only by the
manager's current leader.

A claim older than `claimStalenessSeconds` is cleared and the op
dispatched again. A claim held by a former leader is cleared at once.
Neither is the adapter's concern.

## POST /channel/ops/{id}/done

Completes a claimed op.

`POST $MANAGER_URL/channel/ops/{id}/done`

| Status | Body |
|---|---|
| `200` | `{"ok": true}`. Duplicate completions are legal (at-least-once) and always `200`. |
| `400` | malformed JSON. |

### The completion result

| Field | Type | Required | Description |
|---|---|---|---|
| `threadId` | string | no | Required for `ensure-topic` — the opaque thread id the adapter minted, in its own id space. |
| `error` | string | no | Set on failure. Surfaced as a Conversation condition and regenerated, except `close-topic` (below). |

| ID | Requirement |
|---|---|
| `OPS-6` | An `ensure-topic` completed with no `error` and no `threadId` MUST be treated by the manager as a failure. |
| `OPS-7` | `close-topic`'s failure MUST be logged rather than written as a condition — its conversation is being deleted, so no object will survive to carry one — and the op MUST NOT be regenerated. |
| `OPS-8` | `delete-conversation`'s failure is TERMINAL: logged, never a condition, never regenerated. Deletion MUST NOT wait longer than a 2-minute grace, so a down adapter cannot wedge it. An already-closed thread is treated as success. |

### Backpressure is the adapter's to absorb

A transport that says "slow down" — HTTP 429, a `retry_after`, a
token-bucket rejection — is not an operation failure.

| ID | Requirement |
|---|---|
| `ADA-3` | An adapter MUST absorb a retryable backpressure signal inside its claim window, waiting the interval the transport stated rather than a backoff it computed instead. |
| `ADA-3.1` | An adapter MUST report an op failed only when the condition is terminal, or when riding it out would overrun the claim. |
| `ADA-4` | An adapter MUST pace its own sending so bursts are spread rather than rejected, and MUST gate pacing before it CLAIMS an op, never before it sends. |

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
| `ADA-5` | An adapter MUST NOT re-ingest its own outbound post as inbound. Delivery to the other bound surfaces, and busy-acks, happen manager-side — a reply the adapter pushes is delivered back to it only when it declares `echoesOwnMessages: false`. |

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
| `ADA-1` | **Consume the `/channel/*` endpoints it needs** — poll, complete, inbound replies, channel listing, cursor state, channel status. |
| `ADA-2` | **Parse the body grammar** — the markdown subset plus the block grammar, under the shared `structured-agent-output` recognition rules. |
| `ADA-3`/`ADA-3.1`/`ADA-4` | **Absorb retryable backpressure inside the claim window**, and **own its outbound pacing** — see [Backpressure](#backpressure-is-the-adapters-to-absorb). |
| `ADA-5` | **Never re-ingest its own outbound post as inbound.** |
| `ADA-6` | **Declare `echoesOwnMessages` truthfully**, and decide for itself what its transport can express from the UNFILTERED vocabulary. |

### Optional capabilities

| Capability | An adapter may | Trigger |
|---|---|---|
| Read reporting | call [`POST /channel/read`](#post-channelread) and track per-reader watermarks | its surface renders unreadness |
| Config/credential metadata | declare [`spec.configSchema`/`spec.credentialKeys`](#discovering-what-config-needs) | the CR wants its interface machine-checked |
| Reclaim-aware pacing | budget its own wait inside `reclaimAfterSeconds` (`OPS-5`) | it absorbs backpressure in-process |
| Thread reopening | honour `previousThreadId` and resume the archived thread (`ADA-7`) | its transport can un-archive a thread |
| Name translation | translate a vocabulary name on the way out and back (`ADA-6.1`) | its transport rejects the name's own characters |
| Reset-context | expose [`reset-context`](#acting-on-a-bound-conversation) as a user action | its surface offers that control |

An adapter implementing none of these stays fully conformant — see
[Conformance](#conformance).

## Manager's provisions

| ID | Provision |
|---|---|
| `MGR-1` | **The ops long-poll** — `GET /channel/ops?adapter=<name>&contract=<version>&wait=<seconds>`, at-least-once delivery, duplicates tolerated by `id`. |
| `MGR-2` | **The claim window** — a claimed op carries `reclaimAfterSeconds`, and an op not completed within it returns to the queue for another claimant. |
| `MGR-3` | **Typed operations** — `send`, `ensure-topic`, `close-topic`, `delete-conversation`, each a structured payload, never rendered display text. |
| `MGR-4` | **The vocabulary revision** — every long-poll response carries the current revision, and `GET /channel/vocabulary` serves the entries to the same authentication. |
| `MGR-5` | **Derived-token auth** — the master token plus per-adapter tokens derived as `HMAC(masterKey, "adapter:"+name)`, validated by re-derivation, scoped to the adapter's own name. |
| `MGR-6` | **Delivery recording** — completions land on the Conversation: the thread id in `status.threads[]`, the archive in `status.threadsArchived[]`, read watermarks on the thread bindings. |
| `MGR-6.1` | **Delivery exclusion** — an input MUST be delivered to every bound channel except the surface it entered on, unless that surface declares `ChannelAdapter.spec.echoesOwnMessages: false`. |
| `MGR-6.2` | **Idempotent reply ids** — a reply op's id MUST be stable per conversation × channel × run (`send:<conversation>:<channel>:<runId>`), so a manager restart between completion and claim re-derives the answer, appending the channel to `status.runs[].delivered[]`. |

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
| 1.0.0 | `2` |

A document MINOR bump — an additive field, a new optional endpoint — MUST
NOT move the wire `contract` value. Only a change that an adapter speaking
the current wire version could no longer parse safely moves it, and that
change is itself document-MAJOR.

## Conformance

An adapter implementing every requirement on this page except
[Optional capabilities](#optional-capabilities) is fully conformant — no
optional capability is required.

## Changelog

| Version | Date | Changed |
|---|---|---|
| 1.0.0 | 2026-10-17 | Initial release. |
