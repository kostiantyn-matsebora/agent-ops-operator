## ADDED Requirements

### Requirement: Adapter paces its Bot API calls
The Telegram adapter SHALL pace every outbound Bot API call against two
independent budgets: a global per-bot rate and a per-chat rate, the per-chat
budget being the tighter of the two for a group. Pacing SHALL cover
`createForumTopic` as well as `sendMessage`, because a burst of new
conversations spends the same budget as a burst of replies — an incident in
which every topic was created but most stayed empty is the shape this
requirement exists to prevent.

The budgets SHALL be constants of the adapter, not manager-side configuration
and not `Channel.spec.config` fields. They describe Telegram, which the adapter
is the only component that knows.

Pacing SHALL NOT be implemented by holding claimed operations in an adapter-side
queue. The adapter SHALL instead defer requesting its next operation until its
budget allows, leaving unclaimed work in the manager's queue where it stays
derivable from CR state.

#### Scenario: Concurrent conversations do not exhaust the budget
- **WHEN** dozens of conversations open within a few minutes and each requires a topic and a reply
- **THEN** the adapter spreads its calls within the per-chat and global budgets and every topic receives its card and its answer

#### Scenario: Pacing holds no operations
- **WHEN** the adapter is pacing and its budget is momentarily exhausted
- **THEN** it delays its next long-poll rather than claiming operations it cannot yet send, and an adapter restart at that moment loses nothing

#### Scenario: Budgets are not configurable
- **WHEN** an operator inspects the Channel or ChannelAdapter CRs
- **THEN** no rate-limit field is present, because the limits are Telegram's and belong to the implementation

### Requirement: Adapter honors Telegram's retry_after
On a `429 Too Many Requests`, the adapter SHALL read `parameters.retry_after`
from the Bot API response, wait that many seconds, and retry the same call. It
SHALL NOT report the operation as failed while retries remain within budget, and
SHALL NOT substitute its own backoff for a stated `retry_after`.

The adapter's total wait for one operation SHALL be bounded well below the
manager's claim reclaim interval of five minutes. When that bound is reached the
adapter SHALL report the operation as failed and let the manager re-derive it.

A `429` on `createForumTopic` SHALL be retried on the same terms as one on
`sendMessage`; both are ordinary backpressure, not errors.

#### Scenario: 429 on send is retried transparently
- **WHEN** `sendMessage` returns `429` with `retry_after: 30`
- **THEN** the adapter waits 30 seconds, retries, and reports the operation complete on success

#### Scenario: 429 on topic creation is retried transparently
- **WHEN** `createForumTopic` returns `429` with a stated `retry_after`
- **THEN** the adapter waits and retries rather than reporting a failed `ensure-topic`

#### Scenario: Retry budget bounded below reclaim
- **WHEN** repeated `429` responses would push the adapter's total wait for one operation toward five minutes
- **THEN** the adapter stops retrying and reports failure while the claim is still valid, so the manager re-derives the operation instead of a second claimant duplicating it

#### Scenario: A retried call posts exactly once
- **WHEN** the adapter retries a `sendMessage` after a `429`
- **THEN** exactly one message appears in the thread, because the rejected call posted nothing
