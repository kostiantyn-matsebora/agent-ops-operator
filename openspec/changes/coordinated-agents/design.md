# Design: coordinated-agents

`docs/adr/0002-coordinated-agents.md` decided WHAT. This file decides HOW, and
refers to the ADR's decisions as D1–D6.

## Context

- `PipelineSpec` holds six capability fields and two subscription fields.
  `signals.go:646` copies `pipeline.Spec.ChannelRefs` into the conversation;
  `reusableBy` (`signals.go:765`) scopes reuse on `pipelineRef`.
- A conversation gets a turn ONLY from an input. There is no timer.
- `boundChannels` (`router.go:435`) folds the originating surface into a new
  conversation unconditionally.
- `DeliverInputs` (`chat/delivery.go:55`) iterates `spec.inputs` — the queue —
  not `status.runs[].inputs[]`. A channel bound late sees a timing-dependent
  window.
- `Status.Reopens` + the `:r<n>` op-id suffix (`ops.go:177`) already establish a
  thread after creation.
- Derived adapter tokens: HMAC of master + context string; `signal-adapter:<n>`
  and channel contexts exist.
- The runtime pod reaches MCP servers only through the egress proxy (ADR 0001).

## Goals / Non-Goals

**Goals:**
- Every existing Pipeline validates and behaves unchanged.
- One resolver for the capability, whatever spelled it.
- The loop, the bound and the escalation are manager mechanisms, each a field.
- The change is shippable in phases, each leaving master consistent.

**Non-Goals:**
- Choreography (agent-to-agent with no root).
- A member that is itself a Coordinator.
- Replacing the inline Pipeline fields.
- Synchronous invocation.
- Summarising the root's context. (`brief`, D-I, is not that: it is what a
  conversation is ABOUT, one paragraph, not a digest of its transcript.)

## Decisions

### D-A — `AgentCapabilitySpec` is a shared struct, embedded in three places

| Struct | Embeds `AgentCapabilitySpec` as |
|---|---|
| `AgentCapabilitySpec` (new) | itself |
| `PipelineSpec` | inline (`json:",inline"`) — the six fields keep their JSON names |
| `CoordinatorSpec` | inline, for the coordinating agent |

- `PipelineSpec.AgentRef *ObjectRef` beside it; CEL on the Pipeline:
  `!(has(self.capabilityRef) && (has(self.profileRef) || has(self.toolsets) || …))`.
- `profileRef` becomes `+optional` on the struct; a Pipeline with neither
  `capabilityRef` nor `profileRef` fails `Ready`, not admission — CEL cannot express
  "one of" across an embedded struct without a rule per field.
- `dispatch.ResolveCapability(ctx, reader, obj) (AgentCapabilitySpec, error)` is the ONE
  resolver: returns the embedded struct or the referenced AgentCapability's. Everything
  that read `pipeline.Spec.<capability>` calls it.
- Alternative rejected: a `spec.agent` sub-object on Pipeline. Moves every
  existing field one level down — the migration D chose to avoid.

### D-B — `Coordinator` reconciles like a Pipeline, and `PipelinesForSource` grows a second kind

- `internal/chat/pipelines.go` `PipelinesForSource` returns a `[]Claimant`
  interface — name, kind, resolved `AgentCapabilitySpec`, channel refs, `Ready`. Pipeline
  and Coordinator implement it. Every call site iterates claimants.
- `Coordinator.Ready` = own capability resolves ∧ every `agents[].capabilityRef`
  resolves ∧ each AgentCapability `Ready`. Message lists the failing entry names.
- A root conversation created from a Coordinator: `spec.pipelineRef` empty,
  new `spec.coordinatorRef` set, `spec.channelRefs` EMPTY (D3/D4), limits
  snapshotted into `status.budget{maxAgents,maxTurns,deadline}`.
- `internal/addressing` and `HandleCommand` resolve the addressed segment
  across BOTH kinds — one Get per kind, Pipeline first; a name held by both is
  reported by the Coordinator reconciler's `Ready`. An addressed root binds the
  origin surface (`boundChannels` unchanged) and nothing else.
- Alternative rejected: a Coordinator that is a Pipeline with extra fields.
  Then a Coordinator claiming a chat surface would bind it at admission, and
  escalation-by-decision (D4) is gone.

