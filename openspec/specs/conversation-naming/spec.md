# conversation-naming Specification

## Purpose
Conversation object names are deterministic word-chains built from context
already known at creation, never a random or hash-derived suffix, at any
scale.

## Requirements

### Requirement: Conversation names are deterministic word-chains

The manager SHALL set `Conversation.metadata.name` directly (never
`generateName`) to `<kind>-<words>`, where `<kind>` is the existing kind word
(`alert`, `job`, `chat`, `task`, `member`) and `<words>` is a word-chain built
from context already available at that creation site.

| Creation path | Word source |
|---|---|
| Root conversation from a signal (`alert`, `job`, `chat`, `task` kind) | the same title text already derived for `spec.title` |
| Task conversation opened from a channel command | the addressed pipeline name |
| Member conversation created by a Coordinator `invoke` | the invoked `agents[]` entry name |

No creation path SHALL ever produce a name containing a random or
hash-derived token.

#### Scenario: An alert conversation's name hints at the alert

- **WHEN** a signal of kind `alert` with title text "NodeDown — ns-prod" opens a new conversation
- **THEN** the created conversation's name is `alert-node-down-ns`

#### Scenario: A member conversation's name hints at the invoked entry

- **WHEN** a Coordinator's `invoke` creates a member conversation for the `agents[]` entry named `researcher`
- **THEN** the created conversation's name is `member-researcher`

#### Scenario: A task conversation's name hints at the addressed pipeline

- **WHEN** a chat command `/deploy-notes do the thing` opens a new task conversation
- **THEN** the created conversation's name is `task-deploy-notes`

### Requirement: The word-chain keeps significant words, not truncated characters

Word-chain extraction SHALL split source text on punctuation and on
`CamelCase` boundaries, drop a fixed, small stopword list, and keep the
first three remaining words, lowercased and joined by `-`.

The word-chain SHALL be capped at a fixed maximum length of 24 characters.

That length SHALL be chosen against the LONGEST derived name, not the bare
63-character limit. Added to the 24 characters:

- the 18-character `agentops-mcp-conv-` prefix
- the 7-character `member-` kind prefix
- a numeric suffix of up to 4 characters (`-999`)

The total SHALL stay within Kubernetes' object-name and label-value limits.

The 4-character suffix is a budget, not a cap. The remaining 14 characters
of the 63-character limit hold a suffix of up to 13 digits, so a base name
past 999 conversations keeps allocating and the name stays valid.

Where the joined chain exceeds that length, the manager SHALL drop whole
trailing words until it fits, never cut a word in half.

#### Scenario: CamelCase and punctuation both split into words

- **WHEN** the word source text is "NodeDown — ns-prod"
- **THEN** the word-chain is `node-down-ns`, with every token a whole word and only the first three kept

#### Scenario: A stopword is dropped

- **WHEN** the word source text is "Nightly backup of prod-db"
- **THEN** the word-chain is `nightly-backup-prod`, with "of" absent

#### Scenario: Too many words drops from the end, not mid-word

- **WHEN** the first three significant words, joined, would exceed the configured maximum length
- **THEN** the manager drops whole words from the end until the chain fits, and no word in the result is partial

### Requirement: Every creation path has a non-empty word source

For a root conversation from a signal (`alert`, `job`, `chat`, `task`
kind), when neither the signal's own title nor its payload text yields any
significant word, the manager SHALL fall back to the claiming
`SignalSource`'s own name as the word source.

This fallback SHALL apply to every kind, not only `alert`/`job`.

A task-command conversation's word source (the addressed pipeline name) and
a member conversation's word source (the invoked entry name) SHALL always
be non-empty by construction, needing no further fallback.

#### Scenario: A caption-less chat message still slugs

- **WHEN** a chat-kind signal carries no title and payload text with no significant word (a sticker, voice note, or photo with no caption), from a SignalSource named `ops-room`
- **THEN** the created conversation's name is `chat-ops-room`

### Requirement: Collisions are resolved by lookup, never by a random suffix

Every conversation SHALL carry an immutable label naming its own base name
(`agentops.dev/name-base: <kind>-<words>`).

To create a new conversation, the manager SHALL list existing conversations
sharing that label, determine the next unused numeric suffix, and create
with that name in one list-then-create step.

It SHALL NOT retry sequential name guesses one at a time. It SHALL NOT
substitute a random or hash-derived suffix at any point.

A create conflict caused by a genuine race between two concurrent creators
SHALL be resolved by retrying the same list-then-create step, bounded to a
small, fixed number of attempts: 5.

When all 5 attempts end in a create conflict, the manager SHALL return the
last conflict error to its caller, naming the base name. It SHALL NOT fall
back to a random or hash-derived suffix.

#### Scenario: Exhausted retries return an error

- **WHEN** five consecutive list-then-create attempts for one base name all end in a create conflict
- **THEN** the creation fails with the conflict error naming the base name, and no conversation with a random suffix is created

#### Scenario: First conversation for a base name has no suffix

- **WHEN** no conversation labeled `agentops.dev/name-base: alert-node-down-ns` exists yet
- **THEN** the created conversation's name is `alert-node-down-ns`, with no numeric suffix

#### Scenario: A recurring alert gets the next number

- **WHEN** conversations labeled `agentops.dev/name-base: alert-node-down-ns` already exist named `alert-node-down-ns` and `alert-node-down-ns-2`
- **THEN** the next created conversation for that base name is `alert-node-down-ns-3`

#### Scenario: Collision resolution costs one lookup regardless of history

- **WHEN** a base name already has many prior conversations, however many
- **THEN** the manager determines the next suffix with one list call plus one create call, never one attempt per prior conversation
