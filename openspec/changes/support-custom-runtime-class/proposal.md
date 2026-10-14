## Why

Kernel-level sandboxing (gVisor's `runsc`, the de-facto standard for
isolating untrusted workloads on Kubernetes) is unreachable in this chart
today. Nothing anywhere renders `runtimeClassName`.

An agent runtime executes model-directed shell commands. A cluster operator
who wants stronger isolation than RBAC and the egress wall provide today has
no knob to turn.

## What Changes

- Add a generic, vendor-neutral `runtimeClassName` passthrough. The operator
  never validates the name and never knows what gVisor is. A bad name fails
  at pod admission, exactly like an unknown `serviceAccountName` does today.
- **Conversation-pod chain**:
  - `AgentRuntime.spec.runtimeClassName` is the floor.
  - `AgentCapability.spec.runtimeClassName` overrides it. Available inline on
    Pipeline and Coordinator, and through `capabilityRef`/`coordinatorRef`
    for Coordinator members, with no new field needed there.
  - Latest wins. Resolved once and **frozen onto
    `Conversation.spec.runtimeClassName` at creation** — same reasoning and
    timing as `serviceAccountName`. A later re-wire must never move an
    inflight pod's privilege.
- **Component-workload chain** (new capability, a separate mechanism): a
  `global.agentops.runtimeClassName` chart default, overridable per
  component, reaching every chart-rendered workload:
  - the manager, the console
  - every signal/channel adapter, through the single `ensureAdapterWorkload`
    choke point
  - housekeeping, the context-probe CronJob, `mcp-aops`
  - every bundle's own MCP server (`kubernetes`, `prometheus`,
    `home-assistant`), and the Telegram router

  No shared helper covers all of these today. Each hand-rolled template
  gains its own passthrough.
- **gVisor validation**: a spike proving, or disproving, that
  `platform/egress-proxy`'s iptables-NAT-plus-conntrack interception works
  under `runsc`'s virtualized netstack. Runs before that component's final
  design is locked. This change does not presuppose the answer.
  `platform/context-sync`'s rename/symlink/hardlink checkpoint path is
  smoke-tested too, lower risk but unverified.
- CI gains a `runsc` `RuntimeClass` in the system-test cluster, new
  containerd-shim plumbing inside the existing k3d node with no prior
  precedent. It exercises both component workloads and a full conversation
  (AgentRuntime pod, `context-sync` and `egress-proxy` sidecars included)
  under it.
- **BREAKING**: none. Every new field is optional and additive. Unset
  behaves exactly as today — no `runtimeClassName` rendered, cluster default
  kernel.

## Capabilities

### New Capabilities
- `component-runtime-class`: the global-plus-per-component `runtimeClassName`
  chart knob reaching every chart-rendered workload outside the
  AgentRuntime/Conversation chain — console, every signal/channel adapter,
  manager, housekeeping, context-probe, `mcp-aops`, the three bundle MCP
  servers, and the Telegram router.

### Modified Capabilities
- `runtime-declaration`: the runtime defaults block and each declared
  `AgentRuntime` gain `runtimeClassName` as a stated, visible default,
  following the same "defaults are a complete runtime" requirement that
  already governs `nodeSelector`/`resources`.
- `agent-capability-model`: the capability carries a seventh field,
  `runtimeClassName`, overriding the runtime's own. The existing "identity
  and storage snapshot" requirement extends to cover this new field, frozen
  onto the Conversation at creation on the same terms as `serviceAccountName`.
- `pipeline-model`: the Pipeline's "what executes its conversations and
  under whose identity" fields grow from two (`runtimeRef`,
  `serviceAccountName`) to three, adding `runtimeClassName`, with the same
  absent-means-inherit semantics.
- `coordinator-model`: its own requirement names the capability field count
  explicitly (`AgentCapabilitySpec` embedded inline), so the bump from six
  to seven needs a one-line delta there too, even though no code changes —
  see "Not impacted" below for what genuinely needs none.
- `security-posture-page`: the site's threat-model page gains a requirement
  to document `runtimeClassName` as an available kernel-isolation control.
  It must name exactly what has been validated, and any known limitation
  such as `egress-proxy`'s compatibility status, rather than claim blanket
  gVisor support.
