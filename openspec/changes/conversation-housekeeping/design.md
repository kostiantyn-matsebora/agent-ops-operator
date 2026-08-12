## Context

Four things are true today and they interlock:

1. **`Conversation` objects accumulate.** Only `/close` and `kubectl delete`
   remove one — and `/close` requires a human in a thread, so an unattended lane
   has no way to end anything. An install with an observing lane makes them
   continuously — 31 in 35 hours on the reference cluster — each owning
   `ConversationInput` objects and an `agentops-mcp-conv-<name>` ConfigMap that
   GC only reclaims once the owner goes.
2. **Workspace directories accumulate.** `persistence-in-chart` gave each
   conversation `<claim>/<conversation>` via a `subPath` mount. Verified on the
   reference cluster: directories for deleted conversations remain.
3. **Session transcripts accumulate.** `/data/home` is mounted with **no
   `subPath`**, so every conversation's claude-code sessions land in one shared
   `$HOME/.claude/projects/-data-workspace/`, keyed by session id.
4. **Closing is destruction, so nobody closes anything.** `/close` deletes the
   object, and with it `status.runs[].result`, the context handle, and eventually
   the workspace. There is no way to say "I am done with this for now" — only "I
   am done with this forever". (1) is partly a consequence of (4): the only tool
   for reducing the backlog costs more than the backlog does.

Constraints that shape everything below:

- **The manager mounts no PersistentVolume** (`state-durability`). It is not a
  preference — a volume would pin it to a node and become a second source of
  truth beside the CRs.
- **`subPath` isolation is a security property**, not a layout convenience: a
  runtime pod cannot reach the claim root, so it cannot read or write another
  conversation's tree. Anything that reclaims directories needs exactly the
  reach that isolation denies agents.
- The manager already holds `delete` on `conversations`, so the etcd half needs
  no new grant.
- **REFS are snapshotted, CONTENT is not.** A conversation's materialized
  `profileRef` / `channelRefs` / `toolsets` / `mcpConfigs` are re-read at every
  use, which is what makes reopening a solved problem rather than a
  re-resolution problem.
- The console holds **no Kubernetes write path**; its only write anywhere is
  `POST /channel/inbound`.

## Goals / Non-Goals

**Goals:**

- Make closing cheap and reversible, so that it is used.
- Bounded growth for objects, directories and transcripts, each independently
  switchable and all off by default.
- Deletion that is provably safe against a conversation created mid-run.
- Keep the claim root away from anything executing agent code.
- No new grant for the manager, no volume for it, and no Kubernetes write for the
  console.

**Non-Goals:**

- Reclaiming other etcd content. Adapters' cursors are annotations on objects the
  operator does not own.
- Closing a conversation a human might still reply to. The gate is
  **finished and idle**, never old alone.
- Closing a conversation silently. An autoclose that archives a thread without a
  word is indistinguishable from a fault.
- Archiving or exporting a transcript before deletion. Deletion is the operator's
  decision, made with a window in which to change their mind.
- Reclaiming the home claim's non-session content.
- Bulk reopen. Reopening is a decision about one conversation, and a batch of
  them would re-materialise threads on surfaces nobody is watching.

## Decisions

### D1: Closing is a state transition; deletion is a second, separate act

`/close`, a console batch close and the manager's own timer all end with the
conversation at **phase `Closed`** — pod gone, MCP ConfigMap gone, topics
archived, farewell posted, capacity released — and with the object still there.
Deletion is a distinct verb with its own trigger, its own window and its own
flag.

*Why:* the destructive step and the deactivating step were fused, so an operator
who wanted the second had to accept the first. Splitting them costs one phase and
one timestamp and buys the thing that actually gets used: a reversible tidy. It
also puts the irreversible act behind a setting whose name says what it does,
instead of behind a word ("close") that reads as harmless.

*What `Closed` means, exhaustively* — anything not on this list is unchanged:

- no runtime pod, no MCP ConfigMap, no dispatch, no work units;
- not counted as active and not counted in the pending backlog, so it consumes
  no capacity and cannot starve a `Pending` conversation;
- absent from conversation REUSE — a signal whose signature matches a closed
  conversation opens a NEW one;
