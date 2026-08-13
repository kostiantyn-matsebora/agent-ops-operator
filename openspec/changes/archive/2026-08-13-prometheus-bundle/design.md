## Context

Three facts about the current bundle shape the design.

**The ingest core is already vendor-neutral.** `signal-vmalertmanager` parses the
standard Alertmanager webhook payload and reads six fields per alert. It ignores
the envelope (`version`, `groupKey`, `receiver`, `commonLabels`, `externalURL`)
and drops anything whose `status` is not `firing`. Nothing in it is
VictoriaMetrics-specific except `register.go`, which writes a
`VMAlertmanagerConfig`. The module is therefore untouched by this change; only
its packaging and its name in the chart change.

**One MCP server covers both backends.** Probed live against
`vmsingle-...:8428` during research:

```
/api/v1/query?query=up              200  {"status":"success","data":{"resultType":"vector",...
/prometheus/api/v1/query?query=up   200  {"status":"success",...
/api/v1/labels                      200  {"status":"success","data":["GOARCH",...
/api/v1/status/buildinfo            200  {"status":"success","data":{"version":"2.24.0"}}
```

VM answers `buildinfo` with a Prometheus version on purpose, for clients that
version-gate; MetricsQL is a PromQL superset. So `mcp-victoriametrics` buys
VM-only extras (cardinality, TSDB status) at the cost of a second tool namespace
— not the ability to query.

**Wiring is now permissible for a bundle that owns its lane.** The
`k8s-bundle-wiring` change replaced "no subchart renders a Pipeline" with four
conditions: flag-gated, foreign names values-supplied and omitted when unset,
each Pipeline renders with its own profile, and the flag defaults off. This
bundle can satisfy them only if it also ships a profile — which is why the
profile and the wiring are one change and not two.

## Goals / Non-Goals

**Goals:**

- A Prometheus + Alertmanager install can use this bundle without reading
  anything about VictoriaMetrics.
- A VictoriaMetrics install keeps everything that made the bundle worth having,
  self-registration included.
- Enabling the bundle and its wiring flag yields an install that **answers**
  alerts, rather than one that ingests and drops them.
- An agent woken by an alert can query the metric that fired.
- Every removal is loud: nothing that worked silently stops working.

**Non-Goals:**

- Changing `signal-vmalertmanager`'s Go module, image name, or payload handling.
- Log querying in any form — removed, and the adopter's own concern.
- Coupling this bundle to demo mode; it consumes operator-supplied endpoints no
  demo cluster has.
- Relaxing the chart-managed-wiring rule any further than `k8s-bundle-wiring`
  already did.
- Automatic migration of `vm-bundle.*` values. Helm cannot rename a values key
  on an operator's behalf; the CHANGELOG carries it.

## Decisions

### 1. Rename the subchart; keep the adapter module's name

The chart is what an operator reads and configures, so the chart is what gets
renamed. `signal-vmalertmanager/` — module, image
`kmatsebora/agentops-signal-vmalertmanager`, and the
`vm-alertmanager-signal-adapter` spec — keeps its name, because renaming a
published image to describe a payload format it never owned would be churn with
a migration attached and no benefit.

That leaves one visible seam: the bundle's default `SignalAdapter` CR name,
`vm-alertmanager`, which is also the **routing key** every `SignalSource` names
in `spec.adapter`. Changing the default to `alertmanager` reads correctly in a
Prometheus install and **breaks every hand-written source** that says
`adapter: vm-alertmanager`.

**Decision: the default becomes `alertmanager`**, with the one-line override
`prometheus-bundle.alertmanager.name: vm-alertmanager` documented in the
CHANGELOG for installs that would otherwise have to edit their sources. The
rename is already breaking and already requires restating every value under a new
key; carrying a vendor name in the default for a bundle that is no longer about
that vendor would preserve the exact confusion this change exists to remove.

*Alternative considered:* keep `vm-alertmanager` as the default. Cheaper for
existing installs by one value — but they are already restating every value, so
the saving is nil, and every future install inherits the misleading name.

### 2. One metrics component, server key fixed at `prometheus`

