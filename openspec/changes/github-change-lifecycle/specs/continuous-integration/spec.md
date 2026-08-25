## ADDED Requirements

### Requirement: The openspec artifacts are validated on every pull request

CI SHALL validate, on every pull request and every push to the default branch:

1. **every published specification**, always; and
2. **every change the pull request touches.**

It SHALL fail when any of them is invalid, naming what is invalid.

**`openspec/specs/` is the published answer to "is this behaviour intended", and
nothing checked that it parses.** A specification is trusted precisely because it
is not code — no compiler reads it, no test exercises it, and a malformed one is
indistinguishable from a correct one until somebody relies on it. That is the
whole argument for validating it mechanically, and unconditionally.

**A CHANGE IN FLIGHT IS JUDGED ONLY BY THE PULL REQUEST THAT TOUCHES IT.** A
dozen changes are open at any time, and a delta that is incomplete today is
incomplete correctly — the change is not finished. A check that judged all of
them would fail every pull request for work it was not about, and would be
switched off within a day. The scope is what makes the gate survivable, and
therefore what makes it a gate at all.

The check SHALL report through the always-present gate, so that requiring it
needs no change to branch protection.

#### Scenario: A change's artifacts are malformed

- **WHEN** a pull request carries a change whose artifacts do not validate
- **THEN** the check fails, names the change and states what is wrong

#### Scenario: A published specification is broken

- **WHEN** a pull request leaves a specification under `openspec/specs/` invalid
- **THEN** the check fails, whether or not the pull request is the thing that
  broke it

#### Scenario: An unrelated change is mid-flight and incomplete

- **WHEN** a pull request touches no openspec change, and another change in
  flight has an incomplete delta
- **THEN** the check passes, because that change is not what this pull request
  is about

### Requirement: A change's documentation task is verified on every pull request

CI SHALL fail a pull request that FINISHES an openspec change whose task list
does not end in a completed documentation section covering both the reference
docs and the adopter site.

**A pull request finishes a change two ways, and no others:** it ARCHIVES the
change — which is the claim of completion itself — or every task outside the
documentation section is complete. A change a pull request merely TOUCHES is not
judged on its documentation: a proposal's documentation tasks are unticked
because nothing has been written yet, and failing that pull request asks for the
documentation of work nobody has done.

**The check SHALL judge an archived change at its archived location.** Archiving
MOVES the change, so the live task list is absent from the tree the check reads —
and a check scoped to what still exists reports that the pull request touches no
change at all, at the one moment the rule exists to protect.

**Whether the last section IS a documentation section SHALL be judged whatever
the change's phase.** That is a property of the file rather than of the work, and
a proposal is the cheapest moment to catch a task list that never had one.

**The rule already existed and was enforced in one contributor's local
tooling.** A gate that lives in a harness is absent for every other
contributor, for every session whose tooling is not installed, and for
automation — and this repository is public, so "every other contributor" is now
a real population rather than a hypothetical one.

The check SHALL state both halves when it fails, because they are skipped
independently: a change feels finished once the reference docs are right, and
the adopter never reads the reference docs.

Local enforcement SHALL be retained rather than replaced. The local gate fails
open on anything it cannot read, which is what stops it being disabled; the
check is what makes failing open safe, by asserting the same decision where it
cannot be skipped.

#### Scenario: The documentation task is unticked

- **WHEN** a pull request finishes a change whose documentation tasks are not all
  complete
- **THEN** the check fails and names the outstanding tasks

#### Scenario: A change is proposed

- **WHEN** a pull request adds a change whose implementation tasks are all
  outstanding
- **THEN** the check reports it as pending and does not fail

#### Scenario: A change is archived

- **WHEN** a pull request archives a change
- **THEN** its documentation section is judged at the archived location

#### Scenario: The documentation section is missing entirely

- **WHEN** a change's task list does not end in a documentation section
- **THEN** the check fails, naming both halves such a section must cover

#### Scenario: The two enforcement points disagree

- **WHEN** the local gate and the check are given the same task list
- **THEN** they reach the same verdict, and a divergence between them fails a
  test rather than a pull request
