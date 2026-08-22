package main

import (
	"crypto/sha256"
	"encoding/hex"
	"io/fs"
	"net/http"
	"path"
	"strings"
)

// SPA serving, shared by the embedded and `dev` builds so the two cannot drift
// in how they route.

// PRE-COMPRESSED, NOT COMPRESSED PER REQUEST.
//
// The bundle is ~2.6MB of CSS and JS, and served raw that is a two-second first
// paint on a good connection. Gzipped it is around 400KB.
//
// The compression happens at BUILD time, next to the asset it belongs to: the
// files never change after the image is built, so paying for it on every
// request would be spending CPU on a fixed answer. It also buys the best
// setting — a build can afford maximum compression where a request handler
// cannot — and it keeps the runtime a plain file server with one extra lookup.
//
// A client that does not offer gzip still gets the original. Nothing here is
// conditional on the asset existing in both forms.

// compressible is served pre-compressed when a `.gz` twin exists. Images and
// fonts are already compressed; gzipping them costs size rather than saving it.
func compressible(p string) bool {
	switch path.Ext(p) {
	case ".js", ".css", ".html", ".json", ".svg", ".map":
		return true
	}
	return false
}

// NOTHING WAS CACHED, EITHER.
//
// go:embed gives every file a ZERO modtime, so the file server can emit no
// Last-Modified, and it computes no ETag — leaving the browser nothing to
// revalidate against and no Cache-Control to go on. Every reload refetched the
// whole bundle.
//
// The build already solves the hard half: Vite writes CONTENT-HASHED names, so
// `index-CYc0IAx2.js` can never change meaning. Those are immutable and cached
// for a year. `index.html` is the one file with a stable name, so it is never
// cached — it is the thing that names which hashed assets to load, and a stale
// copy pins the whole app to an old build.
const (
	// immutableCache is for content-hashed assets: the name changes when the
	// bytes do, so the old answer is never wrong.
	immutableCache = "public, max-age=31536000, immutable"
	// revalidateCache is for anything whose name is stable. `no-cache` does not
	// mean "do not store" — it means "ask first", which with an ETag is a 304
	// and no body.
	revalidateCache = "no-cache"
)

// cacheControl decides how long a browser may keep an asset.
//
// Keyed on the BUILD's own convention rather than on a list of names: Vite puts
// every hashed artifact under assets/, and nothing else goes there.
func cacheControl(name string) string {
	if strings.HasPrefix(name, "assets/") {
		return immutableCache
	}
	return revalidateCache
}

// spaHandler serves static assets and falls back to index.html for anything
// that is not a file, so the SPA owns its own routes — a deep link like
// /conversations/x must render the app, not 404.
func spaHandler(fsys http.FileSystem, stat fs.FS) http.Handler {
	files := http.FileServer(fsys)
	buildETags(stat)
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		name := trimLeadingSlash(r.URL.Path)
		if _, err := fs.Stat(stat, name); err != nil {
			r = r.Clone(r.Context())
			r.URL.Path = "/"
			name = "index.html"
		}

		w.Header().Set("Cache-Control", cacheControl(name))
		// An ETag is what turns `no-cache` into a 304 rather than a refetch.
		// Derived from the CONTENT, because an embedded file has no modtime to
		// derive anything from.
		if tag := etags[name]; tag != "" {
			w.Header().Set("ETag", tag)
			if match := r.Header.Get("If-None-Match"); match == tag {
				w.WriteHeader(http.StatusNotModified)
				return
			}
		}

		if compressible(name) && acceptsGzip(r) {
			if _, err := fs.Stat(stat, name+".gz"); err == nil {
				// Content-Type comes from the ORIGINAL name: the file server
				// would otherwise sniff the gzip stream and label it
				// application/x-gzip, which a browser downloads rather than
				// runs.
				w.Header().Set("Content-Type", contentType(name))
				w.Header().Set("Content-Encoding", "gzip")
				// The response varies by request header, so a shared cache must
				// key on it or it will hand a gzip body to a client that asked
				// for none.
				w.Header().Add("Vary", "Accept-Encoding")
				r = r.Clone(r.Context())
				r.URL.Path = "/" + name + ".gz"
				files.ServeHTTP(w, r)
				return
			}
		}
		files.ServeHTTP(w, r)
	})
}

// acceptsGzip reports whether the client offered gzip. Deliberately simple:
// anything that names gzip at all gets it, and a q=0 refusal is rare enough
// that the cost of being wrong is one uncompressed-looking response.
func acceptsGzip(r *http.Request) bool {
	return strings.Contains(r.Header.Get("Accept-Encoding"), "gzip")
}

// contentType maps an asset's extension to its type, because the compressed
// twin cannot be sniffed.
func contentType(name string) string {
	switch path.Ext(name) {
	case ".js":
		return "text/javascript; charset=utf-8"
	case ".css":
		return "text/css; charset=utf-8"
	case ".html":
		return "text/html; charset=utf-8"
	case ".json", ".map":
		return "application/json; charset=utf-8"
	case ".svg":
		return "image/svg+xml"
	}
	return "application/octet-stream"
}

// etags maps an asset path to its content ETag, computed once at startup.
//
// Only for the files whose NAME is stable — a hashed asset revalidates never,
// so an ETag for it would be answering a question nobody asks.
var etags = map[string]string{}

// buildETags hashes the stably-named files so they can answer If-None-Match.
func buildETags(stat fs.FS) {
	_ = fs.WalkDir(stat, ".", func(p string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() || strings.HasPrefix(p, "assets/") || strings.HasSuffix(p, ".gz") {
			return nil
		}
		b, err := fs.ReadFile(stat, p)
		if err != nil {
			return nil
		}
		sum := sha256.Sum256(b)
		etags[p] = `"` + hex.EncodeToString(sum[:8]) + `"`
		return nil
	})
}

func trimLeadingSlash(p string) string {
	if p == "" || p == "/" {
		return "index.html"
	}
	return p[1:]
}
