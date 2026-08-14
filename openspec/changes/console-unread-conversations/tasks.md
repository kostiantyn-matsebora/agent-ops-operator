## 1. CRD: the per-thread read watermark

- [ ] 1.1 Add `ReadAt *metav1.Time` and `ReadTracked bool` to `ThreadBinding` in
      `api/v1alpha1/conversation_types.go`, both `+optional`, with comments
      naming why two fields are needed (an absent `readAt` means "never read"
      for a tracked binding and "predates the mechanism" for an untracked one)
      and citing `status.runs[].deliveryTracked` as the precedent.
- [ ] 1.2 Add a helper on `ConversationStatus` (or `ThreadBinding`) answering
      "is this binding unread given `lastActivity`", so the manager, the console
      and the tests share one implementation of the table in design decision 4.
- [ ] 1.3 Regenerate deepcopy and CRDs in the golang container (no local Go
      toolchain):
      `controller-gen object paths=./api/...` and
      `controller-gen crd paths=./api/... output:crd:artifacts:config=chart/files/crds`.
      Confirm `ConversationStatus.DeepCopyInto` now loops over `Threads` instead
      of `copy(*out, *in)` — a plain copy would alias the `*metav1.Time`.
- [ ] 1.4 Confirm `chart/files/crds/` carries `readAt`/`readTracked` under
      `status.threads.items`.

## 2. Manager: stamp bindings, serve `POST /channel/read`

- [ ] 2.1 In `internal/chat/ops.go`, set `ReadTracked: true` on the
      `ThreadBinding` appended when an `ensure-topic` op completes — for every
      channel, so the backfill rule stays one rule.
- [ ] 2.2 Add `POST /channel/read` to `internal/httpapi/server.go`: route under
      `s.adapterAuth`, resolve the named `Channel`, enforce
      `scopeAllows(r, ch.Spec.Adapter)` via the existing `stateChannel` path, and
      document it in the route comment block at the top of the file.
- [ ] 2.3 Implement the handler: decode `{"channel","reads":[{"threadId","readAt"}]}`,
      refuse an empty list and more than 50 entries with 400, resolve each
      `threadId` to its `Conversation` (list by namespace, match
      `status.threads[].{channel,threadId}`), and status-patch the binding.
- [ ] 2.4 Enforce the watermark rules: clamp `readAt` to the manager's `now`;
      skip without writing when the value is at or before the stored one; group
      entries hitting the same conversation into ONE patch.
- [ ] 2.5 Return per-entry outcomes `marked` / `skipped` / `failed` with reasons
      plus totals, 200 for a mixed batch, and never abort on one bad entry.
- [ ] 2.6 Unit tests in `internal/httpapi`: monotonic rejection, future clamp,
      oversized batch, empty batch, unknown thread, cross-adapter scope refusal,
      unauthenticated refusal.
- [ ] 2.7 Envtest in `internal/integration`: report a read against a real API
      server and read the field back off the object — this is what catches the
      CRDs not being re-applied.

## 3. Console BFF: derive, filter, count, report

- [ ] 3.1 `console/conversations.go`: parse `readAt`/`readTracked` into the
      console's `ThreadBinding` view; add `Unread bool` and `ReadAt string` to
      `ConversationSummary`, set in `summarize` from the CONSOLE channel's
      binding only (no binding ⇒ not unread), using `sortKey()` as the activity
      side of the comparison.
- [ ] 3.2 `console/convapi.go`: add `Unread bool` to `ConversationFilter`, wire
      `?unread=true` through `parseFilter` and `matches`.
- [ ] 3.3 In `handleConversations`, compute `unreadTotal` over all conversations
      BEFORE applying the filter, and add it to the response; support `?count=1`
      returning totals with no items.
