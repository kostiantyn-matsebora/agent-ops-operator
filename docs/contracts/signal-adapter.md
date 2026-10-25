---
title: SignalAdapter specification
permalink: /contracts/signal-adapter/
description: >-
  The versioned, normative specification of what a SignalAdapter container
  owes the manager, and what the manager owes it — for building an adapter
  agent-ops does not ship.

next:
  eyebrow: Reference
  title: Every SignalAdapter field
  body: >-
    The kind's own spec page — a commented example and every field, in the
    sidebar's CRDs group.
  url: /agent-ops-operator/crds/signal-adapter/
---

**Version 2.2.0**

This specification gives everything necessary to build and maintain a
`SignalAdapter` — a container agent-ops does not ship, normalizing one
transport into signals the manager routes into conversations by equality on
the sender's own correlation id.

**Signals are one-directional.** This is the [channel adapter
contract](channel-adapter.md) minus the ops queue — an adapter pushes, and
has no inbound surface of its own.

Every signal carries `initiationId`, the sender's own correlation id.

An adapter either MINTS one where its own aggregation already happens, or
PROPAGATES one verbatim when a round trip hands back a manager-issued
`conversationId` — never deriving, defaulting, or re-computing it.

The manager routes by pure equality and owns no grouping policy of its own.

## Conventions

MUST / MUST NOT / SHOULD / MAY follow RFC 2119. Every requirement carries a
stable ID, prefixed by the section it belongs to:

