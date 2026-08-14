# Proposal: end-to-end testing on a real single-node cluster

## Why

The test estate stops exactly where the kubelet starts. `internal/integration/`
runs 22 files against a real API server, but envtest schedules nothing — its own
suite file documents pods from earlier tests sitting Pending forever. Every
invariant whose enforcement belongs to the kubelet, the scheduler, or a live
informer is therefore **asserted nowhere**: that the manager reads no Secrets
because `envFrom` is kubelet-resolved, that adapter RBAC actually denies what it
does not grant, that `resources:` typo'd to `AgentRuntimes` breaks the informer
silently, that `contextStorage: volume` survives a pod restart, that images pull
at all — and that `signal-k8s-events`' self-exclusion stops the loop where a
broken runtime pod emits an event that opens a conversation that creates another
broken runtime pod, forever. That last one has three independent mechanisms
guarding it and no test that can run them, because reproducing it requires a
real pod that really fails.

The obstacle was assumed to be the third parties. It is not. Auditing egress
across every module: only `telegram-router`, `channel-telegram`, the planned
`signal-ha`, and `runtime-claude` call *out*. Every adapter the question was
asked about — vmalertmanager, telegram ingest, k8s-events, cron — is
**inbound**: it hosts a port and waits. Driving it needs a POST, not a fake. And
`console/` is already a `ChannelAdapter` with no third-party dependency at all,
so the full conversation lifecycle runs through production code with nothing
simulated.

## What Changes

- **A `test/e2e/` pack running against k3s** (k3d in CI, in the root Go module so
  it inherits the existing client-go dependency and adds no ninth module). It
  installs the chart from the working tree, waits for readiness, and asserts
  against the live cluster. Its subject is the **substrate**: `envFrom`
  credential projection under prefix `AGENTOPS_CRED_<CHANNEL>_`, RBAC as
  enforced (SubjectAccessReview, not the rendered Role), informer liveness for
  every reconciled kind, PVC-backed context continuity across a pod restart,
  image pulls including `imagePullSecrets`, admission FIFO driven by real pod
  DELETEs, and the signal loop breaker under a runtime image that cannot start.
- **Inbound adapters are driven by captured fixtures, not by their upstreams.**
  `test/fixtures/` holds a scrubbed Alertmanager webhook body, a scrubbed
  Telegram `Update`, and the k8s Events are produced by creating genuinely
  failing workloads. No VictoriaMetrics, no Alertmanager, no Home Assistant is
  deployed. The fixtures double as unit-test inputs in their own modules.
- **The console is the e2e channel.** Conversations are started and continued
  through `POST /api/conversations` and `POST /api/conversations/{name}/messages`
  — the exact path a human takes minus the browser — which exercises the
  console's auth gate, its write path, the `/channel/*` contract, and delivery
  fan-out, with zero test doubles anywhere in the flow.
- **Real Claude is the primary oracle.** The runtime under test is
  `runtime-claude` with a repo-secret token. It answers the questions only an
  agent can: did context survive a gap, is the answer right, does the bound
  toolset actually constrain it (ask for a tool the Pipeline did not bind,
  assert refusal — the only end-to-end proof that capabilities are wiring).
  Assertions are **tolerant** (is the fact present) rather than exact-prose, with
  a bounded retry, because a suite that goes red on phrasing teaches its readers
  to ignore red. Fork PRs cannot read repo secrets; that is the intended access
  gate, not a limitation to work around.
- **A small, fixed stub-runtime set for what an agent cannot be asked to do.**
  A new `test/stubruntime/` image speaks the `/work` contract and is driven by a
  scripted input vocabulary — return a stale `runtimeContextId`, return none,
  exit mid-run, stall past the idle TTL, fail the run. These are manager and
  runtime *mechanisms* (latest-wins handles, promised-and-lost fails the run, the
  storage breaker holds work, FIFO promotion), and no prompt makes a working
  agent exhibit them on cue. The stub is an instrument, not a cost workaround.
- **A fake Bot API server** (`test/fakebotapi/`) implementing `getUpdates`,
  `sendMessage`, `sendDocument`, `createForumTopic`, `closeForumTopic`. Because
  `telegram-router` forwards updates **verbatim**, a replayed `Update` is
  byte-identical to what Telegram would have produced, which is what makes the
  double faithful rather than approximate.
