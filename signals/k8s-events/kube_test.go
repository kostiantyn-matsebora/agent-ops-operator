package main

import (
	"context"
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/json"
	"encoding/pem"
	"errors"
	"math/big"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// This file closes the gap on NewInClusterKube, saDir, bearer's error paths and
// the transport's own decode-failure branches — the real construction and
// failure paths a pod actually exercises at startup and during a long-running
// watch, none of which the existing fixtures (which start from an
// already-built *Kube) ever drove.

// generateTestCA returns a self-signed certificate PEM. NewInClusterKube only
// feeds it to x509.CertPool.AppendCertsFromPEM, which needs a well-formed
// certificate and never dials anywhere with it in this test, so a self-signed
// one is exactly as good as a real cluster CA for this purpose.
func generateTestCA(t *testing.T) []byte {
	t.Helper()
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}
	tmpl := &x509.Certificate{
		SerialNumber: big.NewInt(1),
		Subject:      pkix.Name{CommonName: "test-ca"},
		NotBefore:    time.Now().Add(-time.Hour),
		NotAfter:     time.Now().Add(time.Hour),
		IsCA:         true,
		KeyUsage:     x509.KeyUsageCertSign,
	}
	der, err := x509.CreateCertificate(rand.Reader, tmpl, tmpl, &key.PublicKey, key)
	if err != nil {
		t.Fatal(err)
	}
	return pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der})
}

// writeSAMount builds a fake ServiceAccount mount directory, mirroring what
// the kubelet projects into a pod.
func writeSAMount(t *testing.T, caPEM []byte, token string) string {
	t.Helper()
	dir := t.TempDir()
	if caPEM != nil {
		if err := os.WriteFile(filepath.Join(dir, "ca.crt"), caPEM, 0o600); err != nil {
			t.Fatal(err)
		}
	}
	if token != "" {
		if err := os.WriteFile(filepath.Join(dir, "token"), []byte(token), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	return dir
}

func TestSADirDefaultsAndIsOverridableForTheConformanceSuite(t *testing.T) {
	t.Setenv("KUBERNETES_SERVICEACCOUNT_DIR", "")
	if got := saDir(); got != defaultSADir {
		t.Fatalf("default saDir: got %q want %q", got, defaultSADir)
	}
	t.Setenv("KUBERNETES_SERVICEACCOUNT_DIR", "/custom/sa/dir")
	if got := saDir(); got != "/custom/sa/dir" {
		t.Fatalf("overridden saDir: got %q", got)
	}
}

// Outside a cluster there is no injected host/port at all — the failure a
// misconfigured or locally-run binary actually hits.
func TestNewInClusterKubeRequiresHostAndPort(t *testing.T) {
	t.Setenv("KUBERNETES_SERVICE_HOST", "")
	t.Setenv("KUBERNETES_SERVICE_PORT", "")
	if _, err := NewInClusterKube(); err == nil {
		t.Fatal("expected an error when not running in a cluster")
	}
}

func TestNewInClusterKubeFailsWhenCAFileIsMissing(t *testing.T) {
	dir := writeSAMount(t, nil, "sa-token")
	t.Setenv("KUBERNETES_SERVICEACCOUNT_DIR", dir)
	t.Setenv("KUBERNETES_SERVICE_HOST", "10.0.0.1")
	t.Setenv("KUBERNETES_SERVICE_PORT", "443")
	if _, err := NewInClusterKube(); err == nil {
		t.Fatal("expected an error when ca.crt is absent")
	}
}

func TestNewInClusterKubeFailsWhenCAFileIsNotValidPEM(t *testing.T) {
	dir := writeSAMount(t, []byte("this is not a certificate"), "sa-token")
	t.Setenv("KUBERNETES_SERVICEACCOUNT_DIR", dir)
	t.Setenv("KUBERNETES_SERVICE_HOST", "10.0.0.1")
	t.Setenv("KUBERNETES_SERVICE_PORT", "443")
	if _, err := NewInClusterKube(); err == nil {
		t.Fatal("expected an error when ca.crt is not valid PEM")
	}
}

func TestNewInClusterKubeFailsWhenTokenCannotBeRead(t *testing.T) {
	dir := writeSAMount(t, generateTestCA(t), "") // no token file written
	t.Setenv("KUBERNETES_SERVICEACCOUNT_DIR", dir)
	t.Setenv("KUBERNETES_SERVICE_HOST", "10.0.0.1")
	t.Setenv("KUBERNETES_SERVICE_PORT", "443")
	if _, err := NewInClusterKube(); err == nil {
		t.Fatal("expected an error when the token cannot be read at construction")
	}
}

// The success path: a well-formed ServiceAccount mount produces a client
// pointed at the injected API server address, with a working bearer().
func TestNewInClusterKubeBuildsAClientFromTheServiceAccountMount(t *testing.T) {
	dir := writeSAMount(t, generateTestCA(t), "sa-token-v1\n")
	t.Setenv("KUBERNETES_SERVICEACCOUNT_DIR", dir)
	t.Setenv("KUBERNETES_SERVICE_HOST", "10.0.0.1")
	t.Setenv("KUBERNETES_SERVICE_PORT", "6443")

	k, err := NewInClusterKube()
	if err != nil {
		t.Fatal(err)
	}
	if k.BaseURL != "https://10.0.0.1:6443" {
		t.Fatalf("BaseURL: got %q", k.BaseURL)
	}
	if k.TokenPath != dir+"/token" {
		t.Fatalf("TokenPath: got %q", k.TokenPath)
	}
	tok, err := k.bearer()
	if err != nil || tok != "sa-token-v1" {
		t.Fatalf("bearer(): %q %v", tok, err)
	}
}

// A projected token can go briefly unreadable mid-rotation. A client that
// already has a cached copy must keep serving it rather than fail every
// in-flight call.
func TestBearerKeepsServingOnATransientReadFailure(t *testing.T) {
	tokenPath := filepath.Join(t.TempDir(), "token")
	if err := os.WriteFile(tokenPath, []byte("sa-token-v1\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	k := &Kube{TokenPath: tokenPath}
	first, err := k.bearer()
	if err != nil || first != "sa-token-v1" {
		t.Fatalf("initial token: %q %v", first, err)
	}
	k.mu.Lock()
	k.tokenLoaded = time.Now().Add(-2 * tokenMaxAge) // force a re-read
	k.mu.Unlock()
	if err := os.Remove(tokenPath); err != nil {
		t.Fatal(err)
	}
	again, err := k.bearer()
	if err != nil || again != "sa-token-v1" {
		t.Fatalf("a transient read failure must keep serving the cached token: %q %v", again, err)
	}
}

// The very first read failing, with nothing cached yet, must be a real error
// — there is no old token to fall back to.
func TestBearerFailsWhenNoTokenHasEverBeenCached(t *testing.T) {
	k := &Kube{TokenPath: filepath.Join(t.TempDir(), "never-written")}
	if _, err := k.bearer(); err == nil {
		t.Fatal("expected an error reading the ServiceAccount token for the first time")
	}
}

func TestAPIErrorFormatsCodeAndBody(t *testing.T) {
	err := &apiError{Code: 403, Body: "pods is forbidden"}
	if got := err.Error(); got != "kubernetes API 403: pods is forbidden" {
		t.Fatalf("Error(): %q", got)
	}
}

// get() must surface a bearer failure rather than sending an unauthenticated
// request.
func TestGetFailsWhenBearerFails(t *testing.T) {
	k := &Kube{BaseURL: "http://unused.invalid", HTTP: http.DefaultClient, TokenPath: filepath.Join(t.TempDir(), "missing")}
	if _, err := k.get(context.Background(), "/x", 0); err == nil {
		t.Fatal("expected the bearer error to propagate out of get()")
	}
}

func TestListIntoRejectsMalformedMetadata(t *testing.T) {
	k := testKube(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`not json at all`))
	}))
	if _, err := k.ListInto(context.Background(), "/api/v1/events", nil); err == nil ||
		!strings.Contains(err.Error(), "decoding list metadata") {
		t.Fatalf("expected a metadata decode error, got %v", err)
	}
}

