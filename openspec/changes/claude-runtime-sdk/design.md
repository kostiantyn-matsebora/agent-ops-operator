## Context

`runtimes/claude/runtime.js` polls `/work`, and for each unit spawns
`claude -p … --resume <contextId>` with a set of flags, parsing `stream-json` off
stdout. One process per work unit.

That shape has three costs the current chart defaults, and the current
single-image shape, hide.

A pod handles several units in its life. The serial rule is per conversation,
not per pod. Every one of them pays process start, transcript replay from
`$HOME/.claude/projects/`, and a fresh launch of every bound MCP server. The
replay grows with the conversation.

`runtimeIdleTtlMinutes: 1` is only the value we ship.
`AgentRuntime.spec.idleTtlMinutes` is per-runtime, so an adopter running ten
minutes or hours turns "several units per pod" into the normal case.

And an image carrying both the CLI's global install and the SDK's bundled
binary is as heavy as the heavier of the two, for every adopter, regardless
of which one a given pod ever spawns.

**This change was first built, and a PR opened, as a single image with two
drivers selected by an env var (PR #424, now closed).** That design is
preserved almost intact below — D2 through D7 barely change.

D1 changes completely: the CLI and SDK become two separate `runtimes/`
components, not two code paths behind one Dockerfile.

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
  first in `runtimes/claude`'s pod life.
- Keep MCP servers connected across units in `runtimes/claude`, instead of
  relaunching them per unit.
- One permission model across reference runtimes: a per-invocation callback,
  deny-by-default, that cannot prompt.
- `runtimes/claude` behaves correctly at both ends of the TTL dial — a
  one-minute pod and an hours-long one.
- Keep today's CLI-driven behaviour fully available, as its own component
  (`runtimes/claude-cli`), not folded into the SDK image.
- `runtimes/claude`'s image is measurably lighter once the CLI install is
  gone — no global `@anthropic-ai/claude-code`, no installer step.
- Stay fully conformant with `docs/contracts/runtime.md` throughout, on both
  components — every `API-n`, `LC-n`, `RT-n`, `MGR-n` and `SEC-n` requirement
  holds exactly as it does today.

**Non-Goals:**
- No change to the work contract, the manager, or any CRD.
- No change of vendor, credential, or session-file location. The SDK bundles
  the same Claude Code binary the CLI path ran.
- Not a performance project. The win is per-unit setup, not model latency, and
  the design is judged on continuity correctness first.
- No publication of session reuse as a new optional capability on
  `docs/contracts/runtime.md`. It stays `runtimes/claude`'s own internal
  behaviour, not a contract promise to third-party runtime builders.
- This excludes RT-3's own clarification (D3) — that one DOES amend the
  published spec, since it tells every runtime builder which enforcement
  mechanisms satisfy an existing obligation. The two are separate decisions.
- Not a change to which bundle is chart-default. `runtimes/claude` stays the
  on-by-default, default-eligible bundle. `runtimes/claude-cli` ships off,
  the same as `ollama` and `copilot`.

## Decisions

### D1 — Two components, not two drivers behind one image

The earlier shape of this change built both drivers into `runtimes/claude/`'s
single image, selected by `RUNTIME_CLAUDE_DRIVER` on `AgentRuntime.spec.env`.
That kept the choice reversible with no image swap, at the cost of every
pulled image carrying both vendors' weight.

This redesign drops that trade. Two separate, self-contained components:

**`runtimes/claude/` becomes SDK-only.** `runtime.js` is rewritten around
`@anthropic-ai/claude-agent-sdk`'s `query()`, replacing `spawn('claude', …)`.
The Dockerfile drops the global `@anthropic-ai/claude-code` install and its
installer step, keeping only the SDK and the binary it bundles.

The SDK writes the same session files under
`$HOME/.claude/projects/<encoded-cwd>/`, and resolves auth the same way.
`contextStorage: volume`, the context-volume requirement, and
`CLAUDE_CODE_OAUTH_TOKEN` all carry over untouched.

**`runtimes/claude-cli/` is a new directory.** Today's `runtime.js`,
`tools.js`, `spin.js`, `report.js` and their tests are COPIED into it
VERBATIM, with today's exact Dockerfile — no rewrite. The originals stay at
`runtimes/claude/`, where the SDK rewrite above happens in place.

No shared import exists between the two directories. `structure.md`'s
"BUILD CONTEXT IS THE DIRECTORY" rule applies here exactly as it does to
`runtimes/ollama` and `runtimes/copilot` already duplicating their own
tool-composition logic rather than sharing it.

Choosing one means choosing which `AgentRuntime` (and which chart bundle) a
Pipeline's `runtimeRef` names. There is no in-image switch.

What the SDK component stops doing is parsing `stream-json` and encoding
permission semantics as flags. What it gains is `canUseTool` (D3) and
streaming input (D2). Every decision from here on (D2–D7) describes
`runtimes/claude` only, unless a decision says otherwise.

*Alternative considered:* keep the single-image, env-var-selected shape (this
change's original PR #424). Rejected on the author's explicit direction — a
shared image cannot be light for an SDK-only adopter while it still carries
the CLI's install, whatever selects between them at runtime.

*Alternative considered:* keep only the CLI and reuse its process by feeding
it successive prompts. Rejected — the CLI's `-p` mode is one prompt per
invocation, and there is nothing to feed.

That is exactly why a second component is needed, rather than a change to
the existing one.

### D2 — One live session per pod, fed by streaming input

The poll loop is the generator. The runtime opens
`query({ prompt: <async generator>, options })` and holds it.

Each work unit yields one `SDKUserMessage` into the generator. The runtime
reads the message stream until that turn's `result`, which is the unit's
outcome. Because a conversation is strictly serial, exactly one unit is ever
in flight, so "read until the next result" is unambiguous.

The poll loop keeps its current shape — long-poll, execute, report, idle-TTL
exit. The only change is that "execute" pushes into an open session instead
of spawning one.

### D3 — `canUseTool` decides every call, and `dontAsk` is therefore unusable

The composed allowlist (`tools.js`, unchanged) becomes the callback's rule set
rather than an `allowedTools` array. Three consequences, each load-bearing:

- **Pass no `allowedTools`.** A bare entry auto-approves that tool BEFORE the
  callback is consulted — the SDK warns about exactly this shadowing. An
  allowlist that never reaches our decision point is not our allowlist.
- **`permissionMode` stays `default`.** `dontAsk` denies without calling
  `canUseTool` at all, so it cannot host a rule set. The no-prompt guarantee
  comes from the callback ALWAYS answering, not from the mode.
- **Deny by default**, and an empty composition denies everything — the same
  rule as today, now expressed where it is enforced rather than as an empty
  flag.

This is the shape `runtime-copilot` needs for its own per-invocation matching,
so the two reference runtimes converge on one permission story.

**`runtimes/claude-cli` is unaffected and already compliant.** It keeps
`--allowedTools` plus `--permission-mode dontAsk`, satisfying the same
requirement in its original flag form.

*Alternative considered:* a `PreToolUse` hook, which runs before every step
and cannot be shadowed. Rejected for now — it is the stronger guarantee, but
it moves the decision into a second mechanism with its own contract. Worth
revisiting if shadowing ever bites.

### D4 — The session is reused ONLY when the unit's configuration is identical

This is the decision that keeps session reuse from becoming a correctness
bug. It applies to `runtimes/claude` only — `runtimes/claude-cli` opens a
fresh process per unit, so it has no live session to reuse or misapply.

A live session pins `systemPrompt`, `mcpServers`, `maxTurns`, `cwd` and the
agent at open time. A conversation's next unit may legitimately carry
different ones, because content is re-read on every use and an operator edit
is SUPPOSED to heal a running conversation.

Reusing a session across such a change would apply the old capabilities
while the CR says otherwise — the exact failure the snapshot rule exists to
prevent.

So the runtime fingerprints each unit's session-affecting configuration:

- **identical fingerprint** → push the prompt into the open session.
- **different fingerprint** → close the session and open a new one with
  `resume: <runtimeContextId>`, which is precisely today's behaviour.

The tool allowlist is deliberately NOT in the fingerprint. It lives in the
callback, so it can change between units with no restart. That is the one
piece of wiring that heals without paying for a re-open.

### D5 — The live session is a cache, and the recorded handle stays authoritative

Every unit reports the session id from the SDK (`session_id` on the init
system message, and on the result), including when it was served by the open
session, because a fork or a vendor-side branch must reach `latest-wins`.

A pod with no live session (fresh, evicted, rescheduled, or one whose process
died) resumes from `runtimeContextId`. The manager cannot tell the two apart,
and nothing in the work contract describes which happened.

The continuity ladder from `runtimes/claude` today is unchanged and applies
to the re-open path: re-check the session file, unreadable is not absent,
confirmed absence FAILS the run with a non-empty explanation.
`runtimes/claude-cli` already has this ladder, untouched by this change,
since it always takes the re-open path.

### D6 — Long-lived sessions own two things short-lived ones never did

This is `runtimes/claude` only. A one-minute-TTL `runtimes/claude-cli`
process never lives long enough to face either.

- **MCP servers stay up for the pod's life.** Under a one-minute TTL nothing
  had time to expire or leak. Under hours, credentials expire and child
  processes accumulate. The runtime tears the session down, and with it its
  servers, on idle-TTL exit, on fingerprint change, and on unrecoverable
  session error. It never leaves a session open across a failure it did not
  understand.
- **A process that dies mid-turn** loses the in-memory session and nothing
  else. The unit is reported failed, the handle recorded stands, and the next
  unit resumes from disk.

Conversation context is NOT one of these. `--resume` replays the whole
transcript, so context grows identically whether the session was held open
or re-opened. Nothing about context differs between the two components.

### D7 — The transcript keeps its lines, on both components

Typed SDK messages replace hand-parsed `stream-json` in `runtimes/claude`,
but the rendering does not change: `[init]`, `[claude]`, `[tool]`,
`=== RESULT ===`. Those lines are what a human reads in VictoriaLogs, and a
new component is not a reason to relearn them.

`runtimes/claude`'s rendering must match what `runtimes/claude-cli` already
prints, so a pod log reads the same whichever one served it. The reported
`result` keeps its 2000-character cap and the same
`status`/`exitCode`/`continuity` meanings on both components.

A session-open, session-reuse and session-re-open each log distinctly in
`runtimes/claude`, so the active path is visible from the log alone.

## Risks / Trade-offs

- **Subscription auth is inherited, not documented.** The SDK's quickstart
  names `ANTHROPIC_API_KEY` and the cloud providers, not
  `CLAUDE_CODE_OAUTH_TOKEN`.
  - It works anyway, because the bundled binary reads it, exactly as today.
  - Anthropic states that third-party developers may not OFFER claude.ai
    login or rate limits in their products. That reads as out of scope here
    — every adopter brings their own credential.
  - → First task is a smoke test on the subscription token, before any
    rewrite.
  - If it fails, the change stops at that task. `runtimes/claude-cli` stays
    the only way to run claude-code.
- **The bundled binary ships via npm optional dependencies**, so an image
  built with `npm ci --omit=optional` silently has no Claude Code and fails at
  runtime. → `runtimes/claude`'s Dockerfile installs without omitting
  optionals and the build verifies the binary is present.
- **A wrong fingerprint (D4) is a silent capability bug**, not a crash — the
  conversation would keep running under superseded wiring. → The fingerprint
  is unit-tested field by field, and a re-open is logged with the field that
  changed, so the reason is visible in the pod log.
- **A held session is process state.** It is a cache by construction (D5),
  but a bug that lets it diverge from the recorded handle would be hard to
  see. → Every unit reports the id the SDK gives it, so divergence surfaces
  as a changed handle rather than as silence.
- **Two vendors, one permission shape, different enforcement points.**
  Copilot matches per invocation in its own callback. `runtimes/claude` does
  it in `canUseTool`. The shared thing is the rule set, not the code — resist
  extracting a shared module across components that are deliberately
  dependency-free.
- **Two components share no code.** `tools.js`'s composition logic now lives
  in both `runtimes/claude/` and `runtimes/claude-cli/`, fork-and-diverge by
  construction. → Each directory keeps its own `tools.test.js`, and a future
  change to the composition rule (`agent-definition-tools`) needs a task
  against both, the same discipline `ollama`/`copilot` already follow for
  their own tool logic.
- **A derived image built FROM today's `runtimes/claude/Dockerfile` loses the
  CLI binary.** `docs/concepts.md#runtime-images-are-generic` tells adopters
  to derive their own image from a shipped one. Anyone who derived from
  `runtimes/claude` for its CLI binary must re-derive from
  `runtimes/claude-cli` instead. → Named explicitly in `docs/CHANGELOG.md` as
  a breaking change, not folded into a patch note.
- **Two components double the surface kept green.** Every behavioural test —
  permissions, continuity ladder, transcript rendering — now needs a pass for
  each component, not one. → `runtimes/claude-cli` keeps the existing,
  untouched test suite it inherits, `runtimes/claude` gets its own alongside
  it.

## Migration Plan

This is a real migration, not a reversible env var flip. `runtimes/claude`
changes default behaviour (SDK instead of CLI) AND loses the CLI binary from
its image, in the same release.

An adopter who needs to keep running the CLI must, in order:

1. Enable the new bundle: `chart/charts/claude-cli/`'s `enabled: true`.
2. Point every Pipeline or Coordinator that should keep running CLI-driven
   claude-code at `runtimeRef: claude-cli`.
3. Decide which bundle holds `default: true` for routes naming no
   `runtimeRef`. `defaultRuntimeGuard` still fails the render unless exactly
   one answers `default`.

State these three steps in `docs/CHANGELOG.md` as the upgrade note for a
breaking change, per `documentation.md`.

Conversations in flight keep their handle regardless of which component
serves them. Both resume from `runtimeContextId`.

## Open Questions

- Does an hours-long session accumulate MCP child processes in practice, or
  does the SDK reap them? Measure before adding machinery for it.
- Should a fingerprint change fork (`forkSession`) rather than resume, when
  the change is a capability narrowing? Resume is the conservative default
  here.
- Is `PreToolUse` (D3's rejected alternative) worth adopting later as a
  belt-and-braces layer that cannot be shadowed?
- Should `runtimes/claude-cli` ever become the chart's `default: true`
  bundle instead of `runtimes/claude`? That is now a chart-values change —
  moving which bundle carries the flag — not the one-line env var flip the
  single-image design would have allowed.
