## Purpose

How the console shows that something is happening or has arrived: presence apart from unread, motion on arrival, the new-messages divider, the control that jumps to arrived messages, autoscroll, the notice that a conversation opened, and run events inline in the transcript.

## ADDED Requirements

### Requirement: Presence and unread are two signals
A conversation with a run inflight SHALL show presence on its row and in its
thread header. Presence SHALL never be counted as unread, and unread SHALL
never be inferred from presence.

The manager's acknowledgement notices SHALL render in the thread as a
presence row — the agent is working — and SHALL NOT render as a message
bubble and SHALL NOT count as unread.

#### Scenario: Your question shows working, not unread
- **WHEN** the operator sends a message and the manager acknowledges it and dispatches a run
- **THEN** the row shows presence, the thread shows the working row, and the row's unread count stays zero

#### Scenario: Presence ends with the run
- **WHEN** the run reports done
- **THEN** the presence indicator clears and the answer counts as unread for readers who have not seen it

### Requirement: Arrivals move
A conversation created while the list is open SHALL enter at its sorted
position with a transient highlight and a marker that it is new. A notice
naming the pipeline that opened it SHALL be shown briefly.

The marker SHALL clear when the conversation is first opened.

A message arriving in the open thread SHALL appear with a transient
highlight. Motion SHALL respect the viewer's reduced-motion preference.

#### Scenario: A new conversation announces itself
- **WHEN** an alert opens a conversation while the operator is on the view
- **THEN** its row appears highlighted with the marker new, and a notice names the pipeline

#### Scenario: Reduced motion
- **WHEN** the viewer's browser asks for reduced motion
- **THEN** rows and messages appear without animation, and the marker and notice still appear

### Requirement: The thread opens at the first unread message
Opening a conversation with unread messages SHALL place a divider above the
first counted message the reader has not seen and SHALL scroll to it. The
divider SHALL stay until the conversation is opened again.

#### Scenario: Opening lands on what is new
- **WHEN** the operator opens a conversation with three unread messages
- **THEN** the divider sits above the first of them and it is in view

### Requirement: Autoscroll follows only when at the bottom
While the thread is scrolled to its end, an arriving message SHALL keep it at
the end.

While it is scrolled up, an arriving message SHALL NOT move it. A control
naming how many messages arrived SHALL appear, and SHALL scroll to the end
when chosen.

#### Scenario: Reading history is not interrupted
- **WHEN** the operator has scrolled up and two messages arrive
- **THEN** the view does not move and a control reads that two new messages are below

### Requirement: Run events render inline in the transcript
These events SHALL render inside the transcript as muted one-line items at
their position in time, taken from the activity events the console already
receives:

- run dispatched
- runtime starting
- context restored, skipped or failed
- run completed

A run's events SHALL be collapsible to one line.

An inline event SHALL never count as unread.

#### Scenario: A run's story sits between the messages
- **WHEN** a run is dispatched, restores its context and completes
- **THEN** three muted lines sit between the input that caused it and the answer, collapsible to one

#### Scenario: Lost history is marked
- **WHEN** the activity buffer has a gap
- **THEN** the transcript marks the gap where the events are missing rather than showing the messages as adjacent

### Requirement: The thread header names the bound channels
The thread header SHALL name every channel the conversation is bound to,
so a reply typed here is known to reach the others.

#### Scenario: A multi-channel conversation says so
- **WHEN** a conversation is bound to the console and a chat channel
- **THEN** the header names both
