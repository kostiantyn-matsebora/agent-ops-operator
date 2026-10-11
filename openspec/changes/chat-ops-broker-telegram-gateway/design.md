## Context

`durable-chat-ops-broker`'s own design.md named "Horizontal scaling of op
throughput" a Non-Goal, and that is unaffected here too. Exactly one process
still ever claims or completes an op — the leader, unchanged.

What that design actually under-delivered, measured live: `/channel/ops`
still 503'd any non-leader unconditionally, so a channel adapter's
long-poll connection pinned to one by a Kubernetes Service (which picks a
backend per TCP connection, not per request) could sit stuck for hours.

**A fully leaderless alternative was built first, and abandoned.**

It moved `ensure-topic`/`send`/`close-topic` derivation into `internal/chat`
so any replica could compute and serve them from Conversation CR state
directly.

It also generalized the claim to pure staleness, with no leader-identity
check — matching the archived proposal's own words, "a cache of CR state
rather than the sole owner of it."

It compiled and passed every test, including a concurrency race test. It
was set aside anyway, because it is not what was agreed.

The simpler, chosen design keeps the leader as sole broker and fixes only
the routing.

Kept here as a note for whoever next reaches for "every replica derives
independently" on this problem. It works, it was measured working. It is
still the wrong amount of change for this fix.

## Goals / Non-Goals

**Goals:**

- `/channel/ops` answers correctly from ANY replica a poll happens to land
  on, with no dependence on which one.
- The leader-only claim/queue model, already proven by
  `durable-chat-ops-broker`, is untouched.
- `gateway-telegram` becomes the sole holder of the Telegram bot credential.

**Non-Goals:**

- Any change to WHO claims or completes an op. Still exactly the leader.
- Parallel delivery of several ops for one binding at once.
- A new release. This change ships code, not a tag.

## Decisions

### A non-leader is a dumb proxy, not a second decision-maker

**Chosen**: `handleChannelOps`, on `!isLeader`, resolves the current Lease
holder's pod and reverse-proxies the request there with
`httputil.ReverseProxy`, relaying the response verbatim. It falls back to
the existing 503-and-retry only when the leader cannot be resolved or
reached.

**Why over the leaderless alternative**: it is less code, and it touches no
claim semantics `durable-chat-ops-broker` already proved.

It removes the "which replica do I land on" problem AT THE ROUTING LAYER,
instead of making every replica capable of answering it — one hop, always
correct, instead of a probabilistic redial or a second exclusivity
mechanism to reason about.

**Cost, accepted**: one extra network hop on a non-leader's poll. Bounded
and local — pod-to-pod inside the cluster — and only on the fraction of
polls that land on a non-leader in the first place.

### The leader's pod is resolved from the Lease, not a second lookup

**Chosen**: `leaderIdentity()` already reads the Lease holder as
`<hostname>_<uuid>` (controller-runtime's own `NewResourceLock`). Split on
the last `_` to recover the hostname, then list pods labelled
`app.kubernetes.io/name: agentops-manager` and match by `.Name` — Kubernetes
sets a Deployment pod's hostname to its own name by default.

**Why not a dedicated leader-labelled Service**, the other design floated
in chat before this one.

It needs the leader-election code to label and unlabel its own pod on
every transition. It needs a chart-rendered Service. It needs a second env
var on every caller choosing which one to use.

Resolving the pod directly from the Lease already-read needs none of that.
The information was already being fetched.

### Every replica shares one port

**Chosen**: the proxy target's port is this process's OWN `Server.Addr`
(`API_ADDR`, one value, one Deployment, one image) — only the leader's POD
IP differs. No second port lookup, no annotation on the pod.

### `/activity*` reuses the proxy, but refuses nothing

**Found live** while testing the `/channel/ops` fix on a 3-replica
install: the console's "reasoning disappeared" reports traced to
`internal/activity.Log` being a per-process ring buffer.

A hop the reconciler emits only ever lands in the leader's buffer. A
console's SSE connection to `/activity/stream` pins to one replica, exactly
like a `/channel/ops` poll.

**Chosen**: `handleActivity`, `handleActivityStream` and
`handleActivityReport` all call the SAME `proxyToLeader` on `!isLeader`,
with no new helper.

**Where this differs from `/channel/ops`**: there is no correctness reason
to refuse when the leader is unreachable. A locally-served, partial
activity view is never WRONG, only incomplete.

All three fall through to local behavior instead of a 503. `/channel/ops`
cannot do this — serving from an unpopulated local `OpQueue` would be an
actual wrong answer, not merely a partial one.

### The per-endpoint fix generalized into one middleware

**Found live**: fixing `/channel/ops`, then `/activity*`, one endpoint at
a time was itself the pattern to stop. Generalized into
`leaderProxyMiddleware`, wrapping the whole mux rather than one more
per-handler check.

**Chosen**: on `!isLeader`, proxy EVERY route except `/healthz` before it
reaches a handler. A handler reached anyway — the proxy failed, or this
replica is the leader — falls through to its own unchanged local
behavior.

**Why this is safe for routes that never needed a leader check**: most
endpoints read or write the Kubernetes API, consistent regardless of
replica.

Proxying them anyway costs one extra same-node-network hop on the
fraction of requests landing on a non-leader. It changes nothing about
correctness.

### The NetworkPolicy gap that made the middleware a no-op

**Found live**, testing the middleware on this install: the manager's own
`NetworkPolicy` admitted callers by kind — runtime, adapter, signal
adapter, the aops MCP server — but never admitted `agentops-manager`
itself. A non-leader's proxy dial to the leader was DROPPED.

**Chosen**: add `app.kubernetes.io/name: agentops-manager` to the
existing ingress rule's caller list, alongside the others.

**Why this was invisible from the manager's own logs**: `proxyToLeader`'s
`ErrorHandler` swallows the dial failure by design, and the caller falls
back to its own local behavior on a bare `false`.

A NetworkPolicy DROP and a genuinely unreachable leader are
indistinguishable from inside the process that hit either.

## Risks / Trade-offs

- **[Risk]** A non-leader proxying blind could mask a leader that is
  actually unhealthy (accepting TCP but wedged). **Mitigation**: unchanged
  risk profile from before this change. A wedged leader already broke
  `/channel/ops` for everyone when it alone could answer, and this does not
  make that worse. Liveness/readiness probing the leader pod is outside
  this change's scope.
- **[Risk]** The pod-name-from-Lease-identity parse breaks if a future
  `leaderelection` version changes its holder-identity format.
  **Mitigation**: same assumption `isLeader`'s own prefix match already
  depends on, documented there since the bug that originally measured it.
  Not a new fragility this change introduces.
- **[Risk]** `channel-telegram` losing its own bot token is a behavior
  change for a hand-deployed (non-chart) install. **Mitigation**: the
  fallback env vars (`TELEGRAM_GATEWAY_URL`/`_TOKEN`) mirror the old
  `TELEGRAM_BOT_TOKEN` fallback's shape.

## Migration Plan

1. Deploy the new manager image. No CRD change, no schema change — the
   claim fields and their semantics are exactly `durable-chat-ops-broker`'s
   own, untouched.
2. Deploy the new `channel-telegram`/`gateway-telegram` images together
   with the chart update rendering the new gateway-binding Secret. The two
   MUST move together: an old `channel-telegram` expects a bot token no
   longer in the Channel's credential Secret, and a new one expects a
   gateway binding an old chart does not render.
3. No chart version is tagged or published as part of this change.
