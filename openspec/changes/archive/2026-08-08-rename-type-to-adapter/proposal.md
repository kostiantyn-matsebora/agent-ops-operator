# Proposal: rename-type-to-adapter

## Why

`Channel.spec.type` and `SignalSource.spec.type` are references: the value is
the NAME of the ChannelAdapter/SignalAdapter CR that serves the object, and
that adapter's schema is what governs the sibling `spec.config`. The word
`type` reads as an intrinsic attribute of the object instead, which makes
`config` look like part of one flat schema that `type` belongs to. It does
not: `type` selects whose schema `config` is written against.

Kubernetes names this slot after the thing that owns it —
`StorageClass.provisioner`, `IngressClass.controller`,
`GatewayClass.controllerName` — each sitting beside its config slot. We keep
the layout (config stays a sibling, inline and opaque, delivered through the
adapter contract) and fix the name.

## What Changes

- **`spec.type` → `spec.adapter`** on `Channel` and `SignalSource` (still
  required, still immutable). Printcolumn `TYPE` → `ADAPTER`.
- **Contract parameter `?type=` → `?adapter=`** on `/channel/ops`,
  `/channel/channels` and `/signal/sources`.
- **Env `CHANNEL_TYPE` / `SOURCE_TYPE` → `ADAPTER_NAME`** injected by both
  reconcilers — one name for both surfaces, since it is the adapter CR's name
  in either case.
- All three reference adapters, chart templates, samples and docs follow.
- No compatibility shim: pre-1.0, and a silently-ignored old field would be
  worse than a loud failure.

## Impact

- API: two CRDs (+ regen). `adapter` is immutable, so existing Channels and
  SignalSources must be recreated — their annotations carry adapter cursor
  state (e.g. the Telegram poll offset) and MUST be preserved across the
  recreate or the adapter re-reads old updates.
- Manager: both adapter reconcilers, both Served reconcilers, the conversation
  reconciler, `chat/ops`, and the `/channel/*` + `/signal/*` handlers.
- Adapters: channel-telegram, signal-cron, signal-vmalertmanager (new images).
- Chart 1.12.0 / manager 0.12.0; live migration on the reference install.
