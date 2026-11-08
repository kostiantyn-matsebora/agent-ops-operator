package kube

import (
	"context"
	"errors"
	"fmt"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/watch"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/kostiantyn-matsebora/agent-ops-operator/platform/manager/internal/state"
)

// partCollection is the kube backend of a record that is one part of an object
// that exists regardless: a source's cooldown window, its receipt counters, an
// adapter's value. Create, Update and Delete are merge patches of that part,
// and the revision is the object's own, so a record is refused as conflicting
// when anything else moved the object since the caller read it, which
// state.Mutate answers by reading again.
type partCollection[T any] struct {
	base
	l Layout[T]
}

func newPartCollection[T any](b base, l Layout[T]) *partCollection[T] {
	return &partCollection[T]{base: b, l: l}
}

// fetch reads the object that carries key and the record on it. A key that
// names no object is not found, as is an object holding none.
func (p *partCollection[T]) fetch(ctx context.Context, key state.Key) (obj client.Object, field string, rec T, ok bool, err error) {
	name, field, err := p.l.Part.Locate(key)
	if err != nil {
		return nil, "", rec, false, err
	}
	obj = p.l.New()
	if err := p.getObject(ctx, obj, name, p.l.Decl.Name); err != nil {
		return nil, "", rec, false, err
	}
	rec, ok = p.l.Part.Read(obj, field)
	return obj, field, rec, ok, nil
}

// patch writes the difference between orig and obj as a merge patch of the
// part, through the status subresource when the part lives there.
func (p *partCollection[T]) patch(ctx context.Context, obj, orig client.Object, optimistic bool) error {
	var patch client.Patch
	if optimistic {
		patch = client.MergeFromWithOptions(orig, client.MergeFromWithOptimisticLock{})
	} else {
		patch = client.MergeFrom(orig)
	}
	if p.l.Part.Status {
		return p.client.Status().Patch(ctx, obj, patch)
	}
	return p.client.Patch(ctx, obj, patch)
}

// Get reads straight through to the apiserver. A malformed key names nothing,
// so it reads as absent.
func (p *partCollection[T]) Get(ctx context.Context, key state.Key) (T, state.Revision, error) {
	var zero T
	if _, _, err := p.l.Part.Locate(key); err != nil {
		return zero, "", state.ErrNotFound
	}
	obj, _, rec, ok, err := p.fetch(ctx, key)
	if err != nil {
		return zero, "", err
	}
	if !ok {
		return zero, "", state.ErrNotFound
	}
	return rec, state.Revision(obj.GetResourceVersion()), nil
}

// Create sets the part under the object's current revision: ErrNotFound when
// the object is absent, ErrAlreadyExists when the part already holds a record.
func (p *partCollection[T]) Create(ctx context.Context, key state.Key, rec T) (state.Revision, error) {
	for attempt := 0; attempt < statusUpdateMaxAttempts; attempt++ {
		obj, field, _, held, err := p.fetch(ctx, key)
		if err != nil {
			return "", err
		}
		if held {
			return "", state.ErrAlreadyExists
		}
		orig := obj.DeepCopyObject().(client.Object)
		p.l.Part.Write(obj, field, rec)
		err = p.patch(ctx, obj, orig, true)
		switch {
		case err == nil:
			return state.Revision(obj.GetResourceVersion()), nil
		case apierrors.IsConflict(err):
			continue // the object moved: look again, it may now hold one
		}
		return "", writeError("create "+p.l.Decl.Name, string(key), err)
	}
	return "", fmt.Errorf("kube: create %s %q: conflict after %d attempts", p.l.Decl.Name, key, statusUpdateMaxAttempts)
}

// Update replaces the part under rev, the object's revision. ErrNotFound when
// the object or the part is absent, ErrConflict when rev is not current.
func (p *partCollection[T]) Update(ctx context.Context, key state.Key, rev state.Revision, rec T) (state.Revision, error) {
	obj, field, _, held, err := p.fetch(ctx, key)
	if err != nil {
		return "", err
	}
	if !held {
		return "", state.ErrNotFound
	}
	if obj.GetResourceVersion() != string(rev) {
		return "", state.ErrConflict
	}
	orig := obj.DeepCopyObject().(client.Object)
	p.l.Part.Write(obj, field, rec)
	if err := p.patch(ctx, obj, orig, true); err != nil {
		return "", writeError("update "+p.l.Decl.Name, string(key), err)
	}
	return state.Revision(obj.GetResourceVersion()), nil
}

