## Context

`runtime-claude` is 359 lines because claude-code does the work: the agent loop,
tool execution, the permission model, session storage and resume all live inside
the CLI. The runtime spawns it, formats its `stream-json` to stdout, and maps
its session id onto `runtimeContextId`.

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
  `REPO_URL`, `REPO_REF`, `RUNTIME_IDLE_TTL_M`, `HOME=/data/home`,
  `MCP_CONFIG=/etc/agentops/mcp.json`, and the git auth env, and mounts the
  workspace (subPath per conversation) and home volumes.
- `AgentRuntime.spec.contextStorage` already exists to declare where context
  lives, and `spec.env` already carries per-runtime configuration.

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
  gone.
- Chart values that let one install run both backends and route per profile.
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

`runtime-ollama/` is the ninth module, Go, with its own `go.mod`. Two facts
found while planning shaped the dependency split:

- **`github.com/modelcontextprotocol/go-sdk` requires Go 1.25** (every existing
  module targets 1.23) and brings eight direct dependencies. It is taken anyway:
  an MCP client is real protocol surface — initialize handshake, `tools/list`,
  `tools/call`, stdio and streamable-HTTP transports, notifications — and
  hand-rolling it means owning spec drift forever. The module boundary contains
  the cost: this module pins `go 1.25` and builds in `golang:1.25`; nothing else
  moves.
- **`github.com/ollama/ollama/api` is NOT taken.** That module requires Go 1.26
  and its graph carries gin, cobra, sqlite3 and bubbletea — the whole server —
  for what is two request structs and two response structs. `/api/chat` and
  `/api/show` are hand-rolled over `net/http`, ~120 lines, no dependency.

*Alternative considered:* `mark3labs/mcp-go` (more importers, older Go floor).
Rejected in favour of the official SDK now that one exists and is maintained in
collaboration with Google; the migration cost between them is small either way.

*Escape hatch:* if the dependency surface proves objectionable in review, the
MCP client is one file behind an interface (`toolSource`), and a minimal
hand-rolled JSON-RPC client replaces it without touching the loop.

### 2. File layout

```
runtime-ollama/
  main.go        env, startup checks, /work poll loop, idle TTL
  work.go        contract types, long-poll, /work/done with retry
  agent.go       the agent loop: chat -> tool calls -> execute -> repeat
  ollama.go      /api/chat + /api/show over net/http (the only vendor surface)
  tools.go       allowlist composition + the gate + the tool registry
  builtin.go     Read, Grep, Glob, Edit, Write, Bash
  mcpclient.go   MCP servers from $MCP_CONFIG via the official SDK
  contextstore.go transcripts, handles, the gone-vs-slow re-check
  repo.go        git checkout at /data/workspace (ported from runtime.js)
```

`ollama.go` is the only file that knows the vendor. That is deliberate — see
decision 9.

### 3. The loop, and what bounds it

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

### 4. Tools: two sources, one gate, fail closed

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

### 5. Built-in tools: workspace-confined, bounded, and honest about Bash

`Read`, `Grep`, `Glob`, `Edit`, `Write` resolve every path against
`/data/workspace` and refuse anything that escapes it after symlink resolution —
absolute paths, `..`, and symlinked escapes alike, as a tool error rather than a
crash. Results are size-bounded (default 64 KiB, truncation stated in the
result) because one `Read` of a 5 MB log would otherwise consume the context
window and produce a worse answer than reading nothing.

`Bash` runs in the workspace with a timeout (default 120 s) and bounded captured
output. Its risk is not mitigated and should not be described as if it were: a
route binding `agentops-shell` gives the model this pod's shell with the runtime
ServiceAccount's reach. That is the same posture as the reference runtime, and
it is exactly why the chart ships the vocabulary risk-split — the answer to
"should this agent have a shell" is a Pipeline binding, not a runtime feature.

### 6. Context: the runtime's own transcript, the manager's opaque handle

One JSON file per context under `$HOME/.agentops/contexts/<id>.json`, holding
`{id, conversation, created, updated, messages[]}`. `id` is
`oc-<12 hex>`; nothing outside this module parses it. Writes are atomic
(temp file + rename) because the home volume is RWX and another pod may be
reading.

Given a handle: load, append, report `continuity: continued`. Given none:
create, report `new`. The handle reported is always the one that now exists —
latest-wins, matching the manager's storage rule.

**Gone versus slow** is reproduced from the reference runtime because it was
paid for there: on a miss, re-check after 500 ms, 1.5 s and 3 s; a read that
ERRORS is unavailability of the store, not absence of the context; only a
confirmed absence fails the run, with `continuity: unavailable`, a
`continuityReason` naming the home volume, and a non-empty user-facing message.
Seconds, not minutes — someone is waiting on a chat reply.

`contextStorage: volume` is declared on the CR, so the manager can tell before
promising continuity whether this deployment can keep it.

### 7. Context window: explicit, always

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

### 8. Startup checks that fail loudly

At startup: `GET /api/tags` (endpoint reachable) and `POST /api/show` for the
configured model (present, and whether its capabilities include tools). All
three answers go to the log in one line.

If the model cannot call tools and a unit arrives carrying a non-empty
allowlist, the run FAILS naming the model and the limitation. A text-only model
on a text-only route is a legitimate install; a route whose Pipeline grants
tools to an agent that can never use them is a misconfiguration that must not
look like a quiet agent.

### 9. One vendor file, so an OpenAI-compatible sibling is cheap

