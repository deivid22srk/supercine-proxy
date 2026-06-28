// Package web serves the embedded HTML/CSS/JS for two distinct UIs:
//
//   - /         — the streaming UI (Netflix-style home with search + player)
//   - /admin     — the proxy admin dashboard (logs, stats, API explorer)
//   - /static/* — shared static assets (style.css, app.js, streaming.css, streaming.js)
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

	// Static files (CSS/JS) — shared between admin and streaming UIs.
	staticSub, _ := fs.Sub(staticFS, "static")
	mux.Handle("/static/", http.StripPrefix("/static/", http.FileServer(http.FS(staticSub))))

	// Admin dashboard at /admin (and /admin/*).
	mux.HandleFunc("/admin", func(w http.ResponseWriter, r *http.Request) {
		data, _ := templatesFS.ReadFile("templates/index.html")
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		_, _ = w.Write(data)
	})
	mux.HandleFunc("/admin/", func(w http.ResponseWriter, r *http.Request) {
		// SPA fallback for admin sub-paths
		data, _ := templatesFS.ReadFile("templates/index.html")
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		_, _ = w.Write(data)
	})

	// Streaming UI at / (default route).
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		if strings.HasPrefix(r.URL.Path, "/static/") || strings.HasPrefix(r.URL.Path, "/v1/") ||
			strings.HasPrefix(r.URL.Path, "/wp-json/") || strings.HasPrefix(r.URL.Path, "/embed-api/") {
			// These should have been handled by other handlers; if we land here,
			// return 404.
			http.NotFound(w, r)
			return
		}
		// SPA fallback — serve the streaming index for any other path.
		data, _ := templatesFS.ReadFile("templates/streaming.html")
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		_, _ = w.Write(data)
	})

	return mux
}
