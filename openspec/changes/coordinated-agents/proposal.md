# Proposal: coordinated-agents

Decided in `docs/adr/0002-coordinated-agents.md`, which this change carries.
The ADR holds the options and the trade-offs; this file holds the delivery.

## Why

agent-ops is depth 1 — a signal opens a Conversation and a Conversation opens
nothing — so no composition of agents (routing, orchestrator–agents, recursive
delegation) can be declared, and every way to fake one makes a behaviour depend
on an omission rather than a field.

## What Changes

- **New CRD `AgentCapability`** — the capability: `profileRef`, `runtimeRef`,
  `serviceAccountName`, `toolsets`, `mcpConfigs`, `persistence`. Nothing wired;
  an unwired AgentCapability is inert.
- **`Pipeline` takes its agent inline OR by `capabilityRef`**, mutually exclusive by
  CEL. The inline form is today's Pipeline unchanged. **Not breaking.**
- **New CRD `Coordinator`** — wiring for a composition: `signalSourceRefs`
  (claimed, as a Pipeline's are), `channelRefs` (where it escalates),
  `agents[]{capabilityRef, description}` (its whole outbound reach), `limits`
  (`maxAgents`, `maxTurns`, `deadline`), plus the capability fields of the
  coordinator agent itself.
- **`Conversation.spec.causedBy`** — provenance naming the root conversation;
  written once, resolves nothing. Conversation reuse scopes on it.
- **The manager routes results**: `/work/done` on a caused conversation appends
  the result as an input on the root. A caused conversation binds no human
  channel.
- **Escalation is a verb**: a coordinator binds its `channelRefs` late, with a
  synthesised first message. Close and drop are `/close` with a new
  `closeReason`.
- **Budget on the root**: past any limit the root and its members close
  `budget-exceeded`, through escalation.
- **New component `platform/mcp-aops`** — the aops MCP server: read tools over
  the agentops kinds, and async `invoke` / `close` / `escalate` / `read`; reach
  bounded per derived token to the calling Coordinator's `agents[]`.
- **`Conversation.status.brief`** — what a conversation is about, one
  paragraph, reported by the runtime in `/work/done`, latest-wins. It is what a
  caller reads to pick a conversation without reading any transcript.
- **A second reach class on the aops MCP server** — a token derived with
  context `channel-reader:<channel>` reads the `{name, title, brief, phase,
  pipeline}` projection of conversations bound to that Channel, and no verb.
  The manager decides it from the token context; the server forwards.
- **Loop guard**: `/channel/inbound` refuses an input whose origin surface is its
  own target conversation.
- **Console**: a tree/timeline view rooted at the uncaused conversation;
  unwired AgentCapabilities and Coordinators in the inventory and topology views.
- **Chart**: CRDs, RBAC for the MCP server, its Deployment and Service behind
  the network policy, `global.builtinToolsets` gains the aops vocabulary.
- **Rule change**: `wiring.md` "no other CR carries wiring" → "two CRs carry
  wiring". `terminology.md` adds `AgentCapability` and `Coordinator`. The spec
  `profile-is-identity` is unchanged — a profile still carries no reach.

## Capabilities

### New Capabilities

- `agent-capability-model`: the `AgentCapability` CRD — the six capability fields, inertness when
  unwired, resolution to one `AgentCapabilitySpec` shared with the inline Pipeline form.
- `coordinator-model`: the `Coordinator` CRD — claimed sources, escalation
  channels, the typed `agents[]` list with required descriptions, limits,
  `Ready` naming any member not Ready.
- `conversation-provenance`: `spec.causedBy` — written once, resolves nothing,
  decides no delivery; reuse scoping; the tree it defines.
- `coordination-loop`: result routing from a caused conversation to its root as
  an input; no human channel on a caused conversation; the self-input refusal on
  `/channel/inbound`; the three limits and `budget-exceeded` closure.
- `coordination-escalation`: late binding of the coordinator's channels on
  decision, the synthesised first message, `closeReason` on close and drop.
- `aops-mcp-server`: the component — tools, async verbs, per-token reach in
  two classes (coordinator, channel-reader), placement behind the ADR 0001
  wall, `invoke` reporting attach vs create.
- `console-coordination-view`: the tree/timeline rooted at the uncaused
  conversation; incidents closed without escalation are visible there.

### Modified Capabilities

- `pipeline-model`: the wiring requirement gains the inline-or-`capabilityRef` form
  and its CEL exclusivity; resolution yields one `AgentCapabilitySpec`.
- `conversation-close`: close carries a `closeReason`; a coordinator may close a
  conversation it caused.
- `conversation-opens-with-its-input`: a caused conversation's inputs are
  delivered to no human channel; a root's member results are inputs and are
  delivered as any input is.
- `chat-signal-origination`: a Coordinator is a claimant of a source beside
  Pipelines; fan-out counts both.
- `state-durability`: the restart-resilience matrix gains `causedBy`, the
  root's budget counters, the pending escalation and `brief` — agent-written,
  latest-wins, surviving every restart with the status it lives on.
- `console-topology`: `AgentCapability` and `Coordinator` are graph nodes; a Coordinator's
  member edges are drawn from `agents[]`.
- `chat-addressing-discovery`: `/pipelines` and the choice list name Pipelines
  and Coordinators; `/<coordinator> <task>` resolves as `/<pipeline>` does; an
  AgentCapability is never addressable from a surface.

## Impact

**Code**

- `platform/manager/api/v1alpha1/`: `agentcapability_types.go`, `coordinator_types.go`,
  `Pipeline.spec.capabilityRef` + CEL, `Conversation.spec.causedBy`,
  `status.closeReason`, `status.brief`; deepcopy and CRDs regenerated.
- `platform/manager/internal/`: `controller/` (AgentCapability, Coordinator reconcilers;
  budget enforcement; late binding), `httpapi/` (root routing on `/work/done`,
  `causedBy` reuse scope, self-input refusal, `PipelinesForSource` gains
  Coordinators, `brief` recorded from `/work/done`, the channel-reader
  projection on `/coordinate/*`), `dispatch/` (one `AgentCapabilitySpec`
  resolver; `format.md` asks for the brief), `chat/` (escalation op,
  `closeReason`).
- `platform/mcp-aops/`: new component, own Dockerfile-free Go module on the
  shared recipe; its own token context `mcp-aops`; callers forward a per-conversation token the manager validates.
- `platform/console/`: watch of two more kinds; tree view; inventory rows.
- `chart/`: two CRDs, `mcp-aops` Deployment/Service/RBAC/NetworkPolicy,
  `global.builtinToolsets.agentops-coordinate`, NOTES.txt.
- `.github/components.sh` derives the new component; `retired-vocabulary.json`
  unchanged.

**Documents made untrue — reference docs**

- `docs/concepts.md`: the kind table (eleven → thirteen), wiring precedence,
  provenance, the coordination loop, budget, `status.brief`.
- `docs/contracts.md`: `/work/done` root routing and the `brief` field,
  `/channel/inbound` refusal, the aops MCP tool contract and its two reach
  classes.
- `docs/cr-reference.md` and every generated block: regenerated.
- `docs/console.md`, `docs/console-guide.md`: the tree view.
- `docs/security.md`: a new trust flow — an agent invoking agents — and the
  threat-model diagram re-run.
- `docs/installation.md`: `mcp-aops` values.
- `docs/CHANGELOG.md`: two CRDs to `kubectl apply`, the new component.
- `.claude/rules/wiring.md`, `terminology.md` (the "Agent is TAKEN" entry
  gains the CRD name and why it is not `Agent`), `structure.md`,
  `invariants.md`.

**Documents made untrue — adopter site**

- `docs/index.md` (landing): the kind count and the "what you write" tab.
- `docs/introduction.md`: the model has a second wiring kind.
- `docs/getting-started.md`: unchanged unless the demo wires a Coordinator —
  decided in design.
- `docs/installation.md`: the component list.
- `docs/guides/`: a new guide, "Coordinate agents"; existing guides' generated
  blocks regenerated.
- `README.md`: the kind table, one line under "Pluggable at three seams".
