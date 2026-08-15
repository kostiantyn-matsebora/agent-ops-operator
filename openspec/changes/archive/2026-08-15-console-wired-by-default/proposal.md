## Why

Demo mode installs the console **inert**. It renders the `ChannelAdapter`, the
`Channel` and `SignalSource/console` — and then the only route it ships,
`k8s-observe`, claims `cluster-events` alone and binds no channels. So the
console loads, its composer reports itself unavailable (`Wired=False`), and no
agent answer ever reaches it.

That is a working demo in every part except the one anybody opens first. The flag
whose entire purpose is a turnkey install leaves the most inviting surface in the
product unusable, and the way out — patch the Pipeline to claim the source — is
something a first-time reader has no reason to know exists.

## What Changes

- **A route the chart ships claims the console's source and binds it as a
  channel**, so an install that deploys the console can start a conversation in
  it out of the box. Both are values-supplied NAMES for objects the bundle does
  not render — the same terms `pipelines.channels` is already on — and both are
  omitted when the parent clears them.
- **`global.agentops.console` carries the console's identity** (`signalSource`,
  `channel`). It is the only parent scope a subchart can read, and it is the same
  mechanism `global.agentops.runtime.*` already uses for the substrate.
- **The duplication is guarded, not trusted.** Helm cannot derive one value from
  another, so `global.agentops.console.*` genuinely repeats `console.enabled` /
  `console.signalSourceName` / `console.channelName`. The render **fails** when
  they disagree, in both directions:
  - console enabled but the global names a different source or channel → fail,
    naming the value to set;
  - `console.enabled: false` while the global still names them → fail, because a
    route claiming a source this release does not render is a dangling ref that
    reports `Wired=True` and drops every signal.
- **The getting-started page drops its workaround** — one `--set` and one
  `kubectl patch` disappear from the walkthrough, because the install is now
  correct without them.

Explicitly NOT in this change:

- **No new component and no new default deployment.** The console is already
  default-on; this only wires what is already installed.
- **No change to who may claim what.** Sources stay shareable; this adds one
  claim to a route that already exists, and only where that route renders at all
  (demo mode, or an explicit opt-in to the bundle's wiring).
- **No values migration.** `console.*` stays the user-facing surface.

## Capabilities

### New Capabilities

None.

### Modified Capabilities

- `k8s-bundle`: the bundle's route gains the console source and channel from
  `global.`, and the existing rule about values-supplied names for foreign
  objects now covers both.
- `console-deployment`: the console's identity is published in `global.` for
  subcharts, with a render guard keeping it consistent with `console.*`.

## Impact

- `chart/values.yaml` — new `global.agentops.console` block.
- `chart/templates/console.yaml` — the guard, placed before the enabled gate so
  it also catches the disabled case.
- `chart/charts/k8s-bundle/templates/pipelines.yaml` — the source and channel.
- `chart/Chart.yaml` + `CHANGELOG.md` — a minor bump and an upgrade note: an
  existing install that disabled the console must clear the new globals.
- `docs/getting-started.md`, `docs/k8s-bundle.md`, `docs/console.md`.
- No Go code, no CRDs.
