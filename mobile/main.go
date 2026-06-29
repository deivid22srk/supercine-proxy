// Package main é o entry point do app Android "Supercine Proxy Mobile".
//
// Diferente do `cmd/server/main.go` (que é um servidor HTTP standalone para
// desktop/servidor), este entry point usa `golang.org/x/mobile/app` para
// renderizar uma Activity nativa no Android. Quando o app é aberto:
//
//  1. Um servidor HTTP é inicializado em background na porta 8080 (bind
//     em 127.0.0.1 — só acessível de dentro do dispositivo).
//  2. O servidor expõe a mesma UI de streaming + admin + API REST do
//     supercine-proxy original.
//  3. A tela nativa mostra instruções de uso e a URL local onde a UI
//     pode ser acessada por um navegador no próprio dispositivo.
//
// Como o `gomobile build` não tem acesso direto a WebView a partir de
// Go puro, a abordagem é: o app sobe o servidor, e o usuário abre a UI
// no navegador do celular (Chrome/Firefox) em http://localhost:8080/.
//
// Compilação:
//
//      gomobile build -target=android/arm64,android/amd64 -androidapi=29 \
//        -o supercine-proxy.apk .
//
// O pacote `internal/provider/amenic` é opcional e não é importado aqui
// (a versão atual do repositório não o contém). Apenas o provider
// `supercine` é registrado.
package main

import (
        "context"
        "log"
        "net/http"
        "os"
        "time"

        "golang.org/x/mobile/app"
        "golang.org/x/mobile/event/lifecycle"
        "golang.org/x/mobile/event/paint"
        "golang.org/x/mobile/event/size"
        "golang.org/x/mobile/event/touch"
        "golang.org/x/mobile/exp/app/debug"
        "golang.org/x/mobile/exp/gl/glutil"
        "golang.org/x/mobile/gl"

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
        version = "1.0.0-mobile"
        commit  = "dev"
        date    = "unknown"
)

const (
        // listenAddr é o endereço que o servidor HTTP do app vai escutar.
        // 127.0.0.1 garante que só o próprio dispositivo pode acessar.
        listenAddr = "127.0.0.1:8080"

        // publicURL é o que aparece na tela para o usuário digitar no navegador.
        publicURL = "http://localhost:8080/"
)

func main() {
        app.Main(func(a app.App) {
                var (
                        glctx   gl.Context
                        sz      size.Event
                        fps     *debug.FPS
                        images  *glutil.Images
                        srvErr  <-chan error
                        stopSrv context.CancelFunc
                        started bool
                )

                for {
                        select {
                        case e := <-a.Events():
                                switch e := a.Filter(e).(type) {

                                case lifecycle.Event:
                                        switch e.Crosses(lifecycle.StageVisible) {
                                        case lifecycle.CrossOn:
                                                // App ficou visível: inicializa OpenGL e sobe o servidor.
                                                glctx, _ = e.DrawContext.(gl.Context)
                                                images = glutil.NewImages(glctx)
                                                fps = debug.NewFPS(images)

                                                if !started {
                                                        log.Printf("[mobile] supercine-proxy %s starting (commit=%s, built=%s)", version, commit, date)
                                                        var srv *http.Server
                                                        srv, srvErr, stopSrv = startServer()
                                                        started = true
                                                        _ = srv
                                                        go func() {
                                                                if err := <-srvErr; err != nil && err != http.ErrServerClosed {
                                                                        log.Printf("[mobile] server error: %v", err)
                                                                }
                                                        }()
                                                        log.Printf("[mobile] server listening on %s", publicURL)
                                                }

                                        case lifecycle.CrossOff:
                                                // App foi para background: libera recursos GL e desliga
                                                // o servidor para economizar bateria.
                                                if images != nil {
                                                        images.Release()
                                                        images = nil
                                                }
                                                glctx = nil
                                                if stopSrv != nil {
                                                        log.Printf("[mobile] shutting down server (app backgrounded)")
                                                        stopSrv()
                                                        stopSrv = nil
                                                        started = false
                                                }
                                        }

                                case size.Event:
                                        sz = e

                                case paint.Event:
                                        if glctx == nil || sz.WidthPx == 0 {
                                                continue
                                        }
                                        // Fundo escuro estilo Netflix (#141414 = 0.078, 0.078, 0.078)
                                        glctx.ClearColor(0.078, 0.078, 0.078, 1.0)
                                        glctx.Clear(gl.COLOR_BUFFER_BIT)
                                        if fps != nil {
                                                fps.Draw(sz)
                                        }
                                        a.Publish()
                                        // Mantém redesenhando (caso queiramos animar no futuro).
                                        a.Send(paint.Event{})

                                case touch.Event:
                                        // No futuro: detectar tap e abrir o navegador automaticamente
                                        // via intent. Por ora, apenas loga.
                                        if e.Type == touch.TypeEnd {
                                                log.Printf("[mobile] touch at (%.0f, %.0f)", e.X, e.Y)
                                        }
                                }
                        }
                }
        })
}

