# Tasks: add-channel-adapter-crd

## 1. API types

- [x] 1.1 Add `channeladapter_types.go` (`type` immutable CEL, `image`, `env`, `singleton` default true, `resources`; status conditions + servedChannels) and `CredentialsSecretRef *corev1.LocalObjectReference` on `ChannelSpec`
- [x] 1.2 Regenerate deepcopy + CRDs into `chart/files/crds/`; update `config/samples/` with a ChannelAdapter + credentialed Channel example

## 2. ChannelAdapter reconciler

- [x] 2.1 Deployment ownership: render/own the adapter Deployment (MANAGER_URL, derived token, dedicated zero-RBAC SA with `automountServiceAccountToken: false`, singleton ⇒ replicas 1 + Recreate); GC on delete
- [x] 2.2 Credential projection: envFrom prefix `AGENTOPS_CRED_<CHANNEL>_` (resolution of design open question — per-key `valueFrom` would need Secret key enumeration, i.e. a Secret read) for every served Channel's `credentialsSecretRef`; watch Channels of the type; collision detection → condition; reject secretKeyRef in `spec.env`
- [x] 2.3 Type-conflict guard (one active adapter per type; `TypeConflict` condition on the newer) and status (Deployed, Ready from contract reports, servedChannels)
- [x] 2.4 `Served` condition on Channels: False when type has no Registry entry and no Ready ChannelAdapter
- [x] 2.5 Envtest: CR → Deployment shape (SA, token env, projection, singleton), credential-change rollout, conflict guard, GC (ownerRef), Served condition transitions

## 3. Contract + auth

- [x] 3.1 Per-adapter derived tokens: HMAC(masterKey, adapter name); manager validates by re-derivation, scopes to the adapter's `spec.type` (403 cross-type); master token keeps full scope; unit tests incl. restart-statelessness
- [x] 3.2 `credentialEnvPrefix` in `GET /channel/channels` (from projection metadata, no Secret reads); integration test
- [x] 3.3 Secret-key enumeration source for the mapping — RESOLVED: no enumeration at all; projection is `envFrom` with a deterministic prefix (kubelet injects every key as `<prefix><key>`), the listing advertises `credentialEnvPrefix`, adapters resolve `prefix + key` themselves

## 4. Telegram adapter multi-credential

- [x] 4.1 Resolve per-channel token via `credentialEnvPrefix` + `botToken` with `TELEGRAM_BOT_TOKEN` fallback; not-ready condition when neither present
- [x] 4.2 One getUpdates loop per distinct token (channels sharing a token share a loop); offset cursor written to the group leader, read as max across the group
- [x] 4.3 Bump adapter image to 0.2.0; build + push (also manager 0.4.0 — chart 1.1.0 needs the new reconciler)

## 5. Chart

- [x] 5.1 Delete `telegram-adapter.yaml` Deployment template; render a `ChannelAdapter` CR from the existing `telegramAdapter.*` values (enabled gate unchanged); add the new CRD file
- [x] 5.2 Manager RBAC: add deployments + serviceaccounts (create/update/patch/watch, own namespace); chart 1.1.0; `helm template`/lint across default, enabled, and existingSecret value sets

## 6. Verification, docs, live migration

- [x] 6.1 `go build/vet` both modules; full envtest suite green
- [x] 6.2 Live migration on home-data-center per design: upgrade with adapter disabled (helm removes old Deployment) → add `credentialsSecretRef` to `home-ops` → enable the ChannelAdapter CR → verified single consumer, projection (adapter Ready with no fallback token env), and a stub round-trip (`task-vdxgt`)
- [x] 6.3 README (ChannelAdapter concept, "publish your own adapter" = image + CR, credential model, chart-1.1 migration) + CLAUDE.md (map, invariants: credentials on Channel, projection, derived tokens)
