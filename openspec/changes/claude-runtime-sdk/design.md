## Context

`runtime-claude/runtime.js` polls `/work`, and for each unit spawns
`claude -p … --resume <contextId>` with a set of flags, parsing `stream-json` off
stdout. One process per work unit.

That shape has two costs the current chart defaults hide. A pod handles several
units in its life — the serial rule is per conversation, not per pod — and every
one of them pays process start, transcript replay from
`$HOME/.claude/projects/`, and a fresh launch of every bound MCP server. The
replay grows with the conversation. And `runtimeIdleTtlMinutes: 1` is only the
value we ship: `AgentRuntime.spec.idleTtlMinutes` is per-runtime, so an adopter
running ten minutes or hours turns "several units per pod" into the normal case.

Two things constrain any redesign, and they pull against session reuse:

- **REFS are snapshotted, CONTENT is not.** Every use re-reads the CRs, so an
  edit to a toolset, an MCPConfig or a profile heals a RUNNING conversation. A
  work unit's `allowedTools`, `toolsMode`, `agent`, `systemPrompt` and MCP config
  are therefore per-unit values that CAN differ between two units of the same
  conversation.
- **Continuity is promised, and a lost context FAILS the run.** Whatever the
  runtime keeps in memory, `runtimeContextId` on the CR remains the only thing
  that survives the pod.

## Goals / Non-Goals

**Goals:**
- Remove per-unit re-spawn and transcript re-replay for every unit after the
  first in a pod's life.
- Keep MCP servers connected across units instead of relaunching them per unit.
- One permission model across reference runtimes: a per-invocation callback,
  deny-by-default, that cannot prompt.
- Behave correctly at both ends of the TTL dial — a one-minute pod and an
  hours-long one.

**Non-Goals:**
- No change to the work contract, the manager, any CRD, or the chart.
- No change of vendor, credential, or session-file location — the SDK bundles the
  same Claude Code binary the CLI path ran.
- Not a performance project. The win is per-unit setup, not model latency, and
  the design is judged on continuity correctness first.

## Decisions

### D1 — Drive the SDK in-process; keep the CLI's vendor, files and credential

`@anthropic-ai/claude-agent-sdk`'s `query()` replaces `spawn('claude', …)`. The
SDK bundles the same binary, writes the same session files under
`$HOME/.claude/projects/<encoded-cwd>/`, and resolves auth the same way — so
`contextStorage: volume`, the home-volume requirement, and
`CLAUDE_CODE_OAUTH_TOKEN` all carry over untouched.

What we stop doing is parsing `stream-json` and encoding permission semantics as
flags. What we gain is `canUseTool` (D3) and streaming input (D2).

*Alternative considered:* keep the CLI and reuse the process by feeding it
successive prompts. Rejected: the CLI's `-p` mode is one prompt per invocation;
there is nothing to feed.

### D2 — One live session per pod, fed by streaming input; the poll loop is the generator

The runtime opens `query({ prompt: <async generator>, options })` and holds it.
Each work unit yields one `SDKUserMessage` into the generator and the runtime
reads the message stream until that turn's `result`, which is the unit's outcome.
Because a conversation is strictly serial, exactly one unit is ever in flight, so
"read until the next result" is unambiguous.

The poll loop keeps its current shape — long-poll, execute, report, idle-TTL exit.
The only change is that "execute" pushes into an open session instead of
spawning one.

### D3 — `canUseTool` decides every call, and `dontAsk` is therefore unusable

The composed allowlist (`tools.js`, unchanged) becomes the callback's rule set
rather than an `allowedTools` array. Three consequences, each load-bearing:

- **Pass no `allowedTools`.** A bare entry auto-approves that tool BEFORE the
  callback is consulted — the SDK warns about exactly this shadowing. An
  allowlist that never reaches our decision point is not our allowlist.
- **`permissionMode` stays `default`.** `dontAsk` denies without calling
  `canUseTool` at all, so it cannot host a rule set. The no-prompt guarantee comes
  from the callback ALWAYS answering, not from the mode.
- **Deny by default**, and an empty composition denies everything — the same rule
  as today, now expressed where it is enforced rather than as an empty flag.

This is the shape `runtime-copilot` needs for its own per-invocation matching, so
the two reference runtimes converge on one permission story.

*Alternative considered:* a `PreToolUse` hook, which runs before every step and
cannot be shadowed. Rejected for now: it is the stronger guarantee, but it moves
the decision into a second mechanism with its own contract. Worth revisiting if
shadowing ever bites.

### D4 — The session is reused ONLY when the unit's configuration is identical

This is the decision that keeps session reuse from becoming a correctness bug.

