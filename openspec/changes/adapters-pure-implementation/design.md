# adapters-pure-implementation — design

## Context

User principle, stated after living with the system: **adapters are implementations; Channels/SignalSources are configuration and appliance.** Everything a particular surface needs (connectivity, creds, behavior) lives on the surface CR; the adapter CR only says "run this image". Violations found in audit: `spec.env` on both adapter CRDs (configuration on the implementation), the vm-bundle chart owning webhook connectivity (Service + port value), and `spec.type` duplicating identity that the CR name already provides once implementation↔adapter is 1:1.

## Goals / Non-Goals

**Goals:** adapter CRs describe implementations only; one adapter per implementation by construction; enabling an adapter yields a complete appliance (Service included) with zero chart-side connectivity.
**Non-Goals:** no change to the HTTP contracts' shapes (the `?type=` param name stays — it's the routing-key string, now equal to the adapter CR name for CR-managed adapters); hand-deployed adapters unchanged (they pick their key string freely and need no CR); no change to Channel/SignalSource fields.

## Decisions

- **D1: Name is the key.** `resolve(key)`: in-process registry first (channels), else the adapter CR named `key`. Reconcilers set `CHANNEL_TYPE`/`SOURCE_TYPE` = CR name; token derivation contexts unchanged (already name-based); token scope = the name. `TypeConflict`, `olderClaimant`, and the type printcolumns are deleted — kube name uniqueness IS the guard.
- **D2: No `env`.** Reconciler-injected env is appliance (MANAGER_URL, key, token, LISTEN_ADDR); anything else belongs in the image or the served CRs. The secret-env guard dies with the field.
- **D3: `SignalAdapter.spec.port`** — an implementation declaration ("this image serves HTTP here"). When set: reconciler renders Service `agentops-signal-<name>` (port→port, selector on the deterministic pod label) and injects `LISTEN_ADDR=:<port>`; when unset, no Service. Channel adapters have no inbound surface today — no port field until one does (YAGNI).
- **D4: Bundle ties config to implementation by construction** — vm-bundle's `defaultSource` renders `spec.type: {{ alertmanager.name }}`, so the source always names the adapter the bundle deploys. The `SourceVMAlertmanager` constant is deleted; the adapter binary keeps a default key for hand-deploys only.
- **D5: Live sequencing** rides the already-gated cutover (manager 0.8.0+ not yet deployed pending the VMAlertmanager repoint): at cutover, upgrade to 0.9.0, delete+recreate `vm-alerts` (its `type` is immutable and must become the adapter name), re-apply the pipeline claim, retire the old built-in source.

## Risks / Trade-offs

- [Blue/green adapter replacement by parallel CRs is gone] → explicitly not valued (user); singleton+Recreate already serializes replacements; delete+create has the same brief gap the op queue and sender retries absorb.
- [Immutable `type` on live sources forces recreate when keys change] → one-time cost at cutover for `vm-alerts` only; telegram is already aligned (name==type everywhere).
- [LISTEN_ADDR now reconciler-owned] → image default and CR `port` could disagree only if `port` unset (no Service then — visible immediately).

## Migration Plan

Per D5; everything else is additive-or-delete in one release. Rollback: previous chart/manager.

## Open Questions

- None.
