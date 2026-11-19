## Purpose

A conversation has one identifier the system uses to refer to it, unique for the life of that one conversation, and a friendly name for the people reading a channel or the console. This capability states which is which and where each is used.

## ADDED Requirements

### Requirement: A conversation's identity is its ID, minted by the store

A conversation SHALL carry an `ID`, an opaque string minted by the state backend at create, the same value `A record carries a backend-minted identity` (`state-store`) already requires. It SHALL stay the same for the conversation's whole life, SHALL differ between two conversations created one after the other under the same name, and SHALL NOT be settable by a caller. No component SHALL parse it or derive meaning from its shape.

The `ID` SHALL be safe as a Kubernetes label value on every backend: at most 63 characters, only alphanumerics, `-`, `_` and `.`, beginning and ending with an alphanumeric. The conformance suite SHALL assert this for every backend, so a backend that mints a longer or differently shaped value fails it.

#### Scenario: A reused name has a new identity

- **WHEN** a conversation is deleted and another is created under the same name
- **THEN** the second carries an `ID` different from the first's

#### Scenario: Every backend's ID is label-safe

- **WHEN** the conformance suite creates a conversation on each backend
- **THEN** the minted `ID` is at most 63 characters of the permitted set and begins and ends with an alphanumeric

### Requirement: The name is display and the store key

A conversation's `Name` SHALL be the word-chain `conversation-naming` allocates. It SHALL be what a person sees and types, the key the conversation is stored under, and the stem of the runtime pod's name `agentops-conv-<name>`. It SHALL NOT be the identifier any component uses to refer to a conversation where the `ID` is available.

#### Scenario: The name keeps its three roles

- **WHEN** a conversation named `alert-node-down-ns-2` runs
- **THEN** it is stored under that key, its pod is `agentops-conv-alert-node-down-ns-2`, and a channel shows that name to the people reading

### Requirement: The system refers to a conversation by its ID

Wherever the system, not a person, refers to a conversation, it SHALL use the `ID`:

| Reference | Carries |
|---|---|
| the workspace subdirectory (`runtime-workspace-persistence`) | the `ID` |
| the runtime's `CONVO_ID` (`runtime-contract`) | the `ID` |
| an operation id and a thread id the manager derives | the `ID` |
| a stored cross-reference, `causedBy.parent` and any other conversation named by a record | the `ID` of the conversation it names |
| an operation on the adapter wire (`channel-adapter-contract`) | `conversationUid` carries the `ID`, beside the display name in `conversation` |
| the runtime pod | the label `agentops.dev/conversation-id` |

A reference by `ID` SHALL NOT resolve to a later conversation that reuses the name. A reference whose `ID` names no live conversation SHALL be treated as pointing at a conversation that is gone, and SHALL NOT be re-pointed at a conversation of the same name.

#### Scenario: A reference survives a name's reuse without moving

- **WHEN** a member's `causedBy.parent` holds the `ID` of its parent, the parent is deleted and a new conversation takes the parent's name
- **THEN** the member's reference still names the deleted parent, and the new conversation is not treated as its parent

#### Scenario: Op ids of a reused name do not collide

- **WHEN** two conversations that shared a name over time each enqueue a close-topic operation for the same channel
- **THEN** their operation ids differ, and the second is not dropped as a duplicate of the first

### Requirement: The runtime pod carries its conversation's ID

The runtime pod SHALL carry the label `agentops.dev/conversation-id` whose value is the conversation's `ID`. The pod's name SHALL stay `agentops-conv-<name>`. When the manager goes from a pod to its conversation, including when it reconciles from a pod event, it SHALL use the label and SHALL NOT use the pod's name.

A pod with no such label, or whose label names no live conversation, SHALL NOT be adopted by a conversation of the same name.

#### Scenario: A pod maps to its conversation by label

- **WHEN** a pod event arrives for `agentops-conv-alert-node-down-ns-2` whose label holds an `ID`
- **THEN** the manager reconciles the conversation holding that `ID`

#### Scenario: A stale pod is not adopted by a reused name

- **WHEN** a pod labelled with a deleted conversation's `ID` outlives it and a new conversation takes the same name
- **THEN** the new conversation does not adopt the pod

### Requirement: A typed name resolves to the most recent live match

Where a person types a conversation's name, a chat command such as `/exit <name>` or a console route that accepts a name, the manager SHALL resolve it to the record whose `Name` matches and that is live, taking the newest `CreatedAt` where more than one qualifies. No live match SHALL answer not found, with an error naming the name it could not resolve.

A record is LIVE when it exists in the store and carries no deletion marker. A record being deleted, held only by a finalizer, is not live. A Closed conversation is live: it is intact, inert and still displayed.

Resolution selects the conversation only. Whether the verb then applies is judged by the conversation's phase and answered with its own reason:

| Verb class | Resolves among | A Closed match |
|---|---|---|
| reads and views | live | shown |
| `reopen`, `delete` | live | acted on, a Closed conversation is the point of both |
| `exit`, `close`, `escalate`, adding input | live | refused with a reason naming the phase, never reported as not found |

#### Scenario: A name resolves to the live conversation

- **WHEN** a person types `/exit alert-node-down-ns-2` and one live conversation carries that name
- **THEN** the verb is applied to that conversation

#### Scenario: A name held only by a deleting record is not found

- **WHEN** the only record carrying a name has a deletion marker and a person types the name
- **THEN** the answer is not found, naming the name

#### Scenario: A newer live match wins

- **WHEN** more than one live record carries the typed name
- **THEN** the one with the newest `CreatedAt` is selected

#### Scenario: A Closed conversation answers a verb that cannot apply

- **WHEN** a person types `/exit <name>` and the live match is Closed
- **THEN** the manager answers that the conversation is closed, and does not answer not found

### Requirement: The signal correlation key is `correlationId`

The key a conversation adopts from a signal's `initiationId` at creation SHALL be called `correlationId` everywhere it is named, on the `Conversation` spec, the stored record, the index that lists by it and the field of an adapter operation. The name `conversationId` for it is removed.

Its meaning SHALL NOT change. It is written once at creation, never changed and never backfilled, is opaque, is compared for equality only, may be shared by several conversations over time, and is empty for a conversation a chat message originated. It is not the conversation's identity.

#### Scenario: Several conversations share one correlation key

- **WHEN** a recurrence opens a new conversation after the first one closed
- **THEN** both carry the same `correlationId` and differ by `ID`

#### Scenario: A chat-originated conversation has none

- **WHEN** a conversation is created from a chat message with no `initiationId` to adopt
- **THEN** its `correlationId` is empty and its `ID` is populated

### Requirement: An existing conversation is not migrated

A conversation created before this change is not carried across it. The manager SHALL NOT invent an `ID` for a record that lacks one, SHALL NOT infer a `correlationId` from a retired field, and SHALL NOT rewrite a stored cross-reference. An install upgrading across the change SHALL have its conversations deleted first, and the upgrade documentation SHALL state this.

#### Scenario: A record without an identity is not served as a conversation

- **WHEN** the manager reads a conversation record that carries no `ID`
- **THEN** it reports the record as predating conversation identity, with the instruction to delete it, and starts no work for it
