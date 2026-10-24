## Gotchas

| Gotcha | Fix |
|---|---|
| RBAC `resources:` must be lowercase plurals | wrong case breaks the informer silently |
| SSH deploy keys need LF-only, trailing newline | build the Secret from base64, never `--from-literal` |
| envtest needs `KUBEBUILDER_ASSETS` | `kubectl auth can-i` needs `--subresource=eviction` for `pods/eviction` |
| Clearing `close-topics` first does nothing | `helm uninstall` FIRST, then clear it, or the namespace hangs `Terminating` |
| A rendered pod isn't a running one | pin env-var NAMES, smoke new workloads |
| `resource-policy: keep` isn't retroactive | annotate BEFORE the release that stops rendering it |
| Helm never upgrades an existing CRD | `kubectl apply -f chart/crds/` on every install, or fields silently prune |
| A tag can publish nothing, silently | check the REGISTRY not the tag, 3 tags per push max |
| One smoke run per tag races | reuse a passed check run, `gh api` check-runs needs `--method GET` |
| Idle `worker_status` looks dead | check the ARTIFACT (PR/branch/comment), never the transcript |
| `gh variable set` can store a wrapped newline | `.strip()` can't reach it — `printf`, verify with `cat -A` |
| A cloud routine's push rules | fresh `change/<name>` only — labels arrive via `/fire`, not `issues` |
| `lookup` is empty without a cluster | generate under `.Release.IsInstall` only, verify with `--dry-run=server` |
| A hand-patched field survives `helm upgrade` | 3-way merge only patches diffs, check the OBJECT |
| Kubelet remembers a pulled image ID forever | push a DERIVED image (extra `LABEL`) |
| Cilium: EPERM not ECONNREFUSED on no endpoint | a rollout race, check endpoint/ReplicaSet age |
| HA config-entry failures log under `config_entries` | parse the domain from the MESSAGE TEXT |
| Telegram's `reply_to_message` never nests | name the addressed form in the menu text |
| Two `getUpdates` consumers, one bot token | 409s — stop the old poller, confirm, then start new |
| Two Telegram components held the same bot token, splitting custody the gateway's own name argued against | consolidate into the gateway — `POST /telegram/call/{method}` relay, one chart-generated shared call token |

### Wiring, review and CI mechanics

| Fact | Fix |
|---|---|
| A bundle ships wiring only behind an OFF-by-default flag, values-only refs | `wiringMode` picks the rendered OBJECT, never eligibility |
| `claude -p` on a bare runner runs at DEFAULT effort (198s vs 18s) | always set `--model`/`--effort` |
| A `workflow_dispatch` run's checks never reach the merge box (#131) | fan out via Actions matrix, push via deploy key |
| `apt-get upgrade` in a cached layer patches nothing | every Dockerfile reads a CI-dated `ARG APT_REFRESH` |
| A `pull_request` run's `base.sha` freezes at opening | diff `origin/<base>...<head>`, `fetch-depth: 0` |
| `t.Run` does NOT lower Sonar's `go:S3776` (closure charges the enclosing fn) | extract to a separate named top-level function |

### The conveyor loop — one drifted rule per issue, now in `conveyor.py`

| # | Was | Fixed by |
|---|---|---|
| #201 | a session's own `autofix` label was refused, PR left dead | only a WORKFLOW carries a person's grant forward |
| #51 | the carry required `ci` already green — backwards | `success \|\| failure` |
| #220 | a bot-dispatched round died pre-model | `allowed_bots: github-actions` |
| #243 | no `timeout-minutes`, job hung 15h unseen | sized timeout, `cancelled` handled too |
| #243 | fixer's scratch files landed in commits | real paths in the prompt, new files land only if declared |
| #248/254/255 | one rule drifted across ten copies | `conveyor.py`, every state × event tested |
| #259 | guard never heard a resolved thread | guard is archive-only now, a 15m sweep catches it |
| #259 | action checks the branch's own workflow copy vs master's | check out master's, restore after |
| #53 | moving a station LABEL didn't restart the session | explicit `restart_ok` flag |
| — | `run-name:` breaks a `workflow_run.name` match | match `.path` instead |

### Replica scaling — chat delivery and rollout races above 1 replica

| Bug | Fix |
|---|---|
| `OpQueue` is in-memory, leader-only | claim lives on the Conversation CR (`durable-chat-ops-broker`) |
| `isLeader()` matched a bare hostname, missing the Lease's uuid suffix | prefix match: `HasPrefix(holder, identity+"_")` |
| A kept-alive connection stays pinned to one replica | `CloseIdleConnections()` on the 503 path |
| A reply could mark `Delivered` with an empty `ThreadID` | skip delivery while empty, mark `owed` |
| `ChannelAdapter` re-applies `spec.image` every reconcile | patch the CR, never the Deployment |
| Hard anti-affinity stalls a same-node rollout | scale the OLD ReplicaSet to 0 |
| The old pod's grace period still blocks scheduling | force-delete it right after |
| The new pod can still lose that scheduling race | force-delete Pending pods of the CURRENT ReplicaSet too |
| ...which also caught an unrelated healthy scale-up | scope the delete to calls that cleared old affinity |
| A fresh ReplicaSet may not list right after patching | retry the list up to 30s |
| `t.Cleanup` after a risky call skips on `t.Fatal` | register `t.Cleanup` BEFORE the risky call |
| Idle conversations hold a GLOBAL slot, starving the next lane | each lane closes its own, waits for `Closed` |
| `port-forward` doesn't survive its pod being deleted | a supervisor goroutine relaunches it |
| Leader churn stalls a few conversations for whole staleness cycles | widen only that wait to 5m |
| `/channel/ops` 503'd any non-leader unconditionally — a Service pins one connection to one replica for its whole life | non-leader reverse-proxies to the leader (`leaderProxyMiddleware`), refuses only if truly unreachable. A Telegram topic went uncreated 9+ hours before this was measured. Verify: `TestChannelOpsProxiesToLeaderWhenReachable` |
| `/activity/stream`'s SSE connection has the same pinning, but the activity log is itself a per-process ring buffer a non-leader's copy never fills | proxy it too, but fall through to LOCAL state when unreachable — a partial activity view beats none, unlike `/channel/ops`. Verify: `TestActivityProxiesToLeaderWhenReachable` |
| Fixing one endpoint at a time (`/channel/ops`, then `/activity*`) was itself the failure | generalized into ONE middleware proxying every route except `/healthz`, each handler keeping its own local fallback |
| The middleware was silently a no-op: the manager's own NetworkPolicy never admitted its own kind, so every non-leader dial was DROPPED with no log anywhere — measured as a 503 every ~35s for 15+ minutes across one stable leader | admit `app.kubernetes.io/name: agentops-manager` in the existing ingress rule on 8080 — a dumb-proxy design needs a traffic pattern (a component calling its own kind) nothing else did, and a NetworkPolicy allow-list is blind to it until told. Verify: `TestEveryWiredFlowIsAllowed` |
| `call-auth.yaml`'s Secret body rendered unconditionally, unlike `adapter-auth.yaml`'s own guard — a cluster-less upgrade (GitOps sync, `helm template --is-upgrade`) minted a fresh `gatewayToken` every run | gate the body on `$existingVal` or a fresh install, matching `adapter-auth.yaml`. Verify: `TestTelegramGatewayTokenRendersNoGeneratedCredentialOnUpgrade` |
