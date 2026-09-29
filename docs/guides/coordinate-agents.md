---
title: "Coordinate agents"
permalink: /guides/coordinate-agents/
description: >-
  What a Coordinator is, how its root conversation invokes other agents as
  members instead of wiring one, and the budget and cycle rules that keep a
  composition bounded.

next:
  eyebrow: Reference
  title: Every field of every kind
  body: >-
    The generated custom resource reference, and the contracts the adapter and
    runtime kinds serve.
  url: https://github.com/kostiantyn-matsebora/agent-ops-operator/blob/master/docs/cr-reference.md
---

A `Coordinator` is **the second wiring kind, for a composition of agents
instead of one**. It claims sources and channels exactly as a `Pipeline`
does, but names a LIST of agents its own conversation may invoke, instead of
answering everything itself.

{: .ao-callout}
> **Composing agents lowers privilege per step.** Each invoked member gets its
> own `AgentCapability` — its own profile, tools and identity — rather than
> one prompt holding every tool a task might ever need.

![A Coordinator's root conversation invokes AgentCapabilities as members, and escalates to a channel only when it decides to.]({{ '/assets/img/guides/coordinate-agents-light.svg' | relative_url }}){: .ao-diagram}

## Before you start

Writing a Coordinator is appropriate when:

- **One prompt is doing too much** — triage, then investigate, then decide
  whether to escalate — and each step wants different tools.
- **You want a fixed set of specialists** a task can fan out to, with their
  answers read back automatically.
- **A human should see it only when nothing downstream could resolve it.**

It is **not** what you want when:

- **One agent with one set of tools already does the job** — that is a
  [Pipeline]({{ '/guides/pipeline/' | relative_url }}).
- **The specialists do not exist yet** — write their `AgentCapability`s
  first, or start them as inline `Pipeline` fields and extract later.

Review
[Coordinator](https://github.com/kostiantyn-matsebora/agent-ops-operator/blob/master/docs/concepts.md#coordinator)
and
[AgentCapability](https://github.com/kostiantyn-matsebora/agent-ops-operator/blob/master/docs/concepts.md#agentcapability)
first.

## The overall shape

A `Coordinator` is itself a capability — for the agent that decides — plus a
named list of agents it may invoke:

1. **`signalSourceRefs`, and addressing by `/<coordinator> <task>`** — what
   starts its root conversation, exactly as a Pipeline's.
2. **Its own capability** — inline (`profileRef`, `toolsets`, …) or
   `capabilityRef`, mutually exclusive. This is the agent that reads what
   started the conversation and decides what to invoke.
3. **`agents[]`** — the WHOLE outbound reach. Each entry names `capabilityRef`
   (an ordinary member) or `coordinatorRef` (nesting), plus a `description`
   the coordinating agent reads to decide when to use it.
4. **`limits`** — `maxAgents`, `maxTurns`, `deadline`, enforced on THIS
   conversation alone.
5. **`channelRefs`** — where an escalation opens a human thread. Never bound
   until the root decides to use it.

**An entry without a description is refused.** The coordinating agent has
nothing else to read to decide which member answers which task.

## Declare a member

A member is an ordinary `AgentCapability` — the same six fields a Pipeline
can inline, declared once so a Coordinator can reference it:

<!-- generated: template kind=AgentCapability name=log-analyzer fields=profileRef,toolsets,mcpConfigs comments=off -->
```yaml
apiVersion: agentops.dev/v1alpha1
kind: AgentCapability
metadata:
  name: log-analyzer
spec:
  profileRef:
    name: <name>
  toolsets:
    refs:
    - name: <name>
  mcpConfigs:
    refs:
    - name: <name>
```
<!-- /generated -->

**It is inert until something references it.** An `AgentCapability` nothing
names is exactly as ordinary as an unwired `Channel` — it exists to be
reused.

## Write the Coordinator

<!-- generated: template kind=Coordinator name=k8s-triage fields=signalSourceRefs,channelRefs,profileRef,agents[].name,agents[].capabilityRef,agents[].description,limits.maxAgents,limits.maxTurns,limits.deadline comments=off -->
```yaml
apiVersion: agentops.dev/v1alpha1
kind: Coordinator
metadata:
  name: k8s-triage
spec:
  signalSourceRefs:
  - name: <name>
  channelRefs:
  - name: <name>
  profileRef:
    name: <name>
  limits:
    maxAgents: 1
    maxTurns: 1
    deadline: <deadline>
  agents:
  - name: <name>
    description: <description>
    capabilityRef:
      name: <name>
```
<!-- /generated -->

Filled in for a root that triages a cluster event and can call on two
specialists:

```yaml
spec:
  signalSourceRefs:
    - name: cluster-events
  profileRef:
    name: k8s-triage-agent
  agents:
    - name: log-analyzer
      description: >-
        Reads recent logs for the named workload and reports the likely
        cause. Use first for anything that looks like a crash.
      capabilityRef:
        name: log-analyzer
    - name: remediator
      description: >-
        Restarts or scales the named workload. Use only once the cause is
        known and the fix is routine.
      capabilityRef:
        name: remediator
  channelRefs:
    - name: telegram
  limits:
    maxAgents: 5
    maxTurns: 20
    deadline: 1h
```

**`channelRefs` binds no thread yet.** The root conversation runs unseen
until it calls `escalate`, or until nothing answers and the budget closes it.

## Nest a composition

An entry may name `coordinatorRef` instead of `capabilityRef`. The invoked
conversation is then THAT Coordinator's own root, free to invoke its own
`agents[]` in turn:

```yaml
  agents:
    - name: home-desk
      description: Anything about the house rather than the cluster.
      coordinatorRef:
        name: home-triage
```

- **There is no separate "sub-Coordinator" kind.** Nesting is this same
  entry naming a Coordinator instead of an `AgentCapability`.
- **No depth limit.** A tree nests as deep as each level's own budget allows.
- **A CYCLE is refused, not merely a deep tree.** If invoking an entry would
  reach a Coordinator already on the calling conversation's own chain back to
  the uncaused root, the manager refuses it, naming the repeated Coordinator.
  `Ready` catches the same mistake statically, from the CRDs alone, before
  anything ever runs.

## The budget, per level

Each conversation that is itself a Coordinator's root enforces its OWN
`limits`, snapshotted at creation:

| Limit | Bounds | Enforced |
|---|---|---|
| `maxAgents` | how many agents THIS conversation may ever invoke | on `invoke` |
| `maxTurns` | how many inputs THIS conversation processes | after each run |
| `deadline` | how long THIS conversation may run | on a reconcile requeue |

{: .ao-callout}
> **Nesting never pools a budget.** A nested Coordinator's own conversation
> has its own `limits`, independent of its parent's. A wide-and-deep tree is
> bounded at every level, never as one shared pool.

Past any limit, the conversation closes every member it still has open with
the same reason, then escalates itself with a digest — the limit, the counts,
the member list — as if its own agent had called `escalate`.

## Escalation is a decision, and only the root ever opens a thread

`escalate` is a verb the coordinating agent calls, not something that happens
to it. Where it lands depends on whether the conversation has a parent:

| Calling conversation | `escalate` does |
|---|---|
| the **uncaused root** — no parent | binds `channelRefs`, opens the thread, with the message as the opening post |
| a **member** — invoked by another Coordinator | closes ITSELF, and the message becomes an ordinary result on its parent's next input |

**A nested escalation bubbles one hop at a time.** The parent's agent reads
it like any other member result and decides: handle it, or `escalate` again.
A human thread opens only once that decision reaches the uncaused root.

A member's result — from finishing normally or from escalating — always
lands as an input on the conversation that invoked it. Nothing an
`agents[]` entry names ever answers a human channel directly.

## See what you have to compose

```sh
kubectl -n agent-ops get agentcapabilities,coordinators
```

```powershell
kubectl -n agent-ops get agentcapabilities,coordinators
```

## Apply it, and check it is Ready

```sh
kubectl -n agent-ops apply -f k8s-triage.yaml
kubectl -n agent-ops get coordinator k8s-triage \
  -o jsonpath='{.status.conditions[?(@.type=="Ready")]}'
```

```powershell
kubectl -n agent-ops apply -f k8s-triage.yaml
kubectl -n agent-ops get coordinator k8s-triage `
  -o jsonpath='{.status.conditions[?(@.type==\"Ready\")]}'
```

`Ready=True` means the Coordinator's own capability resolves, and every
`agents[]` entry resolves to something itself Ready. The message names the
first entry that does not.

## Turn on the verbs

A Coordinator's own agent invokes, closes, escalates and reads through the
`agentops-coordinate` MCP toolset, served by `platform/mcp-aops` — off by
default.

{: .ao-callout}
> **`coordination.enabled` must be `true`**, or the MCP server this
> Coordinator's capability binds does not exist and every invoke fails.
> See the [Coordination row]({{ '/installation/#enable-a-bundle' | relative_url }})
> in Installation.

Bind it on the root's own capability like any other toolset:

```yaml
spec:
  toolsets:
    refs:
      - name: agentops-coordinate
  mcpConfigs:
    refs:
      - name: aops-coordinate
```

**Reach is bounded by the manager, per conversation, never by this
allowlist.** The token the manager injects only ever unlocks `invoke` against
THIS Coordinator's own `agents[]`, whatever tools the pod can see.

## What comes next

1. **[Give your agent tools]({{ '/guides/toolsets/' | relative_url }})**
   — bind toolsets and MCP servers on each member's `AgentCapability`.
2. **[Put an agent to work]({{ '/guides/pipeline/' | relative_url }})**
   — the single-agent wiring kind this one composes.
3. **[Every Coordinator and AgentCapability field](https://github.com/kostiantyn-matsebora/agent-ops-operator/blob/master/docs/cr-reference.md#coordinator)**
   — the full shape of both kinds.
