# Changelog

Chart-version migration guides — the upgrade steps for every breaking change,
newest first. See [README.md](README.md) for the product overview and
[docs/](docs/) for reference material.

Entries are keyed by CHART version; the manager image tag moves independently.

## Unreleased

**Cluster events get suppression, workload grouping, and a loop breaker.**
`signal-k8s-events` 0.2.0. Two breaking-ish defaults and one new RBAC grant.

*Why this exists:* the events lane created **hundreds of conversations** on a
healthy cluster, and on an unhealthy one it fed itself — a runtime pod that
could not start emitted a Warning event, which became a signal, which opened a
Conversation, which created another runtime pod under a new name, forever.

**Upgrade steps**

1. **The adapter needs new permissions.** The events component now grants
   read-only `pods` and `replicasets` (`list`/`watch`) alongside `events`. If
   you render the bundle's RBAC (`eventsAdapter.rbac.create: true`, the
   default) this happens for you. If you bind that RBAC yourself, add it — the
   adapter reports `Ready=False` naming the missing permission rather than
   degrading silently.
2. **`grouping.signatureLabels` defaults to `[namespace, workload]`**
   (was `[namespace, kind, name]`). **BREAKING for existing conversations:**
   they keep their old per-pod signature hash and go orphaned. No action is
   needed — they age out of the 7-day reuse window on their own, and new
   conversations group per workload. Override the values path if you need the
   old behavior back.
3. **Default `rules` now ship.** A default install suppresses rollout churn out
   of the box. If you had built your own `excludeReasons` list it still works
   (it translates into leading drop rules), but review it against the new
   defaults — you probably need less of it.

**Worth knowing**

- Reason matching is now **anchored**: `excludeReasons: [Failed]` no longer
  also drops `FailedMount`. If you were relying on the accidental prefix match,
  widen it to a rule with an explicit regex.
- The self-exclusion invariant is **not configurable** for its first two
  mechanisms (name prefix, owner/label). `source.includeOwnNamespace: true`
  relaxes only the coarse namespace rule, for installs that co-locate their own
  workloads with the operator.

**Stopgap for anyone still on 0.1.x:** the conversation explosion is a values
edit away from stopping, with no new image —

```yaml
k8s-bundle:
  eventsAdapter:
    source:
      excludeReasons: [Unhealthy, FailedScheduling, SandboxChanged, Preempting]
      grouping:
        signatureLabels: [namespace, alertname]
```

This bounds conversations by namespaces × reasons instead of pods × rollouts.
Agent *runs* do not drop (cooldown is still per object+reason), and it does not
break the self-reference loop — only 0.2.0 does that.

**The agent-ops console — additive, opt-in, no upgrade steps.** A browser view
of the whole install (CR inventory, wiring graph, live conversation runs) that
is also a channel: conversations on pipelines listing its Channel bind a
console thread, so you can reply to an agent from the run you are watching.
See [docs/console.md](docs/console.md).

- New `console/` module and image `agentops-console`.
- `ChannelAdapter.spec` gains `kubernetesAccess` (parity with SignalAdapter):
  mounts the SA token and injects `POD_NAMESPACE`. **Identity only** — the
  operator still creates and binds no RBAC, so an adapter CR cannot escalate.
  Existing adapters are unaffected (optional, default off). `spec.port` already
  existed and is unchanged; its Service is named after the workload,
  `agentops-adapter-<name>`.
- Enable with `console.enabled=true`. It renders **CRs and RBAC only** — a
  ChannelAdapter, a Channel, the UI token Secret, and a namespaced read-only
  Role for SA `agentops-adapter-console`; the reconciler brings up the
  Deployment and Service.
- Nothing is wired by enabling it: conversations show as *observed* until you
  add the console Channel to a Pipeline's `channelRefs[]`. The chart never
  edits your Pipelines.
- **Trust boundary**: anyone holding the UI token sees every agentops CR in the
  namespace, conversation payloads included. Keep the Service ClusterIP unless
  you mean to expose it.
