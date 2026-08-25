# ADR 0002 — Coordinated agents: one capability kind, two wiring kinds

- **Status**: Proposed
- **Date**: 2026-08-25

## Context

agent-ops is depth 1: a signal opens a Conversation, a Conversation opens
nothing. `Pipeline` is the only wiring object.

Three properties of the model block any composition of agents — routing,
orchestrator–agents, recursive delegation:

| Property | Effect |
|---|---|
| An agent is reached only through a subscription (claimed source, or `/<pipeline>` from a wired surface) | one agent cannot name another |
| `PipelineSpec` fuses a CAPABILITY (`profileRef`, `runtimeRef`, `serviceAccountName`, `toolsets`, `mcpConfigs`, `persistence`) with a SUBSCRIPTION (`signalSourceRefs`, `channelRefs`) | a capability exists only where something wired it |
| Conversations carry no relation to each other | no tree, no budget, no owner |

## Problem

**A composition of agents cannot be declared.** It can be faked, and every
fake makes a behaviour depend on an omission rather than a field:

| Fake | Implicit behaviour |
|---|---|
| coordinator as a `ChannelAdapter` forwarding results into its own conversation | a control loop disguised as a surface |
| a Pipeline with no sources and no channels | that it is a capability, not a misconfiguration |
| a name in another Pipeline's toolset | who may reach an agent |
| `causedBy` deciding delivery | forget it and agent output reaches a human surface |

## Options

| | Is |
|---|---|
| **A** | coordinator as channel; Pipeline unchanged |
| **B** | `Pipeline.spec.addressable` flag; sources and channels forbidden by CEL |
| **C** | extract `Agent`; every Pipeline references one |
| **D** | extract `Agent`; Pipeline takes it INLINE or by ref; new `Coordinator` kind |
| **E** | subagents inside the coordinator's pod (claude-code's own Task tool) |

## Trade-off analysis

Deciding test: **a generated path must be able to run step 2 with LESS power
than step 1.** Composition that cannot lower privilege per step is already
available as E.

| | A | B | C | D | E |
|---|---|---|---|---|---|
| per-step identity and tools | yes | yes | yes | yes | **no** |
| the composition is one readable object | no | no | yes | yes | no |
| reach is a typed list | no | partly | yes | yes | n/a |
| results return by a named mechanism | no | no | yes | yes | yes |
| existing Pipelines untouched | yes | yes | **no** | yes | yes |
| new CRDs | 0 | 0 | 1 | 2 | 0 |

- **E** fails the deciding test.
- **A, B** fail on legibility.
- **C vs D** differs in one row, and it is the cost to every existing install.

## Decisions

**D1 — `Agent` is the capability kind.** The six capability fields, nothing
wired. An unwired Agent is inert.

**D2 — `Pipeline` and `Coordinator` are the two wiring kinds. Neither involves
the other.**

```
        Agent
        ▲   ▲
 Pipeline   Coordinator
 one agent, a composition:
 signals    sources in, escalation out,
 + humans   agents invoked by description
```

| Kind | Field | Rule |
|---|---|---|
| `Pipeline` | inline fields OR `agentRef` | mutually exclusive by CEL; inline IS today's Pipeline unchanged |
| `Coordinator` | `signalSourceRefs` | inbound is CLAIMED, as a Pipeline's is |
| | `channelRefs` | where it ESCALATES to |
| | `agents[]{agentRef, description}` | the WHOLE outbound reach; enforced server-side per token, not by `--allowedTools` |
| | `limits{maxAgents, maxTurns, deadline}` | see D5 |

- Description lives on the entry, not the Agent.
- An entry without a description is refused.

**D3 — The manager routes results.** A conversation the coordinator started
carries `spec.causedBy` (the root). `/work/done` on it appends the result as an
INPUT to the root — the only thing that gives a conversation a turn.

- `causedBy` is provenance, as `pipelineRef` is: written once, resolves nothing,
  decides no delivery.
- A caused conversation binds no human channel.
- Output is the existing block grammar. No new output mode.

**D4 — Escalation is a decision, not an arrival.** A coordinator opens a human
thread when it decides to, on its `channelRefs`, with a synthesised first message.
Close and drop are `/close` with a `closeReason` the object keeps.

**D5 — Budget on the root; exhaustion closes with a reason.**

| Limit | Bounds | Why the others miss it |
|---|---|---|
| `maxAgents` | fan-out per root | the global cap starves other incidents |
| `maxTurns` | the root's own inputs | a loop is height 2, infinite width — depth never fires |
| `deadline` | root age | nothing else has a timer |

Past any: root closed `budget-exceeded`, members with it, through D4.

**D6 — The console shows the tree** rooted at the uncaused conversation. It is
the only place a person sees incidents they were not told about.

## Consequences

- `wiring.md`: "no other CR carries wiring" → "two CRs carry wiring".
- Conversation REUSE scopes on `causedBy` as on `pipelineRef`.
- `/channel/inbound` refuses an input whose origin is its own target conversation.
- `DeliverInputs` mirrors member results to every channel on the root.
- Root context grows per member result; concise member output is the control.
- New component: the aops MCP server — reads, plus async `invoke` / `close` /
  `escalate` / `read`, reach bounded per token behind the ADR 0001 wall.
- Sync `invoke` is forbidden: it holds the root's serial queue and a slot.

## Rejected

| Idea | Because |
|---|---|
| naming a Pipeline from an HTTP tool | `POST /task` returns |
| transferring a conversation between Pipelines | rewrites the identity and storage snapshot in flight |
| depth counter as loop breaker | bounds height, not width |
| `structured` output mode, `audience` on adapters | guard for a leak D3 removes |
| `worker` | retired vocabulary |

## Not decided here

- Whether `Agent` replaces the inline Pipeline fields later.
- `Coordinator` status shape.
- Whether a member may be a Coordinator.
- Choreography (agent-to-agent with no root) — waits for the budget to exist.
