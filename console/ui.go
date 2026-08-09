//go:build !dev

package main

import (
	"embed"
	"io/fs"
	"net/http"
)

// The SPA ships INSIDE the binary. `ui/dist` is produced by the Vite build in an
// earlier Docker stage (or by `make ui` locally) and embedded here, so the
// deployable artifact stays exactly what every other adapter is: one Go image
// serving one port, with nothing fetched at runtime and no CDN in the CSP.
//
// npm exists at BUILD time only, inside this module and its image. No other
// module and not the manager gains a build step — the dependency-free rule that
// governs adapter modules is narrowed here, not broken.
//
// `all:` is required: Vite emits `dist/assets/…` and the default embed pattern
// skips directories whose names begin with `_` or `.`, which hashed asset
// directories can produce.
//
//go:embed all:ui/dist
var uiFS embed.FS

// UIHandler serves the embedded assets, falling back to index.html so the SPA
// owns its own routes (deep links like /conversations/x must not 404).
func UIHandler() http.Handler {
	sub, err := fs.Sub(uiFS, "ui/dist")
	if err != nil {
		panic(err) // embedded at build time: unreachable unless the tree moved
	}
	return spaHandler(http.FS(sub), sub)
}
