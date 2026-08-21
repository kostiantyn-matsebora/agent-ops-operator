# Claude context — agent-ops-operator

Go/controller-runtime Kubernetes operator (README.md for the product view,
docs/concepts.md for the CRD detail).
Self-contained modules — no dependencies outside this directory; keep it that
way. Nine Go modules: the operator (root), `channel-telegram/` (reference
channel adapter), `console/` (the console — a channel adapter that is also the
viewer), `telegram-router/` (the single getUpdates consumer), and
`signal-cron/`, `signal-vmalertmanager/`, `signal-k8s-events/`, `signal-ha/`,
`signal-telegram/` (signal adapters) — the adapters dependency-free.

## Answering (how to report findings)

**LEAD WITH THE SHORT VERSION, THEN OFFER THE LONG ONE.** Any diagnosis, design
or proposal is reported as three short parts — **problem**, **cause**,
**solution(s)** — in plain language, and then ASK whether details are wanted.
Do not open with the reasoning, the evidence trail, or the full design.

The reader decides how deep to go, every time. Volunteering the deep version
takes that choice away and buries the answer they asked for. Log excerpts,
timelines, file-anchored change lists and trade-off analysis are what "details"
MEANS — they are held back until asked for, not omitted from the work.

Applies to chat answers only. Written deliverables under `docs/` follow the
adopter-documentation rules at the foot of this file instead.

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
  bindings — MATERIALIZED state like `profileRef`/`channelRefs`, never hand-set.
  REFS are snapshotted, CONTENT is not: every use
  re-reads the CRs, so edits heal running conversations while re-wiring affects
  only new ones.
  `spec.pipelineRef` names the originating Pipeline as PROVENANCE, NEVER WIRING:
  written once at creation, read for exactly two things — scoping conversation
  REUSE and ATTRIBUTION in displays. Nothing resolves a profile, channel set or
  capability through it; that is what keeps a Pipeline edit from re-wiring a
  running conversation, and resolving anything through it would undo the whole
  snapshot rule. It exists because sources are SHAREABLE: two Pipelines listing
  one source open conversations with the SAME signature, so without it the
  second's next signal lands on the first's conversation under the wrong profile.
  Conversations predating it carry none and nothing backfills them — an empty ref
  is reusable only while ONE Ready Pipeline serves the source. EVERY origination now has a Pipeline to mirror: signals of
  EVERY kind — `alert`, `job`, `task`, `chat` — from the one claiming the
  source, and a `/<pipeline> <task>` chat command from the one it addresses.
  Nothing creates a Conversation without wiring behind it.
- **`runtimeContextId`** = agent-ops' name for the RUNTIME's opaque handle on a
  conversation's accumulated context. NEVER "session" — that is claude-code's
  noun, another backend calls it a thread, another has none; a vendor's word in
  this API teaches the next reader that the manager knows what is inside the
  handle, which it does not. The manager stores it, hands it back on the next
  work unit, and interprets nothing; `--resume` is one runtime's implementation
  and appears nowhere in the contract.
  **LATEST-WINS.** It was write-once, which was unsound: a run may legitimately
  end in a different context than it was asked to continue, so the first handle
  then named something gone — and because dispatch AND ingest both key off it,
  every later message repeated the same failed continuation. One recoverable
  loss became permanent. `Conversation.ContextID()` is the only place the
  retired `sessionId` is read (dual-read for one release; a rename that merely
  moved the field would have stranded every in-flight handle on upgrade).
  Continuity is PROMISED ONLY WHERE POSSIBLE — `AgentRuntime.spec.contextStorage`
  (`volume`|`external`|`none`) versus the configured home volume — and
  never-promised (answer fresh, say so) is not the same as promised-and-lost
  (FAIL the run: a conversation without its context is a new one wearing its
  name). Unavailability is an OUTAGE before it is a LOSS: bounded retry in the
  runtime, then a manager-side breaker that HOLDS work, because failing fast on
  every report would destroy every active conversation's context in one storage
  incident.
- **`context-sync`** = the sidecar that keeps a runtime's LIVE context on
  pod-local storage and a SNAPSHOT on the durable volume. NEVER "manager" — in
  this codebase that word means the operator, and a second thing wearing it
  would make every sentence about either ambiguous.
  It is opt-in per runtime via `AgentRuntime.spec.contextSync`, and ABSENT means
  today's pod exactly: home mounted directly, no sidecar, no migration.
  It learns work boundaries by PROXYING the work contract — the manager points
  the agent's `CONTROL_URL` at it and it forwards to the real manager — which is
  what lets it checkpoint without any runtime image changing. Two orderings are
  guarantees, not details: RESTORE completes before the first `/work` is
  answered, and CHECKPOINT completes before `/work/done` reaches the manager,
  because the manager records the context handle from that report and a handle
  whose bytes were never written names something gone.
  The agent container holds NO mount of the durable volume in this mode. That
  is deliberate twice over: a corrupt volume cannot stop a run already going,
  and an agent cannot read another conversation's context or write to the
  volume at all.
  Checkpoints are CONDITIONAL and INCREMENTAL, and the second half is
  load-bearing rather than an optimisation — a conditional-but-FULL copy every
  two minutes would push the whole context over NFS on every change, increasing
  writes to the very filesystem the mechanism protects. Unchanged files become
  hardlinks into the previous generation.
- **`Pipeline`** = THE wiring, exclusively: sources[] × channels[] + profile
  + TOOL ACCESS. No other CR carries wiring (SignalSource has no
  profile/channel refs, Channel has no default profile) — sources no Ready
  Pipeline lists DROP signals (`Wired=False` + response reason; for a CHAT source
  the reason also goes back to the surface the person typed on, because they are
  waiting). Channels originate NOTHING, so there is no "unwired channel" behavior
  to define: an unlisted chat source is the unwired case.
  **SOURCES ARE SHAREABLE, exactly as channels are** — any number of Ready
  Pipelines may list one, of any kind, with NO conflict condition and no effect
  on `Ready`. Whether two agents watch one thing is the ADOPTER's call. A signal
  admitted on a source N Pipelines serve opens N CONVERSATIONS, one each, with
  their own profiles and capabilities; per-source policy (cooldown, signature
  grouping) is evaluated ONCE ABOVE the fan-out, or the first Pipeline spends the
  window and starves the rest. `Wired` names EVERY server: that count is how many
  conversations one signal opens. Ready pipelines only. There is NO tiebreak left
  anywhere — `sourceConflicts` and oldest-claimant are DELETED; re-adding either
  is a regression, not a fix.
  The ONE lane that does not fan out is a BARE chat message: a person asked one
  question and is owed one answer, and unlike an alert they CAN name the agent.
  One server routes it, several ANSWER WITH THE CHOICES and the
  `/<pipeline> <task>` form, none keeps the unwired drop. Several claimants is
  the EXPECTED shape on a shared surface — see the many-to-many rule above —
  and the choice list is the feature, not a degraded mode. Addressed messages and
  thread replies are untouched. The lane is told apart by the ARRIVING SIGNAL's
  `kind` in ingest — no `SignalSource` or `SignalAdapter` field declares "chat
  source", and no reconciler decides it; adding such a handle buys one `if` at
  the price of a declaration every adapter author can get wrong.
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
  **WIRING IS MANY-TO-MANY, IN EVERY DIRECTION. THIS IS THE MODEL, NOT A HAZARD.**
  A Pipeline claims MANY sources and delivers to MANY channels. A source is
  claimed by MANY Pipelines. A channel carries MANY Pipelines' conversations.
  There is no exclusivity anywhere, no conflict condition, no tiebreak, and
  nothing to warn about: two agents on one surface and two agents on one source
  are ORDINARY CONFIGURATIONS an adopter chooses. Any advice that reads "prefer a
  source of its own" or "claiming this too would cost you X" is WRONG and is to
  be deleted on sight — it was written three times in this repo and reverted
  three times. The only consequence of several claimants is mechanical and
  benign: an UNADDRESSED chat message is answered with the list of agents that
  serve the surface, so the person names one. That is a teaching moment, not a
  cost, and it is the whole of it.
  **CLAIMING AND ADDRESSING ARE INDEPENDENT MECHANISMS.** A CLAIM
  (`signalSourceRefs`) decides who answers an UNADDRESSED message and is read
  from Ready pipelines only. ADDRESSING (`/<pipeline> <task>`, `router.go`
  `HandleCommand`) is a plain `Get` BY NAME — no claim check and no Ready check
  — and `boundChannels` folds the originating channel in, so the reply lands in
  the thread it was asked from whatever the addressed Pipeline declares. Two
  consequences that decide how bundles wire themselves: several pipelines share
  ONE surface without sharing its source, and listing a chat source on a
  Pipeline that is only ever addressed grants that Pipeline NOTHING while making
  every unaddressed message on that surface ambiguous — which the bare-chat lane
  answers by REFUSING. `/agents` lists Ready pipelines only, so an addressable
  Pipeline stays discoverable whether or not it claims anything.
  **REACHED, NEVER NAMED**: a Pipeline is reached two ways and no others — a
  signal posted to a source it CLAIMS, and a `/<pipeline>` chat command on a
  wired surface. There is NO HTTP form that names a Pipeline: `POST /task` was
  deleted, not renamed, because a caller selecting its own wiring is the shape
  this CRD exists to prevent. There is likewise no profile-addressed form and no
  per-profile default — a Pipeline declaring no bindings grants nothing, and
  that is a configuration, not a defect to warn about. Every Pipeline the CHART ships must therefore declare its own tools;
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
  A `kind: task` signal posted to a source X claims carries X's bindings —
  channels AND tooling both; reaching a pipeline gets its wiring, not half of it.
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
  **READ IS PER THREAD, THEREFORE PER CHANNEL** — `status.threads[].readAt` +
  `.readTracked`, written ONLY by the manager on an adapter's report to the
  OPTIONAL `POST /channel/read`. One shared mark would let a Telegram reader
  clear the console's, which is the whole reason it sits on the binding. The
  watermark is MONOTONIC and CLAMPED to the manager's clock (a stale browser
  must not un-read a thread; a skewed one must not mark the future read), a
  report that would not advance is `skipped` with NO write, and the batch is
  bounded at 50. `readTracked` is stamped on EVERY binding the manager creates,
  for every channel, so the backfill rule stays ONE rule: a binding without it
  predates the mechanism and is READ — same shape, same fix, same reason as
  `status.runs[].deliveryTracked`, and without it the first upgrade shows the
  whole namespace as new. An adapter that never reports stays fully conformant.
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
         signal-vmalertmanager signal-k8s-events signal-ha; do
  (cd $m && go build ./... && go vet ./... && go test ./...)
