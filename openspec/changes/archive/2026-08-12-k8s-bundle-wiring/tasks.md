## 1. Bundle values

- [x] 1.1 Add the `pipelines:` block to `chart/charts/k8s-bundle/values.yaml`:
  `enabled: false` (with the one-line note that demo mode forces it on),
  `observe: {enabled: null, name: k8s-observe}`,
  `admin: {enabled: null, name: k8s-operate}`, `channels: []`
- [x] 1.2 Document the derivation at the values, in the bundle's existing
  comment style: `null` follows `global.agentops.runtime.rbacMode` — `full`
  renders the acting route, everything else the observing one — and an explicit
  boolean wins in both directions
- [x] 1.3 State the fan-out cost where both routes can be turned on: two Ready
  Pipelines on one source open two conversations per event, and it is allowed
  because sources are shareable
- [x] 1.4 Add the third effect to the `global.agentops.runtime.rbacMode` comment
  in `chart/values.yaml`: widening to `full` now also promotes the bundle's
  route, alongside the server and the mutating toolset

## 2. Helpers

- [x] 2.1 `k8s-bundle.wiringActive` in the bundle's `_helpers.tpl`: bundle active
  AND `pipelines.enabled` OR demo mode; empty otherwise
- [x] 2.2 `k8s-bundle.observePipelineEnabled`: explicit `pipelines.observe.enabled`
  wins; otherwise render unless `agentops.runtimeRbacMode` is `full`
- [x] 2.3 `k8s-bundle.adminPipelineEnabled`: explicit `pipelines.admin.enabled`
  wins; otherwise render only when `agentops.runtimeRbacMode` is `full`
- [x] 2.4 Each helper carries the same kind of header comment the existing
  derivation helpers carry — what it decides and why it derives rather than
  defaults

## 3. Pipeline template

- [x] 3.1 `chart/charts/k8s-bundle/templates/pipelines.yaml`, gated on
  `k8s-bundle.wiringActive` AND `profile.enabled` — no profile, no route
- [x] 3.2 Render the observing Pipeline when its helper says so: `profileRef` the
  bundle's profile name, `signalSourceRefs` the bundle's source (only when
  `eventsAdapter.source.create`), `toolsets.refs` = the built-in observe toolset
  (only when `global.builtinToolsets.enabled`) + `mcp.toolsets.observe.name`
  (only when `mcp.enabled`), `mcpConfigs.refs` = `mcp.name` (only when
  `mcp.enabled`)
- [x] 3.3 Render the acting Pipeline on the same shape plus
  `mcp.toolsets.admin.name`, and only when the mutating toolset itself renders
  (`k8s-bundle.mcpAdminEnabled`) — a ref to a toolset nobody created is how an
  allowlist rots
- [x] 3.4 Emit `channelRefs` only when `pipelines.channels` is non-empty; the
  key is absent, never null, when the list is empty
- [x] 3.5 Label both with `app.kubernetes.io/name: agentops-k8s-bundle`, as every
  other bundle object is
- [x] 3.6 Fix the header comment in `templates/events.yaml`, which already
  describes rendering "a SignalSource AND the Pipeline claiming it" — the source
  component renders the source; wiring is now its own component

## 4. Post-install notes

- [x] 4.1 In `chart/templates/NOTES.txt`, print what bundle wiring rendered when
  it is active — the Pipeline name and whether it can mutate
- [x] 4.2 Print the fan-out note when an entry in the parent's `pipelines:` also
  lists the bundle's source name: each event now opens two conversations. A
  note, never a render failure
- [x] 4.3 Update the existing "declare it under the parent chart's `pipelines:`"
  prompt so it no longer tells a demo installer to write wiring the chart just
  rendered

## 5. Tests

- [x] 5.1 `internal/integration/charttemplate_test.go`: a default install renders
  no Pipeline from the bundle
- [x] 5.2 `global.demo.enabled=true` alone renders exactly one Pipeline, claiming
  the bundle's source, binding the read toolset and the MCPConfig and NOT the
  mutating toolset
- [x] 5.3 Wiring enabled with `global.agentops.runtime.rbacMode=full` renders
  exactly the acting route, binding the mutating toolset
- [x] 5.4 Explicit values override the derivation in both directions
- [x] 5.5 Both routes enabled renders two Pipelines and does not fail
- [x] 5.6 `pipelines.enabled=false` under demo mode renders none, and every other
  bundle object still renders
- [x] 5.7 `profile.enabled=false` with wiring on renders no Pipeline
- [x] 5.8 `channelRefs` appears only when `pipelines.channels` is set; with
  components off, no ref names an object that was not rendered

## 6. Chart version and migration

- [x] 6.1 Bump `chart/Chart.yaml` (minor) and
  `chart/charts/k8s-bundle/Chart.yaml`
- [x] 6.2 `CHANGELOG.md`, newest first: what demo mode now renders and that it
  starts answering events (LLM spend where there was none), the opt-out
  `k8s-bundle.pipelines.enabled: false`, and the fan-out case for an install
  that already declares its own claim

## 7. Documentation

- [x] 7.1 `docs/k8s-bundle.md`: a row for the wiring component in the component
  table, the derivation table (`rbacMode` → which route), the values-supplied
  channel rule, and the fan-out warning. Correct the statements that the bundle
  ships no Pipeline
- [x] 7.2 `CLAUDE.md`: rewrite the "NO bundle ships a Pipeline" gotcha to the
  conditional rule — flag-gated, values-supplied foreign names, profile-coupled,
  default off except a turnkey mode — keeping `telegram-bundle` and `vm-bundle`
  as the counter-examples
- [x] 7.3 `CLAUDE.md` map: add the wiring component to the k8s-bundle entry
- [x] 7.4 Reconcile with the pending `ha-bundle` change: whichever lands second
  folds the other's wording into the one chart-managed-wiring requirement rather
  than restating it

## 8. Verification

- [x] 8.1 `helm template` the four shapes (default, bundle-only, demo,
  demo + `rbacMode: full`) and diff the rendered Pipelines against what the
  tests assert
- [x] 8.2 Server-side dry-run against the live cluster before any apply —
  repo-side CRD validation is not proof
- [x] 8.3 Smoke: demo install, post a `kind: task` signal to `cluster-events`,
  confirm a conversation is created and the answer appears in
  `status.runs[].result` with no channel bound
