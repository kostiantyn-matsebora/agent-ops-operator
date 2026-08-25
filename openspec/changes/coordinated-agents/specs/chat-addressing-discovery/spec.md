## ADDED Requirements

### Requirement: An Agent is never addressable from a surface

`/pipelines` and every discovery path SHALL list Pipelines (and Coordinators
claiming the surface's source) only. An `Agent` SHALL have no chat command and
SHALL appear in no menu; reaching one from a surface is wrapping it in a
Pipeline.

#### Scenario: Agents absent from the menu
- **WHEN** a surface's discovery list is built
- **THEN** it names no Agent, whether or not one is wired
