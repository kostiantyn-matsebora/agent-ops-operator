---
title: "Run agents on your own backend"
permalink: /guides/agent-runtime/
description: >-
  What an agent runtime is, the three parts it is built from, and how to
  implement one for a backend agent-ops does not ship.

next:
  eyebrow: Reference
  title: Every field of every kind
  body: >-
    The generated custom resource reference, and the contracts the adapter and
    runtime kinds serve.
  url: https://github.com/kostiantyn-matsebora/agent-ops-operator/blob/master/docs/cr-reference.md
---

An **agent runtime** is what actually executes an agent. It is a container image
plus an `AgentRuntime` resource describing it: the image, the identity it runs
as, and the SHAPE of its context storage.

**It declares no volume.** WHERE a route's conversations keep their state is the
route's decision, on its `Pipeline` — see the
[Pipeline guide]({{ '/guides/pipeline' | relative_url }}).

A runtime is an ENGINE. Two routes sharing one must be able to persist to
different volumes without cloning it.

There is one per vendor and trust level, shared by every route that names it.
The image implements a small HTTP contract — it asks the manager for work, runs
it, and reports what happened.

![The manager hands a work unit to your runtime inside the conversation's pod, which runs the agent and reports the result back.]({{ '/assets/img/guides/agent-runtime-light.svg' | relative_url }}){: .ao-diagram}

## Before you start

Writing a runtime is appropriate when:

- The **backend** is different — another vendor's CLI, a local model, your own
  harness.
- You need behaviour the contract allows but `runtimes/claude` does not
  implement.

It is **not** what you want when:

- You only need a **tool in the image**. Derive from `runtimes/claude` and point
  `spec.image` at yours. That is what the field is for.
- You need a different **tool allowlist**. That is wiring, and it lives on the
  Pipeline — see
  [Give your agent tools]({{ '/guides/toolsets/' | relative_url }}).

{: .ao-callout}
> **Your runtime applies the tool allowlist, and nothing checks that it did.**
> A runtime that ignores it voids every toolset binding in the install,
> silently. Read the allowlist section below before you write any of it.

Review
[the work contract](https://github.com/kostiantyn-matsebora/agent-ops-operator/blob/master/docs/contracts.md#the-work-contract)
and
[AgentRuntime](https://github.com/kostiantyn-matsebora/agent-ops-operator/blob/master/docs/concepts.md#agentruntime)
first. This page builds the smallest thing that satisfies them.

## The overall shape

An agent runtime is three parts:

1. **A container image** implementing the work contract — poll for a unit,
   execute it, report the outcome, exit when idle.
2. **An `AgentRuntime` resource** declaring that image, the ServiceAccount it
   runs as **by default**, and what continuity it can promise.
3. **A `Pipeline` naming it** through `runtimeRef`. A Pipeline that names none
   falls back to the runtime called `default`.

**A RUNTIME IS ONE OF SEVERAL.** Declare any number under the chart's
`runtimes:` list, each stating only what DIFFERS from
`global.agentops.runtimeDefaults` — the image, the resources, the idle TTL, the
egress posture and the credential shape are all inherited, so a second vendor is
usually two lines:

```yaml
runtimes:
  - name: my-runtime
    image: registry.example.com/my-agentops-runtime:1.0.0
```

**`default` is what a route naming no `runtimeRef` resolves to**, and the
`claude` bundle ships it. Turn that bundle off with no replacement and the
render FAILS, naming the missing runtime and the routes that needed it — which
is the honest alternative to conversations queueing forever.

A bundle may ship a runtime of its own the same way.

The manager creates one pod per conversation from part 2, and that pod is your
image. It gives the pod `CONTROL_URL`, `CONVO_ID`, `POD_NAME` and
`RUNTIME_IDLE_TTL_M`, and checks the profile's repository out at
`/data/workspace` before your process starts.

**The pod dials the manager, never the reverse.** Your runtime needs no inbound
port and no Service.

## Poll for work

Long-poll. The call blocks until there is a unit or the wait expires.

```sh
curl "$CONTROL_URL/work?convo=$CONVO_ID&pod=$POD_NAME&wait=25"
```

```powershell
curl "$env:CONTROL_URL/work?convo=$env:CONVO_ID&pod=$env:POD_NAME&wait=25"
```

A `204` means nothing is queued. Poll again. A unit looks like this:

```json
{
  "runId": "r-7",
  "convo": "task-kvb55",
  "promptText": "…",
  "agent": "sre",
  "allowedTools": "Read,Grep,mcp__kubernetes__*",
  "toolsMode": "merge",
  "runtimeContextId": "abc123",
  "maxTurns": 60
}
```

`promptText` arrives fully rendered — the manager has already wrapped the
payload in a prompt. A profile that replaces that wrapper sends `promptFile` and
`promptVars` instead, both relative to the checkout, and your runtime renders
them.

## Compose the tool allowlist

**`allowedTools` is half of the allowlist.** The other half is the `tools:`
frontmatter of `.claude/agents/<agent>.md` in the checkout, which only your
runtime can read, because only your runtime holds the repository.

`toolsMode` says how the two combine:

| `toolsMode` | Pass to the model |
|---|---|
| `merge` | the union, the definition's own entries keeping their position |
| `overwrite` | `allowedTools` alone |

Three rules, and each of them was paid for:

| Rule | Because |
|---|---|
| An **empty allowlist means empty** | substituting a default is a grant nobody wrote down |
| **Never re-apply the definition** on top of the composed list | applying it twice as an intersection silently defeats `overwrite` |
| **Never prompt** for a permission | nobody is in a pod to answer, so the run hangs until its idle TTL |

No repository means no definition to read, so `merge` degrades to
`allowedTools`. That is correct, not a fallback.

## Run the unit and report the outcome

Stream progress to **stdout**. That is the live transcript an operator reads
with `kubectl logs`, and the only window into a run while it happens.

```sh
curl -X POST "$CONTROL_URL/work/done" -d '{
  "convo": "task-kvb55", "runId": "r-7", "status": "succeeded",
  "result": "the answer, as markdown",
  "runtimeContextId": "abc123", "continuity": "continued"
}'
```

```powershell
curl -X POST "$env:CONTROL_URL/work/done" -d '{
  "convo": "task-kvb55", "runId": "r-7", "status": "succeeded",
  "result": "the answer, as markdown",
  "runtimeContextId": "abc123", "continuity": "continued"
}'
```

**`result` is the deliverable.** The operator hands it to every bound channel
through their adapters, so your runtime never posts to a transport and holds no
channel credentials.

**`runtimeContextId` is your own handle, and the manager interprets nothing.**
It stores the string and hands it back on the next unit. Report the context you
actually finished on, which is not always the one you were asked to continue.

`continuity` says what happened to the context you were handed:

| Value | Means |
|---|---|
| `continued` | you resumed it |
| `new` | you started fresh, and nothing had been promised |
| `unavailable` | you could not reach it — the manager fails the run rather than answering fresh under its name |

## Exit when idle

Exit `0` after `RUNTIME_IDLE_TTL_M` minutes with no work. The pod outliving the
answer is deliberate, so a follow-up skips the cold start. The manager creates a
fresh pod on the next input.

## Declare the AgentRuntime

<!-- generated: template kind=AgentRuntime name=my-runtime fields=image,contextStorage,serviceAccountName,idleTtlMinutes comments=off -->
```yaml
apiVersion: agentops.dev/v1alpha1
kind: AgentRuntime
metadata:
  name: my-runtime
spec:
  image: <image>
  contextStorage: volume
  serviceAccountName: <serviceAccountName>
  idleTtlMinutes: 10
```
<!-- /generated -->

`contextStorage` is what the manager reads **before** promising a reader that a
follow-up keeps its context. It is the one storage question only your runtime
can answer — whether its backend writes context to a disk at all:

| Value | Your runtime keeps context |
|---|---|
| `volume` | on a context volume, whichever one the conversation resolved |
| `external` | somewhere the manager does not manage |
| `none` | nowhere — every unit starts fresh |

**It does not say WHICH volume, and cannot.** The manager checks this
declaration against what the conversation actually resolved.

Under `volume`, a conversation whose route and release both supply none is told
from its first run that it cannot be continued, rather than failing every
follow-up.

**The ServiceAccount is the agent's in-cluster power**, and a `Pipeline` may
name its own and override what you declare here.

**That is what lets one image serve two trust levels.** An observing route and
an acting route differ in their account, not in their engine, so you write one
`AgentRuntime` and not two.

**A route that names no account gets one bound to nothing.** Leaving this field
empty is the safe default — it falls through to that floor rather than to
anything the release granted.

**If you write your own account, hold it to the rules the chart's own follow**:
no verb on `secrets`, no wildcard, no `escalate` or `bind`, and nothing in the
operator's own namespace.

**And letting an agent create or exec into a pod grants it every Secret in that
namespace, whatever its RBAC says.** The kubelet does the reading. See
[installation](https://github.com/kostiantyn-matsebora/agent-ops-operator/blob/master/docs/installation.md).

## Point a Pipeline at it, and verify

Declaring it through the chart is the ordinary path — the `AgentRuntime` below
is what that renders, and applying one by hand is the same object.

```sh
kubectl -n agent-ops apply -f my-runtime.yaml
kubectl -n agent-ops patch pipeline my-route --type=merge \
  -p '{"spec":{"runtimeRef":{"name":"my-runtime"}}}'
kubectl -n agent-ops logs -f agentops-conv-<conversation>
```

```powershell
kubectl -n agent-ops apply -f my-runtime.yaml
kubectl -n agent-ops patch pipeline my-route --type=merge `
  -p '{\"spec\":{\"runtimeRef\":{\"name\":\"my-runtime\"}}}'
kubectl -n agent-ops logs -f agentops-conv-<conversation>
```

**Conversations already open keep the runtime they were created with.** The
choice is recorded when a conversation starts, so patch the Pipeline and then
ask something new.

Ask your agent something. The pod log should show the unit arriving, then the
allowlist you composed, then the answer.

## What comes next

1. **Read the contract in full** —
   [the work contract](https://github.com/kostiantyn-matsebora/agent-ops-operator/blob/master/docs/contracts.md#the-work-contract)
   covers every field, and the breaker for a tool call the model cannot form.
2. **Compare against the reference** —
   [`runtimes/claude`](https://github.com/kostiantyn-matsebora/agent-ops-operator/blob/master/runtimes/claude/),
   about 200 lines of contract handling.
3. **Add durable context** — declare `contextSync` and the manager points your
   `CONTROL_URL` at a sidecar that checkpoints for you. Nothing in your runtime
   changes.
4. **Wall off an uncooperative agent** —
   [`platform/egress-proxy`](https://github.com/kostiantyn-matsebora/agent-ops-operator/blob/master/platform/egress-proxy/)
   mediates what the pod may reach, for the case an allowlist cannot cover.
5. **Publish multi-arch.** A single-arch image fails at schedule time, not at
   build, and possibly weeks later.
