# router-not-an-adapter — design

## Context

`chat-signal-origination` split Telegram ingest into three components and made
all three adapter CRs. Two of them are adapters in the real sense: they receive
pushed updates and post to `/signal/inbound` or `/channel/inbound`. The third,
`telegram-router`, emits nothing — it reads the one update stream a bot token
allows and forwards each update to whichever of the two should handle it.

Making it a `SignalAdapter` bought the reconciler's workload machinery and
credential projection. The bill arrived as a chain: adapter CRs carry no config
or credentials by invariant, so the router needed a served CR to carry them; the
only servable kind was `SignalSource`; a `SignalSource` nobody claims reports
`Wired=False` forever; so installs added a claim to a Pipeline that can never
receive a signal from it. The `telegram-ingest-router` spec even records the
premise — "a served CR's `config` is therefore the only path per-deployment
settings can travel" — which is true only while the router must be an adapter.

## Goals / Non-Goals

**Goals:**

- The router stops pretending to be an adapter, and the CRs that existed only to
  support that pretence go away.
- Configuration reaches it the direct way: the chart that renders its two
  targets injects their URLs.
- "Exactly one `getUpdates` per bot token" becomes structural rather than
  bookkeeping.

**Non-Goals:**

- Not re-opening the three-component split. The router stays its own component
  and its own container; only its *modelling* changes.
- Not changing the two real adapters, the contract, `spec.port`, credential
  projection, or anything manager-side.
- Not changing offset ownership: the router still delegates storage to
  `channel-telegram`.

## Decisions

### D1: The chart owns the router Deployment

`telegram-bundle` renders it directly — replicas 1, `Recreate`,
`automountServiceAccountToken: false`. This bends "charts ship no adapter
connectivity, reconcilers own adapter workloads", and the bend is the point:
that rule governs adapters, and the whole claim here is that the router is not
one. A chart-owned Deployment is the honest shape, not an exception to a rule.

### D2: Configuration is env, because the chart already knows it

```
SIGNAL_TARGET       http://agentops-signal-<name>.<ns>.svc:<port>
CHANNEL_TARGET      http://agentops-adapter-<name>.<ns>.svc:<port>
TELEGRAM_BOT_TOKEN  from the same Secret the Channel sends with
```

Both URLs are deterministic Service names for adapters this same chart renders.
Fetching them back out of the manager over an authenticated contract, to learn
what the renderer knew all along, was the most circular part of the old shape.
The router consequently drops `MANAGER_URL`, `ADAPTER_TOKEN` and its contract
client entirely, leaving the adapter trust domain.

### D3: One Deployment per bot token

The old router served N sources and kept one poll loop per distinct token, with
a "leader" election among sources sharing one. That machinery existed to make
the single-consumer rule hold across a set of CRs. With one Deployment per
surface the rule is a property of the deployment topology instead, and the
loop-bookkeeping, the leader selection and the token-keyed maps all delete.

**This is a real capability loss**: one router can no longer poll several bots.
It costs nothing today because `telegram-bundle` supports a single surface, and
if surfaces go plural the chart renders one router each — which is the safer
shape anyway, since two tokens can never end up sharing a process.

### D4: Config errors become a failed startup

The old router reported per-source problems as the `SignalSource`'s Ready
condition. Losing the CR loses that surface, so `loadConfig` fails fast: a
missing value logs what is missing and exits, and the pod crash-loops with the
reason in `kubectl describe`.

This is the one thing traded away. It is defensible — a crash-looping pod is
harder to miss than a condition on a CR nobody lists — but it is a downgrade for
anyone who was reading Ready conditions, and it is recorded as a choice rather
than an oversight.

## Risks / Trade-offs

- [Two pollers during the cutover] → The old Deployment is owned by its
  `SignalAdapter` CR, so deleting the CR garbage-collects it. Delete the CR and
  WAIT for the pod to disappear before applying the chart; overlapping pollers
  produce 409s and stolen updates.
- [The bundle now ships a workload the operator's reconcilers do not manage] →
  True, and it means one more thing the chart must keep correct (image, probes,
  strategy). Accepted: it is an ordinary Deployment with three env vars.
- [`spec.port` on `ChannelAdapter` was added for the router's benefit] → It
  stays useful and stays specified; the router still pushes to that port. Only
  the router's own CR goes.

## Migration Plan

1. Delete the `telegram-router` `SignalAdapter`; its Deployment is GC'd.
2. Confirm no router pod remains — this is the step that prevents double-polling.
3. Apply the chart; the new Deployment starts and resumes from the persisted
   offset, which lives in `channel-telegram` and is untouched by any of this.
4. Remove `telegram-router` from any Pipeline's `signalSources`.

Rollback is the reverse, and is safe for the same reason: offset storage never
moved.
