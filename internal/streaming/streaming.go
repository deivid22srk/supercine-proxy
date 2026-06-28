// Package streaming implements the /v1/catalog/* endpoints that back the
// streaming UI. It uses the enricher to combine IMDB search + provider
// lookup into a single metadata record per title.
//
// Endpoints:
//
//   GET /v1/catalog/popular?type=movies&limit=80
//       Curated list of ~80 popular IMDB IDs, enriched with metadata.
//
//   GET /v1/catalog/search?q=...&limit=12
//       Search IMDB by query, then resolve each result against the
//       provider registry.
//
//   GET /v1/catalog/resolve?imdb=tt...&type=movies
//       Resolve a single IMDB ID into full metadata.
//
//   GET /v1/catalog/movie/<imdb>?type=movies
//       Alias path-based for resolve.
//
//   GET /v1/providers
//       List all registered providers with their current health status.
//       Used by the UI to show which providers are active.
//
//   POST /v1/resolve
//       Resolve an IMDB ID into direct video URLs by trying each
//       provider in priority order. Body:
//         {"imdb":"tt...","type":"movies","provider":"supercine"}
//       The "provider" field is optional; if empty, the registry tries
//       all providers in priority order.
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
	"github.com/deivid22srk/supercine-proxy/internal/provider"
)

// Handler exposes catalog/search/resolve endpoints under /v1/.
type Handler struct {
	enricher *enricher.Enricher
	registry *provider.Registry
}

// New constructs a Handler.
func New(en *enricher.Enricher, reg *provider.Registry) *Handler {
	return &Handler{enricher: en, registry: reg}
}

// Register mounts the catalog endpoints on the given mux.
func (h *Handler) Register(mux *http.ServeMux) {
	mux.HandleFunc("/v1/catalog/popular", h.handlePopular)
	mux.HandleFunc("/v1/catalog/search", h.handleSearch)
	mux.HandleFunc("/v1/catalog/resolve", h.handleResolve)
	mux.HandleFunc("/v1/catalog/movie/", h.handleResolvePath)
	mux.HandleFunc("/v1/providers", h.handleProviders)
	mux.HandleFunc("/v1/resolve", h.handleResolveVideo)
}

// handlePopular returns the curated list of popular IMDB IDs, enriched.
//
//   GET /v1/catalog/popular?type=movies&limit=80
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

	// Use type-specific curated lists.
	var ids []string
	if embedType == "movies" {
		ids = imdb.PopularMovies()
	} else {
		ids = imdb.PopularTV()
	}
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

// handleSearch searches IMDB by query, then resolves each result.
//
//   GET /v1/catalog/search?q=homem+aranha&limit=12
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
//   GET /v1/catalog/resolve?imdb=tt2250912&type=movies
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

// handleProviders returns all registered providers with health status.
//
//   GET /v1/providers
func (h *Handler) handleProviders(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), 10*time.Second)
	defer cancel()
	infos := h.registry.Infos(ctx)
	writeJSON(w, http.StatusOK, map[string]any{
		"count":     len(infos),
		"providers": infos,
	})
}

// handleResolveVideo resolves an IMDB ID into direct video URLs by
// trying each provider in priority order.
//
//   POST /v1/resolve
//   body: {"imdb":"tt...","type":"movies","provider":"supercine"}
//
//   GET /v1/resolve?imdb=tt...&type=movies&provider=supercine
func (h *Handler) handleResolveVideo(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), 45*time.Second)
	defer cancel()

	var params struct {
		IMDB     string `json:"imdb"`
		Type     string `json:"type"`
		Provider string `json:"provider"`
	}
	switch r.Method {
	case http.MethodGet:
		params.IMDB = r.URL.Query().Get("imdb")
		params.Type = r.URL.Query().Get("type")
		params.Provider = r.URL.Query().Get("provider")
	case http.MethodPost:
		if err := json.NewDecoder(r.Body).Decode(&params); err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid JSON body"})
			return
		}
	default:
		writeJSON(w, http.StatusMethodNotAllowed, map[string]string{"error": "GET or POST required"})
		return
	}

	if params.IMDB == "" || !strings.HasPrefix(params.IMDB, "tt") {
		writeJSON(w, http.StatusBadRequest, map[string]string{
			"error": "imdb é obrigatório e deve começar com 'tt'",
		})
		return
	}
	if params.Type != "tvshows" {
		params.Type = "movies"
	}

	result, err := h.registry.Resolve(ctx, params.IMDB, params.Type, params.Provider)
	if err != nil {
		writeJSON(w, http.StatusBadGateway, map[string]any{
			"error":    err.Error(),
			"imdb":     params.IMDB,
			"type":     params.Type,
		})
		return
	}
	writeJSON(w, http.StatusOK, result)
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.Header().Set("Cache-Control", "public, max-age=300")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}
