# state-durability — delta

## MODIFIED Requirements

### Requirement: Suppression windows survive a manager restart
Cooldown state keyed on `initiationId` SHALL be durable for the lifetime of its
window. The manager SHALL record suppression on the owning `SignalSource` and
SHALL load it before applying cooldown to a source after a restart.

Entries older than their window SHALL be pruned so the record stays bounded.

#### Scenario: Cooldown holds across a restart
- **WHEN** the manager restarts and an adapter re-delivers a signal whose `initiationId` is inside the source's cooldown window
- **THEN** the signal is suppressed and no duplicate conversation or input is created

#### Scenario: Expired entries do not accumulate
- **WHEN** suppression entries age past their window
- **THEN** they are removed from the `SignalSource` and the recorded state stays bounded
