// Package supercine implements the Provider interface for the
// supercine-tv.net upstream service.
//
// The provider wraps two existing pieces:
//
//   1. The /embed-api/ endpoint, which returns an HTML page with
//      <server-selector> entries (each one an encrypted pointer to a
//      hoster like mixdrop.ps, doodstream.com, etc.).
//   2. The /embed-api/?action=embed&url=<data-server> endpoint, which
//      decrypts the data-server blob server-side and returns an HTML
//      with window.location.href = "<hoster-url>".
//   3. The hoster extractors (internal/extractors) which scrape the
//      hoster page to get the direct mp4/m3u8 URL.
//
// This file is intentionally thin — all the heavy lifting lives in
// the existing proxy/api and extractors packages. The provider just
// wires them together under the common Provider interface so that
// future providers (megahdfilmes, jellyfin, etc.) can be plugged in
// without touching the UI layer.
package supercine

import (
	"context"
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

// ProviderConfig holds the upstream connection settings.
type ProviderConfig struct {
	EmbedBase   string        // https://supercine-tv.net/embed-api/
	UserAgent   string
	HTTPTimeout time.Duration
}

// SupercineProvider implements provider.Provider for supercine-tv.net.
type SupercineProvider struct {
	cfg      ProviderConfig
	http     *http.Client
	registry *extractors.Registry
}

// New constructs a SupercineProvider.
func New(cfg ProviderConfig, registry *extractors.Registry) *SupercineProvider {
	if cfg.HTTPTimeout == 0 {
		cfg.HTTPTimeout = 20 * time.Second
	}
	return &SupercineProvider{
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

func (p *SupercineProvider) Name() string        { return "supercine" }
func (p *SupercineProvider) DisplayName() string { return "Supercine" }
func (p *SupercineProvider) Priority() int       { return 100 }

// HealthCheck pings the embed-api root to see if the upstream is reachable.
func (p *SupercineProvider) HealthCheck(ctx context.Context) error {
	req, _ := http.NewRequestWithContext(ctx, http.MethodGet, p.cfg.EmbedBase, nil)
	req.Header.Set("User-Agent", p.cfg.UserAgent)
	resp, err := p.http.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 500 {
		return fmt.Errorf("supercine: HTTP %d", resp.StatusCode)
	}
	return nil
}

// EmbedServer is one <server-selector> entry parsed from the embed page.
type EmbedServer struct {
	Index       int    `json:"index"`
	Server      string `json:"server"` // the encrypted data-server string
	Name        string `json:"name"`   // e.g. "Player Principal"
	Description string `json:"description"`
}

// EmbedPage is the parsed result of /embed-api/?imdb=...
type EmbedPage struct {
	IMDB        string        `json:"imdb"`
	Type        string        `json:"type"`
	TitlePTBR   string        `json:"title_ptbr"`
	BackdropURL string        `json:"backdrop_url"`
	Servers     []EmbedServer `json:"servers"`
}

var (
	backdropRe     = regexp.MustCompile(`background-image:\s*url\('([^']+)'\)`)
	ititleRe       = regexp.MustCompile(`<ititle>([^<]+)</ititle>`)
	serverRe       = regexp.MustCompile(`<server-selector[^>]*data-server="([^"]+)"[^>]*>[\s\S]*?</server-selector>`)
	serverNameRe   = regexp.MustCompile(`<b>([^<]+)</b>`)
	serverDescRe   = regexp.MustCompile(`<span>([^<]+)</span>`)
	redirectRe     = regexp.MustCompile(`window\.location\.href\s*=\s*"([^"]+)"`)
)

// FetchEmbed downloads the embed-api page and parses out the title,
// backdrop, and server list.
func (p *SupercineProvider) FetchEmbed(ctx context.Context, imdbID, embedType string) (*EmbedPage, error) {
	target := p.cfg.EmbedBase + "?imdb=" + url.QueryEscape(imdbID) + "&type=" + url.QueryEscape(embedType)
	req, _ := http.NewRequestWithContext(ctx, http.MethodGet, target, nil)
	req.Header.Set("User-Agent", p.cfg.UserAgent)
	req.Header.Set("Referer", "https://supercine-tv.net/")
	resp, err := p.http.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	bodyStr := string(body)

	page := &EmbedPage{IMDB: imdbID, Type: embedType}

	if m := ititleRe.FindStringSubmatch(bodyStr); len(m) >= 2 {
		page.TitlePTBR = strings.TrimSpace(m[1])
	}
	if m := backdropRe.FindStringSubmatch(bodyStr); len(m) >= 2 {
		page.BackdropURL = m[1]
	}

	// Parse <server-selector> blocks via goquery.
	doc, err := goquery.NewDocumentFromReader(strings.NewReader(bodyStr))
	if err == nil {
		doc.Find("server-selector").Each(func(i int, sel *goquery.Selection) {
			server, _ := sel.Attr("data-server")
			if server == "" {
				return
			}
			name := sel.Find("b").Text()
			desc := sel.Find("span").Text()
			page.Servers = append(page.Servers, EmbedServer{
				Index:       i,
				Server:      server,
				Name:        strings.TrimSpace(name),
				Description: strings.TrimSpace(desc),
			})
		})
	}

	// Fallback: regex if goquery missed any.
	if len(page.Servers) == 0 {
		matches := serverRe.FindAllStringSubmatch(bodyStr, -1)
		for i, m := range matches {
			nameM := serverNameRe.FindStringSubmatch(m[0])
			descM := serverDescRe.FindStringSubmatch(m[0])
			name := ""
			desc := ""
			if len(nameM) >= 2 {
				name = nameM[1]
			}
			if len(descM) >= 2 {
				desc = descM[1]
			}
			page.Servers = append(page.Servers, EmbedServer{
				Index:       i,
				Server:      m[1],
				Name:        name,
				Description: desc,
			})
		}
	}

	return page, nil
}

// resolveHosterURL calls /embed-api/?action=embed&url=<data-server> and
// extracts the hoster URL from the window.location.href redirect.
func (p *SupercineProvider) resolveHosterURL(ctx context.Context, dataServer, referer string) (string, error) {
	target := p.cfg.EmbedBase + "?action=embed&url=" + url.QueryEscape(dataServer)
	req, _ := http.NewRequestWithContext(ctx, http.MethodGet, target, nil)
	req.Header.Set("User-Agent", p.cfg.UserAgent)
	req.Header.Set("Referer", referer)
	resp, err := p.http.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	m := redirectRe.FindStringSubmatch(string(body))
	if len(m) < 2 {
		return "", fmt.Errorf("supercine: no redirect URL found in action=embed response")
	}
	return m[1], nil
}

// Resolve implements provider.Provider.
//
// Flow:
//   1. Fetch the embed page → get list of servers.
//   2. For each server (up to 3 attempts), resolve the hoster URL.
//   3. Run the appropriate hoster extractor to get the direct video URL.
//   4. Return the first successful result with all servers listed for
//      fallback (the UI can retry with a different server on failure).
func (p *SupercineProvider) Resolve(ctx context.Context, imdbID, embedType string) (*provider.ResolveResult, error) {
	page, err := p.FetchEmbed(ctx, imdbID, embedType)
	if err != nil {
		return nil, fmt.Errorf("%w: %v", provider.ErrProviderDown, err)
	}
	if len(page.Servers) == 0 {
		return nil, provider.ErrUnavailable
	}

	// Build the result skeleton with all servers (so the UI can offer
	// the user a choice even if extraction fails for some).
	result := &provider.ResolveResult{
		Provider: p.Name(),
		IMDB:     imdbID,
		Type:     embedType,
	}
	for _, s := range page.Servers {
		result.Servers = append(result.Servers, provider.Server{
			Index:       s.Index,
			Name:        s.Name,
			Description: s.Description,
		})
	}

	// Try to extract a direct URL. Try at most 3 servers to avoid
	// hammering the upstream when most are down.
	maxAttempts := 3
	if maxAttempts > len(page.Servers) {
		maxAttempts = len(page.Servers)
	}

	embedURL := p.cfg.EmbedBase + "?imdb=" + url.QueryEscape(imdbID) + "&type=" + url.QueryEscape(embedType)
	var lastErr error
	for i := 0; i < maxAttempts; i++ {
		srv := page.Servers[i]
		hosterURL, err := p.resolveHosterURL(ctx, srv.Server, embedURL)
		if err != nil {
			lastErr = err
			continue
		}
		ext, err := p.registry.Dispatch(ctx, hosterURL)
		if err != nil || ext == nil || len(ext.Videos) == 0 {
			lastErr = err
			continue
		}
		// Success — copy video URLs.
		for _, v := range ext.Videos {
			result.Videos = append(result.Videos, provider.VideoURL{
				URL:     v.URL,
				Quality: v.Quality,
			})
		}
		// Tag the server that worked.
		result.Servers[i].Description = fmt.Sprintf("[OK] %s", result.Servers[i].Description)
		return result, nil
	}

	if lastErr != nil {
		return result, lastErr
	}
	return result, provider.ErrUnavailable
}
