# Changelog

Chart-version migration guides — the upgrade steps for every breaking change,
newest first. See [README.md](README.md) for the product overview and
[docs/](docs/) for reference material.

Entries are keyed by CHART version; the manager image tag moves independently.

## Unreleased

### prometheus-bundle gets its network policies — chart 5.24.0

Its metrics MCP server was the third one, and the last unprotected: it
authenticates nobody, so any pod in the cluster could query the whole metrics
backend through it. `global.agentops.networkPolicy.enabled` now restricts it to
runtime pods, like the other two.

The webhook adapter is restricted only once you name the sender:

    prometheus-bundle.alertmanager.webhookFrom:
      - namespaceSelector:
          matchLabels:
            kubernetes.io/metadata.name: monitoring

**Empty leaves it reachable on purpose.** A policy that selects the adapter and
names nobody denies the alert lane, silently, and that is discovered during an
incident. Under-restricting is the recoverable mistake here.

Its MCP server also moves to an exec probe over loopback, for the reason the
Kubernetes one did: the kubelet probes from the node, no policy peer can name a
node, and this server is reached on the port it serves on — so the probe cannot
be opened without undoing the restriction.

### The Alertmanager adapter drops its vendor name — chart 5.24.0

`signal-vmalertmanager/` is now **`signal-alertmanager/`**, and its image is
**`kmatsebora/agentops-signal-alertmanager`**.

The adapter reads the STANDARD Alertmanager webhook payload, which vanilla
Alertmanager and VictoriaMetrics both send. The vendor name described one
sender, not the component — the chart was renamed to `prometheus-bundle` in
5.13.0 for that reason, and this finishes the job.

**Nothing to migrate unless you pin the image yourself.** The chart's default
moves to the new repository at the same tag (`0.6.0`, identical behaviour). The
`SignalAdapter` CR is still named `alertmanager`, so `SignalSource.spec.adapter`
values are unchanged, and the Deployment's selector label
(`agentops.dev/signal-adapter`) is untouched — no immutable-field upgrade
failure. The old image is left published for installs pinned to an older chart.

If you set `prometheus-bundle.alertmanager.image.repository` explicitly, point
it at `kmatsebora/agentops-signal-alertmanager`.

**What deliberately keeps VictoriaMetrics names**, because it names a
VictoriaMetrics API object rather than our component:

- `register.go` writes a **`VMAlertmanagerConfig`** through
  `operator.victoriametrics.com`. Self-registration is the one thing that is not
  standardised — vanilla Alertmanager's config is a file with no object to
  write, which is why NOTES.txt prints a receiver stanza for it instead.
- `metrics.vmServiceScrape` renders a **`VMServiceScrape`**, and the rules
  component renders **`VMRule`**. Both are VictoriaMetrics operator CRDs.

Renaming either would name a thing that does not exist.

### Bound component reach — chart 5.23.0

Two things were true and neither was written down. Both are now closable, and
both are **off by default**, so upgrading changes nothing until you ask.

**Nothing restricted who may reach this release's components.** The MCP servers
accept any caller with no credential — under `rbacMode: full` the Kubernetes one
runs as cluster-admin, so reaching it *is* cluster-admin. The manager's work
contract takes no credential either, which means any pod could take a queued
work unit or post a forged agent answer.

    global.agentops.networkPolicy.enabled: true

renders one NetworkPolicy per component, allowing only the callers your wiring
implies. Name two things when you enable it or they break quietly:
`networkPolicy.metricsFrom` for a collector outside the namespace, and
`networkPolicy.consoleFrom` for your ingress controller.

**Read the note it prints.** A NetworkPolicy on a cluster whose CNI does not
enforce policy applies cleanly, shows up in `kubectl get`, and blocks nothing.
The chart cannot detect that, so NOTES.txt tells you how to check instead of
calling your components protected.

**A route's toolsets bound only a cooperating agent.** `--allowedTools` is
applied by the CLI beside the agent, and an MCP server has never heard of an
`MCPToolset` — so an agent with a shell reached a bound server directly and
called anything it registered. `agentops-shell` is bound on ordinary routes.

    runtime.egressMediation.enabled: true

puts a proxy in the runtime pod that the agent's traffic cannot route around,
and enforces the bound toolsets there. Two things to know first: it adds a
**privileged init container** (refused by a namespace under `restricted` Pod
Security admission), and a container per active conversation.

stdio servers and `https` MCP endpoints stay unenforceable, and are reported on
the conversation's new `EgressMediated` condition rather than passed off as
covered.

**Two things to name when you enable network policy, or a workload crash-loops.**
The kubelet probes from the NODE, and no policy peer can name a node. The
manager's probe port serves only health, so it is opened unconditionally. The
Kubernetes MCP server probes the same port it serves MCP on, so it now probes
itself over loopback instead — no policy on any CNI can block that. If your CNI
does not exempt host traffic, set `networkPolicy.probesFrom` to your node
network for anything else that probes over the network.

Decided in [docs/adr/0001-bound-component-reach.md](docs/adr/0001-bound-component-reach.md).

### The console renders the message you typed — console 0.15.9

**No action on upgrade.** Bump the console image tag (the chart default moves
with it). Nothing else changes.

A conversation started from the composer showed a transcript that began at the
**agent's answer**, with the question that caused it missing. Typing into an
already-open conversation was fine — only the opening message vanished.

**Cause.** The manager posts an input to a conversation's bound threads only
when the person has not already seen it: an alert gets a signal card, and a
message somebody typed gets nothing, because posting it back would be an echo on
the surface it was typed on.

That rule is right for a transport and wrong for a **viewer**. A Telegram user's
own message is already in their thread, put there by Telegram. A console user's
is not — the console renders a thread from what it was SENT, so an input nobody
sends it is an input nobody can read. The input is then pruned once processed,
so nothing could recover it afterwards.

**Fix, console-side only.** The console now watches conversations and records
what people typed into its own transcript buffer, keyed on the input id so the
many watch events one conversation produces render it once. The set is read off
the manager's own rule rather than guessed: an input the manager will not post
BECAUSE THE PERSON TYPED IT is exactly the input the console must render itself
(`origin.kind = channel`, and `origin.kind = signal` with `signalKind = chat`).

An input with **no** origin predates provenance and is skipped, for the reason
the manager skips it: it cannot be told from an alert, and inventing the wrong
bubble is worse than a missing one.

Three things the first cut got wrong, fixed before it was called done:

- **It read as TYPED.** An addressed task (`/ha-control turn the AC on`) reaches
  the conversation as the REST — the manager consumed the address deciding who
  answers — so the stored payload starts mid-sentence. The console posted the
  whole thing and is the only component that still has it.
- **It carries the starter's identity.** The input records provenance, not
  authorship, so without this the opening message read `local` while the reply
  below it read your address: one thread naming one person two ways.
- **A reply is not duplicated.** Typing into an open conversation already puts
  the message on screen, and the input it becomes is the DURABLE IDENTITY of
  that bubble rather than a second one. It is adopted, keeping its id — handing
  the same text a new id is how the duplicate would come back through the live
  stream instead of through the buffer.

The UI also stopped printing `local` as a speaker's name. The transcript kinds
are plumbing vocabulary: `local` means "typed on this console", which is a fact
about where a message entered, not a person.

Unchanged: alerts keep their manager-posted card, and a console restart still
loses what was never CR state.

### The Home Assistant bundle, and a rule about bundle wiring — chart 5.22.0

**No action on upgrade.** Nothing new is on by default. `ha-bundle.enabled`
defaults `false` and demo mode never turns it on, so an existing install renders
exactly what it rendered before.

#### The rule that changed, and what it does NOT permit

A subchart may render a `Pipeline` when — and only when — **all** of these hold:

1. rendering is behind an explicit wiring flag,
2. every reference to an object the bundle does not itself render is a
   values-supplied NAME, omitted when unset,
3. each `Pipeline` renders only with its own profile, and
4. the flag **defaults off**, forced on by nothing but a values path whose
   declared purpose is a turnkey install.

This does **not** make bundle-shipped wiring the norm. The rule's harm is a
subchart wiring only its own lane because it cannot see the others, and a bundle
whose sources and channels come from elsewhere still cannot meet condition 2.
`telegram-bundle` continues to ship none, and is the counter-example: a chat
surface is answered by an agent from somewhere else.

`ha-bundle` is the third bundle to qualify, after `k8s-bundle` and
`prometheus-bundle`. It owns its whole lane — the source, both profiles and both
toolsets — so chat sources and channels are the only foreign names.

#### The bundle

`chart/charts/ha-bundle/` ships a **privilege split** rather than one agent:

| Agent | Profile | Reached by | Job |
|---|---|---|---|
| The house's user | `ha-user` | an ordinary chat message | **use** the house — services, lights, automations |
| The administrator | `ha-operator` | `/ha-ops <task>`, by name | **fix** it — integrations, configuration, repairs |

The split is **use versus fix**, not read versus act: Home Assistant has no
read-only role, so neither credential merely looks. What separates the lanes is
the REST path — Assist intents reach no configuration, so repairing needs a
shell, and only the ops route binds one.

The acting route claims the log source and **no chat source**, so escalating is
a deliberate act. That is not etiquette: claiming and addressing are independent
mechanisms, so listing a chat source there would grant it nothing while making
every unaddressed message on that surface ambiguous.

The **operator credential gates the fixing half**, and the ingest lane needs it
too: Home Assistant's `subscribe_events` is admin-only, so a control token
connects, passes auth, and is then refused the subscription.

There is **no MCP server workload**: Home Assistant serves its own endpoint
through the built-in MCP Server integration.

**The ops lane gets a second MCP server**, off by default (`adminMcp.enabled`).
Home Assistant's built-in server exposes Assist intents only — it turns things on
and off and cannot read a log, reload an integration or disable an entity, which
live in registries served over the WebSocket API. Bound to `ha-ops` alone, so
`ha-control` reaches a server that has no such tools: two walls, not one
allowlist.

