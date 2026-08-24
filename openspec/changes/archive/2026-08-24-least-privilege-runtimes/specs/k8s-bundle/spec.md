## MODIFIED Requirements

### Requirement: The events component packages the adapter with its access
When active, the `eventsAdapter` component SHALL render: the `SignalAdapter` CR (default name `k8s-events` — the routing key SignalSources select with `spec.adapter` — values-configured image, singleton, and `serviceAccountName` NAMING the account this component renders); the adapter's ServiceAccount `agentops-signal-<name>` RENDERED BESIDE ITS GRANT — the operator creates no account and binds no RBAC, so the identity and what it may do are read from one file — carrying `events` `get`/`list`/`watch` AND `pods`/`replicasets` `list`/`watch` (ClusterRole by default, namespaced Role via `rbac.clusterWide: false`, none via `rbac.create: false`); and, when `source.create` is on, a `SignalSource` naming that adapter with `severities` defaulting to `["Warning"]` and values-configurable `namespaces` and `grouping`. The claim on that source belongs to the wiring component or to the install — never to this one, so an events lane can be rendered for a route declared anywhere. The manager SHALL NOT create or require any RBAC verbs on roles or rolebindings.

The pods/replicasets grant is read-only and exists because the adapter resolves workload identity through owner references and re-checks liveness before emitting. Where the events grant is namespaced, the pods/replicasets grant SHALL be namespaced identically — the adapter never reads more broadly than it watches.

The rendered source's default `grouping.signatureLabels` SHALL be `["namespace", "workload"]`, and its default `rules` SHALL be calibrated against both failure modes at once: they SHALL NOT open a conversation for ordinary rollout churn, and they SHALL NOT lose an actionable incident. Specifically the shipped defaults SHALL drop only pure-bookkeeping reasons whose underlying problem another undropped reason still reports, SHALL assign `for: 0` to reasons describing a completed event and to node-level conditions, SHALL assign longer dwells with breadth escalation to the known-flappy reasons, and SHALL end in a catch-all dwell so unanticipated reasons are verified rather than discarded.

#### Scenario: A healthy rollout produces no conversation
- **WHEN** the bundle renders with default values and a ten-replica Deployment rolls out normally, emitting probe and scheduling warnings on pods that then become Ready or terminate
- **THEN** no conversation is created

#### Scenario: A broken rollout produces exactly one conversation
- **WHEN** the same Deployment is rolled out with an unpullable image
- **THEN** exactly one conversation is created for the workload, carrying every contributing reason with its occurrence count

#### Scenario: The events lane renders without a claim
- **WHEN** the events component is active and neither the wiring component nor the install claims its source
- **THEN** the adapter, its RBAC and the source render, and the source reports `Wired=False` and drops its signals

#### Scenario: One values flag yields flowing events
- **WHEN** the chart is installed with `global.demo.enabled=true` and the LLM credential Secret exists
- **THEN** Warning events in the cluster produce conversations executed by the k8s-engineer profile without building images or applying extra manifests, because demo mode turns the wiring component on alongside this one

#### Scenario: The rendered source is always claimed
- **WHEN** the events component renders a SignalSource while the wiring component is active
- **THEN** that component's Pipeline claims it, so signals route instead of dropping with `Wired=False` — the claim comes from the wiring component or the install, never from this one, which is what lets an events lane be rendered for a route declared anywhere

#### Scenario: Event-driven conversations are equipped
- **WHEN** an event routes through whichever Pipeline claims the source
- **THEN** the resulting work unit carries a non-empty allowlist, because that Pipeline declares its own toolsets

#### Scenario: Namespace-scoped events RBAC
- **WHEN** `eventsAdapter.rbac.clusterWide=false`
- **THEN** only a namespaced Role/RoleBinding renders, covering events, pods and replicasets alike, and the adapter can watch only in the release namespace

#### Scenario: A default install groups by workload
- **WHEN** the bundle renders with default values and a ten-replica Deployment crash-loops through several rollouts
- **THEN** the rendered source's `signatureLabels` are `["namespace", "workload"]` and one conversation covers the workload rather than one per pod

#### Scenario: Default rules ship, not an empty filter
- **WHEN** the bundle renders with default values
- **THEN** the rendered source carries a non-empty `rules` list, so rollout churn is suppressed without the installer writing any configuration
