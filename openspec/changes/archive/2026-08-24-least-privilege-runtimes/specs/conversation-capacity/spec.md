## MODIFIED Requirements

### Requirement: Idle runtime pods release capacity within a minute by default
The manager-wide runtime idle TTL (`RUNTIME_IDLE_TTL_M`, chart
`global.agentops.runtimeDefaults.idleTtlMinutes`) SHALL default to **1 minute**.

It SHALL live in the runtime DEFAULTS rather than at parent scope, because a
bundle-shipped runtime can read no parent scope but `global.` — a top-level key
rendered an EMPTY field, and the CRD's structural default of 10 silently
replaced the release's setting.
`AgentRuntime.spec.idleTTLMinutes` SHALL continue to override it per runtime. An
idle runtime pod whose conversation has nothing inflight and nothing queued MAY
still be evicted before its TTL to admit waiting work; eviction SHALL delete only
the pod, leaving the conversation and its session intact so it resumes on its
next input.

The same release SHALL also be reachable on demand, by `/exit` in a
conversation's own thread, for the case automatic eviction cannot serve: nothing
is waiting, so nothing evicts, while the pod goes on holding its slot, its
checkout and whatever its runtime keeps resident until the TTL expires. Installs
that raise the TTL — to avoid re-cloning a large repository or re-warming a
local model on every message — are exactly the ones where that wait is longest.

Both paths SHALL be the same release: the same "nothing inflight and nothing
queued" predicate, defined ONCE and shared; delete only the pod; the
conversation resumes on its next input. Neither SHALL delete a conversation,
archive a thread, or discard queued input.

#### Scenario: Default idle TTL reaches the pod
- **WHEN** a runtime pod is created with no `AgentRuntime` override and no configured TTL
- **THEN** its `RUNTIME_IDLE_TTL_M` environment variable is `1`

#### Scenario: Per-runtime override still wins
- **WHEN** an `AgentRuntime` sets `idleTTLMinutes: 15`
- **THEN** pods for conversations using that runtime receive `RUNTIME_IDLE_TTL_M=15`

#### Scenario: Eviction does not end the conversation
- **WHEN** an idle conversation's pod is evicted to admit waiting work and a new input arrives afterwards
- **THEN** the conversation still exists, gets a fresh runtime pod, and resumes its session
#### Scenario: A slot is released with nothing waiting
- **WHEN** a conversation is finished with, no other conversation is waiting for capacity, and a person sends `/exit` in its thread
- **THEN** its runtime pod is deleted and the slot is free immediately, without waiting out the idle TTL
- **AND** the conversation still exists and resumes on its next input

#### Scenario: The two release paths agree
- **WHEN** the condition for releasing an idle pod is evaluated by automatic eviction and by `/exit`
- **THEN** both use the same predicate, and a conversation releasable by one is releasable by the other