Two ways to have one, both off by default and failing the render if you enable
the config with neither: let the chart deploy
[ha-mcp](https://github.com/homeassistant-ai/ha-mcp) in-cluster
(`adminMcpServer.enabled`), or point `adminMcp.url` at a server you run —
including a HACS integration inside Home Assistant. **Add-ons are not an option
on Home Assistant Core**, which is the usual shape in Kubernetes.

Of the 78 tools that server registers, **52 ship**. The 26 withheld restart Home
Assistant, manage backups, delete registry objects or install software. The
toolset is enumerated and the image tag pinned, because the allowlist names
tools: a server that renames one changes what it means with nothing failing.

**The ops role names the REST path explicitly**, because Home Assistant's MCP
server exposes Assist intents only. Without that, the agent reads its own tool
list, finds device controls and no way to reach a log or a config entry, and
reports the task impossible. Its prompt carries `$HA_URL`, `$HA_TOKEN` and the
calls it needs — error log, states, config entries, reload, service call, config
check — all verified against a live instance.

#### New module: `signal-ha/`

A dependency-free signal adapter reading the instance's WebSocket API over a
hand-written RFC 6455 client. It watches `system_log_event` and carries the same
`rules` / `route` vocabulary as the cluster Events adapter, minus the time axis.

`kubernetesAccess: false` — its data source is the house, not the cluster.

Image: `kmatsebora/agentops-signal-ha:0.1.0`.

#### Enabling it

The Secrets are referenced by name and never created by the chart. Each carries
one token under **two** keys, because the adapter sends the raw token and the
MCP path sends a complete header value:

```sh
kubectl -n <ns> create secret generic ha-admin \
  --from-literal=token="$TOKEN" --from-literal=authorization="Bearer $TOKEN"
```

Then set the endpoint, the credentials and — deliberately — the routes. See
[docs/ha-bundle.md](docs/ha-bundle.md).

### Context survives a corrupt volume — chart 5.21.0

**No action on upgrade.** No values change is required and nothing new is on by
default. `helm upgrade` with the new image tags, and rolling back is reverting
them. Three new features are opt-in and independently switchable.

On 2026-08-20 a node reboot corrupted the ext4 filesystem on the shared
`agentops-home` volume. Longhorn reported the volume **healthy** the whole time,
correctly: it replicates blocks, and all three replicas agreed on the corrupt
ones.

Every runtime pod mounts that volume. Five pods sat in `ContainerCreating` for
fifteen hours, held every capacity slot, and starved six more conversations.
The install was completely down and **said nothing** — no condition, no event,
no log line. The only condition present read `DeliveryPending=False /
AllDelivered`, which looks like health.

#### What changed by default

- **A runtime pod that never starts is now reaped** after
  `RUNTIME_START_DEADLINE` (10m), with per-conversation exponential backoff.
  It still counts against the cap until it is gone — un-counting it would
  provision past the cap against resources the cluster has not released.
- **`Conversation.status` carries `RuntimeStarted`**, whose message is the
  kubelet's own words. A bare "deadline exceeded" would have reproduced the
  original failure with a timer attached.
- **The storage breaker gained a second edge.** It already treated many failed
  continuations as an outage. It now also counts pods that cannot be provisioned
  for a storage reason. That is why it never fired for the incident it was
  written for — no pod started, so no run existed to report. While open it holds
  work in `Pending` with a reason that says STORAGE rather than queue, and
  re-tests with one canary.
- **New metrics**: `agentops_storage_outage`, `agentops_storage_outage_seconds`,
  `agentops_context_operations_total`, `agentops_context_checkpoint_bytes`.

#### What is opt-in

| value | default | what it does |
|---|---|---|
| `runtime.contextSync.paths` | `[]` | moves the live context to pod-local storage, keeping a snapshot on the volume |
| `rbac.drainAware` | `false` | releases idle runtime pods from a cordoned node so the filesystem unmounts cleanly |
| `contextProbe.enabled` | `false` | hourly mount probe, so a damaged idle volume is found in an hour rather than at next use |

**`contextSync` needs `paths`** — only the runtime knows where its backend keeps
context, and the chart must not guess. For the reference runtime:

```yaml
runtime:
  contextSync:
    paths: [".claude/projects/-data-workspace/**"]
```

With it set, the agent container gets ephemeral storage and **no mount of the
durable volume at all**. Only the sidecar holds it. A run already going then
survives the volume going bad underneath it, and an agent can neither read
another conversation's context nor corrupt the filesystem.

Absent, the pod is built exactly as before, and an install that never sets
`paths` needs no migration at all.

**Opting in strands existing context, and the strand is visible.** Without the
sidecar, context sits at the claim ROOT. With it, each conversation reads a
per-conversation subdirectory, which starts empty. Every conversation that
already has a context handle will therefore FAIL its next run rather than answer
without memory — which is the continuity rule working, not a defect.

Recover each one with the new reset verb, which clears the handle and lets it
start fresh:

```sh
curl -sX POST "$MANAGER/channel/conversations/<name>/reset-context" \
  -H "Authorization: Bearer $ADAPTER_TOKEN" -H 'Content-Type: application/json' \
  -d '{"channel":"<a channel it is bound to>"}'
```

Enable `contextSync` on a quiet install, or accept that live conversations lose
their memory once and say so.

**`rbac.drainAware` costs the manager its first cluster-scoped grant** (nodes:
get/list/watch, read-only). Every other permission it holds is namespaced. It
shrinks the corruption window and does not close it, because the storage
provider picks where a shared volume is served independently of where runtime
pods run.

#### New verb

`POST /channel/conversations/{name}/reset-context` clears a conversation's
context handle, keeps the conversation, its threads and its history, and states
the loss on every bound thread. It is explicit and operator-initiated — a failed
continuation never triggers it, because an automatic version would be
indistinguishable from the silent degradation the continuity rules forbid.


### Rate-limited replies are no longer lost — chart 5.20.0

**No action on upgrade.** No CRD change, no values change: `helm upgrade` with
the new image tags, and rolling back is reverting them.

On 2026-08-13 a 44-alert burst was rate-limited by Telegram. Every forum topic
was created, and 22 of them stayed **completely empty** for four and a half
hours while the answers sat in `status.runs[].result` — with the operator
reporting itself healthy.

The cause was one line of asymmetry. The manager's completed-operation window
recorded operations that were **attempted**, and `enqueue` suppresses any stable
id in that window — which is every derivable op. So a failed send disabled its
own recovery: reconciliation re-derived it, dedup dropped it, nothing posted, and
no restart helped, because the re-derivation was what was being suppressed.
`ensure-topic` recovered only because its failure path had been releasing the
entry all along.

- **The window now records what SUCCEEDED.** A failed derivable op releases its
  dedup entry and reconciliation re-derives it. `delete-conversation` is the one
  exemption — its Conversation is disappearing, so there is nothing to re-derive
  from.
- **An owed answer is now visible**: the new `DeliveryPending` condition on a
  Conversation names the channels a recorded run has not reached, and clears
  when they all have.

  ```sh
  kubectl get conv -o json | jq -r '.items[] | select(.status.conditions[]?
    | select(.type=="DeliveryPending" and .status=="True")) | .metadata.name'
  ```
- **Telegram paces itself** — 30 sends/second per bot, 20/minute per chat — and
  honours `retry_after` exactly, retrying in-process instead of reporting a
  failure. Expect a burst to take **minutes** to appear in full: every topic in
  a forum shares one `chat_id`, so ~144 calls against 20/minute is over seven
  minutes. That is Telegram's limit, and the alternative is the old behaviour,
  which lost the messages.
- **`/channel/ops` advertises `reclaimAfterSeconds`**, so an adapter absorbing
  backpressure can bound its waiting inside the claim window rather than
  hardcoding a constant that drifts from the manager's. Additive and optional —
  every existing adapter ignores it.

Rolling this out **re-posts what the incident lost**, including cards to topics
that did receive their answer. Those were never posted, so it is correct, but it
will look like a flood — roll during a quiet window.

### Read state becomes per person — chart 5.19.0

**Additive, and the CRDs must be re-applied again** — `status.threads[]` gains
`readers[]` and `spec` gains `originReader`. Same pruning trap as 5.18.0, and the
same fix:

```sh
kubectl apply -f chart/files/crds/          # or helm upgrade with crds.enabled
```

If any CRD on your cluster was ever applied by hand with `kubectl apply -f`, that
left a `kubectl-client-side-apply` field manager owning `.spec.versions`, and
helm's server-side apply refuses it — rolling the WORKLOADS while silently
leaving the schema behind. Add `--force-conflicts` (helmfile:
`--args "--force-conflicts"`), which only moves ownership. Diagnose with
`kubectl get crd conversations.agentops.dev --show-managed-fields -o json`.

Where the console can tell readers apart, unread is now **per person**: your
badge is yours, and a colleague clearing theirs does not clear yours.

- **No identity is stored.** The console hashes the signed-in identity with a
  salt projected as a channel credential and sends only that opaque key. The
  manager stores it verbatim and cannot reverse it — a conversation records that
  N people read it and when, never who. `/channel/read` refuses a key containing
  `@`, since it cannot otherwise tell a hash from an address.
- **The salt is generated for you.** On install, and on the first upgrade that
  finds the console Secret without one — the only time that Secret renders
  outside an install. It adopts the existing `uiToken` rather than reissuing it,
  so nobody is signed out. Pin it with `console.auth.readerSalt` if you prefer.
  **Rotating it orphans every stored key** and resets everyone to the
  channel-wide mark.
- **A shared UI token is one reader**, because there is one credential and no
  person behind it. With no salt projected, likewise. Both are exactly the
  5.18.0 behaviour, reached without a special case.
- **`readAt` stays** as the channel-wide mark for transports with no reader
  identity. `POST /channel/read` gains an OPTIONAL `reader` field, so
  `channel-telegram` and every other adapter are untouched and fully conformant.
- **Starting a conversation now marks it read for you** — and only you. The key
  travels with the chat signal into `spec.originReader`, and the manager stamps
  that reader when it creates their thread. Before this, a conversation you had
  just typed came back unread before any answer could exist.
- **`readers[]` is bounded at 50 per binding**, oldest watermark evicted first.
  Eviction is not a loss: an evicted reader falls back to the channel-wide mark,
  the same as a teammate who joined today. Neither is handed a backlog.

### The demo wires the console — chart 5.18.0

**No action for an ordinary install.** Where the k8s bundle renders a route, that
route now also claims the console's signal source and binds the console as a
channel. A turnkey install (`global.demo.enabled=true`) can therefore start a
conversation in the console immediately. It previously installed the console
inert: source `Wired=False`, composer unavailable, no answer ever delivered to
it.

The names come from a new `global.agentops.console` block, because a subchart
reads no other parent scope and Helm cannot derive one value from another. They
duplicate `console.signalSourceName` / `console.channelName`, so the render
**fails** when the two disagree rather than leaving a route claiming a source
nothing rendered.

**One combination now fails the render:** demo mode with the console disabled.

```sh
# demo + console.enabled=false must also clear the published names
--set console.enabled=false \
--set global.agentops.console.signalSource= \
--set global.agentops.console.channel=
```

Outside demo mode nothing changed, and `console.enabled=false` still removes
every console object with one value.

### Unread conversations in the console — chart 5.18.0

**Additive, but the CRDs MUST be re-applied.** `Conversation.status.threads[]`
gains two fields, and the API server prunes what its schema does not know: with
stale CRDs every read report answers 200 and changes nothing, so every
conversation reads as unread forever and no amount of clicking clears it.

```sh
kubectl apply -f chart/files/crds/          # or helm upgrade with crds.enabled
```

The conversation list now marks conversations whose **console thread** has
activity newer than its watermark, offers an *Unread only* filter and a **Mark
read** action over a selection, and carries the unread count on the navigation.
Opening a conversation clears its mark.

- **Read is per CHANNEL.** The watermark lives on `status.threads[].readAt`, one
  per bound channel, so reading a conversation in Telegram does not clear it in
  the console and vice versa. Two operators sharing one console share one mark —
  whoever opens a conversation clears it for both. A per-person mark would need
  a per-user identity store the console does not have.
- **The manager writes it**, on an adapter's report to the new, OPTIONAL
  `POST /channel/read`. The console still performs no Kubernetes write. An
  adapter that never reports stays fully conformant; `channel-telegram` and the
  rest are unchanged.
- **The watermark is monotonic and clamped to the manager's clock**, so a stale
  browser cannot un-read a thread and a skewed one cannot mark future activity
  read.
- **Marking read is not behind `console.write.enabled`.** It instructs no agent
  and starts no work, and a read-only console is exactly the install where an
  unread badge earns its keep. The selection column is therefore present on a
  read-only console too, while *Close* and *Delete* stay hidden.

**Threads bound before this upgrade are treated as READ, once.** A binding
without the new `readTracked` marker predates the mechanism and cannot be told
apart from one nobody has read — the same problem, and the same fix, as
`status.runs[].deliveryTracked`. The alternative announces a namespace-sized
backlog nobody can act on. So the list is quiet immediately after the upgrade
and fills as new activity arrives; that is the one-time behaviour, not a bug.

### `/exit` releases a conversation's runtime — chart 5.17.0

**Additive. No CRD change, no contract change, nothing to do on upgrade.**

A runtime pod holds its slot until it times out. Eviction covers half of that —
when a conversation is WAITING for capacity, the longest-idle pod is taken. The
half with no answer was when nothing is waiting: nobody is blocked, so nothing
evicts, and the pod goes on holding its slot, its checkout and whatever its
runtime keeps resident until the idle TTL expires. Installs that RAISE that TTL,
to avoid re-cloning a large repository or re-warming a local model on every
message, are exactly the ones where that wait is longest.

`/exit`, in a conversation's own thread, deletes that conversation's runtime pod
and nothing else. The conversation, its threads, its inputs, its run history and
its context handle all survive; the next message admits it again with a fresh
pod.

**It is not `/close`.** One word apart, and the difference is a thread:

| | releases the pod | ends the conversation | archives the thread |
|---|---|---|---|
| `/exit` | yes | no | no |
| `/close` | yes | yes | yes |

`/agents` now lists both with that distinction.

It refuses rather than forces. A `/exit` during a run is declined, naming the run
and offering `/close`; a `/exit` with queued input is declined because the pod
would be recreated immediately. The mid-run refusal is correctness, not manners:
deleting a pod mid-run creates the replacement at once, hands it nothing from
`/work`, idles it out the LONG TTL and reaps it — which clears the inflight
state, makes the input pending again, and re-runs work that may already have
acted.

The reply says what the release cost, using the same computation dispatch uses to
decide whether to hand back a context handle at all: where the runtime can carry
context across a pod loss it says so, and where it cannot it warns that the next
message starts fresh — the loss the idle TTL would have caused anyway, said while
somebody is choosing it.

One consequence worth knowing: a Pipeline named after a manager command (`exit`,
`close`, `agents`, `help`, `start`) is not reachable by that command. The
interception precedes the Pipeline lookup, which is what makes the commands
reliable.

### Telegram may delete a conversation's topic — chart 5.16.0

**Opt-in, off by default. Nothing changes on upgrade.**

Deleting a conversation leaves its forum topic archived with a tombstone, which
keeps the transcript and costs one dead topic per conversation. A busy group can
now trade the other way:

```yaml
telegram-bundle:
  surface:
    deleteTopicOnDelete: true
```

The topic is deleted instead, and no tombstone is posted. **This destroys the
transcript.** The bot needs `can_delete_messages`; without it the operation is
reported failed rather than quietly falling back to archiving, and the
conversation is still deleted once the grace expires.

The setting lives on the `Channel`, so two Telegram surfaces served by one
adapter can differ. Closing is unaffected — a closed conversation can be
reopened into its topic, so that topic must survive.

A line naming the conversation goes to the chat general surface, so a vanished
topic is attributable to agent-ops rather than looking like a hand deletion.

Image: `channel-telegram` `0.11.0`.

### A deleted conversation tells its threads — chart 5.15.0

**No action needed.** Nothing is configured, nothing changes default.

A closed conversation's threads are told it can be reopened. Deleting it made
that false and said nothing, so the last message a chat thread carried was a
promise nobody could keep — and on an already-closed conversation the deletion
was completely silent, because its topics were already archived.

Deletion now sends a new outbound operation, `delete-conversation`, once per
bound thread: the conversation and its record are gone, and a new message starts
a new one. It **replaces** `close-topic` on the deletion path, so a conversation
being deleted gets one operation, never both.

It is named for the conversation rather than the topic because a conversation is
what ended; what that means for a thread is the adapter's decision.
`channel-telegram` un-archives the topic, posts the notice and closes it again —
a closed forum topic refuses `sendMessage`, and an open one would invite replies
into a conversation that no longer exists. It does **not** delete the topic: the
history above the tombstone is what a person scrolls back to.

**For adapter authors:** implementing the kind is optional. An adapter that does
not is reported failed, the 2-minute grace expires and deletion proceeds — the
same posture `close-topic` always had. Implement it if your transport can say
something useful; the notice arrives as an ordinary `notice` message.

Images: manager `0.31.0`, `channel-telegram` `0.9.0`, console `0.12.0`.

### Closing a conversation no longer deletes it — chart 5.14.0

**Action needed by nobody at upgrade time, but `/close` means something
different from the moment you upgrade.** Both new windows are off by default, so
nothing is reclaimed that was not reclaimed before — and nothing is DESTROYED
that was destroyed before either.

**APPLY THE CRDs BEFORE THE MANAGER IMAGE.** `Conversation` gains phase `Closed`
plus `status.closedAt`, `status.threadsArchived[]` and `status.reopens`. A
manager on this version against the previous CRD cannot close anything: the API
server rejects the phase, and because closing is retried, it fails in a loop —

```
status.phase: Unsupported value: "Closed": supported values: "Pending", "Idle", "Queued", "Working"
```

A normal `helm upgrade` handles this on its own (the chart ships CRDs as
templates and `crds.enabled` defaults `true`). It bites installs that manage
CRDs out of band — `crds.enabled: false`, a separate GitOps stage, or a cluster
where several releases share one set. The schema change is purely additive (a
wider enum and three optional fields), so applying it early is safe: an older
manager never writes any of them.

Closing and reclaiming used to be one act. `/close` deleted the `Conversation`,
and with it `status.runs[].result`, the context handle and eventually the
workspace directory. That made closing irreversible, which is precisely why
nobody closed anything and why conversations accumulated without bound.

**What changes.** A closed conversation now sits at phase `Closed`: no runtime
pod, no MCP ConfigMap, no dispatch, no capacity consumed, absent from
conversation reuse and from every pipeline — with its object, its recorded
answers, its context handle and its volume state all intact. It shows up in
`kubectl get conversations` as a `Closed` row until you delete it or enable
autodelete.

```sh
kubectl -n <ns> get conversations --field-selector status.phase=Closed
```

**It can be reopened**, from the console, back to `Idle` with its wiring and its
history — the materialized refs are left exactly as they are, so no Pipeline
edit leaks into a conversation that already exists. Threads are re-established
through the ordinary `ensure-topic`, carrying the archived thread id as a hint:
an adapter that can un-archive continues in the same thread, one that cannot
opens a fresh one, and ignoring the hint is a valid implementation.

**To get the old behaviour** — closing that reclaims — enable autodelete with a
short window. That is the old semantics with a window bolted on:

```yaml
retention:
  autodelete:
    enabled: true
    closedAge: 1h
```

**Two windows, two clocks, both off by default:**

```yaml
retention:
  autoclose:                 # close a FINISHED conversation after it goes quiet
    enabled: true
    idleAge: 168h            # measured from LAST ACTIVITY, never from creation
  autodelete:                # delete a CLOSED conversation
    enabled: true
    closedAge: 720h          # measured from status.closedAt; a reopen resets it
```

`autoclose` closes only a conversation that is genuinely finished: `Idle`, no
pending inputs, no inflight run, no runtime pod, **and** every recorded run
delivered to every bound thread. That last clause is not decoration — a
conversation goes `Idle` the moment its result is recorded, while the reply may
still be an unclaimed `send` op, so closing on `Idle` alone can archive a thread
out from under its own answer.

**Choose `closedAge` as "how long do I want to be able to read this", not "how
long until it is tidy".** `status.runs[].result` is the only place an answer
lives in the Kubernetes API; the console projects its transcript from the CR,
and metrics keep aggregates only. For a conversation bound to no channel there
is no transport copy anywhere.

**New: an opt-in housekeeping CronJob reclaims disk.** Only something that
mounts the claim roots can, and the manager mounts no volume by invariant:

```yaml
housekeeping:
  enabled: true
  dryRun: true               # take this off only after a run looks right
```

It removes workspace directories with no `Conversation` of that name, and
session transcripts no conversation references that are older than
`sessionGrace`. A **closed** conversation still has a CR, so its state is
protected by the same rule that identifies an orphan — the job is phase-blind on
purpose, and a "only look at live ones" optimisation would reclaim the workspace
of every conversation you were keeping.

It runs under its own ServiceAccount with read-only access to conversations,
never the runtime SA — mounting the claim root is exactly the reach `subPath`
isolation denies agents, and the render fails if the two identities are set
equal.

**Enabling autodelete without the job reclaims the API half and leaves the
disk.** That is correct with persistence off and a silent leak with it on.

**Recommended order:** upgrade, watch the `Closed` rows for a while, use reopen
once to confirm it works, then turn autoclose on with a long window, then the
job with `dryRun`, and autodelete last.

### vm-bundle is now prometheus-bundle — chart 5.13.0, prometheus-bundle 0.2.0

**Action needed if you set any `vm-bundle.*` value. The render FAILS until you
rename the key** — deliberately: Helm never reports an unread values key, so an
unguarded rename would present as a successful upgrade that installed nothing.

The bundle was named for a vendor it does not depend on. Its ingest core parses
the standard Alertmanager webhook payload and nothing else, so any Prometheus
Alertmanager could always post to it; and VictoriaMetrics answers the Prometheus
HTTP query API (`/api/v1/query`, and `buildinfo` reports a Prometheus version)
with MetricsQL as a PromQL superset, so one query server serves both backends.
VictoriaMetrics is now a supported backend rather than the subject.

**1 — rename the values key.** Every `vm-bundle.*` value moves to
`prometheus-bundle.*`. Nothing else about the ingest lane changed.

**2 — the adapter CR name default changed** from `vm-alertmanager` to
`alertmanager`, and the default source's from `vm-alerts` to `alerts`. The
adapter name is the ROUTING KEY every `SignalSource` names in `spec.adapter`, and
the source name is the `{source}` segment of the webhook URL. Restore both with
one value each rather than editing hand-written sources or reconfiguring your
sender:

```yaml
prometheus-bundle:
  alertmanager:
    name: vm-alertmanager
    defaultSource:
      name: vm-alerts
```

**3 — the logs component is REMOVED, not ported.** `mcp.vmlogs` and the
`mcp-victorialogs` workload are gone: VictoriaLogs speaks LogsQL over its own
endpoints, and no Prometheus query server can reach it, so there was nothing to
generalize. If you used it, apply these two objects by hand — the same pair the
bundle used to render — and keep binding them from your Pipeline:

```yaml
apiVersion: agentops.dev/v1alpha1
kind: MCPConfig
metadata:
  name: vm-logs
spec:
  servers:
    victorialogs:
      type: sse
      url: http://mcp-victorialogs.<ns>.svc:8080/sse   # your old mcp.vmlogs.url
---
apiVersion: agentops.dev/v1alpha1
kind: MCPToolset
metadata:
  name: vm-logs-tools
spec:
  tools:
    - "mcp__victorialogs__*"
```

You also need the server itself, which the bundle no longer deploys:
`ghcr.io/victoriametrics/mcp-victorialogs`, env `MCP_SERVER_MODE=sse`,
`MCP_LISTEN_ADDR=:8080`, `VL_INSTANCE_ENTRYPOINT=<your VictoriaLogs URL>`.

**4 — `mcp__victoriametrics__*` stops resolving.** The metrics server key is now
`prometheus`, so allowlists naming the old namespace keep rendering and quietly
grant nothing — the allowlist-rot failure this project names elsewhere. Find
every affected Pipeline and toolset:

```sh
kubectl get pipelines,mcptoolsets -A -o yaml | grep -n "victoriametrics\|victorialogs"
```

Rebind to the bundle's new pair — `MCPToolset/prometheus-observability` and
`MCPConfig/prometheus-api`. The toolset is wildcarded (`mcp__prometheus__*`)
because all six tools the server registers are read-only; there is no read/mutate
split to preserve, unlike `k8s-bundle`'s.

**5 — new: the bundle ships a profile and its own wiring.**
`AgentProfile/alert-investigator` renders by default (identity only, with an
inline role — it has no repository, so no agent definition file can be resolved
for it). `pipelines.enabled` defaults **false** and nothing forces it on; demo
mode never enables this bundle at all. Turning it on renders one
`Pipeline/alert-triage` claiming the bundle's own source, and **every admitted
alert then opens a conversation and spends LLM credits**.

Sources are shareable, so the bundle's route and one you declared under the
parent chart's `pipelines:` may both claim `alerts` — that fans out to one
conversation per claiming Pipeline. NOTES.txt reports it; it is never refused.

**6 — self-registration is unchanged but now labelled VictoriaMetrics-only.** It
configures the sender by writing a `VMAlertmanagerConfig`, and vanilla
Alertmanager's configuration is a file or a Secret, not an object an adapter can
write. With registration off, NOTES.txt prints the exact `receivers:` stanza to
paste into your Alertmanager configuration — including `send_resolved: false`,
because the adapter drops non-firing alerts and a sender left at its default
posts resolutions that are silently discarded.

**No effect on the manager, the CRDs or any Go module.** The
`signal-vmalertmanager` module, its image and its spec keep their names: the
payload handling was always vendor-neutral, and renaming a published image would
be churn with a migration attached and no benefit.

### Demo mode wires itself — chart 5.12.0, k8s-bundle 0.3.0

**Action needed only if you run `global.demo.enabled=true`, or enable
`k8s-bundle` and already claim `cluster-events` from your own `pipelines:`.**

The k8s bundle gains a fourth component: its own wiring. Until now it rendered
the events adapter, the source, the profile and the MCP tooling — everything
except the one object that makes them do anything — so `global.demo.enabled` put
a complete Kubernetes agent in the cluster that answered nothing until you read
NOTES.txt and applied a Pipeline by hand.

**What changes for a demo install.** It now renders `Pipeline/k8s-observe`,
claiming `cluster-events` with the read toolsets and the `k8s-api` MCPConfig. The
source goes `Wired=True` and **every admitted Warning event opens a conversation
and spends LLM credits**, where before it dropped them. That is the fix, and it
is a real bill: on a noisy cluster, check the shipped suppression rules
(`docs/k8s-bundle.md`) before upgrading.

*Keep the old behaviour — the parts without the route:*

```sh
helm upgrade ... --set k8s-bundle.pipelines.enabled=false
```

That value is absolute: it declines the route even under demo mode, and every
other bundle component is untouched.

**What changes for everyone else: nothing.** The flag is OFF outside demo mode.
Enabling the bundle for its adapter and profile still renders no Pipeline, and
the parent chart's `pipelines:` stays where an install declares routes.

**If you already claim `cluster-events` yourself** and also turn this on, both
Pipelines render and the source **fans out** — one event, two conversations, two
profiles' worth of capability, two bills. That is legal (sources have been
shareable since 5.10.0) and deliberately not refused; the post-install notes name
the pipelines involved when it happens. If you meant only your own route, set
`k8s-bundle.pipelines.enabled=false`.

