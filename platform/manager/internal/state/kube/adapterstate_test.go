// Task 3.3: adapter key-value state is the agentops.dev/adapter-state-<key>
// annotation of the Channel or SignalSource, the same object and the same merge
// patch the /channel/state and /signal/state handlers have always used.
package kube

import (
	"context"
	"errors"
	"strings"
	"testing"

	"sigs.k8s.io/controller-runtime/pkg/client"

	agentopsv1alpha1 "github.com/kostiantyn-matsebora/agent-ops-operator/platform/manager/api/v1alpha1"
	"github.com/kostiantyn-matsebora/agent-ops-operator/platform/manager/internal/state"
)

// handlerPrefix is httpapi.StateAnnotationPrefix, restated: this package may
// not import its sibling. A drift fails the byte-compat tests below.
const handlerPrefix = "agentops.dev/adapter-state-"

func newChannel(t *testing.T, ns, name string) {
	t.Helper()
	ch := &agentopsv1alpha1.Channel{}
	ch.Name, ch.Namespace = name, ns
	ch.Spec.Adapter = "telegram"
	if err := wwClient.Create(context.Background(), ch); err != nil {
		t.Fatalf("create channel: %v", err)
	}
}

// handlerWrite is the old /channel/state PUT, verbatim: a merge patch of one
// annotation on the Channel.
func handlerWrite(t *testing.T, ns, name, key, value string) {
	t.Helper()
	ctx := context.Background()
	var ch agentopsv1alpha1.Channel
	if err := wwClient.Get(ctx, client.ObjectKey{Namespace: ns, Name: name}, &ch); err != nil {
		t.Fatal(err)
	}
	patch := client.MergeFrom(ch.DeepCopy())
	if ch.Annotations == nil {
		ch.Annotations = map[string]string{}
	}
	ch.Annotations[handlerPrefix+key] = value
	if err := wwClient.Patch(ctx, &ch, patch); err != nil {
		t.Fatal(err)
	}
}

// handlerRead is the old /channel/state GET: the annotation, "" when unset.
func handlerRead(t *testing.T, ns, name, key string) string {
	t.Helper()
	var ch agentopsv1alpha1.Channel
	if err := wwClient.Get(context.Background(), client.ObjectKey{Namespace: ns, Name: name}, &ch); err != nil {
		t.Fatal(err)
	}
	return ch.Annotations[handlerPrefix+key]
}

func TestAdapterStateWrittenThroughTheCollectionReadsBackThroughTheHandlerPath(t *testing.T) {
	ctx := context.Background()
	ns, s := newTestNamespace(t)
	newChannel(t, ns, "tg")
	key := state.AdapterStateKey(state.SurfaceChannel, "tg", "offset")

	if _, err := s.AdapterState.Create(ctx, key, "12345"); err != nil {
		t.Fatalf("Create: %v", err)
	}
	if got := handlerRead(t, ns, "tg", "offset"); got != "12345" {
		t.Fatalf("the handler path reads %q, want the value written through the collection", got)
	}
	_, rev, err := s.AdapterState.Get(ctx, key)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.AdapterState.Update(ctx, key, rev, "12346"); err != nil {
		t.Fatalf("Update: %v", err)
	}
	if got := handlerRead(t, ns, "tg", "offset"); got != "12346" {
		t.Fatalf("after Update the handler path reads %q", got)
	}
}

func TestAdapterStateWrittenByTheHandlerPathReadsBackThroughTheCollection(t *testing.T) {
	ctx := context.Background()
	ns, s := newTestNamespace(t)
	newChannel(t, ns, "tg")
	key := state.AdapterStateKey(state.SurfaceChannel, "tg", "cursor")

	if _, _, err := s.AdapterState.Get(ctx, key); !errors.Is(err, state.ErrNotFound) {
		t.Fatalf("Get of a key never written = %v, want ErrNotFound", err)
	}
	handlerWrite(t, ns, "tg", "cursor", "written-before-the-contract")
	got, _, err := s.AdapterState.Get(ctx, key)
	if err != nil || got != "written-before-the-contract" {
		t.Fatalf("Get = %q, %v; want the value an older manager wrote", got, err)
	}
	// A value the handler wrote empty is present, not absent.
	handlerWrite(t, ns, "tg", "empty", "")
	if v, _, err := s.AdapterState.Get(ctx, state.AdapterStateKey(state.SurfaceChannel, "tg", "empty")); err != nil || v != "" {
		t.Fatalf("Get of an empty value = %q, %v; want present and empty", v, err)
	}
	// Writing through the collection leaves the same annotation on the same object.
	_, rev, _ := s.AdapterState.Get(ctx, key)
	if _, err := s.AdapterState.Update(ctx, key, rev, "next"); err != nil {
		t.Fatal(err)
	}
	var ch agentopsv1alpha1.Channel
	if err := wwClient.Get(ctx, client.ObjectKey{Namespace: ns, Name: "tg"}, &ch); err != nil {
		t.Fatal(err)
	}
	if ch.Annotations[handlerPrefix+"cursor"] != "next" || ch.Annotations[handlerPrefix+"empty"] != "" {
		t.Fatalf("annotations = %v", ch.Annotations)
	}
	if _, ok := ch.Annotations[ExpiresAtAnnotation]; ok {
		t.Fatal("adapter state must not stamp an expiry")
	}
}

