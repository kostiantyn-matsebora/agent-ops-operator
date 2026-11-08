package kube

import (
	"context"
	"fmt"
	"slices"
	"sync/atomic"
	"time"

	"k8s.io/apimachinery/pkg/api/equality"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/watch"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/kostiantyn-matsebora/agent-ops-operator/platform/manager/internal/state"
)

// Open returns the state.Collection of a kind, scoped to namespace (design D-2
// of the state-store change: scope fixed at construction). c is any
// client.WithWatch: production wires a client.NewWithWatch, a test wires one
// directly against envtest's rest.Config. A layout that cannot serve its kind
// is refused, naming it.
func Open[T any](c client.WithWatch, namespace string, l Layout[T], opts ...Option) (state.Collection[T], error) {
	if err := l.validate(); err != nil {
		return nil, err
	}
	cfg := newConfig(opts)
	if l.Part != nil {
		return newPartCollection(newBase(c, namespace, cfg), l), nil
	}
	return newCollection(newBase(c, namespace, cfg), l), nil
}

// collection is the kube backend of a record that is the whole of an object.
type collection[T any] struct {
	base
	l Layout[T]
	// expiryArmed says the next sweep has something to look for. A sweep lists
	// the whole namespace, and a kind whose records are not yet terminal sets
	// no expiry, so an idle tick would be a pure apiserver cost. It is raised
	// by a write carrying an expiry, by a read or watch event that meets one,
	// and at the sweep's start (a record from a previous process); a sweep that
	// finds none left lowers it.
	expiryArmed atomic.Bool
}

func newCollection[T any](b base, l Layout[T]) *collection[T] {
	return &collection[T]{base: b, l: l}
}

// expiry reads rec's expiry through the declaration, nil for a kind that
// declares none.
func (c *collection[T]) expiry(rec *T) *time.Time {
	if c.l.Decl.Expiry == nil {
		return nil
	}
	return c.l.Decl.Expiry(rec)
}

// arm raises expiryArmed for a record that carries an expiry.
func (c *collection[T]) arm(rec T) {
	if c.expiry(&rec) != nil {
		c.expiryArmed.Store(true)
	}
}

// visible maps obj to its record and reports whether it is visible: held by
// the object and not past its expiry. An expired record is invisible before it
// is gone.
func (c *collection[T]) visible(obj client.Object, now time.Time) (T, bool) {
	rec, ok := c.l.Read(obj)
	if !ok {
		return rec, false
	}
	if at := c.expiry(&rec); at != nil {
		c.expiryArmed.Store(true)
		if at.Before(now) {
			var zero T
			return zero, false
		}
	}
	return rec, true
}

// getVisible reads the object and applies the expiry rule of Get.
func (c *collection[T]) getVisible(ctx context.Context, key state.Key) (client.Object, T, error) {
	obj := c.l.New()
	var zero T
	if err := c.getObject(ctx, obj, string(key), c.l.Decl.Name); err != nil {
		return nil, zero, err
	}
	rec, ok := c.visible(obj, time.Now())
	if !ok {
		return nil, zero, state.ErrNotFound
	}
	return obj, rec, nil
}

// Get reads straight through to the apiserver (design D-4).
func (c *collection[T]) Get(ctx context.Context, key state.Key) (T, state.Revision, error) {
	obj, rec, err := c.getVisible(ctx, key)
	if err != nil {
		var zero T
		return zero, "", err
	}
	return rec, state.Revision(obj.GetResourceVersion()), nil
}

// Create writes a new object. The main Create call persists spec and metadata
// only: the apiserver drops anything under .status on this endpoint when a
// status subresource is in play. A second, explicit write through the status
// subresource lands whatever status the record carries, the same two-call
// shape Update uses. The apiserver's create is the single arbiter of a
// duplicate: AlreadyExists is state.ErrAlreadyExists, with no lookup first.
func (c *collection[T]) Create(ctx context.Context, key state.Key, rec T) (state.Revision, error) {
	obj := c.l.New()
	obj.SetName(string(key))
	obj.SetNamespace(c.namespace)
	c.arm(rec)
	c.l.Init(obj, rec)
	if err := c.createOrReplaceExpired(ctx, key, obj); err != nil {
		return "", err
	}
	if c.l.WriteStatus != nil {
		before := obj.DeepCopyObject()
		c.l.WriteStatus(obj, rec)
		if !equality.Semantic.DeepEqual(before, obj) {
			if err := c.client.Status().Update(ctx, obj); err != nil {
				return "", fmt.Errorf("kube: create %s %q status: %w", c.l.Decl.Name, key, err)
			}
		}
	}
	return state.Revision(obj.GetResourceVersion()), nil
}