- `end-to-end-testing`: the system pack gains a requirement to provision a
  `runsc` RuntimeClass and verify both component workloads and a
  conversation (AgentRuntime) pod run under it. `egress-proxy`'s behavior
  under it must be explicitly asserted or explicitly documented as skipped,
  never silently green.

## Impact

**Code**:
- `platform/manager/api/v1alpha1/` — `AgentRuntimeSpec.RuntimeClassName`,
  `AgentCapabilitySpec.RuntimeClassName`,
  `Conversation.Spec.RuntimeClassName`, plus deepcopy and CRD regen for
  `agentruntimes`, `agentcapabilities`, `pipelines`, `coordinators`,
  `conversations`.
- `internal/runtimepod/podspec.go` — `Config.RuntimeClassName`, the
  `FromRuntime` overlay, `ResolveFor`'s conversation-snapshot overlay, and
  the `corev1.PodSpec` literal in `Build`.
- `internal/controller/adapterworkload.go` — `ensureAdapterWorkload` gains a
  manager-bootstrap-sourced `runtimeClassName`, reaching console and every
  signal/channel adapter workload in one change.
- `chart/templates/_helpers.tpl`, `chart/values.yaml` — `runtimeDefaults`
  grows the key. `agentops.renderRuntime` renders it.
  `agentops.mergedRuntime` already generalizes the per-runtime-bundle
  override.
- `chart/templates/deployment.yaml`, `housekeeping.yaml`,
  `context-probe.yaml`, `coordination.yaml`, and
  `chart/charts/{kubernetes,prometheus,home-assistant}/templates/mcp-server*.yaml`,
  `chart/charts/telegram/templates/router.yaml` — each gains its own
  `values.yaml` key and template block. No shared helper exists for these.
- `platform/manager/test/system/cluster.go` and the system-test workflow —
  new `runsc` shim provisioning and `RuntimeClass` object, plus new
  scenarios.
- `platform/egress-proxy/` — pending the spike's result. Either no code
  change (documented incompatibility), or a follow-up design for an
  alternative interception mechanism.

**Reference docs** — both of these are made untrue by this change:
- `docs/concepts.md` — new fields on the AgentRuntime/AgentCapability/
  Pipeline field tables, and a new subsection under "Execution is wiring
  too" alongside the existing "One runtime image, two trust levels".
- `docs/configuration.md` — the new global-plus-per-component chart values.
  Reversible, not install-time-only.
- `docs/security.md` — a new control/trust-boundary entry for kernel-level
  isolation, stating the validated scope honestly, including the
  `egress-proxy` spike's outcome. Re-run `threat-model.py` — this change
  strengthens a trust boundary.
- `.claude/rules/wiring.md` — the `runtime`/`identity` precedence tables
  gain the `runtimeClassName` chain, kept in lockstep with
  `docs/concepts.md`'s "Resolution order, stated" section.

**Adopter site** — both of these are made untrue by this change:
- A new guide under `docs/guides/` walking through enabling a custom
  runtime class, gVisor included. Hand-written prose plus generated
  chart-value blocks.
- `docs/security.md`'s adopter-facing framing (the same file serves both
  reference and adopter-site roles here) gains the
  gVisor-as-validated-control narrative.
- `README.md` — at most a one-line pointer to the new guide and security
  entry, per its "mirrors the landing page" budget. No restated detail.

**Not impacted**:
`coordinator-model`'s `CoordinatorAgentEntry` member resolution (a
`capabilityRef` or nested `coordinatorRef`) needs no delta. It resolves
through `agent-capability-model`'s shape without restating the field list.

The new field and its freeze-at-creation behavior flow through unchanged.
Only the one line counting the coordinating agent's OWN inline capability
fields needed the matching bump, which has its own delta above.

`runtime-egress-mediation`'s own spec is deliberately left unmodified by
this proposal. Its final behavior under a non-default `runtimeClassName` is
an open question the spike resolves (see design.md). Its own spec delta is
expected as a follow-up once that result is known.
