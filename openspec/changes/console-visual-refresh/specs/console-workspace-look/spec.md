## ADDED Requirements

### Requirement: The workspace look presents conversations as one workspace

The `workspace` look SHALL present conversations as a navigation panel, a list
and a thread. Opening a conversation SHALL replace the thread and SHALL NOT
leave the view.

The navigation panel SHALL carry the pages, the channel, the conversation
scopes with their counts, and the pipelines and coordinators.

The list SHALL collapse to icons and the navigation panel SHALL collapse to
icons. Both SHALL keep their badges when collapsed. The viewer's pane sizes and
collapsed states SHALL be stored per viewer, under keys of this look.

#### Scenario: A link lands in the workspace

- **WHEN** a link naming a conversation is opened
- **THEN** the thread shows that conversation with the list beside it

#### Scenario: Collapsing keeps the counts

- **WHEN** the navigation panel is collapsed
- **THEN** each scope still shows its badge

### Requirement: The workspace fills the viewport

The workspace SHALL use the whole window. It SHALL NOT cap its width and leave
unused bands at the sides, however wide the window is.

The panes SHALL stretch to the window's height and each SHALL scroll inside
itself. The page SHALL NOT scroll.

#### Scenario: A wide screen is used

- **WHEN** the window is 2560 pixels wide
- **THEN** the panes reach both edges of the window, less the side gutter

### Requirement: The workspace fits tablets and phones

At tablet widths the navigation panel SHALL become a drawer and the list and
thread SHALL stay side by side where they fit. At phone widths the view SHALL
show one pane at a time, with a way back from the thread.

On a touch viewport every control SHALL be at least 44 pixels in its smaller
dimension. Tables and diagrams SHALL scroll inside their own container.

A tablet SHALL NOT be treated as a stretched phone or a squeezed desktop. Its
own layout SHALL be checked in both orientations.

#### Scenario: A phone shows one pane

- **WHEN** the window is phone width and a conversation is opened
- **THEN** the thread fills it with a control back to the list

#### Scenario: A tablet keeps the list beside the thread

- **WHEN** the window is tablet landscape width
- **THEN** the list and the thread are both visible and the navigation is a
  drawer

#### Scenario: Nothing scrolls sideways

- **WHEN** any page is shown at any named viewport
- **THEN** the document is no wider than the window

### Requirement: Every message can be copied

Every message in a thread SHALL have a copy control. It covers a person's
message, an agent's reply and a signal.

The control SHALL appear on hover or focus and always on a touch screen. It SHALL
take no space until shown, so no message changes size.

Activating it SHALL put the message's text on the clipboard, as the markdown the
agent wrote and not as rendered HTML. A signal SHALL copy its body and its
attributes as readable text.

The control SHALL confirm in place with a changed icon and label for a moment.
It SHALL NOT raise a toast. Where the clipboard is refused, it SHALL select the
text and say so.

#### Scenario: A reply is copied as written

- **WHEN** the viewer activates copy on an agent reply
- **THEN** the clipboard holds the reply's markdown and the control shows it was copied

#### Scenario: Copy is reachable on a phone

- **WHEN** the window is phone width
- **THEN** every message shows its copy control at least 44 pixels in its smaller dimension

#### Scenario: A refused clipboard

- **WHEN** the browser refuses clipboard access
- **THEN** the message text is selected and the control says to copy it by hand

### Requirement: A signal is distinct from a person's message

A signal that opened a conversation SHALL render in the thread with its own
background and its own attributes. It SHALL share a shape with a person's
message.

Severity SHALL be an icon, never a coloured chip. The values SHALL be the
contract's: `critical`, `error`, `warning` and `info`, and an absent value SHALL
read as unrated. A signal's other attributes SHALL be folded and open on
request.

#### Scenario: Severity is an icon

- **WHEN** a signal carries `severity: warning`
- **THEN** the thread shows a warning icon and no severity chip

#### Scenario: An unrated signal

- **WHEN** a signal carries no severity
- **THEN** it shows no severity icon

### Requirement: Run detail is folded and members are always visible

Under an agent's reply the thread SHALL show an `Invoked` row for each member
the reply opened, always, with a control that opens the member conversation.

Everything else about the run SHALL sit behind one collapsed `Run details` line.
That line SHALL show the run's duration. Opened, it SHALL show the tool calls
as one expandable row, the runtime events and the checkpoint.

A row SHALL NOT repeat a fact another row on the same reply already states.

#### Scenario: Details are collapsed by default

- **WHEN** an agent reply with a finished run is shown
- **THEN** only the reply, any `Invoked` rows and the collapsed `Run details`
  line are visible

#### Scenario: A member is reachable without opening details

- **WHEN** a reply opened a member conversation
- **THEN** its `Invoked` row is visible and opens that conversation

### Requirement: A list row has a stable shape

Every row SHALL have the same height whatever it is showing.

A row SHALL carry the title, the last message, the coordinator or pipeline, a
reserved line for the lifecycle bar, and on its right the time with the unread
badge beside it and a status pill below.

Row actions SHALL appear on hover or focus, and always on a touch screen.

#### Scenario: A status appears without moving the row

- **WHEN** a row's conversation starts working
- **THEN** the lifecycle bar fills its reserved line and no row changes height

### Requirement: The thread header carries the conversation's start

The thread SHALL show the date and time the conversation started at the right
end of the line under its tabs. A further date divider SHALL appear in the
thread only where the day changes.

Two dividers SHALL NOT sit next to each other where one already says why.

#### Scenario: A conversation spans two days

- **WHEN** messages fall on two calendar days
- **THEN** one day divider appears between them

### Requirement: Inline code is quiet

Inline code SHALL be set in monospace and SHALL be distinguishable from the
text around it without drawing attention from that text.

A command long enough to wrap SHALL render as its own block.

#### Scenario: Code reads as part of the line

- **WHEN** a message carries inline code
- **THEN** the sentence reads as one line of prose with the code marked, not as
  a row of boxes

### Requirement: The other pages keep their structure

The Overview, Queues, Configuration and Topology pages SHALL keep the sections,
the order and the fields they have under `classic`. The `workspace` look SHALL
change how they are drawn and SHALL NOT change what they say.

The Configuration browser SHALL keep its kind index, its per-kind tables and its
object pages with Overview and YAML.

#### Scenario: A page has the same sections in both looks

- **WHEN** the Overview page is shown in each look
- **THEN** both show the same cards and tables in the same order

### Requirement: The header carries only controls that work

The header SHALL show a control only where it does something in this
deployment. A sign-out control SHALL appear only where the console's
authentication offers one.

#### Scenario: No sign-out without a session

- **WHEN** the console is configured with a shared token and no session
- **THEN** the header shows no sign-out control
