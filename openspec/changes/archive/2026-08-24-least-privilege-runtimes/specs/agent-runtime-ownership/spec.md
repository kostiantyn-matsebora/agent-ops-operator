## REMOVED Requirements

### Requirement: Unset RBAC mode grants nothing except in demo mode
**Reason**: The setting it governs is removed. `rbacMode` rendered an extra,
named ServiceAccount carrying a preset posture, and that account granted nothing
until a `Pipeline` named it — so the name described a mode the runtime was in,
which is the reading it caused an incident for and was reverted from.

Its one load-bearing behaviour was demo mode resolving empty to `readonly`. That
does not survive scrutiny: an agent reaches the cluster through the MCP SERVER,
which carries its own account and its own grant, and the runtime image ships no
kubectl. Demo's read access is the Kubernetes bundle's to provide, and that
bundle already renders one identity per route it ships.

The posture it protected — an ordinary upgrade widening nothing, and `full` never
inferred — is kept and strengthened below: the default is NO permissions, with no
mode able to change it.

### Requirement: The main chart owns the default agent runtime
**Reason**: Replaced, because both halves of the heading became false. The parent
no longer owns THE runtime — a bundle may ship one — and it no longer renders a
single `runtime:` component. What it still owns is the DEFAULTS every runtime
inherits and the FLOOR account, which the replacement states.

### Requirement: The floor identity can do NOTHING, and a release has many
**Reason**: Replaced. Every scenario it carried was written around `rbacMode` —
one knob rendering one binding set, the mode never widening the floor, targeted
grants composing with the mode. The knob is removed, so those scenarios describe
a mechanism that no longer exists.

The PROPERTY it protected is not lost. The replacement strengthens it: the floor
holds nothing, no setting can widen it, and there is no preset posture at all.

## ADDED Requirements

### Requirement: The main chart owns the runtime DEFAULTS, and a bundle may ship a runtime
The parent chart SHALL own the runtime DEFAULTS — a complete configuration every
runtime inherits — and the FLOOR ServiceAccount. It SHALL NOT be the only thing
that may render an `AgentRuntime`.

A BUNDLE SHALL be able to ship a runtime, declaring it in its own values and
rendering its own CR, exactly as a bundle already ships pipelines, sources and
profiles. A vendor arriving as a bundle SHALL NOT require a hand-written CR.

**THIS REVERSES A RULE THAT WAS AN INVARIANT, AND THE TWO FAILURES BEHIND IT ARE
ANSWERED RATHER THAN FORGOTTEN:**

1. **A chat-only install could execute nothing**, because the runtime lived in a
   bundle and no bundle was on. That is now caught: the render FAILS when no
   runtime answers to the default name while a Pipeline resolves to it — see
   `runtime-declaration`. A failed render is recoverable; conversations stuck in
   `Pending` with the reason in no one's view are not.
2. **Two runtime ServiceAccounts existed and one was granted everything.** That
   was a consequence of a release-wide MODE binding a shared account. Accounts
   are per-route now, the floor is bound to nothing, and no mode exists to widen
   anything.

What stays exclusively the parent's is the DEFAULTS and the FLOOR. A bundle
SHALL NOT render either: defaults differing per bundle would be one fact in as
many places as there are vendors, and a floor account a bundle could render
would make "a route naming nothing holds nothing" a claim no single file checks.

#### Scenario: A bundle-free install still executes
- **WHEN** an install enables no bundle at all and supplies the model credential
- **THEN** a runtime answering to the default name is rendered and conversations execute

#### Scenario: A bundle ships its vendor
- **WHEN** a bundle that ships a runtime is enabled
- **THEN** its `AgentRuntime` renders from the bundle's own values and inherits the release-wide defaults

#### Scenario: Removing the last runtime is refused, not discovered later
- **WHEN** an install disables the only bundle shipping a runtime while a Pipeline resolves to the default name
- **THEN** the render fails naming what is missing

#### Scenario: No bundle renders the defaults or the floor
- **WHEN** any combination of bundles is enabled
- **THEN** the runtime defaults and the floor ServiceAccount come from the parent alone

### Requirement: The floor holds nothing, and there is no preset posture
The account a `Pipeline` naming no `serviceAccountName` runs as SHALL hold no
Kubernetes permissions, and the chart SHALL refuse to bind anything to it.
SILENCE SHALL MEAN NO POWER, with no setting able to change it.

It shipped inverted once: a release-wide mode bound its posture to the account
every unnamed route inherited, and three of four routes in the reference install
held pod-delete and node-patch because nobody typed a field — two of them routes
that reach no Kubernetes API at all.

**THERE SHALL BE NO PRESET POSTURE.** An install wanting more than nothing SHALL
declare an account and name it on the routes that need it, or use one a bundle
renders for its own routes. A named posture is a grant nobody reviewed; a
declared account is one somebody wrote down.

**THE DEFAULT ACCOUNT IS A REFERENCE, NOT A CREATION.** Where an install names
the account a Pipeline inherits, the chart SHALL reference it and SHALL NOT
create it — the posture adapters already have, where naming is not creating.

The chart SHALL always render its own floor account regardless. That is what
keeps it NAMEABLE as a way to restrict one route to nothing, on an install whose
inherited default is an account of the operator's that carries rights.

