## 1. The event carries what changed

- [x] 1.1 Send the changed object on the `delta` event in `console/api.go`, in the shape the snapshot serves — and verify a BFF test asserts a created and an updated object arrive complete, and a delete carries its identity and type
- [x] 1.2 Type the stream payloads in `console/ui/src/api/stream.ts` so a handler cannot read a field the event does not carry — and verify `stream.test.ts` covers each event kind

## 2. Applying, in one place

- [x] 2.1 Add a cache-apply module mapping `(kind, type, object)` onto react-query writes — and verify a unit test applies one event and asserts every cached view holding that object changed
- [x] 2.2 Apply the `message` event to the conversation that owns it, from the payload already on the wire — and verify a test asserts a message appears with no request issued
- [x] 2.3 Verify an applied cache entry equals a fetched one for the same state, so an applier cannot render a shape the snapshot would never produce

## 3. Revisions leave the query key

- [x] 3.1 Remove `useRevision` from every query key in `console/ui/src/api/hooks.ts` and make each key stable — and verify no key contains a change counter
- [x] 3.2 Keep `placeholderData: keepPreviousData` only where the key changes on USER input (paging, filters) and remove it elsewhere — and verify the remaining uses are each justified in a comment
- [x] 3.3 Verify the four refetch reasons are the only ones left — first load, resync, explicit action, and a time-decaying value — and that each timed refresh names its decaying value where it is set

## 4. A bounded cache

- [x] 4.1 Set eviction and freshness explicitly in the query client, in one place with the reasoning beside them — and verify a test asserts data for an off-screen view is released after the bound
- [x] 4.2 Verify a view remounted after the bound loads fresh, and that one held on screen is never refetched by the bound alone
- [x] 4.3 Verify nothing is persisted — no localStorage, IndexedDB or service-worker cache — so closing the tab leaves nothing behind

## 5. Every page

- [x] 5.1 Verify Overview updates in place from a delta with no loading state — it is an aggregate over every kind plus the manager, so it re-reads on a stable key rather than applying
- [x] 5.2 Verify the same for Queues
- [x] 5.3 Verify the same for Topology — likewise derived, likewise re-read — and that its timer remains only for rates
- [x] 5.4 Verify the same for Configuration, including a kind detail view
- [x] 5.5 Verify the same for the Conversations list, including a conversation appearing, changing phase and being deleted — applied with no request, except a filtered or later page, where membership is the server's decision
- [x] 5.6 Verify the same for one Conversation: a message, a run advancing, and the composer's own send
- [x] 5.7 Add the standing test — after first paint, delivering any stream event leaves no view in a loading state — covering all six pages

## 6. Whole-change verification

- [x] 6.1 Run the console's Go and UI suites and verify both pass
- [x] 6.2 Verify a reconnect still resyncs and converges to a cold load, and that a manager-restart gap still renders as a gap
- [x] 6.3 Smoke on a cluster: send a message and confirm the transcript updates with no blank and no request beyond the send itself
- [x] 6.4 Update `docs/console.md` for how the console stays current, and regenerate screenshots if the UI moved
