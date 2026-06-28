// Package amenic implements the Provider interface for the Amenic Plus
// Android app (package media6.app.amenic).
//
// The Amenic Plus app (analyzed from Amenic.Plus_1.7.3_antisplit.apk) is a
// WebView-based streaming app. Its assets/www/home.html loads the main
// UI from https://amenic-file.com/main?<query>, and the player logic is
// injected dynamically via https://amenic-file.com/js/players.js?<query>.
//
// Key findings from the APK analysis:
//
//   1. Main entry: GET https://amenic-file.com/main?v=1.7.3&r=<android_id>
//      Returns the home screen HTML. The query string identifies the
//      device and app version.
//
//   2. Player JS: GET https://amenic-file.com/js/players.js?<query>
//      Injected into the WebView via a `javascript:` URL. Contains the
//      list of available players (hosters) for the currently selected
//      title.
//
//   3. Player data shape (from assets/www/player.html):
//        data['data'][i]['file']     // direct mp4 URL
//        data['data'][i]['label']    // quality label ("720p", "1080p")
//        data['player']['poster_file']  // appended to https://thumb.fvs.io/asset
//        data['captions'][i]['hash']    // subtitle hash
//        data['captions'][i]['id']      // subtitle ID
//        data['captions'][i]['extension']  // subtitle extension (.srt, .vtt)
//
//   4. Thumbnails: https://thumb.fvs.io/asset<poster_file>
//
//   5. Metadata: the app uses The Movie Database (TMDB) API for title
//      metadata (posters, backdrops, synopses). It does NOT have its own
//      catalog API — the home screen is rendered server-side by
//      amenic-file.com/main.
//
// ⚠️ IMPORTANT — Cloudflare protection:
// amenic-file.com is behind Cloudflare with "managed challenge" mode enabled.
// This means datacenter IPs (like Render's) are blocked with HTTP 403 and
// a JS challenge that requires a real browser. The Amenic provider will
// only work from residential IPs or from a browser-like environment that
// can solve the Cloudflare challenge.
//
// When the Cloudflare block is hit, the provider returns ErrProviderDown
// with a clear message so the proxy falls back to the next provider
// (Supercine).
package amenic

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"regexp"
	"strings"
	"time"

	"github.com/PuerkitoBio/goquery"
	"github.com/deivid22srk/supercine-proxy/internal/extractors"
	"github.com/deivid22srk/supercine-proxy/internal/provider"
)

// ProviderConfig holds the upstream connection settings for Amenic.
type ProviderConfig struct {
	// BaseURL is the Amenic file server root. Default: https://amenic-file.com
	BaseURL string

	// ThumbBase is the thumbnail/asset CDN root.
	// Default: https://thumb.fvs.io/asset
	ThumbBase string

	// AppVersion is the version string sent in the `v` query parameter.
	// The APK we analyzed reports 1.7.3.
	AppVersion string

	// DeviceID is sent in the `r` query parameter. The original app uses
	// the Android device ID. We default to a stable pseudo-ID; the
	// server doesn't appear to validate it.
	DeviceID string

	// UserAgent is sent on every outbound request. We default to the
	// exact UA the APK uses (Android WebView).
	UserAgent string

	// HTTPTimeout is the per-request timeout.
	HTTPTimeout time.Duration
}

// AmenicProvider implements provider.Provider for amenic-file.com.
type AmenicProvider struct {
	cfg      ProviderConfig
	http     *http.Client
	registry *extractors.Registry
}