#### Scenario: A route that names nothing can do nothing
- **WHEN** a Pipeline declares no `serviceAccountName`
- **THEN** its conversations run under an account denied every verb on every resource, whatever else the install configures

#### Scenario: Naming the default does not create it
- **WHEN** an install points the inherited default at an account it already owns
- **THEN** the chart references that account and creates only its own floor

#### Scenario: The floor stays available to restrict a route
- **WHEN** the inherited default is an account carrying rights
- **THEN** a Pipeline may name the chart's floor account and hold nothing instead

#### Scenario: No preset can widen an upgrade
- **WHEN** an existing install upgrades without declaring an account
- **THEN** nothing it runs gains a permission it did not already have

#### Scenario: A second trust level needs no second runtime
- **WHEN** an install wants an observing route and an acting route on one runtime image
- **THEN** it declares a second ServiceAccount with its own RBAC and names it on the acting Pipeline, and does NOT clone the `AgentRuntime`

#### Scenario: No bundle renders the floor
- **WHEN** any bundle is enabled
- **THEN** no bundle renders the floor account, though a bundle DOES render an identity for each route it ships and may render a runtime of its own

## MODIFIED Requirements

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

**`global.agentops.runtimeDefaults.allowPodExecution` SHALL therefore exist, SHALL
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

- **WHEN** an account declared under `rbac.runtime.serviceAccounts` states `rbacMode: full`
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
- **WHEN** the release renders with a declared `full` account and every bundle enabled
- **THEN** the agent grant is a small fixed number of `ClusterRole`s and bindings, not one per namespace

### Requirement: The credential is wired, and neither volume is the runtime's
When a runtime's `credentialsSecret.token` is supplied the component SHALL create that Secret, so the credential is release-managed; when it is empty the `AgentRuntime` SHALL reference the named Secret without creating it, and the post-install notes SHALL warn that the reference is unsatisfied. The credential SHALL reach the runtime as env via `valueFrom` — the manager SHALL read no Secrets.

The rendered `AgentRuntime` SHALL declare NO volume. Persistence is wiring: the CONTEXT and WORKSPACE volumes are declared on the `Pipeline`, and the release-wide claims the parent provisions reach a conversation that binds neither through the manager's bootstrap configuration. No operator SHALL have to copy a claim name between values blocks, for either volume, and no route SHALL need a runtime of its own to keep its state somewhere else.

Where either block points at storage the chart did not create — an existing claim, or a pre-created volume the rendered claim binds to — the resolved claim name SHALL flow to every consumer of that volume by the same wiring. An operator SHALL NOT have to restate it for the runtime, the manager's bootstrap default, the reclaiming job or the mount probe. A capability that reaches one consumer and not the others is how a volume ends up half-wired, which reads as a broken feature rather than a missing value.

Context persistence SHALL be enabled by default and workspace persistence SHALL be disabled by default. The asymmetry is deliberate: losing an agent's accumulated context silently costs conversational history, whereas losing a checkout costs a re-clone.

`global.agentops.runtimeDefaults.idleTtlMinutes` SHALL carry the release's ONE number, and a `runtimes:` entry SHALL override it per runtime. It SHALL live in the DEFAULTS rather than at parent scope, because a bundle-shipped runtime can read no parent scope but `global.` — a top-level key rendered an EMPTY field and the CRD's structural default silently replaced the release's setting. The chart SHALL WRITE the resolved value into the rendered CR rather than omitting the field: `AgentRuntime.spec.idleTtlMinutes` carries a CRD default, so an omitted field is not unset — the API server stores the CRD default and the manager prefers any non-zero spec value over its own configured TTL, which makes an omitted field render a correct-looking manifest and a wrong stored object.

#### Scenario: Credential comes back with the release
- **WHEN** a runtime's `credentialsSecret.token` is set from a secret store
- **THEN** the Secret renders with the release and the runtime references it by name only

#### Scenario: Unsatisfied reference is announced, not silent
- **WHEN** a runtime renders and no token is supplied
- **THEN** the install succeeds and the notes state that runtime pods will reach `CreateContainerConfigError` until the named Secret exists, because the kubelet resolves the reference and nothing else reports it

#### Scenario: Persistence needs no second declaration
- **WHEN** context persistence is enabled
- **THEN** the release's claim reaches every conversation whose route binds none, with no runtime-side and no pipeline-side value set

#### Scenario: The runtime declares no volume at all
- **WHEN** the chart renders its `AgentRuntime`
- **THEN** that CR carries neither a context nor a workspace volume, because where state lives is the route's decision

#### Scenario: Context persists without being asked for
- **WHEN** the chart is installed with no persistence values supplied
- **THEN** the context claim is provisioned and reaches conversations as the release default

#### Scenario: Workspace is wired the same way when enabled
- **WHEN** workspace persistence is enabled
- **THEN** the release's workspace claim reaches conversations whose route binds none, with no runtime-side and no pipeline-side value set

#### Scenario: Storage the chart did not create is wired everywhere too
- **WHEN** a volume is configured against an existing claim or a pre-created volume
- **THEN** the manager's bootstrap default, the reclaiming job and the mount probe all resolve to that same claim with no further values set

#### Scenario: Idle TTL has one default
- **WHEN** a runtime states no `idleTtlMinutes` of its own
- **THEN** the rendered `AgentRuntime` carries the number from the runtime defaults, and runtime pods use it rather than the CRD's default
