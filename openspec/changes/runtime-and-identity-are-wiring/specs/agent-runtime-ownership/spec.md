## MODIFIED Requirements

### Requirement: One identity carries the agent's power, declared under global

The runtime ServiceAccount SHALL be
`global.agentops.runtime.serviceAccountName` (default `agentops-runtime`) — the
account a conversation runs under when neither its Pipeline nor its runtime
names another.

EXACTLY ONE DEFAULT SHALL EXIST PER RELEASE. More than one runtime
ServiceAccount MAY exist, because a `Pipeline` may name one and two routes on
one runtime image may legitimately hold different power.

`runtime-rbac.yaml` SHALL be able to render mode-driven bindings for more than
one account.

THE PARENT STILL OWNS EVERY SA IT RENDERS. A bundle naming an account in its
Pipeline values SHALL NOT create one — naming is a reference, and an account the
chart did not create is an external grant that neither the render nor a
reconciler refuses.

#### Scenario: One knob, one binding set
- **WHEN** `global.agentops.runtime.rbacMode=full` is set
- **THEN** a `cluster-admin` ClusterRoleBinding for the runtime SA renders, and no other values need to be set anywhere for the install to be internally consistent

#### Scenario: Targeted grants compose with the mode
- **WHEN** `rbacMode=readonly` and `rbac.runtime.clusterRoles` names an extra role
- **THEN** both the read-only objects and the extra ClusterRole/binding render against the same SA

#### Scenario: No second runtime identity
- **WHEN** any bundle is enabled
- **THEN** no bundle-named runtime ServiceAccount (such as `agentops-runtime-k8s`) renders. A second account is the PARENT's to render and a Pipeline's to name — never a bundle's to invent

#### Scenario: An install that names none is unchanged

- **WHEN** no Pipeline names a service account
- **THEN** exactly one runtime ServiceAccount renders, from the parent, and
  every runtime pod uses it

#### Scenario: A second trust level needs no second runtime

- **WHEN** an install wants an observing route and an acting route on one
  runtime image
- **THEN** it renders a second ServiceAccount with its own RBAC and names it on
  the acting Pipeline — and does NOT clone the `AgentRuntime`

#### Scenario: A bundle still renders no runtime identity

- **WHEN** a bundle's Pipeline values name a service account
- **THEN** the bundle renders no ServiceAccount, no `AgentRuntime` and no
  runtime credential Secret

## ADDED Requirements

### Requirement: No runtime identity is cluster-admin, and none may read Secrets

The chart SHALL NOT bind `cluster-admin` to any runtime ServiceAccount. An
acting runtime SHALL instead be granted an ENUMERATED ClusterRole the chart
renders and owns.

**NO RUNTIME ROLE SHALL CARRY ANY VERB ON `secrets`** — not `get`, not `list`,
not `watch — and none SHALL carry a wildcard resource or apiGroup that would
reach them.

An agent has a shell. `cluster-admin` therefore means every credential in the
cluster is readable by a model, and `--allowedTools` does not constrain that:
the allowlist configures a COOPERATING agent, while a ServiceAccount binding is
what an uncooperative one actually has. The two must not be confused, and the
grant is the one that holds.

**THIS MIRRORS A RULE THE MANAGER ALREADY KEEPS.** The manager reads no Secrets
at all — everything secret-shaped compiles to `valueFrom` and the kubelet
resolves it. The component that runs untrusted model output SHALL NOT hold a
broader grant than the component that orchestrates it.

The enumerated role SHALL be reviewable as a list: an operator reading the chart
SHALL be able to see every verb an agent may use without resolving an aggregated
or built-in role.

Where an install genuinely needs a grant the chart does not render, it SHALL add
it as its own ClusterRole and name the ServiceAccount on a Pipeline — an
external grant, deliberate and separately reviewable, never a mode that widens
the shipped one.

#### Scenario: The full mode grants no cluster-admin

- **WHEN** `global.agentops.runtime.rbacMode` is `full`
- **THEN** no `ClusterRoleBinding` to `cluster-admin` renders, and the acting
  role is an enumerated one the chart defines

#### Scenario: An agent cannot read a Secret

- **WHEN** any runtime ServiceAccount the chart renders is checked against
  `secrets`
- **THEN** it is denied every verb, in every namespace, in every mode

#### Scenario: A wildcard is not a shortcut

- **WHEN** the acting role is written
- **THEN** it enumerates resources and verbs, because `*` on resources or
  apiGroups reaches Secrets without naming them

#### Scenario: A broader grant is the install's to make

- **WHEN** an operator needs an agent to do something the shipped role omits
- **THEN** they render their own ClusterRole and account and name it on a
  Pipeline, and the chart's own roles are unchanged
