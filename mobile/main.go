// Package main é o entry point do app Android "Supercine Proxy Mobile".
//
// Diferente do `cmd/server/main.go` (servidor HTTP standalone para
// desktop/servidor), este entry point usa `golang.org/x/mobile/app` para
// renderizar uma Activity nativa no Android. Quando o app é aberto:
//
//  1. Um servidor HTTP é inicializado em background na porta 8080 (bind
//     em 127.0.0.1 — só acessível de dentro do dispositivo).
//  2. O servidor expõe a mesma UI de streaming + admin + API REST do
//     supercine-proxy original.
//  3. A tela nativa mostra um painel de texto com o status do servidor,
//     a URL local (http://localhost:8080/) e instruções de uso.
//  4. Tocar na tela tenta abrir o navegador default em http://localhost:8080/
//     via `am start -a android.intent.action.VIEW -d <url>`.
//
// Compilação:
//
//	gomobile build -target=android/arm64,android/amd64 -androidapi=29 \
//	  -o supercine-proxy.apk .
//
// O pacote `internal/provider/amenic` é um stub (não existe no repositório
// original). Apenas o provider `supercine` faz resolução real.
package main

import (
	"context"
	"log"
	"net/http"
	"os"
	"os/exec"
	"sync"
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

	// publicURL é o que aparece na tela para o usuário.
	publicURL = "http://localhost:8080/"

	// tapHintX e tapHintY definem a área central da tela onde um tap
	// dispara a abertura do navegador. Qualquer toque fora dessa área
	// apenas loga (para debug).
)

// appState é o estado compartilhado entre o event loop (thread GL) e
// a goroutine do servidor HTTP. Protegido por mu.
type appState struct {
	mu     sync.Mutex
	status string // "starting" | "running" | "error"
	errMsg string
}

func (s *appState) setStatus(status, errMsg string) {
	s.mu.Lock()
	s.status = status
	s.errMsg = errMsg
	s.mu.Unlock()
}

func (s *appState) snapshot() (status, errMsg string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.status, s.errMsg
}

// state é global para simplificar o acesso do event loop.
var state = &appState{status: "starting"}

func main() {
	app.Main(func(a app.App) {
		var (
			glctx   gl.Context
			sz      size.Event
			fps     *debug.FPS
			images  *glutil.Images
			panel   *TextPanel
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

						// Cria o painel de texto. O tamanho da textura é
						// fixo (720x1280) e esticado para cobrir a tela;
						// isso evita re-alocar a textura quando a tela
						// muda (rotação, etc).
						panel = NewTextPanel(images, 720, 1280)

						if !started {
							log.Printf("[mobile] supercine-proxy %s starting (commit=%s, built=%s)", version, commit, date)
							state.setStatus("starting", "")
							_, _, stopSrv = startServer(state)
							started = true
							log.Printf("[mobile] server starting on %s", publicURL)
						}

					case lifecycle.CrossOff:
						// App foi para background: libera recursos GL e
						// desliga o servidor para economizar bateria.
						if panel != nil {
							panel.Release()
							panel = nil
						}
						if images != nil {
							images.Release()
							images = nil
						}
						if fps != nil {
							// debug.FPS não tem Release; as texturas são
							// liberadas via images.Release() acima.
							fps = nil
						}
						glctx = nil
						if stopSrv != nil {
							log.Printf("[mobile] shutting down server (app backgrounded)")
							stopSrv()
							stopSrv = nil
							started = false
							state.setStatus("stopped", "")
						}
					}

				case size.Event:
					sz = e

				case paint.Event:
					if glctx == nil || sz.WidthPx == 0 {
						continue
					}
					// Fundo escuro estilo Netflix (#141414)
					glctx.ClearColor(0.078, 0.078, 0.078, 1.0)
					glctx.Clear(gl.COLOR_BUFFER_BIT)

					// Atualiza e desenha o painel de texto.
					if panel != nil {
						panel.SetLines(buildLines())
						panel.Draw(sz)
					}

					// FPS no canto (translúcido sobre o painel).
					if fps != nil {
						fps.Draw(sz)
					}

					a.Publish()
					// Redesenha continuamente para refletir mudanças de
					// status (starting -> running -> error).
					a.Send(paint.Event{})

				case touch.Event:
					// Um toque em qualquer lugar da tela tenta abrir o
					// navegador default em http://localhost:8080/.
					if e.Type == touch.TypeEnd {
						log.Printf("[mobile] tap at (%.0f, %.0f) — opening browser", e.X, e.Y)
						go openBrowser(publicURL)
					}
				}
			}
		}
	})
}

