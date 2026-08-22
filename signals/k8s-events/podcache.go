package main

import (
	"context"
	"encoding/json"
	"errors"
	"net/url"
	"sync"
)

// The object cache: a trimmed, read-only list/watch view of pods and
// replicasets. It exists to answer three questions the Events stream cannot:
//
//	workload    which controller owns this pod (Pod -> ReplicaSet -> Deployment)
//	health      is this object still unhealthy at the end of a dwell window
//	ownership   is this object agent-ops' own machinery
//
// It caches FIELDS, not objects: a large cluster's pod list is mostly spec, and
// none of it is needed here. Whole-object caching is what makes an informer
// expensive, and this adapter has no reason to pay that.
//
// Replicasets are cached for the SECOND owner hop only, so nothing but their
// owner references and identity is kept.

// ownerRef is the slice of ownerReferences that matters: who controls this.
type ownerRef struct {
	Kind       string `json:"kind"`
	Name       string `json:"name"`
	Controller *bool  `json:"controller,omitempty"`
}

// objectInfo is everything the cache keeps about one object.
type objectInfo struct {
	Kind      string
	Namespace string
	Name      string
	Labels    map[string]string
	Owner     *ownerRef
	Node      string

	// health, meaningful for pods only
	Phase          string
	Ready          bool
	WaitingReasons []string
}

// podObject is the subset of a core/v1 Pod that is decoded. Anything absent
// here is never read, never allocated, and never held.
type podObject struct {
	Metadata struct {
		Name            string            `json:"name"`
		Namespace       string            `json:"namespace"`
		Labels          map[string]string `json:"labels"`
		OwnerReferences []ownerRef        `json:"ownerReferences"`
	} `json:"metadata"`
	Spec struct {
		NodeName string `json:"nodeName"`
	} `json:"spec"`
	Status struct {
		Phase      string `json:"phase"`
		Conditions []struct {
			Type   string `json:"type"`
			Status string `json:"status"`
		} `json:"conditions"`
		ContainerStatuses []struct {
			State struct {
				Waiting *struct {
					Reason string `json:"reason"`
				} `json:"waiting"`
			} `json:"state"`
		} `json:"containerStatuses"`
		InitContainerStatuses []struct {
			State struct {
				Waiting *struct {
					Reason string `json:"reason"`
				} `json:"waiting"`
			} `json:"state"`
		} `json:"initContainerStatuses"`
	} `json:"status"`
}

func (p *podObject) info() *objectInfo {
	oi := &objectInfo{
		Kind:      "Pod",
		Namespace: p.Metadata.Namespace,
		Name:      p.Metadata.Name,
		Labels:    p.Metadata.Labels,
		Owner:     controllerOf(p.Metadata.OwnerReferences),
		Node:      p.Spec.NodeName,
		Phase:     p.Status.Phase,
	}
	for _, c := range p.Status.Conditions {
		if c.Type == "Ready" {
			oi.Ready = c.Status == "True"
		}
	}
	for _, cs := range p.Status.ContainerStatuses {
		if cs.State.Waiting != nil && cs.State.Waiting.Reason != "" {
			oi.WaitingReasons = append(oi.WaitingReasons, cs.State.Waiting.Reason)
		}
	}
	for _, cs := range p.Status.InitContainerStatuses {
		if cs.State.Waiting != nil && cs.State.Waiting.Reason != "" {
			oi.WaitingReasons = append(oi.WaitingReasons, cs.State.Waiting.Reason)
		}
	}
	return oi
}

// replicaSetObject is the second owner hop and nothing more.
type replicaSetObject struct {
	Metadata struct {
		Name            string            `json:"name"`
		Namespace       string            `json:"namespace"`
		Labels          map[string]string `json:"labels"`
		OwnerReferences []ownerRef        `json:"ownerReferences"`
	} `json:"metadata"`
}

func (r *replicaSetObject) info() *objectInfo {
	return &objectInfo{
		Kind:      "ReplicaSet",
		Namespace: r.Metadata.Namespace,
		Name:      r.Metadata.Name,
		Labels:    r.Metadata.Labels,
		Owner:     controllerOf(r.Metadata.OwnerReferences),
	}
}

// controllerOf returns the CONTROLLING owner reference. A pod may carry several
// ownerReferences; only the controller determines the workload.
func controllerOf(refs []ownerRef) *ownerRef {
	for i := range refs {
		if refs[i].Controller != nil && *refs[i].Controller {
			r := refs[i]
			return &r
		}
	}
	return nil
}

// objectCache holds the trimmed view, keyed "namespace/Kind/name".
type objectCache struct {
	mu      sync.RWMutex
	objects map[string]*objectInfo
	// synced reports whether at least one successful list has landed. Before
	// that, "unknown" means "not loaded yet", which callers must not read as
	// "does not exist".
	synced bool
}

func newObjectCache() *objectCache {
	return &objectCache{objects: map[string]*objectInfo{}}
}

func cacheKey(namespace, kind, name string) string {
	return namespace + "/" + kind + "/" + name
}

func (c *objectCache) put(oi *objectInfo) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.objects[cacheKey(oi.Namespace, oi.Kind, oi.Name)] = oi
}

func (c *objectCache) drop(namespace, kind, name string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	delete(c.objects, cacheKey(namespace, kind, name))
}

func (c *objectCache) markSynced() {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.synced = true
}

// Synced reports whether the cache has completed at least one list.
func (c *objectCache) Synced() bool {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.synced
}

