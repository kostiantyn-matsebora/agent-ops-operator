# Claude context — agent-ops-operator

Go/controller-runtime Kubernetes operator (README.md for the product view,
docs/concepts.md for the CRD detail).
Self-contained modules — no dependencies outside this directory; keep it that
way. Eight Go modules: the operator (root), `channel-telegram/` (reference
channel adapter), `console/` (the console — a channel adapter that is also the
viewer), `telegram-router/` (the single getUpdates consumer), and
`signal-cron/`, `signal-vmalertmanager/`, `signal-k8s-events/`,
`signal-telegram/` (signal adapters) — the adapters dependency-free.

## Terminology (binding)

- **Agent runtime**, never "worker": CRD `AgentRuntime`, SA `agentops-runtime`,
  env `RUNTIME_*`, pkg `runtimepod`, pods `agentops-conv-<conversation>`.
- `AgentProfile` = who the agent is — identity ONLY (repo, role, prompts, env,
  limits). It carries NO capabilities: no `allowedTools`, no `mcp`. What an
  agent MAY DO comes exclusively from the Pipeline routing it.
  `AgentRuntime` = what executes it;
  `Conversation` = session + serial input queue + one thread PER bound channel
  (`spec.channelRefs[]` / `status.threads[]{channel,threadId}`).
  `Conversation.spec.toolsets`/`.mcpConfigs` mirror the originating Pipeline's
  bindings — MATERIALIZED state like `profileRef`/`channelRefs`, never hand-set,
  no `pipelineRef` exists. REFS are snapshotted, CONTENT is not: every use
  re-reads the CRs, so edits heal running conversations while re-wiring affects
  only new ones. EVERY origination now has a Pipeline to mirror: signals from
  the claiming one, `POST /task` from the one it names, and a `/<pipeline>
  <task>` chat command from the one it addresses. Nothing creates a
  Conversation without wiring behind it.
- **`Pipeline`** = THE wiring, exclusively: sources[] × channels[] + profile
  + TOOL ACCESS. No other CR carries wiring (SignalSource has no
  profile/channel refs, Channel has no default profile) — unclaimed sources
  DROP signals (`Wired=False` + response reason; for a CHAT source the reason
  also goes back to the surface the person typed on, because they are waiting).
  Channels originate NOTHING, so there is no "unwired channel" behavior to
  define: an unclaimed chat source is the unwired case. One pipeline per source
  (older claimant wins), channels shareable, Ready pipelines only.
  **Capabilities are wiring, exclusively**: two optional stanzas of ordered
  refs — `spec.toolsets` (→ `MCPToolset`, the allowlist) and `spec.mcpConfigs`
  (→ `MCPConfig`, the MCP servers).
  `spec.toolsets.mode` (`merge` | `overwrite`, default `merge`) composes against
  the **AGENT DEFINITION** — the `tools:` frontmatter of
  `.claude/agents/<agent>.md` in the profile's REPO — never against the profile,
  which carries no capabilities. Mistaking the profile for the counterpart is
  what deleted this field once already. `spec.mcpConfigs` has NO mode: a
  definition declares no MCP servers, so there merge/overwrite really would be
  one behavior wearing two names.
  Refs in order: tools concatenate with dedup, server keys overlay (later wins).
  Content stays in the referenced CRs; Ready validates both ref sets.
  **ADDRESSABLE**: `POST /task {"pipeline": X, "task": ...}` names a Pipeline and
  takes its profile, channels, and capabilities from it. There is no
  profile-addressed form and no per-profile default — a Pipeline declaring no
  bindings grants nothing, and that is a configuration, not a defect to warn
  about. Every Pipeline the CHART ships must therefore declare its own tools;
  forgetting that is what made every signal-driven conversation toolless once.
  Consequence: runtimes are generic — one `AgentRuntime` per vendor × trust
  level (the SA stays runtime-level on purpose; a Pipeline choosing an SA
  would make pipeline-edit rights a privilege escalation).
