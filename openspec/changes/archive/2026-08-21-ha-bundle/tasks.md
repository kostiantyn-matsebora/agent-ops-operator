# Tasks: ha-bundle

## 1. Signal adapter module

- [x] 1.1 Scaffold `signal-ha/` as its own Go module (no dependencies outside the repo, no Kubernetes client), with `Dockerfile` and the image build line added to `CLAUDE.md`
- [x] 1.2 Implement the Home Assistant client over `net/http`: endpoint + long-lived token from env, and the read loop for the source settled in the design's first open question
- [x] 1.3 Implement the manager side of the `/signal` contract: normalized signals (`fingerprint`, `labels`, `title`, `payload`, `kind`) posted to `/signal/inbound` with bearer auth
- [x] 1.4 Implement cursor persistence via `/signal/state`, resuming on restart and re-reading rather than stalling when the position is no longer valid upstream
- [x] 1.5 Port the `rules` engine from `signal-k8s-events`: ordered first-match-wins, Alertmanager matchers over labels, drop-or-hold with a Prometheus-named dwell and a re-check before emitting
- [x] 1.6 Port `route` inhibition
- [x] 1.7 Implement self-exclusion so nothing attributable to agent-ops becomes a signal, following the layered approach in `signal-k8s-events/selfexclude.go` (a mechanism that needs no API read must hold with a cold cache)
- [x] 1.8 Fail fast with a stated reason when the endpoint or credential is missing, rather than running and reporting nothing
- [x] 1.9 Ship default rules honouring both vocabulary rules: zero dwell for conditions describing something already completed, and a catch-all LAST rule with a dwell rather than a drop
- [x] 1.10 Tests: rule ordering and first-match-wins, dwell re-check, cursor resume, stale-cursor recovery, self-exclusion with a cold cache, missing-config startup

## 2. Subchart scaffold

- [x] 2.1 Create `chart/charts/ha-bundle/` (Chart.yaml, values.yaml, `_helpers.tpl`), self-gated with `enabled: false`, mirroring `k8s-bundle`'s gating helper
- [x] 2.2 Write `values.yaml` with per-component blocks and documented defaults: ingest lane, `mcp`, `toolsets`, `profiles.user`, `profiles.ops`, `pipelines` (with `enabled: true`), and the surface names (`console`, `telegram`) used by the wiring
- [x] 2.3 Add the `ha-bundle:` block to the parent `chart/values.yaml` with the pointer comment the other bundles use, and bump the chart version

## 3. Ingest lane

- [x] 3.1 `signaladapter.yaml`: the `SignalAdapter` CR with `kubernetesAccess: false`, `singleton: true`, and a `configSchema` describing `rules` and `route` so `kubectl get signaladapter` answers "what goes in spec.config?"
- [x] 3.2 `signalsource.yaml`: the `SignalSource` with grouping, opaque `config` passthrough, and `credentialsSecretRef` for the HA token — referenced by name, never created
- [x] 3.3 Gate the whole lane on its component flag so an ingest-free install renders neither object

## 4. Tooling

- [x] 4.1 `mcp.yaml`: `MCPConfig ha-api` with the server key FIXED in the template, rendered only when the endpoint is configured
- [x] 4.2 `toolsets.yaml`: `MCPToolset ha-observability` (read state, history, logbook) and `ha-admin` (call services, change configuration) as ENUMERATED patterns, never a server-wide wildcard
- [x] 4.3 Render `ha-actions` whenever the MCP component does — NOT gated on a credential, because Home Assistant registers the same Assist intents for any user and has no read-only server mode to detect
- [x] 4.4 ADDED IN IMPLEMENTATION: a SECOND MCP path (`adminMcp` + `adminMcpServer`, both default-off) bound to the ops route alone. Home Assistant's built-in server exposes Assist intents ONLY, so the ops agent could not read a log, reload an integration or disable an entity — the lane did not work without it. Two install paths (chart-deployed container, or a URL for a server you run, since add-ons do not exist on HA Core), tools ENUMERATED from the running server's own `tools/list` (52 of 78 ship), server key FIXED at `homeassistant_admin`, own ServiceAccount

## 5. Profiles

