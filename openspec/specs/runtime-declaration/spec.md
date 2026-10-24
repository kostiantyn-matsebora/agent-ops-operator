# runtime-declaration Specification

## Purpose
How agent runtimes are DECLARED: the defaults every runtime inherits, the list an install declares, the runtime a bundle may ship, the `default` name a Pipeline with no `runtimeRef` resolves to, and the render-time guard that one must exist.

## Requirements

### Requirement: Defaults are a complete runtime, not the leftover fields
The chart SHALL expose ONE block of runtime defaults that is SUFFICIENT ON ITS
OWN. Supplying no runtimes at all SHALL yield one working runtime named
`default`, and the only value an operator SHALL be required to add is the model
credential — a secret has no defensible default.

"Defaults" SHALL NOT mean the fields left over after a list took the interesting
ones. A defaults block that omits the image, the context storage shape or the
resource requests is a block that documents nothing and configures nothing, and
every install then restates the same values.

RESOURCE REQUESTS AND LIMITS SHALL BE STATED IN THE DEFAULTS, never left empty
to be filled in by a number compiled into the operator. The numbers exist either
way; what an empty block costs is that no operator can see or tune them, and the
first sign of one being wrong is an evicted conversation.

#### Scenario: No runtimes declared still executes
- **WHEN** an install supplies the model credential and nothing else
- **THEN** one runtime named `default` is rendered from the defaults alone, and conversations execute on it

#### Scenario: The defaults are visible
- **WHEN** an operator reads the values
- **THEN** the CPU and memory a conversation pod requests and is limited to are stated there, not compiled into the operator

### Requirement: A runtime is one of several, declared by the install or by a bundle
Runtimes SHALL be DECLARED rather than singular. An install SHALL be able to
declare any number, and each SHALL state only what differs from the defaults.

A BUNDLE SHALL be able to ship a runtime, declaring it in its own values and
rendering its own CR — the same way a bundle already ships pipelines, sources
and profiles. A vendor that arrives as a bundle SHALL NOT require the operator
to hand-write a CR.

The defaults SHALL be reachable from a bundle. A bundle-declared runtime that
could not inherit them would restate the idle timeout, the resource requests and
the egress posture in every bundle, which is the same fact in as many places as
there are vendors.

#### Scenario: A second vendor states only its difference
- **WHEN** an install declares a second runtime naming a different image
- **THEN** it inherits every other value from the defaults, and the first runtime is unaffected

#### Scenario: A bundle brings its own runtime
- **WHEN** a bundle that ships a runtime is enabled
- **THEN** its runtime is rendered from the bundle's own values, inheriting the release-wide defaults

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
