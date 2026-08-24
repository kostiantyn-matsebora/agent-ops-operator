## ADDED Requirements

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

WHERE NO DECLARED RUNTIME ANSWERS TO THAT NAME AND ANY PIPELINE DECLARES NONE,
THE RENDER SHALL FAIL, naming both the missing runtime and the routes that
needed it.

This REPLACES the rule that the parent always renders `default`. That rule was
what guaranteed a bundle-free install could execute, and it cannot survive the
runtime shipping in a bundle an operator may turn off. A guard that FAILS is the
honest replacement: the alternative is an install whose conversations reach
`Pending` forever with the reason in no one's view.

The check SHALL NOT depend on reading the cluster, so that it protects an
install rendered by a GitOps controller exactly as it protects an interactive
one.

#### Scenario: Turning off the only runtime is refused
- **WHEN** an install disables the bundle shipping its only runtime while a Pipeline declares no runtime
- **THEN** the render fails naming the missing `default` runtime and the routes that resolve to it

#### Scenario: A replacement satisfies the guard
- **WHEN** an install disables one runtime bundle and declares another answering to `default`
- **THEN** the render succeeds

#### Scenario: Every route naming its own runtime needs no default
- **WHEN** no runtime answers to `default` and every Pipeline names a runtime explicitly
- **THEN** the render succeeds, because nothing resolves to the missing name
