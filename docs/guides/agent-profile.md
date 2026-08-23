---
title: "Add your own agent"
permalink: /guides/agent-profile/
description: >-
  What an AgentProfile is, why it carries no permissions at all, and how to
  declare an agent of your own and route to it.

next:
  eyebrow: Next
  title: "Run your agent from a repository"
  body: >-
    Version the role text, add an agent definition, and get the deploy key
    right first time.
  url: /agent-ops-operator/guides/agent-from-a-repository/
---

An `AgentProfile` is **everything about how an agent behaves** — its role, how
it decides, what it must never do, the repository it works from, how long it may
think, and what executes it.

The system prompt is not a label. It is the whole of the agent's judgement, and
it is the reason two profiles over one runtime are two different agents.

What a profile does **not** carry is **reach**. No tools, no MCP servers, no
channels. Those arrive from the Pipeline that routed the signal, which is why
the same profile serves routes with genuinely different capabilities without
being cloned.

![A Pipeline names an AgentProfile, which holds how the agent behaves, and an AgentRuntime executes it.]({{ '/assets/img/guides/agent-profile-light.svg' | relative_url }}){: .ao-diagram}

## Before you start

Declaring your own profile is appropriate when:

- **None of the installed agents is the one you want** — a different role, a
  different tone, a different model credential.
- You want a second agent beside an existing one, sharing its sources or not.
- You want an agent with **narrower bounds** — fewer turns, tighter resources.

It is **not** what you want when:

- You want an existing agent to **do more**, or to answer somewhere else. That
  is its Pipeline, and the profile does not change —
  [Put an agent to work]({{ '/guides/pipeline/' | relative_url }}).
- You want to change which tools it may call. Also the Pipeline.

**A profile nothing routes to is inert.** You will need a Pipeline naming it, so
read the Pipeline guide first if you have not.

