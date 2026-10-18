# console-origination — delta

## ADDED Requirements

### Requirement: The console's two chat cases are told apart by the id

The console SHALL carry an `initiationId` on every chat signal it emits, applying the contract's one id rule in its two cases:

| Case | Instance of | The id |
|---|---|---|
| a user starts a new task | minting — the request is its own aggregation unit | a FRESH `initiationId`, minted by the console per request, own conversation by construction |
| a user writes into a conversation the console already displays | propagation — the binding carries the manager-issued id back | the `conversationId` the manager stamped on the console's thread binding, passed through verbatim |

The console SHALL NOT mint a continuation id of its own and SHALL NOT invent a second addressing mode. The propagated id routes by the same equality rule every signal routes by.

#### Scenario: A started task carries a fresh id

- **WHEN** a user submits a new task against a wired console source
- **THEN** the emitted `chat` signal carries a fresh `initiationId` and a new conversation adopts it as its `conversationId`

#### Scenario: A reply echoes the stamped id

- **WHEN** a user writes into a console-bound conversation through the signal path
- **THEN** the signal's `initiationId` is the `conversationId` the manager stamped on that binding, and equality routing appends the message to that open conversation
