## ADDED Requirements

### Requirement: The console's identity is published for subcharts, and kept honest by a guard

The console's signal source and channel names SHALL be published under
`global.agentops.console`, because a subchart can read no other parent scope and
a chart cannot derive one value from another.

That publication genuinely duplicates the console's own values. The chart SHALL
therefore FAIL THE RENDER when the two disagree, rather than document the
requirement and trust it:

- when the console is enabled and the published source or channel names
  something other than what the console renders, the failure SHALL name the
  value to set;
- when the console is disabled while a source or channel is still published, the
  render SHALL fail — a route claiming a source this release does not render
  reports itself wired and silently drops every signal to it.

The check SHALL run whether or not the console renders, so the disabled case —
the one it most needs to catch — is not skipped.

#### Scenario: Names disagree

- **WHEN** the console's source is renamed and the published name is not updated
- **THEN** the render fails, naming the value to set

#### Scenario: The console is turned off

- **WHEN** the console is disabled and a source or channel is still published
- **THEN** the render fails and says which values to clear

#### Scenario: The console is turned off cleanly

- **WHEN** the console is disabled and the published names are cleared
- **THEN** the release renders, and no route claims a console source

#### Scenario: Defaults agree

- **WHEN** the chart is installed with its defaults
- **THEN** the published names match what the console renders and nothing fails