- Manager image `0.20.0` — required for `kubernetesAccess` on ChannelAdapter.
  An older manager silently ignores the field: the CR accepts it, the pod comes
  up without a token, and the console crash-loops on the missing CA file.

**The k8s-bundle can now create the agent's credential Secret.** Additive and
default-off. `k8s-bundle.profile.runtime.credentialsSecret.token`, when set,
renders the Secret the `AgentRuntime` already referenced
(`agentops-claude`/`oauthToken`) instead of leaving it as an unstated
prerequisite — so the credential survives a teardown with the release, the same
way `telegram-bundle` handles its bot token. Leaving it empty keeps the old
behavior exactly.

Worth knowing because the old failure mode is silent: the reference is resolved
by the kubelet, not the manager, so a missing Secret produces runtime pods in
`CreateContainerConfigError` and conversations that queue forever, with no
condition anywhere saying why. The post-install notes now name it.

## chart 3.2 — toolsets compose with the agent definition — BREAKING

**The runtime no longer invents a tool.** `runtime-claude` used to pass
`--allowedTools Read` whenever a work unit carried no allowlist — a grant
nobody declared. From image `0.2.0` it passes exactly what was composed, empty
included, and runs with `--permission-mode dontAsk` so an unlisted tool is
denied rather than prompted for (a prompt in a pod hangs until the idle TTL).

**Who is affected:** any conversation whose route binds no `toolsets` *and*
whose agent definition declares no `tools:`. It used to get `Read`; now it gets
nothing, starts, finds it can do nothing, and says so. That is the point — but
check before upgrading:

```sh
# Pipelines that grant no tools; their routes now depend entirely on the
# agent definition's own `tools:` frontmatter.
kubectl get pipelines -A -o json | jq -r \
  '.items[] | select(.spec.toolsets == null)
   | "\(.metadata.namespace)/\(.metadata.name) -> profile \(.spec.profileRef.name)"'
```

Fix either side: bind a toolset on the Pipeline, or declare `tools:` in the
repo's `.claude/agents/<agent>.md`. Both work, and under the default `merge`
they add up.

**`toolsets.mode` is back, with a different counterpart.** It composes against
the **agent definition's** `tools:` frontmatter, never against the profile
(profiles carry no capabilities — that misreading is why the field was removed
in 3.0). `merge` is the default and additive; `overwrite` passes the route's
tools alone. `mcpConfigs` has no mode. Existing Pipelines need no edit: the CRD
defaults `mode: merge`, which reproduces today's behavior wherever the agent
definition declares nothing.

The work unit gained `toolsMode` and `agent` alongside `allowedTools`. Custom
runtimes that ignore them keep working — `allowedTools` alone is what `merge`
degrades to when nothing is declared repo-side.

Steps:

1. Upgrade the operator (manager `0.18.0`) — additive, existing Pipelines
   default to `merge`.
2. Update the runtime image to `agentops-runtime-claude:0.2.0`
   (`k8s-bundle.profile.runtime.image`, or the `AgentRuntime` CRs you manage).
   Staying on `0.1.1` keeps the `Read` substitution and ignores the mode.
3. Run the `jq` check above and grant what each route actually needs.

## chart 3.1 — chat originates as a signal — BREAKING

Two breaking changes land together.

**1. `/channel/inbound` is reply-only.** `threadId` is now REQUIRED and an
unknown thread is no longer adopted. Third-party adapters that posted bare
messages to originate a conversation get a `400` naming the signal path.
`PipelineForChannel` — and with it the "oldest Ready pipeline referencing this
channel answers" tiebreak — is gone, as is any channel default profile.

**2. Telegram ingest is three components.** Telegram serves one update stream
per bot token, so origination and continuation cannot each poll:

```
getUpdates ─▶ telegram-router ─┬─ no topic ─▶ signal-telegram  ─▶ /signal/inbound
              (the only poller) └─ topic    ─▶ channel-telegram ─▶ /channel/inbound
```

