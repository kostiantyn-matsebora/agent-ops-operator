---
title: agent-ops
description: >-
  agent-ops is a Kubernetes operator for LLM agents within boundaries you declare,
  cluster you control and conversations you stay in.
---

A Kubernetes operator for LLM agents within boundaries you declare, cluster
you control and conversations you stay in.

{: .ao-actions}
- [Get started]({{ '/getting-started/' | relative_url }})
- [Install it]({{ '/installation/' | relative_url }})

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

## What agent-ops is

agent-ops gives you a platform that puts an LLM agent behind the systems you
already run. One Helm install and your cluster grows a brain and a pair of
hands.

agent-ops is more than a model and a prompt in a pod: grouping and cooldown, a
capacity cap with a queue, context that survives a restart, per-route cluster
identity, mediated egress and at-least-once delivery are already in it.

{: .ao-cards}
- ![]({{ '/assets/img/logos/kubernetes.svg' | relative_url }}) Kubernetes native

  Eleven custom resources, validated by the API server like anything else you
  deploy. `kubectl get conversations` tells you what is running.

- ![]({{ '/assets/img/claim-gitops.svg' | relative_url }}) GitOps ready

  Every route is text. It reviews as a diff and deploys through the pipeline
  you already run.

- ![]({{ '/assets/img/logos/mcp.svg' | relative_url }}) Open at three seams

  Your own signal source, runtime or channel — Datadog, Sentry, Slack, a sensor
  on your bench. Documented HTTP contracts, no fork.

- Any model you can run
  {: .ao-icon-runtime}

  Claude Code, Ollama and GitHub Copilot ship with it. Point it at your own
  image and the work contract is unchanged.

- A pod per conversation
  {: .ao-icon-conversation}

  Isolated, serial and capped. One runaway conversation cannot take the others
  with it, and an idle one is reaped.

- Tools that come from the wiring
  {: .ao-icon-toolset}

  Not from the prompt. An agent holds the cluster identity its route named and
  nothing else, and the operator never reads a Secret.

> ![]({{ '/assets/img/logos/agent-ops.svg' | relative_url }})
>
> **Console — all in one place**
>
> Every conversation as it happens, what is queued, what is stuck and why — and
> the whole wiring as a graph. It is a channel too, so you answer the agent
> right there.
>
> - ships enabled
> - six views
> - [read-only on your cluster]({{ '/console/' | relative_url }})
{: .ao-console-strip}

## What it gives you

{: .ao-cards}
- ![]({{ '/assets/img/logos/kubernetes.svg' | relative_url }}) A cluster that explains itself

  It reads the events, the pods and the logs, names the cause, and fixes what
  you allowed it to.

- ![]({{ '/assets/img/logos/prometheus.svg' | relative_url }}) Alerts that arrive investigated

  Every firing alert turns up with the work already done, not with a link to a
  dashboard.

- The checks nobody gets to

  Certificates, drift and capacity, on a schedule — the work that is always
  worth doing and never urgent enough.

- ![]({{ '/assets/img/logos/home-assistant.svg' | relative_url }}) An assistant for your home

  Its logs, its devices, its config. Not everything that needs looking at is a
  cluster.

- ![]({{ '/assets/img/logos/telegram.svg' | relative_url }}) An answer where your team already talks

  Ask it in chat and reply in the thread it answers in. The conversation keeps
  its context between runs.

- ![]({{ '/assets/img/logos/ollama-light.svg' | relative_url }}) Routine work on a model you host

  Route the lanes that run all day to a local model. Nothing they read ever
  leaves.

## Where to go next

- **[Getting started]({{ '/getting-started/' | relative_url }})** — a read-only
  demo in fifteen minutes: install it and ask an agent about your cluster.
- **[Introduction]({{ '/introduction/' | relative_url }})** — how the pieces fit
  together: what starts a conversation, what decides what it may touch, what
  runs it.
- **[Installation]({{ '/installation/' | relative_url }})** — the real install:
  what to decide first, what to configure, and how to wire your first route.
- **[Security]({{ '/security/' | relative_url }})** — an agent runs model output
  in your cluster: what a default install grants, and what is still open.
- **The reference** — every CRD in full in
  [Concepts](https://github.com/kostiantyn-matsebora/agent-ops-operator/blob/master/docs/concepts.md),
  and the work and adapter contracts in
  [Contracts](https://github.com/kostiantyn-matsebora/agent-ops-operator/blob/master/docs/contracts.md).
  Both are read on GitHub today.