### D-C — Result routing is a status-write in `handleWorkDone`, and derivable

- `handleWorkDone` on a conversation with `causedBy`: after recording the run,
  append an input on the ROOT `{origin: {kind: member, name: <conv>, entry:
  <name>}, payload: result}` in the same request; on conflict, the reconciler
  backstop re-derives from "member run done ∧ no root input with that run id".
- Input dedup key: `member:<conversation>:<runId>` — one append per run.
- Root closed → skip, and the member's run stays on its own record.
- Turn counting: `status.budget.turns` increments when the ROOT's run is
  recorded, not when an input arrives — N members finishing at once is one turn.
- Alternative rejected: routing at dispatch time by reading the tree. Would make
  a turn depend on a list, which a restart could re-order.

### D-D — Escalation reuses reopen's late-thread path

- The Coordinator's `channelRefs` are SNAPSHOTTED onto the root at creation
  as `spec.escalationChannelRefs` — they are refs, and refs are snapshotted;
  reading them at escalation time would have been the one read of wiring after
  creation, and would have nothing to read once the Coordinator is deleted.
- `escalate` verb → manager sets `spec.channelRefs` = the snapshot, stamps
  `status.escalatedAt`, and enqueues `ensure-topic` with the digest as the
  topic's opening message. Nothing reads the Coordinator.
- `DeliverInputs` MUST NOT replay: it skips inputs with `arrivedAt <
  escalatedAt`. This is the one correction to the queue-iteration behaviour
  noted in Context, and it is scoped to escalated roots.
- After escalation the root is an ordinary multi-channel conversation.
- `closeReason` is a `MaxLength=256` status string; `/close` from a surface
  stamps none; the MCP `close` verb requires one.

### D-E — Budget enforcement has two edges

| Edge | Where | Action |
|---|---|---|
| `maxAgents` | the `invoke` verb, in the manager's handler | refuse, then close root `budget-exceeded` |
| `maxTurns` | `handleWorkDone` on the root | close `budget-exceeded` after recording |
| `deadline` | the conversation reconciler, requeue at the deadline | close `budget-exceeded` |

- Closing a root closes every member with `causedBy` naming it, reason
  `root-closed`.
- `budget-exceeded` runs escalation FIRST (D5) with a manager-written digest
  (limit, counts, member list), then closes — so the thread exists to carry it.
- `agentsInvoked` is incremented on the root's status by the manager under
  optimistic concurrency; a conflict retries. Ten members is the design point.

### D-F — The MCP server is a thin client of the manager

- `platform/mcp-aops/`: standard-library Go, shared Dockerfile recipe, MCP over
  streamable HTTP. It holds ONE credential: the manager's derived token for
  itself, context `mcp-aops`.
- It authenticates CALLERS by a per-conversation token the manager injects into
  the runtime pod as `AOPS_MCP_TOKEN`, derived with context
  `coordinator:<name>:<conversation>`. The server forwards it; the MANAGER
  validates and enforces the `agents[]` list and root scope on a new
  `/coordinate/*` surface. The server never decides reach.
- Tools: `list_agents`, `list_conversations`, `get_conversation`, `get_tree`,
  `invoke`, `close`, `escalate`, `read`. All complete within the request.
  `list_conversations` returns each conversation's `brief` (D-I) beside name,
  title, phase and pipeline, so a caller deciding WHICH conversation it means
  never has to `read` one.
- **A SECOND REACH CLASS, decided by the manager from the token context alone.**
  A token derived with context `channel-reader:<channel>` reaches the
  projection `{name, title, brief, phase, pipeline}` of conversations bound to
  that Channel — `list_conversations` and `get_conversation` return that
  projection and nothing else, and every verb is refused. The server forwards
  the token exactly as it does a coordinator's; it learns nothing about
  channels. First caller: the voice lane's analyzer (a separate change), which
  must pick the conversation a spoken reply belongs to without reading any
  conversation's transcript. The bound is the Channel because "a conversation
  you could mean" is one that has a thread where you are speaking — the same
  bound `conversation-close` uses for who may end one.
- Wired as an `MCPConfig` the chart renders, bound through
  `global.builtinToolsets.agentops-coordinate`; a Coordinator's capability
  binds it as any MCPConfig. Reach through the egress proxy is unchanged.
