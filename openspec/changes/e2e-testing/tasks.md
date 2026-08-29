## 1. Outbound endpoint becomes configuration

- [x] 1.1 `channel-telegram`: resolve the Bot API base from `TELEGRAM_API_BASE`, defaulting to `https://api.telegram.org`, and route every call through it — `sendMessage`, `sendDocument`, `createForumTopic`, `closeForumTopic`
- [x] 1.2 `gateway-telegram`: same resolution for the `getUpdates` poll loop; absent value takes the default silently, unlike the required forwarding targets and bot token
- [x] 1.3 Unit tests in both modules: default when unset, override honoured on every call, and `Channel.spec.config` cannot influence it
- [x] 1.4 `chart/charts/telegram/`: optional `apiBase` value rendering no env entry when unset; assert byte-identical default render in `platform/manager/internal/integration/charttemplate_test.go` — the router gets env, the channel adapter gets an `apiBase` key on the bot Secret (a `ChannelAdapter` carries no `env`)
- [x] 1.5 Add the standing rule to `.claude/rules/invariants.md` — an adapter's outbound base URL is configuration, never a constant

## 2. Fake Bot API server

- [x] 2.1 `test/fakebotapi/`: HTTP server implementing `getUpdates`, `sendMessage`, `sendDocument`, `createForumTopic`, `closeForumTopic`
- [x] 2.2 Scriptable update queue (feed a captured `Update`, serve it once) plus a recorded-call log queryable by tests
- [x] 2.3 Dockerfile + in-cluster Deployment manifest for the e2e pack
- [x] 2.4 Self-test: the fake's own request/response shapes match the captured real ones

## 3. Fixture set

- [x] 3.1 `test/fixtures/`: scrubbed Alertmanager webhook body (firing + resolved)
- [x] 3.2 Scrubbed Telegram `Update` fixtures — general-surface message (origination) and topic message (continuation)
- [x] 3.3 Scrubbing: every identifier in a fixture is a placeholder the publication allowlist permits by name — a new kind is an entry in `.github/publication-allowlist.json` landed first; `python3 .github/scripts/publication-guard.py` passes
- [x] 3.4 Point `signal-alertmanager` and `signal-telegram` unit tests at the canonical fixtures by relative path (test-only read, no `go.mod` change)

## 4. Contract conformance suite

- [x] 4.1 `platform/manager/test/conformance/`: fake manager serving `/channel/*` and `/signal/inbound`, extracted from the inline server in `platform/console/adapter_test.go`
- [x] 4.2 Binary runner — build an adapter, start it with contract env, wait for readiness, tear down; adapters are listed, never imported
- [x] 4.3 Channel set: long-poll, `contract=` declaration and refusal without it, typed-message handling, ack-once under duplicate delivery, inbound push with `threadId`, channel listing, config error as status not crash
- [x] 4.4 Channel set: no relay loop — an outbound post never returns as inbound
- [x] 4.5 Signal set: normalized emission, bearer auth, source scoping, rejected post retried or surfaced rather than dropped
- [x] 4.6 Signal set: a chat-originating adapter always carries the channel label
- [x] 4.7 Run `channel-telegram`, `console`, `signal-cron`, `signal-alertmanager`, `signal-telegram`, `signal-k8s-events`, `signal-ha` through the suite; fix what it finds — found and fixed: `channel-telegram` acted on a redelivered op id twice (now a bounded completed-id set); `signal-k8s-events` and `signal-ha` dropped a rejected post silently (now reported on the source condition as `PostFailed`, cleared on recovery); the console and `signal-k8s-events` gained a `KUBERNETES_SERVICEACCOUNT_DIR` override so the built binary can be driven against a fake API server
- [x] 4.8 Verify no module's `go.mod` but `platform/manager/` changed, over the list `.github/components.sh modules` prints

## 5. Stub runtime

- [x] 5.1 `test/stubruntime/`: `/work` long-poll, `/work/done` report, idle-TTL exit — conforming, with no manager-side special case
- [x] 5.2 Scripted vocabulary keyed on input text: `echo`, `fail`, `stale-context`, `no-context`, `die`, `stall`, `storage-outage`
- [x] 5.3 Determinism check: identical input yields identical reported result
- [x] 5.4 Dockerfile; name it so its presence in a real deployment is obviously wrong
- [x] 5.5 Assert no chart default, sample CR or documented install path references it
- [x] 5.6 Exclude `test/` in `.github/components.sh` and assert `components.sh images` lists neither the stub nor the fake Bot API — the union of Dockerfile-bearing directories would otherwise publish them

