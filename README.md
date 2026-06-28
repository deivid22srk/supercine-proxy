# 🎬 Supercine Proxy

Um **proxy reverso em Go** para a API do app Android **Supercine.tv** (package `tv.supercine`), com **interface web estilo Netflix** para pesquisar e assistir filmes/séries direto no navegador, mais um **dashboard admin** com cache, logging estruturado, e **8 extractors de hosters** (DoodStream, StreamWish, VidHide, FileMoon, FileLions, MixDrop, StreamTape, Voe) portados do código Java/Kotlin do APK.

[![Go](https://img.shields.io/badge/Go-1.23+-00ADD8?logo=go)](https://go.dev)
[![License](https://img.shields.io/badge/license-MIT-blue)](LICENSE)
[![Status](https://img.shields.io/badge/status-working-green)]()

> 🇧🇷 Projeto feito a partir da engenharia reversa do APK `Supercine.tv_1.0.0_antisplit.apk`.
> 🇺🇸 Reverse-engineered Go proxy for the Supercine.tv Android APK with a Netflix-style web UI.

---

## 🚀 Quick start

```bash
# 1. Clonar
git clone https://github.com/deivid22srk/supercine-proxy.git
cd supercine-proxy

# 2. Rodar
go run ./cmd/server

# 3. Acessar
# UI Streaming: http://localhost:8080/         (pesquise e assista!)
# Admin:        http://localhost:8080/admin    (logs, stats, API explorer)
# API:          http://localhost:8080/v1/
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

O proxy faz quatro coisas:

1. **UI de Streaming** em `/` (estilo Netflix) — pesquise por qualquer filme/série, veja capas, backdrops e detalhes, e assista direto no navegador com player de vídeo (hls.js para m3u8, mp4 nativo). A busca usa a API pública do IMDB (sem chave), os detalhes e backdrops vêm do endpoint `/embed-api/` do próprio Supercine, e o player usa os 8 extractors para resolver o link direto.

2. **Proxy transparente** do `https://supercine-tv.net/wp-json/*` em `http://localhost:8080/wp-json/*`. Você pode apontar o APK diretamente para o proxy sem mudar uma linha.

3. **Camada de API REST** em `/v1/*` com caching, logging, estatísticas, e wrappers de conveniência (e.g. `/v1/extract` resolve um IMDB ID para URL direta mp4/m3u8 em um único call). Endpoints `/v1/catalog/*` alimentam a UI de streaming.

4. **Dashboard admin** em `/admin` com visão geral (stats, gráficos), tabela de logs em tempo real, API Explorer, ferramenta de extraction, e docs integradas.

---

## 🎯 Descoberta chave: o Supercine retorna imagens!

Investigando o endpoint `/embed-api/?imdb=...&type=movies` descobrimos que ele retorna uma página HTML customizada contendo:

- `<ititle>` com o **título traduzido em PT-BR** (ex: `tt2250912` → "Homem-Aranha: De Volta ao Lar")
- `<backdrop style="background-image: url('https://image.tmdb.org/...')">` com o **backdrop do TMDB**
- Vários `<server-selector data-server="...">` com os hosters disponíveis

Isso significa que **para qualquer IMDB ID válido**, o Supercine resolve metadados + backdrop + servidores. Combinado com a API pública de sugestões do IMDB (`v3.sg.media-imdb.com/suggestion/...`) que retorna o título original, ano, poster e elenco, temos um catálogo completo **sem precisar de nenhuma API key**.

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
| `GET` | `/v1/catalog/popular?type=movies&limit=80` | Lista de ~80 populares com metadados completos (sem API key) |
| `GET` | `/v1/catalog/search?q=...&limit=12` | Busca por título (via IMDB suggestion API) |
| `GET` | `/v1/catalog/resolve?imdb=tt...&type=movies` | Resolve um IMDB específico |
| `GET` | `/v1/catalog/movie/<imdb>` | Alias path-based para resolve |
| `*` | `/wp-json/*` | Proxy transparente para o upstream |
| `GET` | `/embed-api/?...` | Proxy transparente para o embed |

---

## 🎯 Exemplos

### Buscar e resolver metadados (sem API key!)

```bash
curl 'http://localhost:8080/v1/catalog/search?q=homem+aranha&limit=3' | jq '.items[] | {imdb, title_ptbr, title_orig, year, available, server_count}'
```

```json
{
  "imdb": "tt2250912",
  "title_ptbr": "Homem-Aranha: De Volta ao Lar",
  "title_orig": "Spider-Man: Homecoming",
  "year": 2017,
  "available": true,
  "server_count": 4
}
```

### Listar filmes populares

```bash
curl 'http://localhost:8080/v1/catalog/popular?type=movies&limit=10' | jq '.items[] | "\(.title_ptbr) (\(.year)) [\(.server_count) servidores]"'
```

```
"Um Sonho de Liberdade (1994) [3 servidores]"
"Batman: O Cavaleiro das Trevas (2008) [5 servidores]"
"A Origem (2010) [3 servidores]"
"Matrix (1999) [4 servidores]"
"Interestelar (2014) [7 servidores]"
...
```

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

## 🎬 Interface de Streaming (`/`)

A página inicial do proxy é uma **UI estilo Netflix** onde você pesquisa e assiste filmes/séries direto no navegador.

### Features

- **Home com hero banner** mostrando um título em destaque (com backdrop full-screen do TMDB)
- **3 rows de catálogo**: Filmes populares, Séries populares, Clássicos
- **Busca em tempo real** com debounce de 350ms — busca na API de sugestões do IMDB
- **Modal de detalhes** com backdrop, título, ano, tipo e elenco
- **Player de vídeo embutido** com:
  - Suporte a **mp4 direto** (nativo do navegador)
  - Suporte a **HLS/m3u8** via [hls.js](https://github.com/video-dev/hls.js/) (com fallback nativo para Safari)
  - Lista de servidores disponíveis para escolha manual
  - Botão "tentar outro servidor" quando um falha
- **Responsive**: funciona em desktop, tablet e mobile

### Como funciona o fluxo

```
┌──────────────┐    ┌──────────────────┐    ┌─────────────────┐
│  Browser UI  │ -> │ /v1/catalog/*    │ -> │ IMDB Suggestions│
│  (streaming) │    │ (Go enricher)    │    │ (sem API key)   │
└──────────────┘    └────────┬─────────┘    └─────────────────┘
                             │
                             ▼
                    ┌──────────────────┐
                    │ /v1/embed        │ (Supercine /embed-api/)
                    │ + /v1/extract    │ -> PT-BR title + backdrop
                    └────────┬─────────┘    + server list -> hoster URL
                             │              -> direct mp4/m3u8 URL
                             ▼
                    ┌──────────────────┐
                    │ 8 hoster         │
                    │  extractors      │
                    │  (DoodStream,    │
                    │   StreamWish,    │
                    │   VidHide, etc)  │
                    └──────────────────┘
```

### Configuração opcional

A UI de streaming **não precisa de nenhuma API key**. Mas você pode ajustar:

| Variável | Default | Descrição |
|---|---|---|
| `EMBED_BASE` | `https://supercine-tv.net/embed-api/` | Base do endpoint de embed |

Se quiser adicionar mais filmes à home, edite `internal/imdb/catalog.go` (lista `PopularTitles`).

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
│   ├── api/                     # REST API em /v1 (admin)
│   │   └── api.go
│   ├── cache/                   # cache TTL+LRU
│   │   └── cache.go
│   ├── config/                  # config via env vars
│   │   └── config.go
│   ├── enricher/                # combina IMDB + Supercine embed
│   │   └── enricher.go          # para produzir MovieMeta completo
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
│   ├── imdb/                    # client da API de sugestões IMDB
│   │   ├── imdb.go              # (sem chave) + catálogo popular
│   │   └── catalog.go           # ~80 IMDB IDs populares curados
│   ├── logger/                  # ring buffer + stats
│   │   └── logger.go
│   ├── proxy/                   # HTTP reverse proxy
│   │   └── proxy.go
│   ├── streaming/               # endpoints /v1/catalog/* (UI streaming)
│   │   └── streaming.go
│   ├── types/                   # tipos compartilhados
│   │   └── types.go
│   └── web/                     # HTML/CSS/JS embutido (2 UIs)
│       ├── web.go               # / -> streaming, /admin -> dashboard
│       ├── templates/
│       │   ├── streaming.html   # UI estilo Netflix
│       │   └── index.html       # dashboard admin
│       └── static/
│           ├── streaming.css    # estilos da UI streaming
│           ├── streaming.js     # lógica + hls.js integration
│           ├── style.css        # estilos do admin
│           └── app.js           # lógica do admin
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

### Test suite de resolução

Roda um conjunto de 16 títulos (12 filmes + 4 episódios de séries) conhecidos como disponíveis no Supercine e verifica se todos resolvem para pelo menos uma URL de vídeo direta:

```bash
./scripts/test_resolve.sh
```

Saída esperada:

```
==========================================
  Results: 16 passed, 0 failed
==========================================
```

---

## 🐛 Troubleshooting

### "DoodStream: CAPTCHA (Cloudflare Turnstile)" 

O DoodStream agora exige um CAPTCHA do Cloudflare Turnstile na página de embed. O extractor não consegue resolver o CAPTCHA automaticamente, então ele falha graciosamente e o proxy tenta o próximo servidor (StreamWish, FileMoon, MixDrop, etc.). Se **todos** os servidores de um título forem DoodStream, a resolução vai falhar — isso é uma limitação do hoster, não do proxy.

### "provider: title not available"

O Supercine não tem esse título no catálogo. O embed-api retorna 0 servidores. Não é um bug — significa que o título não está disponível no upstream.

### MixDrop: URLs com `sub1=` e `sub1_label=`

O Supercine às vezes acrescenta parâmetros de legenda na URL do MixDrop (`?sub1=...&sub1_label=...`). O MixDrop rejeita essas URLs com HTTP 400, então o extractor agora stripa a query string antes de fazer a requisição. A legenda não é necessária para extrair a URL direta do vídeo.

### verifyURL: HEAD retorna 403 no MixDrop CDN

O CDN do MixDrop (`*.mxcontent.net`) rejeita requests HEAD com 403, mesmo para URLs válidas. O `verifyURL` agora usa GET com `Range: bytes=0-1` (que é o que o `<video>` faz no navegador) e envia os headers `Origin` e `Referer` corretos para o CDN do hoster.

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
