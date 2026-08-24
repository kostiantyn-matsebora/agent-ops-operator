## Terminology (binding)

### A Pipeline is what a message ADDRESSES, never "an agent"

- **The listing command is `/pipelines`.** `/agents` still answers and is never
  printed, offered or registered — a published word cannot simply stop working,
  but nobody should learn it from us again.
- **"Agent" is TAKEN.** It names a DEFINITION in `.claude/agents/` inside a
  profile's repository, which is what `AgentProfile.spec.agent` selects. Two
  meanings on one word, and the more visible one was wrong.
- **The word is carved into every install's composer.** `internal/chat`
  publishes the vocabulary a transport registers as its command menu, which is
  why this had to be right BEFORE that shipped.
- **`pipelines` joins the reserved set** a Pipeline cannot be reached by.

### A signal STARTS a conversation. NOTHING IS ASLEEP

**"Wake", "woke" and "waking" are BANNED for what a signal does.** No agent is
sitting there waiting to be roused: a signal arrives, the manager OPENS a
Conversation, and only then is a runtime pod provisioned.

The verbs, and they are the ones the invariants already use:

| Instead of | Write |
|---|---|
| a signal WAKES an agent | a signal STARTS a conversation, or OPENS one |
| what WAKES an agent | what STARTS a conversation |
| the route that WOKE it | the route that STARTED it |
| a source that wakes nothing | a source no Pipeline claims |

- **"Wake" is correct for a PERSON.** Paging someone at 03:00 really does wake
  them, so `k8s-bundle`'s "what actually deserves waking someone" stays.
- **It reads as a lie about the architecture**, which is why it is banned rather
  than discouraged. It implies a resident agent and a cheap nudge, when what
  happens is an object being created and a pod being scheduled.
- **It had spread to the landing page, the Introduction, three bundle pages,
  four rules files, the chart values and the DRAWN DIAGRAM** before anyone swept
  it. Fixing the diagram meant editing `agent-ops.drawio` and re-running
  `docs/diagrams/export.py`, because the word was baked into all four SVGs.

### Agent runtime, never "worker"

CRD `AgentRuntime`, SA `agentops-runtime`, env `RUNTIME_*`, pkg `runtimepod`,
pods `agentops-conv-<conversation>`.

### The four conversation-shaped kinds

| Kind | Is |
|---|---|
| `AgentProfile` | **who the agent is and HOW IT BEHAVES** — repo, role, prompts, env, limits. **NOT what executes it**: `spec.runtimeRef` is DEPRECATED and moved to the Pipeline |
| `AgentRuntime` | **what executes it** — an ENGINE. Image and pod-level defaults, plus `spec.contextStorage`. **It declares NO VOLUME**: persistence is wiring |
| `Conversation` | **session + serial input queue + one thread PER bound channel** (`spec.channelRefs[]` / `status.threads[]{channel,threadId}`) |
| `Pipeline` | **the wiring** — see below |

**`AgentProfile` carries NO capabilities.** No `allowedTools`, no `mcp`. What an
agent MAY DO comes exclusively from the Pipeline routing it.

**AND IT SELECTS NO EXECUTION.** `spec.runtimeRef` is deprecated and dual-read
for one release. An `AgentRuntime` carries the ServiceAccount an agent runs as,
so a profile choosing one chose the agent's POWER IN THE CLUSTER — see
`wiring.md` for why that failed the symmetry test.

**BUT "IDENTITY ONLY" IS THE WRONG SHORTHAND FOR THAT, AND IT IS DELETED.** The
split is BEHAVIOUR against REACH, not identity against everything else.

- **The system prompt is the whole of the agent's judgement** — how it decides,
  what it must never do, its method. That is not a label, and calling the object
  that holds it "identity only" reads as thin when it is the substance.
- **What the profile lacks is REACH**: tools, MCP servers, channels. Say that,
  because that is the claim which is load-bearing.
- **Two profiles over one runtime are two different agents.** A phrase implying
  a profile is a name and a role makes that sound impossible.

**AND IT DECLARES NO STORAGE EITHER.** `spec.home`, `spec.context` and
`spec.workspace` are DELETED, with no alias — the concept moved to
`Pipeline.spec.persistence`, so there is nothing on this object for an alias to
point at. Two Pipelines sharing one runtime must be able to persist to different
volumes without cloning it, which is the same failure a second trust level had.

