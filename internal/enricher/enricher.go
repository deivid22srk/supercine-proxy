// Package enricher resolves an IMDB ID into a rich metadata record by
// combining two free sources:
//
//   1. A provider.Provider (currently only the Supercine provider) —
//      which returns the PT-BR title, TMDB backdrop URL, and the list
//      of available streaming servers for the IMDB ID.
//   2. The IMDB suggestion endpoint (which returns the original English
//      title, year, and a poster URL).
//
// The result is a single "MovieMeta" record suitable for display in
// the streaming UI.
//
// Today only the Supercine provider is registered. When new providers
// are added, the enricher keeps working unchanged — it just calls
// provider.Registry.Resolve() and the registry tries each provider
// in priority order.
package enricher

import (
        "context"
        "fmt"
        "strings"
        "sync"
        "time"

        "github.com/deivid22srk/supercine-proxy/internal/imdb"
        "github.com/deivid22srk/supercine-proxy/internal/provider"
        "github.com/deivid22srk/supercine-proxy/internal/provider/supercine"
)

// MovieMeta is the combined metadata for a title shown in the UI.
type MovieMeta struct {
        IMDB        string `json:"imdb"`
        Type        string `json:"type"`         // "movie" | "tv"
        EmbedType   string `json:"embed_type"`   // "movies" | "tvshows"
        TitlePTBR   string `json:"title_ptbr"`   // from Supercine
        TitleOrig   string `json:"title_orig"`   // from IMDB
        Year        int    `json:"year"`         // from IMDB
        PosterURL   string `json:"poster_url"`   // from IMDB
        BackdropURL string `json:"backdrop_url"` // from Supercine (TMDB)
        Cast        string `json:"cast"`         // from IMDB
        Rank        int    `json:"rank"`         // from IMDB
        Available   bool   `json:"available"`    // false if no provider has it
        ServerCount int    `json:"server_count"` // # of servers available
        Provider    string `json:"provider"`     // which provider served it ("" if none)
}

// Enricher combines IMDB search + provider resolution into a MovieMeta.
type Enricher struct {
        imdbClient *imdb.Client
        registry   *provider.Registry
}

// New constructs an Enricher.
func New(registry *provider.Registry) *Enricher {
        return &Enricher{
                imdbClient: imdb.New(),
                registry:   registry,
        }
}

// IMDBClient returns the underlying IMDB client (for direct search calls).
func (e *Enricher) IMDBClient() *imdb.Client { return e.imdbClient }

// Resolve fetches metadata for a single IMDB ID.
//
// It tries each provider in priority order. The first one that returns
// at least an embed page (even with 0 servers) wins for the metadata
// (title PT-BR, backdrop). If a provider returns videos, we also tag
// the meta as Available=true.
func (e *Enricher) Resolve(ctx context.Context, imdbID, embedType string) (*MovieMeta, error) {
        meta := &MovieMeta{
                IMDB:      imdbID,
                EmbedType: embedType,
                Type:      embedTypeToType(embedType),
        }

        // Try to find a Supercine provider (or any provider that exposes
        // the FetchEmbed method) to get the PT-BR title + backdrop.
        // For now we only have Supercine, so we cast directly.
        if sp, ok := e.findSupercine(); ok {
                page, err := sp.FetchEmbed(ctx, imdbID, embedType)
                if err == nil && page != nil {
                        // Some Supercine pages return a JS template placeholder
                        // (e.g. `" + e.title + "`) instead of an actual title when
                        // the title isn't pre-rendered server-side. Detect and ignore.
                        title := strings.TrimSpace(page.TitlePTBR)
                        if title != "" && !looksLikeTemplate(title) {
                                meta.TitlePTBR = title
                        }
                        meta.BackdropURL = page.BackdropURL
                        meta.ServerCount = len(page.Servers)
                        meta.Available = meta.ServerCount > 0
                        meta.Provider = sp.Name()
                }
        }

        // Enrich with IMDB data (title original, year, poster, cast).
        if results, err := e.imdbClient.Search(ctx, imdbID); err == nil {
                for _, r := range results {
                        if r.ID == imdbID {
                                meta.TitleOrig = r.Title
                                meta.Year = r.Year
                                meta.PosterURL = r.PosterURL(400)
                                meta.Cast = r.Cast
                                meta.Rank = r.Rank
                                break
                        }
                }
        }

        // If PT-BR title is empty, fall back to the IMDB title.
        if meta.TitlePTBR == "" {
                meta.TitlePTBR = meta.TitleOrig
        }

        return meta, nil
}

