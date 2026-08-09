# main-chart-owns-runtime — design

## Context

Three facts, all checkable in the tree today:

1. `charts/k8s-bundle/templates/profile.yaml` is the only template in the chart
   that renders an `AgentRuntime`. `grep AgentRuntime chart/templates/` returns
   nothing.
2. `chart/templates/rbac.yaml` already creates `serviceAccounts.runtime`
   (`agentops-runtime`) with the comment "the chart grants it NOTHING", and
   `cmd/manager/main.go` defaults `runtimepod.Config.ServiceAccount` to that
   same name. The bundle then creates `agentops-runtime-k8s` and binds
   `view` or `cluster-admin` to *that*. Two SAs, one of which is dead weight in
   any bundle install.
3. `CHANGELOG.md` (chart 2.x) records the move verbatim:
   `demo.runtimeImage` → `k8s-bundle.profile.runtime.image`,
   `demo.credentialsSecret.*` → `k8s-bundle.profile.runtime.credentialsSecret.*`,
   and — the tell — `(inherited persistence)` →
   `k8s-bundle.profile.runtime.homePvcRef`, "**set this explicitly**, subcharts
   cannot see the parent's `persistence` block".

Item 3 is the whole story. The runtime was a parent concern; it became a bundle
concern because demo mode and the bundle became the same thing, and the values
have been paying interest on that ever since.

The MCP half of this change is the same fault one layer out. `mcp.enabled`
defaults `false` with an explicit justification — an MCPConfig needs an
endpoint, and `mcpServers.enabled: false` supplies none, so a default-on
component would trip its own `fail` guard. True, but it argues for flipping both
defaults together, not for shipping a Kubernetes bundle whose Kubernetes tooling
is off. Meanwhile `mcpServers.readOnly` and `mcpServers.rbac.mode` are
independent knobs bound by an invariant the values only *state*: our own install
carries an eight-line comment explaining that `full` has to mean full on both
sides "or it means nothing".

## Goals / Non-Goals

**Goals:**

- One place that answers "how do agents execute here": image, credential, idle
  TTL, node placement, home volume, and the identity whose RBAC is the agent's
  power.
- A chart where enabling `vm-bundle` or `telegram-bundle` alone produces a
  working install.
- Bundles that contribute *domain* — sources, profiles, tooling, channels — and
  nothing about the substrate.
- One knob for the agent's in-cluster power, with the MCP server's posture
  derived from it rather than restated.
- The k8s bundle's Kubernetes tooling on by default, because a Kubernetes bundle
  whose MCP access is off is an install that looks complete and cannot see the
  cluster through the path the project prefers.

**Non-Goals:**

- Not changing the `AgentRuntime` CRD, the manager's runtime-pod lifecycle, or
  how a profile resolves its runtime (`runtimeRef`, else `default`).
- Not removing `profile.runtimeRef` from the bundle — pointing one profile at a
  higher-trust or different-vendor runtime is exactly what it is for.
- Not shipping more than one runtime from the parent. `runtime:` is singular and
  renders `default`; additional runtimes stay hand-written CRs, which is what
  the vendor × trust-level model asks for.
- Not touching `Bash`/kubectl (see `runtime-drop-kubectl`) or the toolset split.
- Not fusing the two MCP identities. Derivation is a default, not a merge.

## Decisions

### D1: The runtime moves up whole, not partially

Half-measures were considered and rejected: keeping `profile.runtime` in the
bundle with a `create: false` default, or leaving it "for demo mode". Both keep
two spellings of the same object alive, keep the `homePvcRef` copy, and keep the
bundle able to render a second runtime SA. The bundle loses `profile.runtime.*`
and `rbac.*` entirely; what it keeps is identity plus `runtimeRef`.

Demo mode loses nothing, because `runtime.enabled` defaults to `true`: turning
the bundle on still yields a working agent in one flag, and now so does turning
on no bundle at all.

### D2: The runtime SA and its RBAC mode live under `global.`

`k8s-bundle`'s `mcpServers` needs the effective RBAC mode to derive its own
posture, and a subchart can read exactly one parent scope. The chart already
relies on this for `global.demo.enabled`, with the reason stated in
`_helpers.tpl`: "hence the demo flag living under `global.` (the only scope a
subchart can read)".

