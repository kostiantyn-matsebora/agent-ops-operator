# console-live-runs (delta)

## ADDED Requirements

### Requirement: Conversations are filterable and paginated server-side
The conversation list SHALL support filtering by phase, pipeline, profile, bound channel, age and error state, sorting by last activity, and server-side pagination. A namespace can hold thousands of conversations, so the list SHALL never require the browser to hold them all, and SHALL state how many matched beyond the page shown.

Run history SHALL NOT be carried in list rows; a run count SHALL be carried instead.

#### Scenario: A busy namespace stays usable
- **WHEN** thousands of conversations exist
- **THEN** the list returns a bounded page with the total match count, and filtering narrows it server-side

#### Scenario: Finding the failures
- **WHEN** the operator filters to errored conversations
- **THEN** only conversations with a failed run or failing condition are returned

### Requirement: A conversation detail shows its whole record
The detail view SHALL present the transcript with a composer, the run timeline from `status.runs[]` (status, exit code, duration, result, and the inputs each run consumed), the input queue including unprocessed inputs, thread bindings per channel, the runtime pod, the conversation graph and sequence views, and the raw object.

#### Scenario: Every run is accounted for
- **WHEN** a conversation has run several times
- **THEN** each run is listed with its outcome and duration, and unprocessed queued inputs are visible as such

#### Scenario: Multi-channel bindings are visible
- **WHEN** a conversation is bound to several channels
- **THEN** each binding is shown with its channel and thread

### Requirement: The composer follows the channel contract
Messages typed in the console SHALL be submitted through `POST /channel/inbound` with the console thread id, entering the same router as any other channel, preserving serial and busy-ack semantics. The console SHALL never re-ingest its own outbound posts, and SHALL deduplicate at-least-once op delivery by op id.

Conversations the console started SHALL have a live composer without further wiring; for conversations started elsewhere on pipelines that do not list the console channel, the composer SHALL be absent with the reason and the exact patch that would join it.

#### Scenario: A reply travels the normal path
- **WHEN** a user sends a message in a joined conversation
- **THEN** it is queued and acked exactly as a message from any other channel

#### Scenario: Redelivery renders once
- **WHEN** a send op is delivered twice
- **THEN** the transcript shows it once

#### Scenario: No relay loop
- **WHEN** the console receives its own outbound post, or a relay from a sibling channel
- **THEN** it renders it, attributed, and never feeds it back inbound

## MODIFIED Requirements

### Requirement: Real-time run activity per pipeline
Run history and results SHALL be read from `Conversation.status`, which survives console restarts, and SHALL be presented per conversation (list, detail, run timeline) as well as aggregated per pipeline on the topology graph. The live transcript SHALL be in-memory and bounded, and its loss on restart SHALL cost only unscrolled live messages. Where the live overlay and CR status describe the same thing, CR status SHALL be authoritative.

Thread ids SHALL be derived from conversation UIDs so they survive restarts without stored state.

#### Scenario: A restart loses nothing durable
- **WHEN** the console restarts
- **THEN** run history and results are still shown from CR status, thread bindings still resolve, and only unscrolled live messages are gone

#### Scenario: Status wins
- **WHEN** a live message and CR status describe the same run result
- **THEN** the rendered result is the one from CR status

#### Scenario: Inflight run is visible live
- **WHEN** a Conversation's `status.inflight` is set while a work unit executes
- **THEN** connected browsers show that conversation as running on its pipeline without a page refresh, and the indicator clears when the run completes

#### Scenario: Run history with results
- **WHEN** a user opens a Conversation's runs view
- **THEN** each entry from `status.runs[]` is shown with its outcome and result content as recorded by the manager

