package commandbus

import (
	"strings"
	"time"
)

// Backoff is the delay a Retrying task waits out before it is claimable
// again, computed as a jittered value between Min and Max (design D-5).
// Min == Max with Jitter false is a fixed delay.
type Backoff struct {
	Min, Max time.Duration
	Jitter   bool
}

// Policy is everything a queue (or one of its kinds) declares about how it
// runs (design D-5): attempts, backoff, TTL, dedup window, park cool-off and
// the per-invocation in-flight cap. Every Declaration carries one, in full —
// a kind does not merge a partial override onto its queue's policy, because
// D-5's own table already varies TTL (not only attempts and backoff) across
// the "out" family's three kinds; see DefaultInventory's doc comment for why
// this package models a kind as its own complete Declaration rather than a
// sparse override.
type Policy struct {
	Attempts int
	Backoff  Backoff
	// TTL bounds how long a task may sit Queued before it expires unrun.
	// Zero means no TTL.
	TTL time.Duration
	// DedupWindow is how long a Succeeded task keeps suppressing a fresh
	// Submit under the same CommandID.
	DedupWindow time.Duration
	// ParkCoolOff is how long a Dead task keeps suppressing a fresh Submit.
	// Zero is D-5's "never parked": the id is never suppressed by a dead
	// task at all.
	ParkCoolOff time.Duration
	// InFlightCapPerInvocation bounds how many of one InvocationID's tasks
	// may be Running at once, on this queue — the cap a bulk batch paces
	// itself against. Zero means uncapped.
	InFlightCapPerInvocation int
	// DeliveryTTL, when positive, makes the queue a HAND-OVER queue (design
	// D-17). A claim is the consumer's acknowledgement: from it the message is
	// the consumer's and is never offered again, and the lease it holds runs
	// DeliveryTTL, not the bus-wide lease. A message with no report when that
	// passes is failed, and a failure is final: there are no attempts and no
	// backoff, because the consumer retries against its own surface until the
	// TTL. Zero keeps the lease-and-retry queue the other declarations are.
	DeliveryTTL time.Duration
}

// Declaration is one row of the closed queue inventory (design D-5): a
// Queue, the Kind it governs (KindDefault for a queue with none), and that
// combination's full Policy.
type Declaration struct {
	Queue  Queue
	Kind   Kind
	Policy Policy
}

// OutFamily is the Queue DefaultInventory declares the "out.<channel>"
// queues' policies under (design D-5): one queue per Channel resource, named
// for the Channel (out.console, out.home-telegram). A concrete queue never
// appears in the inventory itself — see familyOf.
const OutFamily Queue = "out"

const outFamilyPrefix = "out."

// familyOf maps a concrete runtime Queue to the Queue its Declaration is
// filed under: every "out.<channel>" queue shares the "out" family's
// three kinds, because the Channels the manager will ever route through are
// not a closed set the inventory can enumerate, while the kinds of outbound
// send ARE (design D-5's three out.<channel> rows). Every other queue is its
// own family.
func familyOf(q Queue) Queue {
	if strings.HasPrefix(string(q), outFamilyPrefix) {
		return OutFamily
	}
	return q
}

// OutQueue names the queue one Channel resource's sends travel on. The
// argument is the Channel's name, not its adapter: an adapter serving several
// Channels drains all of their queues.
func OutQueue(channel string) Queue {
	return Queue(outFamilyPrefix + channel)
}

