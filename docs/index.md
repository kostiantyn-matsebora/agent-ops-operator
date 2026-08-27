---
title: agent-ops
description: >-
  agent-ops is a Kubernetes operator for agents you can address: a signal starts
  one, your wiring decides what it may touch, and it answers in a thread you can
  reply to.
---

A Kubernetes operator for agents you can address.

{: .ao-claims}
- ![]({{ '/assets/img/claim-thinks.svg' | relative_url }}) Automation that thinks
- ![]({{ '/assets/img/logos/kubernetes.svg' | relative_url }}) Kubernetes-native
- ![]({{ '/assets/img/claim-gitops.svg' | relative_url }}) GitOps-ready
- ![]({{ '/assets/img/logos/claude.svg' | relative_url }}) Runs Claude Code
- ![]({{ '/assets/img/logos/ollama-light.svg' | relative_url }}) Runs Ollama
- ![]({{ '/assets/img/logos/github-light.svg' | relative_url }}) Runs GitHub Copilot

{: .ao-tabs #tour}
- **How it works**

  {: .ao-presentation}
  1. Something happens.

     ```yaml
     # nothing declared yet
     ```

  2. One Helm install puts an agent in the path.

     ```text
     helm install agent-ops agentops/agent-ops
     ```

  3. You declare the route. One Pipeline.

     ```yaml
     kind: Pipeline
     metadata:
       name: k8s-ops
     ```

  4. What starts it.

     ```yaml
       signalSourceRefs:
         - name: cluster-events
     ```

  5. What it should do.

     ```yaml
       profileRef:
         name: k8s-engineer
     ```

  6. What it may touch — and nothing else.

     ```yaml
       toolsets:
         refs:
           - name: agentops-observe
     ```

  7. Which servers those tools come from.

     ```yaml
       mcpConfigs:
         refs:
           - name: k8s-api
     ```

  8. Where you talk to it.

     ```yaml
       channelRefs:
         - name: telegram
     ```

  9. Then it runs. One conversation, its own pod.

     ```yaml
     # one Conversation, one pod, strictly serial
     ```

  10. Every part of it is a Kubernetes object.

     ```text
     $ kubectl get conversations
     cluster-events-7c1d4e   Running   2m
     ```

- **Watch it work** — One signal, start to finish. A minute, no sound.

  [![The console showing one conversation: the cluster-events signal that opened it, the agent's answer explaining an OOM-killed container, and the box to reply in.]({{ '/assets/video/console-demo-poster-light.png' | relative_url }})]({{ '/assets/video/console-demo-light.mp4' | relative_url }}){: .ao-demo data-captions="{{ '/assets/video/console-demo.vtt' | relative_url }}"}

- **What you write** — One `Pipeline`. It is the whole route, and it is the only place wiring lives.

  ```yaml
  apiVersion: agentops.dev/v1alpha1
  kind: Pipeline
  metadata:
    name: k8s-ops
  spec:
    signalSourceRefs:
      - name: cluster-events      # what starts it
    profileRef:
      name: k8s-engineer          # what it should do
    toolsets:
      refs:
        - name: agentops-observe  # what it may touch
    mcpConfigs:
      refs:
        - name: k8s-api           # where those tools live
    channelRefs:
      - name: telegram            # where you talk to it
  ```

{: .ao-chipsets}
- **Works with**
  - [![]({{ '/assets/img/logos/kubernetes.svg' | relative_url }}) Kubernetes]({{ '/integrations/kubernetes/' | relative_url }})
  - [![]({{ '/assets/img/logos/prometheus.svg' | relative_url }}) Prometheus]({{ '/integrations/prometheus/' | relative_url }})
  - [Cron schedules]({{ '/guides/signal-adapter/' | relative_url }})
  - [![]({{ '/assets/img/logos/home-assistant.svg' | relative_url }}) Home Assistant]({{ '/integrations/home-assistant/' | relative_url }})
  - [![]({{ '/assets/img/logos/telegram.svg' | relative_url }}) Telegram]({{ '/integrations/telegram/' | relative_url }})
  - [![]({{ '/assets/img/logos/agent-ops.svg' | relative_url }}) The console]({{ '/console/' | relative_url }})
  - [![]({{ '/assets/img/logos/mcp.svg' | relative_url }}) any MCP server]({{ '/guides/toolsets/' | relative_url }})
  - [![]({{ '/assets/img/logos/ollama-light.svg' | relative_url }}) Ollama]({{ '/runtimes/ollama/' | relative_url }})
  - [![]({{ '/assets/img/logos/github-light.svg' | relative_url }}) GitHub Copilot]({{ '/runtimes/copilot/' | relative_url }})
  - [your own]({{ '/guides/signal-adapter/' | relative_url }})

## Why agent-ops?

The same wiring, wherever something needs looking at.

| Where | What happens |
|---|---|
| ![]({{ '/assets/img/logos/kubernetes.svg' | relative_url }}) **Watch and fix your cluster** | It reads the events, the pods and the logs, and names the cause. |
| ![]({{ '/assets/img/logos/prometheus.svg' | relative_url }}) **Answer your alerts** | Every firing alert arrives with the investigation already done. |
| **Run the checks nobody gets to** | Certificates, drift and capacity, on a schedule. |
| ![]({{ '/assets/img/logos/home-assistant.svg' | relative_url }}) **An assistant for your home** | Its logs, its devices, its config. Not everything is a cluster. |
| ![]({{ '/assets/img/logos/telegram.svg' | relative_url }}) **Ask it from chat** | It answers in the thread where your team already talks. |
| ![]({{ '/assets/img/logos/ollama-light.svg' | relative_url }}) **Keep it in the cluster** | Route the routine lanes to a model you host. Nothing they read leaves. |
| ![]({{ '/assets/img/logos/mcp.svg' | relative_url }}) **Plug in your own** | Three HTTP contracts: your source, your runtime, your channel. |
{: .ao-areas}

> ![]({{ '/assets/img/logos/agent-ops.svg' | relative_url }})
>
> **And all of it in one place.**
>
> Every conversation as it happens, what is queued, what is stuck and why — and
> the whole wiring as a graph. It is a channel too, so you answer the agent
> right there.
>
> - ships enabled
> - six views
> - [read-only on your cluster]({{ '/console/' | relative_url }})
{: .ao-console-strip}

## When it runs

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
  together: what starts a conversation, what decides what it may touch, what
  runs it.
- **[Getting started]({{ '/getting-started/' | relative_url }})** — a read-only
  demo in fifteen minutes: install it and ask an agent about your cluster.
- **[The console]({{ '/console/' | relative_url }})** — the six views at full
  length, and how to decide who may reach them.
- **[Security]({{ '/security/' | relative_url }})** — an agent runs model output
  in your cluster: what a default install grants, and what is still open.
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
- [Cluster events]({{ '/integrations/kubernetes/' | relative_url }})
  — the events lane, the agent that answers it, and Kubernetes MCP tooling.
- [Telegram]({{ '/integrations/telegram/' | relative_url }})
  — the ingest stack and the chat surface.
- [Prometheus]({{ '/integrations/prometheus/' | relative_url }})
  — the Alertmanager alert lane, its metrics tooling and the agent that answers.
- [Home Assistant]({{ '/integrations/home-assistant/' | relative_url }})
  — the house's log lane, and two agents split by what they may do to it.

## Keep it current

- [Changelog]({{ '/changelog/' | relative_url }})
  — every chart-version upgrade guide, newest first.
- [The repository](https://github.com/kostiantyn-matsebora/agent-ops-operator)
  — source, issues, and the working notes contributors read first.

> The reference pages above are read on GitHub today. Bringing them onto this
> site — with navigation, cross-page links and their own contents — is the next
> change. This one lands the site itself.
