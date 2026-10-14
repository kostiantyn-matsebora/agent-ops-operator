## Context

See proposal.md - Why.

No workload in this chart renders `runtimeClassName` today. Three separate
reach mechanisms exist, and none of them share a helper:

| Mechanism | Covers |
|---|---|
| `AgentRuntime` CRD → `agentops.renderRuntime` → `internal/runtimepod/podspec.go` | conversation pods |
| `internal/controller/adapterworkload.go`'s `ensureAdapterWorkload` | console, every signal/channel adapter |
| Hand-rolled `PodSpec` per template, no shared helper | manager, housekeeping, context-probe, `mcp-aops`, 3 bundle MCP servers, Telegram router |

`platform/egress-proxy` intercepts egress with iptables NAT and a
conntrack-backed `SO_ORIGINAL_DST` lookup, not a userspace proxy redirect.
This is the one mechanism gVisor's virtualized netstack has historically had
partial support for. Its compatibility is unknown, not designed around here.

## Goals / Non-Goals

**Goals:**
- A cascading, frozen-at-creation `runtimeClassName` for conversation pods:
  capability fields (inline on the Pipeline or via `AgentCapability`)
  override `AgentRuntime`, which is the floor.
- A global-plus-per-component `runtimeClassName` default reaching every
  other chart-rendered workload, full coverage in this one change.
- Zero validation of the class name by the operator. A bad name fails at
  pod admission, the same behavior `serviceAccountName` already has.
- An early spike settling whether `egress-proxy` works under `runsc`,
  before that component's design is locked.
- CI automation: a real `runsc` RuntimeClass in the system-test cluster,
  exercising both component workloads and a full conversation.

**Non-Goals:**
- No gVisor-specific logic anywhere in the operator. `runsc` is a value an
  operator sets, never a name the code checks.
- No `tolerations`/`affinity` field. A gVisor node pool is commonly tainted,
  and scheduling onto it is left to the existing `nodeSelector` knob. This
  is a known gap, not solved here.
- No committed answer for `egress-proxy`'s final design. The spike decides
  it, and a follow-up may be needed.

## Decisions

### Two independent mechanisms, not one

The conversation-pod chain and the component-workload chain stay separate.

One resolves through CRDs and freezes onto a Conversation. The other is
pure Helm values, read once at render time. Forcing one model onto both
would either strip the conversation chain's override semantics, or invent
fake CRD plumbing for a static Deployment.

### `runtimeClassName` is frozen onto the Conversation, not live-read

Alternative considered: read `AgentRuntime.spec.runtimeClassName` live at
every pod rebuild, the way `image` and `idleTtlMinutes` already are.

Rejected. Isolation posture is a privilege decision, like
`serviceAccountName`, not an engine-content decision like `image`.

A re-wire mid-flight must never move an inflight pod's privilege. The
field resolves once, at conversation creation, onto
`Conversation.spec.runtimeClassName`.

### The component-workload chain gets one choke point plus seven one-off additions

`ensureAdapterWorkload` already builds console's and every signal/channel
adapter's workload in one function. One bootstrap-config value there
reaches six workloads for one code change.

The other seven workloads (manager, housekeeping, context-probe,
`mcp-aops`, 3 bundle MCP servers, Telegram router) share no helper today.
Alternative considered: build a new shared Helm template for all of them.

Rejected for this change. These templates already differ in shape
(Deployment vs CronJob, parent chart vs sub-chart).

Building a new abstraction to unify them is a larger, separate refactor
with its own risk. Each gets its own `values.yaml` key and template block
instead, mirroring the existing `nodeSelector` block where one already
exists.

### No RuntimeClass existence validation

Matches `serviceAccountName`'s "naming is not creating" precedent. The
operator never calls the API to check a `RuntimeClass` exists. A missing
one fails at pod admission, and that failure is the cluster's to diagnose.

### CI installs the `runsc` shim inside the existing k3d node

Alternative considered: a separate, dedicated VM or managed-cloud node pool
for gVisor validation, outside CI.

Not chosen. Automated regression coverage was the explicit requirement.

The existing k3d node is itself a Docker container. This means installing
`containerd-shim-runsc-v1` inside it, and patching k3s's bundled
containerd config template to register the `runsc` handler.

gVisor's default platforms (`ptrace`, `systrap`) need no nested
virtualization, so this is plausible on a stock GitHub-hosted runner. It
is still new plumbing, with no precedent in this codebase.

## Risks / Trade-offs

| Risk | Mitigation |
|---|---|
| `egress-proxy`'s iptables+conntrack mechanism may not work at all under `runsc`'s virtualized netstack | The spike runs first, before any `egress-proxy` code change is designed. The outcome is documented either way, never assumed |
| `context-sync`'s gofer-mediated rename/link/hardlink path has version- and CSI-dependent edge cases (hardlink support, mtime precision) | An explicit smoke-test task, not an assumption of correctness |
| Installing a `runsc` shim inside a k3d node that is itself a Docker container is new, unproven plumbing | Isolate it as its own system-test lane, so flakiness while it is tuned does not block the rest of the pack — the same tolerance the system-test tier already states for itself |
| Seven hand-rolled templates with no shared helper raises the per-template review surface | Each addition stays minimal: one `values.yaml` key, one template block, mirroring the existing `nodeSelector` pattern exactly |
| No existence validation means a missing `RuntimeClass` surfaces only as a stuck pod at admission time | Documented behavior, consistent with how an unknown `serviceAccountName` already behaves today |

## Migration Plan

Every new field is optional and additive. An install that sets none of them
renders exactly as it does today, no `runtimeClassName` anywhere, cluster
default kernel.

1. Land CRD and chart changes together with a chart version bump.
2. `helm upgrade` picks up the new optional values with no data migration.
3. Rollback is also field-removal only. Unsetting a value reverts a
   workload to the cluster default on the next reconcile or render.

## Open Questions

- **Does `egress-proxy` work under `runsc`?** Resolved by the spike task.
  Either outcome is compatible with the rest of this design. Only
  `egress-proxy`'s own follow-up and the `runtime-egress-mediation` spec
  delta depend on the answer.
- **The exact bootstrap-config name** for `ensureAdapterWorkload`'s new
  value (an env var alongside the existing `CONTEXT_SYNC_IMAGE`-style
  config). A naming detail, left to implementation.
