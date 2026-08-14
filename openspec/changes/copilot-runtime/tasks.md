## 1. Pin the vendor facts before writing code

- [ ] 1.1 Verify against the installed `@github/copilot-sdk`: `SessionConfig.sessionId` is honoured on create, `resumeSession(id)` resumes it, and state lands at `$HOME/.copilot/session-state/<id>/`
- [ ] 1.2 Enumerate the built-in tool wire names the runtime actually registers (`view`/`grep`/`glob`/`edit`/`write`/`bash` and neighbours) and record the list the mapper targets
- [ ] 1.3 Confirm what `resumeSession` throws for an unknown id, so D3's ladder keys on a real failure and not on a message match
- [ ] 1.4 Confirm `availableTools: []` yields NO tools (not "unset means all"), and that `onPermissionRequest` is called for every invocation including built-ins

## 2. The module skeleton

- [ ] 2.1 Create `runtime-copilot/` with `package.json` (`@github/copilot-sdk`, no other runtime dependency), no dependency on any other module in this repo
- [ ] 2.2 Write `runtime.js`: env config (`CONTROL_URL`, `CONVO_ID`, `POD_NAME`, `REPO_*`, `GIT_*`, `RUNTIME_IDLE_TTL_M`, `WORKSPACE`, `MCP_CONFIG`), fail fast on missing `CONTROL_URL`/`CONVO_ID`
- [ ] 2.3 Port the repo sync: clone/fetch/reset at `/data/workspace`, SSH and HTTPS auth, clear contents never rmdir the mount point
- [ ] 2.4 Implement the poll loop: long-poll `/work`, idle-TTL exit `0`, `POST /work/done` with bounded retry

## 3. Tool vocabulary translation

- [ ] 3.1 `tools.js`: read `.github/agents/<agent>.agent.md` frontmatter `tools:` (inline, flow-list and block forms), returning "declares nothing" for absent/unparseable, logging the reason
- [ ] 3.2 Reuse the `merge`/`overwrite` composition semantics verbatim — union with the agent's keeping position, dedup, unknown mode reads as `merge`
- [ ] 3.3 `vocabulary.js`: map composed patterns to `availableTools` entries per the design's table, returning the unmapped ones separately rather than dropping them
- [ ] 3.4 Refuse `mcp__<server>__*` explicitly with its own log line — never widen it to `mcp:*`
- [ ] 3.5 Build the permission handler: approve only invocations matching the composed patterns, enforce sub-command scoping (`Bash(kubectl:*)`), deny anything with shell metacharacters that could smuggle a second command, log every denial with the pattern that failed
- [ ] 3.6 Always pass an explicit `availableTools`, including `[]`; never let the vendor's "no declaration means everything" default apply
- [ ] 3.7 At session start, log any mapped target the runtime did not register — a wrong wire name must surface on the first run, not as a tool that silently never appears

## 4. Session lifecycle and continuity

- [ ] 4.1 Mint a `crypto.randomUUID()` session id when the unit carries no `runtimeContextId`; never derive it from the conversation name
- [ ] 4.2 Resume when the unit carries one; report the established id on `/work/done` under `runtimeContextId`
- [ ] 4.3 Implement the missing-context ladder: re-check `session-state/<id>/` at 500ms/1.5s/3s, treat unreadable as present, retry once when it reappears
- [ ] 4.4 On confirmed absence, FAIL with the same user-facing text `runtime-claude` uses — never an empty result, never a fresh session presented as a continuation
- [ ] 4.5 Prepend `unit.systemPrompt` as a delimited block on session creation only; log its length as `runtime-claude` does

## 5. MCP and prompts

