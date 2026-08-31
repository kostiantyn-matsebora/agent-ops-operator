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

1. **Create the namespace and the model credential.**

   ```sh
   kubectl create namespace agent-ops

   kubectl -n agent-ops create secret generic agentops-claude \
     --from-literal=oauthToken=$(claude setup-token)   # or an Anthropic API key
   ```

   ```powershell
   kubectl create namespace agent-ops

   kubectl -n agent-ops create secret generic agentops-claude `
     --from-literal=oauthToken=$(claude setup-token)   # or an Anthropic API key
   ```

2. **Install the chart** from the registry. There is no repo to add and no
   checkout to clone.

   **Storage is the only setting you cannot change afterwards.** The default
   needs a ReadWriteMany provisioner, so without one add this flag:

   ```text
   --set persistence.context.accessModes[0]=ReadWriteOnce
   ```

   ```sh
   helm install agent-ops \
     oci://ghcr.io/kostiantyn-matsebora/charts/agent-ops-operator \
     --version 13.3.0 -n agent-ops
   ```

   ```powershell
   helm install agent-ops `
     oci://ghcr.io/kostiantyn-matsebora/charts/agent-ops-operator `
     --version 13.3.0 -n agent-ops
   ```

   **No registry credential.** The chart and every image it renders are public
   packages on GHCR, so the pull is anonymous and the install needs no
   `imagePullSecrets`.

   Working from a checkout instead? `helm install agent-ops ./chart -n
   agent-ops` still does the same thing.

3. **Verify.**

   ```sh
   kubectl -n agent-ops rollout status deploy/agentops-manager
   kubectl -n agent-ops get agentruntime default
   ```

   ```powershell
   kubectl -n agent-ops rollout status deploy/agentops-manager
   kubectl -n agent-ops get agentruntime default
   ```

That brings up the manager, `AgentRuntime/default` and the console — and **no
routes**, so nothing answers yet. The next two sections fix that.

## Wire your first route

A source no Ready `Pipeline` claims **drops every signal it admits**. A fresh
install has exactly that, so nothing answers until you declare a route.

The smallest real one names a source, a profile and the tools that profile may
use:

```yaml
apiVersion: agentops.dev/v1alpha1
kind: Pipeline
metadata:
  name: cluster-triage
  namespace: agent-ops
spec:
  profileRef:
    name: k8s-engineer
  signalSourceRefs:
    - name: cluster-events
  toolsets:
    refs:
      - name: agentops-observe
      - name: k8s-observability
  mcpConfigs:
    refs:
      - name: k8s-api
```

This object carries everything, because the profile holds no tools and no
permissions of its own.

Two optional fields decide how the route runs:

| Field | Omitted, it is |
|---|---|
| `runtimeRef` | the runtime named `default` |
| `serviceAccountName` | that runtime's own account |

Every field is in
[concepts](https://github.com/kostiantyn-matsebora/agent-ops-operator/blob/master/docs/concepts.md).

## Enable a bundle

A bundle contributes domain — sources, profiles, tooling, channels. The
substrate they run on comes from this chart.

| Bundle | Set | Its values |
|---|---|---|
| Kubernetes events | `kubernetes.enabled` | [kubernetes]({{ '/integrations/kubernetes/' | relative_url }}) |
| Prometheus alerts | `prometheus.enabled` | [prometheus]({{ '/integrations/prometheus/' | relative_url }}) |
| Telegram | `telegram.enabled` | [telegram]({{ '/integrations/telegram/' | relative_url }}) |
| Home Assistant | `home-assistant.enabled` | [home-assistant]({{ '/integrations/home-assistant/' | relative_url }}) |
| Ollama runtime | `ollama.enabled` | [ollama]({{ '/runtimes/ollama/' | relative_url }}) |
| GitHub Copilot runtime | `copilot.enabled` | [copilot]({{ '/runtimes/copilot/' | relative_url }}) |

All six are off by default. Each bundle's own page owns its values — this page
does not repeat them. The last two are RUNTIMES rather than integrations: they
start no work and answer nowhere, they execute.

## Configure

**Storage is the only setting that must be right before you install.**
Everything else is a `helm upgrade` away and safe at its default, and every
value the chart takes is in the
[configuration reference](https://github.com/kostiantyn-matsebora/agent-ops-operator/blob/master/docs/configuration.md).

### Storage

**Context** keeps what conversations remember. **Workspace** keeps repository
checkouts, and is off by default because agents re-clone instead.

| Key | Default | Consequence |
|---|---|---|
| `persistence.context.accessModes` | `[]` — the chart picks | `ReadWriteMany`, or `ReadWriteOnce` in demo mode where `local-path` refuses RWX. Set it yourself and your value wins |
| `persistence.context.storageClassName` | `""` | empty uses the cluster's default provisioner. To bind a volume you made yourself, see [pointing a volume at existing storage](https://github.com/kostiantyn-matsebora/agent-ops-operator/blob/master/docs/configuration.md#pointing-a-volume-at-storage-the-chart-did-not-create) |

Set the access mode before you install. A claim cannot be edited once it
exists, so changing your mind means deleting it and starting again.

- **`ReadWriteMany`** is the default. Agent pods can run on any node.
- **`ReadWriteOnce`** works too, and pins every agent pod to one node.

Whether context persists at all, its size, and whether workspace persists too
are all reversible — see the [configuration reference](https://github.com/kostiantyn-matsebora/agent-ops-operator/blob/master/docs/configuration.md#storage).

Every route uses these volumes unless it says otherwise. A route can keep its
state somewhere of its own:
[per-route storage](https://github.com/kostiantyn-matsebora/agent-ops-operator/blob/master/docs/configuration.md#per-route-storage).

## Upgrade and uninstall

```sh
helm upgrade agent-ops \
  oci://ghcr.io/kostiantyn-matsebora/charts/agent-ops-operator \
  --version <version> -n agent-ops
```

```powershell
helm upgrade agent-ops `
  oci://ghcr.io/kostiantyn-matsebora/charts/agent-ops-operator `
  --version <version> -n agent-ops
```

Read [the changelog](https://github.com/kostiantyn-matsebora/agent-ops-operator/blob/master/docs/CHANGELOG.md)
first, since it is the only place migration steps live, keyed by chart version
and newest first.

`helm uninstall` removes the workloads but leaves the CRDs, every Conversation
and the context claim, so reinstalling finds your data where it was.

Helm never deletes CRDs that came from a chart's `crds/` directory, so this is
not something the chart decides. Removing them means deleting them yourself,
which takes every Conversation with them:

```sh
kubectl delete crd $(kubectl get crd -o name | grep '\.agentops\.dev$')
```

```powershell
kubectl get crd -o name | Select-String '\.agentops\.dev$' |
  ForEach-Object { kubectl delete $_.ToString() }
```
