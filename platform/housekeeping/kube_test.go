package main

import (
	"context"
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"fmt"
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

// selfSignedCAPEM generates a real, parseable self-signed certificate and
// returns it PEM-encoded — real crypto, not a fixture string, so
// x509.AppendCertsFromPEM does genuine work in the tests below.
func selfSignedCAPEM(t *testing.T) []byte {
	t.Helper()
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}
	tmpl := &x509.Certificate{
		SerialNumber:          big.NewInt(1),
		Subject:               pkix.Name{CommonName: "housekeeping-test-ca"},
		NotBefore:             time.Now().Add(-time.Hour),
		NotAfter:              time.Now().Add(time.Hour),
		IsCA:                  true,
		BasicConstraintsValid: true,
	}
	der, err := x509.CreateCertificate(rand.Reader, tmpl, tmpl, &key.PublicKey, key)
	if err != nil {
		t.Fatal(err)
	}
	return pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der})
}

// withSADir points the package-level saDir at a fresh temp directory for the
// duration of the test, restoring the real path afterward.
func withSADir(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	orig := saDir
	saDir = dir
	t.Cleanup(func() { saDir = orig })
	return dir
}

// NewInClusterKube must refuse to run outside a cluster — this is the branch
// that fires on every developer machine and in every CI job, so it is the
// one most worth getting right.
func TestNewInClusterKubeRequiresHostAndPort(t *testing.T) {
	t.Setenv("KUBERNETES_SERVICE_HOST", "")
	t.Setenv("KUBERNETES_SERVICE_PORT", "")
	if _, err := NewInClusterKube("agent-ops"); err == nil {
		t.Fatal("want an error when neither env var is set")
	}

	t.Setenv("KUBERNETES_SERVICE_HOST", "10.0.0.1")
	t.Setenv("KUBERNETES_SERVICE_PORT", "")
	if _, err := NewInClusterKube("agent-ops"); err == nil {
		t.Fatal("want an error when only the host is set")
	}
}

// The real failure mode of a missing ServiceAccount mount: os.ReadFile fails
// on a directory that does not exist, and that must surface as an error
// rather than a client silently missing its CA pool.
func TestNewInClusterKubeMissingCAFile(t *testing.T) {
	withSADir(t) // an empty temp dir: no ca.crt inside it
	t.Setenv("KUBERNETES_SERVICE_HOST", "10.0.0.1")
	t.Setenv("KUBERNETES_SERVICE_PORT", "443")

	_, err := NewInClusterKube("agent-ops")
	if err == nil || !strings.Contains(err.Error(), "reading API server CA") {
		t.Fatalf("want a CA-read error, got %v", err)
	}
}

// A ca.crt that is present but not valid PEM/DER must be rejected explicitly
// rather than building a client with an empty trust pool.
func TestNewInClusterKubeInvalidCAPEM(t *testing.T) {
	dir := withSADir(t)
	if err := os.WriteFile(filepath.Join(dir, "ca.crt"), []byte("not a certificate"), 0o644); err != nil {
		t.Fatal(err)
	}
	t.Setenv("KUBERNETES_SERVICE_HOST", "10.0.0.1")
	t.Setenv("KUBERNETES_SERVICE_PORT", "443")

	_, err := NewInClusterKube("agent-ops")
	if err == nil || !strings.Contains(err.Error(), "not valid PEM") {
		t.Fatalf("want a not-valid-PEM error, got %v", err)
	}
}

// The success path: a real self-signed cert on disk, a real host/port pair,
// and the returned client must be wired to use both plus the given namespace.
func TestNewInClusterKubeSuccess(t *testing.T) {
	dir := withSADir(t)
	if err := os.WriteFile(filepath.Join(dir, "ca.crt"), selfSignedCAPEM(t), 0o644); err != nil {
		t.Fatal(err)
	}
	t.Setenv("KUBERNETES_SERVICE_HOST", "10.11.12.13")
	t.Setenv("KUBERNETES_SERVICE_PORT", "6443")

	kube, err := NewInClusterKube("agent-ops")
	if err != nil {
		t.Fatalf("want a client, got error: %v", err)
	}
	if kube.BaseURL != "https://10.11.12.13:6443" {
		t.Errorf("BaseURL = %q, want the joined host:port over https", kube.BaseURL)
	}
	if kube.Namespace != "agent-ops" {
		t.Errorf("Namespace = %q", kube.Namespace)
	}
	if kube.TokenPath != filepath.Join(dir, "token") {
		t.Errorf("TokenPath = %q, want the SA dir's token file", kube.TokenPath)
	}
	if kube.HTTP == nil {
		t.Error("HTTP client must be set")
	}
}

