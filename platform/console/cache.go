package main

import (
	"context"
	"errors"
	"log"
	"sort"
	"sync"
	"time"
)

// The console's view of cluster configuration: one in-memory store per watched
// kind, kept current by a list→watch loop that resumes by resourceVersion and
// relists on 410 Gone.
//
// CR state is authoritative and deltas are only a nudge to re-render: nothing
// in the console requires a browser (or the cache itself) to have observed
// every intermediate event. That is what lets a dropped delta be a
// non-incident — the subscriber is told to resync and reads the store again.

// apiSource is the slice of the Kubernetes API the cache needs. An interface
// so the whole convergence behaviour — resume, relist-on-410, reconnect — is
// testable against recorded fixtures with no cluster in sight.
type apiSource interface {
	List(ctx context.Context, kind string) ([]*Object, string, error)
	Watch(ctx context.Context, kind, resourceVersion string, fn func(eventType string, obj *Object)) error
}

// Delta is one change to publish to subscribers.
type Delta struct {
	Type   string  `json:"type"` // ADDED | MODIFIED | DELETED | RESYNC
	Kind   string  `json:"kind"`
	Name   string  `json:"name,omitempty"`
	Object *Object `json:"object,omitempty"`
}

// DeltaResync tells a subscriber that it may have missed events and should
// re-read the snapshot. Emitted after a relist and whenever a subscriber's
// buffer overflows.
const DeltaResync = "RESYNC"

// Cache holds every watched object, keyed kind → name.
type Cache struct {
	src   apiSource
	kinds []string

	mu     sync.RWMutex
	store  map[string]map[string]*Object
	synced map[string]bool

	subMu  sync.Mutex
	subs   map[int]*subscriber
	nextID int

	syncOnce sync.Once
	syncedCh chan struct{}
}

type subscriber struct {
	ch    chan Delta
	stale bool
}

// NewCache builds a cache over the given kinds.
func NewCache(src apiSource, kinds []string) *Cache {
	c := &Cache{
		src:      src,
		kinds:    kinds,
		store:    map[string]map[string]*Object{},
		synced:   map[string]bool{},
		subs:     map[int]*subscriber{},
		syncedCh: make(chan struct{}),
	}
	for _, k := range kinds {
		c.store[k] = map[string]*Object{}
	}
	return c
}

// Run starts one list→watch loop per kind and blocks until ctx is done.
func (c *Cache) Run(ctx context.Context) {
	var wg sync.WaitGroup
	for _, kind := range c.kinds {
		wg.Add(1)
		go func(kind string) {
			defer wg.Done()
			c.runKind(ctx, kind)
		}(kind)
	}
	wg.Wait()
}

// WaitForSync blocks until every kind has completed its initial list (or ctx
// ends). Callers that need a warm cache — the adapter loop deriving thread ids
// from conversation UIDs — wait on this rather than racing the first list.
func (c *Cache) WaitForSync(ctx context.Context) bool {
	select {
	case <-c.syncedCh:
		return true
	case <-ctx.Done():
		return false
	}
}

func (c *Cache) runKind(ctx context.Context, kind string) {
	backoff := time.Second
	resourceVersion := ""
	for ctx.Err() == nil {
		if resourceVersion == "" {
			objs, rv, err := c.src.List(ctx, kind)
			if err != nil {
				if ctx.Err() != nil {
					return
				}
				log.Printf("list %s: %v (retrying in %s)", kind, err, backoff)
				sleepCtx(ctx, backoff)
				backoff = nextBackoff(backoff)
				continue
			}
			c.replace(kind, objs)
			resourceVersion = rv
			backoff = time.Second
		}
		err := c.src.Watch(ctx, kind, resourceVersion, func(eventType string, obj *Object) {
			if obj.Metadata.ResourceVersion != "" {
				resourceVersion = obj.Metadata.ResourceVersion
			}
			if eventType == "BOOKMARK" {
				return // resume point only, no state change
			}
			c.apply(eventType, obj)
		})
		switch {
		case ctx.Err() != nil:
			return
		case errors.Is(err, ErrWatchExpired):
			// the resume point is too old: drop it so the next turn relists
			log.Printf("watch %s expired — relisting", kind)
			resourceVersion = ""
		case err != nil:
			log.Printf("watch %s: %v (reconnecting in %s)", kind, err, backoff)
			sleepCtx(ctx, backoff)
			backoff = nextBackoff(backoff)
			// a failed watch may have been rejected for its resourceVersion;
			// relisting is always safe, so prefer converging over resuming
			resourceVersion = ""
		default:
			// server closed a healthy stream — resume immediately
		}
	}
}

