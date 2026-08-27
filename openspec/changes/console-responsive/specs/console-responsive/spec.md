## Purpose

The console renders on a narrow viewport: the navigation can be put away, a
table becomes a list of labelled cards, and no page scrolls sideways.

## ADDED Requirements

### Requirement: The navigation collapses on a narrow viewport
Below the tablet breakpoint the console SHALL hide the navigation sidebar and
offer a control in the masthead that opens it; above the breakpoint the
sidebar SHALL be shown and the control SHALL still toggle it. Resizing the
window SHALL NOT force the sidebar open.

#### Scenario: Phone viewport
- **WHEN** the console is opened at a viewport narrower than the tablet breakpoint
- **THEN** the sidebar is hidden, the page content takes the full width, and the masthead shows a control that opens the sidebar over the content

#### Scenario: Desktop viewport
- **WHEN** the console is opened at a viewport wider than the tablet breakpoint
- **THEN** the sidebar is shown beside the content and the masthead control collapses it

### Requirement: Tables stack into labelled cards on a narrow viewport
Below the tablet breakpoint every data table in the console SHALL render each
row as a card whose cells are prefixed by their column heading, so a row
remains readable without a header row above it. Columns declared low-value
for a narrow screen SHALL be omitted there rather than stacked.

#### Scenario: Conversations list on a phone
- **WHEN** the conversations list renders at a phone viewport
- **THEN** each conversation is one card showing at least its title, name, phase, pipeline and last activity, each value labelled with its column heading, and the selection control stays available

#### Scenario: Desktop rendering is unchanged
- **WHEN** any table renders above the tablet breakpoint
- **THEN** it renders as a conventional table with a header row and every column

### Requirement: No console page scrolls horizontally
At a phone viewport no console page SHALL be wider than the viewport. Content
that is wider than the column — a code block, a markdown table, the run
timeline — SHALL scroll inside its own box.

#### Scenario: Every route at a phone viewport
- **WHEN** each of the overview, queues, configuration, topology, conversations and conversation pages is rendered at a phone viewport
- **THEN** the document's scroll width does not exceed the viewport width

#### Scenario: Replying from a phone
- **WHEN** a person types a slash command into the composer at a phone viewport
- **THEN** the command menu opens above the composer within the viewport and the send control stays reachable

### Requirement: Filters collapse behind a toggle on a narrow viewport
Below the tablet breakpoint the conversations list SHALL keep its search box
visible and place the phase, pipeline and profile filters behind a toggle.

#### Scenario: Filtering on a phone
- **WHEN** the conversations list renders at a phone viewport
- **THEN** the search box is visible and the remaining filters open from a single control, applying exactly as they do on a desktop