- absent from every pipeline: a closed conversation is not somewhere work lands;
- `status.closedAt` stamped, which is the origin of the delete clock;
- `spec` untouched, materialized refs untouched, `runtimeContextId` untouched,
  `status.runs[]` untouched — the record is the point;
- volume state untouched.

*Alternative:* a `spec.closed: true` field (rejected — closing is something the
manager DOES, and a spec field makes it something a client asserts, which is a
remote close verb wearing a CRD field). A separate `ClosedConversation` kind
(rejected — a second kind holding the same data, and every reader would need to
union two lists).

### D2: The close path keeps exactly one implementation; only its last step changes

Every originator — the `/close` command, the console batch, the manager's
timer — calls the same close, and that close now ends in a status write rather
than a `Delete`.

*Why:* the ONE-implementation rule exists so the farewell, the teardown, the
topic archiving and the capacity release cannot drift between originators. That
argument is untouched by what the final step is. Changing the step in one place
is the cheapest possible way to make this change, which is itself evidence the
rule was right.

### D2a: Topic archiving moves to the transition; the finalizer stays as the guard

`close-topic` ops are enqueued when the conversation reaches `Closed`. The
`agentops.dev/close-topics` finalizer remains, and now covers exactly one case: a
`Conversation` deleted without having been closed — a direct `kubectl delete`.
Deleting an already-`Closed` conversation finds its threads archived and its
finalizer with nothing to do.

*Why:* archiving belongs to closing, not to disappearing. It was on the finalizer
only because those were the same event. Keeping the finalizer for the manual path
preserves the existing guarantee ("deletion by any means archives first") without
making the ordinary path pay for it.

*Consequence to accept:* `close-topic` remains the ONE operation not re-derivable
from CR state, but its window shrinks — it is now outstanding across a status
write rather than across an object's disappearance, so a manager restart mid-close
finds a `Closed` conversation whose ops it can re-derive from `status.threads[]`
against a "topics archived" marker. That is a strict improvement and the reason
not to fight it: this change could make `close-topic` derivable, and should.

### D3: Reopening restores `Idle` and re-reads nothing it should not

A reopen sets phase `Idle`, clears `status.closedAt`, and leaves every
materialized ref exactly as it was.

*Why:* the refs are snapshots by design and are re-read at every use, so a reopen
that "re-resolved" wiring would do the one thing the snapshot rule forbids — let
a Pipeline edit re-wire a conversation that already exists. A reopened
conversation is the SAME conversation, with the same profile and the same
capabilities, or it is a new conversation wearing an old name.

*Continuity is promised only where it was promised.* Under
`contextStorage: volume` the workspace directory and the context handle are both
still there, so a reopen resumes. Under `none` a reopen answers fresh and says
so — the same contract a resume already has, not a new one.

*Failure is explicit:* if a referenced `AgentProfile` or `Channel` no longer
exists, the reopen fails and names the missing ref. It does not partially reopen,
and it does not silently drop a binding.

### D3a: Reopen re-establishes threads through `ensure-topic` with a hint

The reopen enqueues an ordinary `ensure-topic` per bound channel, carrying the
conversation's archived thread id as an OPTIONAL hint. An adapter that can
un-archive returns the same thread id; one that cannot ignores the hint and
returns a new one. `status.threads[]` is updated from what comes back, as it
already is.

*Why not a `reopen-topic` operation:* a new operation kind is a new thing every
adapter must implement, and most transports have no un-archive — so most
implementations would be `reopen-topic` doing exactly what `ensure-topic` does,
which is a second name for one behaviour. The hint inverts the cost: an adapter
that ignores it is already correct. `ensure-topic` also already means "make sure
there is somewhere to post", which is precisely the reopen's need.

*Why the adapter decides and not the manager:* whether a transport can un-archive
is transport knowledge, and the manager holds none — the same rule that keeps
`parse_mode` and message-length limits out of `internal/`.

*Consequence to accept:* a reopened conversation may continue in its old thread on
one channel and a fresh thread on another. That is honest — it is what the two
transports can actually do — and both are recorded in `status.threads[]`.

### D4: Two clocks, two flags, two names

