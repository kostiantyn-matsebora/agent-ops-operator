## Context

`AgentRuntime` promises a pluggable backend — `spec.image` plus the four-step
work contract — and two images implement it: `runtimes/claude/` (a vendor CLI)
and `runtimes/ollama/` (a harness of its own over an inference endpoint). Both
speak agent-ops' tool vocabulary natively: claude-code because the vocabulary
IS claude-code's, ollama because it implements `Read`/`Bash`/`Edit` itself. So
three vendor-shaped claims in the contract are still untested: `runtimeContextId`
is opaque *because we said so*; `allowedTools` patterns are "opaque, translated
at the boundary" though nothing has ever translated one; the agent definition
lives at `.claude/agents/<agent>.md` because that is where claude-code looks.

GitHub Copilot disagrees on all three: a different credential
(`COPILOT_GITHUB_TOKEN`), a vendor-owned tool vocabulary (`shell`, `view`,
`grep`; MCP tools as `mcp:<server>-<tool>`), a different definition path
(`.github/agents/<agent>.agent.md`) — and one inverted default that matters: an
agent definition with no `tools:` gets ALL tools, where agent-ops means
"declares nothing".

The Copilot SDK (`@github/copilot-sdk`, Node, bundles the CLI) also lets the
CALLER supply the session id. That makes this the first backend where
`runtimeContextId` is a handle we mint rather than one scraped back out of a
vendor's stdout — which is what the field always claimed to be.

Constraints carried in unchanged: self-contained component, no dependency on
any other module here; generic by construction (git + shell, no domain
tooling); the manager reads no Secrets; the manager composes meaning, the
runtime renders nothing transport-shaped; `/data/workspace` and `/data/context`
are mount points; `HOME=/data/context`; a bundle ships the vendor and inherits
the substrate; `context-sync` proxies `CONTROL_URL` and the bundle declares the
paths; egress mediation is on by default and the runtime does nothing to earn
it.

## Goals / Non-Goals

**Goals:**
- A third reference runtime that passes the same work contract, proving the
  vocabulary rules by making them survive a vendor that owns its own.
- One toolset vocabulary cluster-wide. A Pipeline binds the same `MCPToolset`
  CRs whichever runtime serves it; the vendor difference is absorbed where
  vendor knowledge already lives.
- Same safety posture as the other two: empty allowlist means empty, no
  permission prompt can hang a pod, and a conversation whose context is gone
  FAILS rather than answering as if it were new.
- A bundle in the ollama bundle's exact shape, off by default, so the parent
  stays the owner of the defaults and the floor.

**Non-Goals:**
- No CRD change. `AgentRuntime` already describes this runtime completely.
- No manager change. Dispatch, ingest, capability resolution and the work unit
  are untouched.
- No Copilot-specific `MCPToolset`s, no vocabulary field on any CR, no
  runtime-kind discriminator anywhere in `internal/`.
- Not a trust boundary. Identity (`pipelines[].serviceAccountName`) and storage
  (`pipelines[].persistence`) are the route's, and a second vendor changes
  neither.

## Decisions

### D1 — Drive Copilot through the SDK, not the `copilot` CLI

`@github/copilot-sdk` in-process, not `spawn('copilot', …)`.

