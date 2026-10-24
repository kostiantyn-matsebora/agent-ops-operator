## MODIFIED Requirements

### Requirement: One container, one directory

Every published container image SHALL have exactly one directory holding its
source and everything its build reads. **A directory is a CONTAINER, and a
container MAY hold several COMPONENTS** — each a package with its own stated
contract and its own dependency class — so co-locating components in one
image is a build decision that moves no source and changes no contract.

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

#### Scenario: Two components share an image

- **WHEN** two components with separate contracts are built into one directory
- **THEN** each keeps its package and its contract, and splitting them later is a new directory and no contract change
