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

## ADDED Requirements

### Requirement: A member's own input is not shown twice when rebuilding from runs
Each entry of `status.runs[].inputs[]` SHALL carry an `origin` field, one of
`signal`, `channel` or `member` — HOW the input reached the manager, never
WHERE it was typed.

`Surface` already carries where. `origin` exists because an `OriginMember`
input and a genuine surfaceless signal (an alert, a job tick) both carry an
empty `Surface`, so `Surface` alone cannot tell them apart.

Rebuilding the transcript from `status.runs[].inputs[]` alone SHALL skip an
entry whose `origin` is `member` — a task `invoke` handed down, or a
member's result routed back up.

The invoke card already shows it, built straight from that member's own
transcript. Showing it again here would duplicate a member's result on the
page it was invoked from.

An entry recorded before the field existed carries none and SHALL be shown
exactly as it always has.

#### Scenario: A member's input is not shown twice
- **WHEN** the transcript is rebuilt from `status.runs[].inputs[]` and one
  entry's `origin` is `member`
- **THEN** that entry is skipped, and the member's own invoke card is the
  only place its content appears

#### Scenario: A surfaceless signal is still shown
- **WHEN** an alert or a job tick recorded an input with no `surface` and
  `origin` is `signal`, not `member`
- **THEN** the input is rendered, distinguishing it from coordination
  plumbing

#### Scenario: An older input has no origin
- **WHEN** a run recorded before the field existed is rendered
- **THEN** its inputs render exactly as they always have, nothing skipped