**`Conversation.spec.toolsets` / `.mcpConfigs` / `.runtimeRef` /
`.serviceAccountName` / `.contextClaimName` / `.workspaceClaimName` mirror the
originating Pipeline's bindings.** MATERIALIZED state like `profileRef` /
`channelRefs`, never hand-set.

**THE IDENTITY SNAPSHOT IS THE SHARPEST CASE OF THE REFS/CONTENT RULE.** Without
it, editing a Pipeline changes what account an INFLIGHT conversation's next pod
runs as — a privilege change applied to work already in progress.

**THE STORAGE SNAPSHOT IS SHARPER STILL**, and it is frozen RESOLVED because
there is no runtime CONTENT below it to heal from. A re-wiring would point an
INFLIGHT conversation's next pod at a different disk — work that has ALREADY
WRITTEN to the old one, coming back to an empty volume and reporting success.

- **The RUNTIME NAME is snapshotted RESOLVED**, so a conversation created while
  its Pipeline named none keeps the one it actually ran on.
- **The SERVICE ACCOUNT is snapshotted ONLY where the PIPELINE named one.** A
  Pipeline's account is wiring and is frozen; an `AgentRuntime`'s own account is
  that runtime's CONTENT, so correcting a mistyped one must heal conversations
  already created. Empty is safe because resolution never reads a Pipeline.

**REFS are snapshotted, CONTENT is not.** Every use re-reads the CRs, so edits
heal running conversations while re-wiring affects only new ones.

#### `spec.pipelineRef` is PROVENANCE, NEVER WIRING

Written once at creation, and read for exactly two things:

1. Scoping conversation REUSE.
2. ATTRIBUTION in displays.

- **Nothing resolves a profile, channel set or capability through it.** That is
  what keeps a Pipeline edit from re-wiring a running conversation, and
  resolving anything through it would undo the whole snapshot rule.
- **It exists because sources are SHAREABLE.** Two Pipelines listing one source
  open conversations with the SAME signature, so without it the second's next
  signal lands on the first's conversation under the wrong profile.
- **Conversations predating it carry none and nothing backfills them.** An empty
  ref is reusable only while ONE Ready Pipeline serves the source.
- **EVERY origination now has a Pipeline to mirror.** Signals of every kind —
  `alert`, `job`, `task`, `chat` — from the one claiming the source, and a
  `/<pipeline> <task>` chat command from the one it addresses. Nothing creates a
  Conversation without wiring behind it.

### `runtimeContextId`

**agent-ops' name for the RUNTIME's opaque handle on a conversation's
accumulated context.**

**NEVER "session".** That is claude-code's noun. Another backend calls it a
thread, another has none. A vendor's word in this API teaches the next reader
that the manager knows what is inside the handle, which it does not.

- **The manager stores it, hands it back on the next work unit, and interprets
  NOTHING.**
- **`--resume` is one runtime's implementation** and appears nowhere in the
  contract.

**LATEST-WINS.** It was write-once, which was unsound:

- A run may legitimately end in a different context than it was asked to
  continue, so the first handle then named something gone.
- Dispatch AND ingest both key off it, so every later message repeated the same
  failed continuation.
- One recoverable loss became permanent.

**`Conversation.ContextID()` is the only place the retired `sessionId` is
read.** Dual-read for one release: a rename that merely moved the field would
have stranded every in-flight handle on upgrade.

**Continuity is PROMISED ONLY WHERE POSSIBLE** —
`AgentRuntime.spec.contextStorage` (`volume` | `external` | `none`) against the
configured context volume.

| Case | Behaviour |
|---|---|
| never-promised | answer fresh, and say so |
| promised-and-lost | FAIL the run — a conversation without its context is a new one wearing its name |

**Unavailability is an OUTAGE before it is a LOSS.** Bounded retry in the
runtime, then a manager-side breaker that HOLDS work. Failing fast on every
report would destroy every active conversation's context in one storage
incident.

### The CONTEXT volume, never "the home volume"

**Chart `persistence.context`, claim `agentops-context`, bootstrap env
`CONTEXT_PVC`, Pipeline field `spec.persistence.context`, pod volume `context`,
MOUNT PATH `/data/context`, `HOME=/data/context`.**

The volume holds a conversation's ACCUMULATED CONTEXT — the thing
`runtimeContextId` is a handle into and `contextStorage` promises continuity on.
`home` named the filesystem path it happened to be mounted at, which is the same
mistake `session` and `worker` are banned for one heading up.