done
# regen after editing api/v1alpha1/ (deepcopy + CRDs):
go run sigs.k8s.io/controller-tools/cmd/controller-gen@v0.16.5 object paths=./api/...
go run sigs.k8s.io/controller-tools/cmd/controller-gen@v0.16.5 crd paths=./api/... output:crd:artifacts:config=chart/files/crds
# full tests (unit + envtest against a real API server):
KUBEBUILDER_ASSETS=$(go run sigs.k8s.io/controller-runtime/tools/setup-envtest@release-0.19 use 1.31.x --bin-dir ~/.envtest -p path) go test ./...
```

### No local Go: use a PERSISTENT container, not `docker run --rm`

This workstation has no Go toolchain, so every command above runs in a
container. Start ONE long-lived container and `docker exec` into it — a
throwaway `docker run --rm` per command pays container setup on every
invocation and throws the build cache away with it. Warm rebuilds are ~2s
through `exec`; they are not through `run --rm`.

```sh
docker volume create agentops-gomodcache; docker volume create agentops-gocache
# volumes are created ROOT-owned; chown once or every write fails as your uid
docker run --rm -u 0 -v agentops-gomodcache:/gomodcache -v agentops-gocache:/gocache \
  golang:1.23 chown -R "$(id -u):$(id -g)" /gomodcache /gocache
docker run -d --name agentops-go -u "$(id -u):$(id -g)" \
  -v "$PWD":"$PWD" -w "$PWD" \
  -v agentops-gocache:/gocache -v agentops-gomodcache:/gomodcache \
  -e GOCACHE=/gocache -e GOMODCACHE=/gomodcache \
  -e HOME=/tmp -e GOFLAGS=-buildvcs=false \
  golang:1.23 sleep infinity
# then, for every go command (-w keeps submodules working):
docker exec -i -w "$PWD" agentops-go go build ./...
```

Four details, each of which cost a debugging round:

- **The caches MUST be named volumes, never host bind mounts.** The module
  cache does heavy rename and hardlink work, and bind mounts through the
  Rancher Desktop VM corrupt it — every package fails with
  `zip: not a valid zip file` on a cache that was written seconds earlier. The
  repo itself is still a bind mount, because it must be edited from the host.
- **Run as the invoking uid.** `controller-gen` writes deepcopy and CRDs INTO
  the repo, and a root-owned generated file is a mess to undo.
- **Mount the repo at its REAL path**, not `/src`: compiler diagnostics then
  carry paths that resolve on the host.
- **`go clean -modcache` fails** (`unlinkat //gomodcache: permission denied`) —
  it tries to remove the mount point. Remove the VOLUME instead.

**A VM-BACKED DAEMON MOUNTS YOUR HOME, NOT `/tmp`.** Rancher Desktop runs the
daemon in a VM, so `-v /tmp/whatever:/data` bind-mounts an EMPTY directory: the
container runs, finds nothing, writes nothing and often says nothing. It reads
as a broken image rather than a missing mount. `docs/diagrams/export.py` builds
its scratch directory BESIDE ITSELF for exactly this reason — anything else
running a container over generated files must do the same. (The `pass`
credential helper also needs an unlocked gpg agent, so a `docker pull` from a
non-interactive session fails with `gpg: decryption failed` before it ever
reaches the registry.)

Two traps that are not the container's fault but look like it: `go build ./...`
piped into `tail` reports `tail`'s exit code, so check `${PIPESTATUS[0]}` or
redirect to a file; and `openspec` needs Node, which is likewise not installed
system-wide (`~/.local/opt/node`, symlinked into `~/.local/bin`).

### EVERY IMAGE IS MULTI-ARCH WHEREVER IT CAN BE

**`buildx --platform linux/amd64,linux/arm64 --push`, never `docker build
--platform linux/amd64`.** A single-arch image on a mixed-arch cluster does not
fail at build, at push, or at render. It fails when the SCHEDULER happens to
place the pod on the other architecture, which may be weeks later and looks like
an unrelated incident:

```
failed to pull and unpack image "...": no match for platform in manifest: not found
```

That is what an amd64-only `agentops-console` did on 2026-08-21 — it had run for
weeks purely because every reschedule had landed on an amd64 node, and the first
one that did not left the console in `ImagePullBackOff`. Nothing in the chart or
the CR was wrong; the image simply had no arm64 half. Every adapter here is
dependency-free Go and cross-compiles for free, so there is no reason to ship
one arch.

The exception is a runtime whose UPSTREAM is single-arch. `runtime-claude` is the
case: pin those with a `nodeSelector` so they only ever schedule where they can
run, and say so in the values — a pod that crash-loops on the wrong
architecture is the same failure one layer down.

Images (bump the tag on every change — never overwrite a pushed tag):

