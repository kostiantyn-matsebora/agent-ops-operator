# Tasks: adapters-pure-implementation

## 1. API — implementation-only adapter CRDs

- [x] 1.1 Remove `Type` (+ CEL) and `Env` from `ChannelAdapterSpec` and `SignalAdapterSpec`; add `Port *int32` to `SignalAdapterSpec` (implementation property); printcolumns drop Type; delete `SourceVMAlertmanager`; regen deepcopy + CRDs

## 2. Manager — name is the key

- [x] 2.1 Both adapter reconcilers: key = CR name (served lookups, `CHANNEL_TYPE`/`SOURCE_TYPE` env); delete `olderClaimant`/`TypeConflict`/`secretShapedEnv` and the env append; SignalAdapter reconciler owns Service `agentops-signal-<name>` + `LISTEN_ADDR` when `port` set (GC on delete/unset)
- [x] 2.2 httpapi auth: derived-token scope = adapter name (both surfaces); Channel/SignalSource Served lookups match adapter NAME == spec.type
- [x] 2.3 Tests: drop conflict/secret-env cases; scoping tests key on names; new Service-rendering assertions (port set/unset); all suites green

## 3. Chart, samples, docs

- [x] 3.1 vm-bundle: SignalAdapter renders `port: 8080`, no `type`; delete the chart Service template + `service.port` value; defaultSource `spec.type: {{ alertmanager.name }}`; NOTES.txt fixed port; telegram-adapter.yaml and samples drop `type:` lines (names already align)
- [x] 3.2 README + CLAUDE.md: name-as-key model, no-config-on-implementations invariant, Service-from-reconciler; chart 1.7.0 / manager 0.9.0 built + pushed; commit

## 4. Live (rides the gated cutover with 0.8.0's)

- [ ] 4.1 At cutover (after VMAlertmanager repoint): upgrade to 0.9.0, recreate `vm-alerts` with `type: vm-alertmanager` (immutable field) + re-claim in its Pipeline, verify reconciler-owned Service replaces the chart one, retire the old built-in source + `am-stub`
