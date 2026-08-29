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

## Threat model

![Trust boundaries and the six flows that cross them. The agent container runs untrusted model output inside the runtime pod, and each crossing carries a control or is named as residual risk.]({{ '/assets/img/security/threat-model-light.svg' | relative_url }}){: .ao-diagram}

**Six flows cross a trust boundary.** Five carry a control. One does not.

| Crossing | Threat | Control |
|---|---|---|
| **1** any pod → operator ns | unauthenticated access to the work contract and every MCP tool | [network segmentation](#network-segmentation) — **off** |
| **2** agent → MCP server | calls a tool its route never bound | [egress control](#egress-control) — **on** |
| **3** agent → Kubernetes API | acts on the cluster beyond its route | [authorization](#cluster-authorization) — **on**, granting nothing |
| **4** agent → Secrets | reads any Secret by creating or entering a pod | [pod execution disabled](#secret-exposure) — **on** |
| **5** agent → context volume | reads another conversation's context | [no mount of the volume](#context-isolation) — **on** |
| **6** agent → pod logs | conversation content readable with `pods/log` | **none** — [residual risk](#residual-risk) |

**Nothing in the cluster is granted by default.** A route naming no identity runs
as an account bound to nothing. Acting power is something a route opts into, by
name.

> This page states the **posture**. [Installation]({{ '/installation/' | relative_url }})
> states the **keys** — every value, its default and its YAML, in one place, so
> nothing here can disagree with it.
{: .ao-callout}

---

## Defence in depth

Three independent controls, each bounding a different thing. **Closing one
leaves the next one open**, which is why they are not alternatives.

### Network segmentation

![Any pod in the cluster may reach the MCP servers and the work contract, because neither authenticates a caller, unless network policy segments them.]({{ '/assets/img/security/connect-light.svg' | relative_url }}){: .ao-diagram}

| | |
|---|---|
| **Threat** | crossing 1 — several components authenticate nobody, so any pod that reaches them can use them |
| **Control** | one NetworkPolicy per component, admitting only the callers your wiring implies |
| **Cost** | on a cluster that enforces policy, one missed flow breaks a working install |
| **Residual risk** | **a policy applies where nothing enforces it, and protects nothing.** No error, and the chart cannot tell the difference |

Two callers break quietly when you enable it: a metrics collector outside the
namespace, and your ingress controller in front of the console. Both are named
in [Who may reach what]({{ '/installation/' | relative_url }}#who-may-reach-what).

### Egress control

![An agent with a shell bypasses the command-line allowlist, but cannot bypass the egress proxy inside its own pod.]({{ '/assets/img/security/tools-light.svg' | relative_url }}){: .ao-diagram}

| | |
|---|---|
| **Threat** | crossing 2 — a route's toolsets reach the agent as a command-line allowlist. **That configures a cooperating agent**, and one with a shell calls a bound MCP server directly |
| **Control** | a proxy inside the runtime pod that the agent's traffic cannot route around |
| **Cost** | a **privileged init container**, plus a container per active conversation |
| **Residual risk** | it covers neither MCP servers run as the agent's own child processes, nor `https` endpoints, nor **tool arguments** |

**It is on by default.** A control that constrains an agent which does not
cooperate should not be something you have to discover.

**Its cost is a hard failure, not a slowdown.** A namespace under `restricted`
Pod Security admission refuses that pod — at admission, when a conversation
starts, not when you render the chart.

Neither uncovered path is passed off as enforced. The conversation's
`EgressMediated` condition names what is not covered.

Reasoning and rejected alternatives in
[ADR 0001](https://github.com/kostiantyn-matsebora/agent-ops-operator/blob/master/docs/adr/0001-bound-component-reach.md).
Keys in
[Enforcing the toolset]({{ '/installation/' | relative_url }}#enforcing-the-toolset).

### Cluster authorization

An allowlist configures a cooperating agent. **A ServiceAccount binding is what
an uncooperative one with a shell actually has.**

| | |
|---|---|
| **Threat** | crossing 3 — the agent acts on the cluster beyond what its route intended |
| **Control** | least privilege: a route naming no identity is bound to nothing, and more is an account you declare, stating its own rules |
| **Grants** | roles the chart writes out, never a built-in role. No `cluster-admin`, no `view` |
| **Residual risk** | what you declare as cluster roles is **cluster-wide**, the operator's own namespace included |

`agentops.dev` is never granted, so Conversations and Pipelines are unreadable to
an agent everywhere. That omission is what protects them, rather than scope.

#### Secret exposure

**No `secrets` verb is not the same as cannot read Secrets.**

![An agent allowed to create or enter a pod reads a Secret through the kubelet, without ever asking the API server for it.]({{ '/assets/img/security/secrets-light.svg' | relative_url }}){: .ao-diagram}

The kubelet resolves a Secret when it builds a pod. The verb is never evaluated.

**So pod execution is the Secrets boundary, and it is disabled by default.** It
gates every write that produces or enters a pod, not one verb: a Job, a
Deployment and a patch to a pod template are the same path.

Keys and worked examples in
[The agent's power]({{ '/installation/' | relative_url }}#the-agents-power).

---

## The platform's own posture

Properties of the product you cannot read off the values.

| Property | What holds |
|---|---|
| **The manager reads no Secrets** | it holds no verb on `secrets` at all. Everything secret-shaped compiles to a pod-spec reference the kubelet resolves |
| **Adapter credentials are not stored** | a per-adapter token is an HMAC of a master key and the adapter's name, validated by re-deriving it. Nothing is minted, stored, or read back |
| **Runtime pods are non-root** | at a fixed uid, and each conversation's workspace is its own subpath of the claim, so concurrent pods cannot see each other's tree |
| **Under egress control the container is hardened further** | all capabilities dropped, privilege escalation refused |

Every published image carries an **SBOM** and **max-mode provenance**, recorded
by the build. **Nothing is signed, and the chart is not attested.**

**Every image is scanned for known vulnerabilities**, twice over. On the pull
request that builds it, a CRITICAL or HIGH finding **with a fix available**
blocks the merge. Weekly, the published images are scanned again against a
newer database.

Both scans report to the repository's security tab, and both fail on the same
fixable finding. The pull-request scan blocks the merge. The weekly scan
reports before it fails, and turns the README's published-images badge red
until the image is re-released.

On a pull request from a fork the gate blocks without reporting — a fork's
token cannot write there.

Security hotspots in the code itself — as opposed to vulnerabilities in what
an image ships — are reported by SonarCloud, one project per component, on
that service's own dashboard rather than the security tab. They are reported
and not gated: a hotspot is a place to review, not a finding, and nothing in
this project's merge protection reads the verdict.

**A finding with no available fix does not block**, knowingly. An unfixable
upstream vulnerability is information rather than a task, and a gate that
cannot be made green is one that gets switched off.

The SBOM is the complete inventory. The scan is the actionable part of it.

Read what an image carries:

{% raw %}
```sh
docker buildx imagetools inspect \
  ghcr.io/kostiantyn-matsebora/agentops-manager:<tag> \
  --format '{{ json .Provenance }}'
```

```powershell
docker buildx imagetools inspect `
  ghcr.io/kostiantyn-matsebora/agentops-manager:<tag> `
  --format '{{ json .Provenance }}'
```
{% endraw %}

### Context isolation

A conversation's accumulated context is everything an agent was told and
everything it produced.

![Under context sync the agent container holds no mount of the durable volume, and a sidecar snapshots to it instead.]({{ '/assets/img/security/context-light.svg' | relative_url }}){: .ao-diagram}

**A default install isolates it.** The reference runtime ships the context paths
its own backend uses, so nothing has to be turned on.

**The isolation is structural, not permissive.** The agent container holds no
mount of the durable volume, so it cannot read another conversation's context
because there is nothing to read from. Durability is not the cost — a sidecar
holds the volume and snapshots to it.

**Where it does not hold.** The mode needs three things: a runtime that declares
its context paths, a sidecar image, and a durable context volume.

Short any of the three, the pod is the unsynchronised one. With a durable volume
and a runtime declaring no paths — what another vendor's runtime gets until its
entry states them — **the whole volume is mounted into the agent container**.

### Logging

**This does not hold uniformly, so it is stated per component.**

| Component | Logs message content |
|---|---|
| the manager, both adapter kinds, the console | **no** — identifiers, counts, operation ids and errors only |
| the runtime | **yes** — the agent's output, its tool-call arguments and its result, to the pod's stdout |

That is crossing 6. Conversation content is readable by anyone holding
`pods/log` in the operator's namespace.

---

## Residual risk

**Read this section.** A security page listing only what is handled is read as a
claim that the rest is handled too.

| Open | What it means |
|---|---|
| **Unauthenticated surfaces** | the MCP servers accept any caller and the manager's work contract takes no credential. Network segmentation is the only thing that bounds who reaches them, and it is off by default |
| **A control this chart cannot verify** | a NetworkPolicy applies where nothing enforces it, silently. The install output says how to check, because nothing else can |
| **Two paths past egress control** | MCP servers run as the agent's own child processes, and `https` endpoints |
| **Tool arguments** | egress control decides which tool may be called, never with what arguments |
| **Context isolation a runtime does not get** | a runtime declaring no context paths mounts the whole shared context volume into its agent container |
| **Conversation content in pod logs** | the runtime writes what the agent produced to its pod's stdout |
| **Signing and attestation** | no image is signed, and the chart carries no attestation |

**None of this is a surprise to the project.** These are decisions with reasons,
and the reasons are in
[ADR 0001](https://github.com/kostiantyn-matsebora/agent-ops-operator/blob/master/docs/adr/0001-bound-component-reach.md)
and in the values documentation. An adopter who cannot see a gap cannot
compensate for it.

## Reporting

**Report privately, never in a public issue.**
[SECURITY.md](https://github.com/kostiantyn-matsebora/agent-ops-operator/blob/master/SECURITY.md)
carries the private advisory link, what to include, and the response targets.

**A chart default that grants more than it needs is a finding**, not a
configuration choice.
