## 1. CRD: the per-thread read watermark

- [x] 1.1 Add `ReadAt *metav1.Time` and `ReadTracked bool` to `ThreadBinding` in
      `api/v1alpha1/conversation_types.go`, both `+optional`, with comments
      naming why two fields are needed (an absent `readAt` means "never read"
      for a tracked binding and "predates the mechanism" for an untracked one)
      and citing `status.runs[].deliveryTracked` as the precedent.
- [x] 1.2 Add a helper on `ConversationStatus` (or `ThreadBinding`) answering
      "is this binding unread given `lastActivity`", so the manager, the console
      and the tests share one implementation of the table in design decision 4.
- [x] 1.3 Regenerate deepcopy and CRDs in the golang container (no local Go
      toolchain):
      `controller-gen object paths=./api/...` and
      `controller-gen crd paths=./api/... output:crd:artifacts:config=chart/files/crds`.
      Confirm `ConversationStatus.DeepCopyInto` now loops over `Threads` instead
      of `copy(*out, *in)` — a plain copy would alias the `*metav1.Time`.
- [x] 1.4 Confirm `chart/files/crds/` carries `readAt`/`readTracked` under
      `status.threads.items`.

## 2. Manager: stamp bindings, serve `POST /channel/read`

- [x] 2.1 In `internal/chat/ops.go`, set `ReadTracked: true` on the
      `ThreadBinding` appended when an `ensure-topic` op completes — for every
      channel, so the backfill rule stays one rule.
- [x] 2.2 Add `POST /channel/read` to `internal/httpapi/server.go`: route under
      `s.adapterAuth`, resolve the named `Channel`, enforce
      `scopeAllows(r, ch.Spec.Adapter)` via the existing `stateChannel` path, and
      document it in the route comment block at the top of the file.
- [x] 2.3 Implement the handler: decode `{"channel","reads":[{"threadId","readAt"}]}`,
      refuse an empty list and more than 50 entries with 400, resolve each
      `threadId` to its `Conversation` (list by namespace, match
      `status.threads[].{channel,threadId}`), and status-patch the binding.
- [x] 2.4 Enforce the watermark rules: clamp `readAt` to the manager's `now`;
      skip without writing when the value is at or before the stored one; group
      entries hitting the same conversation into ONE patch.
- [x] 2.5 Return per-entry outcomes `marked` / `skipped` / `failed` with reasons
      plus totals, 200 for a mixed batch, and never abort on one bad entry.
- [x] 2.6 Unit tests in `internal/httpapi`: monotonic rejection, future clamp,
      oversized batch, empty batch, unknown thread, cross-adapter scope refusal,
      unauthenticated refusal.
- [x] 2.7 Envtest in `internal/integration`: report a read against a real API
      server and read the field back off the object — this is what catches the
      CRDs not being re-applied.

## 3. Console BFF: derive, filter, count, report

- [x] 3.1 `console/conversations.go`: parse `readAt`/`readTracked` into the
      console's `ThreadBinding` view; add `Unread bool` and `ReadAt string` to
      `ConversationSummary`, set in `summarize` from the CONSOLE channel's
      binding only (no binding ⇒ not unread), using `sortKey()` as the activity
      side of the comparison.
- [x] 3.2 `console/convapi.go`: add `Unread bool` to `ConversationFilter`, wire
      `?unread=true` through `parseFilter` and `matches`.
- [x] 3.3 In `handleConversations`, compute `unreadTotal` over all conversations
      BEFORE applying the filter, and add it to the response; support `?count=1`
      returning totals with no items.
- [x] 3.4 `console/adapter.go`: add `ReportRead(ctx, reads []{conversation, readAt})`
      — map conversation names to console thread ids via the existing
      `ThreadFor`, batch, and POST to the manager's `/channel/read`. Return
      per-name outcomes; skip names with no console thread using the existing
      `notJoinedReason`.
- [x] 3.5 `console/convapi.go` + `api.go`: add `POST /api/conversations/read`
      taking `{"names":[…]}`, bounded at `conversationPageSize`, authenticated
      and identity-logged but NOT behind `a.write(...)` (design decision 7 —
      flip this to `a.write` if that call is reversed in review).
- [x] 3.6 Go tests in `console/`: unread derivation for joined / observed /
      untracked / never-read bindings; the count is computed pre-filter; the
      unread filter narrows; the batch bound is enforced; observed names come
      back skipped.

## 4. Console UI

- [x] 4.1 `ui/src/api/types.ts`: add `unread`, `readAt` to the conversation
      summary and `unreadTotal` to the list response.
- [x] 4.2 `ui/src/api/hooks.ts` + `client.ts`: a `useMarkRead()` mutation, and a
      shared count-only query for the navigation badge.
