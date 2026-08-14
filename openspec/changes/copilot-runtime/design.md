## Context

`AgentRuntime` has always promised a pluggable backend — `spec.image` plus the
four-step work contract — and exactly one image has ever implemented it
(`runtime-claude/`, Node + claude-code). Everything vendor-shaped in the contract
was therefore written by looking at one vendor: `runtimeContextId` is an opaque
handle *because we said so*, not because a second backend ever tested it;
`allowedTools` carries claude-code's vocabulary; the agent definition lives at
`.claude/agents/<agent>.md` because that is where claude-code looks.

GitHub Copilot is a good second backend precisely because it disagrees on all
three: a different credential (`COPILOT_GITHUB_TOKEN`), a different tool
vocabulary (`bash`, `view`, `grep`; MCP tools as `mcp:<server>-<tool>`), a
different definition path (`.github/agents/<agent>.agent.md`) — and one inverted
default that matters: in Copilot an agent definition with no `tools:` gets ALL
tools, where agent-ops means "declares nothing".

The Copilot SDK (`@github/copilot-sdk`, Node, bundles the CLI) also lets the
CALLER supply the session id. That makes the copilot runtime the first backend
where `runtimeContextId` is a handle we mint rather than one we scrape back out
of a vendor's stdout — which is what the field always claimed to be.

Constraints carried in unchanged: self-contained module, no dependencies outside
this directory; generic by construction (git + shell, no domain tooling); the
manager reads no Secrets; the manager composes meaning, the runtime renders
nothing transport-shaped; `/data/workspace` and `/data/home` are mount points.

## Goals / Non-Goals

**Goals:**
- A second reference runtime that passes the same work contract, proving the
  contract is vendor-neutral by making it survive a vendor that disagrees.
- One toolset vocabulary cluster-wide. A Pipeline binds the same `MCPToolset`
  CRs whichever runtime serves it; the vendor difference is absorbed at the
  boundary where vendor knowledge already lives.
- Same safety posture as `runtime-claude`: empty allowlist means empty, no
  permission prompt can hang a pod, and a conversation whose context is gone
  FAILS rather than answering as if it were a new one.
- Chart support that keeps the substrate parent-owned: one runtime SA, one RBAC
  mode, one home volume, N vendor runtimes.

**Non-Goals:**
- No CRD change. `AgentRuntime` already describes this runtime completely.
- No manager change. Dispatch, ingest, capability resolution and the work unit
  are untouched.
- Not a trust boundary. A second runtime is a second VENDOR; a second trust
  level still means a second ServiceAccount, which stays a hand-written CR.
- No Copilot-specific `MCPToolset`s, no vocabulary field on any CR, no
  runtime-kind discriminator anywhere in `internal/`.

## Decisions

### D1 — Drive Copilot through the SDK, not the `copilot` CLI

`@github/copilot-sdk` in-process, not `spawn('copilot', …)`.

