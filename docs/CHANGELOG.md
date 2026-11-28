All notable changes to this project are documented here, in
[Keep a Changelog](https://keepachangelog.com/en/1.1.0/) format. Versions are
CHART versions. Image tags move independently and are named in each entry.

This file holds the **ten most recent versions**. Older entries are in
`changelog/`, linked at the [foot of this page](#older-versions).

See [the repository](https://github.com/kostiantyn-matsebora/agent-ops-operator)
for the source and the reference material beside this file.

## [25.0.0] — 2026-11-22

Images: manager 0.66.0, console 0.46.0, channel-telegram 0.28.0.

### Added

- **A testing station in the delivery workflow.** Between implement and fix, a
  session runs the unit, system and e2e tests, measures the coverage of the lines
  the pull request adds against floors of 90, 70 and 50 percent, and has a
  different model write what is missing. Behaviour that depends on the model is
  tested against a real one. The fixing loop now also merges the base branch in.
  Delivery tooling only: no chart value, image or CRD changes, and no upgrade step.

### Changed

- **The Home Assistant bundle's MCP server now defaults to streamable HTTP at
  `/api/mcp`.** It was the legacy SSE transport at `/mcp_server/sse`, and a
  POST there is refused with `405`, which is all the pi runtime (the default
  runtime) can send. `ha-control` and every agent bound to `ha-api` lost all its
  Home Assistant tools under pi, silently. `home-assistant.mcp.transport: sse`
  keeps the old endpoint for a runtime that speaks it, and a `home-assistant.mcp.url`
  you set is untouched. `home-assistant.mcp.transport: http` with no `url` used
  to resolve to `/mcp`, which Home Assistant does not serve, and now resolves to
  `/api/mcp`.
- **BREAKING.** A conversation's `ID` is now its system identity, minted once
  by the backend at `Create` and never reused. `Name` stays the readable
  word-chain — the store key and the pod name — reusable once a prior life
  under that name is deleted. Every internal reference moves to `ID`: the
  workspace subpath, `CONVO_ID`, the runtime pod's
  `agentops.dev/conversation-id` label, op ids and `causedBy.Parent`. The
  signal and chat correlation key, formerly `ConversationID`/`conversationId`,
  is renamed to `CorrelationID`/`correlationId` throughout the API types, the
  CRD and both contract pages. The channel adapter wire field
  `conversationId` is repurposed to carry the new system identity instead of
  the correlation key — the correlation key moves to a new `correlationId`
  field on the same operation. The channel adapter contract moves to wire
  `contract=3`, and an adapter still declaring `contract=2` is refused `400`.

### Fixed

- **The console lost the parent-child relation between a Coordinator root and
  its members.** A member's `causedBy.parent` is its parent's system ID now, and
  the console's tree, scopes, breadcrumbs and Coordinator counts resolve it by
  name, so every member sat at the top level beside its root. The console's API
  names the parent by name again, and a member counts toward its root's
  Coordinator.
- **A Coordinator's `aops` tool calls were refused with "token does not match
  the declared identity".** The chart's `aops` MCPConfig forwards `CONVO_ID` as
  the caller's identity, which is now the conversation's system ID, and the
  `/coordinate/*` handlers still looked the caller up by name. They now resolve
  the caller by ID; the token is still derived from the name. A target a tool
  names stays a name.
- **`runtime-claude` read a failed resume against an unreadable context store as
  `continued`.** The vendor opens a new session and returns its id on the error
  result, and that id was counted as a recovery, so the outage check never ran. A
  failed turn now counts as recovered only when it says something, and the run
  reports the outage. Takes effect with the next `runtime-claude` image.

### Upgrade

1. **Delete every `Conversation` before upgrading.** The old CRD has no
   `correlationId` field, and `kubectl apply` would prune it from any object
   still carrying the renamed one.
2. **Apply the CRDs before running `helm upgrade`**:
   `kubectl apply -f chart/crds/`. Helm never upgrades an existing CRD on its
   own.
3. **Rolling back** to the previous chart version needs the same step in
   reverse: delete every `Conversation` again first, since the old CRD would
   otherwise prune `spec.correlationId` from each one.

## [24.0.0] — 2026-11-22

Images: manager 0.66.0, console 0.46.0, runtime-pi 0.2.0, runtime-ollama 0.2.0. Every other image is unchanged.

### Added

- **`maxActiveMembers` (`MAX_ACTIVE_MEMBERS`, default `5`) bounds Coordinator
  members on their own pool**, separate from `maxActiveConversations`. A
  member is a conversation an `invoke` created, `spec.causedBy` set.
- **Model limits as chart values.** `contextWindow` and `maxOutputTokens`, on
  the `pi` and `ollama` bundles and in `global.agentops.runtimeDefaults`, the
  bundle's own value winning. `pi` writes them into its `models.json` entry
  (it assumed 128000 and 16384 otherwise). The bundle defines them, 400000 and
  128000, for its default model. `ollama` takes `contextWindow` as the new name
  of `numCtx` and sends `maxOutputTokens` as `num_predict`. `claude`,
  `claude-cli` and `copilot` have no setting and refuse the value on their own
  bundle. `0` leaves a runtime's own behaviour.
- **`global.agentops.platformNodeSelector` places every platform pod.** The
  manager, NATS, the Telegram gateway and every MCP server take it as their
  `nodeSelector`, and a component's own `nodeSelector` replaces it for that
  component. The default is empty, so nothing about an install changes until it
  is set. Adapters, signals and the console are not covered, because the
  manager builds their pods from their own resources.

### Changed

- **BREAKING (capacity): runtime-pod capacity is now two independent pools.**
  `maxActiveConversations` keeps its default of `5` and now counts
  conversations only. Coordinator members draw from the new
  `maxActiveMembers` pool instead, so total simultaneous pods can reach 10
  where it was 5. Admission, FIFO ordering, idle eviction and the
  freed-slot pickup all run per pool — a full pool never blocks the other.
  The `agentops_runtime_slots_in_use` and `agentops_runtime_slots_max`
  gauges gain a `pool` label (`conversations`, `members`), and the old
  unlabelled series are retired in this same change.

- **The command bus delivers with plain JetStream and a sequential convoy.**
  A message is stored once and its task is a record. Order per key comes from
  a temporary convoy queue per key with one consumer and one message in flight,
  and redelivery from the broker's own acknowledgement wait. Accepting work no
  longer waits on any consumer's view, which is what stopped every producer when
  a replica's view stalled. Delivery mode is configuration per message type, so
  a new type needs a routing entry and no change to the bus. The `Bus` interface
  is unchanged. The per-invocation in-flight cap of the `conv` queue is dropped.
  The consumer pool's concurrency bounds parallel handlers.
- **A message is never processed for ever.** `commandBus.handlerTimeout`
  (default `5m`, `COMMANDBUS_HANDLER_TIMEOUT`) bounds each message. At the
  deadline the handler's context is cancelled and the attempt fails with a
  retryable code. While a handler runs, the pool keeps its lease alive.
- **The earlier bus's streams are deleted by the manager.** On start the bus
  deletes `<scope>_cmdbus_log` and `<scope>_cmdbus_dead` if they exist, and logs
  which it removed. Nothing reads them any more. What they held, work accepted
  but not yet run by the earlier backend, is not handed to the new one.

### Fixed

- A turn that stops on `length` with no text now fails the run naming the token
  counts, on `pi` and `ollama`, instead of "no answer".
- `pi` reports `tokensIn` as the contract defines it, cache parts included, so
  the console's "Fresh" column is no longer negative, and reports each tool
  call's `resultBytes`, so "Result size" is filled.
- **A closed conversation no longer serves work, so it stays closed.** A runtime
  pod still polling when `/close` landed, with an input queued before it, was
  handed that input, and dispatching it wrote `Working` over `Closed`. Closed is
  sticky until a reopen, so the conversation never read Closed again and its pod
  ran on until its idle timeout. The work endpoint now answers nothing for a
  closed conversation.
- **A command bus that cannot accept messages no longer stops conversations
  from starting.** A reconcile posted to the chat channels (topics, the thread
  entries, member results) before it created the agent's pod, all on one
  deadline. With the bus stuck, the first post used the whole deadline, the
  pod was never created, and every slot stayed free while conversations waited
  (#527). The chat steps now share a short budget (ten seconds) and starting
  the agent comes after them within the rest of the reconcile.
- **Reading a conversation's runs no longer creates a NATS consumer per
  call.** Each read, trim and cascade listed the runs bucket's keys, which made
  the server create and delete a consumer. During a disturbance a quarter of
  those never delivered, so a reconcile almost never completed, and the dead
  consumers piled up (637 on one stream). The runs are now served from a
  watch-fed mirror, like the conversation records. The orphan sweep also
  trims a conversation holding more than its ten runs back to ten, which clears
  what failed trims left behind.
- **The manager's watch on the state store backs off instead of retrying every
  second.** A mirror whose watch could not be opened retried once a second,
  per bucket and per replica, and every attempt that timed out could leave a
  consumer behind on the NATS server. After a storage disturbance that grew
  to about two thousand dead consumers on one bucket and kept the manager's
  list reads timing out. The pause now doubles on each failure up to 30
  seconds and starts over once a watch has delivered. The same applies to the
  command bus's queue mirror.
- **One stuck call no longer stops every conversation from starting or
  closing.** The conversation controller reconciled one conversation at a time
  with no time limit, so a single call that never answered held the only
  worker and everything behind it waited, with free capacity (#527). It now runs
  four workers, each reconcile has a two-minute deadline that turns a hang into
  a retried error, and the active-conversation cap is decided and claimed in
  one guarded step so parallel workers cannot exceed it.

### Upgrade

- **To hold today's total pod count, lower `maxActiveConversations` and
  `maxActiveMembers` between them.** Each has a minimum of `1`. Size node
  resources for the sum of both, not for `maxActiveConversations` alone.
- **A Prometheus rule or dashboard reading `agentops_runtime_slots_in_use`
  or `agentops_runtime_slots_max` with no `pool` label stops matching.** Add
  `pool="conversations"` to keep today's reading, or sum both pools to
  recover the old combined figure.

## [23.0.0] — 2026-11-21

Images: console 0.44.0. Every other image is unchanged.

### Changed

- **BREAKING (console only): `GET /api/conversations` drops `showClosed` and
  `rootTotal`.** A browser's stored `showClosed` preference is harmless and
  ignored.
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

## [22.0.0] — 2026-11-19

Images: manager 0.64.0. Every other image is unchanged.

### Changed

- **Demo mode runs one manager and one NATS server.** `global.demo.enabled`
  defaults `replicas`, `nats.replicas` and `nats.storageReplicas` to `1`, so the
  demo schedules on a single node. The manager's required anti-affinity is
  rendered only above one replica. A value you type still wins in both modes.
- **`nats.storageReplicas` is new** and sets how many servers keep a copy of each
  stream, apart from the pod count. It defaults to `3`. Scaling the pods no longer
  changes it.
- **Breaking:** outside demo mode `nats.replicas` below `3` is refused, and so is
  `nats.storageReplicas` above `nats.replicas`.

### Fixed

- The manager no longer carries a replica count of its own. The command bus's
  hardcoded `3` and the state store's drift check are gone. Both take the count
  the chart passes (`NATS_REPLICAS`, `COMMANDBUS_NATS_REPLICAS`), so a one-server
  install works.
- A one-replica NATS runs standalone, without cluster routes it waits on forever.
- The manager no longer exits when NATS is not up yet. It retries the connection
  and waits, and the chart's probes decide how long that may last.

### Upgrade

- **Outside demo mode, `nats.replicas` below `3` now fails the render.** Set it to
  `3` or more. An install that ran one NATS pod on purpose must either move to
  three or turn demo mode on.
- **`nats.storageReplicas` is new and defaults to `3`.** It is how many servers
  keep a copy of each stream. The manager applies it to the streams that exist
  when it starts, so a changed value takes effect on the next manager start.
  Raise `nats.replicas` before you raise it, and lower it before you lower
  `nats.replicas`.

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

## Older versions

| Archive | Covers |
|---|---|
| [CHANGELOG-15.0.0-16.0.0.md](changelog/CHANGELOG-15.0.0-16.0.0.md) | chart 15.0.0 through 16.0.0 |
| [CHANGELOG-14.0.1.md](changelog/CHANGELOG-14.0.1.md) | chart 14.0.1 |
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