A live session pins `systemPrompt`, `mcpServers`, `maxTurns`, `cwd` and the agent
at open time. A conversation's next unit may legitimately carry different ones,
because content is re-read on every use and an operator edit is SUPPOSED to heal
a running conversation. Reusing a session across such a change would apply the
old capabilities while the CR says otherwise — the exact failure the
snapshot rule exists to prevent.

So the runtime fingerprints each unit's session-affecting configuration and:

- **identical fingerprint** → push the prompt into the open session;
- **different fingerprint** → close the session and open a new one with
  `resume: <runtimeContextId>`, which is precisely today's behaviour.

The tool allowlist is deliberately NOT in the fingerprint: it lives in the
callback, so it can change between units with no restart. That is the one piece
of wiring that heals without paying for a re-open.

### D5 — The live session is a cache; the recorded handle stays authoritative

Every unit reports the session id from the SDK (`session_id` on the init system
message, and on the result) — including when it was served by the open session,
because a fork or a vendor-side branch must reach `latest-wins`.

A pod with no live session (fresh, evicted, rescheduled, or one whose process
died) resumes from `runtimeContextId`. The manager cannot tell the two apart, and
nothing in the work contract describes which happened. The continuity ladder from
`runtime-claude` today — re-check the session file, unreadable is not absent,
confirmed absence FAILS the run with a non-empty explanation — is unchanged and
applies to the re-open path.

### D6 — Long-lived sessions own two things short-lived ones never did

- **MCP servers stay up for the pod's life.** Under a one-minute TTL nothing had
  time to expire or leak; under hours, credentials expire and child processes
  accumulate. The runtime tears the session down (and with it its servers) on
  idle-TTL exit, on fingerprint change, and on unrecoverable session error —
  never leaving a session open across a failure it did not understand.
- **A process that dies mid-turn** loses the in-memory session and nothing else.
  The unit is reported failed, the handle recorded stands, and the next unit
  resumes from disk.

Conversation context is NOT one of these: `--resume` replays the whole
transcript, so context grows identically whether the session was held open or
re-opened. Nothing about context differs between the two designs.

### D7 — The transcript keeps its lines

Typed SDK messages replace hand-parsed `stream-json`, but the rendering does not
change: `[init]`, `[claude]`, `[tool]`, `=== RESULT ===`. Those lines are what a
human reads in VictoriaLogs, and a runtime rewrite is not a reason to relearn
them. The reported `result` keeps its 2000-character cap and the same
`status`/`exitCode`/`continuity` meanings.

## Risks / Trade-offs

- **Subscription auth is inherited, not documented.** The SDK's quickstart names
  `ANTHROPIC_API_KEY` and the cloud providers; `CLAUDE_CODE_OAUTH_TOKEN` works
  because the bundled binary reads it, exactly as today. Anthropic also states
  that third-party developers may not OFFER claude.ai login or rate limits in
  their products — which reads as out of scope for an operator running its own
  credential, since every adopter brings their own. → First task is a smoke test
  on the subscription token before any rewrite; if it fails, the change stops at
  that task rather than after the rewrite.
- **The bundled binary ships via npm optional dependencies**, so an image built
  with `npm ci --omit=optional` silently has no Claude Code and fails at runtime.
  → The Dockerfile installs without omitting optionals and the build verifies the
  binary is present.
- **A wrong fingerprint (D4) is a silent capability bug**, not a crash: the
  conversation would keep running under superseded wiring. → The fingerprint is
  unit-tested field by field, and a re-open is logged with the field that
  changed, so the reason is visible in the pod log.
- **A held session is process state.** It is a cache by construction (D5), but a
  bug that lets it diverge from the recorded handle would be hard to see. → Every
  unit reports the id the SDK gives it, so divergence surfaces as a changed
  handle rather than as silence.
- **Two vendors, one permission shape, different enforcement points.** Copilot
  matches per invocation in its own callback; claude does it in `canUseTool`. The
  shared thing is the rule set, not the code — resist extracting a shared module
  across modules that are deliberately dependency-free.

## Migration Plan

The image is the whole migration: build a new `agentops-runtime-claude` tag and
point `AgentRuntime.spec.image` at it. No CR, chart value or manager version has
to move with it, and rolling back is the previous tag. Conversations in flight
keep their handle either way — a pod on the old image and a pod on the new one
both resume from `runtimeContextId`.

## Open Questions

- Does an hours-long session accumulate MCP child processes in practice, or does
  the SDK reap them? Measure before adding machinery for it.
- Should a fingerprint change fork (`forkSession`) rather than resume, when the
  change is a capability narrowing? Resume is the conservative default here.
- Is `PreToolUse` (D3's rejected alternative) worth adopting later as a
  belt-and-braces layer that cannot be shadowed?
