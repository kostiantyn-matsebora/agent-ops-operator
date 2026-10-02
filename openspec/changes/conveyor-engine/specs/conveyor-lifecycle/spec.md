## MODIFIED Requirements

### Requirement: The line's decisions are one machine that every program asks

The stations, the loops and every decision that moves them SHALL be defined
once, as a declared workflow every program and workflow that acts on the
line reads and runs through the conveyor engine, rather than restating any
part of it.

A workflow's transitions SHALL be complete for the states it declares. A
state paired with a trigger the workflow KNOWS either names a next state,
gated by zero or more guards, or is left undeclared and leaves the state
alone.

A trigger that no workflow declares anywhere is unknown and is an error. An
undeclared pairing of a known trigger is not.

A state label SHALL be written only by the engine evaluating a declared
transition, never by naming the value directly.

**Unreadable facts fail closed.** Where a guard's underlying read fails, the
guard SHALL return the answer that does not start new unattended work.

#### Scenario: A caller names an event the table skips

- **WHEN** a green CI arrives for a pull request whose loop workflow declares
  no transition for that trigger from the current state
- **THEN** the label is left as it is

#### Scenario: A caller tries to name a state

- **WHEN** the state writer is invoked with a state value instead of a
  recognized trigger
- **THEN** it refuses and writes nothing

#### Scenario: The fire records cannot be read

- **WHEN** a person places the standing instruction and the comments cannot be
  read
- **THEN** no session starts

#### Scenario: A guard's underlying read fails

- **WHEN** a person places the standing instruction and a guard's read of live
  state fails
- **THEN** no session starts