- `retention.autoclose.enabled` (default `false`) / `retention.autoclose.idleAge`
  — measured from **last activity**.
- `retention.autodelete.enabled` (default `false`) /
  `retention.autodelete.closedAge` — measured from **`status.closedAt`**.

*Why two names rather than one `retention.age`:* they measure different things
from different origins with different consequences, and a shared prefix with one
duration would invite reading the second as "and then a bit longer". The names
say what the clock starts on.

*Why `status.closedAt` and not the `Closed` condition's transition time:* a
condition's `lastTransitionTime` is rewritten by any reason change on the same
condition, so a clock built on it can be reset by an unrelated status update. A
dedicated timestamp is written once, at the transition, and read by exactly one
thing.

*Why autodelete needs its own flag rather than "delete when both are set":*
closing and deleting are for different people. Autoclose with autodelete off is
the common configuration — a lane that tidies itself and keeps its record — and
it must not require declining the destructive half by leaving a duration blank.

### D4a: The autoclose window is IDLE time, never lifetime

Measured from last activity — the most recent run or input — and never from
`metadata.creationTimestamp`.

*Why:* the feature people mean by "autoclose after a period" is an inactivity
timeout. A creation clock closes a conversation that answered an hour ago simply
because it was opened last week, which is the one outcome nobody asks for. The
requeue in D5 schedules against the same instant the console already shows as
`ageSeconds`, so the list and the behaviour agree.

*Why there is still no "close as soon as the answer is delivered" mode:* an
earlier draft had one, `immediate`, that deleted a conversation the moment its
reply was marked delivered. Under the two-stage lifecycle its cost lands
differently and is worth restating, because it is easy to re-propose: closing
immediately is now cheap — the record survives — but it is still wrong, because a
person who just received an answer is the single most likely person to reply to
it, and a closed conversation sends their reply to a NEW conversation with none of
the context. The idle window is not there to protect the record any more; it is
there to protect the follow-up question.

**The record argument has moved to the DELETE window**, where it is now the whole
argument: `status.runs[].result` is the only place the answer lives in the
Kubernetes API, the console projects its transcript from the CR, and metrics keep
aggregates only. A short `closedAge` destroys what the operator is trying to read,
and for a conversation bound to no channel destroys it with no transport copy
anywhere.

*Alternatives:* a bare duration where `0` means immediate (rejected — ambiguous
between "immediately" and "disabled"); a `maxLifetime` ceiling alongside the idle
window (deferred — it backstops a conversation kept alive by trickle traffic,
which no observed install produces yet).

### D4b: Delivery is an eligibility rule for closing, not a mode's special case

A conversation with a recorded run not yet marked delivered to every bound channel
is NOT finished, and autoclose leaves it alone regardless of age.

*Why:* a conversation reaches `Idle` the instant `POST /work/done` records the
result, while the reply may still be an unclaimed `send` op. A long window makes
that unlikely, not impossible: an adapter down for the length of the window is
exactly when it happens, and then the close archives a thread whose answer never
arrived. Retaining is the safe direction; `status.runs[].delivered[]` already
records this, so the gate is a check rather than a new mechanism.

*Note that this survives the split:* it constrains CLOSING, not deleting, because
it is about a message reaching a thread — and threads are archived at the close.

*Consequence to accept:* a channel whose adapter is permanently down holds its
conversations open forever. That is visible in the queue statistics as outstanding
ops, and it is the correct direction to fail.

### D4c: An autoclose says goodbye and names its reason

Autoclose closes through the same path as `/close`, and the farewell states that
the close was automatic and names the idle window that elapsed, so the person
reading it can find the setting responsible.

*Why:* a closed thread must read as closed. Archiving one with no message is
indistinguishable from a fault, and the person in it did not ask for the close. A
conversation ended by a timer needs the farewell more than one ended by hand.

*Cost:* the reconciler gains a dependency on the chat router, which today it does
not have. The alternative — duplicating the farewell enqueue in the reconciler —
is a second implementation of the thing D2 exists to keep single.

*What the farewell should now also say:* that the conversation can be reopened.
Under the old design a farewell was a death notice; under this one it is a
"paused, and here is how to resume" — and a farewell that does not say so
under-sells the thing the split was built for.

