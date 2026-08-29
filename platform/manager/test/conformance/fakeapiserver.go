//go:build conformance

package conformance

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/json"
	"encoding/pem"
	"fmt"
	"math/big"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"
)

// FakeAPIServer is the slice of the Kubernetes API the console and
// signal-k8s-events read: LIST and WATCH on arbitrary resource paths, over
// TLS with a CA the adapter is handed through a fake ServiceAccount mount.
// Objects are seeded per resource path; Push broadcasts a watch event.
type FakeAPIServer struct {
	srv   *httptest.Server
	SADir string // ca.crt + token, for KUBERNETES_SERVICEACCOUNT_DIR
	Host  string
	Port  string

	mu       sync.Mutex
	objects  map[string][]map[string]any // resource path (no ?query) → items
	watchers map[string][]chan []byte
	rv       int
	requests []string
}

// NewFakeAPIServer starts the fake and writes the ServiceAccount files.
func NewFakeAPIServer(t *testing.T) *FakeAPIServer {
	t.Helper()
	f := &FakeAPIServer{objects: map[string][]map[string]any{}, watchers: map[string][]chan []byte{}, rv: 1}
	mux := http.NewServeMux()
	mux.HandleFunc("/", f.handle)
	f.srv = httptest.NewUnstartedServer(mux)
	f.srv.TLS = &tls.Config{Certificates: []tls.Certificate{selfSigned(t)}}
	f.srv.StartTLS()
	t.Cleanup(f.srv.Close)

	host, port, _ := net.SplitHostPort(strings.TrimPrefix(f.srv.URL, "https://"))
	f.Host, f.Port = host, port
	f.SADir = t.TempDir()
	cert := f.srv.TLS.Certificates[0].Certificate[0]
	caPEM := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: cert})
	if err := os.WriteFile(filepath.Join(f.SADir, "ca.crt"), caPEM, 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(f.SADir, "token"), []byte("fake-sa-token\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(f.SADir, "namespace"), []byte("agent-ops"), 0o644); err != nil {
		t.Fatal(err)
	}
	return f
}

// Env is what an in-cluster client reads to find the API server.
func (f *FakeAPIServer) Env() []string {
	return []string{
		"KUBERNETES_SERVICE_HOST=" + f.Host,
		"KUBERNETES_SERVICE_PORT=" + f.Port,
		"KUBERNETES_SERVICEACCOUNT_DIR=" + f.SADir,
	}
}

// Seed sets the items a LIST of path returns (path without query, e.g.
// /apis/agentops.dev/v1alpha1/namespaces/agent-ops/conversations).
func (f *FakeAPIServer) Seed(path string, items ...map[string]any) {
	f.mu.Lock()
	defer f.mu.Unlock()
	for _, it := range items {
		f.stamp(it)
	}
	f.objects[path] = append(f.objects[path], items...)
}

// Push broadcasts a watch event (ADDED | MODIFIED | DELETED) for path and
// updates the listed items so a re-list sees it too.
func (f *FakeAPIServer) Push(path, eventType string, obj map[string]any) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.stamp(obj)
	name := objName(obj)
	items := f.objects[path]
	kept := items[:0]
	for _, it := range items {
		if objName(it) != name {
			kept = append(kept, it)
		}
	}
	if eventType != "DELETED" {
		kept = append(kept, obj)
	}
	f.objects[path] = kept
	ev, _ := json.Marshal(map[string]any{"type": eventType, "object": obj})
	for _, ch := range f.watchers[path] {
		select {
		case ch <- ev:
		default:
		}
	}
}

// Requests lists the paths the adapter asked for.
func (f *FakeAPIServer) Requests() []string {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]string{}, f.requests...)
}

func (f *FakeAPIServer) stamp(obj map[string]any) {
	f.rv++
	meta, _ := obj["metadata"].(map[string]any)
	if meta == nil {
		meta = map[string]any{}
		obj["metadata"] = meta
	}
	meta["resourceVersion"] = fmt.Sprint(f.rv)
	if _, ok := meta["uid"]; !ok {
		meta["uid"] = "uid-" + objName(obj)
	}
}

func objName(obj map[string]any) string {
	meta, _ := obj["metadata"].(map[string]any)
	n, _ := meta["name"].(string)
	return n
}

func (f *FakeAPIServer) handle(w http.ResponseWriter, r *http.Request) {
	if !strings.HasPrefix(r.Header.Get("Authorization"), "Bearer ") {
		http.Error(w, "unauthorized", 401)
		return
	}
	path := r.URL.Path
	f.mu.Lock()
	f.requests = append(f.requests, r.URL.RequestURI())
	f.mu.Unlock()
	q := r.URL.Query()
	if q.Get("watch") == "1" || q.Get("watch") == "true" {
		f.watch(w, r, path)
		return
	}
	f.mu.Lock()
	items := append([]map[string]any{}, f.objects[path]...)
	rv := f.rv
	f.mu.Unlock()
	if items == nil {
		items = []map[string]any{}
	}
	writeJSON(w, 200, map[string]any{
		"apiVersion": "v1", "kind": "List",
		"metadata": map[string]any{"resourceVersion": fmt.Sprint(rv)},
		"items":    items,
	})
}

func (f *FakeAPIServer) watch(w http.ResponseWriter, r *http.Request, path string) {
	ch := make(chan []byte, 64)
	f.mu.Lock()
	f.watchers[path] = append(f.watchers[path], ch)
	f.mu.Unlock()
	defer func() {
		f.mu.Lock()
		ws := f.watchers[path]
		for i, c := range ws {
			if c == ch {
				f.watchers[path] = append(ws[:i], ws[i+1:]...)
				break
			}
		}
		f.mu.Unlock()
	}()
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(200)
	flusher, _ := w.(http.Flusher)
	if flusher != nil {
		flusher.Flush()
	}
	timeout := time.After(300 * time.Second)
	for {
		select {
		case <-r.Context().Done():
			return
		case <-timeout:
			return
		case ev := <-ch:
			if _, err := w.Write(append(ev, '\n')); err != nil {
				return
			}
			if flusher != nil {
				flusher.Flush()
			}
		}
	}
}

// selfSigned mints a certificate for 127.0.0.1 that is its own CA.
func selfSigned(t *testing.T) tls.Certificate {
	t.Helper()
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	tmpl := &x509.Certificate{
		SerialNumber:          big.NewInt(1),
		Subject:               pkix.Name{CommonName: "fake-apiserver"},
		NotBefore:             time.Now().Add(-time.Hour),
		NotAfter:              time.Now().Add(24 * time.Hour),
		KeyUsage:              x509.KeyUsageDigitalSignature | x509.KeyUsageKeyEncipherment | x509.KeyUsageCertSign,
		ExtKeyUsage:           []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
		BasicConstraintsValid: true,
		IsCA:                  true,
		IPAddresses:           []net.IP{net.ParseIP("127.0.0.1")},
		DNSNames:              []string{"localhost"},
	}
	der, err := x509.CreateCertificate(rand.Reader, tmpl, tmpl, &key.PublicKey, key)
	if err != nil {
		t.Fatal(err)
	}
	keyDER, _ := x509.MarshalECPrivateKey(key)
	cert, err := tls.X509KeyPair(
		pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der}),
		pem.EncodeToMemory(&pem.Block{Type: "EC PRIVATE KEY", Bytes: keyDER}))
	if err != nil {
		t.Fatal(err)
	}
	return cert
}
