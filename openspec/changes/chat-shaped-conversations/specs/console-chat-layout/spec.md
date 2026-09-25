## Purpose

The console's conversations surface as one chat-shaped view: an inbox of scopes with counts, the list, the thread, and secondary views, with panes a viewer resizes and an inbox that collapses to icons.

## ADDED Requirements

### Requirement: Conversations are one view with four columns
The console SHALL present conversations as ONE view: a navigation rail, an
inbox column of scopes, the conversation list, and the thread pane. Opening a
conversation SHALL replace the thread pane and SHALL NOT leave the view.

The URL SHALL name the open conversation, so a link opens the view with that
conversation selected and the list beside it.

#### Scenario: Switching stays in place
- **WHEN** a conversation is open and the operator selects another row
- **THEN** the thread pane shows the second conversation and the list stays where it was

#### Scenario: A link lands in the view
- **WHEN** a link naming a conversation is opened
- **THEN** the view opens with that conversation in the thread pane and the list beside it

### Requirement: The inbox column lists scopes, each with its count
The inbox column SHALL list, in this order: the filters All, Unread, Working,
Mine, Errored and Incidents, then every Ready Pipeline and Coordinator, then
the manager's commands, then Closed. Each scope SHALL show its unread count
where one is non-zero, computed by the console-unread rule.

Mine SHALL mean conversations the viewer's own identity started or replied
in. Incidents SHALL mean root conversations that have members.

#### Scenario: A pipeline scope narrows the list
- **WHEN** the operator selects a Pipeline in the inbox
- **THEN** the list shows only conversations attributed to that Pipeline, and the scope's count matches the unread rows

#### Scenario: Counts are unread messages, not changed rows
- **WHEN** a run is dispatched on a conversation the viewer has read
- **THEN** no scope's count changes

### Requirement: Keyboard moves the selection and opens it
`↑` and `↓` SHALL move the highlighted row, `Enter` SHALL open it, and
`⌘`-click or `Ctrl`-click SHALL add a row to the selection without opening
it. `Esc` SHALL clear the selection.

#### Scenario: Arrow keys walk the list
- **WHEN** a row is highlighted and the operator presses `↓` then `Enter`
- **THEN** the next row is highlighted and then opened in the thread pane

### Requirement: Panes resize, and the layout is a per-viewer convenience
A splitter between the inbox and the list, and one between the list and the
thread, SHALL resize the adjoining panes by dragging.

A double-click on a splitter SHALL restore that pane's default width. Each
pane SHALL have a minimum width below which it cannot be dragged.

The widths and the inbox's collapsed state are the viewer's own:

- they SHALL be remembered in the viewer's browser and nowhere else
- they SHALL NOT be written to any Conversation or server-side state
- a browser that cannot store them SHALL fall back to the defaults without error

#### Scenario: A width survives a reload
- **WHEN** the operator widens the list pane and reloads the page
- **THEN** the list pane opens at the width they chose

#### Scenario: Storage unavailable
- **WHEN** the browser refuses local storage
- **THEN** the view renders at the default widths and every function still works

### Requirement: The inbox collapses to icons and keeps its badges
The inbox column SHALL collapse to an icon strip and expand again from a
control at its foot. Collapsed, every scope SHALL keep its icon, its name as
a hover label, and its unread badge.

#### Scenario: Collapsed badges still count
- **WHEN** the inbox is collapsed and a Pipeline scope has two unread conversations
- **THEN** that Pipeline's icon carries the badge 2

### Requirement: Secondary views are reached from the thread header
Runs, Graph, Sequence and YAML SHALL be reachable from the thread pane's
header and SHALL replace the transcript within the thread pane, never the
whole view. The transcript SHALL be the default.

#### Scenario: Opening the runs view
- **WHEN** the operator chooses Runs in the thread header
- **THEN** the run timeline replaces the transcript and the list stays beside it

### Requirement: The view fits narrow windows
Below a stated width the view SHALL show one column at a time — the list, or
the thread with a way back — so a phone or a narrow window is usable. No
column SHALL scroll sideways.

#### Scenario: A phone shows one column
- **WHEN** the window is narrower than the stated width
- **THEN** the list fills it, and opening a conversation shows the thread with a control back to the list