// startServer sobe o servidor HTTP do supercine-proxy em 127.0.0.1:8080.
//
// Reutiliza todos os pacotes internos (api, cache, config, extractors,
// provider, proxy, streaming, web) — a única diferença em relação ao
// `cmd/server/main.go` é:
//
//   - Omit o provider `amenic` (que não existe no repositório atual).
//   - Fixa o listenAddr em 127.0.0.1:8080 em vez de :8080.
//
// Retorna:
//   - *http.Server: instância do servidor (para Shutdown)
//   - <-chan error: canal que recebe o erro fatal (ou nil em shutdown limpo)
//   - context.CancelFunc: chame para desligar o servidor graciosamente
func startServer() (*http.Server, <-chan error, context.CancelFunc) {
        cfg := config.Default()
        // Sobrescreve o listenAddr para garantir bind local.
        cfg.ListenAddr = listenAddr

        logr := logger.New(cfg.LogMaxEntries)
        cacheStore := cache.New(cfg.CacheTTL, cfg.CacheMaxEntries)
        registry := extractors.NewRegistry()
        proxyServer := proxy.New(cfg, cacheStore, logr)
        apiServer := api.New(cfg, logr, proxyServer, registry)

        // Provider registry — apenas Supercine (o provider amenic não existe
        // neste repositório). A arquitetura permite adicionar mais providers
        // depois sem tocar na UI.
        providerReg := provider.NewRegistry()
        sp := supercine.New(supercine.ProviderConfig{
                EmbedBase:   cfg.EmbedBase,
                APIBase:     cfg.UpstreamBase + "/api",
                UserAgent:   cfg.UserAgent,
                HTTPTimeout: cfg.RequestTimeout,
        }, registry)
        providerReg.Register(sp)
        log.Printf("[mobile] registered provider: %s (priority=%d)", sp.Name(), sp.Priority())

        en := enricher.New(providerReg)
        streamingHandler := streaming.New(en, providerReg)

        mux := http.NewServeMux()
        streamingHandler.Register(mux)
        mux.Handle("/v1/", apiServer.Handler())
        mux.Handle("/wp-json/", proxyServer.Handler())
        mux.HandleFunc("/embed-api/", embedHandler(cfg))
        mux.Handle("/", web.Handler())

        srv := &http.Server{
                Addr:              cfg.ListenAddr,
                Handler:           mux,
                ReadHeaderTimeout: 10 * time.Second,
                IdleTimeout:       60 * time.Second,
        }

        errCh := make(chan error, 1)
        ctx, cancel := context.WithCancel(context.Background())

        go func() {
                log.Printf("[mobile] HTTP server starting on http://%s", cfg.ListenAddr)
                err := srv.ListenAndServe()
                select {
                case <-ctx.Done():
                        errCh <- nil
                default:
                        errCh <- err
                }
        }()

        go func() {
                <-ctx.Done()
                shutdownCtx, c := context.WithTimeout(context.Background(), 5*time.Second)
                defer c()
                _ = srv.Shutdown(shutdownCtx)
        }()

        return srv, errCh, cancel
}

// embedHandler faz o proxy simples para /embed-api/ do upstream.
// Equivalente ao handler inline em cmd/server/main.go.
func embedHandler(cfg *config.Config) http.HandlerFunc {
        return func(w http.ResponseWriter, r *http.Request) {
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
                buf := make([]byte, 32*1024)
                for {
                        n, rerr := resp.Body.Read(buf)
                        if n > 0 {
                                _, _ = w.Write(buf[:n])
                        }
                        if rerr != nil {
                                break
                        }
                }
        }
}

// init garante que temos um logger escrevendo para logcat no Android.
func init() {
        log.SetOutput(os.Stderr)
        log.SetFlags(log.LstdFlags | log.Lshortfile)
}
