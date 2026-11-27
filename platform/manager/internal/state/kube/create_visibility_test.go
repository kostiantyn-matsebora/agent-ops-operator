package kube

import (
	"context"
	"errors"
	"testing"
	"time"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/interceptor"

	agentopsv1alpha1 "github.com/kostiantyn-matsebora/agent-ops-operator/platform/manager/api/v1alpha1"
	"github.com/kostiantyn-matsebora/agent-ops-operator/platform/manager/internal/state"
)

var errInjectedKube = errors.New("kube test: injected failure")

// brokenVisibilityPartLayout is CooldownLayout's own shape, except its Read
// can never see what Write just stored — the only way to drive
// partCollection.Create into reporting "stored part is not visible" (part.go)
// through the public Layout/Part extension point, since every shipped
// Layout's own Read/Write pair is self-consistent by construction.
func brokenVisibilityPartLayout() Layout[string] {
	return Layout[string]{
		Decl:    state.Decl[string]{Name: "broken-part-visibility-test"},
		New:     func() client.Object { return &agentopsv1alpha1.SignalSource{} },
		NewList: func() client.ObjectList { return &agentopsv1alpha1.SignalSourceList{} },
		Part: &Part[string]{
			Locate: sourceKey,
			Read:   func(client.Object, string) (string, bool) { return "", false },
			Write: func(obj client.Object, _, v string) {
				ann := obj.GetAnnotations()
				if ann == nil {
					ann = map[string]string{}
				}
				ann["broken-part-visibility-test"] = v
				obj.SetAnnotations(ann)
			},
			Clear: func(obj client.Object, _ string) {
				ann := obj.GetAnnotations()
				delete(ann, "broken-part-visibility-test")
				obj.SetAnnotations(ann)
			},
			Enumerate: func(client.Object) []Entry[string] { return nil },
		},
	}
}

// TestPartCreateReportsWhenTheStoredPartIsNotVisible covers part.go's own
// "stored part is not visible" guard (Create, lines 98-99): a write that the
// apiserver genuinely accepted must still fail the call, rather than answer
// a caller with a fabricated "success", when this backend's own Layout
// cannot read back what it just wrote.
func TestPartCreateReportsWhenTheStoredPartIsNotVisible(t *testing.T) {
	ns := newNamespace(t)
	src := newSignalSource(t, ns, "src")
	store, err := Open(wwClient, ns, brokenVisibilityPartLayout())
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	if _, _, err := store.Create(context.Background(), state.Key(src.Name), "value"); err == nil {
		t.Fatal("Create succeeded even though its own Layout's Read can never see what Write just stored")
	}
}

// TestWholeObjectCreateReportsWhenTheStoredObjectIsNotVisible is the same
// scenario for the whole-object collection (store.go's Create, lines
// 138-139): the apiserver's own create genuinely lands the object, but this
// backend's own Layout.Read can never see it, which must fail the call
// rather than report success.
func TestWholeObjectCreateReportsWhenTheStoredObjectIsNotVisible(t *testing.T) {
	ns := newNamespace(t)
	l := Layout[string]{
		Decl:    state.Decl[string]{Name: "broken-whole-visibility-test"},
		New:     func() client.Object { return &agentopsv1alpha1.SignalSource{} },
		NewList: func() client.ObjectList { return &agentopsv1alpha1.SignalSourceList{} },
		Read:    func(client.Object) (string, bool) { return "", false },
		Init: func(obj client.Object, rec string) {
			asSource(obj).Spec.Adapter = "cron"
			ann := map[string]string{"broken-whole-visibility-test": rec}
			obj.SetAnnotations(ann)
		},
		WriteMain: func(client.Object, string) {},
	}
	store, err := Open(wwClient, ns, l)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	if _, _, err := store.Create(context.Background(), "obj-1", "value"); err == nil {
		t.Fatal("Create succeeded even though its own Layout's Read can never see what Init just stored")
	}
}

// TestPartCreateReportsAGenuineWriteFailure covers part.go's Create default
// error branch (line 104): a patch failure that is not a conflict (a real
// write refusal) must be returned, never retried as if it were contention.
func TestPartCreateReportsAGenuineWriteFailure(t *testing.T) {
	ns := newNamespace(t)
	src := newSignalSource(t, ns, "src")
	ic := interceptor.NewClient(wwClient, interceptor.Funcs{
		SubResourcePatch: func(context.Context, client.Client, string, client.Object, client.Patch, ...client.SubResourcePatchOption) error {
			return errInjectedKube
		},
	})
	store, err := Open(ic, ns, CooldownLayout())
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	window := state.CooldownWindow{Entries: []state.CooldownEntry{{InitiationID: "i1", At: time.Now()}}}
	if _, _, err := store.Create(context.Background(), state.Key(src.Name), window); !errors.Is(err, errInjectedKube) {
		t.Fatalf("Create = %v, want the write failure propagated, never swallowed as a conflict", err)
	}
}

// TestPartCreateGivesUpAfterRepeatedConflicts covers part.go's Create retry
// exhaustion (line 106): under ENDLESS contention (every write conflicts),
// the loop must give up after statusUpdateMaxAttempts tries rather than
// retry forever or silently succeed.
func TestPartCreateGivesUpAfterRepeatedConflicts(t *testing.T) {
	ns := newNamespace(t)
	src := newSignalSource(t, ns, "src")
	attempts := 0
	ic := interceptor.NewClient(wwClient, interceptor.Funcs{
		SubResourcePatch: func(_ context.Context, _ client.Client, _ string, obj client.Object, _ client.Patch, _ ...client.SubResourcePatchOption) error {
			attempts++
			return apierrors.NewConflict(schema.GroupResource{Group: "agentops.dev", Resource: "signalsources"}, obj.GetName(), errInjectedKube)
		},
	})
	store, err := Open(ic, ns, CooldownLayout())
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	window := state.CooldownWindow{Entries: []state.CooldownEntry{{InitiationID: "i1", At: time.Now()}}}
	if _, _, err := store.Create(context.Background(), state.Key(src.Name), window); err == nil {
		t.Fatal("Create succeeded under endless contention")
	} else if errors.Is(err, state.ErrConflict) {
		t.Fatalf("Create under endless contention reported ErrConflict %v, want it to give up with its own failure instead of a single-conflict answer", err)
	}
	if attempts != statusUpdateMaxAttempts {
		t.Fatalf("the write was attempted %d times, want exactly the bounded retry budget %d", attempts, statusUpdateMaxAttempts)
	}
}

// TestConversationCreateReportsAStatusUpdateFailure covers store.go's Create
// status-write failure (line 132): the main object write can succeed while
// the follow-up status write genuinely fails, and that failure must reach
// the caller rather than be reported as a successful Create.
func TestConversationCreateReportsAStatusUpdateFailure(t *testing.T) {
	ns := newNamespace(t)
	ic := interceptor.NewClient(wwClient, interceptor.Funcs{
		SubResourceUpdate: func(context.Context, client.Client, string, client.Object, ...client.SubResourceUpdateOption) error {
			return errInjectedKube
		},
	})
	store := openBackend(t, ic, ns).Stores
	if _, _, err := store.Conversations.Create(context.Background(), "conv-status-fail", fixtureRecord("conv-status-fail")); !errors.Is(err, errInjectedKube) {
		t.Fatalf("Create = %v, want the status update failure propagated", err)
	}
}
