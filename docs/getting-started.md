---
title: Getting started
permalink: /getting-started/
description: >-
  Install agent-ops, ask an agent a question about your cluster from the
  console, and see what a first run looks like.

next:
  eyebrow: Next
  title: Wire a route of your own
  body: >-
    The agent could read your cluster because its Pipeline granted a toolset,
    not because of anything in the profile. Change the wiring and the same
    agent has a different reach.
  url: https://github.com/kostiantyn-matsebora/agent-ops-operator/blob/master/docs/concepts.md
---

Install the operator and get your first question answered, in the console. About
fifteen minutes, most of it the install.

## Before you begin

- A cluster you can `kubectl` into, and Helm.
- A model credential — a Claude subscription token or an Anthropic API key.
- **RWX storage, or one flag.** Sessions persist on a `ReadWriteMany` claim. No
  RWX provisioner? Add `--set persistence.enabled=false` below — otherwise the
  claim sits `Pending`, no runtime pod ever starts, and nothing says why.

## Install

```sh
kubectl create namespace agent-ops

kubectl -n agent-ops create secret generic agentops-claude \
  --from-literal=oauthToken=$(claude setup-token)   # or an Anthropic API key

helm install agent-ops ./chart -n agent-ops --create-namespace \
  --set global.demo.enabled=true \
  --set console.auth.uiToken=demo \
  --set 'k8s-bundle.pipelines.channels[0]=console'

kubectl -n agent-ops rollout status deploy/agentops-manager
```

`global.demo.enabled` installs [the k8s bundle](https://github.com/kostiantyn-matsebora/agent-ops-operator/blob/master/docs/k8s-bundle.md)
with its defaults. The other two make the console usable: a sign-in token you can
actually type, and the console bound as the channel the agent answers on. Pick a
real token for anything but a laptop — whoever holds it can instruct the agent.

Four objects matter here:

| | |
|---|---|
| `SignalSource/cluster-events` | where cluster events arrive |
| `AgentProfile/k8s-engineer` | who the agent is. Identity only, no tools |
| `Pipeline/k8s-observe` | the wiring: that source, that profile, a read-only toolset |
| `AgentRuntime/default` | the image it runs in, and the credential it uses |

One more step, and it is the whole idea of the product in one command. The
console can *show* conversations, but starting one means posting a signal from
its own source — and nothing claims that source yet. Let the route claim it:

```sh
kubectl -n agent-ops patch pipeline k8s-observe --type=json \
  -p '[{"op":"add","path":"/spec/signalSourceRefs/-","value":{"name":"console"}}]'
```

Without that, the console loads and its composer is unavailable, saying exactly
this.

## Ask it something

```sh
kubectl -n agent-ops port-forward svc/agentops-adapter-console 8080:8080
```

Open **<http://localhost:8080>**, sign in with `demo`, and press **New
conversation**. Ask it something about the cluster:

> How many nodes does this cluster have?

Your question is not a special API call — the console posts it as an ordinary
signal to its own source, and the Pipelines claiming that source decide who
answers ([contracts](https://github.com/kostiantyn-matsebora/agent-ops-operator/blob/master/docs/contracts.md)).
The answer comes back into the same thread, because the install bound the console
as a channel.

## What a good run looks like

Expect a pause before anything happens — a pod has to start and an image to
pull. The conversation appears in the console and moves through:

| | |
|---|---|
| `Working` | input admitted, runtime pod created |
| `Idle` | run finished, nothing queued |

Then the answer arrives in the thread. Ask a follow-up in the same thread and it
keeps its context.

The same thing from the outside, if you want to watch it land:

```sh
kubectl -n agent-ops get conversations -w
```

The name is generated — `chat-` or `task-` plus a suffix. Its runtime pod is
`agentops-conv-<name>`, which goes `1/1 Running` and **stays running after the
answer**, so a follow-up does not pay for a cold start; it exits on its own after
the runtime's idle TTL.

Live transcript:

```sh
kubectl -n agent-ops logs -f agentops-conv-<name>
```

```
[runtime] tools agent=- declared=0 wiring=24 mode=merge -> Read,Grep,Glob,Bash,mcp__kubernetes__…
[init] model=claude-sonnet-5 tools=55 mcp=kubernetes:connected
[tool] mcp__kubernetes__resources_list {"apiVersion":"v1","kind":"Node"}
[claude] 🤖 **5 nodes, all Ready**
=== RESULT (success, 3 turns, 8s) ===
```

The durable copy — read it after the pod is gone:

```sh
kubectl -n agent-ops get conversation <name> -o jsonpath='{.status.runs[0].result}'
```

## When nothing happens

| Cause | Where it shows |
|---|---|
| Bad or missing credential | `status.runs[].status: failed` with a non-zero exit code; reason in `kubectl logs` |
| No RWX storage class | `agentops-home` PVC `Pending`; conversation exists, never gets a pod |
| Nothing claims the source | the source's `Wired` condition is `False` with a reason; the console says its composer is unavailable, and signals are dropped rather than queued |
| At capacity | phase `Pending`, no pod, no thread. Five run at once by default |

## Where to go next

- **[Introduction]({{ '/introduction/' | relative_url }})** — the model behind
  what you just did.
- **[Concepts](https://github.com/kostiantyn-matsebora/agent-ops-operator/blob/master/docs/concepts.md)**
  — every CRD, and how tool access resolves.
- **A real lane** —
  [cluster events](https://github.com/kostiantyn-matsebora/agent-ops-operator/blob/master/docs/k8s-bundle.md),
  [Prometheus alerts](https://github.com/kostiantyn-matsebora/agent-ops-operator/blob/master/docs/prometheus-bundle.md),
  [Telegram](https://github.com/kostiantyn-matsebora/agent-ops-operator/blob/master/docs/telegram-bundle.md).
- **[The console](https://github.com/kostiantyn-matsebora/agent-ops-operator/blob/master/docs/console.md)**
  — the same conversations as a live graph, and a channel you can reply from.
- **[Contracts](https://github.com/kostiantyn-matsebora/agent-ops-operator/blob/master/docs/contracts.md)**
  — bring your own signal source, runtime or channel.
