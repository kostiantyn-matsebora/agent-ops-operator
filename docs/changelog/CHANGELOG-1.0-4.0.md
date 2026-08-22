# Changelog archive — chart 1.0 to 4.0.0

Migration guides for chart versions **1.0.0 through 4.0.0**, newest first, in
[Keep a Changelog](https://keepachangelog.com/en/1.1.0/) format.

Moved here from [CHANGELOG.md](../CHANGELOG.md), which holds the ten most recent
versions.

## [4.0.0] — 2026-08-09

No image changes. This is a values and object-ownership move.

### Changed

**BREAKING: the main chart owns the agent runtime.**

`AgentRuntime` was created in exactly one place, the `k8s-bundle` subchart, and
nothing about it is Kubernetes-shaped. Image, credential, idle TTL, node
placement and home volume describe *how an agent executes*, which is the same for
a metrics lane, a chat-only install or a Kubernetes one.

The placement was never a decision. Chart 2.x relocated the parent's `demo.*`
block wholesale and the runtime rode along, because demo mode and the bundle
became the same thing.

The interest paid since:

- An install with only `telegram-bundle` rendered nothing that could execute a
  conversation.
- Two runtime ServiceAccounts existed — the parent's, granted nothing, and the
  bundle's, granted everything.
- `homePvcRef` was a documented copy of a claim the parent creates.
- Idle TTL had two defaults that disagreed, 1 and 10.

**Idle TTL stops being configured twice, and drops from 10 to 1.** The bundle
defaulted `idleTtlMinutes: 10` while the manager's `runtimeIdleTtlMinutes` was
`1`.

An empty `runtime.idleTtlMinutes` now follows the release value, so a finished
conversation gives its slot back in a minute rather than ten.

Set `runtime.idleTtlMinutes: 10` to keep the old behaviour for this runtime, or
raise `runtimeIdleTtlMinutes` for the release.

The chart WRITES the field rather than omitting it.
`AgentRuntime.spec.idleTtlMinutes` has a CRD default of `10`, so an omitted field
is stored as `10` and the manager prefers any non-zero spec value.

Omitting it renders a correct-looking manifest and a wrong stored object.

**The bundle keeps its identity, not its substrate.** `profile.enabled` now
renders exactly one object, the `AgentProfile`. `profile.runtimeRef` still points
at a runtime other than `default`. `eventsAdapter.rbac` is a different block and
is untouched.

### Added

**A default install now renders an `AgentRuntime`.** `runtime.enabled` is `true`,
so a chart with no bundle — or with only `telegram-bundle` — can execute a
conversation.

If you manage `AgentRuntime` CRs yourself, set `runtime.enabled: false`. A name
collision on `default` makes Helm adopt or conflict loudly rather than silently
change behaviour.

**`rbacMode` is one knob, and empty grants nothing.** `none` | `readonly` |
`full`, defaulting to empty, which resolves to `readonly` under
`global.demo.enabled` and to `none` otherwise.

A release that never set an RBAC value therefore holds no bindings after upgrade,
exactly as before. `full` is never selected by a default or inferred path.
`rbac.runtime.{clusterRoles,bindClusterRoles,namespaced}` stay additive.

**The MCP server's posture derives from that knob.** `full` yields a
write-capable server under a `full` SA and renders `k8s-admin`. Every other mode
yields a read-only server under a `readonly` SA.

It derives because a read-only server under a `full` agent pushes every write
back onto kubectl, which is the single-wall path the component exists to replace.

The cost is real. Widening the agent widens the server unless you set
`k8s-bundle.mcpServers.readOnly: true`, which recovers "kubectl writes, MCP reads
only".

### Upgrade

**Two upgrade-visible effects that are not value renames. Read these first.**

1. **The runtime ServiceAccount is renamed** — `agentops-runtime-k8s` →
   `agentops-runtime`, the one the parent already created and the manager already
   defaults runtime pods onto.

   Helm replaces the bundle-named bindings with global-named ones in the same
   upgrade. **If you bound your own (Cluster)Roles to `agentops-runtime-k8s`,
   re-point them.** Nothing else will.

   ```sh
   kubectl get clusterrolebinding | grep agentops-runtime-k8s   # expect nothing
   ```
2. **An install that enabled `k8s-bundle` without touching MCP now gets an MCP
   server workload.** `mcp.enabled` and `mcpServers.enabled` both default `true`
   and flip as a pair, because the config's URL needs a Service to default onto.

   Hold position: `k8s-bundle.mcpServers.enabled: false`, plus
   `mcp.enabled: false` or an `mcp.url` of your own.

**Values migration:**

| 3.x | 4.0 |
|---|---|
| `serviceAccounts.runtime` | `global.agentops.runtime.serviceAccountName` (setting the old key now FAILS the render) |
| `k8s-bundle.profile.runtime.image` | `runtime.image` |
| `k8s-bundle.profile.runtime.credentialsSecret.*` | `runtime.credentialsSecret.*` |
| `k8s-bundle.profile.runtime.nodeSelector` | `runtime.nodeSelector` |
| `k8s-bundle.profile.runtime.resources` | `runtime.resources` |
| `k8s-bundle.profile.runtime.idleTtlMinutes` | `runtime.idleTtlMinutes` (empty follows `runtimeIdleTtlMinutes`) |
| `k8s-bundle.profile.runtime.homePvcRef` | *automatic from `persistence`* |
| `k8s-bundle.profile.runtime.name` | `runtime.name` |
| `k8s-bundle.profile.runtime.create` | `runtime.enabled` |
| `k8s-bundle.profile.runtime.serviceAccountName` | `global.agentops.runtime.serviceAccountName` |
| `k8s-bundle.rbac.mode` | `global.agentops.runtime.rbacMode` |
| `k8s-bundle.rbac.enabled: false` | `global.agentops.runtime.rbacMode: none` |
| `k8s-bundle.mcpServers.readOnly` | *derived from `rbacMode`, still settable* |
| `k8s-bundle.mcpServers.rbac.mode` | *derived from `rbacMode`, still settable* |

Moved a value into `k8s-bundle.profile.runtime.*` during the 1.x to 2.x hop? That
table is under 2.0.0 below. This one names where it went next.

**Rollback is chart 3.4.0.** The CRDs are unchanged, so a downgrade re-renders
the bundle-owned objects. The only manual step is deleting the parent-owned
`AgentRuntime` if its name differs from the bundle's.

## [3.4.0] — 2026-08-09

manager `0.21.0`, `channel-telegram` `0.6.1`, `console` `0.2.0`.

**Ship them together.** An adapter that does not know the new `close-topic` op
leaves topics open.

### Added

Nothing bounded how many conversations ran at once in terms an operator
recognises. The only cap was `MAX_RUNTIMES`, named after pods, and an idle
runtime held its slot for ten minutes after the agent had stopped working.

Conversations also never ended. There was no way to say "this one is done", so
threads accumulated and capacity came back only by eviction.

- **Over-cap work waits in `Pending` with NOTHING provisioned** — no runtime pod,
  no MCP ConfigMap and no chat topic. That last one is the point: a burst of
  signals no longer becomes a burst of chat threads.

  Admission is FIFO by creation time. `Queued` keeps its old meaning — admitted,
  waiting its turn.
- **The backlog is bounded too**, at `maxQueuedConversations: 50`. Past it
  `/signal/inbound` declines to create a conversation and reports the batch
  dropped for capacity. Chat senders are told on the surface they typed on.

  Window reuse is unaffected. The bound gates new objects, not new inputs.
- **`/close` ends a conversation** from its thread. Any sender who can post there
  may use it, and it is honoured mid-run — the farewell names the abandoned work.

  It deletes the `Conversation`. The pod and `agentops-mcp-conv-<name>` follow by
  owner reference. `kubectl delete conversation` behaves identically, archiving
  threads first.

  On a general surface `/close` answers with usage, so a Pipeline named `close`
  is no longer addressable from chat.

### Changed

**Two defaults change on upgrade, both visible.**

| Old | New | Effect |
|---|---|---|
| `maxRuntimes: 8` | `maxActiveConversations: 5` | less throughput, more queueing — nothing is dropped, only delayed |
| `runtimeIdleTtlMinutes: 10` | `runtimeIdleTtlMinutes: 1` | a finished conversation gives its slot back within a minute |

Raise `maxActiveConversations` to restore the old figure. `maxRuntimes` still
works for ONE release — unset by default, and setting it emits `MAX_RUNTIMES`
while the manager logs the deprecation — then it is removed.

The idle TTL trade is latency, not memory. Sessions live in `/data/home` and
resume with full context. Raise it, or set `AgentRuntime.spec.idleTTLMinutes`,
for runtimes with expensive startup.

### Upgrade

1. Review the two changed defaults above.
2. **Third-party channel adapters should handle `close-topic`.** It carries the
   `threadId` to archive and completes with an EMPTY body.

   An adapter that ignores it is not broken. The op fails its 2-minute grace and
   deletion proceeds, leaving one open thread per closed conversation.
3. **Nothing to migrate for existing objects.** `Pending` is an additive phase
   value. No old manager writes it, and rolling back is a chart rollback — phase
   is status-only and nothing keys behaviour off it.

## [3.3.0] — 2026-08-09

manager `0.20.0`, `signal-k8s-events` `0.2.0`, `console` `0.1.0`.

### Changed

**Cluster events get suppression, workload grouping and a loop breaker.**

The events lane created **hundreds of conversations** on a healthy cluster.

On an unhealthy one it fed itself. A runtime pod that could not start emitted a
Warning event, which became a signal, which opened a Conversation, which created
another runtime pod under a new name, forever.

**BREAKING for existing conversations: `grouping.signatureLabels` defaults to
`[namespace, workload]`**, was `[namespace, kind, name]`.

They keep their old per-pod signature hash and go orphaned. No action is needed —
they age out of the 7-day reuse window on their own, and new conversations group
per workload.

**Default `rules` now ship.** A default install suppresses rollout churn out of
the box. If you had built your own `excludeReasons` list it still works, since it
translates into leading drop rules, but review it against the new defaults.

**Reason matching is now anchored.** `excludeReasons: [Failed]` no longer also
drops `FailedMount`. If you were relying on the accidental prefix match, widen it
to a rule with an explicit regex.

The self-exclusion invariant is **not configurable** for its first two mechanisms
— name prefix, and owner/label. `source.includeOwnNamespace: true` relaxes only
the coarse namespace rule, for installs that co-locate their own workloads with
the operator.

### Added

**The agent-ops console.** A browser view of the whole install — CR inventory,
wiring graph, live conversation runs — that is also a channel.

Conversations on pipelines listing its Channel bind a console thread, so you can
reply to an agent from the run you are watching.

- New `console/` module and image `agentops-console`.
- `ChannelAdapter.spec` gains `kubernetesAccess`, for parity with SignalAdapter.
  It mounts the SA token and injects `POD_NAMESPACE`. **Identity only** — the
  operator still creates and binds no RBAC, so an adapter CR cannot escalate.
- Enable with `console.enabled=true`. It renders **CRs and RBAC only** — a
  ChannelAdapter, a Channel, the UI token Secret, and a namespaced read-only Role
  for SA `agentops-adapter-console`. The reconciler brings up the Deployment and
  Service.
- **Nothing is wired by enabling it.** Conversations show as *observed* until you
  add the console Channel to a Pipeline's `channelRefs[]`. The chart never edits
  your Pipelines.
- **Trust boundary:** anyone holding the UI token sees every agentops CR in the
  namespace, conversation payloads included. Keep the Service ClusterIP unless
  you mean to expose it.

Manager image `0.20.0` is required for `kubernetesAccess` on ChannelAdapter. An
older manager silently ignores the field — the CR accepts it, the pod comes up
without a token, and the console crash-loops on the missing CA file.

**The k8s-bundle can create the agent's credential Secret.** Additive and
default-off. `k8s-bundle.profile.runtime.credentialsSecret.token`, when set,
renders the Secret the `AgentRuntime` already referenced
(`agentops-claude`/`oauthToken`).

The credential then survives a teardown with the release, the same way
`telegram-bundle` handles its bot token. Leaving it empty keeps the old behaviour
exactly.

The old failure mode was silent. The reference is resolved by the kubelet, not
the manager, so a missing Secret produces runtime pods in
`CreateContainerConfigError` and conversations that queue forever, with no
condition anywhere saying why. The post-install notes now name it.

### Upgrade

**The events adapter needs new permissions.** The events component now grants
read-only `pods` and `replicasets` (`list`/`watch`) alongside `events`.

If you render the bundle's RBAC (`eventsAdapter.rbac.create: true`, the default)
this happens for you. If you bind that RBAC yourself, add it. The adapter reports
`Ready=False` naming the missing permission rather than degrading silently.

**Stopgap for anyone still on `signal-k8s-events` 0.1.x.** The conversation
explosion is a values edit away from stopping, with no new image:

```yaml
k8s-bundle:
  eventsAdapter:
    source:
      excludeReasons: [Unhealthy, FailedScheduling, SandboxChanged, Preempting]
      grouping:
        signatureLabels: [namespace, alertname]
```

That bounds conversations by namespaces × reasons instead of pods × rollouts.
Agent *runs* do not drop, since cooldown is still per object plus reason, and it
does not break the self-reference loop. Only 0.2.0 does that.

## [3.2.0] — 2026-08-08

manager `0.18.0`, runtime-claude `0.2.0`.

### Changed

**BREAKING: the runtime no longer invents a tool.** `runtime-claude` used to pass
`--allowedTools Read` whenever a work unit carried no allowlist — a grant nobody
declared.

From image `0.2.0` it passes exactly what was composed, empty included, and runs
with `--permission-mode dontAsk` so an unlisted tool is denied rather than
prompted for. A prompt in a pod hangs until the idle TTL.

**Who is affected:** any conversation whose route binds no `toolsets` *and* whose
agent definition declares no `tools:`. It used to get `Read`. Now it gets
nothing, starts, finds it can do nothing, and says so.

### Added

**`toolsets.mode` is back, with a different counterpart.** It composes against
the **agent definition's** `tools:` frontmatter, never against the profile.
Profiles carry no capabilities, and that misreading is why the field was removed
in 3.0.

`merge` is the default and additive. `overwrite` passes the route's tools alone.
`mcpConfigs` has no mode.

Existing Pipelines need no edit. The CRD defaults `mode: merge`, which reproduces
today's behaviour wherever the agent definition declares nothing.

The work unit gained `toolsMode` and `agent` alongside `allowedTools`. Custom
runtimes that ignore them keep working — `allowedTools` alone is what `merge`
degrades to when nothing is declared repo-side.

### Upgrade

1. Upgrade the operator to manager `0.18.0`. Additive — existing Pipelines
   default to `merge`.
2. Update the runtime image to `agentops-runtime-claude:0.2.0`. Staying on
   `0.1.1` keeps the `Read` substitution and ignores the mode.
3. Find the routes that will lose their implicit `Read` and grant what each one
   actually needs:

   ```sh
   kubectl get pipelines -A -o json | jq -r \
     '.items[] | select(.spec.toolsets == null)
      | "\(.metadata.namespace)/\(.metadata.name) -> profile \(.spec.profileRef.name)"'
   ```

   Fix either side — bind a toolset on the Pipeline, or declare `tools:` in the
   repo's `.claude/agents/<agent>.md`. Under the default `merge` they add up.

## [3.1.0] — 2026-08-08

Two breaking changes land together.

### Changed

**BREAKING: `/channel/inbound` is reply-only.** `threadId` is now REQUIRED and an
unknown thread is no longer adopted.

Third-party adapters that posted bare messages to originate a conversation get a
`400` naming the signal path.

`PipelineForChannel` is gone, and with it the "oldest Ready pipeline referencing
this channel answers" tiebreak. So is any channel default profile.

**BREAKING: Telegram ingest is three components.** Telegram serves one update
stream per bot token, so origination and continuation cannot each poll:

```
getUpdates ─▶ telegram-router ─┬─ no topic ─▶ signal-telegram  ─▶ /signal/inbound
              (the only poller) └─ topic    ─▶ channel-telegram ─▶ /channel/inbound
```

`channel-telegram` no longer polls. `Channel.spec.config.pollingEnabled` is
removed. Ingest is on when the router runs.

### Upgrade

**Step 3 is the one where ordering matters.**

1. **Upgrade the operator first.** `ChannelAdapter.spec.port` and the reconciler's
   Service parity are inert until used.
2. **Add the chat `SignalSource` and claim it**, plus the router's
   credential-carrying source. Nothing changes behaviourally yet, since the old
   adapter is still polling.

   ```yaml
   apiVersion: agentops.dev/v1alpha1
   kind: SignalSource
   metadata: {name: home-ops-chat, namespace: agent-ops}
   spec:
     adapter: telegram          # signal-telegram, no credentials needed
     config:
       chatId: "-1001234567890"
       channel: home-ops        # the Channel this chat is the general surface of
     grouping: {cooldownHours: 0}
   ```

   Then add `home-ops-chat` and `telegram-router` to your Pipeline's
   `signalSourceRefs`. **A chat source no Pipeline claims answers nobody.** The
   user is told so on the surface, but nothing runs.
3. **Scale the old adapter to zero and CONFIRM no `getUpdates` consumer remains**
   before enabling the new stack. Two consumers of one token means 409s and
   stolen updates. This is the failure that costs the most debugging.

   ```sh
   kubectl scale deploy/agentops-adapter-telegram -n agent-ops --replicas=0
   kubectl get pods -n agent-ops | grep telegram   # expect none
   ```
4. **Carry the offset across.** The cursor lives in the
   `agentops.dev/adapter-state-telegram-offset` annotation on the Channel the old
   adapter wrote to. The new stack reads it from the FIRST served channel by
   name, so if your old leader differs, copy the value over:

   ```sh
   kubectl get channel home-ops -n agent-ops \
     -o jsonpath='{.metadata.annotations.agentops\.dev/adapter-state-telegram-offset}'
   ```
5. **Enable the bundle.** Telegram now ships as the embedded `telegram-bundle`
   subchart, so the flat `telegramAdapter` values are gone:

   ```yaml
   telegram-bundle:
     enabled: true
     surface:
       enabled: true
       chatId: "-1001234567890"
       credentials:
         existingSecret: agentops-telegram   # key: botToken
   ```

   `enabled` alone ships the three IMPLEMENTATIONS and wires nothing, which is
   the right shape if you manage Channels yourself. `surface.enabled` also ships
   the Channel and the chat `SignalSource`, and then requires everything it
   cannot guess.

   The router's credential is the SAME one the Channel uses. It polls the bot the
   channel sends as.
6. **Claim the sources in a Pipeline.** Your existing Pipeline keeps its channels.
   Add the new sources to its `signalSourceRefs`:

   ```yaml
   signalSourceRefs:
     - name: telegram-ops-chat   # the chat surface
     - name: telegram-router     # emits nothing, claimed so it is not Wired=False
   ```

   Until this lands, chat messages drop with `Wired=False` and the user is told
   so. The post-install notes print this block with your names filled in.

Rollback is steps 3 and 5 together: re-enable the old single-container adapter
AND revert the operator, since the origination paths must come back with it.
Conversations already created are unaffected either way.

## [3.0.0] — 2026-08-08

### Removed

**BREAKING: `AgentProfile.spec.allowedTools` and `spec.mcp` are removed.**

`ToolingBinding.mode` is gone too. Drop `mode:` from any existing stanza.

### Upgrade

**Removing a CRD field prunes that data on the next write**, so audit before
upgrading, not after:

```sh
# every profile that still declares capabilities, and what they are
kubectl get agentprofile -A -o custom-columns=\
'NS:.metadata.namespace,NAME:.metadata.name,TOOLS:.spec.allowedTools,MCP:.spec.mcp'
```

For each profile that lists anything, move it before you upgrade:

| Was on the profile | Goes on |
|---|---|
| `allowedTools: "Read,Bash"` | an `MCPToolset`, referenced from the routing Pipelines' `toolsets` |
| `mcp.configRefs: [x]` | the routing Pipelines' `mcpConfigs.refs` |
| `mcp.servers: {...}` | an `MCPConfig`, then referenced as above |
| `mcp.configMapRef` / `secretRef` | an `MCPConfig` with the same field, bound alone |

If the profile should be reachable, give it a Pipeline. That is what routes to
it.

At chart 3.0 a task named that Pipeline via `POST /task`. The endpoint was
removed in chart 5.0.0, and a task now names a source instead.

## [2.0.0] — 2026-08-08

### Changed

**BREAKING: the demo block moves wholesale into `k8s-bundle`.** A subchart can
read no parent scope except `global.`, which is why the toggle moved there.

| Chart 1.x | Chart 2.x |
|---|---|
| `demo.enabled` | `global.demo.enabled` |
| `demo.runtimeImage` | `k8s-bundle.profile.runtime.image` |
| `demo.credentialsSecret.*` | `k8s-bundle.profile.runtime.credentialsSecret.*` |
| `demo.readOnlyRbac: true` | `k8s-bundle.rbac.mode: readonly` (the default) |
| `demo.readOnlyRbac: false` | `k8s-bundle.rbac.enabled: false` |
| *inherited `persistence`* | `k8s-bundle.profile.runtime.homePvcRef` — **set this explicitly**, subcharts cannot see the parent's `persistence` block |
| *inherited `runtimeIdleTtlMinutes`* | `k8s-bundle.profile.runtime.idleTtlMinutes` |

### Upgrade

1. Restate every `demo.*` value under its Chart 2.x path, from the table above.
2. **Set `homePvcRef` explicitly.** It was inherited from the parent's
   `persistence` block, and a subchart cannot see one.

The runtime ServiceAccount is renamed `agentops-runtime-demo` →
`agentops-runtime-k8s`. `helm upgrade` removes the old objects with the deleted
`demo.yaml` template.

The `AgentRuntime` named `default` re-renders with identical semantics, so
existing conversations keep resolving their runtime.

## [1.12.0] — 2026-08-08

### Changed

**BREAKING: `Channel.spec.type` and `SignalSource.spec.type` are now
`spec.adapter`.**

The value was always a reference to the serving adapter CR. The old name read as
an intrinsic attribute, making the sibling `config` look like part of one flat
schema.

The contract follows:

| Was | Now |
|---|---|
| `?type=` on `/channel/ops`, `/channel/channels`, `/signal/sources` | `?adapter=` |
| `CHANNEL_TYPE` / `SOURCE_TYPE` | `ADAPTER_NAME` |

The retired parameter returns 400 naming its replacement rather than an empty
list.

### Upgrade

`adapter` is immutable, so live Channels and SignalSources are delete-and-recreate.

**Carry their annotations across.** Adapter cursor state — the Telegram
`getUpdates` offset, cron last-fire — lives in `agentops.dev/adapter-state-*`
annotations on those objects. A bare recreate makes the adapter re-read old
updates.

Manager and adapter images must be upgraded together, since both sides of the
contract change at once.

## [1.8.0] — 2026-08-07

### Added

`SignalAdapter.spec.kubernetesAccess`, default false. It mounts the adapter's
ServiceAccount token and injects `POD_NAMESPACE`.

**The operator still grants adapters no RBAC whatsoever.** Permissions come from
the chart or from you, bound against the deterministic SA
`agentops-signal-<name>`.

The vm-bundle uses it for `alertmanager.registration`, which replaces the manual
VMAlertmanager repoint.

### Upgrade

Nothing. Existing adapters are untouched.

## [1.7.0] — 2026-08-07

### Changed

**BREAKING: `ChannelAdapter` and `SignalAdapter` lose `spec.type` and
`spec.env`.** The CR **name** is now the type key, and adapter CRs carry no
configuration at all.

`SignalAdapter` gains `spec.port`. When set, the reconciler owns the Service
`agentops-signal-<name>` and injects `LISTEN_ADDR`. The vm-bundle chart no longer
ships one.

### Upgrade

Rename adapter CRs, or re-type channels and sources, so names and `spec.type`
values match. The shipped `telegram`, `cron` and `vm-alertmanager` names already
do.

`spec.type` on sources and channels is immutable, so a source whose type key
changes — `vmAlertmanagerWebhook` to `vm-alertmanager`, for instance — is deleted
and recreated, then re-claimed by its Pipeline.

## [1.6.0] — 2026-08-07

### Removed

**BREAKING: the manager's built-in Alertmanager endpoint is gone.**
`POST /ingest/alertmanager/{source}` and the built-in `alertmanagerWebhook` type
are removed.

The `signal-vmalertmanager` adapter accepts the identical webhook format and is
the only Alertmanager path.

### Upgrade

1. **Before upgrading**, repoint senders to the adapter Service
   (`/webhook/{source}`) and move sources to the adapter's type key, claimed by a
   Pipeline.

   Sender retries plus fingerprint cooldown make the switchover itself lossless.
2. After upgrading, retire the old `alertmanagerWebhook` sources.

## [1.4.0] — 2026-08-07

manager `0.7`.

### Removed

**BREAKING: wiring exists only on `Pipeline`.**
`SignalSource.spec.channelRef`, `SignalSource.spec.profileRef` and
`Channel.spec.defaultProfileRef` are removed.

### Upgrade

Order matters so alert routing never gaps.

1. **Apply a Pipeline first**, claiming every live source with the intended
   profile and channels. The old manager ignores it. The new one requires it.
2. Upgrade to chart 1.4 and manager 0.7. Unclaimed sources now show
   `Wired=False` and drop signals with an explicit response reason. Bare messages
   on channels outside any pipeline get usage guidance.
3. Re-apply your CR manifests without the removed fields.

## [1.3.0] — 2026-08-07

### Changed

**BREAKING: a conversation binds many channels.**
`Conversation.spec.channelRef` becomes `spec.channelRefs[]`, and
`status.threadId` becomes `status.threads[]` — `{channel, threadId}` per bound
channel.

Upgrading is behaviour-neutral. With no Pipeline CRs, single-channel flows are
unchanged.

Mirroring is opt-in per Pipeline CR. Deleting the Pipeline reverts new
conversations to source-level routing.

### Upgrade

Existing chat-bound conversations lose their binding fields. To keep replying in
their existing topics with session continuity, patch each one:

```sh
kubectl -n <ns> patch conversation <name> --type=merge \
  -p '{"spec":{"channelRefs":[{"name":"<channel>"}]}}'
kubectl -n <ns> patch conversation <name> --subresource=status --type=merge \
  -p '{"status":{"threads":[{"channel":"<channel>","threadId":"<thread>"}]}}'
```

Unmigrated topics still work. Replying triggers re-adoption as a fresh
conversation, without the old session.

## [1.1.0] — 2026-08-07

### Changed

The chart's Telegram adapter Deployment is replaced by a `ChannelAdapter` CR. The
reconciler owns the workload.

The bot token moves to the Channel as `spec.credentialsSecretRef`, Secret key
`botToken`. `TELEGRAM_BOT_TOKEN` env remains a fallback for hand-deployed
adapters only.

### Upgrade

On chart 3.1 and later, `telegramAdapter.*` no longer exists. Telegram moved into
the `telegram-bundle` subchart and the adapter stopped polling. Read these steps
for the shape of the change, but use `telegram-bundle.enabled` and follow the
3.1.0 entry.

For a live install running the chart-1.0 adapter:

1. **Upgrade with the adapter disabled** (`telegramAdapter.enabled=false`, the
   default). Helm removes the old Deployment, the bot token's single getUpdates
   slot is free, and the new CRD and manager are in place.
2. **Move the bot token to the Channel.** Add
   `spec.credentialsSecretRef: {name: <your bot-token secret>}` to each telegram
   Channel.
3. **Enable the adapter** with `--set telegramAdapter.enabled=true`, or apply
   your own `ChannelAdapter` CR. The reconciler deploys the workload with the
   projected credentials as the sole getUpdates consumer.

Rollback: delete the `ChannelAdapter` CR, which GCs the reconciler's Deployment,
then redeploy chart 1.0.x whose template restores the old wiring.

## [1.0.0] — 2026-08-05

### Changed

**BREAKING: the `Channel` CRD is restructured and Telegram moves out of the
manager** into the `channel-telegram` adapter.

`status.threadId` is now a **string**. Existing numeric ids remain valid as
decimal strings. Update anything that parsed it as a number.

### Upgrade

1. **Stop the old manager.** This stops the in-process poller, freeing the bot
   token's single getUpdates slot.

   ```sh
   kubectl -n <ns> scale deploy agentops-manager --replicas=0
   ```
2. **Migrate Channel CRs** from the typed sub-struct to metadata plus config:

   ```yaml
   # before                              # after
   spec:                                 spec:
     telegram:                             type: telegram
       botTokenSecretRef: {…}              defaultProfileRef: {name: …}
       chatId: "-100…"                     config:
       approvers: [1, 2]                     chatId: "-100…"
       pollingEnabled: true                  approvers: [1, 2]
     defaultProfileRef: {name: …}            pollingEnabled: true
   ```

   The bot token secretRef moves out of the CR entirely, since the manager reads
   no Secrets at all any more. It returns as `spec.credentialsSecretRef` on the
   Channel in chart 1.1.0.
3. **Upgrade.** The new CRD applies, the manager restarts without Telegram code,
   and the adapter starts as the sole getUpdates consumer — replicas 1, Recreate.

   ```sh
   helm upgrade … --set telegramAdapter.enabled=true
   ```

   On chart 3.1 and later this flag is `telegram-bundle.enabled`, and the sole
   consumer is the router.

Rollback is the reverse order: disable the adapter, then restore the previous
chart version and Channel CR shape.
