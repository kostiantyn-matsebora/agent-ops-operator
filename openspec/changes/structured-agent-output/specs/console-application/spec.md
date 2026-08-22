## ADDED Requirements

### Requirement: The transcript renders agent output as structure, not as raw text

The console SHALL render an outbound message's blocks as interface elements: the
title as a heading, named sections as labelled sections, and the folded region
behind a disclosure control that is collapsed by default.

Rendering SHALL compose elements. No string SHALL be handed to the DOM as
markup, so the application continues to need neither a markup parser nor a
sanitizer for message content.

A message with no blocks SHALL render as text, as it does today.

#### Scenario: A long answer opens with its conclusion

- **WHEN** an agent answer carrying a fold is displayed in a conversation
- **THEN** the title and named sections are visible, and the detail is behind a
  collapsed control

#### Scenario: Markup in text is still never markup

- **WHEN** any block's text contains tag-shaped characters
- **THEN** they display as characters, and nothing is interpreted as markup

#### Scenario: Unstructured messages still render

- **WHEN** a message carries prose and no blocks
- **THEN** the console renders it as text, unchanged from today
