package main

import (
	"bytes"
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"errors"
	"io"
	"math/big"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// kube.go carried no dedicated test file: every existing test drove the cache
// through the fakeSource interface, never the real in-cluster HTTP client.
// These tests exercise that client directly — apiError, saDir,
// NewInClusterKube's whole construction path, token caching/refresh, and the
// real Watch method's frame handling — against a real httptest.Server rather
// than a mock of net/http.

// closes a real gap: apiError formats the code and body an operator sees in a
// log line, and nothing had ever called it.
func TestAPIErrorMessage(t *testing.T) {
	err := &apiError{Code: 403, Body: "forbidden"}
	if got := err.Error(); got != "kubernetes API 403: forbidden" {
		t.Fatalf("got %q", got)
	}
}

// closes a real gap: saDir's env override is what lets the conformance suite
// point the client at a fake ServiceAccount mount, and nothing tested it.
func TestSaDirDefaultAndOverride(t *testing.T) {
	old, had := os.LookupEnv("KUBERNETES_SERVICEACCOUNT_DIR")
	t.Cleanup(func() {
		if had {
			os.Setenv("KUBERNETES_SERVICEACCOUNT_DIR", old)
		} else {
			os.Unsetenv("KUBERNETES_SERVICEACCOUNT_DIR")
		}
	})
	os.Unsetenv("KUBERNETES_SERVICEACCOUNT_DIR")
	if saDir() != defaultSADir {
		t.Fatalf("want default %q, got %q", defaultSADir, saDir())
	}
	os.Setenv("KUBERNETES_SERVICEACCOUNT_DIR", "/custom/sa")
	if saDir() != "/custom/sa" {
		t.Fatalf("override not honored: %q", saDir())
	}
}

// selfSignedCAPEM builds a minimal valid CA certificate in PEM form, real
// enough for x509.CertPool.AppendCertsFromPEM to accept — the same shape a
// kubelet-projected ca.crt has, without needing a live cluster.
func selfSignedCAPEM(t *testing.T) []byte {
	t.Helper()
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	tmpl := &x509.Certificate{
		SerialNumber:          big.NewInt(1),
		Subject:               pkix.Name{CommonName: "test-ca"},
		NotBefore:             time.Now().Add(-time.Hour),
		NotAfter:              time.Now().Add(time.Hour),
		IsCA:                  true,
		KeyUsage:              x509.KeyUsageCertSign,
		BasicConstraintsValid: true,
	}
	der, err := x509.CreateCertificate(rand.Reader, tmpl, tmpl, &key.PublicKey, key)
	if err != nil {
		t.Fatal(err)
	}
	var buf bytes.Buffer
	if err := pem.Encode(&buf, &pem.Block{Type: "CERTIFICATE", Bytes: der}); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}

// writeFakeSAMount lays out a ca.crt + token file the way the kubelet projects
// one, and points KUBERNETES_SERVICEACCOUNT_DIR at it for the duration of t.
func writeFakeSAMount(t *testing.T, caPEM []byte, token string) string {
	t.Helper()
	dir := t.TempDir()
	if caPEM != nil {
		if err := os.WriteFile(filepath.Join(dir, "ca.crt"), caPEM, 0o600); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.WriteFile(filepath.Join(dir, "token"), []byte(token), 0o600); err != nil {
		t.Fatal(err)
	}
	old, had := os.LookupEnv("KUBERNETES_SERVICEACCOUNT_DIR")
	os.Setenv("KUBERNETES_SERVICEACCOUNT_DIR", dir)
	t.Cleanup(func() {
		if had {
			os.Setenv("KUBERNETES_SERVICEACCOUNT_DIR", old)
		} else {
			os.Unsetenv("KUBERNETES_SERVICEACCOUNT_DIR")
		}
	})
	return dir
}

func clearKubeServiceEnv(t *testing.T) {
	t.Helper()
	for _, k := range []string{"KUBERNETES_SERVICE_HOST", "KUBERNETES_SERVICE_PORT"} {
		old, had := os.LookupEnv(k)
		os.Unsetenv(k)
		t.Cleanup(func() {
			if had {
				os.Setenv(k, old)
			} else {
				os.Unsetenv(k)
			}
		})
	}
}

// The whole construction path succeeds: bearer() only reads the local token
// file at construction time, so no real API server is needed to prove it.
func TestNewInClusterKubeSuccess(t *testing.T) {
	clearKubeServiceEnv(t)
	writeFakeSAMount(t, selfSignedCAPEM(t), "sa-token")
	os.Setenv("KUBERNETES_SERVICE_HOST", "192.0.2.1")
	os.Setenv("KUBERNETES_SERVICE_PORT", "443")

	k, err := NewInClusterKube("agent-ops")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if k.BaseURL != "https://192.0.2.1:443" || k.Namespace != "agent-ops" {
		t.Fatalf("client built wrong: %+v", k)
	}
	tok, err := k.bearer()
	if err != nil || tok != "sa-token" {
		t.Fatalf("token not loaded from the mount: %q, %v", tok, err)
	}
}

func TestNewInClusterKubeMissingHostOrPort(t *testing.T) {
	clearKubeServiceEnv(t)
	if _, err := NewInClusterKube("agent-ops"); err == nil {
		t.Fatal("want an error with neither var set")
	}
	os.Setenv("KUBERNETES_SERVICE_HOST", "192.0.2.1")
	if _, err := NewInClusterKube("agent-ops"); err == nil {
		t.Fatal("want an error with only the host set")
	}
}

func TestNewInClusterKubeMissingCA(t *testing.T) {
	clearKubeServiceEnv(t)
	writeFakeSAMount(t, nil, "sa-token") // no ca.crt written
	os.Setenv("KUBERNETES_SERVICE_HOST", "192.0.2.1")
	os.Setenv("KUBERNETES_SERVICE_PORT", "443")
	if _, err := NewInClusterKube("agent-ops"); err == nil {
		t.Fatal("want an error when ca.crt is missing")
	}
}

func TestNewInClusterKubeInvalidCA(t *testing.T) {
	clearKubeServiceEnv(t)
	writeFakeSAMount(t, []byte("not a pem certificate"), "sa-token")
	os.Setenv("KUBERNETES_SERVICE_HOST", "192.0.2.1")
	os.Setenv("KUBERNETES_SERVICE_PORT", "443")
	_, err := NewInClusterKube("agent-ops")
	if err == nil || !strings.Contains(err.Error(), "not valid PEM") {
		t.Fatalf("want a PEM validation error, got %v", err)
	}
}

// bearer() caches the token for tokenMaxAge, re-reads it once stale, and falls
// back to the cached value on a transient read failure — three branches no
// other test in the suite reaches, because every other fixture uses an empty
// TokenPath (the "no projected token" branch, which IS already covered).
func TestBearerCachesAndRefreshes(t *testing.T) {
	dir := t.TempDir()
	tokenPath := filepath.Join(dir, "token")
	if err := os.WriteFile(tokenPath, []byte("first"), 0o600); err != nil {
		t.Fatal(err)
	}
	k := &Kube{TokenPath: tokenPath}

	tok, err := k.bearer()
	if err != nil || tok != "first" {
		t.Fatalf("first read: %q, %v", tok, err)
	}

	// change the file; within tokenMaxAge the cached value must still win
	if err := os.WriteFile(tokenPath, []byte("second"), 0o600); err != nil {
		t.Fatal(err)
	}
	if tok, err = k.bearer(); err != nil || tok != "first" {
		t.Fatalf("must serve the cached token before it goes stale: %q, %v", tok, err)
	}

	// force staleness (same package: touching the private field directly is
	// the only way to simulate five minutes passing)
	k.tokenLoaded = time.Now().Add(-2 * tokenMaxAge)
	if tok, err = k.bearer(); err != nil || tok != "second" {
		t.Fatalf("must re-read once stale: %q, %v", tok, err)
	}

	// a transient read failure after a successful load falls back to the last
	// good token rather than failing the caller outright
	if err := os.Remove(tokenPath); err != nil {
		t.Fatal(err)
	}
	k.tokenLoaded = time.Now().Add(-2 * tokenMaxAge)
	if tok, err = k.bearer(); err != nil || tok != "second" {
		t.Fatalf("must keep serving the last good token on a transient failure: %q, %v", tok, err)
	}
}

// bearer() with no cached token and an unreadable path must fail rather than
// silently authenticate as nobody.
func TestBearerFailsWithNoCacheAndNoFile(t *testing.T) {
	k := &Kube{TokenPath: filepath.Join(t.TempDir(), "missing")}
	if _, err := k.bearer(); err == nil {
		t.Fatal("want an error reading a token that was never there")
	}
}

// get() wraps a 4xx/5xx response into an *apiError carrying the status and
// body, which Watch depends on to recognise 410 Gone.
func TestGetWrapsErrorResponses(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusForbidden)
		_, _ = io.WriteString(w, "no rbac")
	}))
	defer srv.Close()
	k := &Kube{BaseURL: srv.URL, HTTP: srv.Client()}
	_, err := k.get(context.Background(), "/api/v1/namespaces/x/pods", 0)
	var apiErr *apiError
	if !errors.As(err, &apiErr) || apiErr.Code != http.StatusForbidden || apiErr.Body != "no rbac" {
		t.Fatalf("want a wrapped 403, got %v", err)
	}
}

