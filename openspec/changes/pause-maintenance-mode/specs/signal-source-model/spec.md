## MODIFIED Requirements

### Requirement: SignalSource CRD splits shared metadata from type-specific config

The `SignalSource` CRD SHALL consist of type-agnostic metadata plus an
opaque `spec.config`:

| Field | Is |
|---|---|
| `spec.adapter` | required, immutable. The name of the serving `SignalAdapter` CR — a REFERENCE whose implementation defines and validates the sibling config. Replaced the open-string `spec.type` |
| `spec.grouping` | typed: signatureLabels, windowDays, cooldownHours |
| `spec.paused` | mutable bool, default `false` (`operational-pause-controls`) |
| `spec.credentialsSecretRef` | optional |
| `spec.config` | optional, opaque (`x-kubernetes-preserve-unknown-fields`). Only the serving signal implementation defines and validates its shape |

The operator SHALL never interpret `spec.config` semantically, and SHALL
never read the credential Secret's values (name-only projection).

When the serving `SignalAdapter` CR declares a config schema, the manager
SHALL mechanically validate `spec.config` against it and report the result
as an advisory `ConfigValid` condition. Admission still accepts arbitrary
config, and a violation never blocks serving or ingestion.

**The source SHALL carry no wiring.** `channelRef` and `profileRef` are
removed (BREAKING) — a `Pipeline` claim is the only way a source reaches a
profile and channels.

#### Scenario: Wiring fields no longer accepted
- **WHEN** a manifest sets `spec.channelRef` or `spec.profileRef` against the new CRD
- **THEN** the fields are pruned/rejected, and the documented migration (a Pipeline claiming the source) provides the routing

#### Scenario: Credentials declared on the source, materialized only by the kubelet
- **WHEN** a SignalSource sets `credentialsSecretRef: {name: pd-api-key}`
- **THEN** the operator references that Secret name in the serving adapter's pod spec without ever reading the Secret through the API

#### Scenario: Arbitrary config accepted for any adapter
- **WHEN** a SignalSource is applied with `adapter: pagerduty` and a `config` object the operator has never seen, and no SignalAdapter declares a schema
- **THEN** the API server accepts it and the operator stores the config without validation or interpretation, and no `ConfigValid` condition is set

#### Scenario: Adapter reference is required and immutable
- **WHEN** a SignalSource is applied without `spec.adapter`, or an existing one's `spec.adapter` is changed
- **THEN** the API server rejects the request with a validation error

#### Scenario: Schema violation surfaces as an advisory condition
- **WHEN** a SignalSource's `config` violates the schema its serving SignalAdapter declares
- **THEN** the API server still accepts the SignalSource, its status gains `ConfigValid=False` naming the violation, and `Served`/`Wired`/signal ingestion are unaffected

#### Scenario: The former type field is gone
- **WHEN** a SignalSource is applied carrying the removed `spec.type`
- **THEN** the field is not part of the schema and is pruned rather than honoured, so a stale manifest cannot silently select an adapter

#### Scenario: Pausing is a mutable field, unlike the adapter reference
- **WHEN** an existing SignalSource's `spec.paused` is changed from `false` to `true` or back
- **THEN** the API server accepts the change, unlike a change to the immutable `spec.adapter`

### Requirement: Unwired sources are visible and drop signals loudly

A SignalSource SHALL carry a `Wired` condition, True when at least one
Ready, NOT PAUSED `Pipeline` or `Coordinator` lists it, False otherwise.

The condition SHALL name ALL the claimants serving it, not the first. A
source several claimants watch fans its signals out to every one of them,
so "who answers here" is only readable if the condition says all of them.

Signals arriving for an unwired source SHALL NOT create conversations. The
ingest/inbound response SHALL state the reason explicitly.

**`Wired` is about CLAIMING, independent of the source's OWN `spec.paused`.**
A source claimed by a Ready, unpaused Pipeline reports `Wired=True`
whether or not the source itself is paused — a paused source still has a
claimant.