- [x] 4.3 `ui/src/pages/Conversations.tsx`:
      - mark unread rows (title emphasis plus a small label), using theme tokens
        only — no literal colours;
      - an **Unread only** switch next to *Errored only*, resetting the page;
      - a **Mark read** button over the selection, disabled with nothing
        selected;
      - decouple the selection column from `canClose` so a read-only console can
        still select and mark read.
- [x] 4.4 `ui/src/pages/Conversation.tsx`: report the console thread read on
      open, and again when `lastActivity` advances while the view is mounted;
      never send a locally generated timestamp; skip when it would not advance.
- [x] 4.5 `ui/src/App.tsx`: unread count badge on the Conversations nav item
      from the count-only query.
- [x] 4.6 Vitest coverage in `Conversations.test.tsx`: unread rows render marked,
      the filter round-trips to the query string, mark-read posts the selection
      and clears it, the button is present with writes disabled, and observed
      rows report skipped.

## 5. Chart, images, docs

- [x] 5.1 Build and push new `agentops-manager` and `agentops-console` images
      with a fresh tag; update `chart/values.yaml` image refs and bump the chart
      version.
- [x] 5.2 `docs/concepts.md`: `status.threads[].readAt` / `readTracked`, the
      per-channel grain, the monotonic and clamping rules, and the backfill rule.
- [x] 5.3 `docs/contracts.md`: `POST /channel/read` — request, response, bound,
      scope, and that it is OPTIONAL for an adapter.
- [x] 5.4 `docs/console.md`: the unread mark, the filter, mark-read, and — said
      plainly — that read is per CHANNEL, so two operators share one console
      watermark and whoever opens a conversation clears it for both.
- [x] 5.5 `CHANGELOG.md`, newest first: the CRDs MUST be re-applied on upgrade
      (the API server prunes the field otherwise and every conversation reads as
      unread forever), and threads bound before the upgrade are treated as read
      once.
- [x] 5.6 `CLAUDE.md`: extend the `Channel adapter` terminology entry with the
      read watermark and the per-channel rule, since it is a contract fact a
      future change could otherwise re-litigate.

## 6. Verification

- [x] 6.1 `go build ./... && go vet ./...` at the root and in `console/`,
      plus `go test ./...` in both (envtest with `KUBEBUILDER_ASSETS` for the
      root suite).
- [x] 6.2 `cd console && make test` for the frontend suite.
- [x] 6.3 `helm template` the chart and confirm the CRD carries the new fields.
- [x] 6.4 Server-side dry-run against the live cluster before applying, then
      upgrade and confirm: an existing conversation shows read, a fresh signal
      shows unread, opening it clears the mark, a second browser sees it cleared,
      and `kubectl get conversation <name> -o yaml` shows `readAt` on the console
      binding only.
      DONE against the live cluster (chart 5.18.0, revision 74, manager 0.33.0 /
      console 0.13.0, applied via `_home-data-center`). Verified: 31 existing
      conversations all untracked -> `unreadTotal=0` (backfill, no invented
      backlog); a fresh `kind: task` signal opened `task-kjb5p` with
      `readTracked: true` on BOTH bindings while the alert beside it has none;
      it rendered `unread=true` and `?unread=true` returned exactly it; marking
      read through the console cleared it to 0 and wrote `readAt` to the
      **console binding only**, `home-ops` untouched. Rules confirmed live: an
      earlier report is `skipped` with the watermark unmoved, a +48h report is
      clamped to the manager's `now`, an unknown thread is `failed` without
      stopping the batch, 51 entries is 400, no token is 401.
      NOTE the upgrade needed `--args "--force-conflicts"`: `.spec.versions` on
      the conversations CRD was owned by an abandoned `kubectl-client-side-apply`
      field manager, and helm's server-side apply refused it. The first sync
      rolled the WORKLOADS and left the CRD behind — new manager, old schema,
      the field silently pruned. Worth knowing for the next CRD change.

---

*Sections 7–10 extend the change after the per-channel work above shipped
(chart 5.18.0, manager 0.33.0, console 0.13.0). Read is becoming per IDENTITY;
the channel-wide mark stays as the fallback. Nothing above is undone.*

## 7. CRD: the per-identity overlay

- [x] 7.1 Add `ReaderMark{Key string; ReadAt *metav1.Time}` and
      `Readers []ReaderMark` to `ThreadBinding` in
      `api/v1alpha1/conversation_types.go`, `+optional`, with comments naming
      why `readAt` STAYS (transports with no reader identity) and that `Key` is
      opaque to the manager — never an address, never derived here.
- [x] 7.2 Extend the `Unread` helper to take an optional reader key: the
      reader's own mark when it has an entry, the channel-wide `readAt`
      otherwise (a never-seen reader and an EVICTED one take the same path).
      Keep it the ONE implementation the manager, the console and the tests
      share.
