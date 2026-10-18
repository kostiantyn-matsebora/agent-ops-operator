---
title: SignalAdapter specification
permalink: /contracts/signal-adapter/
description: >-
  The versioned, normative specification of what a SignalAdapter container
  owes the manager, and what the manager owes it — for building an adapter
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
`SignalAdapter` — a container agent-ops does not ship, normalizing one
transport into signals the manager groups into conversations.

**Signals are one-directional.** This is the [channel adapter
contract](channel-adapter.md) minus the ops queue — an adapter pushes, and
has no inbound surface of its own.

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
 "signals": [{"fingerprint": "…", "labels": {}, "title": "…",
              "payload": "…", "kind": "alert"}]}
```

| Status | Body |
|---|---|
| `400` | `{"error": "<message>"}` — missing `source`/`signals`, a signal with no `fingerprint`, or a `chat` signal missing its channel label (`LANE-3`) |
| `401` / `403` | see [Security posture](#security-posture) |
| `200` | `{"queued": <n>, "conversations": <n>, "reason": "<optional>"}` |

### Normalized signal

| Field | Type | Required | Description |
|---|---|---|---|
| `fingerprint` | string | yes | Dedup identity of the recurring THING, never of one occurrence — see `ADP-2`. |
| `labels` | map of string to string | no | Feeds the source's `grouping.signatureLabels`. Bounded to 32 keys, sorted before truncation. |
| `title` | string | no | Overrides the conversation title when this signal opens one. Bounded to 500 runes. |
| `payload` | string | no | The raw content handed to the agent, stored out of band. Never parsed as prose by the manager. |
| `kind` | string | no | Selects the [lane](#lanes). Absent reads as `alert`. |
| `reader` | string | no | `chat` lane only. An opaque, salted hash of the sender — never an identity — used only to stamp their own read watermark so their own message is not shown back to them as unread. |

| ID | Requirement |
|---|---|
| `API-2` | `queued`/`conversations` count the signals accepted and the conversations they touched, which may be fewer than `len(signals)` under cooldown. |
| `API-3` | At-least-once delivery MUST be safe. A re-sent fingerprint within the source's cooldown window collapses rather than opening a second conversation. |

## Lanes

`kind` picks the prompt wrapper and decides how a later signal is keyed when
the source declares no `signatureLabels`.

| `kind` | Subject | Keyed on (no `signatureLabels`) |
|---|---|---|
| `alert` (default) | a problem that recurs | default labels `alertgroup`/`alertname`/`namespace` |
| `job` | a job that recurs | the same default labels |
| `task` | a caller asking once | the signal's own fingerprint |
| `chat` | a person asking once | the signal's own fingerprint |

| ID | Requirement |
|---|---|
| `LANE-1` | `alert` and `job` are recurring-subject lanes — a second signal is more news about the same thing, so it folds into the open conversation and resumes the session. |
| `LANE-2` | `task` and `chat` are one-shot lanes — a second signal is a second request, never folded by the alert-vocabulary default labels. |
| `LANE-3` | A `chat` signal MUST carry the label `agentops.dev/channel`, naming the surface it arrived on, or the post is refused with `400` — its reply has nowhere to go otherwise. |
| `LANE-4` | A `task` signal MUST NOT carry `agentops.dev/channel`. Its reply goes to the claiming Pipeline's `channelRefs`. |
| `LANE-5` | A source declaring `signatureLabels` groups by them in every lane, `task` included — the lane table is a FALLBACK, never an override. |

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
| `ADP-2` | Fingerprint honestly: the same recurring thing MUST reuse the same fingerprint across occurrences, so at-least-once re-sends collapse rather than opening a new conversation each time. |
| `ADP-3` | Normalize and push. An adapter MUST NOT implement dedup, cooldown, window reuse or recurrence itself — those are the manager's, per `MGR-1`. |
| `ADP-4` | An observing adapter MUST NOT emit a signal about agent-ops' own machinery — see [Self-exclusion](#self-exclusion). |

### Self-exclusion

An adapter watching cluster-shaped state can observe agent-ops' own failures
and feed them back in. A runtime pod that fails to start emits a Warning
event, and nothing downstream breaks the loop that follows:

1. The event becomes a signal.
2. The signal opens a Conversation.
3. That Conversation's pod is the next thing to fail, emitting its own
   Warning event.
4. The fingerprint is fresh — the pod name is new — so nothing collapses
   the repeat.

An observing adapter MUST exclude an object before emitting a signal about
it, matching any of:

| ID | Requirement |
|---|---|
| `ADP-5` | Name prefix — the object's name starts with an agent-ops-reserved prefix. |
| `ADP-6` | Owner or label — the object is owned by, or labeled as, an agent-ops kind. |
| `ADP-7` | Own namespace — the object lives in agent-ops' own namespace. |

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
| `MGR-1` | The manager owns grouping: fingerprint cooldown, signature from `labels` × `signatureLabels`, window reuse, and recurrence-on-session. An adapter implements none of it. |
| `MGR-2` | A source's grouping policy MUST be evaluated once, above every Pipeline claiming it — never once per claiming Pipeline, or the first one starves the rest. |
| `MGR-3` | A `SignalSource` naming no served adapter carries `Served=False`. One claimed by no Ready Pipeline carries `Wired=False`, and every signal posted to it is dropped. |
| `MGR-4` | The manager MUST supply every variable in [Environment](#environment). |
| `MGR-5` | This endpoint set is the ONLY way work originates from outside a chat surface. No route names a `Pipeline` directly — a caller posts to a `SignalSource`, and the Ready Pipeline claiming it decides which agent answers. |
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
| `COMPAT-1` | The manager MUST ignore inbound payload fields it does not know. |
| `COMPAT-2` | An adapter MUST ignore source-listing fields it does not know. |
| `COMPAT-3` | A published section anchor on this page is a stable name. Renaming one is MAJOR. |

| Version part | Moves when |
|---|---|
| MAJOR | an adapter conformant with the previous version stops conforming |
| MINOR | a new optional capability or field is added |
| PATCH | a clarification that changes no obligation |

## Conformance

An adapter implementing every requirement on this page except [Optional
capabilities](#optional-capabilities) is fully conformant — no optional
capability is required, see [Compatibility](#compatibility).

## Changelog

| Version | Date | Changed |
|---|---|---|
| 1.0.0 | 2026-10-17 | Initial release. |
