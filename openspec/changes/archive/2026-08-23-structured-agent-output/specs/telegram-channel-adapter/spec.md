## ADDED Requirements

### Requirement: The adapter parses the grammar and folds the detail

The adapter SHALL parse the block grammar out of an agent-reported body and
render it: the title first, named sections labelled and in order, and the folded
region using the transport's own collapsed presentation.

It parses the body itself. Nothing upstream has, and the tags reach it exactly as
the agent wrote them.

Chunking SHALL prefer BLOCK boundaries. Because the above-the-fold content is
bounded by the manager, the FIRST chunk of a split message SHALL contain the
title and the named sections — a reader who sees only one message still sees the
conclusion.

A body with no recognized tag SHALL render exactly as today. That is the whole
backward-compatibility story on this surface: untagged prose parses to one block
and looks unchanged.

A `signal` SHALL NOT be parsed. Its structured fields render as a CARD, and its
payload is a machine document — quoted, and folded once tall enough to dominate
the thread.

#### Scenario: The detail arrives collapsed

- **WHEN** an answer carries a folded region
- **THEN** it is posted collapsed and the reader expands it in place

#### Scenario: The conclusion leads the first chunk

- **WHEN** a long answer must be split across several messages
- **THEN** the first message carries the title and the named sections, and the
  fold's content follows

#### Scenario: An unstructured message is unchanged

- **WHEN** a notice or a relay arrives with prose carrying no tags
- **THEN** it renders as it does today

#### Scenario: A signal card is not prose

- **WHEN** a signal arrives whose payload happens to contain a tag-shaped line
- **THEN** the card renders from the signal's fields and the payload is quoted
  verbatim, with nothing folded by the grammar
