---
paths:
  - "chart/**"
  - "chart/**/*"
---

## The chart

### `chart/charts/k8s-bundle/`

The cluster Events lane and the Kubernetes tooling.

| Component | Renders |
|---|---|
| events | adapter + RBAC + `SignalSource`. **The events component renders the source, never the claim on it** |
| profile | the `k8s-engineer` profile — ONE object, identity only |
| `pipelines` | the WIRING component — see below |
| `mcp` / `mcpServers` | the `MCPConfig`, the toolsets and the server workload — see below |

**The `pipelines` component is the one bundle route that ships**, because it
owns its whole lane: at most ONE Pipeline claiming its own source with its own
profile and toolsets, channels values-supplied and omitted when unset.

- **OFF inside an active bundle and forced on by `global.demo.enabled` ALONE**,
  which is why `pipelines.enabled` is nullable — an explicit `false` must
  decline the route under demo too.
- **WHICH route is a fourth derivation from
  `global.agentops.runtime.rbacMode`.** `full` renders the acting `k8s-operate`
  (binds `k8s-admin`), everything else the observing `k8s-observe`.
- **Per-route booleans win both ways**, and both at once is ALLOWED and fans
  out.

**NO substrate**: no AgentRuntime, no runtime SA, no credential, no runtime
RBAC. All of that is the parent's `runtime:` + `global.agentops.runtime.*`.

**The profile has no repository**, so it carries an inline `systemPrompt` role.
Otherwise an event wakes a personality-free agent.

**Self-gated on `enabled OR global.demo.enabled`.** Demo mode IS this bundle —
`chart/templates/demo.yaml` is gone.

The `mcp` component:

- **`MCPConfig` `k8s-api`**, server key FIXED at `kubernetes`.
- **TWO MCPToolsets split by risk** — `k8s-observability` (14 read tools) and
  `k8s-admin` (6 mutating), ENUMERATED not wildcarded, because
  `mcp__kubernetes__*` spans both halves and defeats the split.
- **`k8s-admin` renders only when a server that REGISTERS those tools exists.**
- **`mcp` and `mcpServers` are ON by default and flip as a PAIR.** The config's
  URL defaults onto the deployed Service, which is the only reason the component
  used to be off. The endpoint guard stays and still fails `mcp.enabled` with no
  server and no `url`.
- **`mcpServers` runs `containers/kubernetes-mcp-server`** (`--read-only`,
  filtering at REGISTRATION not listing) under a SECOND SA `agentops-mcp-k8s` —
  never the runtime SA, and the render fails if they are equal.
- **That second identity IS the component's reason to exist.** MCP reach = the
  server SA's RBAC ∩ the toolset. Two walls.
- **Since runtime 0.5.0 it is also the ONLY cluster path** — no CLI in the image
  — so `mcp.enabled: false` leaves an agent that cannot see the cluster.
- **`readOnly` / `rbac.mode` are null and DERIVE from
  `global.agentops.runtime.rbacMode`.** `full` gives a write-capable server
  under a full SA, anything else a read-only server under `readonly`.
- **Explicit wins.** `readOnly: true` under `full` is a strictly observing
  agent: broad grants on the runtime SA that nothing can exercise.

### `chart/charts/prometheus-bundle/`

**WAS `vm-bundle` through chart 5.12.0.** It ships:

- **The Alertmanager ingest lane.**
- **ONE metrics MCP component.** `MCPConfig` server key FIXED at `prometheus`,
  plus a WILDCARD `MCPToolset` — all six tools the server registers are
  read-only, so unlike k8s-bundle there is no risk split to enumerate. The
  PINNED tag is what keeps the wildcard honest.
- **Its deployable server under a SECOND SA.**
- **The `alert-investigator` profile** — identity only, inline role, no
  repository, so no agent definition resolves.
- **ONE default-off route.**

**NAMED FOR THE PROTOCOL, NOT A VENDOR.** The ingest core reads the standard
Alertmanager payload, and VM answers the Prometheus query API — buildinfo
reports a Prometheus version, and MetricsQL is a PromQL superset — so one server
key serves both backends.

- **The LOGS component is DELETED, not ported.** VictoriaLogs speaks LogsQL and
  no Prometheus server reaches it.
