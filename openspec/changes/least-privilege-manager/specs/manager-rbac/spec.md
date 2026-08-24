## Purpose

What the operator's own Kubernetes Role may hold. The manager orchestrates
untrusted model output, so the component running the orchestration must not
out-rank the thing it orchestrates — and a permission it never exercises is
indistinguishable from one it needs until somebody checks.

## ADDED Requirements

### Requirement: The manager is granted only what it calls

The chart SHALL grant the manager's ServiceAccount a verb on a resource only
where the manager's own code exercises that verb.

A permission SHALL be removed when the caller that needed it is removed, in the
SAME change that removes the caller. A grant that outlives its caller is a
standing capability nobody reviewed, and it reads as intentional to every later
reader.

**A verb is "exercised" by non-test code**, which SHALL include an informer:
a controller watching or owning a type needs `list` and `watch` on it even where
no explicit read appears.

#### Scenario: A reconciler stops creating an object

- **WHEN** a change removes the code that created some kind of object
- **THEN** the chart's grant for that kind is removed in the same change
- **AND** any test that asserted the grant's SHAPE is rewritten to assert its
  ABSENCE, rather than left to fail

#### Scenario: A grant is audited

- **WHEN** a reader asks why the manager holds some verb
- **THEN** a caller in non-test code exercises it, or the grant is a defect

### Requirement: The manager holds no verb on ServiceAccounts

The chart SHALL grant the manager NO verb on `serviceaccounts` — not `create`,
and not `get`, `list` or `watch`.

**No reconciler creates a ServiceAccount**, which
`channel-adapter-lifecycle` and `signal-adapter-lifecycle` already require. This
requirement removes the permission that would let one, so the rule is enforced by
the cluster rather than trusted to the code.

**NAMING AN ACCOUNT IN A POD SPEC NEEDS NO VERB ON IT.** The API server resolves
the account when it admits the pod, so the manager assigns an identity to every
runtime pod and every adapter workload while holding nothing on the resource.

- **A name nothing backs is refused AT ADMISSION**, naming the account, and no
  requester permission is consulted.
- **This is what lets `Ready` decline to validate `spec.serviceAccountName`**
  — the reason `pipeline-model` gives for not checking it is that the manager
  holds no such read, and this requirement is what makes that sentence true.

#### Scenario: The manager assigns an identity it cannot read

- **WHEN** the manager builds a runtime pod or an adapter workload naming a
  ServiceAccount
- **THEN** the pod is admitted, the account binds and its token is projected
- **AND** the manager held no verb on `serviceaccounts` at any point

#### Scenario: A named account does not exist

- **WHEN** a workload names a ServiceAccount nothing rendered
- **THEN** the API server refuses the pod at admission, naming the account
- **AND** the manager reports no condition about it, because validating one
  would need the read this requirement forbids

#### Scenario: The grant is reintroduced

- **WHEN** a change adds any `serviceaccounts` verb to the manager's Role
- **THEN** the integration suite fails, naming the resource
