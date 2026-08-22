# console-adapter Specification

## Purpose
TBD - created by archiving change visualize-agent-ops. Update Purpose after archive.
## Requirements
### Requirement: Console serves the channel adapter contract
The console SHALL be a conforming channel adapter: it long-polls `GET /channel/ops?adapter=console`, completes operations via `POST /channel/ops/{id}/done`, and submits UI-typed user messages via `POST /channel/inbound`, authenticating every call with its injected per-adapter derived token. It SHALL tolerate at-least-once op delivery by deduplicating on operation id.

#### Scenario: Ensure-topic binds a console thread
- **WHEN** the console receives an `ensure-topic` op for a conversation
- **THEN** it completes the op with a thread id derived deterministically from the conversation's UID (`console-<uid>`), so the same conversation maps to the same thread across console restarts

#### Scenario: Redelivered op renders once
- **WHEN** a `send` op is delivered twice (manager retry)
- **THEN** the transcript shows the message exactly once, deduplicated by op id

#### Scenario: UI message enters through the shared router
- **WHEN** a user types a message in a joined conversation's transcript view
- **THEN** the console posts it to `/channel/inbound` with the console thread id and it is queued/acked exactly as an inbound message from any other channel

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

### Requirement: The console never re-ingests its own outbound posts
Messages the console receives as `send` ops (including relays of its own users' messages fanned back by the manager) SHALL never be posted back to `/channel/inbound`. A UI-typed message SHALL be sent inbound exactly once and rendered locally as pending until the corresponding ack/relay op confirms it.

#### Scenario: No relay loop through the console
- **WHEN** the manager fans a console user's own message back to the console thread as part of multi-channel relay
- **THEN** the console renders it (confirming the pending message) and does not re-submit it inbound

### Requirement: Observed vs joined conversations
A conversation SHALL be **joined** when the console Channel holds a thread binding in `status.threads[]`, and **observed** otherwise. Observed conversations SHALL be fully visible — phase, runs, results, graph, sequence — but SHALL carry no composer, because there is no thread to post into.

Conversations the console **originated** SHALL arrive joined without any Pipeline edit: the router appends the originating channel, so a console-started conversation already holds a console thread. Joining a Pipeline SHALL therefore be required only to observe and reply to conversations that other signal sources started.

Where a conversation is observed and not joined, the UI SHALL state why and print the exact patch that would add the console Channel to that Pipeline's `channelRefs[]`. Neither the chart nor the console SHALL make that edit.

#### Scenario: Unjoined pipeline is watchable but not writable
- **WHEN** a Conversation belongs to a Pipeline whose channels do not include the console channel
- **THEN** the console shows its phase, runs, and results from CR status but offers no message input

#### Scenario: Self-started conversations are immediately replyable
- **WHEN** a user starts a conversation from the console against a pipeline that does not list the console channel
- **THEN** the conversation holds a console thread and its composer is live, with no Pipeline edit

#### Scenario: Another source's work is visible but read-only
- **WHEN** a conversation started by a cluster-event source belongs to a pipeline that does not list the console channel
- **THEN** it is fully visible, has no composer, and the UI shows the reason and the joining patch

#### Scenario: The console makes no wiring edits
- **WHEN** any unjoined pipeline is displayed
- **THEN** the console has written nothing to the Pipeline, and only prints the patch

### Requirement: The console holds a signal identity alongside its channel identity
The console SHALL serve both the channel adapter contract (carrying conversations) and the signal adapter contract (originating them), using two distinct derived tokens in one pod. Each token SHALL open only its own contract surface. The channel identity SHALL never be used to originate, and the signal identity SHALL never be used to reply.

#### Scenario: Identities do not substitute for one another
- **WHEN** the channel token is presented to `/signal/inbound`, or the signal token to `/channel/inbound`
- **THEN** the request is rejected

#### Scenario: One workload, two contracts
- **WHEN** the console is running
- **THEN** a single pod long-polls `/channel/ops` and posts to `/signal/inbound`, and no second workload exists for the signal role

