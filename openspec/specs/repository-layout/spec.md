# repository-layout Specification

## Purpose
Where a component's source lives, and what its location means.

This repository ships many small containers. The tree is what a reader meets
first and what a new module has to be placed in, and two of its properties are
load-bearing rather than cosmetic: **a directory holds exactly what its
container builds from**, and **a directory's PATH is a published identity**.
Both are relied on by machinery that derives the release inventory
from the filesystem, so breaking either fails at release time rather than at
review time.

## Requirements

### Requirement: One container, one directory

Every published container image SHALL have exactly one directory holding its
source and everything its build reads.

**The CONTEXT is per-directory; the RECIPE need not be.** A component MAY be
built by a shared Dockerfile held outside its directory, with its OWN directory
as the build context — `docker build -f` separates the two. What this requirement
constrains is what a build may READ, and a component that reached outside its
context — `COPY ../shared` — could not be built by the same rule as every other
component. Shared code therefore lives inside the component that uses it until
something else needs it, at which point sharing is a decision with its own cost
rather than a side effect of where a file was put.

A component needing something the shared recipe does not do SHALL declare that by
putting its own Dockerfile in its directory, and an own Dockerfile SHALL always
win. The component inventory SHALL be the union of the directories bearing one
and those bearing a module manifest, so neither list restates the other.

#### Scenario: A component's source is complete where it lives

- **WHEN** a container image is built from its directory as the context
- **THEN** every file it needs is inside that directory, whether the recipe
  building it sits there or is the shared one

#### Scenario: A component is added

- **WHEN** a new container is introduced
- **THEN** it gets one directory, and the build machinery discovers it without
  any list being edited

### Requirement: A component's PATH is its published identity

A component's name SHALL be derived from its directory path, and SHALL be
unique across the repository.

A group whose name is PLURAL names a kind of component and SHALL contribute its
singular form as a prefix. A group whose name is SINGULAR is a namespace and
SHALL contribute nothing. The published image is that name, prefixed once by the
project.

The release inventory is derived from the filesystem, so moving or renaming a
directory renames an image that installs pin and that is never deleted once
published. Uniqueness SHALL be asserted rather than assumed: a derived name is
not unique by construction the way a single flat directory name was.

There SHALL be no exception for the repository root. A component whose Dockerfile
sat there would take its name from the checkout directory, which is not an
identity at all.

**A module's declared path SHALL follow its directory**, because a module that
claims a path where its own manifest does not sit cannot be resolved by anyone
outside the repository.

#### Scenario: The kind is said once

- **WHEN** a signal adapter's directory is read
- **THEN** the group names the kind and the leaf names the instance, and the
  published name carries the kind exactly once

#### Scenario: A rename is a rename

- **WHEN** a component's directory is moved or renamed
- **THEN** the published image name changes with it, and that is understood as
  a release decision rather than a tidy-up

#### Scenario: Two components cannot claim one name

- **WHEN** two directories would derive the same component name
- **THEN** the derivation fails loudly, because one image name cannot serve two
  components and a release tag would resolve to either

### Requirement: A component's directory states what it IS

Component directories SHALL be grouped by the kind of runtime element they are,
and a group SHALL be named for that kind rather than for what installs it or
what it integrates with.

| Group | Holds |
|---|---|
| platform | the product's own components — the manager, the console, the retention job, and the pod-side components that are transparent to whatever they sit beside |
| runtimes | components implementing the client side of the work contract |
| signals | components that push normalized signals to the signal contract |
| channels | components that serve the channel contract |
| gateways | components that speak no agent-ops contract at all, terminating a foreign transport and forwarding into ones that do |

**Grouping by what installs a component is a different view** — the chart
already carries it, and a component moving between the parent chart and a bundle
must not move its source.

Directories that are not components — the chart, the documentation site, the
specs, the repository's own tooling — SHALL NOT be placed in a component group.

#### Scenario: A new adapter places itself

- **WHEN** a signal adapter is added
- **THEN** its directory is a leaf of the signals group, decided by what it is
  and needing no further judgement

#### Scenario: A component that implements two contracts

- **WHEN** one container serves more than one contract
- **THEN** it still gets one directory, in the group that names what the
  component is, because a container is one element of the runtime view

#### Scenario: Packaging changes, the tree does not

- **WHEN** a component moves between the parent chart and a bundle
- **THEN** its source directory does not move
