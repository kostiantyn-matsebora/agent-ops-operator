# web-chat-ui

## ADDED Requirements

### Requirement: Embedded chat UI served by the manager
The manager SHALL serve a single-page chat application at `GET /chat/` from assets embedded in the binary (`go:embed`, no external fetches, no build toolchain), compatible with the distroless image. The UI shell SHALL be served without auth; API calls from it carry the bearer token the user provides, kept in browser local storage.

#### Scenario: UI loads with no external dependencies
- **WHEN** a browser requests `/chat/` from the manager (e.g., via port-forward)
- **THEN** the page renders using only embedded assets, with no requests to third-party hosts

#### Scenario: Token prompt on unauthorized
- **WHEN** the API returns 401 for the stored (or absent) token
- **THEN** the UI prompts for a token and retries after it is entered

### Requirement: Core chat interactions
The UI SHALL let the user: see conversations on the web channel with their state (idle/working), open one and read the transcript, start a new conversation (picking an AgentProfile or typing `/profile[:agent]` command syntax), and send follow-up messages to an existing conversation. It SHALL apply live updates from the SSE stream, including ephemeral acks, and indicate when transcript history may have been pruned.

#### Scenario: Start a conversation and see the reply
- **WHEN** the user starts a new conversation with a profile and a task message
- **THEN** the conversation appears in the list, shows a working state, and displays the agent's reply when the run completes — without a manual page refresh

#### Scenario: Follow-up on a busy conversation
- **WHEN** the user sends a message to a conversation whose agent is mid-run
- **THEN** the UI shows the queued/ack state and the message is processed after the current unit finishes

### Requirement: Agent reply rendering is sanitized
Agent replies SHALL be rendered supporting the chat HTML subset used by the message format spec (bold, italic, code, pre, links), and all other markup SHALL be escaped or stripped before insertion into the DOM.

#### Scenario: Formatted reply renders
- **WHEN** a reply contains `<b>`, `<code>`, or `<pre>` per the format spec
- **THEN** the UI renders the formatting

#### Scenario: Hostile markup neutralized
- **WHEN** a reply contains `<script>` or event-handler attributes
- **THEN** the markup is not executed and appears inert (escaped or stripped)
