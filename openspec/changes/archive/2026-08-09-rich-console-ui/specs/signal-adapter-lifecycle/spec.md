# signal-adapter-lifecycle (delta)

## ADDED Requirements

### Requirement: A SignalAdapter may be served by another adapter's workload
`SignalAdapterSpec.image` SHALL become optional, and `SignalAdapterSpec.servedBy` (`{kind, name}`, where `kind` is `ChannelAdapter`) SHALL be added. When `servedBy` is set, the SignalAdapter reconciler SHALL create NO Deployment, Service or ServiceAccount, and SHALL report `Ready=True` with reason `ServedBy`. Exactly one of `image` or `servedBy` SHALL be set.

This exists so an implementation that is both a surface and an originator — a chat transport, or the console — runs as ONE pod holding two identities, instead of a second Deployment whose only purpose is to make a SignalSource `Served`.

#### Scenario: An externally-served adapter owns no workload
- **WHEN** a SignalAdapter sets `servedBy: {kind: ChannelAdapter, name: console}` and omits `image`
- **THEN** no Deployment, Service or ServiceAccount is created for it, and it reports `Ready=True` with reason `ServedBy`

#### Scenario: SignalSources it serves still resolve
- **WHEN** a SignalSource names an externally-served SignalAdapter in `spec.adapter`
- **THEN** the source reports `Served=True` exactly as it would for an adapter that owns a workload

#### Scenario: The modes are mutually exclusive and reversible
- **WHEN** both `image` and `servedBy` are set
- **THEN** the CR is rejected by validation
- **WHEN** `servedBy` is removed and `image` supplied
- **THEN** the reconciler creates the workload, so the adapter returns to self-served operation

#### Scenario: A dangling servedBy is diagnosable
- **WHEN** `servedBy` names a ChannelAdapter that does not exist
- **THEN** the SignalAdapter reports `Ready=False` with a reason naming the missing adapter, rather than silently reporting ready

### Requirement: The serving pod receives the signal-adapter identity
When a SignalAdapter names a ChannelAdapter in `servedBy`, the ChannelAdapter reconciler SHALL inject `SIGNAL_ADAPTER_TOKEN` into that adapter's pod, derived as `HMAC-SHA256(masterKey, "signal-adapter:"+<signal adapter name>)`, base64url — the same derivation the SignalAdapter reconciler uses today. Nothing SHALL be minted, stored, or read back from a Secret, and the two identities SHALL remain distinct: each contract surface validates only against its own CRD list.

#### Scenario: One pod holds two distinct tokens
- **WHEN** ChannelAdapter `console` is the `servedBy` target of SignalAdapter `console`
- **THEN** its pod carries both `ADAPTER_TOKEN` (channel identity) and `SIGNAL_ADAPTER_TOKEN` (signal identity), and the two values are not equal

#### Scenario: Each token opens only its own surface
- **WHEN** the channel token is presented to a `/signal/*` route, or the signal token to a `/channel/*` route
- **THEN** the request is rejected with 401

#### Scenario: Removing the link removes the identity
- **WHEN** the SignalAdapter is deleted or its `servedBy` cleared
- **THEN** `SIGNAL_ADAPTER_TOKEN` is removed from the serving pod on the next reconcile
