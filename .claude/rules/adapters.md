## Adapters (binding)

**Channel adapter** — out-of-process, consumes `/channel/*` (ops long-poll
plus inbound push). **Signal adapter** — inbound-only, no ops queue, pushes
to `/signal/inbound`.

| Field/Concept | Is |
|---|---|
| `Channel.spec` metadata | type-agnostic (`adapter`, `credentialsSecretRef`), NO wiring/delivery mode |
| `Channel.spec.config` / `status.threadId` | opaque to the manager / opaque STRING |
| Body grammar | markdown + block grammar, manager parses NEITHER |
| Parsed types | `answer`/`notice` only — `signal` is a CARD (own renderer) |
| Section vocabulary | OPEN, generic — never pinned to one agent |
| Recognition rules | `structured-agent-output`, shared by `blocks.go`+`blocks.ts` |
| `readAt`/`.readTracked` | manager-written on `POST /channel/read`, MONOTONIC+CLAMPED, no-advance = `skipped`, ≤50/batch. Missing = READ by default |
| `ChannelAdapter`/`SignalAdapter` CR | pure: `image` + workload knobs, NO `type`/`env`. `configSchema`/`credentialKeys` are metadata only |
| Workload/Service name | `agentops-adapter-<name>` / `agentops-signal-<name>` |
| `spec.serviceAccountName` | REFERENCE — naming MOUNTS its token. Absent = FLOOR, bound to nothing, no reconciler-created RBAC |
| Channel credentials | per-surface `envFrom`, prefix `AGENTOPS_CRED_<CHANNEL>_` |
| Signal grouping/cooldown | MANAGER-side — adapters only normalize. NO built-in signal types |
| `SignalAdapter.spec.port`/`.serviceAccountName` | Service+`LISTEN_ADDR` when set / self-registering senders' push-target binding |

**The CR NAME is the ROUTING KEY** — `Channel`/`SignalSource.spec.adapter`
is a REFERENCE, never an attribute. It drives `?adapter=`, `ADAPTER_NAME`,
credential projection, token scope and `Served`. One adapter per
implementation by construction. Adapter CRs carry NO configuration —
connectivity, credentials and config live only on Channel/SignalSource.
