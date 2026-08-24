## 1. Prove the assumption the mode's removal rests on

- [x] 1.1 On a CLEAN local install with `global.demo.enabled=true` and NOTHING else — no inherited production values — post a task that actually reads the cluster and confirm it succeeds. The claim is that demo's reads come from the MCP server's own account, not from a runtime posture. Verify: the answer contains real cluster data while the runtime pod's service account holds no ClusterRoleBinding. **If it FAILS, stop** — `k8s-bundle` must grant its route what it needs before the mode can go. **VERDICT: PASSES.** Clean local install, demo mode plus the credential and one storage access-mode line the environment's provisioner requires. A posted `kind: task` was answered with real cluster data read through `mcp__kubernetes__namespaces_list`.
- [x] 1.2 Record which account carried the read. Verify: name the ServiceAccount on the MCP server's pod and the one on the runtime pod, and state they differ. **VERDICT: they differ, and only one is bound.** The runtime pod ran as the bundle's own route account `agentops-k8s-observe` — ZERO ClusterRoleBindings, and `auth can-i list namespaces` answers `no`. The MCP server pod runs as `agentops-mcp-k8s`, which is bound and answers `yes`. The account `rbacMode` rendered (`agentops-runtime-readonly`) was bound and named by NOTHING. The read was the MCP server's.

## 2. The values shape

- [x] 2.1 Create `global.agentops.runtimeDefaults` holding a COMPLETE runtime: `image`, `contextStorage`, `idleTtlMinutes`, `resources` WRITTEN OUT (100m/256Mi requests, 1/1536Mi limits — the values `podspec.go` already applies), `nodeSelector`, `contextSync`, `egressMediation`, `credentialsSecret` shape, `serviceAccountName`, `allowPodExecution`. Verify: `runtimes: []` plus a credential renders one working runtime named `default`.
- [x] 2.2 Add the parent-level `runtimes:` list, each entry stating only what differs. Verify: a second entry naming only `name` and `image` renders a CR carrying every other default.
- [x] 2.3 DELETE `runtime:`, `global.agentops.runtime.rbacMode` and `rbac.runtime.{clusterRoles,bindClusterRoles,namespaced}`. Keep `rbac.runtime.serviceAccounts`. Verify: `grep -rn "rbacMode" chart/` returns nothing outside guards and the per-account posture field, which is what a DECLARED account states.
- [x] 2.4 `egressMediation.enabled` defaults to `true`. Verify: a default render carries the proxy and the redirect init container.

## 3. Rendering, and the one place a name is not created

- [x] 3.1 `runtime.yaml` renders a LIST from `runtimes:` merged over the defaults, one CR each. Verify: a render test with two entries produces two `AgentRuntime` objects differing only where the values differ.
- [x] 3.2 `rbac.yaml` ALWAYS renders the chart's floor account, bound to nothing, and NEVER creates the account `serviceAccountName` points at. Verify: a render naming an operator-owned default produces exactly one ServiceAccount — the floor.
- [x] 3.3 `runtime-rbac.yaml` loses the mode's account, ClusterRole and binding; keeps `rbac.runtime.serviceAccounts` and the guard refusing to bind anything to the floor. Verify: an envtest confirms a route naming nothing is denied every verb.
- [x] 3.4 `_helpers.tpl`: delete `agentops.runtimeRbacMode`; `runtimeWriteRules` keeps reading `allowPodExecution` from `.Values.global` — **do not move that read**, a subchart calls this helper and only `global` resolves there. Verify: render `k8s-bundle`'s MCP ClusterRole with `allowPodExecution` both ways and confirm the pod verbs change.

## 4. The default-runtime guard

- [x] 4.1 Add `agentops.defaultRuntimeGuard`: FAIL the render when no declared runtime answers to `default` and any Pipeline resolves to it, naming both the missing runtime and the routes. No `lookup` — it must fire under `helm template` and for a GitOps renderer.
- [x] 4.2 Verify: a render test disabling the only runtime-shipping bundle with a Pipeline naming no `runtimeRef` FAILS; the same render with every Pipeline naming its own runtime SUCCEEDS.

## 5. Bundles: rename, and one may ship a runtime

- [x] 5.1 Rename the subchart directories and `Chart.yaml` aliases: `k8s-bundle`→`kubernetes`, `ha-bundle`→`home-assistant`, `telegram-bundle`→`telegram`, `prometheus-bundle`→`prometheus`. Use `git mv` so history follows. Verify: `helm dependency build` resolves and a default render is unchanged apart from key names.
- [x] 5.2 Extract the claude runtime into a `claude` subchart, ENABLED by default, that renders its `AgentRuntime` from its own values inheriting `global.agentops.runtimeDefaults`. Verify: disabling it with no replacement trips 4.1.
- [x] 5.3 `kubernetes`: the three derived settings become stated ones — the MCP server's read-only flag, that server's RBAC width, which route ships. They may still be set together; none may be selected by a release-wide value. Verify: setting the mutating toolset does not change the server's read-only flag, and vice versa.
- [x] 5.4 Confirm no bundle renders the floor account or the runtime defaults. Verify: render every bundle on and assert exactly one floor ServiceAccount, from the parent.

