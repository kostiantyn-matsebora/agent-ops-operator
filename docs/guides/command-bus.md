---
title: "Accept work durably"
permalink: /guides/command-bus/
description: >-
  What the command bus is, what it changes about a close, delete or
  reset-context call, and how to run it on your own server or in memory.

next:
  eyebrow: Reference
  title: Every command bus value
  body: >-
    `commandBus.*` in full — the external-server option, its credential, and
    what `memory` costs you.
  url: /agent-ops-operator/configuration/#command-bus
---

Closing a conversation, deleting one, resetting its context — every one of
these used to run inside the HTTP request that asked for it. A caller waited
for work it could not observe, and a manager restart mid-request lost
whatever that request was doing.

**The command bus accepts the request durably, then runs it later**, on
whichever replica next picks it up. The reply means accepted, not performed.

## What changes for a caller

| Endpoint | Before | Now |
|---|---|---|
| `reopen`, `delete`, `reset-context`, `/channel/inbound` | the status you already got | the same status, plus `invocationId` and `taskIds` |
| `/coordinate/close`, `/coordinate/escalate` | `200 {"ok":true}` | the same `200`, plus `invocationId` and `taskIds` |

`reset-context` is the one status code that changed, from `200` to `202`.
Every other endpoint's code was already `202` or `200`.

A caller that needs the outcome reads it back:

```sh
curl -s http://agentops-manager.agent-ops.svc:8080/state/tasks/$TASK_ID \
  -H "Authorization: Bearer $ADAPTER_TOKEN"
```

```powershell
curl.exe -s http://agentops-manager.agent-ops.svc:8080/state/tasks/$env:TASK_ID `
  -H "Authorization: Bearer $env:ADAPTER_TOKEN"
```

A task that succeeds is removed from its queue, so reading it afterwards may answer `404`. A failed one settles to `dead`, with a code — `not_found`, `precondition_failed`, `forbidden`, `conflict`, `unavailable`, `invalid_command` or `internal` — on the ones that fail.

`conflict`, `unavailable` and `internal` retry with backoff before giving up. The others go straight to a dead-letter record, listed at `GET /state/dead-letters`.

**`POST /signal/inbound` is unchanged.** It still validates, stores and
replies synchronously.

## What changes for a channel adapter

Every outbound channel operation is a message on its channel's own queue, and
an adapter takes it in three steps.

1. **Read.** `GET /channel/ops` offers the next op and changes nothing.
2. **Acknowledge.** `POST /channel/ops/{id}/ack` hands it over. The manager
   never sends it again, and a delivery TTL starts.
3. **Report.** `POST /channel/ops/{id}/done` says done, or failed with an
   `error`.

```
POST /channel/ops/send:conv-1:tg-ops:run-7/ack?adapter=telegram
200  {"ok": true, "deliveryTtlSeconds": 600, "consumed": false}
```

An op with no report when its TTL passes is failed, and a failure is final. The manager does not retry it. What is still owed is derived again from the conversation after a cool-off.

The TTLs are per kind, 10 minutes for replies, 2 for notices and 5 for topic operations, and they are chart values under `commandBus.deliveryTtl`.

**The console acknowledges on receipt.** It reads `GET /channel/ops/stream`, and
the manager deletes each op at the acknowledgement. Which adapters work this way
is `commandBus.channelReceiptAdapters`. An adapter image from before this
release never acknowledges, so upgrade adapters together with the manager.

## Running it

With the state store on its default `nats`, the command bus shares that server and credential in buckets of its own, and renders nothing second. With the `kube` store it renders a three-server bundle of its own.

Either way it is selected independently of [the state store](/agent-ops-operator/installation/#storage):

```yaml
commandBus:
  backend: nats   # the default
```

<!-- generated: renders bundle=commandbus -->

```text
# Always
```

See [Configuration](/agent-ops-operator/configuration/#command-bus) for
every value, including the external-server and credential options.

**`commandBus.backend: memory`** keeps the queue logs in the manager's own
process instead. It is for tests and local development — refused above one
manager replica, since a second replica's own heap knows nothing the first
one accepted.

## What it does not do yet

- A plain chat `/close` still closes its whole conversation tree in one
  task, not one per conversation.
- An `escalate` on a caused member still runs as one task, not a close
  task followed by a separate result append.
- Attempts, backoff, TTL and the other per-queue knobs have no chart value
  yet — every queue runs at its built-in starting point.

None of these change what a caller sees. They bound how finely
`/state/tasks` can show its progress.
