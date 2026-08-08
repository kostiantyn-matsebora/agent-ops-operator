# pipeline-addressed-conversations

## Why

The Pipeline initiates a conversation, so the Pipeline is what a caller should address. Today `POST /task` addresses a **profile**, which left it with no wiring and therefore no capabilities once `capabilities-are-wiring` landed — and that hole was patched with "capability-only Pipelines": a Pipeline with no sources and no channels, declaring a profile's baseline. That is the profile's default capabilities stored one indirection away, and it exists only to serve the two paths that address a profile instead of a route.

It also produced a live regression. Both bundles render routing Pipelines that declare no capabilities, and the baseline does not reach them, so every signal-driven conversation in `master` dispatches with an empty allowlist — the product's headline flow, an alert arriving and an agent investigating it, currently yields an agent that can do nothing.

Addressing conversations to Pipelines removes the exception rather than patching it. `chat-signal-origination` closes the other profile-addressed path independently.

## What Changes

- **BREAKING**: `POST /task` addresses a Pipeline. `{"pipeline": "...", "task": "..."}` — the profile, channels, and capabilities all come from it. The `profile` field is removed; a request without a resolvable Pipeline is rejected rather than silently creating a capability-less conversation.
- **The baseline concept is deleted**: capability-only Pipelines, `CapabilityPipelineForProfile`, `IsCapabilityPipeline`, the `BaselineConflict` condition and its reconciler check, and the k8s-bundle's baseline template and values. A Pipeline with no sources and no channels goes back to meaning nothing in particular.
- **Capabilities are declared per Pipeline, never inferred** — no inheritance, no default, no warning when a route grants nothing (that is the operator's call). The bundles ship agents, so THEIR Pipelines gain explicit `toolsets` bindings; that is the regression fix.
- **The chart ships addressable Pipelines**: k8s-bundle renders one per agent so the demo has something to address, under demo mode and behind a values flag for production installs.
- Chat addressing (`/<pipeline>` rather than `/<profile>`) is **out of scope here** — `chat-signal-origination` restructures chat origination wholesale and owns it. This change removes the baseline that path relied on; the two must land together or that path loses capabilities in between.

## Capabilities

### Modified Capabilities

- `profile-is-identity`: the capability-only Pipeline requirement is removed; a profile's capabilities come from whichever Pipeline routes a given conversation, with no per-profile default.
- `mcp-toolset-model`: bindings materialize from the originating Pipeline alone — no baseline resolution, and "no bindings" means the Pipeline declared none.
- `pipeline-model`: a sourceless, channelless Pipeline is no longer a declared concept; Pipelines are addressable by name via the task API.
- `k8s-bundle`: renders addressable per-agent Pipelines instead of a baseline, and its events Pipeline declares capabilities explicitly.
- `vm-bundle`: its default-source Pipeline declares capabilities explicitly.

## Impact

- **API**: none on CRDs. `POST /task` request shape changes (BREAKING).
- **Controller**: `pipeline_controller.go` loses the baseline conflict check; `internal/chat/pipelines.go` loses two helpers; `internal/httpapi/server.go` reworks `handleTask`; `internal/chat/router.go` loses its baseline lookup.
- **Chart**: k8s-bundle baseline template → addressable Pipelines; both bundles' routing Pipelines gain `toolsets`; README demo curl names a pipeline.
- **Docs**: README capability + demo sections, CLAUDE.md terminology.
- **Interacts with `chat-signal-origination`**: that change removes `/<profile>` chat origination and routes chat through a claimed source. Land them together; this one alone would leave chat commands capability-less.
- **Regression closed**: signal-driven conversations get capabilities again, pinned by a test asserting the bundle's own events Pipeline yields a non-empty allowlist.
