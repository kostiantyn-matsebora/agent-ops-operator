## 1. Non-leader proxies `/channel/ops` to the leader — backend-developer

- [x] 1.1 Add `Server.leaderProxyTarget` (`internal/httpapi/status.go`): resolve the Lease holder's pod (hostname split from `<hostname>_<uuid>`, matched against pods labelled `app.kubernetes.io/name: agentops-manager`) and return `"<podIP><thisProcess'sOwnAddr>"`.
- [x] 1.2 Add `Server.proxyToLeader` (`internal/httpapi/server.go`): `httputil.ReverseProxy` to that target, relaying the response verbatim. Replace the unconditional 503 in `handleChannelOps` with "proxy, then fall back to 503 only if the proxy itself could not reach anything."
- [x] 1.3 Leave the claim and `OpQueue` exactly as `durable-chat-ops-broker` built them — leader-identity gated, no change. Verify: every existing claim test in `internal/controller`/`internal/integration` passes unmodified.
- [x] 1.4 Verify the proxy actually works end to end: `TestChannelOpsProxiesToLeaderWhenReachable` (`internal/integration/claim_test.go`) — two real `httptest.Server`s with SEPARATE `OpQueue`s, a real envtest Pod carrying the leader's IP, and a real Lease. Polling the FOLLOWER returns the LEADER's own op, and the follower's own queue is never touched.
- [x] 1.5 Verify the fallback still holds: `TestChannelOpsRejectsNonLeader` passes unmodified — with no reachable pod behind the Lease holder's identity, the proxy finds nothing and falls back to the same 503 it always returned.
- [x] 1.6 Found live while testing 1.4's fix on a 3-replica install: `/activity`, `/activity/stream` and `POST /activity` have the SAME connection-pinning exposure for a different reason (`internal/activity.Log` is a per-process ring buffer, and a reconciler-emitted hop only ever lands in the leader's own buffer). Reuse `proxyToLeader` unmodified in all three handlers (`internal/httpapi/activity.go`), falling back to serving/recording locally rather than a 503 when the leader is unreachable — unlike `/channel/ops`, a partial local view is never wrong here. Verify: `TestActivityProxiesToLeaderWhenReachable` (`internal/integration/claim_test.go`), proving a non-leader relays an event only the leader's log holds, and the follower's own log stays empty.

## 2. Gateway becomes the sole Telegram credential holder — backend-developer

- [x] 2.1 Add `gateways/telegram/call.go`: `POST /telegram/call/{method}`, a verbatim relay (body, `Content-Type`, status, response body) to the real Bot API, gated by a bearer token (`TELEGRAM_CALL_TOKEN`). Verify: `call_test.go`.
- [x] 2.2 Remove `Token`/`apiBase()`-to-`api.telegram.org` from `channels/telegram/telegram.go`. Every outbound call (`API`, `SendDocument`) now posts to `<gatewayURL>/telegram/call/<method>` with a bearer header. Verify: `gatewaycall_test.go` replaces `apibase_test.go`.
- [x] 2.3 Resolve the gateway binding per channel from `<prefix>gatewayUrl`/`<prefix>gatewayToken`, falling back to `TELEGRAM_GATEWAY_URL`/`TELEGRAM_GATEWAY_TOKEN`, using the existing `credentialEnvPrefix` projection mechanism (`channels/telegram/main.go`) — the only path available, since `ChannelAdapter` carries no general `env` field. Verify: existing credential-resolution tests updated to the new keys.
- [x] 2.4 Render `chart/charts/telegram/templates/call-auth.yaml`: Secret `agentops-telegram-gateway` (`gatewayUrl` + chart-generated `gatewayToken`), install-only generation guarded by `agentops.generatedSecretGuard`. Point the Channel's `credentialsSecretRef` at it instead of the bot Secret. Inject `TELEGRAM_CALL_TOKEN`/`LISTEN_ADDR` on the router. Verify: `helm lint`, `helm template` across surface on/off and apiBase-override permutations.
- [x] 2.5 Update `openspec/specs/telegram-ingest-router/spec.md` and `openspec/specs/telegram-channel-adapter/spec.md` via this change's delta specs (never edited directly — see `.claude/rules/documentation.md` on when published specs change).

