---
title: "Coordinate agents"
permalink: /guides/coordinate-agents/
description: >-
  What a Coordinator is, how its root conversation invokes other agents as
  members instead of wiring one, and the budget and cycle rules that keep a
  composition bounded.

next:
  eyebrow: Reference
  title: Every Coordinator field
  body: >-
    The kind's own spec page — a commented example and every field, in the
    sidebar's CRDs group.
  url: /agent-ops-operator/crds/coordinator/
---

A `Coordinator` wires a composition of agents, each invoked on demand through its own `AgentCapability`. Claims sources and binds channels like a `Pipeline`, plus an `agents[]` list.

{: .ao-callout}
> **Lowers privilege per step.** Each member gets its own profile, tools, identity.

![A Coordinator's root conversation invokes AgentCapabilities as members, and its agent escalates through a channel already open.]({{ '/assets/img/guides/coordinate-agents-light.svg' | relative_url }}){: .ao-diagram}

## Declare a member

An `AgentCapability` is inert until referenced. Declare once, reference from `agents[]`:

```yaml
kind: AgentCapability
metadata:
  name: log-analyzer
spec:
  profileRef: { name: log-analyzer-profile }
  toolsets: { refs: [{ name: agentops-observe }] }
  mcpConfigs: { refs: [{ name: k8s-api }] }
```

## Write the Coordinator

```yaml
kind: Coordinator
metadata:
  name: k8s-triage
spec:
  signalSourceRefs: [{ name: cluster-events }]
  profileRef: { name: k8s-triage-agent }
  agents:
    - name: log-analyzer
      description: Reads logs, events, usage. Reports crash cause. Cannot change anything.
      capabilityRef: { name: log-analyzer }
    - name: remediator
      description: Restarts or scales a workload. Cannot diagnose on its own.
      capabilityRef: { name: remediator }
  channelRefs: [{ name: telegram }]
  limits: { maxAgents: 5, maxTurns: 20, deadline: 1h }
```

{: .ao-callout}
> **Entries without `description` are refused.** The root reads these to decide which member to invoke.

`channelRefs` binds the thread at creation — visible even if `escalate` is never called.

## Nest a composition

Use `coordinatorRef` instead of `capabilityRef` to nest another Coordinator. No depth limit; cycles refused at `Ready` and invoke:

```yaml
  agents:
    - name: home-desk
      description: Household coordinator. Operates the house through its members. Cannot act on cluster.
      coordinatorRef: { name: home-triage }
```

## The budget

| Limit | Bounds | Enforced |
|---|---|---|
| `maxAgents` | how many agents THIS conversation may invoke | on `invoke` |
| `maxTurns` | how many inputs THIS conversation processes | after each run |
| `deadline` | how long THIS conversation may run | on reconcile requeue |

Nesting never pools — each nested Coordinator has its own budget.

## Apply it

```sh
kubectl -n agent-ops apply -f k8s-triage.yaml
```

```powershell
kubectl -n agent-ops apply -f k8s-triage.yaml
```

## Auto-wiring

Set `global.agentops.wiringMode: coordinator` and the chart renders a Coordinator that gathers every enabled bundle's capability under `agents[]`. Also brings up the Coordination bundle so `invoke` works.

## Turn on the verbs

{: .ao-callout}
> **`coordination.enabled` must be `true`** or every invoke fails. See [Installation]({{ '/installation/#enable-a-bundle' | relative_url }}).

Bind `agentops-coordinate` on the root's capability:

```yaml
spec:
  toolsets: { refs: [{ name: agentops-coordinate }] }
  mcpConfigs: { refs: [{ name: aops-coordinate }] }
```

The token unlocks `invoke` against THIS Coordinator's `agents[]` only.

## Self-heal: the reaper

Set `reaper.enabled: true` to ship an hourly survey agent that re-invokes domain members to check if open roots can close. Off by default.

## Verify

Check `Ready=True` and that every `agents[]` entry resolves:

```sh
kubectl -n agent-ops get coordinator k8s-triage \
  -o jsonpath='{.status.conditions[?(@.type=="Ready")]}'
```

```powershell
kubectl -n agent-ops get coordinator k8s-triage `
  -o jsonpath='{.status.conditions[?(@.type==\"Ready\")]}'
```

The message names the first entry that fails to resolve.

## What comes next

1. **[Give your agent tools]({{ '/guides/toolsets/' | relative_url }})** — bind toolsets and MCP servers on each member.
2. **[Every Coordinator field]({{ '/crds/coordinator/' | relative_url }})** — the full shape of the kind.
