## MODIFIED Requirements

### Requirement: The line's decisions are one machine that every program asks

The stations, the loops and every decision that moves them SHALL be defined
once, as a declared workflow every program and workflow that acts on the
line reads and runs through the conveyor engine, rather than restating any
part of it.

A workflow's transitions SHALL be complete for the states it declares. A
state paired with a trigger either names a next state, gated by zero or more
guards, or is left undeclared and leaves the state alone.

A trigger outside a workflow's own declared vocabulary is an error.

A state label SHALL be written only by the engine evaluating a declared
transition, never by naming the value directly.

**Unreadable facts fail closed.** Where a guard's underlying read fails, the
guard SHALL return the answer that does not start new unattended work.

#### Scenario: A caller names a trigger the table skips

- **WHEN** a green CI arrives for a pull request whose loop workflow declares
  no transition for that trigger from the current state
- **THEN** the label is left as it is

#### Scenario: A caller tries to name a state

- **WHEN** the state writer is invoked with a state value instead of a
  recognized trigger
- **THEN** it refuses and writes nothing

#### Scenario: A guard's underlying read fails

- **WHEN** a person places the standing instruction and a guard's read of live
  state fails
- **THEN** no session starts
