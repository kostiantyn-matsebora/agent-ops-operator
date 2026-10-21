package main

import (
	"encoding/json"
	"fmt"
	"unicode/utf8"
)

// The payload clamp: every batch this adapter posts fits the contract's
// `POST /signal/inbound` body bound.
//
// A log record's payload carries Home Assistant's own text — the message and
// the exception traceback — and neither is bounded at the source. One runaway
// traceback, multiplied by a full batch, would exceed the bound and the manager
// would refuse the WHOLE batch: every other signal in it lost for one record's
// size. So the one unbounded field is clamped before the post, and the clamp is
// written into the payload itself, so a reader knows the text was cut rather
// than mistaking a truncated traceback for the whole of it.
//
// Each signal is held to an equal share of the bound. A batch never carries
// more than maxBatchPerSrc signals, so equal shares bound the batch by
// construction — no second pass, no signal penalized for a neighbour's size.

const (
	// maxInboundBody is the contract's bound on a `POST /signal/inbound`
	// request body (docs/contracts/signal-adapter.md, Bounds).
	maxInboundBody = 1 << 20
	// inboundEnvelopeReserve is held back from the bound for the request's
	// own envelope — the source name, the keys, the array's brackets and
	// separators — so the shares below never add up to the bound exactly.
	inboundEnvelopeReserve = 4 << 10
	// maxEncodedSignal is one signal's share of the bound, measured as its
	// JSON encoding, which is what the bound counts.
	maxEncodedSignal = (maxInboundBody - inboundEnvelopeReserve) / maxBatchPerSrc
	// clampNoteFormat is appended to a clamped payload, naming what was kept
	// of how much and why.
	clampNoteFormat = "\n\n[payload clamped: %d of %d bytes kept to fit the 1 MiB /signal/inbound bound]"
)

// clampBatch returns sigs with every payload clamped to its share of the
// inbound bound. The caller's slice is not modified.
func clampBatch(sigs []Signal) []Signal {
	out := make([]Signal, len(sigs))
	for i, sig := range sigs {
		out[i] = clampSignal(sig, maxEncodedSignal)
	}
	return out
}

// clampSignal cuts sig's payload until the signal's JSON encoding fits within
// limit bytes, ending the payload with the clamp note. A signal already within
// the limit is returned unchanged.
//
// JSON escaping inflates some bytes up to six-fold, so how much raw text fits
// is found by measuring, not by arithmetic: a binary search for the longest
// kept prefix whose encoding fits. Encoding never shrinks text, so no more
// than limit raw bytes can fit, which keeps every measurement small.
func clampSignal(sig Signal, limit int) Signal {
	if encodedLen(sig) <= limit {
		return sig
	}
	original := sig.Payload
	fits := func(keep int) bool {
		sig.Payload = clampedPayload(original, keep)
		return encodedLen(sig) <= limit
	}
	lo, hi := 0, min(len(original), limit)
	for lo < hi {
		mid := lo + (hi-lo+1)/2
		if fits(mid) {
			lo = mid
		} else {
			hi = mid - 1
		}
	}
	// lo == 0 is the note alone: the fields outside the payload are bounded
	// upstream (title, labels), so the payload is all there is to give up.
	sig.Payload = clampedPayload(original, lo)
	return sig
}

// clampedPayload keeps at most keep bytes of payload, cut back to a rune
// boundary so the kept text stays valid UTF-8, followed by the clamp note.
func clampedPayload(payload string, keep int) string {
	if keep > len(payload) {
		keep = len(payload)
	}
	for keep > 0 && keep < len(payload) && !utf8.RuneStart(payload[keep]) {
		keep--
	}
	return payload[:keep] + fmt.Sprintf(clampNoteFormat, keep, len(payload))
}

// encodedLen is the size sig occupies in the request body.
func encodedLen(sig Signal) int {
	b, err := json.Marshal(sig)
	if err != nil {
		// Signal holds only strings and a string map: Marshal cannot fail.
		// Should that ever change, report the signal as oversize so it is
		// clamped rather than posted unmeasured.
		return maxInboundBody + 1
	}
	return len(b)
}
