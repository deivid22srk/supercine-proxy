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
// For TV series, two additional endpoints are used:
//
//   4. /wp-json/api/tvshows?what=seasons&tmdb=<TMDB_ID>&version=1.0&origin=web
//      Returns JSON with all seasons and episodes (each episode has its
//      own backdrop and title in PT-BR when available).
//   5. /wp-json/api/tvshows?what=player&tmdb=<TMDB_ID>&season=X&episode=Y&version=1.0&origin=web
//      Returns JSON with the list of available players for the specific
//      episode. Each player has a `url` (same encrypted format as the
//      movie data-server) and a `type` field naming the hoster
//      ("mixdrop", "streamwish", "vidhide", "doodstream", etc.).
//
// This file is intentionally thin — all the heavy lifting lives in
// the existing proxy/api and extractors packages. The provider just
// wires them together under the common Provider interface so that
// future providers (megahdfilmes, jellyfin, etc.) can be plugged in
// without touching the UI layer.
package supercine

import (
        "context"
        "encoding/json"
        "fmt"
        "io"
        "net/http"
        "net/url"
        "regexp"
        "sort"
        "strconv"
        "strings"
        "sync"
        "time"

        "github.com/PuerkitoBio/goquery"
        "github.com/deivid22srk/supercine-proxy/internal/extractors"
        "github.com/deivid22srk/supercine-proxy/internal/provider"
)

