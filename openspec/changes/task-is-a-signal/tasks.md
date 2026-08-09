## 1. Additive: the `task` kind

Nothing is removed in this group — both origination paths work throughout, so it
can land and be verified on its own.

- [ ] 1.1 Add `KindTask = "task"` beside `KindChat` in `internal/httpapi/signals.go`, with a comment stating what separates it from `job` (no `jobName`, no recurrence-on-session) and from `chat` (no channel label required)
- [ ] 1.2 Extend the input-lane switch in `routeSignalGroup` (`signals.go:283-292`) so `KindTask` selects `InputTask` and leaves `jobName` empty
- [ ] 1.3 Set `conv.GenerateName = "task-"` for the `KindTask` branch of the conversation-naming switch (`signals.go:307-312`), matching what `handleTask` produced
- [ ] 1.4 Confirm `isChat` still matches only `KindChat`, so a `task` signal routes through `routeSignals` and never through `routeChatSignals` — a task payload beginning with `/` must not be parsed as a command
- [ ] 1.5 Confirm the chat-channel validation at `signals.go:418` stays scoped to `KindChat`, so a `task` signal is accepted without `agentops.dev/channel`

## 2. Additive: lane-aware signature keying

- [ ] 2.1 Replace the chat-only branch at `signals.go:136` with a lane predicate: when the source declares no `signatureLabels`, key on the fingerprint for `task` and `chat`; leave `alert` and `job` on the `ingest.DefaultSignatureLabels` fallback
- [ ] 2.2 Write the rule's rationale into the comment — `alert`/`job` are recurring-subject lanes that group and resume, `task`/`chat` are one-shot lanes — so the next reader does not "simplify" it back to a blanket rule
- [ ] 2.3 Leave `internal/ingest/grouping.go` untouched; `DefaultSignatureLabels` remains the alert-lane fallback and is simply reached by fewer kinds

## 3. Regression fixtures for the carve-outs

These pin the two behaviors that a blanket keying rule would have broken. Write
them before touching the endpoint, so a later revert cannot pass silently.

- [ ] 3.1 Fixture: two `kind: alert` signals, distinct fingerprints, same `alertname`/`namespace`, source with `grouping: {}` → one conversation (the `vm-bundle` shape)
- [ ] 3.2 Fixture: two `kind: job` signals with distinct fingerprints (`src@tick1`, `src@tick2`), source with no `signatureLabels` → one conversation, second input `InputRecurrence` once a session exists (the `signal-cron` shape pinned by `cron-signal-adapter/spec.md:29`)
- [ ] 3.3 Fixture: two `kind: task` signals with distinct fingerprints, source with no `signatureLabels` → two conversations, each with an `InputTask` and no `jobName`
- [ ] 3.4 Fixture: `kind: task` accepted with no `agentops.dev/channel` label, conversation bound to the claiming Pipeline's `channelRefs`
- [ ] 3.5 Fixture: source WITH `signatureLabels` groups `task` signals sharing those values, proving explicit labels still win in every lane

## 4. Move callers off `/task`

- [ ] 4.1 Rewrite `internal/integration/tooling_test.go:270-281` (the "addresses the SAME Pipeline and gets its whole wiring" case) to post a `kind: task` signal to a source the same Pipeline claims, asserting the identical toolsets/mcpConfigs materialization
- [ ] 4.2 Replace the two negative cases at `tooling_test.go:294-298` (missing pipeline → 400, unknown pipeline → 404) with their signal equivalents: missing `fingerprint` → 400, unknown `source` → 404
- [ ] 4.3 Grep the repo for any remaining test or fixture posting to `/task` and move or delete it

## 5. Remove the endpoint

- [ ] 5.1 Delete the `mux.HandleFunc("POST /task", ...)` registration (`internal/httpapi/server.go:78`)
- [ ] 5.2 Delete `handleTask` and `taskReq` (`server.go:344-411`), including the `---- task lane ----` section marker
- [ ] 5.3 Remove the `POST /task` line from the `httpapi` package doc comment (`server.go:6`)
- [ ] 5.4 Check whether deleting the handler orphans any import in `server.go` (`strconv`, `time`, `strings` are used elsewhere — verify rather than assume) and drop what is now unused
- [ ] 5.5 Confirm `chat.Router.CreateTaskConversation` still has its caller in `HandleCommand` and is NOT deleted — it is the surviving builder for the chat-command path

## 6. Documentation, per the ownership table

- [ ] 6.1 `docs/contracts.md`: delete the `POST /task` row from the HTTP API table (it documents a stale `{"profile",...}` body the handler already rejected), and extend the signal-contract section with `kind: task` and the lane-keying rule
- [ ] 6.2 `docs/contracts.md`: state Decision 4's consequence explicitly — a posted task inherits the target source's `grouping`, so a source with `signatureLabels` groups tasks that share those label values
- [ ] 6.3 `docs/concepts.md:112`: rewrite the "A task addresses a Pipeline" paragraph — a task addresses a SOURCE, and the Pipeline claiming it answers
- [ ] 6.4 `docs/concepts.md:43`: drop "It is ADDRESSABLE by name — `POST /task` names a Pipeline, not a profile" from the Pipeline description
- [ ] 6.5 `README.md`: replace the curl at line 86 with the `/signal/inbound` equivalent and update the `/task API` node in the diagram at line 15; re-check `wc -l README.md` against the 150-line budget
- [ ] 6.6 `chart/templates/NOTES.txt:4`: rewrite the "Ask an agent" line as a `/signal/inbound` post naming a rendered source, with the bearer token
- [ ] 6.7 `chart/values.yaml`: fix the curl comment at line 31 and the `POST /task` reference at line 61
- [ ] 6.8 `chart/charts/k8s-bundle/templates/profile.yaml:82` and `config/samples/samples.yaml:131`: update the comments naming `POST /task`
- [ ] 6.9 `CLAUDE.md`: update the three `/task` references (lines 27, 50, 75), the `httpapi` line in the Map (line 178), and the smoke-test command (line 159) — whose `{"profile":"stub"}` body is stale regardless
- [ ] 6.10 `CHANGELOG.md`: new entry at the top with the before/after curl and the note that no deprecation shim is offered, because a 410 would preserve the doorway this change closes

## 7. Generated artifacts

- [ ] 7.1 Update the comment at `api/v1alpha1/conversation_types.go:61` that names `POST /task` as an origination path
- [ ] 7.2 Regenerate CRDs so the change reaches `chart/files/crds/agentops.dev_conversations.yaml:185`, and confirm that file is the ONLY generated diff (no schema field is added or removed by this change)

## 8. Verification

The toolchain runs in a container — there is no local Go on this machine.

- [ ] 8.1 `go build ./... && go vet ./...` at the root
- [ ] 8.2 `go test ./...` with `KUBEBUILDER_ASSETS` set, so the envtest integration suite runs
- [ ] 8.3 Confirm the three new lane fixtures from group 3 fail if the keying predicate is reverted to a blanket rule — the point of writing them was that a regression here is invisible
- [ ] 8.4 `helm template` the chart with `global.demo.enabled=true` and diff rendered objects against the pre-change render: only NOTES text and comments may differ, no object may change
- [ ] 8.5 `grep -rn "POST /task\|/task" --include=*.go --include=*.md --include=*.yaml --include=*.txt .` outside `openspec/changes/archive/` returns nothing live
- [ ] 8.6 Smoke against the cluster: server-side dry-run the chart, then post a real `kind: task` signal and confirm one conversation per post, with the claiming Pipeline's toolsets materialized on its spec