// New constructs an AmenicProvider.
func New(cfg ProviderConfig, registry *extractors.Registry) *AmenicProvider {
	if cfg.BaseURL == "" {
		cfg.BaseURL = "https://amenic-file.com"
	}
	if cfg.ThumbBase == "" {
		cfg.ThumbBase = "https://thumb.fvs.io/asset"
	}
	if cfg.AppVersion == "" {
		cfg.AppVersion = "1.7.3"
	}
	if cfg.DeviceID == "" {
		cfg.DeviceID = "supercine-proxy-amenic"
	}
	if cfg.UserAgent == "" {
		// Match the UA the APK sends (Android WebView). Cloudflare's
		// managed challenge is more lenient with mobile UAs.
		cfg.UserAgent = "Mozilla/5.0 (Linux; Android 13; SM-S901B Build/TP1A.220624.014; wv) AppleWebKit/537.36 (KHTML, like Gecko) Version/4.0 Chrome/120.0.6099.230 Mobile Safari/537.36"
	}
	if cfg.HTTPTimeout == 0 {
		cfg.HTTPTimeout = 20 * time.Second
	}
	return &AmenicProvider{
		cfg: cfg,
		http: &http.Client{
			Timeout: cfg.HTTPTimeout,
			CheckRedirect: func(req *http.Request, via []*http.Request) error {
				req.Header.Set("User-Agent", cfg.UserAgent)
				return nil
			},
		},
		registry: registry,
	}
}

func (p *AmenicProvider) Name() string        { return "amenic" }
func (p *AmenicProvider) DisplayName() string { return "Amenic Plus" }

// Priority is higher than Supercine (100) so Supercine is tried first.
// Amenic is the fallback — its Cloudflare protection makes it less
// reliable from datacenter IPs.
func (p *AmenicProvider) Priority() int { return 200 }

// HealthCheck pings the Amenic base URL to see if it's reachable.
// Note: amenic-file.com is behind Cloudflare, so a 403 from Cloudflare
// still counts as "reachable" — the provider just can't fetch content
// from datacenter IPs.
func (p *AmenicProvider) HealthCheck(ctx context.Context) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodHead, p.cfg.BaseURL+"/", nil)
	if err != nil {
		return err
	}
	req.Header.Set("User-Agent", p.cfg.UserAgent)
	resp, err := p.http.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	// 200, 403 (Cloudflare challenge), 404 all indicate the host is up.
	// Only 5xx and network errors count as down.
	if resp.StatusCode >= 500 {
		return fmt.Errorf("amenic: HTTP %d", resp.StatusCode)
	}
	return nil
}

// queryFor builds the query string the APK sends to amenic-file.com.
//   v=<app_version>&r=<device_id>
// The server uses these for analytics and rate-limiting; it doesn't
// validate them strictly.
func (p *AmenicProvider) queryFor() string {
	q := url.Values{}
	q.Set("v", p.cfg.AppVersion)
	q.Set("r", p.cfg.DeviceID)
	return q.Encode()
}

// fetchURL fetches a URL from amenic-file.com with the proper headers
// and returns the response body. Returns ErrProviderDown if Cloudflare
// blocks the request with a managed challenge.
func (p *AmenicProvider) fetchURL(ctx context.Context, path string) ([]byte, int, error) {
	target := p.cfg.BaseURL + path
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, target, nil)
	if err != nil {
		return nil, 0, err
	}
	req.Header.Set("User-Agent", p.cfg.UserAgent)
	req.Header.Set("Accept", "text/html,application/xhtml+xml,application/xml;q=0.9,*/*;q=0.8")
	req.Header.Set("Accept-Language", "pt-BR,pt;q=0.9,en-US;q=0.8,en;q=0.7")
	req.Header.Set("X-Requested-With", "media6.app.amenic")
	req.Header.Set("Referer", p.cfg.BaseURL+"/")

	resp, err := p.http.Do(req)
	if err != nil {
		return nil, 0, err
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)

	// Detect Cloudflare's managed challenge. The response is HTML with
	// a <title>Attention Required! | Cloudflare</title> or
	// "Sorry, you have been blocked" message.
	if resp.StatusCode == http.StatusForbidden {
		bodyStr := string(body)
		if strings.Contains(bodyStr, "cloudflare") ||
			strings.Contains(bodyStr, "Attention Required") ||
			strings.Contains(bodyStr, "you have been blocked") ||
			strings.Contains(bodyStr, "cf-mitigated") {
			return nil, resp.StatusCode, fmt.Errorf("%w: amenic-file.com bloqueado por Cloudflare (managed challenge) — requer IP residencial ou navegador real", provider.ErrProviderDown)
		}
	}
	return body, resp.StatusCode, nil
}

