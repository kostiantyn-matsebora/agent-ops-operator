## ADDED Requirements

### Requirement: The transcript renders agent output as structure, not as raw text

The console SHALL parse the block grammar out of an agent-reported body and
render it as interface elements: the title as a heading, named sections as
labelled sections, and the folded region behind a disclosure control that is
collapsed by default.

IT PARSES THE SAME CHARACTERS WHEREVER THEY COME FROM — a live message, or
`status.runs[].result` read back when a transcript is rebuilt. A conversation
reopened after a restart SHALL therefore render exactly as it did live.

Rendering SHALL compose elements. No string SHALL be handed to the DOM as
markup, so the application continues to need neither a markup parser nor a
sanitizer for message content.

A body with no recognized tag SHALL render as text, as it does today.

A `signal` SHALL NOT be parsed. Its structured fields render as a card — the
source stated plainly, labels as a table, the payload behind its own control.

#### Scenario: A long answer opens with its conclusion

- **WHEN** an agent answer carrying a fold is displayed in a conversation
- **THEN** the title and named sections are visible, and the detail is behind a
  collapsed control

#### Scenario: Markup in text is still never markup

- **WHEN** any block's text contains tag-shaped characters
- **THEN** they display as characters, and nothing is interpreted as markup

#### Scenario: Unstructured messages still render

- **WHEN** a message carries prose with no tags
- **THEN** the console renders it as text, unchanged from today

#### Scenario: History survives a restart

- **WHEN** the console restarts and a reader reopens an earlier conversation
- **THEN** its agent answers render with their titles, sections and folds,
  because the transcript is rebuilt from the text the agent wrote

#### Scenario: An event card is not an answer

- **WHEN** a signal opens a conversation
- **THEN** the reader sees which source it came from without expanding
  anything, and its payload does not fill the view
