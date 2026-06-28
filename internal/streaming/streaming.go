// Package streaming implements the /v1/catalog/* endpoints that back the
// streaming UI. It uses the enricher to combine IMDB search + Supercine
// embed lookup into a single metadata record per title.
package streaming

import (
	"context"
	"encoding/json"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/deivid22srk/supercine-proxy/internal/enricher"
	"github.com/deivid22srk/supercine-proxy/internal/imdb"
)

// Handler exposes catalog/search endpoints under /v1/catalog/*.
type Handler struct {
	enricher *enricher.Enricher
}

// New constructs a Handler.
func New(en *enricher.Enricher) *Handler {
	return &Handler{enricher: en}
}

// Register mounts the catalog endpoints on the given mux at /v1/catalog/.
func (h *Handler) Register(mux *http.ServeMux) {
	mux.HandleFunc("/v1/catalog/popular", h.handlePopular)
	mux.HandleFunc("/v1/catalog/search", h.handleSearch)
	mux.HandleFunc("/v1/catalog/resolve", h.handleResolve)
	mux.HandleFunc("/v1/catalog/movie/", h.handleResolvePath)
}

// handlePopular returns the curated list of ~80 popular IMDB IDs, each
// enriched with title (PT-BR + original), poster, backdrop, year, and
// availability status. This is what the home screen displays.
//
// GET /v1/catalog/popular?type=movies&limit=80
//
// Query params:
//
//	type  - "movies" (default) or "tvshows"
//	limit - max entries to return (default 80, max 200)
func (h *Handler) handlePopular(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), 90*time.Second)
	defer cancel()

	embedType := strings.ToLower(r.URL.Query().Get("type"))
	if embedType != "tvshows" {
		embedType = "movies"
	}
	limit := 80
	if l := r.URL.Query().Get("limit"); l != "" {
		if n, err := strconv.Atoi(l); err == nil && n > 0 && n <= 200 {
			limit = n
		}
	}

	ids := imdb.DedupPopular()
	if len(ids) > limit {
		ids = ids[:limit]
	}

	metas := h.enricher.ResolveMany(ctx, ids, embedType)
	writeJSON(w, http.StatusOK, map[string]any{
		"type":  embedType,
		"count": len(metas),
		"items": metas,
	})
}

// handleSearch searches IMDB by query, then resolves each result against
// Supercine to get PT-BR title, backdrop, and availability.
//
// GET /v1/catalog/search?q=homem+aranha&limit=12
//
// Query params:
//
//	q     - search query (required)
//	limit - max entries (default 12, max 30)
func (h *Handler) handleSearch(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), 60*time.Second)
	defer cancel()

	q := strings.TrimSpace(r.URL.Query().Get("q"))
	if q == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{
			"error": "parâmetro 'q' é obrigatório",
		})
		return
	}
	limit := 12
	if l := r.URL.Query().Get("limit"); l != "" {
		if n, err := strconv.Atoi(l); err == nil && n > 0 && n <= 30 {
			limit = n
		}
	}

	metas, err := h.enricher.SearchAndResolve(ctx, q, limit)
	if err != nil {
		writeJSON(w, http.StatusBadGateway, map[string]string{
			"error": err.Error(),
		})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"query": q,
		"count": len(metas),
		"items": metas,
	})
}

// handleResolve resolves a single IMDB ID into a full metadata record.
//
// GET /v1/catalog/resolve?imdb=tt2250912&type=movies
//
// Query params:
//
//	imdb  - IMDB ID (required)
//	type  - "movies" (default) or "tvshows"
func (h *Handler) handleResolve(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), 30*time.Second)
	defer cancel()

	imdbID := strings.TrimSpace(r.URL.Query().Get("imdb"))
	if imdbID == "" || !strings.HasPrefix(imdbID, "tt") {
		writeJSON(w, http.StatusBadRequest, map[string]string{
			"error": "parâmetro 'imdb' é obrigatório e deve começar com 'tt'",
		})
		return
	}
	embedType := strings.ToLower(r.URL.Query().Get("type"))
	if embedType != "tvshows" {
		embedType = "movies"
	}

	meta, err := h.enricher.Resolve(ctx, imdbID, embedType)
	if err != nil {
		writeJSON(w, http.StatusBadGateway, map[string]string{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, meta)
}

// handleResolvePath handles /v1/catalog/movie/<imdb>?type=movies
// It's an alias for /v1/catalog/resolve that puts the IMDB ID in the path.
func (h *Handler) handleResolvePath(w http.ResponseWriter, r *http.Request) {
	imdbID := strings.TrimPrefix(r.URL.Path, "/v1/catalog/movie/")
	if imdbID == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{
			"error": "IMDB ID faltando no path",
		})
		return
	}
	q := r.URL.Query()
	q.Set("imdb", imdbID)
	r.URL.RawQuery = q.Encode()
	r.URL.Path = "/v1/catalog/resolve"
	h.handleResolve(w, r)
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.Header().Set("Cache-Control", "public, max-age=300")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}