So the canonical keys are:

```yaml
global:
  agentops:
    runtime:
      serviceAccountName: agentops-runtime   # its RBAC IS the agent's power
      rbacMode: ""                           # "" | none | readonly | full
```

The parent's `runtime:` block and `rbac.runtime` rendering read them; so does
the bundle's MCP server component. One value, two readers, no consistency check
needed.

Rejected: duplicating `serviceAccountName`/`mode` into the bundle's values with
a render-time `fail` when they disagree. It works, but it makes an operator
maintain agreement between two keys that describe one fact — the exact problem
this change is removing from `environments/default.yaml` downstream.

### D3: `rbacMode: ""` means none, except under demo

The parent chart's promise today is "the chart grants NOTHING unless
configured". Defaulting the new knob to `readonly` would silently bind cluster
`view` to the runtime SA of every existing install on upgrade — a real widening,
for a chart whose stated posture is least privilege.

Defaulting it to `none` would instead break the demo promise the k8s-bundle spec
pins: `global.demo.enabled=true` and nothing else must produce a read-only
working agent.

So `""` resolves to `readonly` when `global.demo.enabled` is true and to `none`
otherwise. One conditional, attached to the flag that is already the chart's
"give me something that works" switch. `none`, `readonly` and `full` are
explicit and never inferred; `full` remains something no default path selects.

### D4: `home.pvcRef` is wired, not copied

When `persistence.enabled` (or `persistence.existingClaim`) is set, the rendered
`AgentRuntime` gets `home.pvcRef` from the parent's own `persistence` block.
`runtime.homePvcRef` exists only to point at a claim the chart did not create.
The subchart-visibility workaround it replaces was never a design, only a
consequence of D1's fault.

### D5: Idle TTL stops being configured twice

The parent's `runtimeIdleTtlMinutes` is the manager's default for every runtime
pod; `AgentRuntime.spec.idleTtlMinutes` overrides it per runtime. The new
`runtime.idleTtlMinutes` therefore defaults to **empty** and is omitted from the
rendered CR, so the manager default applies and there is one number unless an
operator deliberately wants two. This also quietly fixes a live inconsistency:
the bundle's default was `10` while the manager's is `1`.

### D6: MCP defaults on, and the server follows the RBAC mode

`mcp.enabled: true` and `mcpServers.enabled: true` flip together — the guard at
`mcp.yaml:23` stays exactly as it is, and keeps failing loudly for the one
combination that is genuinely broken (`mcp.enabled` with no server and no
`url`).

`mcpServers.readOnly` and `mcpServers.rbac.mode` become `null` and derive:

| `global.agentops.runtime.rbacMode` | `mcpServers.readOnly` | `mcpServers.rbac.mode` |
|---|---|---|
| `full` | `false` | `full` |
| `readonly`, `none`, `""` | `true` | `readonly` |

This is the `null`-follows-context pattern the bundle already uses for
`mcp.toolsets.admin.enabled` ("null = follow the deployed server"), and it makes
`rbacMode: full` render the `k8s-admin` toolset as a consequence rather than a
fourth thing to remember, since `mcpAdminEnabled` already follows `!readOnly`.

The MCP server's SA never derives `full` from anything but an explicit `full`;
`none` deliberately maps to a **readonly** server SA rather than to nothing,
because that is the useful shape the two-identity design exists to offer — an
agent that can read the cluster through MCP and do nothing at all through its
own identity.

### D7: What derivation costs, stated plainly

Today the two identities are independently reviewable *by default*. After this,
widening the agent's kubectl RBAC to `full` also widens the MCP server unless
the operator says otherwise. That is a real reduction in default separation, and
it is accepted for two reasons: the safe direction (both read-only) is what
every default path produces, and the separation stays *reachable* — explicit
`mcpServers.readOnly: true` under `rbacMode: full` still gives "kubectl writes,
MCP reads only", and the toolset split still requires a Pipeline to bind
`k8s-admin` deliberately before any mutation is callable.

