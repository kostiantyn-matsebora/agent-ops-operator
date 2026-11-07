package kube

import (
	"context"
	"fmt"
	"strings"
	"sync"

	"k8s.io/apimachinery/pkg/util/validation"
	"sigs.k8s.io/controller-runtime/pkg/client"

	agentopsv1alpha1 "github.com/kostiantyn-matsebora/agent-ops-operator/platform/manager/api/v1alpha1"
	"github.com/kostiantyn-matsebora/agent-ops-operator/platform/manager/internal/state"
)

// AdapterStateAnnotationPrefix namespaces the key-value state an adapter
// persists, kept as annotations on the Channel or SignalSource it registered
// under (for example agentops.dev/adapter-state-offset). It is the prefix the
// /channel/state and /signal/state handlers have always written, so a cursor
// written before the collection reads back after it, and the reverse.
const AdapterStateAnnotationPrefix = "agentops.dev/adapter-state-"

// adapterStateSeparator joins the parts of an AdapterStateKey, the separator
// state.IndexValue composes with.
const adapterStateSeparator = "\x00"

// parseAdapterStateKey splits a state.AdapterStateKey into its surface, the
// name of the object and the adapter's own key.
func parseAdapterStateKey(key state.Key) (surface, name, adapterKey string, err error) {
	parts := strings.Split(string(key), adapterStateSeparator)
	if len(parts) != 3 || parts[0] == "" || parts[1] == "" || parts[2] == "" {
		return "", "", "", fmt.Errorf("kube: adapter state key %q is not surface, name and key", strings.ReplaceAll(string(key), adapterStateSeparator, "/"))
	}
	return parts[0], parts[1], parts[2], nil
}

// adapterStateLayout maps the adapter state of one surface onto the
// annotations of the object kind it registers under.
func adapterStateLayout(surface string, newObj func() client.Object, newList func() client.ObjectList) Layout[string] {
	return Layout[string]{
		Decl:    state.StoreDeclarations.AdapterState,
		New:     newObj,
		NewList: newList,
		Part: &Part[string]{
			Locate: func(key state.Key) (string, string, error) {
				s, name, k, err := parseAdapterStateKey(key)
				if err != nil {
					return "", "", err
				}
				if s != surface {
					return "", "", fmt.Errorf("kube: adapter state key %q names surface %q, this layout serves %q", name, s, surface)
				}
				field := AdapterStateAnnotationPrefix + k
				if errs := validation.IsQualifiedName(field); len(errs) > 0 {
					return "", "", fmt.Errorf("kube: adapter state key %q is not a valid annotation name: %s", k, strings.Join(errs, "; "))
				}
				return name, field, nil
			},
			Read: func(obj client.Object, field string) (string, bool) {
				v, ok := obj.GetAnnotations()[field]
				return v, ok
			},
			Write: func(obj client.Object, field string, v string) {
				ann := obj.GetAnnotations()
				if ann == nil {
					ann = map[string]string{}
				}
				ann[field] = v
				obj.SetAnnotations(ann)
			},
			Clear: func(obj client.Object, field string) {
				ann := obj.GetAnnotations()
				delete(ann, field)
				obj.SetAnnotations(ann)
			},
			Enumerate: func(obj client.Object) []Entry[string] {
				var out []Entry[string]
				for k, v := range obj.GetAnnotations() {
					if suffix, ok := strings.CutPrefix(k, AdapterStateAnnotationPrefix); ok && suffix != "" {
						out = append(out, Entry[string]{Key: state.AdapterStateKey(surface, obj.GetName(), suffix), Record: v})
					}
				}
				return out
			},
		},
	}
}

// ChannelStateLayout is adapter state on a Channel's annotations.
func ChannelStateLayout() Layout[string] {
	return adapterStateLayout(state.SurfaceChannel,
		func() client.Object { return &agentopsv1alpha1.Channel{} },
		func() client.ObjectList { return &agentopsv1alpha1.ChannelList{} })
}