`channel-telegram` no longer polls; `Channel.spec.config.pollingEnabled` is
removed. Ingest is on when the router runs.

Steps — **step 3 is the one where ordering matters**:

1. **Upgrade the operator first.** `ChannelAdapter.spec.port` and the
   reconciler's Service parity are inert until used.
2. **Add the chat `SignalSource` and claim it**, plus the router's
   credential-carrying source. Nothing changes behaviorally yet — the old
   adapter is still polling.

   ```yaml
   apiVersion: agentops.dev/v1alpha1
   kind: SignalSource
   metadata: {name: home-ops-chat, namespace: agent-ops}
   spec:
     adapter: telegram          # signal-telegram; no credentials needed
     config:
       chatId: "-1004369687194"
       channel: home-ops        # the Channel this chat is the general surface of
     grouping: {cooldownHours: 0}
   ```

   Then add `home-ops-chat` (and `telegram-router`) to your Pipeline's
   `signalSourceRefs`. **A chat source no Pipeline claims answers nobody** —
   the user is told so on the surface, but nothing runs.
3. **Scale the old adapter to zero and CONFIRM no `getUpdates` consumer
   remains** before enabling the new stack. Two consumers of one token means
   409s and stolen updates — this is the failure that costs the most debugging.

   ```sh
   kubectl scale deploy/agentops-adapter-telegram -n agent-ops --replicas=0
   kubectl get pods -n agent-ops | grep telegram   # expect none
   ```
