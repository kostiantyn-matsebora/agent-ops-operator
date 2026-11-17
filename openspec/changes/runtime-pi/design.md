## Context

See `proposal.md` for motivation. The shape to follow is `runtimes/copilot`:
a Node runtime, a vendor SDK in-process, one translation boundary per vendor
fact (`vocabulary.js`, `tools.js`, `mcp.js`, `continuity.js`, `report.js`),
and a bundle that declares only the vendor.

Constraints that shape the approach:

- `docs/contracts/runtime.md` is the one law for this surface. Every
  obligation is walked against its 35-item checklist.
- The manager reads no secret and grants the runtime no Kubernetes
  permission. The credential is env from a Secret reference.
- `platform/egress-proxy` walls MCP traffic only. It sees no built-in tool
  call, so it cannot be what enforces `RT-3` for `Bash`, `Edit` or `Write`.
- Pi is a 0.x package whose npm scope has moved once, from
  `@mariozechner/pi-coding-agent` to `@earendil-works/pi-coding-agent`.
  Its SDK surface can change between minors.
- Another in-flight change, `agent-definition-adopted-by-runtime`, specifies
  that each runtime appends the agent definition's body to its system prompt.
  Pi implements the same rule, so the two changes agree and neither
  restates the other.

## Goals / Non-Goals

**Goals:**
- A route moves to pi by changing `runtimeRef` and nothing else.
- Pi's tool surface is exactly what the composed allowlist names.
- The runtime consumes `mcp.json` and nothing else for MCP.

**Non-Goals:**
- Per-command shell narrowing. Pi's built-ins are enabled whole or not at
  all, as in `runtimes/ollama`.
- A pi-specific toolset, CRD field or chart value for tools or MCP.
- Running pi's own extension ecosystem. Only the one extension we ship is
  loaded.
- A conformance lane if pi cannot be pointed at a fake provider. The unit
  tier carries the assertions instead, and the design records that.

## Decisions

### D1. The SDK in-process, not RPC

The runtime embeds pi's SDK as `runtimes/copilot` embeds its own.

| Option | For | Against |
|---|---|---|
| SDK in-process | per-turn events and tool registration live in our process, one place to filter tools | tracks a 0.x API surface |
| RPC subprocess | a process boundary absorbs API churn | a second protocol to keep, events arrive as text to re-parse |

→ SDK. The fallback is RPC, and D1 is revisited only if a spike shows the SDK
cannot express D2 or D5.

### D2. Built-ins are enabled by name, whole or not at all

The composed allowlist is mapped to pi's built-in names. Only a built-in the
allowlist names is enabled for the session.

| agent-ops | Pi built-in |
|---|---|
| `Read` | read |
| `Write` | write |
| `Edit` | edit |
| `Bash` | bash |
| `Grep` | grep |
| `Glob` | find |

The pi names above are CONFIRMED live by spike S1 against SDK 1.5.2 — see
Spike findings below.

A `Bash(<prefix>:*)` pattern enables nothing and is logged, the posture
`runtimes/ollama` already takes. An empty composition enables nothing,
confirmed live. A session created with `tools: []` never declares a tool to
the model at all.

Alternative: a per-call permission hook that honours `Bash(kubectl:*)`.
Rejected for now.

- It would make this runtime the second one able to narrow a shell.
- S1 found pi DOES expose a per-call hook (`pi.on('tool_call', ...)`,
  confirmed live). This alternative is technically available if a later
  change wants it. Rejected here on the YAGNI ground below, not for lack of
  a hook.
- Enablement is availability-level and needs nothing beyond selecting tools.

### D3. The MCP extension is ours, small, and a client of `mcp.json`

Pi core has no MCP, so the image carries one extension we write. It is the
client half only.

```
MCP_CONFIG ──▶ parse ──▶ expand ${ENV} ──▶ connect (listed servers only)
                                                │
                                   tools/list ──┘
                                        │
                    name = mcp__<server>__<tool>
                                        │
              register only if the composed allowlist permits it
```

