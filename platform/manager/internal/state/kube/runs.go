package kube

import (
	"context"
	"fmt"
	"reflect"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/watch"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/kostiantyn-matsebora/agent-ops-operator/platform/manager/internal/state"
)

// appended is the kube backend of state.Appended[P, T]: the items live on their
// parent's object (Conversation.status.runs[] for runs), so an append is a
// read-append-CAS loop against the status subresource, retried on the
// apiserver's own optimistic-concurrency conflict. Appends and grows commute,
// so retrying is always correct, and the combined read, the listing with items
// and the feed's put carrying them are free: the items arrive with the object.
type appended[P, T any] struct {
	base
	l AppendedLayout[P, T]
}

// OpenAppended returns the state.Appended capability of an item kind on its
// parent's object, scoped to namespace.
func OpenAppended[P, T any](c client.WithWatch, namespace string, l AppendedLayout[P, T], opts ...Option) (state.Appended[P, T], error) {
	if err := l.Parent.validate(); err != nil {
		return nil, err
	}
	if l.Parent.Part != nil || l.ReadItems == nil || l.WriteItems == nil || l.Items.ID == nil || l.Items.Merge == nil {
		return nil, fmt.Errorf("kube: appended layout %q needs its parent's own object, ReadItems, WriteItems and the item declaration's ID and Merge", l.Items.Name)
	}
	return &appended[P, T]{base: newBase(c, namespace, newConfig(opts)), l: l}, nil
}

// Append adds item to the parent's history (state.Appended: takes no revision,
// never fails with a conflict). Only a caller that starved for
// statusUpdateMaxAttempts consecutive retries sees one.
func (a *appended[P, T]) Append(ctx context.Context, parent state.Key, item T) error {
	return a.mutate(ctx, parent, "append "+a.l.Items.Name+" to", func(obj client.Object) (bool, error) {
		items := append(a.l.ReadItems(obj), item)
		if a.l.MaxItems > 0 && len(items) > a.l.MaxItems {
			items = items[len(items)-a.l.MaxItems:]
		}
		a.l.WriteItems(obj, items)
		return true, nil
	})
}

// Grow applies edit to the item with identity id by the declaration's Merge. An
// edit the item already satisfies writes nothing.
func (a *appended[P, T]) Grow(ctx context.Context, parent state.Key, id string, edit T) error {
	return a.mutate(ctx, parent, "grow "+a.l.Items.Name+" of", func(obj client.Object) (bool, error) {
		items := a.l.ReadItems(obj)
		for i := range items {
			if a.l.Items.ID(&items[i]) != id {
				continue
			}
			before := items[i]
			a.l.Items.Merge(&items[i], edit)
			if sameItem(before, items[i]) {
				return false, nil
			}
			a.l.WriteItems(obj, items)
			return true, nil
		}
		return false, state.ErrNotFound
	})
}

// mutate is the read-edit-CAS loop on the parent's status. edit reports
// whether it changed the object; an unchanged object is not written.
func (a *appended[P, T]) mutate(ctx context.Context, parent state.Key, what string, edit func(client.Object) (bool, error)) error {
	for attempt := 0; attempt < statusUpdateMaxAttempts; attempt++ {
		obj := a.l.Parent.New()
		if err := a.getObject(ctx, obj, string(parent), a.l.Parent.Decl.Name); err != nil {
			return err
		}
		changed, err := edit(obj)
		if err != nil || !changed {
			return err
		}
		if err := a.client.Status().Update(ctx, obj); err != nil {
			if apierrors.IsConflict(err) {
				continue
			}
			return fmt.Errorf("kube: %s %s %q: %w", what, a.l.Parent.Decl.Name, parent, err)
		}
		return nil
	}
	return state.ErrConflict
}

// List returns the parent's items in append order, empty for a parent that has
// none or does not exist.
func (a *appended[P, T]) List(ctx context.Context, parent state.Key) ([]T, error) {
	obj := a.l.Parent.New()
	if err := a.getObject(ctx, obj, string(parent), a.l.Parent.Decl.Name); err != nil {
		return nil, err
	}
	return a.l.ReadItems(obj), nil
}

// GetWithItems serves the record and its items from one apiserver read.
func (a *appended[P, T]) GetWithItems(ctx context.Context, parent state.Key) (P, state.Revision, []T, error) {
	var zero P
	obj := a.l.Parent.New()
	if err := a.getObject(ctx, obj, string(parent), a.l.Parent.Decl.Name); err != nil {
		return zero, "", nil, err
	}
	rec, ok := a.l.Parent.Read(obj)
	if !ok {
		return zero, "", nil, state.ErrNotFound
	}
	return rec, state.Revision(obj.GetResourceVersion()), a.l.ReadItems(obj), nil
}

// ListWithItems lists the parents under one declared index, each with the items
// of the object it was read from: one list, no read per record.
func (a *appended[P, T]) ListWithItems(ctx context.Context, idx state.Index[P], value string) ([]state.WithItems[P, T], error) {
	if !a.l.Parent.Decl.Declares(idx) {
		return nil, state.ErrUnknownIndex
	}
	objs, err := a.listObjects(ctx, a.listReader(), a.l.Parent.NewList, a.l.Parent.Decl.Name)
	if err != nil {
		return nil, err
	}
	prefilter := a.l.Parent.Prefilter[idx.Name()]
	var out []state.WithItems[P, T]
	for _, obj := range objs {
		if prefilter != nil && !prefilter(obj, value) {
			continue
		}
		rec, ok := a.l.Parent.Read(obj)
		if !ok {
			continue
		}
		if v, listed := idx.Value(&rec); !listed || v != value {
			continue
		}
		out = append(out, state.WithItems[P, T]{Record: rec, Revision: state.Revision(obj.GetResourceVersion()), Items: a.l.ReadItems(obj)})
	}
	return out, nil
}

// WatchWithItems is the parent's feed with each put carrying the items as of
// that revision.
func (a *appended[P, T]) WatchWithItems(ctx context.Context, resume state.Revision) (<-chan state.Event[state.WithItems[P, T]], error) {
	return watchObjects(ctx, &a.base, a.l.Parent.NewList, resume, a.l.Parent.Decl.Name, func(ev watch.Event) []state.Event[state.WithItems[P, T]] {
		return translateObject(ev, func(obj client.Object) (state.WithItems[P, T], bool) {
			rec, ok := a.l.Parent.Read(obj)
			return state.WithItems[P, T]{Record: rec, Revision: state.Revision(obj.GetResourceVersion()), Items: a.l.ReadItems(obj)}, ok
		})
	})
}

// sameItem reports whether a grow left the item as it was.
func sameItem[T any](a, b T) bool {
	return reflect.DeepEqual(a, b)
}
