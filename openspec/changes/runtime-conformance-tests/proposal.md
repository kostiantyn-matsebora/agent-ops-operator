## Why

`contract-conformance-suite` already catches a channel or signal adapter that
drifts from its contract. It builds the real binary and drives it against a
fake manager, gating every pull request through `ci-green`.

`runtime-contract` carries the same obligation in prose only
(`.claude/rules/runtime-spec.md`: "a spec change carries conformance... in
the SAME change"). A human reviewer checks it, not CI. An edit to
`runtimes/ollama` that breaks allowlist composition, bounds trimming, or the
idle-TTL exit today ships green.

## What Changes

- `FakeManager` (`platform/manager/test/conformance/fakemanager.go`) gains
  `GET /work` and `POST /work/done` handlers, mirroring the `/channel/ops` +
  `/channel/ops/{id}/done` pair already there.
- A new `platform/manager/test/conformance/runtime_test.go` builds the real
  `runtimes/ollama` binary and runs it as a subprocess against `FakeManager`,
  with a small fake Ollama HTTP double standing in for the vendor call
  (`OLLAMA_URL`), scripted by a directive word in the task text — the same
  convention `test/stubruntime` already uses, scoped narrowly to what these
  assertions need rather than cloned in full.
- Asserts `docs/contracts/runtime.md` IDs `API-1,2,3,4,5,7,8,9`, `LC-1,2,3`,
  `RT-1`, `RT-2`, `RT-3`/`3.1`–`3.4`, `SEC-1,2,3`, `COMPAT-1` behaviorally, the same
  way `channel_test.go`/`signal_test.go` assert their contract IDs. `API-6`
  and every `MGR-n` row are the manager's own obligations and stay owned by
  envtest/system — not asserted here.
- Runs inside the existing `conformance` job (`go test -tags conformance
  ./test/conformance/`) — no new CI job, no new toolchain. `runtimes/` is
  already a path-filtered component group, so the job already triggers on a
  runtime change. Only what it asserts grows.
- **Not in this change**: `runtimes/claude` and `runtimes/copilot`.
  `runtimes/claude` spawns its vendor CLI via a `CLAUDE_BIN`/`resolveBin`
  seam a fake binary could exploit today, but the in-progress
  `claude-runtime-sdk` change (0/41 tasks) is replacing that spawn with the
  Claude Agent SDK driven in-process, removing the seam this change would
  have keyed off. `runtimes/copilot`'s own seam is unconfirmed. Both are
  recorded as a follow-on in `design.md`, not as a task here.

## Capabilities

### New Capabilities

(none)

### Modified Capabilities

- `contract-conformance-suite`: adds a runtime conformance requirement
  (scoped to `runtimes/ollama`), parallel to the existing channel and signal
  adapter requirements.
- `continuous-integration`: the conformance job's description moves from
  "every adapter's built binary" to naming runtimes too — no scenario or
  trigger changes, since `runtimes/` already falls under the existing
  path-filtered component group.

## Impact

- Code: `platform/manager/test/conformance/fakemanager.go`,
  `platform/manager/test/conformance/runtime_test.go` (new), a new fake
  Ollama HTTP double alongside the existing `test/fakebotapi` (either a new
  `test/fakeollama` or an in-package double — a design.md decision).
- CI: no workflow file changes — the existing `conformance` job in
  `.github/workflows/ci.yml` picks this up as-is.
- Reference docs: `docs/testing.md`'s "Contract conformance" tier row changes
  from "that an adapter speaks the adapter contracts" to naming runtimes
  too. `docs/contracts/runtime.md` is NOT edited — no spec version bump, no
  obligation changes, only an automated check added for an obligation that
  already existed.
- Adopter site: none — this is contributor-facing test infrastructure, no
  page under `docs/` that an adopter reads describes it.
