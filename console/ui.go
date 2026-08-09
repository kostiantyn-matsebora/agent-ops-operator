package main

import (
	"embed"
	"io/fs"
	"net/http"
)

// The SPA ships inside the binary: hand-written HTML/JS/CSS, no build step, no
// npm, nothing to fetch at runtime. That keeps the image distroless-friendly
// and this module dependency-free, and it is enough — the graph is tens of
// nodes of hand-rolled SVG, not a visualization library's problem.

//go:embed ui
var uiFS embed.FS

// UIHandler serves the embedded assets, falling back to index.html so the SPA
// owns its own routes.
func UIHandler() http.Handler {
	sub, err := fs.Sub(uiFS, "ui")
	if err != nil {
		panic(err) // embedded at build time: unreachable unless the tree moved
	}
	files := http.FileServer(http.FS(sub))
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if _, err := fs.Stat(sub, trimLeadingSlash(r.URL.Path)); err != nil {
			r = r.Clone(r.Context())
			r.URL.Path = "/"
		}
		files.ServeHTTP(w, r)
	})
}

func trimLeadingSlash(p string) string {
	if p == "" || p == "/" {
		return "index.html"
	}
	return p[1:]
}
