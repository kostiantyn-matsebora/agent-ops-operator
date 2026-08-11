## 1. Surface the closing state

- [ ] 1.1 Add `DeletionTimestamp string \`json:"deletionTimestamp,omitempty"\`` to `Metadata` in `console/kube.go`
- [ ] 1.2 Add `Closing bool \`json:"closing"\`` to `ConversationSummary` in `console/conversations.go` and set it in `summarize` from the deletion timestamp
- [ ] 1.3 Unit-test in `console/conversations_test.go` (or the nearest existing summary test) that an object with a deletion timestamp summarizes as `closing`

## 2. The bulk-close handler

- [ ] 2.1 In `console/convapi.go`, define the request `{names []string, includeWorking bool}` and the result types: `CloseResult{Name, Outcome, Reason}` with outcomes `closed | skipped | failed`, plus `closed/skipped/failed` totals
- [ ] 2.2 Implement `handleBulkClose`: reject empty `names` and more than 50 names with 400; walk names IN ORDER, never aborting the walk on one failure
- [ ] 2.3 Per name, decide server-side from cached CR state: unknown name → `failed`; `closing` → `skipped`; phase `Working` without `includeWorking` → `skipped: working`; no console thread → `skipped: observed`, with the reason naming the fix (add the console channel to the pipeline's `channels[]`)
- [ ] 2.4 For each remaining conversation, post the literal text `/close` via `a.adapter.Send(ctx, name, Identity(r), "/close")`; map `errNotJoined` to `skipped: observed` and any other error to `failed` with a safe reason
- [ ] 2.5 Log each close as `console write: action=bulk-close identity=… conversation=…`
- [ ] 2.6 Return 200 with the per-item results and totals — a mixed batch is a successful request
- [ ] 2.7 Register `POST /api/conversations/close` in `console/api.go` behind `a.write("bulk-close", a.handleBulkClose)`

## 3. Handler tests

- [ ] 3.1 Test: a batch of joined, idle conversations closes them all and reports the totals
- [ ] 3.2 Test: an observed conversation is `skipped` with the binding reason, and the joined siblings in the same batch still close
- [ ] 3.3 Test: a `Working` conversation is skipped by default and closed when `includeWorking` is true
- [ ] 3.4 Test: the phase decision is taken from CR state, not from anything the client sends
- [ ] 3.5 Test: empty `names` and 51 names are both 400 and close nothing
- [ ] 3.6 Test: a failure on one name does not stop the remaining names
- [ ] 3.7 Test: the route is refused with 403 when writes are disabled, 403 with no identity, 401 unauthenticated
- [ ] 3.8 Test: a conversation already closing is `skipped` and no second `/close` is posted

## 4. Console UI

- [ ] 4.1 Add the request/result types to `console/ui/src/api/types.ts` and the `closeConversations` call to `client.ts`
- [ ] 4.2 Add the mutation hook to `console/ui/src/api/hooks.ts`, invalidating the conversations query on settle
- [ ] 4.3 In `Conversations.tsx`, add a selection column plus a select-all checkbox scoped to the current page; clear the selection whenever filters, search, page or per-page change
- [ ] 4.4 Make `closing` rows visibly closing (a label) and not selectable
- [ ] 4.5 Add a `Close selected` toolbar action, disabled with nothing selected
- [ ] 4.6 Add the confirmation modal: the count, how many of the selection are working, the `include working — abandons in-progress runs` toggle (default off), and that closing cannot be undone
- [ ] 4.7 Render the outcome after the batch — closed/skipped/failed totals with the per-conversation reasons for anything not closed
- [ ] 4.8 Hide the action entirely when the console is read-only, consistent with the other write surfaces

## 5. UI tests

- [ ] 5.1 Test: select-all selects only the rows on the current page
- [ ] 5.2 Test: the confirmation states the count and the working count, and the working toggle defaults to off
- [ ] 5.3 Test: a mixed result renders per-conversation reasons rather than a single verdict
- [ ] 5.4 Test: closing rows cannot be selected

## 6. Documentation

- [ ] 6.1 Document the action in `docs/console.md`: what it closes, that it is `/close` fanned out, why observed conversations are out of reach, the working opt-in, and the write gate
- [ ] 6.2 Cross-reference `conversation-close` semantics in `docs/concepts.md` only if the close section there implies a single-thread-only gesture

## 7. Verification

- [ ] 7.1 `cd console && go build ./... && go vet ./... && go test ./...`
- [ ] 7.2 Build the SPA and run the UI tests; confirm `console/ui/tsconfig.tsbuildinfo` and the embedded assets are regenerated as the repo expects
- [ ] 7.3 Live check against the cluster: bind the console channel to a pipeline, originate two conversations, bulk close them from the list, and confirm both farewell posts, both topics archived, both `Conversation` objects gone and the freed slots admitted
- [ ] 7.4 Live check the reach boundary: a conversation the console does not thread comes back `skipped` and is still present afterwards
