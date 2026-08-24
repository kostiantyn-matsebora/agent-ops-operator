## Why

**`rbacMode` is a setting nobody can explain in one sentence, and it grants
nothing.** It renders an extra, named ServiceAccount carrying a preset posture;
that account does nothing until a `Pipeline` names it. The name says "the mode
the runtime's RBAC is in", which is what it USED to mean and is the reading that
caused the incident it was reverted for. Every reader arrives at the wrong model
and has to be corrected.

**Its one load-bearing use does not survive scrutiny.** Empty resolves to
`readonly` under demo mode, which is what makes demo a one-flag working install.
But an agent reaches the cluster through the MCP SERVER, which carries its own
account and its own grant — the runtime image ships no kubectl. Demo's read
access is `k8s-bundle`'s to provide, and that bundle already renders one
identity per route it ships.

**`runtime` names three different values blocks**, and no page states the rule
that separates them:

| Block | Actually means |
|---|---|
| `runtime:` | the engine — parent-only, invisible to every subchart |
| `global.agentops.runtime:` | whatever a SHARED HELPER or a subchart must resolve |
| `rbac.runtime:` | how accounts get bound |

The middle one is not a style choice. `agentops.runtimeWriteRules` is a
parent-defined helper that `k8s-bundle` calls to build its MCP server's RBAC,
and inside a template invoked from a subchart only `.Values.global` resolves.
That is the whole reason, and it is written down nowhere an operator reads.

**And `runtime:` renders exactly ONE runtime.** A second vendor is a
hand-written CR. That is the wrong shape for a project whose model is "one
`AgentRuntime` per VENDOR".

## What Changes

- **BREAKING** — **`global.agentops.runtime.rbacMode` is DELETED**, along with
  `rbac.runtime.clusterRoles`, `.bindClusterRoles` and `.namespaced`, which
  attach to the account that mode renders and have no other target.

  **THE DEFAULT BECOMES NO PERMISSIONS, FULL STOP.** The chart always creates
  the floor account and binds it to nothing. More rights means an account you
  declare, or one a bundle ships for its own routes.

  - **`k8s-bundle`'s three derivations become primary controls.**
    `mcpServers.readOnly`, that server's RBAC and which route renders were
    consequences of a mode; they become settings stated where they apply.

- **BREAKING** — **`serviceAccountName` names the DEFAULT account and is a
  REFERENCE.** The chart no longer creates the account it points at, which is
  the same posture adapters already have: naming is not creating.

  Two accounts can then exist, and the second is the useful half:

  | Account | Created by | Is |
  |---|---|---|
  | `agentops-runtime` | always, the chart | bound to nothing |
  | whatever `serviceAccountName` names | **you** | the default a Pipeline inherits |

  So an install that points the default at its own account keeps
  `agentops-runtime` available to RESTRICT one route back to nothing.

- **BREAKING** — **three runtime blocks become two.**

  | Now | Holds |
  |---|---|
  | `global.agentops.runtimeDefaults` | a COMPLETE, WORKING runtime configuration |
  | `runtimes:` | the runtimes an INSTALL declares, stating only what differs |

  **"Defaults" means sufficient**, not "the fields left over after the list took
  the interesting ones". `runtimes: []` must yield one working runtime named
  `default`. The only value that cannot be defaulted is the credential.

  - **It must live under `global.`**, for two reasons and not for tidiness: the
    shared write-rules helper and three bundles resolve `serviceAccountName` and
    `allowPodExecution` from subchart context, and a bundle-shipped runtime has
    no other scope to inherit from.
  - **`resources` is written out**, not `{}`. The numbers exist today, hardcoded
    in `podspec.go`, where no operator can see or tune them.

- **BREAKING** — **a bundle MAY ship a runtime**, declaring it in its own values
  and rendering its own CR, exactly as bundles already ship pipelines. This
  REVERSES `invariants.md` — see below for why the original failure no longer
  applies.

- **BREAKING** — **the claude runtime becomes its own bundle**, on by default
  and disableable, so an install using another vendor stops carrying it.

- **BREAKING** — **`egressMediation` defaults to ON.** The wall for an
  uncooperative agent should not be opt-in. It costs a privileged `NET_ADMIN`
  init container, which a namespace under `restricted` Pod Security admission
  refuses — so the NOTES must name that failure.

- **BREAKING** — **every subchart is renamed for the system it integrates**, and
  the `-bundle` suffix is dropped: `kubernetes`, `home-assistant`, `telegram`,
  `prometheus`, plus `claude` and `ollama`. `k8s` alone is not descriptive;
  `kubernetes` is, and it does not collide with the `k8s-ops` and `ha-ops`
  PIPELINE names an install declares.

- **A NEW GUARD REPLACES A DELETED INVARIANT.** "The parent always renders
  `default`" cannot hold once the runtime ships in a disableable bundle. In its
  place: **the render FAILS when no runtime resolves `default` and a Pipeline
  names none.** It needs no cluster, so it protects a GitOps install too.

- **Retired values keys FAIL the render**, naming where each went, and the
  retired-vocabulary guard gains an entry per name in this same change.

