package kube

import (
	"context"
	"fmt"
	"time"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/kostiantyn-matsebora/agent-ops-operator/platform/manager/internal/state"
)

// ExpiresAtAnnotation stores a record's expiry. There is no CRD schema change
// in this backend, so the backend-agnostic expiry the contract declares is
// carried as plain ObjectMeta, not a typed field.
const ExpiresAtAnnotation = "agentops.dev/state-expires-at"

// Expiring is the kube backend of state.Expiring[T]: a collection whose records
// carry an expiry, with the sweep that removes them. Run is not started by
// OpenExpiring (an unsolicited goroutine would leak in every test that merely
// constructs one); the caller that wires the backend into the manager starts it.
type Expiring[T any] struct {
	*collection[T]
	sweepInterval time.Duration
}

// OpenExpiring is Open for a kind whose declaration carries an expiry. A
// record past it is invisible to Get and List, and the sweep deletes it.
func OpenExpiring[T any](c client.WithWatch, namespace string, l Layout[T], opts ...Option) (*Expiring[T], error) {
	if err := l.validate(); err != nil {
		return nil, err
	}
	if l.Decl.Expiry == nil || l.Part != nil {
		return nil, fmt.Errorf("kube: layout %q is not an expiring collection: it needs an expiry and its own object", l.Decl.Name)
	}
	cfg := newConfig(opts)
	return &Expiring[T]{collection: newCollection(newBase(c, namespace, cfg), l), sweepInterval: cfg.sweepInterval}, nil
}

// Expiry reads rec's expiry through the declaration, the instant the store
// itself applies.
func (e *Expiring[T]) Expiry(rec T) *time.Time {
	return e.l.Decl.Expiry(&rec)
}

// Run starts the expiry sweep: every sweep interval, list the namespace's
// objects of the kind and delete every one whose expiry has passed. The
// resulting delete flows through the ordinary apiserver watch like any other,
// so a Watch consumer sees the delete event the contract requires. The delete
// carries the UID and revision it read as preconditions, so a write that
// landed since keeps the record.
func (e *Expiring[T]) Run(ctx context.Context) {
	// One sweep at start covers a record a previous process (or another
	// replica) stamped; after it only an armed tick lists the namespace.
	e.expiryArmed.Store(true)
	ticker := time.NewTicker(e.sweepInterval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			if e.expiryArmed.Swap(false) {
				e.sweepExpired(ctx)
			}
		}
	}
}

// sweepExpired deletes every expired record. The caller has already lowered
// expiryArmed; it is raised again here when a record still carries a future
// expiry, or when the list failed or a delete lost a race, so the next tick
// looks again.
func (e *Expiring[T]) sweepExpired(ctx context.Context) {
	// Best-effort: a transient list failure is indistinguishable from "nothing
	// to sweep this tick" to any caller, and the next tick tries again.
	objs, err := e.listObjects(ctx, e.client, e.l.NewList, e.l.Decl.Name)
	if err != nil {
		e.expiryArmed.Store(true)
		return
	}
	now := time.Now()
	for _, obj := range objs {
		rec, ok := e.l.Read(obj)
		if !ok {
			continue
		}
		at := e.expiry(&rec)
		switch {
		case at == nil:
		case at.Before(now):
			uid, rv := obj.GetUID(), obj.GetResourceVersion()
			err := e.client.Delete(ctx, obj, client.Preconditions{UID: &uid, ResourceVersion: &rv})
			if err != nil && !apierrors.IsNotFound(err) {
				e.expiryArmed.Store(true) // a conflict or a failure: look again
			}
		default:
			e.expiryArmed.Store(true)
		}
	}
}

// expiresAtFromAnnotations reads the expiry annotation, nil for none or for a
// value this backend did not write.
func expiresAtFromAnnotations(annotations map[string]string) *time.Time {
	v, ok := annotations[ExpiresAtAnnotation]
	if !ok || v == "" {
		return nil
	}
	t, err := time.Parse(time.RFC3339Nano, v)
	if err != nil {
		// A malformed value never written by this backend. Treat it the
		// conservative way every other unparsable-timestamp path in this
		// codebase does: as carrying no usable information, rather than
		// panicking on data this process did not itself produce.
		return nil
	}
	t = t.UTC()
	return &t
}

// setExpiresAt writes or clears the expiry annotation to match at, so the
// annotation always reflects the caller's record exactly, never merged with
// whatever was there before.
func setExpiresAt(meta *metav1.ObjectMeta, at *time.Time) {
	if at == nil {
		delete(meta.Annotations, ExpiresAtAnnotation)
		return
	}
	if meta.Annotations == nil {
		meta.Annotations = map[string]string{}
	}
	meta.Annotations[ExpiresAtAnnotation] = at.UTC().Format(time.RFC3339Nano)
}

var _ state.Expiring[struct{}] = (*Expiring[struct{}])(nil)