// ProviderConfig holds the upstream connection settings.
type ProviderConfig struct {
        EmbedBase   string // https://supercine-tv.net/embed-api/
        APIBase     string // https://supercine-tv.net/wp-json/api
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
        if cfg.APIBase == "" {
                cfg.APIBase = "https://supercine-tv.net/wp-json/api"
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
        TMDBID      string        `json:"tmdb_id,omitempty"` // populated for tvshows
        Servers     []EmbedServer `json:"servers"`
}

// ===== TV series types =====

// Season represents one season of a TV show.
type Season struct {
        Number   int       `json:"number"`  // 1-based
        ID       string    `json:"id"`      // Supercine internal ID
        Episodes []Episode `json:"episodes"`
}

// Episode represents one episode of a season.
type Episode struct {
        Number   int    `json:"number"`   // 1-based
        ID       string `json:"id"`       // Supercine internal ID
        Title    string `json:"title"`    // PT-BR when available, else original
        Date     string `json:"date"`     // human-readable date in PT-BR
        Backdrop string `json:"backdrop"` // TMDB backdrop URL
}

// rawSeason is the on-wire shape returned by the Supercine API. The API
// returns season and episode numbers as strings ("1", "2", ...) so we
// need a separate type to parse them, then convert to Season/Episode
// with proper int fields.
type rawSeason struct {
        Season   string       `json:"season"`
        ID       string       `json:"id"`
        Episodes []rawEpisode `json:"episodes"`
}

type rawEpisode struct {
        Title    string `json:"title"`
        ID       string `json:"id"`
        Date     string `json:"date"`
        Ep       string `json:"ep"`
        Backdrop string `json:"backdrop"`
}

// SeasonsResponse is the JSON returned by ?what=seasons.
type SeasonsResponse struct {
        Status      string   `json:"status"`
        SeasonCount int      `json:"seasonCount"`
        Seasons     []Season `json:"seasons"`
}

// rawSeasonsResponse is the on-wire shape that we parse then convert.
type rawSeasonsResponse struct {
        Status      string      `json:"status"`
        SeasonCount int         `json:"seasonCount"`
        Seasons     []rawSeason `json:"seasons"`
}

// Player is one playable source returned by ?what=player.
type Player struct {
        Title string `json:"title"` // e.g. "Player Principal"
        Lang  string `json:"lang"`  // "dublado" or "legendado"
        URL   string `json:"url"`   // encrypted data-server blob
        Type  string `json:"type"`  // hoster name: "mixdrop", "streamwish", etc.
}

// PlayerResponse is the JSON returned by ?what=player.
type PlayerResponse struct {
        Status  string   `json:"status"`
        Title   string   `json:"title"`
        TMDB    int      `json:"tmdb"`
        Season  int      `json:"season"`
        Episode string   `json:"episode"`
        Players []Player `json:"players"`
}

var (
        backdropRe     = regexp.MustCompile(`background-image:\s*url\('([^']+)'\)`)
        ititleRe       = regexp.MustCompile(`<ititle>([^<]+)</ititle>`)
        serverRe       = regexp.MustCompile(`<server-selector[^>]*data-server="([^"]+)"[^>]*>[\s\S]*?</server-selector>`)
        serverNameRe   = regexp.MustCompile(`<b>([^<]+)</b>`)
        serverDescRe   = regexp.MustCompile(`<span>([^<]+)</span>`)
        redirectRe     = regexp.MustCompile(`window\.location\.href\s*=\s*"([^"]+)"`)
        tmdbRe         = regexp.MustCompile(`tmdb\s*=\s*["'](\d+)["']`)
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
        // Extract TMDB ID for TV shows (used later to call ?what=seasons).
        if m := tmdbRe.FindStringSubmatch(bodyStr); len(m) >= 2 {
                page.TMDBID = m[1]
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
//
// The Supercine embed-api returns HTML with HTML entities (e.g. "&amp;"
// instead of "&") inside the window.location.href string. We decode the
// common entities so the hoster extractor gets a clean URL. Without this
// decoding, MixDrop URLs with subtitle parameters
// (e.g. "...?sub1=...&amp;sub1_label=...") would be fetched as-is and
// MixDrop would return an empty page, causing the extractor to fail with
// "wurl não encontrado".
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
        // Decode HTML entities that the embed-api leaves in the URL.
        // The most common one is &amp; -> &, but we also handle &lt; and &gt;
        // for safety. We don't use a full HTML decoder because the URL may
        // legitimately contain characters that look like entities.
        decoded := m[1]
        decoded = strings.ReplaceAll(decoded, "&amp;", "&")
        decoded = strings.ReplaceAll(decoded, "&lt;", "<")
        decoded = strings.ReplaceAll(decoded, "&gt;", ">")
        decoded = strings.ReplaceAll(decoded, "&quot;", "\"")
        decoded = strings.ReplaceAll(decoded, "&#39;", "'")
        return decoded, nil
}

// verifyURL checks that the video URL is accessible.
//
// We use a GET request with a small Range header (0-1) instead of HEAD
// because many hoster CDNs (notably MixDrop's mxcontent.net and VidHide's
// CDN) reject HEAD requests with 403/502, even though the same URL works
// fine in a browser. A ranged GET mimics what hls.js / <video> do and is
// accepted by every CDN we tested.
//
// MixDrop's CDN additionally requires an `Origin` and `Referer` header
// matching the hoster page (https://mixdrop.ps) — without them, every
// request returns 403. We set Origin and Referer based on the hoster the
// URL came from so the CDN accepts the verification request.
//
// On network errors we return true (be lenient) because the URL may still
// be reachable from the user's network — we'd rather return a URL and
// let the player retry than filter out a URL that the user could
// actually play.
func (p *SupercineProvider) verifyURL(ctx context.Context, videoURL string) bool {
        req, err := http.NewRequestWithContext(ctx, http.MethodGet, videoURL, nil)
        if err != nil {
                return false
        }
        req.Header.Set("User-Agent", p.cfg.UserAgent)
        req.Header.Set("Range", "bytes=0-1")
        req.Header.Set("Accept", "*/*")
        req.Header.Set("Accept-Language", "pt-BR,pt;q=0.9,en;q=0.8")
        // Some hoster CDNs (notably MixDrop's mxcontent.net) require an
        // Origin and Referer header that matches the hoster page. Without
        // them, every request returns 403 even though the URL is valid.
        // We set the Origin and Referer based on the URL's host so the CDN
        // accepts the request.
        if hosterOrigin := originForURL(videoURL); hosterOrigin != "" {
                req.Header.Set("Origin", hosterOrigin)
                req.Header.Set("Referer", hosterOrigin+"/")
        } else {
                // Default to the Supercine referer for unknown CDNs.
                req.Header.Set("Referer", "https://supercine-tv.net/")
        }
        client := &http.Client{
                Timeout:       6 * time.Second,
                CheckRedirect: func(req *http.Request, via []*http.Request) error { return nil },
        }
        resp, err := client.Do(req)
        if err != nil {
                // Network error — be lenient and assume the URL is OK.
                // The browser may still be able to reach it from the user's network.
                return true
        }
        defer resp.Body.Close()
        // Drain the small body so the connection can be reused.
        _, _ = io.Copy(io.Discard, resp.Body)
        // 2xx, 3xx, and even 416 (Range Not Satisfiable, returned by some
        // CDNs when the file is smaller than the range) all indicate the URL
        // is reachable. Only treat 4xx (except 416) and 5xx as failures.
        if resp.StatusCode == http.StatusRequestedRangeNotSatisfiable {
                return true
        }
        return resp.StatusCode >= 200 && resp.StatusCode < 400
}

// originForURL returns the Origin header value to use when verifying a
// video URL. Many hoster CDNs require an Origin and Referer that matches
// the hoster page (e.g. https://mixdrop.ps for mxcontent.net URLs). We
// infer the origin from the video URL's host by mapping known CDN hosts
// back to their hoster page.
func originForURL(videoURL string) string {
        low := strings.ToLower(videoURL)
        switch {
        case strings.Contains(low, "mxcontent.net"):
                // MixDrop CDN
                return "https://mixdrop.ps"
        case strings.Contains(low, "cdn-centaurus.com"):
                // StreamWish / VidHide CDN (shared)
                return "https://tln-hg.top"
        case strings.Contains(low, "premilkyway.com"):
                // StreamWish CDN
                return "https://tln-hg.top"
        case strings.Contains(low, "dramiyos-cdn.com"):
                // VidHide CDN
                return "https://tln-earn.top"
        }
        return ""
}

// Resolve implements provider.Provider.
//
// For movies (embedType="movies"):
//   1. Fetch the embed page → get list of servers.
//   2. Reorder servers so reliable hosters (StreamWish, FileMoon, Voe,
//      MixDrop, FileLions, VidHide) are tried before unreliable ones
//      (DoodStream now requires a Cloudflare Turnstile CAPTCHA, StreamTape
//      frequently returns "Video not found").
//   3. For each server, resolve the hoster URL and run the extractor.
//   4. Verify the extracted video URL is accessible via a ranged GET.
//
// We try ALL servers (not just the first 3) because the upstream frequently
// returns 6+ servers and the first few may all be DoodStream (which fails
// the CAPTCHA check) or StreamTape (which may have "Video not found").
// Stopping at 3 attempts means the user sees "no video" even when later
// servers would resolve fine.
//
// For TV shows (embedType="tvshows"):
//   This method returns an error immediately because series require a
//   specific season+episode to be resolved. The caller should use
//   ResolveEpisode() instead.
func (p *SupercineProvider) Resolve(ctx context.Context, imdbID, embedType string) (*provider.ResolveResult, error) {
        // TV shows require season+episode — refuse to resolve blindly.
        if embedType == "tvshows" {
                return nil, fmt.Errorf("séries requerem season+episode — use ResolveEpisode(imdb, season, episode) em vez de Resolve()")
        }

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

        // Build a list of (originalIndex, server) pairs and reorder them so
        // the most reliable hosters are tried first. We don't drop any server
        // — we just change the order. The result.Servers slice keeps the
        // original order so the UI's server buttons stay stable.
        embedURL := p.cfg.EmbedBase + "?imdb=" + url.QueryEscape(imdbID) + "&type=" + url.QueryEscape(embedType)
        type attempt struct {
                origIdx int
                srv     EmbedServer
        }
        attempts := make([]attempt, len(page.Servers))
        for i, s := range page.Servers {
                attempts[i] = attempt{origIdx: i, srv: s}
        }
        // First pass: probe each server's hoster URL in parallel (capped) so
        // we know which hoster each one resolves to. This lets us sort by
        // hoster reliability without paying the latency cost of sequential
        // resolution.
        type probeResult struct {
                origIdx   int
                hosterURL string
                hoster    string
                err       error
        }
        probeCh := make(chan probeResult, len(attempts))
        sem := make(chan struct{}, 4) // 4 concurrent probes
        var wg sync.WaitGroup
        for _, a := range attempts {
                wg.Add(1)
                go func(a attempt) {
                        defer wg.Done()
                        sem <- struct{}{}
                        defer func() { <-sem }()
                        hu, err := p.resolveHosterURL(ctx, a.srv.Server, embedURL)
                        if err != nil {
                                probeCh <- probeResult{origIdx: a.origIdx, err: err}
                                return
                        }
                        // Identify the hoster from the URL so we can prioritize.
                        probeCh <- probeResult{origIdx: a.origIdx, hosterURL: hu, hoster: hosterFromURL(hu)}
                }(a)
        }
        wg.Wait()
        close(probeCh)

        // Collect probe results, keyed by original index.
        probes := make(map[int]probeResult, len(attempts))
        for pr := range probeCh {
                probes[pr.origIdx] = pr
        }

        // Order attempts by hoster priority. Lower priority value = tried first.
        order := make([]int, 0, len(attempts))
        for _, a := range attempts {
                order = append(order, a.origIdx)
        }
        sort.SliceStable(order, func(i, j int) bool {
                pi, pj := probes[order[i]], probes[order[j]]
                return hosterPriority(pi.hoster) < hosterPriority(pj.hoster)
        })

        // Try each server in priority order. We try ALL servers (no longer
        // capped at 3) because the first few may all be DoodStream (CAPTCHA)
        // or StreamTape (Video not found), and the working one is further down.
        var lastErr error
        for _, origIdx := range order {
                pr := probes[origIdx]
                if pr.err != nil {
                        lastErr = pr.err
                        continue
                }
                ext, err := p.registry.Dispatch(ctx, pr.hosterURL)
                if err != nil || ext == nil || len(ext.Videos) == 0 {
                        if err != nil {
                                lastErr = err
                        } else {
                                lastErr = fmt.Errorf("supercine: extractor %s returned no videos for server %d", pr.hoster, origIdx)
                        }
                        continue
                }
                // Verify the extracted video URL is accessible before returning.
                // We use a ranged GET (see verifyURL docs) which works around
                // CDNs that reject HEAD requests.
                verified := make([]provider.VideoURL, 0, len(ext.Videos))
                for _, v := range ext.Videos {
                        if p.verifyURL(ctx, v.URL) {
                                verified = append(verified, provider.VideoURL{
                                        URL:     v.URL,
                                        Quality: v.Quality,
                                })
                        }
                }
                if len(verified) == 0 {
                        lastErr = fmt.Errorf("supercine: extracted video URL for server %d (%s) is not accessible", origIdx, pr.hoster)
                        continue
                }
                // Success — copy verified video URLs.
                result.Videos = verified
                // Tag the server that worked.
                result.Servers[origIdx].Description = fmt.Sprintf("[OK] %s", result.Servers[origIdx].Description)
                return result, nil
        }

        if lastErr != nil {
                return result, lastErr
        }
        return result, provider.ErrUnavailable
}

// ===== TV series methods =====

// FetchSeasons returns all seasons and episodes for a TV show.
//
// It first fetches the embed page to extract the TMDB ID, then calls
// /wp-json/api/tvshows?what=seasons&tmdb=<TMDB_ID> to get the JSON.
func (p *SupercineProvider) FetchSeasons(ctx context.Context, imdbID string) (*SeasonsResponse, error) {
        // Step 1: get the TMDB ID from the embed page.
        page, err := p.FetchEmbed(ctx, imdbID, "tvshows")
        if err != nil {
                return nil, err
        }
        if page.TMDBID == "" {
                return nil, fmt.Errorf("supercine: TMDB ID não encontrado no embed de %s", imdbID)
        }
        return p.fetchSeasonsByTMDB(ctx, page.TMDBID)
}

// fetchSeasonsByTMDB calls ?what=seasons directly with a TMDB ID.
func (p *SupercineProvider) fetchSeasonsByTMDB(ctx context.Context, tmdbID string) (*SeasonsResponse, error) {
        target := p.cfg.APIBase + "/tvshows?what=seasons&tmdb=" + url.QueryEscape(tmdbID) + "&version=1.0&origin=web"
        req, _ := http.NewRequestWithContext(ctx, http.MethodGet, target, nil)
        req.Header.Set("User-Agent", p.cfg.UserAgent)
        req.Header.Set("Referer", "https://supercine-tv.net/")
        req.Header.Set("Accept", "application/json")
        resp, err := p.http.Do(req)
        if err != nil {
                return nil, err
        }
        defer resp.Body.Close()
        body, _ := io.ReadAll(resp.Body)

        // The API returns season/episode numbers as strings, so we parse
        // into rawSeasonsResponse and convert to typed SeasonsResponse.
        var raw rawSeasonsResponse
        if err := json.Unmarshal(body, &raw); err != nil {
                return nil, fmt.Errorf("supercine: failed to decode seasons response: %w", err)
        }
        if raw.Status != "success" {
                return nil, fmt.Errorf("supercine: seasons request failed: %s", raw.Status)
        }

        sr := &SeasonsResponse{
                Status:      raw.Status,
                SeasonCount: raw.SeasonCount,
                Seasons:     make([]Season, 0, len(raw.Seasons)),
        }
        for _, rs := range raw.Seasons {
                s := Season{
                        Number:   atoiSafe(rs.Season),
                        ID:       rs.ID,
                        Episodes: make([]Episode, 0, len(rs.Episodes)),
                }
                for _, re := range rs.Episodes {
                        s.Episodes = append(s.Episodes, Episode{
                                Number:   atoiSafe(re.Ep),
                                ID:       re.ID,
                                Title:    re.Title,
                                Date:     re.Date,
                                Backdrop: re.Backdrop,
                        })
                }
                sr.Seasons = append(sr.Seasons, s)
        }
        return sr, nil
}

// atoiSafe parses a string to int, returning 0 on error.
func atoiSafe(s string) int {
        n, _ := strconv.Atoi(strings.TrimSpace(s))
        return n
}

// FetchEpisodePlayers returns the list of available players for a
// specific episode. Each player has a `url` (encrypted blob) and a
// `type` (hoster name).
//
// The TMDB ID is required. If you only have the IMDB ID, call
// FetchEmbed first to extract it.
func (p *SupercineProvider) FetchEpisodePlayers(ctx context.Context, tmdbID string, season, episode int) (*PlayerResponse, error) {
        target := fmt.Sprintf("%s/tvshows?what=player&tmdb=%s&season=%d&episode=%d&version=1.0&origin=web",
                p.cfg.APIBase, url.QueryEscape(tmdbID), season, episode)
        req, _ := http.NewRequestWithContext(ctx, http.MethodGet, target, nil)
        req.Header.Set("User-Agent", p.cfg.UserAgent)
        req.Header.Set("Referer", "https://supercine-tv.net/")
        req.Header.Set("Accept", "application/json")
        resp, err := p.http.Do(req)
        if err != nil {
                return nil, err
        }
        defer resp.Body.Close()
        body, _ := io.ReadAll(resp.Body)

        var pr PlayerResponse
        if err := json.Unmarshal(body, &pr); err != nil {
                return nil, fmt.Errorf("supercine: failed to decode player response: %w", err)
        }
        if pr.Status != "success" {
                return nil, fmt.Errorf("supercine: player request failed: %s", pr.Status)
        }
        return &pr, nil
}

// ResolveEpisode resolves a specific episode to a direct video URL.
//
// Flow:
//   1. Fetch the embed page to get the TMDB ID.
//   2. Call ?what=player&tmdb=X&season=Y&episode=Z to get the player list.
//   3. Reorder players so reliable hosters (StreamWish, FileMoon, Voe,
//      MixDrop, FileLions, VidHide) are tried before unreliable ones
//      (DoodStream requires CAPTCHA, StreamTape often returns 404).
//   4. For each player, resolve the hoster URL via
//      /embed-api/?action=embed&url=<encrypted>, then run the hoster
//      extractor to get the direct mp4/m3u8.
//   5. Verify the extracted video URL is accessible via a ranged GET.
//
// We try ALL players (not just the first 3) because for many episodes the
// first few players are DoodStream (which fails the CAPTCHA check) or
// StreamTape (which may have "Video not found"), and the working player
// is further down the list.
func (p *SupercineProvider) ResolveEpisode(ctx context.Context, imdbID string, season, episode int) (*provider.ResolveResult, error) {
        // Step 1: get TMDB ID
        page, err := p.FetchEmbed(ctx, imdbID, "tvshows")
        if err != nil {
                return nil, fmt.Errorf("%w: %v", provider.ErrProviderDown, err)
        }
        if page.TMDBID == "" {
                return nil, provider.ErrUnavailable
        }

        // Step 2: get players for this episode
        pr, err := p.FetchEpisodePlayers(ctx, page.TMDBID, season, episode)
        if err != nil {
                return nil, err
        }
        if len(pr.Players) == 0 {
                return nil, provider.ErrUnavailable
        }

        // Build result skeleton
        result := &provider.ResolveResult{
                Provider: p.Name(),
                IMDB:     imdbID,
                Type:     "tvshows",
        }
        for i, pl := range pr.Players {
                // Deduplicate similar player titles by appending lang
                name := pl.Title
                if pl.Lang != "" {
                        name = fmt.Sprintf("%s (%s)", pl.Title, pl.Lang)
                }
                result.Servers = append(result.Servers, provider.Server{
                        Index:       i,
                        Name:        name,
                        Description: pl.Type,
                })
        }

        // Step 3: order players by hoster priority (lower priority value =
        // tried first). The Supercine API returns a `type` field naming the
        // hoster ("mixdrop", "streamwish", "vidhide", "doodstream", ...) so
        // we can sort without making any HTTP requests.
        order := make([]int, len(pr.Players))
        for i := range pr.Players {
                order[i] = i
        }
        sort.SliceStable(order, func(i, j int) bool {
                return hosterPriority(pr.Players[order[i]].Type) < hosterPriority(pr.Players[order[j]].Type)
        })

        // Step 4: try each player in priority order. We try ALL players
        // because the first few may all be DoodStream (CAPTCHA) or
        // StreamTape (Video not found).
        embedURL := p.cfg.EmbedBase + "?imdb=" + url.QueryEscape(imdbID) + "&type=tvshows"
        var lastErr error
        for _, origIdx := range order {
                pl := pr.Players[origIdx]
                hosterURL, err := p.resolveHosterURL(ctx, pl.URL, embedURL)
                if err != nil {
                        lastErr = err
                        continue
                }
                ext, err := p.registry.Dispatch(ctx, hosterURL)
                if err != nil || ext == nil || len(ext.Videos) == 0 {
                        if err != nil {
                                lastErr = err
                        } else {
                                lastErr = fmt.Errorf("supercine: extractor %s returned no videos for player %d", pl.Type, origIdx)
                        }
                        continue
                }
                verified := make([]provider.VideoURL, 0, len(ext.Videos))
                for _, v := range ext.Videos {
                        if p.verifyURL(ctx, v.URL) {
                                verified = append(verified, provider.VideoURL{
                                        URL:     v.URL,
                                        Quality: v.Quality,
                                })
                        }
                }
                if len(verified) == 0 {
                        lastErr = fmt.Errorf("supercine: extracted video URL for player %d (%s) is not accessible", origIdx, pl.Type)
                        continue
                }
                result.Videos = verified
                result.Servers[origIdx].Description = fmt.Sprintf("[OK] %s", result.Servers[origIdx].Description)
                return result, nil
        }

        if lastErr != nil {
                return result, lastErr
        }
        return result, provider.ErrUnavailable
}

// hosterFromURL inspects a hoster URL and returns the canonical hoster
// name (e.g. "mixdrop", "streamwish", "doodstream"). Used by Resolve()
// to sort servers by hoster reliability before trying them.
func hosterFromURL(u string) string {
        low := strings.ToLower(u)
        switch {
        case strings.Contains(low, "mixdrop"):
                return "mixdrop"
        case strings.Contains(low, "streamwish") || strings.Contains(low, "asnwish") ||
                strings.Contains(low, "tlnwish") || strings.Contains(low, "playerwish") ||
                strings.Contains(low, "tln-hg"):
                return "streamwish"
        case strings.Contains(low, "vidhide") || strings.Contains(low, "vidhidevip") ||
                strings.Contains(low, "tlnhide") || strings.Contains(low, "megahide") ||
                strings.Contains(low, "niikaplayerr") || strings.Contains(low, "tln-earn"):
                return "vidhide"
        case strings.Contains(low, "filemoon") || strings.Contains(low, "96ar") ||
                strings.Contains(low, "tlnmoons"):
                return "filemoon"
        case strings.Contains(low, "filelions"):
                return "filelions"
        case strings.Contains(low, "streamtape") || strings.Contains(low, "streamadblockplus") ||
                strings.Contains(low, "stapewithadblock") || strings.Contains(low, "shavetape") ||
                strings.Contains(low, "tapenoads") || strings.Contains(low, "tapeantiads"):
                return "streamtape"
        case strings.Contains(low, "voe") || strings.Contains(low, "donaldlineelse") ||
                strings.Contains(low, "jamessoundcost"):
                return "voe"
        case strings.Contains(low, "doodstream") || strings.Contains(low, "dood.") ||
                strings.Contains(low, "vidply") || strings.Contains(low, "do7go"):
                return "doodstream"
        }
        return "unknown"
}

// hosterPriority returns a priority value for a hoster name. Lower value =
// tried first. The order is based on empirical reliability:
//
//   - StreamWish, FileMoon, Voe: HLS-based hosters that work reliably and
//     return accessible URLs. Tried first.
//   - MixDrop: returns direct mp4 URLs that work in browsers but the CDN
//     rejects HEAD requests (we use ranged GET to work around this).
//   - FileLions, VidHide: similar to FileMoon/MixDrop but less reliable.
//   - StreamTape: frequently returns "Video not found" for older content.
//   - DoodStream: now requires a Cloudflare Turnstile CAPTCHA, so the
//     extractor cannot resolve without a real browser. Tried last so we
//     don't waste time on it when other hosters are available.
func hosterPriority(hoster string) int {
        switch strings.ToLower(hoster) {
        case "streamwish":
                return 1
        case "filemoon":
                return 2
        case "voe":
                return 3
        case "mixdrop":
                return 4
        case "filelions":
                return 5
        case "vidhide":
                return 6
        case "streamtape":
                return 7
        case "doodstream":
                return 8
        default:
                return 9
        }
}

// Helpers for parsing strings (kept for parity with the original code).
var _ = strconv.Atoi
var _ = strings.TrimSpace

// ===== Home / discovery =====

// HomeCategory is one of the named sections shown on the app home screen.
// The Supercine app has hardcoded labels matching these strings.
type HomeCategory string

const (
        CategoryLancamentos HomeCategory = "lancamentos" // "Lançamentos" — new releases
        CategoryDestaques   HomeCategory = "destaques"   // "Destaques" — featured
        CategoryRecentes    HomeCategory = "recentes"    // "Recentes" — recently added
        CategorySugeridos   HomeCategory = "sugeridos"   // "Sugeridos" — recommended
        CategoryRecomendados HomeCategory = "recomendados" // alias for sugeridos
)

// HomeItem is one title returned by the home endpoint. The shape is the
// on-wire response from /api/<type>?what=<category>&version=1.0&origin=web.
type HomeItem struct {
        Type         string         `json:"type"`          // "movies" or "tvshows"
        PostID       string         `json:"post_id"`       // Supercine internal ID
        Title        string         `json:"title"`         // PT-BR title
        Category     []HomeCategory `json:"category"`      // [{name: "Crime"}, ...]
        IMDB         string         `json:"imdb"`          // IMDB ID
        Poster       string         `json:"poster"`        // TMDB poster URL
        BackdropPath string         `json:"backdrop_path"` // TMDB backdrop URL
        IMDBRating   float64        `json:"imdbRating"`    // IMDB rating
        Year         int            `json:"year"`          // release year
        Runtime      string         `json:"runtime"`       // human-readable runtime
}

// rawHomeItem is the on-wire shape (Category comes as []struct{Name string}).
type rawHomeItem struct {
        Type         string `json:"type"`
        PostID       string `json:"post_id"`
        Title        string `json:"title"`
        Category     []struct {
                Name string `json:"name"`
        } `json:"category"`
        IMDB         string `json:"imdb"`
        Poster       string `json:"poster"`
        BackdropPath string `json:"backdrop_path"`
        IMDBRating   string `json:"imdbRating"`
        Year         string `json:"year"` // comes as string ("2026"), parse with atoiSafe
        Runtime      string `json:"runtime"`
}

// HomeResponse is the JSON returned by /api/<type>?what=<category>.
type HomeResponse struct {
        Status string     `json:"status"`
        Data   []HomeItem `json:"data"`
}

// rawHomeResponse is the on-wire shape that we parse then convert.
type rawHomeResponse struct {
        Status string         `json:"status"`
        Data   []rawHomeItem  `json:"data"`
}

// FetchHome returns the list of titles for a given (type, category) combo.
//
//   type:     "movies" or "tvshows"
//   category: "lancamentos", "destaques", "recentes", "sugeridos", "recomendados"
//
// This is what the Supercine app shows on the home screen under each row.
// We call /wp-json/api/<type>?what=<category>&version=1.0&origin=web and
// normalize the response.
func (p *SupercineProvider) FetchHome(ctx context.Context, embedType string, category HomeCategory) (*HomeResponse, error) {
        if embedType != "movies" && embedType != "tvshows" {
                return nil, fmt.Errorf("supercine: tipo inválido %q (esperado movies ou tvshows)", embedType)
        }
        if category == "" {
                category = CategoryLancamentos
        }

        target := fmt.Sprintf("%s/%s?what=%s&version=1.0&origin=web",
                p.cfg.APIBase, embedType, url.QueryEscape(string(category)))
        req, _ := http.NewRequestWithContext(ctx, http.MethodGet, target, nil)
        req.Header.Set("User-Agent", p.cfg.UserAgent)
        req.Header.Set("Referer", "https://supercine-tv.net/")
        req.Header.Set("Accept", "application/json")
        resp, err := p.http.Do(req)
        if err != nil {
                return nil, err
        }
        defer resp.Body.Close()
        body, _ := io.ReadAll(resp.Body)

        var raw rawHomeResponse
        if err := json.Unmarshal(body, &raw); err != nil {
                return nil, fmt.Errorf("supercine: failed to decode home response: %w", err)
        }
        if raw.Status != "success" {
                return nil, fmt.Errorf("supercine: home request failed: %s", raw.Status)
        }

        hr := &HomeResponse{
                Status: raw.Status,
                Data:   make([]HomeItem, 0, len(raw.Data)),
        }
        for _, ri := range raw.Data {
                item := HomeItem{
                        Type:         ri.Type,
                        PostID:       ri.PostID,
                        Title:        ri.Title,
                        IMDB:         ri.IMDB,
                        Poster:       ri.Poster,
                        BackdropPath: ri.BackdropPath,
                        IMDBRating:   atofSafe(ri.IMDBRating),
                        Year:         atoiSafe(ri.Year),
                        Runtime:      ri.Runtime,
                }
                for _, c := range ri.Category {
                        item.Category = append(item.Category, HomeCategory(c.Name))
                }
                hr.Data = append(hr.Data, item)
        }
        return hr, nil
}

// atofSafe parses a string to float64, returning 0 on error.
func atofSafe(s string) float64 {
        f, _ := strconv.ParseFloat(strings.TrimSpace(s), 64)
        return f
}

// FetchAllHome returns all 4 home categories (lancamentos, destaques,
// recentes, sugeridos) for a given type, in parallel. Useful for
// populating the home screen of the UI in one call.
//
// Returns a map keyed by category name (e.g. "lancamentos", "destaques",
// "recentes", "sugeridos").
func (p *SupercineProvider) FetchAllHome(ctx context.Context, embedType string) (map[HomeCategory][]HomeItem, error) {
        categories := []HomeCategory{CategoryLancamentos, CategoryDestaques, CategoryRecentes, CategorySugeridos}
        out := make(map[HomeCategory][]HomeItem, len(categories))
        var mu sync.Mutex
        var wg sync.WaitGroup
        errs := make([]error, len(categories))

        for i, cat := range categories {
                wg.Add(1)
                go func(idx int, c HomeCategory) {
                        defer wg.Done()
                        hr, err := p.FetchHome(ctx, embedType, c)
                        mu.Lock()
                        defer mu.Unlock()
                        if err != nil {
                                errs[idx] = err
                                return
                        }
                        out[c] = hr.Data
                }(i, cat)
        }
        wg.Wait()

        // Return the first non-nil error, if any.
        for _, e := range errs {
                if e != nil {
                        return out, e
                }
        }
        return out, nil
}
