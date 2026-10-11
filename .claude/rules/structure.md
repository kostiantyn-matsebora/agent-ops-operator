## Repository map

**A DIRECTORY IS A COMPONENT, AND ITS PATH IS AN IMAGE NAME.** One
container, one directory, grouped by runtime role.

| Group | Holds | Type |
|---|---|---|
| `platform/` | `manager` `console` `housekeeping` `context-sync` `egress-proxy` `mcp-aops` | product components |
| `runtimes/` | `claude` `ollama` `copilot` | client side of the work contract |
| `signals/` | `cron` `alertmanager` `k8s-events` `ha` `telegram` | push to `/signal/inbound` |
| `channels/` | `telegram` | serve `/channel/*` |
| `gateways/` | `telegram` | no agent-ops contract |
| `test/` | `stubruntime` `fakebotapi` `fixtures` | NOT components — excluded by `components.sh`, asserted by test |

- **PATH = PUBLISHED IDENTITY.** Plural group → singular prefix, singular
  group → nothing: `signals/cron`→`agentops-signal-cron`,
  `platform/console`→`agentops-console`. `.github/components.sh` derives
  and asserts uniqueness — moving a directory renames a published image.
- **BUILD CONTEXT IS THE DIRECTORY.** `COPY ../shared` cannot work. `-f`
  lets 9 components share one Dockerfile while building only from their own
  dir. An own Dockerfile (`manager`, `console`, `egress-proxy`,
  `runtime-claude`) always wins. Shared binary path is `/app`, forced.
- **Self-contained modules** — stdlib only, nothing outside
  `platform/manager/` imports its `api/`/`internal/`.
- **`test/` is excluded from discovery** (`-not -path`) — its two images
  would otherwise publish. Suites live in the manager's module
  (`test/conformance`, `test/system`, build-tag gated).
- **Exactly one `docs/`, at the root** — anything resolving a path there
  must ASSERT it exists, never create it.
- **Count from the repo**: `.github/components.sh images`/`modules`.

### `platform/manager/` — the operator

Own Go module, the only one with dependencies.

| Path | Holds |
|---|---|
| `api/v1alpha1/` | CRD types + deepcopy. CRD YAML in `chart/crds/` |
| `cmd/manager/main.go` | wiring: reconciler, httpapi, chat, env config |
| `internal/ingest/` | signature grouping, fingerprint cooldown |
| `internal/runtimepod/` | runtime pod builder |
| `internal/addressing/` | `/<pipeline> <task>` parsing, one segment, no `:<agent>` override |
| `internal/controller/` | Conversation/ChannelAdapter/SignalAdapter/Channel/SignalSource/Pipeline reconcilers, admission gate (FIFO, idle eviction), close-topics finalizer |
| `internal/httpapi/` | `/work`, `/channel/*`, `/signal/*`. NO origination endpoint (`POST /task` deleted) |
| `internal/chat/` | Provider/Registry, OpQueue (`ensure-topic`\|`send`\|`close-topic`\|`delete-conversation`), Router |
| `internal/dispatch/` | input → work-unit, `templates/format.md` is the mandatory output format |
| `internal/mcpcompile/` | bound MCPConfigs → `mcp.json`. Raw hand-written `mcp.json` is EXCLUSIVE |
| `internal/integration/` | envtest suite, real API server, no kubelet |
| `config/samples/` | example CRs only |

### Runtimes

| Runtime | Is | Key fact |
|---|---|---|
| `runtimes/claude/` | reference `AgentRuntime`, Node + claude-code | GENERIC — no domain tooling, what an agent reaches is WIRING |
| `runtimes/ollama/` | the harness IS the runtime, Go, own loop/tools/transcript | `ollama.go` is the only vendor-aware file. Gate applies ONCE before the request — `Bash(kubectl:*)` grants nothing, never widens |
| `runtimes/copilot/` | vendor owns tool vocabulary, Node + copilot-sdk | `vocabulary.js` is the boundary — `Bash(kubectl:*)` ENFORCED per call (opposite of ollama, each enforces what it can) |

All three are optional bundles under `chart/charts/<name>/`, off by default.

### Signals and channels

| Component | Is |
|---|---|
| `channels/telegram/` | reference channel adapter. Does NOT poll — receives router-pushed updates |
| `gateways/telegram/` | the ONLY getUpdates consumer — not an adapter, emits no signals |
| `signals/telegram/` | chat origination adapter, holds no credentials |
| `signals/cron/` | reference signal adapter, 5-field cron parser |
| `signals/alertmanager/` | webhook adapter, standard Alertmanager payload (renamed from `signal-vmalertmanager`, `register.go` keeps `VMAlertmanagerConfig`) |
| `signals/ha/` | Home Assistant adapter, polls `system_log/list` every 15s. Fingerprint keys on LOGGER + SOURCE LOCATION |
| `signals/k8s-events/` | cluster Events adapter, in-cluster `net/http`, no client-go — fingerprint keys on OBJECT + reason |

### Beside a conversation

| Component | Is |
|---|---|
| `platform/housekeeping/` | disk GC CronJob, PHASE-BLIND listing (a closed conversation still has a CR) |
| `platform/egress-proxy/` | tool-access wall inside the runtime pod — the wall for an agent that doesn't cooperate with `--allowedTools` |
| `platform/context-sync/` | context sidecar, atomic generations + `current` symlink |
| `platform/mcp-aops/` | thin MCP client forwarding `AOPS_MCP_TOKEN` — decides nothing, the MANAGER enforces every verb |

### `platform/console/`

A ChannelAdapter that is ALSO the viewer, read-only watch of the ten
agentops kinds plus pods/deployments/cronjobs. **NO write path to the
Kubernetes API** — the only write anywhere is `POST /channel/inbound`.
Topology is THREE VIEWS (Model, Components, Infrastructure) of one
activity feed. `docs/console.md` owns the detail.