// writeToken drops a fake bearer token where Kube.TokenPath expects to find
// one, exactly as the kubelet projects a real ServiceAccount token.
func writeToken(t *testing.T, content string) string {
	t.Helper()
	p := filepath.Join(t.TempDir(), "token")
	if err := os.WriteFile(p, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
	return p
}

// ListConversations against a real HTTP server: the request must carry the
// bearer token and the escaped namespace, and the response must be parsed
// including the dual-read fallback from the retired sessionId field.
func TestListConversationsParsesAndDualReads(t *testing.T) {
	var gotAuth, gotPath, gotAccept string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotAuth = r.Header.Get("Authorization")
		gotPath = r.URL.Path
		gotAccept = r.Header.Get("Accept")
		fmt.Fprint(w, `{"items":[
			{"metadata":{"name":"conv-new"},"status":{"runtimeContextId":"ctx-new"}},
			{"metadata":{"name":"conv-old"},"status":{"sessionId":"ctx-old"}},
			{"metadata":{"name":"conv-none"},"status":{}}
		]}`)
	}))
	defer srv.Close()

	kube := &Kube{
		BaseURL:   srv.URL,
		HTTP:      srv.Client(),
		TokenPath: writeToken(t, "sekrit-token\n"),
		Namespace: "team-ns",
	}

	convs, err := kube.ListConversations(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if gotAuth != "Bearer sekrit-token" {
		t.Errorf("Authorization = %q, want a trimmed bearer token", gotAuth)
	}
	if gotAccept != "application/json" {
		t.Errorf("Accept = %q", gotAccept)
	}
	wantPath := "/apis/agentops.dev/v1alpha1/namespaces/team-ns/conversations"
	if gotPath != wantPath {
		t.Errorf("path = %q, want %q", gotPath, wantPath)
	}

	want := map[string]string{"conv-new": "ctx-new", "conv-old": "ctx-old", "conv-none": ""}
	if len(convs) != len(want) {
		t.Fatalf("got %d conversations, want %d: %+v", len(convs), len(want), convs)
	}
	for _, c := range convs {
		if got, ok := want[c.Name]; !ok || got != c.ContextID {
			t.Errorf("conversation %+v: want ContextID %q", c, want[c.Name])
		}
	}
}

// A non-2xx response must surface as an error naming the status, not be
// silently decoded as an empty conversation list.
func TestListConversationsErrorStatus(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusForbidden)
		fmt.Fprint(w, "namespaces \"team-ns\" is forbidden")
	}))
	defer srv.Close()

	kube := &Kube{BaseURL: srv.URL, HTTP: srv.Client(), TokenPath: writeToken(t, "t"), Namespace: "team-ns"}
	_, err := kube.ListConversations(context.Background())
	if err == nil || !strings.Contains(err.Error(), "403") {
		t.Fatalf("want an error naming the status, got %v", err)
	}
	if !strings.Contains(err.Error(), "forbidden") {
		t.Fatalf("want the body's reason surfaced, got %v", err)
	}
}

// Malformed JSON must be reported as a decode error rather than an empty,
// misleadingly successful list.
func TestListConversationsDecodeError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, "{not json")
	}))
	defer srv.Close()

	kube := &Kube{BaseURL: srv.URL, HTTP: srv.Client(), TokenPath: writeToken(t, "t"), Namespace: "ns"}
	if _, err := kube.ListConversations(context.Background()); err == nil {
		t.Fatal("want a decode error for malformed JSON")
	}
}

// A missing token file must fail before any request is made — the manager
// invariant that reads no Secrets applies here too: no token, no call.
func TestListConversationsMissingToken(t *testing.T) {
	called := false
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		called = true
		io.WriteString(w, `{"items":[]}`)
	}))
	defer srv.Close()

	kube := &Kube{
		BaseURL:   srv.URL,
		HTTP:      srv.Client(),
		TokenPath: filepath.Join(t.TempDir(), "does-not-exist"),
		Namespace: "ns",
	}
	if _, err := kube.ListConversations(context.Background()); err == nil {
		t.Fatal("want an error reading the missing token")
	}
	if called {
		t.Fatal("must not call the API server without a token")
	}
}
