## 1. Module — the router stops being an adapter

- [x] 1.1 Delete `telegram-router/manager.go` (the contract client) and drop `MANAGER_URL` / `ADAPTER_TOKEN` / `ADAPTER_NAME`
- [x] 1.2 Replace `refreshSources` with `loadConfig`: read `SIGNAL_TARGET`, `CHANNEL_TARGET` and the bot token (plain `TELEGRAM_BOT_TOKEN` or a projected `AGENTOPS_CRED_*botToken`), and exit naming whatever is missing
- [x] 1.3 Collapse the per-token loop machinery — token maps, leader selection, refresh ticker — to one poll loop, one token
- [x] 1.4 Update the package doc: this is plumbing, not an adapter, and one Deployment serves one token
- [x] 1.5 Update `telegram_test.go` for the new `config` type; `go build ./... && go vet ./... && go test ./...` clean
- [x] 1.6 Build and push a multi-arch `agentops-telegram-router` tag (0.2.0)

## 2. Chart — the bundle owns the workload

- [x] 2.1 New `templates/router.yaml`: Deployment (replicas 1, `Recreate`, `automountServiceAccountToken: false`), the two target URLs as env, the bot token via `secretKeyRef` on the shared Secret
- [x] 2.2 Remove the router's `SignalAdapter` from `templates/adapters.yaml`
- [x] 2.3 Remove the router's `SignalSource` from `templates/surface.yaml`
- [x] 2.4 Values: replace the `router.name` / `router.source` block with image + env + resources
- [x] 2.5 Chat `SignalSource` takes the surface name itself (no `-chat` suffix), so one name identifies the surface
- [x] 2.6 `NOTES.txt`: drop the router-source claim from the printed Pipeline

## 3. Verify and cut over

- [x] 3.1 `helm template`: two adapter CRs, one router Deployment, no router `SignalAdapter` or `SignalSource`
- [x] 3.2 Server-side dry run against the live API before applying
- [x] 3.3 Cutover: delete the `SignalAdapter`, WAIT for its pod to go, then apply — the step that prevents two pollers
- [x] 3.4 Verify live: router polling, zero 409s, sources `Wired=True`, an end-to-end task delivering to a Telegram topic
- [x] 3.5 Remove `telegram-router` from the deployment config's Pipeline `signalSources`

## 4. Docs

- [x] 4.1 CLAUDE.md: the router entry still describes a SignalAdapter reading its own SignalSource
- [x] 4.2 README: Telegram section — three components, two of them adapters
- [x] 4.3 Note the multi-bot trade-off (D3) wherever the bundle's single-surface limit is documented
