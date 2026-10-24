## 1. Prove the credential first

- [ ] 1.1 Build a throwaway image with `@anthropic-ai/claude-agent-sdk` and run one `query()` against `CLAUDE_CODE_OAUTH_TOKEN` (the chart's `agentops-claude` secret). If a subscription token does not authenticate, STOP here and report — nothing below is worth writing, and `driver-cli.js` stays the only driver
- [ ] 1.2 Confirm the bundled Claude Code binary is present in the image and record the install flags that keep it (npm optional dependencies — `npm ci --omit=optional` silently drops it)
- [ ] 1.3 Confirm session files still land under `$HOME/.claude/projects/<encoded-cwd>/`, so `contextStorage: volume` and the context-volume requirement are unchanged

## 2. Pin the SDK behaviours the design rests on

- [ ] 2.1 Verify `canUseTool` is reached for every tool call — built-ins and MCP tools alike — with no `allowedTools` passed and `permissionMode: 'default'`
- [ ] 2.2 Verify a `deny` result is answered immediately and the run continues, rather than prompting or stalling
- [ ] 2.3 Verify streaming input: a second `SDKUserMessage` yielded into a live `query()` is served by the same session, and each turn ends with its own `result`
- [ ] 2.4 Capture where the session id appears (init system message and result) and confirm it is stable across turns of one session
- [ ] 2.5 Verify what `resume` does with an id whose session file is gone, so the continuity ladder keys on a real failure

## 3. The poll loop, the live session, and the driver split

- [x] 3.1 Split `runtime.js` into a thin entrypoint and two driver modules: `driver-cli.js` (today's spawn-per-unit logic, moved verbatim, no behaviour change) and `driver-sdk.js` (restructured around one `query()` per open session, fed by an async generator the poll loop pushes into)
- [x] 3.2 Add a `RUNTIME_CLAUDE_DRIVER` env var read by the entrypoint — `sdk` default, `cli` the alternative — and log the active driver on startup. An unrecognized value logs a warning and falls back to `sdk`
- [x] 3.3 In `driver-sdk.js`, read each unit's outcome by consuming until that turn's `result`. Keep the 2000-character cap and the existing `status`/`exitCode` meanings (`exitCode` itself is omitted — no process exit to report, per `docs/contracts/runtime.md`'s API-7)
- [x] 3.4 Keep the loop's existing shape otherwise, on both drivers — long-poll, report with bounded retry, idle-TTL exit `0`
- [x] 3.5 In `driver-sdk.js`, tear the session down on idle exit, on configuration change, and on unattributable session failure

## 4. Configuration fingerprint

All of 4.1–4.4 apply to `driver-sdk.js` only — `driver-cli.js` opens a fresh process per unit and has no live session to fingerprint.

- [x] 4.1 Fingerprint the session-affecting configuration of a unit: system prompt, MCP servers, `maxTurns`, cwd, agent
- [x] 4.2 Deliberately EXCLUDE the composed tool allowlist from the fingerprint — it lives in the callback and changes per unit with no re-open
- [x] 4.3 Re-open with `resume: <runtimeContextId>` on a fingerprint change, logging which field changed
- [x] 4.4 Unit-test the fingerprint field by field, including that an allowlist-only change does not trigger a re-open

## 5. Permissions

- [x] 5.1 In `driver-sdk.js`, move the composed allowlist from `--allowedTools` into a `canUseTool` callback, deny-by-default, reusing `tools.js` composition unchanged. Leave `driver-cli.js` passing the composed list as `--allowedTools` with `--permission-mode dontAsk`, exactly as today
- [x] 5.2 In `driver-sdk.js`, pass NO `allowedTools` and keep `permissionMode: 'default'`. Assert in a test that neither is reintroduced
- [x] 5.3 In `driver-sdk.js`'s callback, preserve the existing pattern semantics (bare names and scoped forms), so a bound toolset means what it meant under `driver-cli.js` — base-name/server-name matching only (same granularity `tools.js`'s `logUngrantedTools` already uses), NOT the CLI's own sub-command glob parser for a scoped form like `Bash(git *)`: that form is matched at the base tool name here. Flagged for review — see the PR description
- [x] 5.4 In `driver-sdk.js`, log every denial with the pattern that failed to match, as `driver-cli.js`'s `dontAsk` denials were visible

## 6. Continuity

- [x] 6.1 Mint nothing: continue reporting the vendor's session id every unit on both drivers, including units `driver-sdk.js` served from an open session
- [x] 6.2 Port the missing-context ladder to `driver-sdk.js`'s re-open path — re-check at 500ms/1.5s/3s, unreadable is not absent, retry once. `driver-cli.js` already has it, unchanged by the move
- [x] 6.3 On confirmed absence, FAIL with the existing user-facing text and a non-empty result, on either driver
- [x] 6.4 Keep reading the retired `resumeSessionId` alongside `runtimeContextId` for one release, on both drivers (via `driver-cli.js`'s unchanged `contextIdOf`, reused by `driver-sdk.js`)

## 7. Transcript

- [x] 7.1 Render `driver-sdk.js`'s typed SDK messages to the existing `[init]` / `[claude]` / `[tool]` / `=== RESULT ===` lines, matching what `driver-cli.js` already prints (reuses `driver-cli.js`'s `formatEvent` directly — the field shapes line up)
- [x] 7.2 In `driver-sdk.js`, log a session open, a session reuse and a session re-open distinctly, so a pod log shows which path a unit took

## 8. Image

- [x] 8.1 Update `runtimes/claude/Dockerfile`: keep the global `@anthropic-ai/claude-code` install, needed by `driver-cli.js`, and add `@anthropic-ai/claude-agent-sdk` with optional deps intact, needed by `driver-sdk.js`. Keep the no-domain-tooling comment, and verify both binaries are present in the built image (verified the npm mechanics locally — optional deps resolve and the bundled binary runs under `--ignore-scripts` — but NOT inside an actual built container image: no docker daemon in this remote session, see PR description)
- [ ] 8.2 Build and push a new `agentops-runtime-claude` tag carrying both drivers (never overwrite a pushed tag) — WORKSTATION-ONLY, not performed in this remote session (no docker daemon)

## 9. Unit tests

- [x] 9.1 `tools.test.js` continues to pass untouched — composition is not what changed
- [x] 9.2 `driver-cli.js`'s existing test suite (today's `runtime.test.js`, moved alongside it) continues to pass untouched — the move must not change behaviour
- [x] 9.3 Permission-callback tests for `driver-sdk.js`: allow, deny, empty composition denies everything, scoped patterns
- [x] 9.4 Session-reuse tests for `driver-sdk.js`: identical fingerprint reuses, changed fingerprint re-opens with resume, allowlist-only change reuses
- [x] 9.5 Continuity tests over the ladder, on both drivers, as today
- [x] 9.6 A driver-selection test: `RUNTIME_CLAUDE_DRIVER=sdk`, `=cli`, unset, and an unrecognized value each wire the entrypoint to the right module
- [x] 9.7 `node --test` passes with no network access (204/204, `runtimes/claude/`)

## 10. System tests

- [ ] 10.1 Point one `AgentRuntime.spec.image` at the new tag. Confirm a `kind: task` signal produces an answer on every bound thread, with the default `sdk` driver active
- [ ] 10.2 Send two messages in quick succession and confirm from the pod log that the second was served by the open session
- [ ] 10.3 Edit a bound `MCPConfig` mid-conversation and confirm the next unit re-opens and picks it up
- [ ] 10.4 Edit a bound `MCPToolset` mid-conversation and confirm the next unit picks it up WITHOUT a re-open
- [ ] 10.5 Raise `idleTtlMinutes` to 60 on a test runtime, run a long conversation, and check for leaked MCP child processes
- [ ] 10.6 Delete the session state and confirm the run FAILS with the cannot-be-continued message rather than answering fresh
- [ ] 10.7 Set `RUNTIME_CLAUDE_DRIVER=cli` on the test `AgentRuntime` and confirm behaviour is identical to the pre-change runtime — one spawn per unit, no session reuse. This is the documented rollback path
- [ ] 10.8 Walk `docs/contracts/runtime.md` requirement by requirement (`API-n`, `LC-n`, `RT-n`, `MGR-n`, `SEC-n`) against both drivers and confirm none is violated, per the conformance requirement in `specs/runtime-session-reuse/spec.md`

## 11. Documentation

- [x] 11.1 `docs/contracts/runtime.md`: amend RT-3 (Composing the allowlist) to state that a per-invocation decision satisfies it as well as passing a flag, provided the callback is reached for every governed call and denies by default. Bump the version PATCH (clarifies an obligation, changes none) and add a changelog row
- [x] 11.2 Move `docs/claude.md`'s claimed runtime-specification version to match
- [x] 11.3 Move `docs/runtimes/ollama.md`'s claimed runtime-specification version to match, even though `runtimes/ollama/` needs no code change for this clarification
- [x] 11.4 Move `docs/runtimes/copilot.md`'s claimed runtime-specification version to match, even though `runtimes/copilot/` needs no code change for this clarification
- [x] 11.5 `.claude/rules/runtime-spec.md`: note that `runtime-claude` ships two drivers — `sdk` (default, holds a session per pod) and `cli` (today's per-unit spawn, kept as the selectable alternative via `RUNTIME_CLAUDE_DRIVER=cli`) — and that the allowlist is a callback rule set on the `sdk` driver only. Root `CLAUDE.md` stays an index and gets no new line for this. Done in this PR, since it is a rules-file note, not code
- [x] 11.6 `docs/claude.md`: document `RUNTIME_CLAUDE_DRIVER` for adopters, alongside the existing `RUNTIME_UNPARSED_REPEAT_LIMIT` entry — both values, the `sdk` default, and the unrecognized-value fallback
