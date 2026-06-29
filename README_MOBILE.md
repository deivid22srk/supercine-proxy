# 📱 Supercine Proxy Mobile

App Android nativo em **Go puro** que executa o `supercine-proxy` dentro
do próprio dispositivo. Quando aberto, o app sobe um servidor HTTP em
`127.0.0.1:8080` expondo a mesma UI de streaming + admin + API REST do
proxy desktop — acessível pelo navegador do celular em
`http://localhost:8080/`.

## Por que Go Mobile puro (sem bindings Java/Kotlin)?

O `gomobile build` gera um APK com uma `GoNativeActivity` que executa
código Go diretamente. Isso significa:

- **Zero Java/Kotlin** — todo o código (servidor HTTP, extractors, UI
  web embutida) é Go.
- **Sem Android Studio** — só Go + NDK + `gomobile`.
- **Reuse total** dos pacotes `internal/` — o entry point mobile
  (`mobile/main.go`) importa e inicializa exatamente os mesmos módulos
  que o servidor desktop (`cmd/server/main.go`).

A única desvantagem: a Activity nativa não tem acesso direto a uma
`WebView` a partir de Go puro. Por isso, o app mostra uma tela
OpenGL escura (estilo Netflix) com o contador de FPS, e o usuário
abre a UI de streaming no navegador do celular. Em uma versão
futura, podemos adicionar um botão que dispara uma `Intent` para
abrir o Chrome automaticamente.

## Como funciona

```
┌─────────────────────────────────────────────────────┐
│                 APK (GoNativeActivity)              │
│                                                     │
│  ┌──────────────┐    ┌──────────────────────────┐  │
│  │ app.Main()   │───▶│ startServer()            │  │
│  │ (lifecycle)  │    │ (HTTP em 127.0.0.1:8080) │  │
│  └──────────────┘    └──────────┬───────────────┘  │
│         │                       │                  │
│         ▼                       ▼                  │
│  ┌──────────────┐    ┌──────────────────────────┐  │
│  │ Tela OpenGL  │    │ web.Handler()            │  │
│  │ (fundo #141) │    │ ├── /        streaming   │  │
│  │ + FPS counter│    │ ├── /admin   dashboard   │  │
│  └──────────────┘    │ ├── /v1/*    REST API    │  │
│                      │ ├── /wp-json proxy       │  │
│                      │ └── /embed-api proxy     │  │
│                      └──────────────────────────┘  │
└─────────────────────────────────────────────────────┘
                  │
                  ▼
        Navegador do celular
        http://localhost:8080/
```

## Estrutura

```
supercine-proxy/
├── cmd/server/main.go        # servidor desktop (não-Android)
├── mobile/main.go            # ⭐ entry point Android (gomobile build)
├── internal/                 # pacotes compartilhados
│   ├── api/                  # /v1/* REST
│   ├── cache/                # cache TTL+LRU
│   ├── config/               # config via env
│   ├── enricher/             # combina IMDB + Supercine
│   ├── extractors/           # 8 hoster extractors
│   ├── imdb/                 # IMDB suggestions (sem API key)
│   ├── logger/               # ring buffer + stats
│   ├── provider/             # Provider interface + Supercine impl
│   ├── proxy/                # HTTP reverse proxy
│   ├── streaming/            # /v1/catalog/*
│   ├── types/
│   └── web/                  # HTML/CSS/JS embutido
├── .github/workflows/build2.yml  # CI: gera APK
└── go.mod
```

## Compilação local

Pré-requisitos:
- Go 1.25+
- Android SDK Platform 34 + Build-Tools 34.0.0
- Android NDK 26.x

```bash
go install golang.org/x/mobile/cmd/gomobile@latest
gomobile init

cd mobile
gomobile build \
  -target=android/arm64,android/amd64 \
  -androidapi=34 \
  -o supercine-proxy-mobile.apk \
  .
```

## Instalação no dispositivo

```bash
adb install -r supercine-proxy-mobile.apk
adb shell am start -n org.golang.app/.GoNativeActivity
```

Depois abra o navegador do celular em `http://localhost:8080/` para ver
a UI de streaming estilo Netflix.

Para ver os logs do app:

```bash
adb logcat -s GoLog
```

## CI

O workflow `.github/workflows/build2.yml` pode ser disparado manualmente
(`workflow_dispatch`) ou a cada push em `main` que toque em `mobile/`,
`internal/`, `go.mod` ou `go.sum`. Ele:

1. Configura Go 1.25 + Android SDK 34 + NDK 26
2. Instala e inicializa o `gomobile`
3. Roda `go mod tidy` para resolver dependências transitivas
4. Compila todos os pacotes `internal/` (sanity check)
5. Roda `go vet ./mobile/...`
6. Executa `gomobile build` com `-androidapi=34`
7. Publica o APK como artifact

## Diferenças em relação ao servidor desktop

| Aspecto | Desktop (`cmd/server`) | Mobile (`mobile/`) |
|---|---|---|
| Listen addr | `:8080` (todas interfaces) | `127.0.0.1:8080` (só localhost) |
| Provider amenic | Importado (mas não existe no repo) | Não importado |
| UI nativa | Nenhuma (HTTP puro) | Tela OpenGL com FPS |
| Lifecycle | SIGINT/SIGTERM | lifecycle.Event (CrossOn/CrossOff) |
| Shutdown | `srv.Shutdown(ctx)` | `cancel()` no contexto |

## Aviso legal

Mesmo aviso do proxy original — ver [`README.md`](README.md).
