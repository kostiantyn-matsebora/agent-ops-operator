## 1. Pin the vendor facts before writing code

- [x] 1.1 Verify against the installed `@github/copilot-sdk`: `SessionConfig.sessionId` is honoured on create, `resumeSession(id)` resumes it, and state lands at `$HOME/.copilot/session-state/<id>/` — record the layout the bundle's `contextSync.paths` will name
- [x] 1.2 Enumerate the built-in tool wire names the runtime actually registers (`view`/`grep`/`glob`/`edit`/`write`/`shell` and neighbours) and correct the design's D4 table to the list the mapper targets
- [x] 1.3 Confirm what `resumeSession` throws for an unknown id, so D3's ladder keys on a real failure and not on a message match
- [x] 1.4 Confirm `availableTools: []` yields NO tools (not "unset means all"), and that `onPermissionRequest` is called for every invocation including built-ins
- [x] 1.5 Confirm the SDK does not discover `.github/agents/` on its own from `workingDirectory`; if it does, find the option that disables it (D5)
- [x] 1.6 Confirm the manager needs no change: `WorkUnit` carries `agent`, `allowedTools`, `toolsMode`, `maxTurns`, `systemPrompt`, `runtimeContextId`; `/work/done` accepts `continuity`/`continuityReason`; `runtimepod` injects `HOME=/data/context`, `WORKSPACE`, `MCP_CONFIG`; `context-sync` proxies `CONTROL_URL`. Anything missing is a contract change to raise, not to work around

## 2. The component skeleton

- [x] 2.1 Create `runtimes/copilot/` with `package.json` (`@github/copilot-sdk` the only dependency), no dependency on any other module in this repo; confirm `.github/components.sh images` lists `runtime-copilot`
- [x] 2.2 Write `runtime.js`: env config (`CONTROL_URL`, `CONVO_ID`, `POD_NAME`, `REPO_*`, `GIT_*`, `RUNTIME_IDLE_TTL_M`, `WORKSPACE`, `MCP_CONFIG`, `COPILOT_GITHUB_TOKEN`, optional `COPILOT_MODEL`, `COPILOT_MAX_AI_CREDITS`), exit non-zero naming a required one that is missing
- [x] 2.3 Port the repo sync: clone/fetch/reset at `/data/workspace`, SSH and HTTPS auth, clear contents never rmdir the mount point
- [x] 2.4 Implement the poll loop: long-poll `/work`, idle-TTL exit `0`, `POST /work/done` with the reference runtime's retry cadence

## 3. Tool vocabulary translation

- [x] 3.1 `tools.js`: read `.github/agents/<agent>.agent.md` frontmatter `tools:` (inline, flow-list and block forms), returning "declares nothing" for absent/unparseable, logging the reason
- [x] 3.2 Reuse the `merge`/`overwrite` composition semantics verbatim — union with the agent's keeping position, dedup, unknown mode reads as `merge`
- [x] 3.3 `vocabulary.js`: map composed patterns to `availableTools` entries per the design's table, returning the unmapped ones separately rather than dropping them
- [x] 3.4 Refuse `mcp__<server>__*` explicitly with its own log line — never widen it to `mcp:*`
- [x] 3.5 Build the permission handler: approve only invocations matching the composed patterns, enforce sub-command scoping (`Bash(kubectl:*)`), deny anything with shell metacharacters that could smuggle a second command, log every denial with the pattern that failed
- [x] 3.6 Always pass an explicit `availableTools`, including `[]`; never let the vendor's "no declaration means everything" default apply
- [x] 3.7 At session start, log any mapped target the runtime did not register — a wrong wire name must surface on the first run, not as a tool that silently never appears

## 4. Session lifecycle and continuity

- [x] 4.1 Mint a `crypto.randomUUID()` session id when the unit carries no `runtimeContextId`; never derive it from the conversation name; report `continuity: new`
- [x] 4.2 Resume when the unit carries one; report the established id on `/work/done` under `runtimeContextId` with `continuity: continued`
- [x] 4.3 Implement the missing-context ladder: re-check `session-state/<id>/` at 500ms/1.5s/3s, treat unreadable as present, retry once when it reappears
- [x] 4.4 On confirmed absence, FAIL with `continuity: unavailable`, a `continuityReason` naming the context volume, and the same user-facing text the other runtimes use — never an empty result, never a fresh session presented as a continuation
- [x] 4.5 Prepend `unit.systemPrompt` as a delimited block on session creation only; log its length as `runtime-claude` does

## 5. MCP and prompts

- [x] 5.1 Translate `$MCP_CONFIG` into the SDK's `mcpServers` record — stdio (`command`/`args`/`env`) and http (`type`/`url`/`headers`)
- [x] 5.2 Expand `${VAR}` placeholders from `process.env` in-process; never log the resolved value
- [x] 5.3 Fail an individual server's registration with a logged reason when a placeholder cannot be resolved, rather than passing the literal text through
- [x] 5.4 Resolve `promptText`, or `promptFile` + `promptVars` read relative to the checkout, failing the unit with a readable reason when neither yields a prompt
- [x] 5.5 Log the requested `maxTurns` without pretending to enforce it; wire optional `COPILOT_MAX_AI_CREDITS` to `sessionLimits.maxAiCredits`

## 6. Transcript

