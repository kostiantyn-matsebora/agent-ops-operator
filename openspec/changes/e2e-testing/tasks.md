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

- [ ] 4.1 `platform/manager/test/conformance/`: fake manager serving `/channel/*` and `/signal/inbound`, extracted from the inline server in `platform/console/adapter_test.go`
- [ ] 4.2 Binary runner — build an adapter, start it with contract env, wait for readiness, tear down; adapters are listed, never imported
- [ ] 4.3 Channel set: long-poll, `contract=` declaration and refusal without it, typed-message handling, ack-once under duplicate delivery, inbound push with `threadId`, channel listing, config error as status not crash
- [ ] 4.4 Channel set: no relay loop — an outbound post never returns as inbound
- [ ] 4.5 Signal set: normalized emission, bearer auth, source scoping, rejected post retried or surfaced rather than dropped
- [ ] 4.6 Signal set: a chat-originating adapter always carries the channel label
- [ ] 4.7 Run `channel-telegram`, `console`, `signal-cron`, `signal-alertmanager`, `signal-telegram`, `signal-k8s-events`, `signal-ha` through the suite; fix what it finds
- [ ] 4.8 Verify no module's `go.mod` but `platform/manager/` changed, over the list `.github/components.sh modules` prints

## 5. Stub runtime

- [x] 5.1 `test/stubruntime/`: `/work` long-poll, `/work/done` report, idle-TTL exit — conforming, with no manager-side special case
- [x] 5.2 Scripted vocabulary keyed on input text: `echo`, `fail`, `stale-context`, `no-context`, `die`, `stall`, `storage-outage`
- [x] 5.3 Determinism check: identical input yields identical reported result
- [x] 5.4 Dockerfile; name it so its presence in a real deployment is obviously wrong
- [x] 5.5 Assert no chart default, sample CR or documented install path references it
- [x] 5.6 Exclude `test/` in `.github/components.sh` and assert `components.sh images` lists neither the stub nor the fake Bot API — the union of Dockerfile-bearing directories would otherwise publish them

## 6. Cluster harness

- [ ] 6.1 `platform/manager/test/e2e/` behind a build tag; typed clients over the project's own API types
- [ ] 6.2 Cluster lifecycle: k3d create with traefik and servicelb disabled, teardown, and a reuse flag for local iteration
- [ ] 6.3 Build-and-import path for all images; chart install from the working tree; readiness wait for manager and every enabled adapter
- [ ] 6.4 Unconditional failure diagnostics — manager and adapter logs, pod list, cluster events, full YAML of every agentops CR — emitted as a workflow artifact
- [ ] 6.5 Wall-clock budget assertion for the gating tier, so growth is visible rather than gradual
- [ ] 6.6 Egress mediation stays at its default (on); the pack does not disable the `NET_ADMIN` init container to simplify the cluster

## 7. Substrate assertions (stub runtime, no secret)

- [ ] 7.1 Credential projection: `Channel.credentialsSecretRef` reaches the adapter pod as `AGENTOPS_CRED_<CHANNEL>_*`, kubelet-resolved
- [ ] 7.2 RBAC as enforced via `SubjectAccessReview` — manager denied all `secrets` verbs; adapter SA denied everything the chart did not grant
- [ ] 7.3 Informer liveness for every reconciled kind, so a miscased `resources:` entry fails instead of looping forbidden
- [ ] 7.4 Context continuity across a runtime pod restart under `contextStorage: volume` with the context volume
- [ ] 7.5 Admission FIFO: saturate the cap with pod-backed conversations, delete a pod, assert the oldest `Pending` is promoted
- [ ] 7.6 Local authenticated registry: one dedicated image-pull test with `imagePullSecrets` (imports do not exercise pulls)
- [ ] 7.7 Mechanism paths against the stub — `stale-context` fails the run, `no-context` does not, `die` clears inflight, `stall` evicts, `storage-outage` holds work

## 8. Signal loop breaker

- [ ] 8.1 Pipeline pointed at a runtime image that cannot start; `signal-k8s-events` watching the namespace
- [ ] 8.2 Assert bounded Conversation count **and** bounded creation rate across the window, so a slow leak fails
- [ ] 8.3 Cold-cache variant: restart the adapter while failing agentops pods emit Warning events; assert the name-prefix mechanism holds

