All notable changes to this project are documented here, in
[Keep a Changelog](https://keepachangelog.com/en/1.1.0/) format. Versions are
CHART versions. Image tags move independently and are named in each entry.

This file holds the **ten most recent versions**. Older entries are in
`changelog/`, linked at the [foot of this page](#older-versions).

See [the repository](https://github.com/kostiantyn-matsebora/agent-ops-operator)
for the source and the reference material beside this file.

## [Unreleased]

### Added

- A normalized signal may carry an optional `severity` field — one of
  `critical`, `error`, `warning`, `info`. `POST /signal/inbound` validates
  it and refuses an unknown value with a `400` naming the four accepted
  values. Absent severity behaves exactly as before this field existed —
  not breaking for any adapter or manifest that sends none. No shipped
  adapter populates it yet.
- A new `/control-plane/*` HTTP surface on the manager, guarded by its own
  `controlPlaneAuth` token, for operational configuration going forward. It
  ships with one disposable placeholder CRD, `ControlPlaneProbe`, proving the
  surface end to end — see [contracts](contracts.md#the-control-plane-contract)
  and [concepts](concepts.md#controlplaneprobe).
- `SignalAdapter`/`ChannelAdapter` workloads carry `app.kubernetes.io/instance`
  (the CR name), on top of the unchanged `app.kubernetes.io/name`. Every
  instance of a given adapter kind used to render the same
  `app.kubernetes.io/name` with no instance label, so label-grouping tooling
  — a Cilium Hubble service map, Grafana, `kubectl get pods -l` — could not
  tell `cron` apart from `alertmanager`, or the console's own adapter from
  `telegram`.
- The [signal adapter specification](contracts/signal-adapter.md) moves to
  2.2.0. It names the post-emit refractory window as the one carve-out from
  its no-dedup rule (`ADP-8`), and lists `signals/pulse` beside the other
  five shipped adapters. Additive — no adapter needs to change to stay
  conformant.

### Changed

**BREAKING: the Pulse signal adapter's default `initiationId` now folds in
the alert's start time, not just its Pulse-assigned id.** A `SignalSource`
served by `signal-pulse` gains `spec.config.correlationStrategy`, `"id"` or
`"id+startTime"`.

Pulse can reuse an alert id for what it calls a new occurrence. The new
default, `"id+startTime"`, opens a fresh conversation for that re-fire
instead of continuing an old one.

### Fixed

- Every agent runtime (`claude`, `ollama`, `copilot`) now reports continuity
  truthfully — `continued` only when a context was actually resumed, never
  on a failed resume. A false `continued` claim used to record a recovery
  on the storage breaker that never happened.
- The `copilot` runtime now extracts `<brief>` from the agent's output, so
  `status.brief` updates instead of the block rendering in chat as a named
  section.
- The `ollama` runtime now sends MCP auth headers on HTTP MCP servers —
  dropped before, so an authenticated server (including `aops`) failed.
- The Telegram channel adapter's in-process retry budget is now per
  operation, not per transport call, so a chunked or multi-call send can no
  longer exceed the manager's reclaim window and double-post.
- A console send carrying no thread id is now visible on the channel's
  general feed, instead of completing as success with nothing to show for
  it.
- **`signal-pulse`: a rotated signing secret now heals the registered
  webhook on its own.** Before, every delivery after a rotation was refused
  `401` until the webhook was re-registered by hand.
- `signal-pulse`: the `404` for an unmatched source now names the source.
- `signal-alertmanager` and `signal-k8s-events`: a degraded registration now
  points at `spec.serviceAccountName`, not a field the CRD no longer has.
- `signal-alertmanager`: a malformed `register:` block now degrades to a
  manual-registration message naming the parse problem. It used to fail
  silently.
- `signal-k8s-events`: a relist pass that hits its batch cap now reports the
  clip on the source's Ready condition. It used to skip the remaining events
  without a trace.
- `signal-k8s-events`: a suppression rule shadowed by an earlier one now
  surfaces as a warning on the source's Ready condition.
- `signal-k8s-events`: a mute window now reports its close even when the
  lane has gone quiet, with a count of zero.
- **`signal-k8s-events`: an unreadable object cache now fails OPEN.** A pod
  whose health cannot be read at the close — the cache has not synced, or its
  pod reads are denied — is emitted, marked unverified in its payload. It
  used to be dropped unless it was still recurring.
- **`signal-ha`: an unreadable system log now fails OPEN.** A record still
  failing when the log becomes unreadable is emitted, where it used to be
  dropped as churn. Expect MORE signals during a Home Assistant outage, not
  fewer — that is the specification's fail-open rule working.

### Security

- **`signal-pulse`: a configured `signingSecret` is now enforced on every
  delivery, whether or not the adapter registered the webhook itself.** A
  source whose `baseUrl`/`webhookUrl` was absent, incomplete or invalid used
  to accept unsigned posts. A hand-registered Pulse presenting the same
  secret now verifies exactly as an adapter-registered one does.

### Upgrade

To keep the previous, id-alone behavior, set `correlationStrategy: id` on
each `SignalSource` served by `signal-pulse`. The chart value is
`signal.defaultSource.correlationStrategy` in the `pulse` bundle.

Helm never upgrades an existing CRD. Run
`kubectl apply -f chart/crds/` before `helm upgrade` to pick up the new
`ControlPlaneProbe` CRD, or the `/control-plane/*` surface's handlers fail
against a kind the cluster does not have.

## [17.0.0] — 2026-10-18

- `manager` 0.60.0, `console` 0.39.1.
- `signal-alertmanager` 0.8.0, `signal-pulse` 0.1.1, `signal-ha` 0.4.2,
  `signal-k8s-events` 0.5.0, `signal-cron` 0.2.1, `signal-telegram` 0.6.6,
  `channel-telegram` 0.26.1.
- Bundles `prometheus` 0.2.1, `pulse` 0.1.1, `home-assistant` 0.2.1,
  `kubernetes` 0.4.0, `telegram` 0.5.1 — each ships the signal adapter it
  embeds, above.

### Changed

**BREAKING: a signal names its own correlation id now, instead of the
manager computing one from labels.** Every `POST /signal/inbound` signal
carries `initiationId`, replacing `fingerprint`. The manager routes a
recurrence by comparing it against an open conversation's `conversationId`
— pure equality, never a derived grouping key.

- `SignalSource.spec.grouping` (`signatureLabels`, `windowDays`,
  `cooldownHours`) is GONE. The one surviving knob is a top-level
  `spec.cooldownHours` (default 6).
- `Conversation.spec.signature` and the `agentops.dev/signature-hash` label
  are GONE, replaced by `spec.conversationId` — the adopted `initiationId`,
  opaque, written once at creation.
- **Every shipped adapter mints or propagates its own id**: Alertmanager
  uses the webhook's own `groupKey`, Pulse its alert id, Home Assistant
  `<source>@<integration>`, Kubernetes Events `<source>@<namespace>/<workload>`
  (dropping the event's reason and the pod name from the key), cron the
  source's own job identity, `signal-telegram` a fresh mint per
  general-surface message (deterministic from the update id, so a replay
  converges), and the console a fresh mint per new task with verbatim
  propagation on continuation. A thread reply's continuation travels
  `channel-telegram`'s own path (`POST /channel/inbound`), correlated by the
  binding's `conversationId` rather than a propagated `initiationId`.
- `/channel/ops` and every outbound op now carry `conversationId` alongside
  the thread it binds (additive, `docs/contracts/channel-adapter.md` 1.1.0).
- A reopen is refused while another OPEN conversation already carries the
  same `conversationId` for that pipeline, naming the successor.

See [the signal adapter specification](contracts/signal-adapter.md) (now
2.0.0) for the full shape.

### Upgrade

**Apply the CRDs before you upgrade** — `SignalSource.spec.grouping` and
`Conversation.spec.signature` are removed fields, and Helm never upgrades an
existing CRD:

```sh
kubectl apply -f chart/crds/
```

**A 1.x signal adapter posting `fingerprint` instead of `initiationId` gets
a `4xx`, not a silent drop.** There is no synthesis and no fallback — update
every custom adapter to mint or propagate `initiationId` before you upgrade
a production install. A shipped bundle adapter already does this at 17.0.0.

A `SignalSource.spec.grouping` value left in your own manifests is simply
unread after the CRD upgrade — move any non-default `cooldownHours` you set
under `grouping` to the new top-level field.

## [16.0.0] — 2026-10-18

- `signal-pulse` 0.1.0, `mcp-pulse` 0.1.0, bundle `pulse` 0.1.0.
- `manager` 0.59.1, `console` 0.39.0, `runtime-claude` 0.11.0.
- Bundles `kubernetes` 0.3.2, `prometheus` 0.2.0 and `home-assistant` 0.2.0
  take the breaking `wiringEnabled` rename (see Changed).

### Added

- **The `pulse` bundle.** A webhook-receiving `SignalAdapter` for Pulse
  (rcourtman/pulse — Proxmox VE/PBS/PMG, Docker/Podman, Kubernetes, TrueNAS),
  a first-party `mcp-pulse` MCP server, and two AgentProfiles split by
  privilege: `pulse-observer` only investigates, `pulse-operator` can
  additionally record a Community-tier operator-state annotation
  (maintenance window, criticality tag). See
  [the integration page](integrations/pulse.md).
  - The real wall between the two profiles is which pre-scoped Pulse API
    token each route's `MCPConfig` carries, not the toolset — `mcp-pulse`
    forwards whichever token the caller presented and decides no reach of
    its own.
  - The adapter self-registers its webhook against Pulse's own
    `POST /api/notifications/webhooks`, reading the base URL and token from
    the served `SignalSource`'s own `spec.config`/`spec.credentialsSecretRef`
    — the `SignalAdapter` CRD carries no generic pod-env passthrough, so
    that is the only delivery path that reaches the adapter's pod.
- **The claude runtime reports cache writes.** `cacheCreationTokens` carries
  through the work contract (`report.js` → `workreport.go` → the console),
  and the Runs table splits usage into In / Cached / Written / Fresh / Out
  instead of folding a cache write into In unexplained. A vendor reporting
  no cache facts renders dashes, never zeros.

### Changed

**BREAKING: every bundle's wiring switch is now `<bundle>.wiringEnabled`,**
not `<bundle>.pipelines.enabled`. `kubernetes`, `prometheus` and
`home-assistant` all move. `pulse` is new this release and ships the new name.

The old spelling named one of the two shapes a bundle's route can take — a
`Pipeline`, under `global.agentops.wiringMode: pipelines` — even though the
same flag also gates the `AgentCapability` objects `wiringMode: coordinator`
renders instead. `wiringEnabled` says only WHETHER the bundle's own route(s)
render. `wiringMode` still says AS WHAT.

Per-route sub-flags are unaffected: `pulse.pipelines.observer.enabled` and
`pulse.pipelines.operator.enabled` keep their names and their place under
`pipelines:`.

### Fixed

- **The chart-rendered Coordinator escalates to an acting agent instead of
  claiming it has no tools.** Its prompt told it to delegate but never to
  retry through an acting agent once an observing one ran out of evidence
  for lack of a tool. The `kubernetes` bundle's profile had the matching
  gap: turning `allowPodExecution` on removed the "no shell fallback"
  paragraph instead of replacing it, leaving the agent no reason to connect
  a tool-less workload to the `pods_exec` tool it had just been granted.
- **The Coordinator anchors on its roster, not its own toolbox.** A
  conversation that had been delegating correctly could drift into
  inventorying its own generic tools and answering "this can't be answered
  live" from them. The prompt now mandates `list_agents` as the first tool
  call of every conversation, re-scopes the generic tools as housekeeping
  rather than denying them, and reserves infeasibility verdicts to an
  invoked agent's own report.
- **The chart pinned `runtime-claude` and `runtime-copilot` releases that
  predate the work-report contract**, three weeks after it landed — every
  install's Runs view showed no model/tool-call statistics, with every code
  test green since each half of the contract passed alone.
  `pinned-runtime-guard.py` now fails `ci-green` on a pin whose tagged tree
  doesn't carry the runtime's report module.
- **A coordinator's internal round-trips with an invoked member no longer
  inflate the console's unread counts.** A run triggered solely by a member
  reaching back up with its report — already shown in full on the invoke
  card — was counted as a new unread message like any other reply.
- **`close-topic` no longer retries forever for a thread with no topic
  yet.** `reconcileClosed` enqueued it unconditionally, even when
  `ensure-topic` had never succeeded (auth failure, bot removed from a
  group, rate limit), logging an error every few seconds for as long as
  the conversation existed.

### Upgrade

Action needed if you set any `<bundle>.pipelines.enabled` value. **The
render FAILS until you move it**, deliberately — Helm never reports an
unread values key, so an unguarded rename would present as a successful
upgrade that silently turned wiring off.

Rename `<bundle>.pipelines.enabled` to `<bundle>.wiringEnabled` for each
bundle you set it on:

```yaml
kubernetes:
  wiringEnabled: true   # was: pipelines.enabled
```

## [15.0.0] — 2026-10-12

- `manager` 0.58.0, `channel-telegram` 0.26.0, `gateway-telegram` 0.7.0.

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

- **Breaking.** `gateway-telegram` is now the SOLE holder of the Telegram bot
  credential, inbound and outbound alike. The telegram bundle's
  `surface.credentials` Secret no longer carries an `apiBase` key, and the
  channel adapter reads a new `gatewayUrl`/`gatewayToken` Secret instead.
  Hand-built Secrets must change.
- **`channel-telegram` no longer calls `api.telegram.org` directly.** Every
  send, document upload, forum topic operation and command registration is
  relayed through `gateway-telegram`'s new `POST /telegram/call/{method}`,
  authenticated with a shared token the chart generates. A new chart-managed
  Secret (`agentops-telegram-gateway`) carries the binding through the same
  `credentialsSecretRef` projection.
- **`gateway-telegram` gains a Service and a NetworkPolicy** for its new call
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
  handler, exempting only `/healthz`. `/channel/ops` and `/activity*` are
  proxied like every other route — they just keep their own unchanged local
  fallback (refuse, or serve/record locally) when the leader can't be
  reached.
- **The manager's own `NetworkPolicy` now admits its own kind.** The proxy
  needs manager-to-manager traffic on port 8080, a pattern nothing
  previously needed — without this rule the whole fix was a silent no-op
  on any install enforcing `networkPolicy.enabled: true`, confirmed live
  (every poll from the console fell back to its pre-fix local behavior for
  15+ minutes across a stable leader).

### Upgrade

1. Rebuild any hand-made channel Secret for the new shape. It carries
   `gatewayUrl`, `gateway-telegram`'s call endpoint, and `gatewayToken`,
   which must equal `gateway-telegram`'s `TELEGRAM_CALL_TOKEN`. It no longer
   carries `botToken` or `apiBase` — a chart-rendered Secret picks up the new
   shape automatically.

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
  `list`/`watch`** (cluster-wide RBAC only. A namespaced install has no
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
  through `ci-green`. Contributor-facing only. Nothing in the chart or the
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
bundle.** GitHub Copilot driven through its SDK, in process.

The first vendor to own its own tool vocabulary, its own agent-definition
format and its own session store. Every one of those is translated inside
the runtime.

A Pipeline binds the same toolsets it binds for claude. No manager, CRD or
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
conversation.** Both dwell adapters re-check a matched event at the end of
its `for` window.

For a kind with no health predicate — every kind but Pod in the
cluster-events lane, every integration without a config-entry state in the
Home Assistant lane — the re-check asked *did it recur*.

A controller that retried every few seconds for half a minute and then
healed answered yes.

On the reference install that was most of the backlog: twelve conversations
about Longhorn snapshot purges that had healed two minutes before the
catch-all fired, each concluding "self-resolved, no action needed".

### Changed


- `signal-k8s-events` 0.4.4, `signal-ha` 0.2.4: the second verification rung
  asks whether the event was **still recurring as the window closed** — its
  last third, floored at thirty seconds, derived from the window actually
  waited so escalation keeps the proportion. A burst that went quiet before
  that is dropped as churn. One still arriving is reported once, and its
  evidence names how long before the close the last event arrived.
- The Home Assistant re-check applies the same rule to the log's own count: a
  count that rose early in the window and stopped rising is a blip that
  healed, not an integration still failing.

**Trade-off, named.** A controller backing off to a retry period longer than
the closing window — ninety seconds under a three-minute rule — is dropped
at that deadline, and its next retry opens a fresh window.

The report is delayed by one window, not lost. Anyone preferring the
previous behaviour restates the rule with a shorter `for`. There is no
switch.

No configuration, CRD or RBAC change. `NodeNotReady` and the other
`for: "0"` reasons are unaffected. They are never re-checked — a scheduled
reboot is the time axis's job (`route.muteTimeIntervals`, see the
Kubernetes page).

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
bundle.** A local-model runtime over an Ollama endpoint you already run.

The RUNTIME is the harness here — the agent loop, tool dispatch, the
transcript and the context handle are its own. Ollama is called only for
the next message.

Building it needed no manager, CRD or work-contract change, which is the
finding: the contract is vendor-neutral.

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

## Older versions

| Archive | Covers |
|---|---|
| [CHANGELOG-13.0.1.md](changelog/CHANGELOG-13.0.1.md) | chart 13.0.1 |
| [CHANGELOG-13.0.md](changelog/CHANGELOG-13.0.md) | chart 13.0.0 |
| [CHANGELOG-11.0-12.0.md](changelog/CHANGELOG-11.0-12.0.md) | chart 11.0.0 through 12.0.0 |
| [CHANGELOG-channel-telegram-0.24.2.md](changelog/CHANGELOG-channel-telegram-0.24.2.md) | channel-telegram 0.24.2 |
| [CHANGELOG-9.0-10.0.md](changelog/CHANGELOG-9.0-10.0.md) | chart 9.0.0 through 10.0.0 |
| [CHANGELOG-8.0.md](changelog/CHANGELOG-8.0.md) | chart 8.0.0 |
| [CHANGELOG-5.22-7.0.md](changelog/CHANGELOG-5.22-7.0.md) | chart 5.22.0 through 7.0.0 |
| [CHANGELOG-5.0-5.21.md](changelog/CHANGELOG-5.0-5.21.md) | chart 5.0.0 through 5.21.0 |
| [CHANGELOG-1.0-4.0.md](changelog/CHANGELOG-1.0-4.0.md) | chart 1.0 through 4.0.0 |