- [ ] 3.4 `console/adapter.go`: add `ReportRead(ctx, reads []{conversation, readAt})`
      — map conversation names to console thread ids via the existing
      `ThreadFor`, batch, and POST to the manager's `/channel/read`. Return
      per-name outcomes; skip names with no console thread using the existing
      `notJoinedReason`.
- [ ] 3.5 `console/convapi.go` + `api.go`: add `POST /api/conversations/read`
      taking `{"names":[…]}`, bounded at `conversationPageSize`, authenticated
      and identity-logged but NOT behind `a.write(...)` (design decision 7 —
      flip this to `a.write` if that call is reversed in review).
- [ ] 3.6 Go tests in `console/`: unread derivation for joined / observed /
      untracked / never-read bindings; the count is computed pre-filter; the
      unread filter narrows; the batch bound is enforced; observed names come
      back skipped.

## 4. Console UI

- [ ] 4.1 `ui/src/api/types.ts`: add `unread`, `readAt` to the conversation
      summary and `unreadTotal` to the list response.
- [ ] 4.2 `ui/src/api/hooks.ts` + `client.ts`: a `useMarkRead()` mutation, and a
      shared count-only query for the navigation badge.
- [ ] 4.3 `ui/src/pages/Conversations.tsx`:
      - mark unread rows (title emphasis plus a small label), using theme tokens
        only — no literal colours;
      - an **Unread only** switch next to *Errored only*, resetting the page;
      - a **Mark read** button over the selection, disabled with nothing
        selected;
      - decouple the selection column from `canClose` so a read-only console can
        still select and mark read.
- [ ] 4.4 `ui/src/pages/Conversation.tsx`: report the console thread read on
      open, and again when `lastActivity` advances while the view is mounted;
      never send a locally generated timestamp; skip when it would not advance.
- [ ] 4.5 `ui/src/App.tsx`: unread count badge on the Conversations nav item
      from the count-only query.
- [ ] 4.6 Vitest coverage in `Conversations.test.tsx`: unread rows render marked,
      the filter round-trips to the query string, mark-read posts the selection
      and clears it, the button is present with writes disabled, and observed
      rows report skipped.

## 5. Chart, images, docs

- [ ] 5.1 Build and push new `agentops-manager` and `agentops-console` images
      with a fresh tag; update `chart/values.yaml` image refs and bump the chart
      version.
- [ ] 5.2 `docs/concepts.md`: `status.threads[].readAt` / `readTracked`, the
      per-channel grain, the monotonic and clamping rules, and the backfill rule.
- [ ] 5.3 `docs/contracts.md`: `POST /channel/read` — request, response, bound,
      scope, and that it is OPTIONAL for an adapter.
- [ ] 5.4 `docs/console.md`: the unread mark, the filter, mark-read, and — said
      plainly — that read is per CHANNEL, so two operators share one console
      watermark and whoever opens a conversation clears it for both.
- [ ] 5.5 `CHANGELOG.md`, newest first: the CRDs MUST be re-applied on upgrade
      (the API server prunes the field otherwise and every conversation reads as
      unread forever), and threads bound before the upgrade are treated as read
      once.
- [ ] 5.6 `CLAUDE.md`: extend the `Channel adapter` terminology entry with the
      read watermark and the per-channel rule, since it is a contract fact a
      future change could otherwise re-litigate.

## 6. Verification

- [ ] 6.1 `go build ./... && go vet ./...` at the root and in `console/`,
      plus `go test ./...` in both (envtest with `KUBEBUILDER_ASSETS` for the
      root suite).
- [ ] 6.2 `cd console && make test` for the frontend suite.
- [ ] 6.3 `helm template` the chart and confirm the CRD carries the new fields.
- [ ] 6.4 Server-side dry-run against the live cluster before applying, then
      upgrade and confirm: an existing conversation shows read, a fresh signal
      shows unread, opening it clears the mark, a second browser sees it cleared,
      and `kubectl get conversation <name> -o yaml` shows `readAt` on the console
      binding only.
