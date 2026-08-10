## MODIFIED Requirements

### Requirement: Conversations originate only from claimed signal sources

A Conversation SHALL be created only from a signal routed against a
`SignalSource` served by a Ready `Pipeline`. A message arriving on a channel's
general surface (no thread id) SHALL be delivered as a normalized signal from a
chat `SignalSource`, not created directly by the channel router. No channel
SHALL supply a default profile, and resolution SHALL NOT fall back to any
creation-timestamp ordering among pipelines.

A bare general-surface message — one addressing no Pipeline — SHALL be routed
only when the answering Pipeline is UNAMBIGUOUS:

- exactly ONE Ready Pipeline lists the chat source → the message routes to it;
- TWO OR MORE list it → no Conversation is created and the surface is answered
  with the Pipelines available and the addressed form to use;
- NONE list it → unchanged: `Wired=False` and the drop reason returns to the
  surface.

An addressed message SHALL be routed by the name it carries, independent of
which Pipelines list the source and independent of how many do.

Messages arriving with a thread id SHALL be unaffected: they continue an
existing Conversation, never travel the signal path, and SHALL NOT require an
address.

#### Scenario: Bare chat message opens a conversation when one agent serves the surface

- **WHEN** a user sends `check the disk` on a channel's general surface and
  exactly one Ready Pipeline lists the paired chat `SignalSource`
- **THEN** the signal core creates the conversation with that Pipeline's profile
  and channel set, and the source's `receivedTotal` increments

#### Scenario: Ambiguous bare message is answered with the choices

- **WHEN** a user sends a bare message on a surface two Ready Pipelines serve
- **THEN** no Conversation is created and the surface receives the available
  Pipelines with the `/<pipeline> <task>` form to use

#### Scenario: Addressing works however many pipelines serve the surface

- **WHEN** a user addresses a Pipeline by name on a surface served by several
- **THEN** the conversation is created for the named Pipeline, with its profile
  and capabilities, and no ambiguity arises

#### Scenario: Unclaimed chat source drops with a reason

- **WHEN** a general-surface message arrives for a chat source no Ready Pipeline
  serves
- **THEN** no conversation is created, the source reports `Wired=False`, and the
  user receives the drop reason on the originating surface

#### Scenario: No timestamp tiebreak remains

- **WHEN** two Ready Pipelines serve the same chat source and a bare
  general-surface message arrives
- **THEN** it is refused as ambiguous rather than resolved by pipeline creation
  order

#### Scenario: Thread replies need no address

- **WHEN** a user replies inside a conversation's thread with no prefix, on a
  surface served by several Pipelines
- **THEN** the reply is appended to that conversation as an input, unaffected by
  this rule

#### Scenario: Topic messages are not originations

- **WHEN** a message arrives in a topic the manager does not recognize
- **THEN** no conversation is adopted or created
