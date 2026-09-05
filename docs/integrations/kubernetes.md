---
title: Kubernetes
permalink: /integrations/kubernetes/
description: >-
  A bundle that turns cluster events into answered conversations. Configure it,
  tune what reaches you, and bind its parts into your own pipelines.

next:
  eyebrow: Next
  title: Answer your alerts
  body: >-
    The same shape for Prometheus and Alertmanager — a firing alert arrives
    with the investigation already done.
  url: /agent-ops-operator/integrations/prometheus/
---

**Your cluster already says what is wrong. Nobody reads it.** This bundle reads
the events, decides which are real, and gives an agent the tools to look.

![Two lanes meet on the route: cluster events filtered by your suppression rules, and the MCP server's tools filtered by the toolsets you bind.]({{ '/assets/img/integrations/kubernetes-light.svg' | relative_url }}){: .ao-diagram}

## What you get

One flag gives you a working lane, end to end.

| You get | Which means |
|---|---|
| **Triaged events** | A probe failing twenty seconds from Ready is churn. A pod still broken five minutes later is not. Only the second reaches you. |
| **An answer, not an alert** | The agent reads the pods, logs and events around the failure, then names the cause. |
| **Read-only until you say otherwise** | It explains the cluster and changes nothing. |
| **Somewhere to read them** | Answers arrive in the console, which the route binds for you. Reply there and the conversation continues. |
| **Parts you can reuse** | The source, the profile and the tooling are ordinary CRs. Bind them from your own pipelines — see [Adopt it](#adopt-it). |

## Turn it on

```sh
helm upgrade --install agent-ops oci://ghcr.io/kostiantyn-matsebora/charts/agent-ops-operator \
  -n agent-ops --create-namespace \
  --set kubernetes.enabled=true \
  --set kubernetes.pipelines.enabled=true
```

```powershell
helm upgrade --install agent-ops oci://ghcr.io/kostiantyn-matsebora/charts/agent-ops-operator `
  -n agent-ops --create-namespace `
  --set kubernetes.enabled=true `
  --set kubernetes.pipelines.enabled=true
```

**No credential.** It authenticates to the cluster it runs in.

`enabled` renders the parts. `pipelines.enabled` wires them into a working
route — leave it off and nothing answers, which is what you want if you are
[bringing your own](#adopt-it).

## The values you set

Everything below is under `kubernetes:`. These are the ones you decide — the
rest have working defaults, and `helm show values` is the full list.

### What exists

| Value | Default | Set it to |
|---|---|---|
| `enabled` | `false` | `true` to render the bundle at all |
| `pipelines.enabled` | unset — **on under demo** | `true` for a working route, `false` to wire it yourself |
| `allowMutations` | `false` | `true` to let the agent change the cluster |
| `eventsAdapter.enabled` | `true` | `false` if you post cluster events yourself |
| `eventsAdapter.source.create` | `true` | `false` to declare your own `SignalSource` |
| `profile.enabled` | `true` | `false` if you bring your own agent |
| `mcp.enabled` | `true` | `false` for an agent with no cluster reach |
| `mcpServers.enabled` | `true` | `false` to point at an MCP server you already run |

### What things are called

Rename any of these and your own pipelines reference the new name.

```yaml
kubernetes:
  eventsAdapter:
    source:
      name: cluster-events              # the SignalSource your pipeline claims
  profile:
    name: k8s-engineer                  # the AgentProfile
  mcp:
    name: k8s-api                       # the MCPConfig
    toolsets:
      observe:
        name: k8s-observability         # the read toolset
      admin:
        name: k8s-admin                 # the acting toolset
  pipelines:
    observe:
      name: k8s-observe                 # the read-only route
    admin:
      name: k8s-operate                 # the acting route
```

### Already run a Kubernetes MCP server?

Point the bundle at it and skip the workload:

```sh
--set kubernetes.mcpServers.enabled=false \
--set kubernetes.mcp.url=http://my-k8s-mcp.agent-ops.svc:8080/mcp
```

```powershell
--set kubernetes.mcpServers.enabled=false `
--set kubernetes.mcp.url=http://my-k8s-mcp.agent-ops.svc:8080/mcp
```

`mcp.transport` is `http` — set it to `sse` for a server on the legacy
transport. Enumerate `mcp.toolset.tools` yourself when the server is not the one
this chart deploys, since the shipped list names that server's tools.

### Where the answers turn up

**The console, already.** The route binds it as a channel and claims its
composer, so you can watch conversations arrive and reply to them without
setting anything.

```sh
kubectl -n agent-ops port-forward svc/agentops-adapter-console 8080:8080
```

```powershell
kubectl -n agent-ops port-forward svc/agentops-adapter-console 8080:8080
```

**Add a chat surface beside it.** `channels` merges with the console rather than
replacing it, so the answer reaches both:

```sh
--set kubernetes.pipelines.channels={team-chat}
```

```powershell
--set kubernetes.pipelines.channels={team-chat}
```

Every answer is on the CR too, whatever is bound:

```sh
kubectl get conversations -o jsonpath='{.items[*].status.runs[*].result}'
```

```powershell
kubectl get conversations -o jsonpath='{.items[*].status.runs[*].result}'
```

### May it act?

Off, it explains. On, it can also fix.

```sh
--set kubernetes.allowMutations=true
```

```powershell
--set kubernetes.allowMutations=true
```

| | Off (default) | On |
|---|---|---|
| **It can** | read pods, logs, events, nodes | those, plus delete, scale, restart, exec |
| **The route is called** | `k8s-observe` | `k8s-operate` |

> **`allowPodExecution` is separate, and defaults off.** An agent that can start
> a pod can read any Secret that pod mounts, whatever else you granted. Turn it
> on deliberately.
{: .ao-callout}

**One flag moves both walls together**, because an operator who asks for an
acting agent and leaves the server read-only has asked for something and given
it no way to do it.

**Each is still settable on its own.** The useful one is an acting route on a
server that cannot act:

```sh
--set kubernetes.allowMutations=true \
--set kubernetes.mcpServers.readOnly=true
```

```powershell
--set kubernetes.allowMutations=true `
--set kubernetes.mcpServers.readOnly=true
```

That gives you the `k8s-operate` route and the `k8s-admin` toolset, on a server
whose mutating tools are never registered — broad grants that nothing can
exercise, which is the shape to hold while you decide.

## Tune what reaches you

Turned it on and got paged about noise? Everything below goes under
`kubernetes.eventsAdapter.source`.

### "Stop telling me about probe warnings"

```yaml
eventsAdapter:
  source:
    rules:
      - matchers: ['reason=~"ProbeWarning|SandboxChanged"']
        action: drop
      - matchers: []
        for: 3m
```

**Always keep a last rule with no matchers.** It is what catches the failure
nobody anticipated.

### "Wait before telling me a pod is unhealthy"

```yaml
      - matchers: ['reason="Unhealthy"']
        for: 10m
        escalateAfterObjects: 3
```

`for` holds, then looks again — recovered or gone means you never hear about it.
`escalateAfterObjects` overrides the wait when three objects fail at once,
because that is no longer one flapping pod.

**Only a Pod can be looked at.** For any other kind — a Node, a Job, a storage
operator's own objects — the adapter asks the events instead: was this still
arriving as the window closed? "Closed" is the last third of the wait, never
less than thirty seconds. A controller that retried for forty seconds and then
healed has recurred, and is dropped; one still retrying at the deadline is
reported once, with the whole burst attached and the time of its last event.

### "Tell me immediately about OOM kills"

```yaml
      - matchers: ['reason="OOMKilling"']
        for: "0"
```

**Anything already finished needs `for: "0"`.** Wait on an OOM kill and the
re-check finds the healthy replacement, so the report is dropped.

**Don't add `NodeNotReady` to this one.** The bundle's own shipped rule
already reports it at `for: "0"` when it is genuinely a NODE-level event —
qualified `kind="Node"`, on purpose. A bare `reason=~"...|NodeNotReady"` with
no kind matcher catches the PER-POD copies the node lifecycle controller
stamps on every workload scheduled there, so one reboot fires once per
DaemonSet instead of dwelling to see whether the node came back.

### "Ignore my scratch namespace"

```yaml
      - matchers: ['app.kubernetes.io/part-of="scratch"']
        action: drop
```

Pod labels are copied onto the signal, so anything you already label by is
available here.

### "A reboot manager cordons my nodes one at a time — stay quiet through that"

This is already the default: **nothing to configure.** A node that is cordoned
or maintenance-tainted (kured is the common case) suppresses every event on
its objects for as long as it stays that way, whatever reason they carry and
however many DaemonSets have pods scheduled there.

```yaml
eventsAdapter:
  source:
    route:
      # The default, spelled out. Narrow it, or opt out per source:
      drainingNodes: suppress          # "report" evaluates as if this axis did not exist
      drainingNodeMatchers:
        - reason=~"NodeNotReady|Unhealthy|FailedMount|FailedScheduling"
      drainingNodeBound: 1h            # a drain nobody ends is reported once, past this
```

- **Needs `rbac.clusterWide: true`** (the default). Nodes are cluster-scoped,
  so a namespaced install has no equivalent grant — drain awareness is off
  there, and the source's condition says so once.
- **A drain outliving the bound is reported, not hidden.** Past
  `drainingNodeBound` (default `1h`) you get ONE conversation, `kind: Node`,
  reason `NodeDrainExceeded`, naming the node and how long it has been
  draining — a forgotten `kubectl cordon` should not go silent forever.
- **This is the mechanism that fixed a real 18-conversation night**: one node
  reboot used to fan out into one conversation per DaemonSet pod on it, each
  closed as "already recovered". No time window, no schedule to guess at —
  it starts when the cordon does and ends when the uncordon does.

### "Stay quiet during the nightly reboot" (no node ever gets cordoned)

The mute window below is for something the API server has NO state for at
all — a router power-cycling, an ISP maintenance slot — where drain awareness
above cannot help because no node's `spec.unschedulable` or taints ever change.

```yaml
eventsAdapter:
  source:
    route:
      timeIntervals:
        - name: nightly-restart
          times:
            - startTime: "04:00"
              endTime: "04:20"
          location: Europe/Kyiv
      muteTimeIntervals:
        - name: nightly-restart
          matchers:
            - reason=~"NodeNotReady|Unhealthy|FailedMount"
```

- **Name your zone.** The default is UTC, and a UTC window drifts an hour every
  daylight-saving change — at an hour nobody is watching.
- **Name the reasons.** With no `matchers` you silence everything, and an OOM
  kill at 04:05 is as real as one at noon.
- **Anything still broken at 04:20 reports normally.**
- **A rolling reboot drifting outside a fixed window is exactly why the
  section above exists** — this axis has no clock to fall behind.

### "Don't drown me during an incident"

Rules replace the whole list, so restate the ones you want to keep. Matchers are
Alertmanager's (`=`, `!=`, `=~`, `!~`) and regexes are anchored — `reason=~"Failed"`
does not match `FailedMount`.

## Adopt it

**The bundle's parts are ordinary CRs with stable names.** You do not have to
use its wiring to use its work.

Turn the bundle's own route off and declare your own:

```sh
--set kubernetes.enabled=true --set kubernetes.pipelines.enabled=false
```

```powershell
--set kubernetes.enabled=true --set kubernetes.pipelines.enabled=false
```

### Your agent, its tools

```yaml
pipelines:
  - name: my-cluster-agent
    profile: my-engineer          # YOUR profile, from your repository
    signalSources: [cluster-events]
    toolsets: [agentops-observe, k8s-observability]
    mcpConfigs: [k8s-api]
    channels: [team-chat]
```

Your agent now answers cluster events with the bundle's Kubernetes tooling, and
its behaviour is entirely yours. See
[Add your own agent]({{ '/guides/agent-profile/' | relative_url }}).

### One agent for the cluster *and* your chat

```yaml
pipelines:
  - name: ops
    profile: k8s-engineer         # the bundle's agent is fine to keep
    signalSources: [cluster-events, team-chat]
    toolsets: [agentops-observe, k8s-observability]
    mcpConfigs: [k8s-api]
    channels: [team-chat]
```

**This is the case the bundle cannot ship for you.** Its route sees only its own
objects, and your chat surface comes from a different bundle.

### The tools without the events

Want an agent you can ask about the cluster, with no events starting anything?
Bind the tooling and claim no cluster source:

```yaml
pipelines:
  - name: ask-the-cluster
    profile: k8s-engineer
    signalSources: [team-chat]
    toolsets: [agentops-observe, k8s-observability]
    mcpConfigs: [k8s-api]
    channels: [team-chat]
```

### Run yours beside the bundle's

**Sources are shareable, and nothing conflicts.** Leave `pipelines.enabled=true`
and declare your own claiming `cluster-events` as well — each admitted event
then opens **one conversation per claiming pipeline**, with its own agent and
its own tools.

Two conversations means two sets of model credits. That is the only cost.

### What the bundle renders

Everything the flags above produce, so you know what a name refers to before you
bind it.

<!-- generated: renders bundle=kubernetes -->
```text
# Events lane  (eventsAdapter.enabled)
ClusterRole/agentops-signal-k8s-events-events-agent-ops
ClusterRoleBinding/agentops-signal-k8s-events-events-agent-ops
ServiceAccount/agentops-signal-k8s-events
SignalAdapter/k8s-events
SignalSource/cluster-events

# Profile  (profile.enabled)
AgentProfile/k8s-engineer

# MCP tooling  (mcp.enabled)
MCPConfig/k8s-api
MCPToolset/k8s-observability

# MCP server  (mcpServers.enabled)
ClusterRole/agentops-mcp-k8s-agent-ops
ClusterRoleBinding/agentops-mcp-k8s-agent-ops
Deployment/agentops-mcp-k8s
Service/agentops-mcp-k8s
ServiceAccount/agentops-mcp-k8s

# Wiring  (pipelines.enabled)
Pipeline/k8s-observe
```
<!-- /generated -->

- **`MCPToolset/k8s-admin` appears only under `allowMutations: true`.**
- **The `SignalSource` appears only under `source.create`.**
- **The profile is behaviour only.** No tools and no cluster rights come with
  it, so binding it grants nothing by itself.

## Going deeper

How events are verified, grouped and suppressed, and why an agent reaches the
cluster through a server with its own identity:
[concepts]({{ '/concepts.md' | relative_url }}).
