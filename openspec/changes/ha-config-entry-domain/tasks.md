## 1. Domain extraction

- [ ] 1.1 In `signals/ha/config.go`, add domain extraction for
      `homeassistant.config_entries` records: a small ordered set of regexes
      matching the observed HA format-string shapes (`Error setting up entry
      %s for %s`, `Setup failed for '%s': %s`, `Config entry '%s' for %s
      integration not ready yet: ...`, and the `could not` variant already
      matched by the shipped rule), each anchored to the literal structure
      around the domain capture (see `design.md` — Decisions). Verify by
      compiling and running the new function against a raw message string in
      a throwaway `go run`/test snippet before wiring it in.
- [ ] 1.2 Wire the extractor into `normalize()`: when `rec.Name ==
      "homeassistant.config_entries"` and the extractor finds a domain, use
      that domain as the `integration` label (and therefore as the pending
      queue's grouping key and the `health()` lookup key) instead of
      `integrationOf(rec.Name)`. When it finds none, keep today's
      `integrationOf` result unchanged. Every other logger's path through
      `normalize()` is untouched. Verify by reading the diff: the change must
      touch only the `homeassistant.config_entries` branch.

## 2. Unit tests

- [ ] 2.1 Extend `TestIntegrationOf` (or add a sibling table-driven test) in
      `signals/ha/config_test.go` covering: the three known message shapes
      resolving to their domain (including the live-captured Tuya message
      `"Error setting up entry kmatsebora@gmail.com for tuya"` reduced to a
      placeholder title), a message with no recognizable domain falling back
      to the logger name, and a message containing an embedded "for ..."
      inside an unrelated exception string NOT being misattributed as a
      domain. Verify with `go test ./...` in `signals/ha/` (containerized per
      `.claude/rules/build-test.md` — no local Go toolchain).
- [ ] 2.2 Add or extend a `pending_test.go` / `adapter_test.go` case
      reproducing the incident end-to-end at the normalize→health layer: a
      single `homeassistant.config_entries` record naming a domain whose
      snapshot reports a failed config-entry state is confirmed (rung 1,
      `verdictUnhealthy`) even though it never recurs — proving the fix
      closes the exact gap in `proposal.md` — Why. Verify with `go test
      ./...` in `signals/ha/`.
- [ ] 2.3 Run the full module suite (`go build ./... && go vet ./... && go
      test ./...` in `signals/ha/`, per `.claude/rules/build-test.md`) and
      confirm it passes with no regressions to the existing `TestIntegrationOf`,
      `pending_test.go`, and `adapter_test.go` cases.

## 3. E2E tests

- [ ] 3.1 Not applicable — this change is pure log-message normalization
      inside `signals/ha/`, decided entirely by Go string matching. Nothing a
      cluster decides (the kubelet, RBAC, an informer, a pod's lifecycle,
      context continuity) is touched, so no lane in
      `platform/manager/test/e2e/` is added or extended.

## 4. Documentation

### Reference docs
- [ ] 4.1 Check `docs/concepts.md` and `docs/contracts.md` for any mention of
      the HA adapter's health-predicate/verification-ladder mechanism; none
      is expected (confirmed absent from `docs/integrations/home-assistant.md`
      during proposal), but confirm neither describes the old (buggy) behavior
      before closing this task.
- [ ] 4.2 Add a `.claude/rules/gotchas.md` entry recording the pitfall: Home
      Assistant's config-entry setup failures log under the core
      `homeassistant.config_entries` logger rather than under the failing
      integration's own logger namespace, so any future log-based
      classification keyed on logger-name prefix stripping will silently miss
      this class of record unless it special-cases this logger — exactly as
      `signal-ha`'s `integrationOf` did before this change. Add the matching
      entry to `.github/retired-vocabulary.json` only if this change removes
      or renames anything project vocabulary already names (expected: no —
      this is a bugfix, not a rename).

### Adopter site
- [ ] 4.3 Check `docs/integrations/home-assistant.md` for any adopter-facing
      claim about alerting reliability for failed integrations (e.g. "you'll
      be notified when an integration fails to set up"); if the page already
      makes that claim, no wording changes are needed since the claim becomes
      more true, not less — confirm and tick. If the page says nothing on the
      subject, no edit is owed.
