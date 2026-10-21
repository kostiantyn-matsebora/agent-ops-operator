package main

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"testing"
	"time"
	"unicode/utf8"
)

// A full batch of records each carrying an oversized exception must still reach
// the manager — every signal in it, each payload saying it was clamped — rather
// than the whole batch being refused for exceeding the inbound bound. The
// traceback is full of `<` and `>`, which JSON escapes six-fold, so a clamp
// measuring raw bytes instead of the encoding would overshoot.
func TestOversizedExceptionBatchFitsTheInboundBound(t *testing.T) {
	fm := newFakeManager(t)
	a := newTestAdapter(fm)
	exception := strings.Repeat("Traceback <module> -> raise ValueError()\n", (2<<20)/40)

	sigs := make([]Signal, 0, maxBatchPerSrc)
	for i := 0; i < maxBatchPerSrc; i++ {
		rec := record("homeassistant.components.hue", "boom", time.Now(), 1)
		rec.Source[1] = json.RawMessage(fmt.Sprint(i + 1)) // a distinct location per record
		rec.Exception = exception
		sigs = append(sigs, normalize("ha-logs", &rec))
	}
	if body, _ := json.Marshal(map[string]any{"source": "ha-logs", "signals": sigs}); len(body) <= contractInboundBound {
		t.Fatalf("fixture is %d bytes, it must exceed the bound to test anything", len(body))
	}

	if !a.post(context.Background(), "ha-logs", sigs) {
		t.Fatalf("the clamped batch was refused: %+v", fm.Status())
	}
	posted := fm.Posted()
	if len(posted) != maxBatchPerSrc {
		t.Fatalf("posted %d signals, want all %d", len(posted), maxBatchPerSrc)
	}
	for _, sig := range posted {
		if !strings.Contains(sig.Payload, "[payload clamped: ") {
			t.Fatalf("a clamped payload must say so, got tail %q", sig.Payload[len(sig.Payload)-120:])
		}
		if !strings.HasPrefix(sig.Payload, "{") {
			t.Fatalf("the clamp must keep the payload's head, got %q", sig.Payload[:40])
		}
	}
	if sigs[0].Payload == posted[0].Payload {
		t.Fatal("the caller's batch must not be modified by the clamp")
	}
}

// A signal within its share passes untouched — the clamp is invisible until
// it is needed.
func TestClampLeavesASmallSignalUnchanged(t *testing.T) {
	sig := Signal{InitiationID: "ha-logs@x", Payload: "small"}
	if got := clampSignal(sig, maxEncodedSignal); got.Payload != "small" {
		t.Fatalf("payload = %q, want it untouched", got.Payload)
	}
}

// The cut lands on a rune boundary, so the kept text stays valid UTF-8, and the
// note names what was kept of how much.
func TestClampCutsOnARuneBoundary(t *testing.T) {
	payload := strings.Repeat("é", 1000) // two bytes each
	sig := clampSignal(Signal{InitiationID: "ha-logs@x", Payload: payload}, 501)
	if !utf8.ValidString(sig.Payload) {
		t.Fatal("clamped payload is not valid UTF-8")
	}
	if encodedLen(sig) > 501 {
		t.Fatalf("encoded %d bytes, over the 501-byte limit", encodedLen(sig))
	}
	if !strings.Contains(sig.Payload, fmt.Sprintf("of %d bytes kept", len(payload))) {
		t.Fatalf("the note must name the original size, got %q", sig.Payload)
	}
}

// A limit too small for any of the payload keeps the note alone.
func TestClampKeepsTheNoteWhenNothingFits(t *testing.T) {
	sig := clampSignal(Signal{InitiationID: "ha-logs@x", Payload: strings.Repeat("x", 4096)}, 64)
	if !strings.HasPrefix(sig.Payload, "\n\n[payload clamped: 0 of 4096 bytes kept") {
		t.Fatalf("payload = %q, want the note alone", sig.Payload)
	}
}
