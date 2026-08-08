# k8s-bundle — design

## Context

Today `chart/templates/demo.yaml` bundles, behind `demo.enabled`, a demo-suffixed runtime SA, an `AgentRuntime` named `default` (claude image + LLM credential env), the `k8s-engineer` AgentProfile, and read-only RBAC (`view` binding + a nodes/namespaces/metrics ClusterRole) gated on `demo.readOnlyRbac`. The merged `k8s-events-signal-source` change designed a k8s-events signal adapter but had no profile to point its wiring at, because the chart ships no profile outside demo mode.

The identity chain the RBAC rides on: `AgentProfile.spec.runtimeRef` → `AgentRuntime.spec.serviceAccountName` → that SA's RBAC *is* the agent's power (`rbac.runtime` comment in values.yaml). A profile with no `runtimeRef` falls back to the AgentRuntime named `default`.

This change is a merge: everything from `k8s-events-signal-source` (adapter module, normalization/cursor design) plus the new subchart packaging. The merged change's standalone chart plan (`k8sEventsAdapter` parent-values block) is superseded by the bundle.

**Landed since this change was planned** (all three archived, and this plan is written against them): `spec.type` → `spec.adapter` on Channel/SignalSource, where the adapter CR's NAME is the routing key and there are no built-in signal-type constants; pipeline-only wiring, so a SignalSource carries no `profileRef` and an unclaimed source drops signals with `Wired=False`; and `SignalAdapterSpec.kubernetesAccess`, which is exactly the SA-token opt-in this change originally proposed as `automountServiceAccountToken`. The API, controller, and lifecycle-spec work this change once carried is therefore already shipped — see D4.

## Goals / Non-Goals

**Goals:**

- One subchart, three independently toggleable components: events signal source, k8s-engineer profile (with runtime + SA), RBAC (readonly default / full opt-in / off).
- Bundle off by default; demo mode enables it as-is (read-only RBAC), replacing `demo.yaml` without losing any current demo behavior.
- Self-consistent when fully enabled: the events SignalSource references the bundle's own profile — no external prerequisites beyond the LLM credential Secret.
- Carry the merged change's scope intact (adapter module, API opt-in, controller threading).

**Non-Goals:**

- No new manager behavior beyond the merged change's automount threading; grouping/cooldown, Secret-free manager, and the `/signal/*` contract are untouched.
- No multi-namespace runtime placement, no Ingress, no alternate RBAC vocabularies beyond readonly/full (custom rules remain the parent `rbac.runtime` mechanism).
- Not preserving the `demo.*` values paths (breaking rename, documented).

## Decisions

### D1: Embedded subchart, self-gated templates, `global.demo.enabled`

The bundle lives at `chart/charts/k8s-bundle/` (embedded subchart — auto-included, no `dependencies:` entry, no Helm `condition:`). Every template gates itself on `(or .Values.enabled .Values.global.demo.enabled)`. Rationale: Helm's `condition:` evaluates only the first existing values path, so with `k8s-bundle.enabled: false` present in defaults, demo mode could never flip the subchart on via condition — self-gating is the only mechanism that expresses "A or B". Demo's toggle must therefore be visible to the subchart, which only `global.*` provides: the parent moves the flag to `global.demo.enabled` (**BREAKING** rename from `demo.enabled`). Demo does not force values — it enables the bundle with the bundle's defaults, and read-only is the default RBAC mode, which satisfies "demo = readonly" while still letting an operator explicitly override any bundle value alongside demo mode.

### D2: Component decomposition and flags

Subchart values (defaults shown; parent `values.yaml` carries a commented `k8s-bundle:` override block for discoverability):

