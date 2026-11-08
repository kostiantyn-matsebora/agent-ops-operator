package controller

import (
	"context"
	"errors"
	"testing"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/event"

	agentopsv1alpha1 "github.com/kostiantyn-matsebora/agent-ops-operator/platform/manager/api/v1alpha1"
	"github.com/kostiantyn-matsebora/agent-ops-operator/platform/manager/internal/state"
)

// memStringCollection records every key a test calls Delete on, and can be
// told to fail one of them — releaseAdapterStateKeys is best-effort, so a
// failing Delete must not panic or stop the rest of the loop.
type memStringCollection struct {
	state.Collection[string]
	deleted []state.Key
	failOn  state.Key
}

func (m *memStringCollection) Delete(_ context.Context, k state.Key) error {
	m.deleted = append(m.deleted, k)
	if m.failOn != "" && k == m.failOn {
		return errors.New("delete failed")
	}
	return nil
}

func TestReleaseAdapterStateKeysDeletesEveryAnnotationUnderThePrefix(t *testing.T) {
	coll := &memStringCollection{}
	ch := &agentopsv1alpha1.Channel{
		ObjectMeta: metav1.ObjectMeta{
			Name: "tg",
			Annotations: map[string]string{
				adapterStateAnnotationPrefix + "cursor": "v1",
				adapterStateAnnotationPrefix + "offset": "v2",
				"unrelated-annotation":                  "kept",
			},
		},
	}
	releaseAdapterStateKeys(context.Background(), coll, state.SurfaceChannel, ch)

	want := map[state.Key]bool{
		state.AdapterStateKey(state.SurfaceChannel, "tg", "cursor"): true,
		state.AdapterStateKey(state.SurfaceChannel, "tg", "offset"): true,
	}
	if len(coll.deleted) != len(want) {
		t.Fatalf("deleted %v, want exactly %v", coll.deleted, want)
	}
	for _, k := range coll.deleted {
		if !want[k] {
			t.Errorf("released a key an unrelated annotation must not have produced: %q", k)
		}
	}
}

func TestReleaseAdapterStateKeysSkipsAnEmptySuffixAndAnUnprefixedAnnotation(t *testing.T) {
	coll := &memStringCollection{}
	src := &agentopsv1alpha1.SignalSource{
		ObjectMeta: metav1.ObjectMeta{
			Name: "cron",
			Annotations: map[string]string{
				adapterStateAnnotationPrefix: "no suffix, nothing to release",
				"agentops.dev/other":         "not this prefix at all",
			},
		},
	}
	releaseAdapterStateKeys(context.Background(), coll, state.SurfaceSignalSource, src)
	if len(coll.deleted) != 0 {
		t.Fatalf("deleted %v, want nothing released", coll.deleted)
	}
}

func TestReleaseAdapterStateKeysIsNilSafe(t *testing.T) {
	// Neither call may panic: a nil Stores field (an untouched test double
	// elsewhere) or a nil tombstone object are both real inputs this
	// best-effort release must survive.
	releaseAdapterStateKeys(context.Background(), nil, state.SurfaceChannel,
		&agentopsv1alpha1.Channel{ObjectMeta: metav1.ObjectMeta{Name: "x"}})
	releaseAdapterStateKeys(context.Background(), &memStringCollection{}, state.SurfaceChannel, nil)
}

func TestReleaseAdapterStateKeysLogsRatherThanStopsOnAFailedDelete(t *testing.T) {
	coll := &memStringCollection{}
	ch := &agentopsv1alpha1.Channel{
		ObjectMeta: metav1.ObjectMeta{
			Name: "tg",
			Annotations: map[string]string{
				adapterStateAnnotationPrefix + "a": "1",
				adapterStateAnnotationPrefix + "b": "2",
			},
		},
	}
	coll.failOn = state.AdapterStateKey(state.SurfaceChannel, "tg", "a")

	releaseAdapterStateKeys(context.Background(), coll, state.SurfaceChannel, ch)

	if len(coll.deleted) != 2 {
		t.Fatalf("a failed Delete must not stop the loop over the rest of the keys, got %v", coll.deleted)
	}
}

func TestDeleteOnlyPredicateAdmitsOnlyDelete(t *testing.T) {
	p := deleteOnlyPredicate()
	if p.CreateFunc(event.CreateEvent{}) {
		t.Error("Create must be refused")
	}
	if p.UpdateFunc(event.UpdateEvent{}) {
		t.Error("Update must be refused")
	}
	if p.GenericFunc(event.GenericEvent{}) {
		t.Error("Generic must be refused")
	}
	if !p.DeleteFunc(event.DeleteEvent{}) {
		t.Error("Delete must be admitted — it is the whole point of this predicate")
	}
}