- The runtime starts no server of its own choosing and reads no other MCP
  source. `internal/mcpcompile` already decides what a conversation binds.
- A stdio entry is spawned by the runtime, as the claude and copilot runtimes
  spawn it. That is the residual risk `docs/security.md` already states.
  HTTP endpoints stay behind the egress proxy.
- `${ENV}` placeholders are expanded in-process and never logged. A missing
  one fails that server alone.
- The translation mirrors `runtimes/copilot/mcp.js` field for field, so a
  reviewer reads one shape twice.

Alternative: a third-party pi MCP package. Rejected. It would put an
unreviewed dependency between the manager's compiled config and the agent,
and its server-selection logic is exactly what this design keeps out.

### D4. The agent definition's body joins the system prompt

The runtime reads `.claude/agents/<agent>.md`, appends the body to pi's
system prompt, then the unit's `systemPrompt`, on every run. Its `tools:`
compose per `toolsMode` through the same `composeAllowedTools` shape the
other Node runtimes share.

Appended, not replacing. The other runtimes append to the vendor's own
prompt, and a route moving between vendors should see one behaviour.

Whether pi's SDK offers append or only replace is spike S2.

CONFIRMED live. It offers both, as two separate `DefaultResourceLoader`
options. Neither is persisted by the vendor session. The runtime must
resupply whichever one it uses on every call, resume included — see Spike
findings below.

This needs `RT-3.4` to name pi's path, a PATCH edit to the specification,
2.0.1 to 2.0.2. The user approved that edit explicitly for this change.

### D5. Context is a pi session under `$HOME`

The runtime mints an identifier, creates the session under it and reports it
as `runtimeContextId`. The bundle declares the session directory in
`contextSync.paths`.

Whether pi accepts a caller-chosen id is spike S3. Whether its state root can
sit under `$HOME` is spike S4.

The runtime always mints the identifier it reports, as the spec requires.

Spike S3 found pi accepts the minted id directly as its own session id,
through `SessionManager.create(cwd, sessionDir, {id})`.

It resolves the id back to a session by that same id,
`SessionManager.findById(cwd, id, sessionDir)`. No mapping file is needed.
The runtime gives each context its own `sessionDir` and lets
`findById`/`open` do the rest. See Spike findings (S3/S4) below for the
live evidence.

### D6. Telemetry comes from pi's events

`turns[]` and `toolCalls[]` are built from the SDK's events, through a
recorder shaped like `runtimes/copilot/report.js`. A figure pi does not
report is omitted. Whether per-turn usage and cache counts are exposed is
spike S5.

CONFIRMED live, and fuller than assumed. Pi reports cache WRITE tokens too,
a field copilot's shape never carried. See Spike findings below for the
field map.

### D7. One bundle, off by default

`chart/charts/pi/` follows `chart/charts/copilot/`: the credential Secret, one
`AgentRuntime` through the parent's shared renderer, a parent
`agentops.piRuntimeEntry` helper beside `agentops.copilotRuntimeEntry`, and the
branch in the declared-runtime list. Provider, model and credential are env.
The bundle adds no value that is not env, an image or a context path.

### D8. Pin exactly, from a lockfile

`package.json` pins one version of the SDK package and `npm ci` installs from
the lockfile. The Dockerfile copies copilot's npm CVE-pin block, since the
base image is the same. The package scope is pinned in the lockfile, and a
rename upstream is a deliberate bump.

## Spikes

Each is a task that ends in a written finding under this section, before any
code depends on the answer.

| # | Question | Decides |
|---|---|---|
| S1 | Can a session enable a chosen set of built-in tools, and does pi expose a per-call hook | D2, whether narrowing is possible later |
| S2 | System-prompt option: append or replace, and does it apply on resume | D4 |
| S3 | Create and resume a session by caller-chosen id | D5 |
| S4 | Relocate the state directory under `$HOME`, and which paths are the session | D5, `contextSync.paths` |
| S5 | Per-turn usage, cache counts and stop reason in events | D6 |
| S6 | Does pi load `AGENTS.md` or `CLAUDE.md` from the workspace by itself, and can that be turned off | D4, the transcript contract |
| S7 | Can the provider base URL point at a fake, for a conformance lane | the system and conformance tiers |

