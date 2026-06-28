# Supercine Proxy — Output API

> **Versão:** 1.4.0
> **Base URL:** `http://localhost:8080` (local) ou `https://supercine-proxy.onrender.com` (deploy Render)
> **Formato:** JSON (UTF-8) para endpoints de metadados; streaming binário para `/v1/stream`
> **Idioma das mensagens:** Português brasileiro (PT-BR)

Esta é a **API unificada de saída** do proxy. Independente de qual provedor
de streaming esteja ativo no backend (hoje apenas Supercine, mas a arquitetura
suporta adicionar MegaHD, Jellyfin, etc. sem mudar a API), todas as respostas
desta documentação **sempre terão o mesmo formato**.

Clientes (app mobile, web UI, scripts) só precisam conhecer esta API — nunca
a API interna de cada provedor. Quando novos provedores forem adicionados, a
única mudança visível será o campo `provider` em algumas respostas, indicando
qual provedor serviu o conteúdo.

> ⚠️ **Importante para web apps**: os CDNs dos hosters (MixDrop, StreamWish,
> VidHide) rejeitam requisições do navegador que não carregam o
> `Origin`/`Referer` do próprio hoster. Por isso, **sempre use o endpoint
> [`/v1/stream`](#get-v1stream) para reproduzir vídeos no navegador** em vez
> de apontar o `<video>` diretamente para a URL retornada por `/v1/resolve`.
> Clientes nativos (ExoPlayer, VLC, mpv) não têm essa restrição e podem
> usar a URL direta.

---

## 📑 Índice

1. [Convenções](#convenções)
2. [Autenticação](#autenticação)
3. [Quick start — rodando localmente](#quick-start--rodando-localmente)
4. [Endpoints de catálogo](#endpoints-de-catálogo)
   - [GET /v1/catalog/popular](#get-v1catalogpopular)
   - [GET /v1/catalog/home](#get-v1cataloghome)
   - [GET /v1/catalog/search](#get-v1catalogsearch)
   - [GET /v1/catalog/resolve](#get-v1catalogresolve)
   - [GET /v1/catalog/movie/&lt;imdb&gt;](#get-v1catalogmovieimdb)
5. [Endpoints de resolução de vídeo](#endpoints-de-resolução-de-vídeo)
   - [GET /v1/resolve](#get-v1resolve)
   - [GET /v1/resolveEpisode](#get-v1resolveepisode)
   - [GET /v1/seasons](#get-v1seasons)
   - [GET /v1/stream](#get-v1stream) ⭐ proxy de reprodução
6. [Endpoints de provedores](#endpoints-de-provedores)
   - [GET /v1/providers](#get-v1providers)
7. [Endpoints administrativos](#endpoints-administrativos)
   - [GET /v1/health](#get-v1health)
   - [GET /v1/stats](#get-v1stats)
   - [GET /v1/logs](#get-v1logs)
   - [POST /v1/logs/clear](#post-v1logsclear)
   - [GET /v1/extractors](#get-v1extractors)
   - [GET /v1/routes](#get-v1routes)
   - [GET /v1/auth/plans](#get-v1authplans)
8. [Esquemas de dados](#esquemas-de-dados)
9. [Formato de erro](#formato-de-erro)
10. [Comportamento multi-provedor](#comportamento-multi-provedor)
11. [Integração com clientes](#integração-com-clientes)
12. [Rate limiting e cache](#rate-limiting-e-cache)
13. [Exemplos completos](#exemplos-completos)

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

## Quick start — rodando localmente

Esta seção mostra como subir o proxy localmente em menos de 1 minuto e
fazer a primeira chamada de ponta a ponta (catálogo → resolução → stream).

### Pré-requisitos

- **Go 1.23+** instalado ([https://go.dev/dl/](https://go.dev/dl/))
- **Git** para clonar o repositório
- Acesso à internet (o proxy precisa falar com `supercine-tv.net` e os
  CDNs dos hosters)

### Passo 1 — Clonar e rodar

```bash
git clone https://github.com/deivid22srk/supercine-proxy.git
cd supercine-proxy

# Roda em foreground (Ctrl+C para parar)
go run ./cmd/server

# Ou compila um binário estático e roda
go build -o supercine-proxy ./cmd/server
./supercine-proxy
```

Saída esperada:

```
2026/06/28 18:00:00 [boot] supercine-proxy 1.0.0 starting
2026/06/28 18:00:00 [boot] config: listen=:8080 upstream=https://supercine-tv.net/wp-json ...
2026/06/28 18:00:00 [boot] registered provider: supercine (priority=100)
2026/06/28 18:00:00 [boot] listening on http://:8080
```

O proxy sobe em `http://localhost:8080`.

### Passo 2 — Health check

```bash
curl http://localhost:8080/v1/health
# {"cache":0,"status":"ok","upstream":"https://supercine-tv.net/wp-json","version":"1.0.0"}
```

### Passo 3 — Buscar um filme

```bash
curl -s 'http://localhost:8080/v1/catalog/search?q=interestelar&limit=1' | jq
```

```json
{
  "query": "interestelar",
  "count": 1,
  "items": [
    {
      "imdb": "tt0816692",
      "type": "movie",
      "embed_type": "movies",
      "title_ptbr": "Interestelar",
      "title_orig": "Interstellar",
      "year": 2014,
      "available": true,
      "server_count": 7,
      "provider": "supercine"
    }
  ]
}
```

### Passo 4 — Resolver URL de vídeo

```bash
curl -s 'http://localhost:8080/v1/resolve?imdb=tt0816692&type=movies' | jq '.videos[0]'
```

```json
{
  "url": "https://qCsH3mKRD1SG2.premilkyway.com/hls2/01/02446/sl48hlzrfb4b_n/master.m3u8?t=...&s=...&e=129600&f=...&srv=...&i=0.4&sp=500&p1=...&p2=...&asn=7029",
  "quality": "HLS (m3u8)"
}
```

### Passo 5 — Reproduzir no navegador (via proxy de stream)

Como o CDN do hoster rejeita `Origin` estrangeiro, web apps **devem** usar
`/v1/stream?url=...` em vez da URL direta. Em um arquivo `demo.html`:

```html
<!DOCTYPE html>
<html>
<head>
  <title>Supercine demo</title>
  <script src="https://cdn.jsdelivr.net/npm/hls.js@1.5.17/dist/hls.min.js"></script>
</head>
<body>
  <video id="v" controls width="800"></video>
  <script>
    const API = 'http://localhost:8080';

    (async () => {
      // 1. Resolve o filme
      const r = await fetch(`${API}/v1/resolve?imdb=tt0816692&type=movies`);
      const j = await r.json();
      const videoURL = j.videos[0].url;

      // 2. Reproduz via proxy /v1/stream (contorna o bloqueio de Origin)
      const proxied = `${API}/v1/stream?url=${encodeURIComponent(videoURL)}`;
      const video = document.getElementById('v');

      if (videoURL.includes('.m3u8') && window.Hls && Hls.isSupported()) {
        const hls = new Hls();
        hls.loadSource(proxied);
        hls.attachMedia(video);
        hls.on(Hls.Events.MANIFEST_PARSED, () => video.play());
      } else {
        video.src = proxied;
        video.play();
      }
    })();
  </script>
</body>
</html>
```

Abra o `demo.html` no navegador — o vídeo vai tocar. Para mp4 direto
(MixDrop), o mesmo código funciona sem hls.js.

### Passo 6 — Reproduzir fora do navegador (VLC, mpv, ExoPlayer)

Clientes nativos não enviam `Origin`, então podem usar a URL direta
retornada por `/v1/resolve` sem o proxy:

```bash
# Linux/Mac: pega a URL e abre no VLC
curl -s 'http://localhost:8080/v1/resolve?imdb=tt0816692&type=movies' \
  | jq -r '.videos[0].url' \
  | xargs vlc

# Ou joga a URL num arquivo .m3u8 e abre no mpv
curl -s 'http://localhost:8080/v1/resolve?imdb=tt0816692&type=movies' \
  | jq -r '.videos[0].url' > /tmp/interstellar.m3u8
mpv /tmp/interstellar.m3u8
```

### Passo 7 — Variáveis de ambiente úteis

```bash
# Mudar a porta
LISTEN_ADDR=:9000 go run ./cmd/server

# Log verbeto (mostra cada chamada upstream)
VERBOSE=true go run ./cmd/server

# Proteger endpoints mutantes com token
ADMIN_TOKEN=segredo go run ./cmd/server
```

Lista completa em [Rate limiting e cache → Variáveis de ambiente](#variáveis-de-ambiente-do-servidor).

### Deploy no Render

O projeto já está deployado em `https://supercine-proxy.onrender.com` no
plano **free** (cold start de ~30s após 15 min de inatividade). Para testar
o deploy sem rodar localmente, basta trocar `http://localhost:8080` por
`https://supercine-proxy.onrender.com` em qualquer exemplo desta doc:

```bash
curl 'https://supercine-proxy.onrender.com/v1/health'
curl 'https://supercine-proxy.onrender.com/v1/resolve?imdb=tt0816692&type=movies'
```

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

### GET /v1/catalog/home

Retorna as **4 linhas de destaque da home** do Supercine (Lançamentos,
Destaques, Recentes, Sugeridos), cada uma com 12 itens. É o endpoint que
a UI de streaming em `/` consome para montar a tela inicial estilo
Netflix. Diferente do `/popular` (que resolve um catálogo curado contra
o provedor), o `/home` espelha exatamente o que o app original mostra na
home — então os títulos sempre têm `available: true` desde que o
provedor esteja online.

#### Query params

| Parâmetro | Tipo | Default | Descrição |
|---|---|---|---|
| `type` | string | `movies` | `movies` ou `tvshows` |

#### Exemplo

```bash
curl 'http://localhost:8080/v1/catalog/home?type=movies'
```

#### Resposta 200

```json
{
  "type": "movies",
  "count": 4,
  "rows": [
    {
      "category": "lancamentos",
      "label": "🔥 Lançamentos",
      "count": 12,
      "items": [
        {
          "imdb": "tt42192165",
          "type": "movie",
          "embed_type": "movies",
          "title_ptbr": "O Assassinato de Rachel Nickell",
          "title_orig": "",
          "year": 2026,
          "poster_url": "https://image.tmdb.org/t/p/w300/ArCOJVC5fYvmsxEyzoxvgugJnon.jpg",
          "backdrop_url": "https://image.tmdb.org/t/p/w500/yKeYZOmBMQUMKpMlJJF8q4riYpl.jpg",
          "cast": "",
          "rank": 0,
          "available": true,
          "server_count": 0,
          "provider": "supercine",
          "imdb_rating": 6.8,
          "runtime": "96",
          "categories": ["Crime", "Documentário", "Lançamentos"],
          "post_id": "1447376"
        }
      ]
    },
    {
      "category": "destaques",
      "label": "⭐ Destaques",
      "count": 12,
      "items": [ ... ]
    },
    {
      "category": "recentes",
      "label": "🆕 Recentes",
      "count": 12,
      "items": [ ... ]
    },
    {
      "category": "sugeridos",
      "label": "💡 Sugeridos",
      "count": 12,
      "items": [ ... ]
    }
  ]
}
```

#### Campos extras (vs. `MovieMeta`)

Cada item de `/home` retorna os mesmos campos de `MovieMeta`, mais:

| Campo | Tipo | Descrição |
|---|---|---|
| `imdb_rating` | number | Nota IMDB (0–10) quando retornada pelo provedor |
| `runtime` | string | Duração em minutos como string (ex: `"96"`) |
| `categories` | array | Lista de categorias PT-BR (ex: `["Crime","Drama"]`) |
| `post_id` | string | ID interno do post no Supercine |

> **Nota**: `server_count` vem como `0` e `title_orig` como `""` porque
> o endpoint `/home` do Supercine não retorna esses campos. Se você
> precisa deles, chame `/v1/catalog/resolve?imdb=...` para o item
> específico.

#### Resposta 503 (sem provedor)

```json
{
  "error": "nenhum provedor com home endpoint está registrado"
}
```

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

### GET /v1/stream

⭐ **Proxy de reprodução de vídeo.** Essencial para web apps — sem ele o
navegador recebe 403 dos CDNs dos hosters.

Os CDNs dos hosters (MixDrop `*.mxcontent.net`, StreamWish `*.premilkyway.com`,
VidHide `*.cdn-centaurus.com`) rejeitam requisições com `Origin` estrangeiro.
Quando um `<video>` ou `hls.js` no navegador tenta tocar a URL direta
retornada por `/v1/resolve`, o navegador envia
`Origin: https://seu-site.com`, que o CDN rejeita com `403 Forbidden`.

O `/v1/stream` resolve isso fazendo o fetch server-side com o
`Origin`/`Referer` corretos e streamando a resposta de volta para o
navegador com CORS permissivo.

```
navegador ──>  /v1/stream?url=<cdn_url>  ──>  CDN (com Origin do hoster)
       <──  vídeo/mp4 ou video/MP2T       <──
```

#### Query params

| Parâmetro | Tipo | Default | Descrição |
|---|---|---|---|
| `url` | string | _(obrigatório)_ | URL de vídeo retornada por `/v1/resolve` ou `/v1/resolveEpisode` (mp4 ou m3u8) |

#### Comportamento por tipo de URL

| Tipo de URL | Content-Type retornado | Comportamento |
|---|---|---|
| mp4/mkv direto | `video/mp4` | Stream binário com suporte a `Range` (seek). HTTP 206 se o navegador pedir range. |
| m3u8 (master) | `application/vnd.apple.mpegurl` | **Reescreve** a playlist: cada segmento `.ts` e cada atributo `URI="..."` dentro das tags HLS (`#EXT-X-MEDIA`, `#EXT-X-I-FRAME-STREAM-INF`) é convertido para `/v1/stream?url=...` |
| m3u8 (variant) | `application/vnd.apple.mpegurl` | Mesma reescrita — segmentos `.ts` viram URLs proxy |
| segmento `.ts` | `video/MP2T` | Stream binário direto |

#### Headers repassados do navegador para o CDN

| Header | Valor |
|---|---|
| `User-Agent` | Chrome 137 (fixo no servidor) |
| `Range` | Repassado do navegador (para seek em mp4) |
| `Origin` | Inferido do host da URL (ex: `https://mixdrop.ps` para `*.mxcontent.net`) |
| `Referer` | `<origin>/` (ex: `https://mixdrop.ps/`) |
| `Accept` | `*/*` |
| `Accept-Language` | `pt-BR,pt;q=0.9,en;q=0.8` |

#### Headers adicionados na resposta ao navegador

| Header | Valor |
|---|---|
| `Access-Control-Allow-Origin` | `*` |
| `Access-Control-Allow-Methods` | `GET, HEAD, OPTIONS` |
| `Access-Control-Allow-Headers` | `Range` |
| `Access-Control-Expose-Headers` | `Content-Length, Content-Range, Accept-Ranges` |
| `Accept-Ranges` | `bytes` |
| `Cache-Control` | `public, max-age=3600` |

#### Exemplo 1 — reproduzir mp4 (MixDrop) no `<video>`

```bash
# 1. Resolver o filme
curl -s 'http://localhost:8080/v1/resolve?imdb=tt0137523&type=movies' | jq '.videos[0].url'
# -> https://a-delivery33.mxcontent.net/v2/k013ew6esgvnxn.mp4?s=...&e=...

# 2. Encapsular na URL do proxy
ENC=$(python3 -c "import urllib.parse,sys; print(urllib.parse.quote(sys.argv[1]))" \
  "https://a-delivery33.mxcontent.net/v2/k013ew6esgvnxn.mp4?s=...")
curl -sI "http://localhost:8080/v1/stream?url=$ENC" -H 'Range: bytes=0-1'
# HTTP/1.1 206 Partial Content
# Content-Type: video/mp4
# Content-Range: bytes 0-1/...
# Accept-Ranges: bytes
# Access-Control-Allow-Origin: *
```

```html
<!-- HTML -->
<video id="v" controls></video>
<script>
  const API = 'http://localhost:8080';
  (async () => {
    const r = await fetch(`${API}/v1/resolve?imdb=tt0137523&type=movies`);
    const j = await r.json();
    const proxied = `${API}/v1/stream?url=${encodeURIComponent(j.videos[0].url)}`;
    document.getElementById('v').src = proxied;
    document.getElementById('v').play();
  })();
</script>
```

#### Exemplo 2 — reproduzir HLS (StreamWish) com hls.js

```bash
# 1. Resolver
curl -s 'http://localhost:8080/v1/resolve?imdb=tt2250912&type=movies' | jq '.videos[0].url'
# -> https://vwyn3lxe5fv5.cdn-centaurus.com/hls2/01/02526/8mrpm9z6cas0_n/master.m3u8?...

# 2. Proxy reescreve a playlist
ENC=$(python3 -c "import urllib.parse,sys; print(urllib.parse.quote(sys.argv[1]))" \
  "https://vwyn3lxe5fv5.cdn-centaurus.com/.../master.m3u8?...")
curl -s "http://localhost:8080/v1/stream?url=$ENC" | head -5
# #EXTM3U
# #EXT-X-MEDIA:...,URI="/v1/stream?url=https%3A%2F%2F...%2Findex-a1.m3u8%3F..."
# #EXT-X-STREAM-INF:...
# /v1/stream?url=https%3A%2F%2F...%2Findex-v1-a2.m3u8%3F...
```

```html
<script src="https://cdn.jsdelivr.net/npm/hls.js@1.5.17/dist/hls.min.js"></script>
<video id="v" controls></video>
<script>
  const API = 'http://localhost:8080';
  (async () => {
    const r = await fetch(`${API}/v1/resolve?imdb=tt2250912&type=movies`);
    const j = await r.json();
    const proxied = `${API}/v1/stream?url=${encodeURIComponent(j.videos[0].url)}`;
    const video = document.getElementById('v');
    if (window.Hls && Hls.isSupported()) {
      const hls = new Hls();
      hls.loadSource(proxied);   // hls.js vai buscar segmentos via /v1/stream
      hls.attachMedia(video);
      hls.on(Hls.Events.MANIFEST_PARSED, () => video.play());
    } else {
      video.src = proxied;       // Safari nativo
      video.play();
    }
  })();
</script>
```

#### Resposta 400 (URL ausente)

```json
{
  "error": "parâmetro 'url' é obrigatório"
}
```

#### Resposta 400 (URL inválida)

```json
{
  "error": "URL inválida — deve ser http ou https"
}
```

#### Resposta 502 (CDN offline)

```json
{
  "error": "falha ao buscar o stream do CDN",
  "detail": "dial tcp: lookup a-delivery33.mxcontent.net: no such host",
  "url": "https://a-delivery33.mxcontent.net/v2/abc.mp4?s=..."
}
```

#### Resposta 4xx/5xx (CDN rejeitou — body em JSON)

Se o CDN retornar 403/404/500, o proxy repassa o status code e inclui o
body do CDN como `detail`:

```json
{
  "error": "CDN retornou HTTP 403",
  "detail": "<html><body>403 Forbidden</body></html>",
  "url": "https://..."
}
```

#### Quando NÃO usar o proxy

Clientes **nativos** que não enviam `Origin` (ExoPlayer no Android,
AVPlayer no iOS, VLC, mpv, FFmpeg) podem usar a URL direta sem o proxy.
Isso economiza largura de banda do servidor do proxy e reduz latência:

```kotlin
// Android ExoPlayer — pode usar a URL direta
val result = api.resolveVideo("tt2250912", "movies")
val mediaItem = MediaItem.fromUri(result.videos[0].url)  // URL direta, sem proxy
exoPlayer.setMediaItem(mediaItem)
```

Já apps baseados em navegador (Chrome, Firefox, Safari, Electron com
`<webview>`, WebView2) **devem** usar o proxy.

#### Performance e limites

- **Streaming, não buffering**: o proxy não lê o vídeo inteiro na memória —
  repassa em chunks de 32KB. Vídeos de 2GB+ funcionam.
- **Timeout**: 60s por requisição. HLS segments (~200KB cada) completam
  em <1s; um filme mp4 inteiro pode demorar dependendo do bitrate.
- **Sem cache**: cada requisição busca fresca do CDN. O cache fica no
  navegador (`Cache-Control: max-age=3600`).
- **Largura de banda**: o proxy duplica o tráfego (navegador → proxy →
  CDN). Em produção, considere um plano Render pago ou hospede atrás de
  uma CDN como Cloudflare.

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

### HomeResponse

Retornado por `/v1/catalog/home`.

```typescript
interface HomeResponse {
  type:  "movies" | "tvshows";
  count: number;          // sempre 4 (lancamentos, destaques, recentes, sugeridos)
  rows:  HomeRow[];
}

interface HomeRow {
  category: "lancamentos" | "destaques" | "recentes" | "sugeridos";
  label:    string;       // "🔥 Lançamentos", "⭐ Destaques", etc.
  count:    number;       // geralmente 12
  items:    HomeItem[];   // estende MovieMeta com campos extras
}

interface HomeItem extends MovieMeta {
  imdb_rating?: number;     // 0–10
  runtime?:      string;    // "96" (minutos como string)
  categories?:   string[];  // ["Crime", "Drama", ...]
  post_id?:      string;    // ID interno do Supercine
}
```

> **Nota**: `HomeItem` herda todos os campos de `MovieMeta`, mas
> `server_count` vem como `0` e `title_orig` como `""` (o endpoint
> `/home` do Supercine não os retorna). Para obter esses campos, chame
> `/v1/catalog/resolve?imdb=...` para o item específico.

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

> **Regra de ouro**: cliente navegador (Chrome, Firefox, Safari, Electron,
> WebView) deve **sempre** passar a URL de vídeo por `/v1/stream`. Cliente
> nativo (ExoPlayer, AVPlayer, VLC, mpv) pode usar a URL direta. Veja
> [`/v1/stream` → Quando NÃO usar o proxy](#quando-não-usar-o-proxy) para
> detalhes.

### App Web (React + hls.js)

```typescript
import Hls from 'hls.js';

const API = 'http://localhost:8080';  // ou https://supercine-proxy.onrender.com

async function playMovie(imdbId: string) {
  // 1. Verifica disponibilidade
  const meta = await fetch(
    `${API}/v1/catalog/resolve?imdb=${imdbId}&type=movies`
  ).then(r => r.json());

  if (!meta.available) {
    alert('Título indisponível');
    return;
  }

  // 2. Resolve URL direta
  const result = await fetch(
    `${API}/v1/resolve?imdb=${imdbId}&type=movies`
  ).then(r => r.json());

  if (!result.videos?.length) {
    alert('Falha ao extrair vídeo');
    return;
  }

  // 3. Encapsula na URL do proxy para contornar CORS/Origin do CDN
  const videoURL = result.videos[0].url;
  const proxiedURL = `${API}/v1/stream?url=${encodeURIComponent(videoURL)}`;

  const video = document.querySelector('video')!;
  if (videoURL.includes('.m3u8') && Hls.isSupported()) {
    const hls = new Hls();
    hls.loadSource(proxiedURL);
    hls.attachMedia(video);
    hls.on(Hls.Events.MANIFEST_PARSED, () => video.play());
  } else {
    video.src = proxiedURL;
    video.play();
  }
}
```

### App Android (Kotlin + ExoPlayer) — sem proxy

ExoPlayer é nativo e não envia `Origin`, então pode usar a URL direta:

```kotlin
suspend fun resolveAndPlay(imdbId: String, type: String) {
    val api = "http://10.0.2.2:8080"  // 10.0.2.2 = host do emulador Android

    // 1. Resolve metadados
    val meta = api.get("${api}/v1/catalog/resolve?imdb=${imdbId}&type=${type}")
    if (!meta.available) {
        showError("Indisponível")
        return
    }

    // 2. Resolve URL direta
    val result = api.get("${api}/v1/resolve?imdb=${imdbId}&type=${type}")
    if (result.videos.isEmpty()) {
        showError("Falha ao extrair")
        return
    }

    // 3. ExoPlayer usa a URL direta (sem /v1/stream)
    val mediaItem = MediaItem.fromUri(result.videos[0].url)
    exoPlayer.setMediaItem(mediaItem)
    exoPlayer.prepare()
    exoPlayer.playWhenReady = true
}
```

### App Android (Kotlin + ExoPlayer) — com proxy WebView

Se você usa WebView dentro do Android, o navegador interno envia `Origin`,
então precisa do proxy:

```kotlin
val html = """
  <video id="v" controls></video>
  <script src="https://cdn.jsdelivr.net/npm/hls.js@1.5.17/dist/hls.min.js"></script>
  <script>
    (async () => {
      const r = await fetch('${api}/v1/resolve?imdb=${imdbId}&type=movies');
      const j = await r.json();
      const proxied = '${api}/v1/stream?url=' + encodeURIComponent(j.videos[0].url);
      const v = document.getElementById('v');
      if (j.videos[0].url.includes('.m3u8') && Hls.isSupported()) {
        const hls = new Hls();
        hls.loadSource(proxied);
        hls.attachMedia(v);
      } else {
        v.src = proxied;
      }
    })();
  </script>
""".trimIndent()
webView.loadDataWithBaseURL(api, html, "text/html", "utf-8", null)
```

### App iOS (Swift + AVPlayer) — sem proxy

AVPlayer é nativo e pode usar a URL direta:

```swift
func playMovie(imdbId: String) async {
    let api = "http://localhost:8080"
    let resolveURL = URL(string: "\(api)/v1/resolve?imdb=\(imdbId)&type=movies")!
    let (data, _) = try! await URLSession.shared.data(from: resolveURL)
    let result = try! JSONDecoder().decode(ResolveResult.self, from: data)

    // AVPlayer usa a URL direta (sem proxy)
    let videoURL = URL(string: result.videos[0].url)!
    let player = AVPlayer(url: videoURL)
    playerViewController.player = player
    player.play()
}
```

### Script CLI (curl + jq + mpv)

```bash
# Buscar um filme e imprimir o link direto
curl -s "http://localhost:8080/v1/catalog/search?q=matrix" \
  | jq '.items[0] | {title: .title_ptbr, year, imdb}'

# Resolver URL direta e abrir no VLC/mpv (nativo, sem proxy)
curl -s "http://localhost:8080/v1/resolve?imdb=tt0133093&type=movies" \
  | jq '.videos[0].url' -r \
  | xargs mpv
```

### Python (requests + subprocess)

```python
import requests
import subprocess
import urllib.parse

API = "http://localhost:8080"

def play(imdb_id, content_type="movies"):
    # 1. Resolve URL direta
    r = requests.get(f"{API}/v1/resolve",
                     params={"imdb": imdb_id, "type": content_type})
    r.raise_for_status()
    video_url = r.json()["videos"][0]["url"]

    # 2a. Cliente nativo (VLC) — URL direta
    subprocess.run(["vlc", video_url])

    # 2b. Cliente navegador (abre no Chrome) — precisa do proxy
    # proxied = f"{API}/v1/stream?url={urllib.parse.quote(video_url)}"
    # subprocess.run(["google-chrome", proxied])

play("tt0816692")
```

### Pontos importantes para clientes

1. **Use `/v1/stream` em navegadores**. Os CDNs dos hosters rejeitam
   `Origin` estrangeiro com 403. Sem o proxy, o `<video>` mostra uma tela
   preta silenciosa. Veja [`/v1/stream`](#get-v1stream) para o porquê.
2. **Sempre trate `available: false`**. Mesmo que um título apareça na
   busca, ele pode não ter servidores disponíveis no momento.
3. **Use HLS.js para m3u8**. Browsers sem suporte nativo a HLS (Firefox,
   Chrome) precisam de hls.js. Safari tem suporte nativo. O proxy
   reescreve as URLs dos segmentos automaticamente — basta apontar o
   `hls.js` para `/v1/stream?url=<master.m3u8>`.
4. **Tenha retry de servidor**. Mesmo que `/v1/resolve` retorne vários
   servidores, o vídeo do servidor 0 pode falhar (URL expirada, hoster
   offline). Faça fallback clicando no próximo servidor.
5. **Cache no cliente**. Metadados não mudam — cacheie `MovieMeta` por
   pelo menos 1h no localStorage. URLs de vídeo **não** devem ser
   cacheadas (expiram em minutos).
6. **Timeout**. Use timeout de 30s para `/resolve` e 60s para `/popular`
   (que resolve 80 títulos em paralelo).

---

## Rate limiting e cache

### Cache do servidor

| Endpoint | TTL | Condição |
|---|---|---|
| `/v1/catalog/popular` | 5 min | Configurável via `CACHE_TTL` |
| `/v1/catalog/home` | 5 min | Por `type` (movies/tvshows) |
| `/v1/catalog/search` | 5 min | Por query |
| `/v1/catalog/resolve` | 5 min | Por imdb+type |
| `/v1/seasons` | 5 min | Por imdb |
| `/v1/resolve` | 0 | Não cacheia (URLs expiram) |
| `/v1/resolveEpisode` | 0 | Não cacheia |
| `/v1/stream` | 0 | Não cacheia no servidor; navegador cacheia 1h (`Cache-Control: max-age=3600`) |
| `/v1/providers` | 0 | Sempre consulta provedores |

### Rate limiting

O proxy **não implementa rate limiting** próprio. Se você expor publicamente,
coloque um nginx/Cloudflare na frente com rate limit por IP.

### Timeout por endpoint

| Endpoint | Timeout |
|---|---|
| `/v1/catalog/popular` | 90s |
| `/v1/catalog/home` | 30s |
| `/v1/catalog/search` | 60s |
| `/v1/catalog/resolve` | 30s |
| `/v1/resolve` | 45s |
| `/v1/resolveEpisode` | 45s |
| `/v1/seasons` | 30s |
| `/v1/stream` | 60s (por requisição; streams longos continuam até o cliente desconectar) |
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

### Fluxo completo: do catálogo à reprodução (navegador)

```bash
# 1. Listar filmes populares
curl -s 'http://localhost:8080/v1/catalog/popular?type=movies&limit=5' | jq '.items[] | .title_ptbr'

# 2. Buscar um filme específico
curl -s 'http://localhost:8080/v1/catalog/search?q=interestelar' | jq '.items[0]'

# 3. Resolver metadados
curl -s 'http://localhost:8080/v1/catalog/resolve?imdb=tt0816692&type=movies' | jq

# 4. Resolver URL direta
curl -s 'http://localhost:8080/v1/resolve?imdb=tt0816692&type=movies' | jq '.videos[0]'

# 5. Reproduzir em cliente nativo (VLC) — URL direta
curl -s 'http://localhost:8080/v1/resolve?imdb=tt0816692&type=movies' | jq -r '.videos[0].url' | xargs vlc

# 6. Reproduzir em navegador — usar /v1/stream (contorna CORS)
VIDEO_URL=$(curl -s 'http://localhost:8080/v1/resolve?imdb=tt0816692&type=movies' | jq -r '.videos[0].url')
ENC=$(python3 -c "import urllib.parse,sys; print(urllib.parse.quote(sys.argv[1]))" "$VIDEO_URL")
echo "Abra no navegador: http://localhost:8080/v1/stream?url=$ENC"
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

# 5. Reproduzir (nativo — URL direta)
curl -s 'http://localhost:8080/v1/resolveEpisode?imdb=tt0903747&season=1&episode=1' | jq -r '.videos[0].url' | xargs vlc

# 6. Reproduzir no navegador (via /v1/stream)
EP_URL=$(curl -s 'http://localhost:8080/v1/resolveEpisode?imdb=tt0903747&season=1&episode=1' | jq -r '.videos[0].url')
ENC=$(python3 -c "import urllib.parse,sys; print(urllib.parse.quote(sys.argv[1]))" "$EP_URL")
echo "Abra: http://localhost:8080/v1/stream?url=$ENC"
```

### Fluxo completo: home da UI estilo Netflix

```bash
# 1. Buscar 4 linhas de filmes (Lançamentos, Destaques, Recentes, Sugeridos)
curl -s 'http://localhost:8080/v1/catalog/home?type=movies' | jq '.rows[] | {label, count}'

# 2. Mesmas 4 linhas para séries
curl -s 'http://localhost:8080/v1/catalog/home?type=tvshows' | jq '.rows[] | {label, count}'

# 3. Pegar o primeiro lançamento e tocar
IMDB=$(curl -s 'http://localhost:8080/v1/catalog/home?type=movies' | jq -r '.rows[0].items[0].imdb')
echo "Lançamento da semana: $IMDB"

# 4. Resolver URL
curl -s "http://localhost:8080/v1/resolve?imdb=$IMDB&type=movies" | jq '.videos[0]'

# 5. Tocar no navegador (via /v1/stream)
VIDEO_URL=$(curl -s "http://localhost:8080/v1/resolve?imdb=$IMDB&type=movies" | jq -r '.videos[0].url')
ENC=$(python3 -c "import urllib.parse,sys; print(urllib.parse.quote(sys.argv[1]))" "$VIDEO_URL")
xdg-open "http://localhost:8080/v1/stream?url=$ENC"  # Linux
# open "http://localhost:8080/v1/stream?url=$ENC"     # macOS
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

### Fallback de servidor no cliente (navegador)

```javascript
async function resolveWithFallback(imdb, type) {
  const API = 'http://localhost:8080';
  const r = await fetch(`${API}/v1/resolve?imdb=${imdb}&type=${type}`);
  const j = await r.json();

  if (!j.videos || j.videos.length === 0) {
    throw new Error('Sem vídeo disponível');
  }

  // O proxy já tentou todos os servidores internamente e retornou o
  // primeiro que funcionou. Aqui só fazemos um retry de re-resolução
  // caso o player falhe (URL pode ter expirado entre resolve e play).
  const videoURL = j.videos[0].url;
  const proxiedURL = `${API}/v1/stream?url=${encodeURIComponent(videoURL)}`;

  try {
    await tryPlay(proxiedURL);   // envia para <video> ou hls.js
  } catch (e) {
    // Re-resolve e tenta de novo (token do CDN pode ter expirado)
    return resolveWithFallback(imdb, type);
  }
}
```

---

## Changelog da API

### v1.4.0 (atual)
- ⭐ **Adicionado `GET /v1/stream`** — proxy de reprodução que contorna o
  bloqueio de `Origin` dos CDNs dos hosters (MixDrop, StreamWish, VidHide).
  Essencial para web apps — sem ele o navegador recebe 403 ao tentar
  tocar a URL direta.
- ✨ Adicionado `GET /v1/catalog/home` — 4 linhas de destaque (Lançamentos,
  Destaques, Recentes, Sugeridos) do Supercine, 12 itens cada.
- 🔧 `/v1/resolve` agora tenta **todos** os servidores (antes eram 3) e os
  reordena por prioridade de hoster (StreamWish/FileMoon primeiro,
  DoodStream por último).
- 🔧 `verifyURL` agora usa `GET` com `Range: bytes=0-1` (antes `HEAD`) e
  envia `Origin`/`Referer` corretos por CDN. Resolve falsos negativos no
  MixDrop.
- 🔧 `resolveHosterURL` agora decodifica HTML entities (`&amp;` → `&`) na
  URL do hoster.
- 🔧 MixDrop extractor agora stripa query parameters (`?sub1=...`) que
  causavam HTTP 400.
- 🔧 DoodStream extractor detecta Cloudflare Turnstile CAPTCHA e falha
  graciosamente com mensagem clara.
- 📚 Adicionada seção [Quick start — rodando localmente](#quick-start--rodando-localmente)
  com exemplo completo de HTML + hls.js.
- 📚 Adicionados exemplos para Android (ExoPlayer), iOS (AVPlayer),
  Python e CLI no section [Integração com clientes](#integração-com-clientes).

### v1.3.0
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

**Fim da documentação da Output API v1.4.0.**

Para a documentação da API interna do Supercine (upstream), veja
[`UPSTREAM_API.md`](UPSTREAM_API.md). Para a análise do APK original,
veja [`APK_ANALYSIS.md`](APK_ANALYSIS.md).
