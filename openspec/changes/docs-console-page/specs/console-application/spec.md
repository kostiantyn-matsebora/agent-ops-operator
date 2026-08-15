## ADDED Requirements

### Requirement: The set of trusted identity headers is a published interface

The identity headers the console reads from a fronting proxy are the interface
between the console and whatever authenticates in front of it. That set SHALL be
treated as published: it SHALL be documented for an operator in full, in the
order the console prefers them, on the adopter-facing page that tells an operator
how to expose the console.

Changing the set — adding a header, removing one, or reordering preference —
SHALL be accompanied by the documentation change in the same change, because an
operator's proxy configuration is written from that list. A header the console
trusts but the documentation omits is a header an install will not strip, and a
client-supplied copy of it then reaches the console unopposed.

The console SHALL NOT gain a second, undocumented way to assert an identity.

#### Scenario: An operator configures a forward-auth proxy

- **WHEN** an operator configures a proxy to front the console
- **THEN** the documentation names every header the console will trust, so all
  of them can be set by the proxy and none accepted from a client

#### Scenario: A header is added to the trusted set

- **WHEN** the console is changed to read an additional identity header
- **THEN** the adopter-facing documentation of the set is updated in the same
  change

#### Scenario: The documented list is checked against the code

- **WHEN** the published list is compared to the console's own source
- **THEN** the two contain the same headers in the same order