// findSupercine returns the registered Supercine provider, if any.
// This is a temporary convenience — once we have multiple providers
// with their own FetchEmbed-like methods, we'll extract this into a
// common MetadataProvider interface.
func (e *Enricher) findSupercine() (*supercine.SupercineProvider, bool) {
        p := e.registry.Get("supercine")
        if p == nil {
                return nil, false
        }
        sp, ok := p.(*supercine.SupercineProvider)
        return sp, ok
}

// ResolveMany resolves a list of IMDB IDs in parallel (max 6 concurrent).
// Returns one MovieMeta per input ID, preserving order. Errors for
// individual IDs are swallowed (the entry will have Available=false).
func (e *Enricher) ResolveMany(ctx context.Context, ids []string, embedType string) []*MovieMeta {
        out := make([]*MovieMeta, len(ids))
        sem := make(chan struct{}, 6)
        var wg sync.WaitGroup
        for i, id := range ids {
                wg.Add(1)
                go func(idx int, imdbID string) {
                        defer wg.Done()
                        sem <- struct{}{}
                        defer func() { <-sem }()
                        meta, _ := e.Resolve(ctx, imdbID, embedType)
                        if meta == nil {
                                meta = &MovieMeta{
                                        IMDB:      imdbID,
                                        EmbedType: embedType,
                                        Type:      embedTypeToType(embedType),
                                }
                        }
                        out[idx] = meta
                }(i, id)
        }
        wg.Wait()
        return out
}

// SearchAndResolve searches IMDB by query, then resolves each result
// against the provider to get PT-BR title, backdrop, and availability.
//
// The `limit` parameter caps how many results are returned.
func (e *Enricher) SearchAndResolve(ctx context.Context, query string, limit int) ([]*MovieMeta, error) {
        results, err := e.imdbClient.Search(ctx, query)
        if err != nil {
                return nil, err
        }
        if limit > 0 && len(results) > limit {
                results = results[:limit]
        }

        out := make([]*MovieMeta, 0, len(results))
        for _, r := range results {
                meta, _ := e.Resolve(ctx, r.ID, r.EmbedType())
                if meta == nil {
                        // Even on error, include the basic IMDB info.
                        meta = &MovieMeta{
                                IMDB:      r.ID,
                                Type:      r.Type(),
                                EmbedType: r.EmbedType(),
                                TitleOrig: r.Title,
                                TitlePTBR: r.Title,
                                Year:      r.Year,
                                PosterURL: r.PosterURL(400),
                                Cast:      r.Cast,
                                Rank:      r.Rank,
                        }
                }
                out = append(out, meta)
        }
        return out, nil
}

// embedTypeToType converts the Supercine embed type to our internal type.
func embedTypeToType(embedType string) string {
        if embedType == "tvshows" {
                return "tv"
        }
        return "movie"
}

// TypeToEmbed converts our internal type to the Supercine embed type.
func TypeToEmbed(t string) string {
        if t == "tv" {
                return "tvshows"
        }
        return "movies"
}

// fmt import safety check (used in case we add formatted errors later).
var _ = fmt.Sprintf
var _ = strings.TrimSpace
var _ = time.Second

// looksLikeTemplate reports whether the title looks like a JS template
// placeholder (e.g. `" + e.title + "` or `${something}`) that the Supercine
// server returns when it doesn't have a real title to render.
func looksLikeTemplate(s string) bool {
        if strings.Contains(s, "+ e.") || strings.Contains(s, "${") {
                return true
        }
        if strings.HasPrefix(s, `" + `) || strings.HasSuffix(s, ` + "`) {
                return true
        }
        return false
}