4. **Carry the offset across.** The cursor lives in the
   `agentops.dev/adapter-state-telegram-offset` annotation on the Channel the
   old adapter wrote to. The new stack reads it from the FIRST served channel
   by name, so if your old leader differs, copy the value over:

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
       chatId: "-1004369687194"
       credentials:
         existingSecret: agentops-telegram   # key: botToken
   ```

   `enabled` alone ships the three IMPLEMENTATIONS and wires nothing — the
   right shape if you manage Channels yourself. `surface.enabled` also ships
   the Channel and the chat `SignalSource`, and then requires everything it
   cannot guess. The router's credential is the SAME one the Channel uses — it
   polls the bot the channel sends as.

6. **Claim the sources in a Pipeline.** Your existing Pipeline keeps its
   channels; add the new sources to its `signalSourceRefs`:

   ```yaml
   signalSourceRefs:
     - name: telegram-ops-chat   # the chat surface
     - name: telegram-router     # emits nothing; claimed so it is not Wired=False
   ```

   Until this lands, chat messages drop with `Wired=False` and the user is told
   so. The post-install notes print this block with your names filled in.

Rollback is steps 3 and 5 together: re-enable the old single-container adapter
AND revert the operator, since the origination paths must come back with it.
Conversations already created are unaffected either way.

## chart 2.x → 3.0 — BREAKING

`AgentProfile.spec.allowedTools` and `spec.mcp` are removed. **Removing a CRD
field prunes that data on the next write**, so audit before upgrading, not
after:

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
| `mcp.configMapRef` / `secretRef` | an `MCPConfig` with the same field (bound alone) |

If the profile should be reachable via `POST /task`, give it a Pipeline to
address — that is what a task names.
`ToolingBinding.mode` is gone; drop `mode:` from any existing stanza.


## chart 2.x — demo values move into k8s-bundle — BREAKING

The demo block moved wholesale into the bundle. A subchart can read no parent
scope except `global.`, which is why the toggle moved there.

| Chart 1.x | Chart 2.x |
|---|---|
| `demo.enabled` | `global.demo.enabled` |
| `demo.runtimeImage` | `k8s-bundle.profile.runtime.image` |
| `demo.credentialsSecret.*` | `k8s-bundle.profile.runtime.credentialsSecret.*` |
| `demo.readOnlyRbac: true` | `k8s-bundle.rbac.mode: readonly` (the default) |
| `demo.readOnlyRbac: false` | `k8s-bundle.rbac.enabled: false` |
| *(inherited `persistence`)* | `k8s-bundle.profile.runtime.homePvcRef` — **set this explicitly**, subcharts cannot see the parent's `persistence` block |
| *(inherited `runtimeIdleTtlMinutes`)* | `k8s-bundle.profile.runtime.idleTtlMinutes` |

The runtime ServiceAccount is renamed `agentops-runtime-demo` →
`agentops-runtime-k8s`; `helm upgrade` removes the old objects with the deleted
`demo.yaml` template. The `AgentRuntime` named `default` re-renders with
identical semantics, so existing conversations keep resolving their runtime.


## chart 1.12 — `spec.type` → `spec.adapter` — BREAKING

`Channel.spec.type` and `SignalSource.spec.type` are now `spec.adapter`: the
value was always a reference to the serving adapter CR, and the old name read
as an intrinsic attribute, making the sibling `config` look like part of one
flat schema. The contract follows — `?type=` becomes `?adapter=` on
`/channel/ops`, `/channel/channels` and `/signal/sources` (the retired
parameter returns 400 naming its replacement rather than an empty list), and
`CHANNEL_TYPE`/`SOURCE_TYPE` collapse into `ADAPTER_NAME`.

`adapter` is immutable, so live Channels and SignalSources are delete-and-
recreate. **Carry their annotations across**: adapter cursor state (the
Telegram `getUpdates` offset, cron last-fire) lives in
`agentops.dev/adapter-state-*` annotations on those objects, and a bare
recreate makes the adapter re-read old updates. Manager and adapter images
must be upgraded together, since both sides of the contract change at once.

## chart 1.8 — adapters can configure their senders

Additive. `SignalAdapter.spec.kubernetesAccess` (default false) mounts the
adapter's ServiceAccount token and injects `POD_NAMESPACE` — **the operator
still grants adapters no RBAC whatsoever**; permissions come from the chart
or you, bound against the deterministic SA `agentops-signal-<name>`. The
vm-bundle uses it for `alertmanager.registration`, which replaces the manual
VMAlertmanager repoint. Existing adapters are untouched.

## chart 1.7 — adapters are pure implementation — BREAKING

`ChannelAdapter`/`SignalAdapter` lose `spec.type` and `spec.env` — the CR
**name** is now the type key (`Channel`/`SignalSource.spec.type` names the
serving adapter), and adapter CRs carry no configuration at all.
`SignalAdapter` gains `spec.port`: when set, the reconciler owns the Service
`agentops-signal-<name>` and injects `LISTEN_ADDR` (the vm-bundle chart no
longer ships one). To upgrade: rename adapter CRs (or re-type
channels/sources) so names and `spec.type` values match — the shipped
`telegram`, `cron`, and `vm-alertmanager` names already do; `spec.type` on
sources/channels is immutable, so a source whose type key changes (e.g.
`vmAlertmanagerWebhook` → `vm-alertmanager`) is deleted and recreated, then
re-claimed by its Pipeline.

## chart 1.6 — built-in alertmanager removed — BREAKING

The manager's `POST /ingest/alertmanager/{source}` endpoint and the built-in
`alertmanagerWebhook` type are gone — the `signal-vmalertmanager` adapter
(vm-bundle) accepts the identical webhook format and is the only Alertmanager
path. BEFORE upgrading: repoint senders to the adapter Service
(`/webhook/{source}`, see the VM bundle section) and move sources to the
adapter's type key claimed by a Pipeline; sender retries plus
fingerprint cooldown make the switchover itself lossless. After upgrading,
retire the old `alertmanagerWebhook` sources.

## chart 1.4 — pipeline-only wiring — BREAKING

`SignalSource.spec.channelRef`/`profileRef` and `Channel.spec.defaultProfileRef`
are removed — wiring exists only on `Pipeline`. Upgrade sequence (order
matters so alert routing never gaps):

1. **Apply a Pipeline first**, claiming every live source with the intended
   profile and channels (the old manager ignores it; the new one requires it).
2. Upgrade to chart 1.4 / manager 0.7. Unclaimed sources now show
   `Wired=False` and drop signals with an explicit response reason; bare
   messages on channels outside any pipeline get usage guidance.
3. Re-apply your CR manifests without the removed fields.

## chart 1.3 — Pipelines, multi-channel conversations — BREAKING

`Conversation.spec.channelRef`/`status.threadId` became `spec.channelRefs[]` /
`status.threads[]` (`{channel, threadId}` per bound channel). Upgrading is
behavior-neutral (no Pipeline CRs = single-channel flows unchanged), but
existing chat-bound conversations lose their binding fields; to keep replying
in their existing topics with session continuity, patch each one:

```sh
kubectl -n <ns> patch conversation <name> --type=merge \
  -p '{"spec":{"channelRefs":[{"name":"<channel>"}]}}'
