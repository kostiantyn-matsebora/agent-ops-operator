## Repository map

**A DIRECTORY IS A COMPONENT, AND ITS PATH IS AN IMAGE NAME.** The tree is the C&C
view projected onto files: one container, one directory, grouped by what that
container IS at runtime.

| Group | Holds | The type |
|---|---|---|
| `platform/` | `manager` `console` `housekeeping` `context-sync` `egress-proxy` | the product's own components |
| `runtimes/` | `claude` `ollama` `copilot` | client side of the work contract |
| `signals/` | `cron` `alertmanager` `k8s-events` `ha` `telegram` | push to `/signal/inbound` |
| `channels/` | `telegram` | serve `/channel/*` |
| `gateways/` | `telegram` | speaks no agent-ops contract at all |
| `test/` | `stubruntime` `fakebotapi` `fixtures` | **NOT components** — the e2e pack's doubles and captured payloads; `components.sh` EXCLUDES the directory, and a test asserts it |

- **THE PATH IS THE PUBLISHED IDENTITY.** A PLURAL group names a kind and lends
  its singular as a prefix; a SINGULAR group is a namespace and lends nothing:

  ```
  signals/cron       -> signal-cron       -> agentops-signal-cron
  channels/telegram  -> channel-telegram  -> agentops-channel-telegram
  runtimes/claude    -> runtime-claude    -> agentops-runtime-claude
  gateways/telegram  -> gateway-telegram  -> agentops-gateway-telegram
  platform/console   -> console           -> agentops-console
  ```

  So the kind is written once in the tree and once in the image, never twice in
  either. `.github/components.sh` derives it, asserts it is unique, and a
  release tag is matched against exactly that list — **moving a directory
  renames a published image.**
- **A MODULE PATH FOLLOWS ITS DIRECTORY.** Go resolves a module by looking for
  `go.mod` where the module says it lives, so a module claiming the repository
  root from two directories down is unfetchable.
- **A DIRECTORY HOLDS EVERYTHING ITS CONTAINER BUILDS FROM.** The build context
  IS the component's directory, so `COPY ../shared` cannot work. Shared code
  lives inside its consumer until sharing is a decision someone makes.
  - **THE CONTEXT IS PER-DIRECTORY. THE RECIPE NEED NOT BE.** `docker build -f`
    separates the two, so nine components sharing
    `.github/docker/go-module.Dockerfile` still build from their own directory
    and still copy nothing across a boundary. This rule constrains what a build
    may READ, and that has not moved.
  - **Nine byte-identical Dockerfiles were the state before**, and the tell was
    an edit that had to be SCRIPTED across all of them. A base-image bump was
    nine places to apply it in eight of.
  - **AN OWN DOCKERFILE ALWAYS WINS**, so needing something different is
    declared by putting one in the directory — `manager`, `console`,
    `egress-proxy` and `runtime-claude` are the four. `.github/components.sh`
    takes the union of Dockerfile-bearing and `go.mod`-bearing directories, so
    neither list restates the other.
  - **The shared image's binary is `/app`, and that is FORCED.** An exec-form
    `ENTRYPOINT` does not expand build arguments and distroless has no shell for
    the shell form, so a per-component entrypoint path cannot be parameterised.
- **Self-contained modules.** Every submodule has ZERO requires — standard
  library only — and nothing outside `platform/manager/` imports its `api/` or
  `internal/`.
- **Grouping is by component type, never by what installs it.** The chart is the
  allocation view and carries that; a component moving between the parent chart
  and a bundle must not move its source.

**`test/` IS OUTSIDE COMPONENT DISCOVERY, BY AN EXPLICIT `-not -path` IN
`components.sh`.** The stub runtime and the fake Bot API each carry a
`Dockerfile` and a `go.mod` because they run in a cluster during the e2e pack,
and the union `components.sh` takes would otherwise publish
`agentops-stubruntime` on the next release tag and hand every matrix two more
components — which reads as a new component rather than as a double.
`TestComponentsDiscoveryExcludesTheTestTree` fails the moment either is listed.

