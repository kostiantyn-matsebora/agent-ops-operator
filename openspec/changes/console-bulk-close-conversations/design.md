## Context

The console is a channel adapter that is also the viewer. Its trust boundary is
deliberate and narrow: read-only list/watch of the eight agentops kinds, and
**one** write anywhere in the module — `POST /channel/inbound`. Everything the
browser can cause the cluster to do goes through that hole.

Ending a conversation already has exactly one shape in the system: the `/close`
command, intercepted by the router on the reply path before the text could
become an input. It posts a farewell to every bound thread, deletes the
`Conversation`, and lets the `agentops.dev/close-topics` finalizer archive the
threads before the object disappears. Owner references then GC the runtime pod
and the MCP ConfigMap, and the freed slot admits a `Pending` conversation.

What is missing is not a mechanism but a gesture. An operator staring at 31
finished conversations after an event storm has to open 31 threads. The console
already filters and paginates that list server-side; the batch belongs there.

Constraints that shape everything below:

- The console module has **no write path to the Kubernetes API** and must not
  grow one. Bulk close is therefore not "delete these objects".
- `/channel/inbound` is **reply-only**: `threadId` required, unknown threads
  dropped, no adoption. So the console can only speak into threads it holds.
- A conversation the console merely **observes** has no console thread. That is
  not a permission gap to close; it is the joined/observed distinction the
  console-adapter spec already states.

## Goals / Non-Goals

**Goals:**

- An operator can select conversations in the console list and end them in one
  action, with an honest per-conversation outcome.
- The close taken is *the same close* — same command, same code path, same
  archiving, teardown and capacity release. Nothing about closing forks.
- Nothing new is added to the manager, the channel adapter contract, any CRD,
  RBAC, or the chart.
- A conversation that cannot be closed says why, in the terms the console
  already uses (observed, working).

**Non-Goals:**

- Closing observed conversations. That needs a manager-side close verb every
  adapter would inherit, and it would weaken "reply-only" as the shape of the
  channel contract. Explicitly deferred, not designed around.
- Automatic/age-based reclamation — that is `conversation-housekeeping`, the
  unattended half. This change does not anticipate it and does not conflict
  with it.
- Filter-scoped close ("close everything matching"), across pages or otherwise.
- Undo. `/close` is not reversible and a batch of them is not either.
- Any change to `/close` semantics, its farewell text, or the finalizer.

## Decisions

### D1. Bulk close is a fan-out of `/close` over the console's own threads

The handler resolves each named conversation's console thread and posts the
literal text `/close` through the same `Adapter.Send` path a person typing in
the console composer uses. The manager does the rest.

*Why:* it is the only design where "bulk close" and "close" cannot drift. Any
manager-side batch verb would be a second implementation of ending a
conversation, and the first divergence (a farewell not posted, a finalizer not
run, a slot not released) would be found in production rather than in review.
It also keeps the console's single-write-hole invariant literally true.

*Alternatives considered:* a manager endpoint `POST /channel/close` — rejected
above; console `delete` RBAC on conversations — rejected, it breaks the module's
defining invariant.

*Consequence to accept:* the console's transcript gets a local `/close` entry
per closed conversation, exactly as a typed close does. That is correct — the
close really was said on that thread, by that identity.

### D2. Reach is joined-only, and "observed" is an OUTCOME, not a filter

Observed conversations are selectable in the UI but come back as
`skipped: observed`. They are not hidden from selection and they are not a
request-level error.

*Why:* hiding them teaches that they closed; erroring the whole request for one
of them makes a 20-item batch fail on its first bad row. Reporting per item is
what makes the joined/observed distinction *visible* — the operator learns the
rule from the result instead of from documentation. The message names the fix
already in the codebase's vocabulary: add the console channel to the pipeline's
`channels[]`.

### D3. Working conversations are excluded by default, with a named opt-in

The request carries `includeWorking` (default `false`). With it false, a
conversation whose phase is `Working` comes back `skipped: working`. With it
true, those conversations are closed and their inflight runs abandoned, which is
`/close`'s existing honest behaviour.

