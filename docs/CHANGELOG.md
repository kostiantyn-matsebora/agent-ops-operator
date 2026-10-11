All notable changes to this project are documented here, in
[Keep a Changelog](https://keepachangelog.com/en/1.1.0/) format. Versions are
CHART versions. Image tags move independently and are named in each entry.

This file holds the **ten most recent versions**. Older entries are in
`changelog/`, linked at the [foot of this page](#older-versions).

See [the repository](https://github.com/kostiantyn-matsebora/agent-ops-operator)
for the source and the reference material beside this file.

## [Unreleased]

## [14.0.2] — 2026-10-11

### Changed

- **The chart-rendered Coordinator's own agent is an orchestrator, not a
  dispatcher.**
  - **The old failure.** Asked to act, it answered that it had no tools while
    an acting entry sat in its own `agents[]`. Its prompt picked an entry
    whose description matched what arrived. Every shipped description was a
    trigger ("a cluster event", "once the cause is known"), so a person's
    instruction matched none of them.
  - **The new prompt.** It analyses what arrived, delegates to the
    `agents[]` entry whose purpose covers it, and reads the result back
    before deciding what to do next.
  - **Authority.** A person's instruction authorises its own action. A
    change the agent discovers on its own is proposed first.
  - **Descriptions.** Every bundle route's description now states what that
    agent IS, what it CAN do, what it CANNOT, and what to HAND it. That
    covers `kubernetes`'s `k8s-observe`/`k8s-operate`, `home-assistant`'s
    `ha-control`/`ha-ops` and `prometheus`'s `alert-investigator`.
    `ha-control` stops being described as read-only, which it never was.
  - **The cap.** `Coordinator.spec.agents[].description` rises from 512 to
    2048 bytes, since the new shape did not fit the old one.

- **Breaking.** The router (`gateway-telegram`) is now the SOLE holder of the Telegram bot
  credential, inbound and outbound alike. The telegram bundle's
  `surface.credentials` Secret no longer carries an `apiBase` key, and the
  channel adapter reads a new `gatewayUrl`/`gatewayToken` Secret instead.
  Hand-built Secrets must change.
- **Not yet releasable.** The router and `channel-telegram` need new
  image releases before this is installable.
- **`channel-telegram` no longer calls `api.telegram.org` directly.** Every
  send, document upload, forum topic operation and command registration is
  relayed through the router's new `POST /telegram/call/{method}`,
  authenticated with a shared token the chart generates. A new chart-managed
  Secret (`agentops-telegram-gateway`) carries the binding through the same
  `credentialsSecretRef` projection.
- **The router gains a Service and a NetworkPolicy** for its new call
  endpoint, on `router.callPort` (default `8080`).

### Removed

- **`Channel.spec.config.pollingEnabled` is deleted for the Telegram
  channel adapter.** It never polls, so the setting selected nothing. Delete
  the key from the Channel's `spec.config`. A leftover key is ignored.

### Fixed

- **`GET /channel/ops` no longer refuses a non-leader manager replica with
  `503`.** A non-leader now reverse-proxies the poll to whichever replica
  the leader-election Lease names, instead of rejecting it, so a channel
  adapter's long-poll connection pinned to a non-leader pod is answered
  instead of starved. The leader stays the sole claimant and dispatcher —
  only the HTTP routing changed. Falls back to the old 503 only when the
  leader cannot be resolved or reached. Fixes a measured incident on a
  3-replica install where a Telegram topic was never created and three
  replies sat undelivered for 9+ hours — see
  `.claude/rules/invariants.md` and `.claude/rules/gotchas.md` for the
  detail.
- **`GET /activity`, `GET /activity/stream` and `POST /activity` get the
  same dumb-proxy treatment**, for a different reason: the activity log is a
  per-process, in-memory ring buffer with no cross-replica fan-out, and a
  hop the Conversation reconciler emits (reconciliation is leader-elected)
  only ever lands in the leader's own buffer. A console SSE connection
  pinned to a non-leader replica would silently stop seeing reconciler-
  driven events — invoked agents, final answers — for the life of that
  connection, looking exactly like "the coordinator stopped reasoning."
  Unlike `/channel/ops` there is no correctness reason to refuse when the
  leader is unreachable, so these three fall back to serving/recording
  locally instead of a 503.
- **The proxy is now the DEFAULT for every manager route**, not two
  endpoints fixed one at a time. `leaderProxyMiddleware` wraps the whole
  mux: a non-leader proxies any request to the leader before it reaches a
  handler, exempting only `/healthz`, `/channel/ops` and `/activity*`. A handler the proxy didn't reach
  keeps its own unchanged local behavior.
- **The manager's own `NetworkPolicy` now admits its own kind.** The proxy
  needs manager-to-manager traffic on port 8080, a pattern nothing
  previously needed — without this rule the whole fix was a silent no-op
  on any install enforcing `networkPolicy.enabled: true`, confirmed live
  (every poll from the console fell back to its pre-fix local behavior for
  15+ minutes across a stable leader).

### Upgrade

1. `chart/charts/telegram/values.yaml` still names the LAST released
   `channelAdapter.image.tag` and `router.image.tag`. Those binaries do not
   yet implement this change.
2. Tag and release both components.
3. Bump both tags before this chart version is installed anywhere.
4. Rebuild any hand-made channel Secret for the new shape. It carries `gatewayUrl`, the router's call endpoint, and `gatewayToken`, which must equal the router's `TELEGRAM_CALL_TOKEN`. It no longer carries `botToken` or `apiBase`.

## [14.0.1] — 2026-10-10

- `signal-cron` 0.2.0.

### Fixed

- **The self-heal reaper's `cronAdapter` still pinned `signal-cron:0.0.1-rc3`**,
  an unreleased candidate rather than a real version — 14.0.0's own fix for
  `signal-cron:0.2.0` (a version that had never shipped) landed on the closest
  tag that DID exist instead of actually cutting one. Off by default
  (`reaper.enabled: false`), so no install pulled it, but an install that turns
  the reaper on now gets a real release: `signal-cron:0.2.0`, built from the
  same unchanged source the `rc3` tag already shipped.

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
  [contracts.md](contracts.md#what-the-run-did-turns-and-toolcalls). Manager
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
  after its own text; `kubernetes.profile.podExecutionWithheldPrompt: ""`
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

## [13.4.0] — 2026-09-06

**Every first-party image is rebuilt and re-pinned.** Beside the two entries
below, each image carries what landed on it since its last tag:

- `manager` 0.57.3 — status patches on ensure-topic use optimistic locking
  and retry on conflict. Send op ids carry a per-process epoch, so a restart
  cannot reuse one. `x/crypto` and `x/net` bumped.
- `channel-telegram` 0.25.0 — the outbound base URL is configuration
  (`TELEGRAM_API_BASE`, or the bot Secret's `apiBase` key), completed ops
  are remembered in a bounded set, logs are sanitised.
- `gateway-telegram` 0.6.0 — `TELEGRAM_API_BASE`, and the poll loop split
  into named steps.
- `signal-telegram` 0.6.4 — logs are sanitised.
- `console` 0.38.3 — the session cookie is `Secure` behind TLS or
  `X-Forwarded-Proto: https`. Logs are sanitised. The image builds with
  `npm ci --ignore-scripts`.
- `egress-proxy` 0.2.5 — refuses a connection whose original destination is
  the proxy itself.
- `runtime-claude` 0.9.0 — `@anthropic-ai/claude-code` pinned to 2.1.252
  (was `@latest`) and installed with `--ignore-scripts` plus its own
  installer. `apt-get upgrade`, so a cached layer cannot keep a patched CVE
  out. The prompt file is confined to the workspace. Logs are sanitised.
- `runtime-copilot` 0.1.1 and `runtime-ollama` 0.1.1 — the same image
  hygiene: pinned npm, `--ignore-scripts`, `apt-get upgrade`, confined
  prompt file, sanitised logs.
- `signal-alertmanager` 0.7.4, `housekeeping` 0.2.4, `context-sync` 0.2.4 —
  rebuilt on the current base image, no behaviour change.
- `signal-ha` 0.4.0 and `signal-k8s-events` 0.4.5 — below.

### Added

- **`signal-k8s-events` gained a fourth suppression axis: node STATE.** While
  a node is cordoned or maintenance-tainted, every event on its objects (and
  on the Node itself) is suppressed for exactly as long as the drain lasts —
  a rolling reboot manager (kured is the canonical case) now opens ZERO
  conversations, whatever it does to the pods scheduled there. Configured
  under a source's `spec.config.route`: `drainingNodes` (`suppress`, the
  default, or `report` to opt a source out), `drainingNodeMatchers` to
  narrow it, `drainingNodeBound` (default `1h`) — past which a forgotten
  cordon is reported once (`kind: Node`, reason `NodeDrainExceeded`) and
  suppression on that node is released. Needs the events `ClusterRole`'s new
  `nodes` `list`/`watch` grant (cluster-wide installs only — see below).
- **`signal-ha` 0.4.0: the Home Assistant lane watches the four health
  surfaces beside the log.** Home Assistant reports most of what is wrong
  with a house somewhere other than its log. An integration that cannot
  reach its device sits in a config entry state and logs one line at
  startup. A problem it has diagnosed is a repair. A device fault is a
  `problem` or `connectivity` binary sensor. A package behind is an
  `update.*` entity. Each is now a record kind on the same source, through
  the same rules, dwell and self-exclusion, labelled `surface=`
  (`log`, `config-entry`, `repair`, `sensor`, `update`) so a rule can select
  it. One signal per failed entry (with its reason), per repair, per faulting
  sensor. ONE digest listing every pending update, re-posted when the set
  grows. Each surface is switchable and tunable under
  `logsAdapter.source.surfaces` — config entries, repairs and sensors on by
  default, the update digest off — and a surface that is off is never read.
  The shipped rules open with one per surface, ahead of the log rules. A
  values file that replaces `rules` replaces those too.

### Changed


- **The `kubernetes` bundle's events `ClusterRole` gains `nodes`
  `list`/`watch`** (cluster-wide RBAC only; a namespaced install has no
  equivalent, since nodes are cluster-scoped — drain awareness is simply off
  there, and every source's Ready condition notes it once).
- **The shipped tier-3 node-condition rule now matches `kind="Node"`.**
  `NodeHasDiskPressure` / `NodeHasMemoryPressure` are unaffected — the
  kubelet already reports those once, against the Node object. `NodeNotReady`
  is NOT: the node lifecycle controller stamps it on every pod scheduled on
  the affected node, so before this change one node blip fired once per
  workload at `for: 0`, with no chance to notice the node recovering. A
  pod-level `NodeNotReady` now falls to the catch-all rule instead, which
  already dwells and re-checks the pod's own health before emitting — so a
  reboot that recovers within that dwell (the ordinary case) is no longer
  reported at all, and one that does not is reported once it recurs.
  **If your `spec.config.rules` overrides the shipped tier 3** (rather than
  inheriting the chart default), add the `kind="Node"` matcher yourself to
  get the same benefit — an override is untouched by this change otherwise,
  and keeps firing on every pod-level `NodeNotReady` exactly as before.
- **The SonarCloud quality gate is a required check.** The scan step in CI
  waits on the gate and fails the component's job on `ERROR`, reported
  through `ci-green`. Contributor-facing only; nothing in the chart or the
  operator changes. See `CONTRIBUTING.md`, *Code analysis* and *Pull
  requests* (the `autofix` label).

### Fixed

- **`signal-ha` 0.3.0: the Home Assistant log lane observes records on a
  default Home Assistant install.** The adapter learned of live records only
  from `system_log_event`, which Home Assistant fires solely under
  `system_log: fire_event: true` — off by default, and named nowhere in this
  project. On such an install the adapter saw the log once per reconnect,
  judged every record "quiet" because nothing ever recurred from its point
  of view, and posted nothing while its source reported `Ready=True`. It now
  POLLS `system_log/list` every fifteen seconds and feeds what is newer than
  its cursor through the same rule path. The event stays as a lower-latency
  path where the instance fires it, and an occurrence is considered once
  whichever path brings it. `backfill: false` keeps its meaning by moving
  the cursor past the listing on connect. No configuration changes. The
  bundle's pinned tag moves to 0.3.0.

## [13.3.0] — 2026-08-28

**A third runtime: `agentops-runtime-copilot` 0.1.0, shipped as the `copilot`
bundle.** GitHub Copilot driven through its SDK, in process. The first vendor
to own its own tool vocabulary, its own agent-definition format and its own
session store — and every one of those is translated inside the runtime, so a
Pipeline binds the same toolsets it binds for claude. No manager, CRD or
toolset change was needed.

### Added

- `chart/charts/copilot/` — OFF by default. `copilot.enabled: true` with a
  GitHub token (`copilot.credentialsSecret.token`, or a Secret you manage)
  renders one `AgentRuntime` named `copilot` through the parent's shared
  renderer, inheriting `global.agentops.runtimeDefaults`. A route selects it
  with `pipelines[].runtimeRef: copilot`. `copilot.model` and
  `copilot.maxAiCredits` are optional.
- The runtime translates the composed allowlist into Copilot's two layers:
  `availableTools` (`Read` → `view`, `Bash` → `bash`, `mcp__s__t` →
  `mcp:s-t`) and a permission callback that decides every call. An unmapped
  pattern is withheld and logged. `mcp__<server>__*` is refused rather than
  widened to every server. `Bash(kubectl:*)` is enforced per invocation. An
  empty allowlist stays empty — Copilot's "no `tools:` means everything" never
  applies — and a denial is fed back to the model, never a hang.
- It reads `.github/agents/<agent>.agent.md`, Copilot's location, with the same
  frontmatter shapes and `merge` / `overwrite` composition as the reference
  runtime. Bound MCP servers reach Copilot from the same `mcp.json`, with
  secret placeholders resolved in the pod.
- The context id is minted by the runtime, so `runtimeContextId` is a handle
  it chose. State lives under `$HOME/.copilot/session-state/<id>/`, declared to
  `context-sync` by the bundle. A resume whose state is gone re-checks, then
  FAILS with `continuity: unavailable` and a readable reason.
- `docs/runtimes/copilot.md`, and the Copilot chip on the landing page.
- CI runs `node --test` for the Node runtimes (`runtimes/claude`,
  `runtimes/copilot`) when their directories change. Neither suite ran in CI
  before.

**Nothing changes for an install that does not enable the bundle.** The
rendered manifest is identical.

## [13.2.0] — 2026-08-27

**A burst that healed before its dwell closed no longer opens a
conversation.** Both dwell adapters re-check a matched event at the end of its
`for` window. For a kind with no health predicate — every kind but Pod in the
cluster-events lane, every integration without a config-entry state in the
Home Assistant lane — the re-check asked *did it recur*, and a controller that
retried every few seconds for half a minute and then healed answered yes. On
the reference install that was most of the backlog: twelve conversations
about Longhorn snapshot purges that had healed two minutes before the
catch-all fired, each concluding "self-resolved, no action needed".

### Changed


- `signal-k8s-events` 0.4.4, `signal-ha` 0.2.4: the second verification rung
  asks whether the event was **still recurring as the window closed** — its
  last third, floored at thirty seconds, derived from the window actually
  waited so escalation keeps the proportion. A burst that went quiet before
  that is dropped as churn; one still arriving is reported once, and its
  evidence names how long before the close the last event arrived.
- The Home Assistant re-check applies the same rule to the log's own count: a
  count that rose early in the window and stopped rising is a blip that
  healed, not an integration still failing.

**Trade-off, named.** A controller backing off to a retry period longer than
the closing window — ninety seconds under a three-minute rule — is dropped at
that deadline, and its next retry opens a fresh window. The report is delayed
by one window, not lost. Anyone preferring the previous behaviour restates the
rule with a shorter `for`; there is no switch.

No configuration, CRD or RBAC change. `NodeNotReady` and the other `for: "0"`
reasons are unaffected — they are never re-checked; a scheduled reboot is the
time axis's job (`route.muteTimeIntervals`, see the Kubernetes page).

## [13.1.1] — 2026-08-27

**Every image rebuilt on the current toolchain.** The weekly scan of the
published images reported fixable findings in every Go binary: 22 Go
standard-library CVEs, CVE-2025-68121 among them at CRITICAL.

The manager carried eight more from `golang.org/x/net`, `x/oauth2` and
`x/text`, and `runtime-claude` five inside npm's bundled dependencies. The tree
had already moved past all of them (Go 1.25, the manager's `x/*` bumped). The
published images had not.

### Changed


Twelve components moved to versions built from that tree:
`channel-telegram` 0.24.4, `console` 0.38.2, `context-sync` 0.2.3,
`egress-proxy` 0.2.4, `gateway-telegram` 0.5.3, `housekeeping` 0.2.3,
`manager` 0.57.2, `runtime-claude` 0.8.3, `signal-alertmanager` 0.7.3,
`signal-ha` 0.2.3, `signal-k8s-events` 0.4.3, `signal-telegram` 0.6.3.

**No behaviour changes.** `runtime-ollama` 0.1.0 was already built on Go 1.25
and scanned clean, so it is unchanged.

## [13.1.0] — 2026-08-26

**A second runtime: `agentops-runtime-ollama` 0.1.0, shipped as the `ollama`
bundle.** A local-model runtime over an Ollama endpoint you already run, in
which the RUNTIME is the harness — the agent loop, tool dispatch, the
transcript and the context handle are its own, and Ollama is called only for
the next message. Building it needed no manager, CRD or work-contract change,
which is the finding: the contract is vendor-neutral.

### Added

- `chart/charts/ollama/` — OFF by default. `ollama.enabled: true` with
  `ollama.endpoint` and `ollama.model` renders one `AgentRuntime` named
  `ollama` through the parent's shared renderer, inheriting
  `global.agentops.runtimeDefaults`. A route selects it with
  `pipelines[].runtimeRef: ollama`. The bundle deploys no model server, and
  the render FAILS naming the key when the endpoint is missing. The model is
  optional while the server has exactly one pulled.
- The runtime implements the six built-in tools natively — `Read`, `Grep`,
  `Glob`, `Edit`, `Write`, `Bash` — so `agentops-observe` / `-shell` / `-edit`
  mean the same thing on both runtimes, connects the bound MCP servers from the
  same `mcp.json`, and keeps context as one transcript per conversation under
  `$HOME/.agentops/contexts/`, declared to `context-sync` by the bundle.
- `docs/runtimes/ollama.md`, and the Ollama chip on the landing page.

- **Every runtime has its own name, and `default` is a copy.** The claude
  bundle's CR is now named `claude`, the ollama one `ollama`, and the parent
  renders one more `AgentRuntime` named `default` — a copy of the runtime
  flagged `default: true` (`claude.default`, `ollama.default`,
  `runtimes[].default`), or of the first configured when none is. So a fresh
  install is unchanged (`default` is claude's copy), turning claude off and
  ollama on needs no rename, and `runtimeRef: claude` now works. Two flags fail
  the render. A `Pipeline` naming `runtimeRef: default` keeps resolving.

**Nothing changes for an install that does not enable the bundle.** The
rendered manifest is identical.

## [13.0.1] — 2026-08-24

**Every component image is now published BY CI, from a tag.**

### Changed


Ten components moved to versions the release workflow built and pushed:
`channel-telegram` 0.24.3, `console` 0.38.1, `context-sync` 0.2.2,
`egress-proxy` 0.2.3, `gateway-telegram` 0.5.2, `housekeeping` 0.2.2,
`signal-alertmanager` 0.7.2, `signal-ha` 0.2.2, `signal-k8s-events` 0.4.2,
`signal-telegram` 0.6.2.

**No behaviour changes.** The images are built from the same source as the
versions they replace; what is new is HOW they got there. The previous ones were
pushed by hand before this repository was public, so nothing in the tree
recorded which commit produced them and the registry's immutability rule meant
a tag could never be added after the fact — only a new version could carry one.

Component tags exist for that provenance and deliberately create no GitHub
Release: the chart version is the one an adopter types.

## [13.0.0] — 2026-08-24

**The one-flag demo works on a laptop cluster, with its memory intact.**

### Changed


**`persistence.context.accessModes` and `.workspace.accessModes` ship EMPTY, and
the chart answers them.** `ReadWriteMany` for an ordinary install, exactly as
before; `ReadWriteOnce` under `global.demo.enabled`.

The demo failed on every cluster a reader is likely to try it on. `local-path`
is the only storage class rancher-desktop, k3d, kind and minikube ship, and it
REFUSES an RWX claim:

```
failed to provision volume with StorageClass "local-path":
NodePath only supports ReadWriteOnce and ReadWriteOncePod (1.22+) access modes
```

The claim sat `Pending`, no runtime pod was ever created, and the conversation
waited forever. Getting started documented the workaround —
`persistence.context.enabled=false` — which buys a working demo by taking away
the thing the demo is showing.

- **It is not about node COUNT.** Any cluster whose only provisioner is RWO-only
  hits it; a single-node cluster with an RWX provisioner never does.
- **RWO is CORRECT under demo, not merely tolerable.** Many pods share an RWO
  volume on one node and the volume's affinity is what puts them there, so the
  default cap of five concurrent conversations still holds.
- **An explicit value wins in both modes.** The default is empty rather than a
  mode because an empty list is the one thing a chart can tell apart from a
  choice somebody typed.

### Upgrading

**A DEMO install whose context claim already BOUND cannot be upgraded in place.**
A PVC's `accessModes` is immutable, so Helm's patch is refused by the API server
and the upgrade fails naming the field.

Two ways out, and the first is the one a demo deserves:

```sh
# the demo is disposable — reinstall it
helm uninstall agent-ops -n agent-ops && kubectl delete ns agent-ops

# or keep the claim exactly as it is
helm upgrade ... --set 'persistence.context.accessModes={ReadWriteMany}'
```

**No ordinary install is affected.** Outside demo mode the rendered claim is
`ReadWriteMany` before and after, so there is no patch to refuse.

## Older versions

| Archive | Covers |
|---|---|
| [CHANGELOG-11.0-12.0.md](changelog/CHANGELOG-11.0-12.0.md) | chart 11.0.0 through 12.0.0 |
| [CHANGELOG-channel-telegram-0.24.2.md](changelog/CHANGELOG-channel-telegram-0.24.2.md) | channel-telegram 0.24.2 |
| [CHANGELOG-9.0-10.0.md](changelog/CHANGELOG-9.0-10.0.md) | chart 9.0.0 through 10.0.0 |
| [CHANGELOG-8.0.md](changelog/CHANGELOG-8.0.md) | chart 8.0.0 |
| [CHANGELOG-5.22-7.0.md](changelog/CHANGELOG-5.22-7.0.md) | chart 5.22.0 through 7.0.0 |
| [CHANGELOG-5.0-5.21.md](changelog/CHANGELOG-5.0-5.21.md) | chart 5.0.0 through 5.21.0 |
| [CHANGELOG-1.0-4.0.md](changelog/CHANGELOG-1.0-4.0.md) | chart 1.0 through 4.0.0 |
