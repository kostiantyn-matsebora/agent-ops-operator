## MODIFIED Requirements

### Requirement: Defaults are a complete runtime, not the leftover fields
The chart SHALL expose ONE block of runtime defaults that is SUFFICIENT ON ITS
OWN. Supplying no runtimes at all SHALL yield one working runtime named
`default`.

The only value an operator SHALL be required to add is the model credential.
A secret has no defensible default.

"Defaults" SHALL NOT mean the fields left over after a list took the
interesting ones. A defaults block that omits the image, the context storage
shape or the resource requests documents nothing and configures nothing.
Every install then restates the same values.

RESOURCE REQUESTS AND LIMITS SHALL BE STATED IN THE DEFAULTS, never left
empty to be filled in by a number compiled into the operator. The numbers
exist either way.

What an empty block costs is that no operator can see or tune them. The
first sign of one being wrong is an evicted conversation.

THE DEFAULTS SHALL ALSO STATE `runtimeClassName`, defaulting to unset.
Unset SHALL mean exactly what it means today: no `RuntimeClassName` on the
rendered pod, the cluster's own default kernel.

The operator SHALL NOT validate the value. A name the cluster has no
`RuntimeClass` for fails at pod admission, not at render time.

#### Scenario: No runtimes declared still executes
- **WHEN** an install supplies the model credential and nothing else
- **THEN** one runtime named `default` is rendered from the defaults alone, and conversations execute on it

#### Scenario: The defaults are visible
- **WHEN** an operator reads the values
- **THEN** the CPU and memory a conversation pod requests and is limited to are stated there, not compiled into the operator

#### Scenario: An unset runtime class changes nothing
- **WHEN** no runtime or defaults block sets `runtimeClassName`
- **THEN** conversation pods render with no `RuntimeClassName`, exactly as before this capability existed

#### Scenario: A named runtime class is never checked against the cluster
- **WHEN** the defaults or a declared runtime set `runtimeClassName` to a value naming no `RuntimeClass` in the cluster
- **THEN** the render and the reconcile both succeed, and the conversation pod fails at admission instead
