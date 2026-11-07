## MODIFIED Requirements

### Requirement: Volume reclamation runs outside the manager
Reclaiming storage SHALL be performed by a workload separate from the manager,
because it requires mounting the claims at their ROOT and the manager mounts no
PersistentVolume.

That workload SHALL run no agent code and SHALL use its own ServiceAccount. The
chart SHALL fail to render if this identity is configured to be the runtime
ServiceAccount.

It SHALL read conversations only through the manager's state read surface. It
SHALL hold no Kubernetes grant and perform no writes, since both stages of a
conversation's ending belong to the manager.

#### Scenario: The claim root is not handed to agents
- **WHEN** an operator configures the reclaiming workload to run as the runtime ServiceAccount
- **THEN** the render fails, because that identity executes agent code and per-conversation isolation exists to keep the claim root away from it

#### Scenario: Reclamation needs no write access
- **WHEN** the reclaiming workload runs
- **THEN** it lists conversations through the manager and makes no call to the Kubernetes API

#### Scenario: The workload holds no Kubernetes grant
- **WHEN** the chart renders the reclaiming workload
- **THEN** it renders no Role and no RoleBinding for it and mounts no ServiceAccount token
