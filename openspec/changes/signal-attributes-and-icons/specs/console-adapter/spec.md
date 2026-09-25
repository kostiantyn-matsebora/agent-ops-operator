## ADDED Requirements

### Requirement: The signal card draws its rating, its type and its icons
The transcript's signal card SHALL draw the `signal` message's `severity`, `type` and `icons` fields.

- The severity leads the card with its icon and its value, tinted per value from the theme's tokens.
- The type follows with its icon where one was published, else the bare value.
- The source and the pipeline are named with their resolved icons.
- A card whose message carries none of these SHALL render exactly as today.

#### Scenario: A critical security alert
- **WHEN** a `critical` alert labelled `type: security` opens a conversation and the type has a declared icon
- **THEN** the card leads with the critical mark and value, then the type's icon and `security`, then the source and pipeline with their icons

#### Scenario: A task card is unchanged
- **WHEN** a `kind: task` signal with no severity opens a conversation
- **THEN** the card renders as before, with the source and pipeline icons only
