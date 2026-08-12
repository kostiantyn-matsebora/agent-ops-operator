## Context

Wiring is pipeline-only: a `SignalSource` no Ready Pipeline claims reports
`Wired=False` and drops every signal. `k8s-bundle` renders the source, the
adapter, the profile, the MCPConfig and both toolsets — everything except the
one object that makes them do anything. Demo mode is defined as "this bundle
with its defaults", so `global.demo.enabled: true` currently installs a complete
Kubernetes agent that answers nothing until the operator reads NOTES.txt and
applies a Pipeline by hand.

Three existing facts shape the solution:

- **The pending `ha-bundle` change already relaxes the rule** in
  `openspec/specs/pipeline-model/spec.md` to a conditional form: flag-gated,
  values-supplied foreign names, pipeline-renders-with-its-profile. That change
  sets its own flag to default TRUE. This change adopts the same conditions and
  sets k8s-bundle's default to FALSE-except-demo, so the two deltas must be
  reconciled into one requirement rather than fighting over it.
- **Sources are shareable since chart 5.10.0.** Two Ready Pipelines claiming
  `cluster-events` is legal and opens two conversations per event. There is no
  conflict condition and no tiebreak; `sourceConflicts` was deleted and
  re-adding one is a regression. Anything this change does about double-claiming
  is therefore a WARNING, never a guard.
- **The bundle already derives three things from
  `global.agentops.runtime.rbacMode`**: `mcpServers.readOnly`, the MCP server
  SA's RBAC mode, and whether the `k8s-admin` toolset renders at all. A fourth
  derivation from the same value is the established idiom here, not a new one.

## Goals / Non-Goals

**Goals:**

- `global.demo.enabled: true` alone produces an install that answers cluster
  events, with no hand-written CR.
- An operator who enables the bundle for its parts still gets NO wiring unless
  they ask for it.
- The route's power matches the release's declared posture without a second
  knob to keep in sync.
- The relaxed rule states its own limits, so "bundle ships wiring" does not
  spread to bundles whose routes span components.

**Non-Goals:**

- Changing fan-out, shareability or Pipeline validation in the manager.
- Shipping wiring from `telegram-bundle` or `vm-bundle`.
- Any attempt to detect or prevent double-claiming from inside the subchart.
- A per-Pipeline runtime or ServiceAccount — the SA stays runtime-level, because
  a Pipeline choosing an SA would make pipeline-edit rights an escalation.

## Decisions

### 1. The rule becomes conditional, and shipped wiring defaults OFF

The absolute form ("a subchart SHALL NOT render `Pipeline` objects") is replaced
by the `ha-bundle` conditions plus one this change adds: **a subchart's wiring
flag SHALL default to off**, and MAY be forced on only by a values path whose
declared purpose is a working turnkey install — which today is exactly
`global.demo.enabled`.

The default matters more than the permission. An install that enables a bundle
for its adapter and profile, then declares its own cross-bundle route, must not
silently acquire a second one; the flag being off is what keeps the parent's
`pipelines:` the normal answer and bundle wiring the exception.

*Consequence for `ha-bundle`:* its flag defaults true, which this wording
forbids. The reconciliation is stated in the delta: the default-off rule binds
bundles whose lane is shared with other components, and `ha-bundle`'s own spec
carries its exception explicitly rather than the general requirement carrying
it. Whichever change lands second updates the other's delta rather than
re-arguing the rule.

### 2. At most one Pipeline, selected by `rbacMode`

`k8s-observe` and `k8s-operate` differ in exactly one binding — the `k8s-admin`
toolset — and both claim `cluster-events`. Rendering both by default would mean
every event opens two conversations, one of which can mutate the cluster; that
is a bill and a blast radius nobody asked for.

Selection therefore derives from the release's single posture value:

| `global.agentops.runtime.rbacMode` | renders |
|---|---|
| unset, `none`, `readonly` | `k8s-observe` |
| `full` | `k8s-operate` |

with `pipelines.observe.enabled` / `pipelines.admin.enabled` as explicit
booleans that win in both directions. This is the same
explicit-wins-else-derive shape as `mcpServers.readOnly` and
`mcp.toolsets.admin.enabled`, and it composes correctly with them: under `full`
the server registers mutating tools, the `k8s-admin` toolset renders, and the
route that binds it renders too — one decision, three consistent effects.

*Alternative considered:* two independent booleans, both false, no derivation.
Simpler to read; the failure it permits is an operator who sets `full`, gets a
write-capable server and a `k8s-admin` toolset, and a route that binds neither —
cluster-admin granted and unusable. Rejected for the same reason
`mcpServers.readOnly` derives.

*Alternative considered:* one Pipeline whose toolsets vary by mode. No fan-out
is possible by construction, but it forecloses running an observing route beside
an acting one later, and it makes `kubectl get pipelines` report a name that
does not say what it may do.

### 3. `k8s-operate`, not `k8s-admin`, for the mutating route's CR name

