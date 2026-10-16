package main

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"
	"time"
)

// sign independently computes the signature a correctly-configured Pulse
// would send for a given secret, timestamp and body — HMAC-SHA256 over
// timestamp + "." + body, per the documented scheme — so a bug shared
// between this fixture and verifySignature's own construction could not
// hide a broken check.
func sign(t *testing.T, secret, timestamp string, body []byte) string {
	t.Helper()
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write([]byte(timestamp))
	mac.Write([]byte("."))
	mac.Write(body)
	return hex.EncodeToString(mac.Sum(nil))
}

func TestVerifySignatureValidIsAccepted(t *testing.T) {
	now := time.Now()
	ts := strconv.FormatInt(now.Unix(), 10)
	body := []byte(firingBody("x"))
	sig := sign(t, "s3cret", ts, body)
	if !verifySignature("s3cret", ts, sig, body, now) {
		t.Fatal("a correctly signed, fresh request must be accepted")
	}
}

func TestVerifySignatureInvalidIsRejected(t *testing.T) {
	now := time.Now()
	ts := strconv.FormatInt(now.Unix(), 10)
	body := []byte(firingBody("x"))
	if verifySignature("s3cret", ts, "deadbeef", body, now) {
		t.Fatal("a mismatched signature must be rejected")
	}
	// signed with the wrong secret
	wrongSecretSig := sign(t, "wrong", ts, body)
	if verifySignature("s3cret", ts, wrongSecretSig, body, now) {
		t.Fatal("a signature computed with a different secret must be rejected")
	}
}

func TestVerifySignatureMissingHeadersIsRejected(t *testing.T) {
	now := time.Now()
	ts := strconv.FormatInt(now.Unix(), 10)
	body := []byte(firingBody("x"))
	sig := sign(t, "s3cret", ts, body)
	if verifySignature("s3cret", "", sig, body, now) {
		t.Fatal("a missing timestamp header must be rejected")
	}
	if verifySignature("s3cret", ts, "", body, now) {
		t.Fatal("a missing signature header must be rejected")
	}
	if verifySignature("s3cret", "not-a-number", sig, body, now) {
		t.Fatal("an unparsable timestamp must be rejected")
	}
}

func TestVerifySignatureStaleTimestampIsRejected(t *testing.T) {
	now := time.Now()
	stale := now.Add(-10 * time.Minute)
	ts := strconv.FormatInt(stale.Unix(), 10)
	body := []byte(firingBody("x"))
	sig := sign(t, "s3cret", ts, body)
	if verifySignature("s3cret", ts, sig, body, now) {
		t.Fatal("a timestamp outside the tolerance window must be rejected")
	}

	future := now.Add(10 * time.Minute)
	tsFuture := strconv.FormatInt(future.Unix(), 10)
	sigFuture := sign(t, "s3cret", tsFuture, body)
	if verifySignature("s3cret", tsFuture, sigFuture, body, now) {
		t.Fatal("a timestamp too far in the future must be rejected too")
	}
}

// TestHandleWebhookSignatureEndToEnd exercises the full handler, not just
// verifySignature in isolation: valid signature accepted and parsed,
// invalid signature 401'd before parsing, missing signature 401'd when a
// secret is configured, and unsigned mode accepting a bare post when no
// secret is configured at all.
func TestHandleWebhookSignatureEndToEnd(t *testing.T) {
	f := &fakeManager{sources: []SourceInfo{{Name: "pulse-default"}}}
	a := testAdapter(t, f)
	a.sources["pulse-default"] = "s3cret"
	h := a.handler()
	body := firingBody("sig-1")

	now := time.Now()
	ts := strconv.FormatInt(now.Unix(), 10)
	sig := sign(t, "s3cret", ts, []byte(body))

	// valid signature, inside tolerance → parsed and pushed
	req := httptest.NewRequest("POST", "/webhook/pulse-default", strings.NewReader(body))
	req.Header.Set("X-Pulse-Timestamp", ts)
	req.Header.Set("X-Pulse-Signature", sig)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != 200 {
		t.Fatalf("valid signature: %d %s", rec.Code, rec.Body.String())
	}

	// invalid signature → 401, nothing parsed/pushed
	req = httptest.NewRequest("POST", "/webhook/pulse-default", strings.NewReader(body))
	req.Header.Set("X-Pulse-Timestamp", ts)
	req.Header.Set("X-Pulse-Signature", "not-the-right-signature")
	rec = httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != 401 {
		t.Fatalf("invalid signature: %d %s", rec.Code, rec.Body.String())
	}

	// missing signature headers entirely, secret configured → 401
	rec = post(t, h, "/webhook/pulse-default", body)
	if rec.Code != 401 {
		t.Fatalf("missing signature with secret configured: %d %s", rec.Code, rec.Body.String())
	}

	f.mu.Lock()
	pushed := len(f.inbounds)
	f.mu.Unlock()
	if pushed != 1 {
		t.Fatalf("exactly one signed request should have been accepted: %d pushes", pushed)
	}

	// unsigned mode: no secret configured at all → any body accepted
	a.sources["pulse-default"] = ""
	rec = post(t, h, "/webhook/pulse-default", firingBody("sig-2"))
	if rec.Code != 200 {
		t.Fatalf("unsigned mode should accept a bare post: %d %s", rec.Code, rec.Body.String())
	}
}
