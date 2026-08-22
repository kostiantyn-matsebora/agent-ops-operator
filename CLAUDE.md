# Claude context — agent-ops-operator

**Go/controller-runtime Kubernetes operator.** `README.md` for the product
view, `docs/concepts.md` for the CRD detail.

**Self-contained modules.** No dependencies outside this directory. Keep it that
way.

| Count | What |
|---|---|
| **Twelve Go modules** | the operator (root) plus eleven submodules, every submodule dependency-free |
| **Thirteen images** | those twelve plus `runtime-claude/`, which has no `go.mod` |

| The eleven submodules | Are |
|---|---|
| `channel-telegram/`, `console/` | channel adapters — the reference one, and the viewer that is also an adapter |
| `signal-cron/`, `signal-alertmanager/`, `signal-k8s-events/`, `signal-ha/`, `signal-telegram/` | signal adapters |
| `telegram-router/` | neither — the single getUpdates consumer |
| `context-sync/`, `egress-proxy/`, `housekeeping/` | run BESIDE a conversation, serving no CR |

**Count from the repo, not from this sentence.** It was wrong twice, and
`egress-proxy/` was missing from this file entirely.

## Answering (how to report findings)

**LEAD WITH THE SHORT VERSION, THEN OFFER THE LONG ONE.**

Any diagnosis, design or proposal is reported as three short parts, in plain
language:

1. **Problem**
2. **Cause**
3. **Solution(s)**

- **Then ASK whether details are wanted.** Never open with the reasoning, the
  evidence trail or the full design.
- **The reader decides how deep to go, every time.** Volunteering the deep
  version takes that choice away and buries the answer they asked for.
- **Details are held back, not omitted from the work.** Log excerpts, timelines,
  file-anchored change lists and trade-off analysis are what "details" MEANS.
- **Chat answers only.** Written deliverables under `docs/` follow
  `docs/CLAUDE.md`.

## Session naming (say what this window is doing)

**NAME THE SESSION `<phase> <change>`, AND RENAME IT WHENEVER EITHER CHANGES.**
Several of these run at once, in several windows and over Remote Control, and a
row reading `agent-ops-operator-05` says nothing about which one is mid-migration
and which is idle on a docs tweak.

```sh
.claude/hooks/session-title.sh set 'opsx:apply discoverable-addressing'
```

That writes the title and paints it.

| Hook | Does |
|---|---|
| `UserPromptSubmit`, `Stop`, `SessionStart` | REPAINT at every turn boundary |
| `SessionEnd` | forgets the title |

- **The repaint is required, not belt-and-braces.** Claude Code writes the
  terminal title itself, so a one-off escape does not survive.
- **All four are wired in `.claude/settings.json`.**
- **The script is a silent no-op** with no title file, no terminal or no `jq`,
  so a hook never becomes an error.

**BOTH LIVE IN THIS REPO AND ARE COMMITTED**, so a clone gets the behaviour and
the rule that names it in one checkout. `$CLAUDE_PROJECT_DIR` is what keeps the
wiring path-independent.

- **Deliberately NOT `~/.claude`.** The rule is about THIS repo's changes and
  its opsx phases, and a title convention that only exists on the machine that
  wrote it is a rule nobody else follows.
- **The title file itself stays per-session under `$XDG_RUNTIME_DIR`**, never in
  the repo. It is scratch keyed by session id, not configuration.

- **The PHASE is the opsx verb driving the work** — `opsx:explore`,
  `opsx:propose`, `opsx:update`, `opsx:apply`, `opsx:archive`.
- **The CHANGE is its directory name** under `openspec/changes/`.
- **Set it at the START and again at every transition**, never once at the end.
  The title exists to be read while the work is still running.
- **Work with no change behind it says what it is**, in the same two-word shape:
  `review chart-values`, `debug telegram-409`, `docs installation`.
- **A title reading only `claude` is the failure this rule names.**

**The script moves the TERMINAL TITLE ONLY.** Two other names exist and neither
is reachable from inside a session:

| name | where it shows | who sets it |
|---|---|---|
| terminal title | the window or tab | this script, every turn |
| session display name | prompt box, `/resume` picker, terminal title | the USER: `/rename <name>`, or `claude -n "<name>"` at launch |
| peer name (`agent-ops-operator-05`) | `ListAgents`, Remote Control rows | nobody — derived from the directory |

- **When a window's name matters anywhere but its own title bar, ASK for
  `/rename`** rather than reporting a rename that did not happen.
- **`terminalTitleFromRename` governs the terminal-title half**, defaults ON,
  and therefore beats this script until the next repaint.

## Terminology (binding)

### A Pipeline is what a message ADDRESSES, never "an agent"

- **The listing command is `/pipelines`.** `/agents` still answers and is never
  printed, offered or registered — a published word cannot simply stop working,
  but nobody should learn it from us again.
- **"Agent" is TAKEN.** It names a DEFINITION in `.claude/agents/` inside a
  profile's repository, which is what `AgentProfile.spec.agent` selects. Two
  meanings on one word, and the more visible one was wrong.
- **The word is carved into every install's composer.** `internal/chat`
  publishes the vocabulary a transport registers as its command menu, which is
  why this had to be right BEFORE that shipped.
- **`pipelines` joins the reserved set** a Pipeline cannot be reached by.

### Agent runtime, never "worker"

CRD `AgentRuntime`, SA `agentops-runtime`, env `RUNTIME_*`, pkg `runtimepod`,
pods `agentops-conv-<conversation>`.

### The four conversation-shaped kinds

| Kind | Is |
|---|---|
| `AgentProfile` | **who the agent is** — identity ONLY: repo, role, prompts, env, limits |
| `AgentRuntime` | **what executes it** |
| `Conversation` | **session + serial input queue + one thread PER bound channel** (`spec.channelRefs[]` / `status.threads[]{channel,threadId}`) |
| `Pipeline` | **the wiring** — see below |

**`AgentProfile` carries NO capabilities.** No `allowedTools`, no `mcp`. What an
agent MAY DO comes exclusively from the Pipeline routing it.

**`Conversation.spec.toolsets` / `.mcpConfigs` mirror the originating
Pipeline's bindings.** MATERIALIZED state like `profileRef` / `channelRefs`,
never hand-set.

**REFS are snapshotted, CONTENT is not.** Every use re-reads the CRs, so edits
heal running conversations while re-wiring affects only new ones.

#### `spec.pipelineRef` is PROVENANCE, NEVER WIRING

Written once at creation, and read for exactly two things:

1. Scoping conversation REUSE.
2. ATTRIBUTION in displays.

- **Nothing resolves a profile, channel set or capability through it.** That is
  what keeps a Pipeline edit from re-wiring a running conversation, and
  resolving anything through it would undo the whole snapshot rule.
- **It exists because sources are SHAREABLE.** Two Pipelines listing one source
  open conversations with the SAME signature, so without it the second's next
  signal lands on the first's conversation under the wrong profile.
- **Conversations predating it carry none and nothing backfills them.** An empty
  ref is reusable only while ONE Ready Pipeline serves the source.
- **EVERY origination now has a Pipeline to mirror.** Signals of every kind —
  `alert`, `job`, `task`, `chat` — from the one claiming the source, and a
  `/<pipeline> <task>` chat command from the one it addresses. Nothing creates a
  Conversation without wiring behind it.

### `runtimeContextId`

**agent-ops' name for the RUNTIME's opaque handle on a conversation's
accumulated context.**

**NEVER "session".** That is claude-code's noun. Another backend calls it a
thread, another has none. A vendor's word in this API teaches the next reader
that the manager knows what is inside the handle, which it does not.

- **The manager stores it, hands it back on the next work unit, and interprets
  NOTHING.**
- **`--resume` is one runtime's implementation** and appears nowhere in the
  contract.

**LATEST-WINS.** It was write-once, which was unsound:

- A run may legitimately end in a different context than it was asked to
  continue, so the first handle then named something gone.
- Dispatch AND ingest both key off it, so every later message repeated the same
  failed continuation.
- One recoverable loss became permanent.

**`Conversation.ContextID()` is the only place the retired `sessionId` is
read.** Dual-read for one release: a rename that merely moved the field would
have stranded every in-flight handle on upgrade.

**Continuity is PROMISED ONLY WHERE POSSIBLE** —
`AgentRuntime.spec.contextStorage` (`volume` | `external` | `none`) against the
configured home volume.

| Case | Behaviour |
|---|---|
| never-promised | answer fresh, and say so |
| promised-and-lost | FAIL the run — a conversation without its context is a new one wearing its name |

**Unavailability is an OUTAGE before it is a LOSS.** Bounded retry in the
runtime, then a manager-side breaker that HOLDS work. Failing fast on every
report would destroy every active conversation's context in one storage
incident.

### `context-sync`

**The sidecar that keeps a runtime's LIVE context on pod-local storage and a
SNAPSHOT on the durable volume.**

**NEVER "manager".** In this codebase that word means the operator, and a second
thing wearing it would make every sentence about either ambiguous.

- **Opt-in per runtime** via `AgentRuntime.spec.contextSync`. ABSENT means
  today's pod exactly: home mounted directly, no sidecar, no migration.
- **It learns work boundaries by PROXYING the work contract.** The manager
  points the agent's `CONTROL_URL` at it and it forwards to the real manager,
  which is what lets it checkpoint without any runtime image changing.
- **Two orderings are guarantees, not details.** RESTORE completes before the
  first `/work` is answered, and CHECKPOINT completes before `/work/done`
  reaches the manager — the manager records the context handle from that report,
  and a handle whose bytes were never written names something gone.
- **The agent container holds NO mount of the durable volume in this mode.**
  Deliberate twice over: a corrupt volume cannot stop a run already going, and
  an agent cannot read another conversation's context or write to the volume at
  all.
- **Checkpoints are CONDITIONAL and INCREMENTAL.** The second half is
  load-bearing rather than an optimisation — a conditional-but-FULL copy every
  two minutes would push the whole context over NFS on every change, increasing
  writes to the very filesystem the mechanism protects. Unchanged files become
  hardlinks into the previous generation.

### `Pipeline`

**THE wiring, exclusively:** sources[] × channels[] + profile + TOOL ACCESS.

**No other CR carries wiring.** SignalSource has no profile or channel refs.
Channel has no default profile.

- **Sources no Ready Pipeline lists DROP signals** — `Wired=False` plus a
  response reason. For a CHAT source the reason also goes back to the surface
  the person typed on, because they are waiting.
- **Channels originate NOTHING**, so there is no "unwired channel" behavior to
  define. An unlisted chat source is the unwired case.

#### SOURCES ARE SHAREABLE, exactly as channels are

