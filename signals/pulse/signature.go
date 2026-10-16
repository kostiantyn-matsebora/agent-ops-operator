package main

// HMAC verification for inbound Pulse webhooks, per the pulse-signal-adapter
// spec's "Inbound webhook signatures are verified when a signing secret is
// configured" requirement: HMAC-SHA256 over timestamp + "." + body, checked
// against X-Pulse-Timestamp and X-Pulse-Signature. Gated on
// PULSE_SIGNING_SECRET — register.go sets the same value as Pulse's own
// signingSecret when it registers the webhook, so the two sides agree on
// the secret without either carrying the other's configuration.

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"strconv"
	"time"
)

// signatureTolerance bounds how far a signed request's timestamp may sit
// from now, in either direction, rejecting a stale replay of an otherwise
// validly-signed body.
const signatureTolerance = 5 * time.Minute

// verifySignature checks a Pulse webhook's HMAC-SHA256 signature against the
// configured secret and bounds its timestamp to signatureTolerance of now.
// A missing header, an unparsable timestamp, a stale timestamp or a
// mismatched signature all return false — handleWebhook treats every one of
// them identically: reject with 401, never parse the body.
func verifySignature(secret, timestampHeader, signatureHeader string, body []byte, now time.Time) bool {
	if timestampHeader == "" || signatureHeader == "" {
		return false
	}
	ts, err := strconv.ParseInt(timestampHeader, 10, 64)
	if err != nil {
		return false
	}
	if age := now.Sub(time.Unix(ts, 0)); age > signatureTolerance || age < -signatureTolerance {
		return false
	}

	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write([]byte(timestampHeader))
	mac.Write([]byte("."))
	mac.Write(body)
	want := hex.EncodeToString(mac.Sum(nil))

	return hmac.Equal([]byte(want), []byte(signatureHeader))
}
