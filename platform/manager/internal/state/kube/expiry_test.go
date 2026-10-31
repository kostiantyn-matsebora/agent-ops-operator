// Task 2.6: expiry — invisible before gone, eventually swept, observable
// on Watch.
package kube

import (
	"context"
	"errors"
	"testing"
	"time"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/types"

	agentopsv1alpha1 "github.com/kostiantyn-matsebora/agent-ops-operator/platform/manager/api/v1alpha1"
	"github.com/kostiantyn-matsebora/agent-ops-operator/platform/manager/internal/state"
)

// TestExpiredRecordInvisibleBeforeSweep is the spec's own "An expired
// record is invisible before it is gone" scenario: Get and every List
// omit it the instant its expiry has passed, with no sweep having run at
// all (newTestNamespace's Store never calls Run).
func TestExpiredRecordInvisibleBeforeSweep(t *testing.T) {
	_, s := newTestNamespace(t)
	ctx := context.Background()

	past := time.Now().Add(-time.Hour)
	rec := fixtureRecord("conv-expired")
	rec.ExpiresAt = &past
	if _, err := s.Create(ctx, state.Key(rec.Name), rec); err != nil {
		t.Fatalf("Create: %v", err)
	}

	if _, _, err := s.Get(ctx, state.Key(rec.Name)); !errors.Is(err, state.ErrNotFound) {
		t.Fatalf("Get on an expired record: got %v, want ErrNotFound", err)
	}

	for _, probe := range []struct {
		idx state.Index
		key state.ListKey
	}{
		{state.ByConversationID, state.ListKey{ConversationID: rec.ConversationID}},
		{state.ByPipelineAndPhase, state.ListKey{PipelineRef: rec.PipelineRef, Phase: rec.Phase}},
		{state.ByPhase, state.ListKey{Phase: rec.Phase}},
	} {
		got, err := s.List(ctx, probe.idx, probe.key)
		if err != nil {
			t.Fatalf("List(%s): %v", probe.idx, err)
		}
		for _, r := range got {
			if r.Name == rec.Name {
				t.Fatalf("List(%s) returned an expired record: %+v", probe.idx, r)
			}
		}
	}
}

// TestExpiredRecordEventuallySweptAndObservedOnWatch runs Run with a short
// sweep interval and asserts the expired record is (a) actually deleted,
// not merely filtered, and (b) its deletion is observable on Watch, like
// any other delete (spec: "Expiry is observable on the feed").
func TestExpiredRecordEventuallySweptAndObservedOnWatch(t *testing.T) {
	nsName, s := newTestNamespaceWithSweepInterval(t, 50*time.Millisecond)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	past := time.Now().Add(-time.Hour)
	rec := fixtureRecord("conv-expired-swept")
	rec.ExpiresAt = &past
	rev, err := s.Create(ctx, state.Key(rec.Name), rec)
	if err != nil {
		t.Fatalf("Create: %v", err)
	}

	events, err := s.Watch(ctx, rev)
	if err != nil {
		t.Fatalf("Watch: %v", err)
	}

	go s.Run(ctx)

	ev := recvEvent(t, events)
	if ev.Type != state.EventDelete || ev.Key != state.Key(rec.Name) {
		t.Fatalf("expiry sweep's delete was not observed on Watch: %+v", ev)
	}

	// Confirm the object is genuinely gone from the apiserver, not merely
	// filtered by Get/List's own expiry check — bypass the Store and ask
	// the raw client directly.
	deadline := time.Now().Add(5 * time.Second)
	for {
		var conv agentopsv1alpha1.Conversation
		err := wwClient.Get(ctx, types.NamespacedName{Namespace: nsName, Name: rec.Name}, &conv)
		if apierrors.IsNotFound(err) {
			return
		}
		if time.Now().After(deadline) {
			t.Fatalf("expired record was never actually deleted by the sweep (last error: %v)", err)
		}
		time.Sleep(10 * time.Millisecond)
	}
}
