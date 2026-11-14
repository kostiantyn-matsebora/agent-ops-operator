## MODIFIED Requirements

### Requirement: Each command has one queue and one ordering key
Every asynchronous command SHALL map to one queue and one ordering key:

| Command | Queue | Ordering key |
|---|---|---|
| `ingest-chat` | `conv` | the root conversation of the thread's conversation |
| `close` | `conv` | the root conversation of the target |
| `exit` | `conv` | the root conversation of the target |
| `reopen` | `conv` | the root conversation of the target |
| `delete` | `conv` | the root conversation of the target |
| `reset-context` | `conv` | the root conversation of the target |
| `escalate` | `conv` | the root conversation of the target |
| `ensure-topic`, `send`, `close-topic`, `delete-conversation` | `out.<channel>` | the conversation |
| `ingest-signal` | `in.signal` | the source and the signal's `initiationId` |

What the commands cover:

- `ingest-chat` covers an inbound chat message. A console send SHALL go through it.
- The typed commands `/close`, `/exit` and `/reopen` arrive as `ingest-chat`, and the processor turns them into the tasks above.
- `ingest-signal` covers an admitted inbound signal. `POST /signal/inbound`'s accept step enqueues one per admitted signal, after authentication, body shape, the unknown-source refusal, the "signal received" receipt and the source's `attributeFilters` — never before them. The process step answers an in-place chat command or creates the signal's `Signal`, per `async-signal-ingest`.
- `send` covers an input card, a run reply, a notice, a farewell, a reopen notice and an escalation message.

#### Scenario: Two closes under one root are serialized
- **WHEN** a close for a member and a delete for its root are accepted together
- **THEN** both tasks carry the root's key and run one after the other in submission order

#### Scenario: Two unrelated conversations run in parallel
- **WHEN** closes for conversations of two different roots are accepted together
- **THEN** their tasks may run at the same time

#### Scenario: A signal's key is its source and initiationId
- **WHEN** two signals from different sources are accepted sharing one `initiationId`
- **THEN** their `ingest-signal` tasks carry different keys and may run in parallel
