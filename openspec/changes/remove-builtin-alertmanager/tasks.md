# Tasks: remove-builtin-alertmanager

## 1. Remove the built-in transport

- [x] 1.1 Delete `POST /ingest/alertmanager/{source}` route, `handleAlertmanager`, `amAlert`/`amPayload` from `internal/httpapi/server.go`; delete `SourceAlertmanager` from `api/v1alpha1`; drop the built-in branch from `SignalSourceReconciler` (Served = adapter-only)
- [x] 1.2 Re-express AM grouping parity via the contract: replace `TestAlertGroupingAndRecurrence` with a `/signal/inbound` test covering multi-signal same-signature batching (one conversation, one combined input) — recurrence/cooldown already covered by signal tests

## 2. Finish the single-path operator experience (user feedback: no half-baked surface)

- [x] 2.1 Fix the dead `Profile` printcolumn on SignalSource (leftover from pipeline-only-wiring): columns become Type / Wired / Served-relevant state / Received / Age; regen CRDs
- [x] 2.2 Make the webhook endpoint discoverable: the vmalertmanager adapter's Ready message names the webhook path (`POST .../webhook/<source>`), and the parent chart gains `NOTES.txt` printing the full Service URL per rendered vm-bundle source when the bundle is enabled
- [x] 2.3 Docs: README quickstart/API table/migration section (built-in endpoint gone → adapter path), CLAUDE.md (no built-in signal types; SignalAdapter `type` field rationale in terminology), samples drop the built-in source

## 3. Verify, ship, cut over

- [x] 3.1 Full verification: all modules build/vet/test, envtest green, helm lint + template matrix; manager 0.8.0 + chart 1.6.0 built and pushed; commit
- [x] 3.2 LIVE (gated on operator repointing VMAlertmanager to the adapter Service and confirming arrivals on `vm-alerts`): upgrade to 0.8.0, retire the old `alertmanager` source + its `home-ops-pipeline` claim + `am-stub`, verify Served/Wired states and that `/ingest/alertmanager` is gone
