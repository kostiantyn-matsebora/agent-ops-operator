package main

import (
	"io/fs"
	"net/http"
)

// SPA serving, shared by the embedded and `dev` builds so the two cannot drift
// in how they route.

// spaHandler serves static assets and falls back to index.html for anything
// that is not a file, so the SPA owns its own routes — a deep link like
// /conversations/x must render the app, not 404.
func spaHandler(fsys http.FileSystem, stat fs.FS) http.Handler {
	files := http.FileServer(fsys)
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if _, err := fs.Stat(stat, trimLeadingSlash(r.URL.Path)); err != nil {
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
