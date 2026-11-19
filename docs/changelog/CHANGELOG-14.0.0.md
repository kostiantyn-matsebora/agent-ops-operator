## [14.0.0] — 2026-10-09

**Every image below is rebuilt against `golang:1.27`, no behaviour change
beyond clearing two stdlib HIGH CVEs:** CVE-2026-78667 (`net/http`) and
CVE-2026-97031 (`crypto/tls`), fixed upstream in 1.26.9 and 1.27.2.

- `manager` 0.57.4, `console` 0.38.4, `housekeeping` 0.2.5, `context-sync`
  0.2.5, `egress-proxy` 0.2.6.
- `channel-telegram` 0.25.1, `gateway-telegram` 0.6.1, `signal-telegram`
  0.6.5.
- `signal-alertmanager` 0.7.5, `signal-ha` 0.4.1, `signal-k8s-events` 0.4.6.
- `signal-cron` 0.0.1-rc3 and `runtime-ollama` 0.1.2.

### Added

- **The console's Conversations page is one chat-shaped view**, not a table
  plus a separate detail page: an inbox of scopes, a list rendered as a
  coordination tree, and a thread pane that switches in place. Replaces
  `pages/Conversations.tsx` and `pages/Conversation.tsx`.
  - Unread counts MESSAGES (`signal`/`agent`/`relay`) rather than lateness,
    summed per scope, and a new **Mark unread** reclaims one as a
    reader-scoped rewind on the read verb.
  - Pane widths and the inbox collapse state persist under a guarded
    `localStorage` key, `agentops.console.layout`, and the `/close` confirm
    dialog's "don't ask again" opt-out persists under a second guarded key,
    `agentops.console.skipCloseConfirm` — browser preferences beside the nav
    fold, and still no conversation state.
  - See [console.md](console.md#conversations).
- **Two new CRDs let a conversation's own agent invoke other agents.**
  `AgentCapability` extracts the six capability fields a Pipeline already
  carries inline (`profileRef`, `runtimeRef`, `serviceAccountName`,
  `toolsets`, `mcpConfigs`, `persistence`), referenced with `capabilityRef`
  instead of repeated. `Coordinator` wires a composition: sources claimed
  like a Pipeline's, escalation `channelRefs`, a typed `agents[]` list of
  members it may invoke (`capabilityRef` for a plain member, `coordinatorRef`
  to nest one Coordinator inside another), and per-level `limits`
  (`maxAgents`, `maxTurns`, `deadline`).
  - A member's result returns as an input on its caller (`spec.causedBy`, one
    hop). Only the tree's uncaused root ever opens a human thread, by calling
    `escalate` — a nested member bubbles the message to its own parent
    instead, one hop at a time, until a call reaches the root.
  - A new component, **`agentops-mcp-aops`**, is the MCP server a
    Coordinator's capability binds to reach these verbs — a thin forwarder
    holding no credential of its own, behind the egress-proxy wall, rendered
    only under `coordination.enabled` (off by default).
  - Not breaking: every existing Pipeline is unaffected, and both CRDs are
    inert until referenced. See
    [concepts.md](concepts.md#coordinated-agents),
    [contracts.md](contracts.md#the-aops-mcp-server-contract) and
    [ADR 0002](adr/0002-coordinated-agents.md).
- **The console's topology is a network, in three views of one activity
  feed.** Model draws the declared objects, Components one node per component
  plus the systems outside, and Infrastructure every pod. Each view has its
  own element classes, and remembers its own layout (concentric, cola or
  dagre) and boxes (bundle or route on Model, cluster node on
  Infrastructure). Traffic is a rate-driven stream with every recorded hop
  drawn as a clickable pulse.
  - A hop feed, per-edge hop history and a content panel show what each hop
    carried, joined from the conversation's status.
  - A pipeline selector, scope-to-route with depth, and find and hide narrow
    the picture, each counting what it hides.
  - Window replay (1, 5, 10 or 30 minutes, ten-second frames, three speeds)
    and conversation replay (one run, hop by hop).
  - The runtime edge is drawn from the Pipeline, never from the profile.
  - The canvas fits the viewport. The navigation folds to its icons, the
    column beside the canvas folds to a strip, and the Display, hop feed and
    History cards each fold to their title, so the picture takes the whole
    screen. Every mark's glyph clears its outline, and an unrouted edge bows
    around the marks between its ends, both held by tests. A pipeline wears
    the icon `spec.icon` declares in its mark and beside its name wherever the
    console names it.
  - See [console.md](console.md#topology). Console 0.39.0.
- **Model calls and tool calls are recorded as activity.** A runtime may
  report `turns[]` and `toolCalls[]` with its work result, and the manager
  emits one `model.call` or `tool.call` hop each, with the runtime image as
  `from`. Events gain a bounded `data` map, and the metrics gain
  `agentops_model_calls_total`, `agentops_model_call_tokens`,
  `agentops_tool_calls_total` and `agentops_tool_call_duration_seconds`. A
  report over its bounds is refused with `400`. See
  [contracts/runtime.md](contracts/runtime.md#completion-report). Manager
  0.58.0, runtime-claude 0.10.0, runtime-ollama 0.2.0, runtime-copilot 0.2.0.
- **`SignalAdapter` and `ChannelAdapter` gain `spec.externals[]`**, a `name`
  and a `kind` (`sender`, `api` or `kubernetes`) for each system the
  implementation faces. Metadata only: nothing verifies it and it grants
  nothing. The console draws each one beside its adapter. The console,
  `prometheus`, `kubernetes`, `home-assistant` and `telegram` bundles declare
  the externals of the adapters they ship.
- **The console reads `cronjobs` in its namespace**, read-only, beside its
  existing pod and deployment grant, so the housekeeping CronJob is drawn as a
  component.
- A fourth built-in, risk-split toolset, `agentops-websearch` (`WebSearch`),
  beside `agentops-observe`/`-shell`/`-edit`. The kubernetes bundle's admin
  route (`k8s-operate`) and the home-assistant bundle's ops route (`ha-ops`)
  bind it by default when that bundle's own wiring is enabled — the
  read-only/control routes (`k8s-observe`, `ha-control`) do not gain it. Not
  breaking: both routes are off by default, and an install declaring its own
  Pipeline is unaffected until it adds the toolset name itself.

### Changed

- **A Conversation's object name is now a deterministic word-chain**
  (`alert-node-down-ns`, `member-researcher`), never
  `metadata.generateName`'s random suffix. Non-breaking: existing
  conversations keep their current names. See
  [concepts.md](concepts.md#conversation).
- **`manager.replicas` is a value again, default `2`.** It was hardcoded at
  `1`, because `internal/chat.OpQueue` was in-memory and per-process,
  populated only by the leader's reconciler — and `/channel/ops` served
  every replica, so a poll landing on the non-leader saw a permanently
  empty queue. `ensure-topic` silently never completed for roughly half of
  all new chat-bound conversations.
  - The reconciler now writes a claim on the Conversation CR itself before
    dispatching an op, and a non-leader rejects a poll with `503` (retried
    at once) instead of answering from that queue.
  - A crashed leader's in-flight claim recovers through the same
    leader-election failover every reconciler already uses.
  - New value: `claimStalenessSeconds` (default `90`), the bound past which
    the current leader treats its own claim as abandoned and retries.
  - **Breaking for a hand-rolled channel adapter.** `GET /channel/ops` may
    now answer `503`. A conforming adapter retries at once, the same way it
    already retries an empty `204`, and never logs it as a failure. Both
    shipped adapters (`channels/telegram`, `platform/console`) do this.
- **Closing or deleting a conversation now cascades to every conversation it
  caused**, for every originator — a human `/close`, the console's bulk
  close, the idle timer, and a bulk delete — not only the coordinator's own
  MCP `close` verb. Close reaches every live descendant recursively, through
  the same helper that verb already used. Delete reaches every descendant,
  already left `Closed` by that cascade. Neither widens who may close or
  delete anything — both were already bounded exactly as any other close or
  delete is.
- **The fixing loop re-runs an unexplained red check once before asking, and
  its fixing step runs on branches that lag master.** A check the fixer finds
  nothing in the tree for gets one empty commit pushed, so CI judges the head
  under the merged workflows. Red again, it is disputed. The `fix` job checks
  out the default branch's copy of `review-dispatch.yml` for the action's
  validation, which refused every branch cut before that file last changed.
- **A red pull request is the fixing loop's to fix, and a person is owed
  nothing until it is green or only genuine disputes remain.** Measured on
  #259, where the loop ended "clean" beside a red pull request over a dispute
  it had manufactured, and could not hear the thread being resolved.
  - The dispute guard leaves CI: `docs-task` no longer fails on an unanswered
    dispute, since the open thread already holds the merge. The archive hook
    keeps the refusal. `dispute-answered.yml` and `rerun-ci-job.py` are
    deleted, and a person's comment on a waiting pull request starts a round
    instead.
  - Every failed required check is work, with no exemption. A fix the report
    claims but the patch does not evidence is unaddressed, never a dispute,
    and a round that lands nothing while items remain eligible starts the next
    round itself, within the bound.
  - `loop:waiting` is a state of its own (a person is owed an answer) beside
    `loop:stalled` (the machine stopped). A new `conveyor-sweep.yml` re-reads
    waiting pull requests every fifteen minutes and resumes one whose disputes
    are answered or resolved.
  - A round starts only once the head's CI run and review run have both
    concluded, and the checks count as consulted only when every required
    check run has completed.

- **The conveyor is one state machine.** The label-driven line (implement, fix,
  archive) decided its rules in five workflows and eleven programs, and ten of
  them disagreed. `conveyor.py` now holds the station and loop tables and every
  decision, and each program gathers facts, asks it and acts.
  - A round that ran a model counts toward `max_rounds`, a timed-out one
    included, and the cap is enforced before a round starts.
  - `docs-task` no longer reports a running round, so a round cannot start the
    next one through its own refusal. The `/opsx:archive` hook still does.
  - The gate accepts the grant the carry accepts, and re-checks it on review
    and CI completions too, so removing `conveyor:run` stops a running loop.
  - The archive station starts only from a finished change.
  - `carry-grant.py` and `carry-from-pr.sh` are replaced by `carry.py`.

- The kubernetes bundle's `k8s-engineer` profile now states the install's
  pod-execution posture to the agent. While
  `global.agentops.runtimeDefaults.allowPodExecution` is off, its role ends
  with a paragraph saying pods cannot be created or entered and workload pod
  templates cannot be edited, so a request for one is declined with the reason
  instead of attempted and reported as an RBAC refusal. Rendered from the same
  value the RBAC reads, and absent when the gate is on. An install overriding
  `kubernetes.profile.systemPrompt` gains the paragraph on upgrade, appended
  after its own text. `kubernetes.profile.podExecutionWithheldPrompt: ""`
  declines it. Bundle `kubernetes` 0.3.2.
- The home-assistant bundle's `ha-admin` toolset now ships ten registry-removal
  tools by default — entities, devices, areas/floors, zones, helpers and
  integrations, categories, labels, groups, to-do items and calendar events. 62
  of 78 registered tools ship, up from 52. Sixteen stay withheld, restated in
  three classes: restart and backups, software and preferences, and deletes of
  what a person WROTE (automations, scripts, scenes, dashboards). The
  `ha-operator` profile's describe-and-stop rule now states it is a
  confirmation gate: a person's reply in the thread authorizes the described
  action, and the agent neither asks again nor tells them to run it
  themselves. Not breaking: the admin toolset renders only under
  `adminMcp.enabled`, off by default, and an install restating
  `adminMcp.toolset.tools` keeps its own list. An install on the default list
  gains the ten tools on `helm upgrade`. Bundle `home-assistant` 0.2.0.
- The release workflow's cluster smoke is now keyed to the tagged COMMIT, not
  the tag. A release publishing many artifacts from one commit smokes it
  once. A passed smoke from any earlier run on the commit is reused. One
  already in flight is waited for rather than raced. Contributor-facing only.
  No published artifact changes.
- **Breaking.** A Pipeline's `spec.runtimeRef` naming an `AgentRuntime` that
  does not exist now turns `Ready` `False`, naming the missing runtime. It
  used to report `Ready=True` and fail only later, when a pod was built.
  `spec.serviceAccountName` is unaffected — that field stays unchecked, since
  confirming it exists needs RBAC the manager does not hold. Manager 0.57.4.

### Fixed

- The home-assistant bundle's `ha-observability` and `ha-actions` toolsets
  matched no tool on Home Assistant 2026.9 or later. That release prefixes every
  LLM tool with the domain of the integration offering it, and the prefix
  differs per tool: `GetLiveContext` became `homeassistant__GetLiveContext`,
  `HassTurnOn` became `intent__HassTurnOn`, `HassLightSet` became
  `light__HassLightSet`. The allowlists fail closed, so an agent on the
  `ha-control` route simply had no Home Assistant tool, and nothing reported
  it. Both toolsets now list each tool under its bare name and its prefixed
  name, and the half an instance does not register is inert. Not breaking: an
  install overriding `home-assistant.mcp.toolsets.*.tools` keeps its own list
  and must add the prefixed names itself.
- **The self-heal reaper's `cronAdapter` pinned `signal-cron:0.2.0`**, a
  version `signal-cron` never shipped — only `0.0.1-rc1`/`rc2`/`rc3` exist.
  Off by default (`reaper.enabled: false`), so no install pulled it, but an
  install already on `wiringMode: coordinator` is one flag from
  `ImagePullBackOff`. Now `0.0.1-rc3`.

### Upgrade

1. **Apply the CRDs first** — `kubectl apply -f chart/crds/` — before
   `helm upgrade`. Helm never upgrades a CRD, and an old one silently prunes
   `spec.externals` from every adapter CR, and `spec.coordinatorRef`,
   `spec.causedBy` and the budget fields from every `Conversation` — the
   coordination verbs then refuse every caller with "token does not match
   the declared identity", since the field they authenticate against was
   never written. This step is also what INSTALLS the two new kinds this
   release adds, `AgentCapability` and `Coordinator` — Helm never creates a
   CRD it did not already own either.
2. `helm upgrade`. Nothing to restate — the check arrives with the manager
   image.
3. A Pipeline already carrying a dangling `runtimeRef` turns `Ready=False`.
   Apply the missing `AgentRuntime`, or correct the name, and `Ready`
   converges with no further edit.
4. **Nothing else is owed, since the topology change is additive.**
   - A runtime image older than this release reports no turns, and the graph
     simply draws no model or tool hops for it.
   - An adapter CR with no `externals` draws no senders beside it.
   - The console's persisted display selections are re-keyed per view, so a
     selection saved by the old graph is dropped rather than misapplied.
5. **The two new CRDs ship inert.** Nothing references an `AgentCapability`
   or a `Coordinator` until you write one, and `coordination.enabled` (off
   by default) is what renders the new `agentops-mcp-aops` component —
   leaving it off costs nothing.