- **Any number of Ready Pipelines may list one**, of any kind, with NO conflict
  condition and no effect on `Ready`. Whether two agents watch one thing is the
  ADOPTER's call.
- **A signal admitted on a source N Pipelines serve opens N CONVERSATIONS**, one
  each, with their own profiles and capabilities.
- **Per-source policy is evaluated ONCE ABOVE the fan-out** — cooldown,
  signature grouping — or the first Pipeline spends the window and starves the
  rest.
- **`Wired` names EVERY server.** That count is how many conversations one
  signal opens. Ready pipelines only.
- **There is NO tiebreak left anywhere.** `sourceConflicts` and oldest-claimant
  are DELETED. Re-adding either is a regression, not a fix.

#### The ONE lane that does not fan out is a BARE chat message

A person asked one question and is owed one answer, and unlike an alert they CAN
name the agent.

| Ready Pipelines serving the chat source | What happens |
|---|---|
| one | it routes |
| several | ANSWER WITH THE CHOICES and the `/<pipeline> <task>` form |
| none | the unwired drop |

- **Several claimants is the EXPECTED shape on a shared surface** — see the
  many-to-many rule below. The choice list is the feature, not a degraded mode.
- **Addressed messages and thread replies are untouched.**
- **The lane is told apart by the ARRIVING SIGNAL's `kind` in ingest.** No
  `SignalSource` or `SignalAdapter` field declares "chat source", and no
  reconciler decides it. Adding such a handle buys one `if` at the price of a
  declaration every adapter author can get wrong.

#### Capabilities are wiring, exclusively

Two optional stanzas of ordered refs:

| Stanza | Points at | Is |
|---|---|---|
| `spec.toolsets` | `MCPToolset` | the allowlist |
| `spec.mcpConfigs` | `MCPConfig` | the MCP servers |

**`spec.toolsets.mode`** (`merge` | `overwrite`, default `merge`) composes
against the **AGENT DEFINITION** — the `tools:` frontmatter of
`.claude/agents/<agent>.md` in the profile's REPO.

- **Never against the profile**, which carries no capabilities. Mistaking the
  profile for the counterpart is what deleted this field once already.
- **`spec.mcpConfigs` has NO mode.** A definition declares no MCP servers, so
  there merge/overwrite really would be one behavior wearing two names.

**Refs apply in order.** Tools concatenate with dedup, server keys overlay
(later wins). Content stays in the referenced CRs, and Ready validates both ref
sets.

#### WIRING IS MANY-TO-MANY, IN EVERY DIRECTION. THIS IS THE MODEL, NOT A HAZARD

- A Pipeline claims MANY sources and delivers to MANY channels.
- A source is claimed by MANY Pipelines.
- A channel carries MANY Pipelines' conversations.

- **There is no exclusivity anywhere** — no conflict condition, no tiebreak,
  nothing to warn about. Two agents on one surface, or on one source, are
  ORDINARY CONFIGURATIONS an adopter chooses.
- **Any advice reading "prefer a source of its own" or "claiming this too would
  cost you X" is WRONG, and is deleted on sight.** Written three times in this
  repo, reverted three times.
- **The ONLY consequence of several claimants:** an UNADDRESSED chat message is
  answered with the list of agents serving the surface, so the person names one.
  A teaching moment, not a cost, and the whole of it.

#### CLAIMING AND ADDRESSING ARE INDEPENDENT MECHANISMS

| Mechanism | Is | Checks |
|---|---|---|
| **CLAIM** (`signalSourceRefs`) | who answers an UNADDRESSED message | read from Ready pipelines only |
| **ADDRESSING** (`/<pipeline> <task>`, `router.go` `HandleCommand`) | reaching one by name | a plain `Get` BY NAME — no claim check, no Ready check |

**`boundChannels` folds the originating channel in.** The reply lands in the
thread it was asked from, whatever the addressed Pipeline declares.

Two consequences that decide how bundles wire themselves:

1. **Several pipelines share ONE surface without sharing its source.**
2. **Listing a chat source on a Pipeline that is only ever addressed grants that
   Pipeline NOTHING**, while making every unaddressed message on that surface
   ambiguous — which the bare-chat lane answers by REFUSING.

`/pipelines` lists Ready pipelines only, so an addressable Pipeline stays
discoverable whether or not it claims anything.

#### REACHED, NEVER NAMED

A Pipeline is reached two ways and no others:

1. A signal posted to a source it CLAIMS.
2. A `/<pipeline>` chat command on a wired surface.

- **There is NO HTTP form that names a Pipeline.** `POST /task` was deleted, not
  renamed, because a caller selecting its own wiring is the shape this CRD
  exists to prevent.
- **There is likewise no profile-addressed form and no per-profile default.** A
  Pipeline declaring no bindings grants nothing, and that is a configuration,
  not a defect to warn about.
- **Every Pipeline the CHART ships must therefore declare its own tools.**
  Forgetting that is what made every signal-driven conversation toolless once.
- **Consequence: runtimes are generic** — one `AgentRuntime` per vendor × trust
  level. The SA stays runtime-level on purpose: a Pipeline choosing an SA would
  make pipeline-edit rights a privilege escalation.

### `MCPToolset`

**A pure LIST of tool patterns** (`spec.tools`).

- **No servers, no status.** Patterns are opaque, passed through like
  `allowedTools`. Servers live ONLY in `MCPConfig`.
- **Manager RBAC on it is read-only.**
- **Bound from `Pipeline.spec.toolsets` ONLY** — capabilities are wiring, never
  profile fields.

**What the pipeline binds is HALF the allowlist.** The RUNTIME composes it with
the agent definition's own `tools:` per the unit's `toolsMode`, since it alone
holds the checkout.

Verified against the CLI:

- **`--allowedTools` is the sole permission authority**, and a definition's
  `tools:` neither widens nor narrows the main session. The union must be built
  here or it does not happen.
- **Never pass `--agent <name>`.** That re-applies the definition as an
  availability INTERSECTION and silently defeats `overwrite`.
- **No `|| 'Read'` fallback.** Empty is passed as empty, with
  `--permission-mode dontAsk` — a permission prompt in a pod is a hang.

**The chart ships the built-in vocabulary risk-split** under
`global.builtinToolsets` (`agentops-observe` / `-shell` / `-edit`). `global.`
because subcharts read no other parent scope.

**A `kind: task` signal posted to a source X claims carries X's bindings** —
channels AND tooling both. Reaching a pipeline gets its wiring, not half of it.

**Multi-channel conversations.** The manager:

- Fans replies and acks to every bound thread.
- Delivers a user message to every bound channel EXCEPT the surface that
  displayed it (attributed text, per DESTINATION — not "siblings").
- Dispatches once ≥1 thread binding exists.

**The OPERATOR delivers.** Agent output reaches every bound thread through the
manager's adapters, single- and multi-channel alike.

- **Agents never post to a transport**, and Channel carries no delivery mode.
- **So prompts carry no transport steps**, and runtimes hold no channel
  credentials.

### Channel adapter

**Out-of-process channel-type implementation consuming `/channel/*`** — ops
long-poll plus inbound push.

`Channel.spec` is two halves:

| Half | Holds |
|---|---|
| type-agnostic metadata | `adapter`, `credentialsSecretRef` — NO wiring, NO delivery mode |
| opaque `config` | only the serving adapter interprets it |

`status.threadId` is an opaque STRING.

#### READ IS PER THREAD, THEREFORE PER CHANNEL

`status.threads[].readAt` + `.readTracked`, written ONLY by the manager on an
adapter's report to the OPTIONAL `POST /channel/read`.

- **One shared mark would let a Telegram reader clear the console's**, which is
  the whole reason it sits on the binding.
- **The watermark is MONOTONIC and CLAMPED to the manager's clock.** A stale
  browser must not un-read a thread, and a skewed one must not mark the future
  read.
- **A report that would not advance is `skipped` with NO write.**
- **The batch is bounded at 50.**
- **`readTracked` is stamped on EVERY binding the manager creates**, for every
  channel, so the backfill rule stays ONE rule: a binding without it predates
  the mechanism and is READ. Same shape, same fix, same reason as
  `status.runs[].deliveryTracked` — and without it the first upgrade shows the
  whole namespace as new.
- **An adapter that never reports stays fully conformant.**

### `ChannelAdapter` CR

**Pure implementation** — `image` plus workload knobs. NEVER configuration or
credentials: no `type`, no `env`.

**Interface METADATA is allowed and encouraged:**

- `configSchema` — JSON Schema for the served CRs' `config`.
- `credentialKeys` — docs only, the manager reads no Secrets.

No config VALUES, connectivity, or credentials.

**Its reconciler owns the adapter Deployment**, and — when `spec.port` is set —
the Service, which is named after the WORKLOAD: `agentops-adapter-<name>`.
**There is no `agentops-channel-<name>`.** Two changes have now written that
name by mistake.

**`spec.kubernetesAccess` mirrors SignalAdapter's:** mounts the SA token and
injects `POD_NAMESPACE`, IDENTITY ONLY. Permissions stay an external grant
against SA `agentops-adapter-<name>`, and no reconciler ever creates RBAC.

**Credentials are per-surface** on `Channel.credentialsSecretRef`.

- **Projected into the adapter pod** as `envFrom`, prefix
  `AGENTOPS_CRED_<CHANNEL>_`, resolved by the KUBELET.
- **The contract's channel listing advertises `credentialEnvPrefix`.**

### `SignalAdapter` CR / signal adapter

**The same pattern for ingest, but inbound-only** — no ops queue.

Adapters push normalized signals (`fingerprint`, `labels`, `title?`, `payload`,
`kind: alert|job`) to `/signal/inbound`.

- **Grouping, cooldown and recurrence stay MANAGER-side** from
  `SignalSource.spec.grouping`. Adapters normalize, the manager groups.
- **Workload names `agentops-signal-<name>`.** Token derivation context is
  `signal-adapter:<name>`, never interchangeable with channel tokens.
- **There are NO built-in signal types.** The manager hosts no signal
  transports, so every type needs a serving adapter.
- **`SignalAdapter.spec.port` is an implementation property.** When set, the
  reconciler owns the Service `agentops-signal-<name>` and injects
  `LISTEN_ADDR`. Charts ship NO adapter connectivity.
- **`spec.kubernetesAccess`** mounts the SA token and `POD_NAMESPACE` for
  implementations that self-register with their SENDER. Push-model senders hold
  the "where to push" binding, so the adapter writes it from
  `SignalSource.spec.config.register`, degrading to instructions in the Ready
  condition when it can't.

### On both adapter kinds, the CR NAME is the ROUTING KEY

`Channel` / `SignalSource.spec.adapter` names the serving adapter.

- **A REFERENCE, not an attribute.** That adapter's implementation defines the
  schema of the sibling `config`.
