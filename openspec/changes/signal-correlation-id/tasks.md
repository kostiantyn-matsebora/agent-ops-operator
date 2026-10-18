# Tasks — signal-correlation-id

## 1. Contract pages — api-architect

- [x] 1.1 Rewrite `docs/contracts/signal-adapter.md` to 2.0.0: `initiationId` required with the one propagate-or-create rule, `fingerprint` removed, requirement IDs (`API-n`/`ADP-n`/`MGR-n`) renumbered, COMPAT stating reject-never-synthesize, changelog row under 2.0.0. The user approves the wording before it merges (`signal-rules.md`). Verify: the page's claims match the delta specs and `rules_compliance.py` passes on it.
- [x] 1.2 Bump `docs/contracts/channel-adapter.md` MINOR: ops carry `conversationId` (additive, opaque), a channel adapter never mints an id. Verify: wire `contract=` value unchanged on the page, changelog row added.
- [x] 1.3 Check the open `channel-adapter-specification` change for an overlapping `channel-adapter-contract` delta and rebase whichever lands second. Verify: `openspec validate --strict` passes for both changes after the rebase.

## 2. CRD and API types — backend-developer

- [x] 2.1 Remove `SignalSource.spec.grouping`, add top-level `spec.cooldownHours` (default 6). Regen deepcopy and CRDs per `build-test.md`. Verify: `go build ./...` and the regenerated `chart/crds/` show the removal and the new field.
- [x] 2.2 Replace `Conversation.spec.signature` with `spec.conversationId` (written once, opaque string). Regen. Verify: envtest suite compiles and passes.

## 3. Manager ingest and routing — backend-developer

- [x] 3.1 Require `initiationId` at `POST /signal/inbound`: `4xx` naming the field, no synthesis. Remove the alert-vs-oneshot fallback in `httpapi/signals.go` and `ingest.Signature`/`SignatureHash`/`DefaultSignatureLabels`. Verify: httpapi unit tests cover the rejection and the removed symbols are gone from the module.
- [x] 3.2 Route by equality through an informer field index on `spec.conversationId`, scoped to the claiming pipeline and OPEN conversations only. Remove the `agentops.dev/signature-hash` label and window reuse. Verify: envtest covers open-match recurrence, fresh-id creation, distinct-ids-never-merge and closed-conversation exclusion.
- [x] 3.3 Re-key `ingest.Cooldown` to `initiationId`, read `spec.cooldownHours`, keep SignalSource durability and chat's 0. Verify: ingest unit tests pass with the new key.
- [x] 3.4 Refuse reopening a closed conversation while another open conversation carries the same `conversationId` for that pipeline, reason naming the successor. Verify: envtest case for the refusal and for a permitted reopen with no successor.
- [x] 3.5 Stamp `conversationId` on everything leaving the conversation: every `/channel/ops` operation, member spawns (`invoke`), the runtime context. Verify: chat and dispatch unit tests assert the field on each surface.
- [x] 3.6 Update the dispatch/ingest test fixtures to the new semantics — a deliberate change, per `invariants.md`. Verify: the fixture diff only reflects decisions recorded in `design.md`.

## 4. Shipped signal adapters — backend-developer

- [ ] 4.1 `signals/alertmanager`: `initiationId` = webhook `groupKey` verbatim, hash of sorted `groupLabels` when absent. Verify: adapter unit tests cover both paths.
- [ ] 4.2 pulse adapter: `initiationId` = the Pulse alert id, stable across retries and firing/resolved. Verify: a unit test pins the motivating case — two unrelated alerts yield two ids and never merge.
- [ ] 4.3 `signals/ha`: mint in the existing pending/dwell logic, keyed on condition identity. Verify: adapter unit tests pass with the new field.
- [ ] 4.4 `signals/k8s-events`: `initiationId` = `<source>@<namespace>/<workload>` via the existing owner-reference resolution. Verify: unit tests pin one-conversation-per-workload across pod churn.
- [ ] 4.5 `signals/cron`: `initiationId` = the source's job identity, keeping the last-fired cursor. Verify: unit tests pin all-ticks-one-conversation without the empty-signature collapse.
- [ ] 4.6 `signals/telegram` and console origination: fresh mint per new request, verbatim propagation of the stamped binding id on continuation. Verify: unit tests cover both cases on each surface.

