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

A **signal** is anything worth an agent's attention — an alert firing, a
schedule coming due, a log line, somebody typing. A **signal adapter** is the
container that watches one transport, normalises what it sees, and pushes the
result to the manager.

**There are no built-in signal types.** The manager hosts no transports, so
every kind of thing that can originate a conversation has an adapter behind
it.

![Your signal adapter watches your transport and posts normalised signals to the manager, which groups them and opens a conversation.]({{ '/assets/img/guides/signal-adapter-light.svg' | relative_url }}){: .ao-diagram}

## Before you start

Writing one is appropriate when:

- Nothing here serves your **transport**. The shipped adapters are cron,
  Alertmanager webhooks, Kubernetes Events, Home Assistant logs and Telegram.
- You need a different **normalisation** of a transport that is served.

It is **not** what you want when you need a new source of an **existing** type.
That is a `SignalSource` naming the adapter that already serves it, and no code
at all.

{: .ao-callout}
> **An observing adapter must never emit a signal about agent-ops' own
> machinery.** A runtime pod that fails to start emits a Warning event, that
> event becomes a signal, the signal opens a Conversation, and that creates
> another runtime pod under a new name. Design for this before you write
> anything.

**Nothing downstream catches that loop.** The fingerprint is fresh because the
pod name is new, and the workload is fresh because its owner is a new
Conversation.

Even a liveness re-check passes, because the pod really is broken. The caps
bound pods and backlog, so it fills etcd more slowly and never stops.