## 6. Cluster harness

- [x] 6.1 `platform/manager/test/e2e/` behind a build tag; typed clients over the project's own API types
- [x] 6.2 Cluster lifecycle: k3d create with traefik and servicelb disabled, teardown, and a reuse flag for local iteration
- [x] 6.3 Build-and-import path for all images; chart install from the working tree; readiness wait for manager and every enabled adapter
- [x] 6.4 Unconditional failure diagnostics — manager and adapter logs, pod list, cluster events, full YAML of every agentops CR — emitted as a workflow artifact
- [x] 6.5 Wall-clock budget assertion for the gating tier, so growth is visible rather than gradual
- [x] 6.6 Egress mediation stays at its default (on); the pack does not disable the `NET_ADMIN` init container to simplify the cluster

## 7. Substrate assertions (stub runtime, no secret)

- [x] 7.1 Credential projection: `Channel.credentialsSecretRef` reaches the adapter pod as `AGENTOPS_CRED_<CHANNEL>_*`, kubelet-resolved
- [x] 7.2 RBAC as enforced via `SubjectAccessReview` — manager denied all `secrets` verbs; adapter SA denied everything the chart did not grant
- [x] 7.3 Informer liveness for every reconciled kind, so a miscased `resources:` entry fails instead of looping forbidden
- [x] 7.4 Context continuity across a runtime pod restart under `contextStorage: volume` with the context volume
- [x] 7.5 Admission FIFO: saturate the cap with pod-backed conversations, delete a pod, assert the oldest `Pending` is promoted — the slots are held by STALLING conversations (an idle pod is evicted the moment a waiter exists) and the cron lane is paused for the test (its older conversation would out-rank the waiters, which is FIFO too)
- [x] 7.6 Local authenticated registry: one dedicated image-pull test with `imagePullSecrets` (imports do not exercise pulls) — a `registry:2` on the cluster network with a per-run htpasswd, the pull Secret on the floor ServiceAccount, the cluster created with a `registries.yaml` naming it plain-HTTP; the pushed image is a DERIVED one with its own id, because the kubelet remembers an id pulled with credentials and forces every later credential-less pod on that id to re-pull
- [x] 7.7 Mechanism paths against the stub — `stale-context` fails the run, `no-context` does not, `die` clears inflight, `stall` evicts, `storage-outage` holds work — `stall` is exercised as the FIFO test's slot holder; the breaker's HOLD is asserted and that subtest runs LAST, because what is NOT asserted is that the breaker closes again: it closes only on a CONTINUED run, and the one canary the manager let through never reached a pod in two runs — an open question left to the manager, stated in the test rather than hidden behind a longer wait

## 8. Signal loop breaker

- [x] 8.1 Pipeline pointed at a runtime image that cannot start; `signal-k8s-events` watching the namespace
- [x] 8.2 Assert bounded Conversation count **and** bounded creation rate across the window, so a slow leak fails
- [x] 8.3 Cold-cache variant: restart the adapter while failing agentops pods emit Warning events; assert the name-prefix mechanism holds

## 9. Lifecycle through the console

- [x] 9.1 Start a conversation via `POST /api/conversations`, continue via `POST /api/conversations/{name}/messages`, assert delivery to the bound thread
- [x] 9.2 Assert the console write path refuses an unauthenticated request rather than bypassing the console
- [x] 9.3 `/close` sets phase `Closed` and archives every bound thread at the transition; the object survives; delete is the second verb and refuses anything not `Closed`
- [x] 9.4 Multi-channel fan-out: console plus the Telegram lane bound to one conversation, both threads receive the answer — FOUND AND FIXED a manager lost-update: `markDelivered`, `markThreadArchived` and `finishEnsureTopic` patched `status` arrays with a plain merge patch (no resourceVersion), so two channels completing within milliseconds overwrote each other — one thread binding or one delivery mark was lost for good; the patches are optimistically locked now and the retry loops fire. Also `send:<seq>` notice ids restarted at 1 with the manager and collided across restarts against the adapters' ack-once dedup; they carry a per-process epoch now

## 10. Inbound lanes end to end

