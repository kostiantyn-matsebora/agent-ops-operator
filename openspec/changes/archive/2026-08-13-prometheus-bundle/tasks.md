## 1. Rename the subchart, guard the old key

- [x] 1.1 `git mv chart/charts/vm-bundle chart/charts/prometheus-bundle`; rename
  the chart in its `Chart.yaml` and rewrite the description, which still
  advertises vmlogs/vmmetrics MCPConfigs
- [x] 1.2 `chart/Chart.yaml`: rename the dependency and its
  `condition: prometheus-bundle.enabled`; regenerate `chart/Chart.lock`
  (`helm dependency update` — helmfile refuses a stale lock)
- [x] 1.3 `chart/values.yaml`: rename the `vm-bundle:` pointer block to
  `prometheus-bundle:` and rewrite its comment — the bundle is named for the
  payload format and query API, VictoriaMetrics is one backend
- [x] 1.4 Add the retired-key guard: a `vm-bundle:` key still present FAILS the
  render naming `prometheus-bundle`, in the shape of the existing
  `serviceAccounts.runtime` guard. Helm never reports an unread values key, so
  without this an upgrade presents as a successful install rendering nothing
- [x] 1.5 `chart/templates/NOTES.txt`: rename `index .Values "vm-bundle"` and
  every reference in that block
- [x] 1.6 Default the adapter CR name to `alertmanager` (it is the routing key),
  keeping it values-configurable so an install restores `vm-alertmanager` with
  one value instead of editing every hand-written source
- [x] 1.7 Verify the rename alone renders byte-identical output apart from names:
  `helm template` before and after with the bundle enabled, diffed

## 2. Metrics MCP component

- [x] 2.1 Replace `mcp.vmlogs`/`mcp.vmmetrics` with a single `mcp` component
  rendering the `MCPConfig` under the FIXED server key `prometheus` (no values
  path — the key IS the tool namespace) plus its `MCPToolset`
- [x] 2.2 Keep `headers` passthrough with `valueFrom` secret refs, and the loud
  failure for an enabled component with no deployed server and no `url`
- [x] 2.3 Document at the values why the URL is never derived: single-node VM
  serves `/api/v1`, cluster mode `/select/<accountID>/prometheus/api/v1`,
  Prometheus whatever external URL it was given — and why ONE key serves both
  backends (VM answers the Prometheus query API and reports a Prometheus version
  on `buildinfo`; MetricsQL is a PromQL superset)
- [x] 2.4 Settle the design's first open question: pick the Prometheus MCP server
  image, pin it, and record what it registers
- [x] 2.5 Settle the second: enumerate the toolset's tools or wildcard
  `mcp__prometheus__*`, decided by that inventory — a query-only server has no
  read/mutate split to preserve, unlike `k8s-bundle`'s

## 3. MCP server workload

- [x] 3.1 Replace `mcpServers.vmlogs`/`.vmmetrics` with one `mcpServers`
  component deploying the chosen image against a REQUIRED `backend` URL, failing
  the render when empty
- [x] 3.2 Default the `MCPConfig` URL onto the deployed Service when the config's
  own `url` is empty; an explicit `url` still wins
- [x] 3.3 Both components default OFF and flip together — unlike `k8s-bundle`'s,
  because there is no in-cluster endpoint to default onto. Say why in the comment
- [x] 3.4 Render the server's own ServiceAccount and FAIL the render when it
  equals the runtime SA. Render NO RBAC for it: it reads an HTTP endpoint, not
  the Kubernetes API

## 4. Remove the logs component

- [x] 4.1 Delete `mcp.vmlogs`, the `mcp-victorialogs` workload, and every values
  key, template branch and toolset entry that served them
- [x] 4.2 Write the replacement into the CHANGELOG as copy-pasteable objects: the
  `MCPConfig` with server key `victorialogs` and the same URL the bundle used,
  plus an `MCPToolset` granting `mcp__victorialogs__*`. Removing it silently
  costs a working install its log access

## 5. Alert-investigator profile

- [x] 5.1 `templates/profile.yaml`: one `AgentProfile`, identity ONLY — no
  repository, no `allowedTools`, no `mcp`, no substrate — mirroring
  `k8s-bundle/templates/profile.yaml`
- [x] 5.2 Ship the inline `systemPrompt` role: read the alert, query the metric
  that fired BEFORE concluding, state the likely cause with its evidence, answer
  briefly. Without it an alert wakes a personality-free agent, since a profile
  with no repository can resolve no agent definition file
- [x] 5.3 `maxTurns` and an optional `runtimeRef` that emits nothing when empty,
  falling back to the runtime the parent guarantees

## 6. Wiring component

- [x] 6.1 Create the subchart's `_helpers.tpl` — it has none today — with the
  `active` gate and a `wiringActive` helper mirroring `k8s-bundle`'s. Note in the
  header comment that the demo branch is inert here because demo mode never
  enables this bundle, and that the shape is kept identical on purpose
- [x] 6.2 `templates/pipelines.yaml` gated on `wiringActive` AND
  `profile.enabled` — no profile, no route. ONE route only: a query server is
  read-only, so there is no second posture and no `rbacMode` derivation