- [x] 5.1 `profile-user.yaml`: `AgentProfile ha-user` — inline `systemPrompt` role, connectivity `env` via `valueFrom`, `maxTurns`, optional `runtimeRef`; rendered when EITHER the MCP endpoint OR the read credential is set
- [x] 5.2 `profile-ops.yaml`: `AgentProfile ha-operator` (renamed from `ha-ops` — the Pipeline keeps that name) — same shape, admin credential as its rendering prerequisite, MCP optional
- [x] 5.3 Confirm neither template emits any tool allowlist or MCP server field — capabilities are wiring, and this is the field a previous change deleted by mistake
- [x] 5.4 Write both role prompts: `ha-user` asks and reports; `ha-ops` acts, and describes-then-stops for anything destructive or outside its remit

## 6. Wiring

- [x] 6.1 `pipelines.yaml`: `Pipeline ha-control` — profile `ha-user`, claiming the chat sources named in values, delivering to the named channels, binding `ha-observability`
- [x] 6.2 `Pipeline ha-ops` — profile `ha-operator`, listing `ha-logs` ONLY as a source (never a chat source: addressing already ignores claims, so listing one grants nothing — and, since `sourceConflicts` was deleted, what it now costs is the unaddressed lane, which the manager REFUSES when two pipelines claim one chat surface), delivering to the same channels, binding both toolsets
- [x] 6.3 Gate both on `pipelines.enabled` (default FALSE — the fourth condition of the invariant this change relaxes; see design D3), and each additionally on its own profile rendering
- [x] 6.4 Omit every unset surface reference entirely, so no rendered Pipeline names an object no component created
- [x] 6.5 Extend `NOTES.txt` with the escalation step (`/ha-ops <task>` from any wired surface) and the parent-scope wiring to declare when `pipelines.enabled` is false

## 7. Specs and invariants

- [x] 7.1 Rewrite the `pipeline-model` invariant in `CLAUDE.md` — ALREADY REWRITTEN by a later change, with four conditions rather than three, and `k8s-bundle`/`prometheus-bundle` now ship routes. Added `ha-bundle` as the third qualifying case; `telegram-bundle` remains the counter-example
- [x] 7.2 Add the terminology and map entries for `signal-ha/` and `chart/charts/ha-bundle/` to `CLAUDE.md`
- [x] 7.3 Record in `CLAUDE.md` that CLAIMING and ADDRESSING are independent — a claim decides who answers an unaddressed message, while `/<pipeline>` resolves by name with no claim and no Ready check — so several pipelines share one surface without sharing its source, and listing a source a Pipeline does not need costs `Ready=False` and its place in `/agents`

## 8. Documentation

- [x] 8.1 Write `docs/ha-bundle.md`: components and their gates, values table, prerequisites, the two lanes and how to reach each, and the adoption path for hand-applied installs
- [x] 8.2 Document the credential-path asymmetry: toolsets constrain the MCP path, an admin credential plus a shell tool is a second and wider path, and omitting the shell toolset from the ops Pipeline is the tighter posture
- [x] 8.3 Document the adapter's `rules`/`route` configuration in `docs/ha-bundle.md`, pointing at the cluster-events page rather than restating the vocabulary
- [x] 8.4 Add the `CHANGELOG.md` entry (newest first), leading with the relaxed subchart-wiring rule and what it does and does not permit

## 9. Verification

- [x] 9.1 `go build ./... && go vet ./... && go test ./...` in `signal-ha/`, plus the root and every other satellite module
- [x] 9.2 `helm lint` and a `helm template` matrix: default (nothing); enabled-full; ingest-only; chat-only; no admin credential (no ops profile, no ops Pipeline); no telegram (no telegram references anywhere); `pipelines.enabled=false` (no Pipeline, everything else intact)
- [x] 9.3 Assert in every partial render that no Pipeline names an object absent from that render
- [x] 9.4 Extend `internal/integration/charttemplate_test.go` to pin: the wiring flag default, `ha-ops` claiming no chat source, the enumerated (non-wildcard) toolsets, and the fixed MCP server key
- [x] 9.5 Server-side dry-run of the enabled-full render against the live cluster before any apply
- [x] 9.6 Live smoke: post a signal to `ha-logs` and confirm it reaches the ops agent; type `/ha-ops <task>` on a console surface claimed by `ha-control` and confirm the reply lands in that thread with the ops capabilities
