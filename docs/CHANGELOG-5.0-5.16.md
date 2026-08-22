# Changelog archive — chart 5.0.0 to 5.16.0

Migration guides for chart versions **5.0.0 through 5.16.0**, newest first, in
[Keep a Changelog](https://keepachangelog.com/en/1.1.0/) format.

Moved here from [CHANGELOG.md](CHANGELOG.md), which holds the ten most recent
versions.

## [5.16.0] — 2026-08-14

Image: `channel-telegram` `0.11.0`.

### Added

`telegram-bundle.surface.deleteTopicOnDelete`, off by default. Deleting a
conversation deletes its forum topic instead of archiving it with a tombstone.

```yaml
telegram-bundle:
  surface:
    deleteTopicOnDelete: true
```

**This destroys the transcript.** The default trade is the other way — the topic
stays, keeping the history, at one dead topic per conversation. A busy group may
prefer the reverse.

- The bot needs `can_delete_messages`. Without it the operation is reported
  failed rather than quietly falling back to archiving, and the conversation is
  still deleted once the grace expires.
- The setting lives on the `Channel`, so two Telegram surfaces served by one
  adapter can differ.
- **Closing is unaffected.** A closed conversation can be reopened into its
  topic, so that topic must survive.
- A line naming the conversation goes to the chat general surface, so a vanished
  topic is attributable to agent-ops rather than to a hand deletion.

### Upgrade

Nothing. Opt-in, off by default.

## [5.15.0] — 2026-08-14

Images: manager `0.31.0`, `channel-telegram` `0.9.0`, console `0.12.0`.

### Fixed

A closed conversation's threads are told it can be reopened. Deleting it made
that false and said nothing.

The last message a chat thread carried was therefore a promise nobody could keep.
On an already-closed conversation the deletion was completely silent, because its
topics were already archived.

### Added

Outbound operation **`delete-conversation`**, sent once per bound thread. The
conversation and its record are gone, and a new message starts a new one.

It **replaces** `close-topic` on the deletion path, so a conversation being
deleted gets one operation, never both.

It is named for the conversation rather than the topic, because a conversation is
what ended. What that means for a thread is the adapter's decision.

`channel-telegram` un-archives the topic, posts the notice and closes it again. A
closed forum topic refuses `sendMessage`, and an open one would invite replies
into a conversation that no longer exists.

It does **not** delete the topic. The history above the tombstone is what a
person scrolls back to.

**For adapter authors:** implementing the kind is optional. An adapter that does
not is reported failed, the 2-minute grace expires and deletion proceeds — the
same posture `close-topic` always had. The notice arrives as an ordinary `notice`
message.

### Upgrade

Nothing is configured and nothing changes default.

## [5.14.0] — 2026-08-14

`/close` means something different from the moment you upgrade. Both new windows
are off by default, so nothing is reclaimed or destroyed that was not before.

### Changed

**BREAKING (semantics): closing no longer deletes.** `/close` used to delete the
`Conversation`, and with it `status.runs[].result`, the context handle and
eventually the workspace directory. That made closing irreversible, which is
precisely why nobody closed anything and why conversations accumulated without
bound.

A closed conversation now sits at phase `Closed` — no runtime pod, no MCP
ConfigMap, no dispatch, no capacity consumed, absent from conversation reuse and
from every pipeline.

Its object, its recorded answers, its context handle and its volume state all
stay intact.

```sh
kubectl -n <ns> get conversations --field-selector status.phase=Closed
```

**It can be reopened** from the console, back to `Idle` with its wiring and its
history. Materialized refs are left exactly as they are, so no Pipeline edit
leaks into a conversation that already exists.

Threads are re-established through the ordinary `ensure-topic`, carrying the
archived thread id as a hint. An adapter that can un-archive continues in the
same thread. One that cannot opens a fresh one, and ignoring the hint is a valid
implementation.

### Added

Two retention windows, two clocks, **both off by default**:

```yaml
retention:
  autoclose:                 # close a FINISHED conversation after it goes quiet
    enabled: true
    idleAge: 168h            # measured from LAST ACTIVITY, never from creation
  autodelete:                # delete a CLOSED conversation
    enabled: true
    closedAge: 720h          # measured from status.closedAt, reset by a reopen
```

`autoclose` closes only a conversation that is genuinely finished: `Idle`, no
pending inputs, no inflight run, no runtime pod, **and** every recorded run
delivered to every bound thread.

That last clause is not decoration. A conversation goes `Idle` the moment its
result is recorded, while the reply may still be an unclaimed `send` op. Closing
on `Idle` alone can archive a thread out from under its own answer.

**Choose `closedAge` as "how long do I want to be able to read this", not "how
long until it is tidy".**

`status.runs[].result` is the only place an answer lives in the Kubernetes API.
The console projects its transcript from the CR, and metrics keep aggregates
only. For a conversation bound to no channel there is no transport copy
anywhere.

**An opt-in housekeeping CronJob reclaims disk.** Only something that mounts the
claim roots can, and the manager mounts no volume by invariant.

```yaml
housekeeping:
  enabled: true
  dryRun: true               # take this off only after a run looks right
```

It removes workspace directories with no `Conversation` of that name, and session
transcripts no conversation references that are older than `sessionGrace`.

A **closed** conversation still has a CR, so its state is protected by the same
rule that identifies an orphan. The job is phase-blind on purpose. An "only look
at live ones" optimisation would reclaim the workspace of every conversation you
were keeping.

It runs under its own ServiceAccount with read-only access to conversations,
never the runtime SA. Mounting the claim root is exactly the reach `subPath`
isolation denies agents, and the render fails if the two identities are equal.

**Enabling autodelete without the job reclaims the API half and leaves the disk.**
That is correct with persistence off and a silent leak with it on.

### Upgrade

**APPLY THE CRDs BEFORE THE MANAGER IMAGE.** `Conversation` gains phase `Closed`
plus `status.closedAt`, `status.threadsArchived[]` and `status.reopens`.

A manager on this version against the previous CRD cannot close anything. The API
server rejects the phase, and because closing is retried, it fails in a loop:

```
status.phase: Unsupported value: "Closed": supported values: "Pending", "Idle", "Queued", "Working"
```

A normal `helm upgrade` handles this on its own — the chart ships CRDs as
templates and `crds.enabled` defaults `true`. It bites installs that manage CRDs
out of band: `crds.enabled: false`, a separate GitOps stage, or a cluster where
several releases share one set.

The schema change is purely additive, so applying it early is safe. An older
manager never writes any of the new fields.

**Recommended order:**

1. Upgrade and watch the `Closed` rows for a while.
2. Use reopen once to confirm it works.
3. Turn autoclose on with a long window.
4. Turn the housekeeping job on with `dryRun`.
5. Turn autodelete on last.

**To get the old behaviour** — closing that reclaims — enable autodelete with a
short window:

```yaml
retention:
  autodelete:
    enabled: true
    closedAge: 1h
```

## [5.13.0] — 2026-08-13

prometheus-bundle `0.2.0`.

### Changed

**BREAKING: `vm-bundle` is now `prometheus-bundle`.** The bundle was named for a
vendor it does not depend on.

- Its ingest core parses the standard Alertmanager webhook payload and nothing
  else, so any Prometheus Alertmanager could always post to it.
- VictoriaMetrics answers the Prometheus HTTP query API, and MetricsQL is a
  PromQL superset, so one query server serves both backends.

VictoriaMetrics is now a supported backend rather than the subject.

**The adapter CR name default changed** from `vm-alertmanager` to `alertmanager`,
and the default source's from `vm-alerts` to `alerts`.

The adapter name is the ROUTING KEY every `SignalSource` names in `spec.adapter`.
The source name is the `{source}` segment of the webhook URL.

**`mcp__victoriametrics__*` stops resolving.** The metrics server key is now
`prometheus`, so allowlists naming the old namespace keep rendering and quietly
grant nothing.

**Self-registration is unchanged but now labelled VictoriaMetrics-only.** It
configures the sender by writing a `VMAlertmanagerConfig`, and vanilla
Alertmanager's configuration is a file or a Secret, not an object an adapter can
write.

With registration off, NOTES.txt prints the exact `receivers:` stanza to paste
into your Alertmanager configuration. It includes `send_resolved: false`, because
the adapter drops non-firing alerts and a sender left at its default posts
resolutions that are silently discarded.

### Removed

**The logs component is REMOVED, not ported.** `mcp.vmlogs` and the
`mcp-victorialogs` workload are gone. VictoriaLogs speaks LogsQL over its own
endpoints, and no Prometheus query server can reach it.

### Added

`AgentProfile/alert-investigator` renders by default — identity only, with an
inline role, since it has no repository and no agent definition file can be
resolved for it.

`pipelines.enabled` defaults **false** and nothing forces it on. Demo mode never
enables this bundle at all. Turning it on renders one `Pipeline/alert-triage`
claiming the bundle's own source, and **every admitted alert then opens a
conversation and spends LLM credits**.

Sources are shareable, so the bundle's route and one you declared under the
parent chart's `pipelines:` may both claim `alerts`. That fans out to one
conversation per claiming Pipeline. NOTES.txt reports it, and it is never
refused.

### Upgrade

Action needed if you set any `vm-bundle.*` value. **The render FAILS until you
rename the key**, deliberately — Helm never reports an unread values key, so an
unguarded rename would present as a successful upgrade that installed nothing.

1. **Rename the values key.** Every `vm-bundle.*` value moves to
   `prometheus-bundle.*`. Nothing else about the ingest lane changed.
2. **Restore the old names** if you do not want to edit hand-written sources or
   reconfigure your sender:

   ```yaml
   prometheus-bundle:
     alertmanager:
       name: vm-alertmanager
       defaultSource:
         name: vm-alerts
   ```
3. **Rebind the metrics allowlists** to `MCPToolset/prometheus-observability` and
   `MCPConfig/prometheus-api`. Find every affected object:

   ```sh
   kubectl get pipelines,mcptoolsets -A -o yaml | grep -n "victoriametrics\|victorialogs"
   ```

   The toolset is wildcarded (`mcp__prometheus__*`) because all six tools the
   server registers are read-only. There is no read/mutate split to preserve,
   unlike `k8s-bundle`'s.
4. **If you used the logs component**, apply these two objects by hand and keep
   binding them from your Pipeline:

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

**No effect on the manager, the CRDs or any Go module.** The
`signal-vmalertmanager` module, its image and its spec keep their names in this
release.

## [5.12.0] — 2026-08-13

k8s-bundle `0.3.0`.

### Added

The k8s bundle gains a fourth component: **its own wiring**. Until now it
rendered the events adapter, the source, the profile and the MCP tooling —
everything except the one object that makes them do anything.

`global.demo.enabled` therefore put a complete Kubernetes agent in the cluster
that answered nothing until you read NOTES.txt and applied a Pipeline by hand.

**Which route renders derives from `global.agentops.runtime.rbacMode`**, the same
value the MCP server's `--read-only` flag, the server SA's RBAC and the
`k8s-admin` toolset already follow:

| `rbacMode` | Route | What it can do |
|---|---|---|
| `readonly`, `none`, `""` (incl. demo) | `k8s-observe` | reads the cluster, changes nothing |
| `full` | `k8s-operate` | binds `k8s-admin` — **mutates the cluster** |

Widening to `full` therefore promotes the route as well.
`pipelines.observe.enabled` and `pipelines.admin.enabled` are explicit booleans
that win in both directions, and both may be true at once.

**No channel is bound by default**, so answers land in `status.runs[].result`.
`pipelines.channels: [<name>]` delivers them to a surface instead.

### Upgrade

Action needed only if you run `global.demo.enabled=true`, or enable `k8s-bundle`
and already claim `cluster-events` from your own `pipelines:`.

**For a demo install:** it now renders `Pipeline/k8s-observe`, claiming
`cluster-events` with the read toolsets and the `k8s-api` MCPConfig. The source
goes `Wired=True` and **every admitted Warning event opens a conversation and
spends LLM credits**, where before it dropped them.

That is the fix, and it is a real bill. On a noisy cluster, check the shipped
suppression rules before upgrading.

Keep the old behaviour — the parts without the route:

```sh
helm upgrade ... --set k8s-bundle.pipelines.enabled=false
```

That value is absolute. It declines the route even under demo mode, and every
other bundle component is untouched.

**For everyone else: nothing.** The flag is off outside demo mode. Enabling the
bundle for its adapter and profile still renders no Pipeline.

**If you already claim `cluster-events` yourself** and also turn this on, both
Pipelines render and the source **fans out** — one event, two conversations, two
profiles' worth of capability, two bills.

That is legal, since sources have been shareable since 5.10.0, and deliberately
not refused. The post-install notes name the pipelines involved.

## [5.10.0] — 2026-08-11

Two behaviour changes. Both need action only if a `SignalSource` is listed by more
than one Pipeline.

### Changed

**BREAKING: a shared source now routes to EVERY Pipeline listing it.**
Exclusivity is gone. `sourceConflicts`, the `SourceConflict` condition and the
oldest-claimant tiebreak are deleted.

A Pipeline that sat at `Ready=False, reason=SourceConflict` becomes `Ready=True`
and starts answering. One alert on a shared source now opens **two conversations
— two agents, two runtimes, two LLM bills.**

A `SourceConflict` condition left on such a Pipeline is **cleared automatically**
on the first reconcile. Deleting the rule that wrote it does not delete what it
already wrote, so the manager removes it for one release.

Per-source cooldown and signature grouping are evaluated once, above the fan-out.
A fingerprint is admitted once and delivered to each server. The ingest response
reports `queued` (signals) and `conversations` (one per server) separately, and
`receivedTotal` still counts signals.

**BREAKING: a bare message on a chat surface several Pipelines serve is
refused.** With one Pipeline serving the chat source, nothing changes.

With several, an unaddressed message opens no conversation. The surface is
answered with the Pipelines available and the `/<pipeline> <task>` form.

**Thread replies are unaffected** and never needed a prefix. Addressed messages
are unaffected too.

### Added

- **`Conversation.spec.pipelineRef`** — provenance only, written at creation,
  never read to resolve wiring. It is what keeps two Pipelines fanning out from
  one source from appending to each other's conversation.

  Conversations created before this release carry none and nothing backfills
  them. Such a conversation keeps grouping while one Pipeline serves its source,
  and is left alone once a second joins.
- **`/agents` lists each Pipeline with its answering profile**, matching the
  console's new composer typeahead. Typing `/` in "New conversation" lists the
  Ready Pipelines and inserts the addressed form. No new RBAC, no new manager
  endpoint, no console values to set.

### Upgrade

**Apply the CRDs before the manager image.** `pipelineRef` is additive, so an
older manager ignores it, but a newer one cannot write it against an un-updated
CRD.

Check whether any source has more than one claimant. If nothing prints, this
release changes no behaviour for you.

```sh
kubectl get pipelines -o json | jq -r '
  .items[] | .metadata.name as $p | .spec.signalSourceRefs // [] | .[] |
  "\(.name)\t\($p)"' | sort | awk '{c[$1]=c[$1]" "$2} END {for (s in c) if (split(c[s],a," ")>1) print s ":" c[s]}'
```

If the fan-out is not what you want, drop the source from every Pipeline but the
intended one:

```sh
kubectl patch pipeline <the-one-that-should-not-answer> --type=json \
  -p '[{"op":"remove","path":"/spec/signalSourceRefs/<index>"}]'
```

If it IS what you want, nothing to do. That is now a supported configuration.

## [5.9.0] — 2026-08-11

### Changed

**You will stop getting a conversation per evicted pod.** `Evicted` moves from
the past-tense tier (`for: "0"`, one signal per displaced pod) into the tier-1
drop list.

An eviction was already reported from both ends, and per pod from neither:

| Eviction | Still reported by |
|---|---|
| kubelet, under node pressure | `NodeHasMemoryPressure` / `NodeHasDiskPressure` — tier 3, `for: 0`, **one node-level signal** instead of one per pod |
| API-initiated (a drain) | nothing, deliberately — a drain is an operator doing what they were told, and it is unattended wherever a reboot manager such as Kured runs |
| a pod that does not come back | `FailedScheduling` — tier 5, `for: 5m`, confirmed by a dwell |

What the drop costs is the case where pods evict, reschedule cleanly, and the
node reports no pressure — a cluster working as designed. What it buys is that a
node drain no longer produces one conversation per pod on that node.

This applies the existing rule that a reason may be dropped only where its
underlying problem is still caught by a reason that is not dropped.

It is not an exception to the rule that past-tense reasons never dwell. `Evicted`
is still never given a non-zero dwell. It is simply not emitted on its own.

Because the drop leans on those two substitutes, the render test now pins them
*together* with it. Re-tuning node pressure or `FailedScheduling` cannot silently
leave eviction unreported from every direction at once.

### Upgrade

Nothing, unless you relied on per-pod eviction signals.

To restore them, restate the whole `rules` list. Helm replaces list values rather
than merging them, so overriding a single tier silently drops the other five:

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

## [5.8.0] — 2026-08-11

### Fixed

The chart generates two credentials — the console UI token and the adapter master
token — and both were meant to survive upgrades via a cluster `lookup`.

On a real `helm upgrade` they did. But `lookup` returns nothing wherever the
renderer has no cluster, so every `helmfile diff` reported both as changed on an
install where nothing had:

```
agent-ops, agentops-adapter-token, Secret (v1) has changed:
-   token: '-------- # (32 bytes)'
+   token: '++++++++ # (32 bytes)'
```

Cosmetic on the diff, and not cosmetic anywhere the render is applied.
`helm template | kubectl apply`, CI, a GitOps controller and a client-side dry
run all produce a *fresh* token.

That signs every console session out and invalidates every adapter at once, since
per-adapter tokens are HMACs of the master.

**A generated credential now leaves the upgrade path entirely.** With no explicit
value the Secret renders on install only, carrying
`helm.sh/resource-policy: keep`. Nothing random exists on the upgrade path to be
applied, whichever renderer runs.

**An explicitly configured value now wins.** Precedence is explicit → existing
Secret → generate.

`console.auth.uiToken` was checked *last*, so on any install that already had a
token it was accepted, documented and silently ignored. Rotating is now a values
edit rather than "delete the Secret, then upgrade".

### Added

**`adapterAuth.token`**, matching `console.auth.uiToken` and the
`runtime.credentialsSecret.token` pattern. Supply it and the credential is
release-managed from your secret store. Leave it empty and it is generated.

Changing it 401s every adapter until its pod restarts with the new env. That is
inherent to rotating a master credential, and stated at the setting.

An explicit value is rendered on install **and every upgrade**, because that is
how changing it takes effect. It is also the way back if a generated Secret is
deleted by hand — once the chart has stopped managing the object, no upgrade
restores it.

`NOTES.txt` no longer prints "fetch your token" after every deploy. It names the
source in effect, since that instruction is where the belief that the token
rotates on every deploy came from.

### Upgrade

**ONE COMMAND REQUIRED before upgrading an existing install.** Annotate the two
generated Secrets, or the upgrade deletes them:

```sh
kubectl -n <ns> annotate secret agentops-adapter-token agentops-console-console \
  helm.sh/resource-policy=keep
```

The chart refuses to render without it and prints this command, so a forgotten
step is a failed upgrade rather than a lost credential.

Skip it only for a fresh install, or where you supply both credentials yourself.

Why the step exists: Helm reads that annotation off the **live object**, not off
the manifest dropping it. A Secret created by an earlier chart carries none and
gets deleted by the first upgrade that stops rendering it.

Once annotated, the object stays put with the same value. It is a removal from
the release manifest, not from the cluster.

## [channel-telegram 0.7.1] — 2026-08-11

### Fixed

An agent's answer describing its own tools never reached Telegram.

Inline code was converted to `<code>` **in place**, and the emphasis regexes then
ran over the tags it had just written. One `*` inside `` `.claude/agents/*.md` ``
and another inside `` `mcp__kubernetes__*` `` paired with **each other**, across
the prose between them.

That opened `<i>` inside one `<code>` and closed it inside the next. Telegram
rejects a message with overlapping entities outright, so the send op failed and
the answer never arrived.

```
can't parse entities: Unmatched end tag ... expected "</i>", found "</code>"
```

The failure had a nasty shape. It needed TWO inline-code spans each containing a
star, which is unremarkable prose for an agent describing its own tooling — glob
patterns and tool allowlists are exactly where stars live.

The console showed the answer, having no HTML, while the Telegram thread stayed
silent. It read as a Telegram-side problem.

Inline code is now lifted out before emphasis, the same treatment fenced blocks
already had, and restored afterwards. Emphasis *spanning* code still nests
correctly.

Both are pinned by tests, one of which checks tag NESTING rather than substrings.
A "contains `<i>`" assertion cannot see the defect that broke this.

### Upgrade

Bump the image tag. No values change.

## [5.7.0] — 2026-08-11

console `0.8.1`.

### Added

An install that fronts the console with oauth2-proxy, Cloudflare Access or an
Envoy ext_authz filter used to authenticate twice: once at the proxy, then again
with a shared token that identifies nobody.

The second adds no security — the request already got past the proxy — and costs
a credential to distribute and rotate.

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
  verify the claim, but it can insist you make it.
- **An empty `uiToken` still authorizes nobody.** "No credential configured" and
  "no credential required" stay independent, because the whole hazard is one
  being read as the other. Half a declaration leaves the console closed.
- **Writes then require a forwarded identity.** With token auth off there is no
  `token` fallback. A write log naming a credential nobody presented is worse
  than none, since it looks like an audit trail.

  A proxy that authenticates but forwards no identity therefore yields a
  **read-only console** — reads served, writes refused, the UI showing `unknown`
  and saying why. Forward `X-Forwarded-Email` or `X-Auth-Request-User`.
- **The identity headers are BELIEVED.** The fronting proxy must strip
  client-supplied copies of them, or a caller picks their own identity. The
  console cannot tell the two apart, since they arrive on the same connection.
- **The token Secret is still rendered.** The console Channel projects it with
  `envFrom`, so removing it would turn "disable auth" into "the console will not
  start". Re-enabling authentication stays one value.

The SPA follows: no login form on a console that accepts no token, no sign-out
button where there is no session to end, and the composer says why a reply is
unavailable instead of failing on submit.

The pod logs the mode twice, on purpose. `authDefault=` at startup is the process
env. `console auth: external:<name>` is the EFFECTIVE mode, logged when the
served Channel's config resolves it.

The startup line alone would report `token` on an externally-authenticated
console, because the config arrives after boot. That is the one state this
setting must not be able to hide in.

### Upgrade

Nothing. Every existing install keeps requiring its token.

## [5.6.0] — 2026-08-11

signal-k8s-events `0.3.0`.

### Added

Some outages are on a schedule. A router that restarts at 04:00 takes the
cluster's connectivity with it for a quarter of an hour, every night, and the
events it produces are *real*.

None of the three existing suppression axes can silence them:

| Axis | Why it cannot |
|---|---|
| `for:` | verifies a condition the outage genuinely satisfies |
| inhibition | needs a cause event a power cut never produces |
| matchers | select on labels, and there is no label for the time of day |

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
Alertmanager's forms. A window spanning midnight is two entries. Overlapping
intervals union.

Three things to know before writing one:

- **Name your `location`.** It defaults to UTC, as in Alertmanager, but "four in
  the morning" is a local fact. A UTC-pinned window drifts by an hour at each
  daylight-saving change and stops covering the outage it was written for, on a
  date nobody chose. The IANA database is compiled into the adapter image.
- **Narrow with `matchers`.** With none, the source goes deaf for the whole
  window. A restarting router produces connectivity reasons. It does not produce
  `OOMKilling`, and an OOMKill at 04:05 is as real as one at noon.
- **Muting is evaluated at emit**, after the dwell and before the emit cap. A
  problem that outlives the window is still reported once it closes, and a muted
  burst never spends the emit budget.

Muting is not silent. While a window is active the source's `Ready` condition
stays true and names the interval (reason `Muted`), and reports the muted count
when it ends (`MuteEnded`).

A malformed interval fails the source rather than being ignored. A typo producing
a window that never fires looks exactly like one that works.

### Upgrade

Nothing changes unless you configure a window. Requires
`k8s-bundle.eventsAdapter.image.tag` 0.3.0, which is the chart default.

## [5.5.0] — 2026-08-11

manager `0.28.0`, runtime-claude `0.6.0`.

### Changed

**BREAKING (API): `Conversation.status.sessionId` is renamed to
`runtimeContextId`.** "Session" is claude-code's noun. agent-ops has
Conversations, and what a runtime returns is its own handle for one.

Both fields are READ for one release — preferring the new, adopting the old — and
only the new is written, so no in-flight conversation loses its handle on
upgrade.

The work unit carries both names for the same period, so a runtime image upgrades
independently of the manager.

Anything reading `sessionId`, including the console, should move.

**A context that cannot be continued now FAILS the run.** The runtime no longer
retries without its context and answers anyway.

A conversation without its context is a new one wearing the same name and thread,
and an agent asked to undo something it has no memory of will guess.

The failure is articulate — a stated reason, a message on the thread naming the
remedy — which is what the old fallback existed to avoid. A failed run's result
now reaches bound threads instead of a bare "run failed".

### Fixed

The context handle was recorded write-once:

```go
if d.SessionID != "" && conv.Status.SessionID == "" {   // before
```

When a continuation failed, the runtime correctly started a new context, and that
handle was never recorded because the field was already set.

The conversation then named a context that no longer existed, so **every
subsequent message** repeated the same failed continuation. One transient loss
became permanent.

The handle is now latest-wins, and is recorded on FAILED runs too, so a crash
after a context was established does not strand it.

### Added

**Unavailability is an outage before it is a loss.** Bounded retry in the runtime
distinguishes a store that says GONE from one that did not ANSWER. A
manager-side circuit breaker then holds work rather than failing it when many
conversations report unavailability at once.

Without it, one two-minute storage incident would permanently destroy every
active conversation's context.

**`AgentRuntime.spec.contextStorage`** (`volume` | `external` | `none`, default
`volume`). A runtime keeping context on a home volume the deployment does not
provide can never continue anything.

No handle is then issued and the conversation is single-run **by declaration** —
answering each message fresh instead of failing every follow-up for a
configuration you chose.

`NOTES.txt` says so, and names the single-node topology (RWO or a `local` PV plus
`runtime.nodeSelector`) for clusters without distributed storage.

**`ContextContinuity` condition** carries the runtime's own reason, verbatim. The
manager does not know where a given runtime keeps context and does not guess.

### Upgrade

**Manager first, runtime after.** The manager is compatible with the current
runtime image, which simply makes no continuity claim. The new runtime works
against either.

Do not remove the dual read until no conversation can still carry only
`sessionId`.

## [5.3.0] — 2026-08-10

runtime-claude `0.5.0`.

### Removed

**BREAKING for anything that shelled out to `kubectl` inside a runtime pod.**

`runtime-claude` shipped a pinned `kubectl` v1.34.3, the one domain-specific
dependency in an image whose other contents are runtime responsibilities (git,
openssh-client for the checkout) or generic shell utilities.

That contradicted the rule the rest of the project already follows. An
`AgentRuntime` differs by vendor backend and trust level, and what an agent may
reach is wiring — `MCPConfig` plus `MCPToolset` bound by a Pipeline.

A CLI in the vendor layer was the same category error as bundling an MCP server
would be, and it carried a version pin that could skew against whatever cluster
it ran near.

**What breaks.** An agent whose route relied on `Bash` plus `kubectl` for cluster
access has none after upgrading the image. `Bash` still works. It just no longer
reaches the Kubernetes API.

Also dropped: two tool patterns from `k8s-observability`
(`configuration_contexts_list`, `targets_list`) that the shipped server does not
register. Inert entries, but they implied capability the install never had.

### Added

**New warning.** Enabling the k8s bundle with `mcp.enabled=false` now leaves an
agent that cannot see the cluster at all, so the post-install notes say so. The
render still succeeds — pointing `mcp.url` at your own server is legitimate.

### Upgrade

**The hold position is one line.** `AgentRuntime.spec.image` is why that field
exists. Pin the previous tag and nothing changes:

```yaml
runtime:
  image: kmatsebora/agentops-runtime-claude:0.4.0
```

**To migrate**, give the route MCP tooling. With the k8s bundle that is already
the default: `mcp` and `mcpServers` are on, and a Pipeline binds
`k8s-observability` (reads) and optionally `k8s-admin` (mutations).

Verified against the shipped server: crashlooping pods, node pressure and failed
workloads are all answerable through `pods_list`, `pods_log`, `events_list`,
`resources_list` and `nodes_top`, which return kubectl-shaped tables.

**What MCP does not give you:** patch semantics, rollout/drain/wait,
port-forward, `auth can-i`, and any text processing over results, because there
is no pipe.

If you need those, build a derived image — three lines — and point an
`AgentRuntime` at it. The version pin becomes yours to own against your cluster,
which is the right place for it.

## [5.2.0] — 2026-08-10

Chart only. No operator, image, CRD or contract change.

### Added

**New `console.ingress` keys:** `extraHosts[]` (additional hostnames serving the
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

`tls[].hosts` is DERIVED from `host` plus `extraHosts`, so a rule host and a
certificate host cannot drift apart.

**The old list form still works.** `console.ingress.tls` supplied as a list of
raw Ingress `tls` entries — the only form before this release — is detected and
rendered verbatim.

`helm upgrade --reuse-values` carries a previous release's `console:` map forward
wholesale, so an upgrade needs no values edit.

**New warning, no new failure.** Enabling the Ingress with no TLS configured
still renders, because TLS often terminates upstream at a load balancer or in a
mesh and the chart cannot see what sits in front of it.

The post-install notes now state that the UI token crosses the network in clear
text, and name both remedies. If your TLS terminates upstream, ignore it.

### Changed

**BREAKING (render-time), only if you set a sub-path.** `console.ingress.path`
must be `/`.

The console's SPA is embedded at build time with an absolute asset base and emits
`/assets/...` URLs, so serving it under `/console` routes correctly and then
renders a blank page.

The chart now refuses that configuration instead of producing it. Give the
console its own hostname or subdomain.

### Upgrade

Nothing unless you had already set a path that did not work. Existing values
files keep rendering the same Ingress.

## [5.1.0] — 2026-08-10

manager `0.26.0`, console `0.7.1`.

### Changed

**BREAKING for value-reset installs: `persistence.enabled` now defaults to
`true`.** A fresh install requests a ReadWriteMany claim for `/data/home`, so
agent session files survive a runtime pod restart instead of dying with it.

A `helm upgrade` on an existing release is unaffected unless values are reset.

On a cluster with no default StorageClass or no RWX provisioner, the claim sits
`Pending`, runtime pods never schedule and conversations queue forever — with no
error anywhere else, because the kubelet is what waits.

```sh
helm upgrade ... --set persistence.enabled=false
```

Sessions then live in `emptyDir` and die with each runtime pod. An agent resuming
a lost session answers without prior context. `NOTES.txt` prints the diagnosis,
and `chart/ci/default-values.yaml` sets it false for test installs.

### Fixed

**The agent's answer survives a manager restart.** Previously, a restart between
`POST /work/done` and an adapter claiming the outbound op dropped the reply
permanently. The result was durably in `status.runs[].result` and delivered to
nobody.

Replies now carry a stable op id (`send:<conversation>:<channel>:<runId>`),
delivery is recorded per bound thread in `status.runs[].delivered[]`, and
reconciliation re-enqueues anything still owed. A partially delivered fan-out
completes the remaining threads without repeating the delivered one.

**Ingest cooldown survives a restart.** Fingerprint suppression is recorded on
`SignalSource.status.cooldown[]`, pruned past its window and bounded at 200
entries, and read back on first use per source.

A restart mid-incident no longer re-opens conversations for alerts still being
suppressed. Only an admitted fingerprint writes. Suppressed re-deliveries, the
high-volume case, write nothing.

**Telemetry gaps are reported, not rendered as silence.** The activity ring stays
bounded, in-memory and lossy.

A cursor it cannot serve — evicted, or issued by a previous manager process — is
now answered with a resync, and the console renders that as an explicit gap in
its timeline instead of an empty window that reads as "nothing happened".

Conversations, topology and configuration are unaffected, being read from
Kubernetes. The gap is carried on the console's stream health, so a browser
opened *after* the incident sees it too.

**The console's `live` chip recovers on its own.** `EventSource` gives up
permanently on an HTTP error — a 502 during a rollout, a 401 on session expiry —
rather than retrying, and nothing reopened it.

The masthead stuck on `stream disconnected` and the graph stayed frozen until the
page was reloaded. The client now reconnects with backoff, leaving genuine
transport blips to the browser.

### Added

`persistence.workspace`, off by default — a second claim backing the repository
checkout, mounted per-conversation via `subPath` so concurrent runtime pods never
share a working tree.

Turning it on preserves uncommitted agent work across a pod restart and skips the
re-clone.

Off by default because a fresh checkout is cheap and always correct, whereas a
stale shared one is neither. Nothing reclaims a conversation's directory after
deletion, so size the claim accordingly.

New CRD fields, all optional, old objects stay valid:

- `AgentRuntime.spec.workspace`
- `Conversation.status.runs[].delivered[]` and `.deliveryTracked`
- `SignalSource.status.cooldown[]`

### Upgrade

**Upgrading re-posts nothing.** Runs recorded before this release carry no
`deliveryTracked` marker. On first observation they are recorded as delivered
*without* sending, so no bound thread receives an old answer again.

Rollback: revert the chart and the image. The new fields are ignored by an older
manager, and both claims carry `helm.sh/resource-policy: keep`, so no data is
destroyed by a downgrade.

## [5.0.0] — 2026-08-10

manager `0.25.1`, channel-telegram `0.7.0`, console `0.6.0`.

**Upgrade all three together.** The manager and every channel adapter share the
outbound message contract, and version 2 is not compatible with version 1 in
either direction.

The signal adapters and `telegram-router` are unaffected. They never consume
`/channel/ops`.

### Changed

**BREAKING: outbound ops carry a typed message, and adapters render it.** An op
no longer has `text` or `title`.

- `send` carries `op.message`: `signal` (the event, with `source`, `labels{}`,
  the payload inline and `inputRef`), `answer` (`body`, `status`), `relay`
  (`origin`, `sender`, `body`) or `notice` (`level`, `body`).
- Prose is markdown in a named subset — `**bold**`, `*italic*`, `` `code` ``,
  fenced blocks, `[text](url)`. Anything outside it is undefined.
- `ensure-topic` carries `op.topic`, a descriptor the adapter NAMES the thread
  from. Telegram's 128-character topic limit is now enforced where it is known.
- `GET /channel/ops` requires `contract=2`. An adapter that declares nothing, or
  an older version, gets a 400 naming the replacement, because one still reading
  `op.text` would post empty messages and look healthy doing it.

The manager stops guaranteeing anything about how a message looks. Escaping, the
4096-character split and topic naming moved into `channel-telegram`, the
reference renderer.

In-process providers get the same treatment. Exempting them would put
presentation back inside the manager through a side door.

**Two bugs close with it.** A message over 4096 characters used to fail the op
outright, and a payload containing `<`, `>` or `&` broke HTML parsing — so the
alerts most worth reading were the ones that did not arrive.

**The agent's own output moved with it.** `dispatch/templates/format.md`, the
mandatory message-format spec shipped to every profile, told the agent to write a
"chat HTML subset" (`<b>`, `<code>`, `<pre>`), which used to pass through to
Telegram untouched.

Adapters escape what they are handed, so that HTML now reaches chat as literal
characters. The six templates are markdown. Custom prompts that instruct HTML
must be updated the same way. A resumed session picks the new spec up on its next
work unit.

**BREAKING: `InputItem.jobName` is replaced by `origin`**
(`{kind: signal|channel, name, signalKind}`). The old field recorded the source
for `kind: job` only, so a conversation could not say what woke it. Anything
reading `jobName` breaks — read `origin.name`.

**Signature keying now splits on the lane** when a source declares no
`signatureLabels`.

`alert` and `job` keep the default `alertgroup`/`alertname`/`namespace` labels,
so alert grouping and cron-tick folding are unchanged. `task` and `chat` key on
the signal's own fingerprint, so each request opens its own conversation.

A source that DOES declare `signatureLabels` groups by them in every lane, which
means **a posted task inherits the target source's grouping**. Create your own
source if you want an isolated ask lane.

**`console.enabled` flips from `false` to `true`.** Upgrading STARTS A POD that
was not running before.

It reads every `agentops.dev` CR in the namespace, plus Deployments and Pods, and
it can instruct any agent it is joined to. That is why this is a major bump
rather than a feature.

### Removed

**BREAKING: `POST /task` is gone.** The endpoint, its handler and its request
type are deleted. There is now no HTTP route that names a `Pipeline`.

Programmatic origination is an ordinary signal posted to a `SignalSource` that a
Ready Pipeline claims. Which agent answers, on which channels, with which
capabilities is decided by declared wiring rather than chosen by the caller.

**No deprecation shim is offered.** A 410 would preserve exactly the doorway this
change closes, and the API group is provisional pre-1.0.

**Dropped without replacement:** the `agent` override, since per-call role
selection remains available through the chat form `/<pipeline>:<agent>`, and the
`channel` field, which let a caller add a surface. That is wiring, and wiring is
declared on the Pipeline.

### Added

- **New signal kind `task`**, alongside `alert`, `job` and `chat`. Task-lane
  prompt, no `jobName`, no recurrence-on-session, and — unlike `chat` — no
  `agentops.dev/channel` label required, because replies go to the claiming
  Pipeline's channels.
- **A conversation thread now opens with the event that caused it.** An alert
  thread used to be a topic title, then silence, then the agent's interpretation.
  If the agent hung, the thread never said what had happened.

  Every input a human has not already seen is now posted to the bound threads as
  a `signal` card, in parallel with dispatch. `ConversationInput` gains `labels`,
  kept beside the payload so an adapter can render them.

  **An input with no origin posts nothing**, so upgrading does not fill open
  threads with history.
- **Per-hop activity telemetry.** `GET /activity`, `GET /activity/stream` (SSE)
  and `POST /activity`, under the existing adapter bearer scheme. A bounded
  in-memory ring (`ACTIVITY_BUFFER`, default 10000), never persisted. The durable
  record stays `status.runs[]`.
- **Introspection.** `GET /status` — runtime slots, op queue depth with the
  oldest stuck item's identity, cooldowns, leader — and
  `GET /pipelines/{name}/resolved`, the authoritative capability resolution.
- **Prometheus metrics** on the existing `:9090`. No new listener. Optional
  scrape templates under `metrics:` (VMServiceScrape, ServiceMonitor, example
  alert rules), all default-disabled because neither CRD is guaranteed present.
- **`SignalAdapter.spec.servedBy`.** `spec.image` becomes optional. An adapter
  declaring `servedBy` owns no workload and reports `Ready=True/ServedBy`.
- **`console.metrics.url`** points the console at a Prometheus or VictoriaMetrics
  query endpoint, which lets it render windows far beyond the activity buffer as
  clearly labelled aggregates. Unset, every view still works and long windows are
  reported unavailable rather than drawn empty.

### Upgrade

**Migrating a `POST /task` caller.** Before:

```sh
curl -sX POST http://agentops-manager.<ns>.svc:8080/task \
  -H 'Content-Type: application/json' \
  -d '{"pipeline":"k8s-engineer","task":"why is pod X crashlooping?"}'
```

After:

```sh
TOKEN=$(kubectl -n <ns> get secret agentops-adapter-token \
  -o jsonpath='{.data.token}' | base64 -d)
curl -sX POST http://agentops-manager.<ns>.svc:8080/signal/inbound \
  -H "Authorization: Bearer $TOKEN" \
  -d '{"source":"cluster-events","signals":[{"fingerprint":"ask-1",
       "kind":"task","payload":"why is pod X crashlooping?"}]}'
```

Three things change: a different URL, a bearer token (`ADAPTER_TOKEN`, the same
one adapters use), and a **source** name where a pipeline name used to go.

Pick a source the Pipeline you want already claims:

```sh
kubectl get pipeline <name> -o jsonpath='{.spec.signalSourceRefs}'
```

The `fingerprint` is yours to choose and is what cooldown dedups on. A build id
or a timestamp is the usual answer.

**The console opt-out is one value:**

```yaml
console:
  enabled: false
```

With that set, nothing about your install changes.

One caveat if you later turn it off again. Once a Pipeline references the console
— `channelRefs: [console]` or `signalSourceRefs: [console]` — disabling the
console removes those objects and the Pipeline correctly reports
`unresolved references: signalsource/console, channel/console` and stops being
Ready.

Remove the references in the same change as the opt-out. `helm upgrade --wait`
will otherwise fail on that Pipeline.

**If you keep the console on**, it is a `ChannelAdapter` **and** a
`SignalAdapter`, and still ONE Deployment. The SignalAdapter declares `servedBy`
the ChannelAdapter, so it owns no workload and simply receives a second token in
the same pod.

- **What it reads.** Its Role gains `apps/deployments` and `pods`
  (get/list/watch, namespaced, read-only) on top of the `agentops.dev` kinds.
  Image digests, restart counts and pod failure reasons exist in no CR. There are
  still no write verbs anywhere in it.
- **What it can do.** `console.write.enabled` defaults `true`, so the chat
  composer and "new conversation" are live. Set it `false` for a strict viewer —
  the affordances disappear AND both endpoints refuse.
- **Origination is refused until you wire it.** The chart renders a
  `SignalSource` named `console` and NO Pipeline. Until some Ready Pipeline
  claims that source it sits at `Wired=False`, and the UI shows that reason with
  the patch:

  ```sh
  kubectl patch pipeline <name> --type=json \
    -p '[{"op":"add","path":"/spec/signalSourceRefs/-","value":{"name":"console"}}]'
  ```
- **Exposure.** Still `ClusterIP` with `console.ingress.enabled: false`. If you
  expose it, put an authenticating proxy in front. Without one every write is
  recorded as `token`.

**For third-party channel adapters:** implement the four kinds, name the topic
from the descriptor, and send `contract=2`. `channel-telegram/render.go` is ~180
lines and is the whole job.

No rendered chart object changes, no CRD schema field is added or removed, and no
stored object changes shape.
