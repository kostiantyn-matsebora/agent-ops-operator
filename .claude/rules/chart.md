---
paths:
  - "chart/**"
  - "chart/**/*"
---

## The chart

### `chart/charts/kubernetes/`

The cluster Events lane and the Kubernetes tooling.

| Component | Renders |
|---|---|
| events | adapter + RBAC + `SignalSource`. **The events component renders the source, never the claim on it** |
| profile | the `k8s-engineer` profile — ONE object, behaviour only |
| `pipelines` | the WIRING component — see below |
| `mcp` / `mcpServers` | the `MCPConfig`, the toolsets and the server workload — see below |

**The `pipelines` component is the one bundle route that ships**, because it
owns its whole lane: at most ONE Pipeline claiming its own source with its own
profile and toolsets, channels values-supplied and omitted when unset.

- **OFF inside an active bundle and forced on by `global.demo.enabled` ALONE**,
  which is why `pipelines.enabled` is nullable — an explicit `false` must
  decline the route under demo too.
- **WHICH route is one of FOUR things `kubernetes.allowMutations` moves.** True
  renders the acting `k8s-operate` (binds `k8s-admin`), false the observing
  `k8s-observe`. It is the BUNDLE'S OWN setting — a release-wide permission mode
  drove all four once and named none of them.
- **Per-route booleans win both ways**, and both at once is ALLOWED and fans
  out.

**NO substrate**: no AgentRuntime, no floor SA, no credential, no context
volume. All of that is the parent's `global.agentops.runtimeDefaults` +
`runtimes:`. It DOES render one identity per route it ships, each holding no
Kubernetes RBAC — the grant is on the MCP server's own account.

**The profile has no repository**, so it carries an inline `systemPrompt` role.
Otherwise an event reaches a personality-free agent.

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
- **`readOnly` / `rbac.mode` are null and follow `kubernetes.allowMutations`.**
  True gives a write-capable server under the chart's enumerated acting grant,
  false a read-only server. EACH IS STILL OVERRIDABLE ALONE, and none derives
  from another — setting the toolset does not move the server's flag.
- **It bound `cluster-admin` under `full` and the built-in `view` under
  `readonly`, and does NEITHER now.** An agent reaches the cluster THROUGH this
  server, so both were the same hole the runtime account's was — fixing one wall
  and not the other moves it one indirection along. And `view` is cluster-wide,
  so a "read-only" server could read the Conversations it was serving.
- **It carries the SAME rules a declared acting account gets**, from the same
  helpers, so the two walls cannot disagree about what `full` means.
- **Explicit wins.** `readOnly: true` under `allowMutations: true` is a strictly
  observing agent: broad grants on the server that nothing can exercise.

### `chart/charts/prometheus/`

**WAS `vm-bundle` through chart 5.12.0, and `prometheus-bundle` through 9.0.0.** It ships:

- **The Alertmanager ingest lane.**
- **ONE metrics MCP component.** `MCPConfig` server key FIXED at `prometheus`,
  plus a WILDCARD `MCPToolset` — all six tools the server registers are
  read-only, so unlike the `kubernetes` bundle there is no risk split to enumerate. The
  PINNED tag is what keeps the wildcard honest.
- **Its deployable server under a SECOND SA.**
- **The `alert-investigator` profile** — behaviour only, inline role, no
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

### `chart/charts/home-assistant/`

The Home Assistant lane and a PRIVILEGE SPLIT:

- **The ingest lane: the log plus four health surfaces**, each switched and
  tuned under `logsAdapter.source.surfaces` (config entries, repairs and
  sensors on, the update digest off), with one rule per surface AHEAD of the
  log rules in the shipped `rules`. Selected by `surface=`, never by
  message: a log rule's pattern must not capture a repair's text.
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
`system_log/list` — the listing the lane polls — and `subscribe_events` are
both admin-only, so a control token authenticates and is then refused, which
reads like a network fault.

- **Never enabled by demo mode.**
- **Two default-off routes, and BOTH claim the chat sources.** Wiring is
  many-to-many, so a shared surface offering both agents is the point.
- **`ha-ops` additionally claims the log source**, which is the only asymmetry.
- **`pipelines.restAccess` is PER ROUTE** — on for ops, off for control.
- **Credentials come as a NAME or as the TOKEN ITSELF.** The token form makes
  the bundle create the Secret and derive BOTH keys (`token` +
  `authorization`), which is what lets a secret manager's ref go straight into
  values.

### `chart/charts/telegram/`

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

Manager Deployment, RBAC and Service, plus the CRDs.

