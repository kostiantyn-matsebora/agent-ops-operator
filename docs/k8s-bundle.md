# Kubernetes bundle (subchart)

The Kubernetes subchart: cluster Events, the agent that answers them, and its MCP tooling.


`chart/charts/k8s-bundle/` packages the whole "watch my cluster and let an agent
act on what it sees" experience as four independently toggleable components.
Off by default; `global.demo.enabled=true` and `k8s-bundle.enabled=true` are
equivalent ways to turn it on.

| Component | Flag | What it renders |
|---|---|---|
| Events lane | `eventsAdapter.enabled` | The `SignalAdapter` (`k8s-events`, `kubernetesAccess: true`), RBAC bound to its ServiceAccount (`events get/list/watch` plus read-only `pods`/`replicasets`), and — under `source.create` — a `SignalSource`. **Not the Pipeline**: claim it under `pipelines:` |
| Profile | `profile.enabled` | The `k8s-engineer` `AgentProfile` (identity only, with an inline `systemPrompt` role), its runtime `ServiceAccount` (`agentops-runtime-k8s`), and an `AgentRuntime` (named `default`) |
| RBAC | `rbac.enabled` | Bindings for that ServiceAccount — see `rbac.mode` below |
| MCP tooling | `mcp.enabled` | An `MCPConfig` (`k8s-api`, server key `kubernetes`) and an `MCPToolset` (`k8s-observability`), both bound automatically by the Pipelines above — see below |
| MCP server | `mcpServers.enabled` | The MCP server workload itself: `Deployment` + `Service` (`agentops-mcp-k8s`), **its own `ServiceAccount`**, and that SA's RBAC |

Two things worth knowing:

- **The source renders without wiring.** Claim it under the parent chart's
  `pipelines:`, or it reports `Wired=False` and drops every event. Wiring is
  pipeline-only, so a `SignalSource` nobody claims reports `Wired=False` and
  drops every event. Shipping the source alone would look installed and do
  nothing.
- **`rbac.mode: full` is cluster-admin.** It binds unrestricted cluster control
  to an LLM-driven agent. It is never a default and never what demo mode
  selects. `readonly` (the default) plus targeted grants under the parent
  chart's `rbac.runtime` block is almost always the better answer.
- **The agent's credential can be release-managed.** The `AgentRuntime`
  references a Secret (`profile.runtime.credentialsSecret.name`, default
  `agentops-claude`, key `oauthToken`) holding a `claude setup-token` result or
  an Anthropic API key. Set `profile.runtime.credentialsSecret.token` and the
  bundle **creates** that Secret — point it at your secret store and the
  credential comes back with the release. Leave it empty and the Secret is
  yours to create.

  Getting this wrong fails quietly and late: the kubelet resolves the
  reference, not the manager, so runtime pods sit in
  `CreateContainerConfigError` while conversations queue behind them and
  nothing reports a config error. The post-install notes call it out when no
  token is supplied.
- **Withholding shell is per-route**: bind
  `toolsets: {refs: [{name: agentops-observe}]}` on one Pipeline and only that
  route loses `Bash`, while every other route sharing the profile keeps it.
  `profile.addressable.grantShell: false` does the same for the shipped
  addressable Pipeline.

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

**Past-tense reasons must carry `for: 0`.** `OOMKilling`, `Evicted`,
`BackoffLimitExceeded`, `DeadlineExceeded` describe something that already
happened. A dwell would find the healthy *replacement* and delete the evidence.
This is the easiest way to build a rule set that looks careful and loses the
incidents that matter most.

**The last rule must be a catch-all with a dwell, not a drop.** That is the
"do not miss issues" guarantee: a reason nobody anticipated — a third-party
controller's warning, a reason added in a future Kubernetes release — is
verified and reported rather than discarded.

Both are pinned by test in `internal/integration/charttemplate_test.go`, so the
tuning numbers stay editable without anyone having to re-derive the shape.

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
`MAX_RUNTIMES` caps pods, not Conversation creation.