- **The suites themselves live in the manager's module** —
  `platform/manager/test/conformance/` (build tag `conformance`) and
  `platform/manager/test/e2e/` (build tag `e2e`) — because it is the one module
  with dependencies, and a second module would be discovered as a component.
- **`test/fixtures/` is read by relative path from `_test.go` files** in the
  owning modules. A test-only read adds no `go.mod` entry, so every module stays
  self-contained in the sense that matters.

**THERE IS EXACTLY ONE `docs/`, AT THE ROOT.** No component owns a docs
directory, and a second one appearing is a broken relative path, never a new
home for anything.

- **The restructure that gave every component a directory named for its image
  added a level**, and `platform/console/ui`'s screenshot and demo harnesses
  still pointed at `../../docs`. That resolves to `platform/docs/`, which is
  writable — so both wrote there, REPORTED SUCCESS, and the published assets
  silently stopped updating for a day.
- **Both harnesses now ASSERT the output directory exists** rather than creating
  it. A missing path means the file moved and its relative path did not.
- **Anything else resolving a path out of a component directory owes the same
  check.** `find . -type d -name docs` returns one line, and that is the test.

**Count from the repo, not from this file.** The counts were wrong twice, and
`platform/egress-proxy/` was missing from the context entirely — `.github/components.sh
images` and `modules` answer both questions from the tree.

### `platform/manager/` — the operator

Its own Go module, and the only one with dependencies. It moved out of the
repository root so that every component sits in a directory named for the image
it publishes — which is also what deleted the one hardcoded name in
`components.sh`.

| Path | Holds |
|---|---|
| `api/v1alpha1/` | CRD types plus generated deepcopy. CRD YAML in `chart/crds/` |
| `cmd/manager/main.go` | wiring: reconciler, httpapi, chat registry/ops/router, env config |
| `internal/ingest/` | signature grouping, fingerprint cooldown |
| `internal/runtimepod/` | runtime pod builder (AgentRuntime CR over bootstrap Config) |
| `internal/addressing/` | `/<pipeline> <task>` parsing — ONE segment. The `:<agent>` override is deleted: a Pipeline names one profile and a profile names one agent, so the agent comes from the wiring, and a sender picking their own reached past it |
| `internal/integration/` | envtest suite — real API server, fake chat, no kubelet. `chartDir()` names the way out to `chart/`, once |
| `config/samples/` | example CRs, the only `config/` content. Deployment-specific config belongs with the deployment, never in this module |

**`internal/controller/`** — the Conversation reconciler, plus the adapter and
wiring reconcilers:

- **Topic op enqueue** (async).
- **MCP ConfigMap**, always conversation-owned `agentops-mcp-conv-<name>`.
  Profiles declare no MCP, so there is nothing to collide over.
- **The ADMISSION GATE** — conversation cap → `Pending`, FIFO promotion driven
  by a runtime-pod DELETE watch, idle eviction.
- **The close-topics finalizer**, ownerRef GC, input pruning.
- **ChannelAdapter + SignalAdapter reconcilers** on shared workload machinery.
  `adapterworkload.go` holds ownership, credential projection and the
  type-conflict guard.
- **Channel + SignalSource reconcilers** — the `Served` condition.
- **Pipeline reconciler** — wiring validation ONLY. No source-conflict guard,
  because sources are shareable.

**`internal/httpapi/`** — `/work` long-poll dispatch, `/work/done`, and the
`/channel/*` + `/signal/*` adapter contracts (bearer auth via `ADAPTER_TOKEN`
env).

- **The pending-backlog bound lives here**, in `signals.go`.
- **NO origination endpoint.** `POST /task` is deleted.
- **The signature fallback in `signals.go` splits on LANE.** Do not collapse it
  into one rule.
  - **`alert` / `job`** keep `ingest.DefaultSignatureLabels`, which
    prometheus and signal-cron depend on.
  - **`task` / `chat`** key on the fingerprint.

**`internal/chat/`** — the channel-type-agnostic core:

| Piece | Is |
|---|---|
| Provider + Registry | in-process built-ins |
| OpQueue | outbound ops, at-least-once: `ensure-topic` \| `send` \| `close-topic` \| `delete-conversation` |
| Router | transport-neutral inbound, `/close` intercepted on the reply path |