Review
[AgentProfile](https://github.com/kostiantyn-matsebora/agent-ops-operator/blob/master/docs/concepts.md#agentprofile)
first.

## The overall shape

A profile is two declarations and one dependency:

1. **The behaviour** — `systemPrompt` for inline role text, or `repository` plus
   `agent` to take it from git. This is the substance of the object.
2. **The bounds** — `maxTurns`, and `resources` for the runtime pod.
3. **A Pipeline that names it.** Not part of the profile, and nothing happens
   without one.

That last line is the whole model in one sentence. Behaviour and reach are two
objects because the same agent, reached two ways, may touch two different
things.

**A profile does NOT select what executes it.** `runtimeRef` moved to the
Pipeline, because an `AgentRuntime` carries the ServiceAccount an agent runs as
— so choosing one chose the agent's power in the cluster, and that belongs
beside the tools the same route grants.

## Write the profile

`systemPrompt` is inline role text, appended to the runtime's own system prompt.
It is what a profile with no repository uses.

<!-- generated: template kind=AgentProfile name=my-agent fields=systemPrompt,maxTurns comments=off -->
```yaml
apiVersion: agentops.dev/v1alpha1
kind: AgentProfile
metadata:
  name: my-agent
spec:
  outputFormat: blocks   # blocks | none
  systemPrompt: <systemPrompt>
  maxTurns: 60
```
<!-- /generated -->

`outputFormat` is **required**, and declares what shape this agent's answers
take:

| Value | The agent is told |
|---|---|
| `blocks` | write a title, sections it names for the job, and `<details>` for the long tail. Every surface then renders that shape its own way — a fold in the console, an expandable quote in Telegram |
| `none` | nothing. This profile's own prompt owns formatting entirely |

**There is no default, and that is deliberate.** `none` leaves answers
unformatted unless your prompt says otherwise, and `blocks` would shape them by
something you never asked for. So you declare it, and a profile without it is
refused.

**Pick `blocks` unless your prompt already specifies an output shape.** The four
profiles the chart ships all declare it.

`maxTurns` bounds the agent's turns within **one work unit**. It is a runaway
bound, not a budget, and the conversation is unaffected.

`spec.runtimeRef` on a profile is **deprecated**. It is read for one release so
an existing profile keeps working, and is removed in the next major.

**Move it to every Pipeline that routes to this profile.** Setting both is
harmless and the Pipeline wins, so the two can be moved one route at a time.

## Route to it

A profile with no Pipeline is unreachable. The smallest one that reaches this
agent by name:

<!-- generated: template kind=Pipeline name=my-route fields=channelRefs comments=off -->
```yaml
apiVersion: agentops.dev/v1alpha1
kind: Pipeline
metadata:
  name: my-route
spec:
  profileRef:
    name: <name>
  channelRefs:
  - name: <name>
```
<!-- /generated -->

Point `profileRef.name` at your profile, and `channelRefs` at a surface you can
type on.
[Put an agent to work]({{ '/guides/pipeline/' | relative_url }})
covers claiming sources, binding toolsets and the addressing rules.

## Apply it, and verify

```sh
kubectl -n agent-ops apply -f my-agent.yaml
kubectl -n agent-ops get pipeline my-route \
  -o jsonpath='{.status.conditions[?(@.type=="Ready")]}'
```

```powershell
kubectl -n agent-ops apply -f my-agent.yaml
kubectl -n agent-ops get pipeline my-route `
  -o jsonpath='{.status.conditions[?(@.type==\"Ready\")]}'
```

`Ready=True` means the Pipeline resolved your profile. Then address it on the
surface you bound:

```
/my-route introduce yourself
```

The agent answers with an **empty allowlist** until the Pipeline binds toolsets,
so a first reply that can do nothing but talk is the expected result.

## Compare against a working profile

The chart's `k8s-bundle` ships this one:

<!-- generated: example preset=tier1 kind=AgentProfile name=k8s-engineer -->
```yaml
# Source: agent-ops-operator/charts/k8s-bundle/templates/profile.yaml
apiVersion: agentops.dev/v1alpha1
kind: AgentProfile
metadata:
  name: k8s-engineer
  namespace: agent-ops
  labels:
    app.kubernetes.io/name: agentops-k8s-bundle
spec:
  # Behaviour only — no repository, and no capabilities. What this agent may DO
  # comes from the Pipelines routing it: the bundle's own wiring component when
  # it is on, otherwise whatever the install declared.
  maxTurns: 40
  # Inline role, because this profile has no repository to hold a definition
  # file. Appended to the runtime's system prompt; it grants nothing.
  systemPrompt: |
    You are a Kubernetes site reliability engineer operating a live cluster.
    
    The mcp__kubernetes__* tools are how you reach the cluster. The runtime
    image ships no Kubernetes CLI, so there is no shell fallback: if a tool for
    something is not in your allowlist, you cannot do it — say so plainly
    instead of trying to improvise one.
    
    Those tools return kubectl-shaped tables and cannot filter server-side, so
    ask for the narrowest scope you can (a namespace rather than the cluster)
    and read what comes back rather than fetching everything twice.
    
    When a cluster event opens this conversation, investigate before you act: identify
    the failing object, read its recent events and logs, and state the likely
    cause. Make a change only when you are confident it is correct and its blast
    radius is bounded, and say plainly what you changed. Prefer the narrowest
    action that fixes the problem.
    
    Never delete or mutate anything in kube-system, and never touch a resource
    you were not asked about and did not diagnose. If a fix is risky, ambiguous,
    or would affect the control plane, describe what you would do and stop.
    
    Answer briefly. Lead with the finding, then the evidence.
  # REQUIRED, and with no default on purpose: `none` leaves output unformatted
  # unless this profile's prompt says otherwise, and `blocks` shapes it by
  # something the author never asked for. The author declares it.
  outputFormat: blocks
```
<!-- /generated -->

Nearly all of it is the prompt, which is the point — that is the agent. Note
what is **not** there: no tools, no MCP servers, no channels, no sources. Every
one of those is on the Pipeline that routes to it.

## What comes next

1. **[Run your agent from a repository]({{ '/guides/agent-from-a-repository/' | relative_url }})**
   — version the role text, add an agent definition under `.claude/agents/`,
   and get the deploy key right.
2. **[Give your agent tools]({{ '/guides/toolsets/' | relative_url }})**
   — so it can do more than talk.
3. **[Every AgentProfile field](https://github.com/kostiantyn-matsebora/agent-ops-operator/blob/master/docs/cr-reference.md#agentprofile)**
   — environment for the agent process, prompts, resources.
