# The agent-ops console

A browser view of the whole install — every `agentops.dev` CR, the wiring
graph, and live conversation runs — that is **also a channel**. Conversations
on pipelines that list the console Channel bind a console thread, so you watch
a run and reply to the agent on the same screen.

It ships as an opt-in part of the chart (`console.enabled`, default `false`)
and as its own module, `console/`.

## Why it is an adapter and not a manager feature

The manager reads no Secrets, holds no browser surface, and grants adapters no
RBAC. A UI needs read access to every agentops CR — access the manager itself
does not have and should not proxy. Making the console a `ChannelAdapter`
keeps that reach in a separate identity with a chart-granted, read-only Role,
and gets the chat half for free: `spec.adapter: console` on a `Channel` is all
the wiring machinery it needs.

## What it shows

| View | Source | Notes |
|---|---|---|
| Topology | `Pipeline.spec` + adapter refs | Nodes: sources, pipelines, profiles, channels, adapters. Colored by the conditions the reconcilers already write (`Ready`, `Served`, `Wired`) — the console computes no health of its own, so the graph cannot disagree with `kubectl`. |
| Resources | every watched kind | Per-kind inventory with conditions, plus a detail view of the full spec and status. Opaque `config` is shown verbatim, uninterpreted. |
| Conversations | `Conversation.status` | Phase, inflight run, `runs[]` history with results, thread bindings, runtime pod. Live, from watch events. |
| Transcript | the channel contract | For **joined** conversations: agent output, acks, and attributed relays from sibling channels, with a send box. |

Unclaimed sources render **detached**, carrying their `Wired=False` reason —
the signal-dropping state that is otherwise only visible in `kubectl describe`.
A `spec.adapter` naming an adapter that does not exist renders as a broken edge
to a placeholder node.

### Joined vs observed

A conversation is **joined** when the console Channel holds a thread binding in
`status.threads[]`, and **observed** otherwise. Observed conversations are
fully visible — phase, runs, results — but read-only: there is no thread to
post into, so there is no send box. This is not a UI rule; it is the channel
contract (`/channel/inbound` requires a `threadId`).

To join a pipeline, add the console Channel to its `channelRefs[]`. That edit
is yours to make: the chart never mutates a Pipeline, and neither does the
console. The UI lists the unjoined pipelines and prints the exact patch.
Existing conversations keep the bindings they were created with; new ones get a
console thread.

### Pipeline attribution is inferred

A `Conversation` records the wiring it materialized (`profileRef`,
`channelRefs`, tooling) but never names the Pipeline that produced it — there
is no `pipelineRef` by design. The console reconstructs the link by matching
those bindings, and shows a conversation as **unattributed** when the match is
ambiguous (two pipelines with the same profile and channels) or when the
pipeline has since been re-wired. Activity badges count only attributed
conversations. If a `agentops.dev/pipeline` label is ever written, it wins.

## What it can and cannot do

- Reads `agentprofiles`, `agentruntimes`, `channels`, `channeladapters`,
  `conversations`, `pipelines`, `signaladapters`, `signalsources` — `get`,
  `list`, `watch`, in its own namespace, and nothing else. There is no write
  path to the Kubernetes API anywhere in the module.
- The only thing it writes at all is `POST /channel/inbound`, through the same
  adapter contract every other channel uses.
- It never re-ingests its own outbound posts, so cross-channel relay cannot
  loop through it.

### Trust boundary

**Whoever holds the UI token sees everything the console's ServiceAccount can
read** — every agentops CR in the namespace, conversation payloads included
(alert bodies, agent results). The Role is namespaced and read-only, and the
console can change nothing in the cluster; the token is still the whole
boundary. Keep the Service `ClusterIP` unless you mean to expose it, and put
TLS in front of any Ingress.

## Install

```yaml
console:
  enabled: true
  # optional: pin the browser token instead of generating one
  # auth:
  #   existingSecret: my-console-token   # key: uiToken
  ingress:
    enabled: false
```

Enabling it renders **CRs and RBAC only**: a `ChannelAdapter`, a `Channel`, the
UI token Secret, and a read-only Role/RoleBinding. The adapter reconciler owns
the Deployment and — because `spec.port` is set — the Service
`agentops-adapter-console`, so the chart ships no connectivity.

```sh
kubectl -n <ns> port-forward svc/agentops-adapter-console 8080:8080
kubectl -n <ns> get secret agentops-console-console -o jsonpath='{.data.uiToken}' | base64 -d
```

Disabling it again is non-destructive: the workload and Service are removed,
Channels naming `adapter: console` report `Served=False`, and conversations
keep their other threads.

## Values

| Key | Default | What it does |
|---|---|---|
| `console.enabled` | `false` | Renders nothing when false. |
| `console.name` | `console` | ChannelAdapter name — and therefore the workload, SA (`agentops-adapter-<name>`) and Service names. |
| `console.channelName` | `""` | Channel name pipelines reference (defaults to `name`). |
| `console.image.repository` / `.tag` | `kmatsebora/agentops-console` / `0.1.0` | Console image. |
| `console.port` | `8080` | Browser port; the reconciler owns the Service and injects `LISTEN_ADDR`. |
| `console.auth.existingSecret` | `""` | Use an existing Secret (key `uiToken`) instead of generating one. |
| `console.auth.uiToken` | `""` | Pin the token explicitly. |
| `console.ingress.*` | disabled | Optional Ingress (`host` required when enabled). |
| `console.resources` | `{}` | Pod resources. |

## Implementation notes

- **Dependency-free**, like every adapter module: raw list/watch over
  `net/http` (no client-go), SPA embedded with `go:embed` (no npm, no build
  step).
- **Snapshots are authoritative, the stream is a hint.** The SSE connection
  says "something of this kind changed"; the browser re-fetches. Nothing
  depends on having observed every event, which is why a tab that slept for an
  hour reconnects correctly.
- **Built for a busy namespace.** Lists are paginated (`limit` + `continue`),
  and the conversations view returns the 200 most recently active with a count
  of what it left out — a namespace can hold thousands of conversations when a
  signal lane is noisy, and the browser re-fetches on every stream hint. Run
  history is carried by the detail view only; list rows show a count.
- **Transcripts are ephemeral and bounded.** The durable record is
  `status.runs[]`, which the console reads from its watch. A restart loses
  unscrolled live messages and nothing else; thread ids derive from
  conversation UIDs, so they survive restarts without stored state.
- **Message kinds are a best reading of rendered text.** `send` ops carry chat
  HTML, not semantics, so relays and acks are recognized by their prefixes.
  Mislabelling a bubble is the worst case — nothing is dropped or duplicated —
  and it stops being a guess when messages become semantic.
- Text from the cluster and the wire is rendered as plain text: markup is
  stripped rather than trusted.
