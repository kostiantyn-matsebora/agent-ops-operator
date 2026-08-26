## MODIFIED Requirements

### Requirement: Exactly one runtime answers to the default name, and its absence FAILS the render
`default` SHALL be the name a `Pipeline` declaring no runtime resolves to.

EVERY RUNTIME SHALL RENDER UNDER ITS OWN NAME — a bundle's under the name its
values state (`claude`, `ollama`), a `runtimes:` entry under its `name` — so a
route can select any of them by name. The parent chart SHALL render ONE MORE
`AgentRuntime` named `default`, a copy of one declared runtime, annotated with
the name it was copied from and rendering no credential Secret of its own.

WHICH ONE is decided by a per-runtime flag, `default: true`, accepted on a
`runtimes:` entry and on each runtime bundle, and OFF by default. With none
flagged, the FIRST CONFIGURED runtime is the default: `runtimes:` entries in
order, then the bundles in the order the chart lists them. One runtime alone is
therefore always the default. Two or more flagged SHALL fail the render naming
them.

EVERY RUNTIME IS OPTIONAL. The reference runtime is the first one shipped and
the one ON by default, and nothing more: turning it off and another on moves
the default with no rename.

WHERE NO RUNTIME IS DECLARED AT ALL AND ANY PIPELINE DECLARES NONE, THE RENDER
SHALL FAIL, naming the routes that needed it. The check SHALL NOT depend on
reading the cluster, so that it protects an install rendered by a GitOps
controller exactly as it protects an interactive one.

#### Scenario: The only runtime is the default without being renamed
- **WHEN** an install disables the reference bundle and enables one other runtime, while a Pipeline declares no runtime
- **THEN** the render succeeds, and the CR named `default` is a copy of that runtime

#### Scenario: The first configured is the default when none is flagged
- **WHEN** two runtimes are declared and neither carries `default: true`
- **THEN** the CR named `default` is a copy of the first configured

#### Scenario: The flag moves the default
- **WHEN** two runtimes are declared and one carries `default: true`
- **THEN** the CR named `default` is a copy of the flagged one, and both keep their own CRs

#### Scenario: Two flags are refused
- **WHEN** two runtimes both carry `default: true`
- **THEN** the render fails naming both

#### Scenario: A replacement satisfies the guard
- **WHEN** an install disables one runtime bundle and declares another
- **THEN** the render succeeds, and the CR named `default` is a copy of the replacement

#### Scenario: Turning off the only runtime is refused
- **WHEN** an install disables the bundle shipping its only runtime while a Pipeline declares no runtime
- **THEN** the render fails naming the routes that resolve to `default`

#### Scenario: Every route naming its own runtime needs no default
- **WHEN** no runtime is declared and every Pipeline names a runtime explicitly
- **THEN** the render succeeds, because nothing resolves to the missing name
