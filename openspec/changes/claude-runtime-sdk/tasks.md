## 1. Prove the credential first

*Not run here: no Anthropic subscription credential is available in this
remote sandbox.*

*Grounded instead against the real, installed
`@anthropic-ai/claude-agent-sdk@0.3.310` package and its shipped types.*

*A workstation session with a real credential owes this section, before
task 8.2 and section 10.*

- [ ] 1.1 Build a throwaway image with `@anthropic-ai/claude-agent-sdk` and run one `query()` against `CLAUDE_CODE_OAUTH_TOKEN` (the chart's `agentops-claude` secret). If a subscription token does not authenticate, STOP here and report — nothing below is worth writing, and `runtimes/claude-cli` stays the only way to run claude-code
- [ ] 1.2 Confirm the bundled Claude Code binary is present in the image and record the install flags that keep it (npm optional dependencies — `npm ci --omit=optional` silently drops it)
- [ ] 1.3 Confirm session files still land under `$HOME/.claude/projects/<encoded-cwd>/`, so `contextStorage: volume` and the context-volume requirement are unchanged

## 2. Pin the SDK behaviours the design rests on

*Same blocker as section 1 — no live credential here. `runtime.js`'s own
comments flag every assumption made in place of running this section.*

- [ ] 2.1 Verify `canUseTool` is reached for every tool call — built-ins and MCP tools alike — with no `allowedTools` passed and `permissionMode: 'default'`
- [ ] 2.2 Verify a `deny` result is answered immediately and the run continues, rather than prompting or stalling
- [ ] 2.3 Verify streaming input: a second `SDKUserMessage` yielded into a live `query()` is served by the same session, and each turn ends with its own `result`
- [ ] 2.4 Capture where the session id appears (init system message and result) and confirm it is stable across turns of one session
- [ ] 2.5 Verify what `resume` does with an id whose session file is gone, so the continuity ladder keys on a real failure

## 3. Split into two components

- [x] 3.1 Create `runtimes/claude-cli/` as a new directory. COPY today's `runtime.js`, `tools.js`, `spin.js`, `report.js` and every one of their test files into it VERBATIM — no behaviour change. The originals stay in place at `runtimes/claude/`, for 3.3 below to rewrite. Copy today's `runtimes/claude/Dockerfile` unchanged as `runtimes/claude-cli/Dockerfile`
- [x] 3.2 Add `runtimes/claude-cli` to `.github/components.sh`'s component map (`runtimes/claude-cli` → `runtime-claude-cli` → `agentops-runtime-claude-cli`) and confirm `.github/components.sh images`/`modules` picks it up
- [x] 3.3 In `runtimes/claude/runtime.js`, replace `spawn('claude', …)` with `@anthropic-ai/claude-agent-sdk`'s `query({ prompt: <async generator>, options })`, held open across the pod's work units
- [x] 3.4 Read each unit's outcome by consuming the message stream until that turn's `result`. Keep the 2000-character cap and the existing `status`/`exitCode` meanings
- [x] 3.5 Keep the poll loop's existing shape — long-poll, report with bounded retry, idle-TTL exit `0`
- [x] 3.6 Tear the session down on idle exit, on configuration change, and on unattributable session failure

## 4. Configuration fingerprint

All of 4.1–4.4 apply to `runtimes/claude` only — `runtimes/claude-cli` opens a fresh process per unit and has no live session to fingerprint.

- [x] 4.1 Fingerprint the session-affecting configuration of a unit: system prompt, MCP servers, `maxTurns`, cwd, agent
- [x] 4.2 Deliberately EXCLUDE the composed tool allowlist from the fingerprint — it lives in the callback and changes per unit with no re-open
- [x] 4.3 Re-open with `resume: <runtimeContextId>` on a fingerprint change, logging which field changed
- [x] 4.4 Unit-test the fingerprint field by field, including that an allowlist-only change does not trigger a re-open

## 5. Permissions

- [x] 5.1 In `runtimes/claude`, move the composed allowlist from `--allowedTools` into a `canUseTool` callback, deny-by-default, reusing `tools.js` composition unchanged. Leave `runtimes/claude-cli` passing the composed list as `--allowedTools` with `--permission-mode dontAsk`, exactly as today
- [x] 5.2 In `runtimes/claude`, pass NO `allowedTools` and keep `permissionMode: 'default'`. Assert in a test that neither is reintroduced
- [x] 5.3 In `runtimes/claude`'s callback, preserve the existing pattern semantics (bare names and scoped forms), so a bound toolset means what it meant under `runtimes/claude-cli`
- [x] 5.4 In `runtimes/claude`, log every denial with the pattern that failed to match, as `runtimes/claude-cli`'s `dontAsk` denials were visible

## 6. Continuity

- [x] 6.1 Mint nothing: continue reporting the vendor's session id every unit on both components, including units `runtimes/claude` served from an open session
- [x] 6.2 Port the missing-context ladder to `runtimes/claude`'s re-open path — re-check at 500ms/1.5s/3s, unreadable is not absent, retry once. `runtimes/claude-cli` already has it, unchanged by the move
- [x] 6.3 On confirmed absence, FAIL with the existing user-facing text and a non-empty result, on either component
- [x] 6.4 Keep reading the retired `resumeSessionId` alongside `runtimeContextId` for one release, on both components

## 7. Transcript

- [x] 7.1 Render `runtimes/claude`'s typed SDK messages to the existing `[init]` / `[claude]` / `[tool]` / `=== RESULT ===` lines, matching what `runtimes/claude-cli` already prints
- [x] 7.2 In `runtimes/claude`, log a session open, a session reuse and a session re-open distinctly, so a pod log shows which path a unit took

## 8. Images and chart bundle

- [x] 8.1 Update `runtimes/claude/Dockerfile`: drop the global `@anthropic-ai/claude-code` install and its installer step entirely, add `@anthropic-ai/claude-agent-sdk` with optional deps intact. Keep the no-domain-tooling comment, and verify the SDK's bundled binary is present in the built image
- [ ] 8.2 Build and push a new `agentops-runtime-claude` tag (SDK-only, lighter) and a new `agentops-runtime-claude-cli` tag (the moved CLI code). Never overwrite a pushed tag

  *Not run here: no docker daemon in this remote sandbox. Needs a
  workstation session.*
- [x] 8.3 Create `chart/charts/claude-cli/` — `Chart.yaml`, `values.yaml`, `templates/runtime.yaml` — mirroring `chart/charts/ollama/`'s shape: `enabled: false`, `default: false`, its own `image`, `credentialsSecret` and `contextSync` entries matching `chart/charts/claude/values.yaml`'s claude-specific values
- [x] 8.4 Confirm `agentops.defaultRuntimeGuard` still passes with `claude-cli` added and off — the guard re-derives wiring per bundle, so an off bundle must contribute nothing to the default check

## 9. Unit tests

- [x] 9.1 `tools.test.js` passes unchanged in BOTH `runtimes/claude/` and `runtimes/claude-cli/` — composition is not what changed in either copy
- [x] 9.2 `runtimes/claude-cli/`'s copied test suite (today's `runtime.test.js` and siblings) passes unchanged — the copy must not change behaviour
- [x] 9.3 Permission-callback tests for `runtimes/claude`: allow, deny, empty composition denies everything, scoped patterns
- [x] 9.4 Session-reuse tests for `runtimes/claude`: identical fingerprint reuses, changed fingerprint re-opens with resume, allowlist-only change reuses
- [x] 9.5 Continuity tests over the ladder, on both components, as today
- [x] 9.6 `node --test` passes with no network access, in both `runtimes/claude/` and `runtimes/claude-cli/`
- [x] 9.7 The chart's render tests cover `chart/charts/claude-cli/` — `helm template` with it enabled and with it off, and with both `claude` and `claude-cli` enabled together with explicit `default:` flags