**Which route renders derives from `global.agentops.runtime.rbacMode`**, the same
value the MCP server's `--read-only` flag, the server SA's RBAC and the
`k8s-admin` toolset already follow:

| `rbacMode` | route | what it can do |
|---|---|---|
| `readonly`, `none`, `""` (incl. demo) | `k8s-observe` | reads the cluster, changes nothing |
| `full` | `k8s-operate` | binds `k8s-admin` — **mutates the cluster** |

So widening to `full` now promotes the route as well. `pipelines.observe.enabled`
and `pipelines.admin.enabled` are explicit booleans that win in both directions,
and both may be true at once.

**No channel is bound by default**, so answers land in
`status.runs[].result` — `pipelines.channels: [<name>]` delivers them to a
surface instead.

### Signal sources are shareable, and bare chat messages stop being guessed — chart 5.10.0

**Two behaviour changes. Both need action only if a `SignalSource` is listed by
more than one Pipeline — a state that reads as broken today, so most installs
have none. Check first:**

```sh
kubectl get pipelines -o json | jq -r '
  .items[] | .metadata.name as $p | .spec.signalSourceRefs // [] | .[] |
  "\(.name)\t\($p)"' | sort | awk '{c[$1]=c[$1]" "$2} END {for (s in c) if (split(c[s],a," ")>1) print s ":" c[s]}'
```

