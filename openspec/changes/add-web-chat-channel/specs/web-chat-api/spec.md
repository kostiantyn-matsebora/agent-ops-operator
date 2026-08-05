# web-chat-api

## ADDED Requirements

### Requirement: Browser-facing chat endpoints
The manager's HTTP server SHALL expose, on the existing API port and without leader gating:
- `GET /chat/api/profiles` — available AgentProfiles
- `GET /chat/api/conversations?channel=<name>` — conversations referencing the named web Channel
- `POST /chat/api/conversations` (`{channel, profile?, agent?, text}`) — create a task conversation via the shared router (command syntax in `text` honored)
- `GET /chat/api/conversations/{name}` — the conversation transcript
- `POST /chat/api/conversations/{name}/messages` (`{text}`) — append a reply input via the shared router
- `GET /chat/api/conversations/{name}/events` — SSE stream of transcript updates and ephemeral acks

Existing endpoints (`/work`, `/work/done`, `/task`, `/ingest/*`, `/healthz`) SHALL be unchanged.

#### Scenario: Posting a message creates a conversation
- **WHEN** `POST /chat/api/conversations` is called with a valid channel, profile, and text
- **THEN** the server responds 202 with the created conversation name, and the Conversation CR carries a channelRef, profileRef, and one task input

#### Scenario: Endpoints serve on a non-leader replica
- **WHEN** a `/chat/api/*` request reaches a manager replica that is not the leader
- **THEN** the request is served normally

#### Scenario: Unknown conversation
- **WHEN** a transcript, message, or events request names a conversation that does not exist or does not belong to a web channel
- **THEN** the server responds 404

### Requirement: Transcript derived from the Conversation CR
The transcript SHALL be derived from the Conversation CR: user messages from `spec.inputs` and agent replies from `status.runs[].result`, ordered chronologically, with the conversation's current state (idle/inflight) included. To make replies useful, `POST /work/done` SHALL store results up to 16384 characters (raised from 2000), appending a truncation marker when the cap is hit.

#### Scenario: Transcript shows both sides
- **WHEN** a conversation has processed a task input and its run completed with a result
- **THEN** `GET /chat/api/conversations/{name}` returns the user message and the agent reply in order

#### Scenario: Long result is truncated with a marker
- **WHEN** an agent posts a `/work/done` result longer than 16384 characters
- **THEN** the stored result is truncated to the cap with an explicit truncation marker

### Requirement: SSE stream delivers updates without client polling
The events endpoint SHALL emit an event when the conversation's transcript or inflight state changes (detected via the manager's cached client at an interval of roughly 2 seconds) and when the web Provider publishes an ephemeral ack. The stream SHALL send periodic keep-alives so intermediaries do not drop idle connections.

#### Scenario: Reply arrives while browser is connected
- **WHEN** a run completes for a conversation with an open events stream
- **THEN** the subscriber receives an event containing the new reply within a few seconds, without reconnecting

### Requirement: Bearer-token auth on the chat API
When the manager is configured with a web chat token (`WEB_CHAT_TOKEN` env, chart-provisioned Secret injected as env — the manager performs zero Secret API reads), every `/chat/api/*` request SHALL require `Authorization: Bearer <token>` with constant-time comparison. Requests with a missing or wrong token SHALL get 401. When no token is configured, the chat API SHALL be open (explicit opt-out for trusted networks).

#### Scenario: Valid token accepted
- **WHEN** a request carries the bearer token matching the channel's auth Secret
- **THEN** the request is processed

#### Scenario: Missing or wrong token rejected
- **WHEN** a request to a token-protected chat API has no token or a wrong token
- **THEN** the server responds 401 without touching any Conversation

#### Scenario: Token rotation takes effect
- **WHEN** the auth Secret's value is changed and the manager pod restarts (env-sourced)
- **THEN** the new token is required and the old token stops working
