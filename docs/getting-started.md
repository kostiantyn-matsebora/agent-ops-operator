---
title: Getting started
permalink: /getting-started/
description: >-
  Install the read-only demo, ask an agent about your cluster from the console,
  and see what a first run looks like. A first look, not a deployment.

next:
  eyebrow: Next
  title: The screen you are looking at
  body: >-
    A tour of the console's six views, what each one answers, and the one
    decision to make before you let anyone else reach it.
  url: /agent-ops-operator/console/
---

Install the operator and get your first question answered, in the console. About
fifteen minutes, most of it the install.

{: .ao-callout}
> **This is a demo, not a deployment**
>
> It shows you the product in fifteen minutes, and
> [Installation]({{ '/installation/' | relative_url }}) is the one to build on.
>
> **The agent is read-only and cannot change your cluster**
>
> Three walls, not one:
>
> - the MCP server runs `--read-only`
> - its identity is granted reads and nothing else
> - the route grants observing tools alone

## Before you begin

- A cluster you can `kubectl` into, and Helm.
- A model credential — a Claude subscription token or an Anthropic API key.

## Install

1. **Create the namespace and the model credential.**

   ```sh
   kubectl create namespace agent-ops

   kubectl -n agent-ops create secret generic agentops-claude \
     --from-literal=oauthToken=$(claude setup-token)   # or an Anthropic API key

   # ...or let the chart create it: --set claude.credentialsSecret.token=<token>
   ```

   ```powershell
   kubectl create namespace agent-ops

   kubectl -n agent-ops create secret generic agentops-claude `
     --from-literal=oauthToken=$(claude setup-token)   # or an Anthropic API key

   # ...or let the chart create it: --set claude.credentialsSecret.token=<token>
   ```

2. **Install the chart.** The flag brings up
   [the Kubernetes integration]({{ '/integrations/kubernetes/' | relative_url }}),
   wired to the console. The token is just to sign in — pick a real one outside
   a laptop.

   ```sh
   helm install agent-ops \
     oci://ghcr.io/kostiantyn-matsebora/charts/agent-ops-operator \
     -n agent-ops --create-namespace \
     --set global.demo.enabled=true \
     --set console.auth.uiToken=demo
   ```

   ```powershell
   helm install agent-ops `
     oci://ghcr.io/kostiantyn-matsebora/charts/agent-ops-operator `
     -n agent-ops --create-namespace `
     --set global.demo.enabled=true `
     --set console.auth.uiToken=demo
   ```

3. **Wait for the manager.**

   ```sh
   kubectl -n agent-ops rollout status deploy/agentops-manager
   ```

   ```powershell
   kubectl -n agent-ops rollout status deploy/agentops-manager
   ```

## Ask it something

1. **Forward the console's port.**

   ```sh
   kubectl -n agent-ops port-forward svc/agentops-adapter-console 8080:8080
   ```

   ```powershell
   kubectl -n agent-ops port-forward svc/agentops-adapter-console 8080:8080
   ```

2. **Open <http://localhost:8080>** and sign in with `demo`.

3. **Press New conversation** and ask:

   > How many nodes does this cluster have?

Your question is an ordinary signal, and the
[route]({{ '/guides/pipeline/' | relative_url }}) claiming the console decides
who answers.

## What you just installed

That answer came from four objects the chart created:

| Object | What it is |
|---|---|
| `SignalSource/console` | where your questions enter |
| `AgentProfile/k8s-engineer` | who the agent is. Behaviour only, no tools and no runtime |
| `Pipeline/k8s-observe` | the wiring: those sources, that profile, a read-only toolset |
| `AgentRuntime/default` | the image it runs in, and the credential it uses. It comes from the `claude` bundle, which is on by default |

The agent has no permissions of its own and reads your cluster through the
bundle's read-only MCP server.

## What a good run looks like

Expect a pause first — a pod has to start and an image to pull. Then, in the
console:

| Phase | Meaning |
|---|---|
| `Working` | input admitted, runtime pod created |
| `Idle` | run finished, nothing queued |

The answer lands in the thread. Follow-ups in the same thread keep their context.

**It opens with the conclusion**, then a few named sections, with the long
detail behind a control you can expand.

From the outside:

```sh
kubectl -n agent-ops get conversations -w              # names are generated
kubectl -n agent-ops logs -f agentops-conv-<name>      # live transcript
kubectl -n agent-ops get conversation <name> \
  -o jsonpath='{.status.runs[0].result}'               # the answer, durably
```

```powershell
kubectl -n agent-ops get conversations -w              # names are generated
kubectl -n agent-ops logs -f agentops-conv-<name>      # live transcript
kubectl -n agent-ops get conversation <name> `
  -o jsonpath='{.status.runs[0].result}'               # the answer, durably
```

The transcript names the tools the route granted, then every call:

```
[runtime] tools agent=- declared=0 wiring=24 mode=merge -> Read,Grep,Glob,Bash,mcp__kubernetes__…
[init] model=claude-sonnet-5 tools=55 mcp=kubernetes:connected
[tool] mcp__kubernetes__resources_list {"apiVersion":"v1","kind":"Node"}
[claude] 🤖 **5 nodes, all Ready**
=== RESULT (success, 3 turns, 8s) ===
```

The pod outlives the answer on purpose, so a follow-up skips the cold start. It
exits on the idle TTL.

## When nothing happens

| Cause | Where it shows |
|---|---|
| Bad or missing credential | the run fails — `status.runs[].status: failed`, non-zero exit code, reason in `kubectl logs` |
| No storage provisioner | the `agentops-context` PVC sits `Pending` and no pod is created. Reinstall with `--set persistence.context.enabled=false` |
| Nothing claims the source | the source's `Wired` condition is `False` with a reason. The console reports its composer unavailable, and signals are dropped |
| At capacity | phase `Pending`, no pod, no thread. Five run at once by default |

## Where to go next

- **[Put an agent to work]({{ '/guides/pipeline/' | relative_url }})**
  — the wiring, built from what you just installed. Nothing new to create, and
  the first guide of seven.
- **[Introduction]({{ '/introduction/' | relative_url }})** — the model behind
  what you just did, and the rest of the guides.
- **A real lane** —
  [cluster events]({{ '/integrations/kubernetes/' | relative_url }}),
  [Prometheus alerts]({{ '/integrations/prometheus/' | relative_url }}),
  [Telegram]({{ '/integrations/telegram/' | relative_url }}),
  [Home Assistant]({{ '/integrations/home-assistant/' | relative_url }}).
- **[Concepts](https://github.com/kostiantyn-matsebora/agent-ops-operator/blob/master/docs/concepts.md)**
  and **[the CR reference](https://github.com/kostiantyn-matsebora/agent-ops-operator/blob/master/docs/cr-reference.md)**
  — every kind, every field, and how tool access resolves.
