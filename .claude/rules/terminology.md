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
| `AgentRuntime` | **what executes it** |
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

**`Conversation.spec.toolsets` / `.mcpConfigs` / `.runtimeRef` /
`.serviceAccountName` mirror the originating Pipeline's bindings.** MATERIALIZED
state like `profileRef` / `channelRefs`, never hand-set.

**THE IDENTITY SNAPSHOT IS THE SHARPEST CASE OF THE REFS/CONTENT RULE.** Without
it, editing a Pipeline changes what account an INFLIGHT conversation's next pod
runs as — a privilege change applied to work already in progress.

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
configured home volume.

| Case | Behaviour |
|---|---|
| never-promised | answer fresh, and say so |
| promised-and-lost | FAIL the run — a conversation without its context is a new one wearing its name |

**Unavailability is an OUTAGE before it is a LOSS.** Bounded retry in the
runtime, then a manager-side breaker that HOLDS work. Failing fast on every
report would destroy every active conversation's context in one storage
incident.

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
