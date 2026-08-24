## MODIFIED Requirements

### Requirement: Reads and mutations are separate toolsets
The mutating toolset SHALL render on its OWN setting, and SHALL NOT appear as a
consequence of a release-wide permission mode.

It was derived: widening one value rendered the mutating toolset, dropped the MCP
server's read-only flag and widened that server's RBAC together. Moving as a
group was deliberate — both walls sit on one path, and fixing one leaves the hole
one indirection along — but the value driving it named none of the three, so what
an install actually granted could not be read off its values.

They SHALL still be able to move together, stated as such. What is refused is a
setting whose NAME describes none of what it changes.

The split itself is unchanged: reads and mutations stay separate toolsets, each
enumerated and never wildcarded, so a route binds one without the other.

#### Scenario: Mutations are asked for, not inherited
- **WHEN** an install wants the mutating toolset
- **THEN** it enables that toolset, and no release-wide permission value renders it instead

#### Scenario: The walls can still move together
- **WHEN** an install wants an acting agent
- **THEN** it can state the mutating toolset and the writable server together, and each remains visible in its own values

#### Scenario: Read-only server grants no mutating tools
- **WHEN** the bundle deploys the MCP server in read-only mode
- **THEN** only the read toolset renders, and no toolset names a mutating tool

#### Scenario: Write-mode server renders both
- **WHEN** the deployed server runs without read-only
- **THEN** both toolsets render, and binding writes is a separate decision from binding reads

#### Scenario: A route can read without mutating
- **WHEN** a Pipeline binds the read toolset alone
- **THEN** its conversations can inspect the cluster and cannot change it, even though the server serves mutating tools to other routes
