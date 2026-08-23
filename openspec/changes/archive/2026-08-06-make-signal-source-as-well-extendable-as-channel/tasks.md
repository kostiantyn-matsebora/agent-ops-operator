# Tasks: make-signal-source-as-well-extendable-as-channel

## 1. API types

- [x] 1.1 Restructure `signalsource_types.go`: `type` open string (drop enum, add MinLength + CEL immutability), add opaque `config` (RawExtension + preserve-unknown-fields) and `credentialsSecretRef`; delete `CronSpec`/`EventsSpec`; keep `grouping`/`channelRef`/`profileRef` as-is
- [x] 1.2 Add `signaladapter_types.go` mirroring `ChannelAdapter` (type immutable, image, env, singleton default true, resources; conditions + servedSources)
- [x] 1.3 Regenerate deepcopy + CRDs; update `config/samples/` (cron SignalAdapter + cron SignalSource with config; keep the alertmanager sample unchanged as the compat proof)

## 2. Normalized-signal core

- [x] 2.1 Extract `routeGroup` into a normalized-signal entry point (`internal/httpapi/signals.go`): `{fingerprint, labels, title?, payload, kind}` × per-source grouping policy → cooldown, signature, window reuse, recurrence; `kind: job` → `InputJob`; `title` override; `receivedTotal`/`lastReceived` bookkeeping in one place
- [x] 2.2 Refactor `handleAlertmanager` to parse AM payloads into normalized signals and feed the shared core (combine closure preserves the exact `{receivedAt, alerts}` payload shape); alert-grouping integration test passes unchanged
- [x] 2.3 Tests for the core (envtest, the project's idiom for client-coupled code): job-lane routing, title override, cooldown absorption of duplicate fingerprints

## 3. Signal contract endpoints + auth

- [x] 3.1 Add `/signal/inbound`, `/signal/sources?type=`, `/signal/state/{source}/{key}` (GET/PUT, annotation-backed), `/signal/sources/{name}/status` to `internal/httpapi`
- [x] 3.2 Auth: signal-surface middleware accepting master token or SignalAdapter tokens derived with context `signal-adapter:<name>` (`chat.DeriveSignalAdapterToken`), type-scoped (403 cross-type), rejecting channel-adapter tokens (401)
- [x] 3.3 Integration tests (envtest): inbound → conversation with grouping/recurrence, job-kind routing, source listing with `credentialEnvPrefix`, state round-trip, status reporting, 401/403 matrix incl. cross-surface tokens

## 4. SignalAdapter reconciler

- [x] 4.1 Extract shared workload-rendering helper (`internal/controller/adapterworkload.go`: SA, automount, envFrom projection, singleton, collision detection, shared condition consts) — channeladapter envtests pass unchanged
- [x] 4.2 Implement `SignalAdapterReconciler` on the helper (SOURCE_TYPE env, SignalSource credential projection, signal token context, `agentops-signal-<name>` workload names) + `SignalSourceReconciler` for the `Served` condition (built-in `alertmanagerWebhook` always served)
- [x] 4.3 Wire both into `cmd/manager/main.go`; envtests: SignalAdapter → Deployment shape, conflict guard, Served transitions incl. built-in type

## 5. Reference cron adapter

- [x] 5.1 Scaffold `signal-cron/` module: contract client, five-field cron parser (table-tested), per-source scheduler with `last-fire` state, `kind: job` signals with `<source>@<tick>` fingerprints, invalid-config status reporting
- [x] 5.2 Dockerfile + image `agentops-signal-cron:0.1.0`; built + pushed
- [x] 5.3 LIVE smoke test on the reference install: minute-schedule stub source fired on schedule (18:40 tick → `job-58fnj`, stub run succeeded); pod restart did NOT re-fire (cursor via state API); second tick (18:41) landed in the SAME conversation as a recurrence; CR deletion GC'd the workload; smoke CRs cleaned up

## 6. Chart, verification, docs

- [x] 6.1 Chart: `signaladapters` CRD file lands via regen; manager RBAC + `signaladapters`(+status); chart 1.2.0 (appVersion/manager 0.5.0, built + pushed); helm lint/template green
- [x] 6.2 `go build/vet` all three modules; full envtest suite green; live upgrade on the reference install verified: `alertmanager` SignalSource untouched (generation 1), `Served=True(InProcessProvider)`, webhook URL unchanged, telegram adapter unaffected
- [x] 6.3 README (SignalSource/SignalAdapter rows, signal adapter contract section, cron reference) + CLAUDE.md (terminology, modules, map incl. `signal-cron/`, build line)
