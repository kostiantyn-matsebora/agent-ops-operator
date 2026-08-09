# ai-signal-triage

## ADDED Requirements

### Requirement: Agent triage is opt-in and returns one of three verdicts
A `SignalSource` MAY opt in to agent-decided triage. When enabled, the manager SHALL ask an agent for a verdict at the deduplication seam and SHALL act on one of **create**, **attach** to a named candidate Conversation, or **drop** with a reason. The single verdict covers semantic deduplication and importance filtering together.

Triage SHALL be off by default.

#### Scenario: Semantically identical signals share one conversation
- **WHEN** triage is enabled and a signal describing the same incident as an open Conversation arrives with labels that hash to a different signature
- **THEN** the verdict may attach it to that Conversation, which no label hash could have done

#### Scenario: An unimportant signal is dropped with a reason
- **WHEN** triage is enabled and the agent judges a signal not worth opening an investigation
- **THEN** no Conversation is created and the reason is recorded

#### Scenario: Triage disabled changes nothing
- **WHEN** a source has not opted in
- **THEN** the deterministic strategy decides and no agent is consulted

### Requirement: Triage executes as a toolless work unit on the default AgentRuntime
The triage agent SHALL execute through the existing work contract on the `default` AgentRuntime — the established namespace fallback used when a profile names no runtime. The manager SHALL NOT call any model directly.

This follows from an invariant, not a preference: the manager reads no Secrets and therefore holds no model credentials. Every model call in the system happens inside a runtime pod, which receives credentials as `valueFrom` resolved by the kubelet.

The triage unit SHALL run with an **empty allowlist** and no MCP servers. It reads text and returns a verdict; it requires no repository, no cluster access, and no tools. It SHALL bind no channels, so no triage output is posted to any surface — the verdict is read from the run's result.

#### Scenario: Triage needs no credentials in the manager
- **WHEN** triage is enabled
- **THEN** the manager performs no Secret reads and issues no model API calls; the model call happens in the runtime pod

#### Scenario: Triage cannot act on anything
- **WHEN** a triage unit executes
- **THEN** its allowlist is empty and it has no MCP servers, so no tool call is available to it regardless of the content it reads

#### Scenario: Triage posts nothing to a channel
- **WHEN** a triage unit completes
- **THEN** no send op is enqueued to any Channel, because the triage host binds none

### Requirement: The model is consulted last and only for would-be new conversations
Triage SHALL run only after self-exclusion, fingerprint cooldown, signature grouping, window reuse, and the creation cap have all been applied. A signal that attaches to an existing Conversation or is refused by the cap SHALL NOT consult the agent.

Verdicts SHALL be cached by signature for a bounded period, so a problem flapping between novel signatures is asked about once rather than once per flap.

This ordering bounds cost to the rate of NOVEL problems rather than to event volume.

#### Scenario: An event storm does not become a model-call storm
- **WHEN** hundreds of signals arrive that collapse into existing conversations or exceed the creation cap
- **THEN** no triage verdict is requested for any of them

#### Scenario: A repeated novel signature is asked once
- **WHEN** the same signature reaches the seam repeatedly within the cache period
- **THEN** the cached verdict is reused

### Requirement: Triage fails open
When the triage agent is unavailable, times out, returns an unparsable verdict, or names a Conversation outside the supplied candidate set, the manager SHALL **create** the Conversation.

A missed incident is worse than a surplus one. Failing open is safe only because the creation cap is independent and unconditional: an unavailable triage agent during a storm degrades to capped default behavior, never to unbounded creation.

The triage timeout SHALL be short relative to how long an operator would tolerate not being told about an incident.

#### Scenario: Unavailable triage degrades to default behavior
- **WHEN** no runtime is available to execute the triage unit, or the unit does not complete within the timeout
- **THEN** the Conversation is created as the deterministic strategy would have created it

#### Scenario: A malformed verdict is not obeyed
- **WHEN** the agent returns text that does not parse as a verdict
- **THEN** the Conversation is created and the parse failure is recorded

### Requirement: Every drop is auditable
The manager SHALL record recent triage verdicts, including the reason text for every drop, on the `SignalSource` status in a bounded record. A drop SHALL NOT be recorded without a reason.

A drop is the only verdict that leaves no Conversation, no input, and nothing to inspect. Unrecorded, it makes "why was I not paged" unanswerable, which would make the feature unsafe to enable at all.

#### Scenario: A suppressed incident can be explained after the fact
- **WHEN** an operator asks why a known problem produced no Conversation
- **THEN** the source's status carries the verdict and the agent's stated reason

#### Scenario: The record is bounded
- **WHEN** many verdicts accumulate
- **THEN** only the most recent are retained, so the status object does not grow without limit

### Requirement: Triage never triages itself
Signals whose subject is the triage lane SHALL take the deterministic path and SHALL NOT be triaged. A triage failure SHALL NOT produce a signal; it SHALL be reported as a condition.

The triage agent runs in a pod, and a failing pod emits events — the same cycle the signal self-exclusion invariant exists to break. agent-ops' own health is status, never signal.

#### Scenario: A failing triage pod creates no conversation
- **WHEN** the triage unit's pod cannot start or repeatedly fails
- **THEN** the failure is reported as a condition, no signal is produced, and no Conversation is created about it

#### Scenario: Triage is not asked to judge itself
- **WHEN** a signal concerns the triage lane
- **THEN** the deterministic strategy decides it
