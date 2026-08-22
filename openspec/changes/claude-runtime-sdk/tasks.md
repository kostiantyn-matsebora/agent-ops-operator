## 1. Prove the credential first

- [ ] 1.1 Build a throwaway image with `@anthropic-ai/claude-agent-sdk` and run one `query()` against `CLAUDE_CODE_OAUTH_TOKEN` (the chart's `agentops-claude` secret) — if a subscription token does not authenticate, STOP here and report; nothing below is worth writing
- [ ] 1.2 Confirm the bundled Claude Code binary is present in the image and record the install flags that keep it (npm optional dependencies — `npm ci --omit=optional` silently drops it)
- [ ] 1.3 Confirm session files still land under `$HOME/.claude/projects/<encoded-cwd>/`, so `contextStorage: volume` and the home-volume requirement are unchanged

## 2. Pin the SDK behaviours the design rests on

- [ ] 2.1 Verify `canUseTool` is reached for every tool call — built-ins and MCP tools alike — with no `allowedTools` passed and `permissionMode: 'default'`
- [ ] 2.2 Verify a `deny` result is answered immediately and the run continues, rather than prompting or stalling
- [ ] 2.3 Verify streaming input: a second `SDKUserMessage` yielded into a live `query()` is served by the same session, and each turn ends with its own `result`
- [ ] 2.4 Capture where the session id appears (init system message and result) and confirm it is stable across turns of one session
- [ ] 2.5 Verify what `resume` does with an id whose session file is gone, so the continuity ladder keys on a real failure

## 3. The poll loop and the live session

- [ ] 3.1 Restructure `runtime.js` around one `query()` per open session, fed by an async generator the poll loop pushes into
- [ ] 3.2 Read each unit's outcome by consuming until that turn's `result`; keep the 2000-character cap and the existing `status`/`exitCode` meanings
- [ ] 3.3 Keep the loop's existing shape otherwise — long-poll, report with bounded retry, idle-TTL exit `0`
- [ ] 3.4 Tear the session down on idle exit, on configuration change, and on unattributable session failure

## 4. Configuration fingerprint

- [ ] 4.1 Fingerprint the session-affecting configuration of a unit: system prompt, MCP servers, `maxTurns`, cwd, agent
- [ ] 4.2 Deliberately EXCLUDE the composed tool allowlist — it lives in the callback and changes per unit with no re-open
- [ ] 4.3 Re-open with `resume: <runtimeContextId>` on a fingerprint change, logging which field changed
- [ ] 4.4 Unit-test the fingerprint field by field, including that an allowlist-only change does not trigger a re-open

## 5. Permissions

- [ ] 5.1 Move the composed allowlist from `--allowedTools` into a `canUseTool` callback, deny-by-default, reusing `tools.js` composition unchanged
- [ ] 5.2 Pass NO `allowedTools` and keep `permissionMode: 'default'`; assert in a test that neither is reintroduced
- [ ] 5.3 Preserve the existing pattern semantics (bare names and scoped forms) so a bound toolset means what it meant under the CLI
- [ ] 5.4 Log every denial with the pattern that failed to match, as the CLI path's `dontAsk` denials were visible

## 6. Continuity

- [ ] 6.1 Mint nothing: continue reporting the vendor's session id every unit, including units served from an open session
- [ ] 6.2 Port the missing-context ladder to the re-open path — re-check at 500ms/1.5s/3s, unreadable is not absent, retry once
- [ ] 6.3 On confirmed absence, FAIL with the existing user-facing text and a non-empty result
- [ ] 6.4 Keep reading the retired `resumeSessionId` alongside `runtimeContextId` for one release

## 7. Transcript

- [ ] 7.1 Render typed SDK messages to the existing `[init]` / `[claude]` / `[tool]` / `=== RESULT ===` lines
- [ ] 7.2 Log a session open, a session reuse and a session re-open distinctly, so a pod log shows which path a unit took

## 8. Tests

- [ ] 8.1 `tools.test.js` continues to pass untouched — composition is not what changed
- [ ] 8.2 Permission-callback tests: allow, deny, empty composition denies everything, scoped patterns
- [ ] 8.3 Session-reuse tests: identical fingerprint reuses, changed fingerprint re-opens with resume, allowlist-only change reuses
- [ ] 8.4 Continuity tests over the ladder, as today
- [ ] 8.5 `node --test` passes with no network access

## 9. Image and docs

- [ ] 9.1 Update `runtimes/claude/Dockerfile`: install the SDK (optional deps intact), drop the global `@anthropic-ai/claude-code` install, keep the no-domain-tooling comment
- [ ] 9.2 Build and push a new `agentops-runtime-claude` tag (never overwrite a pushed tag)
- [ ] 9.3 `docs/contracts.md`: the allowlist may be enforced per invocation, and what that obliges (nothing shadows the decision point)
- [ ] 9.4 `CLAUDE.md`: note that the reference runtime drives the SDK and holds a session per pod, and that the allowlist is a callback rule set

## 10. Verify on the live install

- [ ] 10.1 Point one `AgentRuntime.spec.image` at the new tag; confirm a `kind: task` signal produces an answer on every bound thread
- [ ] 10.2 Send two messages in quick succession and confirm from the pod log that the second was served by the open session
- [ ] 10.3 Edit a bound `MCPConfig` mid-conversation and confirm the next unit re-opens and picks it up
- [ ] 10.4 Edit a bound `MCPToolset` mid-conversation and confirm the next unit picks it up WITHOUT a re-open
- [ ] 10.5 Raise `idleTtlMinutes` to 60 on a test runtime, run a long conversation, and check for leaked MCP child processes
- [ ] 10.6 Delete the session state and confirm the run FAILS with the cannot-be-continued message rather than answering fresh