S6 matters because the checkout is the profile's repository. A file pi loads
on its own would be a prompt nobody composed, so if it cannot be turned off
the runtime's page says so.

## Spike findings

Every spike ran against `@earendil-works/pi-coding-agent@1.5.2`, live,
unless marked otherwise.

The driver was a local Node HTTP server speaking the OpenAI streaming
chat-completions shape. SSE `data:` chunks, never a plain JSON body, since
pi always requests `stream: true` (measured, not documented). It was
reached through a `models.json` provider entry pointing `baseUrl` at
`http://127.0.0.1`.

Every script lived under a scratch directory outside the repository and is
not shipped. Source citations below are against the package's own
`dist/*.d.ts` and `dist/core/resource-loader.js`.

### 1.1 SDK version, scope and entry points

| Fact | Value |
|---|---|
| Package | `@earendil-works/pi-coding-agent` (not `pi-agent-core` — that package has the agent loop but none of the coding tools) |
| Pinned version | `1.5.2`, the registry's `latest` dist-tag as of 2026-11-17 |
| Node floor the SDK declares | `>=22.19.0` (`package.json` `engines`), ABOVE task 2.1's `>=22.12` — flagged for that task, not fixed here |

Entry points this runtime will use:

| Surface | From |
|---|---|
| `createAgentSession()` | one call per run, mints or resumes a session |
| `ModelRuntime.create()` / `.getModel()` | provider, model and credential, from env |
| `DefaultResourceLoader` | `systemPromptOverride` / `appendSystemPromptOverride` / `agentsFilesOverride` / `extensionFactories` options |
| `SessionManager.create()` / `.open()` / `.findById()` | continuity, by caller-chosen id |
| `AgentSession` | `.prompt()`, `.subscribe()`, `.getActiveToolNames()`, `.systemPrompt`, `.sessionId`, `.sessionFile`, `.dispose()` |

The `pi.registerTool()` extension surface (`06-extensions.ts`) is how the MCP
extension (section 3) registers `mcp__<server>__<tool>` as a custom tool —
not spiked directly here, but the same `extensionFactories` mechanism S1
exercises live.

### 1.2 S1 — built-in tool selection and a per-call hook

CONFIRMED live. `createAgentSession({ tools: [...] })` takes an exact
allowlist of built-in names. A name the allowlist omits is never declared
to the model.

Verified by inspecting the request the fake server received. A session's
`tools:['read']` listed only `read`, in both the composed system prompt and
the request's native `tools` array. A session's `tools: []` listed none.

D2's mapping table is correct as written, confirmed against the live
registry:

| agent-ops | Pi built-in | Confirmed by |
|---|---|---|
| `Read` | `read` | `createReadToolDefinition`, live session declared it |
| `Write` | `write` | `createWriteToolDefinition` |
| `Edit` | `edit` | `createEditToolDefinition` |
| `Bash` | `bash` | `createBashToolDefinition` |
| `Grep` | `grep` | `createGrepToolDefinition` |
| `Glob` | `find` | `createFindToolDefinition` — its own prompt text is "Find files by glob pattern (respects .gitignore)" |

Two built-ins exist that agent-ops has no pattern for: `ls` and
`powershell`. Neither is reachable through any agent-ops allowlist entry.
Neither is ever enabled. No change needed, noted for completeness.

A per-call hook EXISTS: `pi.on('tool_call', handler)`. It registers through
an `extensionFactories` entry on the `resourceLoader` passed to
`createAgentSession`.

NOT through `session.bindExtensions()` — tried first. That is a different
surface for UI/mode bindings only, confirmed by its own types to take no
`extensionFactories` option, and it never fires a `tool_call` hook. The
handler returns `{ block, reason, terminate }` and runs before the tool
executes.

