## Context

See proposal.md, Why. The pieces this design moves, as they stand today:

| Piece | Today |
|---|---|
| `/coordinate/invoke`, `/escalate`, `/close` | the manager's own handlers (`internal/httpapi/coordinate.go`), authenticated by the caller conversation's token. They act and record nothing on the caller |
| `/work/done` | records the run, routes a member's result upward, counts a root's turn against `maxTurns`, fans the reply out. Judges nothing about what the run did |
| `status.inflight` | `runId`, `inputIds`, `dispatchedAt`. Cleared when the run completes |
| `status.runs[].inputs[].origin` | `signal`, `channel` or `member` |
| dispatch templates | `task.md`, `investigate.md`, `reply.md`, shared by every conversation. All three frame a repository agent |
| `runtimes/claude` | composes the allowlist, passes NO `allowedTools` to the SDK, denies every ungranted call in `canUseTool`. The SDK still offers the model its whole built-in set |
| close cascade | `closeWithCascade` closes the conversation, then walks `causedBy` children |

Constraints:

- `docs/contracts/runtime.md` is not edited (`runtime-spec.md`). Every runtime
  behaviour here must be a form of an obligation the page already states.
- The manager composes meaning and decides nothing a model should
  (`invariants.md`). It may refuse an undecided turn. It may never decide
  the turn.
- A CRD field addition re-applies `chart/crds/` and re-runs
  `docs-generate.py`.

## Goals / Non-Goals

**Goals:**

- Detect a Coordinator root's turn that ended without a move, from evidence
  the manager produced itself.
- Hand the turn back once, then escalate. Never pick the move.
- Stop offering the coordinating model tools it may not use.
- Stop framing a root as a repository agent in the work unit.
- Close a subtree children-first.

**Non-Goals:**

- Any read of `toolCalls[]` for a decision. It stays telemetry (`API-6`).
- Any change to the three other runtimes. Nothing here is owed by a runtime.
- Any change to the Claude Code preset system prompt. The profile is
  appended to it today and stays so.
- Any auto-invoke, auto-close, or idle policy. See proposal, out of scope.

## Decisions

### D-A — The verbs stamp the move. `/work/done` reads the stamp

**Chosen.** `InflightRun` gains `moved bool`. The invoke, escalate and
close handlers set it on the CALLER's inflight run after the verb succeeds,
in a status patch of their own. `/work/done` reads it from the inflight
entry it already loads, before clearing it.

**Rejected: read `toolCalls[]` at `/work/done`.** Three reasons:

- it needs a runtime contract change, since `toolCalls[]` is optional today
- it trusts a runtime's report for a decision
- it breaks on a runtime that reports none

The handlers ARE the verbs. The manager already knows every move the moment
it happens.

**Rejected: an `end-turn` verb the model must call.** The model can skip it
exactly as it skips `close`. Absence is only visible when the run ends.

Details:

- An `invoke` that ATTACHES counts. The model delegated.
- An `escalate` on an already-escalated root counts. The model decided.
- A refused verb (out of scope, unknown agent, cycle, spent budget, missing
  reason) does not count. Nothing happened.
- `close` on the caller itself: the conversation is `Closed` when
  `/work/done` lands. A Closed conversation is never judged. The stamp is
  still written, for the record.
- A caller with no inflight run (a verb from a pod with no unit) stamps
  nothing. Nothing is dispatched, so nothing is judged.
- The stamp is written in its own optimistic-locked patch, retried on
  conflict, so a concurrent `/work/done` racing the last verb never loses
  the mark. Lost mark = false miss, which is the one failure to design out.

### D-B — The judgement runs in `/work/done`, after the record, before the phase

Order inside `handleWorkDone` for a root (`spec.coordinatorRef != nil`):

1. Record the run and its inputs, as today.
2. Judge: `status == succeeded` AND `inflight.moved == false` AND no recorded
   input of this run has origin `channel`. Otherwise no miss.
3. On a miss, decide first or second: the run's recorded inputs are exactly
   one input of origin `manager` → second. Anything else → first.
4. First: append the corrective input (D-C). Second: `Router.Escalate` with
   the digest.
5. Phase, fan-out, member routing, budget edge, as today. A corrective input
   is pending, so the phase lands `Queued` and the next dispatch is the
   ordinary one.

A member (`causedBy` set, `coordinatorRef` nil) is never judged. A member's
result IS its move, and it reaches the parent by the existing route.

A nested Coordinator's root (both set) is judged as a root.

### D-C — The corrective input is an ordinary input of origin `manager`

| Field | Value |
|---|---|
| type | `task` |
| origin | `{kind: manager, name: move-per-turn}` |
| payload | fixed text naming the rule and the roster, the Coordinator's `agents[]` names read at the moment of the miss |

Dispatched through the same template as any root input (D-E), so the model
sees "from the manager" where it sees "from a member" otherwise.

Delivered to the root's bound threads by the EXISTING per-destination rule
(`conversation-opens-with-its-input`), attributed to the manager. A person
watching the thread sees the hand-back when it happens.

No exemption and no new clause. That spec's own rule is that a new input
kind inherits delivery unedited.

**Rejected: a new input type.** The type says what the text IS (a task). The
origin says who sent it. The record already has both axes.

**Rejected: re-dispatching the ORIGINAL input.** The original is already
recorded as consumed. Re-queuing it would record it twice and make the
history lie about what the person or member said.

`OriginKind` gains `manager`. The CRD enum widens, the console's origin
rendering gets one more label, nothing else reads the enum by value.