## Capabilities

### New Capabilities

- `runtime-declaration`: how runtimes are DECLARED — the defaults every runtime
  inherits, the install's list, a bundle's own, the `default` name a Pipeline
  with no `runtimeRef` resolves, and the guard that one must exist.

### Modified Capabilities

- `agent-runtime-ownership`: the parent no longer owns the runtime exclusively.
  A bundle may ship one; what stays the parent's is the DEFAULTS every runtime
  inherits and the floor account. The two failures that made this an invariant
  are answered rather than ignored.
- `pipeline-model`: `serviceAccountName` resolves to a default that is a
  reference the chart does not create, and the floor account stays nameable so a
  route can be restricted to nothing.
- `k8s-bundle`: renamed to `kubernetes`; its MCP server's read-only posture,
  RBAC width and which route it ships stop being derived from a mode and become
  stated settings; it keeps rendering one identity per route.
- `ha-bundle`: renamed to `home-assistant`.
- `prometheus-bundle`: renamed to `prometheus`.
- `k8s-mcp-tooling`: the `k8s-admin` toolset renders on its own setting rather
  than as a consequence of a release-wide mode.
- `runtime-egress-mediation`: enabled by default, with the admission cost named.

## Impact

**Chart**

- `chart/values.yaml` — `runtime:` and `rbac.runtime` collapse into
  `global.agentops.runtimeDefaults` plus `runtimes:`; `rbacMode` deleted;
  `resources` written out; `egressMediation.enabled` flips to `true`.
- `chart/templates/runtime.yaml` — renders a LIST, not one CR.
- `chart/templates/runtime-rbac.yaml` — loses the mode's account, role and
  binding; keeps the floor account and the operator's own declarations.
- `chart/templates/rbac.yaml` — the floor is created, the named default is not.
- `chart/templates/_helpers.tpl` — `agentops.runtimeRbacMode` deleted;
  `runtimeWriteRules` keeps reading `allowPodExecution` from `global`; new guard
  for the missing `default` runtime; guards for every retired key.
- `chart/charts/` — four directories renamed, plus a new `claude` and the
  `Chart.yaml` dependency aliases that name them.
- `chart/charts/kubernetes/` — the three derived settings become primary.

**Manager**

- Nothing required. `podspec.go`'s hardcoded resources stay as the fallback for
  an install with no `AgentRuntime` at all.

**Reference docs**

- `docs/CHANGELOG.md` — written FIRST. Every subchart key an operator has set is
  renamed, which is the widest values break this chart has shipped.
- `docs/concepts.md` — the runtime is declared, not singular; permissions are
  opt-in.
- `docs/installation.md` — the two blocks and the rule separating them.
- `docs/k8s-bundle.md`, `docs/ha-bundle.md`, `docs/prometheus-bundle.md`,
  `docs/telegram-bundle.md` — renamed pages plus their values.
- `docs/cr-reference.md` and every guide `yaml` block — GENERATED, re-run
  `.github/scripts/docs-generate.py`.

**Adopter site**

- `docs/getting-started.md`, `docs/guides/agent-runtime.md`,
  `docs/guides/pipeline.md` — a runtime is one of several, and cluster power is
  something a route opts into by name.

**Context rules**

- `.claude/rules/invariants.md` — the substrate invariant is rewritten, not
  deleted: what stays the parent's is the DEFAULTS and the floor.
- `.claude/rules/wiring.md` — the identity chain loses the mode.
- `.claude/rules/chart.md` — the rule separating the two values blocks, stated
  where a chart edit will meet it.
- `.github/retired-vocabulary.json` — `rbacMode`, `rbac.runtime.*`, `runtime.*`
  and every `-bundle` subchart key.

---

## REOPENED — what the first pass got wrong

**This change was archived and reopened.** Three things it set out to do were
left half-done, and each is the same failure: something moved out of the parent
without its consequences.

### 1. A preset posture survived one level down

The change deleted `global.agentops.runtime.rbacMode` and wrote **"THERE SHALL
BE NO PRESET POSTURE"** into the spec — then shipped
`rbac.runtime.serviceAccounts[].rbacMode: readonly|full`. The identical
mechanism, one level down, so the requirement described half of what was built.

**On the reference install it is a second cluster-write path.** The runtime pod
mounts its ServiceAccount token and the acting route binds a shell, while the
runtime image ships no kubectl precisely so cluster reach goes through the MCP
server and its toolset split. `full` on the runtime identity walks around both
walls with `curl`.

**Fix:** `rbacMode` DELETED with no alias. A declared account states
`clusterRoles` / `bindClusterRoles` / `namespaced`, or holds nothing.

### 2. Bundles render accounts for routes that need none

The rule kept was "an account for EVERY route a bundle ships". Rendered against
the reference install, four accounts are bound to nothing at all —
`agentops-ha-ops`, `agentops-ha-control`, `agentops-mcp-ha`,
`agentops-mcp-prometheus` — indistinguishable from the floor, while adding four
names to every audit of who holds what.

