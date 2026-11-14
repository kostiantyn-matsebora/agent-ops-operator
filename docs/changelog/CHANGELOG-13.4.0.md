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
