## ADDED Requirements

### Requirement: A shipped route claims the console so it can originate

Where the bundle renders a route at all, that route SHALL claim the console's
signal source and bind the console as a channel, so an install that deploys the
console can start a conversation in it without further wiring.

Both SHALL be values-supplied NAMES read from `global.` — the only parent scope a
subchart can read — for objects the bundle does not itself render, and both SHALL
be omitted when the parent names none. The channel SHALL be merged with any
channels the operator named rather than replacing them.

The claim SHALL ride the route the bundle already ships rather than adding a
second one: two routes claiming the console source would make every unaddressed
console message ambiguous, since a bare chat message with more than one claimant
is refused.

#### Scenario: Turnkey install can be used from the console

- **WHEN** the chart is installed with its turnkey flag and the console enabled
- **THEN** the rendered route claims the console's signal source and binds the
  console channel, and the console's composer is available

#### Scenario: The console is not deployed

- **WHEN** the console is disabled and the parent names no console source or
  channel
- **THEN** the route claims only the bundle's own source and binds no console
  channel

#### Scenario: An operator named their own channel

- **WHEN** the operator names a channel for the route and the console is enabled
- **THEN** both are bound, and neither replaces the other

#### Scenario: One route, not two

- **WHEN** the console source is claimed
- **THEN** it is claimed by the route the bundle already renders, so an
  unaddressed console message still has exactly one claimant
