## MODIFIED Requirements

### Requirement: Activity badges on the topology
Pipeline AND Coordinator nodes SHALL carry live activity badges (counts of
active and recent Conversations) derived from the Conversation cache, so the
topology answers "what is running right now" at a glance.

A conversation a Coordinator caused — `spec.causedBy` resolving to it
directly, or the uncaused root it opened — SHALL be attributed to that
Coordinator's node, exactly as a Pipeline-opened conversation is attributed
to its Pipeline's node.

#### Scenario: Idle vs active pipelines distinguishable
- **WHEN** one pipeline has two conversations with inflight work and another has none
- **THEN** the first pipeline node shows an active count of 2 and the second shows idle

#### Scenario: A Coordinator's members count toward its node
- **WHEN** a Coordinator has opened a root conversation and several caused member conversations, some with inflight work
- **THEN** the Coordinator's node shows an active count matching how many of them have inflight work, and a recent count matching the total attributed to it

#### Scenario: A Coordinator with no running conversations looks like an idle pipeline
- **WHEN** a Coordinator node has zero attributed conversations
- **THEN** it renders exactly as an idle Pipeline node does, with no activity fact and no badge, and never as a newly rendered zero

### Requirement: Conversations are filterable and paginated server-side
The conversation list SHALL support filtering by phase, pipeline, profile,
bound channel, age, error state and unread state, sorting by last activity,
and server-side pagination.

A namespace can hold thousands of conversations. The list SHALL never
require the browser to hold them all, and SHALL state how many matched
beyond the page shown.

Unread state SHALL be evaluated server-side like every other filter, from
the console's own thread binding, so a narrowed list still reports a
correct total and pages correctly.

The unread COUNT SHALL be computed before any filter is applied, so
narrowing the view never changes it.

Every scope the inbox offers as a badge SHALL report a count equal to what
opening that scope returns. The Working, Mine and Errored scopes SHALL be
counted over their own filter predicate alone — phase, ownership or error
state.

Those three scopes SHALL NOT be additionally restricted to unread rows.

Only the Unread scope's count SHALL intersect with unread state, since
unread IS that scope.

Run history SHALL NOT be carried in list rows. A run count SHALL be carried
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

#### Scenario: A scope's badge matches its list
- **WHEN** three conversations are in phase `Working`, and one of the three is unread
- **THEN** the Working scope's badge reads 3, and opening the Working scope lists all three

#### Scenario: A row shows its last message
- **WHEN** an agent answered a conversation last
- **THEN** its row shows the agent as speaker and the answer's first line
