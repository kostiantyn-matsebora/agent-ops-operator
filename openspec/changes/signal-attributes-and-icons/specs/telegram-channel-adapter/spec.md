## ADDED Requirements

### Requirement: The signal card leads with the severity and the menu uses the icon ladder
The adapter SHALL render a `signal` message's severity and type, and SHALL draw every icon it can express.

- A rated card SHALL lead with the severity's emoji and its value. The adapter holds one emoji per vocabulary value.
- The type follows as its value, with the type's emoji where the published reference is one.
- The source and pipeline lines carry their emoji where the published reference is one, and omit the icon otherwise.
- The command menu's icon for a Pipeline is the published, ladder-resolved reference, so a Pipeline that declares none still leads with its kind's emoji.
- An unrated, untyped card SHALL render exactly as today.

#### Scenario: A critical card leads with its mark
- **WHEN** a `critical` signal opens a topic
- **THEN** the card's first line carries the critical emoji and `critical`

#### Scenario: A named-set icon is omitted
- **WHEN** the type's published icon is `mdi:shield`
- **THEN** the card names the type with no icon, and nothing fails

#### Scenario: An undeclared Pipeline icon falls back
- **WHEN** a Pipeline declares no icon
- **THEN** its command menu entry leads with the built-in pipeline emoji