// buildLines constrói as linhas do painel com base no status atual.
// Chamado a cada paint.Event (mas SetLines só re-renderiza se mudou).
func buildLines() []textLine {
	status, errMsg := state.snapshot()

	lines := []textLine{
		{text: "Supercine Proxy Mobile", col: colTitle},
		{text: "v" + version + "  (" + commit + ")", col: colDim},
		{text: "", col: colNormal},
	}

	switch status {
	case "starting":
		lines = append(lines,
			textLine{text: "Status: INICIANDO SERVIDOR...", col: colAccent},
			textLine{text: "", col: colNormal},
			textLine{text: "Aguarde alguns segundos.", col: colNormal},
		)
	case "running":
		lines = append(lines,
			textLine{text: "Status: SERVIDOR RODANDO", col: colOK},
			textLine{text: "", col: colNormal},
			textLine{text: "URL: " + publicURL, col: colAccent},
			textLine{text: "", col: colNormal},
			textLine{text: ">>> TOQUE NA TELA PARA ABRIR <<<", col: colOK},
			textLine{text: ">>> O NAVEGADOR NO CEL <<<", col: colOK},
			textLine{text: "", col: colNormal},
			textLine{text: "Ou abra manualmente:", col: colDim},
			textLine{text: publicURL, col: colAccent},
			textLine{text: "", col: colNormal},
			textLine{text: "UI de streaming estilo Netflix,", col: colNormal},
			textLine{text: "admin dashboard em /admin,", col: colNormal},
			textLine{text: "API REST em /v1/.", col: colNormal},
		)
	case "error":
		lines = append(lines,
			textLine{text: "Status: ERRO AO INICIAR", col: colErr},
			textLine{text: "", col: colNormal},
			textLine{text: "Erro: " + truncate(errMsg, 40), col: colErr},
			textLine{text: "", col: colNormal},
			textLine{text: "Verifique o logcat:", col: colDim},
			textLine{text: "adb logcat -s GoLog", col: colDim},
		)
	case "stopped":
		lines = append(lines,
			textLine{text: "Status: PARADO (app em background)", col: colDim},
			textLine{text: "", col: colNormal},
			textLine{text: "Reabra o app para reiniciar.", col: colDim},
		)
	default:
		lines = append(lines, textLine{text: "Status: " + status, col: colNormal})
	}

	return lines
}

// truncate encurta uma string para no máximo n chars, adicionando "...".
func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n-3] + "..."
}

// openBrowser tenta abrir o navegador default do Android na URL dada.
// Usa o comando `am start` (Activity Manager) que está disponível em
// /system/bin/am em todos os dispositivos Android.
//
// Se falhar (sem permissão, comando não encontrado, etc), apenas loga —
// o usuário ainda pode abrir o navegador manualmente.
func openBrowser(url string) {
	// Tenta caminhos comuns do `am` em diferentes versões do Android.
	candidates := []string{
		"/system/bin/am",
		"am",
	}
	var lastErr error
	for _, amPath := range candidates {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		cmd := exec.CommandContext(ctx, amPath,
			"start",
			"-a", "android.intent.action.VIEW",
			"-d", url,
		)
		out, err := cmd.CombinedOutput()
		cancel()
		if err == nil {
			log.Printf("[mobile] browser opened via %s (output: %s)", amPath, string(out))
			return
		}
		lastErr = err
		log.Printf("[mobile] %s failed: %v (output: %s)", amPath, err, string(out))
	}
	log.Printf("[mobile] could not open browser automatically: %v", lastErr)
	log.Printf("[mobile] please open manually: %s", url)
}

// startServer sobe o servidor HTTP do supercine-proxy em 127.0.0.1:8080.
//
// Reutiliza todos os pacotes internos (api, cache, config, extractors,
// provider, proxy, streaming, web). Atualiza `state` com o status.
//
// Retorna:
//   - *http.Server: instância do servidor (para Shutdown)
//   - <-chan error: canal que recebe o erro fatal (ou nil em shutdown limpo)
//   - context.CancelFunc: chame para desligar o servidor graciosamente
func startServer(state *appState) (*http.Server, <-chan error, context.CancelFunc) {
	cfg := config.Default()
	cfg.ListenAddr = listenAddr

	logr := logger.New(cfg.LogMaxEntries)
	cacheStore := cache.New(cfg.CacheTTL, cfg.CacheMaxEntries)
	registry := extractors.NewRegistry()
	proxyServer := proxy.New(cfg, cacheStore, logr)
	apiServer := api.New(cfg, logr, proxyServer, registry)

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
			// Shutdown limpo
			state.setStatus("stopped", "")
			errCh <- nil
		default:
			// Erro fatal
			if err != nil && err != http.ErrServerClosed {
				log.Printf("[mobile] server fatal: %v", err)
				state.setStatus("error", err.Error())
			}
			errCh <- err
		}
	}()

	// Pequeno atraso para dar tempo do ListenAndServe subir antes
	// de marcar como "running". Em prática, ListenAndServe é quase
	// instantâneo, mas isso garante que a porta esteja realmente
	// aceitando conexões antes de mudar o status.
	go func() {
		time.Sleep(300 * time.Millisecond)
		// Testa se a porta responde.
		client := &http.Client{Timeout: 2 * time.Second}
		resp, err := client.Get("http://" + listenAddr + "/v1/health")
		if err != nil {
			log.Printf("[mobile] health check failed: %v (server may still be starting)", err)
			// Mesmo assim marca como running — o servidor pode estar
			// inicializando os extractors.
			state.setStatus("running", "")
			return
		}
		resp.Body.Close()
		log.Printf("[mobile] health check OK (status=%d) — server is running", resp.StatusCode)
		state.setStatus("running", "")
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

func init() {
	log.SetOutput(os.Stderr)
	log.SetFlags(log.LstdFlags | log.Lshortfile)
}