// Get returns the cached object, and whether the cache can speak to it at all.
// The second return is false before the first list completes, so an unsynced
// cache never reports an object as absent.
func (c *objectCache) Get(namespace, kind, name string) (*objectInfo, bool) {
	c.mu.RLock()
	defer c.mu.RUnlock()
	if !c.synced {
		return nil, false
	}
	oi, ok := c.objects[cacheKey(namespace, kind, name)]
	if !ok {
		// Synced and absent IS an answer for the kinds we track: the object is
		// gone. For kinds we do not track we cannot say.
		return nil, c.tracks(kind)
	}
	return oi, true
}

// tracks reports whether this cache holds the given kind at all.
func (c *objectCache) tracks(kind string) bool {
	return kind == "Pod" || kind == "ReplicaSet"
}

// OwnedByAgentOps implements self-exclusion mechanism 2: an agent-ops label on
// the object, or an owner chain reaching a Conversation.
func (c *objectCache) OwnedByAgentOps(namespace, kind, name string) (bool, bool) {
	oi, known := c.Get(namespace, kind, name)
	if !known {
		return false, false
	}
	if oi == nil {
		return false, true // synced and absent: nothing to be owned
	}
	if isOwnedLabels(oi.Labels) {
		return true, true
	}
	// Walk the controller chain. A runtime pod's controller IS the Conversation
	// CR, which is not a kind this cache tracks — so the check is on the owner
	// REFERENCE, which needs no lookup.
	for hop, cur := 0, oi; hop < 3 && cur != nil && cur.Owner != nil; hop++ {
		if cur.Owner.Kind == "Conversation" {
			return true, true
		}
		next, ok := c.Get(cur.Namespace, cur.Owner.Kind, cur.Owner.Name)
		if !ok || next == nil {
			break
		}
		if isOwnedLabels(next.Labels) {
			return true, true
		}
		cur = next
	}
	return false, true
}

// Workload resolves the controller that owns an object, following the chain to
// its top (Pod -> ReplicaSet -> Deployment). An object with no controller is
// its own workload.
//
// Resolution is by OWNER REFERENCE only. Deriving a workload by stripping
// segments off a pod name breaks on StatefulSets (app-0), DaemonSets
// (app-xk2p9), bare pods, and any name whose own segments look like a hash —
// so it is not done here, at any point, for any kind.
func (c *objectCache) Workload(namespace, kind, name string) (string, string, bool) {
	oi, known := c.Get(namespace, kind, name)
	if !known || oi == nil {
		return "", "", false
	}
	curKind, curName, owner := oi.Kind, oi.Name, oi.Owner
	for hop := 0; hop < 3 && owner != nil; hop++ {
		curKind, curName = owner.Kind, owner.Name
		next, ok := c.Get(namespace, owner.Kind, owner.Name)
		if !ok || next == nil {
			break // top of the chain we can see
		}
		owner = next.Owner
	}
	return curKind, curName, true
}

// ---- the watcher ------------------------------------------------------------

// cacheWatcher keeps an objectCache current for one resource in one scope.
type cacheWatcher struct {
	kube  *Kube
	cache *objectCache
	// kind is the object kind stored ("Pod", "ReplicaSet").
	kind string
	// pathFor builds the collection path for a namespace scope.
	pathFor func(namespace string) string
	// decode turns one raw object into cache form.
	decode func(json.RawMessage) (*objectInfo, error)
	// onError reports a terminal permission failure, which must be named
	// rather than retried silently.
	onError func(error)
}

func podsPath(namespace string) string {
	if namespace == "" {
		return "/api/v1/pods"
	}
	return "/api/v1/namespaces/" + url.PathEscape(namespace) + "/pods"
}

func replicaSetsPath(namespace string) string {
	if namespace == "" {
		return "/apis/apps/v1/replicasets"
	}
	return "/apis/apps/v1/namespaces/" + url.PathEscape(namespace) + "/replicasets"
}

func decodePod(raw json.RawMessage) (*objectInfo, error) {
	var p podObject
	if err := json.Unmarshal(raw, &p); err != nil {
		return nil, err
	}
	return p.info(), nil
}

func decodeReplicaSet(raw json.RawMessage) (*objectInfo, error) {
	var r replicaSetObject
	if err := json.Unmarshal(raw, &r); err != nil {
		return nil, err
	}
	return r.info(), nil
}

// run keeps one scope current: list, then watch, relisting on expiry. It
// mirrors watchScope's shape deliberately — same relist contract, same
// cancellation semantics.
func (w *cacheWatcher) run(ctx context.Context, scope string) {
	for ctx.Err() == nil {
		var list struct {
			Items []json.RawMessage `json:"items"`
		}
		rv, err := w.kube.ListInto(ctx, w.pathFor(scope), &list)
		if err != nil {
			if ctx.Err() != nil {
				return
			}
			if IsForbidden(err) && w.onError != nil {
				// Not retryable by waiting: the grant is external and absent.
				w.onError(err)
			}
			sleepCtx(ctx, relistBackoff)
			continue
		}
		for _, raw := range list.Items {
			if oi, err := w.decode(raw); err == nil {
				w.cache.put(oi)
			}
		}
		w.cache.markSynced()

		err = w.kube.WatchFrames(ctx, w.pathFor(scope), rv, func(frameType string, obj json.RawMessage) {
			oi, decErr := w.decode(obj)
			if decErr != nil {
				return
			}
			if frameType == "DELETED" {
				w.cache.drop(oi.Namespace, oi.Kind, oi.Name)
				return
			}
			w.cache.put(oi)
		})
		switch {
		case ctx.Err() != nil:
			return
		case errors.Is(err, ErrWatchExpired):
			// expected: relist immediately, no backoff
		case err != nil:
			sleepCtx(ctx, relistBackoff)
		}
	}
}
