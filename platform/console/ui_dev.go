//go:build dev

package main

import (
	"log"
	"net/http"
	"os"
)

// `go build -tags dev` serves the SPA from disk instead of embedding it, so a
// local console can be run against `npm run dev` output (or Vite's own dev
// server, proxying /api here) without a Go rebuild per UI edit.
//
// This exists so the embedded build is never the only way to work: a UI change
// that required a full image build to see would make the frontend half of this
// module miserable to develop, and misery is how a dev tag stops being used and
// starts rotting.
func UIHandler() http.Handler {
	dir := os.Getenv("UI_DIR")
	if dir == "" {
		dir = "ui/dist"
	}
	log.Printf("dev build: serving the SPA from %s (not embedded)", dir)
	return spaHandler(http.Dir(dir), os.DirFS(dir))
}
