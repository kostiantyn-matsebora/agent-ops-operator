## REMOVED Requirements

### Requirement: The VM bundle ships as a self-gated subchart, off by default and independent of demo mode
**Reason**: The subchart is renamed to `prometheus-bundle`. Its ingest core
accepts the standard Alertmanager webhook payload from any sender and its metrics
path speaks the Prometheus HTTP query API, so naming the capability after one
vendor described a general capability as a specific one. The capability does not
survive the rename under two names.

**Migration**: Superseded by the `prometheus-bundle` requirement "The bundle
ships as a self-gated subchart named for the protocol, not a vendor", which keeps
the default-off and demo-independent gating verbatim and adds a render guard that
FAILS when a retired `vm-bundle:` key is still present.

### Requirement: The alertmanager component packages the adapter with its webhook Service
**Reason**: Carried over to `prometheus-bundle` with two corrections. The
requirement described `defaultSource.profileRef` and "the Pipeline claiming it",
neither of which any template rendered — the same spec-versus-template
contradiction resolved in `k8s-bundle`. And the adapter CR name, which is the
routing key, defaulted to a vendor name in a bundle that is no longer about that
vendor.

**Migration**: Superseded by the `prometheus-bundle` requirement "The ingest
component packages the adapter with its webhook Service". The claim on the source
now belongs to the wiring component or the install, never to the ingest
component. The adapter CR name defaults to `alertmanager`; installs with
hand-written sources naming `vm-alertmanager` restore the old value with
`prometheus-bundle.alertmanager.name: vm-alertmanager` rather than editing every
source.

### Requirement: MCP components ship vmlogs and vmmetrics as referencable MCPConfig CRs
**Reason**: Two vendor-keyed components collapse to one protocol-keyed component,
and logs leave the bundle entirely. VictoriaMetrics answers the Prometheus HTTP
query API and reports a Prometheus version on `buildinfo`, and MetricsQL is a
PromQL superset, so the `victoriametrics` key was a second name for a capability
the `prometheus` key already covers. VictoriaLogs speaks LogsQL over its own
endpoints, which no Prometheus server can query, so there was nothing to
generalize.

**Migration**: Metrics are superseded by the `prometheus-bundle` requirement "One
metrics MCP component, keyed for the query API it speaks" — point its URL at the
same endpoint and replace `mcp__victoriametrics__*` in every Pipeline allowlist
with `mcp__prometheus__*`; the old namespace stops resolving rather than failing,
so it must be found and edited deliberately. Logs have no replacement in the
chart: apply the `MCPConfig` by hand (server key `victorialogs`, the same URL the
bundle used) together with an `MCPToolset` granting `mcp__victorialogs__*`, and
bind both from the Pipeline that needs them. The CHANGELOG prints both objects.

### Requirement: MCP components can optionally deploy the MCP server workloads
**Reason**: The deployable workloads were the VictoriaMetrics MCP server images,
one of which is redundant against a Prometheus server and the other of which
serves a query language leaving the bundle.

**Migration**: Superseded by the `prometheus-bundle` requirement "The metrics MCP
server workload is deployable under its own identity", which keeps the
defaults-onto-the-deployed-Service behavior and the loud failure for an enabled
component with no backend, adds the own-ServiceAccount rule, and drops the logs
workload. Installs that were deploying `mcp-victorialogs` from the bundle deploy
it themselves.

### Requirement: The bundle ships a ready-made observability toolset
**Reason**: The toolset tracked which vendor-keyed components were enabled. With
one metrics component there is one namespace to grant, and the logs namespace is
no longer the bundle's to grant.

**Migration**: Superseded by the toolset described in the `prometheus-bundle`
requirement "One metrics MCP component, keyed for the query API it speaks".
Pipelines referencing `vm-observability` must reference the new toolset name; a
reference to a toolset nobody renders grants nothing silently.

### Requirement: The bundle ships no profile — tool wiring targets the operator's own Pipeline
**Reason**: Reversed deliberately. Shipping no profile is what made the bundle
unable to ship wiring, since a subchart may render a Pipeline only when that
Pipeline renders with its own profile — so an install got an ingest path that
dropped every alert at `Wired=False` until the operator supplied both a profile
and a route.

**Migration**: Superseded by the `prometheus-bundle` requirements "The bundle
ships an alert-investigator profile" and "The wiring component ships one claiming
Pipeline, off by default". Installs with their own alert-handling profile are
unaffected: the profile component can be turned off, the wiring component
defaults off, and a route declared in the parent chart's `pipelines:` keeps
working unchanged.

### Requirement: The registration component wires the in-cluster VMAlertmanager declaratively
**Reason**: Kept, but restated in the renamed capability so that its
VictoriaMetrics-only nature is normative rather than incidental. Inside a
vendor-neutral bundle, a registration feature with no such statement reads as one
merely awaiting implementation for other senders.

**Migration**: Superseded by the `prometheus-bundle` requirement "Sender
self-registration is a VictoriaMetrics-only sub-feature and says so", which keeps
every behavior — the target reference, the Role and RoleBinding, the `register`
block, the loud failure without a target, and degradation to instructions rather
than an unserved source — and adds the requirement to state that vanilla
Alertmanager cannot be registered because its configuration is a file rather than
a Kubernetes object.