func nextBackoff(d time.Duration) time.Duration {
	d *= 2
	if d > 30*time.Second {
		return 30 * time.Second
	}
	return d
}

// replace swaps a kind's whole store after a (re)list and tells subscribers to
// resync — the one event whose meaning is "stop trusting your deltas".
func (c *Cache) replace(kind string, objs []*Object) {
	next := make(map[string]*Object, len(objs))
	for _, o := range objs {
		next[o.Metadata.Name] = o
	}
	c.mu.Lock()
	c.store[kind] = next
	c.synced[kind] = true
	allSynced := len(c.synced) == len(c.kinds)
	c.mu.Unlock()

	if allSynced {
		c.syncOnce.Do(func() { close(c.syncedCh) })
	}
	c.publish(Delta{Type: DeltaResync, Kind: kind})
}

func (c *Cache) apply(eventType string, obj *Object) {
	c.mu.Lock()
	kindStore := c.store[obj.Kind]
	if kindStore == nil {
		kindStore = map[string]*Object{}
		c.store[obj.Kind] = kindStore
	}
	if eventType == "DELETED" {
		delete(kindStore, obj.Metadata.Name)
	} else {
		kindStore[obj.Metadata.Name] = obj
	}
	c.mu.Unlock()
	c.publish(Delta{Type: eventType, Kind: obj.Kind, Name: obj.Metadata.Name, Object: obj})
}

// List returns a kind's objects sorted by name (stable rendering order).
func (c *Cache) List(kind string) []*Object {
	c.mu.RLock()
	defer c.mu.RUnlock()
	items := c.store[kind]
	out := make([]*Object, 0, len(items))
	for _, o := range items {
		out = append(out, o)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Metadata.Name < out[j].Metadata.Name })
	return out
}

// Get returns one object, or nil.
func (c *Cache) Get(kind, name string) *Object {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.store[kind][name]
}

// Synced reports whether a kind has completed its initial list.
func (c *Cache) Synced(kind string) bool {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.synced[kind]
}

// Subscribe returns a delta channel and its cancel function. The channel is
// buffered; an overflowing subscriber gets a single RESYNC instead of a
// backlog, because a slow browser must never stall the watch loops.
func (c *Cache) Subscribe() (<-chan Delta, func()) {
	c.subMu.Lock()
	defer c.subMu.Unlock()
	id := c.nextID
	c.nextID++
	s := &subscriber{ch: make(chan Delta, 256)}
	c.subs[id] = s
	return s.ch, func() {
		c.subMu.Lock()
		defer c.subMu.Unlock()
		if cur, ok := c.subs[id]; ok {
			delete(c.subs, id)
			close(cur.ch)
		}
	}
}

func (c *Cache) publish(d Delta) {
	c.subMu.Lock()
	defer c.subMu.Unlock()
	for _, s := range c.subs {
		if s.stale {
			// already owes a resync; queue that instead of piling up deltas
			select {
			case s.ch <- Delta{Type: DeltaResync, Kind: d.Kind}:
				s.stale = false
			default:
			}
			continue
		}
		select {
		case s.ch <- d:
		default:
			s.stale = true // buffer full: the subscriber re-reads the snapshot
		}
	}
}

func sleepCtx(ctx context.Context, d time.Duration) {
	select {
	case <-ctx.Done():
	case <-time.After(d):
	}
}
