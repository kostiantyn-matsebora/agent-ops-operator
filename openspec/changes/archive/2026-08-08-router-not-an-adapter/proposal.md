# router-not-an-adapter

## Why

`telegram-router` is modelled as a `SignalAdapter`, but it produces no signals.
It is the Telegram integration's internal plumbing: one poll loop that
classifies each update and forwards it to whichever of the two real adapters
should handle it.

It was made an adapter CR to borrow the reconciler's workload machinery and
credential projection. That borrowing then forced a chain of consequences, each
reasonable given the last and none of them wanted:

- a `SignalSource` that emits nothing, existing only as a credential envelope —
  because credentials and config reach an adapter pod ONLY by projection from a
  CR it serves, and adapter CRs carry neither by invariant
- that source sitting at `Wired=False` forever unless some Pipeline claims it,
  so an install must add a fake claim to a route that will never receive a
  signal from it — wiring that documents a lie
- an `ADAPTER_TOKEN` and a manager contract round-trip whose entire purpose is
  fetching two in-cluster Service URLs the chart already knows, because the
  chart renders both of those adapters itself

Nothing here is a bug. Every step follows from treating plumbing as a
first-class adapter, which it is not.

## What Changes

- **The router stops being an adapter CR.** `telegram-bundle` owns its
  `Deployment` directly, the way a chart owns any workload that is part of how
  its component works.
- **Its configuration becomes env**, injected by the chart: `SIGNAL_TARGET` and
  `CHANNEL_TARGET` (both deterministic Service names the bundle renders), and
  the bot token via `envFrom` on the same Secret the `Channel` uses.
- **The `telegram-router` `SignalAdapter` and its `SignalSource` are removed**,
  and with them the `Wired=False` wart and the fake Pipeline claim.
- **The router no longer contacts the manager at all** — no `MANAGER_URL`, no
  `ADAPTER_TOKEN`, no contract client. It leaves the adapter trust domain
  entirely, which is the honest description of a process that only forwards
  bytes between two in-cluster Services.
- **One router Deployment per surface** instead of one router serving N sources.
  The "exactly one `getUpdates` per bot token" invariant stops being internal
  loop bookkeeping guarded by `singleton` and becomes structural: one
  Deployment, one token.
- Config errors surface as a **failed startup** (crash-looping pod) rather than
  a Ready condition on a CR — see design D4, which is the one place this trades
  something away.

## Capabilities

### Modified Capabilities

- `telegram-ingest-router`: the router ceases to be an adapter. The requirement
  permitting it to read its own served `SignalSource` is removed along with the
  premise that made it necessary; the bundle requirement gains the router
  Deployment and loses the router's two CRs.

## Impact

- **`telegram-router/`**: delete `manager.go` and the contract client; replace
  `refreshSources` with env parsing at startup. Net less code.
- **`chart/charts/telegram-bundle/`**: a `Deployment` template for the router;
  drop its `SignalAdapter` and the credential-carrying `SignalSource`.
- **Deployment configs**: `signalSources` entries naming `telegram-router` must
  be dropped from the claiming Pipeline — including this repo's own
  gitops config, where that claim exists only to silence
  `Wired=False`.
- **Docs**: CLAUDE.md's router entry, README's Telegram section, and the
  `telegram-ingest-router` spec.
- **No API or manager change.** `SignalAdapter`, `spec.port`, credential
  projection and the contract are all untouched — one consumer stops using them.
- **Migration**: replacing the adapter CR with a chart Deployment means the old
  reconciler-owned Deployment is garbage-collected when its CR goes. Ordering
  matters only in that two pollers must never overlap (see tasks).
