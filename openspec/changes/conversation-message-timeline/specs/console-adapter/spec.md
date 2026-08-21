## MODIFIED Requirements

### Requirement: Fan-out traffic renders as a live transcript
`send` ops received by the console (agent results, acks, attributed relays, and the console's own users' messages delivered back) SHALL append to a bounded in-memory per-thread transcript streamed to connected browsers in real time. Relayed messages SHALL be rendered as attributed remote-user messages, visually distinct from agent output and from messages typed in the console itself.

The transcript SHALL be a CACHE of the conversation's durable message record, never its only copy. Every read SHALL merge the live buffer with that record, so a thread shows its full history whether or not the buffer still holds it — including the message that STARTED the conversation.

The console SHALL NOT reconstruct the record by watching the input queue, and SHALL NOT recover a message's text or its sender by matching text it posted earlier. Both were workarounds for messages that had no durable home and never arrived by the channel path, and they SHALL be removed rather than kept as a fallback.

A message SHALL be rendered ONCE however many paths carry it. A message typed in this console and delivered back by the manager is the same message, not two.

The speaker SHALL be named for a reader: a message SHALL be attributed to its sender when one is known, and otherwise to a word describing who spoke. Internal message-kind identifiers SHALL NOT be shown as a speaker's name.

#### Scenario: Sibling-channel relay is attributed
- **WHEN** a conversation is bound to both a telegram channel and the console, and a user replies in Telegram
- **THEN** the console transcript shows the relayed message attributed to its Telegram sender, not as agent output

#### Scenario: A conversation started from the console reads from its first message
- **WHEN** a person starts a conversation from the console composer and the agent answers
- **THEN** the transcript shows the question then the answer, in that order

#### Scenario: A message typed here appears once
- **WHEN** a person types a reply and the manager delivers that same message back to this console
- **THEN** the transcript shows one message, confirmed rather than duplicated

#### Scenario: Transcript is ephemeral, runs are durable
- **WHEN** the console restarts and a browser reconnects to a conversation
- **THEN** the view is rebuilt from the conversation's durable message record — every message, not only the answers — with the live transcript resuming from the restart point
