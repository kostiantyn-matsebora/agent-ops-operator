package main

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func newStore(t *testing.T) *ContextStore {
	return &ContextStore{Dir: filepath.Join(t.TempDir(), "contexts"), Sleep: func(time.Duration) {}}
}

func TestCreateAndContinue(t *testing.T) {
	s := newStore(t)
	tr := s.New("conv-1")
	if !validID(tr.ID) {
		t.Errorf("id shape: %q", tr.ID)
	}
	tr.Messages = append(tr.Messages, Message{Role: "user", Content: "hi"})
	if err := s.Save(tr); err != nil {
		t.Fatal(err)
	}
	got, err := s.Load(tr.ID)
	if err != nil || len(got.Messages) != 1 || got.Conversation != "conv-1" {
		t.Errorf("load: %+v %v", got, err)
	}
	if entries, _ := os.ReadDir(s.Dir); len(entries) != 1 {
		t.Errorf("atomic write left temp files: %d entries", len(entries))
	}
}

func TestSlowThenFound(t *testing.T) {
	s := newStore(t)
	os.MkdirAll(s.Dir, 0o755)
	tr := s.New("c")
	var checks int
	s.Sleep = func(time.Duration) {
		checks++
		if checks == 2 {
			s.Save(tr)
		}
	}
	got, err := s.Load(tr.ID)
	if err != nil || got.ID != tr.ID {
		t.Errorf("a store that was slow must not read as gone: %v", err)
	}
}

func TestConfirmedMissing(t *testing.T) {
	s := newStore(t)
	os.MkdirAll(s.Dir, 0o755)
	var delays []time.Duration
	s.Sleep = func(d time.Duration) { delays = append(delays, d) }
	_, err := s.Load("oc-000000000000")
	if !errors.Is(err, ErrContextMissing) {
		t.Errorf("want ErrContextMissing, got %v", err)
	}
	if len(delays) != 3 || delays[0] != 500*time.Millisecond || delays[2] != 3*time.Second {
		t.Errorf("re-check cadence: %v", delays)
	}
	if _, err := s.Load("not-a-handle/../x"); !errors.Is(err, ErrContextMissing) {
		t.Errorf("a foreign handle is missing, never a path: %v", err)
	}
}

func TestStoreErrorIsNotAbsence(t *testing.T) {
	s := newStore(t)
	// A read that ERRORS — here a file sitting where the directory should be,
	// so every read fails with ENOTDIR — is the store not answering. A stale
	// mount looks the same. It must not be reported as the context being gone.
	os.WriteFile(s.Dir, []byte("not a directory"), 0o644)
	_, err := s.Load("oc-000000000000")
	if errors.Is(err, ErrContextMissing) || err == nil || !strings.Contains(err.Error(), "unavailable") {
		t.Errorf("an unreadable store is unavailability, got %v", err)
	}
}

func TestTrim(t *testing.T) {
	sys := Message{Role: "system", Content: strings.Repeat("s", 40)}
	cur := Message{Role: "user", Content: strings.Repeat("u", 40)}
	var hist []Message
	for i := 0; i < 10; i++ {
		hist = append(hist, Message{Role: "user", Content: strings.Repeat("h", 40)}, Message{Role: "tool", Content: strings.Repeat("t", 40)})
	}
	// each message costs 14; system+current = 28; budget 100 keeps 5
	out, dropped := Trim(sys, hist, cur, 100)
	if dropped == 0 || out[0].Role != "system" || out[len(out)-1].Role != "user" || out[len(out)-1].Content != cur.Content {
		t.Errorf("system and current must survive: dropped=%d first=%s last=%s", dropped, out[0].Role, out[len(out)-1].Role)
	}
	if out[1].Role == "tool" {
		t.Error("kept history must not open on an orphaned tool result")
	}
	out, dropped = Trim(sys, hist, cur, 1_000_000)
	if dropped != 0 || len(out) != 22 {
		t.Errorf("a large budget drops nothing: dropped=%d len=%d", dropped, len(out))
	}
}
