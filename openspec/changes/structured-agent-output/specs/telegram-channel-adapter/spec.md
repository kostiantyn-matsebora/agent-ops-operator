## ADDED Requirements

### Requirement: The adapter renders blocks and folds the detail

The adapter SHALL render an outbound message's blocks: the title first, named
sections labelled and in order, and the folded region using the transport's own
collapsed presentation.

Chunking SHALL prefer BLOCK boundaries. Because the above-the-fold content is
bounded by the manager, the FIRST chunk of a split message SHALL contain the
title and the named sections — a reader who sees only one message still sees the
conclusion.

A message with no blocks SHALL render from its body, exactly as today.

#### Scenario: The detail arrives collapsed

- **WHEN** an answer carries a folded region
- **THEN** it is posted collapsed and the reader expands it in place

#### Scenario: The conclusion leads the first chunk

- **WHEN** a long answer must be split across several messages
- **THEN** the first message carries the title and the named sections, and the
  fold's content follows

#### Scenario: An unstructured message is unchanged

- **WHEN** a notice or a relay arrives with prose and no blocks
- **THEN** it renders as it does today
