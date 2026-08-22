---
title: The console
permalink: /console/
description: >-
  A browser view of the whole install that is also a channel and a signal
  source. What each view answers, and how to decide who may reach it.

next:
  eyebrow: Next
  title: Install it for real
  body: >-
    The decisions to make before a real install, the values that matter, how to
    enable a bundle, and the one route without which nothing answers.
  url: /agent-ops-operator/installation/
---

The console is where you watch agents work, and where you talk to them.

It ships **enabled by default**. `console.enabled: false` removes every console
object in one value.

## What it is

Three things at once, and the third is the one people miss.

- **A viewer.** Every agentops object in the namespace, every conversation, and
  the live traffic between components.
- **A channel.** An agent's answer arrives in a thread on the screen you are
  already watching, and you can reply there.
- **A signal source.** *New conversation* starts work, so you never need a chat
  surface to ask an agent something.

It reads Kubernetes and it reads the manager. It is **not a second source of
truth** — every view is the cluster, rendered.

## The tour

Six views, one question each.

{: .ao-tabs #console}
- **Overview** — Is anything wrong right now?

  The manager, capacity, the workloads and every condition that is not `True`,
  on one page.

  ![Manager version and leader, two of five runtime slots in use, the runtime images, live-activity telemetry, tables of workloads and adapters, and one problem: a signal source no pipeline claims.]({{ '/assets/img/console/overview-light.png' | relative_url }})

- **Topology** — What is moving between components?

  The whole install as a graph, in four lanes: where signals enter, what claims
  them, who answers, and where answers go. Click any element to narrow the graph
  to **what it is connected to**.

  ![Signal adapters and sources on the left, then three pipelines, the profiles and runtimes that execute them, and the channels the answers reach, with traffic rates on the edges between them.]({{ '/assets/img/console/topology-light.png' | relative_url }})

- **Conversations** — What has the fleet been asked, and what has nobody read?

  Filter by phase, pipeline or profile. Unread is **per identity**, so clearing
  it in Telegram never clears it here.

  ![Six conversations with mixed phases — Working, Pending, Idle and Closed — two marked unread, each showing its pipeline, run count, queue depth and last activity.]({{ '/assets/img/console/conversations-light.png' | relative_url }})

- **Conversation** — What did one agent actually do?

  The transcript, every run with its result, the graph of that conversation
  alone, and the object's YAML.

  The transcript reads from the **message that started it**, and rebuilds after
  a reload or a restart. Only acks are lost.

  ![One conversation: the signal that started it, the agent's answer explaining an OOM-killed container, a reply relayed in from another channel, and a box to reply from.]({{ '/assets/img/console/conversation-light.png' | relative_url }})

- **Queues** — What is waiting, and what is stuck?

  The two queues are separate because they fail separately, and every stalled
  row names its **cause**.

  ![The work queue with three conversations, one flagged at runtime ceiling, the delivery queue per adapter with the oldest operation in each, and a suppressed-signal cooldown.]({{ '/assets/img/console/queues-light.png' | relative_url }})

- **Configuration** — What is wired to what?

  Every kind, and per kind the columns that matter. A Pipeline row is the whole
  route on one line.

  ![Three pipelines, each showing its profile, the sources it claims, the channels it posts to, its toolsets, tools mode, MCP configs and Ready status.]({{ '/assets/img/console/configuration-light.png' | relative_url }})

## What it does for you

{: .ao-cards}
- **Watch a run happen**

  The transcript updates live. You see the tools the route granted and every
  call the agent made, not just the answer.

- **Reply where you are watching**

  The console is a channel. No context switch to a chat app, and the reply lands
  in the same conversation.

- **Start work without a chat surface**

  *New conversation* posts to the console's own signal source. The route
  claiming it decides who answers.

- **Tell queued from stuck**

  "The agent has not replied" has several causes, and they need different fixes.
  Every stalled row names which one.

- **See the wiring, not a diagram of it**

  The graph is built from the CRs. When the wiring changes, the picture changes.

- **Read the objects themselves**

  Conditions, findings, resolved capabilities and the YAML. The console adds no
  state of its own.

## Authentication

The console can instruct an agent. In an install where that agent holds
cluster-admin, this is a control plane rather than a viewer, so decide this
before you expose it.

{: .ao-callout}
> **An unconfigured token authorizes nobody.** "No token set" never means "no
> authentication required". A console with no token answers a wrong password and
> an absent one identically.

### The shipped mode: a shared token

The chart generates a token on install and stores it in a Secret.

| What | Where |
|---|---|
| Secret | `agentops-console-<console.name>`, default `agentops-console-console` |
| Key | `uiToken` |
| Pin or rotate it | `console.auth.uiToken` |
| Supply your own Secret | `console.auth.existingSecret` |

Read the generated one:

```sh
kubectl -n agent-ops get secret agentops-console-console \
  -o jsonpath='{.data.uiToken}' | base64 -d
```

```powershell
kubectl -n agent-ops get secret agentops-console-console `
  -o jsonpath='{.data.uiToken}' | base64 -d
```

**A redeploy does not sign anyone out.** The Secret is generated on install
only, and it is kept rather than regenerated. Rotating the token is something
you ask for with `console.auth.uiToken`, never something an upgrade does to you.

A signed-in browser holds a session cookie for 12 hours.

### Declaring your own authenticator

Put oauth2-proxy, Cloudflare Access or an ext-authz filter in front, and tell
the release that you did. **Two values, and both are required.**

```yaml
console:
  auth:
    enabled: false
    externalAuthenticator: oauth2-proxy
```

The render **fails** if `enabled` is false and nothing is named. Half of it is
not a configuration — it is the failure mode where a fresh install is wide open,
and it is refused rather than warned about.

The name is recorded, not verified. The chart cannot see what sits in front of
the Service, so it makes the release answer *what protects this console?*

### What the proxy must do

The console trusts these headers, in this order. It takes the first one with a
value and stops.

| Order | Header |
|---|---|
| 1 | `X-Forwarded-Preferred-Username` |
| 2 | `X-Forwarded-Email` |
| 3 | `X-Forwarded-User` |
| 4 | `X-Auth-Request-Preferred-Username` |
| 5 | `X-Auth-Request-Email` |
| 6 | `X-Auth-Request-User` |

Three requirements follow, and none of them is optional.

1. **Be the only route to the Service.** A port-forward, or a pod in the same
   namespace, bypasses the proxy entirely.
2. **Strip all six from the client.** A header the proxy does not strip is a
   header a browser can set, and the console has no way to tell the two apart.
3. **Forward one of them.** Whichever your identity provider gives you.

Forwarding no identity is allowed, and its consequence is stated rather than
hidden.

| The request carries | The console does |
|---|---|
| An identity header | Serves reads, allows writes, logs every write against that identity |
| No identity header | Serves reads, **refuses writes**, and invents no name |

The console says so on screen. The masthead reads `unknown` instead of a name,
which is the only visible sign that the proxy in front is forwarding nothing.

Writes can also be turned off outright, for everyone, with
`console.write.enabled: false`. The affordances disappear and the endpoints
refuse, so it is a boundary rather than a preference.

For the ingress annotations, the read-only Role and the full values list, see
[the console reference](https://github.com/kostiantyn-matsebora/agent-ops-operator/blob/master/docs/console.md).

## What it cannot do

**No write path to the Kubernetes API exists in the console module.** Not
disabled, not gated — absent. Its Role carries no write verb on any agentops
kind, so it cannot edit a Pipeline, a profile or a toolset even if something
asked it to.

The one write anywhere is a message posted to the manager, over the ordinary
channel contract. Everything else you see is a read.

Wiring is declared in YAML and belongs to whatever manages your cluster. A
console that edited it would be competing with your GitOps.