func TestAdapterStateOfTwoSurfacesNeverShareAKey(t *testing.T) {
	ctx := context.Background()
	ns, s := newTestNamespace(t)
	newChannel(t, ns, "same")
	newSignalSource(t, ns, "same")

	chKey := state.AdapterStateKey(state.SurfaceChannel, "same", "k")
	srcKey := state.AdapterStateKey(state.SurfaceSignalSource, "same", "k")
	if _, err := s.AdapterState.Create(ctx, chKey, "channel-value"); err != nil {
		t.Fatal(err)
	}
	if _, err := s.AdapterState.Create(ctx, srcKey, "source-value"); err != nil {
		t.Fatal(err)
	}
	if v, _, err := s.AdapterState.Get(ctx, chKey); err != nil || v != "channel-value" {
		t.Fatalf("channel reads %q, %v", v, err)
	}
	if v, _, err := s.AdapterState.Get(ctx, srcKey); err != nil || v != "source-value" {
		t.Fatalf("signal source reads %q, %v", v, err)
	}
	var src agentopsv1alpha1.SignalSource
	if err := wwClient.Get(ctx, client.ObjectKey{Namespace: ns, Name: "same"}, &src); err != nil {
		t.Fatal(err)
	}
	if src.Annotations[handlerPrefix+"k"] != "source-value" {
		t.Fatalf("the source's annotation = %v", src.Annotations)
	}
}

func TestAdapterStateEdges(t *testing.T) {
	ctx := context.Background()
	ns, s := newTestNamespace(t)
	newChannel(t, ns, "tg")
	key := state.AdapterStateKey(state.SurfaceChannel, "tg", "k")

	if _, err := s.AdapterState.Create(ctx, state.AdapterStateKey(state.SurfaceChannel, "gone", "k"), "v"); !errors.Is(err, state.ErrNotFound) {
		t.Fatalf("Create on a Channel that does not exist = %v, want ErrNotFound", err)
	}
	if _, err := s.AdapterState.Create(ctx, key, "v"); err != nil {
		t.Fatal(err)
	}
	if _, err := s.AdapterState.Create(ctx, key, "again"); !errors.Is(err, state.ErrAlreadyExists) {
		t.Fatalf("second Create = %v, want ErrAlreadyExists", err)
	}
	_, rev, _ := s.AdapterState.Get(ctx, key)
	// Another key on the same object moves its revision: this Update is stale.
	if _, err := s.AdapterState.Create(ctx, state.AdapterStateKey(state.SurfaceChannel, "tg", "other"), "o"); err != nil {
		t.Fatal(err)
	}
	if _, err := s.AdapterState.Update(ctx, key, rev, "late"); !errors.Is(err, state.ErrConflict) {
		t.Fatalf("Update on a stale revision = %v, want ErrConflict", err)
	}
	// Mutate re-reads and applies, which is how a last-write-wins caller writes.
	if _, err := state.Mutate(ctx, s.AdapterState, key, func(v *string) error { *v = "late"; return nil }); err != nil {
		t.Fatal(err)
	}
	if got := handlerRead(t, ns, "tg", "k"); got != "late" {
		t.Fatalf("after Mutate the annotation is %q", got)
	}
	if err := s.AdapterState.Delete(ctx, key); err != nil {
		t.Fatal(err)
	}
	if got := handlerRead(t, ns, "tg", "k"); got != "" {
		t.Fatalf("after Delete the annotation is %q", got)
	}
	if got := handlerRead(t, ns, "tg", "other"); got != "o" {
		t.Fatalf("Delete took another key with it: %q", got)
	}
	if err := s.AdapterState.Delete(ctx, key); err != nil {
		t.Fatalf("Delete of an absent key = %v, want nil", err)
	}
	// A key naming no surface or no valid annotation reads absent and refuses a write.
	for _, bad := range []state.Key{"not-a-key", state.AdapterStateKey("nosuch", "tg", "k"), state.AdapterStateKey(state.SurfaceChannel, "tg", "bad key!")} {
		if _, _, err := s.AdapterState.Get(ctx, bad); !errors.Is(err, state.ErrNotFound) {
			t.Errorf("Get(%q) = %v, want ErrNotFound", strings.ReplaceAll(string(bad), "\x00", "/"), err)
		}
		if _, err := s.AdapterState.Create(ctx, bad, "v"); err == nil || errors.Is(err, state.ErrNotFound) {
			t.Errorf("Create(%q) = %v, want a descriptive refusal", strings.ReplaceAll(string(bad), "\x00", "/"), err)
		}
	}
}
