## RENAMED Requirements

- FROM: `### Requirement: One identity carries the agent's power, declared under global`
- TO: `### Requirement: The floor identity can do NOTHING, and a release has many`

## MODIFIED Requirements

### Requirement: The main chart owns the default agent runtime

The parent chart SHALL render the default `AgentRuntime` from a top-level
`runtime:` component, enabled by default. The component SHALL own everything
describing how agents execute — image, LLM credential reference, idle TTL,
`nodeSelector`, resources, and the home volume — and SHALL render the runtime
named `runtime.name` (default `default`), which is the name a PIPELINE with no
`runtimeRef` falls back to.

**NO SUBCHART SHALL RENDER THE SUBSTRATE** — an `AgentRuntime`, a runtime
credential Secret, a home volume, or the FLOOR ServiceAccount. Bundles
contribute domain and reference what the parent provides.

**A BUNDLE SHALL RENDER THE ServiceAccounts ITS OWN ROUTES NEED**, which is the
reverse of the rule this requirement used to state, and is set out in its own
requirement below. The earlier blanket ban was protecting against a bundle
rendering an account sized for EVERYTHING. An account sized DOWN to one route is
the opposite, and only the bundle knows what its routes do.

A bundle MAY name a different runtime through its own Pipeline's `runtimeRef`,
but SHALL NOT create one.

`runtime.enabled: false` SHALL render nothing from the component, for installs
that manage `AgentRuntime` CRs themselves.

#### Scenario: A default install can execute a conversation
- **WHEN** the chart is installed with default values and no bundle enabled
- **THEN** an `AgentRuntime` named `default` renders, so any Pipeline reaching the manager resolves a runtime — and no bundle objects render

#### Scenario: A bundle install renders exactly one runtime
- **WHEN** the chart is installed with any combination of bundles enabled
- **THEN** exactly one `AgentRuntime` renders, from the parent. The number of ServiceAccounts is NOT one: the parent renders the floor and whatever `rbacMode` produces, and each bundle renders one per route it ships

#### Scenario: Chat-only install works
- **WHEN** only `telegram-bundle.enabled=true` is set
- **THEN** the install renders a runtime and can execute conversations started from chat, without enabling a Kubernetes bundle for its substrate — and `telegram-bundle` renders no ServiceAccount, because it ships no Pipeline

#### Scenario: Bring your own runtime
- **WHEN** `runtime.enabled=false`
- **THEN** no `AgentRuntime`, credential Secret, or runtime object renders from the component, and Pipelines resolve whichever runtimes the operator applied

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

#### Scenario: One knob, one binding set

- **WHEN** `global.agentops.runtime.rbacMode=full` is set
- **THEN** the acting account and its enumerated ClusterRole render together,
  and no other value need be set for the install to be internally consistent.
  It NO LONGER renders a `cluster-admin` binding, and it binds nothing to the
  floor account

#### Scenario: No second runtime identity

- **WHEN** any bundle is enabled
- **THEN** no bundle renders the SUBSTRATE — no `AgentRuntime`, no credential,
  no home volume, no floor account. A bundle DOES render an identity for each
  route it ships, which is the reverse of the earlier rule and is stated in
  its own requirement below

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

## ADDED Requirements

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

**NO `secrets` VERB IS NOT THE SAME AS CANNOT READ SECRETS, AND THE RULES ABOVE
DO NOT HOLD ON THEIR OWN.** The KUBELET resolves a Secret when it builds a pod.
An agent that may create a pod mounting one — or exec into a pod that already
has one — reads the value having never asked the API server, so `secrets: get`
is never evaluated and no rule refusing it applies. Demonstrated against the
shipped role on a live cluster: pod created, pod log read, secret value
returned, with all seven `secrets` verbs denied throughout.

**`global.agentops.runtime.allowPodExecution` SHALL therefore exist, SHALL
default to FALSE, and is what makes this requirement TRUE rather than merely
written down.**

