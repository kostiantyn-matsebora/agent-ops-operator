## Purpose

This change depends on `coordinated-agents` landing first. The
`conversation-provenance` and `coordination-loop` specs cited here are
defined there and are not yet archived.

The Coordinator-owner reach class on the aops MCP server: a caller acting
for its own Coordinator may list and close that Coordinator's OPEN ROOT
conversations — scoped to one Coordinator, roots only, never a member and
never another Coordinator's tree.

## ADDED Requirements

### Requirement: A new verb lists a Coordinator's own open roots

The server SHALL expose `list_open_roots()`, returning the UNCAUSED
conversations — no `causedBy` — that carry `spec.coordinatorRef` naming the
SAME Coordinator the calling conversation ACTS FOR (below).

A conversation that is itself a member (carries `causedBy`) SHALL NOT
appear in the result, whatever Coordinator it carries as its own
`coordinatorRef`.

The caller's OWN ANCESTOR root — the uncaused root its `causedBy` chain
leads to — SHALL also be excluded. It is itself an uncaused root of the
caller's own Coordinator, but closing it would cascade to close the
caller's own conversation before the caller finishes running.

`list_open_roots` SHALL return each root's name, title, brief and phase —
the same projection shape `list_conversations` already returns — plus
`members`, the `agents[]` entry names of the root's direct members. The
reaper re-invokes those entries. Never a transcript or a run.

#### Scenario: Only uncaused roots of the caller's own Coordinator are listed

- **WHEN** a caller invokes `list_open_roots`
- **THEN** it receives every open conversation with no `causedBy` whose
  `coordinatorRef` names the caller's own Coordinator, and nothing else

#### Scenario: A member root is excluded even when open

- **WHEN** one of the caller's Coordinator's members is itself an open
  Coordinator's root (nested) but carries `causedBy` from its own parent
- **THEN** `list_open_roots` does not include it, because it is not an
  UNCAUSED root

#### Scenario: A closed root is excluded

- **WHEN** one of the caller's Coordinator's roots has already reached phase
  `Closed`
- **THEN** `list_open_roots` does not return it

#### Scenario: The caller's own ancestor root is excluded

- **WHEN** the caller is a member whose `causedBy` chain leads to an
  UNCAUSED root of its own Coordinator
- **THEN** `list_open_roots` excludes that root, because closing it would
  cascade to close the caller's own conversation mid-run

### Requirement: The caller must itself be acting for a Coordinator

`list_open_roots` and the widened `close` (the Coordinator-owner bound on
`close` in `aops-mcp-server`) SHALL be refused for a
caller that resolves to no Coordinator by the walk below — an ordinary
Pipeline-addressed conversation has no access to this reach class at all,
whatever AgentCapability it runs.

#### Scenario: A Pipeline-addressed conversation is refused

- **WHEN** a conversation opened through a Pipeline calls `list_open_roots`
- **THEN** the walk below resolves no Coordinator, the manager refuses the
  call, and the server has made no decision

### Requirement: The manager resolves which Coordinator a caller acts for by walking causedBy

A caller's own conversation carries `coordinatorRef` ONLY when it is itself
a Coordinator's uncaused root, per `conversation-provenance`. An ordinary
`capabilityRef` member — which is what the reaper is, invoked as a plain
member of the cron-triggered root — carries no `coordinatorRef` of its own.

The manager SHALL resolve which Coordinator a caller acts for the SAME way
the invoke-time cycle guard already does (`coordination-loop`'s cycle
check). It SHALL read the caller's own `coordinatorRef` first.

If empty, it SHALL walk the caller's `causedBy` chain to the UNCAUSED root
and read that root's `coordinatorRef` instead. This is a read of existing
fields, never a new one.

The manager SHALL decide whether a target root belongs to that resolved
Coordinator by comparing it against `spec.coordinatorRef` on the target
root — `conversation-provenance`'s existing field, read and compared, never
a new field.

#### Scenario: A member resolves its Coordinator through its uncaused root

- **WHEN** the reaper's conversation (a plain member, `causedBy` naming the
  cron-triggered root, no `coordinatorRef` of its own) calls
  `list_open_roots`
- **THEN** the manager walks `causedBy` to the uncaused root, reads that
  root's `coordinatorRef`, and resolves the reaper as acting for that
  Coordinator

#### Scenario: Reach is decided by comparing the resolved Coordinator

- **WHEN** the caller resolves to Coordinator `incident-coord` and a
  candidate root also carries `coordinatorRef: incident-coord`
- **THEN** the manager treats that root as belonging to the caller's own
  Coordinator

#### Scenario: A different Coordinator's root is invisible

- **WHEN** the caller resolves to Coordinator `incident-coord` and a
  candidate root carries `coordinatorRef: billing-coord`
- **THEN** that root is excluded from `list_open_roots` and refused as a
  `close` target

### Requirement: A Coordinator-owner reach call is refused across Coordinators

A call targeting a root belonging to a DIFFERENT Coordinator than the
caller's own SHALL be refused by the manager, regardless of the caller's
tool allowlist.

#### Scenario: Cross-Coordinator close is refused

- **WHEN** a caller acting for Coordinator A asks to close a root whose
  `coordinatorRef` names Coordinator B
- **THEN** the manager refuses it, and the server has made no decision

### Requirement: A Coordinator-owner reach call is refused against a member

A call targeting a conversation that carries `causedBy` — a member, however
shallow — SHALL be refused, even where that member's `coordinatorRef` (if
set, for a nested Coordinator's own root) names the caller's own
Coordinator.

This reach class SHALL NEVER reach a member. Closing a member the caller did
not directly cause remains the ordinary `close` bound's province
(`conversation-close`, `aops-mcp-server`), unaffected by this class.

#### Scenario: A member within the caller's own tree is still refused

- **WHEN** a caller asks to close a conversation that carries `causedBy`,
  even though that conversation's `coordinatorRef` names the caller's own
  Coordinator
- **THEN** the manager refuses it as out of scope for this reach class

#### Scenario: A nested Coordinator's own root is not a target either

- **WHEN** a caller lists open roots and one of its Coordinator's members is
  itself a nested Coordinator's root
- **THEN** that member is absent from `list_open_roots`, because it carries
  `causedBy` from the invoking parent
