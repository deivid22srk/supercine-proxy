// Package enricher resolves an IMDB ID into a rich metadata record by
// combining two free sources:
//
//   1. The Supercine /embed-api/ endpoint (which returns the PT-BR title
//      and a TMDB backdrop URL even when the v1.0.0 REST API is locked).
//   2. The IMDB suggestion endpoint (which returns the original English
//      title, year, and a poster URL).
//
// The result is a single "MovieMeta" record suitable for display in the
// streaming UI.
package enricher

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"regexp"
	"strings"
	"sync"
	"time"

	"github.com/deivid22srk/supercine-proxy/internal/imdb"
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
	Available   bool   `json:"available"`    // false if Supercine has no servers
	ServerCount int    `json:"server_count"` // # of <server-selector> entries
}

// Enricher combines IMDB search + Supercine embed lookup.
type Enricher struct {
	http       *http.Client
	embedBase  string
	userAgent  string
	imdbClient *imdb.Client
}

// New constructs an Enricher.
func New(embedBase, userAgent string) *Enricher {
	return &Enricher{
		http:       &http.Client{Timeout: 12 * time.Second},
		embedBase:  embedBase,
		userAgent:  userAgent,
		imdbClient: imdb.New(),
	}
}

// IMDBClient returns the underlying IMDB client (for direct search calls).
func (e *Enricher) IMDBClient() *imdb.Client { return e.imdbClient }

var (
	backdropRe = regexp.MustCompile(`background-image:\s*url\('([^']+)'\)`)
	ititleRe   = regexp.MustCompile(`<ititle>([^<]+)</ititle>`)
	serverRe   = regexp.MustCompile(`<server-selector[^>]*data-server="[^"]+"`)
)

// Resolve fetches the Supercine embed for a single IMDB ID and returns the
// combined metadata. Returns the meta with Available=false if Supercine
// doesn't have the title.
func (e *Enricher) Resolve(ctx context.Context, imdbID, embedType string) (*MovieMeta, error) {
	target := e.embedBase + "?imdb=" + imdbID + "&type=" + embedType
	req, _ := http.NewRequestWithContext(ctx, http.MethodGet, target, nil)
	req.Header.Set("User-Agent", e.userAgent)
	req.Header.Set("Referer", "https://supercine-tv.net/")
	resp, err := e.http.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	bodyStr := string(body)

	meta := &MovieMeta{
		IMDB:      imdbID,
		EmbedType: embedType,
		Type:      embedTypeToType(embedType),
	}

	// Title PT-BR
	if m := ititleRe.FindStringSubmatch(bodyStr); len(m) >= 2 {
		meta.TitlePTBR = strings.TrimSpace(m[1])
	}

	// Backdrop URL (TMDB)
	if m := backdropRe.FindStringSubmatch(bodyStr); len(m) >= 2 {
		meta.BackdropURL = m[1]
	}

	// Server count — proxy for availability
	meta.ServerCount = len(serverRe.FindAllString(bodyStr, -1))
	meta.Available = meta.ServerCount > 0

	// Try to enrich with IMDB data (title original, year, poster)
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

// ResolveMany resolves a list of IMDB IDs in parallel (max 6 concurrent).
// Returns one MovieMeta per input ID, preserving order. Errors for individual
// IDs are swallowed (the entry will have Available=false).
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
			meta, err := e.Resolve(ctx, imdbID, embedType)
			if err != nil || meta == nil {
				out[idx] = &MovieMeta{
					IMDB:      imdbID,
					EmbedType: embedType,
					Type:      embedTypeToType(embedType),
				}
				return
			}
			out[idx] = meta
		}(i, id)
	}
	wg.Wait()
	return out
}

// SearchAndResolve searches IMDB by query, then resolves each result against
// Supercine to get the PT-BR title, backdrop, and availability.
//
// The `limit` parameter caps how many results are returned (the IMDB endpoint
// typically returns 8–20 results).
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
		meta, err := e.Resolve(ctx, r.ID, r.EmbedType())
		if err != nil || meta == nil {
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