// DefaultInventory is the closed queue inventory (design D-5), with its
// starting values — numbers the proposal names explicitly as provisional,
// settled later by the bench and the system lanes, and every one a chart
// value by the time task 8 wires it in. This function, not a package-level
// var, is the inventory's home: each call returns a fresh slice of value
// (not pointer) Declarations, so a caller that needs to parameterize one
// field — task 5's wiring sets the delete-conversation TTL from the same
// config the finalizer's own grace period reads, rather than duplicating
// the number — edits its own copy without racing every other caller or
// test.
//
// Six entries: "conv" and "in.signal" each
// have one policy (KindDefault); the "out" family has four, one per kind,
// each a COMPLETE Declaration rather than a sparse override of attempts and
// backoff alone — the table's own TTL column already varies across them
// (none, 2 minutes, the finalizer's grace), so an override limited to
// attempts and backoff could not express it. The judgment call, and why: a
// queue name per kind (three out.* Declarations sharing one family) reads
// cleaner here than a nested override map that still needed an escape
// hatch for TTL.
func DefaultInventory() []Declaration {
	return []Declaration{
		{
			// Key: the root conversation. In-flight cap paces a bulk batch
			// (a console bulk close, a cascade) sharing one InvocationID.
			Queue: "conv",
			Kind:  KindDefault,
			Policy: Policy{
				Attempts:                 5,
				Backoff:                  Backoff{Min: time.Second, Max: 30 * time.Second, Jitter: true},
				TTL:                      0,
				DedupWindow:              time.Minute,
				ParkCoolOff:              5 * time.Minute,
				InFlightCapPerInvocation: 5,
			},
		},
		{
			// Key: source + initiationId.
			Queue: "in.signal",
			Kind:  KindDefault,
			Policy: Policy{
				Attempts:    5,
				Backoff:     Backoff{Min: time.Second, Max: 30 * time.Second, Jitter: true},
				TTL:         0,
				DedupWindow: time.Minute,
				ParkCoolOff: 5 * time.Minute,
			},
		},
		{
			// Key: the conversation (the queue is the channel). Run replies, input
			// cards and notices that are re-derived on every pass. A hand-over
			// queue (design D-17): the consumer acknowledges, delivers with its own
			// retries within DeliveryTTL, then reports; a failure is final.
			Queue: OutFamily,
			Kind:  KindOutStable,
			Policy: Policy{
				Attempts:    1,
				DedupWindow: time.Minute,
				ParkCoolOff: 5 * time.Minute,
				DeliveryTTL: 10 * time.Minute,
			},
		},
		{
			// Key: the conversation (the queue is the channel). Acks and notices — fire and
			// forget, so a short TTL keeps a restart from delivering a
			// stale one.
			Queue: OutFamily,
			Kind:  KindOutEphemeral,
			Policy: Policy{
				Attempts:    1,
				TTL:         2 * time.Minute,
				DedupWindow: 30 * time.Second,
				ParkCoolOff: time.Minute,
				DeliveryTTL: 2 * time.Minute,
			},
		},
		{
			// Key: the conversation (the queue is the channel). ensure-topic and
			// close-topic: the topic id travels back in the consumer's done report.
			Queue: OutFamily,
			Kind:  KindOutTopic,
			Policy: Policy{
				Attempts:    1,
				DedupWindow: time.Minute,
				ParkCoolOff: 5 * time.Minute,
				DeliveryTTL: 5 * time.Minute,
			},
		},
		{
			// Key: the conversation (the queue is the channel). TTL approximates the
			// finalizer's own grace period — the two must be kept equal
			// operationally. The constant here is a starting value;
			// whatever constructs a Bus for the manager should override
			// it from the same config the finalizer reads, rather than
			// the two drifting apart (left for task 5's wiring).
			Queue: OutFamily,
			Kind:  KindOutDeleteConversation,
			Policy: Policy{
				Attempts:    1,
				TTL:         5 * time.Minute,
				DedupWindow: 0,
				ParkCoolOff: 0, // D-5: never parked.
				DeliveryTTL: 5 * time.Minute,
			},
		},
	}
}

// Resolve finds the Declaration governing (queue, kind) in inv, matching
// queue by family (see familyOf) and kind exactly.
func Resolve(inv []Declaration, queue Queue, kind Kind) (Declaration, bool) {
	family := familyOf(queue)
	for _, d := range inv {
		if d.Queue == family && d.Kind == kind {
			return d, true
		}
	}
	return Declaration{}, false
}