- **CRDs LIVE IN `chart/crds/`, NOT IN `templates/`, AND THAT IS LOAD-BEARING.**
  Helm applies that directory out-of-band, invalidates discovery and waits for
  the CRDs to establish BEFORE it builds the rest of the manifest.
  - **This chart ships CRDs AND INSTANCES OF THEM** — eleven CRDs beside eight
    Pipelines, Channels, profiles and toolsets. Helm resolves every kind in a
    manifest before applying any of it, so as templates the CRs could not map
    and a clean install died on `ensure CRDs are installed first`. It only ever
    worked because the cluster already had the CRDs from a previous install.
  - **cert-manager is NOT a counter-example.** It templates its CRDs and ships
    ZERO instances of them, so it never resolves an unknown kind. Measured:
    6 CRDs, 0 CRs. Ours is 11 and 8.
  - **Helm's own guidance names two methods and no third** — this directory, or
    two separate charts. There is no annotation that orders resources inside one
    release.
- **THE COST, and it is real: Helm never UPGRADES them either.** A CRD field
  change ships with a `kubectl apply -f chart/crds/` line in the release notes.
  `--dry-run` also does not cover them.
- **`crds.enabled` / `crds.keep` ARE GONE.** `enabled` is the `--skip-crds`
  flag, and `keep` is inherent because Helm never deletes what it installed
  from `crds/`. `templates/crds-guard.yaml` FAILS the render on either key —
  Helm never reports an unread value, so a silently-ignored `crds.enabled:
  false` would install them anyway and look successful.
- **CRD source of truth is `chart/crds/`** — controller-gen output, untemplated
  by construction now.
- **`templates/runtime.yaml` renders a LIST**, one CR per `runtimes:` entry,
  through `agentops.renderRuntime` in `_helpers.tpl` — SHARED, because a bundle
  may ship a runtime and two renderers would be two places for the CR's shape to
  drift. The `claude` subchart calls the same helper.
- **`templates/runtime-rbac.yaml`** renders every entry of
  `rbac.runtime.serviceAccounts`, one ClusterRole each. There is no MODE. It
  binds NOTHING to the floor account, refuses to be asked to, and uses no
  built-in role — see `invariants.md`.
- **`templates/rbac.yaml` ALWAYS renders the floor** and NEVER creates the
  account `runtimeDefaults.serviceAccountName` points at. Naming is not
  creating; rendering the floor regardless is what keeps it nameable.
- **`agentops.defaultRuntimeGuard` fails the render** when nothing answers to
  `default` while a route resolves to it — parent `pipelines:` AND bundle-shipped
  routes, re-derived through each bundle's own wiring helper so the check cannot
  drift from what rendered.

#### THE TWO VALUES BLOCKS, AND WHY `allowPodExecution` CANNOT MOVE

| Block | Holds |
|---|---|
| `global.agentops.runtimeDefaults` | what EVERY runtime inherits |
| `runtimes:` | the runtimes that EXIST, each stating only what DIFFERS |

**A SUBCHART READS NO PARENT SCOPE BUT `global.`** — Helm's rule, not a
convention this chart chose.

**AND A PARENT HELPER CALLED FROM A SUBCHART SEES ONLY `.Values.global` TOO.
THIS IS THE PART NOBODY REMEMBERS.** Named templates are global in Helm, so
`charts/kubernetes/templates/mcp-server.yaml` calls the PARENT's
`agentops.runtimeWriteRules` to build its MCP server's RBAC — and inside that
call, `dig "agentops" "runtimeDefaults" "allowPodExecution" false $g` resolves
against the SUBCHART's `.Values.global`.

- **Move that read to parent scope and the MCP server's write rules silently
  lose their gate.** DO NOT.
- It is also what makes `wiring.md`'s "BOTH WALLS MOVE TOGETHER" true rather
  than aspirational: one value gates a declared acting account's role and the
  MCP server's role, through one shared helper.
- **The second forcing is that a BUNDLE MAY SHIP A RUNTIME**, and it has no
  other scope to inherit the defaults from.
- **`agentops.mergedRuntime` merges TWO LEVELS BY HAND rather than calling
  `mergeOverwrite`.** mergo skips ZERO values in the source, so a runtime
  declaring `egressMediation: {enabled: false}` would silently keep the default
  `true` — the one override the egress requirement exists to guarantee.

### The DEMO WIRES THE CONSOLE

Where the `kubernetes` bundle renders a route, that route claims the console's
source and
binds it as a channel, from `global.agentops.console` — a subchart reads no
other parent scope, and helm cannot derive a value from a value.

- **Those names DUPLICATE `console.signalSourceName` / `channelName`**, so the
  render FAILS when they disagree.
- **Scoped to demo mode**, because `console.enabled: false` is pinned to remove
  every console object with ONE value.
- **The claim rides the EXISTING route.** A second claimant makes every
  unaddressed console message ambiguous.
