## Context

`runtime-claude` is small because claude-code does the work: the agent loop,
tool execution, the permission model, context storage and resume all live inside
the CLI. The runtime spawns it, formats its `stream-json` to stdout, and maps
its own handle onto `runtimeContextId`.

Ollama supplies exactly one of those things — a `/api/chat` endpoint that
returns the next assistant message, optionally with `tool_calls`. Everything
else is ours to build. That is the point of the change: it is the first
implementation that has to satisfy the contract on its own terms, and whatever
it cannot satisfy is a place where the contract quietly described claude-code.

What the manager already provides, verified against the code:

- `internal/dispatch.WorkUnit` carries `promptText`/`promptFile`+`promptVars`,
  `agent`, `systemPrompt`, `allowedTools`, `toolsMode`, `maxTurns`, `threadId`,
  `runtimeContextId`.
- `internal/httpapi.workDone` accepts `status`, `exitCode`, `result`,
  `runtimeContextId`, and — already, with runtime-agnostic wording —
  `continuity` (`continued` | `new` | `unavailable`) and `continuityReason`.
- `internal/runtimepod` injects `CONTROL_URL`, `CONVO_ID`, `POD_NAME`,
  `REPO_URL`, `REPO_REF`, `RUNTIME_IDLE_TTL_M`, `HOME=/data/context`,
  `WORKSPACE`, `MCP_CONFIG=/etc/agentops/mcp.json`, and the git auth env
  (`GIT_AUTH_TYPE`, `GIT_SSH_KEY`, `GIT_TOKEN`); it mounts the workspace
  (subPath per conversation) and, in the unsynchronised pod, the context volume
  at `/data/context`. Under `context-sync`, `$HOME` is pod-local storage the
  sidecar restores before the first `/work` and snapshots before `/work/done`
  reaches the manager — and `CONTROL_URL` points at the sidecar, which proxies
  the work contract. A runtime that speaks the contract through `CONTROL_URL`
  gets all of this for free.
- Under egress mediation the pod's outbound traffic is redirected through the
  proxy, which enforces the bound toolsets on MCP traffic and copies everything
  else through untouched. The runtime's own calls to Ollama are "everything
  else"; its MCP calls are gated twice, once here and once there, and that is
  the intended posture — the runtime's gate configures a cooperating loop, the
  proxy constrains an uncooperative shell.
- `AgentRuntime.spec.contextStorage` already exists to declare the backend's
  SHAPE, `spec.env` carries per-runtime configuration, and `spec.contextSync`
  declares which paths under `$HOME` the sidecar keeps.

So the manager side of this change is empty. That is the design's first claim
and the tasks verify it rather than assume it.

## Goals / Non-Goals

**Goals:**

- A second runtime image that satisfies the work contract with no manager,
  CRD or contract change.
- Tools that mean the same thing as on the reference runtime: the chart's
  built-in vocabulary implemented natively, MCP servers connected from the same
  mounted `mcp.json`, one allowlist gate over both.
- Continuity implemented to the same standard as the reference runtime,
  including the distinction between a store that is slow and a context that is
  gone — and the same context-sync posture, so one damaged volume does not stop
  a run already going.
- A bundle that lets one install run both backends and route per Pipeline.
- Prove or disprove the contract's vendor-neutrality, and report what breaks.

**Non-Goals:**

- Deploying, scheduling or managing Ollama, models or GPUs.
- A per-profile or per-pipeline model field. The model is what executes, so it
  belongs to `AgentRuntime`; two models are two CRs.
- Matching claude-code's agent quality. A local 8B model with six tools is a
  different product; the routes it suits are chosen by the operator, per
  Pipeline.
- Embeddings, RAG, vector stores, sub-agents, or a second prompt-template
  system.

## Decisions

### 1. Go module, official MCP SDK, hand-rolled Ollama client

`runtimes/ollama/` is a Go component with its own `go.mod`; the module path
follows the directory
(`github.com/kostiantyn-matsebora/agent-ops-operator/runtimes/ollama`), as
`structure.md` requires. Two facts found while planning shaped the dependency
split:

- **`github.com/modelcontextprotocol/go-sdk` requires Go 1.25** (every existing
  module targets 1.23) and brings eight direct dependencies. It is taken anyway:
  an MCP client is real protocol surface — initialize handshake, `tools/list`,
  `tools/call`, stdio and streamable-HTTP transports, notifications — and
  hand-rolling it means owning spec drift forever. The module boundary contains
  the cost: this module pins `go 1.25`, and nothing else moves. **Re-verify the
  floor at apply time** — it was read on 2026-08-22 and the SDK moves.
