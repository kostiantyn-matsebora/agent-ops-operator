## MODIFIED Requirements

### Requirement: Conversations are one view with four columns
In the `classic` look the console SHALL present conversations as ONE view: a
navigation rail, an inbox column of scopes, the conversation list, and the
thread pane. Opening a conversation SHALL replace the thread pane and SHALL NOT
leave the view.

Other looks present conversations by their own capability. Every other
requirement in this capability describes the `classic` look and no other.

The URL SHALL name the open conversation, so a link opens the view with that
conversation selected and the list beside it. This SHALL hold in every look.

#### Scenario: Switching stays in place
- **WHEN** a conversation is open and the operator selects another row
- **THEN** the thread pane shows the second conversation and the list stays where it was

#### Scenario: A link lands in the view
- **WHEN** a link naming a conversation is opened
- **THEN** the view opens with that conversation in the thread pane and the list beside it
