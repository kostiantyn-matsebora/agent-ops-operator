package kube

import (
	"context"
	"fmt"

	"sigs.k8s.io/controller-runtime/pkg/client"
)

// EnumerateAdapterState lists every adapter-state entry any Channel or
// SignalSource in namespace currently carries, key and value together — the
// one bridge a backend migrating away from kube needs (the generic
// AdapterState collection declares no index, so nothing outside this
// package, where the guarded annotation prefix may be named, can discover
// "every key" on its own).
//
// This is the one export this backend offers purely for a MIGRATION: no
// production code path uses it, since every live read goes through
// OpenAdapterState's own Collection.
func EnumerateAdapterState(ctx context.Context, c client.Reader, namespace string) ([]Entry[string], error) {
	var out []Entry[string]
	for _, l := range []Layout[string]{ChannelStateLayout(), SourceStateLayout()} {
		objs, err := listLayoutObjects(ctx, c, namespace, l)
		if err != nil {
			return nil, err
		}
		for _, obj := range objs {
			out = append(out, l.Part.Enumerate(obj)...)
		}
	}
	return out, nil
}

// listLayoutObjects lists every object of l's kind in namespace.
func listLayoutObjects(ctx context.Context, c client.Reader, namespace string, l Layout[string]) ([]client.Object, error) {
	list := l.NewList()
	if err := c.List(ctx, list, client.InNamespace(namespace)); err != nil {
		return nil, fmt.Errorf("kube: list %s for EnumerateAdapterState: %w", l.Decl.Name, err)
	}
	return objectsOf(list, l.Decl.Name)
}