| Prefix | Section |
|---|---|
| `API-n` | the four signal endpoints |
| `LANE-n` | [Lanes](#lanes) |
| `ADP-n` | [Adapter's obligations](#adapters-obligations) |
| `MGR-n` | [Manager's provisions](#managers-provisions) |
| `SEC-n` | [Security posture](#security-posture) |
| `COMPAT-n` | [Compatibility](#compatibility) |

## GET /signal/sources

Lists this adapter's own `SignalSource`s.

`GET $MANAGER_URL/signal/sources?adapter=$ADAPTER_NAME`

| Status | Body |
|---|---|
| `401` | unauthorized — see [Security posture](#security-posture) |
| `403` | the token's scope does not match `adapter` |
| `200` | an array of sources, below |

**Example — `200`:**

```json
[{"name": "my-source", "config": {"schedule": "*/5 * * * *"},
  "credentialEnvPrefix": "AGENTOPS_CRED_MY_SOURCE_"}]
```

### Source entry

| Field | Type | Required | Description |
|---|---|---|---|
| `name` | string | yes | The `SignalSource`'s own name. |
| `config` | object | no | Opaque to the manager — whatever shape the adapter defines. |
| `credentialEnvPrefix` | string | no | Set only when the source has credentials. Env var `<prefix><key>` holds each key, keyed by the source's name. |

| ID | Requirement |
|---|---|
| `API-1` | An adapter MUST read its sources from this endpoint, never from a CR watch — `config` and credentials are the manager's to serve and validate. |

## POST /signal/inbound

Pushes normalized signals. The only way work originates from outside a chat
surface.

`POST $MANAGER_URL/signal/inbound`

```json
{"source": "my-source",
 "signals": [{"initiationId": "…", "labels": {}, "title": "…",
              "payload": "…", "kind": "alert"}]}
```

| Status | Body |
|---|---|
| `400` | `{"error": "<message>"}` — missing `source`/`signals`, a signal with no `initiationId` (naming the field, never synthesized — `MGR-3`), a `chat` signal missing its channel label (`LANE-2`), or any signal's `severity` outside the four accepted values (`API-6`) |
| `401` / `403` | see [Security posture](#security-posture) |
| `200` | `{"queued": <n>, "conversations": <n>, "reason": "<optional>"}` |

### Normalized signal

| Field | Type | Required | Description |
|---|---|---|---|
| `initiationId` | string | yes | The sender's own correlation id — MINTED where its aggregation already happens, or PROPAGATED verbatim from a round trip carrying a manager-issued `conversationId` back. See `ADP-2`. |
| `labels` | map of string to string | no | Opaque context carried onto the conversation and its topic descriptor. Bounded to 32 keys, sorted before truncation. |
| `title` | string | no | Overrides the conversation title when this signal opens one. Bounded to 500 runes. |
| `payload` | string | no | The raw content handed to the agent, stored out of band. Never parsed as prose by the manager. |
| `kind` | string | no | Selects the [lane](#lanes). Absent reads as `alert`. |
| `reader` | string | no | `chat` lane only. An opaque, salted hash of the sender — never an identity — used only to stamp their own read watermark so their own message is not shown back to them as unread. |
| `severity` | string | no | One of `critical`, `error`, `warning`, `info` — see [Enums](#enums). Absent means unrated, the only state a signal carried before this field existed. Unrelated to any adapter's own `severity` label — see [Compatibility](#compatibility). |

| ID | Requirement |
|---|---|
| `API-2` | `queued`/`conversations` count the signals accepted and the conversations they touched, which may be fewer than `len(signals)` under cooldown. |
| `API-3` | At-least-once delivery MUST be safe. A re-sent `initiationId` within the source's cooldown window collapses rather than opening a second conversation. |
| `API-6` | A `severity` outside the four accepted values MUST refuse the WHOLE batch with `400`, naming the four accepted values. No adapter is required to send it. |

## Lanes

`kind` selects only the prompt template an input dispatches with. Routing
itself is uniform — pure `initiationId`/`conversationId` equality for every
kind, with no manager-side branch by lane.

| `kind` | Subject | Prompt | Recurs when |
|---|---|---|---|
| `alert` (default) | a problem that recurs | the read-only investigation template | the adapter reuses the same `initiationId` for the same recurring problem |
| `job` | a job that recurs | the task-lane prompt | the adapter reuses the same `initiationId` across ticks, resuming the agent |
| `task` | a caller asking once | the task-lane prompt | never by default — each task mints its own `initiationId`, per `ADP-2` |
| `chat` | a person asking once | the task-lane prompt | only when a thread reply propagates the manager-stamped `conversationId` back as `initiationId` |

| ID | Requirement |
|---|---|
| `LANE-1` | `kind` picks only the prompt template. Whether a second signal recurs onto the open conversation is decided by id equality alone (`MGR-2`), never by `kind`. |
| `LANE-2` | A `chat` signal MUST carry the label `agentops.dev/channel`, naming the surface it arrived on, or the post is refused with `400` — its reply has nowhere to go otherwise. |
| `LANE-3` | A `task` signal MUST NOT carry `agentops.dev/channel`. Its reply goes to the claiming Pipeline's `channelRefs`. |

## GET /signal/state/{source}/{key}

Reads one persisted cursor.

`GET $MANAGER_URL/signal/state/<source>/<key>`

| Status | Body |
|---|---|
| `404` | `source` names no `SignalSource` this token may read |
| `200` | `{"value": "<string, empty if never set>"}` |

## PUT /signal/state/{source}/{key}

Persists one cursor, replacing any value already stored.

`PUT $MANAGER_URL/signal/state/<source>/<key>`

```json
{"value": "…"}
```

| Status | Body |
|---|---|
| `400` | invalid JSON |
| `404` | `source` names no `SignalSource` this token may read |
| `200` | `{"ok": true}` |

| ID | Requirement |
|---|---|
| `API-4` | The value is opaque to the manager — a byte string, not a schema. |
| `API-5` | An adapter MUST persist cursors and offsets here rather than on local disk. Its pod is replaceable. |

## POST /signal/sources/{name}/status

Reports a configuration problem (or clears one) on the source's `Ready`
condition.

`POST $MANAGER_URL/signal/sources/<name>/status`

```json
{"ready": false, "reason": "BadSchedule", "message": "…"}
```

| Field | Type | Required | Description |
|---|---|---|---|
| `ready` | boolean | yes | Sets `Ready` to `True`/`False`. |
| `reason` | string | no | Defaults to `AdapterReady`/`AdapterError`. |
| `message` | string | no | Free text surfaced on the condition. |

| Status | Body |
|---|---|
| `400` | invalid JSON |
| `404` | `name` names no `SignalSource` this token may read |
| `200` | `{"ok": true}` |

## Adapter's obligations

| ID | Requirement |
|---|---|
| `ADP-1` | Read sources from [`GET /signal/sources`](#get-signalsources) — never assume connectivity or credentials, both are the manager's to serve. |
| `ADP-2` | Supply `initiationId` one of two ways: MINTED where the adapter's own aggregation already happens — the same recurring problem reusing the same id across occurrences — or PROPAGATED verbatim when a round trip hands back a manager-issued `conversationId`, with no re-derivation. For example, a chat reply in a bound thread propagates the `conversationId` stamped on that binding, and an adapter fronting an external system that accepts input can carry a `conversationId` outward and propagate it back unchanged when that system's response arrives — both are the propagated case, not a special lane. The manager never determines an id and treats it opaquely either way. |
| `ADP-3` | Normalize and push. An adapter MUST NOT implement dedup, cooldown, or recurrence itself — those are the manager's, per `MGR-2`. The one named carve-out is the [post-emit refractory window](#post-emit-refractory-window), `ADP-8`. |
| `ADP-4` | An observing adapter MUST NOT emit a signal about agent-ops' own machinery — see [Self-exclusion](#self-exclusion). |

### Self-exclusion

An adapter watching cluster-shaped state can observe agent-ops' own failures
and feed them back in. A runtime pod that fails to start emits a Warning
event, and nothing downstream breaks the loop that follows:

1. The event becomes a signal.
2. The signal opens a Conversation.
3. That Conversation's pod is the next thing to fail, emitting its own
   Warning event.
4. The `initiationId` is fresh — the pod name is new — so nothing collapses
   the repeat.

An observing adapter MUST exclude an object before emitting a signal about
it, matching any of:

| ID | Requirement |
|---|---|
| `ADP-5` | Name prefix — the object's name starts with an agent-ops-reserved prefix. |
| `ADP-6` | Owner or label — the object is owned by, or labeled as, an agent-ops kind. |
| `ADP-7` | Own namespace — the object lives in agent-ops' own namespace. |

### Post-emit refractory window

The one carve-out beside `ADP-3`. It bounds an adapter's own re-post chatter
after it emits, and decides nothing about recurrence.

| ID | Requirement |
|---|---|
| `ADP-8` | An adapter MAY suppress its own re-posts of a signal it emitted, for a window after that emit, only within every bound below. Outside them `ADP-3` applies as written, and an undeclared suppression is a conformance gap. |

| Bound | Rule |
|---|---|
| post-emit only | The window opens only when the adapter itself emitted a signal. A condition never reported is never suppressed. |
| one id | It suppresses only re-posts carrying the emitted signal's own `initiationId`, never collapsing distinct ids. |
| length | No longer than the emitting rule's own dwell, with a floor of one minute. |
| no substitute | The manager's cooldown stays the dedup authority. The window only saves round trips cooldown would have absorbed. |
| declared | An adapter relying on the window MUST declare it, with its length, in its own specification. |

A condition still recurring after the window closes is re-posted under the
same `initiationId`, and the manager routes it as a recurrence (`MGR-2`).

`signals/ha` and `signals/k8s-events` declare the window, each sized to the
emitting rule's own dwell, floored at one minute. No other shipped adapter
uses it.

### Security posture

| ID | Requirement |
|---|---|
| `SEC-1` | Every call MUST carry `Authorization: Bearer $ADAPTER_TOKEN`. The master token has full scope. A per-`SignalAdapter` derived token is scoped to that adapter's own name, in a derivation context distinct from channel adapters — the two never share a token even sharing a name. |
| `SEC-2` | `ADAPTER_TOKEN` MUST be derived, never minted or stored. The manager re-derives and compares in constant time, reading no Secret. |
| `SEC-3` | The operator MUST grant an adapter no Kubernetes permissions of its own. `spec.serviceAccountName` is a reference only. |

### Optional capabilities

| Capability | An adapter may | Trigger |
|---|---|---|
| Cursor persistence | use [`GET`/`PUT /signal/state`](#get-signalstatesourcekey) instead of local disk | the transport is polled rather than pushed, so a restart needs an offset |
| Push mode | bind `LISTEN_ADDR` and serve inbound webhooks | `SignalAdapter.spec.port` is set |
| Status reporting | report config problems via [`POST /signal/sources/{name}/status`](#post-signalsourcesnamestatus) | the adapter's own `config` can be invalid at runtime |

An adapter implementing none of these stays fully conformant — see
[Compatibility](#compatibility).

## Manager's provisions

| ID | Requirement |
|---|---|
| `MGR-1` | The manager owns the one inbound endpoint: `POST /signal/inbound` accepts every normalized signal, and it is the ONLY way work originates from outside a chat surface. No route names a `Pipeline` directly — a caller posts to a `SignalSource`, and the Ready Pipeline claiming it decides which agent answers. |
| `MGR-2` | Routing and cooldown are pure id equality, evaluated once per source above every claiming Pipeline — never once per Pipeline, or the first one starves the rest. An OPEN conversation whose `conversationId` equals the signal's `initiationId`, for the claiming pipeline, receives the signal as a recurrence. Otherwise a new conversation is created, adopting the `initiationId` as its `conversationId`. Cooldown keys on `initiationId`. |
| `MGR-3` | The manager MUST NOT mint, derive, or default an `initiationId`. A signal arriving without one is rejected `4xx`, naming the missing field — including from a `1.x` adapter during rollout. There is no compatibility shim: a manager-synthesized id would re-mint grouping policy the manager no longer owns. |
| `MGR-4` | A `SignalSource` naming no served adapter carries `Served=False`. One claimed by no Ready Pipeline carries `Wired=False`, and every signal posted to it is dropped. |
| `MGR-5` | The manager MUST supply every variable in [Environment](#environment). |
| `MGR-6` | The manager authenticates every call by `SEC-1`: the master token, or a per-`SignalAdapter` derived token scoped to the adapter's own name. |
| `MGR-7` | An adapter needs no Kubernetes access: source listing, cursor state and status reporting reach it through this page's own endpoints. |

## Environment

| Kind | Variable | Meaning |
|---|---|---|
| always | `MANAGER_URL` | Where to read sources and push signals. |
| always | `ADAPTER_NAME` | This adapter's own name — the `adapter=` scope on every call. |
| always | `ADAPTER_TOKEN` | Bearer credential — see `SEC-1`. |
| conditional | `LISTEN_ADDR` | Set only when `SignalAdapter.spec.port` is set — this adapter is pushed to rather than polling, and the reconciler owns the matching Service. |
| per-source | `AGENTOPS_CRED_<SOURCE>_<KEY>` | A source's own credentials, keyed by its sanitized name — see `credentialEnvPrefix` on the [source entry](#source-entry). |

## Enums

| Enum | Values | Reading |
|---|---|---|
| `kind` | `alert`, `job`, `task`, `chat` | Absent or unrecognized reads as `alert`. |
| `severity` | `critical`, `error`, `warning`, `info` | Absent means unrated. Unrecognized refuses the whole batch with `400` (`API-6`). |

## Bounds

| Bound | Value |
|---|---|
| `labels` keys per signal | ≤ 32, sorted, extra keys dropped |
| `title` | truncated to 500 runes |
| request body, `POST /signal/inbound` | ≤ 1 MiB |
| request body, `PUT /signal/state/{source}/{key}` | ≤ 1 KiB |

## Compatibility

| ID | Requirement |
|---|---|
| `COMPAT-1` | There is no compatibility shim for a missing `initiationId`. A `1.x` adapter, or any signal with no `initiationId`, gets the same `4xx` naming the field — never a manager-synthesized id, per `MGR-3`. |
| `COMPAT-2` | The manager MUST ignore inbound payload fields it does not know. |
| `COMPAT-3` | An adapter MUST ignore source-listing fields it does not know. |
| `COMPAT-4` | A published section anchor on this page is a stable name. Renaming one is MAJOR. |
| `COMPAT-5` | `severity` is UNRELATED to any adapter-defined `severity` **label**. `signals/k8s-events` and `signals/ha` each write their own `severity` label on their own scale, read by nothing as this field — see [concepts](../concepts.md#signalsource). |

| Version part | Moves when |
|---|---|
| MAJOR | an adapter conformant with the previous version stops conforming |
| MINOR | a new optional capability or field is added |
| PATCH | a clarification that changes no obligation |

## Conformance

An adapter implementing every requirement on this page except [Optional
capabilities](#optional-capabilities) is fully conformant — no optional
capability is required, see [Compatibility](#compatibility).

`severity` is purely additive and optional to send (`COMPAT-2`).

Every shipped adapter claims the version it conforms to on its own page.

| Adapter | Claims its version on | Sends `severity` |
|---|---|---|
| `signals/cron` | `docs/guides/signal-adapter.md` | no |
| `signals/alertmanager` | `docs/integrations/prometheus.md` | no |
| `signals/k8s-events` | `docs/integrations/kubernetes.md` | no |
| `signals/ha` | `docs/integrations/home-assistant.md` | no |
| `signals/pulse` | `docs/integrations/pulse.md` | no |
| `signals/telegram` | `docs/integrations/telegram.md` | no |

All six conform to this version.

## Changelog

| Version | Date | Changed |
|---|---|---|
| 2.2.0 | 2026-10-21 | The [post-emit refractory window](#post-emit-refractory-window) is named as the one carve-out beside `ADP-3` (`ADP-8`): a bounded quiet period on an emitted `initiationId`, declared per adapter. Only `signals/ha` and `signals/k8s-events` use it. `signals/pulse` joins every shipped-adapter enumeration on this page — it was omitted, which never exempted it. ADDITIVE: no adapter needs to change to stay conformant. |
| 2.1.0 | 2026-10-21 | A normalized signal may carry an optional `severity` — one of `critical`, `error`, `warning`, `info` (`API-6`, [Enums](#enums)). Unmapped refuses the whole batch with `400` naming the four values. Unrelated to any adapter's own `severity` **label** (`COMPAT-5`). ADDITIVE: no shipped adapter sends it yet, and none needs to change to stay conformant. |
| 2.0.0 | 2026-10-18 | BREAKING. `fingerprint` is replaced by the sender-minted `initiationId`, required on every signal — minted where the sender's aggregation already happens, or propagated verbatim from a round-tripped `conversationId`. The manager's grouping provision (`labels`×`signatureLabels`, window reuse) is removed — routing is pure `conversationId` equality, and cooldown keys on `initiationId`. A missing `initiationId` is rejected `4xx` naming the field, with no compatibility shim — a `1.x` adapter gets the same rejection. |
| 1.0.0 | 2026-10-17 | Initial release. |
