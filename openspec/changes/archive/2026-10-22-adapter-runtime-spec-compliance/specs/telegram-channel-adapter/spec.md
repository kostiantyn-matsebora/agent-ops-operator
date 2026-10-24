# telegram-channel-adapter — delta

## MODIFIED Requirements

### Requirement: Adapter closes forum topics on close-topic operations
The Telegram channel adapter SHALL serve the `close-topic` operation by closing
the corresponding forum topic through the Bot API (`closeForumTopic`) for the
chat the Channel's config names, then completing the operation with an empty
result.

In-window backpressure on `closeForumTopic` SHALL be absorbed like any other
call: a `429` carrying `retry_after`, and transient transport errors, are
retried inside the operation's retry budget under the contract's backpressure
rule.

A definitive Bot API failure SHALL be reported as the operation's error. A
COMPLETED operation SHALL NOT be re-attempted by the adapter — recovery is the
manager's re-derivation, which the contract guarantees while the Conversation
exists.

#### Scenario: Forum topic is closed
- **WHEN** the adapter receives a `close-topic` operation for thread id `9876`
- **THEN** it calls `closeForumTopic` for that message thread and completes the operation with an empty result

#### Scenario: Backpressure on a close is absorbed
- **WHEN** `closeForumTopic` returns `429` with a stated `retry_after` inside the operation's budget
- **THEN** the adapter waits and retries the call rather than completing the operation with an error

#### Scenario: Bot API failure is reported, not retried
- **WHEN** `closeForumTopic` fails for a reason retrying cannot fix
- **THEN** the adapter completes the operation with that error and does not re-attempt it, and the manager re-derives the operation

#### Scenario: Duplicate close is tolerated
- **WHEN** the same `close-topic` operation id is delivered twice
- **THEN** the adapter completes it without treating an already-closed topic as a failure

### Requirement: Chart deploys the adapter opt-in
The chart SHALL ship the Telegram adapter as a `ChannelAdapter` CR instead of a
bespoke Deployment template. The chart SHALL contain no channel-type-specific
workload templates.

The CR SHALL be gated on the Telegram bundle's `telegram.enabled` value
(default **false**), the one gate the whole bundle renders under. The retired
flat `telegramAdapter.enabled` value gates nothing.

The `ChannelAdapter` reconciler owns the workload, auth injection, and
credential projection. The chart's remaining role is the CRD and the gated CR.

#### Scenario: Disabled by default
- **WHEN** the chart renders with default values
- **THEN** no Telegram adapter resources are produced

#### Scenario: Enabled renders only a CR
- **WHEN** `telegram.enabled=true`
- **THEN** the rendered output contains the telegram `ChannelAdapter` and no Deployment for it (the reconciler creates the workload)

### Requirement: The adapter parses the grammar and folds the detail

The adapter SHALL parse the block grammar out of an agent-reported body and
render it: the title first, named sections labelled and in order, and the folded
region using the transport's own collapsed presentation.

It parses the body itself. Nothing upstream has, and the tags reach it exactly as
the agent wrote them.

Chunking SHALL prefer BLOCK boundaries. NO component bounds above-the-fold
content, so the ADAPTER SHALL guarantee the fold order itself: above-the-fold
content fills the leading chunks, and folded content never precedes any of it.

When the title and the named sections fit one message, the FIRST chunk SHALL
carry them all — a reader who sees only one message still sees the conclusion.

A body with no recognized tag SHALL render exactly as today. That is the whole
backward-compatibility story on this surface: untagged prose parses to one block
and looks unchanged.

A `signal` SHALL NOT be parsed. Its structured fields render as a CARD, and its
payload is a machine document — quoted, and folded once tall enough to dominate
the thread.

#### Scenario: The detail arrives collapsed

- **WHEN** an answer carries a folded region
- **THEN** it is posted collapsed and the reader expands it in place

#### Scenario: The conclusion leads the first chunk

- **WHEN** a long answer must be split and its title and named sections fit one message
- **THEN** the first message carries the title and the named sections, and the
  fold's content follows

#### Scenario: Oversized above-the-fold content still leads

- **WHEN** the title and the named sections alone exceed one message
- **THEN** they fill consecutive leading messages in the agent's order, and no
  folded content appears before the last of them

#### Scenario: An unstructured message is unchanged

- **WHEN** a notice or a relay arrives with prose carrying no tags
- **THEN** it renders as it does today

#### Scenario: A signal card is not prose

- **WHEN** a signal arrives whose payload happens to contain a tag-shaped line
- **THEN** the card renders from the signal's fields and the payload is quoted
  verbatim, with nothing folded by the grammar

### Requirement: Adapter honors Telegram's retry_after
On a `429 Too Many Requests`, the adapter SHALL read `parameters.retry_after`
from the Bot API response, wait that many seconds, and retry the same call. It
SHALL NOT report the operation as failed while retries remain within budget, and
SHALL NOT substitute its own backoff for a stated `retry_after`.

The adapter's total wait for one operation SHALL be bounded well below the
`reclaimAfterSeconds` the manager advertised for that claim. When that bound is
reached the adapter SHALL report the operation as failed and let the manager
re-derive it.

A `429` on `createForumTopic` SHALL be retried on the same terms as one on
`sendMessage`. Both are ordinary backpressure, not errors.

#### Scenario: 429 on send is retried transparently
- **WHEN** `sendMessage` returns `429` with `retry_after: 30`
- **THEN** the adapter waits 30 seconds, retries, and reports the operation complete on success

#### Scenario: 429 on topic creation is retried transparently
- **WHEN** `createForumTopic` returns `429` with a stated `retry_after`
- **THEN** the adapter waits and retries rather than reporting a failed `ensure-topic`

#### Scenario: Retry budget bounded below reclaim
- **WHEN** repeated `429` responses would push the adapter's total wait for one operation toward the claim's `reclaimAfterSeconds`
- **THEN** the adapter stops retrying and reports failure while the claim is still valid, so the manager re-derives the operation instead of a second claimant duplicating it

#### Scenario: A retried call posts exactly once
- **WHEN** the adapter retries a `sendMessage` after a `429`
- **THEN** exactly one message appears in the thread, because the rejected call posted nothing