- **`MCPToolset`** = a pure LIST of tool patterns (`spec.tools`), no servers,
  no status — patterns are opaque, passed through like `allowedTools`. Servers
  live ONLY in `MCPConfig`. Manager RBAC on it is read-only.
  Bound from `Pipeline.spec.toolsets` ONLY — capabilities are wiring, never
  profile fields. What the pipeline binds is HALF the allowlist: the RUNTIME
  composes it with the agent definition's own `tools:` per the unit's
  `toolsMode` (it alone holds the checkout). Verified against the CLI:
  `--allowedTools` is the sole permission authority and a definition's `tools:`
  neither widens nor narrows the main session — so the union must be built
  here or it does not happen. Never pass `--agent <name>`: that re-applies the
  definition as an availability INTERSECTION and silently defeats `overwrite`.
  No `|| 'Read'` fallback — empty is passed as empty with
  `--permission-mode dontAsk` (a permission prompt in a pod is a hang).
  The chart ships the built-in vocabulary risk-split under
  `global.builtinToolsets` (`agentops-observe` / `-shell` / `-edit`); `global.`
  because subcharts read no other parent scope.
  `POST /task {"pipeline": X}` carries X's bindings — channels AND tooling both;
  naming a pipeline asks for its wiring, not half of it.
  Multi-channel conversations: manager fans replies/acks to every bound
  thread, relays user messages to sibling channels as attributed text, and
  dispatches once ≥1 thread binding exists.
  **The OPERATOR delivers** — agent output is posted to every bound thread by
  the manager via the adapters, for single- and multi-channel alike. Agents
  never post to a transport (no delivery mode on Channel), so prompts carry no
  transport steps and runtimes hold no channel credentials.
- **Channel adapter** = out-of-process channel-type implementation consuming
  `/channel/*` (ops long-poll + inbound push). `Channel.spec` = type-agnostic
  metadata (`adapter`, `credentialsSecretRef` — NO wiring, NO delivery mode)
  + opaque `config` that only the serving adapter interprets.
  `status.threadId` is an opaque STRING.
- **`ChannelAdapter` CR** = pure implementation (`image` + workload knobs,
  NEVER configuration or credentials — no `type`, no `env`). Interface
  METADATA is allowed and encouraged: optional `configSchema` (JSON Schema for
  the served CRs' `config`) + `credentialKeys` (docs only — the manager reads
  no Secrets). No config VALUES, connectivity, or credentials. Its reconciler
  owns the adapter Deployment, and — when `spec.port` is set — the Service,
  which is named after the WORKLOAD: `agentops-adapter-<name>`. There is no
  `agentops-channel-<name>`; two changes have now written that name by mistake.
  `spec.kubernetesAccess` mirrors SignalAdapter's: mounts the SA token +
  injects `POD_NAMESPACE`, IDENTITY ONLY — permissions stay an external grant
  against SA `agentops-adapter-<name>`, and no reconciler ever creates RBAC.
  Credentials are per-surface on
  `Channel.credentialsSecretRef`, projected into the adapter pod as `envFrom`
  with prefix `AGENTOPS_CRED_<CHANNEL>_` (kubelet-resolved; the contract's
  channel listing advertises `credentialEnvPrefix`).
