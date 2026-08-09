# Tasks: rich-console-ui

Phases 1 and 2 are independently useful and land first — they are manager/API
work with no console dependency. Phase 9 deletes the old console only once the
replacement is proven.

## 1. Activity telemetry (manager)

- [ ] 1.1 Implement `internal/activity`: fixed-size ring buffer with monotonic cursors, oldest-first eviction, non-blocking emit (a full buffer drops, never waits), and a subscriber fan-out for streaming
- [ ] 1.2 Define the event type — `cursor`, `ts`, `kind`, `from`/`to` (`{kind,name}`), `status`, `conversation`, `pipeline`, `runId`, `opId`, `inputId`, `latencyMs`, `detail` — with node references using the SAME kind/name vocabulary the topology graph uses
- [ ] 1.3 Emit from ingest: `signal.received`, `signal.claimed`, `signal.dropped` (carrying the `Wired=False` reason), `conversation.created`, `input.queued`
- [ ] 1.4 Emit from dispatch: `run.dispatched` (`GET /work`), `run.completed` (`POST /work/done`, with exit code and derived latency)
- [ ] 1.5 Emit from the channel op pipeline: `channel.op.enqueued`, `channel.op.completed`, `channel.inbound`
- [ ] 1.6 Serve `GET /activity?since=&limit=` and `GET /activity/stream` (SSE, keep-alives, `X-Accel-Buffering: no`), both under the adapter bearer scheme; evicted-cursor requests answer with an explicit resync rather than a silent gap
- [ ] 1.7 Serve `POST /activity` for adapter-reported hops, authenticated per-adapter, rejecting events attributed to another adapter
- [ ] 1.8 Unit tests: eviction order, emit never blocks, cursor monotonicity, resync on evicted cursor, adapter attribution rejection
- [ ] 1.9 envtest: drive one conversation end to end and assert the full event sequence, `from`/`to` pairs, shared `runId`, and latencies consistent with timestamps
- [ ] 1.10 Invariant test: no emission path reaches `/signal/inbound`; a synthetic storm of events (including errors about agent-ops' own components) creates ZERO Conversations and writes ZERO Kubernetes objects
- [ ] 1.11 `docs/contracts.md`: document the activity endpoints and event schema (the stale `POST /task` row is `task-is-a-signal`'s to delete — do not touch it here)

## 1b. Manager introspection

- [ ] 1b.1 Implement `GET /status` (adapter-token authenticated): build version, leader lease holder, runtime slots in use against `MAX_RUNTIMES`, per-adapter op queue depth with oldest queued and oldest claimed-but-uncompleted op ids and ages, and active cooldowns — no CR spec or status in the payload
- [ ] 1b.2 Register the metric set into the controller-runtime registry serving the existing `:9090` — counters (`agentops_signals_received_total`, `agentops_signals_dropped_total`, `agentops_conversations_created_total`, `agentops_runs_total`, `agentops_channel_ops_total`), gauges (`agentops_channel_ops_queued`, `agentops_channel_ops_claimed`, `agentops_channel_op_oldest_{queued,claimed}_age_seconds`, `agentops_runtime_slots_{in_use,max}`, `agentops_conversations_inflight`, `agentops_cooldowns_active`) and histograms (`agentops_run_duration_seconds`, `agentops_channel_op_latency_seconds`); standard conventions — `agentops_` prefix, `_total` on counters, base units, HELP/TYPE, OpenMetrics-parseable; no new listener
- [ ] 1b.2a Emit metrics from the SAME call sites as activity events (one instrumentation pass feeding ring buffer and registry), and assert in tests that an event and its metric observation cannot occur independently
- [ ] 1b.2b Cardinality guard: a test asserting no metric declares a label that can carry a conversation, run or op id, and that series count is unchanged after driving thousands of conversations
- [ ] 1b.3 Implement `GET /pipelines/{name}/resolved`: composed tool allowlist after composition-mode application, effective toolsets, effective MCP configs and servers, and the resolving runtime; 404 for an unknown pipeline; an empty allowlist reported as empty
- [ ] 1b.4 Tests: `/status` reflects a queued-but-unclaimed op, a claimed-but-uncompleted op, and slot exhaustion; `/pipelines/{name}/resolved` equals what dispatch composes for the same pipeline (assert against the dispatch path, not a reimplementation); both endpoints 401 without a token
- [ ] 1b.5 `docs/contracts.md`: document both endpoints and the boundary rule — the manager exposes only what only the manager knows; CR state is never proxied

## 2. Externally-served SignalAdapter (API + controllers)

- [ ] 2.1 `api/v1alpha1/signaladapter_types.go`: make `Image` optional, add `ServedBy *AdapterRef` (`{kind, name}`), CEL or webhook validation for exactly-one-of; regen deepcopy + CRDs into `chart/files/crds/`
- [ ] 2.2 `signaladapter_controller.go`: when `servedBy` is set, create no Deployment/Service/SA and report `Ready=True/ServedBy`; report `Ready=False` naming the target when it does not exist
- [ ] 2.3 `channeladapter_controller.go`: inject `SIGNAL_ADAPTER_TOKEN` (`chat.DeriveSignalAdapterToken`) into the pod of any ChannelAdapter named by a `servedBy` SignalAdapter; remove it when the link is cleared; watch SignalAdapters to trigger reconcile
- [ ] 2.4 Confirm `SignalSource` serving resolution treats an externally-served adapter identically (`Served=True`)
- [ ] 2.5 envtest: no workload created; `Served` still resolves; both tokens present on one pod and unequal; each token rejected by the other surface; mode reversal recreates the workload; dangling `servedBy` is diagnosable
- [ ] 2.6 `docs/concepts.md`: document `servedBy` and when an adapter should be both a surface and an originator

## 3. Console backend — Go module scaffold + fan-in

The backend is Go throughout (phases 3 and 4); the frontend is React/PatternFly
(phases 5 and 6). One Go binary ships both.

- [ ] 3.1 New `console/` Go module replacing the old one: env config (`MANAGER_URL`, `ADAPTER_NAME`, `ADAPTER_TOKEN`, `SIGNAL_ADAPTER_TOKEN`, `SIGNAL_SOURCE_NAME`, `LISTEN_ADDR`, `POD_NAMESPACE`, `WRITE_ENABLED`), HTTP server, graceful shutdown
- [ ] 3.2 Kubernetes list/watch cache over the `agentops.dev` kinds: resourceVersion resume, relist-on-410, per-kind stores, delta fan-out; tested against recorded watch JSON fixtures
- [ ] 3.3 Extend the cache to `deployments` and `pods` for install facts (images, digests, readiness, restart counts, phase)
- [ ] 3.4 Activity consumer: one upstream SSE connection to the manager, cursor tracking, resync handling, in-memory windowed index by conversation and by edge
- [ ] 3.5 Channel adapter loop: long-poll `/channel/ops?adapter=console`, complete `ensure-topic` with `console-<conversation-UID>`, dedupe by op id, bounded per-thread transcript
- [ ] 3.6 Signal origination client: `POST /signal/inbound` with `kind: chat`, `agentops.dev/channel` and `agentops.dev/sender`, using the signal identity

## 4. Console backend — browser API (Go)

- [ ] 4.1 Auth: token/session for reads; write gate requiring `WRITE_ENABLED` plus a resolved identity (trusted forward-auth header when present, token identity otherwise); unconfigured token authorizes nobody and is indistinguishable from a wrong one; every write logged with identity
- [ ] 4.2 `GET /api/overview` — versions, manager health, adapters, runtimes, capacity, and the non-`True` condition rollup across every kind plus pod readiness
- [ ] 4.2b `GET /api/queues` — work queue (conversations waiting on a slot, inputs waiting behind an inflight run, from CR watch) and delivery queue (per-adapter queued and claimed-but-uncompleted ops, from `/status`), each entry aged, with stuck-item flags and active cooldowns; BFF polls `/status` on a short interval and pushes deltas over the existing stream
- [ ] 4.3 `GET /api/config/{kind}[/{name}]` — inventory, detail, YAML, inbound references, and cross-object findings (dangling refs, unclaimed sources, unserved channels, configSchema violations, profiles without runtimes), each marked as reported-condition or console-derived
- [ ] 4.4 `GET /api/topology` — nodes for all nine kinds, edges (`feeds`/`answers`/`posts`/`served-by`/`uses`), health from conditions only, plus windowed per-edge event rates and latencies
- [ ] 4.5 `GET /api/conversations` — server-side filtering (phase, pipeline, profile, channel, age, errored), sorting by last activity, pagination with total match count, run history excluded from rows
- [ ] 4.6 `GET /api/conversations/{name}` — runs, inputs queue, thread bindings, runtime pod, transcript, and the object
- [ ] 4.7 `GET /api/conversations/{name}/graph` — elements built from the Conversation's OWN recorded bindings (profile, runtime, toolsets, MCP configs, channels, adapters) plus its events, with a flag when the attributed Pipeline's current wiring differs
- [ ] 4.8 `POST /api/conversations` — origination against a named console source; refuse when unclaimed, carrying the `Wired=False` reason; refuse when writes are disabled
- [ ] 4.9 `POST /api/conversations/{name}/messages` — reply via `/channel/inbound`
- [ ] 4.10 `GET /api/stream` — one SSE multiplexing CR deltas, activity events and transcript appends, each with a cursor; resync on connect
- [ ] 4.10b Optional metrics-backend client (`console.metrics.url`, Prometheus/VictoriaMetrics query API): historical aggregates for windows beyond the ring buffer, clearly typed as aggregate; every view fully functional when unconfigured, with long windows reported as unavailable rather than rendered empty
- [ ] 4.11 Contract tests per endpoint against fixture caches; snapshot/stream convergence test (apply N deltas disconnected, reconnect, assert equality with a cold fetch)

## 5. Console frontend — foundation (React/TypeScript)

- [ ] 5.1 Scaffold `console/ui`: React 18 + TypeScript + Vite + PatternFly 6; app shell, routing, error boundaries, empty/error states
- [ ] 5.2 Data layer: TanStack Query for snapshots, one SSE multiplexer into a Zustand store, cursor-driven query invalidation, reconnect-with-resync
- [ ] 5.3 Auth flow: token login, session cookie, 401 handling, write-disabled mode hiding write affordances
- [ ] 5.4 Build integration: multi-stage Dockerfile (node build → `go:embed all:ui/dist`), `dev` build tag serving from disk, `make ui` for a plain `go build`

## 6. Console frontend — views (React/PatternFly)

- [ ] 6.1 Overview page — version cards, manager/adapter/runtime health, capacity, and the problem rollup linking to objects
- [ ] 6.1b Queues page — work and delivery queues side by side, aged rows, stuck-item flags with the reason (nothing claiming / claimed and wedged / at runtime ceiling / runtime hung), active cooldowns, and links to the conversation, adapter or pipeline concerned
- [ ] 6.2 Configuration pages — per-kind lists with kind-specific columns, detail with conditions/spec/YAML/inbound-references, resolved capabilities on Pipeline detail rendered verbatim from the manager, and cross-object findings distinguished by source
- [ ] 6.3 Topology graph with PatternFly React Topology — all nine node classes, edge kinds, condition-derived health, detached nodes with reasons, broken edges to placeholders, side panel on select
- [ ] 6.4 Display panel — per-class show/hide (sources, channels, adapters, profiles, runtimes, toolsets, MCP configs, runtime pods), traffic animation on/off, idle nodes/edges, edge labels (none/rate/latency); persisted across navigation and reload
- [ ] 6.5 Hidden-element honesty — hidden classes still counted in health summaries and the problem rollup, with an indicator when hidden classes contain failures
- [ ] 6.6 Traffic animation driven by activity events, rate-scaled, with error edges marked and enqueued-but-unconfirmed rendered distinctly from confirmed delivery
- [ ] 6.7 Conversations list — filters, sorting, pagination, state badges, match counts
- [ ] 6.8 Conversation detail — transcript with composer, run timeline, inputs queue, thread bindings, runtime pod, raw object; composer absent with reason and patch when not joined
- [ ] 6.9 Conversation graph tab — same Display panel, elements from recorded bindings, re-wire divergence notice
- [ ] 6.10 Sequence/waterfall tab — hops in time order with per-hop latency
- [ ] 6.11 New-conversation flow — picker listing `Wired=True` console sources labeled by claiming pipeline and profile; unavailable state showing the claiming patch when none are wired
- [ ] 6.12 Time-window control — buffer-bounded windows by default, longer windows served from the metrics backend when configured and labeled as aggregate; unavailable-with-reason rather than empty when neither covers the request
- [ ] 6.12b Historical charts sourced from metrics (throughput per pipeline, run-duration percentiles, queue depth over time) using PF Charts, present only when a backend is configured
- [ ] 6.13 Plain-text rendering of all cluster- and wire-sourced text (markup stripped, never trusted)
- [ ] 6.14 Vitest + React Testing Library across views; Playwright smoke: load topology, drive three synthetic events, assert the animated edges are the ones the events name, toggle a class and assert layout/health stability

## 7. Chart

- [ ] 7.1 `console.enabled` default `true`; add `console.write.enabled` (default `true`) and `console.signalSourceName` (default `console`)
- [ ] 7.2 Render the ChannelAdapter, Channel, externally-served SignalAdapter (`servedBy`), SignalSource, UI token Secret, and Role/RoleBinding
- [ ] 7.3 RBAC: namespaced read-only on every `agentops.dev` kind plus `deployments` and `pods`; assert no write verb
- [ ] 7.4 Keep Service `ClusterIP` and Ingress disabled by default; document OIDC/forward-auth as the answer for exposure
- [ ] 7.5 Optional `VMServiceScrape` and `ServiceMonitor` templates targeting the manager's metrics port, plus example alert rules (ops queued with nothing claiming; runtime slots at ceiling with waiters) — all default-disabled, since neither CRD is guaranteed present
- [ ] 7.6 Chart major bump + `CHANGELOG.md` migration entry: the default flip from `false` to `true`, what the new pod reads, and the one-value opt-out

## 8. Verification on a live cluster

- [ ] 8.1 Deploy at defaults; confirm the console pod runs and exactly ONE Deployment exists despite two adapter identities
- [ ] 8.2 Claim the console source from `k8s-ops`; confirm the source flips `Wired=True` and the picker offers it
- [ ] 8.3 Start a conversation against the `stub` runtime (no LLM cost); confirm it traverses the topology graph, is auto-joined, appears in its own conversation graph, and answers in the transcript
- [ ] 8.4 Negative: with the source unclaimed, confirm origination is refused with the `Wired=False` reason and no Conversation is created
- [ ] 8.5 Negative: `console.write.enabled=false` hides and rejects both write paths
- [ ] 8.6 Set `console.enabled=false`; confirm pod, Deployment and Service are gone, console Channels report `Served=False`, and every other pipeline keeps delivering

## 9. Retire the old console

- [ ] 9.1 Delete the previous `console/` implementation (Go sources and hand-written `ui/` assets) once phase 8 passes
- [ ] 9.2 Rewrite `docs/console.md` for the replacement: pages, Display panel, origination-requires-a-claim, trust boundary, values
- [ ] 9.3 Mark the `console-adapter`, `console-topology`, `console-live-runs` and `console-deployment` specs as superseded by this change's deltas on archive
- [ ] 9.4 `go build ./... && go vet ./...`, full envtest suite, and the frontend test suite green
