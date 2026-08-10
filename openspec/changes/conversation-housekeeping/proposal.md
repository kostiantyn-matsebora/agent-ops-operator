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

**The manager gains optional conversation retention** — no volume required, and
it already holds `delete` on conversations. Retention is an explicit **mode**,
not a number, because a bare duration makes `0` ambiguous between "immediately"
and "disabled":

- `off` (default) — today's behaviour, conversations live until closed by hand.
- `age` — delete a finished conversation once it is older than a configured window.
- `immediate` — delete a conversation as soon as it is finished **and its reply
  has reached every bound thread**.

"Finished" means all of: phase `Idle`, no pending inputs, no inflight run, no
runtime pod — and, for `immediate`, every recorded run marked delivered to every
bound channel. That last clause is not decoration: a run goes `Idle` the moment
its result is recorded, while the reply may still be an unclaimed `send` op, so
deleting on `Idle` alone would archive the thread out from under the answer.
`status.runs[].delivered[]` makes it checkable.

Deletion takes the existing path, so ownerRef GC reclaims inputs and the MCP
ConfigMap, and the `agentops.dev/close-topics` finalizer archives threads first.

`immediate` is off by default and deliberately blunt: it makes a thread
**reply-dead** the moment the agent answers, because a reply into an archived
thread has no conversation to land on. It suits a noisy observing lane where the
answer is the whole product; it is wrong for anywhere a human converses.

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

- `conversation-housekeeping`: what reclaims conversation leftovers — retention
  of finished `Conversation` objects, orphan workspace directories and orphan
  session files, the safety rules that make deletion sound (listing order,
  grace period, per-run bound, dry run), and the identity separation that keeps
  the claim root away from agent workloads.

### Modified Capabilities

- `signal-self-exclusion`: the housekeeping workload must be excluded by NAME
  PREFIX. Today's prefixes are `agentops-conv-`, `agentops-adapter-` and
  `agentops-signal-`, and a housekeeping pod matches none of them — so a failed
  cleanup Job would emit a Warning event, become a signal, and wake an agent
  about agent-ops' own maintenance. That is the "own health is STATUS, not
  SIGNAL" rule, and the prefix mechanism is the one that holds with a cold cache.

## Impact

- **Manager**: conversation retention in `internal/controller/conversation_controller.go`
  (age + finished-state gate), configured by env; no new RBAC — `delete` on
  conversations is already granted.
- **New module + image**: a dependency-free `housekeeping/` module talking to the
  in-cluster API over `net/http`, the same technique `signal-k8s-events` and
  `console` already use; no client-go, no third-party image.
- **Chart**: a `housekeeping` component (CronJob, SA, read-only Role) plus values
  for schedule, retention, per-run bound and dry run; both claims mounted at their
  ROOT, which is why the identity is separate.
- **Adapters**: `signal-k8s-events/selfexclude.go` gains the housekeeping prefix.
- **Docs**: `docs/concepts.md` (retention and what reclaims what),
  `docs/console.md` untouched, `CHANGELOG.md` for the new values.
- **Depends on**: `persistence-in-chart` (archived 2026-08-10), which introduced
  the workspace claim and the per-conversation `subPath` this reclaims.
- **Out of scope**: reclaiming anything from etcd other than conversations, and
  any retention that deletes a conversation a human might still reply to — the
  gate is finished-and-old, never "old" alone.