- [x] 10.1 Alertmanager fixture POSTed to `/webhook/{source}` opens a conversation through its claiming Pipeline; no VictoriaMetrics or Alertmanager deployed
- [x] 10.2 `signal-cron` schedule fires and originates
- [x] 10.3 `signal-k8s-events`: create a genuinely failing workload, assert the event becomes a signal and opens a conversation
- [x] 10.4 Telegram lane against the fake — router classifies and forwards verbatim, ingest and channel adapters handle it, outbound send observed as a recorded call
- [x] 10.5 Assert exactly one `getUpdates` consumer exists against the fake
- [x] 10.6 An unclaimed source drops its signal with `Wired=False`, and a chat source's reason reaches the surface the person typed on

## 11. Real-runtime lane

- [x] 11.1 Continuity by nonce: state a random identifier, restart the runtime pod, ask for it back, assert the exact token appears
- [x] 11.2 Closed-form correctness question with a test-known answer
- [x] 11.3 Toolset enforcement asserted on **effect**: ask for a mutation outside the bound toolset, read the cluster to confirm it did not happen; then a bound read succeeds, so the first is evidence of a boundary
- [x] 11.4 Bounded retries on every agent-dependent assertion, with the retry count reported
- [x] 11.5 Fixed, small conversation count; fail fast on a broken deployment rather than burning turns — the lane is implemented and skips itself without `ANTHROPIC_API_KEY`; it has NOT been run against a real credential yet: that is the repository secret and the first dispatched full run

## 12. Workflows

- [x] 12.1 `.github/workflows/e2e.yml`: PR tier — conformance plus the stub-runtime cluster smoke, no secret required; the gating jobs join `ci-green`'s `needs:` in `ci.yml`, never a new required check — `e2e.yml` is a reusable workflow that `ci.yml` CALLS with `tier: pr`, which is what makes its jobs listable in `needs:`
- [x] 12.2 Full tier on `workflow_dispatch` (and a schedule once cadence is settled), including the real-runtime lane — TWO entry-point workflows, `e2e-smoke.yml` (pr tier) and `e2e-full.yml` (full tier), each nightly and on demand, both calling the one definition in `e2e.yml`
- [x] 12.3 Fork pull requests report skipped tiers as skipped and are never failed for unavailable secrets
- [x] 12.4 The token-consuming tier gates no pull request
- [x] 12.5 Document the boundary with `continuous-integration` in both specs so "what CI runs" has one definition per tier

## 13. Resolve open questions

- [x] 13.1 Cadence for the real-runtime lane — dispatch-only to start, or a schedule: NIGHTLY (04:17 UTC in `e2e-full.yml`, the smoke an hour earlier in `e2e-smoke.yml`) and on demand — decided by the maintainer after the first review; each cron lives in its own file and `docs/testing.md` names it
- [x] 13.2 Whether the e2e profile pins a smaller model, trading spend against exercising the shipped default: NO PIN — the lane exists to exercise the shipped default, and spend is bounded by a fixed conversation count and bounded retries instead
- [x] 13.3 Credentials for the local-registry pull test — per-run throwaway htpasswd: YES, minted by the test into a `registry:2` container on the cluster network; the pull Secret rides on the floor ServiceAccount, the kubelet's own mechanism, never the GHCR token

## 14. Documentation

Reference docs:

- [x] 14.1 New `docs/testing.md` owning the tier model, the oracle split, and how to run each tier locally; one line in `docs/_data/nav.yml`
- [x] 14.2 `docs/contracts.md`: the conformance suite as the stated way an adapter proves the contract
- [x] 14.3 `.claude/rules/build-test.md`: e2e and conformance entry points; `.claude/rules/structure.md`: the `test/` tree and its exclusion from `components.sh`
- [x] 14.4 Record the manual repository setup — the API-key secret and the cadence decision — as prerequisites, not as code
- [x] 14.5 `CONTRIBUTING.md`: the test tiers a pull request meets and how to run them before pushing — its own row in the routing table, neither half

Adopter site:

- [x] 14.6 `docs/integrations/telegram.md`: the `apiBase` value on the telegram bundle — a SUBCHART value belongs to its integration page, never `docs/installation.md`; and confirm the landing page, Introduction, Getting started and Installation say nothing this change makes untrue
- [x] 14.7 Confirm no `docs/CHANGELOG.md` entry is needed (nothing ships), and `python3 .github/scripts/docs-generate.py --check` passes — things DO ship now (the manager's locked status patches and epoch-unique notice ids, the adapters' ack-once and post-failure reporting, `TELEGRAM_API_BASE`, the `apiBase` value) but none breaks an install or needs an upgrade step, which is what the changelog carries; the generator check passes
