## MODIFIED Requirements

### Requirement: A change is approved for automatic fixing by its owner, once

The owner of a change SHALL approve it for automatic fixing ONCE, by a stated
label THEY place — on the change's pull request, or on the tracking issue as the
standing instruction that carries to every station. The label SHALL be honoured
only from a person whose write access is read from the platform.

**A SESSION SHALL NOT PLACE IT.** An automated session acts as an application
with no write access, so a label it places is refused and removed by the gate
that checks who labelled — measured live, on a pull request that then sat green
and unlabelled with no session left to act. Where the owner's grant must reach a
pull request the session opened, a WORKFLOW carries the owner's existing label
forward and records whose it was; the session opens its pull request carrying no
label at all.

The approval is the change's, not the finding's: the owner has read the proposal
— or, having placed the standing instruction on the issue, has said the issue is
to be built and its reviewers satisfied — and is saying "make the reviewers
happy", which is a decision about the change. What the reviewers then say is not
re-decided per thread.

#### Scenario: The owner approves the change

- **WHEN** the owner tells the session the change is approved for fixing
- **THEN** the owner places the label themselves, or places the standing
  instruction on the issue; the session places nothing and says so

#### Scenario: The owner approves interactively, on a workstation

- **WHEN** the owner asks an interactive session, running on their workstation
  where `gh` is already authenticated as them, whether the change is approved
  for fixing, and confirms it is
- **THEN** the session tells the owner the command rather than running it,
  and only the owner themselves — typing it in their own terminal, never the
  session — executes it

#### Scenario: The owner approves the pull request

- **WHEN** the owner places the approve label on a change's pull request
- **THEN** every open finding is fixed or disputed without a reply in each
  thread

#### Scenario: The owner placed the standing instruction on the issue

- **WHEN** a change was started by the standing instruction on its issue, placed
  by a person with write access
- **THEN** the workflow places the approve label on the pull request when it
  opens, recording whose instruction authorised it, and the session places
  nothing

#### Scenario: A session places the label

- **WHEN** an automated session places the approve label on its own pull request
- **THEN** the label is removed with a visible refusal, and nothing is
  authorised

#### Scenario: The label is placed with nobody's word

- **WHEN** no person's grant stands behind a label, carried or placed
- **THEN** it is refused, and a pull request without one is triaged per thread
