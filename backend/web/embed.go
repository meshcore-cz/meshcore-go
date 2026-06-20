package web

import (
	"embed"
	"io/fs"
	"net/http"
	"path"
	"strings"
)

// frontendFS holds the built SvelteKit dashboard. The build output is committed
// so `go build` works without a Node toolchain; run `make web` to rebuild it
// after changing the frontend sources under frontend/.
//
//go:embed all:frontend/build
var frontendFS embed.FS

// staticHandler serves the embedded SPA, falling back to index.html for any
// path that doesn't map to a real asset (client-side routing).
func staticHandler() http.Handler {
	sub, err := fs.Sub(frontendFS, "frontend/build")
	if err != nil {
		panic(err)
	}
	fileServer := http.FileServer(http.FS(sub))
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		clean := strings.TrimPrefix(path.Clean(r.URL.Path), "/")
		if clean == "" {
			clean = "index.html"
		}
		if _, err := fs.Stat(sub, clean); err != nil {
			// Unknown path: hand the SPA shell to the client router.
			r = r.Clone(r.Context())
			r.URL.Path = "/"
		}
		fileServer.ServeHTTP(w, r)
	})
}
