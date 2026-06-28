// Package main is the entrypoint for the supercine-proxy server.
//
// It wires up the configuration, logger, cache, proxy, API, extractors, and
// the embedded web dashboard, then starts the HTTP listener.
package main

import (
        "context"
        "flag"
        "fmt"
        "log"
        "net/http"
        "os"
        "os/signal"
        "syscall"
        "time"

        "github.com/deivid22srk/supercine-proxy/internal/api"
        "github.com/deivid22srk/supercine-proxy/internal/cache"
        "github.com/deivid22srk/supercine-proxy/internal/config"
        "github.com/deivid22srk/supercine-proxy/internal/enricher"
        "github.com/deivid22srk/supercine-proxy/internal/extractors"
        "github.com/deivid22srk/supercine-proxy/internal/logger"
        "github.com/deivid22srk/supercine-proxy/internal/provider"
        "github.com/deivid22srk/supercine-proxy/internal/provider/supercine"
        "github.com/deivid22srk/supercine-proxy/internal/proxy"
        "github.com/deivid22srk/supercine-proxy/internal/streaming"
        "github.com/deivid22srk/supercine-proxy/internal/web"
)

// Build-time injected values (overridable via -ldflags).
var (
        version = "1.0.0"
        commit  = "dev"
        date    = "unknown"
)

func main() {
        showVersion := flag.Bool("version", false, "Print version and exit")
        flag.Parse()
        if *showVersion {
                fmt.Printf("supercine-proxy %s (commit=%s, built=%s)\n", version, commit, date)
                return
        }

        cfg := config.Default()
        log.Printf("[boot] supercine-proxy %s starting", version)
        log.Printf("[boot] config: %s", cfg)

        // Build dependencies.
        logr := logger.New(cfg.LogMaxEntries)
        cacheStore := cache.New(cfg.CacheTTL, cfg.CacheMaxEntries)
        registry := extractors.NewRegistry()
        proxyServer := proxy.New(cfg, cacheStore, logr)
        apiServer := api.New(cfg, logr, proxyServer, registry)

        // Provider registry — currently only Supercine, but the architecture
        // supports adding more providers (megahdfilmes, jellyfin, etc.) later
        // without touching the UI layer.
        providerReg := provider.NewRegistry()
        sp := supercine.New(supercine.ProviderConfig{
                EmbedBase:   cfg.EmbedBase,
                APIBase:     cfg.UpstreamBase + "/api",
                UserAgent:   cfg.UserAgent,
                HTTPTimeout: cfg.RequestTimeout,
        }, registry)
        providerReg.Register(sp)
        log.Printf("[boot] registered provider: %s (priority=%d)", sp.Name(), sp.Priority())

        en := enricher.New(providerReg)
        streamingHandler := streaming.New(en, providerReg)

        // Top-level router.
        mux := http.NewServeMux()

        // /v1/catalog/* and /v1/providers and /v1/resolve — streaming UI endpoints.
        streamingHandler.Register(mux)

        // /v1/* — proxy REST API.
        mux.Handle("/v1/", apiServer.Handler())

        // /wp-json/* — raw reverse proxy to upstream (transparent passthrough).
        // This lets the Android APK be pointed directly at the proxy (replace
        // https://supercine-tv.net/wp-json/ with http://localhost:8080/wp-json/)
        // without any other code change.
        mux.Handle("/wp-json/", proxyServer.Handler())

        // /embed-api/* — also forwarded raw, since the embed page lives outside
        // /wp-json/. The dashboard's /v1/embed wraps this with parsing.
        mux.HandleFunc("/embed-api/", func(w http.ResponseWriter, r *http.Request) {
                // Rewrite to upstream embed endpoint.
                target := cfg.EmbedBase + "?" + r.URL.RawQuery
                req, err := http.NewRequestWithContext(r.Context(), http.MethodGet, target, nil)
                if err != nil {
                        http.Error(w, err.Error(), http.StatusInternalServerError)
                        return
                }
                req.Header.Set("User-Agent", cfg.UserAgent)
                req.Header.Set("Referer", "https://supercine-tv.net/")
                client := &http.Client{Timeout: cfg.RequestTimeout}
                resp, err := client.Do(req)
                if err != nil {
                        http.Error(w, err.Error(), http.StatusBadGateway)
                        return
                }
                defer resp.Body.Close()
                for k, vs := range resp.Header {
                        for _, v := range vs {
                                w.Header().Add(k, v)
                        }
                }
                w.WriteHeader(resp.StatusCode)
                _, _ = copyBody(w, resp.Body)
        })

        // /static/* and / — dashboard SPA.
        mux.Handle("/", web.Handler())

        // Add a request logger middleware for visibility.
        handler := withLogging(logr, mux)

        srv := &http.Server{
                Addr:              cfg.ListenAddr,
                Handler:           handler,
                ReadHeaderTimeout: 10 * time.Second,
                IdleTimeout:       60 * time.Second,
        }

        // Start in background.
        go func() {
                log.Printf("[boot] listening on http://%s", cfg.ListenAddr)
                if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
                        log.Fatalf("[fatal] server failed: %v", err)
                }
        }()

        // Wait for shutdown signal.
        stop := make(chan os.Signal, 1)
        signal.Notify(stop, syscall.SIGINT, syscall.SIGTERM)
        <-stop
        log.Printf("[shutdown] signal received, draining...")

        ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
        defer cancel()
        if err := srv.Shutdown(ctx); err != nil {
                log.Printf("[shutdown] error during shutdown: %v", err)
        }
        log.Printf("[shutdown] done. bye 👋")
}

// withLogging is a minimal access log middleware. The structured per-request
// log lives in the proxy layer; this one is for the stdout stream.
func withLogging(l *logger.Logger, next http.Handler) http.Handler {
        return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
                start := time.Now()
                ww := &statusWriter{ResponseWriter: w, status: 200}
                next.ServeHTTP(ww, r)
                dur := time.Since(start)
                // Stdout access log (skip /static/ noise).
                if r.URL.Path != "/v1/health" && r.URL.Path != "/" {
                        log.Printf("%s %s -> %d in %s (%d bytes)", r.Method, r.URL.Path, ww.status, dur, ww.bytes)
                }
        })
}

type statusWriter struct {
        http.ResponseWriter
        status int
        bytes  int
}

func (s *statusWriter) WriteHeader(code int) {
        s.status = code
        s.ResponseWriter.WriteHeader(code)
}

func (s *statusWriter) Write(b []byte) (int, error) {
        n, err := s.ResponseWriter.Write(b)
        s.bytes += n
        return n, err
}

// copyBody streams r to w with a small buffer.
func copyBody(w http.ResponseWriter, r interface{ Read([]byte) (int, error) }) (int64, error) {
        buf := make([]byte, 32*1024)
        var total int64
        for {
                n, err := r.Read(buf)
                if n > 0 {
                        if _, werr := w.Write(buf[:n]); werr != nil {
                                return total, werr
                        }
                        total += int64(n)
                }
                if err != nil {
                        if err.Error() == "EOF" {
                                return total, nil
                        }
                        return total, err
                }
        }
}
