## Why

Nothing in agent-ops ever reclaims what a conversation leaves behind. `/close`
and `kubectl delete` are the only ways a `Conversation` disappears, so on any
install with an observing lane they accumulate without bound — the reference
cluster holds **31 after 35 hours**, which is thousands over a year, each with
its `ConversationInput` objects and a compiled MCP ConfigMap.

Chart 5.1.0 made the storage half concrete. Workspace persistence gives every
conversation its own directory in a shared claim, and nothing deletes it: on the
reference cluster, directories for conversations that no longer exist are still
there. The home claim is worse, because it is mounted with **no `subPath`** —
every conversation's claude-code session transcripts pile into one shared
`$HOME/.claude/projects/-data-workspace/`, and a resumed conversation keeps
appending to it.

The manager cannot fix the storage half: it **mounts no PersistentVolume**, by
invariant, and that invariant is load-bearing (a volume would pin it to a node
and become a second source of truth beside the CRs). So reclamation has to be
split by what actually needs the disk.

## What Changes

**The manager gains optional conversation autoclose** — no volume required, and
it already holds `delete` on conversations. Two values, both on the manager:

- `retention.enabled` (default `false`) — off is today's behaviour, in which a
  conversation lives until closed by hand.
- `retention.age` — the idle window. A finished conversation is closed once it
  has been inactive for that long.

**The window is idle time, never lifetime.** It is measured from last activity —
the most recent run or input — not from creation. A conversation created ten days
ago that answered an hour ago is not old; it is busy. Measuring from creation
would close conversations mid-use, and it is the reading a bare "age" invites.

"Finished" means all of: phase `Idle`, no pending inputs, no inflight run, no
runtime pod, and every recorded run marked delivered to every bound channel.
That last clause is not decoration: a run goes `Idle` the moment its result is
recorded, while the reply may still be an unclaimed `send` op, so closing on
`Idle` alone can archive the thread out from under the answer — a real risk
whenever an adapter is down, however long the window is.
`status.runs[].delivered[]` makes it checkable.

**Autoclose says goodbye.** It closes through the router's existing
`CloseConversation` rather than issuing a bare delete, so every bound thread gets
a farewell naming the reason before the object goes. A closed thread must read as
closed; archiving one silently leaves a person looking at a conversation that
simply stopped. Closing that way also keeps ownerRef GC reclaiming inputs and the
MCP ConfigMap, and the `agentops.dev/close-topics` finalizer archiving threads.

**There is deliberately no "close as soon as the answer is delivered" mode.** An
earlier draft had one. It is cut because the `Conversation` is the only durable
record of a result: `status.runs[].result` is where the answer lives in the API,
so deleting the object the instant it is delivered leaves the console showing
nothing at all, and a conversation bound to no channel loses the result outright.
Its safety rested on a chat thread retaining the message — a binding it never
checked. A short `retention.age` serves the same noisy-observing-lane case and
leaves a window in which the result can actually be read.

**A chart-shipped, opt-in CronJob reclaims disk**, because only something that
mounts the claim roots can:
- **Orphan workspace directories** — `<claim>/<name>` where no `Conversation` of
  that name exists.
- **Orphan session files** — transcripts on the home claim whose session id no
  longer appears in any live `Conversation.status.sessionId`.

It runs no agent code, under its **own** ServiceAccount with read-only access to
conversations — never the runtime SA, which would hand agent workloads the claim
root that `subPath` isolation exists to deny them.

**Safety is the design, not a flag.** An orphan is only reclaimed if it is
absent from a listing taken *before* the directory scan AND older than a grace
period, so a conversation created mid-run is never mistaken for garbage. Every
run is bounded (`maxDeletions`) so a first run on an old install cannot archive a
thousand chat topics at once, and a `dryRun` mode reports what it would remove.

**BREAKING for nobody**: every component here is off by default. An install that
changes nothing behaves exactly as today.

## Capabilities

### New Capabilities

- `conversation-housekeeping`: what reclaims conversation leftovers — autoclose
  of finished, idle `Conversation` objects, orphan workspace directories and
  orphan session files, the safety rules that make deletion sound (listing
  order, grace period, per-run bound, dry run), and the identity separation that
  keeps the claim root away from agent workloads.

### Modified Capabilities

- `signal-self-exclusion`: the housekeeping workload must be excluded by NAME
  PREFIX. Today's prefixes are `agentops-conv-`, `agentops-adapter-` and
  `agentops-signal-`, and a housekeeping pod matches none of them — so a failed
  cleanup Job would emit a Warning event, become a signal, and wake an agent
  about agent-ops' own maintenance. That is the "own health is STATUS, not
  SIGNAL" rule, and the prefix mechanism is the one that holds with a cold cache.

## Impact

- **Manager**: autoclose in `internal/controller/conversation_controller.go`
  (idle-age + finished-state gate) closing through `chat.Router.CloseConversation`,
  configured by env (`CONVERSATION_RETENTION_ENABLED`, `CONVERSATION_RETENTION_AGE`);
  no new RBAC — `delete` on conversations is already granted.
- **New module + image**: a dependency-free `housekeeping/` module talking to the
  in-cluster API over `net/http`, the same technique `signal-k8s-events` and
  `console` already use; no client-go, no third-party image.
- **Chart**: a `housekeeping` component (CronJob, SA, read-only Role) plus values
  for schedule, `retention.enabled` / `retention.age`, per-run bound and dry run;
  both claims mounted at their ROOT, which is why the identity is separate.
- **Adapters**: `signal-k8s-events/selfexclude.go` gains the housekeeping prefix.
- **Docs**: `docs/concepts.md` (retention and what reclaims what),
  `docs/console.md` untouched, `CHANGELOG.md` for the new values.
- **Depends on**: `persistence-in-chart` (archived 2026-08-10), which introduced
  the workspace claim and the per-conversation `subPath` this reclaims.
- **Coordinates with**: `console-bulk-close-conversations`, whose
  `conversation-close` delta states that closing has ONE teardown implementation.
  Autoclose is a third caller of it — after `/close` and the console batch — and
  that delta is worded to permit a manager-initiated close precisely so this one
  is not a special case. Whichever change lands second inherits the rule; neither
  needs to restate it.
- **Out of scope**: reclaiming anything from etcd other than conversations, and
  any autoclose that ends a conversation a human might still reply to — the gate
  is finished-and-idle, never "old" alone.
