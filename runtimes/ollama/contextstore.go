// The context store: one JSON transcript per context under
// $HOME/.agentops/contexts/<id>.json, and the opaque handle the manager keeps.
//
// The layout is this runtime's and is known to nothing else. Under
// context-sync the directory is what the bundle's contextSync.paths names, so
// the sidecar restores it before the first unit and snapshots it after each.
package main

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// Transcript is the stored context.
type Transcript struct {
	ID           string    `json:"id"`
	Conversation string    `json:"conversation"`
	Created      time.Time `json:"created"`
	Updated      time.Time `json:"updated"`
	Messages     []Message `json:"messages"`
}

// ContextStore reads and writes transcripts.
type ContextStore struct {
	Dir string
	// Sleep is the re-check delay; tests replace it.
	Sleep func(time.Duration)
}

// ErrContextMissing is a CONFIRMED absence, after re-checks.
var ErrContextMissing = errors.New("context not found")

func newContextID() string {
	b := make([]byte, 6)
	_, _ = rand.Read(b)
	return "oc-" + hex.EncodeToString(b)
}

func (s *ContextStore) path(id string) string { return filepath.Join(s.Dir, id+".json") }

// New creates an empty transcript for a conversation.
func (s *ContextStore) New(conversation string) *Transcript {
	now := time.Now().UTC()
	return &Transcript{ID: newContextID(), Conversation: conversation, Created: now, Updated: now}
}

// Load reads a transcript by handle, distinguishing GONE from SLOW.
//
// A miss is re-checked after 500 ms, 1.5 s and 3 s — a shared volume can fail
// to answer for seconds without having lost anything. A read that ERRORS is
// unavailability of the STORE, returned as that error, never as absence. Only
// three consecutive "not there" answers are ErrContextMissing.
func (s *ContextStore) Load(id string) (*Transcript, error) {
	if !validID(id) {
		return nil, fmt.Errorf("%w: %q is not a handle this runtime issued", ErrContextMissing, id)
	}
	t, err := s.read(id)
	if err == nil {
		return t, nil
	}
	if !os.IsNotExist(err) {
		return nil, fmt.Errorf("context store unavailable: %w", err)
	}
	for _, d := range []time.Duration{500 * time.Millisecond, 1500 * time.Millisecond, 3 * time.Second} {
		s.Sleep(d)
		t, err = s.read(id)
		if err == nil {
			return t, nil
		}
		if !os.IsNotExist(err) {
			return nil, fmt.Errorf("context store unavailable: %w", err)
		}
	}
	return nil, ErrContextMissing
}

func (s *ContextStore) read(id string) (*Transcript, error) {
	// Stat the directory first: an unreadable store is an error here, where a
	// missing file alone would read as absence.
	if _, err := os.Stat(s.Dir); err != nil {
		return nil, err
	}
	data, err := os.ReadFile(s.path(id))
	if err != nil {
		return nil, err
	}
	var t Transcript
	if err := json.Unmarshal(data, &t); err != nil {
		return nil, fmt.Errorf("%s: %w", s.path(id), err)
	}
	return &t, nil
}

// Save writes atomically: temp file plus rename.
func (s *ContextStore) Save(t *Transcript) error {
	if err := os.MkdirAll(s.Dir, 0o755); err != nil {
		return err
	}
	t.Updated = time.Now().UTC()
	data, err := json.Marshal(t)
	if err != nil {
		return err
	}
	tmp, err := os.CreateTemp(s.Dir, "."+t.ID+".*.tmp")
	if err != nil {
		return err
	}
	if _, err := tmp.Write(data); err != nil {
		tmp.Close()
		os.Remove(tmp.Name())
		return err
	}
	if err := tmp.Close(); err != nil {
		os.Remove(tmp.Name())
		return err
	}
	return os.Rename(tmp.Name(), s.path(t.ID))
}

func validID(id string) bool {
	return strings.HasPrefix(id, "oc-") && len(id) == 15 && !strings.ContainsAny(id, "/\\")
}

// Trim fits the transcript to a token budget: the system prompt and the
// current turn always stay, oldest exchanges go first. Tokens are estimated at
// four bytes each — a budget, not a count. Returns the messages to send and
// how many were dropped.
func Trim(system Message, history []Message, current Message, budget int) ([]Message, int) {
	cost := func(m Message) int {
		n := len(m.Content)/4 + 4
		for _, tc := range m.ToolCalls {
			n += (len(tc.Function.Name) + len(tc.Function.Arguments)) / 4
		}
		return n
	}
	total := cost(system) + cost(current)
	keepFrom := len(history)
	for i := len(history) - 1; i >= 0; i-- {
		c := cost(history[i])
		if total+c > budget {
			break
		}
		total += c
		keepFrom = i
	}
	// Never start the kept history on a tool result: its call was dropped.
	for keepFrom < len(history) && history[keepFrom].Role == "tool" {
		keepFrom++
	}
	out := make([]Message, 0, 2+len(history)-keepFrom)
	out = append(out, system)
	out = append(out, history[keepFrom:]...)
	out = append(out, current)
	return out, keepFrom
}