// createOrReplaceExpired creates obj, treating an existing object whose expiry
// has passed as already gone: it reads as not found everywhere, so a re-send
// inside the sweep period must not be acked as a duplicate of a record nobody
// can see. The stale object is deleted under the UID and revision just read
// and the create is tried once more; losing either race is a duplicate, not a
// retry.
func (c *collection[T]) createOrReplaceExpired(ctx context.Context, key state.Key, obj client.Object) error {
	err := c.client.Create(ctx, obj)
	if err == nil {
		return nil
	}
	if !apierrors.IsAlreadyExists(err) {
		return fmt.Errorf("kube: create %s %q: %w", c.l.Decl.Name, key, err)
	}
	if c.l.Decl.Expiry == nil {
		return state.ErrAlreadyExists
	}
	existing := c.l.New()
	if gerr := c.client.Get(ctx, c.namespacedName(string(key)), existing); gerr != nil || !c.isExpired(existing) {
		return state.ErrAlreadyExists
	}
	uid, rv := existing.GetUID(), existing.GetResourceVersion()
	if derr := c.client.Delete(ctx, existing, client.Preconditions{UID: &uid, ResourceVersion: &rv}); derr != nil && !apierrors.IsNotFound(derr) {
		return state.ErrAlreadyExists
	}
	obj.SetResourceVersion("")
	if err := c.client.Create(ctx, obj); err != nil {
		if apierrors.IsAlreadyExists(err) {
			return state.ErrAlreadyExists
		}
		return fmt.Errorf("kube: create %s %q: %w", c.l.Decl.Name, key, err)
	}
	return nil
}

// isExpired reports whether obj's record is past its expiry.
func (c *collection[T]) isExpired(obj client.Object) bool {
	rec, ok := c.l.Read(obj)
	if !ok {
		return false
	}
	at := c.expiry(&rec)
	return at != nil && at.Before(time.Now())
}

// Update writes rec in place of the record at rev (design D-2's get-modify-put
// CAS).
//
// THE ONE PLACE THIS BACKEND'S CAS IS NOT ONE ATOMIC APISERVER CALL. The
// contract's record carries both spec-shaped and status-shaped fields in one
// flat struct, but the apiserver splits a write into two REST calls, the main
// object endpoint for spec and metadata and the status subresource for status,
// each bumping its own resourceVersion. Resolved pragmatically:
//
//  1. The stored object is read first. Its resourceVersion must equal rev, and
//     the first write carries it as the precondition: the CAS-checked call. A
//     mismatch at either point is ErrConflict and the second call never runs.
//     Writing onto the object just read, rather than a bare one, leaves every
//     piece of metadata the record does not model as the apiserver holds it.
//  2. Having passed that check, the second write races nothing the caller asked
//     this call to detect. It is retried on its own conflict, but only while
//     the re-read status still equals the one the first write returned apart
//     from what the layout's StatusDrift tolerates (a run append, which
//     commutes). Anything else is a competing writer, and re-applying the
//     caller's whole status would overwrite it: ErrConflict.
//
// The revision handed back is the final resourceVersion, after both calls.
//
// A record marked for deletion (design D-11) is written the same way, with two
// differences. A finalizer the stored object does not already hold is refused
// (ErrDeleting), as the apiserver refuses it, up front so the refusal is a
// contract error. And an Update that leaves the finalizers empty writes status
// first: the main write that drops the last finalizer deletes the object, and
// the status subresource of a deleted object is gone.
func (c *collection[T]) Update(ctx context.Context, key state.Key, rev state.Revision, rec T) (state.Revision, error) {
	c.arm(rec)
	obj := c.l.New()
	if err := c.getObject(ctx, obj, string(key), c.l.Decl.Name); err != nil {
		return "", err
	}
	if c.isExpired(obj) {
		return "", state.ErrNotFound
	}
	if obj.GetResourceVersion() != string(rev) {
		return "", state.ErrConflict
	}
	if c.l.Finalizers != nil {
		deleting := !obj.GetDeletionTimestamp().IsZero()
		next := c.l.Finalizers(rec)
		if deleting && addsFinalizer(obj.GetFinalizers(), next) {
			return "", state.ErrDeleting
		}
		if deleting && len(next) == 0 {
			return c.updateReleasingLastFinalizer(ctx, key, obj, rec)
		}
	}
	if c.l.StatusFirst {
		return c.updateStatusFirst(ctx, key, obj, rec)
	}
	c.l.WriteMain(obj, rec)
	if err := c.client.Update(ctx, obj); err != nil {
		return "", writeError(writeUpdate+c.l.Decl.Name, string(key), err)
	}
	if c.l.WriteStatus == nil {
		return state.Revision(obj.GetResourceVersion()), nil
	}
	// obj now carries the apiserver's response to the main write: the real
	// current status (the items of an Appended kind included, which no Update
	// ever touches), since the main endpoint leaves .status exactly as it was.
	return c.updateStatusRetrying(ctx, key, obj, rec)
}