- **`SignalAdapter` CR / signal adapter** = same pattern for ingest, but
  inbound-only (no ops queue): adapters push normalized signals
  (`fingerprint`, `labels`, `title?`, `payload`, `kind: alert|job`) to
  `/signal/inbound`; grouping/cooldown/recurrence stay MANAGER-side from
  `SignalSource.spec.grouping` — adapters normalize, the manager groups.
  Workload names `agentops-signal-<name>`; token derivation context
  `signal-adapter:<name>` (never interchangeable with channel tokens).
  There are NO built-in signal types — the manager hosts no signal
  transports; every type needs a serving adapter. `SignalAdapter.spec.port`
  (implementation property): when set, the reconciler owns the Service
  `agentops-signal-<name>` and injects `LISTEN_ADDR` — charts ship NO
  adapter connectivity. `spec.kubernetesAccess`: mounts the SA token +
  `POD_NAMESPACE` for implementations that self-register with their SENDER
  (push-model senders hold the "where to push" binding; the adapter writes it
  from `SignalSource.spec.config.register`, degrading to instructions in the
  Ready condition when it can't).
- On both adapter kinds, the CR NAME is the ROUTING KEY:
  `Channel`/`SignalSource.spec.adapter` names the serving adapter — a
  REFERENCE, not an attribute: that adapter's implementation defines the schema
  of the sibling `config` (drives the contract listing `?adapter=`, the
  injected `ADAPTER_NAME`, credential projection, token scope, Served) — one
  adapter per implementation by construction; duplicate adapters for one
  implementation are impossible. Adapter CRs carry no configuration —
  connectivity/creds/config live ONLY on Channel/SignalSource.
- API group `agentops.dev/v1alpha1` (provisional; rename possible pre-1.0).

## Build / test

```sh
go build ./... && go vet ./...
for m in channel-telegram telegram-router signal-telegram signal-cron \
         signal-vmalertmanager signal-k8s-events; do
  (cd $m && go build ./... && go vet ./... && go test ./...)
done
# regen after editing api/v1alpha1/ (deepcopy + CRDs):
go run sigs.k8s.io/controller-tools/cmd/controller-gen@v0.16.5 object paths=./api/...
go run sigs.k8s.io/controller-tools/cmd/controller-gen@v0.16.5 crd paths=./api/... output:crd:artifacts:config=chart/files/crds
# full tests (unit + envtest against a real API server):
KUBEBUILDER_ASSETS=$(go run sigs.k8s.io/controller-runtime/tools/setup-envtest@release-0.19 use 1.31.x --bin-dir ~/.envtest -p path) go test ./...
```

Images (bump the tag on every change — never overwrite a pushed tag):

```sh
docker build --platform linux/amd64 -t <registry>/agentops-manager:<tag> .
docker build --platform linux/amd64 -t <registry>/agentops-runtime-claude:<tag> ./runtime-claude/
docker build --platform linux/amd64 -t <registry>/agentops-channel-telegram:<tag> ./channel-telegram/
docker build --platform linux/amd64 -t <registry>/agentops-telegram-router:<tag> ./telegram-router/
docker build --platform linux/amd64 -t <registry>/agentops-signal-telegram:<tag> ./signal-telegram/
docker build --platform linux/amd64 -t <registry>/agentops-signal-cron:<tag> ./signal-cron/
docker build --platform linux/amd64 -t <registry>/agentops-signal-vmalertmanager:<tag> ./signal-vmalertmanager/
docker build --platform linux/amd64 -t <registry>/agentops-signal-k8s-events:<tag> ./signal-k8s-events/
docker build --platform linux/amd64 -t <registry>/agentops-console:<tag> ./console/
# then update the image refs (chart values for the manager, AgentRuntime CRs for
# runtimes), helm upgrade, and verify with a live task:
#   POST /task {"profile":"stub","task":"..."}   (stub runtime = no LLM cost)
```

## Map

```
api/v1alpha1/            CRD types (+ generated deepcopy); CRD YAML in chart/files/crds/
cmd/manager/main.go      wiring: reconciler, httpapi, chat registry/ops/router, env config
internal/
  controller/            Conversation reconciler: topic op enqueue (async), MCP
                         ConfigMap (always conversation-owned
                         agentops-mcp-conv-<name>; profiles declare no MCP, so
                         there is nothing to collide over), runtime-pod pool (cap + idle eviction),
                         ownerRef GC, input pruning; ChannelAdapter +
                         SignalAdapter reconcilers on shared workload machinery
                         (adapterworkload.go: ownership, credential projection,
                         type-conflict guard); Channel + SignalSource
                         reconcilers (Served condition); Pipeline reconciler
                         (wiring validation, source-conflict guard)
  httpapi/               /work long-poll dispatch, /work/done, /task,
                         /channel/* + /signal/* adapter contracts
                         (bearer auth via ADAPTER_TOKEN env)
  chat/                  channel-type-agnostic core: Provider+Registry
                         (in-process built-ins), OpQueue (outbound ops,
                         at-least-once), Router (transport-neutral inbound)
  dispatch/              input → work-unit resolution + built-in lane templates
                         (templates/format.md = mandatory message format spec);
                         EffectiveAllowedTools = the bound toolsets, per unit
                         (no profile base, no mode)
  ingest/                signature grouping, fingerprint cooldown
  mcpcompile/            bound MCPConfigs → mcp.json + valueFrom env; ONE entry
                         (Compile over an ordered list). A raw hand-written
                         mcp.json is EXCLUSIVE — bound with others = error
  runtimepod/            runtime pod builder (AgentRuntime CR over bootstrap Config)
  addressing/            /<profile>[:<agent>] parsing
  integration/           envtest suite (real API server, fake chat, no kubelet)
runtime-claude/          reference AgentRuntime (Node + claude-code) — /work contract
channel-telegram/        reference channel adapter (own module, no deps) —
                         /channel contract; Bot API sending lives HERE. Does
                         NOT poll: receives topic updates pushed by the router
                         (POST /updates, ChannelAdapter spec.port) and persists
                         the router's offset (GET/PUT /offset -> state API)
telegram-router/         the ONLY getUpdates consumer (own module, no deps) —
                         classifies each update on is_topic_message and
                         forwards it VERBATIM: no topic -> signal-telegram
                         (origination), topic -> channel-telegram
                         (continuation). Holds no channel config, persists
                         nothing, needs no RBAC. NOT AN ADAPTER: it emits no
                         signals, so it has no SignalAdapter CR and no served
                         CR — the telegram-bundle chart owns its Deployment and
                         injects SIGNAL_TARGET / CHANNEL_TARGET / the bot token
                         as env. It never contacts the manager. One Deployment
                         per bot token makes the single-consumer rule
                         structural; a missing env var exits at startup
signal-telegram/         chat ORIGINATION adapter (own module, no deps) —
                         normalizes general-surface updates to
                         {kind: chat, fingerprint: tg-<update_id>, labels:
                         agentops.dev/channel + /sender} and posts
                         /signal/inbound. Holds NO credentials — it never
                         contacts Telegram
signal-cron/             reference signal adapter (own module, no deps) —
                         /signal contract; five-field cron parser + scheduler
signal-vmalertmanager/   webhook-receiving signal adapter (own module, no
                         deps) — hosts /webhook/{source} for Alertmanager-
                         format posts; vm-bundle subchart ships it + Service
                         (pod label agentops.dev/signal-adapter is a CHART
                         CONTRACT, pinned by integration test)
signal-k8s-events/       cluster Events signal adapter (own module, no deps) —
                         in-cluster API over net/http (no client-go): SA token
                         re-read, list+watch per namespace scope, 410 relist.
                         Needs kubernetesAccess: true; the CHART binds its
                         events RBAC (the operator grants adapters nothing).
                         Fingerprint keys on involved OBJECT+reason, never the
                         Event object — k8s recreates those per recurrence
console/                 the agent-ops console (own module, no deps) — a
                         ChannelAdapter that is ALSO the viewer. Config from
                         read-only list/watch of the eight agentops kinds
                         (in-cluster API over net/http, same technique as
                         signal-k8s-events); conversation traffic from the
                         ordinary /channel/* contract; embedded SPA via
                         go:embed (no npm). NO write path to the Kubernetes API
                         exists in the module — the only write anywhere is
                         POST /channel/inbound. Needs kubernetesAccess: true
                         (identity) + the CHART's read-only Role against SA
                         agentops-adapter-console. Conversations carry no
                         pipelineRef, so pipeline attribution is INFERRED from
                         the materialized bindings and left blank when
                         ambiguous — never guessed
chart/charts/k8s-bundle/ subchart: cluster Events lane (adapter + RBAC +
                         SignalSource — the CLAIM lives in the parent chart's
                         `pipelines:`, since NO bundle ships wiring),
                         k8s-engineer profile + runtime + SA, and that SA's RBAC
                         (readonly | full=cluster-admin). The profile has no
                         repository, so it carries an inline `systemPrompt`
                         role — otherwise an event wakes a personality-free
                         agent.
                         Self-gated on `enabled OR global.demo.enabled`; demo
                         mode IS this bundle (chart/templates/demo.yaml is gone).
                         Plus the `mcp` component: MCPConfig `k8s-api` (server
                         key FIXED at `kubernetes`) + TWO MCPToolsets split by
                         risk — `k8s-observability` (16 read tools) and
                         `k8s-admin` (6 mutating), ENUMERATED not wildcarded
                         because `mcp__kubernetes__*` spans both halves and
                         defeats the split. `k8s-admin` renders only when a
                         server that REGISTERS those tools exists. OFF by
                         default (alone among the
                         components): with `mcpServers` off there is no endpoint
                         to default the URL onto, so default-on would fail its
                         own guard on every render. `mcpServers` optionally runs
                         containers/kubernetes-mcp-server (`--read-only`,
                         filters at REGISTRATION not listing) under a SECOND SA
                         `agentops-mcp-k8s` — never the runtime SA (render
                         fails if equal). That second identity IS the component's
                         reason to exist: MCP reach = server SA's RBAC ∩ toolset,
                         two walls, where kubectl+Bash has only the runtime SA's
chart/charts/telegram-bundle/
                         subchart: the three-component Telegram stack (router +
                         signal adapter + channel adapter) as adapter CRs, and
                         — under `surface.enabled` — the Channel, the chat
                         SignalSource, and the router's credential source.
                         surface.enabled makes the unguessable fields REQUIRED:
                         missing chatId, missing credential, or BOTH credential
                         forms at once FAIL the render. Credentials either way:
                         `credentials.existingSecret` OR `credentials.botToken`
                         (bundle creates the Secret). One Secret serves both —
                         the Channel sends with it, the router's source polls.
                         Ships NO Pipeline on purpose (wiring drags in a profile
                         + runtime + creds): the sources sit unclaimed until the
                         installer wires them, so NOTES.txt prints the exact
                         Pipeline to apply
chart/                   Helm chart: manager Deployment/RBAC/Service + CRDs as gated
                         templates (crds.enabled, crds.keep -> helm.sh/resource-policy:
                         keep so uninstall never cascade-deletes CRs); CRD source of
                         truth = chart/files/crds/ (controller-gen output)
config/samples/          example CRs (the only config/ content — deployment-specific
                         config belongs with the deployment, never in this module)
docs/                    reference pages: concepts.md (CRDs + capability
                         resolution), contracts.md (work + adapter contracts +
                         HTTP API), and one page per bundle subchart
CHANGELOG.md             every chart-version migration guide, newest first —
                         the ONLY place upgrade steps live
```

## Invariants (do not break)

- **The manager reads NO secrets — zero Secret API reads.** Everything
  secret-shaped compiles to `valueFrom`/`envFrom` in pod specs (the kubelet
  resolves it); transport credentials are declared per Channel
  (`credentialsSecretRef`) and PROJECTED into adapter pods, never read; the
  adapter auth token reaches the manager via env (`ADAPTER_TOKEN`), and
  per-adapter tokens are DERIVED (HMAC of master + adapter name, validated by
  re-derivation — nothing minted or stored). RBAC grants the manager no
  `secrets` verbs at all — keep it that way.
- **The operator grants adapters NO Kubernetes permissions, ever**: dedicated
  SA, no RBAC objects created or bound by any reconciler. Default posture is
  `automountServiceAccountToken: false`; `SignalAdapter.spec.kubernetesAccess`
  only mounts the token + injects `POD_NAMESPACE` — what it may DO is granted
  externally (chart/user) against SA `agentops-signal-<name>`, so an adapter
  CR can never escalate. Name-is-key makes one adapter per implementation
  structural — there is no conflict machinery to maintain.
- **Strictly serial per conversation** (one inflight unit); parallelism is
  across conversations, capped by `MAX_RUNTIMES` with idle-runtime eviction.
- **CONVERSATIONS ORIGINATE ONLY FROM CLAIMED SIGNAL SOURCES.** A channel
  CARRIES conversations; it never starts one. `/channel/inbound` is
  reply-only — `threadId` REQUIRED, unknown threads dropped, no adoption. A
  message on a chat's general surface arrives as a `kind: chat` signal from a
  chat `SignalSource`, so who answers is DECLARED by the Pipeline claiming it.
  There is no channel default profile and no `PipelineForChannel` — channels
  are shareable on purpose, so "which pipeline answers for this channel" has no
  defensible answer, and the oldest-Ready tiebreak that used to supply one is
  gone. Chat lane: task inputs (never `job` — that resumes sessions), cooldown
  OFF by default, and NO signature grouping unless `signatureLabels` is set
  (chat keys on the fingerprint; the default alert labels would hash every
  message alike into one conversation). Commands whose whole result is a reply
  (`/agents`, unknown agent, usage error) emit a send op and create nothing.
  A chat signal MUST carry `agentops.dev/channel` — `/signal/inbound` refuses
  one it could not answer.
- **HTTP API is NOT leader-gated** (`NeedLeaderElection()=false`) — webhooks
  must serve during rollouts. **Exactly one getUpdates consumer per bot token,
  ever** — that consumer is `telegram-router`, ONE poll loop per Deployment and
  ONE Deployment per token (replicas 1 + Recreate, chart-owned).
  Neither adapter polls and the manager has no poller. Adding a poll loop back
  to `channel-telegram` is the mistake that produces 409s and stolen updates.
- **Channel ops are at-least-once.** `spec.config` is opaque to the operator —
  never parse channel-type config manager-side; adapters validate their own
  and report via the Channel Ready condition. The manager never *interprets*
  config, but it MAY apply an adapter-declared `configSchema` mechanically
  (`internal/configschema`, the one place config content is touched):
  advisory-only `ConfigValid`, no type knowledge, adapter stays authoritative.
- **No relay loops**: channel implementations (adapters AND in-process
  providers) must never re-ingest their own outbound posts as inbound —
  cross-channel relay depends on it.
- **No signal loops** (the same rule one lane over): an observing signal
  adapter must NEVER emit a signal about agent-ops' own machinery. A runtime
  pod that cannot start emits a Warning event; that event becomes a signal, the
  signal opens a Conversation, the Conversation creates another runtime pod
  under a NEW name, forever. Nothing downstream catches it — the fingerprint is
  fresh (new pod name), the workload is fresh (owner is the Conversation CR),
  and even a correct liveness re-check passes it because the pod really is
  broken; `MAX_RUNTIMES` caps pods, not Conversation creation, so it fills etcd.
  `signal-k8s-events/selfexclude.go` implements THREE independent mechanisms
  (name prefix — needs no API read, so it holds with a cold cache; owner/label;
  own-namespace). Only the third is configurable: a deny-list is editable, and
  an editable loop breaker is not one. A nil excluder still applies mechanism 1
  on purpose. **agent-ops' own health is STATUS, not SIGNAL** — the reconciler
  already holds the failure; routing it back through ingest to wake an agent is
  the architectural error, not merely a noisy one.
- Runtime pods: ownerRef → Conversation (GC); repo checkout at
  **`/data/workspace`** (claude-code sessions are keyed by cwd — moving this
  path breaks session resume); `/data/workspace` and `/data/home` are mount
  points — **clear contents, never rmdir**.
- Dispatch/ingest semantics are pinned by test fixtures — change behavior by
  changing tests deliberately, not incidentally.
- **`for:` is Prometheus, `group_wait` is Alertmanager, and they are NOT the
  same thing.** `signal-k8s-events` config is deliberately two halves: `rules`
  (Prometheus — what counts as a problem and how long it must hold) and `route`
  (Alertmanager — inhibition). Alertmanager's `group_wait` batches a group
  before its FIRST notification; `for:` does not exist in Alertmanager at all.
  Spelling dwell as `group_wait` would be an Alertmanager term meaning
  something Alertmanager does not mean. Two further rules the defaults depend
  on: reasons describing a COMPLETED event (`OOMKilling`, `Evicted`,
  `BackoffLimitExceeded`) must carry `for: 0` — a dwell finds the healthy
  replacement and erases the incident; and the LAST rule must be a catch-all
  with a dwell, never a drop, so an unanticipated reason is verified rather
  than discarded. Both are pinned in `internal/integration/charttemplate_test.go`.
- Event grouping is by **workload** (`[namespace, workload]`), resolved through
  OWNER REFERENCES (Pod → ReplicaSet → Deployment) and never by parsing a pod
  name — that breaks on StatefulSets (`api-0`), DaemonSets and bare pods. Pod
  names are unique per replica and regenerated every rollout, so the old
  `[namespace, kind, name]` default made conversations scale with pods ×
  rollouts and the 7-day window reuse could never fire.

## Gotchas (paid for in debugging)

- RBAC `resources:` are lowercase plurals — a blanket rename once produced
  `AgentRuntimes` and silently broke the informer (forbidden loops in the log,
  reconciler does nothing).
- SSH deploy keys in Secrets must be LF-only with a trailing newline — CRLF or
  flattened-to-one-line keys fail with `error in libcrypto`. Prefer building
  the Secret from base64 rather than shell `--from-literal` interpolation.
- envtest needs `KUBEBUILDER_ASSETS`; `kubectl auth can-i` misparses the
  `pods/eviction` slash form — use `--subresource=eviction`.
- Never run two getUpdates consumers against one Telegram bot token (409s and
  stolen updates) — when migrating from another system, or from the old
  single-container adapter, stop its poller and CONFIRM none remains before
  starting `telegram-router`. `Channel.spec.config.pollingEnabled` is gone;
  ingest is on when the router runs.
- The router's bot Secret is the SAME one the Channel uses (it polls the bot
  the channel sends as), injected by the chart as `TELEGRAM_BOT_TOKEN`. It used
  to be an adapter with a signal-free `SignalSource` purely to carry that
  credential — which then sat at `Wired=False` until some Pipeline faked a
  claim. Modelling plumbing as an adapter is what produced that whole chain.
