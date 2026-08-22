## ADDED Requirements

### Requirement: An adapter may declare a signal body structured

An inbound signal MAY declare that its body follows the block grammar. When
declared, the manager parses the body into blocks. When absent, the body is raw
and SHALL reach adapters unparsed, exactly as before this declaration existed.

Raw is the default because a chat signal's body is a person's typed words, and
parsing those by default would consume characters somebody deliberately wrote.

#### Scenario: Existing adapters need no change

- **WHEN** an adapter that predates the declaration posts a signal
- **THEN** the body is raw, and the resulting card is unchanged

#### Scenario: A person's typed markup survives

- **WHEN** a chat signal carries a message in which a person typed a tag
- **THEN** the characters reach the thread as typed and nothing is folded

#### Scenario: A structured signal is folded

- **WHEN** an adapter declares its body tagged and emits a folded region
- **THEN** the card renders that region collapsed on every surface
