# Design: add-web-chat-channel

> **Rebased 2026-08-05** on `make-channel-type-architecture-extendable` (implemented). What changed for this design:
> - **D1 (CRD)**: no `spec.web` sub-struct and no CEL exclusivity rule — a web channel is `spec.type: web` with settings in the opaque `spec.config` (parsed by the in-process web provider, which IS the serving implementation). Auth token: `WEB_CHAT_TOKEN` manager env (chart-provisioned), because the manager performs zero Secret API reads now; D6's `GetAPIReader` approach is dead.
> - **D2 (Router)**: already extracted and landed in `internal/chat/router.go` — this change only consumes it. The web provider registers in the `Registry` under type `web`; router acks reach it through the shared op pipeline (in-process dispatch).
> - **D3 (thread ids)**: `status.threadId` is a string everywhere now — the web provider synthesizes ids like `web-<nanos>`; no int64 gymnastics.
> - **D8 (delivery)**: landed generically (`spec.delivery`, default `result` mode). Web channels simply omit `spec.delivery` — the default printed-answer wording is already what they need; only the `/work/done` result-cap raise (D4) remains to implement here.

## Context

The operator has exactly one channel type, Telegram, and its shape leaks everywhere:

- `Provider` (`internal/chat/provider.go:9`) is send-only (`EnsureTopic`, `Send`); inbound handling lives entirely inside the Telegram poller (`internal/chat/poller.go`), entangled with Telegram update parsing.
- Channel selection is nil-checking `spec.telegram` — there is no `channelType` discriminator. Two hardcoded resolution sites: `ChatFactory` and `TokenReader` closures in `cmd/manager/main.go:88-145`.
- Outbound agent replies bypass the manager: prompt templates instruct the agent to curl the Telegram Bot API from inside the runtime pod. The manager only receives a 2000-char-truncated `result` via `POST /work/done`.
- The HTTP server (`internal/httpapi/server.go`) is non-leader-gated, unauthenticated, ClusterIP-only; the chart has no Ingress and no UI assets exist anywhere (manager image is distroless — assets must be `go:embed`ed).
- `ConversationStatus.ThreadID` is `*int64` (Telegram topic id).

Constraints (binding, from CLAUDE.md): the manager never reads agent secrets (Channel-level secrets via `GetAPIReader` are the sanctioned exception); strictly serial per conversation; HTTP API stays non-leader-gated; chat poller stays leader-only.

## Goals / Non-Goals

**Goals:**
- A `web` channel type usable from a browser with zero external dependencies, deployed by default with the chart.
- One transport-neutral inbound routing path shared by Telegram and web (commands, adoption, default profile, busy-ack).
- Manager-controlled outbound: web replies come from `/work/done` results; web-channel agents need no chat credentials.
- Auth on by default for the browser surface.

**Non-Goals:**
- No multi-user identity/presence, no per-user authorization (single shared bearer token for now; Telegram `approvers` has no web equivalent yet).
- No persistent message store beyond what Conversation CRs already hold; no external DB.
- No frontend build toolchain (no npm/bundler) — hand-written HTML/CSS/JS only.
- No change to Telegram behavior or to the `/work` dispatch contract.
- No WebSocket; SSE is sufficient for one-directional updates.

## Decisions

### D1: `spec.web` sub-struct, not a `channelType` enum
Add `web *WebChannel` alongside `telegram *TelegramChannel` in `ChannelSpec`; selection stays "which sub-struct is non-nil". A required enum would break existing CRs. Add a CEL `XValidation` rule enforcing at most one of `telegram`/`web` is set. `WebChannel` fields: `authTokenSecretRef *corev1.SecretKeySelector` (optional → auth disabled if nil). `defaultProfileRef` already exists at `ChannelSpec` level and is shared.

*Alternative considered:* discriminated union with `type: telegram|web` — cleaner long-term but **BREAKING** for existing Channels; rejected pre-1.0 for churn without benefit.

### D2: Extract a transport-neutral `Router` in `internal/chat`
New `Router{Client, Namespace}` owning the logic currently unexported on `*Poller`: `HandleMessage`, `HandleCommand` (via `addressing.Parse`), `CreateTaskConversation`, `AppendInput`, `ConvByThread` — operating on a neutral `InboundMessage{ThreadID *int64, Text string}` plus the resolved `Provider` for acks. The poller keeps Telegram parsing, offset persistence, and approver filtering (approvers are Telegram user ids — transport-specific by nature) and delegates the rest. Dispatch/ingest test fixtures stay untouched; poller behavior must be provably unchanged (existing integration tests are the guard).

### D3: Web provider synthesizes thread ids; acks are ephemeral
The reconciler calls `EnsureTopic` whenever `ChannelRef != nil && Status.ThreadID == nil`, so the web `Provider.EnsureTopic` returns a synthesized unique `int64` (nanosecond-derived) rather than widening `ThreadID`'s type. In the web UI a Conversation *is* the thread, so the id is only a correlation token. `Provider.Send` (acks/errors from the router) publishes to an in-memory per-conversation broadcast (fed to SSE subscribers) and is not persisted — acks are transient UX, and losing them on manager restart is acceptable.

*Alternative considered:* new `status.webThreadId` string field — more honest but touches the reconciler's topic-ensure logic for no functional gain.

### D4: Transcript is derived from the Conversation CR; no new store
History = user messages from `spec.inputs` + agent replies from `status.runs[]`. This means:
- Raise the `/work/done` result cap from 2000 to 16384 chars (etcd headroom is fine given runs pruning); truncation note appended when hit.
- Input pruning (existing controller behavior) bounds how far back user-message history goes — acceptable for an ops chat; documented as a known limitation rather than fought with a message store.

