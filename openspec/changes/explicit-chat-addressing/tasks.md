# Tasks: explicit-chat-addressing

## 1. Settle what a chat source is

- [ ] 1.1 Decide design D3: infer the chat lane from the serving `SignalAdapter` (leading), declare it on `SignalSource.spec`, or another handle — nothing below starts against a guess
- [ ] 1.2 If inferred from the adapter, add the declaration to `SignalAdapter` beside `configSchema` as interface metadata, regenerate deepcopy and CRDs, and update `docs/contracts.md`
- [ ] 1.3 Add a single helper that answers "is this source a chat lane?" for both the reconciler and the ingest path, so the two can never disagree

## 2. Narrow source exclusivity

- [ ] 2.1 Make `sourceConflicts` in `internal/controller/pipeline_controller.go` skip chat sources, so several Pipelines listing one stay `Ready=True` with no `SourceConflict`
- [ ] 2.2 Keep exclusivity intact for every non-chat source — an alert has no prefix to disambiguate
- [ ] 2.3 Report every serving Pipeline in the source's `Wired` condition, not just the first
- [ ] 2.4 Tests: two Pipelines on one chat source both Ready; two on one alert source still conflict; the condition names all servers

## 3. Route bare messages only when unambiguous

- [ ] 3.1 Add a "Ready Pipelines serving this source" lookup in `internal/chat/pipelines.go` beside `PipelineForSource`
- [ ] 3.2 In `routeChatSignals` (`internal/httpapi/signals.go`), branch the bare-message path on that count: one routes as today, several refuse, none keeps the existing unwired behaviour
- [ ] 3.3 Write the refusal so it names the serving Pipelines and shows the `/<pipeline> <task>` form — the message is the teaching moment
- [ ] 3.4 Leave the addressed path completely untouched: it resolves by name and consults no source refs
- [ ] 3.5 Emit the refusal as activity telemetry with its own code, so a surface that stops answering is diagnosable rather than silent
- [ ] 3.6 Tests: one server routes; two refuse and create nothing; addressed messages work with zero, one and several servers; the refusal names every server

## 4. Do not break thread replies

- [ ] 4.1 Add a regression test proving a prefix-free reply with a `threadId` is appended as an input on a surface served by several Pipelines — the obvious way to implement this wrong
- [ ] 4.2 Confirm `/channel/inbound` is untouched by any change in this work

## 5. Console typeahead

- [ ] 5.1 Add a BFF listing of Ready Pipelines (name + answering profile) over the console's existing `pipelines` cache — no new RBAC, no manager endpoint
- [ ] 5.2 Implement the composer typeahead in `console/ui/src/pages/Conversation.tsx`: prefix opens the list, typing narrows it, selection inserts `/<name> ` with the cursor after it
- [ ] 5.3 Offer Ready Pipelines only, so the typeahead and `/agents` never give different answers
- [ ] 5.4 Handle keyboard interaction the way the rest of the UI does (arrows, enter, escape) and make it dismissible without sending
- [ ] 5.5 Tests: listing filters to Ready, typing narrows, selection inserts the addressed form, and an empty list degrades to no popup rather than an empty box

## 6. Keep the universal path

- [ ] 6.1 Confirm `/agents` still lists exactly the Pipelines the typeahead offers, and add a test asserting the two agree
- [ ] 6.2 Consider showing the answering profile in `/agents` for parity with the typeahead (design's third open question)

## 7. Documentation

- [ ] 7.1 `docs/concepts.md`: what listing a chat source means now — "I serve this surface", not "I own this inbox" — and that the count of servers decides bare-message behaviour
- [ ] 7.2 `docs/console.md`: the composer typeahead
- [ ] 7.3 `CLAUDE.md`: claiming versus addressing as independent mechanisms, and exclusivity narrowed to non-chat sources
- [ ] 7.4 `CHANGELOG.md` (newest first): lead with the behavioural break — bare messages on a surface served by several Pipelines are now refused — and the one-line fix

## 8. Verification

- [ ] 8.1 `go build ./... && go vet ./...` at the root and in every satellite module; `npm test` in `console/ui`
- [ ] 8.2 Full test run with `KUBEBUILDER_ASSETS` (unit + envtest)
- [ ] 8.3 Integration test in `internal/integration/`: two Pipelines on one chat source, both Ready, bare refused, each addressable, replies landing in the right thread
- [ ] 8.4 Live check on the console: two pipelines serving it, typeahead lists both, bare message refused with the choices, `/<pipeline>` reaches each, thread replies still work with no prefix