**`delete-conversation` REPLACES `close-topic` on the deletion path.** It
reports a FACT rather than a thread instruction, and is named for the
conversation because that is what ended.

**`internal/dispatch/`** — input → work-unit resolution, plus the built-in lane
templates:

- **`templates/format.md` is the mandatory message format spec.** The agent
  writes the SAME markdown subset the outbound contract carries, never HTML —
  adapters escape tags, so a `<b>` reaches chat as literal characters.
- **`EffectiveAllowedTools` is the bound toolsets, per unit.** No profile base,
  no mode.

**`internal/mcpcompile/`** — bound MCPConfigs → `mcp.json` plus `valueFrom` env.

- **ONE entry** — `Compile` over an ordered list.
- **A raw hand-written `mcp.json` is EXCLUSIVE.** Bound with others is an
  error.

### `runtimes/claude/`

The reference `AgentRuntime` — Node plus claude-code, implementing the `/work`
contract.

- **GENERIC BY CONSTRUCTION** — git and openssh for the checkout it owns, plus
  generic shell utilities, and NO domain tooling. kubectl was dropped in image
  0.5.0.
- **A CLI here is the same category error as bundling an MCP server.** What an
  agent may reach is WIRING.
- **Need one? Derive an image** and point `AgentRuntime.spec.image` at it
  (README). That is why the field exists.

### `runtimes/ollama/`

**The second runtime, in which the RUNTIME is the harness** — Go, the agent
loop, tool dispatch, the transcript and the context handle all its own; Ollama
is called only for the next message. It is what proved the work contract
vendor-neutral: no manager, CRD or contract change was needed to build it.

- **`ollama.go` is the ONLY file that knows the vendor**, behind a
  `chatter` interface. An OpenAI-compatible sibling (vLLM, llama.cpp) is one
  more file.
- **Built-ins implemented natively** — `Read`, `Grep`, `Glob`, `Edit`, `Write`,
  `Bash` — so the chart's risk-split toolsets mean the same thing here. Paths
  are workspace-confined after symlink resolution; results are bounded.
- **MCP over the official Go SDK**, from the same mounted `mcp.json`,
  advertised as `mcp__<server>__<tool>`. The one dependency-taking module in
  the repository, and the reason it needs Go 1.25 — see `build-test.md`.
- **The gate is applied ONCE, before the request.** Only allowed tools are
  advertised; a narrowing specifier such as `Bash(kubectl:*)` grants NOTHING
  rather than widening to bare `Bash`.
- **Context is one JSON transcript per conversation** under
  `$HOME/.agentops/contexts/`, declared to `context-sync` by the bundle.
  `housekeeping` does not know this layout.
- **`options.num_ctx` is on EVERY request.** The server default truncates the
  front of the prompt silently.
- **Shipped as `chart/charts/ollama/`**, off by default, in the claude
  bundle's exact shape.

### `runtimes/copilot/`

**The third runtime, and the first whose VENDOR owns the tool vocabulary** —
Node plus `@github/copilot-sdk`, in process. Copilot runs the loop and the
tools; the runtime translates, permits and reports.

- **`vocabulary.js` is the boundary.** agent-ops patterns → Copilot's two
  layers: `availableTools` (`Read`→`builtin:view`, `Bash`→`builtin:bash`,
  `mcp__s__t`→`mcp:s-t`) plus `onPermissionRequest`. UNMAPPED DENIES and is
  logged; `mcp__<server>__*` is REFUSED, never widened to `mcp:*`;
  `Bash(kubectl:*)` is ENFORCED per call — the opposite of ollama, and both
  are right: what a runtime can enforce is its own fact.
- **The definition is `.github/agents/<agent>.agent.md`**, Copilot's path,
  composed by `tools.js` exactly as `runtimes/claude/tools.js` does. Copilot's
  own discovery of that directory is OFF (`enableConfigDiscovery: false`) and
  the composed list is ALWAYS passed explicitly, `[]` included, because the
  vendor reads an omitted `tools:` as everything.
