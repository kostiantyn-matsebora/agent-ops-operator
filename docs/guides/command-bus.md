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

The command bus is a durable queue for work the manager does on request:
close, delete, reset-context, escalate and incoming signals. The manager stores
the request and returns. A manager replica runs it afterwards.

A successful reply therefore means the request was accepted. It does not mean
the work is finished.

## For API callers

The endpoints that queue work now return `invocationId` and `taskIds` in
their reply. `reset-context` and `/signal/inbound` also return `202` instead of
`200`. Signal adapters need no change.

To check the result of a task:

```sh
curl -s http://agentops-manager.agent-ops.svc:8080/state/tasks/$TASK_ID \
  -H "Authorization: Bearer $ADAPTER_TOKEN"
```

```powershell
curl.exe -s http://agentops-manager.agent-ops.svc:8080/state/tasks/$env:TASK_ID `
  -H "Authorization: Bearer $env:ADAPTER_TOKEN"
```

| Task result | What happens |
|---|---|
| Succeeded | The task is removed from its queue. Reading it may return `404`. |
| Failed with `conflict`, `unavailable` or `internal` | Retried with backoff, then dead-lettered. |
| Failed with `not_found`, `precondition_failed`, `forbidden` or `invalid_command` | Dead-lettered immediately. |

Dead-lettered tasks and their error codes are listed at `GET /state/dead-letters`.

## For channel adapter authors

An outbound message is an operation on the adapter's queue. The adapter makes
three calls:

1. `GET /channel/ops` returns the next operation without changing it.
2. `POST /channel/ops/{id}/ack` takes the operation. It is not offered again,
   and a delivery timer starts.
3. `POST /channel/ops/{id}/done` reports success, or failure with an `error`.

If the timer expires first, the operation fails and is not retried. The manager
works out what is still owed from the conversation after a short wait. Timers
are 10 minutes for replies, 2 for notices and 5 for topic operations, set by
`commandBus.deliveryTtl`.

The console acknowledges operations when it receives them. Upgrade adapter
images together with the manager, because an older adapter never acknowledges.

## Configuration

With the default `nats` state store, the bus uses the same server and adds no
pods. With the `kube` state store, it deploys its own three-server NATS bundle.
The backend is set separately from [the state store](/agent-ops-operator/installation/#storage):

```yaml
commandBus:
  backend: nats   # the default
```

<!-- generated: renders bundle=commandbus -->

```text
# Always
```

All values, including an external server and its credential, are in the
[configuration reference](/agent-ops-operator/configuration/#command-bus).

`backend: memory` keeps the queues in the manager process. It is meant for tests
and local development, and is refused with more than one manager replica.

## Limits

- A chat `/close` closes the whole conversation tree as one task.
- Retry attempts, backoff and delivery timers are not yet chart values, except `commandBus.deliveryTtl`.