```yaml
enabled: false            # whole bundle; demo mode ORs this on
eventsAdapter:
  enabled: true           # component flags apply when the bundle is active
  name: k8s-events        # SignalAdapter CR name = the routing key sources select
  image: { repository: kmatsebora/agentops-signal-k8s-events, tag: ... }
  resources: {}
  rbac: { create: true, clusterWide: true }   # events get/list/watch for the adapter SA
  source:
    create: true          # renders the SignalSource AND the Pipeline claiming it
    name: k8s-events
    profileRef: ""        # empty = the bundle's profile.name (rendered on the Pipeline)
    channels: []          # optional Channel names the Pipeline mirrors to
    severities: ["Warning"]
    namespaces: []
    grouping: {}          # optional SignalSource.spec.grouping overrides
profile:
  enabled: true
  name: k8s-engineer
  allowedTools: "Read,Grep,Glob,Bash"
  maxTurns: 40
  runtime:
    create: true
    name: default         # preserves today's demo fallback semantics
    image: kmatsebora/agentops-runtime-claude:0.1.1
    credentialsSecret: { name: agentops-claude, key: oauthToken, envName: CLAUDE_CODE_OAUTH_TOKEN }
    serviceAccountName: agentops-runtime-k8s
rbac:
  enabled: true
  mode: readonly          # readonly | full
```

