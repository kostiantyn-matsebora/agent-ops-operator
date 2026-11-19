## 1. Store contract — backend-developer

- [ ] 1.1 Change `Collection[T].Create` to return the stored record with its backend-minted `ID` and `CreatedAt`, update the doc comment in `internal/state/store.go`, and adapt `kube/store.go`, `kube/part.go`, `kube/adapterstate.go` and `nats/collection.go`. Verify `go build ./...` in `platform/manager`.
- [ ] 1.2 Adapt the 34 non-test `Create` callers. Verify `go vet ./...` is clean and no caller discards a record it later needs.
- [ ] 1.3 Extend `internal/state/conformance` to assert, for every kind of `Stores`, that `Create` returns the stored record. Verify both backends pass the suite.
- [ ] 1.4 Add the `ByID` index for conversations in both backends and the `correlationId` index rename. Verify with conformance cases for lookup by `ID` and by correlation id.
- [ ] 1.5 Make `namewords.AllocateName` return the created record and update its three callers (`chat/router.go`, `chat/coordinate.go`, `signalroute/conversation.go`). Verify a test asserts each returned record carries a non-empty `ID`.

## 2. The rename — api-architect

- [ ] 2.1 Rename `ConversationID` to `CorrelationID` in `api/v1alpha1`, the record, the kube mapping and layouts, and the `Op` types, and regenerate deepcopy and CRDs. Verify `chart/crds/` shows `correlationId` and nothing still shows `conversationId` for the correlation key.
- [ ] 2.2 Make the op's `conversationId` carry the conversation `ID`, move the correlation key to `correlationId`, and raise the contract version as the specs state. Verify an adapter declaring the old version gets the documented `400`.

## 3. ID as the system identity — backend-developer

- [ ] 3.1 Name the workspace subdirectory by `ID` in `runtimepod`, and set `CONVO_ID` to the `ID`. Verify pod-spec tests for both.
- [ ] 3.2 Add the `agentops.dev/conversation-id` label to the runtime pod and reconcile from it. Verify a test with a pod whose name matches a different conversation resolves by the label.
- [ ] 3.3 Derive op ids and thread ids from the `ID` and store the `ID` in cross-references such as `causedBy`. Verify the ops and coordination tests, including a recreated name that no longer collides with its predecessor's op.
- [ ] 3.4 Resolve `/work?convo=` strictly by `ID`, refusing an unknown `ID` and a record with no `ID` with the documented reason. Verify with tests for an unknown, a stale and a missing `ID`.
- [ ] 3.5 Resolve every person-typed name through the live-match rule. Verify per verb class: read, reopen, delete, exit, close, escalate and add-input, each with a Closed match.
- [ ] 3.6 Match orphan workspace directories to conversations by `ID` in housekeeping. Verify a recreated name does not protect a deleted conversation's directory.
- [ ] 3.7 Review the remaining `conv.Name` uses (about 130) and move each system use to `ID`, leaving display and store-key uses. Verify with a grep-backed table in the PR description listing the categories, not every line.

## 4. Consumers — backend-developer

- [ ] 4.1 Move `platform/console` to `correlationId` and the new wire field. Verify `go test ./...` in `platform/console`.
- [ ] 4.2 Move `channels/telegram` the same way. Verify `go test ./...` in `channels/telegram`.
- [ ] 4.3 Confirm the runtimes need no change by reading `CONVO_ID` as opaque. Verify `node --test` in each Node runtime and `go test ./...` in `runtimes/ollama`.

## 5. Chart and release wiring — deployment-engineer

- [ ] 5.1 Bump the manager, console and telegram channel image tags and the chart version consistently. Verify the chart render tests and `python3 .github/scripts/docs-generate.py --check`.
- [ ] 5.2 Add the retired-vocabulary entries named in the design, with a pattern that matches `conversationId` only as the signal correlation key, since the same name now carries the identity on the wire. Verify `python3 .github/scripts/retired-vocabulary-guard.py` passes and flags a planted old use.

## 6. Unit tests

- [ ] 6.1 Run `go build ./... && go vet ./... && go test ./...` in `platform/manager` with `KUBEBUILDER_ASSETS` set, and `go test -tags conformance -count=1 ./test/conformance/`.
- [ ] 6.2 Run the same Go gates in `platform/console`, `channels/telegram` and `runtimes/ollama`, and `node --test` in the Node runtimes.
- [ ] 6.3 Run the chart render tests and the script suite touched by the release wiring. The console UI is not changed, so the browser tiers are not owed.

## 7. System tests

- [ ] 7.1 Extend a lane in `platform/manager/test/system/` that deletes a conversation and creates another that reuses its name, and asserts a different workspace directory, a different pod label and no op collision. Verify with `go test -tags system -count=1 -timeout 45m -v ./test/system/` against this working copy's tree, or dispatch `system-smoke.yml` on this branch if k3d is unavailable.
- [ ] 7.2 Assert in the same lane that a signal-originated conversation still routes by `correlationId` and that a typed name reaches the newest live conversation.

## 8. Documentation

### 8.1 Reference docs

- [ ] 8.1.1 Update `docs/contracts/channel-adapter.md` and `docs/contracts/signal-adapter.md` for the rename, the new wire field and the version, and `docs/contracts/runtime.md` and `docs/contracts.md` for `CONVO_ID` and the `ID`.
- [ ] 8.1.2 Update `docs/concepts.md`, `docs/crds/conversation.md` through its markers, `docs/gotchas.md` and `docs/CHANGELOG.md` with the breaking change and the delete-before-upgrade step.
- [ ] 8.1.3 Update `.claude/rules/terminology.md` and `.claude/rules/invariants.md` for the three identifiers.
- [ ] 8.1.4 Re-run `python3 .github/scripts/docs-generate.py` and commit the generated blocks and `docs/cr-reference.md`.

### 8.2 Adopter site

- [ ] 8.2.1 Update `docs/guides/signal-adapter.md` and `docs/guides/channel-adapter.md` for `correlationId` and the wire field.
- [ ] 8.2.2 Update `docs/installation.md` for the delete-before-upgrade step, and any page still showing `conversationId`.
- [ ] 8.2.3 Run `python3 .claude/scripts/rules_compliance.py` over the touched pages and `python3 .github/scripts/publication-guard.py --counts`.