The alternative — keeping them independent — preserves nothing in practice,
because the combination it protects (`rbac.mode: full` with a read-only MCP
server) is the one our own values call out as meaningless.

## Risks / Trade-offs

- [The runtime SA rename orphans working RBAC] → `agentops-runtime-k8s`
  bindings are replaced by `agentops-runtime` ones in the same upgrade; the
  migration note names the old objects so a leftover ClusterRoleBinding can be
  deleted. Anyone who bound their own roles to the old SA must re-point them,
  and the note says so rather than hoping nobody did.
- [An install that never wanted an MCP server now gets one] → Upgrade-visible
  and listed first in the migration table; `mcpServers.enabled: false` is a
  one-line hold, and with `mcp.url` unset the config component turns off with
  it.
- [Default separation of the two MCP identities weakens] → D7. Overridable, and
  the toolset wall is untouched.
- [`runtime.enabled: true` renders an `AgentRuntime` for installs that manage
  their own] → Name collision only if theirs is also called `default`, in which
  case Helm adopts or conflicts loudly rather than silently changing behavior.
  `runtime.enabled: false` is the documented hold, and the migration table
  leads with it.
- [`global.` keys are unusual UX for user-facing values] → True; justified by
  D2's single-scope constraint and precedented by `global.demo.enabled`. The
  README documents them next to the `runtime:` block, not in a footnote.
- [Two changes in one proposal] → The MCP defaults are not independent: their
  derivation source is the RBAC mode this change relocates. Splitting them would
  mean shipping the same values twice.

## Migration Plan

1. Land the parent `runtime:` component and `global.agentops.runtime.*` while
   the bundle still renders its own — both paths coexist for one commit, so the
   templates can be diffed against a real install.
2. Remove `profile.runtime.*`, `rbac.*`, `templates/rbac.yaml` and the SA from
   `k8s-bundle`; re-point `NOTES.txt`.
3. Flip the four MCP defaults and implement the derivation.
4. Chart 4.0.0, migration table, CHANGELOG.
5. Update `_gitops/apps/agent-ops` in the same session and
   `helmfile apply` — that install exercises `rbacMode: full`, the derived
   write-capable server, the console, and the release-managed credential, which
   is every branch this change adds.

Values migration table for the release notes:

| 3.x | 4.0 |
|---|---|
| `k8s-bundle.profile.runtime.image` | `runtime.image` |
| `k8s-bundle.profile.runtime.credentialsSecret.*` | `runtime.credentialsSecret.*` |
| `k8s-bundle.profile.runtime.nodeSelector` | `runtime.nodeSelector` |
| `k8s-bundle.profile.runtime.resources` | `runtime.resources` |
| `k8s-bundle.profile.runtime.idleTtlMinutes` | `runtime.idleTtlMinutes` (empty ⇒ `runtimeIdleTtlMinutes`) |
| `k8s-bundle.profile.runtime.homePvcRef` | *(automatic from `persistence`)* |
| `k8s-bundle.profile.runtime.name` | `runtime.name` |
| `k8s-bundle.profile.runtime.create` | `runtime.enabled` |
| `k8s-bundle.profile.runtime.serviceAccountName` | `global.agentops.runtime.serviceAccountName` |
| `k8s-bundle.rbac.mode` | `global.agentops.runtime.rbacMode` |
| `k8s-bundle.rbac.enabled: false` | `global.agentops.runtime.rbacMode: none` |
| `k8s-bundle.mcpServers.readOnly` | *(derived; still settable)* |
| `k8s-bundle.mcpServers.rbac.mode` | *(derived; still settable)* |

Rollback = chart 3.4.0. The CRDs are untouched, so a downgrade re-renders the
bundle-owned objects; the only manual step is deleting the parent-owned
`AgentRuntime` if its name differs from the bundle's.

## Open Questions

- Should `runtime:` grow a `credentialsSecret.existingSecret` alias, or is
  "leave `token` empty and the reference is yours to satisfy" clear enough? The
  current wording has already produced one `CreateContainerConfigError` in the
  wild.
- Whether `vm-bundle` should also stop assuming an agent exists elsewhere, or
  whether "bundles contribute domain, the parent contributes substrate" is
  enough of a stated rule to leave it implicit.
