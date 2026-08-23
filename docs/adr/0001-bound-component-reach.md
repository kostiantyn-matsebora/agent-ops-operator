# ADR 0001 — Bound component reach: at the network, and inside the runtime pod

- **Status**: Accepted
- **Date**: 2026-08-21

## Context

agent-ops is a set of components that talk to each other over the pod network —
the manager, the adapters, the console, the MCP servers, and a runtime pod per
conversation. Each authenticates its callers, or does not, on its own terms.

Underneath all of them is one shared assumption: **that a caller reaching this
socket is a component we wired.** Nothing enforces it. The chart ships no network
restriction of any kind, so the trust boundary is the pod network, which is the
whole cluster.

Verified live on 2026-08-21, from a pod in another namespace with no credential:

| Surface | Requires a credential | What it grants a stranger |
|---|---|---|
| `agentops-mcp-k8s` | No | cluster-admin, including `pods_exec` and `resources_delete` |
| `agentops-mcp-ha` | No | Home Assistant control, under an operator token |
| Manager `/work`, `/work/done` | No | Steal a queued work unit, or forge an agent's answer |
| Manager `/channel/*`, `/signal/*` | **Yes** — bearer | — |
| Console API | No | Read every conversation, and send messages to agents |
| Telegram adapters `/updates` | No | Inject chat messages, originate conversations as any sender |

The agent is inside this boundary too, and is the one caller we already intend to
restrict. Its reach is meant to be the toolsets its `Pipeline` binds. But
`--allowedTools` is enforced by the CLI **inside the runtime pod**, and the MCP
server has never heard of an `MCPToolset`. Agents here run untrusted input by
design, and ordinary pipelines bind a shell.

## Problem

**Component-to-component communication has no trust boundary, and the one caller
we do restrict is restricted only by its own cooperation.**

That is two failures, not one, and they need different answers.

**P1 — Who may connect is unbounded.** Every component above assumes wired
callers. Any pod in the cluster can act as one.

**P2 — What an allowed caller may do is unbounded, for the agent.** Even with P1
closed, the runtime pod is *legitimately* allowed to reach the MCP servers. The
toolset that is supposed to narrow that is advisory.

## Options for P1

- **Network restriction** — allow only wired callers to reach each component.
  One control, every component, no per-component code.
- **Authenticate every endpoint** — the stronger answer, since it binds identity
  rather than location. It is per-component work, and each unauthenticated
  surface above is its own change.
- **Service mesh mTLS** — strongest, and assumes infrastructure this chart
  cannot require of adopters.

## Options for P2

- **A. In-pod proxy** — interpose a proxy in the runtime pod that the agent's
  traffic cannot route around, and enforce the toolset there.
- **B. Shared mediator** — one component all agents call for MCP. Only it may
  reach the servers.
- **C. Per-server authentication** — each MCP server requires a credential.
- **D. Network restriction** — as P1, applied to the MCP servers.
- **E. Server RBAC only** — accept the toolset as advisory, bound reach by what
  the server's ServiceAccount may do.

## Trade-off analysis

Criteria come from the problem statement, plus the stated intent to govern all
agent egress eventually, not only MCP.

| | A. In-pod proxy | B. Shared mediator | C. Server auth | D. Network | E. RBAC only |
|---|---|---|---|---|---|
| **Binds the agent** | Yes | Only if it cooperates | No — it holds the token | No — its path is the allowed one | No |
| **Holds on any cluster** | Yes | Yes | Needs an OIDC provider | No — CNI-dependent, silently | Yes |
| **Independent of server image** | Yes | Yes | No — one answer per vendor | Yes | Yes |
| **Extends to all agent egress** | Yes | No | No | Partly, and coarsely | No |
| **Strong credential out of the agent's pod** | **No** | Yes | Yes | Yes | Yes |
| **Cross-conversation isolation** | Structural | By code | n/a | n/a | None |
| **Cost** | Per conversation | Per install | Per server | Per install | None |
| **New privilege introduced** | **Yes, at pod start** | No | No | No | No |

Two rows decide it. **Binds the agent** eliminates C, D and E outright — they
govern who may connect, not what the connected agent may ask for. Between A and
B, **extends to all agent egress** is decisive: a mediator is a destination the
agent chooses to use, so it can only ever govern traffic the agent volunteers.

