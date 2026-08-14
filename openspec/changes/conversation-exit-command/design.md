## Context

The machinery this change needs already exists; what is missing is a way for a
person to reach it. From the code:

- **Capacity is counted from live pods.** `liveRuntimePods` lists runtime pods
  in `Running` or `Pending` phase; `admit` compares that count to
  `MaxActiveConversations`. Deleting a pod frees a slot the moment the pod is
  actually gone.
- **Promotion is already event-driven.** `SetupWithManager` watches pods with a
  DELETE-only predicate; `MapRuntimePodToPending` wakes the FIFO-first pending
  conversations, each of which re-runs its own admission decision.
- **Eviction already deletes only the pod.** `createRuntimePod` evicts the
  longest-idle pod whose conversation `!needsWorker`, and the conversation
  survives to get a fresh pod on its next input.
- **`/close` is intercepted on the reply path**, before the text becomes an
  input, and deletes the Conversation — the finalizer archives threads, ownerRefs
  GC the pod and ConfigMap.

The gap is narrow and specific: eviction only runs when something is waiting.
With nothing waiting, an idle pod holds its slot, its checkout and its runtime's
resident memory until `RUNTIME_IDLE_TTL_M` expires — and the installs that
raise that TTL (large repositories, local models worth keeping warm) are exactly
the ones where the wait is longest.

One code fact shapes the whole design. **Deleting a pod mid-run is not merely
rude, it is doubly harmful.** `PendingInputs` treats an inflight run's inputs as
not-pending, so a replacement pod is created (because `Inflight != nil` makes
`needsWorker` true) and then gets nothing from `/work`. It idles until ITS TTL —
the long one — exits, is reaped as `Succeeded`, which clears `Inflight`, which
makes the input pending again and **re-runs work that may already have acted**.
A long stall followed by a duplicate execution. That is why `/exit` refuses while
a run is in flight rather than "just killing it": `/close` already owns
abandonment, and it owns it by deleting the conversation so nothing re-dispatches.

## Goals / Non-Goals

**Goals:**

- Let a person release a conversation's runtime immediately, keeping the
  conversation.
- Free the capacity slot through the paths that already exist, adding no
  scheduling logic.
- Make the release condition ONE predicate shared with eviction.
- Say honestly what the release costs — particularly whether the conversation
  keeps its context.
- Make `/exit` and `/close` impossible to confuse.

**Non-Goals:**

- Cancelling or interrupting a run (that is `/close`, and it deletes the
  conversation so nothing re-dispatches).
- An exit-when-idle deferral, a console button, or an HTTP endpoint.
- Any change to the cap, the FIFO rule, eviction, or the idle TTL.
- Per-sender authorization: no surface in this system authorizes individual
  senders, and `/exit` will not be the first.

## Decisions

### 1. A reply-path intercept, beside `/close`

`Router.HandleMessage` gains `isExitCommand(text)` immediately after the
`isCloseCommand` check, recognised by the same rule: `addressing.Parse`, empty
agent, empty rest, bot-suffixed form accepted. Trailing text disqualifies it, so
"exit the maintenance window when you are done" stays an instruction for the
agent.

Interception must happen before `appendInput`, or the command becomes an input
and the agent is asked to answer it while the pod it names goes on running.

`HandleCommand` gains the general-surface answer, mirroring `/close`'s: usage,
not "unknown agent", and nothing created.

### 2. The router deletes the pod directly

`Router.ReleaseRuntime(ctx, conv)` deletes `runtimepod.PodName(conv.Name)` with
`IgnoreNotFound`. No annotation, no status field, no reconciler round-trip:
immediacy is the entire feature, and a flag for the reconciler to notice later
would add state whose only purpose is to delay the thing being asked for.

`internal/chat` gains an import of `internal/runtimepod` (for the pod name and
the runtime resolution below). No cycle: `runtimepod` imports the API types and
`mcpcompile`, neither of which imports `chat`.

The reconciler needs no change. After the delete, its ordinary flow applies:
`needsWorker` is false, so it does not recreate the pod; the DELETE watch wakes
pending conversations.

*Alternative considered:* have the router set `spec`/annotation and let the
reconciler delete. Rejected — it converts a one-line delete into distributed
state, and the delay it introduces is the opposite of the request.

### 3. One definition of "busy"

`needsWorker` is currently private to the controller. It becomes
`dispatch.NeedsWorker(conv)` — the natural home, since it is
`len(dispatch.PendingInputs(c)) > 0 || c.Status.Inflight != nil` and
`PendingInputs` already lives there. The controller calls it; the router calls
it; the eviction path calls it through the controller.

This matters more than it looks: `/exit` and eviction must mean the same thing
by "idle", or a conversation the operator was told is releasable is one the
manager will not release, and the difference will be discovered as a bug report
about the cap.

