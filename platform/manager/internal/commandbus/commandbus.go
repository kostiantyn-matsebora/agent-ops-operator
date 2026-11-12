// Package commandbus is the contract for accepting work durably and running
// it later, on any replica (design D-1 of
// openspec/changes/async-command-bus/): a Bus submits, claims, settles and
// fails tasks, and reads back their history. It depends on nothing in
// internal/state — a kube-store install runs it exactly as a nats-store one
// does.
//
// Every queue is an append-only log (design D-2): a message is appended once
// at Submit, and a claim, a failure, a completion are further entries, never
// a rewrite. A task's state is folded from its own entries, the last one
// deciding, and its position in the queue is the one the log gave its message.
// That is what lets a claim made through one Bus value complete through
// another: the lease token Claim hands back, not the Bus value or any
// connection it holds, is the only credential Settle and Fail check. The bus
// never rewrites an entry. A consumed (succeeded) task is removed from the log
// with its entries, like a regular queue's, and is thereafter absent: GetTask
// reports ErrNotFound, and its command id stays suppressed for the queue's
// dedup window. Every other entry is retained by the log's own limits.
//
// Two backends exist: commandbus/memory, for tests and local development,
// and commandbus/nats (a later change), for a production install. Both pass
// the commandbus/conformance suite and bench. commandbus/nats is the only
// package besides internal/state/nats allowed to import nats.go, enforced by
// a static test alongside internal/state's own.
package commandbus

import "context"

// Queue names one FIFO lane. "conv" and "in.signal" are used literally;
// "out.<channel>" is a family — DefaultInventory declares its policy
// once, under the family name, and every Channel resource's queue
// (out.home-telegram, out.console, ...) resolves to it (see Resolve).
type Queue string

// Key is the ordering key a producer derives for one task (D-5's "Key"
// column: a root conversation, a signal source plus initiationId, a
// conversation times a channel). Opaque to the bus — it is compared for
// equality only, never parsed. Tasks sharing a Key within a Queue are
// serialized: the next is not claimable until the previous has settled.
type Key string

// CommandID is the producer-derived dedup key (design D-4), distinct from a
// Task's own UID. A repeated Submit under one CommandID is suppressed while
// the latest task for it is queued, running, retrying, parked or succeeded
// within the queue's dedup window.
type CommandID string

// Kind refines a submission's purpose within one Queue, where more than one
// applies (design D-5): the "out" family declares three — KindOutStable,
// KindOutEphemeral and KindOutDeleteConversation — each a full policy of its
// own, not only the attempts and backoff the design text calls out by name.
// KindDefault is every other queue's only kind.
type Kind string

// KindDefault is the kind of a queue that declares no distinct kinds.
const KindDefault Kind = ""

// The three kinds of the "out" family (design D-5).
const (
	KindOutStable             Kind = "stable"
	KindOutEphemeral          Kind = "ephemeral"
	KindOutDeleteConversation Kind = "delete-conversation"
)

// SubmitRequest is what a producer hands Submit. Queue and Kind must resolve
// through DefaultInventory (see Resolve); CommandID and Key must both be
// non-empty — a queue with no real ordering key still needs one, because an
// empty Key would serialize every task in the queue against every other.
type SubmitRequest struct {
	Queue     Queue
	Kind      Kind
	CommandID CommandID
	Key       Key
	Payload   []byte
	// InvocationID tags a batch sharing one accept-time call (a console bulk
	// close, a cascade's member tasks): the per-invocation in-flight cap
	// (design D-5) counts running tasks by this field, within one Queue.
	// Empty means uncapped.
	InvocationID string
}

// Bus is the contract every backend implements. A method takes no state
// beyond its arguments: nothing about *how* a task was claimed — which
// replica, which goroutine, which connection — is needed to Settle or Fail
// it, only the lease token Claim returned.
type Bus interface {
	// Submit durably accepts req and returns the task now current for its
	// CommandID: a fresh one, or the existing one when the submission is
	// suppressed (design D-4). Submit never blocks on anything but the
	// write itself — accept is cheap by contract (design D-9).
	Submit(ctx context.Context, req SubmitRequest) (Task, error)

	// Claim blocks until a task becomes claimable on queue or ctx is done,
	// then returns it with a fresh Lease. consumerID is recorded as the
	// Lease's Holder, for a human reading /state/tasks, and is never
	// consulted by Settle or Fail. Ties among equally-eligible tasks break
	// by log position, oldest first; a task whose Key names an earlier unsettled
	// task on the same queue is skipped until that one settles.
	Claim(ctx context.Context, queue Queue, consumerID string) (Task, error)

	// Settle marks taskUID succeeded. leaseToken must match the task's
	// current Lease.Token, or ErrLeaseLost — including a lease the backend
	// already reclaimed as lapsed, and a task already settled by an
	// earlier, duplicate completion. A handler sees ErrLeaseLost on Settle
	// exactly when at-least-once delivery handed it work already finished;
	// treating that as success rather than an error is the caller's job.
	Settle(ctx context.Context, taskUID string, leaseToken string) error

	// Fail records one failed attempt with code and detail. leaseToken is
	// checked exactly as Settle checks it. Retryability is derived from
	// code (ErrorCode.Retryable) — the caller never chooses it. Exhausting
	// the queue's declared attempts, or a non-retryable code on any
	// attempt, moves the task to Dead instead of back to Retrying.
	Fail(ctx context.Context, taskUID string, leaseToken string, code ErrorCode, detail string) error

	// GetTask reads one task by UID, or ErrNotFound. A lapsed lease or an
	// expired TTL is applied before the read, so the state returned is
	// never stale by more than the backend's own reaping.
	GetTask(ctx context.Context, taskUID string) (Task, error)

	// DeadLetters lists every dead task on queue, oldest first. An empty
	// queue lists across every queue. Dead tasks are kept in a log of their
	// own with its own limits (design D-2) — a backend may cap how far back
	// this reaches, never silently invent an entry.
	DeadLetters(ctx context.Context, queue Queue) ([]Task, error)

	// Running lists the tasks on any of queues that are Running under a lease
	// still in force, in log order: the set a consumer that lost its
	// connection (and possibly its replica) may still be holding. after is the
	// command id of the last entry the consumer acknowledged (design D-16):
	// only tasks whose message was appended after it, on any of queues, are
	// listed, and an id the log does not know (or "") lists every one, which
	// is the oldest unacknowledged entry onward. A lapsed lease is excluded --
	// Claim hands it out again. Each Task carries its current Lease, so a
	// replica that never made the claim can adopt it and Settle with the token.
	Running(ctx context.Context, queues []Queue, after CommandID) ([]Task, error)

	// Feed delivers a gap-free, resumable stream of TaskEvent across every
	// queue, resuming strictly after resume (the zero Sequence starts from
	// now, mirroring internal/state.Watch's empty-Revision convention). The
	// channel closes when ctx is done.
	Feed(ctx context.Context, resume Sequence) (<-chan TaskEvent, error)

	// Stats snapshots the counts for one (queue, kind) pair — the same pair
	// Resolve matches against DefaultInventory. kind is KindDefault for a
	// queue with no distinct kinds.
	Stats(ctx context.Context, queue Queue, kind Kind) (QueueStats, error)
}
