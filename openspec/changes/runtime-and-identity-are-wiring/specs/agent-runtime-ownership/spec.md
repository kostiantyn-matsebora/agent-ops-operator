## MODIFIED Requirements

### Requirement: The floor identity can do NOTHING, and a release has many

`global.agentops.runtime.serviceAccountName` (default `agentops-runtime`) SHALL
name a MINIMUM-PRIVILEGE ServiceAccount the chart always renders: the identity a
conversation runs under when neither its Pipeline nor its runtime names another.

**THAT ACCOUNT SHALL CARRY NO RBAC AT ALL.** No ClusterRole, no Role, no
binding, in any mode. An agent whose route names no account SHALL be able to do
NOTHING in the cluster — it reaches only what its bound toolsets and MCP servers
give it, which are declared on the same object that could have named an account.

**SILENCE SHALL MEAN NO POWER, NEVER MAXIMUM POWER.** A route that inherits the
most powerful identity in the release by not typing a field is the defect this
requirement exists to forbid, and it is what shipped first: three of four routes
in the reference install held pod-delete and node-patch because nobody named an
account, two of them routes that reach no Kubernetes API at all.

`global.agentops.runtime.rbacMode` SHALL NOT bind anything to the floor account.
It declares that an ACTING account exists and what it may do, and a route opts
into that account by NAME.

"EXACTLY ONE RUNTIME SERVICEACCOUNT PER RELEASE" NO LONGER HOLDS AT ALL. A
release SHALL have as many as its routes need:

| Account | Rendered by | Bound to |
|---|---|---|
| the floor | the parent, always | nothing, ever |
| a bundle's route accounts | that bundle | what that bundle's own routes do |
| an operator's accounts | the parent, from values | what the operator declares |

`runtime-rbac.yaml` SHALL render mode-driven bindings for more than one account.

A Pipeline MAY name an account the chart did not create. That is an external
grant, the same posture adapters already have, and neither the render nor a
reconciler SHALL refuse it.

#### Scenario: A route that names nothing can do nothing
- **WHEN** a Pipeline declares no `serviceAccountName`
- **THEN** its conversations run under the floor account, which is denied every verb on every resource — including while `rbacMode` is `full`

#### Scenario: The mode never widens the floor
- **WHEN** `global.agentops.runtime.rbacMode=full` is set
- **THEN** the acting ClusterRole renders and is bound to a NAMED acting account, and NO binding of any kind names the floor account

#### Scenario: Targeted grants compose with the mode
- **WHEN** `rbacMode=readonly` and `rbac.runtime.clusterRoles` names an extra role
- **THEN** both the read-only objects and the extra ClusterRole/binding render against the acting account, and neither reaches the floor

#### Scenario: A second trust level needs no second runtime

- **WHEN** an install wants an observing route and an acting route on one
  runtime image
- **THEN** it renders a second ServiceAccount with its own RBAC and names it on
  the acting Pipeline — and does NOT clone the `AgentRuntime`

### Requirement: A bundle renders the identities its own routes need

A bundle that ships `Pipeline`s SHALL also render the ServiceAccounts and RBAC
those Pipelines require, scoped to what those routes actually do, and SHALL name
each on its own route.

THE BUNDLE IS THE ONLY SCOPE THAT KNOWS. `k8s-bundle` knows its acting route
deletes pods and its observing route does not. `ha-bundle` knows neither of its
routes touches the Kubernetes API. Routing every account through the parent
makes the parent restate each bundle's needs, and in practice yields one shared
account sized to the most demanding route.

**THE SUBSTRATE STAYS THE PARENT'S, EXCLUSIVELY.** No bundle SHALL render an
`AgentRuntime`, a model credential, a home volume or the floor account. The
earlier rule — "no subchart renders a runtime ServiceAccount" — was protecting
against a bundle rendering THE SUBSTRATE, including an account sized for
everything. An account sized DOWN to one route is the opposite, and is required.

A bundle that ships NO Pipeline SHALL render no account.

#### Scenario: A bundle's route carries the bundle's account
- **WHEN** `k8s-bundle` renders its acting route
- **THEN** it also renders that route's ServiceAccount and RBAC, and names the account on that Pipeline

#### Scenario: A bundle whose routes need no cluster access renders no grant
- **WHEN** `ha-bundle` renders its two routes
- **THEN** each names an account with no Kubernetes RBAC, because neither route reaches the Kubernetes API

#### Scenario: A bundle shipping no route ships no identity
- **WHEN** `telegram-bundle` is enabled
- **THEN** it renders no ServiceAccount, because it ships no Pipeline

#### Scenario: A bundle still renders no substrate

- **WHEN** any bundle is enabled
- **THEN** it renders no `AgentRuntime`, no runtime credential Secret and no
  home volume, and does not render the floor account

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


### Requirement: An agent's grant is scoped to namespaces, and never the operator's own

Every write verb, and every read of a NAMESPACED resource, SHALL be granted
through `Role`s bound only in the namespaces an operator names
(`global.agentops.runtime.namespaces` / `.writeNamespaces`). Only resources that
have NO namespace — nodes, namespaces, storage classes, CRDs, the kubelet stats
endpoint — SHALL be granted cluster-wide.

**THE RELEASE'S OWN NAMESPACE SHALL BE REFUSED IN BOTH LISTS, BY THE RENDER.**
It holds the manager, the adapters, the MCP servers and every `Conversation`
object. An agent that can READ there sees every other conversation's content and
the wiring that constrains it. One that can WRITE there rewrites that wiring.

WRITES SHALL BE SCOPED SEPARATELY FROM READS, because they are a different
decision. `pods: create` in a namespace lets a pod spec name any ServiceAccount
IN THAT NAMESPACE, and the kubelet mounts its token — so a write grant is a path
to every identity wherever it applies, which walks around the rule refusing
Secrets. Enumerating verbs does not close that. Only scoping where they apply
does.

RBAC CANNOT EXPRESS "EVERYWHERE EXCEPT", so this is an allow-list and the cost
is stated: an agent cannot diagnose a namespace nobody listed.

`clusterroles` and `clusterrolebindings` SHALL NOT be readable by any agent
identity. That listing is a map of every identity in the install and which one is
worth attacking, and it is cluster-scoped so it cannot be narrowed. Namespaced
`roles` and `rolebindings` stay readable where an agent already works, so it can
explain a `Forbidden` it hits.

BOTH WALLS SHALL BE SCOPED IDENTICALLY. An agent reaches the cluster THROUGH
k8s-bundle's MCP server, so that server's account carries the same split from the
same values. Scoping one and not the other moves the hole rather than closing it.

#### Scenario: The operator's namespace is refused
- **WHEN** the release namespace is named in `namespaces` or `writeNamespaces`
- **THEN** the render FAILS, naming the namespace and why it can never be granted

#### Scenario: An agent cannot read the operator's namespace
- **WHEN** any agent identity is checked against pods or `conversations.agentops.dev` in the release namespace
- **THEN** it is denied, in every mode

#### Scenario: Writes never render cluster-wide
- **WHEN** the acting grant is rendered
- **THEN** every write verb but node cordon appears in a namespaced `Role`, and the `ClusterRole` carries only reads plus that one cluster-scoped write

#### Scenario: Cluster-scoped reads still work
- **WHEN** an agent asks about nodes, namespaces or storage classes
- **THEN** it is allowed, because those resources have no namespace and nothing in them belongs to another tenant

#### Scenario: The RBAC map is not readable
- **WHEN** any agent identity is checked against `clusterroles`
- **THEN** it is denied
