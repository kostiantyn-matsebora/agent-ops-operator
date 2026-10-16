package main

// HMAC verification for inbound Pulse webhooks, per the pulse-signal-adapter
// spec's "Inbound webhook signatures are verified when a signing secret is
// configured" requirement: HMAC-SHA256 over timestamp + "." + body, checked
// against X-Pulse-Timestamp and X-Pulse-Signature. Gated on a configured
// signing secret — register.go sets the same value as Pulse's own
// signingSecret when it registers the webhook, so the two sides agree on
// the secret without either carrying the other's configuration.
//
// THE "v1=" PREFIX IS LOAD-BEARING, confirmed against Pulse's own
// internal/notifications/webhook_signing_test.go: X-Pulse-Signature is
// always "v1=<hex-digest>", never a bare digest. An earlier version of
// this file compared against the bare digest, which would have rejected
// every genuinely signed delivery a real Pulse instance ever sends — this
// is the one place this module's behavior had to match Pulse's actual
// source rather than its own best guess at the Generic webhook contract.

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"strconv"
	"strings"
	"time"
)

const signaturePrefix = "v1="

// signatureTolerance bounds how far a signed request's timestamp may sit
// from now, in either direction, rejecting a stale replay of an otherwise
// validly-signed body.
const signatureTolerance = 5 * time.Minute

// verifySignature checks a Pulse webhook's HMAC-SHA256 signature against the
// configured secret and bounds its timestamp to signatureTolerance of now.
// A missing header, an unparsable timestamp, a stale timestamp, a missing
// "v1=" prefix, or a mismatched signature all return false — handleWebhook
// treats every one of them identically: reject with 401, never parse the
// body.
func verifySignature(secret, timestampHeader, signatureHeader string, body []byte, now time.Time) bool {
	if timestampHeader == "" || signatureHeader == "" {
		return false
	}
	digest, ok := strings.CutPrefix(signatureHeader, signaturePrefix)
	if !ok {
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

	return hmac.Equal([]byte(want), []byte(digest))
}
