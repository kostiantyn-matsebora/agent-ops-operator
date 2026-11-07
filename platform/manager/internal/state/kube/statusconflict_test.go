package kube

import (
	"context"
	"errors"
	"sync"
	"testing"

	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/interceptor"

	"github.com/kostiantyn-matsebora/agent-ops-operator/platform/manager/internal/state"
)

// interferingStore returns a store whose first status write is preceded by
// interfere, so that write loses the apiserver's own resourceVersion check.
func interferingStore(ns string, interfere func(ctx context.Context)) *Store {
	var once sync.Once
	c := interceptor.NewClient(wwClient, interceptor.Funcs{
		SubResourceUpdate: func(ctx context.Context, cl client.Client, sub string, obj client.Object, opts ...client.SubResourceUpdateOption) error {
			once.Do(func() { interfere(ctx) })
			return cl.SubResource(sub).Update(ctx, obj, opts...)
		},
	})
	return NewStoreWithSweepInterval(c, ns, DefaultSweepInterval)
}

// A competing writer changed the status between Update's main write and its
// status write. Re-applying the caller's whole status would overwrite it, so
// the conflict surfaces instead.
func TestUpdateReturnsConflictWhenAStatusWriterRacesTheStatusWrite(t *testing.T) {
	ns, plain := newTestNamespace(t)
	ctx := context.Background()
	rec := fixtureRecord("conv-race")
	rev, err := plain.Create(ctx, "conv-race", rec)
	if err != nil {
		t.Fatal(err)
	}
	s := interferingStore(ns, func(ctx context.Context) {
		other, orev, err := plain.Get(ctx, "conv-race")
		if err != nil {
			t.Errorf("competing Get: %v", err)
			return
		}
		other.RuntimePod = "written-by-someone-else"
		if _, err := plain.Update(ctx, "conv-race", orev, other); err != nil {
			t.Errorf("competing Update: %v", err)
		}
	})
	mine := rec
	mine.Phase = "Closed"
	if _, err := s.Update(ctx, "conv-race", rev, mine); !errors.Is(err, state.ErrConflict) {
		t.Fatalf("Update = %v, want ErrConflict", err)
	}
	got, _, _ := plain.Get(ctx, "conv-race")
	if got.RuntimePod != "written-by-someone-else" {
		t.Fatalf("the competing writer's status was overwritten: %+v", got)
	}
}

// A run append commutes with Update's status write, so it must not turn the
// retry into a conflict.
func TestUpdateRetriesStatusWriteAcrossARunAppend(t *testing.T) {
	ns, plain := newTestNamespace(t)
	ctx := context.Background()
	rec := fixtureRecord("conv-append")
	rev, err := plain.Create(ctx, "conv-append", rec)
	if err != nil {
		t.Fatal(err)
	}
	s := interferingStore(ns, func(ctx context.Context) {
		if err := plain.AppendRun(ctx, "conv-append", state.Run{RunID: "r-new"}); err != nil {
			t.Errorf("AppendRun: %v", err)
		}
	})
	mine := rec
	mine.Phase = "Closed"
	if _, err := s.Update(ctx, "conv-append", rev, mine); err != nil {
		t.Fatalf("Update across a run append = %v", err)
	}
	got, _, _ := plain.Get(ctx, "conv-append")
	runs, _ := plain.ListRuns(ctx, "conv-append")
	if got.Phase != "Closed" || len(runs) == 0 {
		t.Fatalf("phase %q, runs %v: both writes must survive", got.Phase, runs)
	}
}
