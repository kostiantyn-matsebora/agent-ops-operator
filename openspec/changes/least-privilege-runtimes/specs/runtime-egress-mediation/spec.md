## REMOVED Requirements

### Requirement: Egress mediation is opt-in per AgentRuntime and absent by default
**Reason**: The heading states the default this change inverts, and its scenarios
assert it. Mediation is enabled by default now, so "absent by default" and "an
install that does not ask for it is unchanged" are both false.

What the requirement protected — that a runtime declaring nothing gets no added
containers — survives as the OPT-OUT below, which is the same property read from
the other side.

## ADDED Requirements

### Requirement: Egress mediation is on by default and declinable per runtime
Egress mediation SHALL be ENABLED BY DEFAULT. The wall that constrains an
uncooperative agent SHALL NOT be something an operator has to discover.

`--allowedTools` configures a COOPERATING agent. One with a shell can open a
socket to a bound MCP server and call anything that server registers, so the
allowlist is a configuration rather than a boundary. Shipping the boundary off by
default left every install with the weaker of the two until someone read far
enough to find it.

IT SHALL REMAIN DECLARABLE PER RUNTIME, and a runtime SHALL be able to decline
it — a vendor reaching no MCP server has nothing to mediate, and a proxy there is
cost without a boundary.

**THE COST SHALL BE NAMED WHERE THE DEFAULT IS MET.** Mediation adds a privileged
init container requiring `NET_ADMIN`, which a namespace under `restricted` Pod
Security admission REFUSES — so the install fails at POD ADMISSION rather than at
render, far from the setting responsible. The post-install notes SHALL state
that, and SHALL name the value that turns it off.

#### Scenario: A fresh install is mediated
- **WHEN** the chart is installed with no egress values supplied
- **THEN** runtime pods carry the proxy and the redirect, and the tool access their wiring granted is enforced where the agent does not control it

#### Scenario: A runtime that needs no proxy declines it
- **WHEN** a runtime declares mediation disabled
- **THEN** its pods carry the same containers, environment and security context an unmediated pod carries, with nothing added

#### Scenario: Enabling it needs no image change
- **WHEN** mediation applies to a runtime whose image is a derived, adopter-built one
- **THEN** its conversations are mediated without rebuilding that image

#### Scenario: The admission cost is stated, not discovered
- **WHEN** the chart is installed
- **THEN** the notes state that mediation adds a privileged init container which a `restricted` Pod Security namespace refuses, and name the value that disables it