// writeUpdate prefixes the operation named in a write error.
const writeUpdate = "update "

// updateStatusRetrying writes the record's status over obj, re-reading and
// re-applying on a conflict while nothing but what StatusDrift tolerates has
// changed underneath.
func (c *collection[T]) updateStatusRetrying(ctx context.Context, key state.Key, obj client.Object, rec T) (state.Revision, error) {
	base := obj.DeepCopyObject().(client.Object)
	c.l.WriteStatus(obj, rec)
	for attempt := 0; attempt < statusUpdateMaxAttempts; attempt++ {
		err := c.client.Status().Update(ctx, obj)
		if err == nil {
			return state.Revision(obj.GetResourceVersion()), nil
		}
		if !apierrors.IsConflict(err) {
			return "", fmt.Errorf("kube: update %s %q status: %w", c.l.Decl.Name, key, err)
		}
		if err := c.client.Get(ctx, c.namespacedName(string(key)), obj); err != nil {
			return "", fmt.Errorf("kube: re-read %s %q before status retry: %w", c.l.Decl.Name, key, err)
		}
		// The caller's status was computed from base. If anything the layout
		// does not tolerate changed it since, re-applying the caller's whole
		// status would silently overwrite that writer.
		if c.l.StatusDrift(base, obj) {
			return "", state.ErrConflict
		}
		c.l.WriteStatus(obj, rec)
	}
	return "", fmt.Errorf("kube: update %s %q status: exhausted %d attempts on conflict", c.l.Decl.Name, key, statusUpdateMaxAttempts)
}

// updateStatusFirst is Update for a kind that writes its status first, under
// the revision the caller read: that write is the compare-and-swap. The
// metadata is written second, only when it changed; a crash between the two
// leaves a terminal record without an expiry, which the consumer's next pass
// stamps. The reverse order could leave a live record carrying a future expiry.
func (c *collection[T]) updateStatusFirst(ctx context.Context, key state.Key, obj client.Object, rec T) (state.Revision, error) {
	c.l.WriteStatus(obj, rec)
	if err := c.client.Status().Update(ctx, obj); err != nil {
		return "", writeError(writeUpdate+c.l.Decl.Name+" status", string(key), err)
	}
	return c.settleMain(ctx, key, obj, rec)
}

// settleMain brings the metadata the record decides to the record's value and
// returns the final revision. The status write has already won the
// compare-and-swap, so a conflict here is a writer touching only metadata and
// is retried against a fresh read.
func (c *collection[T]) settleMain(ctx context.Context, key state.Key, obj client.Object, rec T) (state.Revision, error) {
	for attempt := 0; attempt < statusUpdateMaxAttempts; attempt++ {
		before := obj.DeepCopyObject()
		c.l.WriteMain(obj, rec)
		if equality.Semantic.DeepEqual(before, obj) {
			return state.Revision(obj.GetResourceVersion()), nil
		}
		err := c.client.Update(ctx, obj)
		if err == nil {
			return state.Revision(obj.GetResourceVersion()), nil
		}
		if !apierrors.IsConflict(err) {
			return "", writeError(writeUpdate+c.l.Decl.Name+" metadata", string(key), err)
		}
		if err := c.client.Get(ctx, c.namespacedName(string(key)), obj); err != nil {
			return "", writeError("re-read "+c.l.Decl.Name+" before metadata retry", string(key), err)
		}
	}
	return "", fmt.Errorf("kube: update %s %q metadata: exhausted %d attempts on conflict", c.l.Decl.Name, key, statusUpdateMaxAttempts)
}

// updateReleasingLastFinalizer is Update for a record marked for deletion whose
// new finalizers are empty: status first (the CAS precondition rides on obj's
// resourceVersion, already checked equal to rev), then the main write that
// drops the finalizer and so deletes the object. The main write is retried on a
// conflict, since the status write has landed and must not be re-asked of the
// caller.
func (c *collection[T]) updateReleasingLastFinalizer(ctx context.Context, key state.Key, obj client.Object, rec T) (state.Revision, error) {
	if c.l.WriteStatus != nil {
		c.l.WriteStatus(obj, rec)
		if err := c.client.Status().Update(ctx, obj); err != nil {
			return "", writeError(writeUpdate+c.l.Decl.Name+" status", string(key), err)
		}
	}
	for attempt := 0; attempt < statusUpdateMaxAttempts; attempt++ {
		c.l.WriteMain(obj, rec)
		err := c.client.Update(ctx, obj)
		if err == nil || apierrors.IsNotFound(err) {
			return state.Revision(obj.GetResourceVersion()), nil
		}
		if !apierrors.IsConflict(err) {
			return "", fmt.Errorf("kube: release %s %q: %w", c.l.Decl.Name, key, err)
		}
		if err := c.client.Get(ctx, c.namespacedName(string(key)), obj); err != nil {
			if apierrors.IsNotFound(err) {
				return state.Revision(obj.GetResourceVersion()), nil
			}
			return "", fmt.Errorf("kube: re-read %s %q before release retry: %w", c.l.Decl.Name, key, err)
		}
	}
	return "", fmt.Errorf("kube: release %s %q: exhausted %d attempts on conflict", c.l.Decl.Name, key, statusUpdateMaxAttempts)
}