// Object.Conditions/Condition on a status with no conditions field, and on
// malformed JSON, must answer "none" rather than panic — the console reads
// every kind through this, including ones a future CR version might not set
// status.conditions on at all yet.
func TestObjectConditionsHandlesAbsentAndMalformed(t *testing.T) {
	empty := &Object{}
	if empty.Conditions() != nil {
		t.Fatal("no status must report no conditions")
	}
	malformed := &Object{Status: []byte("not json")}
	if malformed.Conditions() != nil {
		t.Fatal("malformed status must report no conditions, not panic")
	}
	if malformed.Condition("Ready") != nil {
		t.Fatal("malformed status must report no matching condition")
	}
}

// watchServer streams the exact frames a real API server's watch endpoint
// would, one JSON object per Decode call, so Watch's frame-type switch and
// its expiry/error handling run against real (de)serialization rather than a
// hand-rolled fake of the method.
func watchServer(t *testing.T, body string) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, body)
	}))
}

// ADDED/MODIFIED/DELETED and BOOKMARK frames must all reach the callback, and
// the stream ending cleanly (EOF) must return nil — the caller re-watches.
func TestWatchDeliversEveryFrameKind(t *testing.T) {
	srv := watchServer(t,
		`{"type":"ADDED","object":{"metadata":{"name":"a"}}}`+"\n"+
			`{"type":"MODIFIED","object":{"metadata":{"name":"a"}}}`+"\n"+
			`{"type":"DELETED","object":{"metadata":{"name":"a"}}}`+"\n"+
			`{"type":"BOOKMARK","object":{"metadata":{"resourceVersion":"99"}}}`+"\n")
	defer srv.Close()
	k := &Kube{BaseURL: srv.URL, HTTP: srv.Client()}

	var seen []string
	err := k.Watch(context.Background(), "pipelines", "10", func(eventType string, obj *Object) {
		seen = append(seen, eventType+":"+obj.Metadata.Name+obj.Metadata.ResourceVersion)
	})
	if err != nil {
		t.Fatalf("clean EOF must return nil, got %v", err)
	}
	want := []string{"ADDED:a", "MODIFIED:a", "DELETED:a", "BOOKMARK:99"}
	if len(seen) != len(want) {
		t.Fatalf("got %v, want %v", seen, want)
	}
	for i := range want {
		if seen[i] != want[i] {
			t.Fatalf("frame %d: got %q, want %q", i, seen[i], want[i])
		}
	}
}