- **`github.com/ollama/ollama/api` is NOT taken.** That module requires Go 1.26
  and its graph carries gin, cobra, sqlite3 and bubbletea — the whole server —
  for what is two request structs and two response structs. `/api/chat`,
  `/api/show` and `/api/tags` are hand-rolled over `net/http`, ~120 lines, no
  dependency.

*Alternative considered:* `mark3labs/mcp-go` (more importers, older Go floor).
Rejected in favour of the official SDK now that one exists and is maintained in
collaboration with Google; the migration cost between them is small either way.

*Escape hatch:* if the dependency surface proves objectionable in review, the
MCP client is one file behind an interface (`toolSource`), and a minimal
hand-rolled JSON-RPC client replaces it without touching the loop.

### 2. Build: own Dockerfile, second persistent container

The shared `.github/docker/go-module.Dockerfile` is `golang:1.23` and produces a
distroless image with nothing but `/app`. This component needs 1.25 to compile
and git, openssh-client and a shell to run, so it carries its own `Dockerfile`
— `components.sh` already prefers one that exists. Build stage `golang:1.25`,
runtime stage `debian:bookworm-slim` with `git openssh-client ca-certificates`
and nothing domain-shaped; multi-arch, like every image here.

Locally, `build-test.md`'s `agentops-go` container is 1.23 and will fail on this
module by design. A second container, `agentops-go125`, is started with the
SAME mounts (repo at its real path, the worktrees parent, the two named cache
volumes) from `golang:1.25`, and `build-test.md` records it beside the first so
the failure is expected rather than confusing. CI's matrix reads the Go version
per module, which `components.sh` already supports for a module with its own
Dockerfile.

### 3. File layout

```
runtimes/ollama/
  Dockerfile       golang:1.25 build, slim runtime with git + shell
  main.go          env, startup checks, /work poll loop, idle TTL
  work.go          contract types, long-poll, /work/done with retry
  agent.go         the agent loop: chat -> tool calls -> execute -> repeat
  ollama.go        /api/chat, /api/show, /api/tags over net/http (the only vendor surface)
  tools.go         allowlist composition + the gate + the tool registry
  builtin.go       Read, Grep, Glob, Edit, Write, Bash
  mcpclient.go     MCP servers from $MCP_CONFIG via the official SDK
  contextstore.go  transcripts, handles, the gone-vs-slow re-check
  repo.go          git checkout at /data/workspace (ported from runtime.js)
```

`ollama.go` is the only file that knows the vendor. That is deliberate — see
decision 10.

### 4. The loop, and what bounds it

```
messages = [system] + transcript + [user prompt]
for turn in 1..maxTurns:
    resp = ollama.chat(messages, tools=gated)
    print assistant text
    if no tool_calls: return resp.text          # the answer
    for call in resp.tool_calls:
        result = registry.execute(call)          # gate already applied
        messages += tool result
return failed("turn limit reached")
```

`maxTurns` comes from the unit (default 60 in the reference runtime). Every step
prints: assistant text, `[tool] name args…` truncated, and the result's outcome
and size. A run that ends on the turn limit fails with a stated reason — never
an empty result, and never the tool trace dressed up as an answer, which is the
failure mode that made the reference runtime's empty-result path a bug worth
fixing once already.

Tool calls within one response are executed in order, not concurrently: `Bash`
and `Edit` can interact, and a deterministic order is worth more than the
latency.

### 5. Tools: two sources, one gate, fail closed

The registry holds built-ins (bare names: `Read`, `Grep`, `Glob`, `Edit`,
`Write`, `Bash`) and MCP tools (`mcp__<server>__<tool>`, schema taken straight
from `tools/list` — MCP input schemas are JSON Schema, which is what Ollama's
`tools[].function.parameters` wants).

The gate is applied ONCE, before the request: only allowed tools are advertised.
A model that names an unadvertised tool anyway gets a tool error it can read and
recover from, never an execution.

Composition is a port of `runtimes/claude/tools.js` — including its deliberate
non-YAML-parser posture: read one field of one shape, treat anything not
understood as "declares nothing", never let an unreadable role file stop an
agent answering. Its test cases port with it.

