# Proposal: add-web-chat-channel

> **SUPERSEDED 2026-08-09 by `visualize-agent-ops` — do not implement.** The
> agent-ops console lands the user need this change exists for (chat with an
> agent from a browser, no Telegram account) as an out-of-process channel
> adapter, plus the observability surface this change never had. The
> architecture moved decisively out-of-process since this was written — every
> signal type is adapter-served and `ChannelAdapter` owns workloads — so an
> in-process web channel inside the manager would grow exactly the component
> the design has been shrinking: a browser surface, an auth token, and
> CR-snapshot APIs on the manager.
>
> Nothing of this change was implemented, so there is no code to unwind; the
> five tasks marked `[x]` were groundwork the console does not reuse. One idea
> here is worth keeping and is NOT covered by the console: result-size limits
> on `/work/done`. The console renders whatever `status.runs[].result` carries,
> so truncation tuning remains an open manager-side concern for whoever needs
> it. **Withdraw or re-scope this change to that alone.** (Decision 7 of
> `visualize-agent-ops/design.md`.)

> **Rebased 2026-08-05** on the implemented `make-channel-type-architecture-extendable` change: generic Channel CRD (`type` + opaque `config`), string thread ids, shared Router/OpQueue, and a zero-secret-reads manager are now the ground truth this change builds on.
>
> **Rebase note 2026-08-06 (`wire-it-up` landed)**: conversations are now multi-channel — `spec.channelRefs[]` + `status.threads[]{channel,threadId}` replaced the single `channelRef`/`threadId`. Impacts for this change: (1) conversation listing/transcript queries must filter by `BoundTo(<web channel>)` and read the web channel's own thread binding, not a global thread id; (2) the web provider's `Send` receives fan-out results, acks, AND attributed relay messages from sibling channels (a Pipeline can mirror telegram↔web) — render relays as distinct "remote user" messages; (3) the **no-relay-loop rule is binding**: the web provider must never feed its own outbound posts (including relays) back through the Router as inbound user messages; (4) multi-channel conversations force result delivery, which is already this change's default path — the `/work/done`-sourced transcript needs no change, but fan-out `send` ops now ALSO carry the reply, so the SSE/transcript layer should dedupe (prefer runs[] as the source of truth, treat provider sends as ephemeral display events).

## Why

Today the only chat channel type is Telegram, so every deployment needs an external bot token and a Telegram account before anyone can talk to an agent. A built-in web UI chat channel — served by the manager and deployed by default with the Helm chart — makes the operator usable out of the box with zero external dependencies, and gives a channel whose outbound path the manager fully controls (no agent-side credentials needed).

## What Changes

- Add a built-in `web` channel type: a Channel with `spec.type: web`, its settings under the opaque `spec.config` (per the landed extensible-channel architecture from `make-channel-type-architecture-extendable`), served by an in-process provider registered in the manager's channel `Registry`.
- Reuse the landed transport-neutral `Router` and op pipeline (`internal/chat`) — the web chat API feeds inbound messages through the same code path as external adapters; acks flow back through the in-process provider.
- Add browser-facing HTTP endpoints to the manager (`/chat/...`): list conversations, read message history, post a message, and stream agent replies (SSE). Replies are sourced from `status.runs[].result` (`POST /work/done`) — web-channel agents never need chat credentials.
- Embed a minimal single-page chat UI into the manager binary via `go:embed` (distroless-compatible, no asset layer), served by the existing non-leader-gated HTTP server.
- Add optional bearer-token auth for the `/chat/*` surface. NOTE (rebase): the manager now reads ZERO secrets, so the web chat token cannot come from a `SecretKeySelector` read by the manager — it arrives via manager env (`WEB_CHAT_TOKEN`, chart-provisioned Secret injected as env, same pattern as `ADAPTER_TOKEN`).
- Helm chart: deploy a default web `Channel` CR (plus generated auth token Secret) enabled by default (`webChannel.enabled: true`); add an optional Ingress template; keep the Service ClusterIP by default.
- Prompt templates: for web channels, make the "printed answer is the deliverable" path the primary one (no Telegram curl instructions), and note that `/work/done` result truncation must be raised or removed for web replies to be useful.

## Capabilities

### New Capabilities

- `web-channel-type`: the built-in `type: web` channel — in-process provider (Registry), synthesized string thread ids, web-specific `spec.config` shape, and routing behavior via the landed shared Router.
- `web-chat-api`: browser-facing HTTP API — auth, conversation listing, message history, posting messages, and live reply streaming from run results.
- `web-chat-ui`: the embedded single-page chat application served by the manager.
- `web-chat-deployment`: Helm chart deployment of the default web channel — values, default Channel CR, auth token Secret, optional Ingress.

### Modified Capabilities

<!-- none — no existing specs in openspec/specs/ -->

## Impact

- `api/v1alpha1/`: NO CRD change needed (rebase) — `type: web` + opaque `config` already fit the generic Channel schema.
- `internal/chat/`: new web provider registered in the existing `Registry`; Router and OpQueue are reused as landed.
- `internal/httpapi/server.go`: new `/chat/*` routes, SSE streaming, auth middleware; `/work/done` result size limit revisited.
- `cmd/manager/main.go`: `ChatFactory`/`TokenReader` grow a web branch; UI assets wiring.
- New embedded web assets (single-page app, no build toolchain — hand-written HTML/JS/CSS).
- `internal/dispatch/templates/*.md`: channel-aware reply instructions; `format.md` gets a web variant or channel-neutral wording.
- `chart/`: new `webChannel.*` values, default Channel CR template, token Secret, optional Ingress template; README update.
- Security surface: first browser-facing endpoint set — auth on by default in the chart; documented invariants (HTTP API stays non-leader-gated; strictly-serial-per-conversation unchanged).
