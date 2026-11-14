## ADDED Requirements

### Requirement: The chart names the console's default look

The chart SHALL add `console.defaultLook`, one of `workspace` and `classic`,
with the default `workspace`.

The value SHALL reach the console the way `console.write.enabled` does, as a key
of the console Channel's `config` with a `configSchema` property. The console
SHALL return it with the session. A value that is neither SHALL fail the
render and name both accepted values.

Changing the value SHALL NOT change any viewer's stored choice.

#### Scenario: The default renders

- **WHEN** the chart is rendered with no value for `console.defaultLook`
- **THEN** the console is told `workspace`

#### Scenario: An unknown value fails the render

- **WHEN** the chart is rendered with `console.defaultLook: sparkly`
- **THEN** the render fails and names `workspace` and `classic`
