## MODIFIED Requirements

### Requirement: Simultaneously active conversations are capped
A conversation SHALL be considered **active** while a runtime pod exists for it
(pod phase `Running` or `Pending`). A conversation whose pod has exited is not
active and SHALL NOT consume capacity.

Active conversations SHALL be counted in TWO independent pools:

| Pool | Holds | Limit | Chart value | Default |
|---|---|---|---|---|
| conversations | every conversation with no `causedBy` | `MAX_ACTIVE_CONVERSATIONS` | `maxActiveConversations` | 5 |
| members | every conversation an `invoke` created (`causedBy` set) | `MAX_ACTIVE_MEMBERS` | `maxActiveMembers` | 5 |

The limits SHALL be counted from live runtime pods rather than from conversation
status, so a stuck pod or a lost status patch cannot inflate available capacity.
A pod whose conversation cannot be read SHALL be counted in the conversations pool.

When `MAX_ACTIVE_CONVERSATIONS` is unset and the deprecated `MAX_RUNTIMES` is set,
the manager SHALL honor the deprecated value for the conversations pool and log
that it is deprecated.

#### Scenario: Sixth conversation is not admitted
- **WHEN** five conversations each hold a runtime pod and a signal opens a sixth
- **THEN** no sixth runtime pod is created and the sixth conversation reports phase `Pending`

#### Scenario: Idle conversation frees its slot
- **WHEN** an active conversation's runtime pod exits after its idle TTL
- **THEN** the conversation reports phase `Idle` and no longer counts against the cap

#### Scenario: Deprecated alias is honored
- **WHEN** the manager starts with the deprecated `MAX_RUNTIMES=8` and no `MAX_ACTIVE_CONVERSATIONS`
- **THEN** the effective conversations cap is 8 and a deprecation notice is logged

#### Scenario: The deprecated alias does not size the members pool
- **WHEN** the manager starts with the deprecated `MAX_RUNTIMES=8`, no `MAX_ACTIVE_CONVERSATIONS` and no `MAX_ACTIVE_MEMBERS`
- **THEN** the conversations cap is 8 and the members cap is 5

#### Scenario: A full conversations pool does not block members
- **WHEN** five conversations hold pods and a Coordinator root's `invoke` creates a member
- **THEN** the member is admitted and gets a runtime pod

#### Scenario: A full members pool does not block conversations
- **WHEN** five members hold pods and a signal opens a new conversation
- **THEN** the conversation is admitted and gets a runtime pod

#### Scenario: Sixth member is not admitted
- **WHEN** five members hold runtime pods and a sixth member is created
- **THEN** no sixth member pod is created and the member reports phase `Pending`

#### Scenario: Both pools default to five
- **WHEN** the chart is installed with neither `maxActiveConversations` nor `maxActiveMembers` set
- **THEN** the manager runs with `MAX_ACTIVE_CONVERSATIONS=5` and `MAX_ACTIVE_MEMBERS=5`

## ADDED Requirements

### Requirement: Admission, ordering and eviction are scoped to a pool
FIFO admission by creation time, the waiting set, the freed-slot wake-up and idle
eviction SHALL each operate within one pool.

- A pending conversation SHALL be admitted only when its own pool has a free slot and no older pending conversation of that pool is waiting.
- Making room SHALL evict only an idle runtime pod of the same pool.
- A pod of the other pool SHALL NOT be evicted.
- A younger waiting conversation of one pool SHALL NOT be held behind an older one of the other.

#### Scenario: An older waiting conversation does not hold a member back
- **WHEN** an older conversation waits on a full conversations pool and a younger member waits for a free members slot
- **THEN** the member is admitted

#### Scenario: Eviction stays in the pool
- **WHEN** a member needs a slot, the members pool is full of busy pods and a conversation's pod is idle
- **THEN** the idle conversation pod is not evicted and the member stays `Pending`

#### Scenario: A freed member slot wakes a waiting member
- **WHEN** a member's runtime pod is deleted and another member is pending
- **THEN** the pending member is admitted without waiting for the fallback requeue interval

#### Scenario: Nested coordinators do not starve their own members
- **WHEN** a Coordinator member has issued its own members, its run has ended, and the members pool is full
- **THEN** its idle pod is evictable, a waiting child is admitted, and the parent is re-admitted FIFO when a child's result arrives