Anything printed is a source two or more Pipelines list. If nothing prints, this
release changes no behaviour for you.

**1. A shared source now routes to EVERY Pipeline listing it.** Exclusivity is
gone: `sourceConflicts`, the `SourceConflict` condition, and the oldest-claimant
tiebreak are deleted. A Pipeline that sat at `Ready=False, reason=SourceConflict`
becomes `Ready=True` and starts answering, so one alert on a shared source now
opens **two conversations — two agents, two runtimes, two LLM bills.**

A `SourceConflict` condition left on such a Pipeline is **cleared automatically**
on the first reconcile — deleting the rule that wrote it does not delete what it
already wrote, so the manager removes it for one release.

*Fix, if that is not what you want:* drop the source from every Pipeline but the
intended one.

```sh
kubectl patch pipeline <the-one-that-should-not-answer> --type=json \
  -p '[{"op":"remove","path":"/spec/signalSourceRefs/<index>"}]'
```

*If it IS what you want, nothing to do* — that is now a supported configuration.
Per-source cooldown and signature grouping are evaluated once, above the fan-out,
so a fingerprint is admitted once and delivered to each server; the ingest
response reports `queued` (signals) and `conversations` (one per server)
separately, and `receivedTotal` still counts signals.

**2. A bare message on a chat surface several Pipelines serve is refused.** With
one Pipeline serving the chat source, nothing changes. With several, an
unaddressed message opens no conversation and the surface is answered with the
Pipelines available and the `/<pipeline> <task>` form.

*Fix:* address the agent — `/<pipeline> <task>` — or drop the source as above.
**Thread replies are unaffected** and never needed a prefix; addressed messages
are unaffected too.

**`Conversation.spec.pipelineRef` is added** — provenance only, written at
creation, never read to resolve wiring. It is what keeps two Pipelines fanning
out from one source from appending to each other's conversation. Conversations
created before this release carry none and nothing backfills them; such a
conversation keeps grouping while one Pipeline serves its source, and is left
alone once a second joins. **Apply the CRDs before the manager image** — the
field is additive, so an older manager ignores it, but a newer one cannot write
it against an un-updated CRD.

**`/agents` now lists each Pipeline with its answering profile**, matching the
console's new composer typeahead — typing `/` in "New conversation" lists the
Ready Pipelines and inserts the addressed form. No new RBAC, no new manager
endpoint, no console values to set.

### Pod evictions stop opening conversations — chart 5.9.0

**No action required. Behavior change: you will stop getting a conversation per
evicted pod.** If you relied on that, see the restore snippet at the end.

`Evicted` moves from the past-tense tier (`for: "0"`, one signal per displaced
pod) into the tier-1 drop list. The reason is that an eviction was already
reported from both ends, and per pod from neither:

| Eviction | Still reported by |
|---|---|
| kubelet, under node pressure | `NodeHasMemoryPressure` / `NodeHasDiskPressure` — tier 3, `for: 0`, **one node-level signal** instead of one per pod |
| API-initiated (a drain) | nothing, deliberately — a drain is an operator doing what they were told, and it is unattended wherever a reboot manager such as Kured runs |
| a pod that does not come back | `FailedScheduling` — tier 5, `for: 5m`, confirmed by a dwell |

What the drop costs is the case where pods evict, reschedule cleanly, and the
node reports no pressure — a cluster working as designed. What it buys is that
a node drain no longer produces one conversation per pod on that node.

This is an application of the existing rule that a reason may be dropped only
where its underlying problem is still caught by a reason that is not dropped —
not an exception to the rule that past-tense reasons never dwell. `Evicted` is
still never given a non-zero dwell; it is simply not emitted on its own.

Because the drop leans on those two substitutes, the render test now pins them
*together* with it: re-tuning node pressure or `FailedScheduling` cannot
silently leave eviction unreported from every direction at once.

**To restore per-pod eviction signals**, restate the whole `rules` list —
Helm replaces list values rather than merging them, so overriding a single
tier silently drops the other five:

```yaml
k8s-bundle:
  eventsAdapter:
    source:
      rules:
        # tier 1 without Evicted
        - matchers:
            - reason=~"ProbeWarning|SandboxChanged|Preempting|NodeNotSchedulable|ExternalProvisioning|FailedToUpdateEndpoint.*|FailedPreStopHook|FailedKillPod|ContainerGCFailed|ImageGCFailed"
          action: drop
        # tier 2 with Evicted back
        - matchers:
            - reason=~"OOMKilling|SystemOOM|Evicted|BackoffLimitExceeded|DeadlineExceeded|VolumeFailedDelete"
          for: "0"
        # ...tiers 3-6 unchanged from chart/charts/k8s-bundle/values.yaml
```

### Generated credentials hold still — chart 5.8.0

**ONE COMMAND REQUIRED before upgrading an existing install.** Annotate the two
Secrets the chart generated, or the upgrade deletes them:

```sh
kubectl -n <ns> annotate secret agentops-adapter-token agentops-console-console \
  helm.sh/resource-policy=keep
```

The chart refuses to render without it and prints this command, so a forgotten
step is a failed upgrade rather than a lost credential. Skip it only for a fresh
install, or where you supply both credentials yourself.

The chart generates two credentials — the console UI token and the adapter master
token — and both were meant to survive upgrades via a cluster `lookup`. On a real
`helm upgrade` they did. But `lookup` returns nothing wherever the renderer has
no cluster, so every `helmfile diff` reported both as changed on an install where
nothing had:

```
agent-ops, agentops-adapter-token, Secret (v1) has changed:
-   token: '-------- # (32 bytes)'
+   token: '++++++++ # (32 bytes)'
```

Cosmetic on the diff; not cosmetic anywhere the render is applied. `helm template
| kubectl apply`, CI, a GitOps controller and a client-side dry run all produce a
*fresh* token — signing every console session out and invalidating every adapter
at once, since per-adapter tokens are HMACs of the master.

**A generated credential now leaves the upgrade path entirely.** With no explicit
value the Secret renders on install only, carrying `helm.sh/resource-policy:
keep`. Nothing random exists on the upgrade path to be applied, whichever renderer
runs — rather than a hazard neutralised by a lookup that happens to succeed.

That annotation is why the manual step above exists: Helm reads it off the **live
object**, not off the manifest dropping it, so a Secret created by an earlier
chart carries none and gets deleted by the first upgrade that stops rendering it.
Once annotated, the object stays put with the same value. It is a removal from
the release manifest, not from the cluster.

**An explicitly configured value now wins.** Precedence is explicit → existing
Secret → generate. `console.auth.uiToken` was checked *last*, so on any install
that already had a token it was accepted, documented and silently ignored — the
worst way for a setting to fail. Rotating is now a values edit rather than
"delete the Secret, then upgrade".

**`adapterAuth.token` is new**, matching `console.auth.uiToken` and the
`runtime.credentialsSecret.token` pattern: supply it and the credential is
release-managed from your secret store; leave it empty and it is generated.
Changing it 401s every adapter until its pod restarts with the new env — inherent
to rotating a master credential, and stated at the setting.

An explicit value is rendered on install **and every upgrade**, because that is
how changing it takes effect. It is also the way back if a generated Secret is
deleted by hand: once the chart has stopped managing the object, no upgrade
restores it.

`NOTES.txt` no longer prints "fetch your token" after every deploy — it names the
source in effect, since that instruction is where the belief that the token
rotates on every deploy came from.

### An answer describing its own tools reaches Telegram again — channel-telegram 0.7.1

Bug fix; upgrade the image. No values change beyond the tag.

Inline code was converted to `<code>` **in place**, and the emphasis regexes
then ran over the tags it had just written. One `*` inside `` `.claude/agents/*.md` ``
and another inside `` `mcp__kubernetes__*` `` paired with **each other**, across
the prose between them — opening `<i>` inside one `<code>` and closing it inside
the next. Telegram rejects a message with overlapping entities outright
(`can't parse entities: Unmatched end tag ... expected "</i>", found "</code>"`),
so the send op failed and the answer never arrived.

The failure had a nasty shape: it needed TWO inline-code spans each containing a
star, which is unremarkable prose for an agent describing its own tooling — glob
patterns and tool allowlists are exactly where stars live. The console showed the
answer (no HTML there) while the Telegram thread stayed silent, so it read as a
Telegram-side problem.

Inline code is now lifted out before emphasis, the same treatment fenced blocks
already had, and restored afterwards. Emphasis *spanning* code still nests
correctly. Both are pinned by tests, one of which checks tag NESTING rather than
substrings — a "contains `<i>`" assertion cannot see the defect that broke this.

### Authentication can move in front of the console — chart 5.7.0, console 0.8.1

Additive; no action required. Every existing install keeps requiring its token.

An install that fronts the console with oauth2-proxy, Cloudflare Access or an
Envoy ext_authz filter used to authenticate twice: once at the proxy, then again
with a shared token that identifies nobody. The second one adds no security — the
request already got past the proxy — and costs a credential to distribute and
rotate. The console already believed the proxy about **who** you are; it just
still demanded a token to decide **whether** you get in.

Two new values move the boundary outward:

```yaml
console:
  auth:
    enabled: false                        # the console authenticates nobody
    externalAuthenticator: oauth2-proxy   # REQUIRED, or the render fails
```

- **`auth.enabled: false` alone FAILS the render.** Naming what authenticates
  instead is mandatory, so "what protects this console?" is answerable from
  `helm get values` rather than from an operator's memory. The chart cannot
  verify the claim; it can insist you make it — and it costs one more deliberate
  act than flipping a boolean, which is the point.
- **An empty `uiToken` still authorizes nobody.** The two states — "no credential
  configured" and "no credential required" — stay independent, because the whole
  hazard is one being read as the other. Half a declaration (a `false` with
  nothing named) leaves the console closed, in the chart AND in the pod.
- **Writes then require a forwarded identity.** With token auth off there is no
  `token` fallback: a write log naming a credential nobody presented is worse
  than none, since it looks like an audit trail. A proxy that authenticates but
  forwards no identity therefore yields a **read-only console** — reads served,
  writes refused, the UI showing `unknown` and saying why. The fix is to forward
  `X-Forwarded-Email` or `X-Auth-Request-User`, which a proxy in that position
  should do anyway.
- **The identity headers are BELIEVED.** The fronting proxy must strip
  client-supplied copies of them, or a caller picks their own identity. The
  console cannot tell the two apart — they arrive on the same connection — so
  this is a deployment requirement of the mode, stated in the notes rather than
  reimplemented weakly in the pod.
- **The token Secret is still rendered.** The console Channel projects it with
  `envFrom`, so removing it would turn "disable auth" into "the console will not
  start". Re-enabling authentication stays one value.

The SPA follows: no login form on a console that accepts no token, no sign-out
button where there is no session to end, and the composer says why a reply is
unavailable instead of failing on submit.

The pod logs the mode twice, on purpose: `authDefault=` at startup is the process
env, and `console auth: external:<name>` is the EFFECTIVE mode, logged when the
served Channel's config resolves it. The startup line alone would report `token`
on an externally-authenticated console — the config arrives after boot — which is
the one state this setting must not be able to hide in.

`docs/console.md#trust-boundary` carries the recipe.

### A maintenance window for cluster events — chart 5.6.0, signal-k8s-events 0.3.0

Additive; no action required. Nothing changes unless you configure a window.

Some outages are on a schedule. A router that restarts at 04:00 takes the
cluster's connectivity with it for a quarter of an hour, every night, and the
events it produces are *real* — so none of the three existing suppression axes
can silence them. `for:` verifies a condition the outage genuinely satisfies;
inhibition needs a cause event that a power cut never produces; matchers select
on labels, and there is no label for the time of day.

`route` gains the fourth axis, in Alertmanager's exact vocabulary:

```yaml
k8s-bundle:
  eventsAdapter:
    source:
      route:
        timeIntervals:
          - name: nightly-restart
            times:
              - startTime: "04:00"   # inclusive
                endTime: "04:20"     # exclusive
            location: Europe/Kyiv
        muteTimeIntervals:
          - name: nightly-restart
            matchers:
              - reason=~"NodeNotReady|Unhealthy|FailedMount|FailedScheduling"
```

