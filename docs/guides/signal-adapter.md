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
> anything — see
> [Self-exclusion]({{ '/contracts/signal-adapter/' | relative_url }}#self-exclusion).

Review [the signal adapter
specification]({{ '/contracts/signal-adapter/' | relative_url }}) first.

This page builds the smallest thing that satisfies it, station by station,
and every station names the exact section it builds toward. It states no
requirement of its own. Where it and the specification disagree, the
specification wins and the guide is the defect.

## The overall shape

Four pieces, and the last one is somebody else's:

1. **A container** implementing the signal contract — read your sources, watch
   the transport, push normalised signals.
2. **A `SignalAdapter` resource** — image and workload knobs. No configuration,
   no connectivity, no credentials.
3. **A `SignalSource` naming it.** The CR name is the routing key, and your
   implementation defines what its opaque `config` may hold.
4. **A `Pipeline` claiming that source**, or its signals drop with
   `Wired=False` (`MGR-3`).

**You normalise. The manager groups** (`MGR-1`). Do not implement dedup,
cooldown or recurrence yourself:

| You | The manager |
|---|---|
| Read your sources and their opaque `config` | Serves them, and validates against your declared schema |
| Watch the transport | — |
| Normalise into `{fingerprint, labels, title?, payload, kind}` | Groups by signature, applies cooldown, reuses windows, resumes sessions |
| Persist cursors through the state API | Stores them |
| Report config problems | Surfaces them on the source's `Ready` condition |

A source's policy must be evaluated **once, above** every Pipeline that claims
it (`MGR-2`), or the first Pipeline spends the cooldown window and starves the
rest.

The manager gives your pod `MANAGER_URL`, `ADAPTER_NAME` and `ADAPTER_TOKEN` —
the full list is
[Environment]({{ '/contracts/signal-adapter/' | relative_url }}#environment).
Every call below is bearer-authenticated with that token (`SEC-1`).

## 1. List your sources

```sh
curl -H "Authorization: Bearer $ADAPTER_TOKEN" \
  "$MANAGER_URL/signal/sources?adapter=$ADAPTER_NAME"
```

```powershell
curl -H "Authorization: Bearer $env:ADAPTER_TOKEN" `
  "$env:MANAGER_URL/signal/sources?adapter=$env:ADAPTER_NAME"
```

Every entry carries the source's `config` and a `credentialEnvPrefix` naming
where its credentials landed in your environment — see [Source
entry]({{ '/contracts/signal-adapter/' | relative_url }}#source-entry) (`ADP-1`:
read sources from here, never assume connectivity or credentials yourself).

**Verify:** the call returns `200` with an array, empty before you declare a
`SignalSource` and one entry per source afterward (station 7).

## 2. Normalise into signals

```json
{"fingerprint": "…", "labels": {}, "title": "…", "payload": "…", "kind": "alert"}
```

**The fingerprint is the dedup identity of the THING, not of the occurrence**
(`ADP-2`). `signals/k8s-events` keys on the involved object plus the reason,
never on the Event, because Kubernetes recreates Events per recurrence.

`kind` picks the lane — see
[Lanes]({{ '/contracts/signal-adapter/' | relative_url }}#lanes). A `chat`
signal must carry the label `agentops.dev/channel` (`LANE-3`), or the post is
refused — its reply would have nowhere to go.

**Verify:** two signals with the same `fingerprint`, posted to a source with
`cooldownHours` set, collapse into one conversation rather than two (station
3's at-least-once check).

## 3. Push them

```sh
curl -X POST -H "Authorization: Bearer $ADAPTER_TOKEN" \
  "$MANAGER_URL/signal/inbound" -d @signals.json
```

```powershell
curl -X POST -H "Authorization: Bearer $env:ADAPTER_TOKEN" `
  "$env:MANAGER_URL/signal/inbound" -d '@signals.json'
```

See [`POST /signal/inbound`]({{ '/contracts/signal-adapter/' | relative_url }}#post-signalinbound)
for the full request and response shape.

**At-least-once is safe** (`API-3`). Re-sends collapse under the source's
cooldown, so never build your own dedup to avoid them.

**Verify:** post the same signal twice in a row. The response's
`conversations` count stays at one, and `queued` reflects both posts
accepted.

## 4. Persist cursors (if you poll)

A polling adapter needs an offset to survive a restart. Your pod is
replaceable, so keep it on the manager, not on disk — see
[`GET`/`PUT /signal/state/{source}/{key}`]({{ '/contracts/signal-adapter/' | relative_url }}#get-signalstatesourcekey)
(`API-5`, an [optional capability]({{ '/contracts/signal-adapter/' | relative_url }}#optional-capabilities)
a push-based adapter can skip entirely).

```sh
curl -X PUT -H "Authorization: Bearer $ADAPTER_TOKEN" \
  "$MANAGER_URL/signal/state/my-source/cursor" -d '{"value": "…"}'
```

```powershell
curl -X PUT -H "Authorization: Bearer $env:ADAPTER_TOKEN" `
  "$env:MANAGER_URL/signal/state/my-source/cursor" -d '{\"value\": \"…\"}'
```

**Verify:** `GET` the same key back and it round-trips the value you `PUT`,
surviving a restart of your pod.

## 5. Break the loop

If your adapter observes anything agent-ops itself produces, implement the
breakers before the feature (`ADP-4`).
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
architectural error, not merely a noisy one — see
[Self-exclusion]({{ '/contracts/signal-adapter/' | relative_url }}#self-exclusion).

**Verify:** trigger the condition your adapter watches on an agent-ops-owned
object (a runtime pod, say) and confirm no signal is emitted.

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

Set `port` only if your implementation is pushed to rather than polling — the
[push-mode optional capability]({{ '/contracts/signal-adapter/' | relative_url }}#optional-capabilities).
The reconciler then owns the Service `agentops-signal-<name>` and injects
`LISTEN_ADDR`.

`configSchema` and `credentialKeys` are interface metadata, and they are worth
filling in. They let an operator learn what your `config` needs without reading
your source.

`externals` is interface metadata too. It names each system outside the install
your implementation faces, with a `kind` of `sender`, `api` or `kubernetes`.

The console's topology draws each one beside your adapter. **It grants
nothing**: no RBAC, no network policy and no credential follows from it.

{: .ao-callout}
> **`serviceAccountName` is a reference, and naming one mounts its token**
> (`SEC-3`). No reconciler creates a ServiceAccount and none creates RBAC,
> ever — a `SignalAdapter` is an ordinary namespaced object, so an operator
> that could grant one would make CR-edit rights a privilege escalation.
>
> The chart that grants your adapter renders the account beside the grant, so
> both are read from one file. **Name nothing and it runs as the release floor**
> — an account bound to nothing, denied every verb.

**Verify:** `kubectl apply` the object above and confirm your pod starts with
`MANAGER_URL`, `ADAPTER_NAME` and `ADAPTER_TOKEN` set.

## 7. Declare a SignalSource, and claim it

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
`Wired=False` (`MGR-3`) and every signal you push is dropped.

```sh
kubectl -n agent-ops get signalsource my-source
kubectl -n agent-ops logs -l agentops.dev/signal-adapter=my-signals -f
```

```powershell
kubectl -n agent-ops get signalsource my-source
kubectl -n agent-ops logs -l agentops.dev/signal-adapter=my-signals -f
```

**Verify:** `SERVED` goes `True` when your adapter's Deployment is up,
`WIRED` once a Pipeline claims the source, and `RECEIVED` counts up as
signals land.

## What comes next

1. **Read the specification in full** — [the signal adapter
   specification]({{ '/contracts/signal-adapter/' | relative_url }}) covers
   every field, every enum, the bounds and their refusal, and the
   Conformance section a claim of conformance is checked against.
2. **Copy the reference** —
   [`signals/cron`](https://github.com/kostiantyn-matsebora/agent-ops-operator/blob/master/signals/cron/),
   dependency-free Go with a five-field cron parser and restart-safe cursors.
   It conforms to the [signal adapter
   specification]({{ '/contracts/signal-adapter/' | relative_url }}), version
   1.0.0.
3. **Compare a different transport** —
   [`signals/alertmanager`](https://github.com/kostiantyn-matsebora/agent-ops-operator/blob/master/signals/alertmanager/)
   receives webhooks,
   [`signals/k8s-events`](https://github.com/kostiantyn-matsebora/agent-ops-operator/blob/master/signals/k8s-events/)
   watches the API,
   [`signals/ha`](https://github.com/kostiantyn-matsebora/agent-ops-operator/blob/master/signals/ha/)
   reads a WebSocket.
4. **[Talk to agents from your own chat]({{ '/guides/channel-adapter/' | relative_url }})**
   — the other half, for carrying the conversation.
5. **[Every field of both kinds](https://github.com/kostiantyn-matsebora/agent-ops-operator/blob/master/docs/cr-reference.md#signaladapter)**.