Live transcript, `tools: ['read']`, prompted to call `read`:

```
active tool names: [ 'read' ]
per-call hook fired for: [ 'read' ]
```

Live transcript, `tools: []`, same prompt:

```
active tool names: []
per-call hook fired for: []
```

The hook did not fire in the second run for a clean reason. The tool was
never declared, so the model never attempted the call. The hook itself
never failed.

D2's rejection of a per-call narrowing hook stands on its own YAGNI ground
(below), not on the hook's absence.

### 1.3 S2 — system prompt: append or replace, and resume

CONFIRMED live, both options exist as `DefaultResourceLoader` constructor
options:

| Option | Effect | Confirmed |
|---|---|---|
| `systemPromptOverride: () => text` | REPLACES the vendor base prompt | fake server's logged system message was exactly the override's text |
| `appendSystemPromptOverride: (base) => [...base, extra]` | APPENDS to the vendor base prompt | fake server's logged system message contained both the base text and the appended marker |

Neither persists in the session file. A resumed session (same
`sessionManager`, same file) built WITHOUT a `resourceLoader` lost the
marker. The same resume WITH the same `resourceLoader` resupplied it:

```
first run systemPrompt has marker: true
resume with NO override resupplied, systemPrompt has marker: false
resume WITH override resupplied, systemPrompt has marker: true
```

D4's "on every run, including resumes" is therefore not optional. It is the
only way the behaviour exists at all.

The runtime builds the `resourceLoader` with the agent definition's body
and the unit's `systemPrompt` on every `createAgentSession()` call. First
run or resume, the same way, exactly as D4 already specifies.

### 1.4 S3 and S4 — session by caller-chosen id, state root under `$HOME`

CONFIRMED live, and simpler than D5 assumed.

`SessionManager.create(cwd, sessionDir, { id })` mints a session whose
`sessionId` IS the given id, verbatim:

```
minted session id: runtime-context-abc123 == context id: true
```

`SessionManager.findById(cwd, id, sessionDir)` resolves that id back to the
session's file, given the SAME `sessionDir`, and `SessionManager.open(path,
sessionDir, cwdOverride)` resumes it:

```
findById(cwd, id, sessionDir) -> .../runtime-context-abc123/2026-...jsonl   true
resumed session id matches: true   file matches: true
findById for a missing id -> undefined
```

**This contradicts D5's mapping-file fallback.** No id-to-session mapping
file is needed. The runtime gives each context its own `sessionDir` (for
example `<state root>/<runtimeContextId>/`) and `findById` plus `open` do
the rest. `findById` on an id that was never created returns `undefined`
cleanly, which is what feeds `continuity: unavailable` without a mapping
file to consult.

The state root relocates independently in two places:

| Option | Relocates | Default |
|---|---|---|
| `agentDir` (`createAgentSession`) | `auth.json`, `models.json`, `models-store.json` | `~/.pi/agent` |
| `sessionDir` (`SessionManager.create`/`.open`/`.findById`) | the session's own `.jsonl` file tree, bypassing cwd-based path encoding entirely | `<agentDir>/sessions/<encoded-cwd>/` |

Both sat, in the live run, entirely under a scratch directory standing in
for `$HOME`/`/data/context`.

Every path one session actually wrote, enumerated from the live tree —
these are the candidates for `contextSync.paths`:

| Path (relative to the state root) | Written by | Needed for continuity |
|---|---|---|
| `<agentDir>/<sessionDir>/<timestamp>_<id>.jsonl` | the session itself, one file per session id | YES — this is the context |
| `<agentDir>/auth.json` | `ModelRuntime.create()`, even with no OAuth login | NO — stayed `{}` throughout, since D7 supplies credentials by env, never OAuth |
| `<agentDir>/models-store.json` | dynamic-provider catalog cache | NO — stayed `{}`, the fake provider is static and `models.json` is runtime-supplied, not discovered |
| `<agentDir>/models.json` | NOT written by the SDK, read from what the runtime places there | the runtime's own artifact, not session state |

So `contextSync.paths` only needs the `sessionDir` subtree. Syncing
`auth.json`/`models-store.json` too is harmless (they stay empty under this
design) but carries nothing.

### 1.5 S5 — per-turn usage, cache counts, stop reason

CONFIRMED live, from the `turn_end` event's `message.usage` and
`message.stopReason`:

```
turn_end message.usage: {"input":39,"output":7,"cacheRead":3,"cacheWrite":0,
  "reasoning":0,"totalTokens":49,"cost":{...}}