Three independent mechanisms: **name prefix** (needs no API read, so it holds
before the cache is warm), **owner/label**, and **own namespace**. Only the
third is configurable, via `source.includeOwnNamespace: true` for installs that
co-locate their own workloads with the operator — and it relaxes *only* that
one. No configuration can re-admit agent-ops' own pods.

## Kubernetes as MCP tools (`mcp` / `mcpServers`)

Same two halves `vm-bundle` ships for VictoriaMetrics, for the cluster itself:
an `MCPConfig` (the server the agent connects to) and an `MCPToolset`
(permission to call it). Both halves matter — a config without the toolset gives
the agent a server it may not call.

Unlike the other components this one is **off by default**, because an
`MCPConfig` needs an endpoint and the bundle has none until you supply a URL or
enable `mcpServers`. Either turns it on:

```sh
# point at a server you already run
--set k8s-bundle.mcp.enabled=true --set k8s-bundle.mcp.url=http://my-k8s-mcp.svc:8080/mcp
# or let the bundle run one
--set k8s-bundle.mcp.enabled=true --set k8s-bundle.mcpServers.enabled=true
```

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

**Reads and mutations are separate toolsets.** `k8s-observability` grants the 16
read tools, `k8s-admin` the 6 mutating ones (`resources_create_or_update`,
`resources_delete`, `resources_scale`, `pods_delete`, `pods_exec`, `pods_run`).
They are enumerated rather than `mcp__kubernetes__*` on purpose: a wildcard spans
both halves and defeats the split. Bind the read set alone and the route can
explain the cluster without touching it. `k8s-admin` only renders when a server
that actually registers those tools exists — `mcpServers.readOnly: false`, which
you should pair with `mcpServers.rbac.mode: full` or every mutation just returns
a Forbidden.

**MCP does not replace kubectl.** The served tools have no patch semantics, no
rollout, drain, wait or port-forward, and no text processing — so a route that
grants MCP writes usually still wants `agentops-shell` for what they cannot
express.

**Two identities, and why the server component exists.** The `mcpServers`
workload runs as `agentops-mcp-k8s`, *never* the runtime SA (the chart fails the
render if you set them equal):

| Path | Who authenticates | Walls between the agent and the API |
|---|---|---|
| `Bash` + `kubectl` | the runtime SA (`agentops-runtime-k8s`) | one: that SA's RBAC, which `Bash` hands over whole |
| `mcp__kubernetes__*` | the MCP server's own SA (`agentops-mcp-k8s`) | three: the server's `--read-only` tool registration, the toolset allowlist, and that SA's RBAC |

Revoking `agentops-mcp-k8s`'s grants removes the agent's MCP reach without
touching the runtime SA, and vice versa. Both default to the same shape (`view`
plus node/namespace/metrics reads), so turning the component on widens nothing.

**kubectl remains the fallback.** This changes nothing about the runtime image
or the profile — with `mcp.enabled=false` the agent reaches the cluster exactly
as it did before, and with it on it simply has both paths.

The shipped server is `ghcr.io/containers/kubernetes-mcp-server` (pinned in
values), run with `--read-only` and toolsets `core,config`. `--read-only` filters
at tool *registration*, so mutating tools are uncallable rather than merely
unlisted. `mcp.transport` selects streamable HTTP (`/mcp`, the default) or legacy
SSE (`/sse`); `mcp.toolset.tools` is overridable if you want to enumerate
individual tool names instead of the `mcp__kubernetes__*` wildcard — which is
worth doing when `mcp.url` points at a server this chart did not deploy.

Two tools (`nodes_log`, `nodes_stats_summary`) read through `nodes/proxy`, which
`mcpServers.rbac.mode: readonly` deliberately does not grant; they fail with a
Forbidden the agent can read, and widening is a deliberate grant.

The events adapter (`signal-k8s-events/`) watches core `v1` Events through the
in-cluster API with its own ServiceAccount token — the operator grants adapters
nothing, so those permissions come from this chart, bound to the deterministic
name `agentops-signal-<adapter>`. Its `severities` default to `["Warning"]`,
and it normalizes only: fingerprints key on the involved **object and reason**,
so Kubernetes recreating Event objects for a recurring problem still collapses
into one conversation.