// addsFinalizer reports whether next holds a finalizer cur does not.
func addsFinalizer(cur, next []string) bool {
	for _, f := range next {
		if !slices.Contains(cur, f) {
			return true
		}
	}
	return false
}

// Delete requests the object's deletion. The apiserver's own
// mark-for-deletion is the contract's marker: with a finalizer held the object
// stays, deletionTimestamp set once and kept by a repeated Delete, and the
// watch carries it as a modification; without one it is removed at once. Not
// an error when the key is already absent.
func (c *collection[T]) Delete(ctx context.Context, key state.Key) error {
	obj := c.l.New()
	obj.SetName(string(key))
	obj.SetNamespace(c.namespace)
	if err := c.client.Delete(ctx, obj); err != nil && !apierrors.IsNotFound(err) {
		return fmt.Errorf("kube: delete %s %q: %w", c.l.Decl.Name, key, err)
	}
	return nil
}

// DeleteAt is Delete under the revision the caller read. The delete carries
// the UID and resourceVersion just read as preconditions, so a record
// rewritten since is refused with the conflict error and left intact, and one
// replaced under the same name is not taken with it. An absent record is
// not found. Finalizer semantics are Delete's.
func (c *collection[T]) DeleteAt(ctx context.Context, key state.Key, rev state.Revision) error {
	obj, _, err := c.getVisible(ctx, key)
	if err != nil {
		return err
	}
	if obj.GetResourceVersion() != string(rev) {
		return state.ErrConflict
	}
	uid, rv := obj.GetUID(), obj.GetResourceVersion()
	if err := c.client.Delete(ctx, obj, client.Preconditions{UID: &uid, ResourceVersion: &rv}); err != nil {
		return writeError("delete "+c.l.Decl.Name, string(key), err)
	}
	return nil
}

// List serves every declared index with one list of the namespace, filtered in
// Go. The list comes from the apiserver unless WithCachedLists gave the
// collection a cache, which production does: a full list of a live install
// (552 conversations, 6 MB) took over half a second, and closing one
// conversation walks the child lookup twice.
func (c *collection[T]) List(ctx context.Context, idx state.Index[T], value string) ([]T, error) {
	if !c.l.Decl.Declares(idx) {
		return nil, state.ErrUnknownIndex
	}
	objs, err := c.listObjects(ctx, c.listReader(), c.l.NewList, c.l.Decl.Name)
	if err != nil {
		return nil, err
	}
	return selectRecords(objs, c.l.Prefilter[idx.Name()], value, idx, func(obj client.Object) (T, bool) {
		return c.visible(obj, time.Now())
	}), nil
}

// selectRecords maps the objects an index lists under value. prefilter, when
// the layout has one, spares the mapping of every other object.
func selectRecords[T any](objs []client.Object, prefilter func(client.Object, string) bool, value string,
	idx state.Index[T], visible func(client.Object) (T, bool)) []T {
	var out []T
	for _, obj := range objs {
		if prefilter != nil && !prefilter(obj, value) {
			continue
		}
		rec, ok := visible(obj)
		if !ok {
			continue
		}
		if v, listed := idx.Value(&rec); listed && v == value {
			out = append(out, rec)
		}
	}
	return out
}

// Watch delivers an at-least-once feed of events over the namespace's objects
// of the kind, via the apiserver's own watch.
func (c *collection[T]) Watch(ctx context.Context, resume state.Revision) (<-chan state.Event[T], error) {
	return watchObjects(ctx, &c.base, c.l.NewList, resume, c.l.Decl.Name, func(ev watch.Event) []state.Event[T] {
		if obj, ok := ev.Object.(client.Object); ok {
			c.armObject(obj)
		}
		return translateObject(ev, c.l.Read)
	})
}

// armObject raises expiryArmed when a watched object carries an expiry.
func (c *collection[T]) armObject(obj client.Object) {
	if c.l.Decl.Expiry == nil {
		return
	}
	if rec, ok := c.l.Read(obj); ok && c.expiry(&rec) != nil {
		c.expiryArmed.Store(true)
	}
}

var _ state.Collection[struct{}] = (*collection[struct{}])(nil)
