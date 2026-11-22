---
title: Installation
permalink: /installation/
description: >-
  Install agent-ops on a live cluster: the commands, your first route, the
  bundles, and the one setting that must be right before you start.

next:
  eyebrow: Next
  title: "Put an agent to work"
  body: >-
    The only object that carries any wiring — what starts a conversation, which
    agent answers, and what it may touch. Built from what you already installed.
  url: /agent-ops-operator/guides/pipeline/
---

This page installs agent-ops on a live cluster. If you only want to see it
work, [Getting started]({{ '/getting-started/' | relative_url }}) puts a
read-only demo on your cluster in fifteen minutes.

## Install

1. **Create the namespace.**

   ```sh
   kubectl create namespace agent-ops
   ```

   ```powershell
   kubectl create namespace agent-ops
   ```

2. **Install the chart** from the registry — no repo to add, no checkout, no
   pull credential. Read [Namespaces](#namespaces) and [Storage](#storage)
   first: they are the settings you cannot change afterwards.

   ```sh
   helm install agent-ops \
     oci://ghcr.io/kostiantyn-matsebora/charts/agent-ops-operator \
     --version 25.0.0 -n agent-ops
   ```

   ```powershell
   helm install agent-ops `
     oci://ghcr.io/kostiantyn-matsebora/charts/agent-ops-operator `
     --version 25.0.0 -n agent-ops
   ```

3. **Create the model credential.** The chart has created the conversations
   namespace, where runtime pods run. The default runtime is
   [pi]({{ '/runtimes/pi/' | relative_url }}), on Anthropic. (Or pass
   `--set pi.credentialsSecret.token=<key>` in step 2.)

   ```sh
   kubectl -n agent-ops-conversations create secret generic agentops-pi \
     --from-literal=apiKey=$ANTHROPIC_API_KEY
   ```

   ```powershell
   kubectl -n agent-ops-conversations create secret generic agentops-pi `
     --from-literal=apiKey=$env:ANTHROPIC_API_KEY
   ```

4. **Verify.**

   ```sh
   kubectl -n agent-ops rollout status deploy/agentops-manager
   ```

   ```powershell
   kubectl -n agent-ops rollout status deploy/agentops-manager
   ```

That brings up the manager, the default runtime and the console — and **no
routes**, so nothing answers yet. Enabling a bundle is the quickest way to get
one.

## Enable a bundle

A bundle contributes sources, profiles, tooling or channels. The Kubernetes and
Prometheus bundles also ship a ready-made route, so turning one on is enough
to get answers. All are off by default, and each page owns its values.

| Bundle | Set |
|---|---|
| [Kubernetes events]({{ '/integrations/kubernetes/' | relative_url }}) | `kubernetes.enabled` |
| [Prometheus alerts]({{ '/integrations/prometheus/' | relative_url }}) | `prometheus.enabled` |
| [Telegram]({{ '/integrations/telegram/' | relative_url }}) | `telegram.enabled` |
| [Home Assistant]({{ '/integrations/home-assistant/' | relative_url }}) | `home-assistant.enabled` |
| [Claude CLI runtime]({{ '/runtimes/claude-cli/' | relative_url }}) | `claude-cli.enabled` |
| [Ollama runtime]({{ '/runtimes/ollama/' | relative_url }}) | `ollama.enabled` |
| [GitHub Copilot runtime]({{ '/runtimes/copilot/' | relative_url }}) | `copilot.enabled` |
| [Coordination]({{ '/guides/coordinate-agents/' | relative_url }}) | `coordination.enabled` |

Telegram is a channel and ships no route, so bind it to a bundle's route
(`kubernetes.pipelines.channels`). Want to own the wiring instead? A source no
Ready `Pipeline` claims drops every signal, so
[Put an agent to work]({{ '/guides/pipeline/' | relative_url }}) declares your
own.

## Namespaces

Runtime pods, their volume claims and accounts run in a second namespace,
`global.agentops.conversationNamespace` (default `agent-ops-conversations`,
created by the chart).
Set `createConversationNamespace: false` to create it yourself. Set it to the release namespace for a single-namespace layout.
Changing it later is a manual migration, so pick it before you install — see
[Concepts]({{ '/concepts/#where-things-run-the-control-and-conversations-namespaces' | relative_url }}).

## Storage

A volume claim cannot be edited once it exists, so set the access mode before
you install. Everything else is a `helm upgrade` away — every value is in the
[configuration reference](https://github.com/kostiantyn-matsebora/agent-ops-operator/blob/master/docs/configuration.md).

- **`ReadWriteMany`** is the default. Agent pods can run on any node.
- **`ReadWriteOnce`** works too, and pins every agent pod to one node. Without
  an RWX provisioner, add `--set persistence.context.accessModes[0]=ReadWriteOnce`.

The state servers need at least three pods (`nats.replicas`). Only the demo runs one. Two more keys are fixed once their claims exist: `persistence.context.storageClassName`
and `nats.persistence.storageClassName`. Empty uses the cluster's default
provisioner.

## Upgrade and uninstall

**Apply the CRDs yourself, before every upgrade.** Helm never upgrades CRDs from
a chart's `crds/` directory, so an upgraded install keeps the old shape until you do.

```sh
helm pull oci://ghcr.io/kostiantyn-matsebora/charts/agent-ops-operator \
  --version <version> --untar --untardir /tmp/agentops-chart
kubectl apply -f /tmp/agentops-chart/agent-ops-operator/crds/

helm upgrade agent-ops \
  oci://ghcr.io/kostiantyn-matsebora/charts/agent-ops-operator \
  --version <version> -n agent-ops
```

```powershell
helm pull oci://ghcr.io/kostiantyn-matsebora/charts/agent-ops-operator `
  --version <version> --untar --untardir $env:TEMP\agentops-chart
kubectl apply -f $env:TEMP\agentops-chart\agent-ops-operator\crds\

helm upgrade agent-ops `
  oci://ghcr.io/kostiantyn-matsebora/charts/agent-ops-operator `
  --version <version> -n agent-ops
```

Read [the changelog](https://github.com/kostiantyn-matsebora/agent-ops-operator/blob/master/docs/CHANGELOG.md)
first — it is the only place migration steps live, newest first.

`helm uninstall` removes the workloads and leaves the CRDs, the context claim and
the state servers' volumes, so a reinstall finds your data where it was. To
remove the CRDs too (which deletes every Conversation custom resource on the
`kube` backend):

```sh
kubectl delete crd $(kubectl get crd -o name | grep '\.agentops\.dev$')
```

```powershell
kubectl get crd -o name | Select-String '\.agentops\.dev$' |
  ForEach-Object { kubectl delete $_.ToString() }
```