`timeIntervals` also takes `weekdays`, `daysOfMonth`, `months` and `years` in
Alertmanager's forms; a window spanning midnight is two entries; overlapping
intervals union.

Three things worth knowing before you write one:

- **Name your `location`.** It defaults to UTC, as in Alertmanager, but "four in
  the morning" is a local fact — a UTC-pinned window drifts by an hour at each
  daylight-saving change and stops covering the outage it was written for, on a
  date nobody chose. The IANA database is compiled into the adapter image, so
  any zone name works on distroless.
- **Narrow with `matchers`.** With none, the source goes deaf for the whole
  window. A restarting router produces connectivity reasons; it does not produce
  `OOMKilling`, and an OOMKill at 04:05 is as real as one at noon.
- **Muting is evaluated at emit**, after the dwell and before the emit cap. A
  problem that outlives the window is still reported once it closes, and a muted
  burst never spends the emit budget.

Muting is not silent: while a window is active the source's `Ready` condition
stays true and names the interval (reason `Muted`), and reports the muted count
when it ends (`MuteEnded`). A malformed interval fails the source rather than
being ignored — a typo producing a window that never fires looks exactly like
one that works.

Requires `k8s-bundle.eventsAdapter.image.tag` 0.3.0 (the chart default). Full
reference: [docs/k8s-bundle.md](docs/k8s-bundle.md#maintenance-windows-the-time-axis).

### A conversation either continues or it fails — chart 5.5.0, manager 0.28.0, runtime-claude 0.6.0

**BREAKING (API): `Conversation.status.sessionId` is renamed to
`runtimeContextId`.** "session" is claude-code's noun; agent-ops has
Conversations, and what a runtime returns is its own handle for one. Both fields
are READ for one release — preferring the new, adopting the old — and only the
new is written, so no in-flight conversation loses its handle on upgrade. The
work unit carries both names for the same period, so a runtime image upgrades
independently of the manager. Anything reading `sessionId` (including the
console) should move.

**The bug this fixes.** The handle was recorded write-once:

```go
if d.SessionID != "" && conv.Status.SessionID == "" {   // before
```

When a continuation failed, the runtime correctly started a new context — and
that handle was never recorded, because the field was already set. The
conversation then named a context that no longer existed, so **every subsequent
message** repeated the same failed continuation. One transient loss became
permanent. The handle is now latest-wins, and is recorded on FAILED runs too, so
a crash after a context was established does not strand it.

**A context that cannot be continued now FAILS the run.** The runtime no longer
retries without its context and answers anyway: a conversation without its
context is a new one wearing the same name and thread, and an agent asked to undo
something it has no memory of will guess. The failure is articulate — a stated
reason, a message on the thread naming the remedy — which is what the old
fallback existed to avoid. A failed run's result now reaches bound threads
instead of a bare "run failed".

**Unavailability is an outage before it is a loss.** Bounded retry in the runtime
distinguishes a store that says GONE from one that did not ANSWER; a manager-side
circuit breaker then holds work rather than failing it when many conversations
report unavailability at once. Without it, one two-minute storage incident would
permanently destroy every active conversation's context.

**Continuity is promised only where possible.** New
`AgentRuntime.spec.contextStorage` (`volume` | `external` | `none`, default
`volume`). A runtime keeping context on a home volume the deployment does not
provide can never continue anything, so no handle is issued and the conversation
is single-run **by declaration** — answering each message fresh instead of
failing every follow-up for a configuration you chose. `NOTES.txt` says so, and
names the single-node topology (RWO or a `local` PV + `runtime.nodeSelector`) for
clusters without distributed storage.

New `ContextContinuity` condition carries the runtime's own reason, verbatim —
the manager does not know where a given runtime keeps context and does not guess.

**Upgrade order: manager first, runtime after.** The manager is compatible with
the current runtime image (which simply makes no continuity claim); the new
runtime works against either. Do not remove the dual read until no conversation
can still carry only `sessionId`.


### The runtime image drops kubectl — chart 5.3.0, runtime-claude 0.5.0

**BREAKING for anything that shelled out to `kubectl` inside a runtime pod.**

`runtime-claude` shipped a pinned `kubectl` v1.34.3, the one domain-specific
dependency in an image whose other contents are runtime responsibilities (git,
openssh-client for the checkout) or generic shell utilities. That contradicted
the rule the rest of the project already follows: an `AgentRuntime` differs by
vendor backend and trust level, and what an agent may reach is wiring —
`MCPConfig` + `MCPToolset` bound by a Pipeline. A CLI in the vendor layer was
the same category error as bundling an MCP server would be, and it carried a
version pin that could skew against whatever cluster it ran near.

**What breaks.** An agent whose route relied on `Bash` + `kubectl` for cluster
access has none after upgrading the image. `Bash` still works; it just no longer
reaches the Kubernetes API.

**The hold position is one line.** `AgentRuntime.spec.image` is why that field
exists — pin the previous tag and nothing changes:

```yaml
runtime:
  image: kmatsebora/agentops-runtime-claude:0.4.0
```

**To migrate**, give the route MCP tooling. With the k8s bundle that is already
the default: `mcp` and `mcpServers` are on, and a Pipeline binds
`k8s-observability` (reads) and optionally `k8s-admin` (mutations). Verified
against the shipped server: crashlooping pods, node pressure and failed
workloads are all answerable through `pods_list`, `pods_log`, `events_list`,
`resources_list` and `nodes_top`, which return kubectl-shaped tables.

**What MCP does not give you**: patch semantics, rollout/drain/wait,
port-forward, `auth can-i`, and any text processing over results — there is no
pipe. If you need those, build a derived image (three lines) and point an
`AgentRuntime` at it; the version pin becomes yours to own against your cluster,
which is the right place for it. Recipe in
[docs/concepts.md](docs/concepts.md#runtime-images-are-generic).

**New warning.** Enabling the k8s bundle with `mcp.enabled=false` now leaves an
agent that cannot see the cluster at all, so the post-install notes say so. The
render still succeeds — pointing `mcp.url` at your own server is legitimate.

**Also**: `k8s-observability` dropped two tool patterns
(`configuration_contexts_list`, `targets_list`) that the shipped server does not
register. Inert entries, but they implied capability the install never had.

### Console Ingress gains TLS and hostnames — chart 5.2.0

Chart only: no operator, image, CRD or contract change. Existing values files
keep rendering the same Ingress.

**New `console.ingress` keys.** `extraHosts[]` (additional hostnames serving the
same console, each getting a rule), `labels`, `path` and `pathType`.

**TLS is configurable instead of hand-written.** `console.ingress.tls` is now a
map:

```yaml
console:
  ingress:
    enabled: true
    host: console.example.com
    tls:
      secretName: console-tls      # existing certificate, or...
      clusterIssuer: letsencrypt   # ...cert-manager, which derives secretName
      existing: []                 # raw tls[] entries, verbatim, wins over both
```

`tls[].hosts` is DERIVED from `host` + `extraHosts`, so a rule host and a
certificate host cannot drift apart.

**The old list form still works.** `console.ingress.tls` supplied as a list of
raw Ingress `tls` entries — the only form before this release — is detected and
rendered verbatim. `helm upgrade --reuse-values` carries a previous release's
`console:` map forward wholesale, so an upgrade needs no values edit.

**BREAKING (render-time), only if you set a sub-path.** `console.ingress.path`
must be `/`. The console's SPA is embedded at build time with an absolute asset
base and emits `/assets/...` URLs, so serving it under `/console` routes
correctly and then renders a blank page. The chart now refuses that
configuration instead of producing it. Give the console its own hostname or
subdomain. Nothing to do unless you had already set a path that did not work.

**New warning, no new failure.** Enabling the Ingress with no TLS configured
still renders — TLS often terminates upstream at a load balancer or in a mesh,
and the chart cannot see what sits in front of it — but the post-install notes
now state that the UI token crosses the network in clear text, and name both
remedies. If your TLS terminates upstream, ignore it.

### Restarting anything stops losing state — chart 5.1.0, manager 0.26.0

Storage is on by default and the two silent losses a restart used to cause are
closed. No adapter changes; the CRD gains three optional fields.

**BREAKING for value-reset installs: `persistence.enabled` now defaults to
`true`.** A fresh install requests a ReadWriteMany claim for `/data/home`, so
agent session files survive a runtime pod restart instead of dying with it. A
`helm upgrade` on an existing release is unaffected unless values are reset.

On a cluster with no default StorageClass or no RWX provisioner, the claim sits
`Pending`, runtime pods never schedule and conversations queue forever — with no
error anywhere else, because the kubelet is what waits. The opt-out is one flag:

```sh
helm upgrade ... --set persistence.enabled=false
```

Sessions then live in `emptyDir` and die with each runtime pod; an agent
resuming a lost session answers without prior context. `NOTES.txt` prints the
diagnosis, and `chart/ci/default-values.yaml` sets it false for test installs.

**New `persistence.workspace` block, off by default** — a second claim backing
the repository checkout, mounted per-conversation via `subPath` so concurrent
runtime pods never share a working tree. Turning it on preserves uncommitted
agent work across a pod restart and skips the re-clone. Off by default because a
fresh checkout is cheap and always correct, whereas a stale shared one is
neither. Nothing reclaims a conversation's directory after deletion — size the
claim accordingly. It renders `AgentRuntime.spec.workspace`, wired from the
chart's own values the way `home.pvcRef` already is.

**The agent's answer survives a manager restart.** Previously, a restart between
`POST /work/done` and an adapter claiming the outbound op dropped the reply
permanently: the result was durably in `status.runs[].result` and delivered to
nobody. Replies now carry a stable op id
(`send:<conversation>:<channel>:<runId>`), delivery is recorded per bound thread
in `status.runs[].delivered[]`, and reconciliation re-enqueues anything still
owed. A partially delivered fan-out completes the remaining threads without
repeating the delivered one.

**Upgrading re-posts nothing.** Runs recorded before this release carry no
`deliveryTracked` marker; on first observation they are recorded as delivered
*without* sending, so no bound thread receives an old answer again.

**Ingest cooldown survives a restart.** Fingerprint suppression is recorded on
`SignalSource.status.cooldown[]` (pruned past its window, bounded at 200
entries) and read back on first use per source, so a restart mid-incident no
longer re-opens conversations for alerts still being suppressed. Only an
admitted fingerprint writes; suppressed re-deliveries — the high-volume case —
write nothing.

**Telemetry gaps are reported, not rendered as silence.** The activity ring
stays bounded, in-memory and lossy. A cursor it cannot serve — evicted, or
issued by a previous manager process — is now answered with a resync, and the
console renders that as an explicit gap in its timeline instead of an empty
window that reads as "nothing happened". Conversations, topology and
configuration are unaffected; they are read from Kubernetes. The gap is carried
on the console's stream health, so a browser opened *after* the incident sees it
too.

**Console fix: the `live` chip recovers on its own.** `EventSource` gives up
permanently on an HTTP error (a 502 during a rollout, a 401 on session expiry)
rather than retrying, and nothing reopened it — so the masthead stuck on
`stream disconnected` and the graph stayed frozen until the page was reloaded.
The client now reconnects with backoff, leaving genuine transport blips to the
browser. Console **0.7.1**.

New CRD fields (all optional, old objects stay valid):
`AgentRuntime.spec.workspace`, `Conversation.status.runs[].delivered[]` +
`.deliveryTracked`, `SignalSource.status.cooldown[]`.

Rollback: revert the chart and the image. The new fields are ignored by an older
manager, and both claims carry `helm.sh/resource-policy: keep`, so no data is
destroyed by a downgrade.

### BREAKING — adapters render, `POST /task` is removed, and the console is on by default

Chart **5.0.0**, manager **0.25.1**, channel-telegram **0.7.0**,
console **0.6.0**.

**Upgrade all three together.** The manager and every channel adapter share the
outbound message contract, and version 2 is not compatible with version 1 in
either direction. The signal adapters (`signal-telegram`, `signal-k8s-events`,
`signal-cron`, `signal-vmalertmanager`) and `telegram-router` are unaffected —
they never consume `/channel/ops`.

### BREAKING — outbound ops carry a typed message; adapters render it

**Every channel adapter must be updated in lockstep with the manager.** An op no
longer has `text` or `title`.

- `send` carries `op.message`: `signal` (the event, with `source`, `labels{}`,
  the payload inline and `inputRef`), `answer` (`body`, `status`), `relay`
  (`origin`, `sender`, `body`) or `notice` (`level`, `body`). Prose is markdown
  in a named subset — `**bold**`, `*italic*`, `` `code` ``, ```` ``` ````
  fenced, `[text](url)` — and anything outside it is undefined.
- `ensure-topic` carries `op.topic`, a descriptor the adapter NAMES the thread
  from. Telegram's 128-character topic limit is now enforced where it is known.
- `GET /channel/ops` requires `contract=2`. An adapter that declares nothing, or
  an older version, gets a 400 naming the replacement — because one still
  reading `op.text` would post empty messages and look healthy doing it.

**The agent's own output moved with it.** `dispatch/templates/format.md` — the
mandatory message-format spec shipped to every profile — told the agent to write
a "chat HTML subset" (`<b>`, `<code>`, `<pre>`), which used to pass through to
Telegram untouched. Adapters escape what they are handed, so that HTML now
reaches chat as literal characters. The six templates are markdown. Custom
prompts that instruct HTML must be updated the same way; a resumed session
picks the new spec up on its next work unit.

The manager stops guaranteeing anything about how a message looks. Escaping,
the 4096-character split, and topic naming moved into `channel-telegram`, which
is the reference renderer; in-process providers get the same treatment, since
exempting them would put presentation back inside the manager through a side
door. **Two bugs close with it**: a message over 4096 characters used to fail
the op outright, and a payload containing `<`, `>` or `&` broke HTML parsing —
so the alerts most worth reading were the ones that did not arrive.

Third-party adapters: implement the four kinds, name the topic from the
descriptor, and send `contract=2`. `channel-telegram/render.go` is ~180 lines
and is the whole job.

### A conversation thread now opens with the event that caused it

An alert thread used to be a topic title, then silence, then the agent's
interpretation — and if the agent hung, the thread never said what had happened.
Every input a human has not already seen is now posted to the bound threads as a
`signal` card, in parallel with dispatch.

`InputItem` gains `origin` (`{kind: signal|channel, name, signalKind}`), which
**replaces `jobName`** — that field recorded the source for `kind: job` only, so
a conversation could not say what woke it. `ConversationInput` gains `labels`,
kept beside the payload so an adapter can render them. Both are additive and
optional; **an input with no origin posts nothing**, so upgrading does not fill
open threads with history.

No `pipelineRef` is introduced. A card names its pipeline from the same
inference `/status` and the console already use, and omits it when the wiring is
ambiguous — blank rather than guessed.

Anything reading `InputItem.jobName` breaks: read `origin.name`.

### BREAKING — a task is a signal; `POST /task` is gone

The endpoint, its handler and its request type are deleted. There is now no HTTP
route that names a `Pipeline`. Programmatic origination is an ordinary signal
posted to a `SignalSource` that a Ready Pipeline claims, so which agent answers,
on which channels, with which capabilities is decided by declared wiring rather
than chosen by the caller — the same rule every other origination already
followed.

**Before:**

```sh
curl -sX POST http://agentops-manager.<ns>.svc:8080/task \
  -H 'Content-Type: application/json' \
  -d '{"pipeline":"k8s-engineer","task":"why is pod X crashlooping?"}'
```

**After:**

```sh
TOKEN=$(kubectl -n <ns> get secret agentops-adapter-token \
  -o jsonpath='{.data.token}' | base64 -d)
curl -sX POST http://agentops-manager.<ns>.svc:8080/signal/inbound \
  -H "Authorization: Bearer $TOKEN" \
  -d '{"source":"cluster-events","signals":[{"fingerprint":"ask-1",
       "kind":"task","payload":"why is pod X crashlooping?"}]}'
