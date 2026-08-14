## Why

`runtime-claude` drives claude-code by spawning the CLI once per work unit. That
was the right first implementation, and two things have made it the wrong one
now.

**It re-establishes everything, every unit.** A runtime pod handles more than one
work unit in its life — queued inputs, a fast back-and-forth, a burst on one
conversation — and each of those spawns `claude` afresh with `--resume`, which
replays the conversation transcript off disk and re-launches every bound MCP
server. That cost is not constant: it grows with the conversation, so it is worst
exactly where the conversation is most valuable. Nothing about the current design
lets a second unit land in a session that is already open.

**And the TTL is the adopter's dial, not ours.** The chart ships
`runtimeIdleTtlMinutes: 1`, but `AgentRuntime.spec.idleTtlMinutes` is per-runtime,
and a pod is cheap: an adopter may well run ten minutes, or hours, for an agent
with expensive startup. At that end of the range a live session is not an
optimisation — it is the normal path, and cold resume is the exception. The
runtime should be built for both ends of that dial rather than for the one-minute
end we happen to ship.

Separately, the CLI's permission surface is a set of flags with rules we have had
to reverse-engineer and document (`--allowedTools` is the sole authority; never
pass `--agent`; `--permission-mode dontAsk` because a prompt in a pod is a hang).
The SDK exposes the same decisions as a `canUseTool` callback — the same shape
`runtime-copilot` needs, so the two reference runtimes end up with ONE permission
story instead of two.

## What Changes

- **`runtime-claude` drives `@anthropic-ai/claude-agent-sdk` in-process** instead
  of spawning the `claude` CLI per work unit. The bundled binary is the same one
  the CLI path used, so the vendor, its session files and its credential do not
  change.
- **A session is held open across work units within one pod**, via the SDK's
  streaming-input mode: the runtime pushes the next unit's prompt into the live
  session rather than starting a new process. Re-spawn and transcript re-replay
  disappear for every unit after the first, and bound MCP servers stay connected.
- **The live session is a CACHE, never the source of truth.** A pod that has just
  started, was evicted, was rescheduled, or lost its process resumes from
  `runtimeContextId` exactly as today. Nothing the manager sends or receives
  changes, and the work contract is untouched.
- **Permissions move from flags to `canUseTool`**, evaluated against the composed
  allowlist per invocation, with the empty-allowlist rule and the no-prompting
  rule preserved. The callback answers EVERY unresolved call and denies by
  default, which is what makes prompting impossible — `dontAsk` cannot be the
  backstop here, because that mode skips the callback entirely.
- **The transcript keeps its current shape.** Typed SDK messages replace
  `stream-json` parsing, and the pod log keeps the same `[init]` / `[claude]` /
  `[tool]` / `=== RESULT ===` lines, because they are what a human reads in
  VictoriaLogs.
- **Long-lived sessions get the handling short-lived ones never needed**: MCP
  servers alive for hours (credential expiry, leaked children) and a process that
  dies mid-session, which must fall back to disk resume without the manager
  seeing anything different.

Not in scope: no CRD field, no manager change, no chart change, no change to the
credential or to which vendor runs. `runtime-copilot` is a separate change; the
only thing shared is the permission-callback shape.

## Capabilities

### New Capabilities
- `runtime-session-reuse`: that a runtime MAY hold a vendor session open across
  work units within one pod, what that obliges it to preserve (the recorded
  context handle stays authoritative, a cold pod is indistinguishable to the
  manager), and what a long-lived session must handle that a short-lived one
  never did.

### Modified Capabilities
- `agent-definition-tools`: the composed allowlist is currently expressed as
  something the runtime PASSES to its vendor. It SHALL be expressible as a
  per-invocation DECISION as well, so a runtime whose vendor offers a permission
  callback satisfies the same rules — empty means empty, and nothing may block on
  a prompt — without a flag that happens to spell them.

## Impact

- **Changed**: `runtime-claude/` (`runtime.js` rewritten around `query()`;
  `tools.js` composition logic unchanged), `runtime-claude/Dockerfile`
  (`@anthropic-ai/claude-agent-sdk` replaces the global `@anthropic-ai/claude-code`
  install), a new `agentops-runtime-claude` image tag, `docs/contracts.md`,
  `CLAUDE.md`.
- **Dependencies**: `@anthropic-ai/claude-agent-sdk` (bundles the Claude Code
  binary; Node 18+, the image is Node 22). Auth is unchanged — the bundled binary
  reads `CLAUDE_CODE_OAUTH_TOKEN`, which is what the chart already injects.
- **Unchanged**: `api/v1alpha1/`, `internal/`, `chart/`, every adapter module,
  and the work contract itself.
