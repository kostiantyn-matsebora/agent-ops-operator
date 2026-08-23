# Tasks: rename-type-to-adapter

## 1. API

- [x] 1.1 `ChannelSpec.Type` → `Adapter` and `SignalSourceSpec.Type` → `Adapter` (required + immutable CEL preserved); printcolumns `TYPE` → `ADAPTER`; regen deepcopy + CRDs

## 2. Manager

- [x] 2.1 Both adapter reconcilers select served CRs by `spec.adapter` and inject `ADAPTER_NAME`; both Served reconcilers and the conversation reconciler follow; `chat/ops` carries the adapter name
- [x] 2.2 `/channel/ops`, `/channel/channels`, `/signal/sources` take `?adapter=`; the retired `?type=` returns 400 naming the replacement; scope checks compare adapter names
- [x] 2.3 Tests updated + a case pinning the 400-on-retired-parameter behaviour; all suites green

## 3. Adapters

- [x] 3.1 channel-telegram, signal-cron, signal-vmalertmanager read `ADAPTER_NAME` and call `?adapter=`; new multi-arch images

## 4. Chart, samples, docs

- [x] 4.1 vm-bundle + samples render `adapter:`; README/CLAUDE.md state the reference semantics; chart 1.12.0 / manager 0.12.0 built + pushed; commit

## 5. Live (the reference install)

- [x] 5.1 Apply CRDs; capture annotations of `home-ops` and `vm-alerts` (adapter cursor state), delete + recreate with `adapter:` and the saved annotations, re-claim in pipelines; upgrade manager + adapters together; verify Served/Wired, the Telegram offset did not rewind, and an alert still routes
