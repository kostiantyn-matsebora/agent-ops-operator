# Tasks: add-web-chat-channel

## 1. Rebase prerequisites (landed by make-channel-type-architecture-extendable)

- [x] 1.1 Generic Channel CRD (`type` + opaque `config`), string thread ids — no CRD work left for this change
- [x] 1.2 Shared transport-neutral `Router` + op pipeline in `internal/chat` — reused as-is

## 2. Router (superseded)

- [x] 2.1 Superseded: router already extracted in `internal/chat/router.go` by the sibling change

## 3. Web provider

- [ ] 3.1 Implement `internal/chat/web.go`: `Web` Provider — `EnsureTopic` returns a synthesized unique string id (`web-<nanos>`); `Send` publishes to an in-memory per-conversation broadcaster (hub with subscribe/unsubscribe, used later by SSE)
- [ ] 3.2 Register the provider in the `Registry` under type `web` in `cmd/manager/main.go`; parse web `spec.config` in the provider with Ready-condition reporting
- [ ] 3.3 Unit tests: thread-id uniqueness, broadcaster delivery/unsubscribe, registry resolution for `type: web`

## 4. Chat API endpoints

- [ ] 4.1 Add bearer-token middleware for `/chat/api/*`: token from `WEB_CHAT_TOKEN` manager env (chart-provisioned Secret as env — zero manager secret reads), constant-time compare, 401 on mismatch, open when unset
- [ ] 4.2 Implement `GET /chat/api/profiles`, `GET /chat/api/conversations?channel=`, `POST /chat/api/conversations` (via Router), `GET /chat/api/conversations/{name}`, `POST /chat/api/conversations/{name}/messages` (via Router) in `internal/httpapi`
- [ ] 4.3 Implement transcript derivation (inputs + runs, ordered, with inflight state and pruned-history marker) and raise the `/work/done` result cap to 16384 chars with a truncation marker; verify worst-case Conversation object size against runs pruning bound
- [ ] 4.4 Implement `GET /chat/api/conversations/{name}/events` SSE: ~2s cached-client change detection + broadcaster ack events + keep-alives
- [ ] 4.5 Integration tests (envtest suite): create-conversation flow, reply queueing on busy conversation, transcript content, auth 401/valid/rotation, SSE delivery of a completed run, 404s

## 5. Embedded web UI

- [ ] 5.1 Create `internal/webui` with `go:embed`ed `index.html` + `app.js` + `style.css`; serve at `GET /chat/` from the existing server (no auth on the shell, token from localStorage on API calls, 401 → token prompt)
- [ ] 5.2 Implement UI features: conversation list with state badges, transcript view, composer, new-conversation flow with profile picker and `/profile[:agent]` syntax, SSE live updates + ephemeral acks, pruned-history indicator
- [ ] 5.3 Implement sanitized rendering of the chat HTML subset (whitelist b/i/code/pre/a; escape or strip everything else, no inline handlers/scripts)
- [ ] 5.4 Manual smoke test via port-forward: full conversation round-trip against the stub runtime (`POST /task` profile `stub` or via UI), hostile-markup reply stays inert

## 6. Channel-aware dispatch delivery

- [x] 6.1 Landed generically by the sibling change (`spec.delivery`; web channels use the default `result` mode)
- [x] 6.2 Landed by the sibling change (`format.md` channel-neutral, fixtures updated)

## 7. Helm chart

- [ ] 7.1 Add `webChannel.*` and `ingress.*` values to `values.yaml` (webChannel.enabled default true; ingress.enabled default false) with comments
- [ ] 7.2 Add `chart/templates/web-channel.yaml`: Channel CR (`type: web` + config) + generated token Secret (`randAlphaNum 32` with `lookup` reuse, `existingSecret` passthrough, auth.enabled gate) injected as `WEB_CHAT_TOKEN` manager env, gated on `webChannel.enabled`
- [ ] 7.3 Add `chart/templates/ingress.yaml` routing only the `/chat` path prefix to the API port; verify with `helm template` that default values render no Ingress and opt-out renders no web-channel resources
- [ ] 7.4 Bump chart version; `helm template`/`helm lint` pass for default, opt-out, existingSecret, and ingress-enabled value combinations

## 8. Verification & docs

- [ ] 8.1 `go build ./... && go vet ./...` and full envtest suite green
- [ ] 8.2 End-to-end on a live cluster: fresh `helm upgrade --install`, open `/chat/` via port-forward, run a stub-runtime conversation start-to-reply, confirm token auth and upgrade-stable Secret
- [ ] 8.3 Update README.md (web channel concept, exposure guidance: port-forward or `/chat`-scoped ingress only) and CLAUDE.md map/invariants where affected