**Pattern matching fails closed, including on narrowing syntax.** Exact name
matches itself; a trailing `*` matches a prefix; `mcp__server__*` works. A
claude-style specifier such as `Bash(kubectl:*)` is NOT interpreted as bare
`Bash` — that would widen a grant the operator wrote to narrow it. It grants
nothing and is logged. Nothing the chart ships uses that form, so no shipped
binding changes meaning.

### 6. Built-in tools: workspace-confined, bounded, and honest about Bash

`Read`, `Grep`, `Glob`, `Edit`, `Write` resolve every path against
`/data/workspace` and refuse anything that escapes it after symlink resolution —
absolute paths, `..`, and symlinked escapes alike, as a tool error rather than a
crash. Results are size-bounded (default 64 KiB, truncation stated in the
result) because one `Read` of a 5 MB log would otherwise consume the context
window and produce a worse answer than reading nothing.

`Bash` runs in the workspace with a timeout (default 120 s) and bounded captured
output. Its risk is not mitigated here and should not be described as if it
were: a route binding `agentops-shell` gives the model this pod's shell with
whatever the route's `serviceAccountName` — or the floor — can reach. That is
the same posture as the reference runtime, and it is exactly why the chart
ships the vocabulary risk-split and why egress mediation is on by default: the
answer to "should this agent have a shell" is a Pipeline binding, not a runtime
feature.

### 7. Context: the runtime's own transcript, the manager's opaque handle

One JSON file per context under `$HOME/.agentops/contexts/<id>.json`, holding
`{id, conversation, created, updated, messages[]}`. `id` is `oc-<12 hex>`;
nothing outside this module parses it. Writes are atomic (temp file + rename):
in the unsynchronised pod `$HOME` IS the shared context volume and another pod
may be reading; under `context-sync` the sidecar snapshots the tree and a
half-written file is what `retain` exists to survive.

**The bundle declares `contextSync.paths: [".agentops/contexts/**"]`**, the
exact counterpart of the claude bundle's `.claude/projects/-data-workspace/**`.
The path is this backend's layout, so it lives with the image — never in
`global.agentops.runtimeDefaults`. A run therefore reads and writes pod-local
storage, and the durable volume holds a snapshot; RESTORE completes before the
first `/work` is answered, so the gone-versus-slow re-check below sees the
restored tree, not the volume.

Given a handle: load, append, report `continuity: continued`. Given none:
create, report `new`. The handle reported is always the one that now exists —
latest-wins, matching the manager's storage rule.

**Gone versus slow** is reproduced from the reference runtime because it was
paid for there: on a miss, re-check after 500 ms, 1.5 s and 3 s; a read that
ERRORS is unavailability of the store, not absence of the context; only a
confirmed absence fails the run, with `continuity: unavailable`, a
`continuityReason` naming the context volume, and a non-empty user-facing
message. Seconds, not minutes — someone is waiting on a chat reply.

`contextStorage: volume` is inherited from the defaults and left as is, so the
manager can tell before promising continuity whether this deployment can keep
it — and a route with no durable claim gets the unsynchronised, ephemeral pod
and is told its context is not promised, by the existing rule.

### 8. Context window: explicit, always

Every `/api/chat` carries `options.num_ctx` explicitly (`OLLAMA_NUM_CTX`,
default 8192). Ollama's server default silently drops the front of an
over-long prompt — the system prompt and the alert payload are at the front —
and a truncated prompt produces a confident wrong answer with nothing in any log
to explain it. This is the gotcha most likely to be re-introduced by someone
"simplifying" the request builder.

Trimming policy when the assembled conversation exceeds the budget: keep the
system prompt and the current turn, drop oldest exchanges first, state in the
log how many messages and roughly how many tokens were dropped. Trimming is NOT
`continuity: unavailable` — the context was reached; part of it did not fit.
Summarisation is deliberately not in v1 (see Open Questions).

### 9. Startup checks that fail loudly

At startup: `GET /api/tags` (endpoint reachable) and `POST /api/show` for the
configured model (present, and whether its capabilities include tools). All
three answers go to the log in one line.

If the model cannot call tools and a unit arrives carrying a non-empty
allowlist, the run FAILS naming the model and the limitation. A text-only model
on a text-only route is a legitimate install; a route whose Pipeline grants
tools to an agent that can never use them is a misconfiguration that must not
look like a quiet agent.

### 10. One vendor file, so an OpenAI-compatible sibling is cheap

