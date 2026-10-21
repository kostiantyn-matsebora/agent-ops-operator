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
	exception := strings.Repeat("Traceback <module> -> raise ValueError()\n", (256<<10)/40)

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

// The binary search lands on the LONGEST kept prefix that fits, and that
// prefix ends on a rune boundary. Checked against every possible cut, at every
// limit from the note alone up to the unclamped size, over text mixing one- to
// four-byte runes, six-fold JSON escapes and a quote — and once more with a
// stray continuation byte, which no cut may land in front of.
func TestClampFindsTheLongestFittingRuneBoundary(t *testing.T) {
	for _, tc := range []struct {
		name    string
		payload string
		valid   bool
	}{
		{"valid UTF-8", strings.Repeat("aé€😀<\"", 12), true},
		{"a stray continuation byte", strings.Repeat("aé€😀<\x80\"", 12), false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			assertClampOptimalAtEveryLimit(t, tc.payload, tc.valid)
		})
	}
}

func assertClampOptimalAtEveryLimit(t *testing.T, payload string, valid bool) {
	t.Helper()
	base := Signal{InitiationID: "ha-logs@x", Payload: payload}
	isBoundary := func(k int) bool { return k == len(payload) || utf8.RuneStart(payload[k]) }
	sizeAt := func(k int) int {
		return encodedLen(Signal{InitiationID: base.InitiationID, Payload: clampedPayload(payload, k)})
	}
	for limit := sizeAt(0); limit < encodedLen(base); limit++ {
		best := 0
		for k := 0; k <= len(payload); k++ {
			if isBoundary(k) && sizeAt(k) <= limit {
				best = k
			}
		}
		got := clampSignal(base, limit)
		if n := encodedLen(got); n > limit {
			t.Fatalf("limit %d: encoded %d bytes, over the limit", limit, n)
		}
		kept := got.Payload[:strings.LastIndex(got.Payload, "\n\n[payload clamped: ")]
		if !strings.HasPrefix(payload, kept) || !isBoundary(len(kept)) {
			t.Fatalf("limit %d: kept %d bytes, not a prefix ending on a rune boundary", limit, len(kept))
		}
		if len(kept) != best {
			t.Fatalf("limit %d: kept %d bytes, the longest fitting cut keeps %d", limit, len(kept), best)
		}
		if valid && !utf8.ValidString(got.Payload) {
			t.Fatalf("limit %d: clamped payload is not valid UTF-8", limit)
		}
	}
}

// A limit too small for any of the payload keeps the note alone.
func TestClampKeepsTheNoteWhenNothingFits(t *testing.T) {
	sig := clampSignal(Signal{InitiationID: "ha-logs@x", Payload: strings.Repeat("x", 4096)}, 64)
	if !strings.HasPrefix(sig.Payload, "\n\n[payload clamped: 0 of 4096 bytes kept") {
		t.Fatalf("payload = %q, want the note alone", sig.Payload)
	}
}