// The metadata half of the body can decode fine (it only reads
// metadata.resourceVersion) while the destination-shaped half fails — a real
// possibility since ListInto is generic over the caller's out type.
func TestListIntoRejectsBodyThatDoesNotMatchTheDestinationShape(t *testing.T) {
	k := testKube(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"metadata":{"resourceVersion":"5"},"items":[{"count":"not-a-number"}]}`))
	}))
	if _, _, err := k.ListEvents(context.Background(), ""); err == nil ||
		!strings.Contains(err.Error(), "decoding list from") {
		t.Fatalf("expected a destination decode error, got %v", err)
	}
}

func TestWatchFramesReportsAMalformedFrame(t *testing.T) {
	k := testKube(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte("{not json\n"))
	}))
	err := k.WatchFrames(context.Background(), "/api/v1/events", "1", func(string, json.RawMessage) {})
	if err == nil || !strings.Contains(err.Error(), "decoding watch stream") {
		t.Fatalf("expected a stream decode error, got %v", err)
	}
}

// An ERROR frame that is not a 410/"Expired" must surface as a real failure —
// masquerading it as expiry would hide a lost RBAC grant behind a silent
// relist loop.
func TestWatchFramesNonExpiryErrorFrameIsARealFailure(t *testing.T) {
	k := testKube(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"type":"ERROR","object":{"code":500,"reason":"InternalError"}}` + "\n"))
	}))
	err := k.WatchFrames(context.Background(), "/api/v1/events", "1", func(string, json.RawMessage) {})
	if err == nil || errors.Is(err, ErrWatchExpired) || !strings.Contains(err.Error(), "watch error frame") {
		t.Fatalf("a non-expiry ERROR frame must be a real error, got %v", err)
	}
}

// A frame whose object cannot be parsed as an Event must be skipped, not
// dropped the whole stream for.
func TestWatchEventsSkipsAFrameItCannotParse(t *testing.T) {
	k := testKube(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		enc := json.NewEncoder(w)
		_ = enc.Encode(map[string]any{"type": "ADDED", "object": "not-an-event-object"})
		_ = enc.Encode(map[string]any{"type": "ADDED", "object": evt("Warning", "prod", "Pod", "ok", "BackOff")})
	}))
	var seen []string
	err := k.WatchEvents(context.Background(), "", "1", func(e Event) { seen = append(seen, e.InvolvedObject.Name) })
	if err != nil {
		t.Fatal(err)
	}
	if len(seen) != 1 || seen[0] != "ok" {
		t.Fatalf("an unparsable frame must be skipped rather than stop the stream: %v", seen)
	}
}