- **A denial is `{kind: "reject", feedback}`.** A bare `reject` ENDS THE TURN
  with no text; `deny` is refused as malformed. The feedback is what the model
  reads, which is `--permission-mode dontAsk` one vendor over.
- **The context handle is MINTED here** (`crypto.randomUUID()`), state under
  `$HOME/.copilot/session-state/<id>/` — `session.db`, `events.jsonl` — which
  the bundle declares to `context-sync`. `Session not found` is the resume
  failure the ladder in `continuity.js` keys on.
- **SDK AND CLI ARE PINNED EXACTLY, both.** SDK 1.0.11 declares
  `@github/copilot ^1.0.79`, and CLI 1.0.81 dropped the `./sdk` export the SDK
  resolves at startup — a floating range installs a pair that throws before
  any session exists. `npm ci` from the lockfile is what the image runs.
- **`mode: "empty"`** — the SDK's multi-tenant posture: no telemetry, no
  cross-session store, no skills, no memory, custom instructions skipped.
- **`maxTurns` is logged, not faked.** `COPILOT_MAX_AI_CREDITS` →
  `sessionLimits.maxAiCredits` is the real ceiling; `COPILOT_PROVIDER_JSON`
  is the SDK's BYOK `provider`, verbatim, and is how the smoke test runs
  with no credential.
- **Shipped as `chart/charts/copilot/`**, off by default. The credential
  rides in `env` as a `valueFrom` because `agentops.renderRuntime` refuses an
  entry with both `env` and `credentialsSecret`; the bundle renders the Secret
  itself.

### The Telegram trio

**`channels/telegram/`** — the reference channel adapter, implementing the
`/channel` contract. Bot API sending lives HERE.

**It does NOT poll.** It receives topic updates pushed by the router
(`POST /updates`, `ChannelAdapter spec.port`) and persists the router's offset
(`GET/PUT /offset` → state API).

**`gateways/telegram/`** — the ONLY getUpdates consumer.

- **It classifies each update on `is_topic_message` and forwards it VERBATIM.**
  No topic → `signal-telegram` (origination). Topic → `channel-telegram`
  (continuation).
- **It holds no channel config, persists nothing, needs no RBAC.**
- **NOT AN ADAPTER.** It emits no signals, so it has no `SignalAdapter` CR and
  no served CR.
  - **The telegram chart owns its Deployment**, injecting
    `SIGNAL_TARGET` / `CHANNEL_TARGET` / the bot token as env.
  - **It never contacts the manager.**
- **One Deployment per bot token makes the single-consumer rule structural.** A
  missing env var exits at startup.

**`signals/telegram/`** — the chat ORIGINATION adapter.

It normalizes general-surface updates and posts `/signal/inbound`:

```
{kind: chat, fingerprint: tg-<update_id>,
 labels: agentops.dev/channel + /sender}
```

**It holds NO credentials** — it never contacts Telegram.

### The other signal adapters

**`signals/cron/`** — the reference signal adapter, implementing the `/signal`
contract. Five-field cron parser plus scheduler.

**`signals/alertmanager/`** — webhook-receiving signal adapter. Hosts `/webhook/{source}` for Alertmanager-format posts, and the
prometheus subchart ships it.

- **The pod label `agentops.dev/signal-adapter` is a CHART CONTRACT**, pinned by
  integration test.
- **RENAMED from `signal-vmalertmanager` in chart 5.24.0.** It reads the
  STANDARD Alertmanager payload, which vanilla Alertmanager and VictoriaMetrics
  both send, so the vendor name described one sender rather than the component.
- **The published image was renamed with it.** The old one is left in place for
  installs pinned to an older chart, never deleted.

**THE LINE FOR VM NAMING, everywhere in this repo: rename what names OUR
component, KEEP what names a VictoriaMetrics API OBJECT.**

- `register.go` stays full of `VMAlertmanagerConfig` and
  `operator.victoriametrics.com` because it WRITES that object, and vanilla
  Alertmanager has no object to write — its config is a file.
- `metrics.yaml` keeps `VMServiceScrape` / `VMRule` and the
  `metrics.vmServiceScrape` value on the same grounds.
