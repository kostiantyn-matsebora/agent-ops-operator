## Why

`coordinated-agents` (ADR 0002) ships the `Coordinator` primitive but
defers "does the reference install wire a Coordinator?" as an open question,
answered "no" by default.

Nothing today makes the Coordinator mode testable out of the box. Nothing
lets a bundle's signals converge on one triaging agent instead of many
independent Pipelines. Nothing lets an open incident re-check itself on a
schedule and close when it has healed.

## What Changes

- **New release-wide chart posture** `global.agentops.wiringMode: pipelines |
  coordinator`, defaulting to `pipelines` — today's behavior, byte-identical.
  In `coordinator` mode, every enabled bundle renders an `AgentCapability`
  (not an inline Pipeline) at the SAME privilege level it uses today — a
  bundle shipping two routes at two privilege levels (home-assistant) still
  ships two `AgentCapability` objects, never merged into one. One
  chart-rendered `Coordinator` claims every enabled bundle's signal source
  directly and lists each bundle's `AgentCapability` in its `agents[]`. This
  is a release-wide switch, not per-bundle.
- **Demo mode becomes one instance of `coordinator` mode** with a single
  bundle enabled, rather than separate demo-only logic — "one agent,
  out-of-the-box" falls out of the general mechanism.
- **Per-incident self-resolution is a prompt instruction, not new
  machinery.** Domain agent templates are told they may `/close` their own
  conversation once they judge the problem resolved — reusing the existing
  close path unchanged.
- **New "reaper" capability**, shipped by `coordinator` mode itself (not
  contributed by any domain bundle): a small `AgentProfile` +
  `AgentCapability` pair added as its own `agents[]` entry on the shared
  Coordinator, addressed hourly by a claimed cron signal. Its conversation
  lists its own Coordinator's open root conversations, `invoke()`s the
  original domain agent on each to re-check current state (existing verb,
  existing member-result routing, unchanged), and `close()`s roots judged
  healed.
- **New reach class on `platform/mcp-aops`**: a Coordinator-owner may list
  and close the OPEN ROOT conversations its own Coordinator has opened —
  scoped strictly to that Coordinator's own roots, never another
  Coordinator's tree, never member conversations. This is the one new verb
  surface in this change. Every other mechanism it uses (`invoke`,
  member-result routing, escalation, budgets) is unchanged reuse.
- **No new approval mechanism.** A coordinator "proposing a solution" and
  "coordinating it once approved" is ordinary escalation: the agent proposes
  a plan in its escalation message, a human's reply is the next ordinary
  input on the conversation, and the agent's own judgement decides how to
  proceed — exactly as `coordinated-agents` already defines escalation.

**Explicitly not in scope:**

- No change to `AgentProfile`'s shape or meaning.
- No change to Pipeline behavior in `pipelines` mode.
- No new exclusivity rule between Pipeline and Coordinator wiring. The switch
  controls chart rendering only — an operator may still hand-wire both
  regardless of the flag.
- No structured or enforced approval gate.
- No per-bundle mode granularity.

## Capabilities

### New Capabilities

- `wiring-mode`: the `global.agentops.wiringMode` chart posture — the two
  values, what each renders, the requirement that `pipelines` mode stays
  byte-identical to today, and the explicit absence of any API-server
  exclusivity between Pipeline and Coordinator objects.
- `coordinator-self-heal`: the reaper `AgentProfile`/`AgentCapability` pair,
  its hourly cron claim, and the survey → re-check → close flow inside its
  conversation.
- `coordinator-owner-reach`: the new `mcp-aops` reach class — a
  Coordinator-owner listing and closing its own Coordinator's open root
  conversations, and the bound that keeps it from reaching another
  Coordinator's tree or any member conversation.

### Modified Capabilities

- `coordinator-model`: the chart-rendered Coordinator may claim every
  enabled bundle's source directly, an ordinary use of the existing
  `signalSourceRefs` claiming mechanism.
- `aops-mcp-server`: the verb table gains the Coordinator-owner reach class
  beside the existing coordinator and channel-reader classes, including the
  widened `close` bound.
- `k8s-bundle`, `prometheus-bundle`, `ha-bundle`: each gains a
  `coordinator`-mode rendering branch alongside its existing Pipeline
  rendering, preserving its current privilege split.

**Not given a delta**, and why:

- `agent-capability-model`: a bundle rendering a standalone `AgentCapability`
  changes no requirement of the CRD itself — same six fields, same
  inert-when-unwired rule. Fully covered by `wiring-mode` and the three
  bundle deltas.
- `pipeline-model`: no Pipeline requirement changes in either posture. A
  bundle simply does not render one under `coordinator` mode, which is that
  bundle's own fact to state, not a Pipeline-model requirement.

## Impact

**Code**

- `chart/values.yaml`, `chart/templates/`: `global.agentops.wiringMode`,
  the chart-rendered `Coordinator` object, per-bundle coordinator-mode
  templates (`chart/charts/kubernetes/`, `.../prometheus/`,
  `.../home-assistant/`), the reaper `AgentProfile`/`AgentCapability`
  templates, a `signal-cron` claim for the reaper.
- `platform/manager/internal/httpapi/` (or wherever `/coordinate/*` is
  implemented): the new Coordinator-owner reach class. List open roots, and
  widen `close` from "directly caused" to "any root this Coordinator
  opened," both scoped to the caller's own Coordinator.
- `platform/manager/internal/dispatch/templates/`: a domain-agent prompt
  addition instructing self-close on resolution, and the reaper's prompt.
- `.github/scripts/serviceaccount-guard.py` and any bundle-rendering guard in
  `gotchas.md`'s table, extended to cover the coordinator-mode rendering
  branch per bundle.

**Documents made untrue — reference docs**

- `docs/concepts.md`: the wiring-mode posture, the reaper's role, the new
  reach class in the aops MCP contract table.
- `docs/contracts.md`: the Coordinator-owner reach class added to the
  `/coordinate/*` verb table.
- `docs/configuration.md`: `global.agentops.wiringMode` value documented.
- `docs/console.md` / `docs/console-guide.md`: unchanged unless the reaper's
  conversations need a distinct treatment in the tree view — decided in
  design.
- `.claude/rules/wiring.md`, `gotchas.md` (the bundle-qualification table
  gains the coordinator-mode column).

**Documents made untrue — adopter site**

- `docs/getting-started.md`: if demo mode defaults to `coordinator` posture,
  the walkthrough's description of what gets deployed changes.
- `docs/installation.md`: the new value, and the reaper's cron requirement if
  it needs anything beyond the existing `signals/cron` component.
- `docs/guides/`: the existing "Coordinate agents" guide (from
  `coordinated-agents`) gains the deployment-mode and self-heal sections, or
  a new guide is added — decided in design.
- `README.md`: the "what you write" tab, if the demo's default shape changes.