turn_end message.stopReason: stop   model: fake-model
```

Mapped onto `runtimes/copilot/report.js`'s `turns[]` shape:

| `turns[]` field | Pi source | Notes |
|---|---|---|
| `model` | `message.model` | always present |
| `tokensIn` | `usage.input` | |
| `tokensOut` | `usage.output` | |
| `cacheReadTokens` | `usage.cacheRead` | |
| `cacheCreationTokens` | `usage.cacheWrite` (+ `cacheWrite1h` split) | NOT a new field — `workreport.go`'s `Turn.CacheCreationTokens` already carries this exact fact for `runtimes/claude`/`runtimes/claude-cli`. Corrected from an earlier draft of this finding, which proposed minting `cacheWriteTokens` before checking the existing contract field |
| `stopReason` | `message.stopReason` | `"pending"\|"stop"\|"length"\|"toolUse"\|"error"\|"aborted"\|"deferred"` |
| not in copilot's shape, no existing contract field | `usage.reasoning` | present only when the provider reports a thinking-token breakdown — omit when `undefined`, per the existing "omitted, not zero" rule. `reasoningTokens` is a NEW wire field with no contract entry yet — section 7 decides whether to adopt it |

Nothing D6 expected is UNREPORTED. The only gap is the reverse: pi reports
more than copilot's shape carries a field for.

`toolCalls[]` comes from `tool_execution_start`/`tool_execution_end`:
`toolName`, `args` (never reported, since no tool input ships, per `RT-3`'s
rule), and `durationMs`. The end event's own doc comment says `durationMs`
is absent "when the tool did not run" — matches the existing "omitted where
unknown" rule.

Neither event carries a separate MCP server field. An MCP tool's identity
is the registered name itself, `mcp__<server>__<tool>`. The server is
recovered by parsing that name. D3 already names this shape, so no new
field is needed here.

### 1.6 S6 — AGENTS.md / CLAUDE.md auto-load

CONFIRMED, live for the default case and from source for the full
candidate list.

`DefaultResourceLoader` walks up from `cwd` and loads the FIRST of
`AGENTS.override.md`, `AGENTS.md`, `AGENTS.MD`, `CLAUDE.md`, `CLAUDE.MD`
found in each directory (`dist/core/resource-loader.js:130`). It also loads
one more such file from `agentDir` itself (`loadProjectContextFiles`, same
file).

Confirmed live with a real `AGENTS.md` in the checkout:

```
systemPrompt includes "haiku" (AGENTS.md): true
```

Confirmed OFF: `agentsFilesOverride: () => ({ agentsFiles: [] })` suppresses
it entirely:

```
systemPrompt includes "haiku" (AGENTS.md): false
```

So pi DOES load both names on its own, and it CAN be turned off.

Task 2.9 turns it off. A checkout's own `AGENTS.md`/`CLAUDE.md` would
otherwise be a second, uncomposed prompt the manager never agreed to. It
sits outside the one place agent-ops already puts repository instructions:
`.claude/agents/<agent>.md` (D4). The runtime's page states this
explicitly, per the task.

### 1.7 S7 — fake provider base URL

CONFIRMED live. This is what made S1, S2, S5 and S6 possible to drive live
rather than from types alone.

A `models.json` provider entry with `api: "openai-completions"` and
`baseUrl` pointing at `http://127.0.0.1:<port>/v1` is accepted and used.
`ModelRuntime` imposes no allowlist on `baseUrl`.