### D5: Both timers are self-scheduled requeues, not sweeps

When a timer is on and a conversation qualifies, the reconciler requeues it for
the moment it expires; that reconcile performs the close or the delete. No
listing, no ticker, no cross-conversation coordination.

*Why:* a sweep re-reads every conversation on a timer to find the few that matter,
and the reconciler is already invoked per conversation with the state the decision
needs. Self-scheduling also spreads the work naturally instead of producing a
burst.

*Alternative:* a periodic `List` in the manager (rejected — re-reads everything to
act on almost nothing, and re-introduces the burst the requeue avoids).

### D6: Delete and reopen are manager verbs, and their reach is the BINDING

The console reaches deletion and reopening through manager-side verbs it calls
over its existing authenticated, gated, attributed write path. The console
performs no Kubernetes write. A surface may delete or reopen a conversation whose
`spec.channelRefs` names its channel, and no other. Delete additionally requires
the conversation to be `Closed`.

*Why this does not reopen the door "no remote close verb exists" shut:* that rule
protected a property, not a syntax — **you may only end a conversation you are
part of**, and reach had to be PROVEN rather than asserted by naming a
conversation. Holding a live thread was the proof, because posting `/close` on a
thread is only possible for a surface that has one. A closed conversation has no
thread, so that proof is unavailable, and the next-strongest one is the binding
that put the thread there in the first place — recorded on the conversation, not
supplied by the caller.

*Why delete refuses a live conversation rather than closing it first:* a
close-then-delete verb would let one call do the irreversible thing to a
conversation that was still working, with the confirmation dialog naming only the
delete. Refusing with "close it first" makes the destructive step something the
operator ordered twice.