// EmbedServer is one player/server entry parsed from the Amenic page.
// Mirrors the data['data'][i] shape from assets/www/player.html.
type EmbedServer struct {
	File  string `json:"file"`  // direct mp4 URL
	Label string `json:"label"` // quality label ("720p", "1080p")
	Type  string `json:"type"`  // hoster name (e.g. "mp4", "hls")
}

// PlayerData is the JSON shape the Amenic player.js returns.
type PlayerData struct {
	Player   struct {
		PosterFile string `json:"poster_file"`
	} `json:"player"`
	Data     []EmbedServer `json:"data"`
	Captions []struct {
		Hash      string `json:"hash"`
		ID        string `json:"id"`
		Language  string `json:"language"`
		Extension string `json:"extension"`
	} `json:"captions"`
}

// Resolve implements provider.Provider.
//
// For movies (embedType="movies"):
//   1. Fetch /main?<query> — the home page HTML. We parse it to find
//      the title's player URL.
//   2. Fetch /js/players.js?<query> — the player data (URLs).
//   3. Parse the player data and return direct URLs.
//
// For TV shows (embedType="tvshows"):
//   Amenic supports episodes but the API requires a title ID + season +
//   episode, which we don't have from the IMDB ID alone. Refuse to
//   resolve blindly — caller should use ResolveEpisode().
func (p *AmenicProvider) Resolve(ctx context.Context, imdbID, embedType string) (*provider.ResolveResult, error) {
	if embedType == "tvshows" {
		return nil, fmt.Errorf("amenic: séries requerem season+episode — use ResolveEpisode()")
	}

	// Step 1: fetch the home page to find the title's player URL.
	// The home page is rendered server-side and contains links to
	// /player?id=<title_id> for each title. We search by IMDB ID.
	homeBody, status, err := p.fetchURL(ctx, "/main?"+p.queryFor())
	if err != nil {
		return nil, err
	}
	if status != http.StatusOK {
		return nil, fmt.Errorf("amenic: home returned HTTP %d", status)
	}

	// Parse the home page to find the title's player link.
	titleURL, err := p.findTitleLink(ctx, string(homeBody), imdbID)
	if err != nil {
		return nil, err
	}
	if titleURL == "" {
		return nil, provider.ErrUnavailable
	}

	// Step 2: fetch the player page to get the player.js URL.
	playerBody, _, err := p.fetchURL(ctx, titleURL)
	if err != nil {
		return nil, err
	}

	// Step 3: extract the players.js URL from the player page HTML.
	// The page contains a script tag like:
	//   <script src="https://amenic-file.com/js/players.js?...">
	playersJSURL := p.extractPlayersJSURL(string(playerBody))
	if playersJSURL == "" {
		return nil, fmt.Errorf("amenic: players.js URL não encontrada na página do player")
	}

	// Step 4: fetch the players.js content. It's a JS file that
	// populates a global variable with the player data.
	playersBody, _, err := p.fetchURL(ctx, playersJSURL)
	if err != nil {
		return nil, err
	}

	// Step 5: extract the JSON data from the JS.
	data, err := p.parsePlayersJS(string(playersBody))
	if err != nil {
		return nil, fmt.Errorf("amenic: falha ao parsear players.js: %w", err)
	}

	// Build the result.
	result := &provider.ResolveResult{
		Provider: p.Name(),
		IMDB:     imdbID,
		Type:     embedType,
	}
	for i, s := range data.Data {
		if s.File == "" {
			continue
		}
		quality := s.Label
		if quality == "" {
			quality = "Normal"
		}
		result.Servers = append(result.Servers, provider.Server{
			Index:       i,
			Name:        fmt.Sprintf("Servidor %d", i+1),
			Description: s.Type,
		})
		result.Videos = append(result.Videos, provider.VideoURL{
			URL:     s.File,
			Quality: quality,
		})
	}

	if len(result.Videos) == 0 {
		return result, provider.ErrUnavailable
	}
	result.Servers[0].Description = "[OK] " + result.Servers[0].Description
	return result, nil
}

