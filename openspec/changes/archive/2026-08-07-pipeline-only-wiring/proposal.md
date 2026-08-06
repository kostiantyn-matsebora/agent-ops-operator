# Proposal: pipeline-only-wiring

## Why

With `Pipeline` landed as the wiring object, the old per-CR references are now duplicated wiring: `SignalSource.spec.channelRef`/`profileRef` and `Channel.spec.defaultProfileRef` say the same thing a Pipeline says, with precedence rules to arbitrate. Two places to declare one fact is exactly what the Pipeline was built to end — wiring should exist in exactly one place.

## What Changes

- **BREAKING** — Remove the fallback wiring fields: `SignalSource.spec.channelRef`, `SignalSource.spec.profileRef`, and `Channel.spec.defaultProfileRef`. `Pipeline` is the only way sources reach profiles/channels and the only source of a channel's default profile.
- Routing consequences, made visible instead of silent:
  - signals for a source not claimed by a Ready Pipeline are **dropped with an explicit reason** in the ingest/inbound response, and the source carries a `Wired` condition (False until a Ready Pipeline claims it);
  - bare (non-command) messages on a channel in no Pipeline get the existing "no default profile" warning; `/profile task` commands work on any channel as before.
- Live migration ships in the same release: the pipeline CR is applied **before** the manager upgrade so alert routing never gaps.

## Capabilities

### New Capabilities

<!-- none -->

### Modified Capabilities

- `signal-source-model`: routing metadata loses `channelRef`/`profileRef`; unwired sources drop signals visibly (`Wired` condition).
- `signal-adapter-contract`: inbound routing requires a Ready Pipeline claim; the unclaimed-source fallback scenario is removed.
- `channel-type-model`: Channel metadata loses `defaultProfileRef`; the pipeline supplies default profiles.
- `pipeline-model`: resolution is pipeline-only (the source-level fallback requirement is replaced).

## Impact

- `api/v1alpha1/{signalsource,channel}_types.go` + regen; router default-profile resolution; `routeSignals` requires a pipeline; `SignalSourceReconciler` gains the `Wired` condition; samples lose the removed fields (the Pipeline sample already carries the wiring).
- Tests: helpers lose profile/default-profile params; signal-routing tests create Pipelines.
- Chart 1.4.0 / manager 0.7.0; live install: apply `home-ops-pipeline` first, then upgrade; clean the removed fields from live CR manifests.
