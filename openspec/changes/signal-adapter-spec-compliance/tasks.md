# Tasks — signal-adapter-spec-compliance

The delta specs are normative for behavior, design.md for mechanism. Audit
code locations are in the proposal and deltas — re-verify against the tree,
not the audit.

## 1. Pulse adapter fixes — backend-developer

- [x] 1.1 Register `X-Agentops-Secret-Digest: sha256:<hex>` of the signing
  secret in `webhookHeaders` and keep it in the create-or-repair comparison,
  so a rotated secret updates the webhook in place — verify with a unit test
  rotating the secret against a redacting fake Pulse.
- [x] 1.2 Make signature enforcement follow the secret, not registration
  state: a configured `signingSecret` verifies inbound posts even when
  `baseUrl`/`webhookUrl` are absent or invalid — verify with a
  hand-registered-Pulse unit test.
- [x] 1.3 Name the source in the 404 body for an unknown or unserved
  webhook path — verify by unit test asserting the body names it.
- [x] 1.4 Purge stale comments: manager-side "grouping" (manager.go,
  main.go) and current-tense "fingerprint" (payload.go) — verify
  `grep -rn 'grouping\|fingerprint' signals/pulse/` returns only record
  markers or nothing.

## 2. Alertmanager adapter fixes — backend-developer

- [x] 2.1 Rewrite the degraded-registration remediation to name
  `spec.serviceAccountName` and the chart-rendered account, never a retired
  field — verify by unit test on the message text.
- [x] 2.2 Make `send_resolved` a `*bool` omitted when unset, matching every
  other knob — verify the omission unit test now covers it.
- [x] 2.3 Degrade to RegistrationManual instead of writing a placeholder
  URL when `POD_NAMESPACE` is unset with a mounted token — verify by unit
  test on that branch.
- [x] 2.4 Surface a malformed `register` block as RegistrationManual naming
  the parse problem, distinct from an absent block — verify by unit test
  with a mistyped block.
- [x] 2.5 Purge the stale `kubernetesAccess` comment in main.go — verify by
  grep as in 1.4.

## 3. k8s-events adapter fixes — backend-developer

- [x] 3.1 Rewrite the fatal-error and selfexclude texts naming the deleted
  `kubernetesAccess` field to name `spec.serviceAccountName` — verify by
  unit test on the message text.
- [x] 3.2 Report `ruleSet.warnings` on the source's Ready condition from
  `refreshSources` — verify by unit test with a shadowed rule.
- [x] 3.3 Split the unknown verdict: unreadable own state (unsynced cache,
  denied reads at the close) emits fail-open, a tracked kind absent from a
  synced cache stays recurrence-gated — verify both paths by unit test.
- [x] 3.4 Close mute windows from the existing sweep timer: `MuteEnded`
  with the count fires without a next batch, the active report carries the
  running count — verify by unit test with a lane that goes quiet.
- [x] 3.5 On a relist pass hitting the batch cap, advance the cursor only
  to the newest examined event and report the clip on Ready — verify by
  unit test with >cap matches out of time order.
- [x] 3.6 Bound the per-pod fallback id to one informer resync tick as
  design.md states, and document the `emitDrainExceeded` cap bypass in code
  — verify fallback by unit test, bypass by the comment stating its bound.
- [x] 3.7 Purge stale comments: package-doc fingerprint id shape,
  `signatureLabels` in enrich.go, the outdated nodes claim in pending.go —
  verify by grep as in 1.4.

## 4. HA adapter fixes — backend-developer

- [x] 4.1 Add the unreadable-log verdict (nil or stale snapshot) distinct
  from no-predicate unknown, and emit on it unconditionally in `decide()` —
  verify by replacing `TestUnknownWithNoRecurrenceIsDropped`'s expectation
  for that path and a new stale-snapshot test.
- [x] 4.2 Clamp the signal payload so a batch fits the contract's 1 MiB
  inbound bound, noting the clamp in the payload — verify by unit test with
  an oversized exception.
- [x] 4.3 Purge current-tense "fingerprint" comments (main.go, rules.go,
  surfaces.go) — verify by grep as in 1.4.

## 5. Cron and telegram adapter fixes — backend-developer

- [x] 5.1 Prune cron's `reported` map to the served set on every refresh —
  verify by unit test: delete + recreate a source with identical invalid
  config re-reports Ready=False.
- [x] 5.2 Truncate telegram's `title()` on a rune boundary — verify by unit
  test with a multi-byte message around the cut.
- [x] 5.3 Purge stale "grouping" comments in both adapters and cron's
  two-value kind enum comment — verify by grep as in 1.4.

## 6. Conformance suite — testing-specialist

- [ ] 6.1 Build and run `signals/pulse` in
  `platform/manager/test/conformance/` like the other five, asserting its
  v2.0.0+ signal shape — verify `go test -tags conformance ./test/conformance/`
  passes with the new section.
- [ ] 6.2 Add the fake-manager-observable assertions the delta names: the
  pulse 404 naming its source, and the refractory window never suppressing
  a distinct id — verify the suite run.

## 7. Unit tests

- [ ] 7.1 `go build ./... && go vet ./... && go test ./...` green in every
  touched module: `signals/{pulse,alertmanager,k8s-events,ha,cron,telegram}`.
- [ ] 7.2 `go test ./...` green in `platform/manager` (conformance helpers
  live in its module) plus `go test -tags conformance ./test/conformance/`.

## 8. System tests

- [ ] 8.1 Nothing here is decided by a cluster — every fix is
  adapter-internal logic or spec text, pinned by unit tests and the
  cluster-less conformance suite. No kubelet, RBAC, informer or pod
  lifecycle behavior changed.

## 9. Documentation

Reference docs:

- [ ] 9.1 `docs/contracts/signal-adapter.md`: the refractory allowance, the
  2.2.0 changelog entry, version moved, `signals/pulse` added to every
  shipped-adapter enumeration on the page including the severity-senders
  table — **the approved deltas authorize this edit** (`signal-rules.md`)
  — verify against the `signal-adapter-contract` delta.
- [ ] 9.2 Claimed versions move to 2.2.0: `docs/integrations/pulse.md`
  (claim added), `home-assistant.md`, `kubernetes.md`, `prometheus.md`,
  `telegram.md`, and `docs/guides/signal-adapter.md` for cron — verify by
  grep for the version across the six pages.
- [ ] 9.3 `docs/CHANGELOG.md`: the observable fixes (pulse secret rotation,
  alertmanager degradation, k8s-events clip/mute/fail-open, ha fail-open),
  newest first.
- [ ] 9.4 `.claude/rules/signal-rules.md` and `.claude/rules/structure.md`:
  the five-adapter enumerations gain `signals/pulse` — verify by grep.

Adopter site:

- [ ] 9.5 `docs/guides/signal-adapter.md` is the one adopter page touched
  (9.2). Landing page, introduction, getting-started, installation and the
  other guides describe none of the changed behaviours — re-check at finish
  and tick as the claim.
