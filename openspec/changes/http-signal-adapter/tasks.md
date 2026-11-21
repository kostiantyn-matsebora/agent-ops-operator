## 1. Adapter module

- [ ] 1.1 Create `signals/http/` (own `go.mod`, stdlib only) with the server on `LISTEN_ADDR`, `POST /request/{source}`, source listing and push to `/signal/inbound`.
- [ ] 1.2 Mandatory per-source bearer check, constant time, header only.
- [ ] 1.3 Template engine with `header` and `body`, constants, and unresolved handling per field.
- [ ] 1.4 Output validation: `kind`, `severity` (with value map), `signalId`, `title`, `labels` (32 keys, reserved prefix), request cap with `413`.
- [ ] 1.5 Load-time config validation reporting through `POST /signal/sources/{name}/status`, isolated per source.

## 1b. Channel adapter module

- [ ] 1b.1 Create `channels/http/` (own `go.mod`, stdlib only) with the ops loop on `GET /channel/ops`, the ack and the done or failed report, per `docs/contracts/channel-adapter.md`.
- [ ] 1b.2 Outbound delivery: POST the typed message as JSON to the Channel's target URL with the Channel's bearer. Map 2xx to done and anything else to failed, with a bounded timeout.
- [ ] 1b.3 `POST /reply/{channel}`: per-channel bearer in the header only, `threadId` and text required, forwarded to `/channel/inbound`, `404` for an unknown channel.
- [ ] 1b.4 Config validation of the Channel's `config` (target URL present and `http` or `https`), reported without affecting another Channel.

## 2. Chart and component inventory

- [ ] 2.1 Add `chart/charts/http/` as an embedded subchart: a `Chart.yaml`, templates self-gated on `http.enabled`, and a `values.yaml` reading only `global.`. Render the `SignalAdapter` and the `ChannelAdapter`, each with its `spec.port` where it serves one and the floor service account.
- [ ] 2.2 Register the dependency in the parent `chart/Chart.yaml` with `condition: http.enabled`, add the documented `http:` block (`enabled: false`) to `chart/values.yaml`, and bump the chart version per `chart.md`.
- [ ] 2.3 Confirm the image publishes multi-arch and the component appears in the inventory.

## 3. Unit tests

- [ ] 3.1 `go build ./... && go vet ./...` pass in `signals/http`, and `.github/components.sh images` lists `signal-http` and `adapter-http`.
- [ ] 3.2 Unit tests for the bearer check: wrong, absent, other-source and query-string tokens.
- [ ] 3.3 Table-driven tests covering every row of the default and unresolved table.
- [ ] 3.4 Tests for each output rule: `kind` (including `chat`), `severity` (including `fatal`), `signalId`, `title`, `labels` and the request cap.
- [ ] 3.5 Tests for load-time validation: a bad template, a constant `chat`, a missing token, and a bad source beside a good one.
- [ ] 3.6 The chart's render tests pass for default, enabled and demo, and `helm dependency` and `helm template` succeed with the bundle off and on.
- [ ] 3.7 The structure assertions pass with the new component in the inventory.
- [ ] 3.8 `go test -count=1 -coverpkg=./... ./...` in `signals/http` is green with every spec scenario covered.
- [ ] 3.9 Run the contract conformance suite against the adapter if one exists for signal adapters, and verify it passes.
- [ ] 3.10 Unit tests for `channels/http`: each delivery outcome (2xx, 4xx, 5xx, timeout), no re-send after the ack, reply auth (wrong, absent, other-channel, query-string), a missing `threadId`, an unknown channel and a bad target URL beside a good Channel.
- [ ] 3.11 Run the channel adapter conformance suite against `channels/http`, as `channel-spec.md` requires, and verify it passes.
- [ ] 3.12 `python3 .github/scripts/publication-guard.py` and `python3 .github/scripts/retired-vocabulary-guard.py` report a passing verdict.

## 4. System tests

- [ ] 4.1 Add a lane in `platform/manager/test/system/` that posts to `/request/<source>` through the Service, with the token from the projected Secret, and asserts a conversation opens and a second request with an equal templated id recurs onto it. 
- [ ] 4.2 Add a lane delivering an agent reply through `channels/http` to a receiver in the cluster, then posting `/reply/<channel>` and asserting it lands on the conversation's thread. Run the pack.

## 5. Documentation

- [ ] 5.1 Reference docs: add `docs/integrations/http.md` (config, templates, worked examples, the message definition, the `kind` consequences, the loop warning, plus a channel section covering target URL, reply endpoint, delivery outcomes and the SSRF note), rows (one per module) in `.claude/rules/structure.md`, a `docs/CHANGELOG.md` entry, and `docs/contracts/signal-adapter.md` shipped-adapter enumerations and `docs/contracts/channel-adapter.md` shipped-adapter enumerations only after the user's explicit approval for each (`signal-rules.md`, `channel-spec.md`).
- [ ] 5.2 Adopter site: the site's integrations navigation and Getting started or guides entry for the new adapter. Re-run `python3 .github/scripts/docs-generate.py` and verify `--check` passes.
