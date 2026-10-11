## Why

Measured live on the home install, 2026-10-11: a Telegram-bound conversation
(`chat-tt4gh`) never got its topic. Its three agent replies sat undelivered
for over nine hours.

The `channel-telegram` adapter's long-poll connection to the manager had
landed on a non-leader replica and stayed pinned there. Kubernetes
load-balances per TCP connection, not per request.

`durable-chat-ops-broker` (archived 2026-10-10) was supposed to fix exactly
this class of outage. Its own proposal said the in-memory op queue would
become "a cache of CR state rather than the sole owner of it."

What actually shipped only moved the CLAIM to the Conversation CR, for crash
recovery. The op itself, and `/channel/ops`'s whole serve path, stayed
leader-only. A non-leader still 503s unconditionally.

The adapter's only recourse — `CloseIdleConnections` on 503, redial, hope to
land on the leader — is a probabilistic retry, not a fix.

With 3 replicas that is a 2-in-3 chance of missing on every redial. It was
observed taking many dozens of redials in a row to succeed.

Separately: `channel-telegram` still holds its own copy of the bot token. It
calls `api.telegram.org` directly for every outbound action (`sendMessage`,
`createForumTopic`, etc).

`gateway-telegram` was built to be THE single component that talks to
Telegram. Today that is only true for the inbound `getUpdates` stream.

Splitting outbound sending into a second credential-holding component
undermines the one thing the gateway exists to guarantee: one process, one
credential, no second place a bot token can leak from or be misused.

## What Changes

- The leader stays the SOLE broker. The claim (`status.threads[].claim`) and
  `OpQueue` are unchanged from `durable-chat-ops-broker` — leader-identity
  gated, exactly as before. Nothing about WHO decides moves.
- WHO ANSWERS THE HTTP REQUEST changes. A non-leader no longer 503s
  `/channel/ops` outright — it resolves the Lease holder's pod and reverse-
  proxies the request there, byte for byte, falling back to 503-and-retry
  only when the leader is unreachable from here.
- `gateway-telegram` becomes the sole holder of the Telegram bot credential,
  inbound and outbound. It gains `POST /telegram/call/{method}`, a verbatim
  relay to the real Bot API, authenticated by a shared bearer token.
- `channel-telegram` holds no Telegram credential at all any more. It
  resolves a gateway binding (`gatewayUrl`/`gatewayToken`) per channel
  instead of a bot token, and issues every Bot API call through the gateway.

A fully leaderless design (any replica derives and serves independently) was
built, tested, and set aside in favor of this simpler one — see `design.md`'s
Context.

## Impact

**Capabilities**: `durable-chat-ops-broker` (MODIFIED — one requirement
renamed from "rejects rather than proxies" to "proxies"), `telegram-ingest-
router` (MODIFIED), `telegram-channel-adapter` (MODIFIED).
`channel-adapter-contract` and `state-durability` are UNCHANGED: the
adapter-facing contract (200/204/503-retry-immediately) and the claim's
crash-recovery semantics are exactly what `durable-chat-ops-broker` already
published.

**Code**: `platform/manager/internal/httpapi/server.go` (`proxyToLeader`),
`platform/manager/internal/httpapi/status.go` (`leaderProxyTarget`),
`channels/telegram/`, `gateways/telegram/`, `chart/charts/telegram/`.

**Reference docs**: `docs/CHANGELOG.md`, `docs/integrations/telegram.md`
(generated), `docs/guides/channel-adapter.md` (generated).

**Adopter site**: none. No new install-time decision, no new chart value.

**Rules**: `.claude/rules/invariants.md` (the "HTTP API is NOT leader-gated"
section's one exception now describes a proxy, not a 503), `.claude/rules/
structure.md` (the Telegram trio's credential ownership), `.claude/rules/
gotchas.md` (one new dated entry, naming the abandoned alternative and why).

This is a manager-only correctness fix plus a Telegram credential
consolidation. No new release is cut as part of this change — chart,
`channel-telegram`, and `gateway-telegram` image versions are bumped but not
tagged or published here, since that is a separate, deliberate release step.
