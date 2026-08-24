# agent-runtime-ownership

## Purpose

Who owns the agent execution substrate in the Helm chart: the parent chart contributes the runtime, the floor identity a route naming none runs as, the LLM credential and the context volume, while bundles contribute domain — signal sources, profiles, tooling and channels — and reference what the parent provides.
## Requirements

### Requirement: A bundle renders the identities its own routes need

A bundle that ships `Pipeline`s SHALL render a ServiceAccount for a route ONLY
WHERE THAT ROUTE NEEDS MORE THAN THE FLOOR — that is, only where the bundle also
renders or binds RBAC for it. A route needing nothing beyond the floor SHALL
name no account and inherit the default; a route that must hold nothing on an
install whose inherited default carries rights SHALL name the chart's floor
account explicitly.

**AN ACCOUNT NOBODY GRANTED ANYTHING IS NOT AN IDENTITY, IT IS AN OBJECT.** This
requirement previously read "an account for EVERY route it ships", and rendered
against the reference install four of them were bound to nothing at all —
indistinguishable from the floor every unnamed route already inherits, while
adding four names to every audit of who holds what.

THE BUNDLE IS STILL THE ONLY SCOPE THAT KNOWS. `k8s-bundle` knows its acting
route deletes pods and its observing route does not. `ha-bundle` knows neither
of its routes touches the Kubernetes API — and what follows from knowing that is
that it renders NEITHER account, not that it renders two empty ones.

**The argument for rendering them regardless is real and is already answered.**
A route naming nothing inherits `runtimeDefaults.serviceAccountName`, which on
some installs is an account of the operator's that carries rights — so an empty
per-route account pins that route to nothing. The floor is NAMEABLE for exactly
this, so the guarantee costs a name rather than an object.

**THE SUBSTRATE STAYS THE PARENT'S, EXCLUSIVELY.** No bundle SHALL render an
`AgentRuntime` it does not itself ship as a vendor, a context volume or the
floor account.

A bundle that ships NO Pipeline SHALL render no account.

#### Scenario: A bundle's route carries the bundle's account
- **WHEN** a bundle renders a route for which it also renders or binds RBAC
- **THEN** it renders that route's ServiceAccount and names it on that Pipeline

#### Scenario: A bundle whose routes need no cluster access renders no grant
- **WHEN** `ha-bundle` renders its two routes
- **THEN** it renders NO ServiceAccount for either, and both inherit the floor, because neither route reaches the Kubernetes API

#### Scenario: The reference install renders no unbound runtime identity
- **WHEN** the chart is rendered with every bundle enabled
- **THEN** every ServiceAccount it renders is either bound to rules somebody wrote or is the floor itself

#### Scenario: A bundle shipping no route ships no identity
- **WHEN** `telegram-bundle` is enabled
- **THEN** it renders no ServiceAccount, because it ships no Pipeline

#### Scenario: A bundle still renders no substrate

- **WHEN** any bundle is enabled
- **THEN** it renders no model credential Secret for a vendor it does not ship, no
  context volume, and does not render the floor account

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

- **WHEN** an install grants a declared runtime account the workload verbs an
  agent fixes things with
- **THEN** no `ClusterRoleBinding` to `cluster-admin` renders, and the acting
  role is an enumerated one somebody can read

#### Scenario: An agent cannot read a Secret

- **WHEN** any runtime ServiceAccount the chart renders is checked against
  `secrets`
- **THEN** it is denied every verb, in every namespace, under every grant

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

**THE VENDOR'S IMAGE AND CREDENTIAL BELONG TO THE BUNDLE THAT SHIPS THE VENDOR,
NOT TO THE RELEASE-WIDE DEFAULTS.** `global.agentops.runtimeDefaults` SHALL hold
only what is vendor-neutral. The reference runtime's image reference and its
`credentialsSecret` — a key and an environment variable named for one vendor —
SHALL live in that runtime's own bundle values, and the parent's `values.yaml`
SHALL carry a documented section for the bundle so it is discoverable from
`helm show values`.

This is what the bundle was extracted FOR, and extracting the CR without them
left the stated problem in place: an install running another backend still
inherits one vendor's environment variable, and an install configuring that
vendor does it in the block every other vendor also reads. A subchart's values
are reachable as `.Values.<name>` whether or not the parent declares the key, so
nothing surfaced the omission.

When a runtime's `credentialsSecret.token` is supplied the component SHALL create that Secret, so the credential is release-managed; when it is empty the `AgentRuntime` SHALL reference the named Secret without creating it, and the post-install notes SHALL warn that the reference is unsatisfied. The credential SHALL reach the runtime as env via `valueFrom` — the manager SHALL read no Secrets.

