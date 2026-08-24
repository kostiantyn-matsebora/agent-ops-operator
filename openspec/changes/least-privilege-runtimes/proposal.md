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
