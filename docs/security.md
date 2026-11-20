---
title: Security
permalink: /security/
description: >-
  The threat model for running model output inside your cluster — the trust
  boundaries, what crosses them, the control on each crossing, and the residual
  risk.

next:
  eyebrow: Next
  title: Install it for real
  body: >-
    The decisions to make before a real install, the values that matter, how to
    enable a bundle, and the one route without which nothing answers.
  url: /agent-ops-operator/installation/
---

agent-ops runs a language model's output inside your cluster, using credentials
you gave it. **Treat the agent container as untrusted code with a shell.**

**Nothing in the cluster is granted by default.** A route naming no identity runs
as an account bound to nothing. Acting power is something a route opts into, by
name.

## Threat model

![Trust boundaries and the seven flows that cross them. The agent container runs untrusted model output inside the runtime pod, and each crossing carries a control or is named as residual risk.]({{ '/assets/img/security/threat-model-light.svg' | relative_url }}){: .ao-diagram}

Seven flows cross a trust boundary. Six carry a control. One does not.

| # | Crossing | Control | Catch |
|---|---|---|---|
| 1 | any pod → operator namespace | NetworkPolicy per component ([who may reach what](https://github.com/kostiantyn-matsebora/agent-ops-operator/blob/master/docs/configuration.md#who-may-reach-what)) | applies silently where no CNI enforces it |
| 2 | agent → MCP server | proxy inside the runtime pod ([enforcing the toolset](https://github.com/kostiantyn-matsebora/agent-ops-operator/blob/master/docs/configuration.md#enforcing-the-toolset)) | needs a privileged init container. Misses child-process MCP servers, `https` and tool arguments |
| 3 | agent → Kubernetes API | least privilege: a route naming no identity is bound to nothing ([the agent's power](https://github.com/kostiantyn-matsebora/agent-ops-operator/blob/master/docs/configuration.md#the-agents-power)) | roles you declare at cluster scope are cluster-wide |
| 4 | agent → Secrets | pod execution disabled | see below |
| 5 | agent → context volume | agent container has no mount of it, a sidecar snapshots | needs a runtime that declares context paths, or the whole volume is mounted |
| 6 | agent → pod logs | none | anyone with `pods/log` reads conversations |
| 7 | any pod → state or bus server | chart-managed credential, plus network policy. Default: credential on, policy **off** | one shared credential |

"Operator namespace" is the control namespace. A release may run runtime pods in a second, conversations namespace ([Concepts]({{ '/concepts/#where-things-run-the-control-and-conversations-namespaces' | relative_url }})). Every crossing is the same either way: it is deployment topology, not a trust boundary.

**No `secrets` verb is not the same as cannot read Secrets.** The kubelet
resolves a Secret when it builds a pod, so an agent that may create or enter a
pod reads any Secret without asking the API server. Pod execution is the
boundary, and it is off by default.

**Coordination adds no new crossing.** The manager enforces every bound on
invoked agents (`agents[]`, `maxAgents`, `maxTurns`, `deadline`, no cycles), and
each member runs under its own identity, never the caller's.

Reasoning and rejected alternatives: [ADR 0001](https://github.com/kostiantyn-matsebora/agent-ops-operator/blob/master/docs/adr/0001-bound-component-reach.md).

## What the platform guarantees

- **The manager reads no Secrets** — it holds no verb on `secrets`; secret-shaped values compile to pod-spec references the kubelet resolves.
- **Credentials have one holder each.** Only `agentops-gateway-telegram` holds the bot token. Adapter tokens are an HMAC of a master key and the adapter's name, validated by re-deriving it, never stored.
- **Surfaces do not share credentials.** `/control-plane/*` has its own token and refuses everything when unconfigured. `/state/*` is opened per adapter by `spec.stateAccess`, off by default.
- **Runtime pods are non-root**, each conversation's workspace is its own subpath, and under egress control all capabilities are dropped.
- **The events adapter** has its own identity with `list`/`watch` on events, pods, replicasets and (cluster-wide only) nodes. No agent runs as it and it holds no `secrets` verb.
- **`stateReadAuth` has a second copy** in the conversations namespace, beside the claims housekeeping mounts. It grants read on `/state/*` only, and reading it needs pod execution, which is off by default.
- **Logs:** only the runtime logs message content — crossing 6. Everything else logs identifiers and errors.
- **Images** carry an SBOM and provenance and are scanned on every pull request and weekly. **Nothing is signed, and the chart is not attested.**

## Also open

Beyond the Catch column above:

- **Repository hooks under `pi.piCode`** (on by default). A hook in the checked-out repository's `.claude/settings.json` runs as a shell command in the pod, outside the tool allowlist. `pi.piCode: false` removes it.
- **No signing or attestation.** No image is signed, and the chart is not attested.
- **Coordination depth.** Nothing bounds the number of levels, only a repeated Coordinator is refused.
- **Consent is a prompt, not a gate.** An invoked member is told not to act past its task. The manager enforces who may be invoked, never what it does.

## Reporting

**Report privately, never in a public issue.**
[SECURITY.md](https://github.com/kostiantyn-matsebora/agent-ops-operator/blob/master/SECURITY.md)
has the advisory link and response targets. A chart default that grants more than
it needs is a finding, not a configuration choice.