```sh
# MULTI-ARCH, and --push in the same command: buildx cannot export a
# multi-platform result to the local daemon, so a separate `docker push` would
# silently ship whichever single arch got loaded.
BX="docker buildx build --platform linux/amd64,linux/arm64 --push"
$BX -t <registry>/agentops-manager:<tag> .
$BX -t <registry>/agentops-channel-telegram:<tag> ./channel-telegram/
$BX -t <registry>/agentops-telegram-router:<tag> ./telegram-router/
$BX -t <registry>/agentops-signal-telegram:<tag> ./signal-telegram/
$BX -t <registry>/agentops-signal-cron:<tag> ./signal-cron/
$BX -t <registry>/agentops-signal-vmalertmanager:<tag> ./signal-vmalertmanager/
$BX -t <registry>/agentops-signal-k8s-events:<tag> ./signal-k8s-events/
$BX -t <registry>/agentops-signal-ha:<tag> ./signal-ha/
$BX -t <registry>/agentops-console:<tag> ./console/
$BX -t <registry>/agentops-context-sync:<tag> ./context-sync/
$BX -t <registry>/agentops-housekeeping:<tag> ./housekeeping/
# runtime-claude is the exception — upstream is amd64-only, so it ships amd64
# and its AgentRuntime carries a nodeSelector.
docker build --platform linux/amd64 -t <registry>/agentops-runtime-claude:<tag> ./runtime-claude/

# VERIFY before believing it — the failure mode is invisible until it schedules:
docker manifest inspect <registry>/agentops-console:<tag> \
  | jq -r '.manifests[].platform | "\(.os)/\(.architecture)"'
# then update the image refs (chart values for the manager, AgentRuntime CRs for
# runtimes), helm upgrade, and verify with a live task — a task is an ordinary
# signal to a source a Ready Pipeline claims (there is no /task endpoint):
#   TOKEN=$(kubectl -n <ns> get secret agentops-adapter-token \
#     -o jsonpath='{.data.token}' | base64 -d)
#   curl -sX POST http://<manager>:8080/signal/inbound -H "Authorization: Bearer $TOKEN" \
#     -d '{"source":"<src>","signals":[{"fingerprint":"smoke-1","kind":"task","payload":"..."}]}'
# (point the claiming Pipeline at a stub runtime = no LLM cost)
```

## Map