**`/data/workspace` IS THE LOAD-BEARING PATH, AND `/data/home` IS GONE.** This
page said the opposite, at length, and the wrong version is kept because it is
persuasive: both paths sit under `/data`, both are mounted from a claim, and
only one is baked into stored state.

| Path | Moves? | Because |
|---|---|---|
| `/data/workspace` | **NEVER** | the stored transcript directory is NAMED for it — `.claude/projects/-data-workspace/` — so relocating it strands every stored context |
| `/data/home` → `/data/context` | **it moved** | nothing inside the volume is named for `$HOME`, so nothing inside relocated |

- **MEASURED, NOT REASONED.** A live claim was mounted READ-ONLY at a third path
  and every stored generation resolved exactly as it does at its own mount: a
  claim's contents appear AT the mount path, and the only transcript directory
  in the volume is `-data-workspace`.
- **The earlier argument — "claude-code keys stored context by `$HOME`" — was
  simply false.** It keys by the WORKING directory. Anyone re-deriving it from
  `runtime.js` resolving `${HOME}/.claude/projects` is reading where the tree
  STARTS, not what the leaf is named for.

**THE VOLUME IS NOT THE RUNTIME'S TO DECLARE. `AgentRuntime.spec.home`,
`spec.context` AND `spec.workspace` ARE DELETED**, with no alias — see
`wiring.md`, where persistence now lives. A runtime keeps `spec.contextStorage`
alone, which is BACKEND SHAPE rather than PLACEMENT.

- **NO DUAL-READ, and `sessionId` is not the precedent.** That was a field
  renamed IN PLACE, so an alias pointed at something real. Here the CONCEPT
  moved to a different CR: an alias would resolve to a field that is not on that
  object at all. `HOME_PVC` is deleted on the same grounds.
- **THE CLAIM RENAME IS GUARDED, NOT TRUSTED.** Nothing copies a volume, so an
  upgrade that adopted `agentops-context` unremarked would leave every
  conversation answering without its context while every signal reported
  success. `agentops.contextClaimRenameGuard` FAILS the render — and, because
  `lookup` is blind without a cluster, `docs/CHANGELOG.md` is the only warning a
  GitOps install gets.
- **`agentops.retiredRuntimeVolumeKeysGuard` is its no-cluster half**, failing
  the render on `runtime.contextPvcRef` / `homePvcRef` / `workspacePvcRef`.
- **`-` IS THE WRONG STORAGE CLASS FOR THE MIGRATION, and only doing it found
  that.** A PV that was DYNAMICALLY PROVISIONED — which is what every existing
  install has — KEEPS its `storageClassName` forever, so a claim requesting `""`
  is refused with `VolumeMismatch` and sits `Pending`, indistinguishable from a
  missing provisioner. Name the PV's own class. `-` is for a STATICALLY created
  volume with no class.
  - **And a claim's spec is immutable once created**, so a wrong first attempt
    is not fixed by re-running `helm upgrade`. Delete the claim first.
- **The ordinary English word is untouched** — `state-durability`'s "one
  declared home", every Home Assistant mention, and a home DIRECTORY.

### `context-sync`

**The sidecar that keeps a runtime's LIVE context on pod-local storage and a
SNAPSHOT on the durable volume.**

**NEVER "manager".** In this codebase that word means the operator, and a second
thing wearing it would make every sentence about either ambiguous.

- **Opt-in per runtime** via `AgentRuntime.spec.contextSync`. ABSENT means
  today's pod exactly: home mounted directly, no sidecar, no migration.
- **It learns work boundaries by PROXYING the work contract.** The manager
  points the agent's `CONTROL_URL` at it and it forwards to the real manager,
  which is what lets it checkpoint without any runtime image changing.
- **Two orderings are guarantees, not details.** RESTORE completes before the
  first `/work` is answered, and CHECKPOINT completes before `/work/done`
  reaches the manager — the manager records the context handle from that report,
  and a handle whose bytes were never written names something gone.
- **The agent container holds NO mount of the durable volume in this mode.**
  Deliberate twice over: a corrupt volume cannot stop a run already going, and
  an agent cannot read another conversation's context or write to the volume at
  all.
- **Checkpoints are CONDITIONAL and INCREMENTAL.** The second half is
  load-bearing rather than an optimisation — a conditional-but-FULL copy every
  two minutes would push the whole context over NFS on every change, increasing
  writes to the very filesystem the mechanism protects. Unchanged files become
  hardlinks into the previous generation.


### API group

**`agentops.dev/v1alpha1`.** Provisional — a rename is possible pre-1.0.
