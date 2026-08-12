## Why

Enabling `k8s-bundle` — or turning on demo mode, which IS the bundle with its
defaults — produces an install that looks complete and does nothing. The adapter
watches, the source admits, and every signal drops at `Wired=False`, because
wiring is pipeline-only and no bundle ships a Pipeline. The one thing standing
between a working demo and an inert one is a CR the operator has to hand-write
after reading NOTES.txt.

The rule that produced this is sound but stated too absolutely. Its harm — a
subchart wiring only its own lane because it cannot see the others — does not
apply to a bundle that owns its whole lane: k8s-bundle renders the source, the
profile and both toolsets. The pending `ha-bundle` change already relaxes the
rule to a conditional one for exactly this reason; this change adopts that
relaxation for k8s-bundle and pins the default it must carry.

## What Changes

- **The wiring rule softens from absolute to conditional-and-off-by-default.** A
  subchart MAY render Pipelines when rendering is behind an explicit flag, every
  foreign object it names is values-supplied and omitted when unset, and each
  Pipeline renders only with its own profile. Shipped wiring SHALL default OFF —
  the parent's `pipelines:` stays the place an install declares routes — with
  ONE exception: **demo mode**, which turns the read-only route on, because demo
  mode's whole promise is a working install from one flag.
- **k8s-bundle gains a `pipelines` component**, off by default, self-gated on
  `enabled OR global.demo.enabled` like every other bundle template. It renders
  **at most one** Pipeline claiming the bundle's own `cluster-events` source:
  - `k8s-observe` — binds `agentops-observe` + the `k8s-observability` toolset +
    the `k8s-api` MCPConfig. An agent that reads the cluster and changes nothing.
  - `k8s-operate` — the same plus the `k8s-admin` mutating toolset.
- **Which one renders DERIVES from `global.agentops.runtime.rbacMode`**, the same
  value `mcpServers.readOnly`, the server's RBAC mode and the `k8s-admin` toolset
  already follow: `full` renders `k8s-operate`, every other mode (including unset
  and `none`) renders `k8s-observe`. Per-route booleans override the derivation
  in both directions. Both at once is possible and is NOT blocked — sources are
  shareable — but it fans out: one event, two conversations, two agents.
- **Channels are values-supplied and omitted when unset.** With none bound the
  conversation dispatches immediately and its answer lands in
  `status.runs[].result` — the path the demo instructions already document.
- **The k8s-bundle spec's self-contradiction is resolved.** It currently says
  both "the bundle SHALL render no `Pipeline` of its own" and "a `SignalSource`
  … TOGETHER WITH the `Pipeline` claiming it". After this change one statement
  stands, conditioned on the flag.
- **NOTES.txt reports the fan-out** when the install's own `pipelines:` also
  claims `cluster-events`. Not a render failure: two Ready Pipelines on one
  source is a supported shape, and refusing it would re-introduce the
  source-conflict guard that was deliberately deleted.

Not in scope: shipping wiring from `telegram-bundle` or `vm-bundle` (their
routes genuinely span bundles), any change to the fan-out semantics themselves,
and the `ha-bundle` change's own wiring, which lands with that change.

## Capabilities

### Modified Capabilities

- `pipeline-model`: the chart-managed wiring requirement relaxes from "no
  subchart renders a Pipeline" to "a subchart MAY, under stated conditions, and
  the flag defaults off except where a demo path requires otherwise".
- `k8s-bundle`: a fourth component — the bundle's own wiring — with its
  derivation from the release RBAC mode, its demo behavior, and the resolution
  of the spec's existing contradiction about whether the bundle ships a Pipeline.

## Impact

- **New**: `chart/charts/k8s-bundle/templates/pipelines.yaml`; helpers
  `k8s-bundle.wiringActive`, `k8s-bundle.observePipelineEnabled`,
  `k8s-bundle.adminPipelineEnabled` in the bundle's `_helpers.tpl`.
- **Modified**: `chart/charts/k8s-bundle/values.yaml` (the `pipelines:` block),
  `chart/templates/NOTES.txt` (the fan-out note, and the demo instructions no
  longer telling the reader to hand-write a Pipeline),
  `chart/charts/k8s-bundle/templates/events.yaml` (its header comment already
  describes a Pipeline it does not render), `chart/Chart.yaml` +
  `chart/charts/k8s-bundle/Chart.yaml` (version bump), `CHANGELOG.md`,
  `docs/k8s-bundle.md`, `CLAUDE.md` (the "NO bundle ships a Pipeline" gotcha and
  the k8s-bundle map entry), `internal/integration/charttemplate_test.go`.
- **Behavior change for existing installs**: a `global.demo.enabled` install that
  previously rendered no Pipeline now renders `k8s-observe`, which claims
  `cluster-events` and therefore starts answering events — LLM spend where there
  was none. An install that already declares its own claiming Pipeline gets the
  fan-out unless it sets `k8s-bundle.pipelines.enabled: false`. Both belong in
  the CHANGELOG's upgrade steps.
- **No effect on the manager, any Go module outside the chart test, the CRDs, or
  the console.** Nothing about fan-out, shareable sources or Pipeline validation
  changes; this is a chart that now writes a CR an operator used to write.
