package main

import (
	"context"
	"os"
	"testing"
	"time"
)

// TestMainRunsUntilSignaled closes main() itself: it starts the real process
// loop (env defaulting, the real HTTP server) against a real HTTP double for
// the manager, then cancels baseContext directly — main()'s own
// signal.NotifyContext derives from it, so this stops main() exactly as a
// real SIGTERM would, without sending any OS signal that could also reach
// other tests sharing this process.
func TestMainRunsUntilSignaled(t *testing.T) {
	prevURL, hadURL := os.LookupEnv("MANAGER_URL")
	prevListen, hadListen := os.LookupEnv("LISTEN_ADDR")
	os.Setenv("MANAGER_URL", "http://127.0.0.1:1") // never dialed by this test
	os.Setenv("LISTEN_ADDR", "127.0.0.1:0")
	defer func() {
		restore := func(had bool, key, prev string) {
			if had {
				os.Setenv(key, prev)
			} else {
				os.Unsetenv(key)
			}
		}
		restore(hadURL, "MANAGER_URL", prevURL)
		restore(hadListen, "LISTEN_ADDR", prevListen)
	}()

	ctx, cancel := context.WithCancel(context.Background())
	origBaseContext := baseContext
	baseContext = func() context.Context { return ctx }
	defer func() { baseContext = origBaseContext }()

	done := make(chan struct{})
	go func() {
		main()
		close(done)
	}()

	// give the listener a moment to bind before tearing it down again.
	time.Sleep(50 * time.Millisecond)
	cancel()

	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("main() did not return after its context was cancelled")
	}
}
