## Why

A Coordinator's root conversation decides every turn: invoke an agent,
escalate to a person, close, or answer the person who asked. Nothing checks
that a turn made any of those moves.

Issue #429 is the fourth confirmed shape of the same failure. The
coordinating agent reads its input, writes prose, and stops:

- a verdict nobody receives
- a root nobody closes
- a "this session has no tools" answer with eight agents in `agents[]`

Three prompt rewrites each fixed their own incident, and a new shape
appeared. The prompt is not where this is fixed.

Three mechanical facts keep producing it. None of them is the AgentProfile's
prompt.

- **The manager dispatches a Coordinator root as a repository agent.** The
  shared work-unit templates tell every conversation it has a checkout and a
  `.claude/agents/<name>.md` role file to adopt. A root has neither, and the
  drift transcripts in #429 quote that framing back almost verbatim.
- **The model is offered tools it may not use.** A root's wiring grants
  `mcp__aops__*` and nothing else, yet the vendor SDK still lists Bash, Read,
  Glob and web access, and the runtime denies each call after the model has
  chosen it. A visible-but-denied tool is the contradicted toolbox that makes
  the model distrust the prompt's "your tools are the agents".
- **The manager serves every move and remembers none.** `invoke`, `escalate`
  and `close` are the manager's own `/coordinate/*` handlers. They act and
  forget, so when the run ends the manager cannot tell a turn that made no
  move from a healthy one.

## What Changes

- **The verbs stamp the move.** When `invoke`, `escalate` or `close` succeeds
  for a caller with an inflight run, the handler marks that inflight run as
  having moved. A refused verb and every read leave no mark. The evidence is
  the manager's own, never a runtime's report.
- **A root turn must end in a move, enforced at `/work/done`.** When a run on
  a conversation carrying `coordinatorRef` reports `succeeded`, the manager
  reads that mark. The turn made no move when the mark is absent AND none of
  the run's inputs came from a person (origin `channel`).
  - First miss: the manager appends ONE corrective input to the same
    conversation naming the rule and the roster. The next run dispatches as
    usual.
  - Second consecutive miss: the manager escalates the root with a digest
    saying the coordinating agent ended two turns without a move.
  - Nothing here picks the move. The model still decides every turn. It just
    cannot end a turn without deciding.
  - A corrective turn is an ordinary turn and spends `maxTurns` like any
    other.
- **A fourth input origin, `manager`**, marks the corrective input so the
  record shows who wrote it and the second-miss rule can see the first.
  **BREAKING** only in the CRD sense: `OriginKind` gains a value and
  `status.inflight` gains the move mark, and the CRD is re-applied as every
  CRD change is.
- **Visible equals allowed, in `runtimes/claude`.** The built-in tools the
  model is offered are derived from the composed allowlist. A conversation
  whose allowlist grants no built-in sees none. MCP tools arrive only through
  the bound servers, as today. The per-call `canUseTool` denial stays as the
  second wall. This narrows what is OFFERED, never what is ALLOWED, so it is
  one more form of the denial `RT-3` already requires. `docs/contracts/runtime.md`
  is NOT edited by this change.
- **A Coordinator root gets its own work-unit framing.** A dispatch template
  for conversations carrying `coordinatorRef`. No repository, no role file,
  no "act within your tools" fallback. It carries the input, who it came from
  (a signal, a person, a member's result), and the delivery section. The
  AgentProfile's `systemPrompt` and the chart's `systemPromptPrefix` are
  untouched and still appended. That is the adopter's surface and stays so.
- **The aops tool descriptions state the rule.** Every turn ends in `invoke`,
  `escalate`, `close`, or an answer to a person. The model reads it where it
  reads the tools, so the corrective input refers to something it has seen.
- **A cascading close closes descendants before the ancestor.** Today the
  parent closes first, and a failure mid-walk leaves a closed root over open
  members. Children first, then the parent, so a partial failure leaves a
  reachable open root rather than orphans.

**Explicitly out of scope:**

- No automatic invoke of an `agents[]` entry by signal source. That is
  Pipeline behaviour. The Coordinator's point is that the model routes.
- No automatic close of members or roots on idle. A live member keeps its
  context and is the better target for the next task. Closing remains the
  coordinator's decision, which the turn rule now makes it take.
- No change to the coordinator's prompt wording. The prompt stays as PR
  #363 left it.
- No consent-before-mutation guardrail (a separate, already-scoped item).
- No change to the Claude Code preset system prompt the runtime appends the
  profile to.

## Capabilities

### New Capabilities

None.

### Modified Capabilities

- `coordination-loop`: a Coordinator root's successful run that made no
  move and answered no person is corrected once, then escalated.
- `coordinator-model`: a Coordinator root's work unit is framed as
  coordination, not as a repository agent's task.
- `aops-mcp-server`: the verbs stamp the move on the caller's inflight run,
  and the tool descriptions state the turn rule.
- `agent-definition-tools`: a built-in tool outside the composed allowlist
  is not offered to the model where the vendor permits narrowing.
- `conversation-close`: a cascading close closes descendants before the
  ancestor.
- `conversation-provenance`: the `manager` input origin.

## Impact

**Code**

- `platform/manager/internal/httpapi/coordinate.go`: the three verb handlers
  stamp the caller's inflight run.
- `platform/manager/internal/httpapi/server.go` (`handleWorkDone`): the
  move check, the corrective input, the second-miss escalation.
- `platform/manager/internal/dispatch/`: a `coordinate.md` template and the
  `coordinatorRef` branch in `Next`.
- `platform/manager/api/v1alpha1/conversation_types.go`: `OriginManager`
  and the inflight move mark, with regenerated deepcopy and `chart/crds/`.
- `platform/manager/internal/chat/coordinate.go`: cascade order.
- `platform/mcp-aops/`: tool descriptions.
- `runtimes/claude/runtime.js`, `tools.js`: offered built-ins derived from
  the composed allowlist.
- `runtimes/ollama`, `runtimes/copilot`, `runtimes/claude-cli`: untouched.
  The move mark comes from the manager, so no runtime owes anything new.

**Reference docs made untrue**

- `docs/contracts.md`: the aops section's verb table, the move mark and the
  turn rule.
- `docs/concepts.md`: the Coordinated agents section (the turn rule, the
  corrective input, the `manager` origin) and the provenance table.
- `docs/crds/conversation.md`: generated origin enum and inflight fields.
- `docs/runtimes/claude.md`: the offered built-ins follow the composed
  allowlist.
- `docs/CHANGELOG.md`: the manager, runtime and chart entries, with the CRD
  re-apply line.

**Adopter site made untrue**

- `docs/guides/coordinate-agents.md`: what happens when the coordinating
  agent ends a turn without a move, and what the person sees.
- `docs/getting-started.md`: the coordinator-mode walkthrough's description
  of a stuck root, if it describes one.
- `docs/introduction.md` / `docs/index.md`: only if either claims the
  coordinator "decides when to close" without qualification. Checked at
  documentation time, not assumed.