kubectl -n <ns> patch conversation <name> --subresource=status --type=merge \
  -p '{"status":{"threads":[{"channel":"<channel>","threadId":"<thread>"}]}}'
```

(Unmigrated topics still work — replying triggers re-adoption as a fresh
conversation without the old session.) Mirroring is opt-in per Pipeline CR;
deleting the Pipeline reverts new conversations to source-level routing.

## chart 1.1 — ChannelAdapter CR

Chart 1.1 replaces the chart's Telegram adapter Deployment with a
`ChannelAdapter` CR — the reconciler owns the workload. For a live install
running the chart-1.0 adapter:

> **On chart 3.1+, `telegramAdapter.*` no longer exists** — Telegram moved into
> the `telegram-bundle` subchart and the adapter stopped polling. Read the
> steps below for the shape of the change, but use `telegram-bundle.enabled`
> and follow *Migrating to chart 3.1* above.

1. **Upgrade with the adapter disabled** (`telegramAdapter.enabled=false`, the
   default): helm removes the old Deployment — the bot token's single
   getUpdates slot is free, and the new CRD + manager (with the ChannelAdapter
   reconciler) are in place.
2. **Move the bot token to the Channel**: add
   `spec.credentialsSecretRef: {name: <your bot-token secret>}` (Secret key
   `botToken`) to each telegram Channel. `TELEGRAM_BOT_TOKEN` env remains a
   fallback for hand-deployed adapters only.
3. **Enable the adapter** (`--set telegramAdapter.enabled=true`, or apply your
   own `ChannelAdapter` CR): the reconciler deploys the workload with the
   projected credentials as the sole getUpdates consumer.

Rollback: delete the `ChannelAdapter` CR (the reconciler's Deployment is
GC'd), then redeploy chart 1.0.x whose template restores the old wiring.

## chart 1.0 — extensible channels — BREAKING

Chart 1.0 restructures the `Channel` CRD and moves Telegram out of the manager
into the `channel-telegram` adapter. For a live install:

1. **Stop the old manager** (`kubectl -n <ns> scale deploy agentops-manager
   --replicas=0`) — this stops the in-process poller, freeing the bot token's
   single getUpdates slot.
2. **Migrate Channel CRs** from the typed sub-struct to metadata + config:

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

   The bot token secretRef moves out of the CR entirely (the manager reads no
   Secrets at all anymore) — since chart 1.1 it returns as
   `spec.credentialsSecretRef` on the Channel (see below).
3. **Upgrade**: `helm upgrade … --set telegramAdapter.enabled=true`. The new
   CRD applies, the manager restarts without Telegram code, the adapter starts
   as the sole getUpdates consumer (replicas 1, Recreate). *(On chart 3.1+ this
   flag is `telegram-bundle.enabled`, and the sole consumer is the router — see
   Migrating to chart 3.1.)*
4. `status.threadId` is now a **string** (existing numeric ids remain valid as
   decimal strings) — update anything that parsed it as a number.

Rollback = reverse order: disable the adapter, restore the previous chart
version and Channel CR shape.
