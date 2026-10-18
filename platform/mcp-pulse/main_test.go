package main

import (
	"context"
	"net"
	"os"
	"testing"
	"time"
)

// TestMainRunsUntilSignaled closes main() itself: it starts the real process
// loop (env defaulting, the real HTTP server) against a real HTTP double for
// Pulse, then cancels baseContext directly — main()'s own
// signal.NotifyContext derives from it, so this stops main() exactly as a
// real SIGTERM would, without sending any OS signal that could also reach
// other tests sharing this process.
func TestMainRunsUntilSignaled(t *testing.T) {
	// A free port picked up front, rather than ":0" left for main() to
	// resolve blind, so this test has an address it can actually dial to
	// know the server is up — a real synchronization point instead of a
	// fixed sleep (signals/pulse/main_test.go's TestMainRunsUntilSignaled
	// uses the equivalent real signal on its own loop, a status report).
	probe, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("reserve a free port: %v", err)
	}
	addr := probe.Addr().String()
	probe.Close()

	prevURL, hadURL := os.LookupEnv("PULSE_URL")
	prevListen, hadListen := os.LookupEnv("LISTEN_ADDR")
	os.Setenv("PULSE_URL", "http://127.0.0.1:1") // never dialed by this test
	os.Setenv("LISTEN_ADDR", addr)
	defer func() {
		restore := func(had bool, key, prev string) {
			if had {
				os.Setenv(key, prev)
			} else {
				os.Unsetenv(key)
			}
		}
		restore(hadURL, "PULSE_URL", prevURL)
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

	deadline := time.Now().Add(5 * time.Second)
	for {
		if conn, err := net.Dial("tcp", addr); err == nil {
			conn.Close()
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("main() never started listening")
		}
		time.Sleep(5 * time.Millisecond)
	}
	cancel()

	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("main() did not return after its context was cancelled")
	}
}
