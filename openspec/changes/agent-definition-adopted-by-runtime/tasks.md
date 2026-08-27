## 1. The manager's prompts

- [ ] 1.1 `dispatch/templates/task.md`: drop the "Adopt the agent role" section; keep one sentence of fallback posture (cautious SRE/platform advisor within your tools, observe before you act) with no file, no path, no "mention if missing"
- [ ] 1.2 `dispatch/templates/investigate.md`: the same for its inline sentence
- [ ] 1.3 A dispatch test asserting no rendered prompt for any lane contains `agents/` or `<agent>.md` (the rendered `AGENT_NAME` followed by `.md`) with an empty or a set `AGENT_NAME` — scoped to the definition path, since the Rules line legitimately names `CLAUDE.md`
- [ ] 1.4 Remove the `.claude/agents/<agent>.md` mentions from `dispatch.go`'s comments; the path is the runtime's

## 2. The runtimes adopt the body

- [ ] 2.1 `runtimes/claude/tools.js`: `agentDefinition(workspace, agent, log)` → `{tools, body}`; body is the text after the closing `---` (the whole file when there is no frontmatter); empty agent → `{tools: [], body: ''}`; absent named file logged once
- [ ] 2.2 `runtimes/claude/runtime.js`: `--append-system-prompt` carries `body` then `unit.systemPrompt`, joined by a blank line; tests for both present, one present, neither
- [ ] 2.3 `runtimes/copilot/tools.js` + `runtime.js`: the same reader; `systemMessage.append` content is body + inline, supplied on create and on resume
- [ ] 2.4 `runtimes/ollama/tools.go` + `agent.go`: the reader returns the body; the system message is base + body + inline
- [ ] 2.5 Unit tests in all three: body extraction (frontmatter present / absent / unclosed), empty agent, absent named agent logged

## 3. Images and chart

- [ ] 3.1 Tag `manager`, `runtime-claude`, `runtime-copilot`, `runtime-ollama`; confirm each run passed the scan and the registry holds the tag
- [ ] 3.2 Move the chart's `image.tag` and the three bundles' `image` to the new tags; bump the chart minor and `appVersion`

## 4. Verify on a live install

- [ ] 4.1 Deploy the worktree's chart; a repo-less profile's task answers with no role-file mention in title or body, on copilot and on claude
- [ ] 4.2 A repo-backed profile naming an agent: the pod log shows the body appended, and the answer reflects the role; a reply (resume) still does

## 5. Documentation

Both halves, ticked separately; last on purpose.

- [ ] 5.1 Reference docs: `docs/contracts.md` — the work contract's agent obligation is "read the definition, compose its tools, ADOPT its body; the prompt names no file"; `docs/concepts.md` — the agent role file paragraph and `toolsets.mode`; `docs/CHANGELOG.md`
- [ ] 5.2 Adopter site: `docs/guides/agent-runtime.md` (what a runtime owes), the profile guide's `spec.agent` explanation, `docs/runtimes/copilot.md` and `docs/runtimes/ollama.md` (the role, beside the tools), `docs/claude.md`
- [ ] 5.3 `python3 .github/scripts/docs-generate.py` and `--check`; publication and retired-vocabulary guards
- [ ] 5.4 Context: `.claude/rules/wiring.md` (the definition paragraph), `.claude/rules/structure.md` (`internal/dispatch/` — templates name no file)
