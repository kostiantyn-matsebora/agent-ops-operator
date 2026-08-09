# signal-creation-cap

## ADDED Requirements

### Requirement: A source's Conversation creation is capped
`SignalSource.spec.grouping` SHALL carry a maximum number of NEW Conversations the manager may create for that source within a fixed window. When the cap is reached, the manager SHALL NOT create further Conversations for that source until the window rolls, and SHALL report a `Throttled` condition on the source naming how many creations were refused and when the window ends.

The cap SHALL be deterministic and SHALL NOT depend on any other feature — not on the dedup strategy in use, not on whether agent triage is enabled, and not on whether either is functioning. It is the floor that makes every other decision in this capability safe to fail.

Accounting MAY be in-memory: the manager is a singleton under leader election, and a restart resets at most one window's allowance, matching the tolerance already documented for fingerprint cooldown.

An unset cap SHALL mean no cap, so existing installations are unaffected until a value is set.

#### Scenario: A runaway stops creating conversations
- **WHEN** a source would create more Conversations within its window than the cap allows
- **THEN** creation stops for the remainder of the window and the source reports `Throttled=True` with the refused count

#### Scenario: An unset cap changes nothing
- **WHEN** a source declares no cap
- **THEN** Conversation creation is unbounded, exactly as before this capability existed

#### Scenario: The window rolls
- **WHEN** a throttled source's window ends and a new signal arrives whose signature has no live conversation
- **THEN** a Conversation is created and `Throttled` returns to False

### Requirement: Throttling never blocks attachment to an existing conversation
A throttled source SHALL continue appending inputs to Conversations that already exist, including recurrence on a live session. Only creation is capped.

The failure mode being prevented is unbounded object creation, not the loss of information about a problem already under investigation. Suppressing updates to a live incident would silence it precisely as it escalates.

#### Scenario: An escalating incident keeps reporting while throttled
- **WHEN** a source is throttled and a new signal's signature matches a Conversation inside the reuse window
- **THEN** the input is appended to that Conversation as usual, as a recurrence when a session exists

#### Scenario: Throttling is visible, not silent
- **WHEN** creation is refused by the cap
- **THEN** the refusal is reported on the source's status and is not merely logged
