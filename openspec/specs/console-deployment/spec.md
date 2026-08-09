# console-deployment Specification

## Purpose
TBD - created by archiving change visualize-agent-ops. Update Purpose after archive.
## Requirements
### Requirement: Console ships as an opt-in chart bundle of CRs
The console SHALL ship as a chart bundle of CRs and RBAC — a ChannelAdapter, a Channel, an externally-served SignalAdapter, a SignalSource, the UI token Secret, and a read-only Role/RoleBinding — and SHALL be **enabled by default** (`console.enabled: true`). The chart SHALL ship no workload or connectivity of its own: the ChannelAdapter reconciler owns the Deployment and, because `spec.port` is set, the Service.

Setting `console.enabled: false` SHALL remove the CRs and with them the Deployment, pod and Service; Channels naming `adapter: console` SHALL report `Served=False`, conversations SHALL keep their other threads, and no other component SHALL be affected.

Because this default changes from `false`, an upgrade starts a workload that was not previously running and that reads every `agentops.dev` CR in the namespace. The chart major SHALL be bumped and `CHANGELOG.md` SHALL carry the migration entry naming the one-value opt-out.

#### Scenario: Default install has a console
- **WHEN** the chart is installed at defaults
- **THEN** the console pod runs and is reachable on its ClusterIP Service

#### Scenario: Enabling the console is CRs-only
- **WHEN** the console is enabled and the release upgraded
- **THEN** the ChannelAdapter/Channel/SignalAdapter/SignalSource/Secret are applied and the reconcilers bring up the console workload and Service with no chart-owned workload objects

#### Scenario: Opting out is clean
- **WHEN** `console.enabled=false` is applied to a running install
- **THEN** the Deployment, pod and Service are removed, console-served Channels report `Served=False`, and every other pipeline keeps delivering

#### Scenario: Disabling is non-destructive to conversations
- **WHEN** `console.enabled` is flipped to false
- **THEN** the console workload and Service are removed, referencing Channels report `Served=False`, and existing Conversations keep their other channel threads

### Requirement: Read-only RBAC granted by the chart, not the operator
The chart SHALL grant SA `agentops-adapter-console` a namespaced read-only Role covering `get`, `list`, `watch` on every `agentops.dev` kind **and on `deployments` and `pods`** — the latter because versions, image digests, restart counts and manager health exist in no CR. No write verb SHALL be granted on any resource, and no reconciler SHALL create or bind RBAC.

The trust boundary SHALL be documented: whoever holds the UI token can read everything this ServiceAccount can read, conversation payloads included, and — when writes are enabled — can instruct any agent the console can reach.

#### Scenario: Read-only in fact
- **WHEN** the console's Role is inspected
- **THEN** it contains no write verb on any resource

#### Scenario: Console SA can watch, not write
- **WHEN** access for SA `agentops-adapter-console` is checked
- **THEN** `list`/`watch` on pipelines/conversations succeed and any write verb or Secret read is denied

#### Scenario: A broken pod is visible
- **WHEN** a component is in CrashLoopBackOff
- **THEN** the console reports it, having read pod state directly

### Requirement: Browser access is authenticated
Read access SHALL require a bearer token or a session established from it, sourced from the console Channel's projected `uiToken` credential. An unconfigured token SHALL authorize nobody and SHALL be indistinguishable from a wrong token.

Write actions — replying in a conversation and originating one — SHALL additionally require `console.write.enabled` and a resolved identity, taken from a trusted forward-auth header when present and recorded as the token identity otherwise. Every write SHALL be logged with that identity. The Service SHALL remain `ClusterIP` and Ingress disabled by default; OIDC via forward-auth SHALL be documented as the answer for any Ingress exposure.

#### Scenario: Unconfigured is closed, not open
- **WHEN** no token is configured
- **THEN** every authenticated route is refused, and failures do not reveal whether a token exists

#### Scenario: Viewer-only deployment
- **WHEN** `console.write.enabled=false`
- **THEN** the composer and the new-conversation action are absent and both endpoints reject requests server-side

#### Scenario: No anonymous access
- **WHEN** a browser requests any console page or API without the token (or session established from it)
- **THEN** the console responds 401 / login prompt and serves no topology, CR, or conversation data

### Requirement: Wiring the console into pipelines is explicit
Neither the chart nor the console SHALL mutate a Pipeline. Observing and replying to conversations started elsewhere SHALL require adding the console Channel to that Pipeline's `channelRefs[]`; originating conversations SHALL require adding the console SignalSource to a Pipeline's `signalSourceRefs[]`. In both cases the UI SHALL report the unwired state with its condition reason and print the exact patch.

Conversations the console itself started SHALL need no such edit — the originating channel is appended by the router, so they arrive already joined.

#### Scenario: Origination requires a claim
- **WHEN** no Pipeline claims the console SignalSource
- **THEN** the source reports `Wired=False`, origination is unavailable, and the UI shows the patch that would claim it

#### Scenario: Self-started work needs no wiring edit
- **WHEN** a user starts a conversation from the console
- **THEN** it is joined and replyable without any Pipeline edit

#### Scenario: Unjoined pipeline shows join instructions
- **WHEN** a user views a pipeline whose channels do not include the console Channel
- **THEN** the console displays the `channels[]` addition needed to join it, and performs no mutation itself

### Requirement: The console runs as its own single-replica component
The console SHALL run as its own Deployment, pod, ServiceAccount and image, separate from the manager, so that a console failure cannot affect signal ingest, dispatch or channel delivery. It SHALL run `replicas: 1` with strategy `Recreate`, because it holds the channel op loop and browser SSE connections, both of which a second replica would split.

Holding two adapter identities SHALL NOT produce two workloads: the SignalAdapter is externally served and owns none.

#### Scenario: Console failure does not reach dispatch
- **WHEN** the console pod crashes or is OOM-killed
- **THEN** signal ingest, dispatch and channel delivery continue unaffected

#### Scenario: Two identities, one pod
- **WHEN** the console's ChannelAdapter and its externally-served SignalAdapter are both present
- **THEN** exactly one Deployment exists

### Requirement: Chart values keep their prefix and gain origination and write controls
Values SHALL keep the `console.*` prefix and their existing meanings where they still apply (`name`, `channelName`, `image`, `port`, `auth`, `ingress`, `resources`), and SHALL add `console.write.enabled` (default `true`), `console.signalSourceName` (default `console`), and `console.metrics.url` (default empty — the optional metrics backend for historical windows).

An existing install SHALL migrate by image tag alone: no CR surgery, and Pipelines already listing the console channel SHALL keep working.

#### Scenario: Migration is a tag change
- **WHEN** an install running the previous console upgrades
- **THEN** no CR is edited by hand, and pipelines already listing the console channel keep delivering to it

