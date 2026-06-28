// Package imdb provides a small client for the public IMDB suggestion
// endpoint (https://v3.sg.media-imdb.com/suggestion/<letter>/<query>.json).
//
// This endpoint is free, requires no API key, and returns a JSON list of
// movies / series / people matching the query, each with an IMDB ID, title,
// year, poster URL, and rank. We use it as the catalog backbone for the
// streaming UI — combined with the Supercine /embed-api/ endpoint, this
// gives us a complete search-and-watch experience with zero external
// dependencies.
package imdb

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"time"
)

// Client is the IMDB suggestions client.
type Client struct {
	http *http.Client
}

// New returns a Client.
func New() *Client {
	return &Client{
		http: &http.Client{Timeout: 12 * time.Second},
	}
}

// Result is a single suggestion entry returned by the IMDB endpoint.
type Result struct {
	ID    string `json:"id"`   // IMDB ID, e.g. "tt0133093"
	Title string `json:"l"`    // localized title
	Year  int    `json:"y"`    // release year (0 if absent)
	Rank  int    `json:"rank"` // popularity rank (lower = more popular)
	QID   string `json:"qid"`  // "movie", "tvSeries", "tvMiniSeries", etc.
	Image *struct {
		URL    string `json:"imageUrl"`
		Height int    `json:"height"`
		Width  int    `json:"width"`
	} `json:"i"`
	Cast string `json:"s"` // cast summary string
}

// rawResponse mirrors the JSON shape returned by the endpoint.
type rawResponse struct {
	D []Result `json:"d"`
}

// Search queries the IMDB suggestion endpoint and returns matching entries.
// The query is lowercased and spaces are replaced with underscores to match
// the IMDB URL convention (e.g. "homem aranha" -> /suggestion/h/homem_aranha.json).
//
// Only entries that look like a movie or TV show (id starts with "tt") are
// returned — people (id starts with "nm") and list pages are filtered out.
func (c *Client) Search(ctx context.Context, query string) ([]Result, error) {
	q := strings.TrimSpace(query)
	if q == "" {
		return nil, nil
	}
	normalized := normalize(q)
	if normalized == "" {
		return nil, fmt.Errorf("invalid query")
	}
	first := string(normalized[0])
	endpoint := fmt.Sprintf("https://v3.sg.media-imdb.com/suggestion/%s/%s.json", first, normalized)

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/137.0.0.0 Safari/537.36")
	req.Header.Set("Accept", "application/json, text/plain, */*")
	req.Header.Set("Accept-Language", "pt-BR,pt;q=0.9,en;q=0.8")

	resp, err := c.http.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("imdb: HTTP %d", resp.StatusCode)
	}

	var raw rawResponse
	if err := json.NewDecoder(resp.Body).Decode(&raw); err != nil {
		return nil, fmt.Errorf("imdb: failed to decode response: %w", err)
	}

	// Filter: keep only tt* entries (movies/series).
	out := make([]Result, 0, len(raw.D))
	for _, r := range raw.D {
		if !strings.HasPrefix(r.ID, "tt") {
			continue
		}
		out = append(out, r)
	}
	return out, nil
}

// normalize lowercases, replaces whitespace with underscores, and URL-encodes
// any remaining non-URL-safe characters. IMDB treats "homem_aranha" as a
// single suggestion key.
func normalize(q string) string {
	q = strings.ToLower(strings.TrimSpace(q))
	q = strings.ReplaceAll(q, " ", "_")
	q = strings.ReplaceAll(q, "-", "_")
	// URL-encode but keep underscores readable.
	return url.PathEscape(q)
}

// PosterURL returns the poster URL of a Result, optionally resized via the
// IMDB _V1_ parameter (e.g. _V1_SX300 for 300px wide). If the result has no
// poster (e.g. upcoming movies), returns an empty string.
func (r *Result) PosterURL(width int) string {
	if r.Image == nil || r.Image.URL == "" {
		return ""
	}
	if width <= 0 {
		return r.Image.URL
	}
	// IMDB images are served with a _V1_ suffix that can take size params.
	// Common form: https://m.media-amazon.com/images/M/MV5B...@._V1_.jpg
	// We want: ...@._V1_SX300_.jpg
	u := r.Image.URL
	if idx := strings.Index(u, "_V1_"); idx >= 0 {
		return u[:idx] + fmt.Sprintf("_V1_SX%d_.jpg", width)
	}
	return u
}

// Type returns a normalized type string for the result.
//
//	QID          -> Type
//	movie        -> movie
//	tvSeries     -> tv
//	tvMiniSeries -> tv
//	tvSpecial    -> tv
//	""           -> movie (fallback)
//	other        -> other
func (r *Result) Type() string {
	switch r.QID {
	case "movie":
		return "movie"
	case "tvSeries", "tvMiniSeries", "tvSpecial":
		return "tv"
	case "":
		return "movie"
	default:
		return r.QID
	}
}

// EmbedType returns the type string expected by the Supercine /embed-api/
// endpoint: "movies" for movies and "tvshows" for any TV-like type.
func (r *Result) EmbedType() string {
	if r.Type() == "tv" {
		return "tvshows"
	}
	return "movies"
}