- [x] 6.3 Refs emitted only when the object exists: the source (only under
  `alertmanager.enabled` AND `defaultSource.enabled`), the toolset and MCPConfig
  (only under `mcp.enabled`)
- [x] 6.4 `channelRefs` only when `pipelines.channels` is non-empty — key absent,
  never null
- [x] 6.5 Bundle label on the object, as every other bundle object carries

## 7. Post-install notes

- [x] 7.1 Print the Alertmanager `receivers:` stanza per rendered source: the
  Service webhook URL, the bearer form when the source carries
  `credentialsSecretRef`, and `send_resolved: false` — the adapter drops
  non-firing alerts, so a default sender posts resolutions that are discarded
- [x] 7.2 Print what the wiring rendered when it is active, and the read-only
  nature of what it binds
- [x] 7.3 Print the double-claim note when an install-declared pipeline also
  lists the bundle's alert source — a NOTE, never a render failure
- [x] 7.4 Only print "nothing answers yet" when NOBODY claims the source. The
  same defect was found and fixed twice in `k8s-bundle-wiring`; do not
  reintroduce it here
- [x] 7.5 State in the registration branch that self-registration is
  VictoriaMetrics-only and why, pointing at the printed receiver otherwise

## 8. Tests

- [x] 8.1 `internal/integration/charttemplate_test.go`: a default install renders
  nothing from the bundle, and demo mode renders nothing either
- [x] 8.2 A `vm-bundle:` key still present FAILS the render naming the new key
- [x] 8.3 Bundle enabled with defaults renders no Pipeline; the source reports
  its unclaimed state
- [x] 8.4 Wiring enabled renders exactly one Pipeline claiming the bundle's
  source with its profile, toolset and MCPConfig
- [x] 8.5 Wiring with `profile.enabled=false` renders no Pipeline
- [x] 8.6 `channelRefs` appears only when named; with the metrics component off,
  no ref names an object that was not rendered
- [x] 8.7 The MCP endpoint guard and the backend guard both still fire
- [x] 8.8 The server SA equal to the runtime SA fails the render
- [x] 8.9 The rendered source's `spec.adapter` follows the adapter name value
- [x] 8.10 NOTES: the receiver stanza appears with `send_resolved: false`; the
  double-claim note appears only when both claim; the "nothing answers" prompt
  appears only when nobody does. Disable the bundle's cluster-scoped RBAC in
  these cases so a dry-run install cannot collide with a live release

## 9. Chart version and migration

- [x] 9.1 Bump `chart/Chart.yaml` (minor) and the subchart's version
- [x] 9.2 `CHANGELOG.md`, newest first, covering every breaking edge in one
  entry: the `vm-bundle:` → `prometheus-bundle:` key rename and the guard that
  refuses the upgrade without it; the logs removal with both replacement objects
  printed; `mcp__victoriametrics__*` ceasing to resolve, with the `kubectl` query
  that finds affected Pipelines; the adapter CR name default and its one-value
  override; and the new profile/wiring defaults

## 10. Documentation

- [x] 10.1 `git mv docs/vm-bundle.md docs/prometheus-bundle.md` and rewrite it:
  the component table gains the profile and wiring rows, registration is labelled
  VictoriaMetrics-only, the vanilla Alertmanager receiver is shown, and the logs
  section is replaced by the hand-apply instructions
- [x] 10.2 Correct the stale claim in that page that `defaultSource.enabled` plus
  a `profileRef` renders "a turnkey SignalSource AND the Pipeline claiming it" —
  no template rendered it; the wiring component does now, under its own flag
- [x] 10.3 `README.md` documentation index: rename the entry
- [x] 10.4 `CLAUDE.md` map: rewrite the bundle's entry for the new component set
- [x] 10.5 `CLAUDE.md` wiring gotcha: it names `vm-bundle` as a counter-example
  that ships no wiring. That is no longer true — leave `telegram-bundle` as the
  counter-example and state why this bundle now qualifies (it owns its lane;
  channels are its only foreign name)
- [x] 10.6 `git mv openspec/specs/vm-bundle openspec/specs/prometheus-bundle` at
  archive time, so the delta applies to the renamed capability

## 11. Verification

- [x] 11.1 `helm template` the shapes the tests assert and diff the rendered
  Pipeline, MCPConfig and profile against them
- [x] 11.2 `helm lint`, `go vet`, `gofmt`, and the full `go test ./...` with
  envtest assets
- [x] 11.3 Server-side dry-run against the live cluster before any apply —
  repo-side CRD validation is not proof
- [x] 11.4 Smoke in a THROWAWAY namespace, then delete: check cluster-scoped name
  collisions against the live release FIRST and disable the colliding components;
  post an Alertmanager-shaped webhook to the rendered URL, confirm a conversation
  opens and the answer reaches `status.runs[].result`. Clear the
  `agentops.dev/close-topics` finalizer before deleting the namespace — the
  manager that releases it is uninstalled first, so deletion otherwise wedges
