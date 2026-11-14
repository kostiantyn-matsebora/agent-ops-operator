## ADDED Requirements

### Requirement: A look is a presentation over one behaviour layer

The console SHALL separate what it does from how it looks.

- A **model layer** SHALL own data fetching, the stream, state, commands and
  derived facts. It SHALL return plain data and callbacks.
- A **look** SHALL own markup, styling, motion and layout.

A look SHALL NOT fetch, subscribe or compute a fact the model layer already
provides. Two looks showing the same install SHALL therefore show the same
facts.

#### Scenario: A fact is computed once

- **WHEN** a conversation's unread count, status or ordering is needed
- **THEN** every look reads it from the model layer and none recomputes it

#### Scenario: A look changes nothing the manager sees

- **WHEN** the viewer switches look
- **THEN** no request is made to the manager and no write happens

### Requirement: The model layer holds no presentation

The model layer SHALL NOT import a component library, a stylesheet or any
look. A check that fails the build SHALL enforce it.

A look SHALL NOT import another look. Anything two looks share SHALL live in
the model layer or in a shared folder that holds no look-specific markup.

#### Scenario: The boundary is enforced

- **WHEN** a model-layer file imports a component library or a look
- **THEN** the unit-test tier fails and names the file

### Requirement: Looks are registered and one is the default

Each look SHALL be registered with an identifier, a label and the page
components it provides. The console SHALL ship `classic` and `workspace`.

`workspace` SHALL be the default for a viewer who has chosen nothing and an
install that has named nothing.

Adding a look SHALL require a new folder and one registry entry. It SHALL NOT
require a change to the model layer or to another look.

#### Scenario: A first visit uses the default

- **WHEN** a viewer opens the console with nothing stored
- **THEN** the `workspace` look is shown

#### Scenario: A look is added

- **WHEN** a developer registers a third look that implements the page contract
- **THEN** it appears in the switcher with no other source change

### Requirement: The viewer chooses a look and it persists

The header SHALL carry a control that lists every registered look and marks the
active one. Choosing one SHALL apply it without a reload and SHALL be stored
for that viewer in the browser.

A stored identifier that is no longer registered SHALL fall back to the
default.

#### Scenario: The choice survives a reload

- **WHEN** a viewer selects `classic` and reloads
- **THEN** the console opens in `classic`

#### Scenario: A removed look falls back

- **WHEN** the stored identifier names a look this build does not register
- **THEN** the default look is shown and the stored value is replaced

### Requirement: The install names the default look

An install SHALL be able to name the look a viewer gets when they have chosen
none. The console SHALL report that name to the browser with the session.

The look in force SHALL be resolved in this order:

1. a `look` query parameter,
2. the viewer's stored choice,
3. the install's named default,
4. the build's default, `workspace`.

An install default that names no registered look SHALL be ignored and the build
default used.

#### Scenario: The install pins classic

- **WHEN** the install names `classic` and a viewer has stored nothing
- **THEN** the console opens in `classic`

#### Scenario: A viewer's choice wins over the install

- **WHEN** the install names `classic` and a viewer has chosen `workspace`
- **THEN** the console opens in `workspace`

### Requirement: A look can be forced from the URL

A `look` query parameter naming a registered look SHALL select it for that
page load. It SHALL NOT overwrite the stored choice.

This exists so screenshots, documentation and end-to-end specs can pin a look.

#### Scenario: A link pins a look

- **WHEN** the console is opened with `?look=classic` by a viewer who stored
  `workspace`
- **THEN** `classic` is shown and the stored choice stays `workspace`

### Requirement: Colour scheme and look are independent

Light, dark and automatic SHALL remain a separate choice from the look. Every
look SHALL honour all three.

#### Scenario: Both choices combine

- **WHEN** a viewer selects `workspace` and dark
- **THEN** the workspace look renders with its dark colours, and switching to
  `classic` keeps dark

### Requirement: Every look meets the same behaviour

A look SHALL provide every page the console has and SHALL expose every action
the other looks expose.

The behaviour specifications of the console apply to each look.

The end-to-end specs SHALL run against each registered look. A selector a spec
relies on SHALL be a stable test identifier supplied by the model layer's
contract, not a class or a component name.

#### Scenario: A look that drops an action is rejected

- **WHEN** a look omits an action another look exposes
- **THEN** the per-look conformance run fails and names the action

#### Scenario: A spec runs against both looks

- **WHEN** the end-to-end tier runs
- **THEN** each spec is executed once per registered look

### Requirement: Every look is responsive and tested at set viewports

Every look SHALL be usable on a phone, a tablet and a desktop.

The console SHALL name a fixed set of viewports and the end-to-end tier SHALL
run each look at each of them.

| Name | Size | Pointer |
|---|---|---|
| phone | 390 by 844 | touch |
| tablet portrait | 768 by 1024 | touch |
| tablet landscape | 1024 by 768 | touch |
| laptop | 1280 by 800 | mouse |
| desktop | 1440 by 900 | mouse |
| wide | 2560 by 1300 | mouse |

At every viewport, in every look:

- No page SHALL scroll sideways.
- Every control SHALL be reachable.
- On a touch viewport every control SHALL be at least 44 pixels in its smaller
  dimension.

A change to a look SHALL NOT be finished until the viewport run passes for it.

#### Scenario: The viewport run covers every page

- **WHEN** the end-to-end tier runs
- **THEN** each page of each look is loaded at each viewport and checked for
  sideways scroll and touch-target size

#### Scenario: A regression at one viewport fails the build

- **WHEN** a change makes a page scroll sideways at tablet portrait
- **THEN** the run fails and names the look, the page and the viewport

