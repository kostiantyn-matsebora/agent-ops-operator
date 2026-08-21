## MODIFIED Requirements

### Requirement: Unavailability is treated as an outage before it is treated as a loss
A run SHALL fail for an unavailable context only after bounded retries within the
run have been exhausted AND the system is not experiencing widespread
unavailability.

The manager SHALL track reports of unavailable context across conversations. When
they exceed a threshold within a window, it SHALL treat the cause as
infrastructure rather than as many simultaneous losses: affected inputs SHALL be
HELD for later dispatch rather than failed, dispatch of continuations SHALL pause,
and the state SHALL be reported. Normal dispatch SHALL resume once continuation
succeeds again, and the held work SHALL proceed with its context intact.

The manager SHALL treat a repeated FAILURE TO PROVISION a runtime pod as the same
evidence, where the failure is attributable to context storage — a volume that
will not attach or mount. A pod that never starts reports nothing, so a mechanism
fed only by run reports cannot see the most total form of the outage it exists
for: every conversation blocked, and no run alive to say so.

Failures NOT attributable to storage — an image that will not pull, a pod that
cannot be scheduled for resources — SHALL NOT count toward this threshold, so
that an unrelated fault cannot hold every conversation for a storage reason.

While the manager is treating storage as unavailable it SHALL NOT provision
runtime pods, and it SHALL re-test availability with a single probe rather than
by letting every waiting conversation retry.

This exists because failing fast on every report turns a recoverable
infrastructure incident into the permanent destruction of every active
conversation's context — a worse outcome than the silent degradation it replaces,
and an irreversible one. One conversation reporting an unavailable context is a
loss; every conversation reporting one at the same moment is an outage.

#### Scenario: A storage outage holds work rather than destroying it
- **WHEN** many conversations report an unavailable context within a short window
- **THEN** their inputs are held rather than failed, continuation dispatch pauses, and the reason is reported

#### Scenario: Pods that cannot start are the same outage
- **WHEN** several runtime pods in a row fail to start because their context volume will not attach
- **THEN** the manager treats storage as unavailable, stops provisioning, and holds work rather than failing it

#### Scenario: An unrelated provisioning failure does not hold work
- **WHEN** runtime pods fail to start because their image cannot be pulled
- **THEN** storage is not treated as unavailable and conversations are not held for that reason

#### Scenario: Recovery is probed once, not by everyone
- **WHEN** storage is being treated as unavailable
- **THEN** availability is re-tested by a single probe rather than by every waiting conversation attempting a pod

#### Scenario: Held work resumes with its context
- **WHEN** continuation succeeds again after such an outage
- **THEN** the held inputs are dispatched and continue the contexts they were always meant to

#### Scenario: An isolated loss is still a loss
- **WHEN** one conversation reports an unavailable context while others continue normally
- **THEN** that run fails, because the evidence points at that conversation rather than at the infrastructure

#### Scenario: A genuinely absent store does not queue forever
- **WHEN** unavailability persists and continuation never succeeds
- **THEN** the state remains reported rather than silently accumulating work with no explanation

## ADDED Requirements

### Requirement: A known-bad context volume costs continuity, not availability

When context storage is known to be unusable, the manager SHALL be able to start
a runtime pod WITHOUT its context volume rather than not at all, and SHALL mark
the affected conversation as having lost its context.

The conversation SHALL say so on its bound threads. It SHALL NOT silently answer
as though it had continued, which is the degradation the continuity rules exist
to prevent.

This SHALL NOT weaken the rule that a context which was promised and cannot be
reached fails the run. It applies where the storage is already established to be
unusable, and it exists so that a damaged filesystem stops the memory rather
than the service.

#### Scenario: A broken volume does not stop the system

- **WHEN** the context volume is established to be unusable and a conversation has pending input
- **THEN** a runtime pod starts without the context volume, the conversation is marked context-lost, and the answer says its memory was lost

#### Scenario: The loss is stated, never simulated

- **WHEN** a conversation runs without its context after such a loss
- **THEN** it states that it lost its context, rather than answering as if continuous

### Requirement: A conversation whose context is gone can be reset explicitly

An operator SHALL be able to reset a conversation whose context is
unrecoverable. The reset SHALL clear the recorded context handle, leave the
conversation and its threads intact, and cause the conversation to state that it
is continuing without its previous memory.

The reset SHALL be explicit and operator-initiated. It SHALL NOT be performed
automatically on a failed continuation, because an automatic version would be
indistinguishable from the silent degradation that the continuity rules forbid.

Without this, a conversation whose context is genuinely gone can only fail every
subsequent run or be deleted, and deleting a conversation to recover from a
storage fault destroys its history for an unrelated reason.

#### Scenario: An operator recovers a conversation after unrecoverable loss

- **WHEN** an operator resets a conversation whose context store was destroyed
- **THEN** the context handle is cleared, the conversation and its threads survive, and the next run starts fresh and says so

#### Scenario: Reset never happens on its own

- **WHEN** a run fails because its context could not be reached
- **THEN** the conversation is not reset automatically
