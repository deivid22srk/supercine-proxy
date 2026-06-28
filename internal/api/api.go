// Package api implements the REST endpoints exposed by the proxy on top of
// the raw reverse proxy. These endpoints wrap upstream calls (e.g. /api/filmes)
// and add new capabilities (e.g. /v1/extract) that combine the upstream
// embed-api with the hoster extractors.
package api

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
        "github.com/deivid22srk/supercine-proxy/internal/config"
        "github.com/deivid22srk/supercine-proxy/internal/extractors"
        "github.com/deivid22srk/supercine-proxy/internal/logger"
        "github.com/deivid22srk/supercine-proxy/internal/proxy"
        "github.com/deivid22srk/supercine-proxy/internal/types"
)

// API exposes the proxy's REST endpoints under /v1.
type API struct {
        cfg      *config.Config
        log      *logger.Logger
        proxy    *proxy.Server
        registry *extractors.Registry
        client   *http.Client
}

// New constructs an API instance.
func New(cfg *config.Config, log *logger.Logger, p *proxy.Server, reg *extractors.Registry) *API {
        return &API{
                cfg:      cfg,
                log:      log,
                proxy:    p,
                registry: reg,
                client: &http.Client{
                        Timeout: cfg.RequestTimeout,
                },
        }
}

// Handler returns the http.Handler mounted at /v1.
func (a *API) Handler() http.Handler {
        mux := http.NewServeMux()
        mux.HandleFunc("/v1/health", a.handleHealth)
        mux.HandleFunc("/v1/stats", a.handleStats)
        mux.HandleFunc("/v1/logs", a.handleLogs)
        mux.HandleFunc("/v1/cache/clear", a.handleCacheClear)
        mux.HandleFunc("/v1/logs/clear", a.handleLogsClear)

        // Upstream wrappers
        mux.HandleFunc("/v1/api/", a.handleUpstreamAPI)
        mux.HandleFunc("/v1/auth/plans", a.handleAuthPlans)

        // Embed + extractor
        mux.HandleFunc("/v1/embed", a.handleEmbed)
        mux.HandleFunc("/v1/extract", a.handleExtract)
        mux.HandleFunc("/v1/extractors", a.handleListExtractors)

        // Discovery: list all upstream routes
        mux.HandleFunc("/v1/routes", a.handleRoutes)

        return a.adminGuard(mux)
}

// adminGuard protects mutating endpoints behind ADMIN_TOKEN if set.
// Read endpoints (GET /v1/logs, /v1/stats, etc) are always public.
func (a *API) adminGuard(h http.Handler) http.Handler {
        return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
                if a.cfg.AdminToken == "" {
                        h.ServeHTTP(w, r)
                        return
                }
                // Allow GET/OPTIONS without token.
                if r.Method == http.MethodGet || r.Method == http.MethodOptions {
                        h.ServeHTTP(w, r)
                        return
                }
                // Allow /v1/extract and /v1/embed as POST without admin (they're
                // read operations logically).
                if r.Method == http.MethodPost && (strings.HasSuffix(r.URL.Path, "/extract") || strings.HasSuffix(r.URL.Path, "/embed")) {
                        h.ServeHTTP(w, r)
                        return
                }
                // All other mutating endpoints require the token.
                provided := r.Header.Get("X-Admin-Token")
                if provided == "" {
                        provided = r.URL.Query().Get("admin_token")
                }
                if provided != a.cfg.AdminToken {
                        writeJSON(w, http.StatusUnauthorized, map[string]any{
                                "error": "X-Admin-Token header required",
                        })
                        return
                }
                h.ServeHTTP(w, r)
        })
}

// ---- health / stats / logs ----

func (a *API) handleHealth(w http.ResponseWriter, r *http.Request) {
        writeJSON(w, http.StatusOK, map[string]any{
                "status":   "ok",
                "version":  "1.0.0",
                "upstream": a.cfg.UpstreamBase,
                "cache":    a.log.Stats().CacheHits + a.log.Stats().CacheMisses,
        })
}

func (a *API) handleStats(w http.ResponseWriter, r *http.Request) {
        writeJSON(w, http.StatusOK, a.log.Stats())
}

func (a *API) handleLogs(w http.ResponseWriter, r *http.Request) {
        limit := 100
        if l := r.URL.Query().Get("limit"); l != "" {
                var n int
                _, _ = fmt.Sscanf(l, "%d", &n)
                if n > 0 && n <= 500 {
                        limit = n
                }
        }
        writeJSON(w, http.StatusOK, a.log.Entries(limit))
}