- [x] 6.1 Subscribe with `streaming: true` and render events to stdout in the existing `[init]`/`[copilot]`/`[tool]`/`=== RESULT ===` shape, so pod logs read the same across runtimes
- [x] 6.2 Cap the reported `result` at the same 2000 characters and report `status`/`exitCode`/`continuity` with the same meanings

## 7. Tests

- [x] 7.1 `tools.test.js`: frontmatter parsing (all forms, absent, malformed) and `merge`/`overwrite` composition
- [x] 7.2 `vocabulary.test.js`: every row of the mapping table, unmapped-denies, per-server wildcard refused, sub-command matching including the metacharacter denials
- [x] 7.3 `mcp.test.js`: stdio and http translation, `${VAR}` expansion, unresolvable placeholder fails that server only
- [x] 7.4 A continuity test over the ladder: reappearing state retries, unreadable path is not absence, confirmed absence fails with a non-empty result and `continuity: unavailable`
- [x] 7.5 `node --test` passes with no network access

## 8. Image

- [ ] 8.1 `runtimes/copilot/Dockerfile` on `node:22-bookworm-slim`: git, openssh-client, curl, jq, ca-certificates, procps; install the SDK; `HOME=/data/context`; non-root; the `org.opencontainers.image.source` LABEL; multi-arch — build `linux/arm64` locally and run `--version` before believing it
- [ ] 8.2 Carry the runtime-claude comment forbidding domain tooling, naming the derive-your-own-image escape hatch
- [ ] 8.3 Publish by tag: `git tag runtime-copilot-v0.1.0 && git push origin runtime-copilot-v0.1.0`; confirm the run passed the Trivy gate, then the package's Actions access and visibility flip (UI, once), and check the REGISTRY rather than the tag

## 9. Chart: the `copilot` bundle

- [x] 9.1 Create `chart/charts/copilot/` — `Chart.yaml` 0.1.0, `values.yaml` (`enabled: false`, `name: copilot`, `default: false`, `image`, `credentialsSecret{name: agentops-copilot, key, envName: COPILOT_GITHUB_TOKEN, token: ""}`, `contextSync.paths: [".copilot/session-state/**"]`, optional `model`/`maxAiCredits`, the runtimeDefaults-override note), `templates/runtime.yaml` calling `agentops.renderRuntime`, the credential Secret template in the claude bundle's shape
- [x] 9.2 Add `agentops.copilotRuntimeEntry` to `chart/templates/_helpers.tpl` (model/credits → env) and add `copilot` to the `agentops.declaredRuntimes` bundle range — the default-runtime guard and the bootstrap env see nothing else
- [x] 9.3 `chart/Chart.yaml`: the dependency with `condition: copilot.enabled`; `chart/values.yaml`: the documented `copilot:` section beside `ollama:`
- [x] 9.4 Extend `internal/integration/charttemplate_test.go`: defaults render byte-identically; bundle on renders the runtime under its own name, its Secret when `token` is set, `default` still a copy of claude; bundle on with claude off makes copilot the default; `serviceaccount-guard.py` still passes
- [ ] 9.5 Bump the chart minor and record it in `docs/CHANGELOG.md`, newest first

## 10. Verify against a live install

- [ ] 10.1 Deploy the worktree's chart (`helmfile sync` with `chartPath` pointed at the worktree) after a server-side dry-run; confirm `agentops-conv-*` pods for a copilot route carry the `context-sync` sidecar and the egress init container
- [ ] 10.2 Point one Pipeline's `runtimeRef` at `copilot` and post a `kind: task` signal to a source it claims; confirm the answer reaches every bound thread
- [ ] 10.3 Reply in the thread and confirm the second run resumes the same context — then delete the session state and confirm the run FAILS with the cannot-be-continued message rather than answering fresh
- [ ] 10.4 Bind observation-only toolsets on one route and confirm shell is denied there while the same profile keeps it on another route; bind `Bash(kubectl:*)` and confirm a non-kubectl command is denied and logged

## 11. Documentation

Both halves, ticked separately; this section is last on purpose and the archive hook checks it.

- [ ] 11.1 Reference docs: `docs/contracts.md` — the third implementation and the per-runtime obligations it makes visible (definition path, vendor default neutralisation, unmapped-denies, what each runtime can enforce of a narrowing pattern); `docs/concepts.md` — capability resolution when the runtime's vocabulary differs, `Pipeline.spec.runtimeRef` as the whole vendor switch; `docs/CHANGELOG.md` entry
- [ ] 11.2 Adopter site: new `docs/runtimes/copilot.md` in the ollama page's shape (what it executes, what it needs, where its context lives, what it enforces, the `renders bundle=copilot` marker) plus the `docs-generate.py` bundle entry; `docs/_data/nav.yml`; `docs/installation.md` bundle row and section; `docs/index.md` and `README.md` chips and the "Works with"/runtimes line (`wc -l README.md` ≤ 215)
- [ ] 11.3 `python3 .github/scripts/docs-generate.py` then `--check`; `python3 .github/scripts/retired-vocabulary-guard.py` and `publication-guard.py` pass
- [ ] 11.4 Context: `.claude/rules/structure.md` (the component, the group table), `.claude/rules/wiring.md` (the agent definition path is per-runtime), `docs/CLAUDE.md` (the runtimes page kind now has two)
