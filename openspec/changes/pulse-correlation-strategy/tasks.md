## 1. Adapter config and derivation — backend-developer

- [ ] 1.1 Add a `correlationStrategy` field to the adapter's per-source
  config parsing (alongside `baseUrl`/`webhookUrl`/`webhookHeaders` in
  `signals/pulse/main.go`/`manager.go`), accepting `"id"` or
  `"id+startTime"`, defaulting to `"id+startTime"` on absent, empty, or
  any unrecognized value. Verify with a table-driven unit test covering
  absent, empty, `"id"`, `"id+startTime"`, and an unrecognized string.
- [ ] 1.2 Change `pulseEvent.initiationID()` in `signals/pulse/payload.go`
  to take the resolved strategy and, for `"id+startTime"`, build
  `pulse:<resource>:<id>:<startsAt>` instead of `pulse:<resource>:<id>`.
  Keep the `"id"` strategy's current string shape and the no-ID hash
  fallback unchanged. Verify with `go test ./...` in `signals/pulse`.
- [ ] 1.3 Wire the served source's resolved strategy through to
  `normalize()`/the webhook handler in `signals/pulse/main.go` so the
  right strategy is applied per request. Verify with a unit test posting
  the same alert id with two different start times under each strategy
  and asserting the resulting `initiationId`s.

## 2. Chart support — deployment-engineer

- [ ] 2.1 Add `signal.defaultSource.correlationStrategy` (optional, unset
  by default so the adapter's own default applies) to
  `chart/charts/pulse/values.yaml`, and render it into the `SignalSource`'s
  `spec.config` in `chart/charts/pulse/templates/signal.yaml` the same way
  `webhookHeaders` is rendered. Add the field to the `SignalAdapter`'s
  `configSchema` in the same template, enum `["id", "id+startTime"]`.
  Verify with `helm template` against the chart's render tests.
- [ ] 2.2 Re-run `python3 .github/scripts/docs-generate.py` so any
  generated values table or `cr-reference.md` block picks up the new
  field, and verify with `python3 .github/scripts/docs-generate.py --check`.

## 3. Unit tests

- [ ] 3.1 Run `go test ./...` and `go vet ./...` in `signals/pulse` and
  confirm all new and existing tests pass, including
  `TestInitiationIDStableAcrossFiringAndResolvedPair`,
  `TestInitiationIDFallbackWithoutID`, and the new strategy-selection
  tests from section 1.
- [ ] 3.2 Run the chart's render test suite (`helm` required in the test
  container per `.claude/rules/build-test.md`) and confirm the new
  `correlationStrategy` field renders and validates against the updated
  `configSchema`.

## 4. System tests

- [ ] 4.1 Nothing here is decided by a cluster — `initiationId` derivation
  and the config default are pure adapter-local logic, covered entirely by
  unit tests. The manager treats the resulting id opaquely (`MGR-3`) and
  is untouched. No `platform/manager/test/system/` lane is added.

## 5. Documentation

### Reference docs

- [ ] 5.1 Update `openspec/specs/pulse-signal-adapter/spec.md`'s
  `initiationId` requirement (already drafted as this change's delta —
  confirm it matches the shipped behavior once 1.1–1.3 land).
- [ ] 5.2 Add the `correlationStrategy` field and its default to
  `docs/configuration.md` (a reversible chart/source value, same section
  as other Pulse bundle values).
- [ ] 5.3 Add a breaking-default-change entry to `docs/CHANGELOG.md`,
  newest first, naming the old (`"id"`) and new (`"id+startTime"`)
  behavior and how to opt back into the old one.

### Adopter site

- [ ] 5.4 Update `docs/integrations/pulse.md` with dense, human-first prose
  explaining why Pulse's alert id alone isn't a safe correlation key and
  how to choose a `correlationStrategy` when the default doesn't fit —
  not a generated values table.
