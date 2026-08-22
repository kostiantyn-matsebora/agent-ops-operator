# Changelog

All notable changes to this project are documented here, in
[Keep a Changelog](https://keepachangelog.com/en/1.1.0/) format. Versions are
CHART versions. Image tags move independently and are named in each entry.

This file holds the **ten most recent versions**. Older entries are in
`changelog/`, linked at the [foot of this page](#older-versions).

See [../README.md](../README.md) for the product overview and [./](./) for
reference material. `CLAUDE.md` in this directory owns the rules this file
follows.

## [Unreleased]

**Every image moves.** The repository was regrouped by component type and every
component rebuilt and republished, so the version line is uniform rather than
partial:

| Component | Tag | | Component | Tag |
|---|---|---|---|---|
| `manager` | 0.44.0 | | `signal-cron` | 0.3.0 |
| `console` | 0.26.0 | | `signal-alertmanager` | 0.7.0 |
| `runtime-claude` | 0.8.0 | | `signal-k8s-events` | 0.4.0 |
| `context-sync` | 0.2.0 | | `signal-ha` | 0.2.0 |
| `egress-proxy` | 0.2.0 | | `signal-telegram` | 0.6.0 |
| `housekeeping` | 0.2.0 | | `channel-telegram` | 0.20.0 |
| | | | `gateway-telegram` | 0.5.0 |

No CRD field is removed and the adapter contract version stays `2`.

**One thing to do on upgrade**, and only for Telegram: `gateway-telegram` is
`telegram-router` renamed, workload included — see the entry below. Everything
else is a tag bump.

### Added

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

### Changed

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
- **Sending a message from the console asks for nothing.** The echo, the
  acknowledgement and the answer all arrive on the stream. The composer used to
  re-read the whole conversation on every send — the heaviest read on the page,
  for what was already on its way.
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

Adds the Home Assistant bundle. See [ha-bundle.md](ha-bundle.md).

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

## [5.21.0] — 2026-08-21

On 2026-08-20 a node reboot corrupted the ext4 filesystem on the shared
`agentops-home` volume. Longhorn reported the volume **healthy** throughout,
correctly: it replicates blocks, and all three replicas agreed on the corrupt
ones.

Every runtime pod mounts that volume. Five pods sat in `ContainerCreating` for
fifteen hours, held every capacity slot, and starved six more conversations. The
install was completely down and **said nothing**. The only condition present read
`DeliveryPending=False / AllDelivered`, which looks like health.

### Added

| Value | Default | What it does |
|---|---|---|
| `runtime.contextSync.paths` | `[]` | moves the live context to pod-local storage, keeping a snapshot on the volume |
| `rbac.drainAware` | `false` | releases idle runtime pods from a cordoned node so the filesystem unmounts cleanly |
| `contextProbe.enabled` | `false` | hourly mount probe, so a damaged idle volume is found in an hour rather than at next use |

- `Conversation.status` carries **`RuntimeStarted`**, whose message is the
  kubelet's own words. A bare "deadline exceeded" would have reproduced the
  original failure with a timer attached.
- New verb `POST /channel/conversations/{name}/reset-context` clears a
  conversation's context handle and keeps the conversation, its threads and its
  history. It states the loss on every bound thread. It is operator-initiated
  only — an automatic version would be indistinguishable from the silent
  degradation the continuity rules forbid.
- New metrics: `agentops_storage_outage`, `agentops_storage_outage_seconds`,
  `agentops_context_operations_total`, `agentops_context_checkpoint_bytes`.

### Changed

- **A runtime pod that never starts is now reaped** after
  `RUNTIME_START_DEADLINE` (10m), with per-conversation exponential backoff. It
  still counts against the cap until it is gone. Un-counting it would provision
  past the cap against resources the cluster has not released.
- **The storage breaker gained a second edge.** It already treated many failed
  continuations as an outage. It now also counts pods that cannot be provisioned
  for a storage reason. That is why it never fired for the incident it was
  written for — no pod started, so no run existed to report. While open it holds
  work in `Pending` with a reason naming STORAGE rather than queue, and re-tests
  with one canary.

### Upgrade

`helm upgrade` with the new image tags. No values change is required and nothing
new is on by default. Rolling back is reverting the images.

**`contextSync` needs `paths`.** Only the runtime knows where its backend keeps
context, and the chart must not guess. For the reference runtime:

```yaml
runtime:
  contextSync:
    paths: [".claude/projects/-data-workspace/**"]
```

With it set, the agent container gets ephemeral storage and **no mount of the
durable volume at all**. Only the sidecar holds it.

A run already going then survives the volume going bad underneath it. An agent
can neither read another conversation's context nor corrupt the filesystem.

**Opting in strands existing context, visibly.** Without the sidecar, context
sits at the claim root. With it, each conversation reads a per-conversation
subdirectory, which starts empty.

Every conversation holding a context handle will therefore FAIL its next run
rather than answer without memory. That is the continuity rule working, not a
defect.

Recover each one:

```sh
curl -sX POST "$MANAGER/channel/conversations/<name>/reset-context" \
  -H "Authorization: Bearer $ADAPTER_TOKEN" -H 'Content-Type: application/json' \
  -d '{"channel":"<a channel it is bound to>"}'
```

Enable `contextSync` on a quiet install, or accept that live conversations lose
their memory once and say so.

**`rbac.drainAware` costs the manager its first cluster-scoped grant** — nodes
get/list/watch, read-only. Every other permission it holds is namespaced. It
shrinks the corruption window without closing it, because the storage provider
picks where a shared volume is served independently of where runtime pods run.

## [5.20.0] — 2026-08-15

On 2026-08-13 a 44-alert burst was rate-limited by Telegram. Every forum topic
was created, and 22 of them stayed **completely empty** for four and a half
hours, while the answers sat in `status.runs[].result` and the operator reported
itself healthy.

### Fixed

The cause was one line of asymmetry. The manager's completed-operation window
recorded operations that were **attempted**, and `enqueue` suppresses any stable
id in that window — which is every derivable op.

A failed send therefore disabled its own recovery. Reconciliation re-derived it,
dedup dropped it, nothing posted, and no restart helped.

- **The window now records what SUCCEEDED.** A failed derivable op releases its
  dedup entry and reconciliation re-derives it. `delete-conversation` is the one
  exemption, because its Conversation is disappearing.
- **Telegram paces itself** — 30 sends/second per bot, 20/minute per chat — and
  honours `retry_after` exactly, retrying in-process instead of reporting a
  failure.

### Added

- **`DeliveryPending`** condition on a Conversation, naming the channels a
  recorded run has not reached. It clears when they all have.

  ```sh
  kubectl get conv -o json | jq -r '.items[] | select(.status.conditions[]?
    | select(.type=="DeliveryPending" and .status=="True")) | .metadata.name'
  ```
- **`/channel/ops` advertises `reclaimAfterSeconds`**, so an adapter absorbing
  backpressure can bound its waiting inside the claim window. Additive and
  optional — every existing adapter ignores it.

### Upgrade

`helm upgrade` with the new image tags. No CRD change, no values change. Rolling
back is reverting them.

**Roll during a quiet window.** This re-posts what the incident lost, including
cards to topics that did receive their answer. Those were never posted, so it is
correct, but it will look like a flood.

Expect a burst to take **minutes** to appear in full. Every topic in a forum
shares one `chat_id`, so ~144 calls against 20/minute is over seven minutes. That
is Telegram's limit. The alternative is the old behaviour, which lost the
messages.

## [5.19.0] — 2026-08-15

### Added

Where the console can tell readers apart, unread is now **per person**. Your
badge is yours, and a colleague clearing theirs does not clear yours.

`status.threads[]` gains `readers[]`. `spec` gains `originReader`.

- **No identity is stored.** The console hashes the signed-in identity with a
  salt projected as a channel credential, and sends only that opaque key. The
  manager stores it verbatim and cannot reverse it. A conversation records that N
  people read it and when, never who.
- **The salt is generated for you** on install, and on the first upgrade that
  finds the console Secret without one. It adopts the existing `uiToken` rather
  than reissuing it, so nobody is signed out. Pin it with
  `console.auth.readerSalt` if you prefer.
- **Starting a conversation now marks it read for you**, and only you. Before
  this, a conversation you had just typed came back unread before any answer
  could exist.
- **`readers[]` is bounded at 50 per binding**, oldest watermark evicted first.
  An evicted reader falls back to the channel-wide mark, the same as a teammate
  who joined today.

Unchanged, so no adapter needs work:

- `readAt` stays as the channel-wide mark for transports with no reader identity.
- `POST /channel/read` gains an **optional** `reader` field. `channel-telegram`
  and every other adapter are untouched and fully conformant.
- **A shared UI token is one reader**, because there is one credential and no
  person behind it. With no salt projected, likewise. Both are exactly the 5.18.0
  behaviour, reached without a special case.

**Rotating the salt orphans every stored key** and resets everyone to the
channel-wide mark.

### Upgrade

**The CRDs must be re-applied again.** Same pruning trap as 5.18.0, same fix.

```sh
kubectl apply -f chart/files/crds/          # or helm upgrade with crds.enabled
```

If any CRD on your cluster was ever applied by hand with `kubectl apply -f`, that
left a `kubectl-client-side-apply` field manager owning `.spec.versions`, and
helm's server-side apply refuses it. It rolls the WORKLOADS while silently
leaving the schema behind.

Add `--force-conflicts` (helmfile: `--args "--force-conflicts"`), which only
moves ownership. Diagnose with:

```sh
kubectl get crd conversations.agentops.dev --show-managed-fields -o json
```

## [5.18.0] — 2026-08-15

### Added

**Unread conversations in the console.** The conversation list now:

- marks conversations whose **console thread** has activity newer than its
  watermark,
- offers an *Unread only* filter and a **Mark read** action over a selection,
- carries the unread count on the navigation.

Opening a conversation clears its mark.

- **Read is per CHANNEL.** The watermark lives on `status.threads[].readAt`, one
  per bound channel. Reading a conversation in Telegram does not clear it in the
  console. Two operators sharing one console share one mark.
- **The manager writes it**, on an adapter's report to the new, optional
  `POST /channel/read`. The console still performs no Kubernetes write. An
  adapter that never reports stays fully conformant.
- **The watermark is monotonic and clamped to the manager's clock**, so a stale
  browser cannot un-read a thread and a skewed one cannot mark future activity
  read.
- **Marking read is not behind `console.write.enabled`.** It instructs no agent
  and starts no work, and a read-only console is exactly the install where an
  unread badge earns its keep. *Close* and *Delete* stay hidden.

**The demo wires the console.** Where the k8s bundle renders a route, that route
now also claims the console's signal source and binds the console as a channel.

A turnkey install (`global.demo.enabled=true`) can therefore start a conversation
in the console immediately. It previously installed the console inert — source
`Wired=False`, composer unavailable, no answer ever delivered.

The names come from a new `global.agentops.console` block, because a subchart
reads no other parent scope and Helm cannot derive one value from another.

### Upgrade

**The CRDs MUST be re-applied.** `Conversation.status.threads[]` gains two
fields, and the API server prunes what its schema does not know. With stale CRDs
every read report answers 200 and changes nothing, so every conversation reads as
unread forever and no amount of clicking clears it.

```sh
kubectl apply -f chart/files/crds/          # or helm upgrade with crds.enabled
```

**Threads bound before this upgrade are treated as READ, once.** A binding
without the new `readTracked` marker predates the mechanism and cannot be told
from one nobody has read.

The alternative announces a namespace-sized backlog nobody can act on. The list
is quiet immediately after the upgrade and fills as new activity arrives.

**One combination now fails the render:** demo mode with the console disabled.
The published names duplicate `console.signalSourceName` / `console.channelName`,
so the render fails when they disagree rather than leaving a route claiming a
source nothing rendered.

```sh
# demo + console.enabled=false must also clear the published names
--set console.enabled=false \
--set global.agentops.console.signalSource= \
--set global.agentops.console.channel=
```

Outside demo mode nothing changed, and `console.enabled=false` still removes
every console object with one value.

## [5.17.0] — 2026-08-15

### Added

**`/exit`, in a conversation's own thread, deletes that conversation's runtime
pod and nothing else.** The conversation, its threads, its inputs, its run
history and its context handle all survive. The next message admits it again with
a fresh pod.

It exists for the half eviction cannot serve. Eviction takes the longest-idle pod
when a conversation is WAITING for capacity.

With nothing waiting, nobody is blocked. The pod holds its slot, its checkout and
whatever its runtime keeps resident until the idle TTL expires. Installs that
RAISE that TTL — to avoid re-cloning a large repository or re-warming a local
model — wait longest.

**It is not `/close`.** One word apart, and the difference is a thread:

| | releases the pod | ends the conversation | archives the thread |
|---|---|---|---|
| `/exit` | yes | no | no |
| `/close` | yes | yes | yes |

`/agents` now lists both with that distinction.

**It refuses rather than forces.** A `/exit` during a run is declined, naming the
run and offering `/close`. A `/exit` with queued input is declined because the
pod would be recreated immediately.

The mid-run refusal is correctness, not manners. Deleting a pod mid-run creates
the replacement at once, hands it nothing from `/work`, idles it out the LONG TTL
and reaps it.

That clears the inflight state, makes the input pending again, and re-runs work
that may already have acted.

The reply says what the release cost, using the same computation dispatch uses.
Where the runtime can carry context across a pod loss it says so. Where it cannot
it warns that the next message starts fresh.

One consequence worth knowing: a Pipeline named after a manager command (`exit`,
`close`, `agents`, `help`, `start`) is not reachable by that command. The
interception precedes the Pipeline lookup, which is what makes the commands
reliable.

### Upgrade

Nothing. No CRD change, no contract change.

## Older versions

| Archive | Covers |
|---|---|
| [CHANGELOG-5.0-5.16.md](changelog/CHANGELOG-5.0-5.16.md) | chart 5.0.0 through 5.16.0 |
| [CHANGELOG-1.0-4.0.md](changelog/CHANGELOG-1.0-4.0.md) | chart 1.0 through 4.0.0 |
