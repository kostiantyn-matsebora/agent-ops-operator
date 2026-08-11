# Tasks: explicit-chat-addressing

## 1. Delete source exclusivity

- [x] 1.1 Remove `sourceConflicts` and `ConditionSourceConflict` from `internal/controller/pipeline_controller.go`, including the `SourceConflict` branch that forces `Ready=False`
- [x] 1.2 Drop the `SourceConflict` mentions from `api/v1alpha1/pipeline_types.go` doc comments, regenerate deepcopy and CRDs
- [x] 1.3 Replace `PipelineForSource` in `internal/chat/pipelines.go` with `PipelinesForSource` returning EVERY Ready Pipeline listing the source; delete the oldest-claimant comment along with the behaviour
- [x] 1.4 `internal/controller/signalsource_controller.go`: `Wired` names all serving Pipelines, not the first
- [x] 1.5 Tests: two Pipelines on one source both `Ready=True` with no conflict condition; the `Wired` message names both; replace the `SourceConflict` assertion in `internal/integration/pipeline_test.go`

## 2. Fan signals out

- [x] 2.1 `routeSignals` (`internal/httpapi/signals.go`) routes each signature group to every serving Pipeline — one conversation per Pipeline
- [x] 2.2 Evaluate cooldown and signature grouping ONCE, before the fan-out, so a fingerprint is admitted once and delivered to each server rather than suppressed for all but the first
- [x] 2.3 Emit one `signal.claimed` activity event per serving Pipeline; keep `receivedTotal` counting signals received, not conversations opened
- [x] 2.4 Check the backlog bound per conversation created, so a fan-out cannot exceed `MAX_QUEUED_CONVERSATIONS`
- [x] 2.5 Tests: one alert on a doubly-served source opens two conversations with the right profile and capabilities each; one fingerprint, one cooldown entry; a suppressed re-delivery reaches neither

## 3. Record the originating pipeline

- [x] 3.1 Add `pipelineRef` to `ConversationSpec` — provenance, documented as written once at creation and never read to resolve wiring; regenerate deepcopy and CRDs
- [x] 3.2 Set it in `routeSignalGroup` and on every other conversation-creating path (the addressed-command path in `internal/chat/router.go`)
- [x] 3.3 Scope conversation reuse by it: a group may only append to a conversation its own Pipeline originated
- [x] 3.4 Legacy rule: a conversation with no `pipelineRef` is reusable only while exactly one Ready Pipeline serves the source; nothing backfills the field
- [x] 3.5 Point the console's attribution at the recorded origin instead of `chat.PipelineForConversation` inference; keep the inference helper only where no ref exists
- [x] 3.6 Tests: two Pipelines fanning out never share a conversation; a legacy conversation still groups on a singly-served source and is bypassed on a shared one; attribution is exact for two identically-wired Pipelines

## 4. Route bare chat messages only when unambiguous

- [x] 4.1 In `routeChatSignals`, branch the bare-message path on the number of serving Ready Pipelines: one routes as today, several refuse, none keeps the existing unwired behaviour
- [x] 4.2 Draw the chat/non-chat distinction from the arriving signal kind only — no reconciler and no CRD field decides it
- [x] 4.3 Write the refusal so it names the serving Pipelines and shows the `/<pipeline> <task>` form — the message is the teaching moment
- [x] 4.4 Leave the addressed path completely untouched: it resolves by name and consults no source refs
- [x] 4.5 Emit the refusal as activity telemetry with its own code, so a surface that stops answering is diagnosable rather than silent
- [x] 4.6 Tests: one server routes; two refuse and create nothing; addressed messages work with zero, one and several servers; the refusal names every server

## 5. Do not break thread replies

- [x] 5.1 Add a regression test proving a prefix-free reply with a `threadId` is appended as an input on a surface served by several Pipelines — the obvious way to implement this wrong
- [x] 5.2 Confirm `/channel/inbound` is untouched by any change in this work

## 6. Console typeahead

- [x] 6.1 Add a BFF listing of Ready Pipelines (name + answering profile) over the console's existing `pipelines` cache — no new RBAC, no manager endpoint
- [x] 6.2 Implement the composer typeahead in `console/ui/src/pages/NewConversation.tsx` — the ORIGINATION composer, not the thread-reply one in `Conversation.tsx`, where a prefix addresses nobody: prefix opens the list, typing narrows it, selection inserts `/<name> ` with the cursor after it
- [x] 6.3 Offer Ready Pipelines only, so the typeahead and `/agents` never give different answers
- [x] 6.4 Handle keyboard interaction the way the rest of the UI does (arrows, enter, escape) and make it dismissible without sending
- [x] 6.5 Tests: listing filters to Ready, typing narrows, selection inserts the addressed form, and an empty list degrades to no popup rather than an empty box

## 7. Keep the universal path

- [x] 7.1 Confirm `/agents` still lists exactly the Pipelines the typeahead offers, and add a test asserting the two agree
- [x] 7.2 Consider showing the answering profile in `/agents` for parity with the typeahead (design's open question)

## 8. Documentation

- [x] 8.1 `docs/concepts.md`: sources are shareable, a signal fans out one conversation per serving Pipeline, `pipelineRef` is provenance, and a chat source's server count decides bare-message behaviour
- [x] 8.2 `docs/console.md`: the composer typeahead
- [x] 8.3 `CLAUDE.md`: two invariants change — "one pipeline per source" is gone, and "conversations carry no pipelineRef" becomes "carry it as provenance, never read for wiring". Add fan-out and the chat exception
- [x] 8.4 `CHANGELOG.md` (newest first): lead with both behavioural breaks — a shared source now routes to every Pipeline listing it, and bare messages on a shared chat surface are refused — and the one-line fix for each

## 9. Verification

- [x] 9.1 `go build ./... && go vet ./...` at the root and in every satellite module; `npm test` in `console/ui`
- [x] 9.2 Full test run with `KUBEBUILDER_ASSETS` (unit + envtest)
- [x] 9.3 Integration test in `internal/integration/`: two Pipelines on one source, both Ready, an alert opening two conversations, a bare chat message refused, each addressable, replies landing in the right thread
- [x] 9.4 Live check: two pipelines serving one console surface, typeahead lists both, bare message refused with the choices, `/<pipeline>` reaches each, thread replies still work with no prefix, and a shared alert source opens two investigations
