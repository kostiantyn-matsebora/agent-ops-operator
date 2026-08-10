## MODIFIED Requirements

### Requirement: Manager fans out agent replies on multi-channel conversations
For conversations bound to one or more channels, `POST /work/done` SHALL fan the run's result out as one `send` op per bound channel targeting that channel's thread, carrying an `answer` message with the result as its markdown body. Failed or empty results SHALL fan out a short failure `notice` — mirrored surfaces never mistake silence for success. Each bound channel's adapter renders the same semantic message independently, so one answer MAY be presented differently per surface (chunked, collapsed, attached) without the manager knowing any transport's limits. Composition uses the existing op pipeline only.

#### Scenario: One answer, every surface
- **WHEN** a run completes with a result on a conversation bound to telegram and web
- **THEN** both channels receive the result as a message in the conversation's own thread

#### Scenario: Failure is visible on every surface
- **WHEN** a run finishes with status failed and no result
- **THEN** every bound channel receives a short failure notice in the thread

#### Scenario: The same answer may look different per surface
- **WHEN** a long answer fans out to two channels whose transports have different limits
- **THEN** each adapter renders it for its own surface — one may split it, another may show it whole — from the identical semantic message

#### Scenario: Relayed user messages are semantic too
- **WHEN** a user message on one bound channel is mirrored to its siblings
- **THEN** each sibling receives a `relay` message carrying origin, sender, and body as fields, and composes the attribution itself
