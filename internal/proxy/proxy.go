// Package proxy implements the core HTTP reverse-proxy that fronts the
// supercine-tv.net WordPress REST API. All upstream traffic flows through
// here so the dashboard can log, cache, and inspect it.
package proxy

import (
        "bytes"
        "context"
        "fmt"
        "io"
        "net/http"
        "net/url"
        "strings"
        "time"

        "github.com/deivid22srk/supercine-proxy/internal/cache"
        "github.com/deivid22srk/supercine-proxy/internal/config"
        "github.com/deivid22srk/supercine-proxy/internal/logger"
        "github.com/deivid22srk/supercine-proxy/internal/types"
)

// Server is the HTTP reverse proxy.
type Server struct {
        cfg    *config.Config
        cache  *cache.Cache
        log    *logger.Logger
        client *http.Client
}

// New constructs a proxy Server using the given configuration.
func New(cfg *config.Config, c *cache.Cache, l *logger.Logger) *Server {
        return &Server{
                cfg:   cfg,
                cache: c,
                log:   l,
                client: &http.Client{
                        Timeout: cfg.RequestTimeout,
                        CheckRedirect: func(req *http.Request, via []*http.Request) error {
                                // Preserve our UA across redirects.
                                req.Header.Set("User-Agent", cfg.UserAgent)
                                return nil
                        },
                },
        }
}

// Handler returns the http.Handler that fronts the upstream API.
// All requests are forwarded to {UpstreamBase}{path}?{query} with our
// canonical headers. Successful GETs are cached for CacheTTL.
func (s *Server) Handler() http.Handler {
        return http.HandlerFunc(s.serve)
}

func (s *Server) serve(w http.ResponseWriter, r *http.Request) {
        start := time.Now()

        // Build upstream URL.
        upstreamURL := s.cfg.UpstreamBase + r.URL.Path
        if r.URL.RawQuery != "" {
                upstreamURL += "?" + r.URL.RawQuery
        }

        // Read body for cache key + replay.
        var bodyBytes []byte
        if r.Body != nil {
                var err error
                bodyBytes, err = io.ReadAll(r.Body)
                if err != nil {
                        http.Error(w, "failed to read request body", http.StatusBadRequest)
                        return
                }
                _ = r.Body.Close()
        }

        // Try cache for GET requests.
        cacheKey := cache.Key(r.Method, upstreamURL, bodyBytes)
        if r.Method == http.MethodGet {
                if entry, ok := s.cache.Get(cacheKey); ok {
                        s.writeResponse(w, entry.StatusCode, entry.Headers, entry.Body)
                        s.log.Append(r.Method, r.URL.Path, upstreamURL, entry.StatusCode, time.Since(start), int64(len(entry.Body)), true, "")
                        return
                }
        }

        // Build upstream request.
        upReq, err := http.NewRequestWithContext(r.Context(), r.Method, upstreamURL, bytes.NewReader(bodyBytes))
        if err != nil {
                s.log.Append(r.Method, r.URL.Path, upstreamURL, 0, time.Since(start), 0, false, err.Error())
                http.Error(w, "failed to build upstream request", http.StatusInternalServerError)
                return
        }

        // Copy canonical headers.
        upReq.Header.Set("User-Agent", s.cfg.UserAgent)
        upReq.Header.Set("Accept", "application/json, text/plain, */*")
        upReq.Header.Set("Accept-Language", "pt-BR,pt;q=0.9,en;q=0.8")
        upReq.Header.Set("Referer", "https://supercine-tv.net/")
        if r.Host != "" {
                upReq.Header.Set("X-Forwarded-Host", r.Host)
        }
        if ct := r.Header.Get("Content-Type"); ct != "" {
                upReq.Header.Set("Content-Type", ct)
        }

        // Execute.
        resp, err := s.client.Do(upReq)
        if err != nil {
                s.log.Append(r.Method, r.URL.Path, upstreamURL, 0, time.Since(start), 0, false, err.Error())
                http.Error(w, fmt.Sprintf("upstream error: %v", err), http.StatusBadGateway)
                return
        }
        defer resp.Body.Close()

        body, err := io.ReadAll(resp.Body)
        if err != nil {
                s.log.Append(r.Method, r.URL.Path, upstreamURL, resp.StatusCode, time.Since(start), 0, false, err.Error())
                http.Error(w, "failed to read upstream body", http.StatusBadGateway)
                return
        }

        // Cache successful GET responses.
        if r.Method == http.MethodGet && resp.StatusCode >= 200 && resp.StatusCode < 300 {
                // Copy only safe headers (drop hop-by-hop).
                hdrs := filterHeaders(resp.Header)
                s.cache.Set(cacheKey, &types.CacheEntry{
                        Body:       body,
                        Headers:    hdrs,
                        StatusCode: resp.StatusCode,
                })
        }

        // Forward to client.
        s.writeResponse(w, resp.StatusCode, resp.Header, body)
        s.log.Append(r.Method, r.URL.Path, upstreamURL, resp.StatusCode, time.Since(start), int64(len(body)), false, "")
}

// writeResponse writes status, headers (filtered) and body to the client.
func (s *Server) writeResponse(w http.ResponseWriter, status int, hdrs http.Header, body []byte) {
        for k, vs := range hdrs {
                // Skip hop-by-hop headers.
                if isHopByHop(k) {
                        continue
                }
                for _, v := range vs {
                        w.Header().Add(k, v)
                }
        }
        w.WriteHeader(status)
        _, _ = w.Write(body)
}

// isHopByHop reports whether the header is hop-by-hop (RFC 7230 §6.1).
func isHopByHop(name string) bool {
        switch strings.ToLower(name) {
        case "connection", "keep-alive", "proxy-authenticate", "proxy-authorization",
                "te", "trailer", "transfer-encoding", "upgrade":
                return true
        }
        return false
}

// filterHeaders returns a copy of hdrs without hop-by-hop headers.
func filterHeaders(hdrs http.Header) map[string][]string {
        out := make(map[string][]string, len(hdrs))
        for k, vs := range hdrs {
                if isHopByHop(k) {
                        continue
                }
                cp := make([]string, len(vs))
                copy(cp, vs)
                out[k] = cp
        }
        return out
}

// ForwardURL is a small helper used by the API layer to build absolute upstream URLs.
func (s *Server) ForwardURL(path string, query url.Values) string {
        u := s.cfg.UpstreamBase + path
        if encoded := query.Encode(); encoded != "" {
                u += "?" + encoded
        }
        return u
}

// ForwardGet performs a GET to the upstream with the canonical headers.
// It does NOT use the cache (the API layer may cache at its own granularity).
func (s *Server) ForwardGet(ctx context.Context, path string, query url.Values) (*http.Response, []byte, error) {
        target := s.ForwardURL(path, query)
        req, err := http.NewRequestWithContext(ctx, http.MethodGet, target, nil)
        if err != nil {
                return nil, nil, err
        }
        req.Header.Set("User-Agent", s.cfg.UserAgent)
        req.Header.Set("Accept", "application/json, text/plain, */*")
        req.Header.Set("Accept-Language", "pt-BR,pt;q=0.9,en;q=0.8")
        req.Header.Set("Referer", "https://supercine-tv.net/")

        resp, err := s.client.Do(req)
        if err != nil {
                return nil, nil, err
        }
        defer resp.Body.Close()
        body, err := io.ReadAll(resp.Body)
        if err != nil {
                return resp, nil, err
        }
        return resp, body, nil
}
