# Proposal: add-channel-adapter-crd

## Why

Channel adapters are pluggable at the HTTP-contract level, but *plugging* still happens at the deployment layer: the operator's own chart carries a Telegram-specific Deployment template, adapters are wired by hand, and nothing in the cluster declares which type is served by what. Adopters expect Kubernetes-native pluggability: a new channel type is delivered as an artifact (container image implementing the adapter contract) and plugged in by applying a CR — no helm edits, no operator or chart changes.

## What Changes

- New `ChannelAdapter` CRD — **pure implementation**: `type` (immutable routing key) + `image` (+ non-secret tuning: env, resources, singleton). No credentials live here.
- Credentials move to **usage**: `Channel` gains a typed `credentialsSecretRef` (secret *name* only). The ChannelAdapter reconciler *projects* each served Channel's secret into the adapter pod as `valueFrom` env — kubelet resolves; neither the manager nor the adapter ever reads a secret through the API. Enables multi-bot / multi-workspace: one adapter process, N channels with N credentials.
- New `ChannelAdapter` reconciler: owns the adapter Deployment (zero-permission SA, no SA token automount, `replicas: 1 + Recreate` when singleton), injects `MANAGER_URL` and a **per-adapter derived auth token** (HMAC of the master token — stateless, no secret reads for validation), and reports status.
- Visibility: a `Channel` whose `type` has no serving `ChannelAdapter` (and no in-process provider) gets a not-served condition instead of silently queueing ops forever.
- Contract addition: the channel listing tells adapters which env vars hold each channel's projected credentials.
- Chart: the bespoke `telegram-adapter.yaml` Deployment template is replaced by a `ChannelAdapter` CR — the chart no longer contains any channel-type-specific workload.
- Hand-deployed adapters (plain Deployment + shared token) remain fully supported — this is an additive lifecycle layer over the unchanged contract.

## Capabilities

### New Capabilities

- `channel-adapter-lifecycle`: the `ChannelAdapter` CRD and reconciler — adapter deployment management, per-adapter auth, credential projection, served/not-served visibility.

### Modified Capabilities

- `channel-type-model`: `Channel` gains typed `credentialsSecretRef` metadata (credentials are per-surface, not per-implementation).
- `channel-adapter-contract`: per-adapter derived tokens (shared master token stays valid); channel listing carries the credential env mapping.
- `telegram-channel-adapter`: per-channel credentials from projected env (multi-bot; `TELEGRAM_BOT_TOKEN` env kept as fallback); shipped as a `ChannelAdapter` CR by the chart.

## Impact

- `api/v1alpha1/`: new `channeladapter_types.go` + `ChannelSpec.credentialsSecretRef`; regenerated deepcopy/CRDs.
- `internal/controller/`: new `ChannelAdapterReconciler`; Conversation/Channel condition for unserved types.
- `internal/httpapi/`: token derivation/validation; `credentialEnv` in `/channel/channels`.
- `channel-telegram/`: multi-credential poll loops (one getUpdates loop per distinct token).
- `chart/`: `channeladapters.agentops.dev` CRD; telegram Deployment template → `ChannelAdapter` CR; manager RBAC gains deployments + serviceaccounts (create/update/watch, own-namespace).
- Live install (gitops): `home-ops` Channel gains `credentialsSecretRef: agentops-telegram`; adapter workload ownership moves from helm to the reconciler (upgrade must avoid a double-getUpdates window).
