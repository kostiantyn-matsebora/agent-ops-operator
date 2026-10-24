## 1. Built-in toolset catalog

- [x] 1.1 `chart/values.yaml`: add `global.builtinToolsets.websearch: {name: agentops-websearch, tools: [WebSearch]}` beside `observe`/`shell`/`edit`, matching their shape exactly.
- [x] 1.2 `chart/templates/builtin-toolsets.yaml`: extend the rendered-key loop from `(list "observe" "shell" "edit")` to `(list "observe" "shell" "edit" "websearch")`; verify `helm template chart | grep -A3 'name: agentops-websearch'` shows an `MCPToolset` with `tools: [WebSearch]`.

## 2. Kubernetes bundle wiring

- [x] 2.1 `chart/charts/kubernetes/templates/pipelines.yaml`: append `(dig "builtinToolsets" "websearch" "name" "agentops-websearch" $g)` to `$writeTools` (not `$readTools`), so only the admin route (`k8s-operate` by default) binds it; verify `helm template chart --set kubernetes.enabled=true --set kubernetes.pipelines.enabled=true --set kubernetes.pipelines.admin.enabled=true` shows `agentops-websearch` in the admin route's `Pipeline.spec.toolsets.refs` and absent from the observe route's.
- [x] 2.2 `chart/charts/kubernetes/values.yaml`: comment the new binding beside the existing toolset wiring comments, naming which route gets it and why.

## 3. Home Assistant bundle wiring

- [x] 3.1 `chart/charts/home-assistant/templates/pipelines.yaml`: append `(dig "builtinToolsets" "websearch" "name" "agentops-websearch" $g)` to `$opsOnlyTools` (not `$baseTools`), so only `ha-ops` binds it; verify `helm template chart --set home-assistant.enabled=true --set home-assistant.homeAssistant.endpoint=https://ha.example.org --set home-assistant.homeAssistant.credentials.operatorToken=ci-token --set home-assistant.pipelines.enabled=true` shows `agentops-websearch` in `ha-ops`'s `Pipeline.spec.toolsets.refs` and absent from `ha-control`'s.
- [x] 3.2 `chart/charts/home-assistant/values.yaml`: comment the new binding the same way.

## 4. Documented install-level examples

- [x] 4.1 `docs/configuration.md` and `docs/concepts.md`: add `agentops-websearch` to the `k8s-operate` example's `toolsets` list in both worked examples (the two-tier `k8s-observe`/`k8s-operate` snippet), leaving `k8s-observe`'s list unchanged.
- [x] 4.2 `chart/values.yaml`'s commented `pipelines:` example block: add `agentops-websearch` to its `toolsets:` line (the combined `k8s-ops` example).

## 5. Unit tests

- [x] 5.1 `helm lint chart` passes.
- [x] 5.2 Every permutation in `.github/workflows/ci.yml`'s render matrix still renders and validates (`helm template` + `kubeconform`) locally for the `kubernetes` and `home-assistant` permutations at minimum, run by hand with the same `--set` flags the workflow uses.
- [x] 5.3 `python3 .github/scripts/serviceaccount-guard.py` against the kubernetes and home-assistant permutation renders still passes — this change adds no ServiceAccount, but a `toolsets.refs` edit is exactly the kind of change that should be re-checked against it.

## 6. E2E tests

- [x] 6.1 Nothing here is decided by a cluster: `MCPToolset` is an opaque string-list CR with no controller and no reconcile behavior, and `--allowedTools` composition happens in the runtime process, not in anything envtest or the e2e pack exercises. `platform/manager/test/e2e/` gets no new lane.

## 7. Documentation

### 7.1 Reference docs

- [x] 7.1.1 `docs/concepts.md`: add a fourth row to the built-in toolset table (`| agentops-websearch | WebSearch |`), beside `agentops-observe`/`-shell`/`-edit`.
- [x] 7.1.2 `docs/CHANGELOG.md`: an `Added` entry — the fourth risk-split toolset, and that `k8s-operate`/`ha-ops` bind it by default when their bundle's wiring is enabled. Not a breaking change.

### 7.2 Adopter site

- [x] 7.2.1 `docs/integrations/kubernetes.md`: re-run `python3 .github/scripts/docs-generate.py` to refresh the `<!-- generated: renders bundle=kubernetes -->` block, then add a sentence to the admin-route section noting it can now search the web.
- [x] 7.2.2 `docs/integrations/home-assistant.md`: the same — regenerate the `renders bundle=home-assistant` block and add the same sentence to the `ha-ops` description, naming that `ha-control` does not gain it.
- [x] 7.2.3 `python3 .github/scripts/docs-generate.py --check` passes with no stale generated block remaining anywhere in `docs/`.