`ollama.go` is the only file that knows Ollama's wire format, behind a small
interface (`chat(ctx, messages, tools) (message, error)`). Ollama's
OpenAI-compatible `/v1` endpoint exists, and so do vLLM, llama.cpp and LM
Studio; an `openai.go` implementing the same interface would serve all of them
without touching the loop, the gate, the built-ins or the context store. Not
built now — the native API gives `keep_alive`, `num_ctx` and capability
introspection that the compat layer does not — but the seam is where it needs to
be.

### 11. Chart: a bundle, off by default, the claude bundle's shape exactly

`chart/charts/ollama/` is a second vendor bundle: `Chart.yaml`, `values.yaml`,
one template rendering `agentops.renderRuntime` over
`global.agentops.runtimeDefaults` — the parent's shared renderer, so this CR
cannot drift from one an install declares under `runtimes:`. The parent gains
one dependency line with `condition: ollama.enabled`.

The values carry ONLY what names this vendor, by the rule the claude bundle
states on itself:

| Key | Is |
|---|---|
| `enabled: false` | off — the claude bundle is the one on by default, and a fresh install executes on it |
| `name: ollama` | the `AgentRuntime` name a route's `runtimeRef` selects; `default` is legal if the claude bundle is off and this is the install's only runtime |
| `image` | the one place the tag is written |
| `env` | `OLLAMA_URL`, `OLLAMA_MODEL`, and the tuning knobs (`OLLAMA_NUM_CTX`, `OLLAMA_KEEP_ALIVE`, `OLLAMA_TIMEOUT_S`, `BASH_TIMEOUT_S`, `TOOL_OUTPUT_MAX`) — no defaults for the first two, and the runtime exits naming a missing one |
| `contextSync.paths` | `.agentops/contexts/**` — this backend's layout |
| `credentialsSecret` | OPTIONAL, unlike claude's: Ollama itself is unauthenticated, and a reverse proxy in front of it may want a bearer. Rendered only when named |

Everything else — `contextStorage`, `idleTtlMinutes`, `resources`,
`nodeSelector`, `egressMediation`, the context-sync image — is inherited, and
any of it is overridable on this entry alone. **The bundle renders no
substrate**: no floor account, no defaults, no volume. A route wanting a
different trust level names `serviceAccountName` on the Pipeline; wanting a
different volume, `persistence` — neither is this runtime's to declare.

`ollama.enabled: false` renders nothing, so existing installs are
byte-identical, and the parent's `runtimes:` block keeps its Ollama example
comment as the second way to declare the same thing by hand. `docs/runtimes/ollama.md`
carries the worked values; the bundle cannot know an endpoint.

*Alternative rejected:* a parent-chart list of extra runtimes. It existed as
`runtimes:` before this change was applied, and a vendor's image, env and
sync paths are DOMAIN — `agent-runtime-ownership` already says a vendor arriving
as a bundle must not need a hand-written CR. The bundle is that rule's second
instance.

### 12. Documentation: a Runtimes group on the site, and the landing page says it

`docs/CLAUDE.md` rules that A RUNTIME IS NOT AN INTEGRATION — an integration
page answers what starts work, what it may reach, where it answers and what it
costs, and a runtime would leave three of the four blank. That rule stands, so
the page is NOT `docs/runtimes/ollama.md`. It is `docs/runtimes/ollama.md`,
the first page of a new **Runtimes** nav group, and `docs/CLAUDE.md` gains the
page kind: a runtime page answers what it EXECUTES, what it NEEDS from you,
where its context lives, and what it costs to turn on. `docs/claude.md` stays
the reference page it is — moving it into the group is its own change, and the
asymmetry is visible on purpose.