One behaviour had to be discovered, not assumed. Pi's OpenAI-compatible
client always sends `stream: true`, observed via a request-dumping probe
server built before the real fake existed.

A fake that returns one plain JSON body is rejected. The SDK retries into
a silent `stopReason: "error"` instead. The real fake answers
Server-Sent-Events chunks (`data: {...}\n\n...data: [DONE]\n\n`), matching
the OpenAI `chat.completion.chunk` streaming shape.

**Conformance lane (task 8.4): YES, build it.** The same fake-provider
approach the spikes used works as the conformance lane's provider stand-in.

## Conformance checklist — runtimes/pi

Task 7.2's walk of `docs/contracts/runtime.md`'s 35-item checklist
(version 2.0.2), one verdict per item, against `runtimes/pi`'s own
source and test suite.

"N/A — shared" marks an item the manager or the chart renders the
same way for every runtime. Nothing pi-specific needs verifying there,
beyond what the other three runtimes already prove.

| # | Obligation | Verdict | Evidence |
|---|---|---|---|
| 1 | `API-1` long-polls, `204` not an error | Pass, tested | `runLoop` polls continuously (`runtime.js:674-696`). `pollNextUnit` (`runtime.js:601-609`) returns `undefined` on `204` with no error path. `runtime.test.js:262` ("pollNextUnit returns undefined on a 204"), `runtime.mainentry.test.js:56` exercises the loop end to end |
| 2 | `API-2` exactly one of `promptFile`/`promptText` | Pass, consumption-side | `resolvePrompt` (`runtime.js:306-322`) prefers `promptText`, falls back to `promptFile`. `runtime.test.js:139,143,155`. The exactly-one guarantee itself is the manager's own obligation, not re-verified here |
| 3 | `API-3` continuity restored before `GET /work` answers | N/A — shared | A `contextSync` sidecar's job, not runtime code. The pi runtime only resumes a session already on disk via `SessionManager.findById`/`.open` (`runtime.js:413-417`), same posture as every other runtime |
| 4 | `API-4` `turns`/`toolCalls` trimmed to bounds | Pass, tested | `report.js:37-40,130-135` caps at 200 entries / 200 bytes. `report.test.js:58` ("the report is trimmed to the manager bounds") |
| 5 | `API-5` `runtimeContextId`/`brief` LATEST-WINS | Pass, tested | `reportDone` omits `runtimeContextId` when absent (`runtime.js:622-629`), confirmed by `runtime.test.js:306`. `attempt()`'s `...(brief ? {brief} : {})` (`runtime.js:495`) is confirmed both ways live — an answer with no `<brief>` block omits the field, one with a block reports it and strips it from `result` (`runtime.live.test.js`, "an answer with no `<brief>` block omits the field" and "a `<brief>` block in the answer is reported, and removed from result") |
| 6 | `API-6` `turns`/`toolCalls` become activity hops, never written to the Conversation | N/A — shared | Manager-side (`internal/httpapi`) handling of the posted fields, outside what the runtime controls |
| 7 | `API-7` an unknown fact is omitted, never zero | Pass, tested | `report.js`'s `count()` (line 42-44) returns `undefined` for anything not finite/non-negative, dropped by `drop()`. `report.test.js:24` shows turn two's usage carrying no cache/reasoning keys, omitted rather than zeroed |
| 8 | `API-8` a tool's input/output never reported | Pass, tested | `report.js` only ever stores `{tool, server, durationMs}` (line 113). `report.test.js:42` ("no tool input or output reaches the report") |
| 9 | `API-9` neither array reported when the run made no call | Pass, tested | `report()` (`report.js:130-134`) sets each key only when non-empty. `report.test.js:48` ("reports nothing (API-9)") |
| 10 | `API-10` a definitive 4xx is never retried | Pass, tested | `reportDone`'s definitive branch (`runtime.js:642-648`). `runtime.test.js:338` |
| 11 | `API-11` transport error/timeout/408/429/5xx gets a bounded retry | Pass, tested | Same function, retry loop up to 60×10s. `runtime.test.js:313,325,356,366` |
| 12 | `API-12` every undelivered report is logged, never dropped silently | Pass, tested | `runtime.js:646,652` both `console.error` the status/body. `runtime.test.js:338` asserts the logged text directly ("the status and body are logged (API-12)") |
| 13 | `LC-1` one inflight unit per conversation | Pass, structural | `runLoop` (`runtime.js:674-696`) awaits `runUnit`+`reportDone` fully before the next `pollNextUnit` — no concurrent second poll is ever issued. No dedicated test isolates this, same posture as the other three runtimes' identical serial loop |
| 14 | `LC-2` exits on exactly one condition: idle TTL | Pass, tested | `runtime.js:680-685,695`. `runtime.mainentry.test.js:56` |
| 15 | `LC-3` `/exit` deletes the pod, refused mid-run | N/A — shared | A manager-side (Conversation controller) verb. The runtime implements no corresponding endpoint and needs none |
| 16 | `LC-4` bad config before the first poll crashes, naming the gap | Pass, tested | `runtime.js:90-103`, three `process.exit(1)` branches. `runtime.mainentry.test.js:38,44,50` |
| 17 | `LC-5` honouring the pod's termination signal is not a lifecycle exit | Pass, tested | `SIGTERM`/`SIGINT` handlers (`runtime.js:706-711`) call `process.exit(0)` directly. `runtime.mainentry.test.js`'s "SIGTERM exits 0 promptly through its own handler, not the idle-TTL path (LC-5)" spawns the real process with a long idle TTL and asserts the exit comes from the signal handler, not the idle check |
| 18 | `RT-1` clones `REPO_URL`/`REPO_REF` with env credentials | Pass, tested | `syncRepo`/`gitEnv`/`repoURL` (`runtime.js:129-176`). `runtime.gitclone.test.js:45,51`, `runtime.sshenv.test.js:20,26`, `runtime.httpsenv.test.js:21` |
| 19 | `RT-2` executes the unit, streaming to stdout, no transport of its own | Pass, tested | `onEvent` (`runtime.js:258-302`) writes every line via `process.stdout.write`, nothing else opens a progress channel. `runtime.test.js:102,109,117` |
| 20 | `RT-3` composes the allowlist as `toolsMode` directs | Pass, tested | `composeAllowedTools` (`tools.js:163-168`) plus the per-invocation `tool_call` hook (`runtime.js:350-358`). `tools.test.js:71,76,81,85`, live enforcement at `runtime.live.test.js:178` |
| 21 | `RT-3.1` no repository visibility uses `allowedTools` as-is | Pass, tested | `readAgentDefinition` degrades to `{tools: [], body: ''}`. `tools.test.js:38,66` |
| 22 | `RT-3.2` an empty allowlist stays empty | Pass, tested | `tools.test.js:81` ("produces an empty, explicit composition"). Confirmed live by S1's finding that `tools: []` declares nothing to the model |
| 23 | `RT-3.3` an absent/unreadable definition contributes nothing | Pass, tested | `tools.test.js:38,45,66` cover a missing file, an unclosed frontmatter block, and an absent workspace |
| 24 | `RT-3.4` the definition path is runtime-specific | Pass, verified against source | `tools.js:126-128`'s `definitionPath` reads `.claude/agents/<agent>.md`, the same path `runtimes/claude`/`runtimes/ollama` use — confirmed by reading this file for this task, not assumed. `docs/contracts/runtime.md`'s `RT-3.4` row now names `runtimes/pi` |
| 25 | `SEC-1` the manager never reads a secret | N/A — shared | Manager's own obligation. The pi bundle's credential rides as `valueFrom.secretKeyRef` (`chart/charts/pi/templates/runtime.yaml:26-36`), resolved by the kubelet |
| 26 | `SEC-2` `AOPS_MCP_TOKEN` is derived, never stored | Pass (runtime's half), tested | The "derived" half is the manager's. The runtime's own half — never logging the expanded value — is in `mcp.js:50-58` ("expanded in-process and never logged") and `mcp.test.js:256` |
| 27 | `SEC-3` the operator grants the runtime no Kubernetes permissions | N/A — shared | `chart/charts/pi/templates/runtime.yaml` renders no ServiceAccount and no RBAC, only the AgentRuntime CR and an optional credential Secret |
| 28 | `MGR-1` one pod per conversation | N/A — shared | Manager's own `internal/runtimepod`, independent of which runtime image the pod runs |
| 29 | `MGR-2` context checkpointed before `POST /work/done` | N/A — shared | `contextSync` sidecar's job. The pi bundle enables it via `contextSync.paths: ['.pi/agent/sessions/**']` (`chart/charts/pi/values.yaml:81-83`), matching design's S3/S4 finding of what continuity actually needs |
| 30 | `MGR-3` a pod that never reaches `Running` is reaped | N/A — shared | Manager's own `internal/controller`, independent of the runtime image |
| 31 | `MGR-4` the manager supplies every Environment variable | N/A — shared | The runtime only reads them (`runtime.js:46-83`). The shared `agentops.piRuntimeEntry` helper (`chart/templates/_helpers.tpl:253-266`) projects `PI_PROVIDER`/`PI_MODEL`/credential alongside every runtime's common env |
| 32 | `MGR-5` an empty `REPO_URL` is never an error or "clone the default" | Pass, tested | `runtime.js:164`. `runtime.test.js:60` ("syncRepo is a no-op when no REPO_URL is configured (MGR-5)") |
| 33 | `COMPAT-1` a runtime ignores unknown work-unit fields | Pass, structural | Every consumer (`resolvePrompt`, `runPi`, ...) reads only named properties off the unit object. An unread key has no effect by construction. No dedicated test sends an unknown field, same posture as the other runtimes |
| 34 | `COMPAT-2` the manager ignores unknown report fields | N/A — shared, and actively relied on | Manager's own decode obligation. The pi runtime's `reasoningTokens` (`report.js:105`) is a field with no contract entry yet, sent in reliance on this guarantee (design's S5 finding) |
| 35 | `COMPAT-3` a published section anchor is a stable name | N/A | A documentation-versioning rule about this spec page, not something runtime code conforms to or violates |

Items 5 and 17 were initially flagged as code-confirmed but
untested. Both now carry a dedicated test (`test-coverage.md` leaves
no gap as a documented limit), closed in this same change rather than
deferred.

## Risks / Trade-offs

- **Pi's SDK moves under a pinned version** → exact pin and lockfile, and the
  unit tests assert the SDK surface the runtime uses, so a bump that changes
  it fails there rather than live.
- **A built-in name differs from the table in D2** → the runtime checks every
  mapped name against the session's registered inventory at start and logs
  any missing target, as copilot does.
- **No per-command shell narrowing** → stated on the runtime's page. A route
  needing `Bash(kubectl:*)` semantics stays on a runtime that has it.
- **A stdio MCP entry runs as a child of the agent's pod** → the existing,
  documented residual risk, unchanged by this runtime.
- **Pi's own context files could add an uncomposed prompt** → S6 settles it
  before the runtime's behaviour is claimed.
- **Fifth image to scan and release** → components.sh discovers it, the
  release workflow and the image scan treat it like the others.

## Migration Plan

Nothing migrates. The bundle is off by default and no CRD changes.

1. Release `runtime-pi-v<semver>` by tag, after the image is in the registry.
2. An adopter enables the `pi` bundle and sets `runtimeRef: pi` on a route.
3. Rollback is `runtimeRef` back to the previous runtime, then disabling the
   bundle. Conversation context is runtime-specific, so a conversation does
   not continue across vendors, which the manager already reports as
   `continuity: unavailable` rather than answering from nothing.

## Open Questions

- Whether the pi bundle may become the `default` runtime. The shared helper
  allows it by construction, and the spec scenario for it is omitted until a
  system lane proves it.
