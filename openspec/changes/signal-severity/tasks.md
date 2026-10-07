Every build and test below runs against THIS working copy's tree — the worktree at `../agent-ops-worktrees/signal-severity/` on a workstation (`docker exec -i -w "$PWD" agentops-go …` from inside it), or the session's clone in a remote session — never the default branch's checkout.

## 1. Trim `signal-attributes-and-icons` (#253)

- [ ] 1.1 In `openspec/changes/signal-attributes-and-icons/proposal.md`, drop the severity bullets from "What Changes", narrow the `signal-severity-and-type` capability line to `signal-type` (type only), and drop the severity rows from "Impact". Verify the file names no `severity` field, vocabulary or per-adapter mapping of its own.
- [ ] 1.2 In `design.md` there, drop the "Severity is a field, type is a label" and "Severity rides on provenance and on each input" decisions, and drop severity's row from the migration plan. Verify no remaining decision references a severity field.
- [ ] 1.3 In `tasks.md` there, drop task 1.3 (the `Severity` enum and its two fields), narrow 4.1–4.3 to their `type`-only and matcher-only work, drop any severity-only assertion from the conformance task, and renumber. Verify every remaining task names only `type`, matchers or icons.
- [ ] 1.4 Rename `specs/signal-severity-and-type/` to `specs/signal-type/` and delete the "Severity is a closed vocabulary", "Severity is kept with the conversation and with each input" and "Each shipped adapter maps its own scale once" requirements from it, keeping "Type is a reserved label key" and "The agent is told what it is handling" as written. Verify `openspec validate signal-attributes-and-icons --strict` passes with the renamed capability.

## 2. API types and CRDs — api-architect

- [ ] 2.1 Add the `Severity` enum type (`critical`, `error`, `warning`, `info`) to `api/v1alpha1/common_types.go`, and a `Severity Severity` field to `SignalProvenance` and to `ConversationInputSpec`, both `+optional`. Verify `go build ./...` in `platform/manager`.
- [ ] 2.2 Regenerate deepcopy and CRDs with controller-gen into `chart/crds/`. Verify `git diff --stat chart/crds` touches the Conversation and ConversationInput CRDs and `go vet ./...` is clean.

## 3. Ingest: vocabulary and storage — backend-developer

- [ ] 3.1 Add `Severity` to `NormalizedSignal` in `internal/httpapi/signals.go` and validate every signal's value against the vocabulary before any routing, answering 400 naming the four values and creating nothing from the batch on a violation. Verify a handler test posts `severity: Warning` and gets 400 with the vocabulary in the body, and one posting `severity: critical` is admitted.
- [ ] 3.2 Write `Severity` on the conversation's `SignalProvenance` at creation and on every `ConversationInput`, leaving the provenance unchanged on a later recurrence of a different severity. Verify an envtest where a `warning`-opened conversation receives an `error` recurrence shows provenance `warning` and the new input `error`.

## 4. Adapters map their scale — backend-developer

- [ ] 4.1 `signals/k8s-events`: send `severity` mapped from the Event type (`Warning` → `warning`, `Normal` → `info`) and drop the `severity` key from the payload. Verify a normalize test for both mappings and that the payload carries no `severity` key.
- [ ] 4.2 `signals/ha`: send `severity` mapped by the level table for log records and shaped surface records (`CRITICAL`/`ERROR`/`WARNING`/`INFO`,`DEBUG`). Verify a test per row of the table and one for a config-entry failure shaped as `ERROR`.
- [ ] 4.3 `signals/alertmanager`: parse `config.severityMap`, map the alert's `severity` label with the identity default for `critical`/`warning`/`info`, send the field, and report an unmapped value on the source's Ready message once per value while keeping Ready true. Verify tests for the default, a house map, and the unmapped report.
- [ ] 4.4 Extend the conformance suite's signal-adapter lane so every shipped adapter's emitted signal carries a severity inside the vocabulary or none. Verify `go test -tags conformance -count=1 ./test/conformance/` in `platform/manager` passes for k8s-events, ha, alertmanager and cron.

## 5. The manager publishes severity on the signal message — backend-developer

- [ ] 5.1 Add `Severity` to the `signal` message in `internal/chat/message.go`, filled from the input's recorded severity through the same path that already fills `Labels`. Verify a unit test on a rated signal and one on an unrated task signal, whose message carries no `severity` field.

## 6. Console — frontend-developer

- [ ] 6.1 Add `Severity string` to the `Message` struct in `platform/console/transcript.go`, filled from the inbound op the same way `Payload` already is. Verify a transcript test for a rated op and an unrated one.
- [ ] 6.2 Add `severity?: string` to the frontend `Message` interface in `platform/console/ui/src/api/types.ts`, and draw it leading the signal card in `Conversation.tsx`, tinted per value from the theme's existing severity-family tokens, unchanged for a message carrying none. Verify a component test for a `critical` card and an unrated one, and a screenshot of a rated conversation against the dev server per `visual-check.md`.

## 7. Unit tests

- [ ] 7.1 `platform/manager`: `go test ./...` with `KUBEBUILDER_ASSETS` set, covering 2.x, 3.x and 5.x. Verify green from this working copy.
- [ ] 7.2 `signals/k8s-events`, `signals/ha`, `signals/alertmanager`: `go build ./... && go vet ./... && go test ./...` in each. Verify green.
- [ ] 7.3 `platform/manager`: `go test -tags conformance -count=1 ./test/conformance/`. Verify green.
- [ ] 7.4 `platform/console`: `go test ./...`, and `platform/console/ui`: `npm test`. Verify green.

## 8. E2E tests

- [ ] 8.1 Not applicable: nothing here is decided by a cluster. Vocabulary validation and provenance storage are ingest logic under envtest, adapter mapping is `go test` and the conformance tier, and the console card is UI rendering. No kubelet, RBAC, informer or pod lifecycle changed. Tick once the above is confirmed true at implementation time.

## 9. Documentation

### 9.1 Reference docs

- [ ] 9.1.1 `docs/concepts.md`: severity as a validated field beside the signal's labels, with its vocabulary and where it is stored. Verify it is a section a reader lands on from the nav.
- [ ] 9.1.2 `docs/contracts.md`: `severity` in the inbound signal shape with the refusal, and in the `signal` message shape. Verify the shapes match the Go types.
- [ ] 9.1.3 `docs/integrations/kubernetes.md`, `home-assistant.md`, `prometheus.md`: each adapter's mapping table, and `config.severityMap` with the unmapped report on prometheus. Verify each page carries its table.
- [ ] 9.1.4 `docs/console.md`: what the signal card now draws. Verify it names severity once.
- [ ] 9.1.5 `docs/CHANGELOG.md`: the new field on both CRD-backed types, the CRD apply step, no breaking change. Verify it is the newest entry.
- [ ] 9.1.6 `docs/guides/signal-adapter.md`: severity in the worked emission, through its generated marker. Verify the prose explains the field.
- [ ] 9.1.7 Run `python3 .github/scripts/docs-generate.py` and then `--check`. Verify every generated block and `docs/cr-reference.md` are regenerated and the check passes.

### 9.2 Adopter site

- [ ] 9.2.1 `docs/console-guide.md`: severity marks in the transcript view. Verify the screenshot it embeds shows one.
- [ ] 9.2.2 `docs/introduction.md`: one sentence that a signal can be rated. Verify it links to concepts.
- [ ] 9.2.3 Re-run `npm run screenshots` and `npm run demo` in `platform/console/ui`. Verify the site's screenshots and the landing recording show the rated card.