// findTitleLink searches the home page HTML for a link to the title's
// player page. The Amenic home page uses TMDB IDs, so we look for the
// IMDB ID in the page content.
func (p *AmenicProvider) findTitleLink(ctx context.Context, homeHTML, imdbID string) (string, error) {
	doc, err := goquery.NewDocumentFromReader(strings.NewReader(homeHTML))
	if err != nil {
		return "", fmt.Errorf("amenic: falha ao parsear home: %w", err)
	}

	// Look for any link that contains the IMDB ID in its href or
	// data attribute.
	var titleURL string
	doc.Find("a").EachWithBreak(func(_ int, sel *goquery.Selection) bool {
		href, _ := sel.Attr("href")
		dataImdb, _ := sel.Attr("data-imdb")
		dataID, _ := sel.Attr("data-id")
		_ = dataID

		if strings.Contains(href, imdbID) || dataImdb == imdbID {
			titleURL = href
			return false
		}
		return true
	})

	// If we didn't find a direct link, try a search-style URL.
	// amenic-file.com may expose /search?q=<imdb> or /player?imdb=<imdb>.
	if titleURL == "" {
		// As a last resort, try /player?imdb=<imdb>.
		return "/player?imdb=" + url.QueryEscape(imdbID), nil
	}

	// Make absolute.
	if strings.HasPrefix(titleURL, "/") {
		titleURL = p.cfg.BaseURL + titleURL
	}
	return titleURL, nil
}

// playersJSRe matches the players.js URL inside a <script src="..."> tag.
var playersJSRe = regexp.MustCompile(`https?://[a-zA-Z0-9.-]+\.com/js/players\.js\?[^"'\s]+`)

// extractPlayersJSURL pulls the players.js URL out of the player page HTML.
func (p *AmenicProvider) extractPlayersJSURL(playerHTML string) string {
	m := playersJSRe.FindString(playerHTML)
	return m
}

// jsonInJSRe extracts the JSON object assigned to a variable in the
// players.js file. The JS typically looks like:
//   var players = {"player":{...},"data":[...],"captions":[...]};
// or:
//   window.players = {...};
var jsonInJSRe = regexp.MustCompile(`(?:var\s+players|window\.players)\s*=\s*(\{[\s\S]*?\});`)

// parsePlayersJS extracts the player data JSON from the players.js file.
func (p *AmenicProvider) parsePlayersJS(js string) (*PlayerData, error) {
	m := jsonInJSRe.FindStringSubmatch(js)
	if len(m) < 2 {
		// Fallback: try to find any top-level JSON object in the JS.
		start := strings.Index(js, "{")
		end := strings.LastIndex(js, "}")
		if start < 0 || end < 0 || end <= start {
			return nil, fmt.Errorf("JSON não encontrado em players.js")
		}
		jsonStr := js[start : end+1]
		var data PlayerData
		if err := json.Unmarshal([]byte(jsonStr), &data); err != nil {
			return nil, fmt.Errorf("falha ao decodificar JSON: %w", err)
		}
		return &data, nil
	}
	var data PlayerData
	if err := json.Unmarshal([]byte(m[1]), &data); err != nil {
		return nil, fmt.Errorf("falha ao decodificar JSON: %w", err)
	}
	return &data, nil
}

// ResolveEpisode resolves a specific episode to a direct video URL.
// Amenic supports episodes but requires the title's internal ID, which
// we don't have from the IMDB ID alone. This is a placeholder that
// returns ErrUnavailable — to be implemented when we have a way to
// map IMDB IDs to Amenic title IDs.
func (p *AmenicProvider) ResolveEpisode(ctx context.Context, imdbID string, season, episode int) (*provider.ResolveResult, error) {
	return nil, provider.ErrUnavailable
}

// Ensure imports are used.
var _ = io.ReadAll
var _ = http.StatusOK