## 9. Lifecycle through the console

- [ ] 9.1 Start a conversation via `POST /api/conversations`, continue via `POST /api/conversations/{name}/messages`, assert delivery to the bound thread
- [ ] 9.2 Assert the console write path refuses an unauthenticated request rather than bypassing the console
- [ ] 9.3 `/close` sets phase `Closed` and archives every bound thread at the transition; the object survives; delete is the second verb and refuses anything not `Closed`
- [ ] 9.4 Multi-channel fan-out: console plus the Telegram lane bound to one conversation, both threads receive the answer

## 10. Inbound lanes end to end

- [ ] 10.1 Alertmanager fixture POSTed to `/webhook/{source}` opens a conversation through its claiming Pipeline; no VictoriaMetrics or Alertmanager deployed
- [ ] 10.2 `signal-cron` schedule fires and originates
- [ ] 10.3 `signal-k8s-events`: create a genuinely failing workload, assert the event becomes a signal and opens a conversation
- [ ] 10.4 Telegram lane against the fake — router classifies and forwards verbatim, ingest and channel adapters handle it, outbound send observed as a recorded call
- [ ] 10.5 Assert exactly one `getUpdates` consumer exists against the fake
- [ ] 10.6 An unclaimed source drops its signal with `Wired=False`, and a chat source's reason reaches the surface the person typed on

## 11. Real-runtime lane

- [ ] 11.1 Continuity by nonce: state a random identifier, restart the runtime pod, ask for it back, assert the exact token appears
- [ ] 11.2 Closed-form correctness question with a test-known answer
- [ ] 11.3 Toolset enforcement asserted on **effect**: ask for a mutation outside the bound toolset, read the cluster to confirm it did not happen; then a bound read succeeds, so the first is evidence of a boundary
- [ ] 11.4 Bounded retries on every agent-dependent assertion, with the retry count reported
- [ ] 11.5 Fixed, small conversation count; fail fast on a broken deployment rather than burning turns

## 12. Workflows

- [ ] 12.1 `.github/workflows/e2e.yml`: PR tier — conformance plus the stub-runtime cluster smoke, no secret required; the gating jobs join `ci-green`'s `needs:` in `ci.yml`, never a new required check
- [ ] 12.2 Full tier on `workflow_dispatch` (and a schedule once cadence is settled), including the real-runtime lane
- [ ] 12.3 Fork pull requests report skipped tiers as skipped and are never failed for unavailable secrets
- [ ] 12.4 The token-consuming tier gates no pull request
- [ ] 12.5 Document the boundary with `continuous-integration` in both specs so "what CI runs" has one definition per tier

## 13. Resolve open questions

- [ ] 13.1 Cadence for the real-runtime lane — dispatch-only to start, or a schedule
- [ ] 13.2 Whether the e2e profile pins a smaller model, trading spend against exercising the shipped default
- [ ] 13.3 Credentials for the local-registry pull test — per-run throwaway htpasswd

## 14. Documentation

Reference docs:

- [ ] 14.1 New `docs/testing.md` owning the tier model, the oracle split, and how to run each tier locally; one line in `docs/_data/nav.yml`
- [ ] 14.2 `docs/contracts.md`: the conformance suite as the stated way an adapter proves the contract
- [ ] 14.3 `.claude/rules/build-test.md`: e2e and conformance entry points; `.claude/rules/structure.md`: the `test/` tree and its exclusion from `components.sh`
- [ ] 14.4 Record the manual repository setup — the API-key secret and the cadence decision — as prerequisites, not as code
- [ ] 14.5 `CONTRIBUTING.md`: the test tiers a pull request meets and how to run them before pushing — its own row in the routing table, neither half

Adopter site:

- [ ] 14.6 `docs/integrations/telegram.md`: the `apiBase` value on the telegram bundle — a SUBCHART value belongs to its integration page, never `docs/installation.md`; and confirm the landing page, Introduction, Getting started and Installation say nothing this change makes untrue
- [ ] 14.7 Confirm no `docs/CHANGELOG.md` entry is needed (nothing ships), and `python3 .github/scripts/docs-generate.py --check` passes