```
api/v1alpha1/            CRD types (+ generated deepcopy); CRD YAML in chart/files/crds/
cmd/manager/main.go      wiring: reconciler, httpapi, chat registry/ops/router, env config
internal/
  controller/            Conversation reconciler: topic op enqueue (async), MCP
                         ConfigMap (always conversation-owned
                         agentops-mcp-conv-<name>; profiles declare no MCP, so
                         there is nothing to collide over), the ADMISSION GATE
                         (conversation cap -> Pending, FIFO promotion driven by
                         a runtime-pod DELETE watch, idle eviction), the
                         close-topics finalizer,
                         ownerRef GC, input pruning; ChannelAdapter +
                         SignalAdapter reconcilers on shared workload machinery
                         (adapterworkload.go: ownership, credential projection,
                         type-conflict guard); Channel + SignalSource
                         reconcilers (Served condition); Pipeline reconciler
                         (wiring validation ONLY — no source-conflict guard;
                         sources are shareable)
  httpapi/               /work long-poll dispatch, /work/done,
                         /channel/* + /signal/* adapter contracts
                         (bearer auth via ADAPTER_TOKEN env); the pending-backlog
                         bound lives here, in signals.go. NO origination
                         endpoint: `POST /task` is deleted, and the signature
                         fallback in signals.go splits on LANE — alert/job keep
                         ingest.DefaultSignatureLabels (prometheus-bundle and
                         signal-cron depend on it), task/chat key on the
                         fingerprint. Do not collapse it into one rule
  chat/                  channel-type-agnostic core: Provider+Registry
                         (in-process built-ins), OpQueue (outbound ops,
                         at-least-once: ensure-topic | send | close-topic |
                         delete-conversation — the last REPLACES close-topic on
                         the deletion path, reports a FACT rather than a thread
                         instruction, and is named for the conversation because
                         that is what ended),
                         Router (transport-neutral inbound; /close intercepted
                         on the reply path)
  dispatch/              input → work-unit resolution + built-in lane templates
                         (templates/format.md = mandatory message format spec —
                         the agent writes the SAME markdown subset the outbound
                         contract carries, never HTML: adapters escape tags, so
                         a `<b>` reaches chat as literal characters);
                         EffectiveAllowedTools = the bound toolsets, per unit
                         (no profile base, no mode)
  ingest/                signature grouping, fingerprint cooldown
  mcpcompile/            bound MCPConfigs → mcp.json + valueFrom env; ONE entry
                         (Compile over an ordered list). A raw hand-written
                         mcp.json is EXCLUSIVE — bound with others = error
  runtimepod/            runtime pod builder (AgentRuntime CR over bootstrap Config)
  addressing/            /<profile>[:<agent>] parsing
  integration/           envtest suite (real API server, fake chat, no kubelet)
runtime-claude/          reference AgentRuntime (Node + claude-code) — /work contract.
                         GENERIC BY CONSTRUCTION: git/openssh (the checkout it
                         owns) plus generic shell utilities, and NO domain
                         tooling — kubectl was dropped in image 0.5.0. A CLI
                         here would be the same category error as bundling an
                         MCP server: what an agent may reach is wiring. Need
                         one? Derive an image and point AgentRuntime.spec.image
                         at it (README) — that is why the field exists
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
                         format posts; prometheus-bundle subchart ships it
                         (pod label agentops.dev/signal-adapter is a CHART
                         CONTRACT, pinned by integration test). KEEPS its
                         vendor name on purpose: the module, image and spec
                         were never VM-specific except register.go, and
                         renaming a published image is churn with a migration
                         attached. The CHART is what an operator reads, so the
                         CHART is what got renamed
signal-ha/               Home Assistant log signal adapter (own module, no deps)
                         — reads that instance's WebSocket API over a
                         hand-written RFC 6455 client (`system_log_event`, with
                         `system_log/list` for backfill and for the dwell
                         re-check's evidence). NO Kubernetes client at all:
                         `kubernetesAccess: false`, credential projected per
                         SOURCE. Same `rules`/`route` vocabulary as
                         signal-k8s-events, minus the time axis. Fingerprint
                         keys on LOGGER + SOURCE LOCATION — Home Assistant's own
                         dedup identity — never on the occurrence. Verification
                         ladder: config-entry state, then recurrence. Its loop
                         breaker is the agent SURFACE (`mcp_server`, `api`,
                         `websocket_api`), because a failed agent call is logged
                         there and reporting it would wake the agent that made it
signal-k8s-events/       cluster Events signal adapter (own module, no deps) —
                         in-cluster API over net/http (no client-go): SA token
                         re-read, list+watch per namespace scope, 410 relist.
                         Needs kubernetesAccess: true; the CHART binds its
                         events RBAC (the operator grants adapters nothing).
                         Fingerprint keys on involved OBJECT+reason, never the
                         Event object — k8s recreates those per recurrence
housekeeping/            the disk half of conversation retention (own module,
                         no deps) — a CronJob, not a daemon: scans the claim
                         ROOTS for workspace directories and session transcripts
                         no Conversation backs, and removes them. READ-ONLY on
                         the API (both etcd-side stages are the manager's), its
                         OWN SA, no agent code in the image. Its own workload
                         because THE MANAGER MOUNTS NOTHING and mounting the
                         claim root is the reach subPath isolation denies
                         agents. Every run is bounded (maxDeletions) and
                         dryRun-able; the listing is PHASE-BLIND (see the
                         invariant). Named agentops-housekeeping so
                         signal-k8s-events' prefix self-exclusion catches it — a
                         CronJob fails on a SCHEDULE
context-sync/            the context sidecar (own module, no deps) — keeps the
                         LIVE context on pod-local storage and a SNAPSHOT on the
                         durable volume. PROXIES the work contract (the agent's
                         CONTROL_URL points at it) so it learns work boundaries
                         with NO runtime image change — restore before the first
                         /work, checkpoint before /work/done reaches the manager.
                         Conditional (skip when unchanged) and INCREMENTAL
                         (hardlink the unchanged), because a full copy every two
                         minutes would increase writes to the filesystem this
                         protects. Atomic generations + a `current` symlink;
                         copies labelled quiesced or best-effort. Opt-in per
                         runtime; absent = today's pod exactly
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
                         SignalSource — the events component renders the source,
                         never the claim on it), and the
                         k8s-engineer profile — ONE object, identity only. Plus
                         the `pipelines` WIRING component — the one bundle that
                         ships a route, because it owns its whole lane: at most
                         ONE Pipeline claiming its own source with its own
                         profile and toolsets, channels values-supplied and
                         omitted when unset. OFF inside an active bundle and
                         forced on by `global.demo.enabled` ALONE, which is why
                         `pipelines.enabled` is nullable: an explicit `false`
                         must decline the route under demo too. WHICH route is a
                         fourth derivation from `global.agentops.runtime.rbacMode`
                         — `full` renders the acting `k8s-operate` (binds
                         `k8s-admin`), everything else the observing
                         `k8s-observe`; per-route booleans win both ways and both
                         at once is ALLOWED and fans out. NO
                         substrate: no AgentRuntime, no runtime SA, no
                         credential, no runtime RBAC (all of that is the parent's
                         `runtime:` + `global.agentops.runtime.*`). The profile
                         has no repository, so it carries an inline
                         `systemPrompt` role — otherwise an event wakes a
                         personality-free agent.
                         Self-gated on `enabled OR global.demo.enabled`; demo
                         mode IS this bundle (chart/templates/demo.yaml is gone).
                         Plus the `mcp` component: MCPConfig `k8s-api` (server
                         key FIXED at `kubernetes`) + TWO MCPToolsets split by
                         risk — `k8s-observability` (14 read tools) and
                         `k8s-admin` (6 mutating), ENUMERATED not wildcarded
                         because `mcp__kubernetes__*` spans both halves and
                         defeats the split. `k8s-admin` renders only when a
                         server that REGISTERS those tools exists. `mcp` and
                         `mcpServers` are ON by default and flip as a PAIR — the
                         config's URL defaults onto the deployed Service, which
                         is the only reason the component used to be off; the
                         endpoint guard stays and still fails `mcp.enabled` with
                         no server and no `url`. `mcpServers` runs
                         containers/kubernetes-mcp-server (`--read-only`,
                         filters at REGISTRATION not listing) under a SECOND SA
                         `agentops-mcp-k8s` — never the runtime SA (render
                         fails if equal). That second identity IS the component's
                         reason to exist: MCP reach = server SA's RBAC ∩ toolset,
                         two walls. Since runtime 0.5.0 it is also the ONLY
                         cluster path — no CLI in the image — so `mcp.enabled:
                         false` leaves an agent that cannot see the cluster.
                         `readOnly`/`rbac.mode` are null and DERIVE from
                         `global.agentops.runtime.rbacMode` (full => write-capable
                         server under a full SA; anything else => read-only under
                         readonly). Explicit wins — `readOnly: true` under
                         `full` is a strictly observing agent: broad grants on
                         the runtime SA that nothing can exercise
chart/charts/prometheus-bundle/
                         subchart (WAS vm-bundle through chart 5.12.0): the
                         Alertmanager ingest lane, ONE metrics MCP component
                         (`MCPConfig` server key FIXED at `prometheus`, plus a
                         WILDCARD `MCPToolset` — all six tools the server
                         registers are read-only, so unlike k8s-bundle there is
                         no risk split to enumerate; the PINNED tag is what
                         keeps the wildcard honest), its deployable server under
                         a SECOND SA, the `alert-investigator` profile (identity
                         only, inline role — no repository, so no agent
                         definition resolves), and ONE default-off route.
                         NAMED FOR THE PROTOCOL, NOT A VENDOR: the ingest core
                         reads the standard Alertmanager payload, and VM answers
                         the Prometheus query API (buildinfo reports a
                         Prometheus version; MetricsQL is a PromQL superset), so
                         one server key serves both backends. The LOGS component
                         is DELETED, not ported — VictoriaLogs speaks LogsQL and
                         no Prometheus server reaches it. Self-registration is
                         KEPT and labelled VICTORIAMETRICS-ONLY: it writes a
                         VMAlertmanagerConfig, and vanilla Alertmanager's config
                         is a file, so there is no object to write — NOTES.txt
                         prints the receiver stanza instead, with
                         `send_resolved: false` because the adapter drops
                         non-firing alerts. The backend URL is NEVER derived
                         (single-node VM, cluster mode and Prometheus each serve
                         the query API under a different path). NEVER enabled by
                         demo mode — every component needs an endpoint no demo
                         cluster has, which is why `active` has no demo branch.
                         The retired `vm-bundle:` key FAILS the render: helm
                         never reports an unread values key, so the rename would
                         otherwise install nothing and look successful
chart/charts/ha-bundle/  subchart: the Home Assistant lane and a PRIVILEGE
                         SPLIT — the log ingest lane, ONE `MCPConfig` (server
                         key FIXED at `homeassistant`, and NO server workload:
                         the house serves its own MCP endpoint), TWO risk-split
                         `MCPToolset`s, and TWO identity-only profiles —
                         `ha-user` USES the house, `ha-operator` FIXES it. The
                         split is use-versus-fix, NOT read-versus-act: Home
                         Assistant has no read-only role, so both agents act and
                         what separates them is the REST path (Assist intents
                         reach no configuration, so repairing needs a shell and
                         only the ops route binds one). The OPERATOR credential
                         gates the fixing half AND the ingest lane —
                         `subscribe_events` is admin-only, so a control token
                         authenticates and is then refused the subscription,
                         which reads like a network fault.
                         Never enabled by demo mode. Two default-off routes, and
                         BOTH claim the chat sources — wiring is many-to-many, so
                         a shared surface offering both agents is the point.
                         `ha-ops` additionally claims the log source, which is
                         the only asymmetry. `pipelines.restAccess` is PER
                         ROUTE: on for ops, off for control. Credentials come as
                         a NAME or as the TOKEN ITSELF — the token form makes the
                         bundle create the Secret and derive BOTH keys
                         (`token` + `authorization`), which is what lets a
                         secret manager's ref go straight into values
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
                         truth = chart/files/crds/ (controller-gen output).
                         templates/runtime.yaml = THE SUBSTRATE: one AgentRuntime
                         named `default` + its credential Secret when
                         `runtime.credentialsSecret.token` is set, with
                         `home.pvcRef` WIRED from the parent's own `persistence`
                         (never copied). templates/runtime-rbac.yaml renders the
                         mode-driven bindings; templates/_helpers.tpl resolves
                         BOTH substrate facts from `.Values.global` alone, so a
                         subchart calling them cannot disagree with the parent
config/samples/          example CRs (the only config/ content — deployment-specific
                         config belongs with the deployment, never in this module)
docs/                    reference pages: concepts.md (CRDs + capability
                         resolution), contracts.md (work + adapter contracts +
                         HTTP API), and one page per bundle subchart — AND the
                         published site's Jekyll source: _config.yml, _layouts/,
                         _includes/, _data/nav.yml, assets/ (css, js, vendored
                         Red Hat fonts, the exported diagrams). diagrams/ holds
                         the drawio SOURCE plus export.py — run that, never the
                         exporter by hand: it writes BOTH theme variants of BOTH
                         site pages (four SVGs) and repaints the dark ones' icon
                         ink, which drawio cannot do because the icons are
                         embedded images. THREE drawio pages, and only two are
                         exported: `landing` is the poster's own COMPOSITION
                         compressed to 950px, `site` the full argument behind its
                         full-size link, `why` the standalone poster (rendered on
                         demand, never committed). 950 BECAUSE the content column
                         is 720 and there is no breakout: displayed size is type
                         over canvas, so making it fit means REMOVING ELEMENTS
                         and tightening layout, never shrinking type. Adding
                         detail back is what makes it unreadable.
                         GitHub Pages builds this directory
                         directly from master (Deploy from a branch → /docs), so
                         there is NO workflow, NO Gemfile and no Ruby in anyone's
                         path — a feature needing a plugin Pages does not enable
                         is implemented in the theme's own assets or dropped.
                         index.md (landing — the hero, what it plugs into, ONE
                         tab strip (the diagram, the Pipeline manifest as
                         copyable page text, and the six console views), THEN the
                         stat tiles, then the sections. That order is not written
                         in the page: home.html SPLITS the rendered content at
                         its first <h2> and drops the tiles in the seam, so the
                         page states its words in order and says nothing about
                         placement. There is no diagram block in the layout and
                         no `diagram:` front matter — the strip is page content,
                         which is what lets the alt text and the manifest be the
                         page's own words), introduction.md (the adopter's
                         orientation — the model, the seams, and NO reference
                         detail: a sentence a field rename would break belongs in
                         concepts.md) and getting-started.md (THE walkthrough —
                         install and a first answer IN THE CONSOLE, ending where
                         a getting-started page should: at something working.
                         Wiring is the NEXT card, not its last section — which owns every
                         expectation, flag and failure mode, so README keeps only
                         the commands. Its test is "would the reader TYPE it or
                         READ it": what they type is on the page, what they read
                         ABOUT is a link) and installation.md (THE REAL install,
                         and the only home the PARENT chart's values have —
                         decisions before commands, values grouped by the
                         decision they serve, bundle values left to their own
                         pages) and console-guide.md (permalink `/console/` —
                         what the console is FOR: the six views and the question
                         each answers, and the authentication decision an
                         operator makes before exposing it. Endpoints, the RBAC
                         grant and the values list stay in the untouched
                         reference `docs/console.md`, which keeps its own name
                         and its own URL) are the site's pages. Its screenshots
                         under assets/img/console/ are BUILD OUTPUT — twelve
                         PNGs, two per view, written by `npm run screenshots` in
                         console/ui against a curated fixture that names no real
                         cluster. Never captured by hand, never regenerated by
                         the site build. Every command block on any
                         of them is given for BOTH platforms, as two adjacent
                         fences (`sh` then `powershell`) that assets/js/tabs.js
                         pairs into tabs — the page writes no tab markup.
                         The
                         reference pages above are NOT yet site deliverables.
                         Carrying no front
                         matter they are STATIC FILES to Jekyll — copied verbatim,
                         never converted, never given a layout — so they serve as
                         raw markdown at their URLs and nothing links to them AS
                         SITE PAGES. _layouts/page.html is what introduction.md
                         uses, via jekyll-default-layout: front matter makes a
                         file a page, and then a missing layout DOES fail the
                         build. Publishing a page is the file plus ONE line in
                         _data/nav.yml — and the page DECLARES its permalink,
                         because no permalink style is configured and the
                         sidebar marks the current entry by comparing URLs.
                         introduction.md is TWO SECTIONS — understand the
                         concepts, follow the guides — and stays that way;
                         anything else is a guide or a reference page (the
                         signal-to-answer lifecycle is the first guide owed, not
                         a section here).
                         The DEMO WIRES THE CONSOLE: where k8s-bundle renders a
                         route, that route claims the console's source and binds
                         it as a channel, from `global.agentops.console`
                         (a subchart reads no other parent scope, and helm cannot
                         derive a value from a value). Those names DUPLICATE
                         `console.signalSourceName`/`channelName`, so the render
                         FAILS when they disagree — scoped to demo mode, because
                         `console.enabled: false` is pinned to remove every
                         console object with ONE value. The claim rides the
                         EXISTING route: a second claimant makes every
                         unaddressed console message ambiguous.
                         The SHELL is Astro Starlight's geometry, read off that
                         site's own stylesheet and verified against it live:
                         BOTH rails 18.75rem, text 45rem FIXED, and the leftover
                         SPLIT EVENLY between the left gutter and the right
                         container — the rail keeps its width at that container's
                         left, so its half is empty space PAST the rail, never a
                         fatter rail. Reproduced with an explicit
                         `--ao-leftover`, because `minmax(base, 1fr)` on two
                         tracks does NOT share a remainder: `fr` sizes against
                         the whole free space, so both tracks come out the same
                         WIDTH (that is what once gave an 810px rail beside a
                         66px gutter). Body type is 17px on purpose — Red Hat
                         Text is narrow, and at 16px that 45rem column reads 99
                         characters.
                         A page needing more than prose NAMES a component with a
                         kramdown attribute list — `{: .ao-cards}` (a TWO-column
                         grid, stated not derived; an odd count leaves the last
                         card at normal width) or
                         `{: .ao-callout}` (a blockquote that EMPHASISES — the
                         plain one is an ASIDE in --ao-text-subtle, so rendering
                         a load-bearing claim in it puts a footnote where the
                         weight belongs) or `{: .ao-tabs}` (a list becomes tabbed
                         panels, each item's leading **bold** phrase the label —
                         and WITH NO SCRIPT it stays the labelled list, every
                         panel visible, which is why every word and every image
                         lives in the page). That attribute is the ONLY presentation
                         a page may carry: no <div>, no inline style — and the
                         content never moves to _includes/ or _data/ to get a
                         look, which is the same rule read the other way.
                         FRONT MATTER is the other half of that division: a page
                         declaring `next:` (eyebrow/title/body/url) gets the
                         what-next card at the FOOT of the on-this-page rail, and
                         a page declaring none gets no card — every word is the
                         page's, the include only places it. Same shape as the
                         landing page's `stats:` and its stat-icon include
CHANGELOG.md             every chart-version migration guide, newest first —
                         the ONLY place upgrade steps live
```

