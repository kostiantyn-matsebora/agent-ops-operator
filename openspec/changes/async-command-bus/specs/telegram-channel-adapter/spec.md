## REMOVED Requirements

### Requirement: Adapter honors Telegram's retry_after
**Reason**: The budget it bounded was the claim's reclaim interval. The manager now hands an operation over at its acknowledgement and fails one with no report at its delivery TTL.

**Migration**: Read the replacement requirement below, bounded by the delivery TTL.

## MODIFIED Requirements

### Requirement: Adapter paces its Bot API calls
The Telegram adapter SHALL pace every outbound Bot API call against two
independent budgets: a global per-bot rate and a per-chat rate, the per-chat
budget being the tighter of the two for a group.

Pacing SHALL cover `createForumTopic` as well as `sendMessage`, because a
burst of new conversations spends the same budget as a burst of replies. An
incident in which every topic was created but most stayed empty is the
shape this requirement exists to prevent.

The budgets SHALL be constants of the adapter, not manager-side configuration
and not `Channel.spec.config` fields. They describe Telegram, which the adapter
is the only component that knows.

Pacing SHALL NOT be implemented by holding acknowledged operations in an adapter-side
queue, since an acknowledgement starts the delivery TTL. The adapter SHALL instead defer reading its next operation until its
budget allows, leaving unread work in the manager's queue where it stays
derivable from CR state.

#### Scenario: Concurrent conversations do not exhaust the budget
- **WHEN** dozens of conversations open within a few minutes and each requires a topic and a reply
- **THEN** the adapter spreads its calls within the per-chat and global budgets and every topic receives its card and its answer

#### Scenario: Pacing holds no operations
- **WHEN** the adapter is pacing and its budget is momentarily exhausted
- **THEN** it delays its next read rather than acknowledging operations it cannot yet send, and an adapter restart at that moment loses nothing

#### Scenario: Budgets are not configurable
- **WHEN** an operator inspects the Channel or ChannelAdapter CRs
- **THEN** no rate-limit field is present, because the limits are Telegram's and belong to the implementation

## ADDED Requirements

### Requirement: Adapter delivers an acknowledged operation inside its delivery TTL and reports it
The Telegram adapter SHALL read an operation, acknowledge it, deliver it through the gateway with its own retries, and report it done or failed. It SHALL carry no Telegram credential and issue every Bot API call through the gateway.

The `ensure-topic` thread id SHALL travel back in the done report. An operation the manager will not hand over (the acknowledgement answers `409`) SHALL NOT be delivered. A delivery that cannot finish inside its TTL SHALL be reported failed. After a restart the adapter SHALL NOT deliver an operation it already delivered.

On a `429 Too Many Requests` the adapter SHALL read `parameters.retry_after`, wait that many seconds, and retry the same call. It SHALL NOT report the operation as failed while retries remain within budget, and SHALL NOT substitute its own backoff for a stated `retry_after`.

The adapter's total wait for one operation SHALL be the acknowledged delivery TTL less a margin that leaves room to report. When that bound is reached the adapter SHALL report the operation failed. A `429` on `createForumTopic` SHALL be retried on the same terms as one on `sendMessage`.

#### Scenario: Read, acknowledge, deliver, report
- **WHEN** the adapter is offered a `send` op
- **THEN** it acknowledges the op, posts the message, and reports it done

#### Scenario: An op the manager will not hand over is not delivered
- **WHEN** the acknowledgement of an op is refused because it is not offered
- **THEN** the adapter delivers nothing and reads again

#### Scenario: 429 on send is retried transparently
- **WHEN** `sendMessage` returns `429` with `retry_after: 30`
- **THEN** the adapter waits 30 seconds, retries, and reports the operation done on success

#### Scenario: 429 on topic creation is retried transparently
- **WHEN** `createForumTopic` returns `429` with a stated `retry_after`
- **THEN** the adapter waits and retries rather than reporting a failed `ensure-topic`

#### Scenario: Retry budget bounded by the delivery TTL
- **WHEN** repeated `429` responses would push the adapter's total wait for one operation toward its acknowledged delivery TTL
- **THEN** the adapter stops retrying and reports failure while the TTL still holds

#### Scenario: A retried call posts exactly once
- **WHEN** the adapter retries a `sendMessage` after a `429`
- **THEN** exactly one message appears in the thread, because the rejected call posted nothing

#### Scenario: A restart after the acknowledgement does not duplicate
- **WHEN** the adapter restarts after acknowledging and delivering an op whose report was lost
- **THEN** it does not post the message again and the manager fails or settles the op by its own rules