`mcp.metrics` renders the `MCPConfig` (server key **`prometheus`**, no values
path — the key IS the `mcp__prometheus__*` namespace named in allowlists, so a
values rename would silently strip an agent's tools) plus its `MCPToolset`.
`mcpServers.metrics` deploys the workload. The two flip together and default
**off**, unlike `k8s-bundle`'s, because there is no endpoint to default onto: the
backend URL is operator-supplied by definition.

The endpoint guard is kept in the same shape as `k8s-bundle`'s: `mcp.enabled`
with no deployed server and no `url` fails the render loudly, because an
`MCPConfig` pointing nowhere costs agents their tools silently.

**The backend URL stays values-supplied and is never defaulted from the
backend's shape.** Single-node VM serves `/api/v1`; cluster mode (vmselect)
serves `/select/<accountID>/prometheus/api/v1`; Prometheus itself serves
`/api/v1` under whatever external URL it was given. No template can guess among
those, and guessing wrong produces a server that starts and answers nothing.

The server runs under its **own ServiceAccount**, never the runtime SA — the
same two-identity rule `k8s-bundle` enforces with a render failure. Unlike
`k8s-bundle` it needs no Kubernetes RBAC at all: it talks to an HTTP endpoint,
not the API server. So the SA exists for isolation and any future credential
projection, and the bundle renders no RBAC for it.

*Alternative considered:* keep both `victoriametrics` and `prometheus` keys, one
per backend. Rejected once the probe showed VM answering the Prometheus API —
two names for one capability, chosen by a fact the operator already encoded in
the URL.

### 3. The profile is identity-only, and it is what makes wiring legal

`alert-investigator` (values-configurable name), rendered by a `profile`
component defaulting **on**, in the exact shape of `k8s-engineer`: no
repository, no `allowedTools`, no `mcp`, an inline `systemPrompt`, `maxTurns`,
optional `runtimeRef`, and no substrate of any kind.

Because it has no repository, no agent definition file can be resolved for it,
so the inline role is not optional decoration — without it an alert wakes a
personality-free agent whose only inputs are an allowlist and an alert payload.
The role says: read the alert, query the metric that fired before concluding
anything, state the likely cause with its evidence, and be brief.

The bundle's old spec said the opposite — "the bundle ships no profile — the
alert-handling profile is operator-owned". That was true when the bundle could
not ship wiring. It is the reason it could not.

### 4. Wiring mirrors `k8s-bundle`, minus the demo exception

Same helper shape, same gates: `wiringActive` = bundle active AND the flag;
the template gated on `wiringActive` AND `profile.enabled`; `channelRefs`
emitted only when non-empty (key absent, never null); every ref omitted when the
component that renders it is off; the bundle label on the object.

`k8s-bundle.pipelines.enabled` is nullable for exactly one reason: an explicit
`false` has to beat demo mode's force-on. **Demo mode never enables this
bundle**, so that branch is inert here. The shape is kept identical anyway — one
rule across bundles, and it stays correct if a turnkey path ever reaches this
bundle — and the values comment states that demo does not apply, so nobody
concludes from the `null` that it might.

Only ONE route renders, not two. `k8s-bundle` splits observe/operate because its
toolset splits on cluster mutation; a metrics query server is read-only, so there
is no second posture to express.

### 5. Registration stays, and says what it is

`registration` keeps its current behavior verbatim — the adapter writes a
`VMAlertmanagerConfig` pointing at its own webhook, the bundle renders the
least-privilege Role/RoleBinding for the adapter's deterministic SA, and failure
degrades to instructions on the source's Ready condition rather than unserving
it.

What changes is only the framing: the values, the spec and the docs state that
this is **VictoriaMetrics-only and cannot be generalized**, because vanilla
Alertmanager's configuration is a file or a Secret rather than a CR — there is no
object for an adapter to write. Leaving it in a vendor-neutral bundle without
that sentence would read as a feature that merely has not been implemented yet.

Vanilla Alertmanager is served by `NOTES.txt` printing the receiver stanza it
needs, including `send_resolved: false` — the adapter drops non-firing alerts, so
a sender left at the default posts resolutions that are silently discarded, which
looks like an ingest bug from the sender's side.

### 6. Removals are loud, and the replacement is printed

Removing `mcp.vmlogs` costs a working install its log access, and Helm reports
nothing when a values key stops being read. The CHANGELOG therefore prints the
`MCPConfig` to hand-apply — the same object the bundle used to render — and the
toolset entry that goes with it, so restoring logs is a copy-paste rather than an
archaeology exercise.

The same applies to `mcp__victoriametrics__*` in allowlists: it stops resolving
rather than failing, which is precisely the allowlist-rot failure the project
names elsewhere. The upgrade steps name the query that finds affected Pipelines.

## Risks / Trade-offs

- **A silent no-op upgrade.** An install that upgrades without renaming its
  values key gets a bundle that renders nothing at all — Helm does not warn about
  unread values. → The parent chart FAILS the render when a `vm-bundle:` key is
  present, naming the new key, in the same shape as the `serviceAccounts.runtime`
  guard that already exists for a moved key. This is the single most important
  mitigation in the change.
- **Log access disappears.** → Guard above cannot catch it (the key is gone, not
  moved), so it is CHANGELOG-first with the replacement CR printed, and it is
  called out in the same entry as the rename so it cannot be read past.
- **Allowlists rot silently.** A Pipeline naming `mcp__victoriametrics__*` keeps
  rendering and quietly grants nothing. → Upgrade steps carry the `kubectl` query
  that lists affected Pipelines; the new toolset name differs, so the correct
  edit is visible in a diff.
- **The adapter CR default name changes.** Hand-written sources naming
  `adapter: vm-alertmanager` stop matching. → One documented override restores
  it; the render is already breaking, so it is one line among several rather than
  a surprise.
- **Wiring plus an install-declared claim fans out.** Two Ready Pipelines on one
  alert source open two conversations per alert. → The flag defaults off, and
  `NOTES.txt` reports the double claim. Not refused: sources are shareable and
  reinstating a conflict guard is a regression.
- **This change and `k8s-bundle-wiring` both touch bundle wiring.** →
  `k8s-bundle-wiring` is implemented and archives first; this change assumes its
  relaxed rule and adds no further relaxation, so there is no requirement to fold.

## Migration Plan

1. Land the subchart rename with templates unchanged, plus the parent-chart guard
   that fails a render still carrying a `vm-bundle:` key. Nothing else moves yet,
   so the rename is reviewable on its own.
2. Remove the logs component and replace the metrics component with the
   `prometheus`-keyed one and its deployable server.
3. Add the profile component, then the wiring component that depends on it.
4. `NOTES.txt`: the receiver stanza, the wiring report, the double-claim note.
5. Bump the parent chart minor and the subchart; write the CHANGELOG entry
   covering the key rename, the logs removal with its hand-apply CR, the
   `mcp__victoriametrics__*` breakage, the adapter-name default, and the new
   profile/wiring defaults.
6. Extend `internal/integration/charttemplate_test.go`; rename the spec directory
   and `docs/` page; update `CLAUDE.md`'s map and its wiring gotcha, which names
   `vm-bundle` as a bundle shipping no wiring.

**Rollback:** `prometheus-bundle.enabled: false` removes everything the bundle
renders. There is no rollback to the old values key short of pinning the previous
chart version — which is why the guard fails the render rather than letting the
upgrade proceed quietly.

## Open Questions

- **Which Prometheus MCP server image ships as the default?** The component needs
  a pinned, maintained image exposing PromQL query, range query, labels and
  series over HTTP or SSE. To be settled during implementation by checking what
  the candidate actually registers; the values shape (`image.repository`/`tag`,
  `backend`, `port`, `env`) does not depend on the answer, and `mcp.url` lets an
  operator point at their own server regardless.
- **Should the toolset enumerate tools or wildcard `mcp__prometheus__*`?**
  `k8s-bundle` enumerates because its tools span read and mutate and the split is
  the point. A query-only server has no such split, so a wildcard may be correct
  here — decided once the server's tool inventory is known.