func (a *API) handleCacheClear(w http.ResponseWriter, r *http.Request) {
        if r.Method != http.MethodPost {
                writeJSON(w, http.StatusMethodNotAllowed, map[string]string{"error": "POST required"})
                return
        }
        // Note: cache instance is shared via container — caller will pass through.
        writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

func (a *API) handleLogsClear(w http.ResponseWriter, r *http.Request) {
        if r.Method != http.MethodPost {
                writeJSON(w, http.StatusMethodNotAllowed, map[string]string{"error": "POST required"})
                return
        }
        a.log.Reset()
        writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

// ---- upstream wrappers ----

// handleUpstreamAPI proxies /v1/api/<type> to the upstream /wp-json/api/<type>.
// Example: GET /v1/api/filmes          -> /wp-json/api/filmes
//          GET /v1/api/tvshows?imdb=tt10919420 -> /wp-json/api/tvshows?imdb=...
//
// We always pass through the raw upstream response (including the
// "status: update" force-upgrade JSON) so clients can react to it.
func (a *API) handleUpstreamAPI(w http.ResponseWriter, r *http.Request) {
        ctx, cancel := context.WithTimeout(r.Context(), 25*time.Second)
        defer cancel()

        // path under /v1/api/
        sub := strings.TrimPrefix(r.URL.Path, "/v1/api/")
        if sub == "" {
                writeJSON(w, http.StatusBadRequest, map[string]string{
                        "error": "missing type. Try /v1/api/filmes or /v1/api/tvshows",
                })
                return
        }

        resp, body, err := a.proxy.ForwardGet(ctx, "/api/"+sub, r.URL.Query())
        if err != nil {
                writeJSON(w, http.StatusBadGateway, map[string]string{"error": err.Error()})
                return
        }
        w.Header().Set("Content-Type", "application/json")
        w.WriteHeader(resp.StatusCode)
        _, _ = w.Write(body)
}

// handleAuthPlans is a thin wrapper around POST /auth/plans since the upstream
// is a public endpoint and useful to demo in the dashboard.
func (a *API) handleAuthPlans(w http.ResponseWriter, r *http.Request) {
        ctx, cancel := context.WithTimeout(r.Context(), 15*time.Second)
        defer cancel()

        target := a.cfg.UpstreamBase + "/auth/plans"
        req, _ := http.NewRequestWithContext(ctx, http.MethodPost, target, strings.NewReader("{}"))
        req.Header.Set("User-Agent", a.cfg.UserAgent)
        req.Header.Set("Content-Type", "application/json")
        req.Header.Set("Referer", "https://supercine-tv.net/")
        resp, err := a.client.Do(req)
        if err != nil {
                writeJSON(w, http.StatusBadGateway, map[string]string{"error": err.Error()})
                return
        }
        defer resp.Body.Close()
        body, _ := io.ReadAll(resp.Body)

        var parsed struct {
                Success bool          `json:"success"`
                Plans   []types.Plan  `json:"plans"`
        }
        if err := json.Unmarshal(body, &parsed); err != nil {
                w.Header().Set("Content-Type", "application/json")
                w.WriteHeader(resp.StatusCode)
                _, _ = w.Write(body)
                return
        }
        writeJSON(w, http.StatusOK, parsed)
}

// ---- embed ----

// handleEmbed fetches the embed-api HTML page for a given imdb/type and
// extracts the <server-selector> entries. Each entry's `server` field is the
// encrypted blob the upstream uses to identify a hoster.
//
// GET /v1/embed?imdb=tt2250912&type=movies
// GET /v1/embed?imdb=tt10919420&type=tvshows
func (a *API) handleEmbed(w http.ResponseWriter, r *http.Request) {
        ctx, cancel := context.WithTimeout(r.Context(), 20*time.Second)
        defer cancel()

        imdb := r.URL.Query().Get("imdb")
        typ := r.URL.Query().Get("type")
        if imdb == "" || typ == "" {
                writeJSON(w, http.StatusBadRequest, map[string]string{
                        "error": "imdb and type are required (type = movies | tvshows)",
                })
                return
        }
        if typ != "movies" && typ != "tvshows" {
                writeJSON(w, http.StatusBadRequest, map[string]string{
                        "error": "type must be 'movies' or 'tvshows'",
                })
                return
        }

        target := a.cfg.EmbedBase + "?imdb=" + url.QueryEscape(imdb) + "&type=" + url.QueryEscape(typ)
        req, _ := http.NewRequestWithContext(ctx, http.MethodGet, target, nil)
        req.Header.Set("User-Agent", a.cfg.UserAgent)
        req.Header.Set("Referer", "https://supercine-tv.net/")
        resp, err := a.client.Do(req)
        if err != nil {
                writeJSON(w, http.StatusBadGateway, map[string]string{"error": err.Error()})
                return
        }
        defer resp.Body.Close()
        body, _ := io.ReadAll(resp.Body)

        page, err := parseEmbedPage(imdb, typ, body)
        if err != nil {
                writeJSON(w, http.StatusBadGateway, map[string]string{
                        "error":  "failed to parse embed page",
                        "detail": err.Error(),
                })
                return
        }
        writeJSON(w, http.StatusOK, page)
}

// parseEmbedPage extracts the list of <server-selector> elements from the
// embed-api HTML response.
func parseEmbedPage(imdb, typ string, body []byte) (*types.EmbedPage, error) {
        doc, err := goquery.NewDocumentFromReader(strings.NewReader(string(body)))
        if err != nil {
                return nil, err
        }
        page := &types.EmbedPage{
                IMDB: imdb,
                Type: typ,
        }
        doc.Find("server-selector").Each(func(i int, sel *goquery.Selection) {
                server, _ := sel.Attr("data-server")
                id, _ := sel.Attr("data-id")
                lang, _ := sel.Attr("data-lang")
                // Description from the inner <span> (e.g. "Velocidade ok e poucos anúncios")
                desc := sel.Find("span").Text()
                title := sel.Find("b").Text()
                if server == "" {
                        return
                }
                page.Servers = append(page.Servers, types.EmbedServer{
                        ID:          id,
                        Server:      server,
                        Lang:        lang,
                        Title:       title,
                        Description: strings.TrimSpace(desc),
                })
        })

        // Try to extract page title.
        if t := doc.Find("title").First().Text(); t != "" {
                page.Title = strings.TrimSpace(t)
        }
        return page, nil
}

// ---- extract ----

// handleExtract combines the embed-api and the hoster extractors into a
// single call. The client passes an imdb id and (optionally) a server index,
// and the proxy returns the direct mp4/m3u8 URL.
//
// GET  /v1/extract?imdb=tt2250912&type=movies
// GET  /v1/extract?imdb=tt2250912&type=movies&server=0
// POST /v1/extract  body: {"imdb":"tt...","type":"movies","server":0}
//
// If server is omitted the proxy returns the first server that successfully
// resolves to a direct video URL.
func (a *API) handleExtract(w http.ResponseWriter, r *http.Request) {
        ctx, cancel := context.WithTimeout(r.Context(), 45*time.Second)
        defer cancel()

        var params struct {
                IMDB   string `json:"imdb"`
                Type   string `json:"type"`
                Server int    `json:"server"`
        }
        switch r.Method {
        case http.MethodGet:
                params.IMDB = r.URL.Query().Get("imdb")
                params.Type = r.URL.Query().Get("type")
                if s := r.URL.Query().Get("server"); s != "" {
                        _, _ = fmt.Sscanf(s, "%d", &params.Server)
                }
        case http.MethodPost:
                if err := json.NewDecoder(r.Body).Decode(&params); err != nil {
                        writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid JSON body"})
                        return
                }
        default:
                writeJSON(w, http.StatusMethodNotAllowed, map[string]string{"error": "GET or POST required"})
                return
        }

        if params.IMDB == "" || params.Type == "" {
                writeJSON(w, http.StatusBadRequest, map[string]string{
                        "error": "imdb and type are required",
                })
                return
        }

        // Step 1: fetch the embed page.
        embedTarget := a.cfg.EmbedBase + "?imdb=" + url.QueryEscape(params.IMDB) + "&type=" + url.QueryEscape(params.Type)
        req, _ := http.NewRequestWithContext(ctx, http.MethodGet, embedTarget, nil)
        req.Header.Set("User-Agent", a.cfg.UserAgent)
        req.Header.Set("Referer", "https://supercine-tv.net/")
        embedResp, err := a.client.Do(req)
        if err != nil {
                writeJSON(w, http.StatusBadGateway, map[string]string{"error": err.Error()})
                return
        }
        defer embedResp.Body.Close()
        embedBody, _ := io.ReadAll(embedResp.Body)
        page, err := parseEmbedPage(params.IMDB, params.Type, embedBody)
        if err != nil || len(page.Servers) == 0 {
                writeJSON(w, http.StatusBadGateway, map[string]any{
                        "error":  "failed to parse embed page",
                        "detail": err.Error(),
                })
                return
        }

        // Step 2: pick the server index. If a specific index is requested and
        // in range, try only that one. Otherwise iterate until success.
        indices := []int{params.Server}
        if params.Server < 0 || params.Server >= len(page.Servers) {
                indices = nil
                for i := range page.Servers {
                        indices = append(indices, i)
                }
        }

        var lastErr string
        for _, idx := range indices {
                server := page.Servers[idx]
                // Step 3: GET /embed-api/?action=embed&url=<server.Server>
                resolveURL := a.cfg.EmbedBase + "?action=embed&url=" + url.QueryEscape(server.Server)
                req2, _ := http.NewRequestWithContext(ctx, http.MethodGet, resolveURL, nil)
                req2.Header.Set("User-Agent", a.cfg.UserAgent)
                req2.Header.Set("Referer", embedTarget)
                resp2, err := a.client.Do(req2)
                if err != nil {
                        lastErr = err.Error()
                        continue
                }
                body2, _ := io.ReadAll(resp2.Body)
                _ = resp2.Body.Close()

                // Extract the hoster URL from the JS redirect.
                hosterURL := extractRedirectFromEmbed(body2)
                if hosterURL == "" {
                        lastErr = fmt.Sprintf("no redirect found in action=embed response for server %d", idx)
                        continue
                }

                // Step 4: try to extract direct video URL via hoster scraper.
                ext, err := a.registry.Dispatch(ctx, hosterURL)
                a.log.IncExtractor(err != nil)
                if err != nil {
                        lastErr = err.Error()
                        continue
                }
                writeJSON(w, http.StatusOK, map[string]any{
                        "imdb":        params.IMDB,
                        "type":        params.Type,
                        "server":      idx,
                        "server_meta": server,
                        "hoster_url":  hosterURL,
                        "hoster":      ext.Hoster,
                        "videos":      ext.Videos,
                        "took":        ext.Took,
                })
                return
        }

        writeJSON(w, http.StatusBadGateway, map[string]any{
                "error":         "all servers failed to extract a direct video URL",
                "last_error":    lastErr,
                "server_count":  len(page.Servers),
        })
}

// extractRedirectFromEmbed pulls the URL out of:
//   window.location.href = "https://mixdrop.ps/e/abc123";
//
// The Supercine embed-api returns HTML with HTML entities (e.g. "&amp;"
// instead of "&") inside the URL string. We decode the common entities so
// the hoster extractor gets a clean URL. Without this, MixDrop URLs with
// subtitle parameters would be fetched as-is and MixDrop would return an
// empty page.
func extractRedirectFromEmbed(body []byte) string {
        re := regexp.MustCompile(`window\.location\.href\s*=\s*"([^"]+)"`)
        m := re.FindStringSubmatch(string(body))
        if len(m) < 2 {
                return ""
        }
        decoded := m[1]
        decoded = strings.ReplaceAll(decoded, "&amp;", "&")
        decoded = strings.ReplaceAll(decoded, "&lt;", "<")
        decoded = strings.ReplaceAll(decoded, "&gt;", ">")
        decoded = strings.ReplaceAll(decoded, "&quot;", "\"")
        decoded = strings.ReplaceAll(decoded, "&#39;", "'")
        return decoded
}

// ---- list extractors ----

func (a *API) handleListExtractors(w http.ResponseWriter, r *http.Request) {
        out := []map[string]string{}
        for _, e := range a.registry.All() {
                out = append(out, map[string]string{
                        "name": e.Name(),
                })
        }
        writeJSON(w, http.StatusOK, out)
}

// ---- routes discovery ----

// handleRoutes fetches /wp-json/ from the upstream and returns the list of
// routes in a compact form. Useful for the dashboard "API discovery" panel.
func (a *API) handleRoutes(w http.ResponseWriter, r *http.Request) {
        ctx, cancel := context.WithTimeout(r.Context(), 15*time.Second)
        defer cancel()

        resp, body, err := a.proxy.ForwardGet(ctx, "/", nil)
        if err != nil {
                writeJSON(w, http.StatusBadGateway, map[string]string{"error": err.Error()})
                return
        }
        var root struct {
                Name       string            `json:"name"`
                Namespaces []string          `json:"namespaces"`
                Routes     map[string]any    `json:"routes"`
        }
        if err := json.Unmarshal(body, &root); err != nil {
                w.Header().Set("Content-Type", "application/json")
                w.WriteHeader(resp.StatusCode)
                _, _ = w.Write(body)
                return
        }
        type routeInfo struct {
                Path    string   `json:"path"`
                Methods []string `json:"methods"`
        }
        out := []routeInfo{}
        for path, v := range root.Routes {
                m, _ := v.(map[string]any)
                methods := []string{}
                if ms, ok := m["methods"].([]any); ok {
                        for _, x := range ms {
                                if s, ok := x.(string); ok {
                                        methods = append(methods, s)
                                }
                        }
                }
                out = append(out, routeInfo{Path: path, Methods: methods})
        }
        writeJSON(w, http.StatusOK, map[string]any{
                "name":       root.Name,
                "namespaces": root.Namespaces,
                "routes":     out,
        })
}

// ---- helpers ----

func writeJSON(w http.ResponseWriter, status int, v any) {
        w.Header().Set("Content-Type", "application/json")
        w.WriteHeader(status)
        _ = json.NewEncoder(w).Encode(v)
}
