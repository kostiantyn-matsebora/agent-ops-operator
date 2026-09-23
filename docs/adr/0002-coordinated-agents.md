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
| **C** | extract `AgentCapability`; every Pipeline references one |
| **D** | extract `AgentCapability`; Pipeline takes it INLINE or by ref; new `Coordinator` kind |
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

**D1 — `AgentCapability` is the capability kind.** The six capability fields, nothing
wired. An unwired AgentCapability is inert.

- Not `Agent`: `terminology.md` reserves that word for the definition in
  `.claude/agents/` that `AgentProfile.spec.agent` selects. A third meaning one
  lookup from both is the collision that rule exists to stop. In prose the
  members of a composition are still agents.

**D2 — `Pipeline` and `Coordinator` are the two wiring kinds. Neither involves
the other.**

```
     AgentCapability
        ▲   ▲
 Pipeline   Coordinator
 one agent, a composition:
 signals    sources in, /<coordinator> in,
 + humans   escalation out, agents invoked
```

| Kind | Field | Rule |
|---|---|---|
| `Pipeline` | inline fields OR `capabilityRef` | mutually exclusive by CEL; inline IS today's Pipeline unchanged |
| `Coordinator` | `signalSourceRefs` | inbound is CLAIMED, as a Pipeline's is; `/<coordinator> <task>` addresses it as a Pipeline is addressed, binding the origin surface |
| | `channelRefs` | where it ESCALATES to |
| | `agents[]{capabilityRef, description}` | the WHOLE outbound reach; enforced server-side per token, not by `--allowedTools` |
| | `limits{maxAgents, maxTurns, deadline}` | see D5 |

- Description lives on the entry, not the AgentCapability.
- An entry without a description is refused.

**D3 — The manager routes results.** A conversation a coordinator started
carries `spec.causedBy` — its PARENT, one hop, never the tree's top.
`/work/done` on it appends the result as an input to the parent, the only
thing that gives a conversation a turn.

- `causedBy` is provenance, as `pipelineRef` is: written once, resolves nothing,
  decides no delivery.
- A caused conversation binds no human channel.
- Output is the existing block grammar. No new output mode.

**D4 — Escalation is a decision, not an arrival.** A coordinator opens a human
thread when it decides to, on its `channelRefs`, with a synthesised first message.
Close and drop are `/close` with a `closeReason` the object keeps.

- **Only the uncaused root holds a `channelRefs` to open a thread on** — a
  caused conversation binds no human channel (D3). A nested Coordinator's
  `escalate` closes its own conversation instead, appending the message to its
  parent's inputs, so the decision to open a thread propagates up one hop at a
  time until it reaches the root.

**D5 — Budget on the conversation a Coordinator opens. Exhaustion closes with
a reason, and nesting does not share a budget** — a nested Coordinator
snapshots its OWN `limits`, independent of its parent's. A wide-and-deep tree
is bounded at every level, never as one pool.

| Limit | Bounds | Why the others miss it |
|---|---|---|
| `maxAgents` | fan-out per Coordinator conversation | the global cap starves other incidents |
| `maxTurns` | that conversation's own inputs | a loop is height 2, infinite width — depth never fires |
| `deadline` | that conversation's age | nothing else has a timer |

Past any: that conversation closes `budget-exceeded`, its members with it, through D4.

**D6 — The console shows the tree** rooted at the uncaused conversation, at
any depth — a member's `causedBy` may itself be another member. It is the
only place a person sees incidents they were not told about.

**D7 — A member may be a Coordinator, so the tree may nest.** `causedBy`
names the ONE HOP parent (D3), never the tree's top.

- Walking to the uncaused root means following links, not one lookup.
- Reuse scoping and self-input refusal both compare `causedBy` at one hop,
  unaffected by depth.
- Escalation stays a decision made ONLY by the uncaused root (D4, D6). A
  nested Coordinator's `escalate(message)` closes ITS OWN conversation with
  that message as its result, an ordinary append to its parent's inputs, and
  opens no thread.
- The parent's agent decides whether to handle it or escalate again, so a
  human thread opens only when that decision reaches the top.
- Budget is per-Coordinator (D5). Nesting does not pool `maxAgents`,
  `maxTurns` or `deadline` across levels.
- `coordinatorRef` names the Coordinator a conversation's own entry point is —
  set on a direct address and on a nested member, empty on a Pipeline-addressed
  conversation. The cycle guard collects the calling conversation's OWN
  `coordinatorRef` first, then walks `causedBy` to the uncaused root
  collecting each ancestor's, and refuses an `invoke` whose target resolves to
  a Coordinator already in that list.

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

- Whether `AgentCapability` replaces the inline Pipeline fields later.
- `Coordinator` status shape.
- Choreography (agent-to-agent with no root) — waits for the budget to exist.
