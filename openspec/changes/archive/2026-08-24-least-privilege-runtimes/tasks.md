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

## 9. REOPENED — the four things the first pass left half-done

Archived, then reopened. Each item is a consequence that did not travel with the
thing that moved. See `proposal.md` — REOPENED.

### 9.1 The preset posture that survived one level down

- [x] 9.1.1 Delete `rbacMode` from `chart/templates/runtime-rbac.yaml`. A declared
  account renders exactly what `clusterRoles` / `bindClusterRoles` / `namespaced`
  state, and an account stating none is created bound to nothing
- [x] 9.1.2 Add a retired-key guard FAILING the render, naming the explicit keys.
  Helm never reports an unread value, so a silently-ignored `full` would quietly
  REMOVE a grant the install thought it had
- [x] 9.1.3 Keep `agentops.runtimeReadRules` / `runtimeWriteRules` and document
  them as a copyable starting point. A rule set somebody pasted and can read is
  not a preset; a mode name that expands invisibly is
- [x] 9.1.4 `.github/retired-vocabulary.json` — `rbacMode`, in THIS change.
  **THE TERM ALREADY EXISTED AND DID NOT CATCH THIS**, which is the finding: its
  `allow` list carried `serviceAccounts`, `per account` and `declared account`,
  so a sentence teaching the per-account `rbacMode` read as a RECORD of the
  release-wide one being removed. Those three are removed
- [x] 9.1.5 Chart render tests: explicit rules render; an account with none is
  bound to nothing; `rbacMode` fails the render

### 9.2 Bundle accounts only where the route is granted something

- [x] 9.2.1 `chart/charts/kubernetes/templates/pipeline-identity.yaml` — render a
  ServiceAccount only where that route declares a grant. A route with none names
  nothing and inherits the floor
- [x] 9.2.2 The `home-assistant` equivalent — and `prometheus`, which had the
  identical template and was not in the original task list
- [x] 9.2.3 The MCP server accounts: `agentops-mcp-ha` and
  `agentops-mcp-prometheus` were bound to nothing AND mounted no token, so the
  identity was never presented to the API server. Both are gone.
  `agentops-mcp-k8s` stays — it mounts its token and carries the grant
- [x] 9.2.4 A route that must hold nothing on an install whose inherited default
  carries rights names the FLOOR explicitly. Verified that path still works
- [x] 9.2.5 Pinned: `.github/scripts/serviceaccount-guard.py`, run over EVERY
  permutation in the `chart` CI job. Three explanations, not two — bound,
  authenticating, or the floor
- [x] 9.2.6 The install's OWN pipelines lane is untouched: a custom route declares
  an account and names it. Asserted, or removing the bundle lane looks like
  removing both

### 9.3 The vendor's configuration follows the vendor's bundle

- [x] 9.3.1 Move `image` and `credentialsSecret` to `chart/charts/claude/values.yaml`
- [x] 9.3.2 Add a documented `claude:` section to the parent `chart/values.yaml`,
  so the bundle is discoverable from `helm show values`
- [x] 9.3.3 Verify what remains in `runtimeDefaults` is vendor-NEUTRAL.
  **VERIFIED** — `allowPodExecution`, `contextStorage`, `contextSync`,
  `egressMediation`, `idleTtlMinutes`, `nodeSelector`, `resources`,
  `serviceAccountName`.
  - **`contextSync.paths` IS THE NEXT ONE**, neutral today only because it ships
    empty. `.claude/projects/-data-workspace/**` describes where claude-code
    files transcripts, so it belongs in the `claude:` bundle. Whatever change
    turns synchronisation on by default owes that placement
- [x] 9.3.4 Retired-key guard on the moved keys. Not silently ignored — WORSE:
  left in the defaults they still merge into EVERY runtime, so another backend
  inherits `CLAUDE_CODE_OAUTH_TOKEN`
- [x] 9.3.5 A bundle-free install (`claude.enabled: false`) plus an install's own
  `runtimes:` entry still renders and executes, inheriting nothing vendor-shaped

### 9.4 Verify against a real install

- [x] 9.4.1 **VERIFIED ON A LIVE INSTALL: 6, and the verdict is clean.** Five
  carry a binding to rules somebody wrote; the sixth is the floor, bound to
  nothing. Fourteen before
- [x] 9.4.2 Pods run under the expected identities — a route naming none on the
  floor, the granted adapters on their own accounts
- [x] 9.4.3 The deleted `rbacMode` and the moved credential keys each FAIL the
  render rather than being ignored

### 9.5 The retired-vocabulary guard does not read the chart

- [x] 9.5.1 `scan` covered `openspec/specs/`, `docs/` and `README.md` and NOTHING
  under `chart/`. So NOTES.txt taught `rbacMode: full` as the way to grant a
  route — printed to every operator at install time — and no guard saw it. The
  text is fixed and the BLIND SPOT is closed
- [x] 9.5.2 `scan` widened to `chart/values.yaml`, `chart/charts/*/values.yaml`
  and `chart/templates/NOTES.txt`, and everything it reported is fixed. **31
  occurrences across 10 rules at the start; the chart is at ZERO.** The remainder
  are in `openspec/specs/`, which this change's own deltas replace at sync
