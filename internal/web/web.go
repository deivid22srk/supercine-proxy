// Package web serves the static HTML dashboard at /.
package web

import (
	"embed"
	"io/fs"
	"net/http"
	"strings"
)

//go:embed templates/*
var templatesFS embed.FS

//go:embed static/*
var staticFS embed.FS

// Handler returns the http.Handler that serves the dashboard.
// Static assets are served at /static/, the SPA shell at /.
func Handler() http.Handler {
	mux := http.NewServeMux()

	// Static files (CSS/JS)
	staticSub, _ := fs.Sub(staticFS, "static")
	mux.Handle("/static/", http.StripPrefix("/static/", http.FileServer(http.FS(staticSub))))

	// Index page (dashboard SPA)
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/" && !strings.HasPrefix(r.URL.Path, "/static/") {
			// SPA fallback — let the client router handle it.
			r.URL.Path = "/"
		}
		if r.URL.Path == "/" {
			data, _ := templatesFS.ReadFile("templates/index.html")
			w.Header().Set("Content-Type", "text/html; charset=utf-8")
			_, _ = w.Write(data)
			return
		}
		http.NotFound(w, r)
	})

	return mux
}