## Invariants (do not break)

- **THE PARENT CHART OWNS THE SUBSTRATE; BUNDLES CONTRIBUTE DOMAIN.** How agents
  execute — image, LLM credential, idle TTL, node placement, home volume, and the
  ONE identity whose RBAC is the agent's power — is release-wide and lives in
  `chart/values.yaml` (`runtime:` + `global.agentops.runtime.*`). No subchart
  renders an `AgentRuntime`, a runtime ServiceAccount or a credential Secret;
  bundles ship sources, profiles, tooling and channels and REFERENCE it. Both
  substrate keys are under `global.` because a subchart can read no other parent
  scope and k8s-bundle's MCP server derives from them — restating them in a
  subchart recreates the two-spellings-of-one-fact problem chart 4.0 removed.
  Putting the runtime in a bundle is what made a chat-only install unable to
  execute anything and made TWO runtime SAs exist, one granted everything.
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
  across conversations, capped by `MAX_ACTIVE_CONVERSATIONS` (default 5;
  `MAX_RUNTIMES` is the deprecated alias, honored one release) with
  idle-runtime eviction.
- **THE CAP IS DECIDED BEFORE ANYTHING IS PROVISIONED.** "Active" means
  POD-BACKED and is counted from the live pod list, never from status — a pod
  stuck unschedulable or a lost status patch must not invent capacity. A
  conversation that cannot be admitted gets phase `Pending`: no runtime pod, no
  MCP ConfigMap and — the point of the phase — **no `ensure-topic`**, because
  suppressing the topic is what stops a burst becoming a thousand chat threads.
  `Queued` keeps its old meaning (ADMITTED, waiting behind the serial rule) and
  is never used for capacity waiting; conflating them is the mistake to avoid.
  Admission is FIFO by creation time over a waiting set defined by PODS, not
  phase — keying on phase lets a brand-new conversation reconciled first jump an
  older one. The backlog itself is bounded by `MAX_QUEUED_CONVERSATIONS`
  (default 50), checked in INGEST rather than the reconciler because the point
  is not to create the object at all; it gates CREATION only, so window reuse
  keeps appending to a pending conversation.
