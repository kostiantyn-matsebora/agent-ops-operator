package signalroute

import (
	"context"
	"errors"
	"testing"
	"time"

	agentopsv1alpha1 "github.com/kostiantyn-matsebora/agent-ops-operator/platform/manager/api/v1alpha1"
	"github.com/kostiantyn-matsebora/agent-ops-operator/platform/manager/internal/state"
)

// windowStore is a Collection[CooldownWindow] whose read is scripted; anything
// else is a nil-embedded panic, so a test learns of an unexpected call.
type windowStore struct {
	state.Collection[state.CooldownWindow]
	reads  int
	err    error
	blocks bool
}

func (w *windowStore) Get(ctx context.Context, _ state.Key) (state.CooldownWindow, state.Revision, error) {
	w.reads++
	if w.blocks {
		<-ctx.Done()
		return state.CooldownWindow{}, "", ctx.Err()
	}
	return state.CooldownWindow{}, "", w.err
}

func sourceWithHours(hours int32) *agentopsv1alpha1.SignalSource {
	src := &agentopsv1alpha1.SignalSource{}
	src.Name = "s"
	src.Spec.CooldownHours = hours
	return src
}

func TestRecoverCooldownSkipsTheReadWhenTheWindowIsOff(t *testing.T) {
	store := &windowStore{err: errors.New("store down")}
	deps := Deps{CooldownWindows: store}
	// a chat source defaults the window off: a store error must not fail it
	if cd, err := recoverCooldown(context.Background(), deps, sourceWithHours(0), kindChat); err != nil || cd == nil {
		t.Fatalf("chat window off: %v, %v, want a built window", cd, err)
	}
	if store.reads != 0 {
		t.Fatalf("store read %d times for a disabled window, want none", store.reads)
	}
	// a window that is on still fails the pass: a restart must not re-open ids
	if cd, err := recoverCooldown(context.Background(), deps, sourceWithHours(0), "alert"); err == nil || cd != nil {
		t.Fatalf("alert window on: %v, %v, want the store error", cd, err)
	}
}

func TestRecoverCooldownReadIsBounded(t *testing.T) {
	old := cooldownReadTimeout
	cooldownReadTimeout = 50 * time.Millisecond
	defer func() { cooldownReadTimeout = old }()
	deps := Deps{CooldownWindows: &windowStore{blocks: true}}
	done := make(chan error, 1)
	go func() {
		_, err := recoverCooldown(context.Background(), deps, sourceWithHours(1), "alert")
		done <- err
	}()
	select {
	case err := <-done:
		if !errors.Is(err, context.DeadlineExceeded) {
			t.Fatalf("error = %v, want the deadline", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("a hung store read was not bounded")
	}
}
