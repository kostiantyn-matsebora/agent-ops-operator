## MODIFIED Requirements

### Requirement: Conversations are filterable and paginated server-side
The conversation list SHALL support filtering by phase, pipeline, profile, bound
channel, age, error state, unread state, the viewer's own involvement, and
whether a conversation is the root of a coordination. It SHALL sort by last
activity and paginate server-side.

A namespace can hold thousands of conversations, so the list SHALL never
require the browser to hold them all, and SHALL state how many matched beyond
the page shown.

Unread state SHALL be evaluated server-side like every other filter, by the
console-unread counting rule over the console's own thread binding, so a
narrowed list still reports a correct total and pages correctly.

The unread SUM SHALL be computed before any filter is applied, so narrowing
the view never changes it.

Run history SHALL NOT be carried in list rows. Each row SHALL carry a run
count, its unread count, and the newest counted message's speaker and first
line, so a list reads as a chat list without fetching any transcript.

#### Scenario: A busy namespace stays usable
- **WHEN** thousands of conversations exist
- **THEN** the list returns a bounded page with the total match count, and filtering narrows it server-side

#### Scenario: Finding the failures
- **WHEN** the operator filters to errored conversations
- **THEN** only conversations with a failed run or failing condition are returned

#### Scenario: Finding what is new
- **WHEN** the operator filters to unread conversations
- **THEN** only conversations whose console thread holds counted messages after the reader's watermark are returned, with a correct total and pagination

#### Scenario: A row shows its last message
- **WHEN** an agent answered a conversation last
- **THEN** its row shows the agent as speaker and the answer's first line

### Requirement: A conversation detail shows its whole record
The thread pane SHALL present the transcript with a composer as its default
view, and SHALL offer secondary views holding:

- the run timeline from `status.runs[]`: status, exit code, duration, result, and the inputs each run consumed
- the input queue, including unprocessed inputs
- thread bindings per channel, and the runtime pod
- the conversation graph and sequence views
- the raw object

#### Scenario: Every run is accounted for
- **WHEN** a conversation has run several times
- **THEN** each run is listed with its outcome and duration, and unprocessed queued inputs are visible as such

#### Scenario: Multi-channel bindings are visible
- **WHEN** a conversation is bound to several channels
- **THEN** each binding is shown with its channel and thread
