package commandbus

import (
	"strings"
	"time"
)

// A retained feed is the second shape a queue may take (design D-16): an
// ordered log read by cursor, where the first shape is a queue of tasks that a
// consumer claims, leases and settles. A feed has no claim, lease or
// acknowledgement. An entry is appended once with its command id, every
// follower reads the log after the id it last received, and nothing leaves the
// log because it was delivered: only its age and count remove an entry.
//
// The two shapes are separate on purpose. A task queue removes a consumed
// message, so it cannot be replayed; a feed that is replayed must outlive its
// delivery.

// FeedFamily is the Queue DefaultFeedInventory declares every feed's policy
// under, as OutFamily is for the out queues.
const FeedFamily Queue = "feed"

const feedFamilyPrefix = "feed."

// FeedQueue names the feed of one consumer. The argument is the ADAPTER's
// name: one console serving several Channels reads one log, so one cursor
// orders everything it is sent.
func FeedQueue(name string) Queue { return Queue(feedFamilyPrefix + name) }

// FeedPolicy is everything a feed declares about how long it keeps entries.
type FeedPolicy struct {
	// MaxAge is how long an entry is kept, read or not.
	MaxAge time.Duration
	// MaxEntries caps how many entries are kept, oldest trimmed first. Zero
	// means no cap.
	MaxEntries int
	// DedupWindow is how long an entry suppresses a Publish of the same command
	// id, so a producer re-deriving its entry on every pass appends it once.
	// Past it the id may be appended again, which is how an id a recreated
	// object legitimately reuses reaches its consumer.
	DedupWindow time.Duration
}

// FeedDeclaration is one row of the feed inventory.
type FeedDeclaration struct {
	Queue  Queue
	Policy FeedPolicy
}

// Default feed retention: 30 seconds and 5000 entries. The window only has to
// cover a reconnect: a longer gap is recovered from the stored conversation
// record the console already merges into its transcripts.
const (
	DefaultFeedMaxAge     = 30 * time.Second
	DefaultFeedMaxEntries = 5000
	DefaultFeedDedup      = 30 * time.Second
)

// DefaultFeedInventory is the closed feed inventory: one policy, filed under
// FeedFamily, that every feed resolves to. Each call returns a fresh slice.
func DefaultFeedInventory() []FeedDeclaration {
	return []FeedDeclaration{{
		Queue:  FeedFamily,
		Policy: FeedPolicy{MaxAge: DefaultFeedMaxAge, MaxEntries: DefaultFeedMaxEntries, DedupWindow: DefaultFeedDedup},
	}}
}

// ResolveFeed finds the Declaration governing feed in inv; feed must be a
// FeedQueue name.
func ResolveFeed(inv []FeedDeclaration, feed Queue) (FeedDeclaration, bool) {
	if !strings.HasPrefix(string(feed), feedFamilyPrefix) || len(feed) == len(feedFamilyPrefix) {
		return FeedDeclaration{}, false
	}
	for _, d := range inv {
		if d.Queue == FeedFamily {
			return d, true
		}
	}
	return FeedDeclaration{}, false
}

// FeedEntry is one entry of a feed. ID is the command id the producer derived,
// and is the cursor a follower names to resume after it.
type FeedEntry struct {
	ID      CommandID
	At      time.Time
	Payload []byte
}

// MaxFeedIDLen bounds an entry's command id: it is a cursor a client sends back.
const MaxFeedIDLen = 512
