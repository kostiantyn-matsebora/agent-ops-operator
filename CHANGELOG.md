# Changelog

Chart-version migration guides — the upgrade steps for every breaking change,
newest first. See [README.md](README.md) for the product overview and
[docs/](docs/) for reference material.

Entries are keyed by CHART version; the manager image tag moves independently.

## Unreleased

**BREAKING — `POST /task` is removed, and the console is on by default.**
Chart **5.0.0**, manager **0.24.0**, console **0.3.1**.

### BREAKING — a task is a signal; `POST /task` is gone

The endpoint, its handler and its request type are deleted. There is now no HTTP
route that names a `Pipeline`. Programmatic origination is an ordinary signal
posted to a `SignalSource` that a Ready Pipeline claims, so which agent answers,
on which channels, with which capabilities is decided by declared wiring rather
than chosen by the caller — the same rule every other origination already
followed.

**Before:**

```sh
curl -sX POST http://agentops-manager.<ns>.svc:8080/task \
  -H 'Content-Type: application/json' \
  -d '{"pipeline":"k8s-engineer","task":"why is pod X crashlooping?"}'
```

**After:**

```sh
TOKEN=$(kubectl -n <ns> get secret agentops-adapter-token \
  -o jsonpath='{.data.token}' | base64 -d)
curl -sX POST http://agentops-manager.<ns>.svc:8080/signal/inbound \
  -H "Authorization: Bearer $TOKEN" \
  -d '{"source":"cluster-events","signals":[{"fingerprint":"ask-1",
       "kind":"task","payload":"why is pod X crashlooping?"}]}'
```

Three things change in the call: a different URL, a bearer token
(`ADAPTER_TOKEN`, the same one adapters use), and a **source** name where a
pipeline name used to go. Pick a source the Pipeline you want already claims —
`kubectl get pipeline <name> -o jsonpath='{.spec.signalSourceRefs}'`. The
`fingerprint` is yours to choose and is what cooldown dedups on; a build id or a
timestamp is the usual answer.

**No deprecation shim is offered.** A 410 would preserve exactly the doorway
this change closes, and the API group is provisional pre-1.0.

**Dropped without replacement:** the `agent` override (per-call role selection
remains available through the chat form `/<pipeline>:<agent>`) and the `channel`
field, which let a caller add a surface — that is wiring, and wiring is declared
on the Pipeline.

**New signal kind `task`.** Alongside `alert`, `job` and `chat`: task-lane
prompt, no `jobName`, no recurrence-on-session, and — unlike `chat` — no
`agentops.dev/channel` label required, because replies go to the claiming
Pipeline's channels.

