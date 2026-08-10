## 1. Define the contract

- [x] 1.1 Name the markdown subset in the contract — bold, italic, inline code,
      fenced code, links — and state that anything outside it is undefined.
      Do this first: adapters diverge the moment it is ambiguous
- [x] 1.2 Define the four message kinds (`signal`, `answer`, `relay`, `notice`)
      with their typed fields — `signal` carries `pipeline`, `source`, and
      `inputRef` (named so, not `sourceRef`, since `source` now means the
      SignalSource) — and the `ensure-topic` descriptor (`conversation`,
      `pipeline`, `source`, `title`, `labels`, `kind`)
- [x] 1.3 Define the outbound contract version handshake on `GET /channel/ops`
      and the 400 response naming the expected version
- [x] 1.4 Write the contract into `docs/`/README before implementing, so the
      adapter and manager work from the same document

## 1b. Input provenance

- [x] 1b.1 Do NOT add `ConversationSpec.PipelineRef` — `mcp-toolset-model` and
      `CLAUDE.md` both state no `pipelineRef` is introduced, and
      `pipeline-addressed-conversations` landed without one. Render the
      pipeline from `chat.PipelineForConversation`, omitting it when the helper
      returns blank
- [x] 1b.2 Add `InputItem.Origin` (`{kind: signal|channel, name}`) and fold
      `JobName` into it; `signals.go` currently records the source name for
      `kind: job` only. Two-valued enum — `POST /task` is gone, so there is no
      `api` kind
- [x] 1b.3 Populate provenance at both creation sites: `routeSignalGroup`
      (`signal` + the source name, for every kind including `chat`) and the chat
      router's command/reply paths (`channel` + the channel name)
- [x] 1b.4 Regenerate deepcopy + the Conversation CRD; the field is optional so
      existing conversations stay valid
- [x] 1b.5 Update `internal/integration/signaladapter_test.go:118`, which
      asserts on `JobName`
- [x] 1b.6 Carry the originating signal's `kind` onto the input so D5 can
      exclude chat — a chat signal is a `signal` origin, so origin kind alone
      would echo the user's own message back at them

## 2. Message types in the manager

- [x] 2.1 Replace `Op.Text`/`Op.Title` in `internal/chat/ops.go` with the typed
      message and topic descriptor; keep FIFO, dedup, claim/complete, and
      `ReclaimAfter` untouched
- [x] 2.2 Split `EnqueueSend` into per-kind constructors; give
      `EnqueueEnsureTopic` the descriptor
- [x] 2.3 Enforce the version handshake in the ops endpoint

## 3. Strip transport dialect from the manager

- [x] 3.1 `internal/chat/router.go`: every hand-written HTML string
      (`<b>Agents</b>`, `&lt;profile&gt;`, usage and error text) becomes a
      `notice` with a markdown body
- [x] 3.2 `relayToSiblings` emits `relay` with `{origin, sender, body}` instead
      of composing `💬 <b>…</b>: …`
- [x] 3.3 `internal/httpapi/server.go` `/work/done` fan-out emits `answer`
      (with `status`), and the failure path emits a `notice`
- [x] 3.4 Grep `internal/` for residual markup — `<b>`, `&lt;`, `&gt;`,
      `parse_mode` — and confirm none remains

## 4. Renderers

- [x] 4.1 `channel-telegram`: renderer package translating each kind to
      Telegram HTML; escape all interpolated content
- [x] 4.2 `channel-telegram`: split or truncate at the 4096-char limit in
      `Send` — today it posts verbatim and a long message simply fails
- [x] 4.3 `channel-telegram`: derive the forum-topic name from the descriptor,
      within Telegram's 128-char topic limit
- [x] 4.4 `channel-telegram`: optionally present an oversized `signal` payload
      as a document rather than truncating it
- [x] 4.5 `internal/chat/provider.go`: in-process registry providers render the
      same kinds — no shared manager-side renderer for them to call

## 5. Input cards

- [x] 5.1 Enqueue a `signal` message when an input is appended and a thread
      binding for that channel exists — not at conversation creation, or
      `FanOutSend` drops it
- [x] 5.2 Apply the posting rule from `origin.kind` — `signal` is posted,
      `channel` is not, an ABSENT origin is not (pre-upgrade inputs must not
      spray cards into every open thread) — rather than enumerating input
      types, with chat excluded by the signal's own kind
- [x] 5.3 Stable op id `input:<conversation>:<inputID>:<channel>` so
      reconcile-driven re-enqueues dedup against the pending map and the
      `recent` window
- [x] 5.4 Inline the full payload in the message body, set `inputRef` to the
      `ConversationInput` name, take `source` from the input's origin and
      `pipeline` from `chat.PipelineForConversation` (blank when ambiguous)
- [x] 5.5 Confirm posting neither gates nor is gated by dispatch

## 6. Tests

- [x] 6.1 Unit: no output from `internal/` contains transport markup — assert
      across every message-producing path, not just the ones touched
- [x] 6.2 Unit (telegram): a 10k-char body is split and every part posts; a
      payload containing `<`, `>`, `&` posts intact
- [x] 6.3 Unit (telegram): topic name derived from a descriptor whose rendered
      form exceeds 128 chars
- [x] 6.4 envtest: an alert-originated conversation posts its card before the
      run completes, and the card survives a failed run
- [x] 6.5 envtest: repeated reconciles of a conversation post exactly one card
      per bound channel
- [x] 6.6 envtest: a chat message or reply produces no card; siblings still get
      the relay
- [x] 6.7 envtest: a `kind: task` signal conversation posts its task text
- [x] 6.11 envtest: an alert card names its pipeline and source; a conversation
      whose pipeline cannot be inferred renders without it and does not fail;
      an input with no origin at all produces no card
- [x] 6.8 envtest: two bound channels each render the same answer independently
- [x] 6.9 Contract: an adapter declaring no version, or an old one, gets 400
- [x] 6.10 Full suite: `go build ./... && go vet ./...` in all modules,
      `KUBEBUILDER_ASSETS=… go test ./...`

## 7. Docs and rollout

- [x] 7.1 Update the contract description — it currently says "post a message;
      chat HTML subset", which is the leak being removed
- [x] 7.2 `CLAUDE.md`: add the invariant — the manager composes meaning,
      adapters compose presentation; no transport dialect in `internal/`
- [x] 7.3 Document the input card in README/`docs/`: what a thread looks like
      now, and that the event precedes the answer
- [x] 7.4 Migration note: manager and adapters upgrade together; third-party
      adapters must implement the renderer and declare the version
- [x] 7.5 Sequence the rollout per the design — types and renderers first
      (inert), then flip the manager, then add the card
- [x] 7.6 `chat-signal-origination` has landed, so this change ships second and
      owns the exclusion: confirm a chat signal produces no card even though it
      carries a `signal` origin
