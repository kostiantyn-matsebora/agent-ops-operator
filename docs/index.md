---
title: Something happens. An agent takes care of it.
eyebrows:
  - Automation that thinks
  - Kubernetes-native, end to end
lede: >-
  A signal wakes it, your prompt tells it what to do, your YAML decides what it
  may touch — a crashlooping pod or the hallway lights.
description: >-
  agent-ops is a Kubernetes operator for agents you can address: a signal wakes
  one, your wiring decides what it may touch, and it answers in a thread you can
  reply to.

stats:
  - value: 11
    icon: kinds
    label: custom resource kinds
    note: kubectl, GitOps and RBAC already work.
  - value: 3
    icon: contracts
    label: pluggable contracts
    note: Signals, runtimes, channels.
  - value: 0
    icon: secrets
    label: Secrets the operator reads
    note: Everything secret-shaped is resolved by the kubelet, never read here.
  - value: 3
    icon: bundles
    label: ready-made bundles
    note: Kubernetes, Telegram, Prometheus — switch one on.
---

{: .ao-chipsets}
- **Work arrives from**
  - ![]({{ '/assets/img/logos/kubernetes.svg' | relative_url }}) Kubernetes events
  - ![]({{ '/assets/img/logos/prometheus.svg' | relative_url }}) Alertmanager
  - Cron schedules
  - ![]({{ '/assets/img/logos/home-assistant.svg' | relative_url }}) Home Assistant
  - ![]({{ '/assets/img/logos/telegram.svg' | relative_url }}) A message in chat
  - ![]({{ '/assets/img/logos/agent-ops.svg' | relative_url }}) The console
  - your own

- **Can reach**
  - ![]({{ '/assets/img/logos/kubernetes.svg' | relative_url }}) Kubernetes API
  - ![]({{ '/assets/img/logos/prometheus.svg' | relative_url }}) Prometheus
  - ![]({{ '/assets/img/logos/home-assistant.svg' | relative_url }}) Home Assistant
  - ![]({{ '/assets/img/logos/mcp.svg' | relative_url }}) any MCP server

- **Answers you in**
  - ![]({{ '/assets/img/logos/agent-ops.svg' | relative_url }}) The console
  - ![]({{ '/assets/img/logos/telegram.svg' | relative_url }}) Telegram
  - your own