- **`/close` sets a PHASE; DELETION IS A SECOND VERB.** Closing writes phase
  `Closed` + `status.closedAt` and tears down the pod, the MCP ConfigMap and the
  capacity slot, archiving every bound thread at the TRANSITION — the object,
  its `status.runs[].result`, its `runtimeContextId` and its volume state all
  survive, which is what makes REOPEN mean anything. Closing used to delete, and
  that is exactly why nobody closed anything: the only tidying tool cost more
  than the backlog.
  A closed conversation is INERT: no dispatch, no capacity, no place in the FIFO
  waiting set, **absent from conversation REUSE** (a matching signature opens a
  NEW conversation — this is the rule that makes closing mean anything), and
  absent from every pipeline. A reply typed into a closed thread is ANSWERED
  ("closed, reopen it") and creates nothing; appending an input there would be a
  black hole, and an implicit reopen would re-materialise threads on every bound
  channel because someone typed "thanks".
  **REOPEN NEVER RE-RESOLVES REFS.** Phase → `Idle`, `closedAt` cleared,
  materialized refs left EXACTLY as they are — refs are snapshots whose content
  is re-read at every use, so re-resolving would let a Pipeline edit re-wire an
  existing conversation. Missing profile or channel FAILS the reopen naming it,
  never partially. Threads come back through an ordinary `ensure-topic` carrying
  `previousThreadId` as a HINT: an adapter that can un-archive returns the same
  id, one that cannot returns a new one and is already correct. `status.reopens`
  exists so each reopen's ensure-topic op id is distinct — the ids are stable
  per conversation×channel, so without it the re-establish dedups against the
  original topic creation and never reaches the adapter.
  **`close-topic` IS NOW DERIVABLE.** It was the exception only because it was
  enqueued while the object was disappearing, leaving nothing to record against;
  now `status.threadsArchived[]` marks the done threads and an unarchived one is
  an archive still owed. Do not re-add the "one non-derivable op" clause.
  The `agentops.dev/close-topics` finalizer survives for the ONE path where the
  object really does go away — a direct `kubectl delete` of a conversation
  nobody closed — with its 2-minute grace so a down adapter can never wedge a
  deletion. `/close` is intercepted on the REPLY path before the text could
  become an input, and answers with usage on a general surface.
  **Delete and reopen are MANAGER VERBS whose reach is the BINDING**
  (`spec.channelRefs`, read off the conversation, never off the request), and
  delete REFUSES anything not already `Closed`. That is what the retired
  "no remote close verb exists" rule was actually protecting — you may only end
  a conversation you are PART of — and holding a live thread was the proof;
  a closed conversation has none, so the binding is the next-strongest.
- **`/exit` RELEASES THE RUNTIME; `/close` ENDS THE CONVERSATION.** One word
  apart and not interchangeable: `/exit` deletes the runtime POD and nothing
  else — object, threads, inputs, runs and `runtimeContextId` all survive, and
  the next input admits it again with a fresh pod. It exists for the half
  eviction cannot serve: eviction only runs when something is WAITING, so with
  nothing waiting an idle pod holds its slot, its checkout and whatever the
  runtime keeps resident until the idle TTL — longest on exactly the installs
  that RAISE that TTL for a big checkout or a warm local model.
  **`dispatch.NeedsWorker` is THE ONE definition of idle**, shared by the
  command and the eviction path; the controller's private `needsWorker` is gone
  and restating it either side is the regression to avoid, because the two
  disagreeing surfaces as a bug report about the cap, far from both.
  **REFUSED MID-RUN, on correctness grounds, not politeness**: an inflight run
  still needs a worker, so the replacement pod is created AT ONCE, gets nothing
  from `/work`, idles out the LONG TTL, and is reaped as `Succeeded` — which
  clears `Inflight`, makes the input pending again and RE-RUNS work that may
  already have acted. `/close` owns abandonment and owns it safely. Queued input
  is refused too, merely because the pod would come straight back.
  What the release COSTS is computed, never guessed:
  `ResolveFor(...).ContinuityPossible()` — the same call dispatch uses — decides
  whether the reply promises the context or warns it starts fresh.
  A Pipeline named after a manager command (`exit`, `close`, `agents`, `help`,
  `start`) is unreachable by that command: interception precedes the Pipeline
  lookup, which is what makes the commands reliable.
- **A RUNTIME POD THAT NEVER STARTS IS REAPED, NEVER EXEMPTED FROM THE CAP.**
  Reaping used to handle `Succeeded` and `Failed` only. `Pending` is COUNTED as
  active — correctly, since a stuck pod must not invent capacity — but nothing
  bounded how long it could sit there, so five pods behind a corrupt filesystem
  held an entire install for fifteen hours on 2026-08-20. The fix is a start
  DEADLINE after which the pod stops existing, which frees the slot through the
  DELETE watch that already promotes the FIFO-first waiter. Un-counting it
  instead is the invent-capacity mistake and would provision past the cap
  against resources the cluster has not released.
  The condition carries the KUBELET'S OWN REASON, verbatim. A message reading
  only "deadline exceeded" reproduces the real failure — fifteen hours in which
  nothing said what was wrong — with a timer attached. Classification comes from
  POD STATUS alone (`PodReadyToStartContainers` is the discriminator: false
  exactly while a volume will not attach, true before image pulling begins), so
  the manager needs no event-read RBAC for it.
  A conversation inside its start-failure BACKOFF is skipped by the admission
  waiting set. Leaving it at the FIFO head reproduces the outage one layer up:
  the oldest conversation cannot start, and everything behind it waits on a slot
  nobody will take.
- **ONE STORAGE BREAKER, TWO EDGES.** `internal/storagebreaker` treats
  unavailability as an OUTAGE before a LOSS, and it is fed BOTH by runs that
  report an unreachable context AND by pods that cannot be provisioned for a
  storage reason. It lived in `httpapi` watching only the first, which is why it
  never fired for the incident it was written for: no pod started, so no run
  existed to file a report. A SECOND breaker would be worse than none — two
  judgements about whether storage is down, disagreeing at the worst moment.
  Only STORAGE-attributable provisioning failures count. An unschedulable pod or
  an unpullable image opening a storage breaker would hold every conversation in
  the install for a reason that has nothing to do with storage.
  While open: admit nothing, hold in `Pending` with a reason that says STORAGE
  rather than queue, and re-test with ONE canary. The provisioning edge cannot
  close its own breaker — no pod means no run means no success to report — which
  is the whole reason `ProbeDue` exists.
- **CONDITION TAINTS ARE NOT DRAINS.** `node.kubernetes.io/not-ready`,
  `unreachable` and the pressure taints are applied by Kubernetes from node
  CONDITIONS. Reading them as a drain releases runtime pods during a transient
  NotReady, and during a partition across many nodes at once — precisely when
  acting on a stale view is least affordable. Only `spec.unschedulable` and
  taints outside that set mean a node is being taken down deliberately.
  Drain awareness is OFF by default and gated on `rbac.drainAware`, because
  seeing a cordon means reading NODES and every other permission this manager
  holds is namespaced. It shrinks the corruption window; it does not close it,
  since the storage provider picks where a shared volume is served independently
  of where runtime pods run.
- **THE RECLAIMING JOB'S LISTING IS PHASE-BLIND, ON PURPOSE.** `housekeeping/`
  removes workspace directories and session transcripts with no `Conversation`
  behind them. A CLOSED conversation still HAS a CR, so its state is protected
  by the same rule that identifies an orphan — the job needs no phase knowledge
  at all, and an "only look at live ones" optimisation would reclaim the state
  of every conversation an operator was keeping. Two orderings, each a
  correctness argument: workspaces scan the disk FIRST and list SECOND (the CR
  always predates its directory, since the pod that creates it exists only for a
  conversation that already exists); transcripts need the OPPOSITE plus a grace
  period, because the context handle is written AFTER the file exists. It runs
  under its own SA — mounting the claim ROOT is the reach `subPath` isolation
  denies agents — and the render fails if that SA equals the runtime's.
- **FILESYSTEM STATE GOES ON A PVC; EVERYTHING ELSE GOES IN THE KUBERNETES API,
  and THE MANAGER MOUNTS NOTHING.** A claim would pin it to one node, defeat
  rescheduling, and be a second source of truth beside the CRs — which is the
  failure mode this rule exists to name. Manager state is therefore always one
  of three things: a cache of a Kubernetes object, DERIVABLE from Kubernetes
  objects, or declared lossy telemetry. State fitting none of the three is a
  defect; the matrix in `docs/concepts.md` is where its row goes.
  Consequences that were each a real loss:
  **the reply is a FACT, not a queue entry** — `status.runs[].delivered[]` per
  bound thread plus `.deliveryTracked`, a stable op id
  `send:<conversation>:<channel>:<runId>`, and a reconciler backstop, because
  `/work/done` enqueueing into an in-memory queue meant a restart dropped an
  answer already durable in `status.runs[].result`. Marking happens on op
  COMPLETION — mark on enqueue and a lost op is never re-derived.
  **A run with no `deliveryTracked` is BACKFILLED as delivered, never sent**: it
  predates the mechanism, and no timestamp can tell it from a run lost to a
  restart, since both completed before the current process started. Without it,
  upgrading re-posts every recent answer to every bound thread.
  **Cooldown lives on `SignalSource.status.cooldown[]`**, written only when a
  fingerprint is ADMITTED — a suppressed re-delivery must stay free, or the
  high-volume case cooldown exists for becomes a write storm.
  **`close-topic` is DERIVABLE now** — from a bound thread missing from
  `status.threadsArchived[]` (above). It stopped being the exception when
  closing stopped deleting: the object survives, so there is something to
  record against.
  Telemetry is the declared-lossy class and must REPORT its gaps: a cursor from
  a previous process is `>= next` in the new one's sequence, so answering it
  with an empty list reads as "nothing happened" — the case eviction alone does
  not catch.