- **It drives** the contract listing `?adapter=`, the injected `ADAPTER_NAME`,
  credential projection, token scope and `Served`.
- **One adapter per implementation by construction** — duplicates for one
  implementation are impossible.
- **Adapter CRs carry NO configuration.** Connectivity, credentials and config
  live only on Channel and SignalSource.

### API group

**`agentops.dev/v1alpha1`.** Provisional — a rename is possible pre-1.0.

## Build / test

```sh
go build ./... && go vet ./...
for m in channel-telegram telegram-router signal-telegram signal-cron \
         signal-alertmanager signal-k8s-events signal-ha; do
  (cd $m && go build ./... && go vet ./... && go test ./...)
done
# regen after editing api/v1alpha1/ (deepcopy + CRDs):
go run sigs.k8s.io/controller-tools/cmd/controller-gen@v0.16.5 object paths=./api/...
go run sigs.k8s.io/controller-tools/cmd/controller-gen@v0.16.5 crd paths=./api/... output:crd:artifacts:config=chart/files/crds
# full tests (unit + envtest against a real API server):
KUBEBUILDER_ASSETS=$(go run sigs.k8s.io/controller-runtime/tools/setup-envtest@release-0.19 use 1.31.x --bin-dir ~/.envtest -p path) go test ./...
```

### No local Go: use a PERSISTENT container, not `docker run --rm`

**This workstation has no Go toolchain**, so every command above runs in a
container.

**Start ONE long-lived container and `docker exec` into it.** A throwaway
`docker run --rm` pays container setup per invocation and throws the build cache
away with it — warm rebuilds are ~2s through `exec` and are not through
`run --rm`.

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

- **The caches MUST be named volumes, never host bind mounts.** The module cache
  does heavy rename and hardlink work, and bind mounts through the Rancher
  Desktop VM corrupt it — every package fails with `zip: not a valid zip file`
  on a cache that was written seconds earlier. The repo itself is still a bind
  mount, because it must be edited from the host.
- **Run as the invoking uid.** `controller-gen` writes deepcopy and CRDs INTO
  the repo, and a root-owned generated file is a mess to undo.
- **Mount the repo at its REAL path**, not `/src`. Compiler diagnostics then
  carry paths that resolve on the host.
- **`go clean -modcache` fails** (`unlinkat //gomodcache: permission denied`) —
  it tries to remove the mount point. Remove the VOLUME instead.

**A VM-BACKED DAEMON MOUNTS YOUR HOME, NOT `/tmp`.** Rancher Desktop runs the
daemon in a VM, so `-v /tmp/whatever:/data` bind-mounts an EMPTY directory.

- **The container runs, finds nothing, writes nothing and often says nothing.**
  It reads as a broken image rather than a missing mount.
- **`docs/diagrams/export.py` builds its scratch directory BESIDE ITSELF** for
  exactly this reason. Anything else running a container over generated files
  must do the same.
- **`docker pull` from a non-interactive session fails** with
  `gpg: decryption failed`, before it ever reaches the registry — the `pass`
  credential helper needs an unlocked gpg agent.

Two traps that are not the container's fault but look like it:

- **`go build ./...` piped into `tail` reports `tail`'s exit code.** Check
  `${PIPESTATUS[0]}` or redirect to a file.
- **`openspec` needs Node**, which is likewise not installed system-wide —
  `~/.local/opt/node`, symlinked into `~/.local/bin`.

### EVERY IMAGE IS MULTI-ARCH WHEREVER IT CAN BE

**Use `buildx --platform linux/amd64,linux/arm64 --push`.**
Never `docker build --platform linux/amd64`.

**A single-arch image fails at SCHEDULE TIME, not at build, push or render** —
possibly weeks later, looking like an unrelated incident:

```
failed to pull and unpack image "...": no match for platform in manifest: not found
```

- **An amd64-only `agentops-console` did exactly that on 2026-08-21.** It had
  run for weeks only because every reschedule landed on an amd64 node; the first
  that did not left the console in `ImagePullBackOff`.
- **Nothing in the chart or the CR was wrong.** The image had no arm64 half.
- **Every adapter here is dependency-free Go** and cross-compiles for free, so
  there is no reason to ship one arch.

**THERE IS NO EXCEPTION.** This file used to name one — "a runtime whose
UPSTREAM is single-arch, `runtime-claude` is the case" — and it was wrong.
Building the image settles it:

```
docker buildx build --platform linux/arm64 ./runtime-claude/     # succeeds
docker run --platform linux/arm64 … claude --version
arch: aarch64 / v22.23.2 / 2.1.239 (Claude Code)
```

`node:22-bookworm-slim` plus apt plus one npm global is multi-arch throughout.

**The constraint was the `--platform linux/amd64` in the build command below.**
The runtime `nodeSelector` the chart ships compensates for a build flag rather
than for the vendor, and can be relaxed once a multi-arch runtime image is
published.

**A component may still be single-arch one day.** Establish that by BUILDING it
on the other architecture and running the binary, never by inheriting a claim
from prose — including this prose.

**Bump the tag on every change. Never overwrite a pushed tag.**

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
$BX -t <registry>/agentops-signal-alertmanager:<tag> ./signal-alertmanager/
$BX -t <registry>/agentops-signal-k8s-events:<tag> ./signal-k8s-events/
$BX -t <registry>/agentops-signal-ha:<tag> ./signal-ha/
$BX -t <registry>/agentops-console:<tag> ./console/
$BX -t <registry>/agentops-context-sync:<tag> ./context-sync/
$BX -t <registry>/agentops-egress-proxy:<tag> ./egress-proxy/
$BX -t <registry>/agentops-housekeeping:<tag> ./housekeeping/
$BX -t <registry>/agentops-runtime-claude:<tag> ./runtime-claude/

# VERIFY before believing it — the failure mode is invisible until it schedules:
docker manifest inspect <registry>/agentops-console:<tag> \
  | jq -r '.manifests[].platform | "\(.os)/\(.architecture)"'
```

Then:

1. **Update the image refs** — chart values for the manager, `AgentRuntime` CRs
   for runtimes.
2. **`helm upgrade`.**
3. **Verify with a live task.** A task is an ordinary signal to a source a Ready
   Pipeline claims. There is no `/task` endpoint.

```sh
TOKEN=$(kubectl -n <ns> get secret agentops-adapter-token \
  -o jsonpath='{.data.token}' | base64 -d)
curl -sX POST http://<manager>:8080/signal/inbound -H "Authorization: Bearer $TOKEN" \
  -d '{"source":"<src>","signals":[{"fingerprint":"smoke-1","kind":"task","payload":"..."}]}'
```

Point the claiming Pipeline at a stub runtime and it costs no LLM.

## Map

### The operator

| Path | Holds |
|---|---|
| `api/v1alpha1/` | CRD types plus generated deepcopy. CRD YAML in `chart/files/crds/` |
| `cmd/manager/main.go` | wiring: reconciler, httpapi, chat registry/ops/router, env config |
| `internal/ingest/` | signature grouping, fingerprint cooldown |
| `internal/runtimepod/` | runtime pod builder (AgentRuntime CR over bootstrap Config) |
| `internal/addressing/` | `/<pipeline> <task>` parsing — ONE segment. The `:<agent>` override is deleted: a Pipeline names one profile and a profile names one agent, so the agent comes from the wiring, and a sender picking their own reached past it |
| `internal/integration/` | envtest suite — real API server, fake chat, no kubelet |
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
    prometheus-bundle and signal-cron depend on.
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

### `runtime-claude/`

The reference `AgentRuntime` — Node plus claude-code, implementing the `/work`
contract.

- **GENERIC BY CONSTRUCTION** — git and openssh for the checkout it owns, plus
  generic shell utilities, and NO domain tooling. kubectl was dropped in image
  0.5.0.
- **A CLI here is the same category error as bundling an MCP server.** What an
  agent may reach is WIRING.
- **Need one? Derive an image** and point `AgentRuntime.spec.image` at it
  (README). That is why the field exists.

### The Telegram trio

**`channel-telegram/`** — the reference channel adapter, implementing the
`/channel` contract. Bot API sending lives HERE.

**It does NOT poll.** It receives topic updates pushed by the router
(`POST /updates`, `ChannelAdapter spec.port`) and persists the router's offset
(`GET/PUT /offset` → state API).

**`telegram-router/`** — the ONLY getUpdates consumer.

- **It classifies each update on `is_topic_message` and forwards it VERBATIM.**
  No topic → `signal-telegram` (origination). Topic → `channel-telegram`
  (continuation).
- **It holds no channel config, persists nothing, needs no RBAC.**
- **NOT AN ADAPTER.** It emits no signals, so it has no `SignalAdapter` CR and
  no served CR.
  - **The telegram-bundle chart owns its Deployment**, injecting
    `SIGNAL_TARGET` / `CHANNEL_TARGET` / the bot token as env.
  - **It never contacts the manager.**
- **One Deployment per bot token makes the single-consumer rule structural.** A
  missing env var exits at startup.

**`signal-telegram/`** — the chat ORIGINATION adapter.

It normalizes general-surface updates and posts `/signal/inbound`:

```
{kind: chat, fingerprint: tg-<update_id>,
 labels: agentops.dev/channel + /sender}
