## Purpose
A Pipeline keeps a bounded number of runtime pods past the cold start, each held by a Conversation in phase `Reserved`, so a new conversation on that route begins at its first work unit rather than at pod creation.

## ADDED Requirements

### Requirement: A reservation is a Conversation in phase Reserved
`ConversationPhase` SHALL include `Reserved`: a Conversation the manager created
for a Pipeline ahead of any signal, carrying that Pipeline's materialised
snapshot (`profileRef`, `channelRefs`, toolsets, MCP configs, runtime,
service account, persistence) and `spec.pipelineRef`, with NO inputs, NO
signature label and NO title. Its runtime pod and MCP ConfigMap SHALL be
created through the ordinary path, owned by the Conversation, so the pod's
name, workspace directory and context directory are the conversation's own
and the runtime is already long-polling for that conversation's work.

While `Reserved` the manager SHALL NOT dispatch, SHALL NOT enqueue
`ensure-topic`, SHALL NOT count the conversation in the admission FIFO, and
signature reuse SHALL NOT match it.

#### Scenario: A reservation provisions a pod and nothing visible
- **WHEN** a Pipeline declares `warm: 1` and capacity allows
- **THEN** one `Reserved` conversation exists with a running runtime pod and its MCP ConfigMap, and no chat topic, run or input exists for it

#### Scenario: Reuse ignores reservations
- **WHEN** a signal arrives whose signature matches no open conversation and a `Reserved` conversation exists on its Pipeline
- **THEN** reuse finds nothing; adoption, not reuse, decides what happens next

### Requirement: The pool is bounded per Pipeline and by leftover capacity
`Pipeline.spec.warm` (integer, default 0) SHALL declare how many reservations
the Pipeline keeps. The manager SHALL hold at most `warm` reservations per
Pipeline and SHALL create one only while the number of live runtime pods,
reservations included, is below `MAX_ACTIVE_CONVERSATIONS`. Reservations SHALL
NEVER displace admission: a conversation waiting for a slot is served before
any refill. Refill SHALL wait a short delay after a slot frees, so a slot
released by an idle TTL is not filled and evicted within the same minute.

A Pipeline that is not Ready SHALL hold no reservations, and a Pipeline whose
`warm` is lowered or removed SHALL have its surplus reservations deleted.

#### Scenario: Refill fills leftover capacity only
- **WHEN** the cap is 5, four conversations hold pods and two Pipelines each declare `warm: 2`
- **THEN** at most one reservation exists across both Pipelines

#### Scenario: A waiting conversation beats a refill
- **WHEN** a slot frees while a conversation is `Pending` and a Pipeline is below its `warm` count
- **THEN** the Pending conversation is admitted and no reservation is created for that slot

#### Scenario: Lowering warm deletes the surplus
- **WHEN** a Pipeline with two reservations is edited to `warm: 0`
- **THEN** both reservations and their pods are deleted

### Requirement: A new conversation adopts the oldest reservation
When a signal would create a NEW conversation on a Pipeline holding at least
one `Reserved` conversation, ingest SHALL adopt the OLDEST one instead:
writing the signal's inputs, signature label, title and originating source
onto it and moving its phase out of `Reserved`. The adopted conversation SHALL
then proceed exactly as a freshly admitted one — topics ensured, work
dispatched to the pod it already holds. Adoption SHALL apply to every signal
kind that creates a conversation. Reuse of an existing conversation and the
resumption of an idle one SHALL NOT touch the pool.

#### Scenario: Chat message lands on a warm pod
- **WHEN** a bare chat message opens a new conversation on a Pipeline with a reservation
- **THEN** the conversation's runtime pod is the reservation's pod, already running, and the first work unit is dispatched without a pod being created

#### Scenario: Oldest first
- **WHEN** a Pipeline holds reservations R1 (older) and R2
- **THEN** the next new conversation adopts R1 and R2 remains reserved

#### Scenario: Fan-out adopts per Pipeline
- **WHEN** a source served by two Pipelines admits one signal and only one Pipeline holds a reservation
- **THEN** that Pipeline's conversation adopts it and the other Pipeline's conversation is created cold

### Requirement: Adopt and evict are serialised on the reservation itself
Adoption SHALL be a write conditional on the reservation's current resource
version, and eviction of a reservation SHALL be a deletion with a precondition
on the same. When both race, exactly one SHALL succeed; the loser SHALL re-read
and take the next reservation, or fall back to the cold path (a new object for
ingest, the next eviction candidate for the reconciler). A conversation that
has just been adopted SHALL never be deleted as a warm pod.

#### Scenario: Eviction loses to a concurrent adoption
- **WHEN** ingest adopts reservation R while the reconciler, serving another conversation, chooses R's pod to evict
- **THEN** either the adoption succeeds and the eviction is refused and retried against another candidate, or the eviction succeeds and ingest creates its conversation another way; R's inputs are never lost

### Requirement: An adopted warm pod still releases its slot on idle
A reservation's pod SHALL NOT exit on the runtime's idle clock while waiting.
Once adopted, the conversation SHALL release its pod when it has nothing
inflight and nothing queued for the runtime's effective `idleTTLMinutes`,
measured from its last activity, through a manager-side release that deletes
only the pod. `/exit` and cap eviction SHALL apply to it as to any pod.

#### Scenario: Reserved pod outlives the runtime TTL
- **WHEN** a reservation waits longer than the runtime's idle TTL
- **THEN** its pod is still running

#### Scenario: Adopted pod idles out
- **WHEN** an adopted conversation has answered and stays idle past the runtime's idle TTL
- **THEN** its pod is deleted and the conversation reports `Idle`

### Requirement: Reservations are visible and attributable
A `Reserved` conversation SHALL print its phase in `kubectl get` and in the
console, carry `spec.pipelineRef`, and be excluded from any listing that
counts or presents conversations a person opened.

#### Scenario: Reserved in the list
- **WHEN** an operator lists conversations
- **THEN** each reservation shows phase `Reserved` and the Pipeline it is held for
