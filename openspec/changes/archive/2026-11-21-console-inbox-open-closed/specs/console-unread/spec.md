## MODIFIED Requirements

### Requirement: Unread is a server-side filter, and its count is computed before filtering
The list SHALL offer an unread-only filter evaluated server-side by the
counting rule alongside the existing filters, so pagination and the total
match count stay correct.

The badge counts conversations, not messages. This replaces the earlier rule that summed unread messages.

The response SHALL carry the number of unread ROOT conversations that are not closed, computed over ALL conversations BEFORE any filter is applied, so the badge never moves because a filter hid something.

A member conversation SHALL NOT be counted, and a closed conversation SHALL NOT be counted toward it.

The count per scope SHALL be available for the inbox.

Each scope's count SHALL be the number of unread root conversations the scope lists, with one rule for all of them: a scope other than Closed excludes closed conversations, and Closed counts closed ones only.

No request parameter SHALL change which conversations a count includes.

A count-only form SHALL be available for surfaces that need the number
without the rows.

**Scope.** One of the inbox's groupings of conversations, such as Open, one
pipeline, one coordinator or Closed.

#### Scenario: Unread-only narrows server-side
- **WHEN** the operator turns on the unread filter
- **THEN** only conversations with a count above zero are returned, with a total match count and correct pagination

#### Scenario: The badge is a sum
- **WHEN** two open root conversations hold one and three unread messages
- **THEN** the navigation badge reads two

#### Scenario: A member is not counted
- **WHEN** a root has three members and the root holds two unread messages
- **THEN** the navigation badge reads one

#### Scenario: A closed conversation is not counted toward the navigation badge
- **WHEN** a closed root holds an unread message
- **THEN** the navigation badge does not include it and the Closed scope's count does

#### Scenario: The count does not move when the view narrows
- **WHEN** the operator applies a phase filter that hides unread conversations
- **THEN** the navigation badge and every scope's count are unchanged

#### Scenario: The count is available without the rows
- **WHEN** a surface requests the count-only form
- **THEN** it receives the navigation count, the per-scope counts and the total, and no conversation rows
