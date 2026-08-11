# chat-addressing-discovery

## Purpose

Addressing a Pipeline by name only helps someone who already knows the name.
This capability covers how a chat surface tells a person what it can reach: a
typeahead offered where the client supports it, built from configuration the
surface already observes rather than from new access, plus a command-based path
that works on every surface — and an ambiguous bare message that names the
choices at the moment they are needed.

## Requirements

### Requirement: A surface offers the agents it can reach
Where a surface can offer input assistance, typing the address prefix SHALL
present the Pipelines reachable from that surface, so a person chooses from what
exists rather than recalling a name. The listing SHALL narrow as the person
types and SHALL insert the addressed form, leaving the cursor ready for the
task text.

Only Ready Pipelines SHALL be offered: an unready Pipeline names wiring that
does not resolve, and offering it would invite a request that cannot be served.

Each entry SHALL carry enough to choose between two agents — at minimum the
Pipeline name and the profile that answers for it.

#### Scenario: Prefix opens the listing
- **WHEN** a person types the address prefix at the start of a message
- **THEN** the Ready Pipelines are presented, each showing its name and answering profile

#### Scenario: Typing narrows the list
- **WHEN** a person continues typing after the prefix
- **THEN** the listing narrows to matching Pipelines and selecting one inserts the addressed form followed by a space

#### Scenario: Unready pipelines are not offered
- **WHEN** a Pipeline's wiring does not resolve
- **THEN** it is absent from the listing, matching what the surface's agent-listing command reports

### Requirement: Discovery reads what the surface already knows
The listing SHALL be built from configuration the surface already observes. It
SHALL require no additional Kubernetes permission, no new operator endpoint, and
no new CRD field. A surface that cannot see Pipelines SHALL NOT gain access in
order to offer this.

#### Scenario: No new access
- **WHEN** the discovery listing is served
- **THEN** it uses the surface's existing read-only view of Pipelines and adds no permission, endpoint, or field

### Requirement: Every surface keeps a discovery path that needs no client support
The agent-listing command SHALL remain available on every chat surface and SHALL
report the same Ready Pipelines the typeahead would offer. A surface whose
client cannot present a listing SHALL therefore still have a way to find out
what it can address.

An ambiguous bare message SHALL itself name the available Pipelines, so a person
who does not know the command is told at the moment it matters.

#### Scenario: Command works where typeahead cannot
- **WHEN** a person sends the agent-listing command on a surface with no input assistance
- **THEN** the same Ready Pipelines are posted to that surface

#### Scenario: Ambiguity teaches the form
- **WHEN** a bare message is refused as ambiguous
- **THEN** the reply names the Pipelines that serve the surface and shows the addressed form
