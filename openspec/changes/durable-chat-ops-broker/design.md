## Context

See `proposal.md` — Why, for the measured failure.

`cmd/manager/main.go` already runs `k8s.io/client-go/tools/leaderelection`
(`LeaderElection: true`), writing the `agentops-manager.agentops.dev`
`Lease`. Only the leader's controller-runtime reconcilers run.

`httpapi` is a separate HTTP server in the same process, started
unconditionally. `invariants.md`'s own stated reason is that
`/signal/inbound` and `/work` must keep serving across a rolling leader
handoff.

`internal/chat.OpQueue` is a plain Go map with a mutex, constructed once
per process (`main.go:156`). It is shared between the reconciler and
`httpapi` WITHIN one process, and never shared ACROSS processes.

## Goals / Non-Goals

**Goals:**

- `manager.replicas` above 1 delivers chat ops correctly, with no silent
  stuck conversation, regardless of which replica an adapter's poll lands
  on.
- A crashed leader's in-flight claim recovers without a hand-tuned,
  independently-invented heartbeat mechanism.
- No change to the URL, query parameters, or op payload shapes a conforming
  adapter already speaks (`channel-adapter-contract`) beyond the new 503
  case.

**Non-Goals:**

- Horizontal scaling of op THROUGHPUT across replicas. This design adds
  resilience (a dead leader's work resumes elsewhere), not parallelism —
  exactly one process ever claims or completes an op at a time, same as
  today.
- A generic, reusable "Kubernetes CR as a message queue" library for this
  project's other in-memory state. Scoped to channel ops only.
- Changing `/channel/ops/{id}/done`'s accepted body shape. Only the GET
  side's response codes change.

## Decisions

### The durable store is the Conversation CR, not a new object

**Chosen**: add a claim (holder, claimed-at) to
`status.threads[channel]`, written via ordinary resourceVersion-conditioned
Patches.

**Alternatives considered:**

| Option | Rejected because |
|---|---|
| A new CRD modeling the queue itself | This project's own precedent (`close-topic` was already migrated OFF a dedicated status field and onto `status.threadsArchived[]` deriving from existing Conversation state) argues for folding into the object the op is already about, not minting a new kind for bookkeeping |
| An external broker (Redis, NATS) | `structure.md`: the manager mounts nothing and avoids a new stateful dependency. Durability already has a home — etcd, via the API server — and this project already trusts it for everything else |

### Claim writes are leader-only — a bare CAS claim is not enough

**Chosen**: only the current `Lease` holder writes a claim or clears a
stale one.

**Alternatives considered, in the order they were actually tried and
discarded this session:**

1. **Proxy a non-leader's poll to the leader** (resolve `Lease` holder →
   pod IP → `httputil.ReverseProxy`). Works, but reintroduces exactly the
   complexity it was meant to avoid, and research into existing solutions
   found no generic library for "forward to current lease holder" — it
   would be bespoke code with no reuse value.
2. **Bare CAS claim, any replica may write it, no leader involved.**
   Genuinely stateless on the manager side. Rejected because it has no
   notion of liveness: if the claiming replica crashes mid-work, the claim
   is stuck forever with nothing to notice it was abandoned, unless a TTL
   is bolted on by hand.
3. **Bare CAS claim WITH a hand-rolled TTL.** Closes the stuck-forever gap,
   but reinvents, per op-type, exactly what `leaderelection`'s `Lease`
   already does correctly for the whole process: liveness-tied ownership,
   bounded failover, no guessed timeout.
4. **Leader-only writes (chosen).** Reuses the Lease's own failover for the
   "leader crashed" case — recovery is immediate, bounded by the Lease's
   own `leaseDuration`, not a new timer. A smaller, separate staleness
   bound is still needed for "the SAME leader's claim is legitimately
   slow" (an `ensure-topic` round-trip to a real chat API is not instant),
   but that check is made entirely by the one process ever allowed to
   write the claim — a timestamp comparison, not a second writer.

### A non-leader rejects (503), rather than proxying

**Chosen**: `/channel/ops` on a non-leader returns 503 immediately.

**Why**: the adapter's long-poll client already retries on an
empty/timed-out response by design (`channel-adapter-contract`'s existing
"No ops available" scenario).

A distinguishable 503 triggers an immediate retry instead of the
adapter's normal idle backoff. That costs nothing to add, and it avoids
building IP resolution plus a reverse proxy for a case the client already
tolerates.

### Watermill's interface shape, a custom backend

**Chosen**: model the manager-side API as a `Publisher`/`Subscriber` pair
in the shape `github.com/ThreeDotsLabs/watermill` already defines, backed
by a CUSTOM implementation against the Conversation CR.

**Why not adopt watermill wholesale**: no existing watermill backend is
Kubernetes-CR-based (confirmed: official backends are AMQP, NATS, SQL,
Firestore, GCP Pub/Sub).

The durable half — the part that actually matters for correctness — is
custom work either way. Borrowing the interface shape costs little, and
it gives the migration a familiar, documented contract (`Publish`,
`Subscribe`, `Ack`/`Nack`) instead of inventing fresh naming.

## Risks / Trade-offs

- **[Risk]** The staleness bound is a real tuning decision — too short
  reclaims a legitimately in-flight op, too long leaves a crash
  unrecovered for longer than necessary → **Mitigation**: size it from
  measured `ensure-topic` round-trip latency against the slowest adapter
  in the reference install (Telegram), with margin, and make it a chart
  value rather than a constant so an install with a slower transport can
  raise it.
- **[Risk]** A conforming adapter that does NOT yet retry on 503 (every
  shipped adapter today predates this change) silently degrades to "poll
  fails, backs off, retries later" rather than retrying immediately →
  **Mitigation**: this is not a regression from today's behavior (today
  it silently gets nothing, forever, with no retry signal at all) and
  every shipped adapter (`channels/telegram`, `platform/console`) is
  updated in this change's own tasks, not left for a future one.
- **[Risk]** `go.mod` gains a new dependency in the one module that
  already has them → **Mitigation**: scoped to the interface types only.
  The custom backend has no watermill-side runtime dependency beyond that
  one package, which is widely used and pulls in no broker client of its
  own unless that specific backend package is imported.

## Migration Plan

1. Add the claim fields to `ConversationStatus`, regenerate CRDs
   (`chart/crds/`), apply directly to any live cluster per this project's
   own CRD-upgrade gotcha (Helm never upgrades an installed CRD).
2. Implement the leader-only claim/stale-reclaim logic inside the existing
   Conversation reconciler (already leader-gated by construction — no new
   leader-awareness to add there).
3. Change `/channel/ops` to reject with 503 on a non-leader, and update
   the two shipped adapters (`channels/telegram`, `platform/console`) to
   retry immediately on it.
4. Flip `chart/values.yaml`'s `replicas` default back above 1, update
   `docs/configuration.md`'s row, and the rest of the Impact section's
   named docs.

**Rollback**: revert to `replicas: 1`. Nothing in this design requires a
one-way data migration — the claim fields are additive to
`status.threads[]`, and a single replica simply never contends with
itself.

## Open Questions

- Exact staleness bound value. Deferrable: sizing it from measured
  latency is an implementation-time task (`tasks.md`), not a decision that
  changes the approach, the specs, or the task breakdown.