## 10. System tests

*Not run here: no cluster in this remote sandbox. A workstation session, or
`gh workflow run system-smoke.yml --ref change/claude-runtime-sdk`
(dispatched, not waited on, from this session), owes this section.*

- [ ] 10.1 Point one `AgentRuntime.spec.image` at the new `agentops-runtime-claude` tag. Confirm a `kind: task` signal produces an answer on every bound thread
- [ ] 10.2 Send two messages in quick succession and confirm from the pod log that the second was served by the open session
- [ ] 10.3 Edit a bound `MCPConfig` mid-conversation and confirm the next unit re-opens and picks it up
- [ ] 10.4 Edit a bound `MCPToolset` mid-conversation and confirm the next unit picks it up WITHOUT a re-open
- [ ] 10.5 Raise `idleTtlMinutes` to 60 on a test runtime, run a long conversation, and check for leaked MCP child processes
- [ ] 10.6 Delete the session state and confirm the run FAILS with the cannot-be-continued message rather than answering fresh
- [ ] 10.7 Enable `chart/charts/claude-cli/`, point a Pipeline's `runtimeRef` at it, and confirm behaviour is identical to today's pre-change `runtimes/claude` — one spawn per unit, no session reuse
- [ ] 10.8 Walk `docs/contracts/runtime.md` requirement by requirement (`API-n`, `LC-n`, `RT-n`, `MGR-n`, `SEC-n`) against both components and confirm none is violated, per the conformance requirement in `specs/runtime-session-reuse/spec.md`

