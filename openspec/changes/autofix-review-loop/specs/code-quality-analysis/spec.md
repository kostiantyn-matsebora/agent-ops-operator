## MODIFIED Requirements

### Requirement: A failed submission fails the pull request, and so does a failed quality gate

A scanner that did not run or could not submit its analysis SHALL fail the
job that tested the component, and that job SHALL report through the
always-present check. The quality gate's verdict on a submitted analysis SHALL
be a check the pull request cannot merge without, reported through the same
always-present check, so that a component's gate failing holds the merge
whether or not anyone reads the service.

The verdict was measured before it was required — the first change that
submitted analyses deferred the gate until the counts were known. It is
required now because it is the termination signal of the automatic fixing loop:
a loop that ended while the gate was red would end with the merge blocked by
the one check it never read.

#### Scenario: The scanner cannot submit

- **WHEN** the analysis cannot be submitted — a scanner error, an unreachable
  service, a rejected token
- **THEN** the job that tested the component fails, and the always-present
  check fails naming it

#### Scenario: The quality gate fails

- **WHEN** the submitted analysis fails the project's quality gate
- **THEN** the verdict is visible on the pull request
- **AND** the always-present check fails naming the component

#### Scenario: The quality gate passes on every analysed component

- **WHEN** every component the pull request touched passes its gate
- **THEN** the always-present check is unaffected by the analysis
