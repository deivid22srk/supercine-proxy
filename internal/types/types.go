package types

import "time"

// Jmodel mirrors tv.supercine.supercine.Jmodel — a single extracted video source.
type Jmodel struct {
	URL     string `json:"url"`
	Quality string `json:"quality"`
}

// ExtractorResult is the structured result returned by every hoster extractor.
type ExtractorResult struct {
	Hoster     string   `json:"hoster"`
	URL        string   `json:"url"`
	Videos     []Jmodel `json:"videos"`
	Took       string   `json:"took"`
	Error      string   `json:"error,omitempty"`
	StatusCode int      `json:"status_code,omitempty"`
}

// LogEntry represents one proxied HTTP request visible in the dashboard.
type LogEntry struct {
	ID         string    `json:"id"`
	Timestamp  time.Time `json:"timestamp"`
	Method     string    `json:"method"`
	Path       string    `json:"path"`
	Upstream   string    `json:"upstream"`
	StatusCode int       `json:"status_code"`
	Duration   string    `json:"duration"`
	Size       int64     `json:"size"`
	Cached     bool      `json:"cached"`
	Error      string    `json:"error,omitempty"`
}

// Stats holds aggregate stats for the dashboard.
type Stats struct {
	TotalRequests   int64            `json:"total_requests"`
	CacheHits       int64            `json:"cache_hits"`
	CacheMisses     int64            `json:"cache_misses"`
	Errors          int64            `json:"errors"`
	BytesProxied    int64            `json:"bytes_proxied"`
	ByStatus        map[string]int64 `json:"by_status"`
	ByPath          map[string]int64 `json:"by_path"`
	ExtractorCalls  int64            `json:"extractor_calls"`
	ExtractorErrors int64            `json:"extractor_errors"`
}

// CacheEntry is a single response cached by the proxy.
type CacheEntry struct {
	Body       []byte
	Headers    map[string][]string
	StatusCode int
	StoredAt   time.Time
}

// EmbedServer represents one <server-selector> entry from the embed page.
type EmbedServer struct {
	ID          string `json:"id"`
	Server      string `json:"server"` // the encrypted data-server string
	Lang        string `json:"lang"`
	Title       string `json:"title"`
	Description string `json:"description"`
}

// EmbedPage is the parsed result of /embed-api/?imdb=...
type EmbedPage struct {
	IMDB    string        `json:"imdb"`
	Type    string        `json:"type"` // movies | tvshows
	Title   string        `json:"title"`
	Servers []EmbedServer `json:"servers"`
}

// TvShow describes a series/anime returned by /api/tvshows.
type TvShow struct {
	IMDB    string   `json:"imdb"`
	Title   string   `json:"title"`
	Poster  string   `json:"poster"`
	Seasons []Season `json:"seasons"`
}

// Season groups episodes.
type Season struct {
	Number   int       `json:"number"`
	Episodes []Episode `json:"episodes"`
}

// Episode represents a single episode entry.
type Episode struct {
	Number int    `json:"number"`
	Title  string `json:"title"`
	URL    string `json:"url"`
}

// Plan is a subscription plan returned by /auth/plans.
type Plan struct {
	ID     string  `json:"id"`
	Label  string  `json:"label"`
	Days   int     `json:"days"`
	Amount float64 `json:"amount"`
}
