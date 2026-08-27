# Changelog archive — chart 5.22.0 to 7.0.0

Migration guides for chart versions **5.22.0 through 7.0.0**, newest first, in
[Keep a Changelog](https://keepachangelog.com/en/1.1.0/) format.

Moved here from [CHANGELOG.md](../CHANGELOG.md), which holds the ten most recent
versions.

## [7.0.0] — 2026-08-23

**Every image moves registry.** All twelve first-party images the chart renders
repoint from Docker Hub `kmatsebora/*` to
**`ghcr.io/kostiantyn-matsebora/agentops-*`**. No CRD field changes.

Nine of them also move a patch, because nine components now build from one
shared Dockerfile and their entrypoint became `/app`:

| Component | Tag | | Component | Tag |
|---|---|---|---|---|
| `manager` | 0.53.0 | | `channel-telegram` | 0.24.1 |
| `console` | 0.37.0 | | `gateway-telegram` | 0.5.1 |
| `runtime-claude` | 0.8.0 | | `signal-telegram` | 0.6.1 |
| `context-sync` | 0.2.1 | | `signal-alertmanager` | 0.7.1 |
| `egress-proxy` | 0.2.2 | | `signal-k8s-events` | 0.4.1 |
| `housekeeping` | 0.2.1 | | `signal-ha` | 0.2.1 |

**Docker Hub keeps the old tags**, unchanged and still pullable. A tag never
means two different things: where content changed, the number moved.

The chart itself is now published as an **OCI artifact**, so installing no
longer needs a checkout:

```sh
helm install agent-ops \
  oci://ghcr.io/kostiantyn-matsebora/charts/agent-ops-operator \
  --version 7.0.0 --namespace agent-ops --create-namespace
```

**No credential anywhere.** The GHCR packages are public, so pulls stay
anonymous. The chart gains no `imagePullSecrets` value, and no ServiceAccount
changes — which matters because adapter ServiceAccounts are created by the
manager's reconciler, not by the chart, so a chart-rendered pull secret could
never have reached six of the thirteen images.

### Added

- **CI on every pull request** (`.github/workflows/ci.yml`): the twelve Go
  modules build, vet and test — the operator's against a real API server — the
  console UI builds and runs its own suite, the chart lints and renders across
  its value permutations under `kubeconform`, and all thirteen images build.
- **Tag-driven publishing.** A tag of the form `<component>-v<semver>`
  publishes exactly that component. `chart-v<semver>` publishes the chart.
- **Every image is multi-arch** — `linux/amd64` and `linux/arm64` as one
  manifest list, `runtime-claude` included. The release asserts what it pushed
  against a declaration, as equality, so an image that quietly loses an
  architecture fails the release rather than a reschedule weeks later.
- **A LICENSE.** Apache 2.0. Public packages publish the built binaries, so it
  stopped being a footnote.

### Changed

- **BREAKING for pinned images.** Every default image repository gains the
  `ghcr.io/kostiantyn-matsebora/` prefix — six in `chart/values.yaml` (manager,
  console, housekeeping, `runtime.image`, `contextSync`, `egressMediation`) and
  six across the bundles.
- **A published tag can no longer be overwritten.** The release refuses a tag
  already in the registry, so the recovery from a partial failure is a new
  patch version. This was a note in `CLAUDE.md` and is now a gate.
- **Nine images now share one Dockerfile**, and their entrypoint is **`/app`**
  rather than `/signal-cron`, `/channel-telegram` and so on. The images affected
  are the five signal adapters, `channel-telegram`, `gateway-telegram`,
  `context-sync` and `housekeeping`. Nothing the chart renders names an
  entrypoint path, so this is invisible unless YOU set `command:` on one of
  those containers yourself.
- **`runtime-claude` is multi-arch.** It was published amd64-only because the
  hand-run build command said so, not because its upstream is — building it for
  `linux/arm64` and running `claude --version` on `aarch64` settled that. The
  runtime `nodeSelector` the chart still ships is therefore compensating for
  nothing, and can be relaxed at your discretion.

### Upgrade

**Existing Docker Hub tags stay published and are untouched.** An install that
does nothing keeps running.

Upgrading pulls from GHCR. If your cluster cannot reach `ghcr.io`, or you mirror
Docker Hub, stay on the old registry by naming it:

```sh
helm upgrade agent-ops … \
  --set image.repository=kmatsebora/agentops-manager \
  --set runtime.image=kmatsebora/agentops-runtime-claude:0.8.0
```

The same override applies to `console.image.repository`,
`housekeeping.image.repository`, `runtime.contextSync.image`,
`runtime.egressMediation.image`, and each bundle's
`<bundle>.<component>.image.repository`.

**If you set any image repository yourself, that value wins and nothing
changes** — repoint it when you are ready.

**If you override `command:` on an adapter, the sidecar or the housekeeping
job**, change the path to `/app`. Nothing the chart ships does.

## [6.0.0] — 2026-08-23

**Every image moves.** The repository was regrouped by component type and every
component rebuilt and republished, so the version line is uniform rather than
partial:

| Component | Tag | | Component | Tag |
|---|---|---|---|---|
| `manager` | 0.44.0 | | `signal-cron` | 0.3.0 |
| `console` | 0.26.1 | | `signal-alertmanager` | 0.7.0 |
| `runtime-claude` | 0.8.0 | | `signal-k8s-events` | 0.4.0 |
| `context-sync` | 0.2.0 | | `signal-ha` | 0.2.0 |
| `egress-proxy` | 0.2.1 | | `signal-telegram` | 0.6.0 |
| `housekeeping` | 0.2.0 | | `channel-telegram` | 0.20.0 |
| | | | `gateway-telegram` | 0.5.0 |

No CRD field is removed and **the adapter contract version stays `2`**.

**One thing to do on upgrade**, and only for Telegram: `gateway-telegram` is
`telegram-router` renamed, workload included — see the entry below. Everything
else is a tag bump.

### Added

- **`Pipeline.spec.runtimeRef` and `Pipeline.spec.serviceAccountName`.** The
  Pipeline now selects WHAT EXECUTES its conversations and UNDER WHOSE IDENTITY,
  beside the tools and MCP servers it already granted.

  - **Capabilities and execution identity are the same decision** — one says
    which tools may be called, the other with whose credentials. Split across
    two objects, no single object stated an agent's power.
  - **Both are OPTIONAL and an install setting neither is unchanged.**
    `runtimeRef` falls back to the `AgentRuntime` named `default`, and the
    identity to that runtime's own `serviceAccountName`.
  - **One runtime image can now serve two trust levels.** An observing route and
    an acting route differ in their account, not in their image, so the second
    no longer needs a cloned `AgentRuntime`.
  - **The Conversation SNAPSHOTS both at creation.** Editing a Pipeline never
    moves a conversation already running onto a different identity — that would
    be a privilege change applied to work in progress. The runtime name is
    frozen resolved. The account is frozen only where the Pipeline named one, so
    correcting a runtime's own account still heals existing conversations.
  - **Naming an account does not create one.** No reconciler makes a
    ServiceAccount and `Ready` does not check for one — the pod fails at
    admission naming it. A dangling `runtimeRef` does report `Ready=False`.

- **`rbac.runtime.serviceAccounts`** — additional runtime identities the parent
  renders, each with its own `rbacMode` and targeted grants.

- **EVERY BUNDLE RENDERS THE ACCOUNTS ITS OWN ROUTES NEED**, scoped to what
  those routes do, and names each on its own Pipeline. The bundle is the only
  scope that knows: `k8s-bundle` renders one per route, `ha-bundle` renders two
  with no Kubernetes RBAC at all because neither route touches that API, and
  `telegram-bundle` renders none because it ships no Pipeline.

  **"No subchart renders a runtime ServiceAccount" is REVERSED.** That rule was
  protecting against a bundle rendering the SUBSTRATE — a runtime, a credential,
  an account sized for everything. An account sized DOWN to one route is the
  opposite. The substrate stays the parent's, exclusively.

- **Structured agent output.** An agent's answer now reaches chat as named
  blocks with a fold, so a reader gets the conclusion first and the long tail
  only on request.

  - **Two reserved block tags** — `<title>` and `<details>`, THE FOLD. Every
    other tag is a section the agent names for its own job, rendered above the
    fold in the order written.
  - **Each channel adapter parses the grammar** and renders it to what its
    transport has. The manager passes an agent's text through untouched, exactly
    as it already passes markdown through.
  - **Nothing shortens an agent's output.** What sits above the fold is what the
    agent put there.
  - **`AgentProfile.spec.outputFormat`** — REQUIRED, `blocks` or `none`, with no
    default. It gates the prompt only: output is parsed either way.
  - **Telegram** folds `<details>` into an expandable blockquote and splits at
    block boundaries, so the first message always carries the conclusion.
  - **The console** renders the fold as a real disclosure control.
  - **A `signal` is never parsed.** Its structured fields are the message and the
    adapter renders a card from them — the source stated plainly, labels as a
    table, the payload behind its own control.
  - **History renders like live output.** The tags are in `status.runs[].result`,
    so a conversation reopened after a restart parses the same characters a live
    message carried.

- **`GET /channel/vocabulary`** — what may be typed on a chat surface: the
  manager commands and every Ready Pipeline, each with a `position` (`general`
  or `thread`). A channel adapter holds no Kubernetes access, so this is the
  only way it can know what is addressable.
- **`X-Agentops-Vocabulary-Revision`** on every `GET /channel/ops` response, the
  `200` and the `204` alike. A changed revision means refetch. The manager
  cannot dial an adapter, so the news rides a connection the adapter already
  holds.
- **`choices[]` and `inReplyTo` on an outbound message.** Offered actions, and
  the transport's own handle for the message being answered. Both optional, both
  structured. A transport without controls renders the same list as text.
- **Telegram registers a command menu** per served chat, covering the manager
  commands and your Ready Pipelines. Telegram then shows its own control in the
  composer and completes what you type.
- **Tapping a Pipeline on the ambiguity refusal sends the message you already
  typed**, rather than making you write it again.
- **The console's reply box completes commands** — `/exit` and `/close`, with
  the difference stated. It never offers a Pipeline there.
- `agentops.dev/message`, an optional label on a chat signal carrying the
  transport's handle for the arriving message.
- **Seven adopter guides**, in learning order rather than by risk. Each one
  states what its own mistake costs, because risk is not monotonic along
  that order — a capability binding is pure YAML and can grant more than an
  adapter's code ever could.

  | Guide | Teaches |
  |---|---|
  | [Put an agent to work](guides/pipeline.md) | the wiring, built from what a demo install already has. It creates nothing but the `Pipeline` |
  | [Add your own agent](guides/agent-profile.md) | an `AgentProfile` of your own |
  | [Run your agent from a repository](guides/agent-from-a-repository.md) | the checkout, its deploy key and the agent definition |
  | [Give your agent tools](guides/toolsets.md) | `MCPToolset` and `MCPConfig`, bound from the Pipeline |
  | [React to signals](guides/signal-adapter.md) | implementing a `SignalAdapter` |
  | [Talk to agents from your own chat](guides/channel-adapter.md) | implementing a `ChannelAdapter` |
  | [Run agents on your own backend](guides/agent-runtime.md) | implementing the work contract as an `AgentRuntime` |

  - **The Pipeline guide is FIRST and creates nothing new.** A guide that opens
    by declaring an identity teaches an inert object whose purpose is a Pipeline
    the reader has not met.
  - **Every template and worked example is GENERATED**, never written — the
    templates from the CRDs the chart ships, the examples from each bundle's own
    values. An invented example is a second set of values to keep true.

- **[`docs/cr-reference.md`](cr-reference.md)** — every field of every kind,
  generated from the CRDs. A reference file beside `concepts.md`, not a site
  page, so the guides can carry the minimal resource and link the rest.
- **A drift check in CI.** `python3 .github/scripts/docs-generate.py`
  regenerates the templates, the examples and the reference; CI reruns it with
  `--check` and FAILS on any difference, naming the file and the command.
  Committed generated output without that check is correct the day it is
  written and silently wrong after the next field rename.

### Changed

- **BREAKING: A Pipeline that names no `serviceAccountName` now has NO CLUSTER
  POWER.** It runs as a floor account the chart renders and binds to nothing.

  A route used to inherit whatever the release granted, so it held cluster
  power by not typing a field. Three of four routes in the reference install
  held pod-delete and node-patch that way, and two of them reached no Kubernetes
  API at all.

  **Acting power is now something a route OPTS INTO by name.** `rbacMode`
  renders `agentops-runtime-acting` (or `-readonly`) and a Pipeline names it.

- **The grant is CLUSTER-WIDE, and `clusterroles` is no longer readable.**

  Namespaced Roles were built and reverted before release. RBAC cannot express
  "everywhere except", so bounding an agent meant an allow-list: one binding per
  namespace per account — 224 objects on a 28-namespace cluster — and every new
  namespace invisible to the agent until someone edited values and redeployed.

  **What makes cluster-wide safe is OMISSION, not scope.** `agentops.dev` is
  never granted in any rule, so Conversations and Pipelines are unreadable
  everywhere. Neither is `secrets`. `clusterroles` is dropped too — that listing
  maps every identity in the install and which one is worth attacking.

  **What it costs:** under `full` an agent can restart or delete pods in the
  operator's own namespace, the manager and adapters included.

- **BREAKING: `global.agentops.runtime.allowPodExecution` is OFF, and it is what
  makes "agents cannot read your Secrets" TRUE.**

  No role carries a verb on `secrets`. That was never enough on its own: the
  KUBELET resolves a Secret when it builds a pod, so an agent that can create a
  pod mounting one — or exec into a pod that already has one — reads the value
  having never asked the API server. `secrets: get` is never evaluated.

  Verified on a live cluster against the shipped role: pod created, pod log
  read, secret value returned, all seven `secrets` verbs denied throughout.

  **It gates every write that produces or enters a pod**, not just
  `pods: create` — creating a Job or Deployment writes a pod spec, and patching
  one edits it. With it off an agent still scales, restarts, evicts, cordons,
  deletes workloads and edits ConfigMaps, Services and Ingresses. What it loses
  is the ability to run new code.

- **BREAKING: `rbacMode: full` no longer binds `cluster-admin`, and `readonly`
  no longer binds the built-in `view`.** Every grant is now a role this chart
  writes out, so an operator can read it without resolving an aggregated or
  built-in role. **No runtime role carries any verb on `secrets`, in any mode.**

  An agent has a shell. `--allowedTools` configures a COOPERATING agent, while a
  ServiceAccount binding is what an uncooperative one actually has — so
  `cluster-admin` meant every credential in the cluster was readable by a model,
  and no allowlist changed that. The manager itself holds no `secrets` verbs.

  **BOTH WALLS MOVED.** `k8s-bundle`'s MCP server bound `cluster-admin` under
  `full` and the built-in `view` under `readonly` — and an agent reaches the
  cluster THROUGH it, so fixing one wall and not the other would have left the
  hole one indirection along. It now carries the same split from the same
  values, and cannot read this release's own namespace either.

  **The role grants** every read `readonly` grants, plus the workload verbs an
  agent fixes things with: delete or patch a pod, create a `pods/eviction`,
  update or patch a Deployment / StatefulSet / DaemonSet / ReplicaSet, scale
  through `*/scale`, create, delete or patch a Job, patch a CronJob, patch a
  Node. RBAC objects are readable and never writable, and there is no `escalate`
  or `bind`.

  **It will be wrong at first, and it fails CLOSED** — an agent is refused an
  action and says so, rather than quietly holding power nobody reviewed.

- **BREAKING: `AgentProfile.spec.runtimeRef` is DEPRECATED**, moved to
  `Pipeline.spec.runtimeRef`, and read for ONE release only.

  An `AgentRuntime` carries the ServiceAccount an agent runs as, so a profile
  choosing one chose the agent's power in the cluster — which made profile-edit
  rights into service-account-choice rights. A profile is prompts, a repo ref
  and limits, while a Pipeline already grants tools and MCP servers.

  **Nothing changes for an install that does not act.** The profile ref is read
  below the Pipeline's own field, so a profile applied before the upgrade keeps
  dispatching where it named, and setting both is harmless with the Pipeline
  winning.

- **BREAKING: `AgentProfile.spec.outputFormat` is REQUIRED.** Every profile must
  declare `blocks` or `none`. There is no default, because both candidates are
  wrong — `none` leaves output unformatted unless the prompt says otherwise, and
  `blocks` shapes it by something the author never asked for.

  **`kubectl apply` on a profile omitting it FAILS**, naming the valid values.
  Chart-managed profiles are re-applied with the field by the same upgrade.
  Hand-written ones need a one-line edit.

- **BREAKING (prompt only): the five report templates in the shared format
  specification are REMOVED.** `format.md` is rewritten in the block grammar,
  and its numbered templates become a default set of SECTIONS — a starting
  point, not a mandatory form.

  A profile that relied on the built-in investigation or action templates keeps
  a shared shape only by declaring `outputFormat: blocks`, which injects the
  new default sections instead.

  **Nothing breaks silently.** An agent that formats however it likes produces
  one block, which renders exactly as agent output does today.

- **The outbound contract version does NOT move.** The block grammar adds no
  field and changes no meaning: a body that was markdown is now markdown plus a
  grammar, read by the component that already read the markdown.

  **An adapter with no parser for it renders the tags literally.** That is
  prevented by a profile declaring `outputFormat: none` — the compatibility
  boundary is the profile, not the wire.

- **`/agents` is now `/pipelines`.** The old name still works and always will,
  but it is never printed, offered or registered. It listed Pipelines, and
  "agent" already names a definition inside a profile's repository.
- A Pipeline named `pipelines` joins the set unreachable by command.
- A hyphenated Pipeline is completed on Telegram under an underscored spelling
  (`/k8s_observe`). **The CR is not renamed** and the manager never sees the
  other form — the adapter translates it back. Both forms work when typed.
- **The console applies stream events instead of reloading.** Its `delta` event
  now carries the changed object, projected into the shapes its own snapshot
  endpoints serve, so a listing, a kind detail and an open conversation update
  in place. A message appears from the message event itself.
- **Sending a message from the console asks for nothing WHILE THE STREAM IS
  UP.** The echo, the acknowledgement and the answer all arrive as events. The
  composer used to re-read the whole conversation on every send — the heaviest
  read on the page, for what was already on its way. With the stream DOWN it
  still reads once, because the confirmation that clears `sending…` is itself a
  stream event, and without it a sent message sits unconfirmed until the page is
  reloaded.
- **A console view that has painted never goes back to a spinner.** A change
  counter used to sit in every query key, so each event asked for a cache entry
  that had never been filled — which is what made the page blank for a second
  every time anything moved. Sending a message did it three times.
- **Refetching in the console keeps four reasons**: first load, a resync, an
  explicit action, and a value that decays with time. Overview and Topology keep
  their timers, because rates and ages are wrong when time passes rather than
  when something changes. Aggregates the browser cannot recompute — install
  counts, the traffic graph, cross-object findings, resolved capabilities — are
  re-read on a stable key, so the page stays on screen while the read lands.
- **The browser cache is bounded.** Data for a view that is off screen is
  released after five minutes, and returning to a view after a minute loads
  fresh. Nothing is persisted — no `localStorage`, no IndexedDB — so closing the
  tab still leaves nothing behind.

### Fixed

- **A tool advertised with a parameter that has NO TYPE is repaired before the
  agent sees it.** Home Assistant publishes `GetLiveContext`'s `domain` filter as
  `anyOf: [{}, {type: array…}]` — the first branch is an empty schema, so the
  parameter constrains nothing and a model writes `{"domain": sensor}`, which is
  not valid JSON and never runs. The egress-proxy already rewrites tool listings
  on the way back, and now drops union branches that say nothing, leaving the
  typed branch the server itself published. **It invents no types**: a union of
  typed branches passes through untouched, and a property whose only branch is
  empty is left exactly as published. Requires `runtime.egressMediation`.
- **A tool call the model could not FORM no longer spins.** Arguments that are
  not valid JSON are discarded by claude-code before anything runs — no MCP
  server sees them, no allowlist refuses them — so a run made of them looks busy
  and then answers from whatever the session already held. `runtime-claude` now
  counts them, and ends the run as **failed** when the same tool is called with
  the same unparsable arguments five times in a row, naming the tool and
  quoting what was written. `RUNTIME_UNPARSED_REPEAT_LIMIT` tunes it, `0`
  disables the breaker and keeps the counting.
- **A run that recovers from one says so on the answer.** Recovering usually
  means abandoning the tool rather than fixing the call, and the model then
  answers from what the session already held without mentioning it. The runtime
  appends one line naming how many calls never ran and which tool — the agent's
  answer is still the answer.
- **Both Home Assistant profiles are told to quote `domain`.** Home Assistant
  advertises `GetLiveContext`'s domain filter with an `anyOf` whose first branch
  is an empty schema, so the parameter has no declared type and a model writes
  `{"domain": sensor}`. Measured on one install, 59 of 110 calls to that tool
  never executed. The prompt line is a workaround for a schema the chart does
  not own.

### Changed

- **The repository is grouped by component type** — `platform/` `runtimes/`
  `signals/` `channels/` `gateways/`, one container per directory, with the
  operator now at `platform/manager/`. A component's published name is derived
  from its PATH: a plural group lends its singular as a prefix
  (`signals/cron` → `agentops-signal-cron`), a singular one lends nothing
  (`platform/console` → `agentops-console`). Twelve of thirteen image names are
  unchanged.

  Every Go module path now follows its directory, so `api/v1alpha1` is imported
  as `…/agent-ops-operator/platform/manager/api/v1alpha1`. No CRD, contract or
  runtime behaviour changed.

- **BREAKING — `telegram-router` is now `gateway-telegram`.** The image is
  `agentops-gateway-telegram` and the Deployment is `agentops-gateway-telegram`.
  The old image stays published, as `signal-vmalertmanager` did.

  ### Upgrade

  Helm creates the new Deployment before deleting the old one, so for a few
  seconds **two consumers poll one bot token** — 409s and a couple of stolen
  updates, with the same image on both sides. To avoid the overlap entirely,
  scale `agentops-telegram-router` to zero before upgrading, or uninstall the
  telegram bundle and reinstall it.

  Nothing else moves: the `router:` values key in `telegram-bundle` keeps its
  name, and both adapters, the Channel and the SignalSource are untouched.

### Removed

- **The `/<pipeline>:<agent>` addressed form.** A Pipeline names one profile and
  a profile names one agent, so the agent is decided by the wiring. Letting the
  sender pick a different one reached past it.

  Text after the Pipeline name is now simply the task, colons included.

### Deprecated

- `Conversation.spec.inputs[].agent`. Nothing writes it. Dispatch reads it for
  one release so an input queued before the upgrade still reaches the agent it
  was parsed with. The field is removed in a later release.
- `AgentProfile.spec.runtimeRef`. Moved to `Pipeline.spec.runtimeRef` and read
  for ONE release, below the Pipeline's own field. Removed in the next major.

### Upgrade

**Every route loses its cluster reach until you say otherwise.** That is the
point of the change, and it fails CLOSED — an agent is refused and says so.

**1. Name an account on every route that needs cluster power.** A Pipeline that
names none now runs as an account bound to nothing.

```yaml
global:
  agentops:
    runtime:
      rbacMode: full                # renders agentops-runtime-acting

pipelines:
  - name: <the route that acts>
    profile: <its profile>
    serviceAccountName: agentops-runtime-acting
  - name: <a route that only observes>
    profile: <its profile>
    # name nothing: no cluster power at all
```

**2. Decide `allowPodExecution`, and read why before you set it.**

It is off, and it is what makes "agents cannot read your Secrets" true rather
than merely written down. With it off your agents cannot create a pod, edit a
workload's pod template, or exec into a container.

**Turn it on only if you accept an agent reading every Secret in the cluster** —
the kubelet resolves a Secret when it builds a pod, so pod execution and Secret
access are the same capability.

```yaml
global:
  agentops:
    runtime:
      allowPodExecution: true    # grants pods_run and pods_exec, and Secret reach
```

**3. If you ran `rbacMode: full`, check what the enumerated roles omit.**

```sh
helm template <release> agentops/agent-ops-operator -f your-values.yaml \
  | awk '/^kind: (Cluster)?Role$/,/^---/' | grep -A400 'agentops-runtime-acting'
```

```powershell
helm template <release> agentops/agent-ops-operator -f your-values.yaml `
  | Select-String -Pattern 'agentops-runtime-acting' -Context 0,400
```

Need something they omit? **Add your own `ClusterRole`** rather than widening
the shipped one, and attach it to the route that needs it:

```yaml
rbac:
  runtime:
    serviceAccounts:
      - name: agentops-runtime-special
        rbacMode: full
        clusterRoles:
          - name: extra
            rules:
              - apiGroups: ["example.com"]
                resources: ["widgets"]
                verbs: ["get", "list", "patch"]

pipelines:
  - name: <the route that needs it>
    serviceAccountName: agentops-runtime-special
```

**4. If any `AgentProfile` sets `runtimeRef`, move it to every Pipeline that
routes to that profile.**

```sh
kubectl -n agent-ops get agentprofiles \
  -o jsonpath='{range .items[?(@.spec.runtimeRef)]}{.metadata.name}{"\t"}{.spec.runtimeRef.name}{"\n"}{end}'
```

```powershell
kubectl -n agent-ops get agentprofiles `
  -o jsonpath='{range .items[?(@.spec.runtimeRef)]}{.metadata.name}{\"`t\"}{.spec.runtimeRef.name}{\"`n\"}{end}'
```

Empty output means there is nothing to do. Otherwise set `runtimeRef` on each
routing Pipeline — the profile field keeps working for this release, and the
Pipeline wins wherever both are set, so routes can move one at a time.

## [5.25.0] — 2026-08-22

Images: manager `0.38.1`, console `0.16.0`.

### Added

- `Conversation.status.runs[].inputs[]` — what each run was asked. Text, arrival
  time, origin surface and sender, beside what it answered. Text is inlined to
  2000 characters and marked `truncated` beyond that.
- `ChannelAdapter.spec.echoesOwnMessages`, default `true`. Declares whether the
  transport shows a person the message they just typed. A **viewer** — one that
  renders only what it is sent — sets it `false`. The console does, in the chart.

### Changed

- **A person's message now reaches every bound channel except the surface it was
  typed on.** It used to be withheld from all of them, so a second surface never
  saw what somebody asked.
- The console shows the message that **started** a conversation, and keeps it
  across a reload or a restart. It used to begin at the agent's answer.
- `spec.inputs[]` is still a queue and is still pruned once processed. Pruning is
  no longer the only copy of what a person said.

Nothing is posted back to the surface that displayed it. Nothing is delivered
retroactively.

### Upgrade

1. Apply the CRDs **before** the manager image. Both new fields are optional, but
   a manager writing a field the CRD does not know loses it silently.
2. `helm upgrade` with the new image tags.
3. Nothing is backfilled. Runs recorded before this carry no inputs.

**If you wrote your own channel adapter**, check one rule: it must never
re-ingest its own outbound posts as inbound. One adapter may now serve several
surfaces of one conversation, so a message can be delivered toward the transport
it entered through.

Rolling back is reverting the images. Old records stay readable.

## [5.24.0] — 2026-08-22

### Changed

**BREAKING for pinned images.** `signal-vmalertmanager/` is now
**`signals/alertmanager/`**, and its image is
**`kmatsebora/agentops-signal-alertmanager`** at the same tag (`0.6.0`,
identical behaviour).

The adapter reads the standard Alertmanager webhook payload, which vanilla
Alertmanager and VictoriaMetrics both send. The vendor name described one
sender, not the component.

Unchanged, so no immutable-field upgrade failure:

| Thing | Value |
|---|---|
| `SignalAdapter` CR name | `alertmanager` |
| `SignalSource.spec.adapter` | unchanged |
| Deployment selector label | `agentops.dev/signal-adapter` |

These deliberately keep VictoriaMetrics names, because each names a
VictoriaMetrics API object rather than one of ours:

- `register.go` writes a **`VMAlertmanagerConfig`**. Vanilla Alertmanager's
  config is a file with no object to write, so NOTES.txt prints a receiver stanza
  for it instead.
- `metrics.vmServiceScrape` renders a **`VMServiceScrape`**. The rules component
  renders a **`VMRule`**.

### Security

`global.agentops.networkPolicy.enabled` now covers the prometheus-bundle metrics
MCP server, the third and last unprotected one. It authenticates nobody, so any
pod in the cluster could query the whole metrics backend through it. It is now
restricted to runtime pods.

The webhook adapter is restricted only once you name the sender:

```yaml
prometheus-bundle:
  alertmanager:
    webhookFrom:
      - namespaceSelector:
          matchLabels:
            kubernetes.io/metadata.name: monitoring
```

**Empty leaves it reachable on purpose.** A policy that selects the adapter and
names nobody denies the alert lane silently, and that is discovered during an
incident. Under-restricting is the recoverable mistake.

The metrics MCP server moves to an exec probe over loopback, for the reason the
Kubernetes one did. The kubelet probes from the node, no policy peer can name a
node, and this server is reached on the port it serves on.

### Upgrade

Only if you set `prometheus-bundle.alertmanager.image.repository` yourself. Point
it at `kmatsebora/agentops-signal-alertmanager`. The old image stays published
for installs pinned to an older chart.

## [5.23.0] — 2026-08-22

Two things were true and neither was written down. Both are now closable, and
**both are off by default**. Upgrading changes nothing until you ask.

Decided in [adr/0001-bound-component-reach.md](adr/0001-bound-component-reach.md).

### Added

- `global.agentops.networkPolicy.enabled` renders one NetworkPolicy per
  component, allowing only the callers your wiring implies.
- `runtime.egressMediation.enabled` puts a proxy in the runtime pod that the
  agent's traffic cannot route around, and enforces the bound toolsets there.
- `Conversation.status` gains an `EgressMediated` condition.

### Security

**Nothing restricted who may reach this release's components.**

- The MCP servers accept any caller with no credential. Under `rbacMode: full`
  the Kubernetes one runs as cluster-admin, so reaching it *is* cluster-admin.
- The manager's work contract took no credential either. Any pod could take a
  queued work unit or post a forged agent answer.

**A route's toolsets bound only a cooperating agent.** `--allowedTools` is
applied by the CLI beside the agent.

An MCP server has never heard of an `MCPToolset`. An agent with a shell reached a
bound server directly and called anything it registered. `agentops-shell` is
bound on ordinary routes.

Egress mediation costs two things, so read them before enabling it:

- A **privileged init container**, refused by a namespace under `restricted` Pod
  Security admission.
- A container per active conversation.

stdio servers and `https` MCP endpoints stay unenforceable. They are reported on
`EgressMediated` rather than passed off as covered.

### Upgrade

Nothing, unless you enable one of the two flags. If you enable network policy,
name these four or a workload breaks quietly:

| Value | Names |
|---|---|
| `networkPolicy.metricsFrom` | a collector outside the namespace |
| `networkPolicy.consoleFrom` | your ingress controller |
| `networkPolicy.probesFrom` | your node network, if your CNI does not exempt host traffic |
| `prometheus-bundle.alertmanager.webhookFrom` | your Alertmanager sender |

**Read the note it prints.** A NetworkPolicy on a cluster whose CNI does not
enforce policy applies cleanly, appears in `kubectl get`, and blocks nothing. The
chart cannot detect that, so NOTES.txt tells you how to check.

The manager's probe port serves only health, so it is opened unconditionally. The
Kubernetes MCP server now probes itself over loopback, which no CNI can block.

## [console 0.15.9] — 2026-08-22

### Fixed

A conversation started from the composer showed a transcript beginning at the
**agent's answer**, with the question that caused it missing. Typing into an
already-open conversation was fine. Only the opening message vanished.

**Cause.** The manager posts an input to bound threads only when the person has
not already seen it. An alert gets a signal card. A message somebody typed gets
nothing, because posting it back would echo on the surface it was typed on.

That rule is right for a transport and wrong for a **viewer**. A Telegram user's
own message is already in their thread, put there by Telegram.

A console user's is not — the console renders what it was sent. The input is then
pruned once processed, so nothing could recover it.

**Fix, console-side only.** The console watches conversations and records what
people typed into its own transcript buffer, keyed on the input id. The set is
read off the manager's own rule rather than guessed.

An input with **no** origin is skipped. It cannot be told from an alert, and
inventing the wrong bubble is worse than a missing one.

Three things the first cut got wrong:

- **It read as typed.** An addressed task (`/ha-control turn the AC on`) reaches
  the conversation as the rest, because the manager consumed the address. The
  console posted the whole thing, being the only component that still has it.
- **It carries the starter's identity.** The input records provenance, not
  authorship. Without this the opening message read `local` while the reply below
  it read your address.
- **A reply is not duplicated.** The input a typed message becomes is the durable
  identity of that bubble, not a second one. It is adopted, keeping its id.

The UI also stopped printing `local` as a speaker's name. `local` means "typed on
this console", which is a fact about where a message entered, not a person.

### Upgrade

Bump the console image tag. The chart default moves with it. Nothing else
changes.

Alerts keep their manager-posted card. A console restart still loses what was
never CR state.

## [5.22.0] — 2026-08-21

Adds the Home Assistant bundle. See [the Home Assistant integration](integrations/home-assistant.md).

### Added

`chart/charts/ha-bundle/` ships a **privilege split** rather than one agent:

| Agent | Profile | Reached by | Job |
|---|---|---|---|
| The house's user | `ha-user` | an ordinary chat message | **use** the house — services, lights, automations |
| The administrator | `ha-operator` | `/ha-ops <task>`, by name | **fix** it — integrations, configuration, repairs |

The split is **use versus fix**, not read versus act. Home Assistant has no
read-only role, so neither credential merely looks.

What separates the lanes is the REST path. Assist intents reach no
configuration, so repairing needs a shell, and only the ops route binds one.

The acting route claims the log source and **no chat source**, so escalating is a
deliberate act. Claiming and addressing are independent mechanisms, so listing a
chat source there would grant it nothing while making every unaddressed message
on that surface ambiguous.

The **operator credential gates the fixing half**, and the ingest lane needs it
too. Home Assistant's `subscribe_events` is admin-only, so a control token
connects, passes auth, and is then refused the subscription.

There is **no MCP server workload**. Home Assistant serves its own endpoint
through the built-in MCP Server integration.

**A second MCP server for the ops lane**, off by default (`adminMcp.enabled`).
The built-in server exposes Assist intents only. It cannot read a log, reload an
integration or disable an entity, which live in registries served over the
WebSocket API.

It is bound to `ha-ops` alone, so `ha-control` reaches a server with no such
tools. Two walls, not one allowlist.

Two ways to have one, both off by default:

1. Let the chart deploy [ha-mcp](https://github.com/homeassistant-ai/ha-mcp)
   in-cluster (`adminMcpServer.enabled`).
2. Point `adminMcp.url` at a server you run, including a HACS integration inside
   Home Assistant.

Enabling the config with neither fails the render. **Add-ons are not an option on
Home Assistant Core**, which is the usual shape in Kubernetes.

Of the 78 tools that server registers, **52 ship**. The 26 withheld restart Home
Assistant, manage backups, delete registry objects or install software.

The toolset is enumerated and the image tag pinned. A server that renames a tool
changes what the allowlist means with nothing failing.

**The ops role names the REST path explicitly.** Without that the agent reads its
own tool list, finds device controls and no way to reach a log, and reports the
task impossible.

**New module `signals/ha/`** — a dependency-free signal adapter reading the
instance's WebSocket API over a hand-written RFC 6455 client.

- Watches `system_log_event`.
- Same `rules` / `route` vocabulary as the cluster Events adapter, minus the
  time axis.
- `kubernetesAccess: false`, because its data source is the house.
- Image `kmatsebora/agentops-signal-ha:0.1.0`.

### Changed

A subchart may render a `Pipeline` when — and only when — **all** of these hold:

1. Rendering is behind an explicit wiring flag.
2. Every reference to an object the bundle does not render is a values-supplied
   NAME, omitted when unset.
3. Each `Pipeline` renders only with its own profile.
4. The flag **defaults off**, forced on by nothing but a values path whose
   declared purpose is a turnkey install.

This does **not** make bundle-shipped wiring the norm. A bundle whose sources and
channels come from elsewhere still cannot meet condition 2. `telegram-bundle`
continues to ship none, and is the counter-example: a chat surface is answered by
an agent from somewhere else.

`ha-bundle` is the third bundle to qualify, after `k8s-bundle` and
`prometheus-bundle`.

### Upgrade

Nothing. `ha-bundle.enabled` defaults `false` and demo mode never turns it on.

To enable it, create the Secrets first. They are referenced by name and never
created by the chart. Each carries one token under **two** keys, because the
adapter sends the raw token and the MCP path sends a complete header value:

```sh
kubectl -n <ns> create secret generic ha-admin \
  --from-literal=token="$TOKEN" --from-literal=authorization="Bearer $TOKEN"
```

Then set the endpoint, the credentials and — deliberately — the routes.

