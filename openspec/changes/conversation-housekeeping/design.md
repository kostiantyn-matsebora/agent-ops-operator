## Context

Three things accumulate today and nothing removes any of them:

1. **`Conversation` objects.** Only `/close` and `kubectl delete` remove one — and
   `/close` requires a human in a thread, so an unattended lane has no way to end
   anything. An
   install with an observing lane makes them continuously — 31 in 35 hours on
   the reference cluster — each owning `ConversationInput` objects and an
   `agentops-mcp-conv-<name>` ConfigMap that GC only reclaims once the owner goes.
2. **Workspace directories.** `persistence-in-chart` gave each conversation
   `<claim>/<conversation>` via a `subPath` mount. Verified on the reference
   cluster: directories for deleted conversations remain.
3. **Session transcripts.** `/data/home` is mounted with **no `subPath`**, so
   every conversation's claude-code sessions land in one shared
   `$HOME/.claude/projects/-data-workspace/`, keyed by session id.

Constraints that shape everything below:

- **The manager mounts no PersistentVolume** (`state-durability`). It is not a
  preference — a volume would pin it to a node and become a second source of
  truth beside the CRs.
- **`subPath` isolation is a security property**, not a layout convenience: a
  runtime pod cannot reach the claim root, so it cannot read or write another
  conversation's tree. Anything that reclaims directories needs exactly the
  reach that isolation denies agents.
- The manager already holds `delete` on `conversations`, so the CR half needs no
  new grant.
- Deleting a `Conversation` is never silent: the `agentops.dev/close-topics`
  finalizer archives its threads first.

## Goals / Non-Goals

**Goals:**

- Bounded growth for all three, each independently switchable and all off by default.
- Deletion that is provably safe against a conversation created mid-run.
- Keep the claim root away from anything executing agent code.
- No new grant for the manager, and no volume for it either.

**Non-Goals:**

- Reclaiming other etcd content. Adapters' cursors are annotations on objects the
  operator does not own.
- Deleting a conversation a human might still reply to. The gate is
  **finished and idle**, never old alone.
- Closing a conversation silently. An autoclose that archives a thread without a
  word is indistinguishable from a fault.
- Archiving or exporting before deletion. A conversation's durable record is the
  chat thread the finalizer leaves behind and whatever scrapes metrics.
- Reclaiming the home claim's non-session content.

## Decisions

### D1: Split by what needs the disk

Autoclose of `Conversation` objects goes in the **manager**; reclaiming
directories and transcripts goes in a **separate workload**.

*Why:* the CR half needs no disk, and conversation lifecycle — admission,
eviction, close — already lives in the reconciler, which already holds `delete`.
Putting it in a Job instead would duplicate lifecycle logic and demand a second
identity with write access to conversations. The volume half genuinely cannot go
in the manager without breaking the no-volume invariant.

A useful consequence: an install with persistence off gets autoclose with **no
new workload at all**.

*Alternatives:* everything in one Job (rejected — needs `delete` on conversations
from a pod that also mounts the claim root, which is the widest possible blast
radius); everything in the manager with volumes attached (rejected outright).

### D2: Autoclose is a self-scheduled requeue, not a sweep

When autoclose is on and a conversation is finished, the reconciler requeues it
for the moment it expires; that reconcile closes it. No listing, no ticker, no
cross-conversation coordination.

*Why:* a sweep re-reads every conversation on a timer to find the few that
matter, and the reconciler is already invoked per conversation with the state
the decision needs. Self-scheduling also spreads closes naturally instead of
producing a burst.

*Finished* means all of: phase `Idle`, no pending inputs, no inflight run, no
runtime pod, and every recorded run delivered to every bound channel (D2b). Any
one of those missing means the conversation is live work.

*Alternative:* a periodic `List` in the manager (rejected — re-reads everything
to act on almost nothing, and re-introduces the burst the requeue avoids).

### D2a: A flag and an IDLE window — no third mode, and no lifetime clock

Two values: `retention.enabled` (default `false`) and `retention.age`. The window
is measured from **last activity** — the most recent run or input — and never from
`metadata.creationTimestamp`.

*Why idle and not lifetime:* the feature people mean by "autoclose after a
period" is an inactivity timeout. A creation clock closes a conversation that
answered an hour ago simply because it was opened last week, which is the one
outcome nobody asks for. The requeue in D2 schedules against the same instant the
console already shows as `ageSeconds`, so the list and the behaviour agree.

*Why no "close as soon as the answer is delivered" mode:* an earlier draft had a
third mode, `immediate`, that deleted a conversation the moment its reply was
marked delivered. It is cut, and the reason is worth keeping because it is easy
to re-propose. **The `Conversation` IS the durable record of a result.**
`status.runs[].result` is the only place the answer lives in the Kubernetes API;
the console projects its transcript from the CR, and metrics keep aggregates
only. So `immediate` deleted the answer at the instant of delivery: the console
showed nothing, and a conversation bound to no channel lost the result outright.
Its apparent safety rested on a chat thread retaining the message after the topic
was archived — a binding it never checked, and one that does not exist for a
console-only or unbound conversation. A short `retention.age` covers the same
noisy-observing-lane case and leaves a window in which the result is readable.

