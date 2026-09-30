## Purpose

This change depends on `coordinated-agents` landing first. The Coordinator
and AgentCapability kinds cited here are defined there and are not yet
archived.

The `global.agentops.wiringMode` chart posture: whether an enabled bundle
renders an inline Pipeline per route, or a standalone AgentCapability
gathered under one chart-rendered Coordinator — a release-wide rendering
choice with no API-server exclusivity behind it.

## ADDED Requirements

### Requirement: wiringMode is a release-wide rendering posture with two values

`global.agentops.wiringMode` SHALL accept exactly two values, `pipelines` and
`coordinator`, and SHALL default to `pipelines`.

When the value is absent, the mode is `pipelines`, except that
`global.demo.enabled: true` selects `coordinator`.

The value SHALL decide only how the chart RENDERS each enabled bundle's
route. It SHALL NOT be per-bundle: one value governs every bundle the
release enables.

#### Scenario: The default is today's posture

- **WHEN** a release sets no value for `global.agentops.wiringMode`
- **THEN** the chart renders as though `pipelines` were set

#### Scenario: Demo mode selects coordinator explicitly

- **WHEN** `global.demo.enabled` is true and the release sets no value for
  `global.agentops.wiringMode`
- **THEN** the chart renders as though `coordinator` were set, and the
  default in `values.yaml` remains `pipelines`

#### Scenario: An unrecognized value fails the render

- **WHEN** `global.agentops.wiringMode` is set to a value that is neither
  `pipelines` nor `coordinator`
- **THEN** the render fails naming the value and the two it accepts

#### Scenario: The posture applies release-wide

- **WHEN** a release sets `global.agentops.wiringMode: coordinator` with two
  bundles enabled
- **THEN** both bundles render their coordinator-mode branch — neither
  renders an inline Pipeline, and no per-bundle value overrides the choice

### Requirement: pipelines mode renders byte-identical to a release with no wiringMode set

Under `global.agentops.wiringMode: pipelines`, every object the chart
renders — from the parent chart and from every bundle — SHALL be identical
to the objects the same values render with `global.agentops.wiringMode`
entirely absent and `global.demo.enabled` false.

No new object — no `Coordinator`, no `AgentCapability`, no reaper
`AgentProfile` — SHALL render under `pipelines` mode.

#### Scenario: Setting the default value explicitly changes nothing

- **WHEN** a release with `global.demo.enabled` false renders once with
  `global.agentops.wiringMode` unset and once with it explicitly set to
  `pipelines`, all other values equal
- **THEN** the two renders produce byte-identical manifests

#### Scenario: No coordinator-mode object leaks into pipelines mode

- **WHEN** the chart renders under `pipelines` mode with every bundle enabled
- **THEN** no `Coordinator`, `AgentCapability`, or reaper `AgentProfile`
  appears in the output

### Requirement: coordinator mode renders one Coordinator claiming every enabled bundle's source

Under `global.agentops.wiringMode: coordinator`, the chart SHALL render
exactly one `Coordinator` object. That Coordinator SHALL claim, in its
`signalSourceRefs`, the signal source of every enabled bundle that would
otherwise have rendered a Pipeline — the same sources those bundles'
`pipelines` mode routes claim today.

The Coordinator SHALL also claim the `signals/cron` source of the reaper
(`coordinator-self-heal`), which belongs to no bundle.

Each enabled bundle SHALL contribute one `agents[]` entry per route it would
render in `pipelines` mode, each entry's `capabilityRef` naming that
bundle's own rendered `AgentCapability` at that route's existing privilege
level. A bundle shipping two routes at two privilege levels SHALL contribute
two entries, never merged into one.

#### Scenario: One bundle enabled

- **WHEN** coordinator mode is set with exactly one bundle enabled and wired
- **THEN** the rendered Coordinator claims that bundle's source and lists
  exactly the `AgentCapability` entries that bundle's own rendering branch
  produces, plus the reaper's own entry and the reaper's cron claim

#### Scenario: Two bundles enabled

- **WHEN** coordinator mode is set with two bundles enabled and wired
- **THEN** the rendered Coordinator claims both bundles' sources directly,
  and its `agents[]` lists one entry per route either bundle would have
  rendered as a Pipeline

#### Scenario: A two-privilege bundle keeps both levels separate

- **WHEN** coordinator mode is set with a bundle that ships two routes at two
  privilege levels
- **THEN** that bundle renders two `AgentCapability` objects, and the
  Coordinator's `agents[]` carries two entries, one per capability, never one
  merged entry

### Requirement: The mode switch carries no API-server exclusivity between Pipeline and Coordinator

`global.agentops.wiringMode` SHALL control chart rendering only. It SHALL
NOT introduce any CRD validation, CEL rule or admission check that makes a
`Pipeline` and a `Coordinator` mutually exclusive, whether on one object, one
release or one cluster.

An operator MAY hand-write both a `Pipeline` and a `Coordinator` claiming the
same source in the same cluster, whatever `wiringMode` the chart last
rendered with. The API server SHALL accept both, exactly as it does today
with two Pipelines claiming one source.

#### Scenario: A hand-wired Pipeline coexists with coordinator mode

- **WHEN** the chart renders under `coordinator` mode and an operator applies
  a hand-written `Pipeline` claiming the same source the chart-rendered
  Coordinator claims
- **THEN** the API server accepts the Pipeline, and the source fans out to
  both claimants exactly as any shared source does

#### Scenario: A hand-wired Coordinator coexists with pipelines mode

- **WHEN** the chart renders under `pipelines` mode and an operator applies a
  hand-written `Coordinator`
- **THEN** the API server accepts it, and nothing in the chart's own
  rendering refuses or reports on the coexistence

### Requirement: A bundle's coordinator-mode branch follows the four bundle-wiring conditions

A bundle's coordinator-mode branch SHALL hold all four conditions the
parent chart's bundle-wiring rules set for a bundle shipping its own routes:

1. Rendering is behind an explicit wiring flag.
2. Every reference to an object the bundle does not itself render is a
   values-supplied name, omitted when unset.
3. Each `AgentCapability` renders only with its own profile.
4. The flag defaults off, forced on by nothing but a turnkey-install values
   path, and then only the least-privileged route.

#### Scenario: A bundle branch without its flag renders nothing

- **WHEN** coordinator mode is set and a bundle's wiring flag is off
- **THEN** that bundle renders no `AgentCapability` and contributes no
  `agents[]` entry

#### Scenario: No guard compares the two kinds

- **WHEN** a cluster holds both chart-rendered and hand-written objects of
  both kinds
- **THEN** no controller, webhook or CEL rule in this system evaluates
  whether a Pipeline and a Coordinator claiming one source are in conflict
