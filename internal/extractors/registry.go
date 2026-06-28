// Package extractors ports the original tv.supercine.supercine.sites.* extractors
// from Java/Kotlin to Go. Each extractor takes a hoster URL (e.g.
// https://mixdrop.ps/e/abc123) and returns a list of direct video URLs
// (mp4/m3u8) by scraping the hoster page.
//
// The original APK uses MethodChannel("com.example/links") with method
// "extractLinks" to call ExtractorLinks.find(url), which dispatches to the
// matching site based on RegexExpress patterns. We replicate that here.
package extractors

import (
	"context"
	"fmt"
	"net/http"
	"regexp"
	"strings"
	"time"

	"github.com/deivid22srk/supercine-proxy/internal/types"
)

// UserAgent is the same Chrome UA used by the original APK
// (see tv.supercine.supercine.ExtractorLinks.agent).
const UserAgent = "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/137.0.0.0 Safari/537.36"

// Extractor is the common interface every hoster scraper implements.
type Extractor interface {
	// Name returns the hoster identifier (e.g. "doodstream").
	Name() string
	// Match reports whether the URL belongs to this hoster.
	Match(url string) bool
	// Extract fetches the page and returns direct video URLs.
	Extract(ctx context.Context, url string) (*types.ExtractorResult, error)
}

// Registry holds all known extractors and dispatches by URL.
type Registry struct {
	extractors []Extractor
}

// NewRegistry returns a Registry pre-populated with all built-in extractors.
func NewRegistry() *Registry {
	return &Registry{
		extractors: []Extractor{
			NewDoodStream(),
			NewStreamWish(),
			NewVidHide(),
			NewFileMoon(),
			NewFileLions(),
			NewMixDrop(),
			NewStreamTape(),
			NewVoe(),
		},
	}
}

// Find returns the first extractor that matches the URL, or nil.
func (r *Registry) Find(url string) Extractor {
	for _, e := range r.extractors {
		if e.Match(url) {
			return e
		}
	}
	return nil
}

// All returns every registered extractor.
func (r *Registry) All() []Extractor {
	return r.extractors
}

// Dispatch is the Go equivalent of ExtractorLinks.find(url).
// It returns an error if no extractor matches.
func (r *Registry) Dispatch(ctx context.Context, url string) (*types.ExtractorResult, error) {
	e := r.Find(url)
	if e == nil {
		return nil, fmt.Errorf("URL não encontrada 🛑 (nenhum extractor suporta: %s)", url)
	}
	start := time.Now()
	res, err := e.Extract(ctx, url)
	if err != nil {
		return &types.ExtractorResult{
			Hoster: e.Name(),
			URL:    url,
			Took:   time.Since(start).String(),
			Error:  err.Error(),
		}, err
	}
	res.Took = time.Since(start).String()
	return res, nil
}

// httpClient builds the standard *http.Client used by all extractors.
func httpClient() *http.Client {
	return &http.Client{
		Timeout: 25 * time.Second,
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			// Keep our UA + Referer on redirects.
			req.Header.Set("User-Agent", UserAgent)
			return nil
		},
	}
}

// newGet builds a GET request with the canonical headers used by the APK.
func newGet(ctx context.Context, url, referer string) (*http.Request, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", UserAgent)
	req.Header.Set("Accept", "text/html,application/xhtml+xml,application/xml;q=0.9,*/*;q=0.8")
	req.Header.Set("Accept-Language", "pt-BR,pt;q=0.9,en;q=0.8")
	if referer != "" {
		req.Header.Set("Referer", referer)
	}
	return req, nil
}

// matchAny returns true if the URL matches any of the given regexes.
func matchAny(url string, patterns ...string) bool {
	for _, p := range patterns {
		re := regexp.MustCompile(p)
		if re.MatchString(url) {
			return true
		}
	}
	return false
}

// qualityFor returns a human-readable quality label based on the URL extension.
// Mirrors the StreamWish/VidHide addSource() logic in the APK.
func qualityFor(url string) string {
	switch {
	case strings.Contains(url, ".mp4"):
		return "MP4 Video"
	case strings.Contains(url, ".m3u8"):
		return "HLS (m3u8)"
	case strings.Contains(url, ".mkv"):
		return "MKV Video"
	default:
		return "Normal"
	}
}
