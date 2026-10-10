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
instead of one**. It claims sources and binds channels exactly as a
`Pipeline` does, so any conversation it opens is reachable by a human from
the moment it exists.

It also names a LIST of agents its own conversation may invoke, instead of
answering everything itself.

{: .ao-callout}
> **Composing agents lowers privilege per step.** Each invoked member gets its
> own `AgentCapability` — its own profile, tools and identity — rather than
> one prompt holding every tool a task might ever need.

![A Coordinator's root conversation invokes AgentCapabilities as members, and its agent escalates through a channel already open.]({{ '/assets/img/guides/coordinate-agents-light.svg' | relative_url }}){: .ao-diagram}

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
   stating what that agent IS, what it CAN do, what it CANNOT, and what to
   HAND it — the coordinating agent's tools are the `agents[]` list itself,
   and this is the only thing it reads to match a task to one of them.
4. **`limits`** — `maxAgents`, `maxTurns`, `deadline`, enforced on THIS
   conversation alone.
5. **`channelRefs`** — bound to the root conversation at creation, exactly
   like a Pipeline's. `escalate` posts into these threads, it does not open
   them.

**An entry without a description is refused.** The coordinating agent has
nothing else to read to match a person's instruction, or a signal it is
investigating, to the agent that can act on it.

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
        A log reader for one workload. Can tail and search its recent pod
        logs, events, and resource usage, and report the likely cause of a
        crash or a stuck rollout. Cannot change anything. Hand it a
        workload name and what looks wrong.
      capabilityRef:
        name: log-analyzer
    - name: remediator
      description: >-
        A remediation agent for one workload. Can restart it by deleting
        its pod so its controller recreates it, or scale it to a given
        replica count. Cannot diagnose a cause on its own. Hand it a known
        cause and the fix to apply, once one is decided.
      capabilityRef:
        name: remediator
  channelRefs:
    - name: telegram
  limits:
    maxAgents: 5
    maxTurns: 20
    deadline: 1h
```

**`channelRefs` binds its thread at creation.** The root conversation is
visible from the start, whether or not it ever calls `escalate`.

## Nest a composition

An entry may name `coordinatorRef` instead of `capabilityRef`. The invoked
conversation is then THAT Coordinator's own root, free to invoke its own
`agents[]` in turn:

```yaml
  agents:
    - name: home-desk
      description: >-
        A household coordinator. Can answer questions about the house and
        operate or repair it, through its own members. Cannot act on the
        cluster. Hand it anything about the house rather than the cluster.
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

## Escalation is a decision posted into a thread already open

`escalate` is a verb the coordinating agent calls, not something that happens
to it. Where it lands depends on whether the conversation has a parent:

| Calling conversation | `escalate` does |
|---|---|
| the **uncaused root** — no parent | posts the message as an ordinary message into its already-open thread |
| a **member** — invoked by another Coordinator | closes ITSELF, and the message becomes an ordinary result on its parent's next input |

**A nested escalation bubbles one hop at a time.** The parent's agent reads
it like any other member result and decides: handle it, or `escalate` again.

The human sees it as a posted message only once that decision reaches the
uncaused root. Its thread was already open, long before this moment.

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

## Let the chart wire this for you

Everything above is for hand-authoring a `Coordinator`. The chart can also
render one — for every bundle you already enable — without writing any of
these objects yourself.

```sh
helm upgrade agent-ops oci://ghcr.io/kostiantyn-matsebora/charts/agent-ops-operator \
  -n agent-ops --reuse-values \
  --set global.agentops.wiringMode=coordinator
```

```powershell
helm upgrade agent-ops oci://ghcr.io/kostiantyn-matsebora/charts/agent-ops-operator `
  -n agent-ops --reuse-values `
  --set global.agentops.wiringMode=coordinator
```

| Each enabled bundle renders | The chart also renders |
|---|---|
| a standalone `AgentCapability` per route it would have shipped as a Pipeline — never two privilege levels merged into one | one `Coordinator`, claiming every one of those sources and listing every one of those capabilities in `agents[]` |

- **`global.demo.enabled: true` selects this mode when you leave `wiringMode`
  unset**, so a fresh demo install now exercises the Coordinator model by
  default — see [Configuration]({{ '/configuration/' | relative_url }}) for
  the value and its default.
- **Nothing here is exclusive with a hand-written `Pipeline`.** Both may
  claim the same source at once, and the API server fans it out to each.

{: .ao-callout}
> **The console does not auto-wire itself under this mode, yet.** A
> `Pipeline`-rendering bundle claims the console's signal source and binds
> its channel for you (Installation's demo). An `AgentCapability` carries no
> subscription of its own, and no chart template lists the console's channel
> in the rendered `Coordinator`'s own `channelRefs` — a chart-wiring gap,
> not a manager-level restriction: a Coordinator whose `channelRefs` DID name
> the console would bind it at creation exactly like a Pipeline's. Asking the
> console something will not reach the chart-rendered Coordinator until you
> claim its source AND its channel by hand.

## Self-heal: the hourly reaper

`coordinator` mode can ship one more `agents[]` entry beyond your bundles:
the **reaper**, an ordinary `AgentProfile` / `AgentCapability` pair with no
domain tools of its own.

**OFF by default** (`reaper.enabled: false`). An hourly survey conversation
is real cost whether or not anything is stuck open, so turn it on
(`reaper.enabled: true`) rather than finding it running unasked.

| Step | Does |
|---|---|
| once an hour | a `signals/cron` source the chart deploys and claims for you fires |
| the Coordinator's own agent | recognises that signal and `invoke`s the reaper, exactly as it invokes any domain entry |
| the reaper | calls `list_open_roots`, then re-`invoke`s the SAME capability named in each root's `members` to ask it to re-check |
| a re-check reporting the condition cleared | the reaper `close`s that root |
| a re-check still finding the condition | the root is left open |

- **No new mechanism.** The survey is built entirely from `invoke`,
  member-result routing and `list_open_roots` — the Coordinator-owner reach
  class described below.
- **It never closes the root that invoked it.** That root is excluded from
  its own `list_open_roots` call.
- **Domain agents may also close themselves, in-turn, where a thread is
  bound.** Every bundle's profile carries an instruction to reply `/close`
  once it judges a problem resolved. It is the same `/close` a person types,
  and does nothing in a member conversation, which has no thread. The
  requirement is `coordinator-self-heal`'s "Domain agent profiles carry a
  self-close instruction".

### Coordinator-owner reach: how the reaper sees its own roots

A caller acting for a Coordinator — the reaper included, an ordinary member
rather than a root — may list and close that SAME Coordinator's other open
roots, through two additions to the aops MCP server:

- **`list_open_roots()`** returns every open, uncaused root of the caller's
  own Coordinator, excluding the caller's own root — name, title, brief,
  phase, and `members` (the `agents[]` entry names to re-check).
- **`close` widens** to also reach any of those roots, never a member and
  never a different Coordinator's tree.

See [the aops MCP server contract](https://github.com/kostiantyn-matsebora/agent-ops-operator/blob/master/docs/contracts.md#the-aops-mcp-server-contract)
for the exact bound.

## What comes next

1. **[Give your agent tools]({{ '/guides/toolsets/' | relative_url }})**
   — bind toolsets and MCP servers on each member's `AgentCapability`.
2. **[Put an agent to work]({{ '/guides/pipeline/' | relative_url }})**
   — the single-agent wiring kind this one composes.
3. **[Every Coordinator and AgentCapability field](https://github.com/kostiantyn-matsebora/agent-ops-operator/blob/master/docs/cr-reference.md#coordinator)**
   — the full shape of both kinds.
