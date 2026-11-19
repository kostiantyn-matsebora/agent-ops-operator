# console-workspace-feedback Specification

## Purpose
TBD - created by archiving change console-visual-refresh. Update Purpose after archive.

## Requirements

### Requirement: The viewer's own actions show their state

An action the viewer takes SHALL show its state where the viewer took it, from
the moment it is taken.

| Action | States shown |
|---|---|
| Send a reply | sending, then delivered |
| Start a conversation | a placeholder row, then the real one |
| Close, reopen, exit, delete | the verb in progress on the row, then the result |
| A bulk action | a count against the total, then each failure named |

A failed action SHALL say what did not happen and SHALL offer a retry where one
is safe. An accepted action the manager performs later SHALL show as in
progress until the event that completes it arrives.

#### Scenario: A reply settles on its event

- **WHEN** the viewer sends a reply
- **THEN** it appears at once marked as sending
- **AND** it settles as delivered when the event naming it returns

#### Scenario: A failure offers a retry

- **WHEN** a close is refused
- **THEN** the row says why it was not closed and offers a retry

### Requirement: Work in flight is counted

The header SHALL show how many of the viewer's actions are awaiting their
outcome. It SHALL show nothing when there are none.

#### Scenario: The counter follows the work

- **WHEN** three actions are awaiting their events
- **THEN** the header shows three in flight, and clears as each settles

### Requirement: The connection is stated and its loss is honest

The header SHALL state whether the live stream is connected.

While it is not, the console SHALL keep showing the data it has, say that it is
as it was, and hold the progress of anything still waiting. When the stream
returns it SHALL catch up without a reload.

#### Scenario: A disconnect pauses, a reconnect resumes

- **WHEN** the stream drops and then returns
- **THEN** the header reports the loss and then the recovery
- **AND** waiting actions resume from where they were

### Requirement: Incoming events notify by appearing in place

An event the viewer did not cause SHALL be shown by its own content appearing
where it belongs, with a brief emphasis. It SHALL NOT raise a banner, a toast
or a modal.

| Event | Where it appears |
|---|---|
| A conversation opens | a new row at the top of the list |
| A reply arrives in another conversation | that row's preview, and its unread badge |
| A reply arrives in the open conversation | the thread, or a pill when the viewer is scrolled away |

A transient message SHALL appear only for something the viewer did. It SHALL be
an overlay that moves nothing.

#### Scenario: A new conversation is not announced

- **WHEN** a signal opens a conversation
- **THEN** its row appears at the top of the list and no banner appears

#### Scenario: A pill when scrolled away

- **WHEN** a reply arrives and the viewer is not at the bottom of the thread
- **THEN** a pill offers to jump to the new message and the thread does not scroll

### Requirement: Nothing changes size while it animates

An element that appears, changes state or disappears SHALL have its space
reserved before it does. Only opacity and position SHALL animate.

- A list row has a fixed height, with a reserved line for its lifecycle bar.
- A status, a badge or a preview line changing SHALL NOT change the row's
  height.
- A transient message SHALL overlay the page.
- A live run SHALL occupy one card of a fixed size.

A shift that is a consequence of an insertion, such as the rows below a new row
moving down once, is allowed. A shift that animates a size is not.

#### Scenario: No measured shift during a run

- **WHEN** a simulated conversation opens and runs to its reply
- **THEN** a layout-shift measurement over the header, the tab bar and every
  list row reports none beyond the single insertion

### Requirement: A live run is one stable card

While a run is working, the thread SHALL show one card of fixed size. It SHALL
carry the elapsed time and the latest event with a count.

It SHALL NOT add a line to the thread per event.

When the reply arrives the card SHALL be replaced by the reply. The events
SHALL fold into that reply's collapsed `Run details`.

This SHALL hold for a run the viewer started and for a run a signal started.

#### Scenario: Events are not left behind

- **WHEN** a run finishes
- **THEN** the thread holds the reply and a collapsed `Run details` line, and no
  per-event line

### Requirement: Motion respects the viewer

When the viewer prefers reduced motion, the look SHALL remove movement and
emphasis animation. State SHALL still be conveyed, by text and icon.

#### Scenario: Reduced motion

- **WHEN** the operating system asks for reduced motion
- **THEN** arrivals appear without movement and the working card does not pulse