## 11. Documentation

*11.1–11.4 need explicit user approval before editing `docs/contracts/runtime.md`
— see `.claude/rules/runtime-spec.md`'s "no unapproved spec edit" rule.
Unavailable from this unattended session, so the RT-3.5 clarification and
the three vendor-page version bumps it drives stay undone here.*

- [ ] 11.1 `docs/contracts/runtime.md`: amend RT-3 (Composing the allowlist) to state that a per-invocation decision satisfies it as well as passing a flag, provided the callback is reached for every governed call and denies by default. Bump the version PATCH (clarifies an obligation, changes none) and add a changelog row
- [ ] 11.2 Move `docs/claude.md`'s claimed runtime-specification version to match, and rewrite its Dockerfile description now that the CLI install is gone
- [ ] 11.3 Move `docs/runtimes/ollama.md`'s claimed runtime-specification version to match, even though `runtimes/ollama/` needs no code change for this clarification
- [ ] 11.4 Move `docs/runtimes/copilot.md`'s claimed runtime-specification version to match, even though `runtimes/copilot/` needs no code change for this clarification
- [x] 11.5 Add `docs/runtimes/claude-cli.md`, documenting the new bundle the same way `docs/runtimes/ollama.md` documents its own: image, credential, when to pick it over `runtimes/claude`
- [x] 11.6 `.claude/rules/runtime-spec.md`: replace the two-driver-one-image note with a two-component note — `runtimes/claude` (sdk, default-eligible, the allowlist is a `canUseTool` callback rule set) and `runtimes/claude-cli` (today's per-unit spawn, off by default, the allowlist is `--allowedTools`/`--permission-mode dontAsk`)

  *Already current — a prior PR on this issue updated it ahead of the code.*
- [x] 11.7 `.claude/rules/structure.md`: add a `runtimes/claude-cli` row to the runtimes table, and `.claude/rules/chart.md` if the bundle's shape needs a note beside `ollama`'s
- [x] 11.8 `docs/CHANGELOG.md`: a breaking-change entry naming the default-behaviour change in `runtimes/claude`, the loss of the CLI binary from its image, and the three-step upgrade path (enable `claude-cli`, re-point `runtimeRef`, choose which bundle is `default`) from design.md's Migration Plan
- [x] 11.9 `docs/configuration.md`/`docs/installation.md`: note the new `claude-cli` bundle beside `ollama`/`copilot`, same shape — off by default, enable to run a second vendor variant beside the default