## 6. Guards for every retired name

- [x] 6.1 Fail the render on `runtime.*`, `global.agentops.runtime.rbacMode`, `rbac.runtime.{clusterRoles,bindClusterRoles,namespaced}` and each `<name>-bundle` key, naming the replacement. No cluster needed. Verify: one render test per retired key.
- [x] 6.2 Add the retired-vocabulary terms for every name above, in THIS change — the rule requires the entry to land with the removal. Verify: `python3 .github/scripts/retired-vocabulary-guard.py` clean.
- [x] 6.3 Run every module and the chart. Verify: the `.github/components.sh modules` loop is clean and `helm lint chart` passes.

## 7. Live verification

- [x] 7.1 A default install executes a conversation with no runtime values set at all beyond the credential. **VERIFIED.** Clean local install, demo mode plus the credential (and the one storage access-mode line the environment's provisioner requires). One `AgentRuntime` named `default`, from the `claude` bundle. A posted `kind: task` was answered with real cluster data.
- [x] 7.2 A route naming no account is denied a cluster read it would get under the old `readonly` mode — the default really is nothing. **VERIFIED.** The account the mode used to render does not exist (`NotFound`). The floor and the bundle's route account both answer `no` to `list namespaces`, `list pods -A` and `get nodes`. The MCP server's own account answers `yes` — the read the demo performs is its, which is what task 1 established.
- [x] 7.3 An install pointing the default at its own account gets that account, and a Pipeline naming the floor is powerless on the same install. **VERIFIED on ONE install.** With the inherited default pointed at a declared `readonly` account, a route naming nothing ran its pod as that account (`can-i list namespaces` = yes), while a route naming the floor ran as the floor and is denied every verb. Both executed.
- [x] 7.4 Two runtimes, one from the parent list and one from a bundle, both execute conversations. **VERIFIED.** `default` from the `claude` bundle and `second` from the install's own `runtimes:` list, differing only where the entry differed. One posted signal fanned out to three routes across both runtimes and every conversation answered.
- [x] 7.5 Egress mediation is active on a default install, and a runtime declaring it off builds a pod with no added containers. **VERIFIED on ONE install, both halves.** Pods on the default runtime carry `egress-init` (privileged, `NET_ADMIN`, exits) plus the `egress-proxy` sidecar. A pod on a runtime declaring `egressMediation.enabled: false` carries NO init containers at all — just `worker` — and answered normally. `false` is a zero value, which is exactly what a `mergeOverwrite` would have dropped.

## 8. Documentation

**Reference docs**

- [x] 8.1 `docs/CHANGELOG.md`, newest first and FIRST of the three. Lead with the subchart renames — every operator has set one — then the deleted `rbacMode` and what replaces it, the two values blocks, egress on by default with its Pod Security cost, and the new default-runtime guard.
- [x] 8.2 `docs/concepts.md` — a runtime is one of several; permissions are opt-in; the default account is a reference and the floor stays nameable.
- [x] 8.3 `docs/k8s-bundle.md`, `docs/ha-bundle.md`, `docs/prometheus-bundle.md`, `docs/telegram-bundle.md` — renamed pages, renamed values, plus a `docs/claude.md` for the new runtime bundle. Update `docs/_data/nav.yml` in the same edit. **`nav.yml` NEEDED NO EDIT, and that is the site's rule rather than an omission:** the bundle pages are REFERENCE pages, carry no front matter and have no nav entry, so `docs/CLAUDE.md` forbids giving them one. `docs/claude.md` follows the same rule. Every inbound LINK was updated, including the historical changelog entries' link targets — their prose keeps the old key names, because an entry for a released version is a record.
- [x] 8.4 Re-run `python3 .github/scripts/docs-generate.py` — chart values changed, so every generated block and `docs/cr-reference.md` is stale and CI fails on it. Verify: the script exits clean.

**Adopter site**

- [x] 8.5 `docs/installation.md` — the two runtime blocks and the rule separating them, the account model, and the renamed bundle keys grouped by the decision they serve.
- [x] 8.6 `docs/guides/agent-runtime.md` — declaring a runtime among several, and inheriting the defaults. `docs/guides/pipeline.md` — cluster power is an account you name, and the floor is how a route is restricted.
- [x] 8.7 `docs/getting-started.md` — the demo path after the mode is gone, and the new failure a missing `default` runtime produces.

**Context rules**

- [x] 8.8 `.claude/rules/invariants.md` — rewrite the substrate invariant: the parent owns the DEFAULTS and the FLOOR, a bundle may ship a runtime, and the guard is what replaces "the parent always renders default". Keep both original failures named.
- [x] 8.9 `.claude/rules/wiring.md` — the identity chain loses the mode. `.claude/rules/chart.md` — state the rule separating the two values blocks, INCLUDING that a parent helper called from a subchart sees only `.Values.global`, which is what forces `allowPodExecution` to live there.
