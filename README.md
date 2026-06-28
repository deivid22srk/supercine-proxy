# 🎬 Supercine Proxy

Um **proxy reverso em Go** para a API do app Android **Supercine.tv** (package `tv.supercine`), com **interface web embutida** (dashboard), **cache**, **logging estruturado**, e **8 extractors de hosters** (DoodStream, StreamWish, VidHide, FileMoon, FileLions, MixDrop, StreamTape, Voe) portados diretamente do código Java/Kotlin do APK.

[![Go](https://img.shields.io/badge/Go-1.23+-00ADD8?logo=go)](https://go.dev)
[![License](https://img.shields.io/badge/license-MIT-blue)](LICENSE)
[![Status](https://img.shields.io/badge/status-working-green)]()

> 🇧🇷 Projeto feito a partir da engenharia reversa do APK `Supercine.tv_1.0.0_antisplit.apk`.
> 🇺🇸 Reverse-engineered Go proxy for the Supercine.tv Android APK with a web dashboard.

---

## 🚀 Quick start

```bash
# 1. Clonar
git clone https://github.com/deivid22srk/supercine-proxy.git
cd supercine-proxy

# 2. Rodar
go run ./cmd/server

# 3. Acessar
# Dashboard:  http://localhost:8080
# API:        http://localhost:8080/v1/
```

Build estático:

```bash
go build -o supercine-proxy ./cmd/server
./supercine-proxy
```

Variáveis de ambiente:

| Variável | Default | Descrição |
|---|---|---|
| `LISTEN_ADDR` | `:8080` | Endereço de bind |
| `UPSTREAM_BASE` | `https://supercine-tv.net/wp-json` | Base do upstream WordPress REST |
| `EMBED_BASE` | `https://supercine-tv.net/embed-api/` | Base do endpoint de embed |
| `USER_AGENT` | `Mozilla/5.0 ... Chrome/137` | UA usado nas requisições upstream |
| `CACHE_TTL` | `5m` | TTL do cache in-memory |
| `CACHE_MAX_ENTRIES` | `1000` | Máximo de entradas no cache |
| `LOG_MAX_ENTRIES` | `500` | Máximo de logs em memória |
| `REQUEST_TIMEOUT` | `20s` | Timeout HTTP upstream |
| `ADMIN_TOKEN` | _(vazio)_ | Token para endpoints mutantes |
| `VERBOSE` | `false` | Log detalhado |

---

## 🧩 O que isso faz?

O proxy faz três coisas:

1. **Proxy transparente** do `https://supercine-tv.net/wp-json/*` em `http://localhost:8080/wp-json/*`. Você pode apontar o APK diretamente para o proxy sem mudar uma linha.

2. **Camada de API REST** em `/v1/*` com caching, logging, estatísticas, e wrappers de conveniência (e.g. `/v1/extract` resolve um IMDB ID para URL direta mp4/m3u8 em um único call).

3. **Dashboard web** embutido em `/` com visão geral (stats, gráficos), tabela de logs em tempo real, API Explorer, ferramenta de extraction, e docs integradas.

---

## 📡 Endpoints da API do proxy

| Método | Rota | Descrição |
|---|---|---|
| `GET` | `/v1/health` | Health check |
| `GET` | `/v1/stats` | Estatísticas agregadas (requests, cache, errors, bytes) |
| `GET` | `/v1/logs?limit=100` | Últimas N entradas de log |
| `POST` | `/v1/logs/clear` | Limpa logs e stats |
| `GET` | `/v1/api/<type>` | Proxy direto para `/wp-json/api/<type>` (filmes, series, animes, tvshows, movies) |
| `GET` | `/v1/auth/plans` | Planos de assinatura (R$ 19,90 / 49,90 / 149,90) |
| `GET` | `/v1/embed?imdb=...&type=movies\|tvshows` | Lista os `<server-selector>` do embed |
| `GET` | `/v1/extract?imdb=...&type=...&server=N` | Resolve IMDB → URL direta mp4/m3u8 |
| `POST` | `/v1/extract` | Mesmo do GET, body JSON |
| `GET` | `/v1/extractors` | Lista os 8 hosters suportados |
| `GET` | `/v1/routes` | Descoberta automática das 158 rotas upstream |
| `*` | `/wp-json/*` | Proxy transparente para o upstream |
| `GET` | `/embed-api/?...` | Proxy transparente para o embed |

---

## 🎯 Exemplos

### Resolver um filme para URL direta

```bash
curl 'http://localhost:8080/v1/extract?imdb=tt2250912&type=movies'
```

Resposta:

```json
{
  "imdb": "tt2250912",
  "type": "movies",
  "server": 0,
  "server_meta": {
    "title": "Player Alternativo 3",
    "description": "Velocidade ok e poucos anúncios"
  },
  "hoster": "mixdrop",
  "hoster_url": "https://mixdrop.ps/e/mkqwgplli4okq7",
  "videos": [
    {
      "url": "https://30xplewoo.mxcontent.net/v2/mkqwgplli4okq7.mp4?s=...&e=...",
      "quality": "Normal"
    }
  ],
  "took": "437ms"
}
```

### Listar servers de uma série

```bash
curl 'http://localhost:8080/v1/embed?imdb=tt10919420&type=tvshows'
```

### Ver planos

```bash
curl 'http://localhost:8080/v1/auth/plans'
```

```json
{
  "success": true,
  "plans": [
    {"id": "m1", "label": "1 Mês",  "days":  30, "amount":  19.90},
    {"id": "m3", "label": "3 Meses","days":  90, "amount":  49.90},
    {"id": "a1", "label": "1 Ano",  "days": 365, "amount": 149.90}
  ]
}
```

### Usar o proxy transparente com o APK

No APK original, a URL base é `https://supercine-tv.net/wp-json/api/`. Para usar o proxy:

- Opção A (sem modificar o APK): rodar o proxy e usar `iptables`/proxy DNS para redirecionar `supercine-tv.net` para `localhost:8080`.
- Opção B (modificando o APK): patchear a string `https://supercine-tv.net/wp-json/api/` em `libapp.so` para `http://localhost:8080/wp-json/api/` e re-empacotar o APK.

---

## 🏗️ Arquitetura

```
┌─────────────────────────────────────────────────────────────┐
│                    Go binary (single process)               │
│                                                             │
│  ┌────────────┐   ┌────────────┐   ┌────────────────────┐   │
│  │ /v1/* API  │   │ /wp-json/* │   │ /embed-api/*       │   │
│  │ (wrappers) │   │ (passthru) │   │ (passthru)         │   │
│  └─────┬──────┘   └─────┬──────┘   └─────────┬──────────┘   │
│        │                │                    │              │
│        └────────────────┼────────────────────┘              │
│                         ▼                                   │
│  ┌─────────────────────────────────────┐  ┌─────────────┐   │
│  │ proxy.Server (HTTP reverse proxy)   │  │ cache.Cache │   │
│  │ - canonical headers                 │◀▶│ (TTL + LRU) │   │
│  │ - timeout / redirect handling       │  └─────────────┘   │
│  └────────────────┬────────────────────┘                    │
│                   │                                         │
│                   ▼                                         │
│  ┌─────────────────────────────────────┐  ┌─────────────┐   │
│  │ logger.Logger (ring buffer + stats) │  │  extractors │   │
│  └─────────────────────────────────────┘  │  Registry   │   │
│                                           │ (8 hosters) │   │
│  ┌─────────────────────────────────────┐  └─────────────┘   │
│  │ web.Handler (embedded HTML/CSS/JS)  │                    │
│  └─────────────────────────────────────┘                    │
└─────────────────────────────────────────────────────────────┘
                          │
                          ▼
              https://supercine-tv.net/wp-json/
              https://supercine-tv.net/embed-api/
```

---

## 📁 Estrutura do projeto

```
supercine-proxy/
├── cmd/
│   └── server/
│       └── main.go              # entrypoint
├── internal/
│   ├── api/                     # REST API em /v1
│   │   └── api.go
│   ├── cache/                   # cache TTL+LRU
│   │   └── cache.go
│   ├── config/                  # config via env vars
│   │   └── config.go
│   ├── extractors/              # 8 extractors de hosters
│   │   ├── registry.go          # dispatch por URL
│   │   ├── doodstream.go
│   │   ├── streamwish.go
│   │   ├── vidhide.go
│   │   ├── filemoon.go
│   │   ├── filelions.go
│   │   ├── mixdrop.go
│   │   ├── streamtape.go
│   │   ├── voe.go
│   │   └── jsunpacker.go        # port do JSUnpacker.java
│   ├── logger/                  # ring buffer + stats
│   │   └── logger.go
│   ├── proxy/                   # HTTP reverse proxy
│   │   └── proxy.go
│   ├── types/                   # tipos compartilhados
│   │   └── types.go
│   └── web/                     # dashboard HTML/CSS/JS embutido
│       ├── web.go
│       ├── templates/
│       │   └── index.html
│       └── static/
│           ├── style.css
│           └── app.js
├── examples/
│   └── extract_url.go           # CLI para extrair URL de hoster
├── docs/
│   ├── APK_ANALYSIS.md          # análise detalhada do APK
│   ├── UPSTREAM_API.md          # spec da API do supercine-tv.net
│   ├── EXTRACTORS.md            # como cada extractor funciona
│   └── FUNNY_MESSAGES.md        # mensagens engraçadas encontradas
├── go.mod
├── go.sum
├── README.md
├── LICENSE
└── .gitignore
```

---

## 🧪 Testes rápidos

```bash
# Health
curl http://localhost:8080/v1/health

# Stats
curl http://localhost:8080/v1/stats

# Ver 8 extractors
curl http://localhost:8080/v1/extractors

# Resolver filme
curl 'http://localhost:8080/v1/extract?imdb=tt2250912&type=movies'

# Ver planos
curl http://localhost:8080/v1/auth/plans

# Top rotas do upstream
curl http://localhost:8080/v1/routes | jq '.routes | length'
# 158

# CLI standalone para extrair de um hoster URL
go run ./examples/extract_url.go https://mixdrop.ps/e/mkqwgplli4okq7
```

---

## 🛡️ Aviso legal

Este projeto é para **fins educacionais e de interoperabilidade**. Não hospedamos, redistribuímos ou armazenamos nenhum conteúdo protegido por direitos autorais. O proxy apenas repassa chamadas HTTP para a API pública do site `supercine-tv.net` e implementa extractors de vídeo equivalentes aos que já existem no APK original.

Toda a análise foi feita a partir de um APK publicamente disponível no GitHub do próprio app. Não há bypass de DRM, decrypt de conteúdo protegido, ou qualquer técnica que viole os termos de serviço do app original.

Use por sua conta e risco. O autor não se responsabiliza pelo uso indevido.

---

## 📝 Licença

MIT — veja [LICENSE](LICENSE).

---

## 🙋 Autor

Feito por [@deivid22srk](https://github.com/deivid22srk) com análise do APK `Supercine.tv_1.0.0_antisplit.apk` via `jadx`.