- [x] 7.3 Regenerate deepcopy + CRDs in the golang container; confirm
      `ThreadBinding.DeepCopyInto` now loops over `Readers`.
- [x] 7.4 Confirm `chart/files/crds/` carries `readers` under
      `status.threads.items`.

## 8. Manager: reader-scoped watermarks

- [x] 8.1 `POST /channel/read`: accept an optional `reader` per entry. Absent =
      the channel-wide mark (unchanged behaviour, and the case every existing
      adapter is in). Present = that reader's entry.
- [x] 8.2 Monotonic and clamp rules evaluate against the READER's own stored
      value, so one reader's report is never skipped because another is ahead.
- [x] 8.3 Cap `readers[]` at 50 per binding, evicting the oldest `readAt`
      first; one entry per key, replaced in place.
- [x] 8.4 Refuse a `reader` that looks like an address (contains `@`) with 400 —
      a cheap guard against an adapter author sending the identity itself, since
      the manager cannot otherwise tell an opaque key from a plaintext one.
- [x] 8.5 Unit/envtest: two readers independent; reader-less report still
      advances the channel mark; eviction at 51; per-reader monotonicity; the
      `@` guard; the field round-trips off a real API server.

## 9. Console: hash the identity, derive per viewer, stamp own actions

- [x] 9.1 Read the reader-hash salt from the projected credential env
      (`AGENTOPS_CRED_<CHANNEL>_readerSalt`); with none, fall back to
      channel-wide marks and say so once at startup — a missing salt must
      degrade, never crash or hash unsalted.
- [x] 9.2 `readerKey(identity)` = salted SHA-256, hex, prefixed `sha256:`.
      Identity is the resolved one (forward-auth header, else `token`), so a
      shared token is one reader by construction.
- [x] 9.3 Send `reader` on every read report; derive `Unread`/`ReadAt` in
      `summarize` from the requesting viewer's key, falling back to the
      channel-wide mark.
- [x] 9.4 Stamp the acting reader on origination (`handleStart`) and on send
      (`handleSend`) — their own watermark only.
      Origination could not be stamped console-side: it is asynchronous and the
      console never learns the created conversation's name. Resolved the other
      way round (option A) — the chat signal carries the opaque `reader`, the
      Conversation persists it as `spec.originReader{channel,key}`, and
      `finishEnsureTopic` stamps that reader the moment it creates the binding,
      which is where it already sets `readTracked`. Read exactly once, per
      channel (keys are per-surface), and it never moves the channel-wide mark.
- [x] 9.5 Go tests: two identities diverge; shared token converges; a started
      conversation is not unread for its starter but is for everyone else;
      no identity anywhere in what is sent upstream; missing salt degrades.

## 10. Chart, docs, verification

- [x] 10.1 Generate the reader salt into the console Channel's credentials
      Secret under `.Release.IsInstall` ONLY (`lookup` is empty on every
      renderer without a cluster, so an upgrade-path generator APPLIES a new
      value rather than showing one) — and never regenerate, since rotating it
      orphans every key.
- [x] 10.2 New manager and console images, chart 5.19.0.
- [x] 10.3 `docs/concepts.md` (the overlay, the fallback, the bound, why the
      manager cannot read the key), `docs/contracts.md` (the optional `reader`
      field), `docs/console.md` (per person where an identity exists, per token
      otherwise; and that the object records how many read it and when),
      `CHANGELOG.md` (chart 5.19.0; re-apply the CRDs, with `--force-conflicts`
      where a CRD was ever hand-applied), `CLAUDE.md`.
- [x] 10.4 Full suite, `helm template`, then upgrade via `_home-data-center` and
      confirm live: two identities diverge on one conversation, a shared token
      does not, a conversation started from the console is not unread for its
      starter, and `kubectl get conversation <name> -o yaml` shows opaque keys
      and no address.
      DONE against the live cluster (chart 5.19.0, revision 75, manager 0.34.0 /
      console 0.14.0). Alice started `chat-4g4d5` from the console: the CR
      carried `spec.originReader{console, sha256:b40d…}` and her key was stamped
      on the console binding at thread creation (08:17:54) — twenty seconds
      before the agent answered and moved `lastActivity` to 08:18:14, which is
      what correctly made it unread to her again. The home-ops binding took NO
      reader (keys are per channel) and the channel-wide `readAt` stayed empty
      throughout. After alice marked it read: alice 0 unread, bob 1, and the
      `?unread=true` filter returned exactly that row for bob. No address
      appears anywhere in the object. The upgrade needed no
      `--force-conflicts` this time — helm owns `.spec.versions` since 5.18.0.