- **NO bundle ships a Pipeline.** Wiring names a profile, sources and channels
  that come from DIFFERENT bundles, so only the parent chart sees all of it:
  declare routes in the top-level `pipelines:` values. A Channel is shareable
  across pipelines (one bot, one group, many purposes) but a SignalSource is
  claimed by exactly ONE pipeline — name pipelines for their JOB, not for the
  channel they answer on.

## After changes

Keep commits scoped to this directory, and write documentation to the file that
OWNS that kind of content. "Update README.md" is what grew it to 969 lines —
three documents wearing one filename — so the routing is explicit:

| What changed | Where it goes |
|---|---|
| CRD fields, semantics, how capabilities resolve | `docs/concepts.md` |
| Work contract, adapter contracts, HTTP endpoints | `docs/contracts.md` |
| A subchart's components or values | `docs/<bundle>.md` |
| Breaking change + upgrade steps | `CHANGELOG.md`, newest first |
| Terminology, invariants, hard-won gotchas | this file |
| The console's views, values, or trust boundary | `docs/console.md` |
| The pitch, the kind list, the demo, the install command | `README.md` |

**README.md has a budget: 150 lines** (`wc -l README.md`). It holds the pitch and
diagram, one line per CRD kind, the behaviors that matter, the demo, install, the
Documentation index, development and status — nothing else. Reference material
and migration guides do not belong in it; if it is over budget, something is in
the wrong file.
