## Context

`chart/charts/kubernetes/templates/mcp-server.yaml` renders the
`agentops-mcp-k8s` ClusterRole from two shared helpers,
`agentops.runtimeReadRules` and `agentops.runtimeWriteRules`
(`chart/templates/_helpers.tpl`). The branch chosen depends on
`mcpServers.rbac.mode` (`readonly`/`full`, defaulted from
`kubernetes.allowMutations`).

Those helpers are shared with every declared acting ServiceAccount
(`chart/templates/runtime-rbac.yaml`). They are deliberately fixed. Four
rules they must keep:

1. No verb on `secrets`.
2. No `*` in `resources` or `apiGroups`.
3. No `escalate`/`bind` on `rbac.authorization.k8s.io`.
4. No write on RBAC/CRDs, and no read of `clusterroles`.

See `proposal.md` for why a hand-applied `Role`/`RoleBinding` is the wrong
fix.

## Goals / Non-Goals

**Goals:**
- Let an install grant `agentops-mcp-k8s` read access to resources the fixed
  rule set doesn't cover (third-party CRDs first), through values,
  additively, default off.
- Keep the same four guard rails the fixed rule set already enforces, so the
  extension can't reopen what those rules exist to close.

**Non-Goals:**
- No write-side extension. The mutating toolset's RBAC stays exactly the
  fixed `runtimeWriteRules` set. This change only touches the read side.
- No change to `agentops.runtimeReadRules`/`runtimeWriteRules` themselves,
  and no reach into `chart/templates/runtime-rbac.yaml`'s declared accounts.
  The extension is scoped to this one bundle's MCP server ServiceAccount.
- No kubectl-style allowlist of individually named operations. The
  extension is plain RBAC rule shapes (`apiGroups`/`resources`/`verbs`), same
  as the fixed lists.

## Decisions

**New value: `mcpServers.rbac.extraReadRules`.** A list of
`{apiGroups, resources, verbs}` entries, default `[]`.

A new helper, `kubernetes.mcpServerExtraReadRules`, renders the list right
after `agentops.runtimeReadRules`. It renders in BOTH the `readonly` and
`full` branches of the ClusterRole, so the two walls keep moving together.
An extension granted under `readonly` is the same rule set it would be under
`full`.

Why both branches, not one shared block: the existing template already
renders `runtimeReadRules` twice — once alone under `readonly`, once
alongside `runtimeWriteRules` under `full` — rather than factoring a shared
read block out. Matching that shape keeps the diff minimal. Each branch
stays independently readable.

**Verb allowlist, enforced by `fail`.** Every entry's `verbs` must be a
subset of `["get", "list", "watch"]`. This has to be mechanical, not
documented. An extension that could carry `create`/`patch`/`delete` would be
a second, unreviewed way to grant mutation — parallel to the existing
`mcpServers.readOnly`/toolset split.

**Resource denylist, enforced by `fail`.** Reject an entry matching any of:

| Rejected shape | Mirrors |
|---|---|
| `apiGroups: [""]` with `resources` containing `secrets` | guard rail 1 |
| any `apiGroups` or `resources` entry equal to `"*"` | guard rail 2 |
| `apiGroups: ["rbac.authorization.k8s.io"]`, entirely | guard rails 3 and 4 |
| `apiGroups: ["agentops.dev"]` | `invariants.md` — never granted to anything |

The check runs per entry at render time and names the offending entry in the
failure message. Same pattern `agentops.defaultRuntimeGuard` and the
existing `rbacMode` guard already use in this chart.

**No resource-existence validation.** Helm cannot know whether
`vmrules.operator.victoriametrics.com` is actually installed in the target
cluster.

A rule naming a CRD that isn't there renders a ClusterRole granting nothing
reachable. That failure shape already exists for any other RBAC rule
naming a CRD that hasn't landed yet, so it is not new here.

## Risks / Trade-offs

**An operator could still broaden the server's reach with a rule naming a
sensitive resource outside the three denied groups** — some other
operator's CRD exposing values the install didn't intend to share.

→ Mitigation: the extension is opt-in, per-release, and reviewable in the
same diff as every other values change.

That is this repo's chosen trade-off everywhere else RBAC is configurable
(`rbac.runtime.serviceAccounts`, `pipelines.<route>.rbac.clusterRoles`).

The denylist stops the specific holes this repo has already named as
catastrophic. It does not stop every possible overgrant.

**A future write-side extension could duplicate this denylist logic.**

→ Mitigation: factor the guard into its own named helper
(`kubernetes.mcpServerRuleGuard` or similar) now, even though only the read
side calls it today.

A write-side extension, if ever proposed, then reuses the check instead of
re-deriving it.

## Migration Plan

Purely additive, default `[]`. A release that sets nothing renders exactly
the ClusterRole it renders today.

An install wanting the VictoriaMetrics `vmrules` read adds one entry to its
own values and runs its normal `helmfile sync`/`helm upgrade`. No new
install-time-only decision, so it belongs on `configuration.md`, not
`installation.md`.

Rollback is removing the values entry and upgrading again. Helm's 3-way
merge drops the added rule from the ClusterRole like any other diff.
