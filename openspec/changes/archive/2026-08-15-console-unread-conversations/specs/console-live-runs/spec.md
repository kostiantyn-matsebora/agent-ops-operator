## MODIFIED Requirements

### Requirement: Conversations are filterable and paginated server-side
The conversation list SHALL support filtering by phase, pipeline, profile, bound
channel, age, error state and unread state, sorting by last activity, and
server-side pagination. A namespace can hold thousands of conversations, so the
list SHALL never require the browser to hold them all, and SHALL state how many
matched beyond the page shown.

Unread state SHALL be evaluated server-side like every other filter, from the
console's own thread binding, so a narrowed list still reports a correct total
and pages correctly. The unread COUNT SHALL be computed before any filter is
applied, so narrowing the view never changes it.

Run history SHALL NOT be carried in list rows; a run count SHALL be carried
instead, alongside each row's read state.

#### Scenario: A busy namespace stays usable
- **WHEN** thousands of conversations exist
- **THEN** the list returns a bounded page with the total match count, and filtering narrows it server-side

#### Scenario: Finding the failures
- **WHEN** the operator filters to errored conversations
- **THEN** only conversations with a failed run or failing condition are returned

#### Scenario: Finding what is new
- **WHEN** the operator filters to unread conversations
- **THEN** only conversations whose console thread has activity newer than its read watermark are returned, with a correct total and pagination
