package kube

import (
	"context"
	"testing"

	agentopsv1alpha1 "github.com/kostiantyn-matsebora/agent-ops-operator/platform/manager/api/v1alpha1"
	"github.com/kostiantyn-matsebora/agent-ops-operator/platform/manager/internal/state"
)

// EnumerateAdapterState exists for one caller: a backend migrating away from
// kube, which cannot otherwise discover "every adapter-state key" (the
// generic AdapterState collection declares no index). This pins it against
// both surfaces at once.
func TestEnumerateAdapterStateListsBothSurfaces(t *testing.T) {
	ctx := context.Background()
	ns := newNamespace(t)
	newChannel(t, ns, "chan-a")
	src := &agentopsv1alpha1.SignalSource{}
	src.Name, src.Namespace = "src-a", ns
	src.Spec.Adapter = "cron"
	if err := wwClient.Create(ctx, src); err != nil {
		t.Fatalf("create SignalSource: %v", err)
	}

	collection, err := OpenAdapterState(wwClient, ns)
	if err != nil {
		t.Fatalf("OpenAdapterState: %v", err)
	}
	channelKey := state.AdapterStateKey(state.SurfaceChannel, "chan-a", "cursor")
	sourceKey := state.AdapterStateKey(state.SurfaceSignalSource, "src-a", "offset")
	if _, err := collection.Create(ctx, channelKey, "42"); err != nil {
		t.Fatalf("Create (channel): %v", err)
	}
	if _, err := collection.Create(ctx, sourceKey, "7"); err != nil {
		t.Fatalf("Create (source): %v", err)
	}

	entries, err := EnumerateAdapterState(ctx, wwClient, ns)
	if err != nil {
		t.Fatalf("EnumerateAdapterState: %v", err)
	}
	got := map[string]string{}
	for _, e := range entries {
		got[string(e.Key)] = e.Record
	}
	if got[string(channelKey)] != "42" || got[string(sourceKey)] != "7" {
		t.Fatalf("entries = %+v, want %q=42 and %q=7", got, channelKey, sourceKey)
	}
}