*Alternative:* let the console write to the Kubernetes API directly (rejected —
its trust boundary is that it holds no write path, and "read-only except for two
verbs" is not a boundary anyone can check at a glance).

### D7: Split by what needs the disk

Both timers and both verbs go in the **manager**; reclaiming directories and
transcripts goes in a **separate workload**.

*Why:* the etcd half needs no disk, and conversation lifecycle — admission,
eviction, close — already lives in the reconciler, which already holds `delete`.
Putting it in a Job instead would duplicate lifecycle logic and demand a second
identity with write access to conversations. The volume half genuinely cannot go
in the manager without breaking the no-volume invariant.

A useful consequence: an install with persistence off gets the whole two-stage
lifecycle with **no new workload at all**.

*Alternatives:* everything in one Job (rejected — needs `delete` on conversations
from a pod that also mounts the claim root, which is the widest possible blast
radius); everything in the manager with volumes attached (rejected outright).

### D7a: The two halves never coordinate, and do not need to

Autodelete removes the object; the directory becomes an orphan; the job reclaims
it on its next run. Between the two the state exists with no CR — unreopenable,
because there is nothing to reopen, and reclaimable, because that is exactly what
an orphan is.

*Why no handshake:* a manager that waited for disk reclamation would need to know
whether the job is even installed, and a job that reported back would need a
write. The gap is a delay in reclaiming disk, not a correctness problem, and it
self-heals on a schedule.

*Consequence to state in the values:* enabling autodelete without the housekeeping
job reclaims etcd and leaves the disk. That is a legitimate configuration for an
install with persistence off, and a silent leak for one with persistence on — so
the chart says so where the setting is.

### D8: A closed conversation's state is protected for free

The job reclaims a directory only when no `Conversation` of that name exists. A
closed conversation HAS one. So the job needs no knowledge of phases, no closed
list, and no second rule.

*Why this is worth writing down:* it is the property that makes the split safe,
and it is the one a future "optimisation" would break — a job that skipped
`Closed` conversations while listing, to "only look at live ones", would reclaim
the workspace of every conversation an operator was keeping. The listing must
remain phase-blind.

### D9: Scan the disk BEFORE listing the API — ordering, not guessing

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

*Alternative:* grace period alone (kept as a second guard for clock skew and stale
reads, but not relied on — a race that ordering eliminates should not be managed
by a timeout).

### D10: Session transcripts need the opposite order AND a grace period

A conversation's recorded session id is written by `POST /work/done`, i.e.
**after** the transcript file exists. The ordering argument of D9 therefore runs
backwards here and cannot be used.

So transcripts are reclaimed only when both hold: the session id appears in no
conversation listed *after* the scan, and the file is older than a grace period
that must exceed the longest plausible run. The default is deliberately generous —
reclaiming a transcript early breaks resume for a live conversation, and the cost
of keeping one too long is disk.

*New under the two-stage lifecycle:* a CLOSED conversation still carries its
context handle, so its transcript is still referenced and still retained. Nothing
extra is needed — but a transcript sweep keyed on "recently active conversations"
rather than "all conversations" would silently break every reopen, which is the
same trap as D8 one directory over.

*Alternative:* deriving liveness from file mtime alone (rejected — a long-idle
conversation that is still resumable would lose its history).

### D11: A dependency-free module, not a shell script or the manager image

The job is a small Go module talking to the in-cluster API over `net/http`, the
technique `signal-k8s-events` and `console` already use.

*Why:* the logic — two orderings, a grace period, a per-run bound, dry run — is
exactly the kind that needs tests, and a shell script wrapping `kubectl` is both
untestable and a third-party image this project does not otherwise pull.

*Alternatives:* `kubectl` in a stock image (rejected as above); a subcommand on the
**manager image** (genuinely attractive — no new image to version, and it could
import `api/v1alpha1` instead of re-deriving JSON shapes — but a manager image
running as a Job **with volumes mounted** makes "the manager mounts nothing"
unreadable to the next person checking it; legibility of a load-bearing invariant
beats saving one image).

### D12: The reclaiming identity is not the runtime identity

Its own ServiceAccount, its own Role, **read-only on conversations** — it performs
no API writes at all, since both etcd-side stages are the manager's job. It mounts
both claims at their ROOT and runs no agent code.

*Why it matters:* mounting the claim root is exactly the reach `subPath` isolation
denies runtime pods. Granting that to the runtime SA would hand every agent the
ability to read and delete every other conversation's tree. Rendering fails if the
two identities are configured to be the same — the same guard `k8s-bundle` already
applies to its MCP server SA.

### D13: Bounded and reversible before it is automatic

Every job run takes a `maxDeletions` bound and supports `dryRun`. Both manager
timers spread their first pass rather than firing on everything at once.

*Why:* the first run on an established install is the dangerous one. Enabling
autoclose there makes every conversation eligible simultaneously, and each close
enqueues a farewell and a `close-topic` op per bound thread — hundreds of chat
topics archiving at once, which is alarming even though it is correct. Enabling
autodelete on an install that has been closing for a while is the same burst one
stage later, and that one is irreversible.

### D14: The housekeeping workload must be self-excluded by NAME

`signal-k8s-events` excludes agent-ops' own machinery by three independent
mechanisms, the first being a name prefix — the one that holds with a cold cache.
The current prefixes are `agentops-conv-`, `agentops-adapter-` and
`agentops-signal-`; a housekeeping pod matches none.

Without the prefix a failed cleanup Job emits a Warning event, that event becomes a
signal, and the signal wakes an agent about agent-ops' own maintenance — "own
health is STATUS, not SIGNAL". A CronJob is a particularly bad offender because it
fails on a schedule.

## Risks / Trade-offs

- **`/close` no longer deletes, and someone is relying on that** → the behavioural
  break is real and stated in `CHANGELOG.md`: after upgrade, closed conversations
  remain as `Closed` rows. An install that wants the old behaviour enables
  autodelete with a short `closedAge`, which is the old semantics with a window
  bolted on. Nothing silently deletes more than before.
- **Closed conversations become the new backlog** → this is the honest cost of the
  split: autoclose alone bounds pods and capacity, not etcd. The proposal says so,
  the values say so, and autodelete is the answer. A closed conversation is far
  cheaper than an open one (no pod, no ConfigMap, no dispatch), so the backlog it
  forms is a storage question rather than a capacity one.
- **An operator enables autodelete thinking it is the tidy one** → the values
  comment states plainly that deletion removes the only durable copy of the
  result, and the window should be chosen as "how long do I want to be able to
  read this", not "how long until it is tidy".
- **A first pass acts on a large backlog at once** → `dryRun` on the job,
  `maxDeletions` per run, and jitter on both timers' first pass so conversations
  that all become eligible at manager start do not expire in the same instant.
- **Autoclose ends a conversation someone was about to reply to** → the gate is
  finished AND idle for the window; the window is off by default; the farewell says
  the thread is closed AND that it can be reopened. A reply after the close opens a
  new conversation rather than failing, which is a worse answer than a reopen and
  the reason the farewell must mention reopening.
- **A reopen lands in a thread nobody expected** → the hint makes it the same
  thread wherever the transport allows, and a new one where it does not; either way
  the reopen posts into a live thread with the conversation's history behind it. A
  bulk reopen would make this a surprise at scale, which is why there is not one.
- **A reopen resurrects a conversation whose wiring is gone** → refs are validated
  at reopen and the failure names the missing one, rather than reopening into a
  conversation that cannot dispatch.
- **The job holds the claim root** → no agent code in the image, read-only API, its
  own SA, and a render-time guard against reusing the runtime SA.
- **A transcript is reclaimed while still resumable** → generous grace period plus
  listing after the scan; closed conversations still reference their transcripts,
  so a reopenable conversation's history is retained by the ordinary rule.
- **A phase-aware "optimisation" breaks every reopen** → D8 and D10 both name it:
  the job's listing must be phase-blind. Pinned by a test that closes a
  conversation and asserts its directory survives a run.
- **A close races the reply out of its own thread** → a conversation with an
  undelivered run is not finished (D4b), so it is never closed.
- **The reconciler now depends on the chat router** (D4c) → accepted deliberately:
  the alternative is a second farewell implementation.
- **The delete verb is a remote destructive verb where none existed** → bounded by
  the binding, refused on anything not already `Closed`, gated by the console's
  write gate, and attributed to a forwarded identity. It is strictly narrower than
  `kubectl delete`, which any console operator with cluster access already has.

## Migration Plan

1. Upgrade. Close semantics change immediately: `/close` and the console batch now
   leave a `Closed` conversation. Both timers are off, so nothing is deleted.
2. Watch the `Closed` rows accumulate for a while, and use reopen once — it is the
   feature, and confirming it works is what makes step 4 safe to enable.
3. Turn autoclose on with a long window, watch the farewells and topic archiving,
   then shorten. The first pass on an established install closes everything already
   past the window, so a long window makes that pass small.
4. Turn the housekeeping job on with `dryRun`; it logs counts and names it would
   have removed. Confirm it names no directory belonging to a `Closed` conversation.
5. Turn autodelete on last, with a `closedAge` long enough that a mistake is
   recoverable by reopening. Then take `dryRun` off the job.
6. Rollback: disable the values. Anything closed can be reopened; anything deleted
   cannot — which is why step 5 is last and why dry run precedes the first real run.

## Open Questions

- **Where does the session-file grace default land?** It must exceed the longest
  run, and nothing in the system bounds run duration today. Leaning to 7 days.
- **Should a closed conversation's runtime context be reclaimable separately from
  the conversation?** A conversation kept for its record but not for its resume
  would let the disk go while the object stays. Deferred — it is a third window,
  and two already need explaining.
- **Should a `maxLifetime` ceiling accompany the idle window?** A conversation kept
  alive by trickle traffic never idles out. Deferred rather than declined — no
  observed install produces one.
- **Should the job reclaim `ConversationInput` objects orphaned by a failed
  ownerRef GC?** Probably not — that would paper over a GC bug rather than surface
  it.

**Resolved:** *should autoclose consider a conversation's channel bindings, since
it archives a topic somebody may still be reading?* No special case is needed —
D4c makes the close announce itself on every bound thread and say that it can be
reopened, so a reader is told rather than left with a thread that stopped. The
delivery gate (D4b) covers the one binding-dependent risk that remains.

**Resolved:** *does reopening need its own adapter operation?* No — D3a. The hint
on `ensure-topic` gives the adapter the same decision with none of the cost, and an
adapter that ignores it is already correct.
