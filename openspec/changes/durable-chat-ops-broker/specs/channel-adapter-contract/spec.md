## MODIFIED Requirements

### Requirement: Outbound operations delivered to adapters by long-poll
The manager SHALL expose `GET /channel/ops?adapter=<name>&wait=<seconds>`. It SHALL return the next pending outbound operation for any Channel served by that adapter, 204 on timeout, or 503 when the serving replica is not the current leader and cannot safely claim one on this adapter's behalf.

The parameter names the adapter, the same value Channels carry in `spec.adapter`, replacing the former `?type=`.

A request carrying the retired parameter SHALL fail with 400 naming the replacement, rather than being served an empty list. An outdated adapter then fails loudly instead of appearing to work while delivering nothing.

The polling adapter SHALL additionally declare the outbound contract version it speaks, in the `contract=<version>` query parameter (`GET /channel/ops?adapter=<name>&contract=2&wait=25`). An absent or unsupported declaration SHALL fail with 400 naming what is expected.

A 503 response SHALL carry a `Retry-After` header and SHALL be distinguishable from 204: an adapter MUST retry a 503 immediately rather than waiting out its normal idle backoff, and MUST NOT treat it as an error to surface.

Every operation SHALL carry a stable id, the channel and conversation names, a kind, and a kind-specific **structured** payload — never pre-rendered display text:

| Kind | Payload |
|---|---|
| `send` | a typed message (`signal`, `answer`, `relay`, or `notice`) with markdown-valued free text and typed structured fields, plus the target thread id |
| `ensure-topic` | a topic descriptor (`conversation`, `pipeline?`, `source?`, `title`, `labels`, `kind`), never a rendered title string — `pipeline` is inferred and MAY be empty |
| `close-topic` | the target thread id, asking the adapter to archive or close that thread on its transport |

Escaping, length limits, chunking, truncation, and thread naming SHALL be the adapter's responsibility. The manager SHALL emit no transport markup and declare no maximum message size.

Operations SHALL be derived from CR state or router actions, such that an operation lost in flight (manager restart, adapter crash) is regenerated or safely skipped. Delivery is at-least-once, and adapters MUST tolerate duplicates by id.

A `close-topic` operation SHALL be derivable from CR state for as long as it is outstanding, which the deleting conversation's finalizer guarantees.

#### Scenario: Adapter receives a topic-creation op
- **WHEN** a Conversation referencing a Channel with `adapter: slack` is reconciled with no `threadId` and an adapter is long-polling `/channel/ops?adapter=slack`
- **THEN** the adapter receives an `ensure-topic` operation identifying that conversation, carrying a descriptor it names the thread from

#### Scenario: Adapter receives a topic-close op
- **WHEN** a Conversation bound to a Channel with `adapter: slack` is deleted while holding a thread id
- **THEN** the adapter receives a `close-topic` operation carrying that thread id

#### Scenario: No ops available
- **WHEN** an adapter long-polls and no operation becomes available within `wait`
- **THEN** the manager responds 204 and the adapter re-polls

#### Scenario: Unclaimed op survives a manager restart
- **WHEN** the manager restarts while an `ensure-topic` op is queued but undelivered
- **THEN** reconciliation re-enqueues an equivalent operation and the conversation still gets its topic

#### Scenario: Retired parameter fails loudly
- **WHEN** an adapter built against the old contract polls `/channel/ops?type=slack`
- **THEN** the manager responds 400 naming `adapter` as the expected parameter

#### Scenario: Outdated outbound contract fails loudly
- **WHEN** an adapter that expects string-valued `send` ops polls for operations
- **THEN** the manager responds 400 naming the required contract version, rather than delivering messages it would post as empty text

#### Scenario: Send ops carry meaning, not markup
- **WHEN** a `send` op is delivered for an agent's answer
- **THEN** it carries an `answer` message with a markdown body and no transport markup, and the adapter renders it

#### Scenario: A poll served by a non-leader is rejected, not answered empty
- **WHEN** an adapter's poll is handled by a manager replica that is not the current leader
- **THEN** the manager responds 503 with a `Retry-After` header, never a 204 that would read as "nothing to deliver" when a claim-worthy op may in fact exist

#### Scenario: A conforming adapter retries a 503 immediately
- **WHEN** an adapter receives a 503 from `/channel/ops`
- **THEN** it re-polls at once, the same way it already does on `204`, and does not report the response as a failure
