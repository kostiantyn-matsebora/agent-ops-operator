## MODIFIED Requirements

### Requirement: The overview page reports the installation and what is wrong with it
The console SHALL serve an overview covering:

- Chart version and `appVersion`.
- The manager's image, readiness, replica state and uptime.
- Every adapter with its image, readiness, port and served-CR count.
- Every runtime image.
- `MAX_ACTIVE_CONVERSATIONS` against runtime pods in use.
- Active conversations and queued inputs.

It SHALL also serve a rollup of EVERY condition across every watched kind
that is not `True`, newest first, each linking to its object.

Each rollup row SHALL carry the time its condition has held since, where
the object reports one, so the operator can tell a fresh failure from one
that has stood for days without opening the object.

#### Scenario: Health is answerable on one page
- **WHEN** any component reports a failing condition, or a pod is not ready
- **THEN** it appears in the overview's problem rollup with its reason, without the operator visiting another page

#### Scenario: Versions are concrete
- **WHEN** the overview is loaded
- **THEN** it names the image and version of the manager, every adapter and every runtime present in the namespace

#### Scenario: A stale failure is distinguishable from a fresh one
- **WHEN** two conditions appear in the rollup, one that started seconds ago and one that has held for days
- **THEN** each row shows its own condition's start time, and the newest-first order matches what the rows display