- **CONVERSATIONS ORIGINATE ONLY FROM SERVED SIGNAL SOURCES.** A channel
  CARRIES conversations; it never starts one. `/channel/inbound` is
  reply-only — `threadId` REQUIRED, unknown threads dropped, no adoption. A
  message on a chat's general surface arrives as a `kind: chat` signal from a
  chat `SignalSource`, so who answers is DECLARED by the Pipelines listing it —
  ALL of them for any other kind, and for a bare chat message only when there is
  exactly one (see the Pipeline entry above).
  There is no channel default profile and no `PipelineForChannel` — channels
  are shareable on purpose, so "which pipeline answers for this channel" has no
  defensible answer, and the oldest-Ready tiebreak that used to supply one is
  gone. `PipelineForSource` is gone too, replaced by the plural
  `PipelinesForSource`: a caller wanting ONE answer must now say what it does
  with several. Chat lane: task inputs (never `job` — that resumes sessions), cooldown
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
- **THE MANAGER COMPOSES MEANING; ADAPTERS COMPOSE PRESENTATION.** No transport
  dialect anywhere in `internal/` — no `<b>`, no `&lt;`, no `parse_mode`. An op
  carries a TYPED message (`signal` | `answer` | `relay` | `notice`, prose in a
  named markdown subset) or a TOPIC DESCRIPTOR, never rendered text: there is no
  `op.text` and no `op.title`. Escaping, length limits, splitting and topic
  naming belong to the component that knows them — Telegram caps messages at
  4096 and topics at 128, nothing else does, and a manager-side fix would be one
  transport's limits imposed on all of them. In-process providers are held to
  the same contract (they are a second renderer, not an exemption), and
  `/channel/ops` REFUSES an adapter that does not declare `contract=`, because
  one reading the retired `text` field would post empty messages and look
  healthy doing it. `router.go` used to open with "transport-neutral" and then
  emit Telegram HTML; that is the habit this invariant names. **It binds the
  AGENT too**: `dispatch/templates/format.md` tells it to write the same
  markdown subset, because an adapter escapes what it is given — the first
  version of this change left format.md on HTML and every agent answer reached
  Telegram with its tags showing.
- **A thread opens with the event that caused it.** Every input a human has not
  already seen is posted to the bound threads as a `signal` card, from
  `InputItem.Origin` — `signal` posts, `channel` does not, `kind: chat` does
  not (the person typed it), and an ABSENT origin does not, so upgrading cannot
  spray history into every open thread. Read the rule off the origin
  (`InputItem.PostToChannels`), never by enumerating input types. Card op ids
  are stable per conversation×input×channel or every reconcile reposts the
  alert. A card names its pipeline from `chat.PipelineForConversation`, which
  now READS `spec.pipelineRef` and falls back to binding-matching only for
  conversations predating it — omitting the name when even that is ambiguous.
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
  broken; `MAX_ACTIVE_CONVERSATIONS` caps pods and `MAX_QUEUED_CONVERSATIONS`
  caps the backlog, but neither stops the LOOP — it just fills etcd more slowly.
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
  on: reasons describing a COMPLETED event (`OOMKilling`, `SystemOOM`,
  `BackoffLimitExceeded`, `DeadlineExceeded`) must carry `for: 0` — a dwell
  finds the healthy replacement and erases the incident; and the LAST rule must
  be a catch-all with a dwell, never a drop, so an unanticipated reason is
  verified rather than discarded. Both are pinned in
  `internal/integration/charttemplate_test.go`.
  **`Evicted` is the exception, and is DROPPED** as of chart 5.9.0 — it used to
  sit in the past-tense set. An eviction is reported from both ends already and
  per POD from neither: kubelet evictions are caused by node pressure, which
  tier 3 reports at `for: 0` as ONE node-level signal rather than one per
  displaced pod, and API-initiated evictions are drains — routine, and
  UNATTENDED wherever a reboot manager runs. The case worth waking for is a pod
  that does not come back, which arrives as `FailedScheduling` with a dwell to
  confirm it. The drop is therefore only defensible while BOTH substitutes
  survive, so the test pins node pressure at `for: 0` and `FailedScheduling`'s
  presence TOGETHER with the drop; re-tuning one of them must not silently
  leave eviction unreported from every direction at once.
  The TIME axis (`route.timeIntervals` + `route.muteTimeIntervals`) is
  Alertmanager vocabulary too, borrowed field-for-field — a scheduled outage is
  the one thing the other three axes cannot express, since `for:` verifies a
  condition the outage genuinely satisfies, inhibition needs a cause event a
  power cut never produces, and no label carries the time of day. **Mute is
  evaluated at EMIT** — after the dwell, before the emit cap — and that ordering
  IS the safety property: a problem outliving the window still emits once it
  closes, and a muted burst never spends the emit budget. Two more: `location`
  defaults to UTC but must be NAMED, because a UTC-pinned window drifts an hour
  at each DST change (`_ "time/tzdata"` is imported in `mute.go` — distroless
  carries no zoneinfo, so without it every valid zone is rejected); and a window
  with no `matchers` deafens the source outright, which is why the shipped
  example narrows. Muting reports itself on the source's Ready condition
  (`Ready=True`, reason `Muted`, then `MuteEnded` with the count) — a muted lane
  and an idle lane are otherwise indistinguishable.
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
- **Tearing down a throwaway release: UNINSTALL FIRST, then clear the
  `agentops.dev/close-topics` finalizer.** Clearing it while the manager still
  runs achieves nothing — the reconciler re-adds it within a second, and then
  `helm uninstall` removes the only thing that could ever release it, so the
  namespace hangs in `Terminating` forever. The order is the whole trick, and
  getting it backwards looks identical right up until it wedges. Conversations
  carry the finalizer even with NO channels bound, so "no chat, no problem" is
  not a reason to skip this.
- **A rendered pod is not a running one, and a chart render test cannot tell the
  difference.** `mcpServers` shipped `PROMETHEUS_MCP_TRANSPORT` for a whole
  implementation pass: the real variable is `PROMETHEUS_MCP_SERVER_TRANSPORT`,
  the server silently fell back to stdio, and a stdio process in a pod prints a
  banner and exits — a `Completed` pod behind a Service that answers nothing.
  Every guard, every assertion and `--dry-run=server` all passed. Only starting
  the thing found it. Pin env-var NAMES third-party images read, and smoke any
  new workload before believing its values.
- **`helm.sh/resource-policy: keep` protects nothing retroactively.** Helm reads
  it off the LIVE object when a resource leaves the manifest, not off the
  manifest dropping it, so adding the annotation in the same release that stops
  rendering the resource DELETES it. Verified against helm v4, all three cases.
  Anything that stops being rendered (the generated credential Secrets) needs the
  annotation on the object FIRST — which is why `agentops.generatedSecretGuard`
  fails the render rather than trusting a migration note.