{: .ao-tabs #tour}
- **How it works** — Something happens, you declare what to do about it in your own cluster, the operator runs it.

  ![Something happens — an alert fires, a pod crashloops, a schedule comes due, a room gets too warm, someone asks — and each feeds into one Helm install in your own cluster. There you declare it as custom resources: a Pipeline for what wakes it, an AgentProfile for what it should do, an MCPToolset for what it may touch, shown as a real Pipeline manifest naming its signal source, profile, toolset and channel. The operator then runs it: one conversation per incident in its own thread, its own isolated pod, picking up where it stopped.]({{ '/assets/img/agent-ops-landing-light.svg' | relative_url }}){: .ao-diagram}

- **What you write** — One `Pipeline`. It is the whole route, and it is the only place wiring lives.

  ```yaml
  apiVersion: agentops.dev/v1alpha1
  kind: Pipeline
  metadata:
    name: k8s-ops
  spec:
    signalSourceRefs:
      - name: cluster-events      # what wakes it
    profileRef:
      name: k8s-engineer          # what it should do
    toolsets:
      refs:
        - name: agentops-observe  # what it may touch
    channelRefs:
      - name: telegram            # where you talk to it
  ```

- **Overview** — Everything the install is doing, and every condition that is not `True`.

  ![Manager version and leader, two of five runtime slots in use, the runtime images, live-activity telemetry, tables of workloads and adapters, and one problem: a signal source no pipeline claims.]({{ '/assets/img/console/overview-light.png' | relative_url }})

- **Topology** — The wiring as a live graph: where signals enter, what claims them, who answers.

  ![Signal adapters and sources on the left, then three pipelines, the profiles and runtimes that execute them, and the channels the answers reach, with traffic rates on the edges between them.]({{ '/assets/img/console/topology-light.png' | relative_url }})

- **Conversations** — The whole fleet, filtered by phase, pipeline or profile. Unread is per identity.

  ![Six conversations with mixed phases — Working, Pending, Idle and Closed — two marked unread, each showing its pipeline, run count, queue depth and last activity.]({{ '/assets/img/console/conversations-light.png' | relative_url }})

- **Conversation** — One agent's transcript, every run with its result, and the box you reply in.

  ![One conversation: the signal that started it, the agent's answer explaining an OOM-killed container, a reply relayed in from another channel, and a box to reply from.]({{ '/assets/img/console/conversation-light.png' | relative_url }})

- **Queues** — What is waiting, and what is stuck. Every stalled row names its cause.

  ![The work queue with three conversations, one flagged at runtime ceiling, the delivery queue per adapter with the oldest operation in each, and a suppressed-signal cooldown.]({{ '/assets/img/console/queues-light.png' | relative_url }})

- **Configuration** — Every kind, and the columns that matter. A Pipeline row is the whole route on one line.

  ![Three pipelines, each showing its profile, the sources it claims, the channels it posts to, its toolsets, tools mode, MCP configs and Ready status.]({{ '/assets/img/console/configuration-light.png' | relative_url }})

The last six are the console, which ships enabled. The
[Console page]({{ '/console/' | relative_url }}) takes each view in turn.

## What "takes care of it" means

- **Investigates** — queries the system, reads state.
- **Explains** — in a thread you can reply to.
- **Acts** — only where your wiring granted it.
- **Asks** — when it needs you, it says so.

## Why it is built this way

- **Judgment, not a fixed sequence.** You describe the job in prose. Nobody
  enumerates the steps, and nothing has to have been predicted in advance.
- **Self-hosted, end to end.** It runs in your cluster on your credentials. No
  prompt, transcript or alert is sent to a vendor.
- **Bounded by construction.** One isolated pod per conversation. Tools come
  only from the wiring, and the operator itself never reads a Secret.

## Pluggable at three seams

Documented HTTP contracts, no fork.

- **Your own signal source** — Datadog, Dynatrace, Sentry, a sensor on your
  bench.
- **Your own agent runtime** — Codex, Gemini, Copilot or a script of your own.
  Swap the image. The work contract does not change.
- **Your own channel** — Slack, Teams, Discord, e-mail. The operator sends
  meaning, your adapter renders it.

## Where to start

- **[Introduction]({{ '/introduction/' | relative_url }})** — how the pieces fit
  together: what wakes an agent, what decides what it may touch, what runs it.
- **[Getting started]({{ '/getting-started/' | relative_url }})** — a read-only
  demo in fifteen minutes: install it and ask an agent about your cluster.
- **[The console]({{ '/console/' | relative_url }})** — the six views above at
  full length, and how to decide who may reach them.
- **[Installation]({{ '/installation/' | relative_url }})** — the real install:
  what to decide first, what to configure, and how to wire your first route.
- **[The kinds you will declare](https://github.com/kostiantyn-matsebora/agent-ops-operator/blob/master/docs/concepts.md)** —
  every CRD in full, and how a route's tool access is resolved.

## Understand the model

- [Concepts](https://github.com/kostiantyn-matsebora/agent-ops-operator/blob/master/docs/concepts.md)
  — profiles, runtimes, conversations, channels, signal sources and the
  Pipeline that wires them together.
- [Contracts](https://github.com/kostiantyn-matsebora/agent-ops-operator/blob/master/docs/contracts.md)
  — the work contract a runtime implements, both adapter contracts, and the
  HTTP API.

## Run it

- [The console]({{ '/console/' | relative_url }})
  — the wiring as a graph, live runs, and the channel it also is.
- [Cluster events](https://github.com/kostiantyn-matsebora/agent-ops-operator/blob/master/docs/k8s-bundle.md)
  — the events lane, the agent that answers it, and Kubernetes MCP tooling.
- [Telegram](https://github.com/kostiantyn-matsebora/agent-ops-operator/blob/master/docs/telegram-bundle.md)
  — the ingest stack and the chat surface.
- [Prometheus](https://github.com/kostiantyn-matsebora/agent-ops-operator/blob/master/docs/prometheus-bundle.md)
  — the Alertmanager alert lane, its metrics tooling and the agent that answers.
- [Home Assistant](https://github.com/kostiantyn-matsebora/agent-ops-operator/blob/master/docs/ha-bundle.md)
  — the house's log lane, and two agents split by what they may do to it.

## Keep it current

- [CHANGELOG](https://github.com/kostiantyn-matsebora/agent-ops-operator/blob/master/CHANGELOG.md)
  — every chart-version upgrade guide, newest first.
- [The repository](https://github.com/kostiantyn-matsebora/agent-ops-operator)
  — source, issues, and the working notes contributors read first.

> The reference pages above are read on GitHub today. Bringing them onto this
> site — with navigation, cross-page links and their own contents — is the next
> change. This one lands the site itself.