It simply drops what that claimant would otherwise receive, for the
source's own reason (see the ingest-time requirement below), not for lack
of a claimant.

A source claimed ONLY by paused claimants SHALL report `Wired=False`,
distinguishing a paused claimant from no claimant at all — so an operator
can tell a stopped route from one that was never wired.

The NUMBER of serving Ready, unpaused claimants is what an operator needs
to predict behaviour. For any source it is the number of conversations one
signal will produce.

For a CHAT source it additionally decides bare-message behaviour: one
claimant makes a bare message unambiguous and routable, several make it
ambiguous and answerable with the choices.

An operator SHALL be able to read that number from the condition rather
than by listing Pipelines and matching refs by hand.

#### Scenario: Unwired source reports itself
- **WHEN** a SignalSource exists that no Ready Pipeline references
- **THEN** its status shows `Wired=False`

#### Scenario: Signals for an unwired source are dropped with a reason
- **WHEN** a signal arrives for an unwired source
- **THEN** no conversation or input is created and the response carries queued 0 with an explicit not-wired reason

#### Scenario: Claim flips the condition
- **WHEN** a Ready Pipeline adds the source to its `signalSourceRefs`
- **THEN** the source's `Wired` condition becomes True naming that pipeline and subsequent signals route

#### Scenario: A source served by several pipelines names them all
- **WHEN** two Ready Pipelines list one source
- **THEN** the source reports `Wired=True` naming both, which is also what tells an operator that one signal there will open two conversations

#### Scenario: A chat surface served by several pipelines names them all
- **WHEN** two Ready Pipelines list one chat source
- **THEN** the source reports `Wired=True` naming both, which is also what tells an operator that bare messages there will be refused as ambiguous

#### Scenario: Pausing the source does not change Wired
- **WHEN** a SignalSource with a Ready, unpaused claimant is itself paused
- **THEN** its `Wired` condition stays `True`, and the reason its signals are
  dropped is reported separately as the source's own pause

#### Scenario: A source claimed only by a paused claimant reports Wired=False
- **WHEN** the only Ready Pipeline claiming a source is itself paused
- **THEN** the source reports `Wired=False`, with a reason naming that its
  only claimant is paused rather than that nothing claims it

## ADDED Requirements

### Requirement: A paused SignalSource drops its own signals before grouping or cooldown

A SignalSource with `spec.paused: true` SHALL drop every inbound signal
addressed to it, at the same early point the unclaimed-source check
already applies — before signature grouping, fingerprint cooldown, or
conversation lookup.

No conversation or input SHALL be created. Cooldown state SHALL NOT be
recorded for a signal dropped this way.

The response SHALL carry a reason naming the source's own pause, distinct
from an unwired-source reason and from a maintenance-mode reason.

**Order.** An install-wide `MaintenanceMode` drop (`maintenance-mode`) is
evaluated BEFORE this source-level check. Both can independently cause a
drop: maintenance mode first, the source's own pause second, the source's
`Wired` state third. The reported reason SHALL say which applied.

#### Scenario: A paused source drops its signals
- **WHEN** a signal arrives for a SignalSource with `spec.paused: true`
- **THEN** no conversation or input is created, and the response names the
  source's own pause as the reason

#### Scenario: Pausing does not spend cooldown
- **WHEN** two signals sharing one fingerprint arrive while the source is
  paused
- **THEN** neither is recorded against cooldown, and both would still route
  normally once the source is resumed and re-delivered

#### Scenario: Maintenance mode takes precedence over the source's own pause
- **WHEN** both maintenance mode is active and the signal's own source is
  paused
- **THEN** the response names maintenance mode as the reason, not the
  source's own pause

#### Scenario: Resuming lets subsequent signals route again
- **WHEN** a paused SignalSource is resumed
- **THEN** a signal delivered after the resume is evaluated exactly as it
  would be had the source never been paused — nothing dropped while paused
  is replayed on resume