The values key stays `pipelines.admin` (matching `mcp.toolsets.admin`), but the
rendered CR is named `k8s-operate` because `k8s-admin` is already an
`MCPToolset` name. Two kinds may share a name in Kubernetes; documentation
cannot — "bind `k8s-admin` on `k8s-admin`" is a sentence nobody should have to
parse.

### 4. Foreign names are values-supplied and omitted when unset

The bundle names four objects it renders itself (`cluster-events`,
`k8s-engineer`, `k8s-observability`, `k8s-admin`, `k8s-api`) and one class it
does not: channels. `pipelines.channels` is a list of names, empty by default,
emitted only when set — the `ha-bundle` condition, and the reason the bundle can
ship wiring without ever naming an object no component created.

With no channels bound, dispatch does not wait (the pending-topic gate applies
only to conversations that HAVE channel refs) and the answer lands in
`status.runs[].result` — which is exactly what the demo instructions in
`chart/values.yaml` already tell the reader to read. A demo install therefore
needs no chat surface to be useful, and gains one by adding a name here.

The toolset and MCPConfig refs are omitted when their components are off, so
`mcp.enabled: false` yields a route with observation built-ins and no cluster
reach — degraded, honest, and not a render failure.

### 5. Double-claiming is reported, never refused

The subchart cannot see the parent's `pipelines:` (a subchart reads no parent
scope but `global.`), but the PARENT can see the subchart's values. The check
therefore lives in `chart/templates/NOTES.txt`: when bundle wiring is active and
an install-declared pipeline also lists the bundle's source name, print what
that means — each event opens two conversations, under two profiles, and both
agents act.

It stays a note because shareable sources are a deliberate design property.
Failing the render on two claimants would be `sourceConflicts` returning under a
new name, one layer up.

### 6. Demo mode forces the flag, not the route

`global.demo.enabled` makes `pipelines.enabled` true; it does not pin which
route renders. Demo leaves `rbacMode` empty, which resolves to `readonly` for
the runtime SA and therefore to `k8s-observe` here — the read-only route, as
required. An operator who sets demo mode AND `rbacMode: full` has explicitly
asked for a write-capable agent and gets `k8s-operate`; that is the same
override that already widens the MCP server, and demo mode has never been a
safety ceiling — it is an enablement path.

## Risks / Trade-offs

- **An upgrade starts spending money.** A `global.demo.enabled` install that
  answered nothing now answers every admitted event. This is the intended fix
  and a real behavior change: it goes in the CHANGELOG's upgrade steps with the
  one-line opt-out (`k8s-bundle.pipelines.enabled: false`), and NOTES.txt says
  it at install time.
- **Two claimants after an upgrade.** An install that hand-wrote its own claim
  and also enables the bundle gets the fan-out. Mitigated by the flag defaulting
  off outside demo, and by the NOTES.txt report for the demo case.
- **`full` silently promotes the route.** Widening `rbacMode` to `full` now
  changes which Pipeline exists, on top of widening the server and rendering the
  mutating toolset. That chain is the point — one posture, consistently applied
  — but it is a chain, so the values comment names all three effects at the
  knob, not only here.
- **The relaxation spreads.** Named limits (flag-gated, values-supplied foreign
  names, profile-coupled, default off) plus the surviving counter-examples
  (`telegram-bundle`, `vm-bundle` ship none) are what keep this from becoming
  "every bundle wires itself". The delta says so in the requirement text rather
  than only in a design document.
- **Two changes edit one requirement.** `ha-bundle` and this change both modify
  the chart-managed-wiring requirement. Whichever archives second must fold the
  other's text in; the delta here is written to be a superset so folding is
  additive.

## Migration Plan

1. Land the values block, the templates and the helpers; the bundle stays off,
   so a default install renders exactly what it rendered before.
2. Bump the chart minor version and write the CHANGELOG entry: what demo mode
   now renders, the opt-out, and the fan-out case.
3. Update `docs/k8s-bundle.md` (the wiring table row and the "the bundle ships no
   Pipeline" statements), `CLAUDE.md` (the gotcha and the map entry), and
   NOTES.txt.
4. Extend `internal/integration/charttemplate_test.go`: default install renders
   no Pipeline from the bundle; demo renders exactly `k8s-observe`;
   `rbacMode: full` renders exactly `k8s-operate`; both explicit renders both;
   `pipelines.enabled: false` under demo renders none; channels appear only when
   named.

**Rollback:** `k8s-bundle.pipelines.enabled: false` returns any install to the
previous behavior, and deleting the template removes the component entirely —
nothing else in the bundle references it.

## Open Questions

- **Should the bundle's route also claim a chat source when one exists?** It
  cannot: a chat source comes from `telegram-bundle`, which is a foreign object
  the subchart may only name from values. `pipelines.signalSources` extra names
  could be added later on the same values-supplied-and-omitted rule; left out
  because the cross-bundle route is exactly what the parent's `pipelines:` is
  for.
- **Does `vm-bundle` want the same treatment?** Its spec already describes a
  `defaultSource` Pipeline it does not render. Out of scope here, but the
  relaxed rule is what would let it.
