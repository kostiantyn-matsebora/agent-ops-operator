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
