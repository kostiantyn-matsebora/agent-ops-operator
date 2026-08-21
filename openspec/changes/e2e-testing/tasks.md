## 1. Outbound endpoint becomes configuration

- [ ] 1.1 `channel-telegram`: resolve the Bot API base from `TELEGRAM_API_BASE`, defaulting to `https://api.telegram.org`, and route every call through it — `sendMessage`, `sendDocument`, `createForumTopic`, `closeForumTopic`
- [ ] 1.2 `telegram-router`: same resolution for the `getUpdates` poll loop; absent value takes the default silently, unlike the required forwarding targets and bot token
- [ ] 1.3 Unit tests in both modules: default when unset, override honoured on every call, and `Channel.spec.config` cannot influence it
- [ ] 1.4 `telegram-bundle`: optional `apiBase` value rendering no env entry when unset; assert byte-identical default render in `internal/integration/charttemplate_test.go`
- [ ] 1.5 Add the standing rule to `CLAUDE.md` invariants — an adapter's outbound base URL is configuration, never a constant

## 2. Fake Bot API server

- [ ] 2.1 `test/fakebotapi/`: HTTP server implementing `getUpdates`, `sendMessage`, `sendDocument`, `createForumTopic`, `closeForumTopic`
- [ ] 2.2 Scriptable update queue (feed a captured `Update`, serve it once) plus a recorded-call log queryable by tests
- [ ] 2.3 Dockerfile + in-cluster Deployment manifest for the e2e pack
- [ ] 2.4 Self-test: the fake's own request/response shapes match the captured real ones

## 3. Fixture set

- [ ] 3.1 `test/fixtures/`: scrubbed Alertmanager webhook body (firing + resolved)
- [ ] 3.2 Scrubbed Telegram `Update` fixtures — general-surface message (origination) and topic message (continuation)
- [ ] 3.3 Scrubbing check: no real chat ids, user ids, usernames, hostnames or tokens in any committed fixture
- [ ] 3.4 Point `signal-alertmanager` and `signal-telegram` unit tests at the canonical fixtures by relative path (test-only read, no `go.mod` change)

## 4. Contract conformance suite

- [ ] 4.1 `test/conformance/`: fake manager serving `/channel/*` and `/signal/inbound`, extracted from the inline server in `console/adapter_test.go`
- [ ] 4.2 Binary runner — build an adapter, start it with contract env, wait for readiness, tear down; adapters are listed, never imported
- [ ] 4.3 Channel set: long-poll, `contract=` declaration and refusal without it, typed-message handling, ack-once under duplicate delivery, inbound push with `threadId`, channel listing, config error as status not crash
- [ ] 4.4 Channel set: no relay loop — an outbound post never returns as inbound
- [ ] 4.5 Signal set: normalized emission, bearer auth, source scoping, rejected post retried or surfaced rather than dropped
- [ ] 4.6 Signal set: a chat-originating adapter always carries the channel label
- [ ] 4.7 Run `channel-telegram`, `console`, `signal-cron`, `signal-alertmanager`, `signal-telegram`, `signal-k8s-events` through the suite; fix what it finds
- [ ] 4.8 Verify no adapter module's `go.mod` changed

## 5. Stub runtime

- [ ] 5.1 `test/stubruntime/`: `/work` long-poll, `/work/done` report, idle-TTL exit — conforming, with no manager-side special case
- [ ] 5.2 Scripted vocabulary keyed on input text: `echo`, `fail`, `stale-context`, `no-context`, `die`, `stall`, `storage-outage`
- [ ] 5.3 Determinism check: identical input yields identical reported result
- [ ] 5.4 Dockerfile; name it so its presence in a real deployment is obviously wrong
- [ ] 5.5 Assert no chart default, sample CR or documented install path references it

## 6. Cluster harness

- [ ] 6.1 `test/e2e/` in the root module behind a build tag; typed clients over the project's own API types
- [ ] 6.2 Cluster lifecycle: k3d create with traefik and servicelb disabled, teardown, and a reuse flag for local iteration
- [ ] 6.3 Build-and-import path for all images; chart install from the working tree; readiness wait for manager and every enabled adapter
- [ ] 6.4 Unconditional failure diagnostics — manager and adapter logs, pod list, cluster events, full YAML of every agentops CR — emitted as a workflow artifact
- [ ] 6.5 Wall-clock budget assertion for the gating tier, so growth is visible rather than gradual

## 7. Substrate assertions (stub runtime, no secret)

- [ ] 7.1 Credential projection: `Channel.credentialsSecretRef` reaches the adapter pod as `AGENTOPS_CRED_<CHANNEL>_*`, kubelet-resolved
- [ ] 7.2 RBAC as enforced via `SubjectAccessReview` — manager denied all `secrets` verbs; adapter SA denied everything the chart did not grant
- [ ] 7.3 Informer liveness for every reconciled kind, so a miscased `resources:` entry fails instead of looping forbidden
- [ ] 7.4 Context continuity across a runtime pod restart under `contextStorage: volume` with a home PVC
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
- [ ] 9.3 `/close` deletes the Conversation and the finalizer archives threads first
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

## 12. Workflows (after `sdlc-setup`)

- [ ] 12.1 `.github/workflows/e2e.yml`: PR tier — conformance plus the stub-runtime cluster smoke, no secret required
- [ ] 12.2 Full tier on `workflow_dispatch` (and a schedule once cadence is settled), including the real-runtime lane
- [ ] 12.3 Fork pull requests report skipped tiers as skipped and are never failed for unavailable secrets
- [ ] 12.4 The token-consuming tier gates no pull request
- [ ] 12.5 Document the boundary with `continuous-integration` in both specs so "what CI runs" has one definition per tier

## 13. Documentation

- [ ] 13.1 New docs page owning the tier model, the oracle split, and how to run each tier locally
- [ ] 13.2 `CLAUDE.md`: e2e entry points in the build/test section; the `test/` tree in the Map
- [ ] 13.3 Record the manual repository setup — the API-key secret and the cadence decision — as prerequisites, not as code
- [ ] 13.4 Confirm no `CHANGELOG.md` entry is needed (nothing ships)

## 14. Resolve open questions

- [ ] 14.1 Cadence for the real-runtime lane — dispatch-only to start, or a schedule
- [ ] 14.2 Whether the e2e profile pins a smaller model, trading spend against exercising the shipped default
- [ ] 14.3 Credentials for the local-registry pull test — per-run throwaway htpasswd
