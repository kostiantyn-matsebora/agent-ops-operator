# channel-adapter-contract — delta

## MODIFIED Requirements

### Requirement: Outbound operations delivered to adapters by long-poll
The manager SHALL expose `GET /channel/ops?adapter=<name>&wait=<seconds>` (non-leader-gated) returning the next pending outbound operation for any Channel served by that adapter, or 204 on timeout. The parameter names the adapter (the same value Channels carry in `spec.adapter`), replacing the former `?type=`; a request carrying the retired parameter SHALL fail with 400 naming the replacement rather than being served an empty list, so an outdated adapter fails loudly instead of appearing to work while delivering nothing. Operations SHALL carry a stable id, the channel and conversation names, a kind (`ensure-topic` or `send`), and a kind-specific payload (`send` includes the message text and target thread id). Operations SHALL be derived from CR state or router actions such that an operation lost in flight (manager restart, adapter crash) is regenerated or safely skipped; delivery is at-least-once and adapters MUST tolerate duplicates by id.

#### Scenario: Adapter receives a topic-creation op
- **WHEN** a Conversation referencing a Channel with `adapter: slack` is reconciled with no `threadId` and an adapter is long-polling `/channel/ops?adapter=slack`
- **THEN** the adapter receives an `ensure-topic` operation identifying that conversation

#### Scenario: Retired parameter fails loudly
- **WHEN** an adapter built against the old contract polls `/channel/ops?type=slack`
- **THEN** the manager responds 400 naming `adapter` as the expected parameter

#### Scenario: No ops available
- **WHEN** an adapter long-polls and no operation becomes available within `wait`
- **THEN** the manager responds 204 and the adapter re-polls

#### Scenario: Unclaimed op survives a manager restart
- **WHEN** the manager restarts while an `ensure-topic` op is queued but undelivered
- **THEN** reconciliation re-enqueues an equivalent operation and the conversation still gets its topic