The rendered `AgentRuntime` SHALL declare NO volume. Persistence is wiring: the CONTEXT and WORKSPACE volumes are declared on the `Pipeline`, and the release-wide claims the parent provisions reach a conversation that binds neither through the manager's bootstrap configuration. No operator SHALL have to copy a claim name between values blocks, for either volume, and no route SHALL need a runtime of its own to keep its state somewhere else.

Where either block points at storage the chart did not create — an existing claim, or a pre-created volume the rendered claim binds to — the resolved claim name SHALL flow to every consumer of that volume by the same wiring. An operator SHALL NOT have to restate it for the runtime, the manager's bootstrap default, the reclaiming job or the mount probe. A capability that reaches one consumer and not the others is how a volume ends up half-wired, which reads as a broken feature rather than a missing value.

Context persistence SHALL be enabled by default and workspace persistence SHALL be disabled by default. The asymmetry is deliberate: losing an agent's accumulated context silently costs conversational history, whereas losing a checkout costs a re-clone.

`global.agentops.runtimeDefaults.idleTtlMinutes` SHALL carry the release's ONE number, and a `runtimes:` entry SHALL override it per runtime. It SHALL live in the DEFAULTS rather than at parent scope, because a bundle-shipped runtime can read no parent scope but `global.` — a top-level key rendered an EMPTY field and the CRD's structural default silently replaced the release's setting. The chart SHALL WRITE the resolved value into the rendered CR rather than omitting the field: `AgentRuntime.spec.idleTtlMinutes` carries a CRD default, so an omitted field is not unset — the API server stores the CRD default and the manager prefers any non-zero spec value over its own configured TTL, which makes an omitted field render a correct-looking manifest and a wrong stored object.

#### Scenario: The shipped vendor is configured in its own section
- **WHEN** an install sets the reference runtime's image or model credential
- **THEN** it sets them under the `claude:` bundle section, exactly as every other bundle is configured, and not in the release-wide runtime defaults

#### Scenario: The release-wide defaults name no vendor
- **WHEN** the parent chart's `global.agentops.runtimeDefaults` is read
- **THEN** it contains no image reference, environment variable or Secret key belonging to one vendor

#### Scenario: The bundle's section is discoverable
- **WHEN** an operator runs `helm show values` against the parent chart
- **THEN** the `claude:` section is present and documented, rather than reachable only by reading the subchart

#### Scenario: Another backend inherits nothing of the reference runtime
- **WHEN** an install disables the `claude` bundle and declares its own runtime
- **THEN** it inherits no image, credential key or environment variable named for the vendor it is not running

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

**THERE SHALL BE NO PRESET POSTURE, AT ANY LEVEL.** An install wanting more than
nothing SHALL declare an account, STATE ITS RULES, and name it on the routes that
need it. A named posture is a grant nobody reviewed; a declared account is one
somebody wrote down.

**THIS BANS `rbacMode` TOO, AND THE FIRST VERSION OF THIS CHANGE DID NOT.**
Deleting the release-wide `global.agentops.runtime.rbacMode` removed the preset
from the account every unnamed route inherited, and left the identical mechanism
one level down on `rbac.runtime.serviceAccounts[].rbacMode` — so this requirement
described half of what shipped. `readonly` and `full` SHALL be DELETED with no
alias: a declared account states `clusterRoles`, `bindClusterRoles` or
`namespaced`, or it holds nothing.

- **A reviewer reading `rbacMode: full` sees a word, not the verbs.** That is
  the whole objection to a preset, and it does not become smaller because the
  account was declared rather than rendered by a mode.
- **On the reference install it was a SECOND cluster-write path.** The runtime
  pod mounts its ServiceAccount token and the acting route binds a shell, while
  the runtime image ships no kubectl precisely so that cluster reach goes
  THROUGH the MCP server and its toolset split. An account carrying `full` on
  the runtime identity is a way around both walls.
- **The chart's own rule helpers SHALL remain** as a named, copyable starting
  point for an install writing `clusterRoles`. A set of rules somebody pasted
  and can read is not a preset; a mode name that expands invisibly is.

**THE DEFAULT ACCOUNT IS A REFERENCE, NOT A CREATION.** Where an install names
the account a Pipeline inherits, the chart SHALL reference it and SHALL NOT
create it — the posture adapters already have, where naming is not creating.

The chart SHALL always render its own floor account regardless. That is what
keeps it NAMEABLE as a way to restrict one route to nothing, on an install whose
inherited default is an account of the operator's that carries rights.

#### Scenario: A preset posture is refused at every level
- **WHEN** an install writes `rbacMode` on a declared runtime account
- **THEN** the render FAILS naming the explicit rule keys to write instead, rather than expanding a word into verbs nobody read

#### Scenario: An account with rules is still ordinary
- **WHEN** an install declares an account stating `clusterRoles` and names it on one Pipeline
- **THEN** that account is created and bound exactly to the rules written down, and no other route is affected

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
- **THEN** no bundle renders the floor account, though a bundle DOES render an identity for a route it grants something to, and may render a runtime of its own