It SHALL gate every write that PRODUCES OR ENTERS a pod — not `pods: create`
alone. Creating a `Job`, `Deployment`, `StatefulSet` or `DaemonSet` writes a pod
spec that can mount a Secret, and `update`/`patch` on an existing one edits that
spec. A flag gating the obvious verb while leaving the rest open reads as a
boundary and is not one.

With it off an agent SHALL remain a real operator: it reads what it is granted,
scales, restarts, evicts, cordons, deletes workloads, and creates or edits
ConfigMaps, Services, Ingresses, NetworkPolicies, PDBs, HPAs and PVCs. What it
loses is the ability to RUN NEW CODE — which on Kubernetes is the same
capability as reading a Secret.

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

#### Scenario: Pod execution is what actually closes the Secret path

- **WHEN** `allowPodExecution` is false
- **THEN** no agent identity may create a pod, exec into one, or create or patch
  a workload, and a pod mounting a Secret is refused

#### Scenario: Gating one verb would close nothing

- **WHEN** `allowPodExecution` is false
- **THEN** `jobs`, `deployments`, `statefulsets` and `daemonsets` are refused
  create and patch as well, because each writes a pod spec

#### Scenario: Turning it on is stated to cost every Secret

- **WHEN** `allowPodExecution` is true
- **THEN** the install output says plainly that an agent can read every Secret
  the grant reaches, rather than reporting the `secrets` rules as a boundary

#### Scenario: A broader grant is the install's to make

- **WHEN** an operator needs an agent to do something the shipped role omits
- **THEN** they render their own ClusterRole and account and name it on a
  Pipeline, and the chart's own roles are unchanged


### Requirement: The grant is cluster-wide, and what protects the operator is omission

Agent grants SHALL be `ClusterRole`s bound cluster-wide. There SHALL be no
namespace allow-list, so a namespace created after the install is covered the
moment it exists.

**WHAT PROTECTS THE OPERATOR'S OWN NAMESPACE IS WHAT IS NEVER GRANTED, NOT
SCOPE.** RBAC is deny-by-default, and no agent role SHALL name:

- **`agentops.dev`**, so `Conversation`s, `Pipeline`s and profiles are
  unreadable everywhere, including the namespace they live in.
- **`secrets`**, in any verb.
- **`clusterroles` / `clusterrolebindings`** — that listing maps every identity
  in the install and which one is worth attacking, and being cluster-scoped it
  could not be narrowed anyway. Namespaced `roles` and `rolebindings` stay
  readable so an agent can explain a `Forbidden` it hits.

No component in the release SHALL log message content, so the pod logs an agent
can read carry no conversation text.

**NAMESPACED ROLES WERE BUILT AND REVERTED, and the reason is recorded so the
next reader does not rebuild them.** RBAC cannot express "everywhere except", so
bounding an agent meant an allow-list: one binding per namespace per account —
224 objects on a 28-namespace cluster — and every new namespace invisible to the
agent until an operator edited values and redeployed. The maintenance cost
exceeded the exposure it bought, once the omissions above were understood.

**WHAT IT COSTS, AND IT IS STATED RATHER THAN HIDDEN:** under `full` an agent
can restart or delete workloads in the operator's own namespace, the manager and
the adapters included. It can disrupt its own supervisor. `NOTES.txt` SHALL say
so at install time.

BOTH WALLS SHALL CARRY THE SAME RULES, from the same helpers. An agent reaches
the cluster THROUGH k8s-bundle's MCP server, so granting one differently from
the other moves the hole rather than closing it.

#### Scenario: Conversations are unreadable everywhere
- **WHEN** any agent identity is checked against `conversations.agentops.dev` in any namespace
- **THEN** it is denied, in every mode, because the API group is never granted

#### Scenario: A new namespace needs no configuration
- **WHEN** a namespace is created after the release is installed
- **THEN** an agent can diagnose it immediately, with no values change and no redeploy

#### Scenario: The RBAC map is not readable
- **WHEN** any agent identity is checked against `clusterroles`
- **THEN** it is denied

#### Scenario: The grant is a handful of objects
- **WHEN** the release renders with `rbacMode: full` and every bundle enabled
- **THEN** the agent grant is a small fixed number of `ClusterRole`s and bindings, not one per namespace
