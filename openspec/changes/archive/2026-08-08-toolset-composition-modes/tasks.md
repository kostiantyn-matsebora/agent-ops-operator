> **Gated on 1.1.** The whole design rests on how `--allowedTools` and an agent
> definition's `tools:` frontmatter interact, which is inferred from Claude
> Code's architecture and NOT documented. Verify it against a real run before
> encoding it; guessing wrong produces agents whose declared tools are silently
> ignored, which is the defect this change exists to fix.
>
> Depends on `pipeline-addressed-conversations` landing first.

## 1. Establish the facts

- [x] 1.1 Run `claude -p` in a checkout containing `.claude/agents/probe.md` whose frontmatter declares a restrictive `tools:`, with (a) a broader `--allowedTools`, (b) a narrower one, (c) none plus `--permission-mode dontAsk`. Record what is actually enforced in each case
- [x] 1.2 From 1.1 decide whether `merge` needs runtime-side union at all, and whether `mcpConfigs` needs a mode (the agent definition contributes no MCP servers — see design Open Questions). Record both in `design.md`
- [x] 1.3 If the frontmatter turns out to govern the main session on its own, narrow this change to section 4 and reopen the mode question with the evidence — N/A, it does not

## 2. API and dispatch

- [x] 2.1 Restore `Mode` on `ToolingBinding` (`+kubebuilder:validation:Enum=merge;overwrite`, `+kubebuilder:default=merge`); apply to `toolsets` and, only if 1.2 says so, `mcpConfigs`
- [x] 2.2 Regenerate deepcopy + CRD YAML; existing Pipelines default to `merge`
- [x] 2.3 `internal/dispatch`: `WorkUnit` gains `toolsMode`; reword `EffectiveAllowedTools` so it reads as the WIRING'S CONTRIBUTION, not the final allowlist
- [x] 2.4 `internal/httpapi`: carry the conversation's toolsets mode onto the work unit
- [x] 2.5 Tests: the mode round-trips from Pipeline to work unit; absent mode defaults to `merge`

## 3. Runtime composition

- [x] 3.1 `runtime-claude`: resolve `.claude/agents/<agent>.md`, parse its YAML frontmatter, extract `tools:`; absent file, absent frontmatter, or unparseable frontmatter each contribute nothing and never block the run
- [x] 3.2 Compose per mode: `overwrite` = the work unit's tools alone; `merge` = agent's ∪ work unit's, deduped, agent's keeping position
- [x] 3.3 Remove the `|| 'Read'` fallback and add `--permission-mode dontAsk`, so an empty allowlist denies instead of prompting (a prompt in a pod is a hang)
- [x] 3.4 Runtime tests covering 3.1–3.3, including the unparseable-frontmatter path
- [x] 3.5 Build and push a new `agentops-runtime-claude` tag; update the chart/AgentRuntime defaults referencing it — pushed `agentops-runtime-claude:0.2.0` and `agentops-manager:0.18.0` (the manager changed too: the work unit gained `agent` + `toolsMode`); chart 3.2.0 points at both

## 4. Docs and verification

- [x] 4.1 README: the capabilities section gains the mode and what it composes against; the work-contract section stops describing `allowedTools` as the final word
- [x] 4.2 CLAUDE.md: `MCPToolset`/`Pipeline` terminology — the mode is against the AGENT DEFINITION, not the profile (the mistake that deleted it once already)
- [x] 4.3 `config/samples/samples.yaml`: a Pipeline showing each mode with a comment on what it composes against
- [x] 4.4 Release note: BREAKING for the runtime image — an agent that relied on the `Read` default now has exactly what was declared
- [x] 4.5 Full verification: `go build ./... && go vet ./...` in all modules, CRD regen clean, envtest suite, `helm lint` + template matrix
