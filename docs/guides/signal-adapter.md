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

![Your signal adapter watches your transport, mints an initiationId, and posts it to the manager, which matches it by equality and opens or resumes a conversation.]({{ '/assets/img/guides/signal-adapter-light.svg' | relative_url }}){: .ao-diagram}

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
   `Wired=False` (`MGR-4`).

**You normalise and correlate. The manager only compares** (`MGR-2`). Do not
implement dedup, cooldown or recurrence yourself:

| You | The manager |
|---|---|
| Read your sources and their opaque `config` | Serves them, and validates against your declared schema |
| Watch the transport | — |
| Normalise into `{initiationId, labels, title?, payload, kind}`, minting or propagating the id | Routes by `initiationId`/`conversationId` equality, applies cooldown, resumes sessions |
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
{"initiationId": "…", "signalId": "…", "labels": {}, "title": "…", "payload": "…", "kind": "alert"}
```

**`signalId` is your id for this one MESSAGE** (`ADP-9`, `API-7`). Keep it
stable across every re-send of the message, including after a `5xx`, and
different for two messages even when they share an `initiationId`. The
manager stores a message once per `signalId`. A stable cron tick, a webhook
body hash or an event uid and count all work.

**`initiationId` is your own correlation id for the THING, not the
occurrence** (`ADP-2`). Mint it where your aggregation already decides "this
is the same problem" — the same recurring issue reuses the same id across
occurrences.

`signals/k8s-events` keys on the owning workload alone, never the Event
object or its reason. Kubernetes recreates Event objects for every
recurrence, and a workload can fail for several reasons across its lifetime.

A round trip is the other case: when your adapter fronts a system that hands
a manager-issued `conversationId` back to you, propagate it verbatim as the
next signal's `initiationId` — never re-derive it.

`kind` picks the lane — see
[Lanes]({{ '/contracts/signal-adapter/' | relative_url }}#lanes). A `chat`
signal must carry the label `agentops.dev/channel` (`LANE-3`), or the post is
refused — its reply would have nowhere to go.

A signal may also carry `severity` — one of `critical`, `error`, `warning`,
`info` — see
[Normalized signal]({{ '/contracts/signal-adapter/' | relative_url }}#normalized-signal).
No shipped adapter sets it yet, so leaving it out is correct, not
incomplete.

**Verify:** every signal in your batch names a `kind` from the table above,
and any `chat` one carries `agentops.dev/channel` — station 3 is where the
refusal would otherwise surface.

## 3. Push them

Wrap the signals in the envelope naming your source:

```json
{"source": "my-source",
 "signals": [{"initiationId": "…", "labels": {}, "title": "…",
              "payload": "…", "kind": "alert"}]}
```

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

**The response acknowledges storage, not routing** (`API-2`). `200` is
`{"queued": n}`: the signals the manager stored or deduplicated. It routes
them afterwards, so the response never says whether a conversation opened or
the source was wired. A `5xx` means nothing was acknowledged: re-send the
batch with the same ids.

**At-least-once is safe** (`API-3`). A re-send with the same `signalId` is
stored once, and one without is resolved by the source's cooldown, so never
build your own dedup to avoid them.

**Verify:** post the same signal, `signalId` included, twice in a row. Both
posts answer `queued: 1`, and `kubectl get signals` lists one `Signal`.

Repeating the same `initiationId` after `cooldownHours` has passed still
lands on that conversation while it stays OPEN — closing it first opens a
fresh one on the next repeat instead.

## 4. Persist cursors (if you poll)

A polling adapter needs an offset to survive a restart. Your pod is
replaceable, so keep it on the manager, not on disk.

- Read it with [`GET`]({{ '/contracts/signal-adapter/' | relative_url }}#get-signalstatesourcekey).
- Write it with [`PUT /signal/state/{source}/{key}`]({{ '/contracts/signal-adapter/' | relative_url }}#put-signalstatesourcekey) (`API-5`).

This is an [optional capability]({{ '/contracts/signal-adapter/' | relative_url }}#optional-capabilities).
A push-based adapter can skip it entirely.

```sh
curl -X PUT -H "Authorization: Bearer $ADAPTER_TOKEN" \
  "$MANAGER_URL/signal/state/my-source/cursor" -d '{"value": "…"}'
```

```powershell
curl -X PUT -H "Authorization: Bearer $env:ADAPTER_TOKEN" `
  "$env:MANAGER_URL/signal/state/my-source/cursor" -d '{"value": "…"}'
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

`cooldownHours` is the one knob: how long a repeated `initiationId` is
treated as a re-send rather than a re-notification. Size it to your own
re-notification policy if the default of 6 does not fit.

### Filter which signals this source admits

`attributeFilters` narrows which signals route at all — the same
`key`/`operator`/`values` shape as a Kubernetes `matchExpressions` entry,
evaluated against the labels you normalized the signal with (step 2).

```yaml
spec:
  attributeFilters:
  - key: team
    operator: In
    values: [payments, billing]
```

A signal must satisfy every requirement to be admitted. A failing signal is
dropped before cooldown is spent and before any pipeline sees it.

This needs no adapter change. It is narrower than filtering upstream of the
adapter. Leave `attributeFilters` empty or unset to admit everything, exactly
as before this field existed.

| Operator | Tests |
|---|---|
| `In` / `NotIn` | `values` against the label's value |
| `Exists` / `DoesNotExist` | only whether the key is present — no `values` |

An unknown operator is rejected when you apply the CR, not silently ignored
at ingest.

A filtered-out signal still shows up as a `signal.dropped` activity event by
default, so an over-narrow filter is diagnosable rather than silently eating
signals. Set `logFiltered: false` once you trust the filter, to stop
recording drops from a source you expect to be noisy.

Then list that source on a Ready Pipeline's `signalSourceRefs`. Until you do,
`Wired=False` (`MGR-4`) and every signal you push is dropped.

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
   2.3.0.
3. **Compare a different transport** —
   [`signals/alertmanager`](https://github.com/kostiantyn-matsebora/agent-ops-operator/blob/master/signals/alertmanager/)
   receives webhooks,
   [`signals/k8s-events`](https://github.com/kostiantyn-matsebora/agent-ops-operator/blob/master/signals/k8s-events/)
   watches the API,
   [`signals/ha`](https://github.com/kostiantyn-matsebora/agent-ops-operator/blob/master/signals/ha/)
   reads a WebSocket.
4. **[Talk to agents from your own chat]({{ '/guides/channel-adapter/' | relative_url }})**
   — the other half, for carrying the conversation.
5. **Every field of both kinds** —
   [SignalAdapter]({{ '/crds/signal-adapter/' | relative_url }}) and
   [SignalSource]({{ '/crds/signal-source/' | relative_url }}).
