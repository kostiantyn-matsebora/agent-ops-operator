## Purpose

This change depends on `coordinated-agents` landing first. The
`coordination-loop` spec cited here is defined there and is not yet archived.

The reaper: an ordinary `AgentProfile`/`AgentCapability` pair, shipped by
coordinator mode itself, addressed hourly by a claimed cron signal, that
surveys its own Coordinator's open roots and closes the ones it judges
healed — reusing `invoke` and member-result routing unchanged.

## ADDED Requirements

### Requirement: The reaper is an ordinary agents[] entry, not a new kind

Coordinator mode SHALL render one `AgentProfile` and one `AgentCapability`
for the reaper, and SHALL list the reaper's capability as one more entry in
the chart-rendered Coordinator's `agents[]`, alongside the domain bundles'
entries.

No new CRD SHALL exist for the reaper. It SHALL be reached, invoked and
bounded by exactly the mechanisms `coordinator-model` and `coordination-loop`
already define for any `agents[]` entry.

#### Scenario: The reaper is a capability like any other

- **WHEN** coordinator mode renders
- **THEN** the reaper's `AgentCapability` is one entry in the Coordinator's
  `agents[]`, described like every other entry, and the Coordinator object
  itself carries no reaper-specific field

#### Scenario: No new CRD ships for the reaper

- **WHEN** the chart renders under coordinator mode
- **THEN** the CRDs it installs are unchanged from pipelines mode plus the
  ones `coordinated-agents` already added — no reaper-specific kind exists

### Requirement: The hourly signal reaches the reaper through the coordinating agent's own invoke

Coordinator mode SHALL claim a `signals/cron` source on an hourly schedule,
on the SAME chart-rendered Coordinator that claims every enabled bundle's
source. Claiming SHALL use `Coordinator.spec.signalSourceRefs`, the one
field every claim already uses.

The cron source SHALL NOT be treated specially. No per-entry trigger field
SHALL exist on any `agents[]` entry.

Each admitted cron signal SHALL open an ordinary conversation running the
Coordinator's OWN coordinating agent — the same capability that opens for
any other signal on any other claimed source.

That agent's prompt SHALL instruct it to recognise an hourly cron signal and
respond by `invoke`-ing the reaper's `agents[]` entry, exactly as it invokes
a domain entry in response to a domain signal.

The survey this file's other requirements describe runs inside that invoked
reaper conversation, a member of the cron-triggered root.

No new field, verb or claiming rule is introduced by this requirement. The
cron signal is routed to the reaper by the coordinating agent's own
judgement — the same judgement that already routes every other signal to
the `agents[]` entry that should handle it.

Where coordinator mode is off, no cron source for the reaper SHALL render.

#### Scenario: The reaper's schedule is an ordinary cron claim

- **WHEN** coordinator mode renders
- **THEN** a `signals/cron` source scheduled hourly is claimed on the
  chart-rendered Coordinator's `signalSourceRefs`, exactly as any other
  claimed source is — counted in `Wired`, fanned out like any other
  claimant

#### Scenario: An hourly signal opens the coordinating agent, which invokes the reaper

- **WHEN** the cron source's hourly signal is admitted
- **THEN** a root conversation running the Coordinator's own coordinating
  agent opens, and that agent invokes the reaper's `agents[]` entry as an
  ordinary member, which carries out the survey

#### Scenario: pipelines mode ships no reaper schedule

- **WHEN** the chart renders under `pipelines` mode
- **THEN** no cron source for the reaper is rendered

### Requirement: The reaper holds no domain tools

The reaper's `AgentCapability` SHALL bind no toolset and no `MCPConfig`
beyond the reach `coordinator-owner-reach` grants through the aops MCP
server. It SHALL NOT bind any bundle's Kubernetes, metrics or Home Assistant
toolset.

