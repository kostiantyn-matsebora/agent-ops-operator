## 1. Adapter module

- [ ] 1.1 Create `signals/http/` (own `go.mod`, stdlib only) with the server on `LISTEN_ADDR`, `POST /request/{source}`, source listing and push to `/signal/inbound`. Verify `go build ./... && go vet ./...` and `.github/components.sh images` lists `signal-http`.
- [ ] 1.2 Mandatory per-source bearer check, constant time, header only. Verify unit tests for wrong, absent, other-source and query-string tokens.
- [ ] 1.3 Template engine with `header` and `body`, constants, and unresolved handling per field. Verify table-driven tests covering every row of the default and unresolved table.
- [ ] 1.4 Output validation: `kind`, `severity` (with value map), `signalId`, `title`, `labels` (32 keys, reserved prefix), request cap with `413`. Verify tests for each rule, including `chat` and `fatal`.
- [ ] 1.5 Load-time config validation reporting through `POST /signal/sources/{name}/status`, isolated per source. Verify tests for a bad template, a constant `chat`, a missing token and a bad source beside a good one.

## 2. Chart and component inventory

- [ ] 2.1 Add `chart/charts/http/` rendering the `SignalAdapter` with `spec.port`, default off, not enabled by demo mode. Verify the chart render tests for default, enabled and demo.
- [ ] 2.2 Confirm the image publishes multi-arch and the component appears in the inventory. Verify `.github/components.sh images` and the structure assertions pass.

## 3. Unit tests

- [ ] 3.1 `go test -count=1 -coverpkg=./... ./...` in `signals/http` is green with every spec scenario covered.
- [ ] 3.2 The chart's render tests for the new bundle pass.
- [ ] 3.3 Run the contract conformance suite against the adapter if one exists for signal adapters, and verify it passes.
- [ ] 3.4 `python3 .github/scripts/publication-guard.py` and `python3 .github/scripts/retired-vocabulary-guard.py` report a passing verdict.

## 4. System tests

- [ ] 4.1 Add a lane in `platform/manager/test/system/` that posts to `/request/<source>` through the Service, with the token from the projected Secret, and asserts a conversation opens and a second request with an equal templated id recurs onto it. Run the pack.

## 5. Documentation

- [ ] 5.1 Reference docs: add `docs/integrations/http.md` (config, templates, worked examples, the message definition, the `kind` consequences, the loop warning), a row in `.claude/rules/structure.md`, a `docs/CHANGELOG.md` entry, and `docs/contracts/signal-adapter.md` shipped-adapter enumerations only after the user's explicit approval (`signal-rules.md`).
- [ ] 5.2 Adopter site: the site's integrations navigation and Getting started or guides entry for the new adapter. Re-run `python3 .github/scripts/docs-generate.py` and verify `--check` passes.