## 5. Channel surfaces — backend-developer

- [ ] 5.1 `channels/telegram`: thread mapping from the op's `conversationId` only — drop any signature-hash or backref mapping. Verify: the component's test suite passes with the mapping asserted both directions.
- [ ] 5.2 `platform/console`: read `spec.conversationId` directly, remove signature-hash usages in Go and UI. Verify: console Go tests and `npm run test:coverage` pass.

## 6. Bundles and chart values — deployment-engineer

- [x] 6.1 Drop `grouping:` blocks from the prometheus, pulse, ha and k8s bundle values. Set an explicit `cooldownHours` where a source fires more often than the 6h default (cron-shaped sources). Verify: chart render tests pass and no rendered SignalSource carries `grouping`.
- [x] 6.2 Ship the regenerated `chart/crds/` and the upgrade note (`kubectl apply -f chart/crds/`, or removed fields silently prune). Verify: render tests pass against the updated CRDs from this working copy's tree, not the released chart.

## 7. Retired vocabulary

- [ ] 7.1 Add `signature`, `signatureLabels`, `signature-hash` and `fingerprint` (signal sense) to `.github/retired-vocabulary.json` with replacements, and record them in `.claude/rules/retired-vocabulary.md`. Verify: `python3 .github/scripts/retired-vocabulary-guard.py` passes over the tree.

## 8. Unit tests

- [ ] 8.1 Run `go build ./... && go vet ./... && go test ./...` in every touched module via the `components.sh modules` loop, with `-coverpkg=./...` for coverage. Verify: all green.
- [ ] 8.2 Run the manager suite with envtest (`KUBEBUILDER_ASSETS` per `build-test.md`). Verify: all green including the new routing, reopen and stamping cases.
- [ ] 8.3 Run the console UI suite (`npm run test:coverage`) and the chart render tests. Verify: all green.

## 9. System tests

- [ ] 9.1 Add a correlation lane to `platform/manager/test/system/`: post two signals with distinct `initiationId`s on one source (two conversations), a repeat id outside cooldown (recurrence on the open one), an id-less signal (`4xx`), and a thread binding carrying the id end to end. Verify: the lane passes in the pack run (`-tags system`, k3d — dispatched via `system-smoke.yml` from a remote session).
- [ ] 9.2 Run the full system pack against this working copy's tree. Verify: pack green, run recorded on the PR.

## 10. Documentation

Reference docs:

- [ ] 10.1 `docs/contracts.md`: `/signal/inbound` request shape and the `4xx` rejection. Verify: the page matches the 2.0.0 contract.
- [ ] 10.2 `docs/concepts.md`: grouping/signature vocabulary replaced by the correlation id and equality routing. Verify: retired-vocabulary guard passes on the page.
- [ ] 10.3 Re-run `python3 .github/scripts/docs-generate.py` after the CRD change and confirm `--check` passes — `docs/cr-reference.md` and every generated block.
- [ ] 10.4 `docs/guides/signal-adapter.md`: authoring guidance mints an id, `signatureLabels` gone. Verify: guide's yaml blocks regenerate clean.
- [ ] 10.5 `docs/integrations/prometheus.md`, `pulse.md`, `home-assistant.md`, `kubernetes.md`: state where each integration's id comes from. Verify: pages read as dense adopter prose, no renders table.
- [ ] 10.6 `docs/CHANGELOG.md`: breaking change, upgrade steps (CRD apply, 1.x adapter rejection), newest first. Verify: entry present with the chart/image versions it ships in.

Adopter site:

- [ ] 10.7 `docs/index.md` (landing): grouping vocabulary replaced. Verify: retired-vocabulary guard passes.
- [ ] 10.8 `docs/configuration.md`: bundle grouping values removed, `cooldownHours` documented as the one knob. Verify: matches the shipped values.
- [ ] 10.9 `docs/installation.md`: the CRD upgrade note. Verify: upgrade section names `kubectl apply -f chart/crds/`.
- [ ] 10.10 `docs/getting-started.md` and `docs/introduction.md`: check for grouping mentions, fix or record "none found". Verify: the verdict is recorded in this task on tick.