- **`lookup` returns empty on any renderer without a cluster** (`helm template`,
  CI, a GitOps controller, `--dry-run=client`), so a template that can generate a
  value on the UPGRADE path does not merely show a new credential in a diff — it
  applies one. Generate under `.Release.IsInstall` only. The corollary: a
  `lookup`-driven guard is silent under `helm template`, so it cannot be pinned
  by a chart render test — verify it with `helm upgrade --dry-run=server`.
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
- **THE PARENT CHART IS WHERE WIRING IS DECLARED; a bundle ships it only under
  the four conditions.** Wiring names a profile, sources and channels that
  routinely come from DIFFERENT bundles, and a subchart sees only itself, so one
  that shipped wiring could only ever wire ITSELF — declare routes in the
  top-level `pipelines:` values. A bundle MAY ship its own only when ALL of:
  rendering is behind an explicit wiring flag; every reference to an object the
  bundle does not itself render is a values-supplied NAME, omitted when unset;
  each Pipeline renders only with its own profile; and the flag DEFAULTS OFF —
  forced on by nothing but a values path whose declared purpose is a turnkey
  install (`global.demo.enabled`), and then only the LEAST-PRIVILEGED route.
  `k8s-bundle.pipelines` is the case that qualifies: it owns its whole lane
  (source, profile, both toolsets), so channels are the only foreign name. Its
  `enabled` is NULLABLE for exactly one reason — an explicit `false` must decline
  the route even under demo mode, which a plain `false` default cannot express.
  `prometheus-bundle.pipelines` qualifies on the same grounds and ships one
  route; its `enabled` is a plain `false` rather than nullable, because demo
  mode never enables that bundle at all, so there is nothing for an explicit
  `false` to have to beat. `ha-bundle.pipelines` is the third, on the same plain
  `false` for the same reason, and it ships TWO routes because its lane has two
  privilege levels — the acting one claims the log source and NO chat source, so
  reaching it is `/ha-ops <task>` and never an accident. `telegram-bundle` still
  ships NONE and is the counter-example: its routes genuinely span bundles,
  because a chat surface is answered by an agent from somewhere else.
  Name pipelines for their JOB, not for the
  channel they answer on. And a SignalSource is NOT claimed by exactly one
  pipeline — sources are shareable, so a bundle's route and an install's route
  claiming one source both render and the source fans out to both. That is
  reported in NOTES.txt, never refused: refusing it would be the deleted
  `sourceConflicts` guard returning one layer up.

## After changes

**DOCUMENTATION IS PART OF THE CHANGE, NOT A FOLLOW-UP.** Before a change is
committed, and certainly before it is ARCHIVED, every document the change made
untrue is already updated — in the same commit, not a later one. Archiving a
change whose docs still describe the old behaviour records the work as finished
when the half a reader meets is not.

That explicitly INCLUDES the adopter pages on the site. It is the half most often
skipped, because a behaviour change feels done once `concepts.md` is right — and
the adopter never reads `concepts.md`. Ask of every change: does the landing
page, the Introduction, Getting started or Installation now say something that is
no longer true, or fail to mention something an adopter must now decide? A page
promising a step the chart just automated is as wrong as a stale field name.

Keep commits scoped to this directory, and write documentation to the file that
OWNS that kind of content. "Update README.md" is what grew it to 969 lines —
three documents wearing one filename — so the routing is explicit:

| What changed | Where it goes |
|---|---|
| CRD fields, semantics, how capabilities resolve | `docs/concepts.md` |
| Work contract, adapter contracts, HTTP endpoints | `docs/contracts.md` |
| A subchart's components or values | `docs/<bundle>.md` |
| The PARENT chart's values, install, upgrade, uninstall | `docs/installation.md` |
| Breaking change + upgrade steps | `CHANGELOG.md`, newest first |
| Terminology, invariants, hard-won gotchas | this file |
| What the console is FOR — its views, what each answers, the authentication decision | `docs/console-guide.md` |
| What the console IS — endpoints, RBAC grant, values reference, internals | `docs/console.md` |
| A change to the console's UI | re-run `npm run screenshots` in `console/ui` — the site's screenshots are build output, and the change is not done until they match |
| The pitch, the kind list, the demo, the install command | `README.md` |
| How the site LOOKS or navigates | `docs/_layouts/`, `_includes/`, `_data/nav.yml`, `assets/` |
| What the site SAYS to an adopter | a markdown page under `docs/` |

Both value rows are "values", so the split is stated: the PARENT chart's belong
to `docs/installation.md`, a SUBCHART's to that bundle's own page, and neither
restates the other. installation.md carries the values an operator must DECIDE,
grouped by the decision they serve — `helm show values` is the exhaustive list
and a hand-copied inventory rots.

**`docs/CLAUDE.md` holds the rules for WRITING a page** — structure over prose,
the command tabs, the components a page may name, the table rules and the
pre-flight lint. This file routes what goes where, that one governs how it reads.

The last two rows are one rule read both ways: **the theme holds no prose and
the pages hold no theme.** A layout or include that starts explaining a CRD is in
the wrong file, and so is a markdown page that opens with a `<div>` or an inline
style. Adding a page to the site is a page plus one line in `_data/nav.yml` —
never navigation markup written a second time.

**ADOPTER DOCUMENTATION IS STRUCTURE OVER PROSE.** Everything an adopter reads —
the site pages above all — is written to be SCANNED, and the markdown is the
structure:

- **Structure first.** A procedure is NUMBERED STEPS. A mapping is a TABLE. A
  set is BULLETS. The one claim a page rests on is a callout. A paragraph that
  enumerates is a list that has not been written yet.
- **Short sentences, one idea each.** A sentence with three clauses is three
  sentences. If it has to be read twice, it is wrong.
- **NO SEMICOLONS.** A `;` is a full stop that lost its nerve — it is the tell of
  a sentence that should have been two. Forbidden in adopter prose, whatever the
  grammar allows.
- **Small paragraphs.** Past about three lines, it stops being read — break it,
  or make it a list.
- **Emphasise the load-bearing phrase**, not the sentence around it. Everything
  bold means nothing bold.
- **Cut what earns nothing.** The reasoning belongs in the reference page that
  owns it, in this file, or in the commit message — not in the walkthrough.

The failure mode is recognisable and has been shipped here more than once: long
compound sentences, every point explained twice, nothing scannable — prose doing
a table's job. Reference pages under `docs/` may be dense, a page an adopter
meets first may not.

**CHECK IT, do not remember it.** These rules were written and then broken on the
very next page, twice, and caught each time by the reader rather than the writer.
So the mechanical ones are a command, run before any adopter page is called done:

```sh
awk '/^---$/{fm=!fm;next} fm{next} /^```/{b=!b;next} b{next}
     /;/ && !/&[a-z]+;/ {printf "%s:%d SEMICOLON\n", FILENAME, NR}
     /^\|/||/^[0-9]+\./||/^ /||/^>/{next}
     /^$/{if(n>45)printf "%s:%d LONG PARAGRAPH (%d words)\n",FILENAME,s,n;n=0;next}
     {if(n==0)s=NR;n+=NF}' docs/*.md
```

Silence is the pass. Then BUILD it and look — the squeezed column, the wrapped
key and the headerless table were each invisible until rendered.

**The site's `--ao-*` palette is COPIED from `console/ui/src/theme/theme.css`**
into the token blocks at the head of `docs/assets/css/agentops.css` — so
changing a token is a TWO-FILE change. The copy is deliberate and
one-directional (a Jekyll site must not need a Node build to publish a
paragraph); what makes it survivable is that no colour is stated literally
anywhere else in the site CSS, so the sync is one block, not a hunt:
`grep -n '#[0-9a-fA-F]\{3,6\}' docs/assets/css/agentops.css` must return hits
only inside those blocks. The theme-choice semantics (`assets/js/theme.js` from
`theme/useTheme.ts`) are copied on the same terms, and the MARK is copied on
those terms across THREE files: `console/ui/src/components/Logo.tsx` is the
source, `docs/_includes/logo.svg` the masthead's theme-driven copy, and
`docs/assets/img/logos/agent-ops.svg` the standalone one an `<img>` can load —
which states its colours literally, because an `<img>` is its own document and
inherits no custom properties. Integration marks sit beside it, committed
unaltered from each project's own source with their terms in that directory's
README, and the PAGE names each file: a vendor list in the stylesheet would be
product knowledge in the theme.

**README.md has a budget: 150 lines** (`wc -l README.md`). It holds the pitch and
diagram, one line per CRD kind, the behaviors that matter, the demo, install, the
Documentation index — the site first — development and status, nothing else. A
distinguishing behavior is named in a LINE and the document that owns it is
linked; reference material and migration guides do not belong in it. If it is
over budget, something is in the wrong file.