### D-D — The second-miss digest is an escalation, not a close

`Router.Escalate` on the root: `escalatedAt` + `escalationMessage`, posted to
the bound channels by the existing path. The root stays open for the person.

**Rejected: close `budget-exceeded`.** Nothing was exceeded. The person
should see the stuck root and decide, which is what escalation is for.

The digest is fixed text: the agent ended two turns without invoking,
escalating or closing, the roster, and the last run's result (bounded).

An already-escalated root on a second miss: `Escalate` is a no-op today
(no replayed digest). The miss is then logged and the corrective loop
stops. The person already holds the thread.

### D-E — A root-only dispatch template, selected by `coordinatorRef`

`dispatch.Next` already has the Conversation. A new `coordinate.md` template
is rendered for every input type when `c.Spec.CoordinatorRef != nil`, in
place of `task.md` / `investigate.md` / `reply.md`. Members keep the three.

The template carries:

- the input, verbatim
- who it came from, from the input's origin: `signal <source>`,
  `person on <channel>`, `member <entry>`, `the manager`
- the delivery section, unchanged
- nothing about a checkout, a role file, or acting within tools

`profile.spec.prompt` (a repo-held prompt file) wins over the template today
for `task`/`alert`. It keeps winning. An adopter who ships their own prompt
file for a coordinator profile gets their file.

The profile's `systemPrompt` is appended by the runtime as today. Untouched.

**Rejected: a `kind: coordinator` flag on the template variables.** Three
templates each growing a branch is the drift this change removes.

### D-F — `runtimes/claude` offers only the granted built-ins

The composed allowlist already exists at session open. Derive from it the
set of BUILT-IN base names (every entry that is not `mcp__*`, stripped of
a `(...)` scope). Pass that set to the SDK as the built-in tools to offer.
Empty set → no built-ins offered.

The SDK option is verified at implementation: the Claude Agent SDK's
`tools` option (an explicit list of built-in tools), with `disallowedTools`
as the fallback form if `tools` is unavailable in the pinned version.

- MCP servers stay as today: bound via `mcpServers`, `strictMcpConfig`.
- `canUseTool` stays as today and still decides every call. Offered and
  allowed remain two decisions, so the composed-allowlist tests keep
  passing unchanged.
- It is a session-affecting option, so it joins the session fingerprint
  (`runtime-session-reuse`). A unit with a different allowlist re-opens the
  session, which the fingerprint rule already requires for any
  session-affecting config.

**Why this stays within `RT-3`:** `RT-3` requires the composed allowlist to
be enforced and ungranted calls denied. Offering fewer tools is a stricter
form of the same denial. The page is not edited.

The other runtimes: `ollama`'s harness builds its own tool list from the
allowlist already. `copilot`'s vendor owns the vocabulary and enforces per
call. Neither changes.

### D-G — The aops descriptions state the rule

The MCP tool descriptions on `invoke`, `escalate` and `close` gain one
sentence: every turn ends in one of these three or an answer to a person,
and a turn ending otherwise is handed back once, then escalated. The read
tools say they are reads, not moves.

This is where the model reads the rule, so the corrective input refers to
something it has seen. It is not a prompt edit: the profile and its prefix
are untouched (proposal, out of scope).

### D-H — Children first in the close cascade

`closeWithCascade` reorders: walk `causedBy` children (recursively, each
children-first), then close the conversation itself. Same reason verbatim.
A failure mid-walk returns before the ancestor closes, so a retry resumes
from the open members. Already-closed members are skipped as today.

`CloseBudgetExceeded` already closes members before escalating the root.
Unchanged.

## Risks / Trade-offs

- **A legitimate pause looks like a miss.** A root that must wait for
  something external with nothing to invoke yet. → The rule still holds:
  waiting is `escalate` ("waiting on X, nothing to do until then") or an
  invoke of an agent that can watch. A turn that ends in prose nobody acts
  on is the failure, whatever the prose says.
- **The corrective input costs a model turn.** One extra run per miss. →
  Bounded to one, counted against `maxTurns`, and cheaper than a root that
  sits open for fifteen hours.
- **A stamp lost to a status-patch race reads as a false miss.** → D-A writes
  the stamp optimistic-locked with retry. A false miss costs one corrective
  turn, never a wrong decision.
- **An adopter's `prompt` file for a coordinator profile bypasses D-E.** →
  Deliberate. Their file is their framing.
- **The SDK option for offered tools differs across SDK versions.** → D-F
  names the fallback. The permissions tests pin that `canUseTool` still
  governs, so a version where neither option exists degrades to today's
  behaviour, not to a wider one.
- **A root escalated on a second miss stays open for ever if nobody
  replies.** → Same as any escalated root today. Out of scope, and the
  existing release-wide autoclose covers it where enabled.

## Migration Plan

1. Re-apply `chart/crds/` (the `manager` origin and `inflight.moved`).
2. Roll the manager. Roots already open are judged from their next run.
   A root with a pending corrective input from before an upgrade cannot
   exist, so there is no mixed state.
3. Roll `runtime-claude`. A pod built before the roll keeps offering the
   whole built-in set until it is recycled. Nothing breaks, the narrowing
   just arrives per pod.
4. Roll `mcp-aops` for the descriptions.

Rollback: roll the previous manager. An unread `inflight.moved` and an
unknown origin value are ignored by the older binary. Conversations keep
working. A corrective input already queued dispatches as a plain task.

## Open Questions

None that change the specs or the task list. The exact SDK option name in
D-F is verified at implementation and falls back as stated.
