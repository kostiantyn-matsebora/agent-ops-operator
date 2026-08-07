# signal-adapter-lifecycle — delta

## MODIFIED Requirements

### Requirement: Unserved signal types are visible
A `SignalSource` whose `spec.type` is not claimed by a Ready `SignalAdapter` (nor adapter-reported Ready) SHALL carry `Served=False`; it SHALL flip True when a serving adapter appears — same reason vocabulary as Channel's Served condition. There are no built-in signal types: every type needs an adapter. `kubectl get signalsources` SHALL surface useful state (type, received count) without dead columns.

#### Scenario: Typo'd type is diagnosable
- **WHEN** a SignalSource is created with `type: pagerdutty` and nothing serves it
- **THEN** the source shows `Served=False` instead of silently never producing conversations

#### Scenario: No type is served without an adapter
- **WHEN** a SignalSource uses `type: alertmanagerWebhook` after the built-in removal and no adapter claims that type
- **THEN** it shows `Served=False` — the manager itself serves nothing