// SourceStateLayout is adapter state on a SignalSource's annotations.
func SourceStateLayout() Layout[string] {
	return adapterStateLayout(state.SurfaceSignalSource,
		func() client.Object { return &agentopsv1alpha1.SignalSource{} },
		func() client.ObjectList { return &agentopsv1alpha1.SignalSourceList{} })
}

// adapterState is the one state.Collection[string] over both surfaces: a key
// names its surface, and each surface is a different object kind, so the key
// routes the call. The value is the annotation on the named object, and a
// surface that is not one of the two names nothing.
type adapterState struct {
	bySurface map[string]state.Collection[string]
}

// OpenAdapterState returns the adapter state collection, scoped to namespace.
func OpenAdapterState(c client.WithWatch, namespace string, opts ...Option) (state.Collection[string], error) {
	channel, err := Open(c, namespace, ChannelStateLayout(), opts...)
	if err != nil {
		return nil, err
	}
	source, err := Open(c, namespace, SourceStateLayout(), opts...)
	if err != nil {
		return nil, err
	}
	return &adapterState{bySurface: map[string]state.Collection[string]{
		state.SurfaceChannel:      channel,
		state.SurfaceSignalSource: source,
	}}, nil
}

// route picks the surface a key names.
func (a *adapterState) route(key state.Key) (state.Collection[string], error) {
	surface, _, _, err := parseAdapterStateKey(key)
	if err != nil {
		return nil, err
	}
	c, ok := a.bySurface[surface]
	if !ok {
		return nil, fmt.Errorf("kube: adapter state key names surface %q, which is neither %q nor %q", surface, state.SurfaceChannel, state.SurfaceSignalSource)
	}
	return c, nil
}

// Get reads a value. A key that names no surface, or one no annotation can
// carry, reads absent, as the handlers' read of an unset key always did.
func (a *adapterState) Get(ctx context.Context, key state.Key) (string, state.Revision, error) {
	c, err := a.route(key)
	if err != nil {
		return "", "", state.ErrNotFound
	}
	return c.Get(ctx, key)
}

func (a *adapterState) Create(ctx context.Context, key state.Key, v string) (state.Revision, error) {
	c, err := a.route(key)
	if err != nil {
		return "", err
	}
	return c.Create(ctx, key, v)
}

func (a *adapterState) Update(ctx context.Context, key state.Key, rev state.Revision, v string) (state.Revision, error) {
	c, err := a.route(key)
	if err != nil {
		return "", err
	}
	return c.Update(ctx, key, rev, v)
}

func (a *adapterState) Delete(ctx context.Context, key state.Key) error {
	c, err := a.route(key)
	if err != nil {
		return err
	}
	return c.Delete(ctx, key)
}

func (a *adapterState) DeleteAt(ctx context.Context, key state.Key, rev state.Revision) error {
	c, err := a.route(key)
	if err != nil {
		return err
	}
	return c.DeleteAt(ctx, key, rev)
}

// List serves no index: the collection declares none.
func (a *adapterState) List(ctx context.Context, idx state.Index[string], value string) ([]string, error) {
	return a.bySurface[state.SurfaceChannel].List(ctx, idx, value)
}

// Watch merges the feeds of both surfaces. The channel closes when both have.
func (a *adapterState) Watch(ctx context.Context, resume state.Revision) (<-chan state.Event[string], error) {
	merged := make(chan state.Event[string])
	var feeds []<-chan state.Event[string]
	for _, c := range a.bySurface {
		feed, err := c.Watch(ctx, resume)
		if err != nil {
			return nil, err
		}
		feeds = append(feeds, feed)
	}
	var wg sync.WaitGroup
	for _, feed := range feeds {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for ev := range feed {
				select {
				case merged <- ev:
				case <-ctx.Done():
					return
				}
			}
		}()
	}
	go func() {
		wg.Wait()
		close(merged)
	}()
	return merged, nil
}

var _ state.Collection[string] = (*adapterState)(nil)