The router's existing `busy` (`conv.Status.Inflight != nil ||
len(conv.Spec.Inputs) > 0`, used only to choose an acknowledgement string) is
left alone: it is deliberately coarser — an unpruned processed input still means
"do not promise instant" — and is not a capacity decision.

### 4. Refusals name the alternative

- **Inflight**: refuse, name the run id, offer `/close` for abandoning it.
  Rationale in Context — the mid-run delete costs a TTL-long stall and a
  repeated run.
- **Queued input**: refuse and say the pod would be recreated immediately.
  Releasing here is not dangerous, merely pointless, and a command that appears
  to work and changes nothing is worse than one that explains itself.
- **No pod**: report that there was nothing to release. Not an error — the
  desired state already holds.

### 5. The reply tells the truth about context

`runtimepod.ResolveFor(...).ContinuityPossible()` already answers "can this
conversation carry context across a pod loss" — `contextStorage: none` never,
`external` always, `volume` only with a home PVC. `httpapi` uses it to decide
whether to send a context handle at all.

`/exit` uses the same call to choose between two replies: the conversation keeps
its context and resumes, or the next message starts fresh. The second is not a
new loss — the idle TTL causes exactly the same one — but a release someone
CHOSE should state its price at the moment of choosing.

This gives `Router` one new field, `Runtime runtimepod.Config` (the bootstrap
fallback), wired in `cmd/manager/main.go` from the same value `httpapi.Server`
already receives. Resolution needs the profile, which the router reads via
`conv.Spec.ProfileRef`; a resolution error degrades to the neutral wording
rather than failing the command — the pod is already released, and refusing to
say so because a Get failed would be the worst of both.

### 6. Messages are typed, not rendered

All four replies go out as `chat.Notice` / `chat.Warn`, prose in the named
markdown subset, through the ops queue like every other message. No transport
dialect, no length management, no `parse_mode` — the manager composes meaning,
the adapter composes presentation.

### 7. No new activity event kind

The conversation reconciler emits no activity events today; automatic eviction
is invisible on the graph. Emitting one only for the user-triggered release
would make the telemetry claim manual releases are the only ones that happen.
If capacity release deserves an event, it deserves one for BOTH paths and a row
in the contract's event table — a separate change, noted in Open Questions.

### 8. Discoverability, and the reserved-name consequence

`/agents` gains one line naming `/exit` and `/close` with the difference between
them. Two commands one word apart, one of which archives a thread, is precisely
the pair that must not be guessed at.

Consequence, stated rather than solved: a Pipeline named `exit` can no longer be
reached by `/exit`, exactly as one named `close`, `agents`, `help` or `start`
cannot today. The interception happens before the Pipeline lookup, which is what
makes the command reliable. Documented in `docs/concepts.md`; a Ready-condition
warning for a Pipeline whose name shadows a command is a reasonable follow-up
and not this change.

## Risks / Trade-offs

- **`/exit` mistaken for `/close`** → Both replies state what remains: `/exit`
  says the conversation and thread stay open; `/close` says the thread is
  archived. `/agents` lists both. This is the risk that justifies the
  discoverability requirement.
- **Someone uses `/exit` to stop a runaway agent** → Refused, with `/close`
  named in the same message. The refusal is more useful than the release would
  have been.
- **Context lost where continuity is not durable** → Warned in the reply, using
  the manager's existing computation rather than a guess.
- **A pod that lingers in termination** → It still counts as active until gone,
  so the cap is never exceeded; promotion happens on the DELETE event, not on
  the API call. The reply says the runtime was released, not that a slot is
  free at that instant.
- **Double `/exit`** → Idempotent: `IgnoreNotFound`, and the second reply says
  there was nothing to release.
- **A new input immediately after `/exit`** → An ordinary admission: a fresh pod
  is created and the context resumes. Nothing to prevent; this is the designed
  behavior of an eviction.
- **Moving `needsWorker` into `dispatch`** → A pure move with one call site
  changed; pinned by the existing controller tests, and the router tests added
  here exercise the same helper from the other side.

## Migration Plan

No migration. The command is new, adds no CRD field, no status, no stored state
and no contract change; an install that never types `/exit` behaves identically.

**Rollback:** delete the intercept. Nothing persists that would outlive it —
a released pod is indistinguishable from an evicted or timed-out one.

## Open Questions

- **Exit-when-idle.** `/exit` during a run could set a flag and release when the
  run completes, instead of refusing. It is friendlier and it is also new
  persistent state on the Conversation for a convenience; worth revisiting once
  it is known how often the refusal is hit.
- **A capacity-release activity event** for both the automatic and manual paths,
  with its row in the contract's event table.
- **A console affordance.** The console is a channel adapter, so typing `/exit`
  in a conversation already works. A button would be a console-side change and
  belongs with the console's own work, not here.
- **Shadowed command names.** Whether a Pipeline named after a manager command
  should report it on its Ready condition rather than silently being
  unreachable by that command.