*Alternative considered:* per-conversation ConfigMap message log — durable full history, but a second write path, GC surface, and RBAC noise; not worth it for v1.

### D5: Browser API under `/chat/api/*` on the existing server; SSE by polling the cache
New routes in `httpapi.Server.Handler()` (non-leader-gated, correct for a UI — must serve during rollouts):
- `GET  /chat/api/profiles` — list AgentProfiles (the `/agents` equivalent).
- `GET  /chat/api/conversations?channel=<name>` — conversations referencing that web channel.
- `POST /chat/api/conversations` `{channel, profile?, agent?, text}` — router `CreateTaskConversation`; `/profile:agent` command syntax in `text` also honored via the router, mirroring Telegram General behavior.
- `GET  /chat/api/conversations/{name}` — derived transcript (D4).
- `POST /chat/api/conversations/{name}/messages` `{text}` — router `AppendInput` (reply lane, busy-ack semantics preserved).
- `GET  /chat/api/conversations/{name}/events` — SSE: emits transcript deltas by re-reading the cached client every ~2s plus in-memory ack events (D3). Polling the informer cache is cheap and avoids wiring raw watches through the server.

### D6: Auth — shared bearer token via Channel secretRef, resolved like the bot token
Middleware on `/chat/api/*`: `Authorization: Bearer <token>` compared constant-time against the value from `web.authTokenSecretRef`, read via `GetAPIReader` (uncached GET — same sanctioned pattern and RBAC as the Telegram bot token; the manager-never-reads-agent-secrets invariant holds). Resolved value cached in-process ~30s. If `authTokenSecretRef` is nil, the API is open (explicit opt-out for trusted networks). The static UI shell is served without auth; the token is entered in the UI and kept in `localStorage`.

### D7: UI is a single embedded page in a new `internal/webui` package
`go:embed` of `index.html` + one JS + one CSS file (precedent: embedded dispatch templates). Served at `GET /chat/` from the existing server. Features: conversation list (grouped by channel), transcript view with run/status badges, message composer, profile picker for new conversations, token settings. Rendered agent HTML is sanitized to the same tag subset Telegram allows (replies are authored against that format today).

### D8: Channel-aware delivery instructions in dispatch
`dispatch.Next` learns the channel kind (from the Conversation's resolved Channel) and selects delivery wording: Telegram keeps the curl instructions; web (and chat-less) units get "your final printed answer is the deliverable — it is captured via /work/done and shown in the chat UI". `format.md` stays the message-format spec but its Telegram-only framing becomes "chat HTML subset". Fixture changes are deliberate and reviewed (pinned-fixtures rule).

### D9: Chart ships a default web Channel, on by default; Ingress optional and path-scoped
- `values.yaml`: `webChannel.{enabled: true, name: web, defaultProfile: "", auth: {enabled: true, existingSecret: ""}}` and `ingress.{enabled: false, className, host, tls, annotations}`.
- New template renders the `Channel` CR (gated on `webChannel.enabled`, requires CRDs — same caveat as `demo.yaml`) and, when `auth.enabled` and no `existingSecret`, a token Secret generated with `randAlphaNum 32` + a `lookup`-based keep pattern so `helm upgrade` doesn't rotate it.
- Ingress routes **only the `/chat` path prefix** to the Service's `api` port — never the whole port, because 8080 also serves `/work`, `/task`, `/ingest` which must stay cluster-internal. Service stays ClusterIP.

## Risks / Trade-offs

- [Exposing port 8080 exposes internal endpoints too] → Ingress template is hard-scoped to `/chat`; docs state plainly that port-forward or `/chat`-scoped routing are the only supported exposures. Defense-in-depth option (auth on everything) deferred.
- [Shared bearer token, no per-user identity] → acceptable for a single-team ops tool; token rotation = update Secret. Documented; per-user auth is future work.
- [History bounded by input pruning + runs pruning] → documented limitation (D4); transcript shows "older messages pruned" marker when detectable.
- [Router extraction regresses Telegram behavior] → extraction is move-only where possible; envtest integration suite with fake chat must pass unmodified before/after.
- [Helm `lookup` fails under `--dry-run`/GitOps template-only rendering] → fall back to generating a new token when lookup is unavailable and document `existingSecret` as the GitOps-safe path.
- [SSE cache-poll adds ~2s reply latency] → fine for an ops chat; interval is a constant, trivially tunable later.
- [Synthesized int64 thread ids could collide with real Telegram topic ids in a mixed cluster] → correlation is always scoped per-Channel (`ConvByThread` filters by channelRef), so cross-channel collision is harmless.

## Migration Plan

1. Ship CRD change (new optional field — additive, no conversion needed); `helm upgrade` with `crds.enabled` updates it.
2. Chart default `webChannel.enabled: true` creates the Channel + Secret on upgrade; existing Telegram Channels are untouched. Users who don't want the UI set `webChannel.enabled: false`.
3. Rollback = disable the value and delete the Channel CR; no data migration in either direction (transcripts live in Conversation CRs that already exist).

## Open Questions

- Should `/task` (existing programmatic endpoint) also honor the web channel for its optional `channel` field? (It should Just Work via `ChannelRef`; verify, don't assume.)
- Result cap 16384: confirm runs-list pruning bound keeps worst-case Conversation object size well under etcd limits before merging.