Its privilege SHALL be scoped to the Coordinator-owner reach class
(listing and closing its own Coordinator's open roots) plus its own `invoke`
bound, the `agents[]` entries that Coordinator already lists.

#### Scenario: The reaper cannot reach a domain tool directly

- **WHEN** the reaper's conversation runs
- **THEN** its work unit's allowlist carries no bundle-specific toolset, and
  it can reach a domain agent's capability only by invoking it through the
  Coordinator's `agents[]`, never by calling that agent's tools itself

### Requirement: The reaper surveys, re-invokes the original agent, and closes healed roots

On each hourly run, the reaper's conversation SHALL list its own
Coordinator's open root conversations (the Coordinator-owner reach class,
`coordinator-owner-reach`), and for each one SHALL `invoke` the SAME
domain `AgentCapability` entry named in that root's `members` field
(`coordinator-owner-reach`), asking it to re-check current state.

Where `members` lists more than one entry, the reaper SHALL re-invoke EACH
entry in turn. The root counts as healed only when every re-check reports
the condition cleared.

Where a `members` entry names a nested Coordinator (a `coordinatorRef`
entry in `agents[]`) rather than an `AgentCapability`, the reaper SHALL
`invoke` that entry the same way. The nested Coordinator's own coordinating
agent then re-checks through its own `agents[]`, and its result counts as
that entry's re-check.

The re-check SHALL be an ordinary `invoke` and the result SHALL reach the
reaper's own conversation through the unchanged member-result routing
(`coordination-loop`) — no new routing path is introduced.

Once the re-check's result indicates the incident no longer holds, the
reaper SHALL `close` that root through the Coordinator-owner reach class,
never through the ordinary `close` bound (caller or direct member) since a
survived root is neither.

A root the re-check still finds unhealthy SHALL be left open.

#### Scenario: A healed root is closed

- **WHEN** the reaper re-invokes the original agent on an open root and the
  result reports the condition cleared
- **THEN** the reaper closes that root, naming the re-check as the reason

#### Scenario: A still-broken root is left open

- **WHEN** the reaper re-invokes the original agent on an open root and the
  result reports the condition still present
- **THEN** the reaper takes no close action on that root, and it remains
  open for the next hourly survey or for escalation

#### Scenario: The re-check reuses invoke and member-result routing unchanged

- **WHEN** the reaper invokes a domain agent capability to re-check a root
- **THEN** the invoke is bounded by the reaper's own Coordinator's
  `agents[]` list exactly as any invoke is, and the domain agent's result
  reaches the reaper's conversation as an ordinary member-result input

#### Scenario: A member conversation is never surveyed directly

- **WHEN** the reaper lists open conversations
- **THEN** it receives only UNCAUSED roots of its own Coordinator, never a
  member conversation, per the Coordinator-owner reach class's own bound

### Requirement: Domain agent profiles carry a self-close instruction that applies only where a thread is bound

Every bundle profile's system prompt SHALL end with an instruction that the
agent MAY end its own conversation by replying `/close` once it is confident
the problem is resolved.

The instruction SHALL be the same in pipelines mode and coordinator mode,
since both reference the same `AgentProfile`.

The instruction SHALL state that it applies only where a thread is bound to
the conversation. It SHALL NOT claim a capability a threadless conversation
lacks.

`/close` here is the ordinary surface command a person types. It carries no
reason, and the rule that a Coordinator root's close must carry one governs
the MCP `close` verb alone.

This requirement is the single home of the instruction. The `k8s-bundle`,
`ha-bundle` and `prometheus-bundle` deltas state no copy of it, because each
bundle's profile inherits it from here and a second statement would drift.

A coordinator-mode member conversation binds no channel of its own, so the
instruction does nothing there. Ending a member is the reaper's and the
coordinating agent's job.

#### Scenario: A threaded conversation may end itself

- **WHEN** a domain agent's conversation has a thread bound and the agent
  judges the problem resolved
- **THEN** its prompt permits it to reply `/close`, handled by the ordinary
  reply-path command

#### Scenario: A threadless member has nothing to close through

- **WHEN** a domain agent runs as a coordinator-mode member with no thread
- **THEN** the instruction does nothing and the agent leaves closing to
  whatever invoked it
