All notable changes to this project are documented here, in
[Keep a Changelog](https://keepachangelog.com/en/1.1.0/) format. Versions are
CHART versions. Image tags move independently and are named in each entry.

This file holds the **ten most recent versions**. Older entries are in
`changelog/`, linked at the [foot of this page](#older-versions).

See [the repository](https://github.com/kostiantyn-matsebora/agent-ops-operator)
for the source and the reference material beside this file.

## [Unreleased]

**BREAKING (console only). `GET /api/conversations` drops `showClosed` and
`rootTotal`.** A browser's stored `showClosed` preference is harmless and
ignored.

### Changed

- **The console's Open scope replaces All**, and is the default. It lists
  root conversations with `phase≠Closed`. Closed rows no longer show behind
  a "Show closed" toggle on any other scope — the toggle and its stored
  preference are gone, and Open excludes them the same way Working, Mine and
  Errored already did.
- **Every inbox badge, Open's and Closed's included, counts unread roots.**
  The Closed badge used to count every closed root, read or not — it now
  matches every other scope and counts only the unread ones.
- **"Archive" reads "Closed"** in every user-visible string: the inbox row,
  the thread banner, the close dialog and the `/close` chip.
- **Selection counts roots.** "N selected", the button suffixes and the
  dialog titles count selected roots. A member carries no selection control
  and cannot be selected. The dialogs still name the member count the
  cascade reaches, on its own line.

## [21.0.0] — 2026-11-19

Images: manager 0.63.0, console 0.43.0, signal-k8s-events 0.7.0, runtime-pi
0.1.0. Bundle kubernetes 0.4.3.

**The console opens on the new `workspace` look.** A viewer who has chosen
nothing gets it. The Look switcher in the header, or a `?look=classic` link,
still gives the old one, and a stored choice always wins. To make `classic`
the install's default, set `console.defaultLook=classic`.

**BREAKING. The default runtime is now pi, not claude.** A route that names no
`runtimeRef` runs on `agentops-runtime-pi` 0.1.0, on provider `anthropic` with
model `claude-sonnet-5`. The `claude` runtime stays on and a route selects it
with `runtimeRef: claude`. Read [Upgrade](#upgrade) before upgrading.

### Added

- **A fifth runtime: `agentops-runtime-pi` 0.1.0, shipped as the `pi`
  bundle.** ON by default and flagged `default: true`, so the `default`
  `AgentRuntime` is a copy of it. `pi.provider` (default `anthropic`) and
  `pi.model` (default `claude-sonnet-5`) select the model, and the credential
  is the Secret `agentops-pi` with the key `apiKey`, or
  `pi.credentialsSecret.token` to have the chart create it. A route selects
  the runtime with `pipelines[].runtimeRef: pi`. Pi reads
  `.claude/agents/<agent>.md`, the same path and composition rules as the
  reference runtime, and reaches bound MCP servers through the vendor's
  `pi-mcp` package. State lives under `.pi/agent/sessions/**`, declared to
  `context-sync` by the bundle.
- `pi.piCode` (default `true`) lets pi read a checkout's `.claude/` the way
  Claude Code does, through the `pi-code` package. It adds no tool, and the
  allowlist still gates every tool call. A hook in the repository's
  `.claude/settings.json` runs as a shell command in the pod, and the
  allowlist does not apply to it. Set `pi.piCode: false` to turn this off.
  See [Residual risk](security.md#residual-risk).

### Upgrade

Create the credential before you upgrade, in the namespace runtime pods run in:

```sh
kubectl -n agent-ops-conversations create secret generic agentops-pi \
  --from-literal=apiKey=$ANTHROPIC_API_KEY
```

```powershell
kubectl -n agent-ops-conversations create secret generic agentops-pi `
  --from-literal=apiKey=$env:ANTHROPIC_API_KEY
```

Without it a runtime pod exits at start naming `PI_API_KEY`.

**To keep running on claude**, set `pi.default=false`, and the `default`
runtime is a copy of claude again. Set `pi.enabled=false` to render no pi
runtime at all. Open conversations keep the runtime they started on.

## [20.0.0] — 2026-11-17

Images: manager 0.63.0, console 0.42.0, signal-k8s-events 0.7.0. Bundle
kubernetes 0.4.3.

**BREAKING. A release now resolves two namespaces, split by default.** The
control namespace (the manager, wiring, adapters, state store) is unchanged.

A new conversations namespace holds runtime pods and everything they run
as, mount or resolve. Read [Upgrade](#upgrade) before upgrading. It needs
a manual step per claim you want to keep.

### Added

- `global.agentops.conversationNamespace` (default `agent-ops-conversations`)
  and `global.agentops.createConversationNamespace` (default `true`). Setting
  the former to the release namespace reproduces the single-namespace layout
  exactly, with no object split in two.
- `CONVERSATION_NAMESPACE`, read by the manager at startup, defaulting to
  `NAMESPACE`. Injected as a literal value on every channel and signal
  adapter, beside `POD_NAMESPACE`.
- A second manager Role, in the conversations namespace: pods, configmaps, `conversations`
  and `conversationinputs` and their status, events, PVC
  `get`/`list`/`watch`/`create` — never `delete`, `update` or `patch`. No ClusterRole is added.
  Single-namespace installs still render one merged Role, identical in
  substance to today's. The control Role keeps a read on pods, for the
  manager's own replicas.
- A second console Role, in the conversations namespace: `pods`
  `get`/`list`/`watch` alone, so the Infrastructure view keeps every runtime
  pod. The console reads `CONVERSATION_NAMESPACE`, defaulting to
  `POD_NAMESPACE`.
- An expected-references condition on the `Conversation`, naming every
  Secret and ConfigMap its pod will resolve — profile repository auth,
  profile and runtime `env[].valueFrom`, MCP config `valueFrom` and raw
  references. Derived from the referencing CRs alone. The manager still
  reads no Secret.
- `signals/k8s-events` gains a fourth, unconfigurable self-exclusion
  mechanism: every event in the conversations namespace is dropped whenever
  it differs from `POD_NAMESPACE`, with no object read. The signal adapter
  specification moves to **2.5.0** — `POD_NAMESPACE` and
  `CONVERSATION_NAMESPACE` join [Environment](contracts/signal-adapter.md#environment),
  and `ADP-10` names the new mechanism. Every shipped adapter's claimed
  version moves to 2.5.0. Only `signals/k8s-events` implements `ADP-10` —
  the others carry no namespace-scoped exclusion to extend.
- The chart creates the conversations namespace by default, with the
  release's labels and `helm.sh/resource-policy: keep` — `helm uninstall`
  leaves it, its Secrets and its claims standing.
- A `SignalAdapter` pod receives `ADAPTER_NAME` (the CR's name), the same env
  name channel adapters get, replacing `SOURCE_TYPE`. Rename it in any
  self-registering sender that read the old name.
- The `SignalAdapter` reconciler creates no ServiceAccount. It names the
  account in `spec.serviceAccountName` and falls back to the release floor.

### Changed

- **BREAKING: runtime pods, their MCP ConfigMaps, every chart-rendered and
  manager-created persistence claim, every `rbac.runtime.serviceAccounts`
  entry and bundle route account, and the housekeeping and context-probe
  CronJobs all move to the conversations namespace.** The floor account
  `agentops-runtime` now renders in both namespaces, bound to nothing in
  either.
- **BREAKING: every Secret and ConfigMap a runtime pod resolves — profile
  repository auth, profile env, the runtime's model credential, MCP config
  references, image pull secrets — must exist in the conversations
  namespace.** The manager does not copy them there. See
  [Upgrade](#upgrade).
- The chart renders every Secret it creates that a runtime pod resolves into
  the conversations namespace, so a token-supplied credential needs no copy.
  `agentops-ha-operator` renders in both namespaces, since the admin MCP server
  and the log source read it from the release namespace too.
- Under the `kube` state backend, `Conversation` and `ConversationInput` move
  to the conversations namespace. `Signal`s and the rest of the state store
  stay in control. Under `nats` (the default) nothing here is a CR, so
  nothing moves.
- The chart's generated `stateReadAuth` token now renders into both
  namespaces from the same value, since housekeeping moved with its claims.
  `allowPodExecution` guards this copy exactly as it already guards every
  adopter Secret a runtime pod resolves.
- NetworkPolicy peers (under the existing, default-off
  `global.agentops.networkPolicy.enabled`) gain a `namespaceSelector`
  wherever they cross the split: the manager's own ingress, `mcp-aops`, the
  `kubernetes`, `home-assistant`, `prometheus` and `pulse` MCP servers. The runtime ingress-deny
  policy now renders in the conversations namespace.

### Upgrade

This release moves objects between namespaces. There is no code path that
moves state for you — follow these steps in order, on the release you are
upgrading:

1. **Close every open conversation**, so no pod or claim is in use.
2. **Create the conversations namespace**, or let the chart create it
   (`createConversationNamespace: true`, the default).
3. **Create every Secret a runtime pod resolves in the new namespace** —
   profile repository auth, profile env, the runtime's model credential, MCP
   config references, image pull secrets.
   The Secrets the chart creates itself from a token (the `claude` and
   `copilot` runtime credential, the home-assistant control and operator
   tokens, the pulse observer and operator tokens) are rendered there for you.
   Only a Secret you reference by name, or create out of band, needs this step.
4. **Carry each persistence claim across:**
   1. Set the bound PV's reclaim policy to `Retain`.
   2. Delete the old claim, then clear the PV's `claimRef`.
   3. Pre-create the claim in the new namespace, `volumeName` set to that PV.
   4. Point the chart at it with `persistence.<volume>.existingClaim`. A
      manager-created claim needs no setting — the manager is idempotent by
      existence.
5. **Apply the CRDs, then `helm upgrade`:**

   ```sh
   kubectl apply -f chart/crds/
   helm upgrade agent-ops oci://ghcr.io/kostiantyn-matsebora/charts/agent-ops-operator -n agent-ops
   ```

   ```powershell
   kubectl apply -f chart/crds/
   helm upgrade agent-ops oci://ghcr.io/kostiantyn-matsebora/charts/agent-ops-operator -n agent-ops
   ```
6. **Under the `kube` state backend, delete the `Conversation` CRs left in
   the release namespace** — they receive no reconciliation there.
7. **Verify** with an ordinary signal, and confirm the pod appears in the
   conversations namespace.

**Rollback** is `conversationNamespace` set back to the release namespace.
Objects in the new namespace go inert rather than disappearing — every claim
still carries `resource-policy: keep`.

**Context survives only where the claim was carried.** A conversation
promised context it no longer has fails its run rather than answering
fresh, same as any other `promised-and-lost` case.

## [19.0.1] — 2026-11-14

A security patch. Six images are rebuilt on current base images, with no
source change, so every published image passes the weekly vulnerability scan
again.

- `context-sync` 0.2.6, `egress-proxy` 0.2.7, `gateway-telegram` 0.7.1,
  `mcp-pulse` 0.1.1, `runtime-claude-cli` 0.1.1, `runtime-ollama` 0.1.4.
- Bundles `telegram` 0.6.1 and `pulse` 0.2.2, each shipping its rebuilt
  image, above.

### Security

- Go standard library CVE-2026-111943 (`crypto/x509`), fixed by building
  with Go 1.27.4. Affected `context-sync`, `egress-proxy`,
  `gateway-telegram` and `mcp-pulse`.
- Debian `apt` CVE-2026-109518, fixed in apt 2.6.2. Affected
  `egress-proxy`, `runtime-claude-cli` and `runtime-ollama`.
- Debian `libssl3` CVE-2026-84782, fixed in 3.0.22-1~deb12u2. Affected
  `runtime-claude-cli` and `runtime-ollama`.

### Fixed

- The weekly image scan picked a component's newest tag by version sort,
  which ranks a pre-release tag above its release. It judged
  `manager` 0.62.0-state-store.1, an image no chart ships, and failed on a
  finding `manager` 0.62.0 does not have. It now scans release tags only.

No values or CRDs change. Upgrade with `helm upgrade` as usual.

## [19.0.0] — 2026-11-14

**BREAKING.** The default state backend is now `nats`, the manager accepts
work through a durable command bus, and channel adapters read, acknowledge,
then report each op. Read [Upgrade](#upgrade-1) before upgrading.

- `manager` 0.62.0, `console` 0.41.0, `channel-telegram` 0.27.0,
  `housekeeping` 0.3.0, `mcp-aops` 0.2.0, `runtime-claude` 0.13.0,
  `runtime-copilot` 0.2.2.
- `signal-alertmanager` 0.9.0, `signal-cron` 0.3.0, `signal-ha` 0.5.0,
  `signal-k8s-events` 0.6.0, `signal-pulse` 0.3.0, `signal-telegram` 0.7.0,
  each sending a `signalId`.
- Bundles `prometheus` 0.2.3, `pulse` 0.2.1, `home-assistant` 0.2.3,
  `kubernetes` 0.4.2 and `telegram` 0.6.0, each shipping the adapters it
  embeds, above.

### Added

- A new values key, `stateStore.backend`, selecting where the manager keeps
  conversation state. Exactly one backend may be selected, and the render
  refuses an unknown value. The key covers `Signal`s and each source's
  cooldown window and receipt counters as well as conversations.
- A new `nats` state backend, a clustered JetStream KV and Object Store the
  chart runs as its own three-server bundle. It passes the same conformance
  suite and bench as `kube`.
- **BREAKING: the default backend is now `nats`, not `kube`.** State lives on
  the bundle's own clustered volumes, not the Kubernetes API —
  `kubectl get conv` and `kubectl get sig` no longer show anything live on a
  fresh install. Pin the old behaviour with `stateStore.backend: kube`.
- **BREAKING: a default install now runs three more pods and three more
  volumes**, for the `nats` bundle.
- Retention behaves the same on both backends. Deleting a conversation now
  also releases the bodies of its queued inputs, deleting a source clears its
  receipt counters as well as its cooldown window, and under `nats` a
  conversation's runs are removed with it and a periodic sweep catches what a
  crash left behind. A conversation keeps its newest ten runs either way.
- The state streams write a keep-alive, and the console drops a stream that
  stays silent and relists, so a manager that dies without closing its socket
  no longer freezes the console's view.
- The manager waits for the state server to accept its buckets instead of
  exiting while the cluster forms its quorum.
- An upgrade from `kube` imports every conversation with its runs and queued inputs,
  every signal, cooldown window, receipt counter and adapter state into `nats` on the manager's first start, before it
  reports ready, and leaves the custom resources in place.
- A new NetworkPolicy admits the state server's client port from the manager
  alone, and its peer port from its own pods alone, where network
  restriction is enabled.
- A new chart-managed credential, the Secret `agentops-nats-credentials`,
  resolved explicit value, then existing Secret, then generation on install
  only — the same order as `adapterAuth`/`controlPlaneAuth`.
- A new `Signal` CRD and a publish/consume split on `POST /signal/inbound`.
  The endpoint validates, stores one `Signal` per admitted message and
  acknowledges. A separate consumer pool on the leader then claims it, spends
  cooldown, applies capacity and routes it, so a leader change or a crash
  loses and duplicates nothing. Each `Signal` records a per-claimant routing
  workflow and a roll-up phase.
- An optional per-message `signalId` on a normalized signal. A re-send with
  the same `signalId` is stored once. Every shipped signal adapter now sends
  one.
- Six chart values bound the queue: `signalConsumerConcurrency`,
  `signalRetryDeadline`, `maxSignalsPerSource`, `signalTtlRouted`,
  `signalTtlDropped` and `signalTtlFailed`.
- The console's read-only signals view, with the per-claimant routing detail,
  reached from the Configuration pages.
- Input origin `manager`, for an input the manager appends under a rule of its
  own. Today only the corrective input, named `move-per-turn`. The console
  attributes it to the manager.
- `status.inflight.moved` on a Conversation, written by the manager's own
  verb handlers and read when a run completes.
- A new `/state/*` HTTP surface: `GET /state/conversations`,
  `GET /state/conversations/{name}`, `GET /state/conversations/stream`,
  `GET /state/signals` and `GET /state/signals/stream`. The one door onto
  Conversations and Signals for a reader that is not a reconciler.
- A new CRD field, `spec.stateAccess` (default `false`), on `ChannelAdapter`
  and `SignalAdapter`. Grants that adapter's own derived token the state
  read surface above. Silence grants nothing.
- A new values key, `stateReadAuth.{secretName, existingSecret, token}`, the
  credential guarding `/state/*` for a caller that is not an adapter —
  today, housekeeping. Generated the same way as `adapterAuth` and
  `controlPlaneAuth`, and refused at render if it equals either.
- A new command bus, `platform/manager/internal/commandbus/`, independent of
  the state store: accepts work durably, runs it later on any replica, and
  records what became of it. Two backends, `nats` and `memory`, pass one
  conformance suite and bench.
- A new `commandBus.backend` values key (default `nats`). By default the
  bus shares the state store's NATS server and credential, so nothing is
  added or configured. `commandBus.nats.enabled: true` renders a dedicated
  three-server bundle (`commandBus.nats.*`) and `commandBus.nats.url` points
  at an external one, each independent of `stateStore.backend`. `memory` is for
  tests and local development, refused above one manager replica.
- A new read-only surface: `GET /state/tasks` (per-queue statistics),
  `GET /state/tasks/{id}`, `GET /state/tasks/stream` and
  `GET /state/dead-letters`, on the existing `/state/*` token scheme.
- Every channel adapter now takes an outbound op in three steps: a read that
  changes nothing, an acknowledgement that hands it over, and a report. See
  the breaking change below. The channel operations endpoints are
  `GET /channel/ops`, `POST /channel/ops/{id}/ack` and
  `POST /channel/ops/{id}/done`, and any manager replica answers each. They
  used to proxy to the leader.
- A new endpoint, `GET /channel/ops/stream`, for receipt-only adapters (the
  console): the ops pushed over one server-sent-events connection in queue
  order. It only reads. After a reconnect it pushes again exactly what was
  never acknowledged, with no cursor. The console consumes it, so a dropped
  connection or a rollout no longer leaves it stale until a refresh.
- Two chart values for the channel delivery contract:
  `commandBus.channelReceiptAdapters` (default `[console]`) and
  `commandBus.deliveryTtl.{stable,ephemeral,topic,deleteConversation}`
  (default 10m, 2m, 5m and 5m). They render `CHANNEL_RECEIPT_ADAPTERS` and
  `CHANNEL_DELIVERY_TTL_*`.

### Changed

- A finished `Signal` now expires through the state store: the write that makes
  its phase terminal stamps an expiry (under `kube`, the same expiry annotation
  a conversation carries), and the store's sweep removes it. This replaces the
  consumer's conditional delete and requeue. The TTL values are unchanged.
- Deleting a `SignalSource` now releases its `Signal`s and cooldown window
  through the manager instead of relying on the owner-reference cascade. The
  outcome is the same, except that a source deleted and recreated under one
  name may still lose its new incarnation's signals and window to a narrow
  race, pending the departed-identity release tracked as task 6.9.
- A source's `receivedTotal` and `lastReceived` counters are now written
  through the state store too, so no manager state is left outside it. Under
  `kube` the fields and their values are unchanged.
- An adapter's own cursor (`/channel/state`, `/signal/state`) is now written
  through the state store too. Under `kube` it is the same
  `agentops.dev/adapter-state-<key>` annotation, byte-identical, so an
  adapter upgraded across this change reads back what it wrote before.
- The housekeeping job now lists conversations through the manager's
  `/state/*` surface, never the Kubernetes API, and **holds no RBAC at all**.
  It needs `MANAGER_URL` and `STATE_READ_TOKEN` reachable, both now
  projected by the chart automatically. A release with
  `global.agentops.networkPolicy.enabled: true` must admit it on the
  manager's NetworkPolicy, which this chart version does unconditionally —
  no values change needed.
- The console now reads Conversations and Signals through the same surface
  instead of a Kubernetes watch. Its `ChannelAdapter` now declares
  `spec.stateAccess: true`, so its derived adapter token opens `/state/*`
  (not the `stateReadAuth` credential housekeeping uses). Its Role no longer
  grants either kind.
- A re-sent `signalId` whose earlier signal has expired but is not yet swept
  (up to the sweep period, 30 seconds) is now stored, not acknowledged as a
  duplicate and dropped.
- The per-source cooldown cache no longer keeps a source whose window could not
  be read, so the next signal reads it again instead of running on an empty
  window.
- The manager now reaches `Conversation`, `ConversationInput` and `Signal` through a state
  store contract. There is **no upgrade step and no CRD change**. The objects
  are byte-identical and `kubectl get conv` is unchanged.
- Behaviour under `kube`: the manager's read of a single conversation now goes
  to the Kubernetes API server instead of its informer cache, so a write's
  compare-and-swap never races a stale copy. Listings stay cached. Expect more
  API server reads.
- A closed or evicted conversation's pod and ConfigMap are now deleted by the
  manager explicitly. The owner reference remains, but nothing depends on
  garbage collection.
- The signal adapter specification moves to **2.3.0**. The `200` response is
  now `{"queued": n}`: the `conversations` field is withdrawn, and no routing
  outcome appears on it (`API-2` is replaced). A `5xx` asks for a re-send
  (`API-3`). No shipped adapter read the withdrawn field, so none stops
  conforming.
- The signal adapter specification moves again to **2.3.1**, wording only:
  `API-7` says the manager's per-phase TTL expires a terminal `Signal`, where
  it said the TTL deletes it. Behaviour is unchanged.
- The signal adapter specification moves again to **2.4.0**.
  `POST /signal/inbound` now accepts, then processes: the success status
  moves from `200` to `202`, and gains `invocationId` and `taskIds`, one per
  admitted signal (`API-2`, `API-3` replaced). `queued` now means accepted,
  not stored. No shipped adapter checks for exactly `200`, so none stops
  conforming, and every shipped adapter's claimed version moves to `2.4.0`
  in this same change.
- A machine signal arriving at a full conversation backlog now waits as
  `Pending` and routes when it drains. A chat message is still declined at
  once and told on its surface.
- An addressed conversation adopts the `initiationId` as its
  `conversationId`, like every other claimant.
- A Coordinator root's turn must end in a move: `invoke`, `escalate`, `close`
  or an answer to a person. A succeeded root run that made none of them and
  answered no person is handed back once as an input, then escalated on the
  second consecutive miss. The manager only refuses an undecided turn and
  never picks the move. See [concepts](concepts.md#every-root-turn-ends-in-a-move).
- A Coordinator root's work unit is framed as coordination, without the
  checkout and role-file framing a repository agent gets. The profile's
  `systemPrompt` is appended as before.
- `runtime-claude` offers the model only the built-ins its allowlist grants.
  A route granting none sees none. Per-call enforcement is unchanged.
- The `aops` tool descriptions say which tools are moves and which are reads.
- A cascading close closes descendants before the ancestor.
- **BREAKING (semantic): `close`, `exit`, `reopen`, `delete`, `reset-context`
  and `escalate` now mean accepted, not performed.** `POST
  /channel/conversations/{name}/reset-context` moves from `200` to `202`.
  Every other converted endpoint keeps its existing status and gains
  `invocationId`/`taskIds` in its body. A caller that needs the outcome
  reads `/state/tasks/{id}`.
- `delete` now cascades deepest-first as one task per conversation, instead
  of one request walking the whole subtree inline.
- A reopen naming a conversation with a missing profile, and a
  `/coordinate/close` outside its caller's widened bound, used to answer
  `409`/`403` immediately. Both now accept and the task dies with
  `precondition_failed` or `forbidden`.
- **BREAKING (channel adapters): a channel adapter reads, acknowledges, then
  reports.** `GET /channel/ops` no longer takes the op. `POST
  /channel/ops/{id}/ack` hands it over, after which the manager never sends it
  again and a delivery TTL starts. `POST /channel/ops/{id}/done` reports done,
  or failed with an `error`. An op with no report by its TTL is failed by the
  manager, and a failure is final. The `reclaimAfterSeconds` field and the
  claim window are removed. The channel adapter specification moves to 2.0.0.
- A channel op's failure is final, not retried by the manager. What is still
  owed is derived again from the conversation after a cool-off.
- Every command-bus queue is an append-only log. A message that succeeded is
  removed, and one never consumed is kept 7 days. Dead letters are kept in a
  log of their own (7 days, 10000 records).
- `status.threads[].claim` is no longer written. The command bus's own
  dedup makes the reconciler-level dispatch claim unnecessary. A value a
  pre-upgrade record still carries is ignored, never trusted.
- `POST /signal/inbound` now accepts, then processes, like every other
  converted endpoint. It validates, applies the source's
  `attributeFilters`, durably enqueues one `ingest-signal` task per
  admitted signal on its own queue, `in.signal`, and replies `202`. The
  `in.signal` consumer then answers an in-place chat command, or creates
  the `Signal`, with the bus's bounded retry and dead-letter record. No
  adapter needs to change.

### Fixed

- A change the state feed reported could be skipped for good on a three-server
  state store, because a follower that had not applied a write yet could
  answer the read of its record. Reads now go to the stream leader.
- The manager's liveness probe no longer kills it while a slow state server
  elects a leader. The pod now has a startup probe of up to five minutes.
- A non-leader replica that cannot reach the leader keeps the request body for
  its own handler.
- A chat message whose conversation cannot be read is refused for retry
  instead of dropped.
- The console waits out a refused connection to the manager for a few seconds
  instead of failing the call, so a message or a close typed while the manager
  rolls is no longer lost.
- A state-store feed opened without a resume point no longer skips what is
  written after the server resets its consumer. The reconciler could miss new
  conversations for good, leaving them with no status and no pod.
- The manager's start-up against a state server that is still forming its
  cluster no longer spends the whole two-minute wait on one request the server
  never answers. Each attempt is bounded and retried.
- A changed `nats.replicas` leaves the existing buckets as they are and logs
  the difference. It no longer re-replicates a live bucket.

### Upgrade

**BREAKING: the default state backend changes to `nats`.** Helm never
upgrades an existing CRD, so apply the new and changed ones with the chart
before `helm upgrade`:

```sh
kubectl apply -f chart/crds/
```

```powershell
kubectl apply -f chart/crds/
```

This adds the `Signal` CRD, one Conversation field and one enum value.
Adapter and manager upgrade in either order: an older manager ignores
`signalId`. An adapter that reads `conversations` from the response, if you
wrote one, must stop.

An upgrade that can read the cluster and finds no `agentops-nats-credentials`
Secret and no explicit value fails the render. The error names the Secret and `nats.auth.existingSecret`.

The new manager starts and finds the empty `nats` buckets and the `kube`
records. The replica that wins the leader lease imports them and reports
ready once the import finishes.

Verify through the console or `/state/conversations` before trusting it.
Then delete the old custom resources yourself — the import never does:

```sh
kubectl -n agent-ops delete conversations,conversationinputs,signals --all
```

```powershell
kubectl -n agent-ops delete conversations,conversationinputs,signals --all
```

**Pin the old behaviour instead** with `stateStore.backend: kube` if you rely
on reading conversations through the Kubernetes API.

**Rollback loses what `nats` received after the upgrade.** Setting
`stateStore.backend` back to `kube` and upgrading again does not copy
anything written to `nats` since the first upgrade — every conversation
opened or continued after it is lost. Snapshot (`nats stream snapshot`)
first if that matters.

**BREAKING: `close`, `exit`, `reopen`, `delete`, `reset-context` and
`escalate` now mean accepted, not performed.** A caller that read a `200`
or a `202` body as confirmation the work finished must instead read
`/state/tasks/{id}` for the outcome, using the `taskIds` the same response
now also carries.

**BREAKING: upgrade every channel adapter image together with the manager.** The wire contract of `/channel/ops` changed. An adapter that never calls the acknowledgement is offered the same op forever and delivers nothing.

The shipped `channels/telegram` and the console move with this release. A hand-built adapter follows the [channel adapter specification](contracts/channel-adapter.md), 2.0.0.

With the state store on `nats` (the default), the command bus shares its three-server bundle and credential, adding nothing to render. With the `kube` store it renders its own three-server bundle, three more pods and volumes.

Pin `commandBus.backend: memory` on a single manager replica to avoid them, at the cost of losing every accepted task across a restart.

**Rollback** is the same shape as the state store's own: set
`commandBus.backend: memory` on one replica, or roll back the chart.
Accepted but unprocessed tasks are lost either way.

The queue log starts empty on first install. Work owed at the moment of
upgrade is re-derived by the reconciler, exactly as after any restart
today.

An install that ran a pre-release build of the command bus holds KV buckets
named `tasks`, `ids` and `dead` (under the bus's scope prefix) on its NATS
server. The release reads none of them, and they may be deleted. Its streams are
`cmdbus_log` and `cmdbus_dead`.

## [18.0.1] — 2026-10-25

### Fixed

- `adapterAuth`/`controlPlaneAuth`'s generated Secrets could only ever mint
  on a fresh `helm install`. An existing install adopting 18.0.0's new
  `controlPlaneAuth` token had no automatic path to it — the first upgrade
  that needed the Secret rendered nothing, leaving the manager pod stuck on
  `CreateContainerConfigError`. A render with real cluster access (a real
  `helm upgrade`, never `helm template`) now generates or adopts it on an
  upgrade too, via a new `agentops.clusterConnected` helper that probes
  `kube-system` rather than trusting `.Release.IsInstall` alone. A
  cluster-less renderer (`helm template`, CI, a GitOps repo-server) still
  renders neither Secret, unchanged.

## [18.0.0] — 2026-10-25

- `manager` 0.61.0, `console` 0.40.0, `runtime-claude` 0.12.0,
  `runtime-claude-cli` 0.1.0 (new), `runtime-copilot` 0.2.1,
  `runtime-ollama` 0.1.3.
- `signal-alertmanager` 0.8.1, `signal-pulse` 0.2.0, `signal-ha` 0.4.3,
  `signal-k8s-events` 0.5.1, `signal-cron` 0.2.2, `signal-telegram` 0.6.7,
  `channel-telegram` 0.26.2.
- Bundles `prometheus` 0.2.2, `pulse` 0.2.0, `home-assistant` 0.2.2,
  `kubernetes` 0.4.1, `telegram` 0.5.2 — each ships the signal adapter it
  embeds, above. `claude-cli` 0.1.0 joins `claude`/`ollama`/`copilot` as a
  fourth optional runtime bundle.

### Added

- The adopter site carries a spec page per CRD under `/crds/<kind>/` — hand
  prose around a generated example instance and field table, with a CRDs
  sidebar group. The Introduction presents the model as a domain-model
  diagram, every kind on it linking its spec page. Sidebar groups are
  collapsible, the current page's group open.
- A published page for the reference runtime at `/runtimes/claude/`, first
  in the Runtimes sidebar group — the SDK bundle a fresh install runs on,
  beside the `claude-cli` compatibility bundle's page. The Integrations and
  Runtimes sidebar entries carry their marks.
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
- `chart/charts/claude-cli/`, a new optional bundle (off by default, like
  `ollama`/`copilot`) running today's spawn-per-unit `claude` CLI — see
  [runtimes/claude-cli](runtimes/claude-cli.md).

### Changed

**BREAKING: `runtime-claude` (the chart's default-eligible runtime) now
drives `@anthropic-ai/claude-agent-sdk` in-process instead of spawning the
`claude` CLI.** Its image no longer carries the CLI binary.

It holds one vendor session open across a pod's work units. It re-opens
that session only when the session-affecting configuration changes —
system prompt, MCP servers, `maxTurns`, cwd, or agent. See
[the runtime contract](contracts/runtime.md).

#### Upgrade

An adopter who needs to keep running the CLI-spawning behavior must, in
order:

1. Enable the new bundle: `claude-cli.enabled=true`.
2. Point every Pipeline or Coordinator that should keep running CLI-driven
   `claude-code` at `runtimeRef: claude-cli`.
3. Decide which bundle holds `default: true` for routes naming no
   `runtimeRef` — the render fails unless exactly one does.

Conversations in flight keep their handle regardless of which component
serves them. Both resume from `runtimeContextId`.

**BREAKING: the Pulse signal adapter's default `initiationId` now folds in
the alert's start time, not just its Pulse-assigned id.** A `SignalSource`
served by `signal-pulse` gains `spec.config.correlationStrategy`, `"id"` or
`"id+startTime"`.

Pulse can reuse an alert id for what it calls a new occurrence. The new
default, `"id+startTime"`, opens a fresh conversation for that re-fire
instead of continuing an old one.

### Removed

- `docs/cr-reference.md` — the per-kind spec pages under `/crds/` replaced
  it. Every in-repo pointer moved with it.
- `docs/claude.md` — the published page at `/runtimes/claude/` replaced it.
  Every in-repo pointer moved with it.
- The landing page's animated presentation and its `ao-presentation` theme
  component. The "How it works" tab shows the generated readme-flow picture
  the README already carries.

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

## Older versions

| Archive | Covers |
|---|---|
| [CHANGELOG-14.0.0.md](changelog/CHANGELOG-14.0.0.md) | chart 14.0.0 |
| [CHANGELOG-13.4.0.md](changelog/CHANGELOG-13.4.0.md) | chart 13.4.0 |
| [CHANGELOG-13.3.0.md](changelog/CHANGELOG-13.3.0.md) | chart 13.3.0 |
| [CHANGELOG-13.2.0.md](changelog/CHANGELOG-13.2.0.md) | chart 13.2.0 |
| [CHANGELOG-13.1.1.md](changelog/CHANGELOG-13.1.1.md) | chart 13.1.1 |
| [CHANGELOG-13.1.0.md](changelog/CHANGELOG-13.1.0.md) | chart 13.1.0 |
| [CHANGELOG-13.0.1.md](changelog/CHANGELOG-13.0.1.md) | chart 13.0.1 |
| [CHANGELOG-13.0.md](changelog/CHANGELOG-13.0.md) | chart 13.0.0 |
| [CHANGELOG-11.0-12.0.md](changelog/CHANGELOG-11.0-12.0.md) | chart 11.0.0 through 12.0.0 |
| [CHANGELOG-channel-telegram-0.24.2.md](changelog/CHANGELOG-channel-telegram-0.24.2.md) | channel-telegram 0.24.2 |
| [CHANGELOG-9.0-10.0.md](changelog/CHANGELOG-9.0-10.0.md) | chart 9.0.0 through 10.0.0 |
| [CHANGELOG-8.0.md](changelog/CHANGELOG-8.0.md) | chart 8.0.0 |
| [CHANGELOG-5.22-7.0.md](changelog/CHANGELOG-5.22-7.0.md) | chart 5.22.0 through 7.0.0 |
| [CHANGELOG-5.0-5.21.md](changelog/CHANGELOG-5.0-5.21.md) | chart 5.0.0 through 5.21.0 |
| [CHANGELOG-1.0-4.0.md](changelog/CHANGELOG-1.0-4.0.md) | chart 1.0 through 4.0.0 |
