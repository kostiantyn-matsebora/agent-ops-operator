package kube

import (
	"context"
	"fmt"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/watch"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/kostiantyn-matsebora/agent-ops-operator/platform/manager/internal/state"
)

// base is what every collection shares: the client, the install's namespace
// (design D-2 of the state-store change: scope fixed at construction) and the
// reader a list is served from.
type base struct {
	client    client.WithWatch
	namespace string
	// lister answers List. Nil means the client itself (consistent).
	lister client.Reader
}

func newBase(c client.WithWatch, namespace string, cfg config) base {
	return base{client: c, namespace: namespace, lister: cfg.lister}
}

// listReader is the reader List uses.
func (b *base) listReader() client.Reader {
	if b.lister != nil {
		return b.lister
	}
	return b.client
}

func (b *base) namespacedName(name string) types.NamespacedName {
	return types.NamespacedName{Namespace: b.namespace, Name: name}
}

// listObjects lists the namespace through r and returns the items as objects.
func (b *base) listObjects(ctx context.Context, r client.Reader, newList func() client.ObjectList, what string) ([]client.Object, error) {
	list := newList()
	if err := r.List(ctx, list, client.InNamespace(b.namespace)); err != nil {
		return nil, fmt.Errorf("kube: list %s: %w", what, err)
	}
	return objectsOf(list, what)
}

// objectsOf returns the items of a typed list as the pointers into it.
func objectsOf(list client.ObjectList, what string) ([]client.Object, error) {
	items, err := meta.ExtractList(list)
	if err != nil {
		return nil, fmt.Errorf("kube: read the %s list: %w", what, err)
	}
	out := make([]client.Object, 0, len(items))
	for _, it := range items {
		obj, ok := it.(client.Object)
		if !ok {
			return nil, fmt.Errorf("kube: %s list item %T is not an object", what, it)
		}
		out = append(out, obj)
	}
	return out, nil
}

// getObject reads one object straight from the apiserver, mapping an absent
// one to state.ErrNotFound.
func (b *base) getObject(ctx context.Context, obj client.Object, name, what string) error {
	if err := b.client.Get(ctx, b.namespacedName(name), obj); err != nil {
		if apierrors.IsNotFound(err) {
			return state.ErrNotFound
		}
		return fmt.Errorf("kube: get %s %q: %w", what, name, err)
	}
	return nil
}

// writeError maps the apiserver's refusal of a write onto the contract's.
func writeError(what, name string, err error) error {
	switch {
	case apierrors.IsConflict(err):
		return state.ErrConflict
	case apierrors.IsNotFound(err):
		return state.ErrNotFound
	}
	return fmt.Errorf("kube: %s %q: %w", what, name, err)
}

// startingResourceVersion is the version a feed starts strictly after: the
// resume point, or, for an empty one, the list's own version, so "from now" is
// literal and never replays history.
func (b *base) startingResourceVersion(ctx context.Context, newList func() client.ObjectList, resume state.Revision, what string) (string, error) {
	if resume != "" {
		return string(resume), nil
	}
	list := newList()
	if err := b.client.List(ctx, list, client.InNamespace(b.namespace)); err != nil {
		return "", fmt.Errorf("kube: list %s to start watch: %w", what, err)
	}
	return list.GetResourceVersion(), nil
}

// watchObjects opens the apiserver's own watch over the kind and drains it
// through translate into a feed, until ctx is done or the watch ends. The
// returned channel closes when the feed does: an apiserver-side error is a
// terminal failure and closes it early, with no error value crossing the
// contract.
func watchObjects[X any](ctx context.Context, b *base, newList func() client.ObjectList, resume state.Revision, what string,
	translate func(watch.Event) []state.Event[X]) (<-chan state.Event[X], error) {
	rv, err := b.startingResourceVersion(ctx, newList, resume, what)
	if err != nil {
		return nil, err
	}
	w, err := b.client.Watch(ctx, newList(), client.InNamespace(b.namespace),
		&client.ListOptions{Raw: &metav1.ListOptions{ResourceVersion: rv, AllowWatchBookmarks: true}})
	if err != nil {
		return nil, fmt.Errorf("kube: watch %s: %w", what, err)
	}
	events := make(chan state.Event[X])
	go drainWatch(ctx, w, events, translate)
	return events, nil
}

func drainWatch[X any](ctx context.Context, w watch.Interface, events chan<- state.Event[X], translate func(watch.Event) []state.Event[X]) {
	defer close(events)
	defer w.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case ev, ok := <-w.ResultChan():
			// An apiserver-side watch.Error is terminal: returning at once,
			// rather than waiting for the channel to close, makes the early
			// close immediate rather than best-effort.
			if !ok || ev.Type == watch.Error {
				return
			}
			for _, out := range translate(ev) {
				select {
				case events <- out:
				case <-ctx.Done():
					return
				}
			}
		}
	}
}

// translateObject maps one apiserver watch event to the contract's, trusting
// the raw watch payload and never re-reading the object. This forwards every
// real write exactly once, in order, which is what the "no gap" half of the
// contract rests on: an update lands as two apiserver calls (the main object,
// then the status), so a consumer may see a spec change with the prior status
// still attached as its own, genuinely-once-true state. A design that
// re-read the object per notification was tried and reverted: under a burst of
// writes a slow consumer's catch-up collapsed many real states into repeats of
// whatever was current by the time each was processed, a real gap hiding
// behind the "duplicates permitted" sentence.
//
// read maps a live object to the feed's payload; ok false means the object
// carries none and the event is dropped.
func translateObject[X any](ev watch.Event, read func(client.Object) (X, bool)) []state.Event[X] {
	obj, ok := ev.Object.(client.Object)
	if !ok {
		return nil
	}
	rev := state.Revision(obj.GetResourceVersion())
	switch ev.Type {
	case watch.Added, watch.Modified:
		rec, ok := read(obj)
		if !ok {
			return nil
		}
		return []state.Event[X]{{Type: state.EventPut, Key: state.Key(obj.GetName()), Revision: rev, Record: rec, Resume: rev}}
	case watch.Deleted:
		return []state.Event[X]{{Type: state.EventDelete, Key: state.Key(obj.GetName()), Resume: rev}}
	case watch.Bookmark:
		// Requested so an idle watch's resume point keeps moving; the apiserver
		// sends one carrying only a resourceVersion.
		if rev == "" {
			return nil
		}
		return []state.Event[X]{{Type: state.EventBookmark, Resume: rev}}
	}
	return nil
}