```

**It holds NO credentials** — it never contacts Telegram.

### The other signal adapters

**`signal-cron/`** — the reference signal adapter, implementing the `/signal`
contract. Five-field cron parser plus scheduler.

**`signal-alertmanager/`** — webhook-receiving signal adapter. Hosts `/webhook/{source}` for Alertmanager-format posts, and the
prometheus-bundle subchart ships it.

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

**`signal-ha/`** — Home Assistant log signal adapter.

- **Reads that instance's WebSocket API** over a hand-written RFC 6455 client:
  `system_log_event`, with `system_log/list` for backfill and for the dwell
  re-check's evidence.
- **NO Kubernetes client at all.** `kubernetesAccess: false`, credential
  projected per SOURCE.
- **Same `rules`/`route` vocabulary as `signal-k8s-events`**, minus the time
  axis.
- **The fingerprint keys on LOGGER + SOURCE LOCATION** — Home Assistant's own
  dedup identity — never on the occurrence.
- **Verification ladder:** config-entry state, then recurrence.
- **Its loop breaker is the agent SURFACE** (`mcp_server`, `api`,
  `websocket_api`), because a failed agent call is logged there and reporting it
  would wake the agent that made it.

**`signal-k8s-events/`** — cluster Events signal adapter.

- **In-cluster API over `net/http`**, no client-go: SA token re-read, list+watch
  per namespace scope, 410 relist.
- **Needs `kubernetesAccess: true`.** The CHART binds its events RBAC — the
  operator grants adapters nothing.
- **The fingerprint keys on the involved OBJECT + reason**, never the Event
  object. Kubernetes recreates those per recurrence.

### Beside a conversation

**`housekeeping/`** — the disk half of conversation retention.

- **A CronJob, not a daemon.** It scans the claim ROOTS for workspace
  directories and session transcripts no Conversation backs, and removes them.
- **READ-ONLY on the API** — both etcd-side stages are the manager's — with its
  OWN SA and no agent code in the image.
- **Its own workload because THE MANAGER MOUNTS NOTHING** — see the invariant.
- **Every run is bounded** (`maxDeletions`) and `dryRun`-able.
- **The listing is PHASE-BLIND** — see the invariant.
- **Named `agentops-housekeeping`** so `signal-k8s-events`' prefix
  self-exclusion catches it. A CronJob fails on a SCHEDULE.

**`egress-proxy/`** — the tool-access wall INSIDE the runtime pod.

- **ONE binary, two subcommands:** `install-redirect` (privileged init
  container, writes the redirect rules) and `proxy` (serves the redirected
  connections). One binary because the two must agree exactly on the port and
  the uid, and a second image is a second place for that agreement to rot.
- **It exists because `--allowedTools` configures a COOPERATING agent.** One
  with a shell can open a socket to a bound MCP server and call anything that
  server registers. This is the wall for an agent that does not cooperate.
- **It terminates no TLS, inspects no non-MCP byte and holds no Kubernetes
  credential.** Other traffic is copied through untouched.
- **Opt-in via `runtime.egressMediation.enabled`** — see
  `docs/adr/0001-bound-component-reach.md`.

**`context-sync/`** — the context sidecar. Semantics in the terminology entry;
what lives HERE is the implementation:

- **Atomic generations plus a `current` symlink.** Copies are labelled quiesced
  or best-effort.

### `console/`

**The agent-ops console** — a ChannelAdapter that is ALSO the viewer.

- **Config from read-only list/watch of the eight agentops kinds**, in-cluster
  API over `net/http`, the same technique as `signal-k8s-events`.
- **Conversation traffic from the ordinary `/channel/*` contract.**
- **Embedded SPA via `go:embed`** — no npm at runtime.
- **NO write path to the Kubernetes API exists in the module.** The only write
  anywhere is `POST /channel/inbound`.
- **Needs `kubernetesAccess: true`** for identity, plus the CHART's read-only
  Role against SA `agentops-adapter-console`.
- **Conversations carry no `pipelineRef`**, so pipeline attribution is INFERRED
  from the materialized bindings and left blank when ambiguous. Never guessed.

### `chart/charts/k8s-bundle/`

The cluster Events lane and the Kubernetes tooling.

| Component | Renders |
|---|---|
| events | adapter + RBAC + `SignalSource`. **The events component renders the source, never the claim on it** |
| profile | the `k8s-engineer` profile — ONE object, identity only |
| `pipelines` | the WIRING component — see below |
| `mcp` / `mcpServers` | the `MCPConfig`, the toolsets and the server workload — see below |

**The `pipelines` component is the one bundle route that ships**, because it
owns its whole lane: at most ONE Pipeline claiming its own source with its own
profile and toolsets, channels values-supplied and omitted when unset.

- **OFF inside an active bundle and forced on by `global.demo.enabled` ALONE**,
  which is why `pipelines.enabled` is nullable — an explicit `false` must
  decline the route under demo too.
- **WHICH route is a fourth derivation from
  `global.agentops.runtime.rbacMode`.** `full` renders the acting `k8s-operate`
  (binds `k8s-admin`), everything else the observing `k8s-observe`.
- **Per-route booleans win both ways**, and both at once is ALLOWED and fans
  out.

**NO substrate**: no AgentRuntime, no runtime SA, no credential, no runtime
RBAC. All of that is the parent's `runtime:` + `global.agentops.runtime.*`.

**The profile has no repository**, so it carries an inline `systemPrompt` role.
Otherwise an event wakes a personality-free agent.

**Self-gated on `enabled OR global.demo.enabled`.** Demo mode IS this bundle —
`chart/templates/demo.yaml` is gone.

The `mcp` component:

- **`MCPConfig` `k8s-api`**, server key FIXED at `kubernetes`.
- **TWO MCPToolsets split by risk** — `k8s-observability` (14 read tools) and
  `k8s-admin` (6 mutating), ENUMERATED not wildcarded, because
  `mcp__kubernetes__*` spans both halves and defeats the split.
- **`k8s-admin` renders only when a server that REGISTERS those tools exists.**
- **`mcp` and `mcpServers` are ON by default and flip as a PAIR.** The config's
  URL defaults onto the deployed Service, which is the only reason the component
  used to be off. The endpoint guard stays and still fails `mcp.enabled` with no
  server and no `url`.
- **`mcpServers` runs `containers/kubernetes-mcp-server`** (`--read-only`,
  filtering at REGISTRATION not listing) under a SECOND SA `agentops-mcp-k8s` —
  never the runtime SA, and the render fails if they are equal.
- **That second identity IS the component's reason to exist.** MCP reach = the
  server SA's RBAC ∩ the toolset. Two walls.
- **Since runtime 0.5.0 it is also the ONLY cluster path** — no CLI in the image
  — so `mcp.enabled: false` leaves an agent that cannot see the cluster.
- **`readOnly` / `rbac.mode` are null and DERIVE from
  `global.agentops.runtime.rbacMode`.** `full` gives a write-capable server
  under a full SA, anything else a read-only server under `readonly`.
- **Explicit wins.** `readOnly: true` under `full` is a strictly observing
  agent: broad grants on the runtime SA that nothing can exercise.

### `chart/charts/prometheus-bundle/`

**WAS `vm-bundle` through chart 5.12.0.** It ships:

- **The Alertmanager ingest lane.**
- **ONE metrics MCP component.** `MCPConfig` server key FIXED at `prometheus`,
  plus a WILDCARD `MCPToolset` — all six tools the server registers are
  read-only, so unlike k8s-bundle there is no risk split to enumerate. The
  PINNED tag is what keeps the wildcard honest.
- **Its deployable server under a SECOND SA.**
- **The `alert-investigator` profile** — identity only, inline role, no
  repository, so no agent definition resolves.
- **ONE default-off route.**

**NAMED FOR THE PROTOCOL, NOT A VENDOR.** The ingest core reads the standard
Alertmanager payload, and VM answers the Prometheus query API — buildinfo
reports a Prometheus version, and MetricsQL is a PromQL superset — so one server
key serves both backends.

- **The LOGS component is DELETED, not ported.** VictoriaLogs speaks LogsQL and
  no Prometheus server reaches it.
- **Self-registration is KEPT and labelled VICTORIAMETRICS-ONLY.** It writes a
  `VMAlertmanagerConfig`, and vanilla Alertmanager's config is a file, so there
  is no object to write. NOTES.txt prints the receiver stanza instead, with
  `send_resolved: false` because the adapter drops non-firing alerts.
- **The backend URL is NEVER derived.** Single-node VM, cluster mode and
  Prometheus each serve the query API under a different path.
- **NEVER enabled by demo mode.** Every component needs an endpoint no demo
  cluster has, which is why `active` has no demo branch.
- **The retired `vm-bundle:` key FAILS the render.** Helm never reports an
  unread values key, so the rename would otherwise install nothing and look
  successful.

### `chart/charts/ha-bundle/`

The Home Assistant lane and a PRIVILEGE SPLIT:

- **The log ingest lane.**
- **ONE `MCPConfig`**, server key FIXED at `homeassistant`, and NO server
  workload — the house serves its own MCP endpoint.
- **TWO risk-split `MCPToolset`s.**
- **TWO identity-only profiles** — `ha-user` USES the house, `ha-operator` FIXES
  it.

**The split is use-versus-fix, NOT read-versus-act.** Home Assistant has no
read-only role, so both agents act. What separates them is the REST path: Assist
intents reach no configuration, so repairing needs a shell and only the ops
route binds one.

**The OPERATOR credential gates the fixing half AND the ingest lane.**
`subscribe_events` is admin-only, so a control token authenticates and is then
refused the subscription, which reads like a network fault.

- **Never enabled by demo mode.**
- **Two default-off routes, and BOTH claim the chat sources.** Wiring is
  many-to-many, so a shared surface offering both agents is the point.
- **`ha-ops` additionally claims the log source**, which is the only asymmetry.
- **`pipelines.restAccess` is PER ROUTE** — on for ops, off for control.
- **Credentials come as a NAME or as the TOKEN ITSELF.** The token form makes
  the bundle create the Secret and derive BOTH keys (`token` +
  `authorization`), which is what lets a secret manager's ref go straight into
  values.

### `chart/charts/telegram-bundle/`

**The three-component Telegram stack** — router, signal adapter, channel
adapter — as adapter CRs. Under `surface.enabled` it also renders the Channel,
the chat SignalSource and the router's credential source.

- **`surface.enabled` makes the unguessable fields REQUIRED.** A missing
  `chatId`, a missing credential, or BOTH credential forms at once FAIL the
  render.
- **Credentials either way:** `credentials.existingSecret` OR
  `credentials.botToken`, where the bundle creates the Secret.
- **One Secret serves both** — the Channel sends with it, the router's source
  polls.
- **Ships NO Pipeline on purpose**, because wiring drags in a profile, a runtime
  and credentials. The sources sit unclaimed until the installer wires them, so
  NOTES.txt prints the exact Pipeline to apply.

### `chart/` (the parent)

Manager Deployment, RBAC and Service, plus CRDs as gated templates.

- **`crds.enabled` / `crds.keep`** → `helm.sh/resource-policy: keep`, so
  uninstall never cascade-deletes CRs.
- **CRD source of truth is `chart/files/crds/`** — controller-gen output.
- **`templates/runtime.yaml` is THE SUBSTRATE.** One `AgentRuntime` named
  `default`, plus its credential Secret when `runtime.credentialsSecret.token`
  is set, with `home.pvcRef` WIRED from the parent's own `persistence` and never
  copied.
- **`templates/runtime-rbac.yaml`** renders the mode-driven bindings.
- **`templates/_helpers.tpl` resolves BOTH substrate facts from `.Values.global`
  alone**, so a subchart calling them cannot disagree with the parent.

### `docs/`

**Reference pages** — `concepts.md` (CRDs plus capability resolution),
`contracts.md` (work and adapter contracts plus HTTP API), and one page per
bundle subchart.

**The published site's Jekyll source** — `_config.yml`, `_layouts/`,
`_includes/`, `_data/nav.yml`, `assets/` (css, js, vendored Red Hat fonts, the
exported diagrams, the console screenshots and the landing recording).

**`docs/CLAUDE.md` owns how a page is written and built** — structure over
prose, the command tabs, the components a page may name, the table rules and the
pre-flight lint. This file routes what goes where, that one governs how it
reads.

**`docs/CHANGELOG.md`** holds every chart-version migration guide.

- **The ONLY place upgrade steps live.**
- **Newest first**, Keep a Changelog 1.1.0 format.
- **TEN versions.** Older ones move VERBATIM to
  `docs/changelog/CHANGELOG-<range>.md`, linked from its foot.

#### The site's pages

| Page | Owns |
|---|---|
| `index.md` | the hero, what it plugs into, ONE tab strip (the recording of one incident, the diagram, the Pipeline manifest as copyable page text), then the stat tiles, then the sections |
| `introduction.md` | the adopter's orientation — the model, the seams, and NO reference detail |
| `getting-started.md` | THE walkthrough — install and a first answer IN THE CONSOLE |
| `installation.md` | THE REAL install, and the only home the PARENT chart's values have |
| `console-guide.md` | permalink `/console/` — what the console is FOR |

- **`index.md`'s order is not written in the page.** `home.html` SPLITS the
  rendered content at its first `<h2>` and drops the tiles in the seam, so the
  page states its words in order and says nothing about placement.
  - **There is no diagram block in the layout and no `diagram:` front matter.**
    The strip is page content, which is what lets the alt text and the manifest
    be the page's own words.
- **`introduction.md` carries no sentence a field rename would break.** That
  belongs in `concepts.md`.
  - **It is TWO SECTIONS — understand the concepts, follow the guides — and
    stays that way.** Anything else is a guide or a reference page. The
    signal-to-answer lifecycle is the first guide owed, not a section here.
- **`getting-started.md` ends where a getting-started page should:** at
  something working. Wiring is the NEXT card, not its last section — and that
  card owns every expectation, flag and failure mode.
  - **Its test is "would the reader TYPE it or READ it".** What they type is on
    the page, what they read ABOUT is a link. That is why README keeps only the
    commands.
- **`installation.md` puts decisions before commands**, values grouped by the
  decision they serve, bundle values left to their own pages.
- **`console-guide.md` covers the six views and the question each answers**,
  plus the authentication decision an operator makes before exposing it.
  Endpoints, the RBAC grant and the values list stay in the untouched reference
  `docs/console.md`, which keeps its own name and its own URL.

**How a page reads, how its assets are BUILT, and how Pages serves this
directory are `docs/CLAUDE.md`'s** — the build-output rule for the screenshots
and the recording, the recording's silence, and the reference pages' static-file
status are stated there and not restated here.

Two that stay, because they are about the SHELL rather than a page:

- **The FRAMES are the reproducible artifact, not the MP4.** Review the beat
  script and the frames. Nobody is to make the encoder deterministic.
- **`_layouts/page.html` is what `introduction.md` uses**, via
  `jekyll-default-layout`: front matter makes a file a page, and a missing
  layout then DOES fail the build.

#### `diagrams/`

**Holds the drawio SOURCE plus `export.py`.**

**Run that, never the exporter by hand.** It writes BOTH theme variants of BOTH
site pages (four SVGs) and repaints the dark ones' icon ink, which drawio cannot
do because the icons are embedded images.

THREE drawio pages, and only two are exported:

| Page | Is |
|---|---|
| `landing` | the poster's own COMPOSITION, compressed to 950px |
| `site` | the full argument behind its full-size link |
| `why` | the standalone poster — rendered on demand, never committed |

**950 BECAUSE the content column is 720 and there is no breakout.** Displayed
size is type over canvas, so making it fit means REMOVING ELEMENTS and
tightening layout, never shrinking type. Adding detail back is what makes it
unreadable.

#### The site shell

**Astro Starlight's geometry**, read off that site's own stylesheet and verified
against it live: BOTH rails 18.75rem, text 45rem FIXED, and the leftover SPLIT
EVENLY between the left gutter and the right container.

The rail keeps its width at that container's left, so its half is empty space
PAST the rail, never a fatter rail.

**Reproduced with an explicit `--ao-leftover`**, because `minmax(base, 1fr)` on
two tracks does NOT share a remainder: `fr` sizes against the whole free space,
so both tracks come out the same WIDTH. That is what once gave an 810px rail
beside a 66px gutter.

**Body type is 17px on purpose.** Red Hat Text is narrow, and at 16px that 45rem
column reads 99 characters.

#### The DEMO WIRES THE CONSOLE

Where k8s-bundle renders a route, that route claims the console's source and
binds it as a channel, from `global.agentops.console` — a subchart reads no
other parent scope, and helm cannot derive a value from a value.

- **Those names DUPLICATE `console.signalSourceName` / `channelName`**, so the
  render FAILS when they disagree.
- **Scoped to demo mode**, because `console.enabled: false` is pinned to remove
  every console object with ONE value.
- **The claim rides the EXISTING route.** A second claimant makes every
  unaddressed console message ambiguous.

## Invariants (do not break)

### THE PARENT CHART OWNS THE SUBSTRATE — BUNDLES CONTRIBUTE DOMAIN

How agents execute — image, LLM credential, idle TTL, node placement, home
volume, and the ONE identity whose RBAC is the agent's power — is release-wide
and lives in `chart/values.yaml` (`runtime:` + `global.agentops.runtime.*`).

- **No subchart renders an `AgentRuntime`, a runtime ServiceAccount or a
  credential Secret.** Bundles ship sources, profiles, tooling and channels, and
  REFERENCE it.
- **Both substrate keys are under `global.`** because a subchart can read no
  other parent scope and k8s-bundle's MCP server derives from them. Restating
  them in a subchart recreates the two-spellings-of-one-fact problem chart 4.0
  removed.
- **Putting the runtime in a bundle is what made a chat-only install unable to
  execute anything**, and made TWO runtime SAs exist, one granted everything.

### The manager reads NO secrets — zero Secret API reads

- **Everything secret-shaped compiles to `valueFrom` / `envFrom` in pod specs.**
  The kubelet resolves it.
- **Transport credentials are declared per Channel** (`credentialsSecretRef`)
  and PROJECTED into adapter pods, never read.
- **The adapter auth token reaches the manager via env** (`ADAPTER_TOKEN`).
- **Per-adapter tokens are DERIVED** — HMAC of master plus adapter name,
  validated by re-derivation. Nothing minted or stored.
- **RBAC grants the manager no `secrets` verbs at all.** Keep it that way.

### The operator grants adapters NO Kubernetes permissions, ever

Dedicated SA, and no RBAC objects created or bound by any reconciler.

- **Default posture is `automountServiceAccountToken: false`.**
- **`SignalAdapter.spec.kubernetesAccess` only mounts the token and injects
  `POD_NAMESPACE`.** What it may DO is granted externally, by chart or user,
  against SA `agentops-signal-<name>` — so an adapter CR can never escalate.
- **Name-is-key makes one adapter per implementation structural.** There is no
  conflict machinery to maintain.

### Strictly serial per conversation

- **ONE inflight unit.** Parallelism is across conversations.
- **Capped by `MAX_ACTIVE_CONVERSATIONS`** (default 5), with idle-runtime
  eviction.
- **`MAX_RUNTIMES` is the deprecated alias**, honored one release.

### THE CAP IS DECIDED BEFORE ANYTHING IS PROVISIONED

**"Active" means POD-BACKED and is counted from the live pod list, never from
status.** A pod stuck unschedulable, or a lost status patch, must not invent
capacity.

**A conversation that cannot be admitted gets phase `Pending`:** no runtime pod,
no MCP ConfigMap and — the point of the phase — **no `ensure-topic`**.
Suppressing the topic is what stops a burst becoming a thousand chat threads.

- **`Queued` keeps its old meaning** — ADMITTED, waiting behind the serial rule
  — and is never used for capacity waiting. Conflating them is the mistake to
  avoid.
- **Admission is FIFO by creation time over a waiting set defined by PODS, not
  phase.** Keying on phase lets a brand-new conversation reconciled first jump
  an older one.
- **The backlog itself is bounded by `MAX_QUEUED_CONVERSATIONS`** (default 50),
  checked in INGEST rather than the reconciler because the point is not to
  create the object at all. It gates CREATION only, so window reuse keeps
  appending to a pending conversation.

### `/close` sets a PHASE — DELETION IS A SECOND VERB

Closing writes phase `Closed` plus `status.closedAt` and tears down the pod, the
MCP ConfigMap and the capacity slot, archiving every bound thread at the
TRANSITION.

The object, its `status.runs[].result`, its `runtimeContextId` and its volume
state all survive, which is what makes REOPEN mean anything.

**Closing used to delete, and that is exactly why nobody closed anything:** the
only tidying tool cost more than the backlog.

**A closed conversation is INERT:**

- No dispatch, no capacity, no place in the FIFO waiting set.
- **Absent from conversation REUSE** — a matching signature opens a NEW
  conversation. This is the rule that makes closing mean anything.
- Absent from every pipeline.
- **A reply typed into a closed thread is ANSWERED** ("closed, reopen it") and
  creates nothing. Appending an input there would be a black hole, and an
  implicit reopen would re-materialise threads on every bound channel because
  someone typed "thanks".

**REOPEN NEVER RE-RESOLVES REFS.** Phase → `Idle`, `closedAt` cleared,
materialized refs left EXACTLY as they are.

- **Refs are snapshots whose content is re-read at every use**, so re-resolving
  would let a Pipeline edit re-wire an existing conversation.
- **A missing profile or channel FAILS the reopen naming it**, never partially.
- **Threads come back through an ordinary `ensure-topic` carrying
  `previousThreadId` as a HINT.** An adapter that can un-archive returns the same
  id, one that cannot returns a new one and is already correct.
- **`status.reopens` exists so each reopen's ensure-topic op id is distinct.**
  The ids are stable per conversation × channel, so without it the re-establish
  dedups against the original topic creation and never reaches the adapter.

**`close-topic` IS NOW DERIVABLE.** It was the exception only because it was
enqueued while the object was disappearing, leaving nothing to record against.

**`status.threadsArchived[]` marks the done threads**, so an unarchived one is
an archive still owed. Do not re-add the "one non-derivable op" clause.

**The `agentops.dev/close-topics` finalizer survives for the ONE path where the
object really does go away** — a direct `kubectl delete` of a conversation
nobody closed — with its 2-minute grace so a down adapter can never wedge a
deletion.

`/close` is intercepted on the REPLY path before the text could become an input,
and answers with usage on a general surface.

**Delete and reopen are MANAGER VERBS whose reach is the BINDING**
(`spec.channelRefs`, read off the conversation, never off the request), and
delete REFUSES anything not already `Closed`.

**That is what the retired "no remote close verb exists" rule protected** —
*you may only end a conversation you are PART of*, with a live thread as the
proof. A closed conversation has none, so the binding is the next-strongest.

### `/exit` RELEASES THE RUNTIME — `/close` ENDS THE CONVERSATION

**One word apart and not interchangeable.** `/exit` deletes the runtime POD and
nothing else: object, threads, inputs, runs and `runtimeContextId` all survive,
and the next input admits it again with a fresh pod.

**It exists for the half eviction cannot serve.** Eviction only runs when
something is WAITING, so with nothing waiting an idle pod holds its slot, its
checkout and whatever the runtime keeps resident until the idle TTL.

That wait is longest on exactly the installs that RAISE that TTL, for a big
checkout or a warm local model.

**`dispatch.NeedsWorker` is THE ONE definition of idle**, shared by the command
and the eviction path.

The controller's private `needsWorker` is gone, and restating it either side is
the regression to avoid — the two disagreeing surfaces as a bug report about the
cap, far from both.

**REFUSED MID-RUN, on correctness grounds, not politeness.** An inflight run
still needs a worker, so the replacement pod:

1. Is created AT ONCE.
2. Gets nothing from `/work`.
3. Idles out the LONG TTL and is reaped as `Succeeded`.
4. Clears `Inflight`, makes the input pending again and **RE-RUNS work that may
   already have acted**.

**`/close` owns abandonment, and owns it safely.** Queued input is refused too,
merely because the pod would come straight back.

**What the release COSTS is computed, never guessed.**
`ResolveFor(...).ContinuityPossible()` — the same call dispatch uses — decides
whether the reply promises the context or warns it starts fresh.

**A Pipeline named after a manager command** (`pipelines`, `agents`, `exit`, `close`, `help`,
`start`) **is unreachable by that command.** Interception precedes the Pipeline
lookup, which is what makes the commands reliable.

### A RUNTIME POD THAT NEVER STARTS IS REAPED, NEVER EXEMPTED FROM THE CAP

Reaping used to handle `Succeeded` and `Failed` only.

`Pending` is COUNTED as active — correctly, since a stuck pod must not invent
capacity — but nothing bounded how long it could sit there, so five pods behind
a corrupt filesystem held an entire install for fifteen hours on 2026-08-20.

- **The fix is a start DEADLINE after which the pod stops existing**, which
  frees the slot through the DELETE watch that already promotes the FIFO-first
  waiter.
- **Un-counting it instead is the invent-capacity mistake** and would provision
  past the cap against resources the cluster has not released.
- **The condition carries the KUBELET'S OWN REASON, verbatim.** A message
  reading only "deadline exceeded" reproduces the real failure — fifteen hours
  in which nothing said what was wrong — with a timer attached.
- **Classification comes from POD STATUS alone.**
  `PodReadyToStartContainers` is the discriminator: false exactly while a volume
  will not attach, true before image pulling begins. So the manager needs no
  event-read RBAC for it.
- **A conversation inside its start-failure BACKOFF is skipped by the admission
  waiting set.** Leaving it at the FIFO head reproduces the outage one layer up:
  the oldest conversation cannot start, and everything behind it waits on a slot
  nobody will take.

### ONE STORAGE BREAKER, TWO EDGES

`internal/storagebreaker` treats unavailability as an OUTAGE before a LOSS, and
it is fed BOTH by runs that report an unreachable context AND by pods that
cannot be provisioned for a storage reason.

- **It lived in `httpapi` watching only the first**, which is why it never fired
  for the incident it was written for: no pod started, so no run existed to file
  a report.
- **A SECOND breaker would be worse than none** — two judgements about whether
  storage is down, disagreeing at the worst moment.
- **Only STORAGE-attributable provisioning failures count.** An unschedulable
  pod or an unpullable image opening a storage breaker would hold every
  conversation in the install for a reason that has nothing to do with storage.
- **While open:** admit nothing, hold in `Pending` with a reason that says
  STORAGE rather than queue, and re-test with ONE canary.
- **The provisioning edge cannot close its own breaker** — no pod means no run
  means no success to report — which is the whole reason `ProbeDue` exists.

### CONDITION TAINTS ARE NOT DRAINS

`node.kubernetes.io/not-ready`, `unreachable` and the pressure taints are
applied by Kubernetes from node CONDITIONS.

- **Reading them as a drain releases runtime pods during a transient NotReady**,
  and during a partition across many nodes at once — precisely when acting on a
  stale view is least affordable.
- **Only `spec.unschedulable` and taints outside that set** mean a node is being
  taken down deliberately.
- **Drain awareness is OFF by default and gated on `rbac.drainAware`**, because
  seeing a cordon means reading NODES and every other permission this manager
  holds is namespaced.
- **It shrinks the corruption window, it does not close it.** The storage
  provider picks where a shared volume is served independently of where runtime
  pods run.

### THE RECLAIMING JOB'S LISTING IS PHASE-BLIND, ON PURPOSE

`housekeeping/` removes workspace directories and session transcripts with no
`Conversation` behind them.

**A CLOSED conversation still HAS a CR**, so its state is protected by the same
rule that identifies an orphan.

The job needs no phase knowledge at all, and an "only look at live ones"
optimisation would reclaim the state of every conversation an operator was
keeping.

**Two orderings, each a correctness argument:**

| Target | Order | Because |
|---|---|---|
| Workspaces | scan the disk FIRST, list SECOND | the CR always predates its directory — the pod that creates it exists only for a conversation that already exists |
| Transcripts | the OPPOSITE, plus a grace period | the context handle is written AFTER the file exists |

**It runs under its own SA** — mounting the claim ROOT is the reach `subPath`
isolation denies agents — and the render fails if that SA equals the runtime's.

### FILESYSTEM STATE GOES ON A PVC — EVERYTHING ELSE IN THE KUBERNETES API

**And THE MANAGER MOUNTS NOTHING.** A claim would pin it to one node, defeat
rescheduling, and be a second source of truth beside the CRs — which is the
failure mode this rule exists to name.

Manager state is therefore always one of three things:

1. A cache of a Kubernetes object.
2. DERIVABLE from Kubernetes objects.
3. Declared lossy telemetry.

**State fitting none of the three is a defect.** The matrix in
`docs/concepts.md` is where its row goes.

Consequences that were each a real loss:

- **The reply is a FACT, not a queue entry** — `status.runs[].delivered[]` per
  bound thread plus `.deliveryTracked`, a stable op id
  `send:<conversation>:<channel>:<runId>`, and a reconciler backstop. Otherwise
  `/work/done` enqueueing into an in-memory queue means a restart drops an
  answer already durable in `status.runs[].result`.
  - **Marking happens on op COMPLETION.** Mark on enqueue and a lost op is never
    re-derived.
- **A run with no `deliveryTracked` is BACKFILLED as delivered, never sent.** It
  predates the mechanism, and no timestamp can tell it from a run lost to a
  restart, since both completed before the current process started. Without it,
  upgrading re-posts every recent answer to every bound thread.
- **A person's WORDS are a fact too** — `status.runs[].inputs[]`. Never declared
  lossy and lost anyway, because the only copy lived in a queue built to be
  pruned. Own invariant below.
- **Cooldown lives on `SignalSource.status.cooldown[]`**, written only when a
  fingerprint is ADMITTED. A suppressed re-delivery must stay free, or the
  high-volume case cooldown exists for becomes a write storm.
- **`close-topic` is DERIVABLE now** — from a bound thread missing from
  `status.threadsArchived[]`. It stopped being the exception when closing
  stopped deleting: the object survives, so there is something to record
  against.
- **Telemetry is the declared-lossy class and must REPORT its gaps.** A cursor
  from a previous process is `>= next` in the new one's sequence, so answering
  it with an empty list reads as "nothing happened" — the case eviction alone
  does not catch.

### CONVERSATIONS ORIGINATE ONLY FROM SERVED SIGNAL SOURCES

**A channel CARRIES conversations, it never starts one.**

- **`/channel/inbound` is reply-only** — `threadId` REQUIRED, unknown threads
  dropped, no adoption.
- **A message on a chat's general surface arrives as a `kind: chat` signal from
  a chat `SignalSource`**, so who answers is DECLARED by the Pipelines listing
  it: ALL of them for any other kind, and for a bare chat message only when
  there is exactly one.
- **There is no channel default profile and no `PipelineForChannel`.** Channels
  are shareable on purpose, so "which pipeline answers for this channel" has no
  defensible answer, and the oldest-Ready tiebreak that used to supply one is
  gone.
- **`PipelineForSource` is gone too**, replaced by the plural
  `PipelinesForSource`. A caller wanting ONE answer must now say what it does
  with several.

**The chat lane:**

- Task inputs, never `job` — that resumes sessions.
- Cooldown OFF by default.
- NO signature grouping unless `signatureLabels` is set. Chat keys on the
  fingerprint, and the default alert labels would hash every message alike into
  one conversation.
- **Commands whose whole result is a reply** (`/pipelines`, unknown pipeline, usage
  error) emit a send op and create nothing.
- **A chat signal MUST carry `agentops.dev/channel`.** `/signal/inbound` refuses
  one it could not answer.

### HTTP API is NOT leader-gated

`NeedLeaderElection()=false` — webhooks must serve during rollouts.

**Exactly one getUpdates consumer per bot token, ever.** That consumer is
`telegram-router`: ONE poll loop per Deployment and ONE Deployment per token
(replicas 1 + Recreate, chart-owned).

**Neither adapter polls, and the manager has no poller.** Adding a poll loop
back to `channel-telegram` is the mistake that produces 409s and stolen
updates.

### Channel ops are at-least-once

**`spec.config` is opaque to the operator.** Never parse channel-type config
manager-side — adapters validate their own and report via the Channel Ready
condition.

**The manager never *interprets* config, but it MAY apply an adapter-declared
`configSchema` mechanically** (`internal/configschema`, the one place config
content is touched): advisory-only `ConfigValid`, no type knowledge, adapter
stays authoritative.

### THE MANAGER COMPOSES MEANING, ADAPTERS COMPOSE PRESENTATION

**No transport dialect anywhere in `internal/`** — no `<b>`, no `&lt;`, no
`parse_mode`.

**An op carries a TYPED message** (`signal` | `answer` | `relay` | `notice`,
prose in a named markdown subset) **or a TOPIC DESCRIPTOR, never rendered
text.** There is no `op.text` and no `op.title`.

- **Escaping, length limits, splitting and topic naming belong to the component
  that knows them.** Telegram caps messages at 4096 and topics at 128, nothing
  else does, and a manager-side fix would be one transport's limits imposed on
  all of them.
- **In-process providers are held to the same contract.** They are a second
  renderer, not an exemption.
- **`/channel/ops` REFUSES an adapter that does not declare `contract=`**,
  because one reading the retired `text` field would post empty messages and
  look healthy doing it.
- **`router.go` used to open with "transport-neutral" and then emit Telegram
  HTML.** That is the habit this invariant names.
- **It binds the AGENT too.** `dispatch/templates/format.md` tells it to write
  the same markdown subset, because an adapter escapes what it is given — the
  first version of this change left format.md on HTML and every agent answer
  reached Telegram with its tags showing.

### A thread opens with the event that caused it

**DELIVERY IS DECIDED PER DESTINATION.** Every input is delivered to every bound
channel EXCEPT the surface it entered on, because that surface displayed it as
it was typed.

**ONE rule, ONE implementation** (`chat.DeliverInputs`), called from two places:

1. **The reconciler** — the backstop that makes it derivable.
2. **The router**, the moment an input is appended — the fast path that keeps a
   thread in the order things happened.

Exactly as a run's reply is.

**"Already seen" is a fact about a SURFACE, never about a message.** The
origin-KIND rule (`InputItem.PostToChannels`: `signal` posts, `channel` does
not, `kind: chat` does not) and its stated chat exception are DELETED.

**They asked the question once, per MESSAGE**, and so withheld a person's words
from channels that had never shown them — a console transcript beginning at the
agent's answer was that bug. Re-adding either, in any layer, is the
regression.

- **Whether the origin surface displayed it is TRANSPORT knowledge**, declared
  by the implementation: `ChannelAdapter.spec.echoesOwnMessages`, default TRUE,
  and FALSE on a viewer that renders only what it is sent — which is why the
  console receives its own users' messages. An unreadable channel or adapter
  answers TRUE, the conservative half.
- **The SURFACE itself is resolved in one place for both lanes**
  (`InputItem.OriginSurface`): a channel origin names its channel, a chat signal
  carries `agentops.dev/channel` in its labels.
- **What ARRIVES depends on who said it.** An event is a `signal` card, a
  person's words are a `relay` keeping `origin` and `sender` structured.
- **An ABSENT origin is delivered nowhere**, so upgrading cannot spray history
  into every open thread.
- **Op ids stay stable per conversation × input × channel**, or every reconcile
  reposts everything.
- **A card names its pipeline from `chat.PipelineForConversation`**, which READS
  `spec.pipelineRef` and falls back to binding-matching only for conversations
  predating it, omitting the name when even that is ambiguous.

### A CONVERSATION'S MESSAGES ARE KUBERNETES-API STATE

`status.runs[].inputs[]` holds what each run was asked — text (capped at
`MaxRecordedInputText`, marked `truncated` beyond it), arrival time, origin
surface, sender — beside what it answered.

**THE QUEUE AND THE RECORD ARE DIFFERENT THINGS.** `spec.inputs[]` is a work
queue and `pruneProcessed` keeps emptying it, which is what stops answered work
running twice.

**Pruning must never be the only copy of what a person said.** It was, so a
conversation kept the answers and dropped the questions and a viewer could
rebuild half a thread.

**The ORDERING is the guarantee.** The record is written by the SAME status
write that marks the inputs processed (`handleWorkDone`), therefore strictly
before anything may prune them. Recording in a second pass would let a crash in
between destroy the message permanently.

**A viewer's buffer is a CACHE of that record, never its only copy.** The
console workaround that watched the queue and matched text is deleted, not kept
as a fallback.

### No relay loops

**Channel implementations — adapters AND in-process providers — must never
re-ingest their own outbound posts as inbound.** Cross-channel relay depends on
it.

**LOAD-BEARING IN ONE MORE PLACE now:** one adapter may serve several surfaces
of one conversation, so a message can be delivered TOWARD the transport it
entered through, and an implementation that echoed its own outbound posts would
loop rather than merely duplicate.

### No signal loops

The same rule one lane over: **an observing signal adapter must NEVER emit a
signal about agent-ops' own machinery.**

The cycle: a runtime pod that cannot start emits a Warning event, that event
becomes a signal, the signal opens a Conversation, the Conversation creates
another runtime pod under a NEW name, forever.

**Nothing downstream catches it:**

- The fingerprint is fresh (new pod name).
- The workload is fresh (owner is the Conversation CR).
- Even a correct liveness re-check passes it, because the pod really is broken.
- `MAX_ACTIVE_CONVERSATIONS` caps pods and `MAX_QUEUED_CONVERSATIONS` caps the
  backlog, but neither stops the LOOP. It just fills etcd more slowly.

`signal-k8s-events/selfexclude.go` implements THREE independent mechanisms:

1. **Name prefix** — needs no API read, so it holds with a cold cache.
2. **Owner/label.**
3. **Own-namespace.**

**Only the third is configurable.** A deny-list is editable, and an editable
loop breaker is not one. A nil excluder still applies mechanism 1 on purpose.

**agent-ops' own health is STATUS, not SIGNAL.** The reconciler already holds
the failure. Routing it back through ingest to wake an agent is the
architectural error, not merely a noisy one.

### Runtime pods

- **ownerRef → Conversation**, for GC.
- **Repo checkout at `/data/workspace`.** claude-code sessions are keyed by cwd,
  so moving this path breaks session resume.
- **`/data/workspace` and `/data/home` are mount points.** Clear contents, never
  rmdir.

### Dispatch and ingest semantics are pinned by test fixtures

Change behavior by changing tests deliberately, not incidentally.

### `for:` is Prometheus, `group_wait` is Alertmanager, and they are NOT the same thing

`signal-k8s-events` config is deliberately two halves:

| Half | Borrowed from | Says |
|---|---|---|
| `rules` | Prometheus | what counts as a problem, and how long it must hold |
| `route` | Alertmanager | inhibition |

**Alertmanager's `group_wait` batches a group before its FIRST notification.**
`for:` does not exist in Alertmanager at all. Spelling dwell as `group_wait`
would be an Alertmanager term meaning something Alertmanager does not mean.

Two further rules the defaults depend on, both pinned in
`internal/integration/charttemplate_test.go`:

- **Reasons describing a COMPLETED event must carry `for: 0`** — `OOMKilling`,
  `SystemOOM`, `BackoffLimitExceeded`, `DeadlineExceeded`. A dwell finds the
  healthy replacement and erases the incident.
- **The LAST rule must be a catch-all with a dwell, never a drop**, so an
  unanticipated reason is verified rather than discarded.

**`Evicted` is the exception, and is DROPPED** as of chart 5.9.0. It used to sit
in the past-tense set.

An eviction is reported from both ends already and per POD from neither:

- **Kubelet evictions are caused by node pressure**, which tier 3 reports at
  `for: 0` as ONE node-level signal rather than one per displaced pod.
- **API-initiated evictions are drains** — routine, and UNATTENDED wherever a
  reboot manager runs.
- **The case worth waking for is a pod that does not come back**, which arrives
  as `FailedScheduling` with a dwell to confirm it.

**The drop is therefore only defensible while BOTH substitutes survive**, so the
test pins node pressure at `for: 0` and `FailedScheduling`'s presence TOGETHER
with the drop. Re-tuning one of them must not silently leave eviction unreported
from every direction at once.

**The TIME axis** (`route.timeIntervals` + `route.muteTimeIntervals`) is
Alertmanager vocabulary too, borrowed field-for-field.

A scheduled outage is the one thing the other three axes cannot express: `for:`
verifies a condition the outage genuinely satisfies, inhibition needs a cause
event a power cut never produces, and no label carries the time of day.

- **Mute is evaluated at EMIT** — after the dwell, before the emit cap — and
  that ordering IS the safety property. A problem outliving the window still
  emits once it closes, and a muted burst never spends the emit budget.
- **`location` defaults to UTC but must be NAMED**, because a UTC-pinned window
  drifts an hour at each DST change. `_ "time/tzdata"` is imported in `mute.go`
  — distroless carries no zoneinfo, so without it every valid zone is rejected.
- **A window with no `matchers` deafens the source outright**, which is why the
  shipped example narrows.
- **Muting reports itself on the source's Ready condition** — `Ready=True`,
  reason `Muted`, then `MuteEnded` with the count. A muted lane and an idle lane
  are otherwise indistinguishable.

### Event grouping is by workload

`[namespace, workload]`, resolved through OWNER REFERENCES (Pod → ReplicaSet →
Deployment) and never by parsing a pod name — that breaks on StatefulSets
(`api-0`), DaemonSets and bare pods.

Pod names are unique per replica and regenerated every rollout, so the old
`[namespace, kind, name]` default made conversations scale with pods × rollouts
and the 7-day window reuse could never fire.

## Gotchas (paid for in debugging)

- **RBAC `resources:` are lowercase plurals.** A blanket rename once produced
  `AgentRuntimes` and silently broke the informer — forbidden loops in the log,
  reconciler does nothing.
- **SSH deploy keys in Secrets must be LF-only with a trailing newline.** CRLF
  or flattened-to-one-line keys fail with `error in libcrypto`. Prefer building
  the Secret from base64 rather than shell `--from-literal` interpolation.
- **envtest needs `KUBEBUILDER_ASSETS`.**
- **`kubectl auth can-i` misparses the `pods/eviction` slash form.** Use
  `--subresource=eviction`.

**Tearing down a throwaway release: UNINSTALL FIRST, then clear the
`agentops.dev/close-topics` finalizer.**

- **Clearing it while the manager still runs achieves nothing.** The reconciler
  re-adds it within a second, and then `helm uninstall` removes the only thing
  that could ever release it, so the namespace hangs in `Terminating` forever.
- **The order is the whole trick**, and getting it backwards looks identical
  right up until it wedges.
- **Conversations carry the finalizer even with NO channels bound**, so "no
  chat, no problem" is not a reason to skip this.

**A rendered pod is not a running one, and a chart render test cannot tell the
difference.**

- **`mcpServers` shipped `PROMETHEUS_MCP_TRANSPORT` for a whole implementation
  pass.** The real name is `PROMETHEUS_MCP_SERVER_TRANSPORT`, so the server fell
  back to stdio — and a stdio process in a pod prints a banner and exits, giving
  a `Completed` pod behind a Service that answers nothing.
- **Every guard, every assertion and `--dry-run=server` passed.** Only starting
  the thing found it.
- **Pin env-var NAMES third-party images read**, and smoke any new workload
  before believing its values.

**`helm.sh/resource-policy: keep` protects nothing retroactively.**

- **Helm reads it off the LIVE object** when a resource leaves the manifest,
  never off the manifest dropping it. Adding the annotation in the same release
  that stops rendering the resource DELETES it. Verified against helm v4, all
  three cases.
- **Anything that stops being rendered needs the annotation on the object
  FIRST** — the generated credential Secrets are the case, which is why
  `agentops.generatedSecretGuard` fails the render rather than trusting a
  migration note.

**`lookup` returns empty on any renderer without a cluster** — `helm template`,
CI, a GitOps controller, `--dry-run=client`.

- **A template generating a value on the UPGRADE path APPLIES a new credential**,
  not merely shows one in a diff. Generate under `.Release.IsInstall` only.
- **A `lookup`-driven guard is silent under `helm template`**, so no chart render
  test can pin it. Verify with `helm upgrade --dry-run=server`.

**A HAND-PATCHED FIELD SURVIVES EVERY LATER `helm upgrade`.**

Helm's three-way merge patches only what differs between the PREVIOUS rendered
manifest and the NEW one, so an unchanged rendered value generates no patch at
all.

- **A `kubectl patch` made while debugging is therefore never corrected.**
  `k8s-ops` carried a debugging icon through five chart upgrades that way.
- **Every signal says it worked.** The release reports success and
  `helm get manifest` shows the DECLARED value, while the live object holds the
  other one.
- **A live patch is undone by ANOTHER live patch**, never by re-syncing.
- **Check the OBJECT, not the release**, when the cluster disagrees with the
  values.

**CILIUM ANSWERS A BACKEND-LESS SERVICE WITH EPERM, NOT ECONNREFUSED.**

Under `kube-proxy-replacement: strict` the socket load balancer fails
`connect()` in the pod's own kernel when a ClusterIP has no READY endpoint:

```
dial tcp 192.0.2.187:8080: connect: operation not permitted
```

- **It is a rollout race, not a denial**, and clears the moment an endpoint goes
  ready. kube-proxy would have said `connection refused` at the same instant.
- **`telegram-router` logs it once at startup**, reading the offset while
  `channel-telegram` is mid-rollout, then retries every 5s and recovers.
- **Confirm against the ENDPOINT LIST and the ReplicaSet timestamps before
  suspecting policy.** Three sessions read that line as a NetworkPolicy problem.

**`reply_to_message` IS ONE LEVEL DEEP AND NEVER NESTS.**

**A reply carries the message it answers, and no further.** That message holds
no `reply_to_message` of its own, so a chain walked two links up to recover an
original command finds nil, every time.

- **The menu prompt NAMES the addressed form in its own text** —
  `Reply with the task for /<pipeline>` — and `signal-telegram` reads the first
  slash-token back out.
- **Guarded on `from.is_bot`**, so quoting `/ha-ops` at a colleague starts
  nothing.
- **The question's WORDING is load-bearing**, and is stated on both sides for
  that reason.
- **A payload shape is settled by the live transport or not at all.** It shipped
  broken because the test hand-wrote NESTED JSON Telegram never sends — an
  assumption asserting itself, which a fixture cannot catch.

**Never run two getUpdates consumers against one Telegram bot token** — 409s and
stolen updates.

Migrating from another system, or from the old single-container adapter:

1. Stop its poller.
2. CONFIRM none remains.
3. Start `telegram-router`.

**`Channel.spec.config.pollingEnabled` is gone.** Ingest is on when the router
runs.

**The router's bot Secret is the SAME one the Channel uses**, since it polls the
bot the channel sends as, injected by the chart as `TELEGRAM_BOT_TOKEN`.

**It used to be an adapter with a signal-free `SignalSource`** purely to carry
that credential, which then sat at `Wired=False` until some Pipeline faked a
claim. Modelling plumbing as an adapter produced that whole chain.

### THE PARENT CHART IS WHERE WIRING IS DECLARED

**A bundle ships it only under the four conditions.**

**A subchart sees only itself**, while wiring names a profile, sources and
channels that routinely come from DIFFERENT bundles — so one that shipped wiring
could only ever wire ITSELF. Declare routes in the top-level `pipelines:`
values.

A bundle MAY ship its own only when ALL of:

1. **Rendering is behind an explicit wiring flag.**
2. **Every reference to an object the bundle does not itself render is a
   values-supplied NAME**, omitted when unset.
3. **Each Pipeline renders only with its own profile.**
4. **The flag DEFAULTS OFF**, forced on by nothing but a values path whose
   declared purpose is a turnkey install (`global.demo.enabled`), and then only
   the LEAST-PRIVILEGED route.

| Bundle | Qualifies | `enabled` default | Routes |
|---|---|---|---|
| `k8s-bundle.pipelines` | yes — it owns its whole lane (source, profile, both toolsets), so channels are the only foreign name | **nullable**, so an explicit `false` can decline the route even under demo mode | one |
| `prometheus-bundle.pipelines` | yes, on the same grounds | plain `false` — demo mode never enables that bundle, so there is nothing for an explicit `false` to beat | one |
| `ha-bundle.pipelines` | yes, same plain `false` for the same reason | plain `false` | **two**, because its lane has two privilege levels |
| `telegram-bundle` | **no — the counter-example.** Its routes genuinely span bundles, because a chat surface is answered by an agent from somewhere else | — | none |

**`ha-bundle`'s acting route claims the log source and NO chat source**, so
reaching it is `/ha-ops <task>` and never an accident.

- **Name pipelines for their JOB**, not for the channel they answer on.
- **A SignalSource is NOT claimed by exactly one pipeline.** Sources are
  shareable, so a bundle's route and an install's route claiming one source both
  render and the source fans out to both.
- **That is reported in NOTES.txt, never refused.** Refusing it would be the
  deleted `sourceConflicts` guard returning one layer up.

## After changes

**DOCUMENTATION IS PART OF THE CHANGE, NOT A FOLLOW-UP.**

Before a change is committed, and certainly before it is ARCHIVED, every
document the change made untrue is already updated — in the same commit, not a
later one.

Archiving a change whose docs still describe the old behaviour records the work
as finished when the half a reader meets is not.

**That explicitly INCLUDES the adopter pages on the site.** It is the half most
often skipped, because a behaviour change feels done once `concepts.md` is right
— and the adopter never reads `concepts.md`.

Ask of every change: does the landing page, the Introduction, Getting started or
Installation now say something that is no longer true, or fail to mention
something an adopter must now decide?

**A page promising a step the chart just automated is as wrong as a stale field
name.**

### Where documentation goes

Keep commits scoped to this directory, and write documentation to the file that
OWNS that kind of content.

"Update README.md" is what grew it to 969 lines — three documents wearing one
filename — so the routing is explicit:

| What changed | Where it goes |
|---|---|
| CRD fields, semantics, how capabilities resolve | `docs/concepts.md` |
| Work contract, adapter contracts, HTTP endpoints | `docs/contracts.md` |
| A subchart's components or values | `docs/<bundle>.md` |
| The PARENT chart's values, install, upgrade, uninstall | `docs/installation.md` |
| Breaking change + upgrade steps | `docs/CHANGELOG.md`, newest first |
| Terminology, invariants, hard-won gotchas | this file |
| What the console is FOR — its views, what each answers, the authentication decision | `docs/console-guide.md` |
| What the console IS — endpoints, RBAC grant, values reference, internals | `docs/console.md` |
| A change to the console's UI | re-run BOTH `npm run screenshots` and `npm run demo` in `console/ui` — the site's screenshots and its landing recording are build output, and the change is not done until both match |
| The pitch, the kind list, the demo, the install command | `README.md` |
| How the site LOOKS or navigates | `docs/_layouts/`, `_includes/`, `_data/nav.yml`, `assets/` |
| What the site SAYS to an adopter | a markdown page under `docs/` |
| How a page READS — structure, tabs, components, tables, the lint | `docs/CLAUDE.md` |

**Both value rows are "values", so the split is stated.** The PARENT chart's
belong to `docs/installation.md`, a SUBCHART's to that bundle's own page, and
neither restates the other.

**`installation.md` carries the values an operator must DECIDE**, grouped by the
decision they serve. `helm show values` is the exhaustive list, and a
hand-copied inventory rots.

**The last three rows are one rule read three ways: the theme holds no prose,
the pages hold no theme, and neither holds the rules.**

- A layout or include that starts explaining a CRD is in the wrong file.
- So is a markdown page that opens with a `<div>` or an inline style.
- Adding a page to the site is a page plus one line in `_data/nav.yml`, never
  navigation markup written a second time.

### The palette and the mark are COPIED, and the copy is one block

**The site's `--ao-*` palette is copied from `console/ui/src/theme/theme.css`**
into the token blocks at the head of `docs/assets/css/agentops.css`. Changing a
token is a TWO-FILE change.

**The copy is deliberate and one-directional** — a Jekyll site must not need a
Node build to publish a paragraph.

**What makes it survivable is that no colour is stated literally anywhere else
in the site CSS**, so the sync is one block, not a hunt:

```sh
grep -n '#[0-9a-fA-F]\{3,6\}' docs/assets/css/agentops.css
```

That must return hits only inside those blocks.

**The theme-choice semantics are copied on the same terms** —
`assets/js/theme.js` from `theme/useTheme.ts`.

**The MARK is copied on those terms across THREE files:**

| File | Is |
|---|---|
| `console/ui/src/components/Logo.tsx` | the source |
| `docs/_includes/logo.svg` | the masthead's theme-driven copy |
| `docs/assets/img/logos/agent-ops.svg` | the standalone one an `<img>` can load |

The standalone one states its colours literally, because an `<img>` is its own
document and inherits no custom properties.

**Integration marks sit beside it**, committed unaltered from each project's own
source with their terms in that directory's README, and the PAGE names each
file. A vendor list in the stylesheet would be product knowledge in the theme.

### README.md has a budget: 150 lines

`wc -l README.md`.

**It holds** the pitch and diagram, one line per CRD kind, the behaviors that
matter, the demo, install, the Documentation index (the site first), development
and status. **Nothing else.**

- **A distinguishing behavior is named in a LINE**, and the document that owns
  it is linked.
- **Reference material and migration guides do not belong in it.**
- **If it is over budget, something is in the wrong file.**

## Authoring rules (binding)

**They govern THIS file, not only the pages under `docs/`.** A rule stated in
prose here is as unfindable as one stated in prose there.

**Concise and LLM-optimized.** Cut filler, marketing tone and preambles — every
sentence earns its tokens.

**Structure over prose:**

| Content | Shape |
|---|---|
| Steps | numbered list |
| Choices / mappings | table |
| "X means Y" | **X.** Y, on its own line |
| Multi-rule bullet | parent + sub-bullets, ONE rule per line |
| Prose paragraph stating > 2 rules | restructure |

**Reasoning is not filler.** A rule keeps the sentence saying what it cost —
that is what stops the next reader undoing it. What gets cut is the RESTATEMENT
of the rule, never its why.