`ollama.go` is the only file that knows Ollama's wire format, behind a small
interface (`chat(ctx, messages, tools) (message, error)`). Ollama's
OpenAI-compatible `/v1` endpoint exists, and so do vLLM, llama.cpp and LM
Studio; a `openai.go` implementing the same interface would serve all of them
without touching the loop, the gate, the built-ins or the context store. Not
built now — the native API gives `keep_alive`, `num_ctx` and capability
introspection that the compat layer does not — but the seam is where it needs to
be.

### 10. Chart: an additional-runtimes list, defaulted to empty

`chart/values.yaml` gains `extraRuntimes: []`, rendered by a sibling template to
`runtime.yaml`. Each entry: `name`, `image`, `contextStorage`, `env`,
`idleTtlMinutes`, `nodeSelector`, `resources`, `serviceAccountName`,
`homePvcRef`, `workspacePvcRef`. Unset volume refs resolve through the SAME
helper the default runtime uses, so an extra runtime inherits the release's
persistence rather than restating it — the mistake chart 4.0 removed.

`serviceAccountName` defaults to the release runtime SA. An entry MAY name
another, which is the supported way to give one backend a different trust level;
the identity stays runtime-level, exactly as the invariant requires.

An empty list renders nothing, so existing installs are byte-identical. The
Ollama example lives in the values comment and in `docs/runtimes.md`, not as a
default — the chart cannot know an endpoint.

### 11. Documentation: a new page, because a runtime is neither a CRD nor a bundle

`docs/runtimes.md` covers both shipped images: what each supports (tools,
continuity, cost, latency), how to choose, the env each reads, and how to write
a third. `CLAUDE.md`'s routing table gains the row, since the existing rows
route CRD semantics, contracts and bundles and a runtime image is none of them.

## Risks / Trade-offs

- **This module needs Go 1.25 while the repo is on 1.23** → Module boundary
  handles it: its own `go.mod` and its own `golang:1.25` build container. The
  build documentation must state it explicitly, or the first `go build ./...`
  in the 1.23 container fails confusingly.
- **A dependency-taking module in a dependency-free repo** → Contained to one
  module and to one file behind an interface, with a documented fallback. Stated
  as a decision rather than discovered later as an inconsistency.
- **Small models call tools badly** — hallucinated names, malformed JSON
  arguments, loops of the same call → Unknown names and bad arguments come back
  as readable tool errors the model can recover from; `maxTurns` bounds the
  thrash; the run reports the turn-limit failure rather than silence. Model
  quality is an operator choice, and `docs/runtimes.md` says which sizes are
  worth trying.
- **Ollama serialises work per model** → Five concurrent conversations
  (`MAX_ACTIVE_CONVERSATIONS`) against one Ollama endpoint queue at the server;
  runtime parallelism is not inference parallelism. Documented, with
  `OLLAMA_NUM_PARALLEL` named as the server-side knob.
- **Model unload between runs** (`keep_alive`) → The first run after an idle
  period pays a model load. `OLLAMA_KEEP_ALIVE` is exposed and its interaction
  with `RUNTIME_IDLE_TTL_M` is documented; neither default is changed to hide it.
- **A shell in the pod** → Unchanged posture from the reference runtime, and the
  reason the built-in toolsets are risk-split. Said plainly in the docs rather
  than softened.
- **Transcripts grow on the home volume** → One file per context, trimmed at
  request time, never compacted in v1. A conversation that runs for months keeps
  a large file; the run cost does not grow with it because only the window is
  sent. Compaction is an Open Question, not a silent omission.
- **The agent's output format** → The manager's `format.md` asks for a markdown
  subset; a small model complies less reliably. The failure is cosmetic, not
  structural, because adapters escape what they are given — worst case is ugly
  text in chat, never markup injection.

## Migration Plan

1. Build the module and its tests; nothing renders and nothing changes for any
   install.
2. Publish `agentops-runtime-ollama:0.1.0`. Tags are never overwritten.
3. Land the chart's `extraRuntimes: []` (empty default → identical manifests)
   and the chart render test pinning what a declared entry produces.
4. An adopter opts in: declare the entry with their endpoint and model, point a
   profile's `runtimeRef` at it, and verify with a live task — an ordinary
   signal to a source a Ready Pipeline claims.
5. Documentation lands with the chart change, not after it.

**Rollback:** remove the entry from `extraRuntimes` and re-point the profile's
`runtimeRef`. Conversations already running on the removed runtime keep their
`runtimeContextId`, which the reference runtime cannot read — so those
conversations continue as new ones and say so, which is precisely the
`continuity: unavailable` path behaving as designed. Nothing else unwinds; no
CRD, manager or contract change exists to reverse.

## Open Questions

- **Summarisation instead of a sliding window.** A summary keeps older context
  alive at the cost of an extra inference call per trim and a new failure mode
  (a wrong summary is invisible). Sliding window ships first; revisit with real
  transcripts.
- **Reasoning models' thinking output.** Models that emit thinking blocks could
  have them logged to stdout (useful for debugging, noisy in pod logs) or
  dropped. Default to dropping from the reported `result`, logging behind an env
  flag.
- **Transcript compaction.** Nothing prunes context files today; the home volume
  grows with conversation count. A retention sweep belongs to whoever owns the
  volume, but the runtime is the only component that knows the layout.
- **An OpenAI-compatible sibling** (decision 9) — worth building once someone
  asks for vLLM or llama.cpp, and cheap because of where the seam is.