The landing page carries the local-model story or the proposal's opening claim
is made nowhere an adopter reads: an Ollama chip in "Works with" linking the
page (a mark under `docs/assets/img/logos/`, the same third-party treatment as
the other four), the "Runs Claude Code" claim amended to "Runs Claude Code — or
a local model", and a row in "Why agent-ops?" for keeping the cluster's data in
the cluster. `README.md` mirrors all three — the claims line, the "Works with"
line, the integrations row — within its 215-line budget, by `documentation.md`'s
rule that the two are one story for two audiences. The page covers the
operational facts (per-model serialisation against `MAX_ACTIVE_CONVERSATIONS`,
`keep_alive` against `idleTtlMinutes`, `num_ctx`, a shell is the pod's shell)
and how to choose between the two images. `docs/installation.md`'s hand-written
`runtimes:` Ollama example becomes the bundle's values.
`.claude/rules/structure.md` gains the component in its `runtimes/` row and
`build-test.md` the second container.

## Risks / Trade-offs

- **This module needs Go 1.25 while the repo is on 1.23** → Module boundary
  handles it: its own `go.mod`, its own Dockerfile and its own persistent build
  container. `build-test.md` states it explicitly, or the first `go build ./...`
  in the 1.23 container fails confusingly.
- **A dependency-taking module in a dependency-free repo** → Contained to one
  module and to one file behind an interface, with a documented fallback. Stated
  as a decision rather than discovered later as an inconsistency.
- **Small models call tools badly** — hallucinated names, malformed JSON
  arguments, loops of the same call → Unknown names and bad arguments come back
  as readable tool errors the model can recover from; `maxTurns` bounds the
  thrash; the run reports the turn-limit failure rather than silence. Model
  quality is an operator choice, and `docs/runtimes/ollama.md` says which sizes are worth
  trying.
- **Ollama serialises work per model** → Five concurrent conversations
  (`MAX_ACTIVE_CONVERSATIONS`) against one Ollama endpoint queue at the server;
  runtime parallelism is not inference parallelism. Documented, with
  `OLLAMA_NUM_PARALLEL` named as the server-side knob.
- **Model unload between runs** (`keep_alive`) → The first run after an idle
  period pays a model load. `OLLAMA_KEEP_ALIVE` is exposed and its interaction
  with `idleTtlMinutes` is documented; neither default is changed to hide it.
- **A shell in the pod** → Unchanged posture from the reference runtime, and the
  reason the built-in toolsets are risk-split and egress is mediated. Said
  plainly in the docs rather than softened.
- **Transcripts grow under `$HOME`** → One file per context, trimmed at request
  time, never compacted in v1. A conversation that runs for months keeps a large
  file; the run cost does not grow with it because only the window is sent.
  `housekeeping` knows claude-code's transcript layout and not this one, so
  orphaned contexts here are not reclaimed — an Open Question, not a silent
  omission.
- **The agent's output format** → The manager's `format.md` asks for a markdown
  subset plus the block grammar; a small model complies less reliably. The
  failure is cosmetic, not structural, because adapters escape what they are
  given — worst case is ugly text in chat, never markup injection.

## Migration Plan

1. Build the module and its tests; nothing renders and nothing changes for any
   install.
2. Publish by tag: `runtime-ollama-v0.1.0`. CI builds both architectures and
   pushes; the FIRST push creates the package PRIVATE, so the visibility flip is
   a UI step before any install can pull it. Tags are never overwritten.
3. Land the bundle (`ollama.enabled: false` → identical manifests) and the chart
   render test pinning what an enabled bundle produces.
4. An adopter opts in: `ollama.enabled: true` with their endpoint and model,
   `runtimeRef: ollama` on a Pipeline, and verify with a live task — an ordinary
   signal to a source that Pipeline claims.
5. Documentation lands with the chart change, not after it.

**Rollback:** disable the bundle and re-point the Pipeline's `runtimeRef`.
Conversations already opened on it snapshotted the runtime NAME resolved, so
their next pod resolves a runtime that no longer exists and the manager reports
that on the conversation — close them, or re-enable the bundle. That is the
snapshot rule protecting inflight work, not a defect.
New conversations on the re-pointed route start on the reference runtime with
no handle. Nothing else unwinds; no CRD, manager or contract change exists to
reverse.

## Open Questions

- **Summarisation instead of a sliding window.** A summary keeps older context
  alive at the cost of an extra inference call per trim and a new failure mode
  (a wrong summary is invisible). Sliding window ships first; revisit with real
  transcripts.
- **Reasoning models' thinking output.** Models that emit thinking blocks could
  have them logged to stdout (useful for debugging, noisy in pod logs) or
  dropped. Default to dropping from the reported `result`, logging behind an env
  flag.
- **Transcript reclaim.** `housekeeping` reclaims transcripts by claude-code's
  layout; `.agentops/contexts/` is invisible to it. Either it learns a second
  layout or the layout is declared to it — a change of its own.
- **An OpenAI-compatible sibling** (decision 10) — worth building once someone
  asks for vLLM or llama.cpp, and cheap because of where the seam is.