- Alternative rejected: the server holding the manager's adapter token and
  enforcing reach itself. Two places deciding reach, and a credential stronger
  than any caller's in a component every coordinator reaches.

### D-I — `brief` is what a conversation is about, written by the agent

- `Conversation.status.brief` — a `MaxLength=512` string: what the conversation
  CONCERNS — the objects, the ask — one or two sentences somebody could
  recognise the conversation by. NOT where it stands: status flips per run,
  a brief accretes.
- Reported by the runtime as `brief` in `/work/done`, beside `result`.
  **Latest-wins**, the same rule as `runtimeContextId`: the agent restates
  what the conversation is about, having read the whole context. A report
  omitting it leaves the stored one; a runtime that never sends it leaves
  `title` as the only description, which is today.
- `dispatch/templates/format.md` asks for it in one sentence; the runtime
  extracts it from the run as it extracts the context handle. It is a contract
  field, not a parsed section of the answer — the manager parses nothing.
- Callers: the coordinator's `list_conversations` (D-F), the channel-reader
  projection (D-F), the console's conversation list.
- Alternative rejected: the manager or a reader deriving it from
  `status.runs[]`. Deriving requires reading the transcript, which is exactly
  the reach the projection exists to withhold.
- Named `brief`, not `summary` (a digest of what happened) and not `context`
  (`runtimeContextId` and the context volume already own the word).

### D-G — Console reads two more kinds and derives the tree client-side

- The adapter's watch set grows by `agentcapabilities` and `coordinators` (RBAC in the
  chart).
- The tree is derived from the conversation snapshot by `causedBy`; no new
  endpoint. The incident view is a route on the root; member transcripts link
  up.
- Fixture for `npm run screenshots` gains one root with three members.

### D-H — Delivery is phased, one PR, each phase green on its own

| Phase | Ships | Master consistent because |
|---|---|---|
| 1 | `AgentCapability`, `AgentCapabilitySpec`, `capabilityRef`, resolver | pure addition |
| 2 | `Coordinator`, `causedBy`, routing, budget, escalation, `closeReason`, `brief` | a Coordinator nobody applies changes nothing; a runtime not reporting `brief` changes nothing |
| 3 | `mcp-aops`, chart wiring, toolset | the verb surface behind the wall |
| 4 | console | view only |
| 5 | docs, rules, generators, screenshots | last, per `documentation.md` |

## Risks / Trade-offs

- **Deleting a Coordinator cascades nothing**, exactly as deleting a Pipeline
  does: open roots keep running on their snapshot — budget, escalation
  channels, members — until the budget closes them or a person does. No
  finalizer, no ownerRef, no `delete` on conversations.
- **Root context growth.** Unbounded by design here; the member descriptions
  and `maxTurns` are the controls. A later change may summarise.
- **`DeliverInputs` on the queue** was already timing-dependent; this change
  adds the `escalatedAt` fence and nothing else. A full move to
  `status.runs[].inputs[]` is a separate change.
- **A Coordinator claiming a chat surface** answers a bare message with no
  thread until it escalates. The person sees nothing until then; the source's
  `Wired` count says a claimant exists. Documented, not hidden.
- **Two CRDs to `kubectl apply`** on upgrade, per `gotchas.md`. CHANGELOG.
- **Token derivation contexts** now have five families —
  `coordinator:<name>:<conversation>` and `channel-reader:<channel>` join
  `signal-adapter:<name>`, `channel-adapter:<name>` and `mcp-aops`; the
  derivation is one function, listed in `contracts.md`.
- **`brief` is agent-written and unverified.** A wrong brief misroutes a
  reader's choice, never the manager's: nothing in the manager reads it.

## Migration Plan

1. Apply the two CRDs (`kubectl apply -f chart/crds/`) — Helm never upgrades
   one.
2. `helm upgrade`. No Pipeline changes. `mcp-aops` renders only when
   `coordination.enabled: true`.
3. Adopt: write an `AgentCapability`, list it in a `Coordinator`, point the Coordinator at
   a source.

Rollback: delete Coordinators. Open roots are not touched — they keep their
snapshot and end by budget or by hand, and nothing cascades. Pipelines are
unaffected.

## Open Questions

- Does the reference install's demo wire a Coordinator? Default: no; a guide
  shows it.
- `deadline` default — 1h proposed.
- Whether `get_tree` should return member results inline or by reference
  (size).