The deciding factor is the context handle. `createSession({sessionId})` accepts
an id we choose; `resumeSession(id)` continues it; state lands at
`$COPILOT_HOME/session-state/<id>/` — `session.db`, `events.jsonl`,
`checkpoints/`, `files/` (verified), and `COPILOT_HOME` defaults to
`$HOME/.copilot`. The CLI cannot be told the id of a NEW
session (github/copilot-cli#442) — the documented workaround is to diff the
state directory after the run and guess which directory is ours. That is a race
under concurrent pods and a lie under any failure that writes a directory
without finishing a run.

The SDK and the CLI it drives are PINNED EXACTLY, both of them. The SDK
declares `@github/copilot: ^1.0.79`, and CLI 1.0.81 — published four days after
SDK 1.0.11 — dropped the `./sdk` package export the SDK resolves at startup, so
a floating range installs a pair that throws before any session exists. The
two are one artifact released as two packages; `package.json` names both
versions and `npm ci` from the lockfile is what the image runs.

Everything else the contract needs is a first-class option rather than a flag
scrape: `availableTools`/`excludedTools` (allowlist), `onPermissionRequest`
(per-invocation decisions), `mcpServers`, `workingDirectory`, `streaming: true`
with typed events for the stdout transcript, `sessionLimits`.

*Alternative considered:* the CLI, for symmetry with `runtime-claude`'s
`spawn('claude')`. Rejected: the symmetry is cosmetic and the cost is the one
field the whole continuity story rests on.

*Alternative considered:* the Go SDK, to match `runtimes/ollama/`. Rejected:
the Go SDK requires the `copilot` CLI installed separately, while the Node one
bundles it — and runtimes are their own components, so the language buys
nothing here.

### D2 — `runtimeContextId` is a Copilot session id we mint

On a work unit with no `runtimeContextId`, the runtime generates one
(`crypto.randomUUID()`), passes it as `sessionId`, and reports it on
`/work/done` with `continuity: new`. On a unit carrying one, it calls
`resumeSession(id)` and reports `continuity: continued`.

Opaque stays opaque in both directions: the manager never parses it, and the
runtime never encodes the conversation name into it. Encoding the conversation
would make the id derivable, which sounds convenient and quietly re-introduces
write-once semantics — a lost context could no longer be replaced by a
different handle, which is exactly the failure `latest-wins` exists to undo.

### D3 — Continuity failure is distinguished the same way, and fails the same way

Mirrors the ladder both existing runtimes implement:

1. `resumeSession(id)` throws `Session not found: <id>` (verified) → re-check `$HOME/.copilot/session-state/<id>/`
   after 500ms/1.5s/3s. A directory that reappears means the store was SLOW,
   not empty: retry once. An unreadable path is NOT absent — treat it as
   present and retry.
2. Still absent → the context is genuinely gone. FAIL the run with
   `continuity: unavailable`, a `continuityReason` naming the context volume,
   and the same NON-EMPTY user-facing text the other runtimes use — never an
   empty result, never a fresh session presented as a continuation.
3. A run that legitimately ends in a different session reports the new handle;
   `latest-wins` in the manager records it.

`contextStorage: volume` — session state lives under `$HOME`, which is
`/data/context`: the CONTEXT volume, or the pod-local copy `context-sync`
restores before the first `/work` is answered and checkpoints before
`/work/done` reaches the manager. The bundle declares
`contextSync.paths: [".copilot/session-state/**"]`, because only the runtime
knows its backend's layout; a route with no durable claim gets the
unsynchronised pod and is told its context is not promised by the existing
rule. The storage breaker above all this is the manager's and needs nothing
from here.

### D4 — Vocabulary translation lives in the runtime, in TWO layers

`MCPToolset` patterns stay opaque and claude-flavoured cluster-wide. The copilot
runtime maps them, and the mapping is not one list because Copilot splits what
claude-code fuses: *availability* (which tools exist for the session) and
*permission* (whether a given invocation is approved).

| Bound pattern | `availableTools` | `onPermissionRequest` |
|---|---|---|
| `Read` | `builtin:view` | approve |
| `Grep` | `builtin:grep` | approve |
| `Glob` | `builtin:glob` | approve |
| `Edit` | `builtin:edit` | approve |
| `Write` | `builtin:create` | approve |
| `Bash` | `builtin:bash` | approve every command |
| `Bash(kubectl:*)` | `builtin:bash` | approve only invocations whose command matches `kubectl *` |
| `mcp__<server>__<tool>` | `mcp:<server>-<tool>` | approve |
| anything unmapped | — (withheld) | deny |

PINNED against SDK 1.0.11 / CLI 1.0.80 on 2026-08-27, by advertising
`builtin:*` to a fake model endpoint and reading the request: the runtime
registers `bash read_bash stop_bash list_bash view create edit web_fetch skill
sql read_agent list_agents write_agent grep glob task`. Six are mapped; the
rest — sub-agents, skills, web fetch, the SQL scratchpad, the background-shell
companions — have no agent-ops pattern and are never available. A wrong name
would still be logged at session start (Risks), because the inventory is a
vendor fact that moves.

The permission handler answers `{kind: "reject"}` to deny and
`{kind: "approve-once"}` to allow — the runtime refuses `deny` as a malformed
payload and FAILS the tool call, which reads as a denial but is an error.

Three consequences worth stating outright:

- **Unmapped denies.** A pattern the mapper does not understand is logged once
  and contributes nothing. Passing it through would hand Copilot a string it
  reads as some other tool; dropping it silently would narrow (or, for a
  wildcard, widen) a route with no record that it happened. This is
  `builtin-toolset-catalog`'s report-what-you-cannot rule, applied at a
  vocabulary boundary.
- **`mcp__<server>__*` is REFUSED, not widened.** Copilot's tool filters admit
  `mcp:*` (every server) or an exact wire name — there is no per-server
  wildcard. Mapping a per-server wildcard to `mcp:*` would grant every other
  MCP server bound to that conversation. This is why the `kubernetes` bundle
  enumerates its toolsets instead of wildcarding.
- **A narrowing specifier is HONOURED here and GRANTS NOTHING on ollama**, and
  both are correct: `runtimes/ollama/` has no per-invocation hook and says so;
  this runtime has one and uses it. What a runtime can enforce is that runtime's
  fact, stated on its own page — not a contract-wide rule that flattens to the
  weakest implementation.

*Alternative considered:* ship Copilot-flavoured `MCPToolset`s from the chart
and let the runtime pass through verbatim. Rejected: it doubles the catalog,
and every Pipeline then has to know which vendor serves it — turning a routing
decision into a vendor decision.

### D5 — The definition path is the runtime's fact; the "declares nothing" rule is not

The copilot runtime reads `.github/agents/<agent>.agent.md` (Copilot's
location), parses the same frontmatter shapes `runtimes/claude/tools.js`
handles, and composes with `toolsMode` identically. Absent file, absent
frontmatter, absent `tools:`, unparseable frontmatter → contributes NOTHING,
logged, never fatal.

Copilot's own default — an omitted `tools:` means every tool — is neutralised at
the boundary: the runtime always passes an explicit `availableTools` (possibly
`[]`) and never lets the definition's default apply. `[]` means no tool is
available, which is the correct reading of a composition that produced nothing.

Copilot's own discovery of `.github/agents/` is OFF by default
(`enableConfigDiscovery: false`, verified: a definition in the working directory
left no trace in the session) and the runtime never turns it on. Nor does it
load custom instructions — `skipCustomInstructions: true` — since the profile's
prompt is the whole of the agent's instructions here and `AGENTS.md` or
`CLAUDE.md` in a checkout must not silently join it.

The definition is read and composed BY US, and never handed to Copilot as a
`customAgents` entry with `agent:` selected. That would re-apply the
definition's tools as an availability intersection and silently defeat
`overwrite` — the same trap `--agent` sets on the claude side. Task 1 also
confirms the SDK does not pick up `.github/agents/` on its own when a
`workingDirectory` contains one; if it does, that discovery is disabled.

### D6 — MCP config is translated, and `${VAR}` is expanded here

`$MCP_CONFIG` (written by `internal/mcpcompile`) maps field-for-field onto the
SDK's `mcpServers` record: `{command,args,env}` → stdio, `{type:http,url,headers}`
→ http. HTTP servers are reached through the egress proxy's redirect exactly as
any other client in the pod is; the runtime does nothing to earn that and
nothing to defeat it.

One thing does not carry: the manager writes secret-backed values as `${ENV}`
placeholders and relies on claude-code expanding them. The SDK takes literal
strings, so the copilot runtime expands `${VAR}` from `process.env` itself —
in-process, never logged, and an unresolvable placeholder fails the server's
registration loudly rather than reaching an MCP server as the literal text
`${TOKEN}`.

### D7 — Role text is prepended, not turned into a custom agent

`unit.systemPrompt` (an inline role from a repo-less profile) has no SDK
equivalent to `--append-system-prompt`. It is prepended as a delimited block to
the FIRST prompt of a session, not re-sent on resume, where it is already in the
transcript. The alternative — a single `customAgents` entry carrying it as
`prompt` — drags D5's intersection problem in for a system prompt's sake.

### D8 — `maxTurns` has no equivalent and is not faked

The unit's `maxTurns` bounds a claude run and an ollama loop; Copilot's nearest
control is `sessionLimits.maxAiCredits`, a credit budget, not a turn count. The
runtime logs the requested value and does not pretend to enforce it; an optional
`COPILOT_MAX_AI_CREDITS` env sets a real budget cap for operators who want a
hard ceiling. Mapping turns onto credits would put a made-up number in the one
place an operator would trust as a limit.

### D9 — The chart ships a `copilot` bundle, and the substrate stays the parent's

`chart/charts/copilot/` in the exact shape of `chart/charts/ollama/`:
`enabled: false`, `name: copilot`, `default: false`, `image`,
`credentialsSecret{name,key,envName: COPILOT_GITHUB_TOKEN,token}`,
`contextSync.paths: [".copilot/session-state/**"]`, optional `model` and
`maxAiCredits` becoming env through an `agentops.copilotRuntimeEntry` helper,
and any `runtimeDefaults` key as a per-runtime override. The CR renders through
the parent's `agentops.renderRuntime` so it cannot drift from a hand-declared
one, and the bundle is added to the hand-listed `agentops.declaredRuntimes`
range — the default-runtime guard and the manager's bootstrap env see only what
that list names, and ollama's first render passed every test but that guard.

It ships NO substrate: no defaults, no floor account, no context volume, no
identity of its own. A route selects it with `pipelines[].runtimeRef: copilot`,
and picks its identity and storage on the same object as every other route.

*Alternative considered:* a `runtimes:` entry in the parent's values, no
bundle. Rejected: the adopter would type the vendor's env name, secret key and
context paths by hand — three facts the runtime already holds, and the third
one inert when mistyped. The `runtimes:` list stays the path for a runtime this
project does not ship.

## Risks / Trade-offs

- **Copilot's built-in tool wire names are read off SDK source, not a
  published catalog** → the mapper validates itself at session start against
  the tools the runtime actually registered, and logs any mapping whose target
  is not registered. A wrong name shows up as a log line on the first run
  instead of a tool that silently never appears.
- **Two vocabularies now exist even though only one is written down** → the
  mapping table is spec'd and unit-tested, and unmapped-denies means the failure
  mode is a visibly missing tool, never a surprise grant.
- **Shell sub-patterns are enforced by our own matcher** (`Bash(kubectl:*)` in a
  permission callback) → conservative matching: match on the resolved command's
  first word plus prefix, deny anything with shell metacharacters that could
  smuggle a second command. Deny is the safe direction, and every denial is
  logged with the pattern that failed to match.
- **The role text is not re-sent on resume** → if long conversations drift,
  D7 is cheap to change to per-turn prepending; recorded as an open question.
- **No enforced turn cap** (D8) → a runaway loop is bounded by the idle TTL, the
  conversation cap, and optionally `COPILOT_MAX_AI_CREDITS`.
- **A third image to keep patched, and it carries a bundled CLI** → it is a
  reference implementation sharing no code with the manager, and the Trivy gate
  blocks a fixable CRITICAL/HIGH the way it does for every image. A stale
  copilot image cannot break a claude or ollama install.
- **The SDK's session-state layout is a vendor fact** → `contextSync.paths` is
  pinned by task 1 and lives in the bundle beside the image, so a layout change
  is a bundle edit and a runtime tag, never a release-wide default.

## Migration Plan

Additive throughout. `copilot.enabled` defaults to `false`, so an existing
release renders byte-identically. Adopting the runtime is: enable the bundle
with a GitHub token, then point one `Pipeline.spec.runtimeRef` at `copilot` —
its profile, toolsets and channels do not change, which is the claim this whole
change exists to demonstrate. Rolling back is pointing `runtimeRef` back;
conversations mid-flight keep their handle, and their next run reports a fresh
one under the old vendor via `latest-wins`.

## Open Questions

- Does role drift show up over long resumed conversations under D7?
- Should the runtime surface Copilot's usage/credit events into the run result,
  or is that telemetry the activity contract should carry instead?
- Which Copilot model default belongs in the bundle (`auto` vs a pinned id),
  given that a pinned one ages and `auto` makes cost less predictable?