// Delete empties the part. Not an error when it is already empty or the object
// is gone.
func (p *partCollection[T]) Delete(ctx context.Context, key state.Key) error {
	obj, field, _, held, err := p.fetch(ctx, key)
	if err != nil {
		if errors.Is(err, state.ErrNotFound) {
			return nil
		}
		return err
	}
	if !held {
		return nil
	}
	orig := obj.DeepCopyObject().(client.Object)
	p.l.Part.Clear(obj, field)
	if err := p.patch(ctx, obj, orig, false); err != nil && !apierrors.IsNotFound(err) {
		return fmt.Errorf("kube: delete %s %q: %w", p.l.Decl.Name, key, err)
	}
	return nil
}

// DeleteAt empties the part under the revision the caller read. ErrNotFound
// when the part is absent, ErrConflict when rev is not current and the part is
// left intact.
func (p *partCollection[T]) DeleteAt(ctx context.Context, key state.Key, rev state.Revision) error {
	obj, field, _, held, err := p.fetch(ctx, key)
	if err != nil {
		return err
	}
	if !held {
		return state.ErrNotFound
	}
	if obj.GetResourceVersion() != string(rev) {
		return state.ErrConflict
	}
	orig := obj.DeepCopyObject().(client.Object)
	p.l.Part.Clear(obj, field)
	if err := p.patch(ctx, obj, orig, true); err != nil {
		return writeError("delete "+p.l.Decl.Name, string(key), err)
	}
	return nil
}

// List serves the declared indexes. A part kind declares none, so every index
// is outside its closed set.
func (p *partCollection[T]) List(_ context.Context, idx state.Index[T], _ string) ([]T, error) {
	if !p.l.Decl.Declares(idx) {
		return nil, state.ErrUnknownIndex
	}
	return nil, fmt.Errorf("kube: %s declares index %q but a part collection serves none", p.l.Decl.Name, idx.Name())
}

// Watch delivers a put for each record an object carries whenever the object
// changes, and a delete for each record an object stopped carrying. What was
// carried is remembered for the life of this feed, so a record cleared before
// the feed was opened is not announced, which no consumer of the part kinds
// needs: they are read by key.
func (p *partCollection[T]) Watch(ctx context.Context, resume state.Revision) (<-chan state.Event[T], error) {
	carried := map[string]map[state.Key]struct{}{}
	return watchObjects(ctx, &p.base, p.l.NewList, resume, p.l.Decl.Name, func(ev watch.Event) []state.Event[T] {
		return p.translate(ev, carried)
	})
}

func (p *partCollection[T]) translate(ev watch.Event, carried map[string]map[state.Key]struct{}) []state.Event[T] {
	obj, ok := ev.Object.(client.Object)
	if !ok {
		return nil
	}
	rev := state.Revision(obj.GetResourceVersion())
	switch ev.Type {
	case watch.Bookmark:
		if rev == "" {
			return nil
		}
		return []state.Event[T]{{Type: state.EventBookmark, Resume: rev}}
	case watch.Added, watch.Modified:
		return p.translateUpsert(obj, rev, carried)
	case watch.Deleted:
		return p.translateDeleted(obj, rev, carried)
	}
	return nil
}

// translateUpsert announces every record the object now carries and a delete
// for each one it carried before and no longer does.
func (p *partCollection[T]) translateUpsert(obj client.Object, rev state.Revision, carried map[string]map[state.Key]struct{}) []state.Event[T] {
	name := obj.GetName()
	entries := p.l.Part.Enumerate(obj)
	now := make(map[state.Key]struct{}, len(entries))
	out := make([]state.Event[T], 0, len(entries))
	for _, e := range entries {
		now[e.Key] = struct{}{}
		out = append(out, state.Event[T]{Type: state.EventPut, Key: e.Key, Revision: rev, Record: e.Record, Resume: rev})
	}
	for k := range carried[name] {
		if _, still := now[k]; !still {
			out = append(out, state.Event[T]{Type: state.EventDelete, Key: k, Resume: rev})
		}
	}
	carried[name] = now
	return out
}

// translateDeleted announces a delete for every record the object carried.
func (p *partCollection[T]) translateDeleted(obj client.Object, rev state.Revision, carried map[string]map[state.Key]struct{}) []state.Event[T] {
	name := obj.GetName()
	keys := carried[name]
	if keys == nil {
		keys = map[state.Key]struct{}{}
	}
	for _, e := range p.l.Part.Enumerate(obj) {
		keys[e.Key] = struct{}{}
	}
	delete(carried, name)
	out := make([]state.Event[T], 0, len(keys))
	for k := range keys {
		out = append(out, state.Event[T]{Type: state.EventDelete, Key: k, Resume: rev})
	}
	return out
}

var _ state.Collection[struct{}] = (*partCollection[struct{}])(nil)