- `eventsAdapter` renders the `SignalAdapter` CR (named `k8s-events` — SignalSources select it with `spec.adapter`, `kubernetesAccess: true`, singleton), the events RBAC bound to the deterministic adapter SA `agentops-signal-<name>`, and (when `source.create`) BOTH a `SignalSource` naming that adapter and the `Pipeline` claiming it, whose `profileRef` defaults to `profile.name` — in-bundle, so the merged change's profile chicken-and-egg disappears. The pair is rendered together on purpose: wiring is pipeline-only, so a source without a claiming pipeline is inert (`Wired=False`, signals dropped). This mirrors vm-bundle's `defaultSource`, which already renders exactly this SignalSource + Pipeline pair. Disabling `profile` while keeping the source requires pointing `source.profileRef` at an existing profile (values-overridable); the template fails render with a clear message if `source.create` is on and no profile name resolves.
- `profile` renders the AgentProfile, the runtime SA, and (when `runtime.create`) the AgentRuntime with `serviceAccountName` set to that SA and the credential env wired exactly as demo.yaml does today. `runtime.create: false` + `runtimeRef` values support operators with their own runtimes.
- `rbac` renders bindings for `profile.runtime.serviceAccountName`: `readonly` = ClusterRoleBinding to built-in `view` + the nodes/namespaces/metrics ClusterRole (verbatim today's demo RBAC); `full` = ClusterRoleBinding to **cluster-admin**. `enabled: false` = no bindings (the SA can do nothing in-cluster; profile still answers from general knowledge).

### D3: demo.yaml is deleted, not gated

With the bundle rendering a superset of demo.yaml (same SA/RBAC shape, same runtime `default`, same profile), keeping both would double-create the AgentRuntime `default` and the profile. `templates/demo.yaml` is removed; `global.demo.enabled` becomes purely "turn the bundle on". The demo-specific values (`runtimeImage`, `credentialsSecret`, `readOnlyRbac`) migrate to `k8s-bundle.profile.runtime.*` / `k8s-bundle.rbac.mode`. SA naming changes from `agentops-runtime-demo` to `agentops-runtime-k8s` — a fresh identity; stale demo RBAC objects from old releases are cleaned by helm upgrade removing demo.yaml's render.

### D4: Carried over from the merged change — what is left, and what already shipped

The `k8s-events-signal-source` change directory is retired. Its **API and controller half is already in `master`** and is NOT re-implemented here:

- The SA-token opt-in shipped as `SignalAdapterSpec.kubernetesAccess` (`adapter-sender-self-registration`), which mounts the token *and* injects `POD_NAMESPACE` — a superset of the proposed `automountServiceAccountToken`. Threading (`adapterworkload.go`, `signaladapter_controller.go`) and the envtest coverage (opt-in, default-false, ChannelAdapter byte-identity) all exist. The bundle just sets `kubernetesAccess: true`.
- The proposed `SourceK8sEvents = "k8sEvents"` constant is **dropped, not deferred**: `rename-type-to-adapter` removed `spec.type` in favour of `spec.adapter` naming the adapter CR, and the operator hosts no built-in signal types. Adding the constant would reintroduce the model that change deleted.
- The `signal-adapter-lifecycle` spec delta is likewise dropped — the shipped main spec already states the requirement in richer form (`kubernetesAccess`, `POD_NAMESPACE`, `ADAPTER_NAME`, the `spec.port` Service).

What remains for this change is the adapter module and the packaging:

- **Dependency-free watcher**: `signal-k8s-events/` module with an in-cluster client built on `net/http` (token file re-read for rotation, CA pool, `KUBERNETES_SERVICE_HOST/PORT`), core `v1` Events list+watch per namespace scope, client-side severity/reason filtering, relist on `410 Gone`.
- **Normalization**: `kind: alert`; fingerprint `<source>@<ns>/<kind>/<name>/<reason>`; labels `alertgroup: k8s-events`, `alertname: <reason>`, `namespace`, `kind`, `name`, `severity`, `source`; title `<reason>: <kind>/<name>`; payload = message + count/timestamps. No adapter-side grouping (manager groups).
- **Cursor**: max `lastTimestamp` persisted via the contract state API; startup list skips ≤ cursor; at-least-once, collapsed by deterministic fingerprints + manager cooldown.
- **Config schema**: `severities` (default `["Warning"]`), `namespaces`, `includeReasons`/`excludeReasons`; invalid config → `Ready=False` `InvalidConfig` via status API, other sources keep serving. Declaring it as the adapter CR's `spec.configSchema` is free discoverability and follows `signal-cron`.

### D5: Demo cost containment

Demo mode now includes the events adapter, so a noisy demo cluster auto-creates conversations that consume LLM credits — previously demo only answered explicit `/task` calls. Accepted with mitigations: default severities `["Warning"]` only, manager-side fingerprint cooldown (6h) and signature grouping bound conversation volume, and `k8s-bundle.eventsAdapter.enabled=false` (or `source.create=false`) alongside demo mode restores the old ask-only behavior. README states this explicitly in the demo section.

## Risks / Trade-offs

- [`mode: full` binds cluster-admin to an LLM-driven agent] → Opt-in only, never a default, never demo; README carries a prominent warning and recommends readonly + targeted parent `rbac.runtime` grants instead. The value name `full` (not `admin`) still means exactly cluster-admin — documented.
- [Breaking values rename strands existing demo users] → Major chart version bump; README migration table (`demo.enabled` → `global.demo.enabled`, etc.); `helm upgrade` with old values fails visibly at render (unknown-path lint note) rather than silently no-oping demo.
- [Subchart can't see parent `serviceAccounts.runtime` naming convention] → The bundle's SA name is a literal value (`agentops-runtime-k8s`) rather than derived from the parent value; acceptable since it's overridable and self-contained.
- [Cross-component references break when components are individually disabled] → Each cross-reference (Pipeline `profileRef` → profile, rbac → runtime SA) is a values-resolvable name with render-time `fail` guards and documented combinations.
- [A rendered SignalSource with no claiming Pipeline is silently inert] → The events component renders the pair together and never the source alone; pinned by a `helm template` assertion in the verification matrix.

## Migration Plan

1. Implement the `signal-k8s-events/` module first — a new module and image, inert until a SignalAdapter CR names it. No API or controller work is needed (D4).
2. Add the subchart + parent values rewiring + demo.yaml deletion in one chart version (2.0.0).
3. Upgrade path for demo users: set `global.demo.enabled=true` (and any customized `demo.*` values under their new `k8s-bundle.*` paths); helm upgrade removes old demo objects (name changes: SA `agentops-runtime-demo` → `agentops-runtime-k8s`, RBAC object names follow) and creates bundle objects; the AgentRuntime `default` is re-rendered with identical semantics, so existing conversations keep resolving their runtime.
4. Rollback = revert to chart 1.x values; CRs created by the bundle are release-managed and removed on downgrade (CRDs themselves keep per `crds.keep`).

## Open Questions

- None blocking. Whether `rbac.mode: full` should also grant the events adapter cluster-wide reach when `eventsAdapter.rbac.clusterWide` is false is intentionally not coupled — the two RBAC scopes stay independent.