*Why:* `/close` abandoning an inflight run is right for a deliberate single
close — the operator is looking at the run they are killing. As the silent
default of a batch it is wrong: the operator is looking at a list. Excluding
`Working` entirely would exclude the stuck-agent case, which is the case people
most want to mass-close, so the toggle exists and states its cost in its own
label.

*Where the check lives:* server-side, from cached CR state, never from a
client-supplied phase. The client sends names and the flag; the server decides.
A phase read on the browser is stale by definition and would be an authorization
decision made by the caller.

### D4. One request, per-item results, HTTP 200 for a mixed batch

`POST /api/conversations/close` takes `{"names":[...], "includeWorking":bool}`
and returns `200` with `{"results":[{"name","outcome","reason"}],"closed":N,
"skipped":N,"failed":N}`. Outcomes are `closed | skipped | failed`.

*Why 200 for partial:* the batch was executed; a mixed result is the normal
outcome, not a transport failure. Statuses are reserved for the request itself
failing — `400` malformed or empty `names`, `403` write gate / missing identity,
`401` unauthenticated.

*Why not one request per conversation from the browser:* the write gate,
identity attribution and the batch cap then live in the client, and 50 parallel
inbound posts arrive at the manager unordered and unbounded.

### D5. The batch is bounded and executed sequentially

`names` is capped at the list page size (50); over that is a `400`. The handler
walks them in order with the request's context, not a fan-out.

*Why:* the cap makes the blast radius equal to what one screen can show, which
is the same bound the selection UI already imposes — enforced server-side so it
holds regardless of the client. Sequential execution keeps the manager's inbound
path from seeing a burst, and 50 closes is not a latency problem worth
concurrency. A failing item does not stop the walk; it is recorded and the walk
continues.

### D6. The action is a write, gated and attributed like origination

Registered behind `a.write("bulk-close", …)`: authentication, the install-wide
write gate (`console.write.enabled`), and a forward-auth identity. Each closed
conversation is logged with the identity that ordered it.

*Why:* it is strictly more destructive than sending a message, which is already
gated. A read-only console must not be able to end conversations.

### D7. Selection is explicit rows; the confirmation names the count and the cost

A checkbox column plus a select-all-on-page checkbox, a `Close selected` action
that is disabled with nothing selected, and a confirmation modal that states the
count, the working-inclusion toggle, and that closing is not reversible. No
"select all matching filter".

*Why:* a mis-set filter closing hundreds across pages is the failure this
feature would otherwise introduce. The bound on what can be selected is what is
on screen.

### D8. A conversation being closed is shown as closing, not as still open

The finalizer holds a deleted `Conversation` for up to 2 minutes while
`close-topic` ops drain, so a closed conversation keeps appearing in the list.
`Metadata` gains `deletionTimestamp` (read-only, already present on the watched
objects) and the summary gains `closing: true`; closing rows render as such and
are not selectable.

*Why:* without it the list looks unchanged after a successful batch, the
operator concludes it failed, and re-closes. This is the smallest honest fix and
it adds no API reach — the field is already in the objects being watched.

## Risks / Trade-offs

- **[A batch of closes is a batch of `/close` messages, so a partially applied
  batch is possible]** → per-item outcomes make it visible, and re-running the
  action over the survivors is safe: a conversation already gone resolves to no
  thread and comes back `skipped`/`failed`, never a double close.
- **[Observed conversations remain uncloseable from the console, which will read
  as a gap]** → the skip reason names the exact fix (bind the console channel),
  and the proposal records why the manager-side alternative was declined, so the
  next person meets a decision rather than an omission.
- **[`includeWorking` abandons live agent work]** → off by default, stated in the
  toggle's own label, restated in the confirmation, and the count of working
  conversations in the selection is shown before confirming.
- **[The write gate is install-wide, so enabling replies also enables bulk
  close]** → consistent with origination, which is gated the same way; a
  per-action gate would be a new authorization model for one button.
- **[Sequential execution makes a 50-item batch as slow as 50 inbound posts]** →
  bounded by the cap, and the request returns one result the UI renders at once;
  if this ever hurts, bounded concurrency is an internal change with no contract
  effect.
- **[`deletionTimestamp` on the summary is new surface]** → read-only projection
  of a field already watched; no new RBAC, no new watch.