*Alternatives:* a bare duration where `0` means immediate (rejected — ambiguous
between "immediately" and "disabled", and one keystroke from a very different
behaviour; the flag removes the ambiguity without a mode enum); a `maxLifetime`
ceiling alongside the idle window (deferred — it backstops a conversation kept
alive by trickle traffic, which no observed install produces yet).

### D2b: Delivery is an eligibility rule, not a mode's special case

A conversation with a recorded run not yet marked delivered to every bound
channel is NOT finished, and autoclose leaves it alone regardless of age.

*Why it survives `immediate`'s removal:* a conversation reaches `Idle` the
instant `POST /work/done` records the result, while the reply may still be an
unclaimed `send` op — the window `persistence-in-chart` made survivable. A long
window makes that window unlikely, not impossible: an adapter down for the length
of the retention window is exactly when it happens, and then the close-topics
finalizer archives a thread whose answer never arrived. Retaining is the safe
direction; `status.runs[].delivered[]` already records this, so the gate is a
check rather than a new mechanism.

*Consequence to accept:* a channel whose adapter is permanently down holds its
conversations open forever. That is visible in the queue statistics as
outstanding ops, and it is the correct direction to fail.

### D2c: An autoclose says goodbye — it closes, it does not delete

Autoclose calls the router's existing `CloseConversation` rather than issuing a
bare `Delete`, and the farewell names why the conversation ended.

*Why:* `/close` posts a farewell precisely so that an archived thread reads as
closed rather than as one that stopped. A conversation ended by a timer needs
that more than one ended by hand, because nobody in the thread asked for it. It
also keeps ONE close implementation — `/close`, the console's batch close, and
autoclose all take the same path, so the farewell, the finalizer, ownerRef GC and
the capacity release cannot drift between them.

*Cost:* the reconciler gains a dependency on the chat router, which today it does
not have. The alternative — duplicating the farewell enqueue in the reconciler —
is a second implementation of the thing this decision exists to keep single.

*Note:* this makes the reason text operator-facing. It states the window that
elapsed, so the person reading it can find the setting that closed their thread.

### D3: Scan the disk BEFORE listing the API — ordering, not guessing

For workspace directories the job scans directory entries first (T0), then lists
conversations (T1 > T0), and deletes only what the T1 listing does not contain.

*Why this is sound rather than merely careful:* a directory is created by the
kubelet mounting a `subPath` for a runtime pod, and the reconciler creates that
pod only for a `Conversation` that already exists. **The CR therefore always
predates its directory.** Any directory visible at T0 had a CR before T0, so a
fresh listing at T1 sees it — unless the conversation was deleted in between,
which is precisely the case worth reclaiming.

Reversing the order is what breaks: listing first and scanning second lets a
conversation created in between look like an orphan.

*Alternative:* grace period alone (kept as a second guard for clock skew and
stale reads, but not relied on — a race that ordering eliminates should not be
managed by a timeout).

### D4: Session transcripts need the opposite order AND a grace period

`Conversation.status.sessionId` is written by `POST /work/done`, i.e. **after**
the transcript file exists. The ordering argument of D3 therefore runs backwards
here and cannot be used.

So transcripts are reclaimed only when both hold: the session id appears in no
conversation listed *after* the scan, and the file is older than a grace period
that must exceed the longest plausible run. The default is deliberately generous
— reclaiming a transcript early breaks resume for a live conversation, and the
cost of keeping one too long is disk.

*Alternative:* deriving liveness from file mtime alone (rejected — a long-idle
conversation that is still resumable would lose its history).

### D5: A dependency-free module, not a shell script or the manager image

The job is a small Go module talking to the in-cluster API over `net/http`, the
technique `signal-k8s-events` and `console` already use.

*Why:* the logic — two orderings, a grace period, a per-run bound, dry run — is
exactly the kind that needs tests, and a shell script wrapping `kubectl` is both
untestable and a third-party image this project does not otherwise pull.

*Alternatives:* `kubectl` in a stock image (rejected as above); a subcommand on
the **manager image** (genuinely attractive — no new image to version, and it
could import `api/v1alpha1` instead of re-deriving JSON shapes — but a manager
image running as a Job **with volumes mounted** makes "the manager mounts
nothing" unreadable to the next person checking it; legibility of a load-bearing
invariant beats saving one image).

### D6: The reclaiming identity is not the runtime identity

Its own ServiceAccount, its own Role, **read-only on conversations** — it
performs no API writes at all, since retention is the manager's job. It mounts
both claims at their ROOT and runs no agent code.