The deciding factor is the context handle. `createSession({sessionId})` accepts
an id we choose; `resumeSession(id)` continues it; state lands at
`~/.copilot/session-state/<id>/`. The CLI cannot be told the id of a NEW session
(github/copilot-cli#442) — the documented workaround is to diff
`~/.copilot/session-state/` after the run and guess which directory is ours.
That is a race under concurrent pods and a lie under any failure that writes a
directory without finishing a run.

Everything else the contract needs is a first-class option rather than a flag
scrape: `availableTools`/`excludedTools` (allowlist), `onPermissionRequest`
(per-invocation decisions), `mcpServers`, `workingDirectory`, `streaming: true`
with typed events for the stdout transcript, `sessionLimits`.

*Alternative considered:* the CLI, for symmetry with `runtime-claude`'s
`spawn('claude')`. Rejected: the symmetry is cosmetic and the cost is the one
field the whole continuity story rests on.

*Alternative considered:* the Go SDK, to reuse this repo's primary language.
Rejected: the Go SDK requires the `copilot` CLI installed separately, while the
Node one bundles it — and runtimes are their own modules, so the language buys
nothing here.

### D2 — `runtimeContextId` is a Copilot session id we mint

On a work unit with no `runtimeContextId`, the runtime generates one
(`crypto.randomUUID()`), passes it as `sessionId`, and reports it on
`/work/done`. On a unit carrying one, it calls `resumeSession(id)`.

Opaque stays opaque in both directions: the manager never parses it, and the
runtime never encodes the conversation name into it. Encoding the conversation
would make the id derivable, which sounds convenient and quietly re-introduces
write-once semantics — a lost context could no longer be replaced by a different
handle, which is exactly the failure `latest-wins` exists to undo.

### D3 — Continuity failure is distinguished the same way, and fails the same way

Mirrors `runtime-claude`'s ladder deliberately:

1. `resumeSession(id)` throws → re-check `~/.copilot/session-state/<id>/` after
   500ms/1.5s/3s. A directory that reappears means the store was SLOW, not empty
   (a share-manager restart, a stale handle after the pod moved): retry once.
   An unreadable path is NOT absent — treat it as present and retry.
2. Still absent → the context is genuinely gone. FAIL the run with the same
   user-facing text `runtime-claude` uses, never an empty result, and never a
   fresh session presented as a continuation.
3. A run that legitimately ends in a different session reports the new handle;
   `latest-wins` in the manager records it.

`contextStorage: volume` — Copilot's session state lives under `$HOME`, so the
home volume decides whether continuity is possible here, exactly as for
claude-code.

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
| `Write` | `builtin:write` | approve |
| `Bash` | `builtin:bash` | approve every command |
| `Bash(kubectl:*)` | `builtin:bash` | approve only invocations whose command matches `kubectl *` |
| `mcp__<server>__<tool>` | `mcp:<server>-<tool>` | approve |
| anything unmapped | — (withheld) | deny |

Two consequences worth stating outright:

- **Unmapped denies.** A pattern the mapper does not understand is logged once
  and contributes nothing. Passing it through would hand Copilot a string it
  reads as some other tool; dropping it silently would narrow (or, for a
  wildcard, widen) a route with no record that it happened.
- **`mcp__<server>__*` is REFUSED, not widened.** Copilot's tool filters admit
  `mcp:*` (every server) or an exact wire name — there is no per-server
  wildcard. Mapping a per-server wildcard to `mcp:*` would grant every other MCP
  server bound to that conversation. This is why `k8s-bundle` enumerates its
  toolsets instead of wildcarding, and that decision now pays for itself.

*Alternative considered:* ship Copilot-flavoured `MCPToolset`s from the chart
and let the runtime pass through verbatim. Rejected by the adopter: it doubles
the catalog, and every Pipeline then has to know which vendor serves it —
turning a routing decision into a vendor decision.

### D5 — The definition path is the runtime's fact; the "declares nothing" rule is not

The copilot runtime reads `.github/agents/<agent>.agent.md` (Copilot's location),
parses the same frontmatter shapes `runtime-claude/tools.js` handles, and
composes with `toolsMode` identically. Absent file, absent frontmatter, absent
`tools:`, unparseable frontmatter → contributes NOTHING, logged, never fatal.

Copilot's own default — an omitted `tools:` means every tool — is neutralised at
the boundary: the runtime always passes an explicit `availableTools` (possibly
`[]`) and never lets the definition's default apply. `[]` means no tool is
available, which is the correct reading of a composition that produced nothing.

The definition is read and composed BY US, and never handed to Copilot as a
`customAgents` entry with `agent:` selected. That would re-apply the definition's
tools as an availability intersection and silently defeat `overwrite` — the same
trap `--agent` sets on the claude side, one CLI over.

### D6 — MCP config is translated, and `${VAR}` is expanded here

`/etc/agentops/mcp.json` (written by `internal/mcpcompile`) maps field-for-field
onto the SDK's `mcpServers` record: `{command,args,env}` → stdio,
`{type:http,url,headers}` → http.

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

The unit's `maxTurns` bounds a claude run; Copilot's nearest control is
`sessionLimits.maxAiCredits`, a credit budget, not a turn count. The runtime
logs the requested value and does not pretend to enforce it; an optional
`COPILOT_MAX_AI_CREDITS` env sets a real budget cap for operators who want a
hard ceiling. Mapping turns onto credits would put a made-up number in the one
place an operator would trust as a limit.

### D9 — The chart gains `additionalRuntimes`, and the substrate stays singular

`runtime:` keeps rendering the default runtime exactly as today. A new
`additionalRuntimes: []` renders one more `AgentRuntime` per entry — `name`,
`image`, `credentialsSecret{name,key,envName,token?}`, `idleTtlMinutes`,
`nodeSelector`, `resources`, `contextStorage` — reusing the same helpers for the
runtime SA and the home/workspace claims, so a second vendor cannot disagree
with the parent about the substrate.

Deliberately NOT configurable per entry: the ServiceAccount. Letting an entry
name its own SA would make "add a runtime" a privilege-escalation path through
values, which is the same reason a Pipeline cannot choose one.

*Alternative considered:* a `runtimes:` list replacing the singular `runtime:`.
Rejected: it breaks every existing values file for a cosmetic gain.

## Risks / Trade-offs

- **Copilot's built-in tool wire names are read off docs and SDK source, not a
  published catalog** → the mapper validates itself at session start against the
  tools the runtime actually registered, and logs any mapping whose target is
  not registered. A wrong name then shows up as a log line on the first run
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
  D7 is cheap to change to per-turn prepending; recorded as an open question
  rather than pre-solved.
- **No enforced turn cap** (D8) → a runaway loop is bounded by the idle TTL, the
  conversation cap, and optionally `COPILOT_MAX_AI_CREDITS`. Same class of
  exposure the `maxTurns`-less path already has for any runtime that ignores it.
- **A second image to keep patched** → it is a reference implementation, sharing
  no code with the manager; a stale copilot image cannot break a claude install.

## Migration Plan

Additive throughout. `additionalRuntimes` defaults to `[]`, so an existing
release renders byte-identical output. Adopting the runtime is: add an entry
with the image and a GitHub token, then point one `AgentProfile.runtimeRef` at
it — the Pipeline, its toolsets and its channels do not change, which is the
claim this whole change exists to demonstrate. Rolling back is pointing
`runtimeRef` back; conversations mid-flight keep their handle, and their next
run reports a fresh one under the old vendor via `latest-wins`.

## Open Questions

- Does role drift show up over long resumed conversations under D7?
- Should the runtime surface Copilot's usage/credit events into the run result,
  or is that telemetry the activity contract should carry instead?
- Which Copilot model default belongs in the image (`auto` vs a pinned id), given
  that a pinned one ages and `auto` makes cost less predictable?
