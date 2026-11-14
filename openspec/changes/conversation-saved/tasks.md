## 1. The field and the state — backend-developer

- [ ] 1.1 Add `status.savedAt` to `ConversationStatus`, regenerate deepcopy and `chart/crds/`. Verify: `go build ./...` in `platform/manager` and the CRD diff shows only the new field.
- [ ] 1.2 Carry `savedAt` on the conversation record in every `internal/state` backend, and on the `/state/conversations` read. Verify: the state conformance suite passes under `kube` and `nats`, with a saved case.

## 2. The verbs and commands — api-architect, backend-developer

- [ ] 2.1 Serve `save` and `unsave` under `/channel/conversations/{name}/` as accepted tasks, idempotent, `404` unknown, `409` on a member. Verify: handler tests per scenario in `conversation-saved`.
- [ ] 2.2 Refuse a manual `delete` of a Saved conversation with `409`. Verify: a test deleting a Closed, Saved conversation, then after unsave.
- [ ] 2.3 Add the `/save` and `/unsave` thread commands over the same implementation. Verify: a router test comparing command and verb outcomes.
- [ ] 2.4 Skip Saved conversations, and members of a Saved root, in autoclose and autodelete, rechecked when the task runs. Verify: controller tests for each housekeeping scenario, including save-after-schedule and unsave-resumes-clock.
- [ ] 2.5 Add both endpoints to the conformance fake manager. Verify: `go test -tags conformance ./test/conformance/`.

## 3. The console — frontend-developer

- [ ] 3.1 Save/Unsave in the row menu, absent for members and read-only. Verify: vitest cases for each menu scenario, including delete absent on a Saved closed row.
- [ ] 3.2 The Saved mark on row and thread header, and the Saved filter. Verify: vitest cases, then a Playwright screenshot of the working tree read as a PNG (`visual-check.md`).

## 4. Unit tests — testing-specialist

- [ ] 4.1 Run `go build ./... && go vet ./... && go test ./...` in every module touched, with `KUBEBUILDER_ASSETS` set so the envtest suite runs. Verify: green, no skips.
- [ ] 4.2 Run `npm run test:coverage` in `platform/console/ui` and the Python/script suites only if a script changed. Verify: green.

## 5. System tests — testing-specialist

- [ ] 5.1 Extend a lane in `platform/manager/test/system/`: autoclose and autodelete enabled with short windows, one Saved and one unsaved conversation, assert only the unsaved is reclaimed, then unsave and assert reclaim. Verify: the lane passes against this working copy's tree; the cluster tier is dispatched in a remote session.

## 6. Documentation — last

### Reference docs
- [ ] 6.1 `docs/concepts.md` (`savedAt`), `docs/crds/conversation.md`, `docs/contracts/channel-adapter.md`, `docs/contracts.md` (the two endpoints, the accept table), `docs/console.md`, `docs/CHANGELOG.md`. Verify: each names the field and verbs.
- [ ] 6.2 Run `python3 .github/scripts/docs-generate.py`. Verify: `--check` passes.

### Adopter site
- [ ] 6.3 `docs/console-guide.md` (keeping a conversation) and the retention description in `docs/configuration.md` and the housekeeping guide: autoclose and autodelete skip Saved. Verify: a reader can find how to keep a conversation from each page.
- [ ] 6.4 Re-run `npm run screenshots` and `npm run demo` in `platform/console/ui`. Verify: the Saved mark appears in the regenerated assets.
- [ ] 6.5 Run the publication and retired-vocabulary guards. Verify: both pass.
