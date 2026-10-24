## Terminology (binding)

| Retired | Use instead | Why (one clause) |
|---|---|---|
| "agent" for what a message addresses | `Pipeline` (`/pipelines`) | "agent" names a `.claude/agents/` DEFINITION, capability CRD is `AgentCapability` |
| a signal WAKES / woke / waking an agent | a signal STARTS or OPENS a conversation | nothing is resident. "Wake" stays correct for paging a PERSON |
| "worker" | Agent runtime — CRD `AgentRuntime`, SA `agentops-runtime`, env `RUNTIME_*`, pkg `runtimepod`, pods `agentops-conv-<conversation>` | — |
| "session" for the runtime's context handle | `runtimeContextId` | vendor noun — the manager stores/returns it, interprets nothing |
| "home volume" | the CONTEXT volume — `persistence.context`, claim `agentops-context`, env `CONTEXT_PVC`, mount `/data/context` | `home` named a mount path, not what the volume holds |
| `AgentRuntime.spec.home`/`.context`/`.workspace` | `Pipeline.spec.persistence` | volume is WIRING, not runtime content |
| "manager" for the context sidecar | `context-sync` | "manager" already names the operator |
| six-lane topology column graph | Model / Components / Infrastructure VIEWS of one activity feed | "lane" keeps its other meaning: a SignalSource |
| `AgentProfile` selecting execution/capabilities | the Pipeline routing it | profile = BEHAVIOUR, Pipeline = REACH |
| write-once `runtimeContextId` | LATEST-WINS | a run may end in a different context than it was asked to continue |
| `spec.runtimeRef` on the profile | `Pipeline.spec.runtimeRef` (deprecated on profile) | an `AgentRuntime` carries the SA, so choosing one is choosing cluster power |
| "identity only" for what a profile lacks | REACH | the system prompt IS the agent's judgement, not a label |

### The six conversation-shaped kinds

| Kind | Is |
|---|---|
| `AgentProfile` | who the agent is, how it behaves — repo, role, prompts, limits. Selects no execution |
| `AgentRuntime` | what executes it — image, pod defaults, `spec.contextStorage`. No volume: persistence is wiring |
| `Conversation` | session + serial input queue + one thread per bound channel |
| `Pipeline` | the wiring — see `wiring.md` |
| `AgentCapability` | the capability, standalone — same fields a Pipeline/Coordinator may embed inline. Unwired is inert |
| `Coordinator` | wiring for a composition — `agents[]` of `capabilityRef`/`coordinatorRef`, plus its own capability |

**REFS are snapshotted onto the Conversation, CONTENT is not** — a
re-wire mid-flight must never move an inflight pod's privilege.

### `Coordinator`: root, member, escalate

| Word | Names |
|---|---|
| root | the UNCAUSED conversation a claim or `/<name>` opens — no `spec.causedBy` |
| member | one `invoke` created, `causedBy` naming its PARENT, one hop, never the root |
| escalate | the verb reaching a HUMAN channel — only the root ever opens one. On a member it closes the member instead, bubbling the result upward |

A member may itself be a root (nesting to any depth). Only a repeated
Coordinator on one `causedBy` chain — a CYCLE — is refused. A caused
conversation binds no human channel of its own (`invariants.md`).

**`spec.pipelineRef` is PROVENANCE, never wiring** — written once, read
for REUSE scoping and display ATTRIBUTION only.

### Context continuity

| Case | Behaviour |
|---|---|
| never-promised | answer fresh, say so |
| promised-and-lost | FAIL the run — context-less is a new conversation wearing the old name |
| unavailable | an OUTAGE before a LOSS — bounded retry, then a manager-side breaker holds work |

### `/data/workspace` never moves. `/data/home` → `/data/context` did

| Path | Moves? | Because |
|---|---|---|
| `/data/workspace` | NEVER | the stored transcript dir is NAMED for it (`.claude/projects/-data-workspace/`) |
| `/data/home` → `/data/context` | it moved | nothing inside is named for `$HOME` — claude-code keys context by the WORKING directory, not `$HOME` |

`contextClaimRenameGuard` fails an unremarked render rather than trust a
migration note.

### `context-sync`

The sidecar keeping a runtime's live context on pod-local storage and a
snapshot on the durable volume. No durable-volume mount in the agent
container. Checkpoints are conditional AND incremental.

### API group

`agentops.dev/v1alpha1`. Provisional — a rename is possible pre-1.0.