**Signature keying now splits on the lane** when a source declares no
`signatureLabels`. `alert` and `job` keep the default
`alertgroup`/`alertname`/`namespace` labels, so alert grouping and cron-tick
folding are unchanged; `task` and `chat` key on the signal's own fingerprint, so
each request opens its own conversation. A source that DOES declare
`signatureLabels` groups by them in every lane — which means **a posted task
inherits the target source's grouping**. Posting tasks to a source with
`signatureLabels` (k8s-bundle's `cluster-events` uses `namespace`/`workload`)
groups tasks that share those label values, including tasks carrying none of
them. Create your own source if you want an isolated ask lane.

No rendered chart object changes, no CRD schema field is added or removed, and
no stored object changes shape.

### What changes on upgrade

`console.enabled` flips from `false` to `true`. Upgrading STARTS A POD that was
not running before — one that reads every `agentops.dev` CR in the namespace,
plus Deployments and Pods, and that can instruct any agent it is joined to. That
is why this is a major bump rather than a feature.

**The one-value opt-out:**

```yaml
console:
  enabled: false
```

With that set, nothing about your install changes.

**One caveat, if you later turn it off again.** Once a Pipeline references the
console — `channelRefs: [console]` or `signalSourceRefs: [console]` — disabling
the console removes those objects and the Pipeline correctly reports
`unresolved references: signalsource/console, channel/console` and stops being
Ready. Remove the references in the same change as the opt-out. `helm upgrade
--wait` will otherwise fail on that Pipeline, which is the system telling you
the truth rather than a bug.

### If you keep the console on

The new pod is a `ChannelAdapter` **and** a `SignalAdapter`, and it is still ONE
Deployment: the SignalAdapter declares `servedBy` the ChannelAdapter, so it owns
no workload and simply receives a second token in the same pod.

- **What it reads.** Its Role gains `apps/deployments` and `pods`
  (get/list/watch, namespaced, read-only) on top of the `agentops.dev` kinds.
  Image digests, restart counts and pod failure reasons exist in no CR, and an
  operations console that cannot see a CrashLoopBackOff is not one. There are
  still no write verbs anywhere in it.
- **What it can do.** `console.write.enabled` defaults to `true`, so the chat
  composer and "new conversation" are live. Set it to `false` for a strict
  viewer — the affordances disappear AND both endpoints refuse.
- **Origination is refused until you wire it.** The chart renders a
  `SignalSource` named `console` and NO Pipeline. Until some Ready Pipeline
  claims that source it sits at `Wired=False`, and the UI shows that reason with
  the patch:

  ```sh
  kubectl patch pipeline <name> --type=json \
    -p '[{"op":"add","path":"/spec/signalSourceRefs/-","value":{"name":"console"}}]'
  ```

- **Exposure.** Still `ClusterIP` with `console.ingress.enabled: false`. If you
  expose it, put an authenticating proxy in front — the console reads the
  identity from forwarded headers and logs every write with it; without one every
  write is recorded as `token`.

### New in the manager (additive, no action required)

- **Per-hop activity telemetry.** `GET /activity`, `GET /activity/stream` (SSE)
  and `POST /activity`, under the existing adapter bearer scheme. A bounded
  in-memory ring (`ACTIVITY_BUFFER`, default 10000), never persisted; the durable
  record stays `status.runs[]`.
- **Introspection.** `GET /status` (runtime slots, op queue depth with the oldest
  stuck item's identity, cooldowns, leader) and
  `GET /pipelines/{name}/resolved` (the authoritative capability resolution).
- **Prometheus metrics** on the existing `:9090` — `agentops_*` counters, gauges
  and histograms, emitted from the same call sites as the activity events. No new
  listener. Optional scrape templates under `metrics:` (VMServiceScrape,
  ServiceMonitor, example alert rules), all default-disabled because neither CRD
  is guaranteed present.
- **`SignalAdapter.spec.servedBy`.** `spec.image` becomes optional; an adapter
  declaring `servedBy` owns no workload and reports `Ready=True/ServedBy`. Purely
  additive — existing adapters are unaffected.

### Optional

`console.metrics.url` points the console at a Prometheus/VictoriaMetrics query
endpoint, which lets it render windows far beyond the activity buffer as clearly
labelled aggregates. Unset, every view still works and long windows are reported
unavailable rather than drawn empty.

## 4.0.0

**BREAKING — the main chart owns the agent runtime.** Chart **4.0.0**. No image
changes; this is a values and object-ownership move.

*Why this exists:* `AgentRuntime` was created in exactly one place — the
`k8s-bundle` subchart — and nothing about it is Kubernetes-shaped. Image,
credential, idle TTL, node placement and home volume describe *how an agent
executes*, which is the same for a VictoriaMetrics lane, a chat-only install or
a Kubernetes one. The placement was never a decision: chart 2.x relocated the
parent's `demo.*` block wholesale and the runtime rode along, because demo mode
and the bundle became the same thing. The interest paid since: an install with
only `telegram-bundle` rendered nothing that could execute a conversation, two
runtime ServiceAccounts existed (the parent's, granted nothing, and the
bundle's, granted everything), `homePvcRef` was a documented copy of a claim the
parent creates, and idle TTL had two defaults that disagreed (1 vs 10).

**Two upgrade-visible effects that are not value renames — read these first**

1. **The runtime ServiceAccount is renamed.** `agentops-runtime-k8s` →
   `agentops-runtime` (the one the parent already created and the manager
   already defaults runtime pods onto). Helm replaces the bundle-named bindings
   with global-named ones in the same upgrade. **If you bound your own
   (Cluster)Roles to `agentops-runtime-k8s`, re-point them** — nothing else
   will. Afterwards, confirm the old binding is gone:

   ```sh
   kubectl get clusterrolebinding | grep agentops-runtime-k8s   # expect nothing
   ```

2. **An install that enabled `k8s-bundle` without touching MCP now gets an MCP
   server workload.** `mcp.enabled` and `mcpServers.enabled` both default to
   `true` and flip as a pair — the config's URL needs a Service to default onto,
   which is the only reason the component used to be off. Hold position:
   `k8s-bundle.mcpServers.enabled: false` (and `mcp.enabled: false`, or an
   `mcp.url` of your own).

**Values migration**

| 3.x | 4.0 |
|---|---|
| `serviceAccounts.runtime` | `global.agentops.runtime.serviceAccountName` (setting the old key now FAILS the render) |
| `k8s-bundle.profile.runtime.image` | `runtime.image` |
| `k8s-bundle.profile.runtime.credentialsSecret.*` | `runtime.credentialsSecret.*` |
| `k8s-bundle.profile.runtime.nodeSelector` | `runtime.nodeSelector` |
| `k8s-bundle.profile.runtime.resources` | `runtime.resources` |
| `k8s-bundle.profile.runtime.idleTtlMinutes` | `runtime.idleTtlMinutes` (empty ⇒ follows `runtimeIdleTtlMinutes`) |
| `k8s-bundle.profile.runtime.homePvcRef` | *(automatic from `persistence`)* |
| `k8s-bundle.profile.runtime.name` | `runtime.name` |
| `k8s-bundle.profile.runtime.create` | `runtime.enabled` |
| `k8s-bundle.profile.runtime.serviceAccountName` | `global.agentops.runtime.serviceAccountName` |
| `k8s-bundle.rbac.mode` | `global.agentops.runtime.rbacMode` |
| `k8s-bundle.rbac.enabled: false` | `global.agentops.runtime.rbacMode: none` |
| `k8s-bundle.mcpServers.readOnly` | *(derived from `rbacMode`; still settable)* |
| `k8s-bundle.mcpServers.rbac.mode` | *(derived from `rbacMode`; still settable)* |

Moved a value into `k8s-bundle.profile.runtime.*` during the 1.x → 2.x hop? That
table is still below, under chart 2.0.0 — this one names where it went next.

**Worth knowing**

- **A default install now renders an `AgentRuntime`.** `runtime.enabled` is
  `true`, so a chart with no bundle — or with only `telegram-bundle` — can
  execute a conversation. If you manage `AgentRuntime` CRs yourself, set
  `runtime.enabled: false`; a name collision on `default` makes Helm adopt or
  conflict loudly rather than silently change behavior.
- **`rbacMode` is one knob, and empty grants nothing.** `none` | `readonly` |
  `full`, defaulting to empty, which resolves to `readonly` under
  `global.demo.enabled` and to `none` otherwise — so a release that never set an
  RBAC value holds no bindings after upgrade, exactly as before. `full` is never
  selected by a default or inferred path.
  `rbac.runtime.{clusterRoles,bindClusterRoles,namespaced}` stay additive.
- **The MCP server's posture derives from that knob.** `full` yields a
  write-capable server under a `full` SA and renders `k8s-admin`; every other
  mode yields a read-only server under a `readonly` SA. It derives because a
  read-only server under a `full` agent pushes every write back onto kubectl —
  the single-wall path the component exists to replace. The cost is real:
  widening the agent widens the server unless you set
  `k8s-bundle.mcpServers.readOnly: true`, which recovers "kubectl writes, MCP
  reads only". See
  [docs/k8s-bundle.md](docs/k8s-bundle.md#kubernetes-as-mcp-tools-mcp--mcpservers).
- **Idle TTL stops being configured twice, and drops from 10 to 1.** The bundle
  defaulted `idleTtlMinutes: 10` while the manager's `runtimeIdleTtlMinutes` was
  `1`; now an empty `runtime.idleTtlMinutes` follows the release value, so a
  finished conversation gives its slot back in a minute rather than ten. Set
  `runtime.idleTtlMinutes: 10` to keep the old behavior for this runtime, or
  raise `runtimeIdleTtlMinutes` for the release. The chart WRITES the field
  rather than omitting it: `AgentRuntime.spec.idleTtlMinutes` has a CRD default
  of `10`, so an omitted field is stored as `10` and the manager prefers any
  non-zero spec value over its own setting — omitting it renders a correct-
  looking manifest and a wrong stored object.
- **The bundle keeps its identity, not its substrate.** `profile.enabled` now
  renders exactly one object, the `AgentProfile`; `profile.runtimeRef` still
  points at a runtime other than `default`. `eventsAdapter.rbac` is a different
  block and is untouched.
- **Rollback = chart 3.4.0.** The CRDs are unchanged, so a downgrade re-renders
  the bundle-owned objects; the only manual step is deleting the parent-owned
  `AgentRuntime` if its name differs from the bundle's.

**Conversations are capped, queued, and can be closed.** Chart 3.4.0, manager
0.21.0, `channel-telegram` 0.6.1, `console` 0.2.0 — ship them together: an
adapter that does not know the new `close-topic` op leaves topics open.

*Why this exists:* nothing bounded how many conversations ran at once in terms
an operator recognises. The only cap was `MAX_RUNTIMES`, named after pods, and
an idle runtime held its slot for ten minutes after the agent had stopped
working. Conversations also never ended — no way to say "this one is done", so
threads accumulated and capacity came back only by eviction.

**Upgrade steps**

1. **Two defaults change on upgrade, both visible.**
   - `maxRuntimes: 8` → **`maxActiveConversations: 5`**. Less throughput, more
     queueing. Nothing is dropped, only delayed — raise the new key to restore
     the old figure. `maxRuntimes` still works for ONE release (unset by
     default; setting it emits `MAX_RUNTIMES` and the manager logs the
     deprecation), then it is removed.
   - `runtimeIdleTtlMinutes: 10` → **`1`**. A finished conversation gives its
     slot back within a minute instead of ten, which is what makes a cap of 5
     workable. The trade is latency, not memory: sessions live in `/data/home`
     and resume with full context. Raise it — or set
     `AgentRuntime.spec.idleTTLMinutes` — for runtimes with expensive startup.
2. **Third-party channel adapters should handle `close-topic`.** It carries the
   `threadId` to archive and completes with an EMPTY body. An adapter that
   ignores it is not broken: the op fails its 2-minute grace and deletion
   proceeds, leaving one open thread per closed conversation. See
   [docs/contracts.md](docs/contracts.md#the-channel-adapter-contract).
3. **Nothing to migrate for existing objects.** `Pending` is an additive phase
   value; no old manager writes it, and rolling back is a chart rollback —
   phase is status-only and nothing keys behavior off it.

**Worth knowing**

- **Over-cap work waits in `Pending` with NOTHING provisioned** — no runtime
  pod, no MCP ConfigMap and no chat topic. That last one is the point: a burst
  of signals no longer becomes a burst of chat threads. Admission is FIFO by
  creation time. `Queued` keeps its old meaning (admitted, waiting its turn).
- **The backlog is bounded too**, at `maxQueuedConversations: 50`. Past it
  `/signal/inbound` declines to create a conversation and reports the batch
  dropped for capacity — chat senders are told on the surface they typed on.
  Window reuse is unaffected: the bound gates new objects, not new inputs.
- **`/close` ends a conversation** from its thread — any sender who can post
  there, honored mid-run (the farewell names the abandoned work). It deletes
  the `Conversation`; the pod and `agentops-mcp-conv-<name>` follow by owner
  reference. `kubectl delete conversation` behaves identically, archiving
  threads first. On a general surface `/close` answers with usage, so a
  Pipeline named `close` is no longer addressable from chat.

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

If the profile should be reachable, give it a Pipeline — that is what routes to
it. (At chart 3.0 a task named that Pipeline via `POST /task`; the endpoint is
removed as of chart 5.0 and a task now names a source instead — see the entry at
the top.)
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