- **The outbound Bot API base URL becomes configuration.** `channel-telegram`
  and `telegram-router` hardcode `https://api.telegram.org` while parameterizing
  the *manager* URL in the same files. Both gain `TELEGRAM_API_BASE`, defaulting
  to the real host, surfaced as an optional `telegram-bundle` value that renders
  nothing when unset. The standing rule this establishes — **an adapter's
  outbound base URL is configuration, never a constant** — costs nothing
  anywhere else and is a prerequisite `signal-ha` will need regardless, since
  Home Assistant is self-hosted by definition.
- **A contract conformance suite, needing no cluster and no Go import.** The
  fake manager already built inline in `console/adapter_test.go` becomes a
  harness in the root module that runs each adapter as a **built binary** —
  black-box, over HTTP — and asserts long-poll, ack idempotency, inbound push,
  channel listing, status reporting, and the `contract=` refusal. Driving the
  binary rather than importing a package is not a compromise: importing a shared
  package would put a dependency outside their own directory into every adapter
  module, which the project forbids, and the black-box form tests the artifact
  that actually ships. A new adapter joins by being listed, not by importing.
  This half has no dependency on k3s or on `sdlc-setup` and can land first.
- **Two CI tiers.** Pull requests run conformance plus a thin k3s smoke on the
  stub runtime; a nightly and manually-dispatchable job runs the full pack
  including the real-Claude lane. Fork PRs get the tiers their secret access
  allows and are not failed for the rest.

Not in scope: an eval harness scoring agent answer *quality* (that is a
different project with a different cost model), a browser-driven console test,
deploying any third-party system under test, load or soak testing, and
`signal-ha` (it does not exist yet — this change only fixes the rule that would
have made it untestable).

## Capabilities

### New Capabilities
- `end-to-end-testing`: the k3s pack — how the cluster and the system under test
  are provisioned, the tier boundaries and what gates a PR, the oracle split
  between the real runtime and the stub, fixture-driven inbound adapters, the
  console as the e2e channel, the fake Bot API, and the substrate assertions
  that exist because envtest structurally cannot make them.
- `test-stub-runtime`: the stub runtime image — its `/work` conformance, the
  scripted input vocabulary, and which mechanism each scripted behavior guards.
- `contract-conformance-suite`: the shared fake-manager harness — what an
  adapter must demonstrate to be conformant, and how a module runs it.

### Modified Capabilities
- `telegram-channel-adapter`: the Bot API endpoint becomes configuration
  (`TELEGRAM_API_BASE`, defaulting to `https://api.telegram.org`) rather than a
  compiled-in constant.
- `telegram-ingest-router`: the same, for the single `getUpdates` consumer.

## Impact

- **New**: `test/e2e/` (root module), `test/stubruntime/` (+ Dockerfile),
  `test/fakebotapi/`, `test/fixtures/`, `test/conformance/` (root module, drives
  adapter binaries black-box), and `.github/workflows/e2e.yml`. **No adapter
  module gains a dependency** — the eight-module boundary is preserved.
- **Go changes are two lines of substance**: the Bot API base URL in
  `channel-telegram/telegram.go` and `telegram-router/telegram.go`. **No CRD,
  API, or manager change** — the pack asserts existing behavior.
- **Chart**: `telegram-bundle` gains an optional `apiBase`; the stub runtime is
  selected through the existing `runtime.image` value, so no new template logic.
- **`CLAUDE.md`**: the outbound-base-URL rule joins the invariants, and the
  build/test section gains the e2e entry points.
- **Docs**: a testing page owning the tier model and how to run the pack
  locally; `CHANGELOG.md` gets no entry (no behavior ships).
- **Sequencing — depends on `sdlc-setup`, which is 0/38 tasks done.** The k3s
  tiers need its workflows and its published images to install a chart at all.
  The conformance half does not, and should land ahead of it. Where
  `sdlc-setup`'s `continuous-integration` capability owns per-module build/vet/
  test and chart lint, `end-to-end-testing` owns the e2e jobs only; the boundary
  is stated in both so the two specs do not both claim to define what CI runs.
- **Repo settings (manual, outside the diff)**: an `ANTHROPIC_API_KEY` repo
  secret, and a decision on whether the nightly real-Claude lane runs on a
  schedule or only on dispatch. Its spend is real and recurring.
- **Cost of being wrong here is asymmetric**: the loop breaker's failure mode is
  unbounded etcd growth that no downstream cap arrests, so a slow, expensive
  test that covers it is still cheaper than the incident.