// An ERROR frame reporting Expired (the streaming equivalent of a 410) must
// map to ErrWatchExpired so the caller relists rather than retrying the watch.
func TestWatchErrorFrameExpired(t *testing.T) {
	srv := watchServer(t, `{"type":"ERROR","object":{"code":410,"reason":"Expired"}}`+"\n")
	defer srv.Close()
	k := &Kube{BaseURL: srv.URL, HTTP: srv.Client()}
	err := k.Watch(context.Background(), "pipelines", "1", func(string, *Object) {})
	if !errors.Is(err, ErrWatchExpired) {
		t.Fatalf("want ErrWatchExpired, got %v", err)
	}
}

// An ERROR frame for anything else must be reported as a real error, naming
// the reason, rather than silently swallowed as if the watch just ended.
func TestWatchErrorFrameOther(t *testing.T) {
	srv := watchServer(t, `{"type":"ERROR","object":{"code":500,"reason":"InternalError"}}`+"\n")
	defer srv.Close()
	k := &Kube{BaseURL: srv.URL, HTTP: srv.Client()}
	err := k.Watch(context.Background(), "pipelines", "1", func(string, *Object) {})
	if err == nil || !strings.Contains(err.Error(), "InternalError") {
		t.Fatalf("want an error naming the reason, got %v", err)
	}
}

// A 410 on the INITIAL request (rather than mid-stream) is the other path to
// ErrWatchExpired, via get()'s apiError wrapping.
func TestWatchInitialRequestGone(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusGone)
	}))
	defer srv.Close()
	k := &Kube{BaseURL: srv.URL, HTTP: srv.Client()}
	err := k.Watch(context.Background(), "pipelines", "1", func(string, *Object) {})
	if !errors.Is(err, ErrWatchExpired) {
		t.Fatalf("want ErrWatchExpired, got %v", err)
	}
}

// A cancelled context ends the watch loop with ctx.Err() rather than a decode
// error, distinguishing "we stopped" from "the server sent garbage".
func TestWatchContextCancelled(t *testing.T) {
	block := make(chan struct{})
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.(http.Flusher).Flush()
		<-block // hold the connection open with no frames until the test cancels
	}))
	defer srv.Close()
	defer close(block)
	k := &Kube{BaseURL: srv.URL, HTTP: srv.Client()}

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() {
		done <- k.Watch(ctx, "pipelines", "1", func(string, *Object) {})
	}()
	time.Sleep(50 * time.Millisecond)
	cancel()
	select {
	case err := <-done:
		if !errors.Is(err, context.Canceled) {
			t.Fatalf("want context.Canceled, got %v", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("Watch did not observe context cancellation")
	}
}

// Malformed JSON mid-stream must be reported as a decode error naming the
// kind, not silently dropped.
func TestWatchDecodeErrorMidStream(t *testing.T) {
	srv := watchServer(t, `{"type":"ADDED","object":{"metadata":{"name":"a"}}}`+"\n"+`{not json`)
	defer srv.Close()
	k := &Kube{BaseURL: srv.URL, HTTP: srv.Client()}
	var seen int
	err := k.Watch(context.Background(), "pipelines", "1", func(string, *Object) { seen++ })
	if err == nil || !strings.Contains(err.Error(), "decoding pipelines watch stream") {
		t.Fatalf("want a decode error naming the kind, got %v", err)
	}
	if seen != 1 {
		t.Fatalf("the frame before the bad one must still have been delivered: %d", seen)
	}
}