```

Three things change in the call: a different URL, a bearer token
(`ADAPTER_TOKEN`, the same one adapters use), and a **source** name where a
pipeline name used to go. Pick a source the Pipeline you want already claims —
`kubectl get pipeline <name> -o jsonpath='{.spec.signalSourceRefs}'`. The
`fingerprint` is yours to choose and is what cooldown dedups on; a build id or a
timestamp is the usual answer.

**No deprecation shim is offered.** A 410 would preserve exactly the doorway
this change closes, and the API group is provisional pre-1.0.

**Dropped without replacement:** the `agent` override (per-call role selection
remains available through the chat form `/<pipeline>:<agent>`) and the `channel`
field, which let a caller add a surface — that is wiring, and wiring is declared
on the Pipeline.

**New signal kind `task`.** Alongside `alert`, `job` and `chat`: task-lane
prompt, no `jobName`, no recurrence-on-session, and — unlike `chat` — no
`agentops.dev/channel` label required, because replies go to the claiming
Pipeline's channels.

**Signature keying now splits on the lane** when a source declares no
`signatureLabels`. `alert` and `job` keep the default
`alertgroup`/`alertname`/`namespace` labels, so alert grouping and cron-tick
folding are unchanged; `task` and `chat` key on the signal's own fingerprint, so
each request opens its own conversation. A source that DOES declare
`signatureLabels` groups by them in every lane — which means **a posted task
inherits the target source's grouping**. Posting tasks to a source with
`signatureLabels` (k8s-bundle's `cluster-events` uses `namespace`/`workload`)
groups tasks that share those label values, including tasks carrying none of
them. Create your own source if you want an isolated ask lane.

No rendered chart object changes, no CRD schema field is added or removed, and
no stored object changes shape.

### What changes on upgrade

`console.enabled` flips from `false` to `true`. Upgrading STARTS A POD that was
not running before — one that reads every `agentops.dev` CR in the namespace,
plus Deployments and Pods, and that can instruct any agent it is joined to. That
is why this is a major bump rather than a feature.

**The one-value opt-out:**

```yaml
console:
  enabled: false
```

With that set, nothing about your install changes.

**One caveat, if you later turn it off again.** Once a Pipeline references the
console — `channelRefs: [console]` or `signalSourceRefs: [console]` — disabling
the console removes those objects and the Pipeline correctly reports
`unresolved references: signalsource/console, channel/console` and stops being
Ready. Remove the references in the same change as the opt-out. `helm upgrade
--wait` will otherwise fail on that Pipeline, which is the system telling you
the truth rather than a bug.

### If you keep the console on

The new pod is a `ChannelAdapter` **and** a `SignalAdapter`, and it is still ONE
Deployment: the SignalAdapter declares `servedBy` the ChannelAdapter, so it owns
no workload and simply receives a second token in the same pod.

- **What it reads.** Its Role gains `apps/deployments` and `pods`
  (get/list/watch, namespaced, read-only) on top of the `agentops.dev` kinds.
  Image digests, restart counts and pod failure reasons exist in no CR, and an
  operations console that cannot see a CrashLoopBackOff is not one. There are
  still no write verbs anywhere in it.
- **What it can do.** `console.write.enabled` defaults to `true`, so the chat
  composer and "new conversation" are live. Set it to `false` for a strict
  viewer — the affordances disappear AND both endpoints refuse.
- **Origination is refused until you wire it.** The chart renders a
  `SignalSource` named `console` and NO Pipeline. Until some Ready Pipeline
  claims that source it sits at `Wired=False`, and the UI shows that reason with
  the patch:

  ```sh
  kubectl patch pipeline <name> --type=json \
    -p '[{"op":"add","path":"/spec/signalSourceRefs/-","value":{"name":"console"}}]'
  ```

- **Exposure.** Still `ClusterIP` with `console.ingress.enabled: false`. If you
  expose it, put an authenticating proxy in front — the console reads the
  identity from forwarded headers and logs every write with it; without one every
  write is recorded as `token`.

### New in the manager (additive, no action required)

- **Per-hop activity telemetry.** `GET /activity`, `GET /activity/stream` (SSE)
  and `POST /activity`, under the existing adapter bearer scheme. A bounded
  in-memory ring (`ACTIVITY_BUFFER`, default 10000), never persisted; the durable
  record stays `status.runs[]`.
- **Introspection.** `GET /status` (runtime slots, op queue depth with the oldest
  stuck item's identity, cooldowns, leader) and
  `GET /pipelines/{name}/resolved` (the authoritative capability resolution).
- **Prometheus metrics** on the existing `:9090` — `agentops_*` counters, gauges
  and histograms, emitted from the same call sites as the activity events. No new
  listener. Optional scrape templates under `metrics:` (VMServiceScrape,
  ServiceMonitor, example alert rules), all default-disabled because neither CRD
  is guaranteed present.
- **`SignalAdapter.spec.servedBy`.** `spec.image` becomes optional; an adapter
  declaring `servedBy` owns no workload and reports `Ready=True/ServedBy`. Purely
  additive — existing adapters are unaffected.

### Optional

`console.metrics.url` points the console at a Prometheus/VictoriaMetrics query
endpoint, which lets it render windows far beyond the activity buffer as clearly
labelled aggregates. Unset, every view still works and long windows are reported
unavailable rather than drawn empty.

## 4.0.0

**BREAKING — the main chart owns the agent runtime.** Chart **4.0.0**. No image
changes; this is a values and object-ownership move.

*Why this exists:* `AgentRuntime` was created in exactly one place — the
`k8s-bundle` subchart — and nothing about it is Kubernetes-shaped. Image,
credential, idle TTL, node placement and home volume describe *how an agent
executes*, which is the same for a VictoriaMetrics lane, a chat-only install or
a Kubernetes one. The placement was never a decision: chart 2.x relocated the
parent's `demo.*` block wholesale and the runtime rode along, because demo mode
and the bundle became the same thing. The interest paid since: an install with
only `telegram-bundle` rendered nothing that could execute a conversation, two
runtime ServiceAccounts existed (the parent's, granted nothing, and the
bundle's, granted everything), `homePvcRef` was a documented copy of a claim the
parent creates, and idle TTL had two defaults that disagreed (1 vs 10).

**Two upgrade-visible effects that are not value renames — read these first**

1. **The runtime ServiceAccount is renamed.** `agentops-runtime-k8s` →
   `agentops-runtime` (the one the parent already created and the manager
   already defaults runtime pods onto). Helm replaces the bundle-named bindings
   with global-named ones in the same upgrade. **If you bound your own
   (Cluster)Roles to `agentops-runtime-k8s`, re-point them** — nothing else
   will. Afterwards, confirm the old binding is gone:

   ```sh
   kubectl get clusterrolebinding | grep agentops-runtime-k8s   # expect nothing
   ```

2. **An install that enabled `k8s-bundle` without touching MCP now gets an MCP
   server workload.** `mcp.enabled` and `mcpServers.enabled` both default to
   `true` and flip as a pair — the config's URL needs a Service to default onto,
   which is the only reason the component used to be off. Hold position:
   `k8s-bundle.mcpServers.enabled: false` (and `mcp.enabled: false`, or an
   `mcp.url` of your own).

**Values migration**

| 3.x | 4.0 |
|---|---|
| `serviceAccounts.runtime` | `global.agentops.runtime.serviceAccountName` (setting the old key now FAILS the render) |
| `k8s-bundle.profile.runtime.image` | `runtime.image` |
| `k8s-bundle.profile.runtime.credentialsSecret.*` | `runtime.credentialsSecret.*` |
| `k8s-bundle.profile.runtime.nodeSelector` | `runtime.nodeSelector` |
| `k8s-bundle.profile.runtime.resources` | `runtime.resources` |
| `k8s-bundle.profile.runtime.idleTtlMinutes` | `runtime.idleTtlMinutes` (empty ⇒ follows `runtimeIdleTtlMinutes`) |
| `k8s-bundle.profile.runtime.homePvcRef` | *(automatic from `persistence`)* |
| `k8s-bundle.profile.runtime.name` | `runtime.name` |
| `k8s-bundle.profile.runtime.create` | `runtime.enabled` |
| `k8s-bundle.profile.runtime.serviceAccountName` | `global.agentops.runtime.serviceAccountName` |
| `k8s-bundle.rbac.mode` | `global.agentops.runtime.rbacMode` |
| `k8s-bundle.rbac.enabled: false` | `global.agentops.runtime.rbacMode: none` |
| `k8s-bundle.mcpServers.readOnly` | *(derived from `rbacMode`; still settable)* |
| `k8s-bundle.mcpServers.rbac.mode` | *(derived from `rbacMode`; still settable)* |

Moved a value into `k8s-bundle.profile.runtime.*` during the 1.x → 2.x hop? That
table is still below, under chart 2.0.0 — this one names where it went next.

**Worth knowing**

- **A default install now renders an `AgentRuntime`.** `runtime.enabled` is
  `true`, so a chart with no bundle — or with only `telegram-bundle` — can
  execute a conversation. If you manage `AgentRuntime` CRs yourself, set
  `runtime.enabled: false`; a name collision on `default` makes Helm adopt or
  conflict loudly rather than silently change behavior.
- **`rbacMode` is one knob, and empty grants nothing.** `none` | `readonly` |
  `full`, defaulting to empty, which resolves to `readonly` under
  `global.demo.enabled` and to `none` otherwise — so a release that never set an
  RBAC value holds no bindings after upgrade, exactly as before. `full` is never
  selected by a default or inferred path.
  `rbac.runtime.{clusterRoles,bindClusterRoles,namespaced}` stay additive.
- **The MCP server's posture derives from that knob.** `full` yields a
  write-capable server under a `full` SA and renders `k8s-admin`; every other
  mode yields a read-only server under a `readonly` SA. It derives because a
  read-only server under a `full` agent pushes every write back onto kubectl —
  the single-wall path the component exists to replace. The cost is real:
  widening the agent widens the server unless you set
  `k8s-bundle.mcpServers.readOnly: true`, which recovers "kubectl writes, MCP
  reads only". See
  [docs/k8s-bundle.md](docs/k8s-bundle.md#kubernetes-as-mcp-tools-mcp--mcpservers).
- **Idle TTL stops being configured twice, and drops from 10 to 1.** The bundle
  defaulted `idleTtlMinutes: 10` while the manager's `runtimeIdleTtlMinutes` was
  `1`; now an empty `runtime.idleTtlMinutes` follows the release value, so a
  finished conversation gives its slot back in a minute rather than ten. Set
  `runtime.idleTtlMinutes: 10` to keep the old behavior for this runtime, or
  raise `runtimeIdleTtlMinutes` for the release. The chart WRITES the field
  rather than omitting it: `AgentRuntime.spec.idleTtlMinutes` has a CRD default
  of `10`, so an omitted field is stored as `10` and the manager prefers any
  non-zero spec value over its own setting — omitting it renders a correct-
  looking manifest and a wrong stored object.
- **The bundle keeps its identity, not its substrate.** `profile.enabled` now
  renders exactly one object, the `AgentProfile`; `profile.runtimeRef` still
  points at a runtime other than `default`. `eventsAdapter.rbac` is a different
  block and is untouched.
- **Rollback = chart 3.4.0.** The CRDs are unchanged, so a downgrade re-renders
  the bundle-owned objects; the only manual step is deleting the parent-owned
  `AgentRuntime` if its name differs from the bundle's.

**Conversations are capped, queued, and can be closed.** Chart 3.4.0, manager
0.21.0, `channel-telegram` 0.6.1, `console` 0.2.0 — ship them together: an
adapter that does not know the new `close-topic` op leaves topics open.

*Why this exists:* nothing bounded how many conversations ran at once in terms
an operator recognises. The only cap was `MAX_RUNTIMES`, named after pods, and
an idle runtime held its slot for ten minutes after the agent had stopped
working. Conversations also never ended — no way to say "this one is done", so
threads accumulated and capacity came back only by eviction.

**Upgrade steps**

1. **Two defaults change on upgrade, both visible.**
   - `maxRuntimes: 8` → **`maxActiveConversations: 5`**. Less throughput, more
     queueing. Nothing is dropped, only delayed — raise the new key to restore
     the old figure. `maxRuntimes` still works for ONE release (unset by
     default; setting it emits `MAX_RUNTIMES` and the manager logs the
     deprecation), then it is removed.
   - `runtimeIdleTtlMinutes: 10` → **`1`**. A finished conversation gives its
     slot back within a minute instead of ten, which is what makes a cap of 5
     workable. The trade is latency, not memory: sessions live in `/data/home`
     and resume with full context. Raise it — or set
     `AgentRuntime.spec.idleTTLMinutes` — for runtimes with expensive startup.
2. **Third-party channel adapters should handle `close-topic`.** It carries the
   `threadId` to archive and completes with an EMPTY body. An adapter that
   ignores it is not broken: the op fails its 2-minute grace and deletion
   proceeds, leaving one open thread per closed conversation. See
   [docs/contracts.md](docs/contracts.md#the-channel-adapter-contract).
3. **Nothing to migrate for existing objects.** `Pending` is an additive phase
   value; no old manager writes it, and rolling back is a chart rollback —
   phase is status-only and nothing keys behavior off it.

**Worth knowing**

- **Over-cap work waits in `Pending` with NOTHING provisioned** — no runtime
  pod, no MCP ConfigMap and no chat topic. That last one is the point: a burst
  of signals no longer becomes a burst of chat threads. Admission is FIFO by
  creation time. `Queued` keeps its old meaning (admitted, waiting its turn).
- **The backlog is bounded too**, at `maxQueuedConversations: 50`. Past it
  `/signal/inbound` declines to create a conversation and reports the batch
  dropped for capacity — chat senders are told on the surface they typed on.
  Window reuse is unaffected: the bound gates new objects, not new inputs.
- **`/close` ends a conversation** from its thread — any sender who can post
  there, honored mid-run (the farewell names the abandoned work). It deletes
  the `Conversation`; the pod and `agentops-mcp-conv-<name>` follow by owner
  reference. `kubectl delete conversation` behaves identically, archiving
  threads first. On a general surface `/close` answers with usage, so a
  Pipeline named `close` is no longer addressable from chat.

**Cluster events get suppression, workload grouping, and a loop breaker.**
`signal-k8s-events` 0.2.0. Two breaking-ish defaults and one new RBAC grant.

*Why this exists:* the events lane created **hundreds of conversations** on a
healthy cluster, and on an unhealthy one it fed itself — a runtime pod that
could not start emitted a Warning event, which became a signal, which opened a
Conversation, which created another runtime pod under a new name, forever.

**Upgrade steps**

1. **The adapter needs new permissions.** The events component now grants
   read-only `pods` and `replicasets` (`list`/`watch`) alongside `events`. If
   you render the bundle's RBAC (`eventsAdapter.rbac.create: true`, the
   default) this happens for you. If you bind that RBAC yourself, add it — the
   adapter reports `Ready=False` naming the missing permission rather than
   degrading silently.
2. **`grouping.signatureLabels` defaults to `[namespace, workload]`**
   (was `[namespace, kind, name]`). **BREAKING for existing conversations:**
   they keep their old per-pod signature hash and go orphaned. No action is
   needed — they age out of the 7-day reuse window on their own, and new
   conversations group per workload. Override the values path if you need the
   old behavior back.
3. **Default `rules` now ship.** A default install suppresses rollout churn out
   of the box. If you had built your own `excludeReasons` list it still works
   (it translates into leading drop rules), but review it against the new
   defaults — you probably need less of it.

**Worth knowing**

- Reason matching is now **anchored**: `excludeReasons: [Failed]` no longer
  also drops `FailedMount`. If you were relying on the accidental prefix match,
  widen it to a rule with an explicit regex.
- The self-exclusion invariant is **not configurable** for its first two
  mechanisms (name prefix, owner/label). `source.includeOwnNamespace: true`
  relaxes only the coarse namespace rule, for installs that co-locate their own
  workloads with the operator.

**Stopgap for anyone still on 0.1.x:** the conversation explosion is a values
edit away from stopping, with no new image —

```yaml
k8s-bundle:
  eventsAdapter:
    source:
      excludeReasons: [Unhealthy, FailedScheduling, SandboxChanged, Preempting]
      grouping:
        signatureLabels: [namespace, alertname]
