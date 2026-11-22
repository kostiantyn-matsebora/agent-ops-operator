## MODIFIED Requirements

### Requirement: The agents list is the whole outbound reach

`spec.agents[]` SHALL be a list of entries, each `{name, description}` plus
EITHER `capabilityRef` OR `coordinatorRef`, mutually exclusive by CEL. The
Coordinator SHALL be able to invoke exactly the objects this list names and no
other object of any kind.

- A `capabilityRef` entry invokes an AgentCapability as a plain member.
- A `coordinatorRef` entry NESTS. The invoked Coordinator's own root opens as
  the member, and the member's own `coordinatorRef` (`conversation-provenance`)
  is set to name that invoked Coordinator.
- `description` SHALL be required and non-empty, on the entry rather than the
  target, so two Coordinators may describe one differently.
- An entry MAY carry `workspace` (`own` or `shared`, default `own`) and
  `repoContext` (`load` or `omit`, default `load`). Both are properties of the
  entry, so two Coordinators may invoke one capability differently. Their
  behaviour is specified by `member-workspace-sharing` and `member-repo-context`.

#### Scenario: An entry without a description is refused
- **WHEN** an `agents[]` entry omits `description`
- **THEN** the API server rejects the manifest

#### Scenario: An entry naming both refs is refused
- **WHEN** an `agents[]` entry carries both `capabilityRef` and `coordinatorRef`
- **THEN** the API server rejects the manifest

#### Scenario: An entry naming neither ref is refused
- **WHEN** an `agents[]` entry carries neither `capabilityRef` nor `coordinatorRef`
- **THEN** the API server rejects the manifest

#### Scenario: Invoking outside the list fails
- **WHEN** the coordinating agent asks to invoke an AgentCapability or Coordinator its own Coordinator does not list
- **THEN** the request is refused naming the Coordinator, and no conversation is created

#### Scenario: The two new fields default to today's behaviour
- **WHEN** an entry omits `workspace` and `repoContext`
- **THEN** its members own their workspace directory and load the checkout's context

#### Scenario: Two Coordinators invoke one capability differently
- **WHEN** Coordinator A lists capability `dev` with `workspace: shared` and Coordinator B lists it with no `workspace`
- **THEN** members opened through A share their owner's tree and members opened through B own theirs

### Requirement: Readiness names every member that is not

`Ready` SHALL be False, naming each of them, when any listed `capabilityRef` or
`coordinatorRef` does not resolve, or resolves to an object whose own `Ready`
is False. A Coordinator that is not Ready SHALL claim nothing.

`Ready` SHALL also be False, naming the entry, when an entry declares
`workspace: shared` and the workspace claim the Coordinator's own root would
resolve is absent, because an ephemeral workspace cannot be shared.

Resolution SHALL track the Coordinators already visited on the current path
and detect a STATIC `coordinatorRef` cycle — one reachable with no
conversation involved. A Coordinator whose own name reappears on that path
SHALL be `Ready=False` naming the cycle, rather than recursing.

This is a separate guard from the `invoke`-time one below, which walks a
live conversation's `causedBy` chain instead of the static ref graph.

#### Scenario: A dangling member
- **WHEN** an `agents[]` entry names an AgentCapability or Coordinator that does not exist
- **THEN** the Coordinator's `Ready` is False with the entry's name in its message
- **AND** signals on its sources are not routed to it

#### Scenario: Two Coordinators list each other
- **WHEN** Coordinator A's `agents[]` names a `coordinatorRef` to B, and B's own `agents[]` names one back to A
- **THEN** both A and B are `Ready=False` naming the cycle, and neither claims anything

#### Scenario: A shared entry with no workspace claim
- **WHEN** an entry declares `workspace: shared` and the Coordinator's root would resolve no workspace claim
- **THEN** `Ready` is False with the entry's name in its message, and its sources are not routed to it

#### Scenario: A shared entry with a claim
- **WHEN** an entry declares `workspace: shared` and the Coordinator's root resolves a workspace claim
- **THEN** `Ready` is not affected by the entry