A is also the only option that loses rows. It introduces privilege in the
sensitive pod, and it puts a credential stronger than the agent's own rights
inside the pod the agent runs in.

## Decisions

**D1 — For P1: restrict at the network which pods may reach each agent-ops
component.** Chosen over per-endpoint authentication as the *first* control
because it covers every component at once, including the third-party MCP server
images we do not control. Authentication is the better long-term answer and stays
owed, per component.

**D2 — For P2: enforce the toolset in the runtime pod, by interposing a proxy the
agent cannot route around.** Option A, accepting both costs it carries.

The governing principle for D2: *a component the agent must address is a
component it can decline to address.* Enforcement has to sit in the agent's own
path rather than at the end of it.

The two are complementary, not redundant. D1 bounds who may connect. D2 bounds a
caller that is allowed to.

## Consequences

- The toolset becomes a real bound. A shell no longer defeats it, and that
  guarantee holds on any cluster.
- The runtime pod gains privilege at start, and its non-root identity stops being
  incidental and becomes load-bearing.
- The pod holds a credential stronger than the agent's own rights, so container
  isolation now carries weight it did not carry before.
- D2's cost scales per conversation rather than per install.
- **D1's enforcement cannot be verified from inside the chart.** On a cluster
  whose CNI ignores policy, the objects apply cleanly and protect nothing. An
  install must be told this rather than reassured by the objects existing.
- D1 restricts reach, it does not authenticate. Every unauthenticated surface
  above stays unauthenticated to whoever is still allowed to reach it.
- Tool **arguments** stay ungoverned. D2 decides who may call what, not what they
  may ask for.

## What implementation changed

Two things the analysis got wrong, recorded because a later reader will
otherwise wonder whether they were considered.

**There was no matcher to be in parity with.** The design assumed the manager
interpreted toolset patterns and that the proxy would have to agree with it. The
manager does not: it concatenates and dedups, and the patterns stay opaque to it.
The proxy is the first component here that reads one. Opacity moved rather than
disappearing, and the spec says so.

**Detecting an unenforceable endpoint needs no report from the pod.** The
manager compiled the endpoints, so it already knows their transports. The
condition is written from what it holds, and the proxy sends nothing back — one
fewer channel, and no credential to carry it.

Three more the LIVE cluster taught, each invisible to the test suite.

**An address cannot identify the destination.** Where the CNI does socket-level
load balancing — Cilium's kube-proxy replacement — the destination is rewritten
inside `connect()`, before netfilter sees it, so what the kernel records is a
BACKEND POD IP that no Service name resolves to. Matching by address alone
matched nothing, every tool call fell through to opaque forwarding, and the pod,
the condition and the logs all still reported mediation as active. The HTTP Host
is what the client wrote, so that is what attributes a connection now.

**A probe is not a pod, and a policy cannot name it.** The kubelet probes from
the node. Cilium's default CIDR match mode does not match node addresses, so
even an explicit `ipBlock` of the node network does not authorise it. The
manager survives because its probe port serves nothing else and can be opened.
The Kubernetes MCP server probes the SAME port it serves MCP on, so it cannot —
it now probes itself over loopback, which no policy on any CNI can reach.

**Restricting reach broke the thing it protected, twice.** Both failures were
the policy, not the workload: a blocked probe crash-looped the manager, and the
restarts surfaced elsewhere as `EPERM` on adapter connections. Neither is a
reason to abandon the control. Both are the reason the post-install output tells
an operator to verify rather than assume.

## Not decided here

Authentication for the surfaces that lack it — the manager's work contract, the
console's model, the adapters' inbound push — and the MCP server's cluster-admin
binding. D1 narrows the exposure of all of them. It resolves none.

> **Update.** The `cluster-admin` binding named above is GONE, in both walls.
> `rbacMode: full` now renders an enumerated acting ClusterRole for the runtime
> account and for `agentops-mcp-k8s` alike, and no role either holds carries a
> verb on `secrets`. The rest of this section still stands: those surfaces still
> authenticate nobody, so an unauthenticated caller reaching the MCP server now
> gets that role instead of everything.