- **Renaming either would name a thing that does not exist.**

**`signals/ha/`** — Home Assistant log signal adapter.

- **Reads that instance's WebSocket API** over a hand-written RFC 6455 client,
  and **THE LOG LISTING IS POLLED** — `system_log/list` every fifteen seconds,
  which is also the dwell re-check's evidence. `system_log_event` is
  subscribed as a fast path ONLY: Home Assistant fires it solely under
  `system_log: fire_event: true`, off by default, and the adapter that
  depended on it posted nothing for a day on the reference install while
  reporting Ready. The two paths share one cursor.
- **NO Kubernetes client at all.** It names no ServiceAccount, so it runs as the
  release floor; credential projected per SOURCE.
- **Same `rules`/`route` vocabulary as `signal-k8s-events`**, minus the time
  axis.
- **The fingerprint keys on LOGGER + SOURCE LOCATION** — Home Assistant's own
  dedup identity — never on the occurrence.
- **Verification ladder:** config-entry state, then recurrence.
- **Its loop breaker is the agent SURFACE** (`mcp_server`, `api`,
  `websocket_api`), because a failed agent call is logged there and reporting it
  would reach the agent that made it.

**`signals/k8s-events/`** — cluster Events signal adapter.

- **In-cluster API over `net/http`**, no client-go: SA token re-read, list+watch
  per namespace scope, 410 relist.
- **Names the account the CHART renders beside its events RBAC** — the operator
  grants adapters nothing, and creates no account either.
- **The fingerprint keys on the involved OBJECT + reason**, never the Event
  object. Kubernetes recreates those per recurrence.

### Beside a conversation

**`platform/housekeeping/`** — the disk half of conversation retention.

- **A CronJob, not a daemon.** It scans the claim ROOTS for workspace
  directories and session transcripts no Conversation backs, and removes them.
- **READ-ONLY on the API** — both etcd-side stages are the manager's — with its
  OWN SA and no agent code in the image.
- **Its own workload because THE MANAGER MOUNTS NOTHING** — see the invariant.
- **Every run is bounded** (`maxDeletions`) and `dryRun`-able.
- **The listing is PHASE-BLIND** — see the invariant.
- **Named `agentops-housekeeping`** so `signal-k8s-events`' prefix
  self-exclusion catches it. A CronJob fails on a SCHEDULE.

**`platform/egress-proxy/`** — the tool-access wall INSIDE the runtime pod.

- **ONE binary, two subcommands:** `install-redirect` (privileged init
  container, writes the redirect rules) and `proxy` (serves the redirected
  connections). One binary because the two must agree exactly on the port and
  the uid, and a second image is a second place for that agreement to rot.
- **It exists because `--allowedTools` configures a COOPERATING agent.** One
  with a shell can open a socket to a bound MCP server and call anything that
  server registers. This is the wall for an agent that does not cooperate.
- **It terminates no TLS, inspects no non-MCP byte and holds no Kubernetes
  credential.** Other traffic is copied through untouched.
- **ON BY DEFAULT via `global.agentops.runtimeDefaults.egressMediation.enabled`,
  and declinable per runtime** — see
  `docs/adr/0001-bound-component-reach.md`.

**`platform/context-sync/`** — the context sidecar. Semantics in the terminology entry;
what lives HERE is the implementation:

- **Atomic generations plus a `current` symlink.** Copies are labelled quiesced
  or best-effort.

### `platform/console/`

**The agent-ops console** — a ChannelAdapter that is ALSO the viewer.

- **Config from read-only list/watch of the eight agentops kinds**, in-cluster
  API over `net/http`, the same technique as `signal-k8s-events`.
- **Conversation traffic from the ordinary `/channel/*` contract.**
- **Embedded SPA via `go:embed`** — no npm at runtime.
- **NO write path to the Kubernetes API exists in the module.** The only write
  anywhere is `POST /channel/inbound`.
- **Names `agentops-adapter-console`**, which the CHART renders beside the
  read-only Role it binds to it.
- **Conversations carry no `pipelineRef`**, so pipeline attribution is INFERRED
  from the materialized bindings and left blank when ambiguous. Never guessed.
