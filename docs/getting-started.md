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
    A tour of the console's five views, what each one answers, and the one
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
- An Anthropic API key.

## Install

One command. The token is your model credential; the console token (`demo`) is
just to sign in — pick a real one outside a laptop.

```sh
helm install agent-ops \
  oci://ghcr.io/kostiantyn-matsebora/charts/agent-ops-operator \
  -n agent-ops --create-namespace \
  --set global.demo.enabled=true \
  --set console.auth.uiToken=demo \
  --set pi.credentialsSecret.token=<token>   # an Anthropic API key
```

```powershell
helm install agent-ops `
  oci://ghcr.io/kostiantyn-matsebora/charts/agent-ops-operator `
  -n agent-ops --create-namespace `
  --set global.demo.enabled=true `
  --set console.auth.uiToken=demo `
  --set pi.credentialsSecret.token=<token>   # an Anthropic API key
```

The flag brings up
[the Kubernetes integration]({{ '/integrations/kubernetes/' | relative_url }}),
wired to the console.

## Ask it something

1. **Forward the console's port.** If it refuses, the manager is still starting
   — retry in a few seconds.

   ```sh
   kubectl -n agent-ops port-forward svc/agentops-adapter-console 8080:8080
   ```

   ```powershell
   kubectl -n agent-ops port-forward svc/agentops-adapter-console 8080:8080
   ```

2. **Open <http://localhost:8080>** and sign in with `demo`.

3. **Press New conversation** and address a route by name:

   > /agentops-coordinator How many nodes does this cluster have?

   Demo mode wires two routes to the console — a `Coordinator` and a plain
   `Pipeline` (`k8s-observe-pipeline`) — so an unaddressed message shows a
   choice list instead of answering. Try the other too:

   > /k8s-observe-pipeline How many nodes does this cluster have?

   Both answer from the same read-only toolset. See
   [Coordinate agents]({{ '/guides/coordinate-agents/' | relative_url }}) for
   what the `Coordinator` adds.

Expect a pause first — a pod has to start and an image to pull. Follow-ups in the
same thread keep their context.

## When nothing happens

| Cause | Where it shows |
|---|---|
| Bad or missing credential | the run fails — reason in `kubectl -n agent-ops-conversations logs agentops-conv-<name>` |
| No storage provisioner | the `agentops-context` PVC in `agent-ops-conversations` sits `Pending` and no pod is created. Reinstall with `--set persistence.context.enabled=false` |
| Nothing claims the source | the source's `Wired` condition is `False` with a reason, and signals are dropped |
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
  and **the [per-kind spec pages]({{ '/introduction/' | relative_url }})
  in the sidebar's CRDs group**
  — every kind, every field, and how tool access resolves.
