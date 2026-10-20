## 1. Adapter config and derivation — backend-developer

- [x] 1.1 Add a `correlationStrategy` field to the adapter's per-source
  config parsing (alongside `baseUrl`/`webhookUrl`/`webhookHeaders` in
  `signals/pulse/main.go`/`manager.go`), accepting `"id"` or
  `"id+startTime"`, defaulting to `"id+startTime"` on absent, empty, or
  any unrecognized value. Verify with a table-driven unit test covering
  absent, empty, `"id"`, `"id+startTime"`, and an unrecognized string.
- [x] 1.2 Change `pulseEvent.initiationID()` in `signals/pulse/payload.go`
  to take the resolved strategy and, for `"id+startTime"`, build
  `pulse:<resource>:<id>:<startsAt>` instead of `pulse:<resource>:<id>`.
  Keep the `"id"` strategy's current string shape and the no-ID hash
  fallback unchanged. Verify with `go test ./...` in `signals/pulse`.
  Also corrected the registered template's `startsAt` field from
  `{{.Timestamp}}` (undefined, not occurrence-stable per Pulse's own docs)
  to `{{.StartTime}}` (confirmed via Pulse's own reference templates and
  docs to persist across an occurrence's lifecycle) — see design.md's
  Risks section. Still needs live-instance confirmation.
- [x] 1.3 Wire the served source's resolved strategy through to
  `normalize()`/the webhook handler in `signals/pulse/main.go` so the
  right strategy is applied per request. Verify with a unit test posting
  the same alert id with two different start times under each strategy
  and asserting the resulting `initiationId`s.

## 2. Chart support — deployment-engineer

- [x] 2.1 Add `signal.defaultSource.correlationStrategy` (optional, unset
  by default so the adapter's own default applies) to
  `chart/charts/pulse/values.yaml`, and render it into the `SignalSource`'s
  `spec.config` in `chart/charts/pulse/templates/signal.yaml` the same way
  `webhookHeaders` is rendered. Add the field to the `SignalAdapter`'s
  `configSchema` in the same template, enum `["id", "id+startTime"]`.
  Verify with `helm template` against the chart's render tests.
- [x] 2.2 Re-run `python3 .github/scripts/docs-generate.py` so any
  generated values table or `cr-reference.md` block picks up the new
  field, and verify with `python3 .github/scripts/docs-generate.py --check`.

## 3. Unit tests

- [x] 3.1 Run `go test ./...` and `go vet ./...` in `signals/pulse` and
  confirm all new and existing tests pass, including
  `TestInitiationIDStableAcrossFiringAndResolvedPair`,
  `TestInitiationIDFallbackWithoutID`, and the new strategy-selection
  tests from section 1.
- [x] 3.2 Run the chart's render test suite (`helm` required in the test
  container per `.claude/rules/build-test.md`) and confirm the new
  `correlationStrategy` field renders and validates against the updated
  `configSchema`. Added `TestPulseCorrelationStrategyRidesSourceConfig`,
  `TestPulseCorrelationStrategyOmittedWhenUnset`, and
  `TestPulseConfigSchemaEnumeratesCorrelationStrategyValues` to
  `platform/manager/internal/integration/charttemplate_pulse_test.go`,
  mirroring the existing `webhookHeaders` coverage.

## 4. System tests

- [x] 4.1 Nothing here is decided by a cluster — `initiationId` derivation
  and the config default are pure adapter-local logic, covered entirely by
  unit tests. The manager treats the resulting id opaquely (`MGR-3`) and
  is untouched. No `platform/manager/test/system/` lane is added.

## 5. Documentation

### Reference docs

- [x] 5.1 Update `openspec/specs/pulse-signal-adapter/spec.md`'s
  `initiationId` requirement (already drafted as this change's delta —
  confirmed by the api-architect cross-role review to match the shipped
  behavior).
- [x] 5.2 No Pulse-bundle-specific value exists in `docs/configuration.md`
  today — every other Pulse source/webhook knob (`webhookHeaders`,
  `webhookUrl`, `baseUrl`) is documented on the adopter page instead, with
  `docs/configuration.md` reserved for the parent chart's own values.
  `correlationStrategy` follows that existing precedent (see 5.4) rather
  than starting a new one.
- [x] 5.3 Add a breaking-default-change entry to `docs/CHANGELOG.md`,
  newest first, naming the old (`"id"`) and new (`"id+startTime"`)
  behavior and how to opt back into the old one.

### Adopter site

- [x] 5.4 Update `docs/integrations/pulse.md` with dense, human-first prose
  explaining why Pulse's alert id alone isn't a safe correlation key and
  how to choose a `correlationStrategy` when the default doesn't fit —
  not a generated values table.
