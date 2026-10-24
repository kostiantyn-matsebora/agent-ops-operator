## Wiring (binding)

**`Pipeline` is THE wiring:** sources[] × channels[] + profile + tool access.
`Coordinator` is the only other wiring-carrying kind (sources[], channels[],
`agents[]` fan-out — see `terminology.md`). SignalSource/Channel carry none.

| Rule | Means |
|---|---|
| `global.agentops.wiringMode` | a CHART RENDER CHOICE only, never CRD exclusivity. A Pipeline and a Coordinator may claim the same source |
| Sources/channels SHAREABLE | any Ready Pipeline may list one, no conflict, no tiebreak (deleted) |
| Per-source policy | evaluated ONCE above the fan-out, or the first Pipeline starves the rest |
| Unwired source | `Wired=False` + reason, relayed to a CHAT surface |
| Bare chat, 1 Ready Pipeline | routes |
| Bare chat, several | choice list + `/<pipeline> <task>` form (the feature) |
| Bare chat, none | unwired drop |
| CLAIM (`signalSourceRefs`) | who answers unaddressed, from Ready pipelines only |
| ADDRESSING (`/<pipeline> <task>`) | plain `Get` by name, no claim/Ready check |
| Reached | only by a claimed signal or `/<pipeline>` — no HTTP form names one |

Capabilities (`AgentCapability`, or inline on Pipeline/Coordinator) carry
`spec.toolsets` → `MCPToolset` and `spec.mcpConfigs` → `MCPConfig`, applied
in order, dedup, later wins, composed against the agent definition's
`tools:` — never the profile, which carries none.

**MANY-TO-MANY IN EVERY DIRECTION IS THE MODEL.** No exclusivity, no
tiebreak — "prefer a source of its own" is wrong advice, deleted on sight.

### Execution, identity, storage — also wiring

| Field | Selects | Absent |
|---|---|---|
| `spec.runtimeRef` | `AgentRuntime` | the one named `default` |
| `spec.serviceAccountName` | identity, overriding the runtime's | FLOOR, bound to NOTHING |
| `spec.persistence` | `context`/`workspace` claims | release default, then EPHEMERAL |

**SILENCE MEANS NO POWER.** Tools and identity are the SAME decision.
Split across two objects, no reviewer can approve an agent's power.

**PRECEDENCE:**

| | Chain |
|---|---|
| runtime | `conversation` → `profile` (deprecated) → `AgentRuntime/default` → bootstrap |
| identity | `conversation` → `runtime` → FLOOR |
| storage | `conversation` claims → chart default → EPHEMERAL |

The PIPELINE is in NEITHER chain — fields RESOLVE INTO the conversation at
creation, so a later edit never re-wires an inflight one.

Naming an SA is not creating one, and `Ready` doesn't check it (no RBAC
for that read) — a bad name fails at pod admission instead. A bundle
renders the accounts its own routes need. The substrate (runtime,
credential, floor account) stays the parent's.

### Persistence claims

| Declared on | Renders the claim |
|---|---|
| a Pipeline | the MANAGER |
| the chart | the CHART, same derived name `agentops-<pipeline>-<volume>` |

Mutually exclusive, CEL-enforced — the one place "naming is not creating"
doesn't hold. No ownerRef, no `delete` verb on the manager's side, since
deleting a Pipeline must never delete accumulated context.

Storage class is an EXPLICIT empty string, never absent. Lives here, not
the runtime, since two Pipelines sharing one engine must persist to
different volumes.

### RBAC posture

No binding uses a built-in role. Grants are CLUSTER-WIDE, and omission is
what makes that safe — `agentops.dev`, `secrets`, `clusterroles` NEVER
granted (namespaced Roles were tried and reverted as worse).

`allowPodExecution` is the SECRETS boundary, default OFF. The kubelet
resolves a Secret building a pod, so a pod-creating/exec'ing agent reads
it without ever calling the API.

`--allowedTools` configures a cooperating agent only — the SA binding is
what an uncooperative one actually has, and the manager itself holds no
`secrets` verbs at all.

### `MCPToolset`

A pure list of tool patterns, read-only RBAC, no servers (those live in
`MCPConfig`). The pipeline binds HALF the allowlist — the RUNTIME composes
it with the agent definition's own `tools:`.

`--allowedTools` is the sole permission authority. Never pass `--agent
<name>` (defeats `overwrite`). No `|| 'Read'` fallback — empty passes as
empty, `--permission-mode dontAsk`.

**Multi-channel delivery is the OPERATOR's job**, not the agent's: it fans
replies to every bound thread, delivers a message to every bound channel
except the one that displayed it, and dispatches once ≥1 binding exists.