## 3. Unit tests

- [x] 3.1 `platform/manager`: `go build ./... && go vet ./... && KUBEBUILDER_ASSETS=... go test ./...`. Every package green, including `internal/integration` (envtest) and the new proxy test.
- [x] 3.2 `channels/telegram`: `go build ./... && go vet ./... && go test ./...`. 112 subtests green.
- [x] 3.3 `gateways/telegram`: `go build ./... && go vet ./... && go test ./...`. 35 subtests green.
- [x] 3.4 Chart render tests: `TestTelegramAPIBaseRendersOnlyWhenSet` updated for the new Secret shape (the router's own env, not a Channel-side key) and the `call-auth.yaml` Secret's per-render random token excluded from the byte-identity comparison it broke. Green.

## 4. System tests

- [x] 4.1 `platform/manager/test/conformance/channel_test.go`'s `TestChannelTelegramConformance` rewritten to start a REAL `gateway-telegram` binary (`tgStartGateway`) between the fake Bot API and `channel-telegram`, proving the full relay chain end to end against built binaries — not mocks. The send-token assertion now checks the Bot API saw the GATEWAY's credential, proving consolidation rather than per-channel tokens. Verify: `go test -tags conformance -count=1 ./test/conformance/`, full suite green.
- [x] 4.2 The proxy fix is covered by `internal/integration` envtest (a real API server, real Pod and Lease objects, two real `httptest.Server`s) per `docs/testing.md`'s tier model — nothing here needs a real kubelet, RBAC, or a pod's actual network identity beyond what envtest already models. No lane added to `platform/manager/test/system/`. The live incident (a Kubernetes Service pinning a connection to a non-leader pod) was measured by hand on the home install before this change. It is the thing this fixes, not a repeatable system-pack scenario — it depends on real Service load-balancing, which the pack's single-node k3d cluster does not exercise the same way.

## 5. Documentation

### Reference docs
- [x] 5.1 `.claude/rules/invariants.md`: rewrite "HTTP API is NOT leader-gated — WITH TWO EXCEPTIONS" — `/channel/ops` proxies instead of 503ing, and `/activity`/`/activity/stream`/`POST /activity` now proxy too, for the per-process-buffer reason.
- [x] 5.2 `.claude/rules/gotchas.md`: one new dated entry (2026-10-11) recording the measured incident, the abandoned leaderless alternative, and the shipped proxy fix — appended after the existing `durable-chat-ops-broker` saga entries (kept as history, not edited). A second entry, same date, records the `/activity/stream` exposure found live while testing the first fix.
- [x] 5.3 `.claude/rules/structure.md`: move "Bot API sending lives HERE" from `channels/telegram/`'s entry to `gateways/telegram/`'s.
- [x] 5.4 `docs/contracts.md`, `docs/concepts.md`, `docs/configuration.md`: UNCHANGED. The adapter-facing contract (200/204/503-retry-immediately) and the claim's published semantics are exactly `durable-chat-ops-broker`'s own — nothing publishable moved.
- [x] 5.5 Re-run `python3 .github/scripts/docs-generate.py` — `docs/integrations/telegram.md`, `docs/guides/channel-adapter.md` are generated from the chart/CRD changes in section 2. `--check` passes, 52 files up to date. No CRD regen needed in this version: the claim fields and doc comments are untouched from `durable-chat-ops-broker`.
- [x] 5.6 `docs/CHANGELOG.md`: entry noting this is code only, not a cut release.

### Adopter site
- [x] 5.7 None beyond the CHANGELOG entry — no new install-time decision, no new chart value an adopter sets.