```

This bounds conversations by namespaces × reasons instead of pods × rollouts.
Agent *runs* do not drop (cooldown is still per object+reason), and it does not
break the self-reference loop — only 0.2.0 does that.

**The agent-ops console — additive, opt-in, no upgrade steps.** A browser view
of the whole install (CR inventory, wiring graph, live conversation runs) that
is also a channel: conversations on pipelines listing its Channel bind a
console thread, so you can reply to an agent from the run you are watching.
See [docs/console.md](docs/console.md).

- New `console/` module and image `agentops-console`.
- `ChannelAdapter.spec` gains `kubernetesAccess` (parity with SignalAdapter):
  mounts the SA token and injects `POD_NAMESPACE`. **Identity only** — the
  operator still creates and binds no RBAC, so an adapter CR cannot escalate.
  Existing adapters are unaffected (optional, default off). `spec.port` already
  existed and is unchanged; its Service is named after the workload,
  `agentops-adapter-<name>`.
- Enable with `console.enabled=true`. It renders **CRs and RBAC only** — a
  ChannelAdapter, a Channel, the UI token Secret, and a namespaced read-only
  Role for SA `agentops-adapter-console`; the reconciler brings up the
  Deployment and Service.
- Nothing is wired by enabling it: conversations show as *observed* until you
  add the console Channel to a Pipeline's `channelRefs[]`. The chart never
  edits your Pipelines.
- **Trust boundary**: anyone holding the UI token sees every agentops CR in the
  namespace, conversation payloads included. Keep the Service ClusterIP unless
  you mean to expose it.
- Manager image `0.20.0` — required for `kubernetesAccess` on ChannelAdapter.
  An older manager silently ignores the field: the CR accepts it, the pod comes
  up without a token, and the console crash-loops on the missing CA file.

**The k8s-bundle can now create the agent's credential Secret.** Additive and
default-off. `k8s-bundle.profile.runtime.credentialsSecret.token`, when set,
renders the Secret the `AgentRuntime` already referenced
(`agentops-claude`/`oauthToken`) instead of leaving it as an unstated
prerequisite — so the credential survives a teardown with the release, the same
way `telegram-bundle` handles its bot token. Leaving it empty keeps the old
behavior exactly.

Worth knowing because the old failure mode is silent: the reference is resolved
by the kubelet, not the manager, so a missing Secret produces runtime pods in
`CreateContainerConfigError` and conversations that queue forever, with no
condition anywhere saying why. The post-install notes now name it.

## chart 3.2 — toolsets compose with the agent definition — BREAKING

**The runtime no longer invents a tool.** `runtime-claude` used to pass
`--allowedTools Read` whenever a work unit carried no allowlist — a grant
nobody declared. From image `0.2.0` it passes exactly what was composed, empty
included, and runs with `--permission-mode dontAsk` so an unlisted tool is
denied rather than prompted for (a prompt in a pod hangs until the idle TTL).

**Who is affected:** any conversation whose route binds no `toolsets` *and*
whose agent definition declares no `tools:`. It used to get `Read`; now it gets
nothing, starts, finds it can do nothing, and says so. That is the point — but
check before upgrading:

```sh
# Pipelines that grant no tools; their routes now depend entirely on the
# agent definition's own `tools:` frontmatter.
kubectl get pipelines -A -o json | jq -r \
  '.items[] | select(.spec.toolsets == null)
   | "\(.metadata.namespace)/\(.metadata.name) -> profile \(.spec.profileRef.name)"'
```

Fix either side: bind a toolset on the Pipeline, or declare `tools:` in the
repo's `.claude/agents/<agent>.md`. Both work, and under the default `merge`
they add up.

**`toolsets.mode` is back, with a different counterpart.** It composes against
the **agent definition's** `tools:` frontmatter, never against the profile
(profiles carry no capabilities — that misreading is why the field was removed
in 3.0). `merge` is the default and additive; `overwrite` passes the route's
tools alone. `mcpConfigs` has no mode. Existing Pipelines need no edit: the CRD
defaults `mode: merge`, which reproduces today's behavior wherever the agent
definition declares nothing.

The work unit gained `toolsMode` and `agent` alongside `allowedTools`. Custom
runtimes that ignore them keep working — `allowedTools` alone is what `merge`
degrades to when nothing is declared repo-side.

Steps:

1. Upgrade the operator (manager `0.18.0`) — additive, existing Pipelines
   default to `merge`.
2. Update the runtime image to `agentops-runtime-claude:0.2.0`
   (`k8s-bundle.profile.runtime.image`, or the `AgentRuntime` CRs you manage).
   Staying on `0.1.1` keeps the `Read` substitution and ignores the mode.
3. Run the `jq` check above and grant what each route actually needs.

## chart 3.1 — chat originates as a signal — BREAKING

Two breaking changes land together.

**1. `/channel/inbound` is reply-only.** `threadId` is now REQUIRED and an
unknown thread is no longer adopted. Third-party adapters that posted bare
messages to originate a conversation get a `400` naming the signal path.
`PipelineForChannel` — and with it the "oldest Ready pipeline referencing this
channel answers" tiebreak — is gone, as is any channel default profile.

**2. Telegram ingest is three components.** Telegram serves one update stream
per bot token, so origination and continuation cannot each poll:

```
getUpdates ─▶ telegram-router ─┬─ no topic ─▶ signal-telegram  ─▶ /signal/inbound
              (the only poller) └─ topic    ─▶ channel-telegram ─▶ /channel/inbound
