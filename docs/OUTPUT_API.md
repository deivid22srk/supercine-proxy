# Supercine Proxy — Output API

> **Versão:** 1.3.0
> **Base URL:** `http://localhost:8080`
> **Formato:** JSON (UTF-8)
> **Idioma das mensagens:** Português brasileiro (PT-BR)

Esta é a **API unificada de saída** do proxy. Independente de qual provedor
de streaming esteja ativo no backend (hoje apenas Supercine, mas a arquitetura
suporta adicionar MegaHD, Jellyfin, etc. sem mudar a API), todas as respostas
desta documentação **sempre terão o mesmo formato**.

Clientes (app mobile, web UI, scripts) só precisam conhecer esta API — nunca
a API interna de cada provedor. Quando novos provedores forem adicionados, a
única mudança visível será o campo `provider` em algumas respostas, indicando
qual provedor serviu o conteúdo.

---

## 📑 Índice

1. [Convenções](#convenções)
2. [Autenticação](#autenticação)
3. [Endpoints de catálogo](#endpoints-de-catálogo)
   - [GET /v1/catalog/popular](#get-v1catalogpopular)
   - [GET /v1/catalog/search](#get-v1catalogsearch)
   - [GET /v1/catalog/resolve](#get-v1catalogresolve)
   - [GET /v1/catalog/movie/&lt;imdb&gt;](#get-v1catalogmovieimdb)
4. [Endpoints de resolução de vídeo](#endpoints-de-resolução-de-vídeo)
   - [GET /v1/resolve](#get-v1resolve)
   - [GET /v1/resolveEpisode](#get-v1resolveepisode)
   - [GET /v1/seasons](#get-v1seasons)
5. [Endpoints de provedores](#endpoints-de-provedores)
   - [GET /v1/providers](#get-v1providers)
6. [Endpoints administrativos](#endpoints-administrativos)
   - [GET /v1/health](#get-v1health)
   - [GET /v1/stats](#get-v1stats)
   - [GET /v1/logs](#get-v1logs)
   - [POST /v1/logs/clear](#post-v1logsclear)
   - [GET /v1/extractors](#get-v1extractors)
   - [GET /v1/routes](#get-v1routes)
   - [GET /v1/auth/plans](#get-v1authplans)
7. [Esquemas de dados](#esquemas-de-dados)
8. [Formato de erro](#formato-de-erro)
9. [Comportamento multi-provedor](#comportamento-multi-provedor)
10. [Integração com clientes](#integração-com-clientes)
11. [Rate limiting e cache](#rate-limiting-e-cache)
12. [Exemplos completos](#exemplos-completos)

---

## Convenções

- **Métodos HTTP**: cada endpoint aceita apenas os métodos listados. Outros
  métodos retornam `405 Method Not Allowed`.
- **Codificação**: todas as respostas são `application/json; charset=utf-8`.
- **Datas**: timestamps em ISO 8601 UTC (ex: `2026-06-28T13:50:00Z`).
- **Durações**: strings no formato Go (`1.234s`, `456ms`, `2.5s`).
- **Tamanhos**: em bytes (int64).
- **IDs externos**: IMDB IDs sempre começam com `tt` (ex: `tt0903747`).
- **Campos opcionais**: podem estar ausentes, vazios (`""`), ou zero (`0`)
  quando não aplicáveis. Sempre cheque com `if (field)` no cliente.
- **Campos `null` vs ausente**: omitidos quando vazios. Para `arrays`, um
  campo ausente deve ser tratado como array vazio `[]`.

---

## Autenticação

A maioria dos endpoints é **pública** (sem auth). Apenas endpoints mutantes
(`POST /v1/logs/clear` e futuros endpoints de admin) exigem um token
`X-Admin-Token` quando a variável de ambiente `ADMIN_TOKEN` está configurada
no servidor.

### Header

```
X-Admin-Token: <seu-token-aqui>
```

### Alternativa via query string

```
?admin_token=<seu-token-aqui>
```

Se o token estiver configurado mas não for enviado em um endpoint mutante,
a resposta será:

```json
{
  "error": "X-Admin-Token header required"
}
```

com HTTP 401.

---

## Endpoints de catálogo

Estes endpoints retornam **metadados** sobre títulos (filmes e séries):
título, ano, poster, backdrop, elenco, disponibilidade. Eles **não**
retornam URLs de vídeo diretas — para isso use os [Endpoints de resolução](#endpoints-de-resolução-de-vídeo).

---

### GET /v1/catalog/popular

Retorna uma lista curada de títulos populares, enriquecidos com metadados.

#### Query params

| Parâmetro | Tipo | Default | Descrição |
|---|---|---|---|
| `type` | string | `movies` | `movies` ou `tvshows` |
| `limit` | int | `80` | Quantidade máxima (1–200) |

#### Exemplo

```bash
curl 'http://localhost:8080/v1/catalog/popular?type=movies&limit=3'
```

#### Resposta 200

```json
{
  "type": "movies",
  "count": 3,
  "items": [
    {
      "imdb": "tt0111161",
      "type": "movie",
      "embed_type": "movies",
      "title_ptbr": "Um Sonho de Liberdade",
      "title_orig": "The Shawshank Redemption",
      "year": 1994,
      "poster_url": "https://m.media-amazon.com/images/M/MV5BMDAyY2FhYjctNDc5OS00MDNlLThiMGUtY2UxYWVkNGY2ZjljXkEyXkFqcGc@._V1_SX400_.jpg",
      "backdrop_url": "https://image.tmdb.org/t/p/original/kXfqcdQKsToO0OUXHcrrNCHDBzO.jpg",
      "cast": "Tim Robbins, Morgan Freeman",
      "rank": 78,
      "available": true,
      "server_count": 3,
      "provider": "supercine"
    },
    {
      "imdb": "tt0468569",
      "type": "movie",
      "embed_type": "movies",
      "title_ptbr": "Batman: O Cavaleiro das Trevas",
      "title_orig": "The Dark Knight",
      "year": 2008,
      "poster_url": "https://m.media-amazon.com/images/M/MV5BMTMxNTMwODM0NF5BMl5BanBnXkFtZTcwODAyMTk2Mw@@._V1_SX400_.jpg",
      "backdrop_url": "https://image.tmdb.org/t/p/original/dqK9Hag1054tghRQSqLSfrkvQnA.jpg",
      "cast": "Christian Bale, Heath Ledger",
      "rank": 100,
      "available": true,
      "server_count": 5,
      "provider": "supercine"
    }
  ]
}
```

#### Comportamento multi-provedor

Cada item é resolvido em paralelo (até 6 simultâneos) contra o provider
registry. Se o provedor principal não tiver o título, `available` será
`false` e `server_count` será `0`. Não há fallback automático para
metadados — se um provedor não conhece o IMDB ID, o item aparece marcado
como indisponível.

---

### GET /v1/catalog/search

Busca títulos por texto livre. Usa a API pública de sugestões do IMDB
(`v3.sg.media-imdb.com`) internamente, depois enriquece cada resultado com
disponibilidade via provider registry.

#### Query params

| Parâmetro | Tipo | Default | Descrição |
|---|---|---|---|
| `q` | string | _(obrigatório)_ | Termo de busca (mínimo 1 caractere) |
| `limit` | int | `12` | Quantidade máxima (1–30) |

#### Exemplo

```bash
curl 'http://localhost:8080/v1/catalog/search?q=batman&limit=3'
```

#### Resposta 200

```json
{
  "query": "batman",
  "count": 3,
  "items": [
    {
      "imdb": "tt1877830",
      "type": "movie",
      "embed_type": "movies",
      "title_ptbr": "Batman",
      "title_orig": "The Batman",
      "year": 2022,
      "poster_url": "https://m.media-amazon.com/images/..._V1_SX400_.jpg",
      "backdrop_url": "https://image.tmdb.org/t/p/original/...jpg",
      "cast": "Robert Pattinson, Zoë Kravitz",
      "rank": 351,
      "available": true,
      "server_count": 5,
      "provider": "supercine"
    }
  ]
}
```

#### Resposta 400 (sem query)

```json
{
  "error": "parâmetro 'q' é obrigatório"
}
```

---

### GET /v1/catalog/resolve

Resolve **um único IMDB ID** em metadados completos. Mais rápido que
`/search` porque não precisa buscar no IMDB (mas ainda enriquece com
título original, ano, poster e elenco).

#### Query params

| Parâmetro | Tipo | Default | Descrição |
|---|---|---|---|
| `imdb` | string | _(obrigatório)_ | IMDB ID, ex: `tt2250912` |
| `type` | string | `movies` | `movies` ou `tvshows` |

#### Exemplo

```bash
curl 'http://localhost:8080/v1/catalog/resolve?imdb=tt2250912&type=movies'
```

#### Resposta 200

```json
{
  "imdb": "tt2250912",
  "type": "movie",
  "embed_type": "movies",
  "title_ptbr": "Homem-Aranha: De Volta ao Lar",
  "title_orig": "Spider-Man: Homecoming",
  "year": 2017,
  "poster_url": "https://m.media-amazon.com/images/M/MV5BODY2MTAzOTQ4M15BMl5BanBnXkFtZTgwNzg5MTE0MjI@._V1_SX400_.jpg",
  "backdrop_url": "https://image.tmdb.org/t/p/original/fn4n6uOYcB6Uh89nbNPoU2w80RV.jpg",
  "cast": "Tom Holland, Michael Keaton",
  "rank": 351,
  "available": true,
  "server_count": 4,
  "provider": "supercine"
}
```

#### Resposta 400 (IMDB inválido)

```json
{
  "error": "parâmetro 'imdb' é obrigatório e deve começar com 'tt'"
}
```

---

### GET /v1/catalog/movie/&lt;imdb&gt;

Alias path-based para `/v1/catalog/resolve`. Útil para URLs mais limpas.

#### Exemplo

```bash
curl 'http://localhost:8080/v1/catalog/movie/tt2250912?type=movies'
```

Mesma resposta do `/v1/catalog/resolve`.

---

## Endpoints de resolução de vídeo

Estes endpoints retornam **URLs de vídeo diretas** (mp4, m3u8) prontas para
reprodução. Eles percorrem o provider registry em ordem de prioridade até
encontrar um provedor que tenha o título.

---

### GET /v1/resolve

Resolve um **filme** (ou conteúdo único) para URLs de vídeo diretas.
Tenta cada provedor em ordem de prioridade. Internamente cada provedor
testa até 3 servidores de hoster antes de desistir.

#### Query params

| Parâmetro | Tipo | Default | Descrição |
|---|---|---|---|
| `imdb` | string | _(obrigatório)_ | IMDB ID, ex: `tt0111161` |
| `type` | string | `movies` | `movies` ou `tvshows` |
| `provider` | string | _(vazio)_ | Provedor preferencial (ex: `supercine`). Se vazio, tenta todos em ordem. |

#### Exemplo

```bash
curl 'http://localhost:8080/v1/resolve?imdb=tt0111161&type=movies'
```

#### Resposta 200

```json
{
  "provider": "supercine",
  "imdb": "tt0111161",
  "type": "movies",
  "servers": [
    {
      "index": 0,
      "name": "Player Alternativo 4",
      "description": "[OK] Velocidade ok e poucos anúncios"
    },
    {
      "index": 1,
      "name": "Player Principal",
      "description": "Esee é o Top 1, rápido e poucos anúncios!"
    },
    {
      "index": 2,
      "name": "Player Alternativo",
      "description": "O 2º melhor muito rápido!"
    }
  ],
  "videos": [
    {
      "url": "https://dm545lq.cloudatacdn.com/u5kj62fyahd3sdgge5zbkiyxjfgv23enbzqrxartm6omcvz5r7yqraclkvfa/f1j2qtq.mp4?s=...&e=...&",
      "quality": "Normal"
    }
  ]
}
```

#### Campos da resposta

| Campo | Tipo | Descrição |
|---|---|---|
| `provider` | string | Nome do provedor que serviu (`supercine`, etc.) |
| `imdb` | string | IMDB ID resolvido |
| `type` | string | `movies` ou `tvshows` |
| `servers` | array | Lista de servidores disponíveis |
| `servers[].index` | int | Índice 0-based do servidor |
| `servers[].name` | string | Nome amigável (ex: "Player Principal") |
| `servers[].description` | string | Descrição. Prefixo `[OK]` indica o servidor que funcionou |
| `videos` | array | URLs diretas de vídeo |
| `videos[].url` | string | URL direta (mp4/m3u8) |
| `videos[].quality` | string | `Normal`, `MP4 Video`, `HLS (m3u8)`, etc. |

#### Resposta 502 (falha)

```json
{
  "error": "provider: title not available",
  "imdb": "tt0903747",
  "type": "tvshows"
}
```

---

### GET /v1/resolveEpisode

Resolve um **episódio específico** de uma série para URLs de vídeo diretas.
Disponível apenas para TV shows.

#### Query params

| Parâmetro | Tipo | Default | Descrição |
|---|---|---|---|
| `imdb` | string | _(obrigatório)_ | IMDB ID da série, ex: `tt0903747` |
| `season` | int | _(obrigatório)_ | Número da temporada (≥ 1) |
| `episode` | int | _(obrigatório)_ | Número do episódio (≥ 1) |
| `provider` | string | _(vazio)_ | Provedor preferencial |

#### Exemplo

```bash
curl 'http://localhost:8080/v1/resolveEpisode?imdb=tt0903747&season=1&episode=1'
```

#### Resposta 200

```json
{
  "provider": "supercine",
  "imdb": "tt0903747",
  "type": "tvshows",
  "season": 1,
  "episode": 1,
  "servers": [
    {
      "index": 0,
      "name": "Player Alternativo 3 (dublado)",
      "description": "[OK] mixdrop"
    },
    {
      "index": 1,
      "name": "Player Principal (dublado)",
      "description": "streamwish"
    },
    {
      "index": 2,
      "name": "Player Alternativo (dublado)",
      "description": "vidhide"
    },
    {
      "index": 3,
      "name": "Player Alternativo 4 (dublado)",
      "description": "doodstream"
    }
  ],
  "videos": [
    {
      "url": "https://a-delivery37.mxcontent.net/v2/gjod7rr9fwd47r3.mp4?s=...&e=...&",
      "quality": "Normal"
    }
  ]
}
```

#### Resposta 400 (parâmetros inválidos)

```json
{
  "error": "season e episode são obrigatórios e devem ser >= 1"
}
```

#### Resposta 502 (episódio não disponível)

```json
{
  "error": "provider: title not available",
  "imdb": "tt0903747",
  "season": 99,
  "episode": 99
}
```

---

### GET /v1/seasons

Lista todas as **temporadas e episódios** de uma série. Retorna metadados
riches para cada episódio (título, data, backdrop).

#### Query params

| Parâmetro | Tipo | Default | Descrição |
|---|---|---|---|
| `imdb` | string | _(obrigatório)_ | IMDB ID da série |

#### Exemplo

```bash
curl 'http://localhost:8080/v1/seasons?imdb=tt0903747'
```

#### Resposta 200

```json
{
  "imdb": "tt0903747",
  "status": "success",
  "season_count": 5,
  "seasons": [
    {
      "number": 1,
      "id": "14107",
      "episodes": [
        {
          "number": 1,
          "id": "14100",
          "title": "Piloto",
          "date": "20 de janeiro de 2008",
          "backdrop": "https://image.tmdb.org/t/p/w185/u90Ryx8OztC5OeVTXHPcZ8fnKoA.jpg"
        },
        {
          "number": 2,
          "id": "14101",
          "title": "The Cat's in the Bag",
          "date": "27 de janeiro de 2008",
          "backdrop": "https://image.tmdb.org/t/p/w185/xwQRVskT9IK7ktbrrWc2xoT4nPv.jpg"
        }
      ]
    },
    {
      "number": 2,
      "id": "14121",
      "episodes": [...]
    }
  ]
}
```

#### Campos da resposta

| Campo | Tipo | Descrição |
|---|---|---|
| `imdb` | string | IMDB ID da série |
| `status` | string | Sempre `"success"` em caso de sucesso |
| `season_count` | int | Número total de temporadas |
| `seasons` | array | Lista de temporadas |
| `seasons[].number` | int | Número da temporada (1-based) |
| `seasons[].id` | string | ID interno do provedor |
| `seasons[].episodes` | array | Lista de episódios da temporada |
| `seasons[].episodes[].number` | int | Número do episódio (1-based) |
| `seasons[].episodes[].id` | string | ID interno do episódio |
| `seasons[].episodes[].title` | string | Título (PT-BR quando disponível, senão original) |
| `seasons[].episodes[].date` | string | Data de exibição em PT-BR (pode ser vazia) |
| `seasons[].episodes[].backdrop` | string | URL do thumbnail (TMDB) |

#### Resposta 503 (sem provedor de séries)

```json
{
  "error": "nenhum provedor com suporte a séries está registrado"
}
```

---

## Endpoints de provedores

---

### GET /v1/providers

Lista todos os provedores registrados e seu status de saúde atual.

#### Exemplo

```bash
curl 'http://localhost:8080/v1/providers'
```

#### Resposta 200

```json
{
  "count": 1,
  "providers": [
    {
      "name": "supercine",
      "display_name": "Supercine",
      "priority": 100,
      "healthy": true
    }
  ]
}
```

#### Campos

| Campo | Tipo | Descrição |
|---|---|---|
| `name` | string | Identificador único (`supercine`, `megahdfilmes`, etc.) |
| `display_name` | string | Nome amigável para UI |
| `priority` | int | Ordem de tentativa (menor = maior prioridade) |
| `healthy` | bool | `true` se o `HealthCheck()` do provedor passou na última chamada |

#### Quando adicionar provedores

Novos provedores aparecem automaticamente nesta lista assim que são
registrados no `provider.Registry` em `cmd/server/main.go`. A UI de
streaming consome este endpoint para mostrar o status no canto superior
direito.

---

## Endpoints administrativos

Estes endpoints servem o dashboard admin em `/admin` e ferramentas de
monitoramento. Não são necessários para a UI de streaming.

---

### GET /v1/health

Health check simples do proxy.

#### Exemplo

```bash
curl 'http://localhost:8080/v1/health'
```

#### Resposta 200

```json
{
  "status": "ok",
  "version": "1.0.0",
  "upstream": "https://supercine-tv.net/wp-json",
  "cache": 0
}
```

---

### GET /v1/stats

Estatísticas agregadas desde o início do processo.

#### Resposta 200

```json
{
  "total_requests": 142,
  "cache_hits": 87,
  "cache_misses": 55,
  "errors": 3,
  "bytes_proxied": 892413,
  "by_status": {
    "2xx": 130,
    "4xx": 9,
    "5xx": 3
  },
  "by_path": {
    "/v1/catalog/popular": 42,
    "/v1/resolve": 18,
    "/v1/seasons": 12
  },
  "extractor_calls": 18,
  "extractor_errors": 1
}
```

---

### GET /v1/logs

Últimas N entradas de log (mais recente primeiro).

#### Query params

| Parâmetro | Tipo | Default | Descrição |
|---|---|---|---|
| `limit` | int | `100` | Quantidade (1–500) |

#### Resposta 200

```json
[
  {
    "id": "550e8400-e29b-41d4-a716-446655440000",
    "timestamp": "2026-06-28T13:50:00.123456789Z",
    "method": "GET",
    "path": "/v1/resolve",
    "upstream": "https://supercine-tv.net/embed-api/?imdb=tt0111161&type=movies",
    "status_code": 200,
    "duration": "1.234s",
    "size": 529,
    "cached": false,
    "error": ""
  }
]
```

---

### POST /v1/logs/clear

Limpa todos os logs e zera as estatísticas. Requer `X-Admin-Token` se
configurado.

#### Resposta 200

```json
{
  "status": "ok"
}
```

---

### GET /v1/extractors

Lista os 8 hosters suportados pelos extractors internos (DoodStream,
StreamWish, etc.). Estes são os scrapers que pegam uma URL de hoster
(`https://mixdrop.ps/e/abc123`) e devolvem a URL direta do mp4/m3u8.

#### Resposta 200

```json
[
  {"name": "doodstream"},
  {"name": "streamwish"},
  {"name": "vidhide"},
  {"name": "filemoon"},
  {"name": "filelions"},
  {"name": "mixdrop"},
  {"name": "streamtape"},
  {"name": "voe"}
]
```

---

### GET /v1/routes

Descoberta automática das rotas upstream (WordPress REST). Útil para debug
e para o API Explorer do admin.

#### Resposta 200

```json
{
  "name": "Supercine API",
  "namespaces": ["api", "auth", "inbox", "site", "wp/v2", ...],
  "routes": [
    {"path": "/api/filmes", "methods": ["GET"]},
    {"path": "/auth/login", "methods": ["POST"]},
    ...
  ]
}
```

---

### GET /v1/auth/plans

Wrapper dos planos de assinatura do Supercine. Não requer auth no proxy.

#### Resposta 200

```json
{
  "success": true,
  "plans": [
    {"id": "m1", "label": "1 Mês",   "days":  30, "amount":  19.90},
    {"id": "m3", "label": "3 Meses", "days":  90, "amount":  49.90},
    {"id": "a1", "label": "1 Ano",   "days": 365, "amount": 149.90}
  ]
}
```

---

## Esquemas de dados

### MovieMeta

Retornado por `/v1/catalog/*`. Metadados de um título.

```typescript
interface MovieMeta {
  imdb:          string;   // "tt0111161"
  type:          string;   // "movie" | "tv"
  embed_type:    string;   // "movies" | "tvshows"
  title_ptbr:    string;   // "Um Sonho de Liberdade" (do provider)
  title_orig:    string;   // "The Shawshank Redemption" (do IMDB)
  year:          number;   // 1994 (0 se desconhecido)
  poster_url:    string;   // URL do poster (IMDB images)
  backdrop_url:  string;   // URL do backdrop (TMDB via provider)
  cast:          string;   // "Tim Robbins, Morgan Freeman"
  rank:          number;   // popularidade IMDB (menor = mais popular)
  available:     boolean;  // true se algum provider tem o título
  server_count:  number;   // # de servidores disponíveis
  provider:      string;   // "supercine" | "" se indisponível
}
```

### ResolveResult

Retornado por `/v1/resolve` e `/v1/resolveEpisode`. URLs diretas de vídeo.

```typescript
interface ResolveResult {
  provider: string;
  imdb:     string;
  type:     string;     // "movies" | "tvshows"
  season?:  number;     // só em resolveEpisode
  episode?: number;     // só em resolveEpisode
  servers:  Server[];
  videos:   VideoURL[];
}

interface Server {
  index:       number;
  name:        string;
  description: string;   // "[OK] ..." indica o que funcionou
}

interface VideoURL {
  url:     string;
  quality: string;   // "Normal" | "MP4 Video" | "HLS (m3u8)"
}
```

### SeasonsResponse

Retornado por `/v1/seasons`.

```typescript
interface SeasonsResponse {
  imdb:          string;
  status:        "success";
  season_count:  number;
  seasons:       Season[];
}

interface Season {
  number:   number;       // 1-based
  id:       string;       // ID interno do provider
  episodes: Episode[];
}

interface Episode {
  number:   number;       // 1-based
  id:       string;       // ID interno do provider
  title:    string;       // PT-BR quando disponível
  date:     string;       // PT-BR human-readable
  backdrop: string;       // TMDB URL
}
```

### ProviderInfo

Retornado por `/v1/providers`.

```typescript
interface ProviderInfo {
  name:         string;   // "supercine"
  display_name: string;   // "Supercine"
  priority:     number;   // 100 (menor = tentado primeiro)
  healthy:      boolean;  // HealthCheck() OK na última chamada
}
```

### LogEntry

Retornado por `/v1/logs`.

```typescript
interface LogEntry {
  id:          string;   // UUID
  timestamp:   string;   // ISO 8601
  method:      string;   // "GET" | "POST" | ...
  path:        string;   // "/v1/resolve"
  upstream:    string;   // URL do upstream chamada
  status_code: number;   // 200, 404, 502, ...
  duration:    string;   // "1.234s"
  size:        number;   // bytes
  cached:      boolean;  // servido do cache?
  error:       string;   // mensagem de erro (vazio se OK)
}
```

### Stats

Retornado por `/v1/stats`.

```typescript
interface Stats {
  total_requests:    number;
  cache_hits:        number;
  cache_misses:      number;
  errors:            number;
  bytes_proxied:     number;
  by_status:         Record<string, number>;  // "2xx": 130, ...
  by_path:           Record<string, number>;  // "/v1/resolve": 18, ...
  extractor_calls:   number;
  extractor_errors:  number;
}
```

---

## Formato de erro

Todos os erros seguem o mesmo formato. HTTP status code reflete a categoria:

| HTTP | Quando |
|---|---|
| `400` | Parâmetro inválido ou ausente |
| `401` | Token admin necessário mas não enviado |
| `404` | Rota não encontrada |
| `405` | Método não permitido |
| `500` | Erro interno do proxy |
| `502` | Erro de comunicação com upstream/provedor |
| `503` | Serviço indisponível (sem provedor registrado, etc.) |

### Body

```json
{
  "error": "mensagem human-readable em PT-BR"
}
```

### Exemplos

**Parâmetro ausente (400):**
```json
{"error": "parâmetro 'imdb' é obrigatório e deve começar com 'tt'"}
```

**Provedor offline (502):**
```json
{"error": "provider: upstream unreachable"}
```

**Sem provedor (503):**
```json
{"error": "nenhum provedor com suporte a séries está registrado"}
```

### Códigos de erro específicos do provider

O campo `error` pode conter mensagens do provider. Os três sentinelas são:

| Mensagem | Significado |
|---|---|
| `provider: title not available` | Provedor não tem este título no catálogo |
| `provider: upstream unreachable` | Provedor offline (rede, DNS, 5xx) |
| `provider: no provider registered for this request` | Registry vazio |

Use correspondência exata de string para detectar esses casos no cliente.

---

## Comportamento multi-provedor

### Resolução de vídeos (`/v1/resolve`, `/v1/resolveEpisode`)

Quando um cliente pede um título:

1. Se `provider=X` foi especificado, o registry tenta **X primeiro**.
2. Se X falhar (offline, sem o título, extractor falhou), o registry
   tenta cada outro provedor em ordem crescente de `priority`.
3. O primeiro provedor que retornar **pelo menos 1 URL de vídeo** vence.
4. A resposta inclui `"provider": "supercine"` indicando quem serviu.

```
Cliente: GET /v1/resolve?imdb=tt0111161&type=movies
            │
            ▼
    Provider Registry
            │
            ├─▶ 1. Tenta Supercine (priority=100)
            │       ├─▶ FetchEmbed("tt0111161", "movies")
            │       ├─▶ 3 servers retornados
            │       ├─▶ Tenta server 0: mixdrop → ✓ mp4 direto
            │       └─▶ SUCESSO! Retorna
            │
            └─▶ (não tenta outros provedores porque o primeiro deu OK)
```

### Metadados de catálogo (`/v1/catalog/*`)

Para metadados, o registry **não faz fallback**. Apenas o provedor
principal (Supercine hoje) é consultado. Razão: a maioria dos provedores
não tem endpoint de catálogo próprio — eles apenas resolvem IMDB IDs.

Se você quiser um catálogo agregado no futuro, precisará adicionar um
endpoint `/v1/catalog/aggregate` que dispara contra múltiplos provedores
e mescla resultados.

### Health check

Cada provedor implementa `HealthCheck(ctx)`. O endpoint `/v1/providers`
chama todos em paralelo com timeout de 10s e retorna o status.

---

## Integração com clientes

### App Flutter / React Native / Electron

```typescript
// Exemplo: buscar e reproduzir um filme

async function playMovie(imdbId: string) {
  // 1. Resolver metadados
  const meta = await fetch(
    `https://meu-proxy.com/v1/catalog/resolve?imdb=${imdbId}&type=movies`
  ).then(r => r.json());

  if (!meta.available) {
    alert('Título indisponível');
    return;
  }

  // 2. Resolver URL direta
  const result = await fetch(
    `https://meu-proxy.com/v1/resolve?imdb=${imdbId}&type=movies`
  ).then(r => r.json());

  if (!result.videos?.length) {
    alert('Falha ao extrair vídeo');
    return;
  }

  // 3. Reproduzir (exemplo com video.js)
  const player = videojs('my-video');
  player.src({
    src: result.videos[0].url,
    type: result.videos[0].quality === 'HLS (m3u8)' ? 'application/x-mpegURL' : 'video/mp4'
  });
  player.play();
}
```

### App Android (Kotlin + ExoPlayer)

```kotlin
suspend fun resolveAndPlay(imdbId: String, type: String) {
    // 1. Resolve metadata
    val meta = api.getMovieMeta(imdbId, type)
    if (!meta.available) {
        showError("Indisponível")
        return
    }

    // 2. Resolve direct URL
    val result = api.resolveVideo(imdbId, type)
    if (result.videos.isEmpty()) {
        showError("Falha ao extrair")
        return
    }

    // 3. Play with ExoPlayer
    val dataSource = result.videos[0].url
    val mediaItem = MediaItem.fromUri(dataSource)
    exoPlayer.setMediaItem(mediaItem)
    exoPlayer.prepare()
    exoPlayer.playWhenReady = true
}
```

### Script CLI (curl + jq)

```bash
# Buscar um filme e imprimir o link direto
curl -s "http://localhost:8080/v1/catalog/search?q=matrix" \
  | jq '.items[0] | {title: .title_ptbr, year, imdb}'

# Resolver URL direta
curl -s "http://localhost:8080/v1/resolve?imdb=tt0133093&type=movies" \
  | jq '.videos[0].url' -r \
  | xargs vlc
```

### Pontos importantes para clientes

1. **Sempre trate `available: false`**. Mesmo que um título apareça na
   busca, ele pode não ter servidores disponíveis no momento.
2. **Use HLS.js para m3u8**. Browsers sem suporte nativo a HLS (Firefox,
   Chrome) precisam de hls.js. Safari tem suporte nativo.
3. **Tenha retry de servidor**. Mesmo que `/v1/resolve` retorne 3
   servidores, o vídeo do servidor 0 pode falhar (URL expirada, hoster
   offline). Faça fallback clicando no próximo servidor.
4. **Cache no cliente**. Metadados não mudam — cacheie `MovieMeta` por
   pelo menos 1h no localStorage. URLs de vídeo **não** devem ser
   cacheadas (expiram em minutos).
5. **Timeout**. Use timeout de 30s para `/resolve` e 60s para `/popular`
   (que resolve 80 títulos em paralelo).

---

## Rate limiting e cache

### Cache do servidor

| Endpoint | TTL | Condição |
|---|---|---|
| `/v1/catalog/popular` | 5 min | Configurável via `CACHE_TTL` |
| `/v1/catalog/search` | 5 min | Por query |
| `/v1/catalog/resolve` | 5 min | Por imdb+type |
| `/v1/seasons` | 5 min | Por imdb |
| `/v1/resolve` | 0 | Não cacheia (URLs expiram) |
| `/v1/resolveEpisode` | 0 | Não cacheia |
| `/v1/providers` | 0 | Sempre consulta provedores |

### Rate limiting

O proxy **não implementa rate limiting** próprio. Se você expor publicamente,
coloque um nginx/Cloudflare na frente com rate limit por IP.

### Timeout por endpoint

| Endpoint | Timeout |
|---|---|
| `/v1/catalog/popular` | 90s |
| `/v1/catalog/search` | 60s |
| `/v1/catalog/resolve` | 30s |
| `/v1/resolve` | 45s |
| `/v1/resolveEpisode` | 45s |
| `/v1/seasons` | 30s |
| `/v1/providers` | 10s |

### Variáveis de ambiente do servidor

| Variável | Default | Descrição |
|---|---|---|
| `LISTEN_ADDR` | `:8080` | Endereço de bind |
| `UPSTREAM_BASE` | `https://supercine-tv.net/wp-json` | Base do upstream WordPress |
| `EMBED_BASE` | `https://supercine-tv.net/embed-api/` | Base do endpoint de embed |
| `USER_AGENT` | `Mozilla/5.0 ... Chrome/137` | UA enviado ao upstream |
| `CACHE_TTL` | `5m` | TTL do cache in-memory |
| `CACHE_MAX_ENTRIES` | `1000` | Máximo de entradas no cache |
| `LOG_MAX_ENTRIES` | `500` | Máximo de logs em memória |
| `REQUEST_TIMEOUT` | `20s` | Timeout HTTP upstream |
| `ADMIN_TOKEN` | _(vazio)_ | Token para endpoints mutantes |
| `VERBOSE` | `false` | Log detalhado |

---

## Exemplos completos

### Fluxo completo: do catálogo à reprodução

```bash
# 1. Listar filmes populares
curl -s 'http://localhost:8080/v1/catalog/popular?type=movies&limit=5' | jq '.items[] | .title_ptbr'

# 2. Buscar um filme específico
curl -s 'http://localhost:8080/v1/catalog/search?q=interestelar' | jq '.items[0]'

# 3. Resolver metadados
curl -s 'http://localhost:8080/v1/catalog/resolve?imdb=tt0816692&type=movies' | jq

# 4. Resolver URL direta
curl -s 'http://localhost:8080/v1/resolve?imdb=tt0816692&type=movies' | jq '.videos[0]'

# 5. Reproduzir (requer player de vídeo)
curl -s 'http://localhost:8080/v1/resolve?imdb=tt0816692&type=movies' | jq -r '.videos[0].url' | xargs vlc
```

### Fluxo completo: série com episódios

```bash
# 1. Buscar série
curl -s 'http://localhost:8080/v1/catalog/search?q=breaking+bad' | jq '.items[0]'

# 2. Listar temporadas
curl -s 'http://localhost:8080/v1/seasons?imdb=tt0903747' | jq '.seasons[] | {season: .number, episodes: (.episodes | length)}'

# 3. Ver episódios da temporada 1
curl -s 'http://localhost:8080/v1/seasons?imdb=tt0903747' | jq '.seasons[] | select(.number == 1) | .episodes[] | {ep: .number, title}'

# 4. Resolver episódio S1E1
curl -s 'http://localhost:8080/v1/resolveEpisode?imdb=tt0903747&season=1&episode=1' | jq '.videos[0]'

# 5. Reproduzir
curl -s 'http://localhost:8080/v1/resolveEpisode?imdb=tt0903747&season=1&episode=1' | jq -r '.videos[0].url' | xargs vlc
```

### Fluxo completo: monitoramento

```bash
# 1. Health check
curl -s 'http://localhost:8080/v1/health' | jq

# 2. Status dos provedores
curl -s 'http://localhost:8080/v1/providers' | jq

# 3. Estatísticas
curl -s 'http://localhost:8080/v1/stats' | jq

# 4. Últimos 5 logs
curl -s 'http://localhost:8080/v1/logs?limit=5' | jq '.[] | {path, status_code, duration, error}'

# 5. Limpar logs (requer admin token se configurado)
curl -s -X POST 'http://localhost:8080/v1/logs/clear' -H 'X-Admin-Token: seu-token' | jq
```

### Fallback de servidor no cliente

```javascript
async function resolveWithFallback(imdb, type) {
  const r = await fetch(`/v1/resolve?imdb=${imdb}&type=${type}`);
  const j = await r.json();

  if (!j.videos || j.videos.length === 0) {
    throw new Error('Sem vídeo disponível');
  }

  // Tenta o primeiro vídeo. Se falhar, pede re-resolução.
  for (let i = 0; i < j.servers.length; i++) {
    const video = j.videos[0]; // o proxy já tentou 3 servidores internamente
    try {
      await tryPlay(video.url);
      return;
    } catch (e) {
      console.log(`Servidor ${i} falhou: ${e.message}`);
      // Re-fetch com provider diferente
      // (não há parâmetro de índice de servidor hoje; este é um placeholder)
    }
  }
  throw new Error('Todos os servidores falharam');
}
```

---

## Changelog da API

### v1.3.0 (atual)
- ✨ Adicionado `GET /v1/seasons`
- ✨ Adicionado `GET /v1/resolveEpisode`
- 🔧 `/v1/resolve` agora retorna `videos[]` preenchido (testa 3 servers internamente)

### v1.2.0
- ✨ Adicionado `GET /v1/providers`
- ✨ Adicionado `POST /v1/resolve` (além do GET)
- 🔧 `/v1/catalog/popular` agora aceita `type=movies|tvshows` separadamente

### v1.1.0
- ✨ Adicionados endpoints `/v1/catalog/*`
- ✨ UI de streaming em `/`

### v1.0.0
- 🎉 Release inicial
- Endpoints `/v1/health`, `/v1/stats`, `/v1/logs`, `/v1/extract`, `/v1/embed`,
  `/v1/auth/plans`, `/v1/routes`

---

## Próximos passos (futuro)

Endpoints planejados para versões futuras:

- `GET /v1/catalog/aggregate` — busca em múltiplos provedores e mescla
- `POST /v1/favorites` — lista de favoritos persistida no proxy
- `GET /v1/history` — histórico de reprodução por device
- `WS /v1/events` — WebSocket para updates em tempo real (novo episódio, etc.)
- `GET /v1/subtitles?imdb=...&season=...&episode=...` — legendas

Estes endpoints ainda não existem. Assim que implementados, esta doc será
atualizada.

---

**Fim da documentação da Output API v1.3.0.**

Para a documentação da API interna do Supercine (upstream), veja
[`UPSTREAM_API.md`](UPSTREAM_API.md). Para a análise do APK original,
veja [`APK_ANALYSIS.md`](APK_ANALYSIS.md).