- [x] 9.5.3 Decide per rule whether the comment is a RECORD (add the word) or a
  stale claim (rewrite it).
  **BOTH KINDS WERE PRESENT, AND THE SPLIT WAS NOT WHAT THE TASK ASSUMED.** The
  `kubernetes` bundle's values were not records at all — sixteen of them
  described the DELETED release-wide mode as the live control, three releases
  after it went. Nobody reads a subchart's values.yaml against a parent change,
  and no guard was looking at it. The parent's were mostly genuine records
  needing one word.
  - **A RECORD WORD MUST SIT WITHIN ONE LINE OF THE MATCH**, and hard-wrapped
    prose puts it two lines away about as often as one. Three of these passed
    only after moving the sentence, not after writing it

### 9.6 No account without a grant, everywhere

- [x] 9.6.1 The `home-assistant` and `prometheus` MCP servers render no
  ServiceAccount — both mount no token
- [x] 9.6.2 `agentops-mcp-k8s` keeps its account: it mounts its token and carries
  the cluster grant
- [x] 9.6.3 The guard refusing an MCP account equal to the runtime's still fires
  when one is NAMED, and is a no-op when none is rendered.
  **AND IT WAS DEAD CODE:** it read `global.agentops.runtime.serviceAccountName`,
  a key that now FAILS the render, so it could never fire on any install that
  rendered at all. It reads `runtimeDefaults` now
- [x] 9.6.4 The guard's WORKLOAD exemption now requires the pod to mount the
  token. It was written to justify keeping those two accounts, which is the wrong
  direction for a guard

### 9.7 The adapter's identity is a reference, not the manager's to create

- [x] 9.7.1 `serviceAccountName` on both adapter kinds, a REFERENCE the operator
  never creates, validates or binds
- [x] 9.7.2 `kubernetesAccess` deleted from both, no alias
- [x] 9.7.3 Deepcopy and CRDs regenerated. A deleted CRD field makes
  `kubectl apply -f chart/crds/` an UPGRADE step and an INSTALL step
- [x] 9.7.4 `adapterworkload.go` creates no ServiceAccount; it resolves the name,
  falls back to the floor, mounts the token, injects `POD_NAMESPACE`
  unconditionally
- [x] 9.7.5 The floor's name reaches the manager as BOOTSTRAP CONFIGURATION.
  **AND THE FIRST DEPLOY SHIPPED WITHOUT IT.** The reconciler read
  `FLOOR_SERVICE_ACCOUNT` and the chart never set it, so it resolved EMPTY — and
  an empty `serviceAccountName` on a pod is not "no account", it is the namespace
  `default`, WITH its token mounted. Three adapters ran that way. Every render
  passed, every pod was Running, and the only way to see it was to list the pods'
  identities. `TestTheManagerIsToldTheFloorAccount` now fails the render that
  forgets it
- [x] 9.7.6 Test: reconciling an adapter creates NO ServiceAccount
- [x] 9.7.7 Test: the pod names the floor when the CR names nothing, and the named
  account when it does
- [x] 9.7.8 The chart renders each granted adapter's account BESIDE its grant —
  `signal-k8s-events`, the console, `signal-alertmanager`
- [x] 9.7.9 Every other adapter CR names nothing and inherits the floor
- [x] 9.7.10 A retired-key guard FAILS the render on `kubernetesAccess`
- [x] 9.7.11 `.github/retired-vocabulary.json` — `kubernetesAccess`
- [x] 9.7.12 Upgrading ORPHANS the accounts the manager owned rather than deleting
  them. Nothing is bound to them; named in the changelog

## 10. Documentation — the reopened work

### 10.1 The reference docs

- [x] 10.1.1 `docs/CHANGELOG.md` — the 11.0.0 entry: the deleted `rbacMode`, the
  moved vendor config, the accounts that disappear, the deleted adapter CRD field
  with its `kubectl apply -f chart/crds/` step, and the orphaned accounts
- [x] 10.1.2 `docs/concepts.md` — the account model, one rule for every lane
- [x] 10.1.3 `docs/installation.md` — the values after the move, and where the
  credential now goes
- [x] 10.1.4 `docs/kubernetes.md`, `docs/home-assistant.md` — which routes render
  an account and which inherit the floor
- [x] 10.1.5 `python3 .github/scripts/docs-generate.py` re-run; a stale marker
  naming `kubernetesAccess` FAILED the generator, which is what it is for
- [x] 10.1.6 `.claude/rules/invariants.md`, `wiring.md`, `adapters.md`,
  `structure.md` — `wiring.md`'s "No reconciler makes one" stops being nearly true
- [x] 10.1.7 The adapter contracts lose `kubernetesAccess`
- [x] 10.1.8 The bundle pages name the account each renders and what it grants
- [x] 10.1.9 The CHANGELOG entry covers all four changes

### 10.2 The adopter site

- [x] 10.2.1 `docs/getting-started.md` — the credential line, which is the first
  thing an adopter types
- [x] 10.2.2 `docs/guides/pipeline.md` — cluster power is an account you declare
  with rules you wrote, not a mode you name
- [x] 10.2.3 `docs/guides/signal-adapter.md` — an adapter names its account; the
  chart creates it; naming none means the floor