*Why it matters:* mounting the claim root is exactly the reach `subPath`
isolation denies runtime pods. Granting that to the runtime SA would hand every
agent the ability to read and delete every other conversation's tree, undoing
the isolation the previous change bought. Rendering fails if the two identities
are configured to be the same — the same guard `k8s-bundle` already applies to
its MCP server SA.

### D7: Bounded and reversible before it is automatic

Every run takes a `maxDeletions` bound and supports `dryRun`, which reports what
it would remove and removes nothing.

*Why:* the first run on an old install is the dangerous one. Unbounded, retention
could delete hundreds of conversations at once, and each deletion enqueues a
`close-topic` op per bound thread — hundreds of chat topics archiving
simultaneously, which is alarming even though it is correct.

### D8: The housekeeping workload must be self-excluded by NAME

`signal-k8s-events` excludes agent-ops' own machinery by three independent
mechanisms, the first being a name prefix — the one that holds with a cold cache.
The current prefixes are `agentops-conv-`, `agentops-adapter-` and
`agentops-signal-`; a housekeeping pod matches none.

Without the prefix a failed cleanup Job emits a Warning event, that event becomes
a signal, and the signal wakes an agent about agent-ops' own maintenance —
"own health is STATUS, not SIGNAL". A CronJob is a particularly bad offender
because it fails on a schedule.

## Risks / Trade-offs

- **A first run deletes a large backlog at once** → `dryRun` default on first
  install, `maxDeletions` per run, and jitter on the initial autoclose pass so
  conversations that all become eligible at manager start do not expire in the
  same instant. Enabling autoclose on an established install is the moment this
  bites: every conversation is already past the window, and each close enqueues a
  farewell and a `close-topic` op per bound thread.
- **Autoclose ends a conversation someone was about to reply to** → the gate is
  finished AND idle for the window; the window is off by default and
  operator-chosen, and the farewell says the thread is closed rather than leaving
  it to be discovered. A reply after the close opens a new conversation rather
  than failing, since chat origination does not depend on the old object.
- **The job holds the claim root** → no agent code in the image, read-only API,
  its own SA, and a render-time guard against reusing the runtime SA.
- **A transcript is reclaimed while still resumable** → generous grace period
  plus listing after the scan; the failure mode is a resume that starts a new
  session, which the runtime already handles and reports.
- **Closing conversations loses the result an operator wanted** → this is the
  cost the cut mode made unacceptable and a window makes merely real: once the CR
  is gone, `status.runs[].result` goes with it and the console has nothing to
  show. Chat threads keep their messages and metrics keep aggregates, but neither
  is the record. Stated plainly in the values comment, because "retention" reads
  as harmless until it is not, and the window should be chosen as "how long do I
  want to be able to read this", not "how long until it is tidy".
- **A close races the reply out of its own thread** → a conversation with an
  undelivered run is not finished (D2b), so it is never closed. Without that
  clause a down adapter plus an elapsed window reintroduces, by configuration,
  the exact loss `persistence-in-chart` closed.
- **A run that never delivers keeps a conversation forever** → a channel whose
  adapter is permanently down never marks delivery, so the conversation is
  retained rather than closed. Retaining is the safe direction, and the
  undelivered op is already visible in the queue stats.
- **The reconciler now depends on the chat router** (D2c) → accepted
  deliberately: the alternative is a second farewell implementation, which is the
  drift this decision exists to prevent. The dependency is one call, and the
  reconciler already enqueues topic ops.
- **The job and the manager disagree about what exists** → they never coordinate:
  the manager deletes CRs, the job reclaims only what has no CR. A CR deleted
  between the two runs is simply reclaimed on the next one.

## Migration Plan

1. Ship every component off. An install that changes nothing behaves as today.
2. Enable `dryRun` first; the job logs counts and names it would have removed.
3. Turn autoclose on with a long window, watch the farewells and topic archiving,
   then shorten. The first pass on an established install closes everything
   already past the window, so a long window makes that pass small.
4. Rollback: disable the values. Nothing already closed comes back — which is
   why dry run precedes the first real run rather than following it.

## Open Questions

- **Where does the session-file grace default land?** It must exceed the longest
  run, and nothing in the system bounds run duration today. Leaning to 7 days.
- **Should a `maxLifetime` ceiling accompany the idle window?** A conversation
  kept alive by trickle traffic never idles out. Deferred rather than declined —
  no observed install produces one, and two windows need two explanations.
- **Should the job reclaim `ConversationInput` objects orphaned by a failed
  ownerRef GC?** Probably not — that would paper over a GC bug rather than
  surface it.

**Resolved:** *should autoclose consider a conversation's channel bindings, since
it archives a topic somebody may still be reading?* No special case is needed —
D2c makes the close announce itself on every bound thread, so a reader is told
rather than left with a thread that stopped. The delivery gate (D2b) covers the
one binding-dependent risk that remains.