- **Self-registration is KEPT and labelled VICTORIAMETRICS-ONLY.** It writes a
  `VMAlertmanagerConfig`, and vanilla Alertmanager's config is a file, so there
  is no object to write. NOTES.txt prints the receiver stanza instead, with
  `send_resolved: false` because the adapter drops non-firing alerts.
- **The backend URL is NEVER derived.** Single-node VM, cluster mode and
  Prometheus each serve the query API under a different path.
- **NEVER enabled by demo mode.** Every component needs an endpoint no demo
  cluster has, which is why `active` has no demo branch.
- **The retired `vm-bundle:` key FAILS the render.** Helm never reports an
  unread values key, so the rename would otherwise install nothing and look
  successful.

### `chart/charts/ha-bundle/`

The Home Assistant lane and a PRIVILEGE SPLIT:

- **The log ingest lane.**
- **ONE `MCPConfig`**, server key FIXED at `homeassistant`, and NO server
  workload — the house serves its own MCP endpoint.
- **TWO risk-split `MCPToolset`s.**
- **TWO identity-only profiles** — `ha-user` USES the house, `ha-operator` FIXES
  it.

**The split is use-versus-fix, NOT read-versus-act.** Home Assistant has no
read-only role, so both agents act. What separates them is the REST path: Assist
intents reach no configuration, so repairing needs a shell and only the ops
route binds one.

**The OPERATOR credential gates the fixing half AND the ingest lane.**
`subscribe_events` is admin-only, so a control token authenticates and is then
refused the subscription, which reads like a network fault.

- **Never enabled by demo mode.**
- **Two default-off routes, and BOTH claim the chat sources.** Wiring is
  many-to-many, so a shared surface offering both agents is the point.
- **`ha-ops` additionally claims the log source**, which is the only asymmetry.
- **`pipelines.restAccess` is PER ROUTE** — on for ops, off for control.
- **Credentials come as a NAME or as the TOKEN ITSELF.** The token form makes
  the bundle create the Secret and derive BOTH keys (`token` +
  `authorization`), which is what lets a secret manager's ref go straight into
  values.

### `chart/charts/telegram-bundle/`

**The three-component Telegram stack** — router, signal adapter, channel
adapter — as adapter CRs. Under `surface.enabled` it also renders the Channel,
the chat SignalSource and the router's credential source.

- **`surface.enabled` makes the unguessable fields REQUIRED.** A missing
  `chatId`, a missing credential, or BOTH credential forms at once FAIL the
  render.
- **Credentials either way:** `credentials.existingSecret` OR
  `credentials.botToken`, where the bundle creates the Secret.
- **One Secret serves both** — the Channel sends with it, the router's source
  polls.
- **Ships NO Pipeline on purpose**, because wiring drags in a profile, a runtime
  and credentials. The sources sit unclaimed until the installer wires them, so
  NOTES.txt prints the exact Pipeline to apply.

### `chart/` (the parent)

Manager Deployment, RBAC and Service, plus CRDs as gated templates.

- **`crds.enabled` / `crds.keep`** → `helm.sh/resource-policy: keep`, so
  uninstall never cascade-deletes CRs.
- **CRD source of truth is `chart/files/crds/`** — controller-gen output.
- **`templates/runtime.yaml` is THE SUBSTRATE.** One `AgentRuntime` named
  `default`, plus its credential Secret when `runtime.credentialsSecret.token`
  is set, with `home.pvcRef` WIRED from the parent's own `persistence` and never
  copied.
- **`templates/runtime-rbac.yaml`** renders the mode-driven bindings.
- **`templates/_helpers.tpl` resolves BOTH substrate facts from `.Values.global`
  alone**, so a subchart calling them cannot disagree with the parent.

### The DEMO WIRES THE CONSOLE

Where k8s-bundle renders a route, that route claims the console's source and
binds it as a channel, from `global.agentops.console` — a subchart reads no
other parent scope, and helm cannot derive a value from a value.

- **Those names DUPLICATE `console.signalSourceName` / `channelName`**, so the
  render FAILS when they disagree.
- **Scoped to demo mode**, because `console.enabled: false` is pinned to remove
  every console object with ONE value.
- **The claim rides the EXISTING route.** A second claimant makes every
  unaddressed console message ambiguous.