- [ ] 5.1 Translate `/etc/agentops/mcp.json` into the SDK's `mcpServers` record — stdio (`command`/`args`/`env`) and http (`type`/`url`/`headers`)
- [ ] 5.2 Expand `${VAR}` placeholders from `process.env` in-process; never log the resolved value
- [ ] 5.3 Fail an individual server's registration with a logged reason when a placeholder cannot be resolved, rather than passing the literal text through
- [ ] 5.4 Resolve `promptText`, or `promptFile` + `promptVars` read relative to the checkout, failing the unit with a readable reason when neither yields a prompt
- [ ] 5.5 Log the requested `maxTurns` without pretending to enforce it; wire optional `COPILOT_MAX_AI_CREDITS` to `sessionLimits.maxAiCredits`

## 6. Transcript

- [ ] 6.1 Subscribe with `streaming: true` and render events to stdout in the existing `[init]`/`[copilot]`/`[tool]`/`=== RESULT ===` shape, so pod logs read the same across runtimes
- [ ] 6.2 Cap the reported `result` at the same 2000 characters and report `status`/`exitCode`/`continuity` with the same meanings

## 7. Tests

- [ ] 7.1 `tools.test.js`: frontmatter parsing (all forms, absent, malformed) and `merge`/`overwrite` composition
- [ ] 7.2 `vocabulary.test.js`: every row of the mapping table, unmapped-denies, per-server wildcard refused, sub-command matching including the metacharacter denials
- [ ] 7.3 `mcp.test.js`: stdio and http translation, `${VAR}` expansion, unresolvable placeholder fails that server only
- [ ] 7.4 A continuity test over the ladder: reappearing state retries, unreadable path is not absence, confirmed absence fails with a non-empty result
- [ ] 7.5 `node --test` passes with no network access

## 8. Image

- [ ] 8.1 `runtime-copilot/Dockerfile` on `node:22-bookworm-slim`: git, openssh-client, curl, jq, ca-certificates, procps; install the SDK; `HOME=/data/home`; non-root
- [ ] 8.2 Carry the runtime-claude comment forbidding domain tooling, naming the derive-your-own-image escape hatch
- [ ] 8.3 Build and push `agentops-runtime-copilot:0.1.0` (never overwrite a pushed tag)

## 9. Chart

- [ ] 9.1 Add `additionalRuntimes: []` to `chart/values.yaml` with one-line comments, and amend the `runtime:` comment that currently says additional runtimes stay hand-written
- [ ] 9.2 Render them in `chart/templates/runtime.yaml`: per-entry `AgentRuntime` + optional credential Secret, reusing the SA helper and the parent's home/workspace claim resolution
- [ ] 9.3 Refuse (fail the render) an entry naming its own `serviceAccountName`, and an entry whose name collides with `runtime.name`
- [ ] 9.4 Extend `internal/integration/charttemplate_test.go`: defaults render exactly one runtime; one entry renders two runtimes, two credential Secrets, still one runtime SA
- [ ] 9.5 Bump the chart version and add the `CHANGELOG.md` entry, newest first

## 10. Docs

- [ ] 10.1 `docs/contracts.md`: name the second reference implementation and state the per-runtime obligations it makes visible — definition path, vendor default neutralisation, unmapped-denies
- [ ] 10.2 `docs/concepts.md`: how capabilities resolve when the runtime's vocabulary differs, and that `runtimeRef` is the whole vendor switch
- [ ] 10.3 `README.md`: one line in the module list, staying inside the 150-line budget (`wc -l README.md`)
- [ ] 10.4 `CLAUDE.md`: add `runtime-copilot/` to the map and the module list, and add the build/test lines for it

## 11. Verify against a live install

- [ ] 11.1 Server-side dry-run the rendered chart before applying (`helm upgrade --dry-run=server`)
- [ ] 11.2 Point one profile's `runtimeRef` at the copilot runtime and post a `kind: task` signal to a source its Pipeline claims; confirm the answer reaches every bound thread
- [ ] 11.3 Reply in the thread and confirm the second run resumes the same context — then delete the session state and confirm the run FAILS with the cannot-be-continued message rather than answering fresh
- [ ] 11.4 Bind observation-only toolsets on one route and confirm shell is denied there while the same profile keeps it on another route
