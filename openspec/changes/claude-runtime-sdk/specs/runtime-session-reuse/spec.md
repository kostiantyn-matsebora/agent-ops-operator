## ADDED Requirements

### Requirement: A runtime may hold a vendor session open across work units
A runtime MAY keep its vendor session open between work units within one pod and deliver the next unit's prompt into it, rather than establishing a session per unit. This is an implementation freedom, not an obligation: a runtime that opens one session per unit remains correct, and SHALL NOT be treated as violating this capability.

A runtime that takes the freedom SHALL still honour the serial rule — one unit in flight per conversation — and SHALL satisfy every requirement below, which is the price of holding state the manager cannot see.

The reason it is worth taking is that a pod handles several units in its life — the serial rule is per conversation, not per pod — and re-establishing per unit re-reads the accumulated transcript and relaunches every bound MCP server. That cost grows with the conversation rather than staying constant, and an operator MAY raise `idleTtlMinutes` to hours, which turns several-units-per-pod from the exception into the normal case.

#### Scenario: A second unit lands in the open session
- **WHEN** a pod receives a second work unit for the same conversation while its session is still open and the unit's configuration is unchanged
- **THEN** the unit is delivered into that session, without re-establishing the session or relaunching its MCP servers

#### Scenario: A per-unit runtime is still correct
- **WHEN** a runtime establishes a fresh session for every work unit
- **THEN** it satisfies the work contract, and nothing the manager sends or records differs

### Requirement: A held session is a cache; the recorded handle stays authoritative
A held session SHALL NOT become a second source of truth. `runtimeContextId` on the conversation SHALL remain the only continuity record that outlives the pod, and every completed unit SHALL report the handle its vendor established — including units served from an already-open session, so a vendor-side branch still reaches the manager's latest-wins rule.

A pod holding no session — freshly started, evicted, rescheduled, or one whose process died — SHALL resume from the recorded handle. The manager SHALL NOT be able to tell the two paths apart: nothing in the work contract describes whether a session was held or re-established.

#### Scenario: A cold pod is indistinguishable
- **WHEN** a conversation's next unit is served by a pod with no open session
- **THEN** the runtime resumes from the recorded handle and reports an outcome identical in shape to one served from an open session

#### Scenario: A branch still reaches the manager
- **WHEN** a unit served from an open session ends in a different vendor session than it began in
- **THEN** the new handle is reported and recorded, rather than being hidden by the fact that the session was already open

#### Scenario: A dead process loses no durable state
- **WHEN** the runtime process dies mid-turn
- **THEN** the unit is reported failed, the recorded handle stands, and the next unit resumes from it

### Requirement: A session is reused only while the unit's configuration is unchanged
A held session SHALL be reused ONLY when the incoming unit's session-affecting configuration is identical to the one the session was opened with. When it differs, the runtime SHALL close the session and establish a new one continuing the recorded context.

This is what keeps reuse from silently defeating the rule that REFS are snapshotted while CONTENT is not: every use re-reads the referenced CRs, so an operator's edit is meant to heal a RUNNING conversation. A session pinned to the first unit's prompts, MCP servers or limits would keep applying superseded wiring while the CRs say otherwise.

Configuration that the runtime evaluates per invocation rather than pinning at session open — a tool allowlist enforced through a permission callback, for instance — SHALL NOT force a re-open, because it already changes with each unit.

#### Scenario: An edited MCP binding heals the running conversation
- **WHEN** a conversation's next unit carries different MCP servers from the ones its open session was established with
- **THEN** the runtime establishes a new session for that unit rather than serving it from the open one

#### Scenario: An edited role text heals the running conversation
- **WHEN** the profile's inline role text changes between two units of one conversation
- **THEN** the second unit runs under the new text

#### Scenario: A changed allowlist alone does not re-establish anything
- **WHEN** only the composed tool allowlist differs between two units, and the runtime enforces the allowlist per invocation
- **THEN** the open session serves the second unit under the new allowlist

#### Scenario: A re-open says why
- **WHEN** the runtime closes and re-establishes a session because the configuration changed
- **THEN** it logs which part of the configuration changed, so the reason is visible in the pod log

### Requirement: A long-lived session owns what a short-lived one never faced
A runtime holding sessions open SHALL bound what it keeps alive. It SHALL tear the session down — releasing the processes and connections it owns, including bound MCP servers — when the pod reaches its idle TTL, when the configuration changes, and when the session fails in a way the runtime cannot attribute to the individual unit.

A session SHALL NOT be held open across a failure the runtime does not understand: continuing to feed units into a session in an unknown state risks reporting outcomes produced under conditions nobody chose.

The accumulated conversation context is explicitly NOT in this class. Context is replayed on resume, so it grows the same whether a session was held open or re-established, and holding one open neither adds to it nor bounds it.

#### Scenario: Idle exit releases the session
- **WHEN** a pod reaches its idle TTL with a session open
- **THEN** the session and everything it owns are torn down before the process exits

#### Scenario: An unattributable failure ends the session
- **WHEN** the session fails in a way that cannot be attributed to the unit being served
- **THEN** the runtime ends the session rather than serving the next unit from it

#### Scenario: Holding a session open does not change context growth
- **WHEN** a conversation runs many units through one held session
- **THEN** its accumulated context is what it would have been had each unit resumed from the recorded handle
