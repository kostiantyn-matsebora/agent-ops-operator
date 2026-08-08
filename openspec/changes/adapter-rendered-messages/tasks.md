## 1. Define the contract

- [ ] 1.1 Name the markdown subset in the contract — bold, italic, inline code,
      fenced code, links — and state that anything outside it is undefined.
      Do this first: adapters diverge the moment it is ambiguous
- [ ] 1.2 Define the four message kinds (`signal`, `answer`, `relay`, `notice`)
      with their typed fields — `signal` carries `pipeline`, `source`, and
      `inputRef` (named so, not `sourceRef`, since `source` now means the
      SignalSource) — and the `ensure-topic` descriptor (`conversation`,
      `pipeline`, `source`, `title`, `labels`, `kind`)
- [ ] 1.3 Define the outbound contract version handshake on `GET /channel/ops`
      and the 400 response naming the expected version
- [ ] 1.4 Write the contract into `docs/`/README before implementing, so the
      adapter and manager work from the same document

## 1b. Conversation provenance

- [ ] 1b.1 Check whether `pipeline-addressed-conversations` has already added
      `ConversationSpec.PipelineRef`; consume it if so, add it if not — one
      field, one name, not two
- [ ] 1b.2 Add `InputItem.Origin` (`{kind: signal|api|channel, name}`) and fold
      `JobName` into it; `signals.go:351` currently records the source name for
      `kind: job` only
- [ ] 1b.3 Populate provenance at every creation site: `routeSignalGroup`
      (pipeline + source), `POST /task` (api), the chat router (channel)
- [ ] 1b.4 Regenerate deepcopy + the Conversation CRD; both fields optional so
      existing conversations stay valid
- [ ] 1b.5 Update `internal/integration/signaladapter_test.go:118`, which
      asserts on `JobName`

## 2. Message types in the manager

- [ ] 2.1 Replace `Op.Text`/`Op.Title` in `internal/chat/ops.go` with the typed
      message and topic descriptor; keep FIFO, dedup, claim/complete, and
      `ReclaimAfter` untouched
- [ ] 2.2 Split `EnqueueSend` into per-kind constructors; give
      `EnqueueEnsureTopic` the descriptor
- [ ] 2.3 Enforce the version handshake in the ops endpoint

## 3. Strip transport dialect from the manager

- [ ] 3.1 `internal/chat/router.go`: every hand-written HTML string
      (`<b>Agents</b>`, `&lt;profile&gt;`, usage and error text) becomes a
      `notice` with a markdown body
- [ ] 3.2 `relayToSiblings` emits `relay` with `{origin, sender, body}` instead
      of composing `💬 <b>…</b>: …`
- [ ] 3.3 `internal/httpapi/server.go` `/work/done` fan-out emits `answer`
      (with `status`), and the failure path emits a `notice`
- [ ] 3.4 Grep `internal/` for residual markup — `<b>`, `&lt;`, `&gt;`,
      `parse_mode` — and confirm none remains

## 4. Renderers

- [ ] 4.1 `channel-telegram`: renderer package translating each kind to
      Telegram HTML; escape all interpolated content
- [ ] 4.2 `channel-telegram`: split or truncate at the 4096-char limit in
      `Send` — today it posts verbatim and a long message simply fails
- [ ] 4.3 `channel-telegram`: derive the forum-topic name from the descriptor,
      within Telegram's 128-char topic limit
- [ ] 4.4 `channel-telegram`: optionally present an oversized `signal` payload
      as a document rather than truncating it
- [ ] 4.5 `internal/chat/provider.go`: in-process registry providers render the
      same kinds — no shared manager-side renderer for them to call

## 5. Input cards

- [ ] 5.1 Enqueue a `signal` message when an input is appended and a thread
      binding for that channel exists — not at conversation creation, or
      `FanOutSend` drops it
- [ ] 5.2 Apply the posting rule from `origin.kind` — `signal` and `api` are
      posted, `channel` is not — rather than enumerating input types
- [ ] 5.3 Stable op id `input:<conversation>:<inputID>:<channel>` so
      reconcile-driven re-enqueues dedup against the pending map and the
      `recent` window
- [ ] 5.4 Inline the full payload in the message body, set `inputRef` to the
      `ConversationInput` name, and carry `pipeline` + `source` from the
      conversation and the input's origin
- [ ] 5.5 Confirm posting neither gates nor is gated by dispatch

## 6. Tests

- [ ] 6.1 Unit: no output from `internal/` contains transport markup — assert
      across every message-producing path, not just the ones touched
- [ ] 6.2 Unit (telegram): a 10k-char body is split and every part posts; a
      payload containing `<`, `>`, `&` posts intact
- [ ] 6.3 Unit (telegram): topic name derived from a descriptor whose rendered
      form exceeds 128 chars
- [ ] 6.4 envtest: an alert-originated conversation posts its card before the
      run completes, and the card survives a failed run
- [ ] 6.5 envtest: repeated reconciles of a conversation post exactly one card
      per bound channel
- [ ] 6.6 envtest: a chat message or reply produces no card; siblings still get
      the relay
- [ ] 6.7 envtest: `POST /task` conversation posts its task text
- [ ] 6.11 envtest: an alert card names its pipeline and source; a conversation
      with no pipeline renders without them and does not fail
- [ ] 6.8 envtest: two bound channels each render the same answer independently
- [ ] 6.9 Contract: an adapter declaring no version, or an old one, gets 400
- [ ] 6.10 Full suite: `go build ./... && go vet ./...` in all modules,
      `KUBEBUILDER_ASSETS=… go test ./...`

## 7. Docs and rollout

- [ ] 7.1 Update the contract description — it currently says "post a message;
      chat HTML subset", which is the leak being removed
- [ ] 7.2 `CLAUDE.md`: add the invariant — the manager composes meaning,
      adapters compose presentation; no transport dialect in `internal/`
- [ ] 7.3 Document the input card in README/`docs/`: what a thread looks like
      now, and that the event precedes the answer
- [ ] 7.4 Migration note: manager and adapters upgrade together; third-party
      adapters must implement the renderer and declare the version
- [ ] 7.5 Sequence the rollout per the design — types and renderers first
      (inert), then flip the manager, then add the card
- [ ] 7.6 Reconcile with `chat-signal-origination`: its chat inputs must be
      excluded from posting by rule 5.2 — land the rule with whichever ships
      second