```

`channel-telegram` no longer polls; `Channel.spec.config.pollingEnabled` is
removed. Ingest is on when the router runs.

Steps — **step 3 is the one where ordering matters**:

1. **Upgrade the operator first.** `ChannelAdapter.spec.port` and the
   reconciler's Service parity are inert until used.
2. **Add the chat `SignalSource` and claim it**, plus the router's
   credential-carrying source. Nothing changes behaviorally yet — the old
   adapter is still polling.

   ```yaml
   apiVersion: agentops.dev/v1alpha1
   kind: SignalSource
   metadata: {name: home-ops-chat, namespace: agent-ops}
   spec:
     adapter: telegram          # signal-telegram; no credentials needed
     config:
       chatId: "-1001234567890"
       channel: home-ops        # the Channel this chat is the general surface of
     grouping: {cooldownHours: 0}
   ```

   Then add `home-ops-chat` (and `telegram-router`) to your Pipeline's
   `signalSourceRefs`. **A chat source no Pipeline claims answers nobody** —
   the user is told so on the surface, but nothing runs.
3. **Scale the old adapter to zero and CONFIRM no `getUpdates` consumer
   remains** before enabling the new stack. Two consumers of one token means
   409s and stolen updates — this is the failure that costs the most debugging.

   ```sh
   kubectl scale deploy/agentops-adapter-telegram -n agent-ops --replicas=0
   kubectl get pods -n agent-ops | grep telegram   # expect none
   ```
4. **Carry the offset across.** The cursor lives in the
   `agentops.dev/adapter-state-telegram-offset` annotation on the Channel the
   old adapter wrote to. The new stack reads it from the FIRST served channel
   by name, so if your old leader differs, copy the value over:

   ```sh
   kubectl get channel home-ops -n agent-ops \
     -o jsonpath='{.metadata.annotations.agentops\.dev/adapter-state-telegram-offset}'
   ```
5. **Enable the bundle.** Telegram now ships as the embedded `telegram-bundle`
   subchart, so the flat `telegramAdapter` values are gone:

   ```yaml
   telegram-bundle:
     enabled: true
     surface:
       enabled: true
       chatId: "-1001234567890"
       credentials:
         existingSecret: agentops-telegram   # key: botToken
   ```

   `enabled` alone ships the three IMPLEMENTATIONS and wires nothing — the
   right shape if you manage Channels yourself. `surface.enabled` also ships
   the Channel and the chat `SignalSource`, and then requires everything it
   cannot guess. The router's credential is the SAME one the Channel uses — it
   polls the bot the channel sends as.

6. **Claim the sources in a Pipeline.** Your existing Pipeline keeps its
   channels; add the new sources to its `signalSourceRefs`:

   ```yaml
   signalSourceRefs:
     - name: telegram-ops-chat   # the chat surface
     - name: telegram-router     # emits nothing; claimed so it is not Wired=False
   ```

   Until this lands, chat messages drop with `Wired=False` and the user is told
   so. The post-install notes print this block with your names filled in.

Rollback is steps 3 and 5 together: re-enable the old single-container adapter
AND revert the operator, since the origination paths must come back with it.
Conversations already created are unaffected either way.

## chart 2.x → 3.0 — BREAKING

`AgentProfile.spec.allowedTools` and `spec.mcp` are removed. **Removing a CRD
field prunes that data on the next write**, so audit before upgrading, not
after:

```sh
# every profile that still declares capabilities, and what they are
kubectl get agentprofile -A -o custom-columns=\
'NS:.metadata.namespace,NAME:.metadata.name,TOOLS:.spec.allowedTools,MCP:.spec.mcp'
```

For each profile that lists anything, move it before you upgrade:

| Was on the profile | Goes on |
|---|---|
| `allowedTools: "Read,Bash"` | an `MCPToolset`, referenced from the routing Pipelines' `toolsets` |
| `mcp.configRefs: [x]` | the routing Pipelines' `mcpConfigs.refs` |
| `mcp.servers: {...}` | an `MCPConfig`, then referenced as above |
| `mcp.configMapRef` / `secretRef` | an `MCPConfig` with the same field (bound alone) |

If the profile should be reachable, give it a Pipeline — that is what routes to
it. (At chart 3.0 a task named that Pipeline via `POST /task`; the endpoint is
removed as of chart 5.0 and a task now names a source instead — see the entry at
the top.)
`ToolingBinding.mode` is gone; drop `mode:` from any existing stanza.


## chart 2.x — demo values move into k8s-bundle — BREAKING

The demo block moved wholesale into the bundle. A subchart can read no parent
scope except `global.`, which is why the toggle moved there.

| Chart 1.x | Chart 2.x |
|---|---|
| `demo.enabled` | `global.demo.enabled` |
| `demo.runtimeImage` | `k8s-bundle.profile.runtime.image` |
| `demo.credentialsSecret.*` | `k8s-bundle.profile.runtime.credentialsSecret.*` |
| `demo.readOnlyRbac: true` | `k8s-bundle.rbac.mode: readonly` (the default) |
| `demo.readOnlyRbac: false` | `k8s-bundle.rbac.enabled: false` |
| *(inherited `persistence`)* | `k8s-bundle.profile.runtime.homePvcRef` — **set this explicitly**, subcharts cannot see the parent's `persistence` block |
| *(inherited `runtimeIdleTtlMinutes`)* | `k8s-bundle.profile.runtime.idleTtlMinutes` |

The runtime ServiceAccount is renamed `agentops-runtime-demo` →
`agentops-runtime-k8s`; `helm upgrade` removes the old objects with the deleted
`demo.yaml` template. The `AgentRuntime` named `default` re-renders with
identical semantics, so existing conversations keep resolving their runtime.


## chart 1.12 — `spec.type` → `spec.adapter` — BREAKING

`Channel.spec.type` and `SignalSource.spec.type` are now `spec.adapter`: the
value was always a reference to the serving adapter CR, and the old name read
as an intrinsic attribute, making the sibling `config` look like part of one
flat schema. The contract follows — `?type=` becomes `?adapter=` on
`/channel/ops`, `/channel/channels` and `/signal/sources` (the retired
parameter returns 400 naming its replacement rather than an empty list), and
`CHANNEL_TYPE`/`SOURCE_TYPE` collapse into `ADAPTER_NAME`.

`adapter` is immutable, so live Channels and SignalSources are delete-and-
recreate. **Carry their annotations across**: adapter cursor state (the
Telegram `getUpdates` offset, cron last-fire) lives in
`agentops.dev/adapter-state-*` annotations on those objects, and a bare
recreate makes the adapter re-read old updates. Manager and adapter images
must be upgraded together, since both sides of the contract change at once.

## chart 1.8 — adapters can configure their senders

Additive. `SignalAdapter.spec.kubernetesAccess` (default false) mounts the
adapter's ServiceAccount token and injects `POD_NAMESPACE` — **the operator
still grants adapters no RBAC whatsoever**; permissions come from the chart
or you, bound against the deterministic SA `agentops-signal-<name>`. The
vm-bundle uses it for `alertmanager.registration`, which replaces the manual
VMAlertmanager repoint. Existing adapters are untouched.

## chart 1.7 — adapters are pure implementation — BREAKING

`ChannelAdapter`/`SignalAdapter` lose `spec.type` and `spec.env` — the CR
**name** is now the type key (`Channel`/`SignalSource.spec.type` names the
serving adapter), and adapter CRs carry no configuration at all.
`SignalAdapter` gains `spec.port`: when set, the reconciler owns the Service
`agentops-signal-<name>` and injects `LISTEN_ADDR` (the vm-bundle chart no
longer ships one). To upgrade: rename adapter CRs (or re-type
channels/sources) so names and `spec.type` values match — the shipped
`telegram`, `cron`, and `vm-alertmanager` names already do; `spec.type` on
sources/channels is immutable, so a source whose type key changes (e.g.
`vmAlertmanagerWebhook` → `vm-alertmanager`) is deleted and recreated, then
re-claimed by its Pipeline.

## chart 1.6 — built-in alertmanager removed — BREAKING

The manager's `POST /ingest/alertmanager/{source}` endpoint and the built-in
`alertmanagerWebhook` type are gone — the `signal-vmalertmanager` adapter
(vm-bundle) accepts the identical webhook format and is the only Alertmanager
path. BEFORE upgrading: repoint senders to the adapter Service
(`/webhook/{source}`, see the VM bundle section) and move sources to the
adapter's type key claimed by a Pipeline; sender retries plus
fingerprint cooldown make the switchover itself lossless. After upgrading,
retire the old `alertmanagerWebhook` sources.

## chart 1.4 — pipeline-only wiring — BREAKING

`SignalSource.spec.channelRef`/`profileRef` and `Channel.spec.defaultProfileRef`
are removed — wiring exists only on `Pipeline`. Upgrade sequence (order
matters so alert routing never gaps):

1. **Apply a Pipeline first**, claiming every live source with the intended
   profile and channels (the old manager ignores it; the new one requires it).
2. Upgrade to chart 1.4 / manager 0.7. Unclaimed sources now show
   `Wired=False` and drop signals with an explicit response reason; bare
   messages on channels outside any pipeline get usage guidance.
3. Re-apply your CR manifests without the removed fields.

## chart 1.3 — Pipelines, multi-channel conversations — BREAKING

`Conversation.spec.channelRef`/`status.threadId` became `spec.channelRefs[]` /
`status.threads[]` (`{channel, threadId}` per bound channel). Upgrading is
behavior-neutral (no Pipeline CRs = single-channel flows unchanged), but
existing chat-bound conversations lose their binding fields; to keep replying
in their existing topics with session continuity, patch each one:

```sh
kubectl -n <ns> patch conversation <name> --type=merge \
  -p '{"spec":{"channelRefs":[{"name":"<channel>"}]}}'
kubectl -n <ns> patch conversation <name> --subresource=status --type=merge \
  -p '{"status":{"threads":[{"channel":"<channel>","threadId":"<thread>"}]}}'
```

(Unmigrated topics still work — replying triggers re-adoption as a fresh
conversation without the old session.) Mirroring is opt-in per Pipeline CR;
deleting the Pipeline reverts new conversations to source-level routing.

## chart 1.1 — ChannelAdapter CR

Chart 1.1 replaces the chart's Telegram adapter Deployment with a
`ChannelAdapter` CR — the reconciler owns the workload. For a live install
running the chart-1.0 adapter:

> **On chart 3.1+, `telegramAdapter.*` no longer exists** — Telegram moved into
> the `telegram-bundle` subchart and the adapter stopped polling. Read the
> steps below for the shape of the change, but use `telegram-bundle.enabled`
> and follow *Migrating to chart 3.1* above.

1. **Upgrade with the adapter disabled** (`telegramAdapter.enabled=false`, the
   default): helm removes the old Deployment — the bot token's single
   getUpdates slot is free, and the new CRD + manager (with the ChannelAdapter
   reconciler) are in place.
2. **Move the bot token to the Channel**: add
   `spec.credentialsSecretRef: {name: <your bot-token secret>}` (Secret key
   `botToken`) to each telegram Channel. `TELEGRAM_BOT_TOKEN` env remains a
   fallback for hand-deployed adapters only.
3. **Enable the adapter** (`--set telegramAdapter.enabled=true`, or apply your
   own `ChannelAdapter` CR): the reconciler deploys the workload with the
   projected credentials as the sole getUpdates consumer.

Rollback: delete the `ChannelAdapter` CR (the reconciler's Deployment is
GC'd), then redeploy chart 1.0.x whose template restores the old wiring.

## chart 1.0 — extensible channels — BREAKING

Chart 1.0 restructures the `Channel` CRD and moves Telegram out of the manager
into the `channel-telegram` adapter. For a live install:

1. **Stop the old manager** (`kubectl -n <ns> scale deploy agentops-manager
   --replicas=0`) — this stops the in-process poller, freeing the bot token's
   single getUpdates slot.
2. **Migrate Channel CRs** from the typed sub-struct to metadata + config:

   ```yaml
   # before                              # after
   spec:                                 spec:
     telegram:                             type: telegram
       botTokenSecretRef: {…}              defaultProfileRef: {name: …}
       chatId: "-100…"                     config:
       approvers: [1, 2]                     chatId: "-100…"
       pollingEnabled: true                  approvers: [1, 2]
     defaultProfileRef: {name: …}            pollingEnabled: true
   ```

   The bot token secretRef moves out of the CR entirely (the manager reads no
   Secrets at all anymore) — since chart 1.1 it returns as
   `spec.credentialsSecretRef` on the Channel (see below).
3. **Upgrade**: `helm upgrade … --set telegramAdapter.enabled=true`. The new
   CRD applies, the manager restarts without Telegram code, the adapter starts
   as the sole getUpdates consumer (replicas 1, Recreate). *(On chart 3.1+ this
   flag is `telegram-bundle.enabled`, and the sole consumer is the router — see
   Migrating to chart 3.1.)*
4. `status.threadId` is now a **string** (existing numeric ids remain valid as
   decimal strings) — update anything that parsed it as a number.

Rollback = reverse order: disable the adapter, restore the previous chart
version and Channel CR shape.
