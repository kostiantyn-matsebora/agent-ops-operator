## MODIFIED Requirements


### Requirement: Every pull request builds the container images it could have broken

CI SHALL build images from their Dockerfiles on every pull request with
publishing disabled, using layer caching to keep feedback fast. The image set
SHALL be derived from the Dockerfiles that exist rather than enumerated in
prose.

**IT SHALL BUILD WHAT CHANGED, NOT EVERYTHING.** A component SHALL be built when
a file inside its own directory moved. Thirteen image builds and twelve module
builds on a documentation-only commit is cost with no signal, and the wait it
adds is paid by every contributor on every push.

**THE FILTER SHALL BE DERIVED, LIKE THE MATRIX.** A list of paths maintained
beside the component list is a second thing to keep in step, and the one that
falls behind is the one nobody notices — which is the same argument that made
the matrix derived in the first place.

**FOUR KINDS OF FILE SHALL REBUILD EVERYTHING**, and they are the ones that
touch no component directory while invalidating every component: the shared
Dockerfile that is many components' recipe, the script that decides what
components exist at all, the workflow that decides how any of them is tested,
and the composite actions under `.github/actions/`, which is where the scan
every image runs through lives. A change to the scan that scans nothing has not
been tested, and reports success.

**WHERE THE COMPARISON BASE CANNOT BE ESTABLISHED, EVERYTHING SHALL BUILD.** A
branch's first push has nothing to compare against, and a shallow checkout makes
the comparison fail. Both SHALL build the world rather than build nothing: a
filter that silently matches nothing is a CI that silently tests nothing, and it
reports success while doing so.

#### Scenario: A Dockerfile breaks

- **WHEN** a pull request breaks any Dockerfile
- **THEN** that image's build job fails, naming the component

#### Scenario: A new image is covered without editing the workflow

- **WHEN** a Dockerfile is added to the repository
- **THEN** it is built by the same job without the image set being edited by
  hand

#### Scenario: One component changes

- **WHEN** a pull request touches a single component's directory
- **THEN** that component is built and every other component is skipped

#### Scenario: Only documentation changes

- **WHEN** a pull request touches no component directory
- **THEN** no module or image job runs at all

#### Scenario: The scan action changes

- **WHEN** a pull request changes anything under `.github/actions/`
- **THEN** every image is built and scanned on that pull request

#### Scenario: The shared recipe changes

- **WHEN** a pull request touches the shared Dockerfile, the component-discovery
  script, or the CI workflow itself
- **THEN** every component is built, because the change touches all of them
  without touching any of their directories

#### Scenario: There is nothing to compare against

- **WHEN** the base commit cannot be resolved, as on a branch's first push
- **THEN** everything is built rather than nothing

#### Scenario: No artifact escapes from a pull request

- **WHEN** CI runs for a pull request
- **THEN** no image or chart is pushed to any registry
