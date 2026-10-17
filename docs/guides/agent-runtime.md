---
title: "Run agents on your own backend"
permalink: /guides/agent-runtime/
description: >-
  What an agent runtime is, the three parts it is built from, and how to
  implement one for a backend agent-ops does not ship.

next:
  eyebrow: Reference
  title: Every field of every kind
  body: >-
    The generated custom resource reference, and the contracts the adapter and
    runtime kinds serve.
  url: https://github.com/kostiantyn-matsebora/agent-ops-operator/blob/master/docs/cr-reference.md
---

An **agent runtime** is what actually executes an agent. It is a container image
plus an `AgentRuntime` resource describing it: the image, the identity it runs
as, and the SHAPE of its context storage.

**It declares no volume.** WHERE a route's conversations keep their state is the
route's decision, on its `Pipeline` — see the
[Pipeline guide]({{ '/guides/pipeline' | relative_url }}).

A runtime is an ENGINE. Two routes sharing one must be able to persist to
different volumes without cloning it.

There is one per vendor and trust level, shared by every route that names it.
The image implements a small HTTP contract — it asks the manager for work, runs
it, and reports what happened.

![The manager hands a work unit to your runtime inside the conversation's pod, which runs the agent and reports the result back.]({{ '/assets/img/guides/agent-runtime-light.svg' | relative_url }}){: .ao-diagram}

## Before you start

Writing a runtime is appropriate when:

- The **backend** is different — another vendor's CLI, your own harness. A
  local model over Ollama is [already shipped]({{ '/runtimes/ollama/' | relative_url }}),
  and is the worked example of a runtime that is its own harness.
  [GitHub Copilot]({{ '/runtimes/copilot/' | relative_url }}) is the worked
  example of a vendor SDK whose tool vocabulary has to be translated.
- You need behaviour the contract allows but `runtimes/claude` does not
  implement.

It is **not** what you want when:

- You only need a **tool in the image**. Derive from `runtimes/claude` and point
  `spec.image` at yours. That is what the field is for.
- You need a different **tool allowlist**. That is wiring, and it lives on the
  Pipeline — see
  [Give your agent tools]({{ '/guides/toolsets/' | relative_url }}).

{: .ao-callout}
> **Your runtime applies the tool allowlist, and nothing checks that it did.**
> A runtime that ignores it voids every toolset binding in the install,
> silently. Read the allowlist station below before you write any of it.

Review [the runtime contract]({{ '/contracts/runtime/' | relative_url }}) and
[AgentRuntime](https://github.com/kostiantyn-matsebora/agent-ops-operator/blob/master/docs/concepts.md#agentruntime)
first. This page builds the smallest thing that satisfies them, station by
station, and every station names the exact section of the contract it builds
toward.

## The overall shape

An agent runtime is three parts:

1. **A container image** implementing the work contract — poll for a unit,
   execute it, report the outcome, exit when idle.
2. **An `AgentRuntime` resource** declaring that image, the ServiceAccount it
   runs as **by default**, and what continuity it can promise.
3. **A `Pipeline` naming it** through `runtimeRef`. A Pipeline that names none
   falls back to the runtime called `default`.

**A RUNTIME IS ONE OF SEVERAL.** Declare any number under the chart's
`runtimes:` list, each stating only what DIFFERS from
`global.agentops.runtimeDefaults` — the image, the resources, the idle TTL, the
egress posture and the credential shape are all inherited, so a second vendor is
usually two lines:

```yaml
runtimes:
  - name: my-runtime
    image: registry.example.com/my-agentops-runtime:1.0.0
```

**`default` is what a route naming no `runtimeRef` resolves to.** The chart
renders it as a copy of one declared runtime — the one flagged `default: true`,
or the first configured. Declare yours alone and it is the default.

Declare no runtime at all and the render FAILS, naming the routes that needed
one. That is the honest alternative to conversations queueing forever.

A bundle may ship a runtime of its own the same way.

The manager creates one pod per conversation from part 2, and that pod is your
image. It gives the pod `CONTROL_URL`, `CONVO_ID`, `POD_NAME`,
`RUNTIME_IDLE_TTL_M`, `REPO_URL`, `REPO_REF` and the profile's git
credentials — the full list is [Environment]({{ '/contracts/runtime/' | relative_url }}#environment).

**Checking the repository out is your runtime's own job, never the
manager's.** See [Runtime's obligations]({{ '/contracts/runtime/' | relative_url }}#runtimes-obligations).
The stations below build that sequence in order, starting with the poll.

**The pod dials the manager, never the reverse.** Your runtime needs no inbound
port and no Service.

## 1. Poll for work

Long-poll. The call blocks until there is a unit or the wait expires — see
[`GET /work`]({{ '/contracts/runtime/' | relative_url }}#get-work).

```sh
curl "$CONTROL_URL/work?convo=$CONVO_ID&pod=$POD_NAME&wait=25"
```

```powershell
curl "$env:CONTROL_URL/work?convo=$env:CONVO_ID&pod=$env:POD_NAME&wait=25"
```

A `204` means nothing is queued. Poll again. A unit looks like this — every
field is named in [Work unit]({{ '/contracts/runtime/' | relative_url }}#work-unit):

```json
{
  "runId": "r-7",
  "convo": "task-kvb55",
  "promptText": "…",
  "agent": "sre",
  "allowedTools": "Read,Grep,mcp__kubernetes__*",
  "toolsMode": "merge",
  "runtimeContextId": "abc123",
  "maxTurns": 60
}
```

`promptText` arrives fully rendered — the manager has already wrapped the
payload in a prompt. A profile that replaces that wrapper sends `promptFile`
and `promptVars` instead, both relative to the checkout, and your runtime
renders them.

**Verify:** the `curl` above returns `204` against an idle conversation, and
returns a unit shaped like the one above once you give that conversation
something to do.

## 2. Clone the repository

**Repository checkout is your runtime's obligation, never the manager's** —
see [Runtime's obligations]({{ '/contracts/runtime/' | relative_url }}#runtimes-obligations). The
manager hands you `REPO_URL`, `REPO_REF` and the git credentials the profile
declares, and stops there.

`REPO_URL` and `REPO_REF` are always set and may both be empty — see
[Environment]({{ '/contracts/runtime/' | relative_url }}#environment). An empty `REPO_URL` means
this conversation has no repository, so clone nothing rather than treating
the empty string as an error.

```sh
if [ -n "$REPO_URL" ]; then
  git clone --branch "$REPO_REF" "$REPO_URL" /data/workspace
fi
```

```powershell
if ($env:REPO_URL) {
  git clone --branch $env:REPO_REF $env:REPO_URL /data/workspace
}
```

Authenticate from whichever pair the environment carries: `GIT_AUTH_TYPE`
with `GIT_SSH_KEY`, or `GIT_AUTH_TYPE` with `GIT_TOKEN` — same
[Environment]({{ '/contracts/runtime/' | relative_url }}#environment) table.

**Verify:** `/data/workspace` holds the checkout at `REPO_REF` once
`REPO_URL` is non-empty, and stays empty when `REPO_URL` is empty. That path
never moves — see [Filesystem]({{ '/contracts/runtime/' | relative_url }}#filesystem).

## 3. Compose the tool allowlist

**`allowedTools` is half of the allowlist.** The other half is the `tools:`
frontmatter of `.claude/agents/<agent>.md` in the checkout, which only your
runtime can read, because only your runtime holds the repository.

The rule for combining them is [Composing the
allowlist]({{ '/contracts/runtime/' | relative_url }}#composing-the-allowlist) — this station
demonstrates it and states nothing beyond it.

`toolsMode` directs how the two combine:

| `toolsMode` | Pass to the model |
|---|---|
| `merge` | the union, the definition's own entries keeping their position |
| `overwrite` | `allowedTools` alone |

**An empty allowlist means empty.** The contract is explicit: substituting a
default is a grant nobody wrote down. No repository means no definition to
read, so `merge` degrades to `allowedTools` alone — that is correct, not a
fallback.

In practice, compose the list once and pass exactly that to the model.

Re-applying the definition afterward as an intersection would quietly turn
`overwrite` back into `merge`. Falling back to an interactive prompt for a
missing tool has nobody in the pod to answer it, so the run would simply
hang until `RUNTIME_IDLE_TTL_M`.

**Verify:** log the composed list before it reaches the model.

```sh
echo "allowlist: $COMPOSED_TOOLS"
```

```powershell
Write-Output "allowlist: $env:COMPOSED_TOOLS"
```

Confirm it matches the table above for both a `merge` and an `overwrite`
unit.

## 4. Run the unit and report the outcome

Stream progress to **stdout**. That is the live transcript an operator reads
with `kubectl logs`, and the only window into a run while it happens.

```sh
curl -X POST "$CONTROL_URL/work/done" -d '{
  "convo": "task-kvb55", "runId": "r-7", "status": "succeeded",
  "result": "the answer, as markdown",
  "runtimeContextId": "abc123", "continuity": "continued"
}'
```

```powershell
curl -X POST "$env:CONTROL_URL/work/done" -d '{
  "convo": "task-kvb55", "runId": "r-7", "status": "succeeded",
  "result": "the answer, as markdown",
  "runtimeContextId": "abc123", "continuity": "continued"
}'
```

Every field of this report is named in [Completion
report]({{ '/contracts/runtime/' | relative_url }}#completion-report). `status` is
`succeeded` or `failed` — the manager reads any other value as a failed run,
see [Enums]({{ '/contracts/runtime/' | relative_url }}#enums).

**`result` is the deliverable.** The operator hands it to every bound channel
through their adapters, so your runtime never posts to a transport and holds
no channel credentials.

**`runtimeContextId` is your own handle, and the manager interprets nothing.**
It stores the string and hands it back on the next unit. Report the context
you actually finished on, which is not always the one you were asked to
continue.

`continuity` says what happened to the context you were handed:

| Value | Means |
|---|---|
| `continued` | you resumed it |
| `new` | you started fresh, and nothing had been promised |
| `unavailable` | you could not reach it — the manager fails the run rather than answering fresh under its name |

If you report `turns[]` or `toolCalls[]`, trim each to the
[bounds]({{ '/contracts/runtime/' | relative_url }}#bounds) — 200 entries, 200 bytes per string
field — before posting. The manager refuses the WHOLE report with `400` over
any bound, never trims it for you.

**Verify:** the `POST /work/done` above returns `200`. Post a report over a
bound instead — 201 entries in `turns[]`, say — and confirm it comes back
`400` naming the field and the bound rather than being trimmed and accepted.

## 5. Exit when idle

Exit `0` after `RUNTIME_IDLE_TTL_M` minutes with no work — the one condition
[Lifecycle]({{ '/contracts/runtime/' | relative_url }}#lifecycle) names for exiting on your own.

The pod outliving the answer is deliberate, so a follow-up skips the cold
start. The manager creates a fresh pod on the next input.

**Verify:** set `RUNTIME_IDLE_TTL_M` low on a test deployment and watch the
pod exit `0` on its own once that many minutes pass with no `GET /work`
returning a unit.

## 6. Declare the AgentRuntime

<!-- generated: template kind=AgentRuntime name=my-runtime fields=image,contextStorage,serviceAccountName,idleTtlMinutes comments=off -->
```yaml
apiVersion: agentops.dev/v1alpha1
kind: AgentRuntime
metadata:
  name: my-runtime
spec:
  image: <image>
  contextStorage: volume
  serviceAccountName: <serviceAccountName>
  idleTtlMinutes: 10
```
<!-- /generated -->

`contextStorage` is what the manager reads **before** promising a reader
that a follow-up keeps its context.

That declaration unlocks restoration
([`GET /work`]({{ '/contracts/runtime/' | relative_url }}#get-work), API-4) and
checkpoint ordering
([Manager's provisions]({{ '/contracts/runtime/' | relative_url }}#managers-provisions), MGR-2).

It is the one storage question only your runtime can answer — whether its
backend writes context to a disk at all:

| Value | Your runtime keeps context |
|---|---|
| `volume` | on a context volume, whichever one the conversation resolved |
| `external` | somewhere the manager does not manage |
| `none` | nowhere — every unit starts fresh |

**It does not say WHICH volume, and cannot.** The manager checks this
declaration against what the conversation actually resolved.

Under `volume`, a conversation whose route and release both supply none is told
from its first run that it cannot be continued, rather than failing every
follow-up.

**The ServiceAccount is the agent's in-cluster power**, and a `Pipeline` may
name its own and override what you declare here.

**That is what lets one image serve two trust levels.** An observing route and
an acting route differ in their account, not in their engine, so you write one
`AgentRuntime` and not two.

**A route that names no account gets one bound to nothing.** Leaving this field
empty is the safe default — it falls through to that floor rather than to
anything the release granted.

**If you write your own account, hold it to the rules the chart's own follow**:
no verb on `secrets`, no wildcard, no `escalate` or `bind`, and nothing in the
operator's own namespace.

**And letting an agent create or exec into a pod grants it every Secret in that
namespace, whatever its RBAC says.** The kubelet does the reading. See
[installation](https://github.com/kostiantyn-matsebora/agent-ops-operator/blob/master/docs/installation.md).

**Verify:** `kubectl apply -f my-runtime.yaml` succeeds. The object carries
no condition of its own to watch — the next station's Pipeline `Ready`
condition is what confirms it actually resolves.

## 7. Point a Pipeline at it, and verify

This station builds toward [Manager's
provisions]({{ '/contracts/runtime/' | relative_url }}#managers-provisions), where the manager
creates one pod per conversation from the runtime the Pipeline resolves.

Declaring it through the chart is the ordinary path — the `AgentRuntime` above
is what that renders, and applying one by hand is the same object.

```sh
kubectl -n agent-ops apply -f my-runtime.yaml
kubectl -n agent-ops patch pipeline my-route --type=merge \
  -p '{"spec":{"runtimeRef":{"name":"my-runtime"}}}'
kubectl -n agent-ops logs -f agentops-conv-<conversation>
```

```powershell
kubectl -n agent-ops apply -f my-runtime.yaml
kubectl -n agent-ops patch pipeline my-route --type=merge `
  -p '{\"spec\":{\"runtimeRef\":{\"name\":\"my-runtime\"}}}'
kubectl -n agent-ops logs -f agentops-conv-<conversation>
```

**Conversations already open keep the runtime they were created with.** The
choice is recorded when a conversation starts, so patch the Pipeline and then
ask something new.

**A typo in `runtimeRef` is reported on the Pipeline, not discovered later.**
Naming an `AgentRuntime` that does not exist turns the Pipeline's `Ready`
condition `False`, naming the missing runtime.

**Verify:** apply the missing `AgentRuntime` and `Ready` converges with no
further edit. Ask your agent something — the pod log shows the unit
arriving, then the allowlist you composed, then the answer.

## What comes next

1. **Read the specification in full** — [the runtime
   contract]({{ '/contracts/runtime/' | relative_url }}) covers every field, every enum, the
   bounds and their refusal, and the conformance checklist a claim of
   conformance is walked against.
2. **Compare against the reference** —
   [`runtimes/claude`](https://github.com/kostiantyn-matsebora/agent-ops-operator/blob/master/runtimes/claude/),
   about 200 lines of contract handling.
3. **Declare where your context lives** — `spec.contextSync.paths` is include
   globs relative to `$HOME`, and the manager then points your `CONTROL_URL` at
   a sidecar that checkpoints them for you. Nothing in your runtime changes.

   **Only you can state them.** The `claude` bundle ships the reference
   runtime's paths, and they describe where *claude-code* files transcripts —
   they are in that bundle rather than the release-wide runtime defaults for
   exactly this reason. Yours go with your own image and credential, in your own
   `runtimes:` entry or bundle. Nothing infers a path: **a runtime declaring
   none gets the durable volume mounted into the agent container directly and no
   synchronisation at all**, and an empty list is refused rather than read as
   "copy everything".

   ```yaml
   spec:
     image: ghcr.io/example/my-runtime:1.0.0
     contextStorage: volume                  # `external` if your backend keeps it
     contextSync:
       paths: ["<where your backend writes context>"]
       exclude: ["**/*.lock"]                # churn inside those paths
   ```

   It also needs a durable context volume to snapshot to. Without one the pod is
   the unsynchronised one with ephemeral context, and the conversation is told
   its context is not promised rather than failing to start.
4. **Wall off an uncooperative agent** —
   [`platform/egress-proxy`](https://github.com/kostiantyn-matsebora/agent-ops-operator/blob/master/platform/egress-proxy/)
   mediates what the pod may reach, for the case an allowlist cannot cover.
5. **Publish multi-arch.** A single-arch image fails at schedule time, not at
   build, and possibly weeks later.
