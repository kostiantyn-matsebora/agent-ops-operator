# Design: ha-bundle

## Context

- The reference content exists twice: live on home-data-center (kubectl-applied `AgentProfile ha-engineer`, `MCPConfig home-observability`, `SignalSource alertmanager`) and as `config/samples/samples.yaml`. The bundle turns that shape into values-driven templates.
- The parent chart has no `dependencies` yet; gated bundles so far are flat templates (`demo.yaml`, `telegram-adapter.yaml`). The user explicitly wants a **subchart** — the right call for a domain bundle: own values namespace, own version, cleanly omittable, and the pattern future bundles copy.
- MCP forms available: `MCPConfig` refs + inline servers with `valueFrom` headers/env (`mcpcompile` handles both). Secrets must be referenced by name only (manager-reads-no-secrets invariant shapes everything).
- Signal ingest: built-in `alertmanagerWebhook` works today for log alerts (vmalert → alertmanager → webhook). Sibling planned changes (`vm-alert-manager-as-signal-source`) may later provide a direct adapter type — the bundle's SignalSource `type`+`config` are plain values, so switching types later is a values edit, not a chart change.

## Goals / Non-Goals

**Goals:**
- `--set ha-bundle.enabled=true` + existing secrets + a few URLs = the full HA agent stack.
- Zero hardcoded environment specifics (URLs, chat ids, repo) — everything a value with the sample defaults.
- A reusable precedent: the subchart layout IS the "how to ship your own bundle" documentation.

**Non-Goals:**
- No secret creation or generation (referenced by name; prerequisites documented).
- No Channel/Pipeline objects (surfaces and wiring stay the operator owner's concern; `channelRef` is an optional value).
- No new operator/API/contract behavior — pure packaging.
- No log-tailing signal adapter (the lane is alert-driven via the built-in webhook type; direct VM/log adapters are their own changes).

## Decisions

### D1: True subchart, condition-gated
`chart/charts/ha-bundle/` as an unpacked subchart; parent `Chart.yaml`:
```yaml
dependencies:
  - name: ha-bundle
    version: 0.1.0
    repository: "file://charts/ha-bundle"
    condition: ha-bundle.enabled
```
Disabled by default (`ha-bundle.enabled: false` in parent values). CRDs come from the parent (subchart ships none; same CRD-ordering caveat as every gated CR template). Note: an unpacked `charts/` directory is used directly by helm — no `helm dependency update` needed for local installs; the dependency entry exists for lint/packaging correctness.

### D2: Values surface mirrors the sample CRs
```yaml
ha-bundle:
  enabled: false
  profile:
    name: ha-engineer
    agent: ha-engineer
    repository: {url: "", ref: master, sshSecretName: ""}   # empty url = repo-less profile
    allowedTools: "Read,Grep,Glob,Bash,Skill,mcp__victorialogs__*,mcp__victoriametrics__*,mcp__homeassistant__*"
    maxTurns: 60
    env: []                      # extra agent env (valueFrom allowed, names only)
    haTokenSecret: {name: agentops-ha, key: token, envName: HA_CLAUDE_TOKEN}
  mcp:
    name: ha-observability
    victorialogs: {url: ""}      # empty = omitted from the MCPConfig
    victoriametrics: {url: ""}
    homeassistant:
      url: ""
      authSecret: {name: agentops-ha, key: bearerHeader}   # Authorization header valueFrom
  signalSource:
    name: ha-logs
    type: alertmanagerWebhook    # switchable to a future adapter type via values
    config: {}                   # opaque per-type config passthrough
    channelRef: ""               # optional; "" = chat-less conversations
    grouping: {signatureLabels: [alertgroup, alertname], windowDays: 7, cooldownHours: 6}
```
Empty-string URL/name values omit the corresponding block (templates guard each). The HA MCP server and each observability server are independently omittable, so partial setups render valid CRs.

### D3: Secrets by name only
The bundle renders `secretKeyRef`/`secretRef` names from values and never a Secret manifest — consistent with the manager-reads-no-secrets posture and with how the live install manages its secrets. Prerequisites (repo key LF-only note included, per the known gotcha) go in the subchart README block.

### D4: Adoption path for hand-applied installs
home-data-center already runs same-shaped CRs applied by kubectl. Enabling the bundle with matching names would hit server-side-apply ownership conflicts (same class as the CRD conflict seen with helm v4). Documented options, in order: (a) keep the bundle disabled there (status quo, zero risk); (b) adopt: set bundle values to the live names, upgrade with `--force-conflicts` once, after which helm owns them; (c) fresh names side-by-side then retire the old CRs. The live-verification task uses (b) on a throwaway name first, then leaves the live install per the user's standing config (bundle disabled unless they choose adoption).

## Risks / Trade-offs

- [First subchart adds helm-dependency mechanics to the release flow] → unpacked directory keeps local installs zero-step; `helm lint`/`template` cover both states in CI-style checks.
- [Values drift vs samples.yaml] → samples stay the API documentation; the bundle README points at them; acceptable duplication (different purposes).
- [SignalSource type is a free value] → mis-typed values yield `Served=False` on the source — already diagnosable by design.
- [Adoption conflicts on the live install] → explicitly documented; default-disabled means upgrades never surprise it.

## Migration Plan

1. Ship subchart + parent dependency (chart minor bump). Default-disabled: upgrades render nothing new anywhere.
2. New installs: create the referenced secrets, set URLs, `ha-bundle.enabled=true`.
3. home-data-center: unchanged by default; adoption per D4(b) if/when wanted. Rollback = disable the flag (helm removes bundle-owned CRs; hand-applied ones are untouched).

## Open Questions

- Should the bundle also template an optional `Pipeline` (once `wire-it-up` lands) binding `ha-logs` → channels? Leaning yes as a follow-up value (`ha-bundle.pipeline.channels[]`), kept out of this change to avoid depending on an unimplemented CRD.
