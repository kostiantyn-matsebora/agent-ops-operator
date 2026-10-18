## Why

`chart/templates/coordinator.yaml` derives the chart-rendered Coordinator's
`agents[]` from `agentops.coordinatorContributions` — every enabled bundle's
own capability. Values cannot state a member directly, and cannot add a
`coordinatorRef` member at all, so an install that wants a nested tree of
Coordinators has no way to express it through the chart.

## What Changes

- Add `coordinator.agents` (list, default `[]`) to `chart/values.yaml`.
- Empty (today's default): `chart/templates/coordinator.yaml` renders the
  `agentops.coordinatorContributions`-derived roster exactly as it does today
  — byte-identical output for existing installs.
- Set: the list REPLACES the contributions-derived roster entirely. Each
  entry carries `name`, `description`, and either `capability` (mapped to
  `capabilityRef`) or `coordinator` (mapped to `coordinatorRef`).
- The reaper fold-in (`reaper.enabled`), the console claim, `channelRefs`,
  `limits` and `signalSourceRefs` keep deriving and appending exactly as
  today, on top of whichever roster (derived or overridden) is in play. The
  generated `systemPrompt` is untouched.

## Capabilities

### New Capabilities
(none)

### Modified Capabilities
(none — `coordinator-model` documents the `Coordinator` CRD's own contract,
independent of how a chart populates `spec.agents[]`. This change is a
chart-values rendering choice, not a CRD-level requirement change. No test
tier applies beyond the chart's own render-test suite.)

## Impact

- **Code**: `chart/templates/coordinator.yaml`, `chart/values.yaml`.
- **Tests**: `platform/manager/internal/integration/charttemplate_*_test.go`
  (new render-test cases for empty vs. set `coordinator.agents`, and the
  reaper fold-in on top of an override).
- **Reference docs**: `docs/configuration.md`'s "Wiring posture: pipelines or
  coordinator" section gains the new key (reversible chart value, not an
  install/upgrade/storage concern, so `docs/installation.md` is untouched).
  `docs/concepts.md` is untouched — it describes the `Coordinator` CRD, not
  the chart's own values surface.
- **Adopter site**: `docs/getting-started.md`'s coordinator-mode walkthrough
  is untouched — it demonstrates the derived roster, which stays the
  default. No landing-page, installation, or guide change — this is an
  advanced override with no new install path.

## Non-Goals

- No contribution routing or targeting mechanism.
- No per-bundle knobs.
- No `wiringEnabled` changes.
- No CRD changes — `Coordinator.spec.agents[]` already supports both
  `capabilityRef` and `coordinatorRef` entries.
