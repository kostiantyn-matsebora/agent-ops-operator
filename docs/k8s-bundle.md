# Kubernetes bundle (subchart)

The Kubernetes subchart: cluster Events, the agent that answers them, and its MCP tooling.


`chart/charts/k8s-bundle/` packages the whole "watch my cluster and let an agent
act on what it sees" experience as four independently toggleable components.
Off by default; `global.demo.enabled=true` and `k8s-bundle.enabled=true` are
equivalent ways to turn it on — except for the wiring component, which demo mode
turns on and `enabled: true` does not.

| Component | Flag | What it renders |
|---|---|---|
| Events lane | `eventsAdapter.enabled` | The `SignalAdapter` (`k8s-events`, `kubernetesAccess: true`), RBAC bound to its ServiceAccount (`events get/list/watch` plus read-only `pods`/`replicasets`), and — under `source.create` — a `SignalSource`. **Not the claim on it**: that is the wiring component, or your own `pipelines:` |
| Profile | `profile.enabled` | Exactly one object: the `k8s-engineer` `AgentProfile` (identity only, with an inline `systemPrompt` role) |
| MCP tooling | `mcp.enabled` (**on**) | An `MCPConfig` (`k8s-api`, server key `kubernetes`) and an `MCPToolset` (`k8s-observability`), bound by the wiring component or by whichever Pipeline you declare — see below |
| MCP server | `mcpServers.enabled` (**on**) | The MCP server workload itself: `Deployment` + `Service` (`agentops-mcp-k8s`), **its own `ServiceAccount`**, and that SA's RBAC |
| Wiring | `pipelines.enabled` (**off**, on under demo) | One `Pipeline` claiming the source above with the profile above and both toolsets — see [The bundle's own wiring](#the-bundles-own-wiring) |

**The bundle ships no substrate.** There is no `AgentRuntime`, no runtime
ServiceAccount, no LLM credential Secret and no runtime RBAC here: those are
release-wide facts and live in the parent chart's `runtime:` block and
`global.agentops.runtime.*`
([concepts](concepts.md#the-substrate-runtime-and-globalagentopsruntime)),
including the credential the notes warn about when it is missing. The profile
executes on the parent's `AgentRuntime` named `default`; `profile.runtimeRef`
points it at a different one you applied yourself.

Two things worth knowing:

- **The source is inert until something claims it.** Wiring is pipeline-only, so
  a `SignalSource` nobody claims reports `Wired=False` and drops every event.
  Either turn the wiring component on (demo mode does) or declare the claim
  under the parent chart's `pipelines:` — the latter is what you want when one
  agent should answer this source *and* a chat surface.
- **`global.agentops.runtime.rbacMode: full` is cluster-admin.** It binds
  unrestricted cluster control to an LLM-driven agent, and — because the MCP
  server and the bundle's own route both derive from it — hands the same power to
  the second identity and picks the mutating route too. It is never a default and
  never what demo mode selects. `readonly` plus targeted grants under the parent
  chart's `rbac.runtime` block is almost always the better answer.
- **Withholding shell is per-route**: bind
  `toolsets: {refs: [{name: agentops-observe}]}` on one Pipeline and only that
  route loses `Bash`, while every other route sharing the profile keeps it.

## The bundle's own wiring

Bundles normally ship no `Pipeline`: a route names a profile, sources and
channels that routinely come from *different* components, and a subchart sees
only itself, so one that shipped wiring could only ever wire itself.
`telegram-bundle` therefore ships none.

This bundle is the exception, because it owns its whole lane — it renders the
source, the profile, the `MCPConfig` and both toolsets. The only class of object
a route needs and this bundle cannot see is a channel, and that is supplied by
name and omitted when unset.

It is still **off unless you ask**, with one exception: `global.demo.enabled`
turns it on, because "one flag, a working install" is the whole promise and an
install that answers nothing until you hand-write a CR does not keep it.

```sh
# demo: the route comes with it
--set global.demo.enabled=true
# demo without the route — the parts, none of the spend
--set global.demo.enabled=true --set k8s-bundle.pipelines.enabled=false
# a non-demo install that wants the bundle to wire itself anyway
--set k8s-bundle.enabled=true --set k8s-bundle.pipelines.enabled=true
```

`pipelines.enabled` is `null` by default, meaning "off, except under demo". An
explicit boolean is absolute in **both** directions — which is why `false` is a
working opt-out under demo mode, and the one in the CHANGELOG.

### Which route renders

Two routes, differing in exactly one binding, selected by the same posture value
everything else in this bundle already follows:

| `global.agentops.runtime.rbacMode` | route | binds | what it can do |
|---|---|---|---|
| `readonly`, `none`, `""` (incl. demo) | `k8s-observe` | `agentops-observe`, `k8s-observability`, `k8s-api` | reads the cluster, changes nothing |
| `full` | `k8s-operate` | the same **plus `k8s-admin`** | mutates the cluster |

`pipelines.observe.enabled` / `pipelines.admin.enabled` are explicit booleans
that win in both directions. The derivation exists for the same reason
`mcpServers.readOnly` derives: an operator who sets `full`, gets a write-capable
server and a rendered `k8s-admin` toolset, and a route binding neither, has been
granted cluster-admin they cannot use.

The CR is named `k8s-operate` rather than `k8s-admin` because `k8s-admin` is
already the `MCPToolset` it binds, and "bind `k8s-admin` on `k8s-admin`" is a
sentence nobody should have to parse.

**Both at once is allowed, and it fans out.** Sources are shareable — there is
no conflict condition and no tiebreak — so two Ready Pipelines claiming
`cluster-events` open **two conversations per admitted event**, under two
profiles' worth of capability, and both agents act. Two bills, two blast radii,
one event. The same applies when the bundle's wiring and a Pipeline you declared
in the parent's `pipelines:` both claim the source; the post-install notes name
the pipelines involved rather than failing the render, because refusing it would
be the deleted `sourceConflicts` guard returning one layer up.

### The console is claimed by default

Where the route renders, it also claims the console's signal source and binds the
console as a channel. That is what makes a turnkey install usable from the
browser: without the claim the console's source sits `Wired=False`, its composer
reports itself unavailable, and no answer reaches it.

Both names come from `global.agentops.console` — a subchart reads no other parent
scope, and Helm cannot derive one value from another. The names therefore
duplicate `console.signalSourceName` / `console.channelName`, and the parent
**fails the render** when the two disagree, rather than leaving a route claiming
a source nothing rendered. Turning the console off under demo mode means
clearing both, and the failure says so.

The claim rides the route the bundle already ships. A second route claiming the
console source would make every unaddressed console message ambiguous, since a
bare chat message with more than one claimant is refused.

### Channels, and what happens without one

`pipelines.channels` is a list of names the operator supplies, merged with the
console rather than replacing it. With no console and none named, the
`channelRefs` key is **absent** — not empty-valued. Empty is a working
configuration: with no thread to wait for, the conversation dispatches
immediately and the answer is read from the CR.

```sh
kubectl get conversations -o jsonpath='{.items[*].status.runs[*].result}'
# or bind a surface
--set k8s-bundle.pipelines.channels={console}
```

Every other reference the route makes is to an object this bundle rendered, and
each one disappears with its component: `mcp.enabled=false` yields a route with
built-in observation tools and no cluster reach — degraded and honest, not a
render failure — and never a ref to an `MCPConfig` nobody created.

## Event suppression (`eventsAdapter.source.rules`)

A Kubernetes Event is a point-in-time **fact** about an object whose lifecycle
is churn by design. Ordinary rollout noise — a readiness probe failing on a pod
twenty seconds from Ready, a `FailedScheduling` the next scheduler pass fixes,
a terminating pod on its way out — is indistinguishable from an outage unless
you ask again later. `rules` is how you ask again.

The config has two halves, named after the systems they come from:

| stanza | from | what it decides |
|---|---|---|
| `rules` | Prometheus | what counts as a problem, and how long it must hold (`for`) |
| `route.inhibitRules` | Alertmanager | which consequences to suppress when their cause is already reported |
| `route.timeIntervals` / `route.muteTimeIntervals` | Alertmanager | **when** to stay quiet, regardless of what the events say |

Dwell is **not** spelled `group_wait`. Alertmanager's `group_wait` batches a
group before its first notification; it is not `for:`, which does not exist in
Alertmanager at all.

### What `for` actually does

```
event matches a rule ──► hold `for` ──► re-check the involved object
                                            │
                          gone ─────────────┤ the terminating pod of a rollout
                          recovered ────────┤ the new pod became Ready
                                            │        → DROP
                          still unhealthy ──┴──────► EMIT once, with evidence
```

Verification is a three-rung ladder. **Pod** has a real health predicate
(phase, `Ready`, container waiting reasons). Every other kind has none, and
falls to **"did the event recur during the window"** — a controller with a live
problem keeps re-emitting, a resolved one goes quiet. Only when existence
itself cannot be determined does it **fail open and emit**.

Existence alone is never treated as confirmation. An autoscaler whose metric
lookup flapped once still exists.

### Two rules that keep the defaults honest

**Past-tense reasons must carry `for: 0`.** `OOMKilling`, `SystemOOM`,
`BackoffLimitExceeded`, `DeadlineExceeded` describe something that already
happened. A dwell would find the healthy *replacement* and delete the evidence.
This is the easiest way to build a rule set that looks careful and loses the
incidents that matter most.

**`Evicted` is dropped, not dwelled** (chart 5.9.0; it was previously
past-tense at `for: 0`). An eviction is already reported from both ends, and
per pod from neither:

| Eviction | Reported by | Why not per pod |
|---|---|---|
| kubelet, under node pressure | `NodeHasMemoryPressure` / `NodeHasDiskPressure`, tier 3, `for: 0` | one node-level signal beats one per displaced pod |
| API-initiated (drain) | nothing, deliberately | a drain is an operator doing what they were told — and unattended wherever a reboot manager runs |
| pod does not come back | `FailedScheduling`, tier 5, `for: 5m` | this is the half worth waking for, and it is confirmed by a dwell |

What the drop costs is the case where pods evict, reschedule cleanly, and the
node reports no pressure — a cluster working as designed. Because the drop
leans on those two substitutes, the render test pins them *together* with it:
re-tuning node pressure or `FailedScheduling` cannot silently leave eviction
unreported from every direction at once.

To restore per-pod eviction signals, move `Evicted` out of the tier-1 drop
matcher and back into the tier-2 `for: "0"` rule — restating the whole `rules`
list, since Helm replaces list values rather than merging them.

**The last rule must be a catch-all with a dwell, not a drop.** That is the
"do not miss issues" guarantee: a reason nobody anticipated — a third-party
controller's warning, a reason added in a future Kubernetes release — is
verified and reported rather than discarded.

Both are pinned by test in `internal/integration/charttemplate_test.go`, so the
tuning numbers stay editable without anyone having to re-derive the shape.

### Maintenance windows: the time axis

Some outages are on a schedule. A router that restarts at 04:00 takes the
cluster's connectivity with it for fifteen minutes, every night, and the events
it produces are real — the nodes really are NotReady — so nothing above can
suppress them:

- **`for:` cannot.** A dwell verifies the condition still holds, and during a
  scheduled outage it genuinely does. A dwell long enough to cover the window
  would delay every real incident by the same amount, all day.
- **Inhibition cannot.** It suppresses the consequences of a cause that is
  already reported, keyed on a cause event. A router losing power produces no
  in-cluster object; the cluster only ever sees consequences.
- **Matchers cannot.** They select on labels, and there is no label for the time
  of day.

So the fourth axis is time, in Alertmanager's exact vocabulary:

```yaml
eventsAdapter:
  source:
    route:
      timeIntervals:
        - name: nightly-restart
          times:
            - startTime: "04:00"    # inclusive
              endTime: "04:20"      # exclusive
          location: Europe/Kyiv     # name your zone — see below
      muteTimeIntervals:
        - name: nightly-restart
          matchers:                 # narrow it — see below
            - reason=~"NodeNotReady|Unhealthy|FailedMount|FailedScheduling"
```

`timeIntervals` also takes `weekdays`, `daysOfMonth` (negative values count back
from the end of the month), `months` and `years`, in Alertmanager's forms
(`monday:friday`, `january`, `1:7`). A window spanning midnight is two entries,
as in Alertmanager. Overlapping intervals **union** — muted when any of them
matches — so ordering never has to be reasoned about.

**Name your zone.** `location` defaults to UTC, which is Alertmanager's
behaviour, but "four in the morning" is a *local* fact: a UTC-pinned window
drifts by an hour at each daylight-saving transition, so it stops covering the
outage it was written for, on a date nobody chose, at an hour nobody is
watching. Any IANA name works — the timezone database is compiled into the
adapter image, so the distroless pod needs nothing added.

**Narrow it.** With no `matchers`, the window silences the source completely for
its whole duration, and that is the feature's principal hazard. A restarting
router produces connectivity-shaped reasons; it does not produce `OOMKilling`,
and an OOMKill at 04:05 is exactly as real as one at noon. Naming what you
expect means the configuration is safe without depending on the window staying
short or on anyone reviewing it later.

**Muting is evaluated at emit** — after the dwell, before the emit cap. Two
consequences worth relying on:

- A problem that **outlives** the window still gets reported. The cluster keeps
  producing events for anything genuinely broken, and the first one after 04:20
  emits normally. The window suppresses the transient, not the incident.
- A muted burst never spends the emit budget, so a maintenance window cannot
  read as a runaway.

**Muting is never silent.** While a window is active the source's `Ready`
condition stays true and says so, naming the interval; when the window closes it
reports how many events it muted. A muted lane and an idle lane look identical
from outside, and only one of them means the cluster is healthy.

```
kubectl get signalsource cluster-events -o wide
kubectl get signalsource cluster-events -o jsonpath='{.status.conditions[?(@.type=="Ready")].message}'
```

A malformed interval — an unknown zone, a reversed range, a mute naming an
interval that does not exist — **fails the source** rather than being ignored,
because a typo that silently produced a window which never fires would look
exactly like a window that works.

### Cookbook

```yaml
eventsAdapter:
  source:
    rules:
      # drop noise outright
      - matchers: ['reason=~"ProbeWarning|SandboxChanged"']
        action: drop

      # a flappy reason: wait it out, but do not hide an outage
      - matchers: ['reason="Unhealthy"']
        for: 10m
        escalateAfterObjects: 3   # 3 pods of one workload → emit now

      # urgent: never wait
      - matchers: ['reason=~"NodeNotReady|OOMKilling"']
        for: "0"

      # scope by pod label — pod labels are copied onto the signal
      - matchers: ['app.kubernetes.io/part-of="scratch"']
        action: drop

      # everything else
      - matchers: []
        for: 3m

    route:
      inhibitRules:
        # a down node makes all its pods look broken; report the node
        - sourceMatchers: ['reason="NodeNotReady"']
          targetMatchers: ['reason=~"Unhealthy|FailedScheduling"']
          equal: [node]
```

Matchers use Alertmanager syntax (`=`, `!=`, `=~`, `!~`) over the signal's
labels, and regexes are **anchored** — `reason=~"Failed"` does not also match
`FailedMount`. `reason` is a readable alias for the `alertname` label.

Legacy `includeReasons`/`excludeReasons` still work; they translate into
leading drop rules, so there is one evaluation path.

`escalateAfterObjects` (default 3) exists because a long dwell is right for one
flapping pod and wrong for an outage: the premise it rests on — one object
misbehaving is churn — stops holding when several do at once.

## Grouping: by workload, not by pod

`eventsAdapter.source.grouping.signatureLabels` defaults to
`[namespace, workload]`. `workload` is the owning controller
(`Deployment/api`), resolved through **owner references** — Pod → ReplicaSet →
Deployment — never by parsing the pod's name, which breaks on StatefulSets
(`api-0`), DaemonSets (`api-xk2p9`) and bare pods.

This is what the pods/replicasets RBAC grant is for, and why grouping survives
a rollout: a pod name is unique per replica and regenerated every deploy, so
the old `[namespace, kind, name]` default made conversation count scale with
**pods × rollouts** and the manager's 7-day window reuse could never fire.

## Self-exclusion: agent-ops never signals on itself

The adapter drops every event about agent-ops' own machinery. This is an
invariant, not a filter, because the failure it prevents is unbounded:

```
Conversation → runtime pod → pod cannot start → Warning event
     ↑                                              │
     └──────────── signal ──── new Conversation ◄───┘
```

Nothing downstream catches that cycle — the fingerprint is fresh (new pod
name), the workload is fresh (the owner is the Conversation CR), and even a
correct liveness re-check passes it because the pod really is broken.
`maxActiveConversations` caps pods and `maxQueuedConversations` caps the pending
backlog, but neither stops the cycle — they only slow how fast it fills etcd.

Three independent mechanisms: **name prefix** (needs no API read, so it holds
before the cache is warm), **owner/label**, and **own namespace**. Only the
third is configurable, via `source.includeOwnNamespace: true` for installs that
co-locate their own workloads with the operator — and it relaxes *only* that
one. No configuration can re-admit agent-ops' own pods.

## Kubernetes as MCP tools (`mcp` / `mcpServers`)

Same two halves `prometheus-bundle` ships for metrics, for the cluster itself:
an `MCPConfig` (the server the agent connects to) and an `MCPToolset`
(permission to call it). Both halves matter — a config without the toolset gives
the agent a server it may not call.

Both are **on by default**, and they flip as a pair: an `MCPConfig` needs an
endpoint, and the server component supplies one, so the config's URL always has
a Service to default onto. That pairing is the whole reason this used to default
off — a Kubernetes bundle whose Kubernetes tooling is off is an install that
looks complete and cannot see the cluster through the path this project prefers.

```sh
# point at a server you already run instead of the bundled one
--set k8s-bundle.mcpServers.enabled=false --set k8s-bundle.mcp.url=http://my-k8s-mcp.svc:8080/mcp
# or no MCP at all
--set k8s-bundle.mcp.enabled=false --set k8s-bundle.mcpServers.enabled=false
```

The endpoint guard stays and still fails the render loudly for the one
combination that is genuinely broken — `mcp.enabled` with no server workload and
no `url` — because an `MCPConfig` pointing nowhere silently costs agents their
tools.

**Name both halves from your wiring.** The bundle renders the CRs; the route
that uses them is declared under the parent chart's `pipelines:`:

```yaml
pipelines:
  - name: k8s-ops
    profile: k8s-engineer
    signalSources: [cluster-events, home-ops]
    channels: [home-ops]
    toolsets: [agentops-observe, agentops-shell, k8s-observability, k8s-admin]
    mcpConfigs: [k8s-api]
```

Tooling lives on wiring, so a conversation reaching the same profile through a
Pipeline that binds neither gets no MCP. That is the design, not a gap.

**Reads and mutations are separate toolsets.** `k8s-observability` grants the 14
read tools, `k8s-admin` the 6 mutating ones (`resources_create_or_update`,
`resources_delete`, `resources_scale`, `pods_delete`, `pods_exec`, `pods_run`).
They are enumerated rather than `mcp__kubernetes__*` on purpose: a wildcard spans
both halves and defeats the split. Bind the read set alone and the route can
explain the cluster without touching it. `k8s-admin` only renders when a server
that actually registers those tools exists — which, by default, means
`global.agentops.runtime.rbacMode: full` (see below).

**MCP is the only cluster path.** Runtime image 0.5.0 dropped kubectl, so what
the served tools cannot express, the agent cannot do: there is no patch
semantics, no rollout, drain, wait or port-forward, and no text processing over
results. `agentops-shell` is still worth binding for the workspace, but it no
longer reaches Kubernetes. Operators who need a CLI keep one with a derived
runtime image — see the README.

**Two identities, and why the server component exists.** The `mcpServers`
workload runs as `agentops-mcp-k8s`, *never* the runtime SA (the chart fails the
render if you set them equal):

| Path | Who authenticates | Walls between the agent and the API |
|---|---|---|
| `mcp__kubernetes__*` | the MCP server's own SA (`agentops-mcp-k8s`) | three: the server's `--read-only` tool registration, the toolset allowlist, and that SA's RBAC |
| a derived image with a CLI | the runtime SA (`agentops-runtime`) | one: that SA's RBAC, which a shell hands over whole |

The second row is not shipped — it is what you opt into by building your own
runtime image, and it is listed so the cost of that choice is visible.

Revoking `agentops-mcp-k8s`'s grants removes the agent's MCP reach without
touching the runtime SA, and vice versa. Both default to the same shape (`view`
plus node/namespace/metrics reads), so turning the component on widens nothing.

**The server's posture derives from the one RBAC knob.** `mcpServers.readOnly`
and `mcpServers.rbac.mode` are `null` by default and follow
`global.agentops.runtime.rbacMode`:

| `rbacMode` | `--read-only` | server SA RBAC | `k8s-admin` toolset |
|---|---|---|---|
| `full` | off | `full` | rendered |
| `readonly`, `none`, `""` | on | `readonly` | absent |

They derive because they are bound by an invariant operators used to maintain by
hand in every install's values: an operator who grants the agent `full` and
leaves the server read-only has asked for a write-capable agent and given it no
way to write.

**What derivation costs, plainly.** Widening the agent to `full` widens the
server too, unless you say otherwise — the two identities are no longer
independently reviewable *by default*. That is accepted because the safe
direction (both read-only) is what every default path produces, and because the
separation stays reachable: `mcpServers.readOnly: true` under `rbacMode: full`
makes this a strictly observing agent — broad grants on the runtime SA that
nothing can exercise — and no mutating toolset renders. The
toolset wall is untouched either way — mutations need a Pipeline to bind
`k8s-admin` deliberately no matter what the server serves. `none` maps to a
**readonly** server rather than to nothing, on purpose: an agent that can read
the cluster through MCP and do nothing at all through its own identity is a
useful shape, not an accident.

**Turning the component off leaves a blind agent.** With `mcp.enabled=false`
there is no other path: the runtime image ships no CLI, so the k8s-engineer
profile installs, starts, and cannot see the cluster it was installed to
inspect. The post-install notes say so rather than letting it be discovered by
asking a question and getting an apology.

The shipped server is `ghcr.io/containers/kubernetes-mcp-server` (pinned in
values), run with toolsets `core,config` and `--read-only` unless the derivation
above turns it off. `--read-only` filters
at tool *registration*, so mutating tools are uncallable rather than merely
unlisted. `mcp.transport` selects streamable HTTP (`/mcp`, the default) or legacy
SSE (`/sse`); `mcp.toolset.tools` is overridable if you want to enumerate
individual tool names instead of the `mcp__kubernetes__*` wildcard — which is
worth doing when `mcp.url` points at a server this chart did not deploy.

Two tools (`nodes_log`, `nodes_stats_summary`) read through `nodes/proxy`, which
a `readonly` server SA deliberately does not grant; they fail with a
Forbidden the agent can read, and widening is a deliberate grant.

The events adapter (`signal-k8s-events/`) watches core `v1` Events through the
in-cluster API with its own ServiceAccount token — the operator grants adapters
nothing, so those permissions come from this chart, bound to the deterministic
name `agentops-signal-<adapter>`. Its `severities` default to `["Warning"]`,
and it normalizes only: fingerprints key on the involved **object and reason**,
so Kubernetes recreating Event objects for a recurring problem still collapses
into one conversation.
