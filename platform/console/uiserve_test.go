package main

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"testing/fstest"
)

// uiserve.go had NO test file at all — every function in it, including the
// handler the SPA is served through, was 0% covered. These tests build a
// small in-memory filesystem (testing/fstest.MapFS, a real fs.FS — no mock of
// the file-serving behavior under test) shaped like a Vite build: a stable
// index.html, a content-hashed asset with a pre-built .gz twin, and an
// uncompressed image.

func fakeDist() fstest.MapFS {
	return fstest.MapFS{
		"index.html":           {Data: []byte("<html>app shell</html>")},
		"assets/app-abc123.js": {Data: []byte("console.log('app')")},
		// the pre-compressed twin the build produces next to the real asset
		"assets/app-abc123.js.gz": {Data: []byte("\x1f\x8b-fake-gzip-bytes")},
		"favicon.svg":             {Data: []byte("<svg></svg>")},
	}
}

func TestCompressibleExtensions(t *testing.T) {
	for _, p := range []string{"app.js", "app.css", "index.html", "data.json", "icon.svg", "app.js.map"} {
		if !compressible(p) {
			t.Fatalf("%s must be considered compressible", p)
		}
	}
	for _, p := range []string{"logo.png", "font.woff2"} {
		if compressible(p) {
			t.Fatalf("%s must not be pre-gzipped (already compressed)", p)
		}
	}
}

func TestContentTypeByExtension(t *testing.T) {
	cases := map[string]string{
		"a.js": "text/javascript; charset=utf-8", "a.css": "text/css; charset=utf-8",
		"a.html": "text/html; charset=utf-8", "a.json": "application/json; charset=utf-8",
		"a.map": "application/json; charset=utf-8", "a.svg": "image/svg+xml",
		"a.bin": "application/octet-stream",
	}
	for name, want := range cases {
		if got := contentType(name); got != want {
			t.Fatalf("%s: got %q, want %q", name, got, want)
		}
	}
}

func TestCacheControlByPathConvention(t *testing.T) {
	if cacheControl("assets/app-abc123.js") != immutableCache {
		t.Fatal("a hashed asset under assets/ must be cached forever")
	}
	if cacheControl("index.html") != revalidateCache {
		t.Fatal("a stably-named file must revalidate on every load")
	}
}

func TestTrimLeadingSlash(t *testing.T) {
	if trimLeadingSlash("") != "index.html" || trimLeadingSlash("/") != "index.html" {
		t.Fatal("the root must map to index.html")
	}
	if trimLeadingSlash("/assets/app.js") != "assets/app.js" {
		t.Fatalf("got %q", trimLeadingSlash("/assets/app.js"))
	}
}

func TestAcceptsGzip(t *testing.T) {
	r := httptest.NewRequest("GET", "/", nil)
	if acceptsGzip(r) {
		t.Fatal("no Accept-Encoding must not accept gzip")
	}
	r.Header.Set("Accept-Encoding", "gzip, deflate, br")
	if !acceptsGzip(r) {
		t.Fatal("a client naming gzip must be served it")
	}
}

// A deep link the SPA owns client-side (not a real file) must fall back to
// index.html rather than 404 — this is the whole point of the handler.
func TestSPAHandlerFallsBackToIndexForUnknownPaths(t *testing.T) {
	dist := fakeDist()
	h := spaHandler(http.FS(dist), dist)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest("GET", "/conversations/some-name", nil))
	if rec.Code != http.StatusOK || rec.Body.String() != "<html>app shell</html>" {
		t.Fatalf("deep link not served the app shell: %d %q", rec.Code, rec.Body.String())
	}
	if rec.Header().Get("Cache-Control") != revalidateCache {
		t.Fatalf("index.html must never be cached long: %q", rec.Header().Get("Cache-Control"))
	}
}

// A real, stably-named file gets an ETag, and a matching If-None-Match must
// short-circuit to 304 with no body refetched.
func TestSPAHandlerETagRevalidation(t *testing.T) {
	dist := fakeDist()
	h := spaHandler(http.FS(dist), dist)

	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest("GET", "/favicon.svg", nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("want 200, got %d", rec.Code)
	}
	tag := rec.Header().Get("ETag")
	if tag == "" {
		t.Fatal("a stably-named file must carry an ETag")
	}

	rec = httptest.NewRecorder()
	req := httptest.NewRequest("GET", "/favicon.svg", nil)
	req.Header.Set("If-None-Match", tag)
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusNotModified {
		t.Fatalf("a matching ETag must 304, got %d", rec.Code)
	}
}

// A client that accepts gzip and a file with a pre-built .gz twin gets the
// compressed bytes under the ORIGINAL content type, with Vary set so a shared
// cache does not hand it to a client that never asked for gzip.
func TestSPAHandlerServesPrecompressedAsset(t *testing.T) {
	dist := fakeDist()
	h := spaHandler(http.FS(dist), dist)

	req := httptest.NewRequest("GET", "/assets/app-abc123.js", nil)
	req.Header.Set("Accept-Encoding", "gzip")
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("want 200, got %d", rec.Code)
	}
	if rec.Header().Get("Content-Encoding") != "gzip" {
		t.Fatal("must serve the pre-compressed twin")
	}
	if rec.Header().Get("Content-Type") != "text/javascript; charset=utf-8" {
		t.Fatalf("content-type must come from the ORIGINAL name: %q", rec.Header().Get("Content-Type"))
	}
	if rec.Header().Get("Vary") != "Accept-Encoding" {
		t.Fatal("a response that varies by header must say so")
	}
	if rec.Header().Get("Cache-Control") != immutableCache {
		t.Fatal("a hashed asset under assets/ must still be cached forever, gzipped or not")
	}
	if rec.Body.String() != "\x1f\x8b-fake-gzip-bytes" {
		t.Fatalf("wrong body served: %q", rec.Body.String())
	}
}

// A client that does NOT accept gzip still gets the original bytes, even
// though a .gz twin exists.
func TestSPAHandlerServesOriginalWithoutGzipSupport(t *testing.T) {
	dist := fakeDist()
	h := spaHandler(http.FS(dist), dist)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest("GET", "/assets/app-abc123.js", nil))
	if rec.Header().Get("Content-Encoding") == "gzip" {
		t.Fatal("a client offering no gzip must not receive it")
	}
	if rec.Body.String() != "console.log('app')" {
		t.Fatalf("wrong body: %q", rec.Body.String())
	}
}

// The production build tag's UIHandler must serve the real embedded dist —
// this is the top-level entry point main() wires the whole server through,
// and it was entirely untested. Verified by asking for the SPA's own root,
// which every real deployment does on first load.
func TestUIHandlerServesEmbeddedIndex(t *testing.T) {
	rec := httptest.NewRecorder()
	UIHandler().ServeHTTP(rec, httptest.NewRequest("GET", "/", nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("want 200, got %d", rec.Code)
	}
	if rec.Body.Len() == 0 {
		t.Fatal("the embedded index.html must not be empty")
	}
}
