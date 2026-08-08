## 1. API

- [x] 1.1 `AgentProfileSpec.SystemPrompt` (optional string), documented as identity-not-capability
- [x] 1.2 Regenerate deepcopy + CRDs into `chart/files/crds/`

## 2. Manager and runtime

- [x] 2.1 `dispatch`: carry `systemPrompt` on the work unit from the profile
- [x] 2.2 `runtime-claude`: pass `--append-system-prompt` when the unit has one, and log that it did
- [x] 2.3 Build and push new manager and runtime images; bump the chart's defaults

## 3. Chart

- [x] 3.1 `k8s-bundle.profile.systemPrompt` rendered onto the AgentProfile
- [x] 3.2 Ship a default role for the repo-less `k8s-engineer`: MCP-first, investigate before acting, bounded blast radius, never touch kube-system

## 4. Verify

- [x] 4.1 Render + server-side dry run
- [x] 4.2 Live: the profile carries the prompt, the runtime logs the append, and the agent describes its role back
- [x] 4.3 README: document `systemPrompt` and when to prefer a repo definition
