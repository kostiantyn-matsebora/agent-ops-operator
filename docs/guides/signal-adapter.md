---
title: "React to signals"
permalink: /guides/signal-adapter/
description: >-
  What a signal is, what a signal adapter does and what the manager does
  instead, and how to implement one for a transport agent-ops does not serve.

next:
  eyebrow: Next
  title: "Talk to agents from your own chat"
  body: >-
    Carry a conversation on a transport of your own, and learn why an operation
    carries a typed message rather than rendered text.
  url: /agent-ops-operator/guides/channel-adapter/
---

A **signal** is anything worth an agent's attention. A **signal adapter** watches one transport, normalises what it sees, and pushes it to the manager. **There are no built-in signal types.**

![Your signal adapter watches your transport, mints an initiationId, and posts it to the manager, which matches it by equality and opens or resumes a conversation.]({{ '/assets/img/guides/signal-adapter-light.svg' | relative_url }}){: .ao-diagram}

## Before you start

Write one when nothing shipped (cron, Alertmanager, K8s Events, Home Assistant, Telegram) serves your transport.

{: .ao-callout}
> **Observing adapters must exclude agent-ops objects.** Design breakers first — see [`ADP-4`]({{ '/contracts/signal-adapter/' | relative_url }}#self-exclusion).

## 1. List your sources

```sh
curl -H "Authorization: Bearer $ADAPTER_TOKEN" "$MANAGER_URL/signal/sources?adapter=$ADAPTER_NAME"
```
```powershell
curl -H "Authorization: Bearer $env:ADAPTER_TOKEN" "$env:MANAGER_URL/signal/sources?adapter=$env:ADAPTER_NAME"
```

Each entry carries `config` and `credentialEnvPrefix`.

## 2. Normalise into signals

| Field | Meaning |
|-------|----------|
| `initiationId` | Correlation id for the THING, not occurrence. Same issue reuses same id. For round-trips, propagate `correlationId` verbatim. |
| `signalId` | Id for this MESSAGE, stable across re-sends. Manager stores once per `signalId`. |
| `kind` | Picks the lane. `chat` signals must carry `agentops.dev/channel`. |

## 3. Push them

```json
{"source": "my-source", "signals": [{"initiationId": "…", "signalId": "…", "labels": {}, "title": "…", "payload": "…", "kind": "alert"}]}
```
```sh
curl -X POST -H "Authorization: Bearer $ADAPTER_TOKEN" "$MANAGER_URL/signal/inbound" -d @signals.json
```
```powershell
curl -X POST -H "Authorization: Bearer $env:ADAPTER_TOKEN" "$env:MANAGER_URL/signal/inbound" -d '@signals.json'
```

- `202` acknowledges; `5xx` = re-send with same ids; at-least-once is safe

## 4. Persist cursors (if you poll)

```sh
curl -X PUT -H "Authorization: Bearer $ADAPTER_TOKEN" "$MANAGER_URL/signal/state/my-source/cursor" -d '{"value": "…"}'
```
```powershell
curl -X PUT -H "Authorization: Bearer $env:ADAPTER_TOKEN" "$env:MANAGER_URL/signal/state/my-source/cursor" -d '{"value": "…"}'
```

## 5. Break the loop

If your adapter observes agent-ops objects, implement breakers first. [`signals/k8s-events/selfexclude.go`](https://github.com/kostiantyn-matsebora/agent-ops-operator/blob/master/signals/k8s-events/) uses three: name prefix, owner/label, namespace (only namespace configurable).

## 6. Declare the SignalAdapter

<!-- generated: template kind=SignalAdapter name=my-signals fields=image,port,serviceAccountName,configSchema,credentialKeys,externals comments=off -->
```yaml
apiVersion: agentops.dev/v1alpha1
kind: SignalAdapter
metadata:
  name: my-signals
spec:
  image: <image>
  port: 8080
  serviceAccountName: <serviceAccountName>
  configSchema: {}
  credentialKeys:
  - key: <key>
  externals:
  - kind: sender   # sender | api | kubernetes
    name: <name>
```
<!-- /generated -->

{: .ao-callout}
> **Naming a `serviceAccountName` mounts its token.** No reconciler creates accounts; name nothing = release floor.

## 7. Declare a SignalSource

<!-- generated: template kind=SignalSource name=my-source fields=config,cooldownHours,attributeFilters,logFiltered comments=off -->
```yaml
apiVersion: agentops.dev/v1alpha1
kind: SignalSource
metadata:
  name: my-source
spec:
  adapter: <adapter>
  config: {}
  cooldownHours: 6
  attributeFilters:
  - key: <key>
    operator: In   # In | NotIn | Exists | DoesNotExist
  logFiltered: false
```
<!-- /generated -->

`cooldownHours` = repeated `initiationId` as re-send vs re-notification (default 6). `attributeFilters` narrows routing. List on Pipeline's `signalSourceRefs` or signals drop.

```sh
kubectl -n agent-ops get signalsource my-source; kubectl -n agent-ops logs -l agentops.dev/signal-adapter=my-signals -f
```
```powershell
kubectl -n agent-ops get signalsource my-source; kubectl -n agent-ops logs -l agentops.dev/signal-adapter=my-signals -f
```

## Verify

- `/signal/sources` returns `200`; post same signal twice → both `queued: 1`, one `Signal`
- Cursor `GET` round-trips `PUT` and survives restart
- Breakers work; pod has `MANAGER_URL`/`ADAPTER_NAME`/`ADAPTER_TOKEN`
- `SERVED=True`, `WIRED=True`, `RECEIVED` counts

## What comes next

1. **[The signal adapter specification]({{ '/contracts/signal-adapter/' | relative_url }})** — every field, enum, bound, and the Conformance section
2. **[`signals/cron`](https://github.com/kostiantyn-matsebora/agent-ops-operator/blob/master/signals/cron/)** — dependency-free reference with restart-safe cursors
3. **[Talk to agents from your own chat]({{ '/guides/channel-adapter/' | relative_url }})** — the channel adapter guide
