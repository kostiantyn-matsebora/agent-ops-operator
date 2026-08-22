package main

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// A console with no signal identity cannot start conversations, and says so
// with the fix rather than failing generically.
func TestOriginatorAbsentWithoutASignalIdentity(t *testing.T) {
	if o := NewOriginator("http://mgr", "", "console"); o != nil {
		t.Fatal("no signal token must mean no originator")
	}
	if o := NewOriginator("http://mgr", "tok", ""); o != nil {
		t.Fatal("no source name must mean no originator")
	}
	var o *Originator
	_, err := o.Start(context.Background(), "console-chan", "alice", "sha256:alice", "hi")
	if err == nil || !strings.Contains(err.Error(), "servedBy") {
		t.Fatalf("the error must name the fix, got %v", err)
	}
}

// The signal carries what the manager requires to answer: kind chat, the
// channel label (without it the reply has nowhere to go and /signal/inbound
// refuses the signal) and the sender, using the SIGNAL token.
func TestOriginatorPostsAChatSignal(t *testing.T) {
	var gotAuth string
	var body map[string]any
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/signal/inbound" {
			t.Errorf("origination must use the signal path, got %s", r.URL.Path)
		}
		gotAuth = r.Header.Get("Authorization")
		raw, _ := io.ReadAll(r.Body)
		_ = json.Unmarshal(raw, &body)
		_ = json.NewEncoder(w).Encode(map[string]any{"queued": 1, "conversations": 1})
	}))
	defer srv.Close()

	o := NewOriginator(srv.URL, "signal-token", "console")
	reason, err := o.Start(context.Background(), "console-chan", "alice", "sha256:alice", "check the nodes")
	if err != nil {
		t.Fatal(err)
	}
	if reason != "" {
		t.Fatalf("a claimed source reports no drop reason, got %q", reason)
	}
	if gotAuth != "Bearer signal-token" {
		t.Fatalf("origination must use the signal identity, sent %q", gotAuth)
	}
	if body["source"] != "console" {
		t.Fatalf("source: %v", body["source"])
	}
	signals, _ := body["signals"].([]any)
	if len(signals) != 1 {
		t.Fatalf("signals: %v", body["signals"])
	}
	sig, _ := signals[0].(map[string]any)
	if sig["kind"] != "chat" || sig["payload"] != "check the nodes" {
		t.Fatalf("signal: %v", sig)
	}
	labels, _ := sig["labels"].(map[string]any)
	if labels[labelChatChannel] != "console-chan" || labels[labelChatSender] != "alice" {
		t.Fatalf("labels: %v", labels)
	}
	if fp, _ := sig["fingerprint"].(string); !strings.HasPrefix(fp, "console-") {
		t.Fatalf("fingerprint: %v", sig["fingerprint"])
	}

	// two messages must not dedup: a person asking twice means it twice
	first, _ := signals[0].(map[string]any)["fingerprint"].(string)
	if _, err := o.Start(context.Background(), "console-chan", "alice", "sha256:alice", "check the nodes"); err != nil {
		t.Fatal(err)
	}
	signals, _ = body["signals"].([]any)
	if second, _ := signals[0].(map[string]any)["fingerprint"].(string); second == first {
		t.Fatal("repeated messages must carry distinct fingerprints")
	}
}

// An UNCLAIMED source is not an error: the manager answers with the Wired=False
// reason, and that reason is what the operator needs to see.
func TestOriginatorSurfacesTheDropReason(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{
			"queued": 0, "conversations": 0,
			"reason": "source not claimed by a Ready pipeline (Wired=False) — signals dropped",
		})
	}))
	defer srv.Close()

	o := NewOriginator(srv.URL, "signal-token", "console")
	reason, err := o.Start(context.Background(), "console-chan", "alice", "sha256:alice", "anyone there?")
	if err != nil {
		t.Fatalf("an unclaimed source is a reported reason, not a transport error: %v", err)
	}
	if !strings.Contains(reason, "Wired=False") {
		t.Fatalf("the Wired=False reason must reach the caller verbatim, got %q", reason)
	}
}

// Without a served console Channel there is nowhere to answer, so the console
// refuses before posting rather than letting the manager reject it.
func TestOriginatorRefusesWithoutAChannel(t *testing.T) {
	o := NewOriginator("http://unused", "signal-token", "console")
	if _, err := o.Start(context.Background(), "", "alice", "sha256:alice", "hi"); err == nil {
		t.Fatal("origination without a channel must be refused")
	}
}