**Fix:** a bundle renders an account only where it also renders or binds RBAC
for that route. A route needing nothing names nothing and inherits the floor; a
route that must hold nothing on an install whose inherited default carries
rights names the floor explicitly, which the spec already keeps nameable.

**The two lanes stay distinct:** a BUNDLE-shipped route needing privileges gets
the account the bundle creates and binds. An INSTALL'S OWN pipeline declares its
own account and names it — that lane is unchanged.

### 3. The vendor's bundle does not carry the vendor's configuration

`chart/charts/claude/values.yaml` states the reason it exists: *"an install
using one still carried this one's image reference and credential shape."* Both
are still in `global.agentops.runtimeDefaults` — the image tag, and a
`credentialsSecret` keyed `oauthToken` into `CLAUDE_CODE_OAUTH_TOKEN`.

So an install running another backend inherits Claude's environment variable,
and an install configuring Claude does it in the vendor-neutral block. The
parent's values carry no `claude:` key either, so `helm show values` surfaces
the bundle's section nowhere — a subchart's values are reachable as
`.Values.claude` whether or not the parent declares them, which is what hid it.

**Fix:** image and credential move to `chart/charts/claude/values.yaml`, and the
parent gains a documented `claude:` section.

## Impact — the reopened work

**Chart**

- `chart/templates/runtime-rbac.yaml` — `rbacMode` deleted; a retired-key guard
  FAILS the render naming the explicit keys.
- `chart/charts/kubernetes/templates/pipeline-identity.yaml` and the
  `home-assistant` equivalent — render only where a grant is declared.
- `chart/values.yaml` — `runtimeDefaults.image` and `.credentialsSecret` move
  out; a `claude:` section arrives.
- `chart/charts/claude/values.yaml` — gains `image` and `credentialsSecret`.

**Documentation**

- `docs/CHANGELOG.md` — BREAKING three times: a values key deleted, the
  credential moved, and accounts disappearing from installs that did not name
  them.
- `docs/installation.md`, `docs/concepts.md`, `docs/claude.md`,
  `docs/k8s-bundle.md`, `docs/ha-bundle.md`.
- `.claude/rules/invariants.md`, `wiring.md`, `chart.md`.
- `.github/retired-vocabulary.json` — `rbacMode`.

### 4. The manager creates ServiceAccounts, and it is the only thing that does

`.claude/rules/wiring.md:193` says **"NAMING AN SA IS NOT CREATING ONE. No
reconciler makes one."** That sentence is false in exactly one file:
`platform/manager/internal/controller/adapterworkload.go:134` creates one per
adapter workload, unconditionally.

### The creator does not know what the adapter needs

The manager is forbidden from binding RBAC to an adapter — correctly, because a
`SignalAdapter` is an ordinary namespaced object, and a reconciler that could
attach permissions to one would make CR-edit rights a privilege escalation. So
it creates an account it is not allowed to grant anything to.

The result on the reference install: six adapter accounts, of which ONE is bound
to anything. The other five are indistinguishable from an account denied
everything, which is what an unnamed pod could have had for free.

### The identity and the grant are in different files

The `kubernetes` bundle writes the events ClusterRole and binds it to
`agentops-signal-k8s-events` — a name it does not create and cannot see. Reading
"what can this adapter do" means reading a chart template and a Go reconciler
and knowing that the name in one is the object in the other.

### A guard cannot see an object that does not exist at render time

`.github/scripts/serviceaccount-guard.py` fails any ServiceAccount the chart
renders that nothing grants and nothing runs as. It cannot see these, because
they are created by a controller after the render.

### `kubernetesAccess` is the same decision wearing a second name

It mounts the token and injects `POD_NAMESPACE`, and nothing else. Naming an
account whose token is never mounted grants nothing — the pod never presents that
identity — and mounting a token without naming an account mounts `default`'s.
Two fields, one decision, and the combinations that are not the decision are all
meaningless.

### And an account nothing authenticates as is a name

Two MCP server accounts mount no token (`automountServiceAccountToken: false`)
and are bound to nothing. With no token the pod never presents that identity, so
the account changes nothing about what it can reach — it is the placeholder this
change deleted from the bundles, in a different file.

**The rule, one line, covering the chart and the manager alike:** an account is
justified when something is BOUND to it or something AUTHENTICATES as it.
Otherwise it is a name.

## Impact — the adapter identity work

**API — BREAKING**

- `platform/manager/api/v1alpha1/` — `serviceAccountName` added to both adapter
  kinds, `kubernetesAccess` removed. CRDs regenerate, so
  `kubectl apply -f chart/crds/` is an upgrade AND an install step.

**Manager**

- `internal/controller/adapterworkload.go` — stops creating the account, names
  the resolved one, mounts its token, injects `POD_NAMESPACE` unconditionally.

**Chart**

- Each bundle renders the account it grants, beside the grant, and names it on
  the CR. Adapters granted nothing name nothing and run as the floor.
- The two tokenless MCP server accounts are removed.
- `.github/scripts/serviceaccount-guard.py` — the workload exemption requires the
  pod to MOUNT the token.