Review
[the signal adapter contract](https://github.com/kostiantyn-matsebora/agent-ops-operator/blob/master/docs/contracts.md#the-signal-adapter-contract)
first.

## The overall shape

Four pieces, and the last one is somebody else's:

1. **A container** implementing the signal contract — read your sources, watch
   the transport, push normalised signals.
2. **A `SignalAdapter` resource** — image and workload knobs. No configuration,
   no connectivity, no credentials.
3. **A `SignalSource` naming it.** The CR name is the routing key, and your
   implementation defines what its opaque `config` may hold.
4. **A `Pipeline` claiming that source**, or its signals drop with
   `Wired=False`.

**You normalise. The manager groups.** Do not implement dedup, cooldown or
recurrence yourself:

| You | The manager |
|---|---|
| Read your sources and their opaque `config` | Serves them, and validates against your declared schema |
| Watch the transport | — |
| Normalise into `{fingerprint, labels, title?, payload, kind}` | Groups by signature, applies cooldown, reuses windows, resumes sessions |
| Persist cursors through the state API | Stores them |
| Report config problems | Surfaces them on the source's `Ready` condition |

A source's policy must be evaluated **once, above** every Pipeline that claims
it, or the first Pipeline spends the cooldown window and starves the rest.

## Read your sources

Your pod is given `MANAGER_URL`, `ADAPTER_NAME` and `ADAPTER_TOKEN`. Every call
is bearer-authenticated with that token.

```sh
curl -H "Authorization: Bearer $ADAPTER_TOKEN" \
  "$MANAGER_URL/signal/sources?adapter=$ADAPTER_NAME"
```

```powershell
curl -H "Authorization: Bearer $env:ADAPTER_TOKEN" `
  "$env:MANAGER_URL/signal/sources?adapter=$env:ADAPTER_NAME"
```

Each entry carries the source's `config` — whatever shape you defined — and a
`credentialEnvPrefix` naming where that source's credentials landed in your
environment.

## Normalise into signals

```json
{"source": "my-source",
 "signals": [{"fingerprint": "…", "labels": {}, "title": "…",
              "payload": "…", "kind": "alert"}]}
```

**The fingerprint is the dedup identity of the THING, not of the occurrence.**
`signals/k8s-events` keys on the involved object plus the reason, never on the
Event, because Kubernetes recreates Events per recurrence.

**`kind` says what sort of work this is.** It picks the prompt wrapper the agent
receives — `contracts.md` calls these **lanes** — and it decides how a later
signal is keyed when the source declares no `signatureLabels`:

| `kind` | Subject | Keyed on |
|---|---|---|
| `alert` | a problem that recurs | default alert labels — group and resume |
| `job` | a job that recurs | the same, so successive ticks fold into one conversation |
| `task` | a caller asking once | the signal's own fingerprint |
| `chat` | a person asking once | the signal's own fingerprint |

A `chat` signal **must** carry the label `agentops.dev/channel`, or it is
refused. Its reply would have nowhere to go.

## Push them

```sh
curl -X POST -H "Authorization: Bearer $ADAPTER_TOKEN" \
  "$MANAGER_URL/signal/inbound" -d @signals.json
```

```powershell
curl -X POST -H "Authorization: Bearer $env:ADAPTER_TOKEN" `
  "$env:MANAGER_URL/signal/inbound" -d '@signals.json'
```

**At-least-once is safe.** Re-sends collapse under the source's cooldown, so
never build your own dedup to avoid them.

Persist poll offsets and cursors through `GET`/`PUT
/signal/state/{source}/{key}` rather than on disk. Your pod is replaceable.

## Break the loop

If your adapter observes anything agent-ops itself produces, implement the
breakers before the feature.
[`signals/k8s-events/selfexclude.go`](https://github.com/kostiantyn-matsebora/agent-ops-operator/blob/master/signals/k8s-events/)
uses three, independently:

| Breaker | Why it is separate |
|---|---|
| Name prefix | needs no API read, so it holds with a cold cache |
| Owner and label | catches what a rename would slip past |
| Own namespace | the coarse net under both |

**Only the third is configurable.** A deny-list is editable, and an editable
loop breaker is not one.

**agent-ops' own health is status, not signal.** The reconciler already holds
the failure. Routing it back through ingest to open a conversation is an
architectural error, not merely a noisy one.

## Declare the SignalAdapter

<!-- generated: template kind=SignalAdapter name=my-signals fields=image,port,kubernetesAccess,configSchema,credentialKeys comments=off -->
```yaml
apiVersion: agentops.dev/v1alpha1
kind: SignalAdapter
metadata:
  name: my-signals
spec:
  image: <image>
  port: 8080
  kubernetesAccess: false
  configSchema: {}
  credentialKeys:
  - key: <key>
```
<!-- /generated -->

Set `port` only if your implementation is **pushed to** rather than polling. The
reconciler then owns the Service `agentops-signal-<name>` and injects
`LISTEN_ADDR`.

`configSchema` and `credentialKeys` are interface metadata, and they are worth
filling in. They let an operator learn what your `config` needs without reading
your source.

{: .ao-callout}
> **`kubernetesAccess` mounts a token. It grants nothing.** No reconciler
> creates RBAC, ever. What your adapter may do is an external grant against
> ServiceAccount `agentops-signal-<name>` — from the chart, or from you.

## Declare a SignalSource, and claim it

<!-- generated: template kind=SignalSource name=my-source fields=config,grouping comments=off -->
```yaml
apiVersion: agentops.dev/v1alpha1
kind: SignalSource
metadata:
  name: my-source
spec:
  adapter: <adapter>
  config: {}
  grouping:
    cooldownHours: 6
    signatureLabels:
    - <signatureLabel>
    windowDays: 7
```
<!-- /generated -->

Then list that source on a Ready Pipeline's `signalSourceRefs`. Until you do,
`Wired=False` and every signal you push is dropped.

## Verify

```sh
kubectl -n agent-ops get signalsource my-source
kubectl -n agent-ops logs -l agentops.dev/signal-adapter=my-signals -f
```

```powershell
kubectl -n agent-ops get signalsource my-source
kubectl -n agent-ops logs -l agentops.dev/signal-adapter=my-signals -f
```

`SERVED` goes `True` when your adapter's Deployment is up, `WIRED` when a
Pipeline claims the source, and `RECEIVED` counts up as signals land.

## What comes next

1. **Copy the reference** —
   [`signals/cron`](https://github.com/kostiantyn-matsebora/agent-ops-operator/blob/master/signals/cron/),
   dependency-free Go with a five-field cron parser and restart-safe cursors.
2. **Compare a different transport** —
   [`signals/alertmanager`](https://github.com/kostiantyn-matsebora/agent-ops-operator/blob/master/signals/alertmanager/)
   receives webhooks,
   [`signals/k8s-events`](https://github.com/kostiantyn-matsebora/agent-ops-operator/blob/master/signals/k8s-events/)
   watches the API,
   [`signals/ha`](https://github.com/kostiantyn-matsebora/agent-ops-operator/blob/master/signals/ha/)
   reads a WebSocket.
3. **[Talk to agents from your own chat]({{ '/guides/channel-adapter/' | relative_url }})**
   — the other half, for carrying the conversation.
4. **[Every field of both kinds](https://github.com/kostiantyn-matsebora/agent-ops-operator/blob/master/docs/cr-reference.md#signaladapter)**.
