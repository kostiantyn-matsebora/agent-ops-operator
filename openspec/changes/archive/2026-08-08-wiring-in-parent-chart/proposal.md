# wiring-in-parent-chart

## Why

`k8s-bundle` rendered two Pipelines (the events lane, plus an addressable one)
and `vm-bundle` rendered one for its alert source. `telegram-bundle` rendered
none, deliberately, on the grounds that wiring names a profile and a runtime it
does not own.

`telegram-bundle` was right and the other two were wrong, which a real install
makes obvious: an agent answering cluster events AND Telegram chat needs ONE
Pipeline claiming a source from `k8s-bundle` and a source and channel from
`telegram-bundle`. No subchart can express that — a subchart sees only itself.
So a bundle that ships wiring can only ever wire its own lane, and an install
combining bundles ends up with one Pipeline per lane plus a hand-written one to
join them. Three Pipelines for one agent, all with identical capabilities.

## What Changes

- **No subchart renders a `Pipeline`.** Removed from `k8s-bundle` (both) and
  `vm-bundle` (one). Bundles ship sources, channels, profiles, toolsets and
  configs — the nouns. Wiring is the verb, and it belongs to the install.
- **The parent chart gains a `pipelines:` values list**, the only scope that
  sees every bundle. Each entry names a profile and lists signal sources,
  channels, toolsets and MCP configs by name.
- **Dead values are pruned** with the templates that used them: `profileRef`,
  `channels`, `grantShell`, `addressable` on `k8s-bundle`; `profileRef`,
  `channels`, `extraToolsets`, `extraMCPConfigs` on `vm-bundle`. A values key
  that no longer renders anything is worse than no key.
- **A bundle's source is inert until claimed**, and its values say so rather
  than implying the bundle wires itself.

## Capabilities

### Modified Capabilities

- `pipeline-model`: adds where a chart-managed Pipeline is declared, and the
  naming consequence of a Channel being shareable while a SignalSource is not.
- `k8s-bundle`: no longer renders wiring.
- `vm-bundle`: no longer renders wiring.

## Impact

- **Chart**: new `chart/templates/pipelines.yaml` + `pipelines:` values;
  `k8s-bundle` and `vm-bundle` lose their Pipeline templates and the values that
  fed them.
- **BREAKING for existing installs**: a bundle that used to render a Pipeline no
  longer does, so its source goes `Wired=False` until the install declares the
  route under `pipelines:`. Upgrading without adding it silently stops signal
  handling — the failure is visible on the source, but only if you look.
- **No API or manager change.**
