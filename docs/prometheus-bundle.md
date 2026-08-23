# Prometheus bundle (subchart)

The Prometheus/Alertmanager subchart: alert ingestion, metrics tooling, the
agent that answers, and its opt-in route.


`chart/charts/prometheus-bundle/` packages the whole "an alert fires and an
agent investigates it" experience as five independently toggleable components.

**Off by default and never enabled by demo mode.** Every component consumes an
endpoint no demo cluster has: an Alertmanager that can reach the webhook, and a
metrics backend to query.

**The bundle is named for the payload format and the query API it speaks, not
for one implementation of them.**

The ingest core reads the standard Alertmanager webhook payload and nothing
else, so any Prometheus Alertmanager can post to it.

VictoriaMetrics answers the Prometheus HTTP query API — `/api/v1/query`,
`/api/v1/labels`, and `/api/v1/status/buildinfo` reports a Prometheus version
for clients that version-gate — and MetricsQL is a PromQL superset, so a single
query server serves both backends.

VictoriaMetrics is a supported backend here, not the subject. The only
VictoriaMetrics-specific feature is
[self-registration](#self-registration-is-victoriametrics-only).

| Component | Flag | What it renders |
|---|---|---|
| Ingest lane | `alertmanager.enabled` (**on**) | The `SignalAdapter` (`alertmanager`, reference adapter `signals/alertmanager/`, `port: 8080`) and — under `defaultSource.enabled` — a `SignalSource`. **Not the claim on it**: that is the wiring component, or your own `pipelines:` |
| Profile | `profile.enabled` (**on**) | Exactly one object: the `alert-investigator` `AgentProfile` (behaviour only, with an inline `systemPrompt` role) |
| MCP tooling | `mcp.enabled` | An `MCPConfig` (`prometheus-api`, server key `prometheus`) and an `MCPToolset` (`prometheus-observability`) |
| MCP server | `mcpServers.enabled` | The query server workload: `Deployment` + `Service` (`agentops-mcp-prometheus`) and **its own `ServiceAccount`** |
| Wiring | `pipelines.enabled` (**off**) | One `Pipeline` claiming the source above with the profile above — see [The bundle's own wiring](#the-bundles-own-wiring) |

**The bundle ships no substrate.** There is no `AgentRuntime`, no runtime
ServiceAccount, no LLM credential Secret and no runtime RBAC here.

Those are release-wide facts and live in the parent chart's `runtime:` block and
`global.agentops.runtime.*`
([concepts](concepts.md#the-substrate-runtime-and-globalagentopsruntime)).

The profile executes on the parent's `AgentRuntime` named `default`.
`pipelines.runtimeRef` points it at a different one you applied yourself.

**This bundle renders its route's own ServiceAccount** (`agentops-alert-triage`)
and names it on the Pipeline.

It holds no Kubernetes RBAC. The lane reaches a Prometheus query API through an
MCP server that has its own account, so the agent's pod needs no Kubernetes
permission to investigate an alert.

`pipelines.serviceAccountName` names your own instead.

> **Renamed in chart 5.13.0.** This was `vm-bundle`. Every `vm-bundle.*` value
> must be restated under `prometheus-bundle:` — the render FAILS while the old
> key is present, because Helm never reports an unread values key. The logs
> component was removed and the metrics server key changed. See
> [CHANGELOG.md](CHANGELOG.md).

## The ingest lane (`alertmanager`)

The chart renders the `SignalAdapter` CR only — the reconciler owns the workload
and, via `spec.port`, the webhook Service `agentops-signal-<name>` and
`LISTEN_ADDR`. The chart ships no connectivity.

The adapter CR name is the **routing key**: sources select this implementation
with `spec.adapter: <name>`. It defaults to `alertmanager`, and
`defaultSource.spec.adapter` renders from that same value so configuration
cannot drift from the implementation it targets.

The adapter accepts the standard Alertmanager webhook payload and reads six
fields per alert (`status`, `fingerprint`, `startsAt`, `labels`, `annotations`,
`generatorURL`). It ignores the envelope and **drops anything whose status is not
`firing`** — which is why the printed receiver sets `send_resolved: false`.

### Pointing a vanilla Alertmanager at it

With registration off, the post-install notes print the exact stanza, filled in
with your Service URL and source name:

```yaml
receivers:
  - name: agentops
    webhook_configs:
      - url: http://agentops-signal-alertmanager.<ns>.svc:8080/webhook/alerts
        # the adapter drops non-firing alerts, so resolutions would be discarded
        send_resolved: false
route:
  receiver: agentops
```

Set `defaultSource.credentialsSecretRef` to require a bearer token, and the notes
print the matching `http_config.authorization` block alongside it.

### Self-registration is VictoriaMetrics-only

With `registration.enabled=true` (plus its target) the **adapter configures the
sender itself**.

It writes a `VMAlertmanagerConfig agentops-<source>` — a webhook receiver
pointing at its own endpoint, with a route carrying `continue: true` so existing
receivers keep their alerts — and the bundle renders the least-privilege
Role/RoleBinding that makes it possible.

**This cannot be generalized to vanilla Alertmanager**, and that is a property of
Alertmanager rather than a gap: its configuration is a file or a Secret, not a
Kubernetes object an adapter can write. Running vanilla Alertmanager? Leave this
off and paste the receiver above.

The routing decision lives entirely in the source's `register` block
(`matchers`, `groupWait`, `groupInterval`, `repeatInterval`, `maxAlerts`,
`sendResolved`), so it can **replace** a hand-written receiver rather than sit
beside one.

**Two things decide whether the replacement actually receives anything, and both
live on the sender:**

- **Order.** vm-operator appends these routes *after* the ones in your base
  config, so an earlier route matching the same alerts needs `continue: true` or
  it terminates matching first.
- **Namespace scope.** It scopes them to their own namespace unless the
  VMAlertmanager sets `spec.disableNamespaceMatcher`.

Registration failure never unserves the source: the webhook stays live and the
source's Ready condition names the cause plus the manual step, retried every 15s
so granting the permission heals it without a restart.

## Metrics as MCP tools (`mcp` / `mcpServers`)

Two halves, and both matter — a config without the toolset gives an agent a
server it may not call:

- **`mcp`** renders the `MCPConfig` (`prometheus-api`) whose single server entry
  uses the **fixed** key `prometheus`, plus the `MCPToolset`
  (`prometheus-observability`) granting `mcp__prometheus__*`. The key has no
  values path: it IS the tool namespace named in allowlists, so a rename would
  silently strip an agent's tools instead of failing.
- **`mcpServers`** deploys the server itself
  (`ghcr.io/pab1it0/prometheus-mcp-server`) against a required `backend` URL.
  With it deployed, an empty `mcp.url` defaults onto the deployed Service. An
  explicit `url` still wins.

The toolset is **wildcarded**, unlike `k8s-bundle`'s enumerated lists. All six
tools this server registers — `execute_query`, `execute_range_query`,
`list_metrics`, `get_metric_metadata`, `get_targets`, `health_check` — are
read-only, so there is no read/mutate boundary to preserve. The pinned image tag
is what keeps that true across upgrades.

Both components default **off** and flip together, because an `MCPConfig` needs
an endpoint and the server component supplies one. Unlike `k8s-bundle`'s, there
is no in-cluster endpoint to default onto: the backend is operator-supplied by
definition.

**The backend URL is never derived.** Single-node VictoriaMetrics serves
`/api/v1`, cluster mode serves `/select/<accountID>/prometheus/api/v1`, and
Prometheus serves `/api/v1` under whatever external URL it was given.

No template can guess among those, and guessing wrong produces a server that
starts and answers nothing. An enabled `mcp` component with neither a deployed
server nor a URL fails the render.

The server runs under **its own ServiceAccount**, never the runtime's — setting
them equal fails the render.

It needs no Kubernetes RBAC at all, because it reads an HTTP query endpoint
rather than the API server, so the bundle renders none for it. The separate
identity is where a backend credential would be projected.

```sh
# deploy the server and let the config default onto it
--set prometheus-bundle.mcp.enabled=true \
--set prometheus-bundle.mcpServers.enabled=true \
--set prometheus-bundle.mcpServers.backend=http://vmsingle-vm.monitoring.svc:8429

# or point at a server you already run
--set prometheus-bundle.mcp.enabled=true \
--set prometheus-bundle.mcp.url=http://mcp-prometheus.<ns>.svc:8080/mcp \
--set prometheus-bundle.mcpServers.enabled=false
```

`headers` pass through with `valueFrom` secret refs for authenticated endpoints,
resolved in the runtime pod — the manager reads no Secrets.

## The profile (`profile`)

One object: the `alert-investigator` `AgentProfile`. Identity only — no
repository, no `allowedTools`, no `mcp`. What this agent may DO comes from
whichever Pipeline routes it.

Because it has **no repository**, no `.claude/agents/<name>.md` can be resolved
for it, so the inline `systemPrompt` is not decoration: without it an alert would
open a conversation with a personality-free agent whose only inputs are an
allowlist and a payload.

The shipped role tells it to query the metric that fired before concluding
anything, to show the evidence behind its finding, and to recommend rather than
claim to have applied a fix.

Shipping this profile is also what makes the bundle's own wiring permissible: a
subchart may render a Pipeline only when that Pipeline renders with its own
profile.

## The bundle's own wiring

`pipelines.enabled` defaults **false**, and unlike `k8s-bundle`'s nothing forces
it on — no turnkey mode enables this bundle at all. Turning it on renders one
`Pipeline/alert-triage` claiming the bundle's source with the bundle's profile,
toolset and MCPConfig:

```sh
--set prometheus-bundle.enabled=true \
--set prometheus-bundle.alertmanager.defaultSource.enabled=true \
--set prometheus-bundle.pipelines.enabled=true
```

**Every admitted alert then opens a conversation and spends LLM credits**, where
before it dropped at `Wired=False`. Exactly one route renders: a metrics query
server is read-only, so there is no second posture to express and no derivation
from the release's RBAC mode.

Every reference to an object the bundle does not itself render is a
values-supplied name, omitted when unset. Channels are the only such name:

```sh
--set prometheus-bundle.pipelines.channels={console}
```

With none bound the conversation dispatches immediately and the answer is read
from `status.runs[].result`. A ref to a component you turned off is omitted
rather than left dangling.

**Sources are shareable.** The bundle's route and a route you declared under the
parent chart's `pipelines:` may both claim `alerts`. Both render, and each
admitted alert opens one conversation **per claiming Pipeline**, under each
profile's own capabilities. That is reported in the post-install notes, never
refused. Declare the route yourself instead when one agent should answer these
alerts *and* a chat surface — that route spans bundles, so only the parent chart
can see all of it.
