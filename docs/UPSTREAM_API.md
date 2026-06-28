# Upstream API — `supercine-tv.net`

> Base URL: `https://supercine-tv.net/wp-json/`
> Embed: `https://supercine-tv.net/embed-api/`
> Plataforma: WordPress + plugin customizado "warezcdn" (mesmo nome do tema ativo)
> Idioma das mensagens: Português brasileiro (PT-BR)

Este documento descreve em detalhes cada endpoint descoberto, com método,
parâmetros, exemplo de chamada e resposta observada.

---

## 1. Namespaces

A API retorna 11 namespaces:

```
oembed/1.0
litespeed/v1
litespeed/v3
api
inbox
auth
site
wp/v2
wp-site-health/v1
wp-block-editor/v1
wp-abilities/v1
```

Dos quais 4 são do plugin warezcdn: **api, inbox, auth, site**.

---

## 2. Endpoint `GET /wp-json/`

Retorna a lista completa de 158 rotas e os metadados do site.

**Exemplo:**

```bash
curl https://supercine-tv.net/wp-json/
```

**Resposta (resumo):**

```json
{
  "name": "Supercine API",
  "description": "",
  "url": "https://supercine-tv.net",
  "namespaces": ["api", "auth", "inbox", "site", "wp/v2", ...],
  "routes": {
    "/api": {"methods": ["GET"], ...},
    "/api/(?P<type>[a-zA-Z0-9-]+)": {"methods": ["GET"], ...},
    "/api/add": {"methods": ["GET"], ...},
    "/auth/login": {"methods": ["POST"], ...},
    ...
  }
}
```

---

## 3. Namespace `api` — conteúdo

### `GET /api/<type>`

Lista conteúdo por tipo. `<type>` é um slug arbitrário — o app tenta `filmes`, `series`, `animes`, `movies`, `tvshows`, etc.

> ⚠️ Na versão 1.0.0 do APK, **todos** os retornos são `{"status":"update",...}` forçando upgrade. Não conseguimos inspecionar o schema de sucesso sem uma versão mais recente.

**Chamada:**

```bash
curl https://supercine-tv.net/wp-json/api/filmes
```

**Resposta (versão 1.0.0):**

```json
{
  "status": "update",
  "url": "https://play.google.com/store/apps/details?id=tv.supercine"
}
```

**Tipo aceitos pelo app (vistos nas strings do libapp.so):**
- `filmes`
- `series`
- `animes`
- `movies` (alias)
- `tvshows` (alias)

### `GET /api/add`

Adiciona conteúdo — provavelmente usado pelo painel admin, não pelo app.

**Sem parâmetros:**

```bash
curl https://supercine-tv.net/wp-json/api/add
```

```json
{
  "response": false,
  "message": "Preencha os dados necessários"
}
```

Os parâmetros exatos não foram identificados (testamos `url`, `tipo`, `link`, `title`, `id` — todos retornam a mesma mensagem).

---

## 4. Namespace `auth` — ativação e cobrança

### `POST /auth/login`

Ativa o app via código + device. Retorna `premium: true` se o código é válido e não foi usado em outro device.

**Body:**

```json
{
  "code": "TEST123",
  "device": "android-abcdef123456"
}
```

**Respostas:**

| Cenário | Resposta |
|---|---|
| Sem `code` ou `device` | `{"success":false,"premium":false,"error":"Código e device são obrigatórios"}` |
| Código inválido | `{"success":false,"premium":false,"error":"Código inválido"}` |
| (presumido) Código OK | `{"success":true,"premium":true,"device":"...","expires":"..."}` |

### `POST /auth/plans`

Lista os planos de assinatura via PIX.

**Body:** `{}` (vazio)

**Resposta:**

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

### `POST /auth/checkout`

Inicia um checkout PIX.

**Body:**

```json
{
  "device": "android-abcdef123456",
  "plano": "m1"
}
```

**Respostas:**

| Cenário | Resposta |
|---|---|
| Sem `device` ou `plano` | `{"success":false,"premium":false,"error":"Device e plano são obrigatórios"}` |
| (presumido) OK | `{"success":true,"pix_id":"...","qr_code":"...","pix_code":"..."}` |

### `POST /auth/checkout-status`

Consulta status de um PIX.

**Body:**

```json
{
  "pix_id": "...",
  "device": "..."
}
```

**Respostas:**

| Cenário | Resposta |
|---|---|
| Sem `pix_id` ou `device` | `{"success":false,"premium":false,"error":"pix_id e device são obrigatórios"}` |
| Pedido não encontrado | `{"success":false,"premium":false,"error":"Pedido não encontrado"}` |
| (presumido) Pago | `{"success":true,"premium":true,"status":"paid"}` |

### `POST /auth/history`

Histórico de pedidos do device.

**Body:**

```json
{ "device": "android-abcdef123456" }
```

**Resposta (sem pedidos):**

```json
{
  "success": true,
  "orders": []
}
```

### `POST /auth/logout**

Desativa o device.

**Body:**

```json
{ "device": "android-abcdef123456" }
```

**Resposta:**

```json
{ "success": true }
```

---

## 5. Namespace `inbox` — relatórios

### `POST /inbox/report`

Provavelmente usado para reportar links quebrados ou problemas.

**Body (vazio):**

```json
{}
```

**Resposta:**

```json
{
  "success": false,
  "message": "Ação desconhecida"
}
```

**Possíveis actions** (não descobertas): provavelmente `broken_link`, `dead_video`, `report_bug`, etc. O parâmetro `action` é necessário mas os valores exatos não foram identificados na análise.

---

## 6. Namespace `site` — extractor server-side

### `GET /site/extractor?url=...`

Tentativa de extractor server-side. Rejeita todos os 8 hosters do APK:

**Chamada:**

```bash
curl 'https://supercine-tv.net/wp-json/site/extractor?url=https://doodstream.com/e/abc'
```

**Resposta:**

```json
{
  "status": "error",
  "message": "site não suportado"
}
```

Provavelmente reserved para uso futuro ou para sites específicos não suportados pelo APK. O app não usa este endpoint — todo o scraping é feito client-side no Kotlin.

### `GET /site/extractor` (sem url)

```json
{
  "status": "error",
  "message": "falta dados"
}
```

---

## 7. Endpoint HTML `GET /embed-api/`

Fora do `/wp-json/`, mas essencial para o fluxo. Retorna uma página HTML customizada com elementos `<server-selector>`.

### `GET /embed-api/?imdb=<imdb>&type=<movies|tvshows>`

**Chamada:**

```bash
curl 'https://supercine-tv.net/embed-api/?imdb=tt2250912&type=movies'
```

**HTML resultante (estrutura completa):**

```html
<!-- ⭐ Título traduzido em PT-BR -->
<ititle>Homem-Aranha: De Volta ao Lar</ititle>

<!-- ⭐ Backdrop (imagem de fundo) vindo do TMDB -->
<backdrop style="background-image: url('https://image.tmdb.org/t/p/original/fn4n6uOYcB6Uh89nbNPoU2w80RV.jpg');"></backdrop>

<!-- Lista de servidores disponíveis -->
<playeroptions class="visible">
  <playeroptions-audios>
    <audio-selector class="active" data-lang="1"></audio-selector>
  </playeroptions-audios>
  <playeroptions-servers class="active" data-lang="1">
    <server-selector data-server="pq5XG9_s-NStk...REI828wRP">
      <span>Velocidade ok e poucos anúncios</span>
    </server-selector>
    <server-selector data-server="uddfNVBR2hO4SF...hO0n3Rs">
      <span>Velocidade ok e poucos anúncios</span>
    </server-selector>
    <server-selector data-server="67sZjX-555wzuE...adoQNyw">
      <span>Esee é o Top 1, rápido e poucos anúncios!</span>
    </server-selector>
    <server-selector data-server="X69yzq76PTAszt...E+JzO">
      <span>O 2º melhor muito rápido!</span>
    </server-selector>
  </playeroptions-servers>
</playeroptions>
```

> 💡 **Descoberta importante**: o Supercine usa o TMDB internamente para buscar backdrops. Isso significa que **para qualquer IMDB ID válido**, este endpoint retorna:
>
> - Título traduzido em PT-BR (via `<ititle>`)
> - URL do backdrop full-resolution do TMDB (via `<backdrop style="background-image:...">`)
> - Lista de servidores de streaming disponíveis
>
> Isso é o que permite à UI de streaming deste proxy mostrar um catálogo visualmente rico **sem precisar de nenhuma API key do TMDB**.

### `GET /embed-api/?action=embed&url=<data-server>`

Decodifica o `data-server` (criptografado server-side) e retorna um HTML com `window.location.href = "<hoster-url>"`.

**Chamada:**

```bash
curl 'https://supercine-tv.net/embed-api/?action=embed&url=pq5XG9_s-NStk...REI828wRP'
```

**Resposta:**

```html
<!DOCTYPE html>
<html lang="pt-br">
<head>
  <title>Steam Video</title>  <!-- typo: "Steam" em vez de "Stream" -->
</head>
<body>
  <script>
    window.location.href = "https://mixdrop.ps/e/mkqwgplli4okq7";
  </script>
</body>
</html>
```

### Hashes de customização (URL fragments)

| Hash | Efeito |
|---|---|
| `#transparent` | Remove o background do embed |
| `#nobackground` | Esconde background do `<main>` |
| `#whitetheme` | Tema claro |
| `#noEpList` | Esconde o botão "Mostrar todos os episódios" |
| `#first` | Auto-clica no primeiro server |
| `#color<HEX>` | Aplica cor customizada (ex: `#color6c5ce7`) |

Exemplo: `https://supercine-tv.net/embed-api/?imdb=tt2250912&type=movies#transparent#whitetheme`

### Embed via JavaScript (plugin warezcdn)

Para usar em sites de terceiros:

```html
<div id="embedsupercine"></div>
<script>
  var type = "serie";        // "filme" ou "serie" (anime também usa "serie")
  var imdb = "tt0050079";
  var season = "1";
  var episode = "1";
  warezPlugin(type, imdb, season, episode);
  function warezPlugin(type, imdb, season, episode) {
    if (type == "filme") { season=""; episode=""; }
    else {
      if (season !== "") season = "/" + season;
      if (episode !== "") episode = "/" + episode;
    }
    var frame = document.getElementById('embedsupercine');
    frame.innerHTML += '<iframe src="https://supercine-tv.net/embed-api/?imdb='
      + type + '/' + imdb + season + episode
      + '" scrolling="no" frameborder="0" allowfullscreen=""></iframe>';
  }
</script>
```

---

## 8. Endpoints WordPress padrão (também expostos)

Além dos namespaces customizados, a API expõe o `/wp/v2/` completo do WordPress. Embora os posts estejam vazios (`/wp/v2/posts` retorna `[]`), há 29 categorias visíveis:

| Slug | Nome | Count |
|---|---|---|
| `acao` | Ação | 2545 |
| `action-adventure` | Action & Adventure | 1379 |
| `animacao` | Animação | 3430 |
| `animes` | Animes | 2249 |
| `aventura` | Aventura | 1634 |
| `cinema-tv` | Cinema TV | 355 |
| `comedia` | Comédia | 4563 |
| `crime` | Crime | 1782 |
| `documentario` | Documentário | 429 |
| `drama` | Drama | 5830 |
| `familia` | Família | 1446 |
| `fantasia` | Fantasia | 1101 |
| `faroeste` | Faroeste | 178 |
| `ficcao-cientifica` | Ficção científica | 1120 |
| `guerra` | Guerra | 283 |
| `historia` | História | 391 |
| `kids` | Kids | 159 |
| `lancamentos` | Lançamentos | 406 |
| `misterio` | Mistério | 1376 |
| `musica` | Música | 268 |
| `news` | News | 2 |
| `reality` | Reality | 79 |
| `romance` | Romance | 1349 |
| `sci-fi-fantasy` | Sci-Fi & Fantasy | 1492 |
| `soap` | Soap | 12 |
| ... | ... | ... |

---

## 9. Headers recomendados

```http
User-Agent: Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/137.0.0.0 Safari/537.36
Accept: application/json, text/plain, */*
Accept-Language: pt-BR,pt;q=0.9,en;q=0.8
Referer: https://supercine-tv.net/
```

O `User-Agent` acima é **o mesmo hardcoded** no APK em `ExtractorLinks.agent`.

---

## 10. Fluxo completo de resolução de um filme

```
1. App pede lista de filmes:
   GET /wp-json/api/filmes
   → atualmente retorna {"status":"update"} para versão 1.0.0

2. App pega IMDB ID de um filme (ex: tt2250912 = John Wick: Chapter 4)

3. App abre o embed:
   GET /embed-api/?imdb=tt2250912&type=movies

4. App recebe HTML com 4 <server-selector data-server="...">

5. App escolhe um server (ou clica no primeiro):
   GET /embed-api/?action=embed&url=<data-server>

6. App recebe HTML com window.location.href = "https://mixdrop.ps/e/abc"

7. App passa essa URL para o native via MethodChannel("com.example/links"):
   method = "extractLinks", args = {url: "https://mixdrop.ps/e/abc"}

8. Native ExtractorLinks.find() detecta hoster (mixdrop), chama MixDrop.fetch()

9. MixDrop.fetch():
   - GET https://mixdrop.ps/e/abc
   - Regex: eval(function(p,a,c,k,e,d)(.*?)split
   - Identifica o bloco MDCore
   - JSUnpacker.unpack() -> código desofuscado
   - Regex: wurl="(.*?)";
   - Retorna "https:" + wurl

10. App recebe URL direta do mp4 no Flutter, abre no player VLC nativo.
```

---

## 11. Curl cookbook

```bash
# Lista de rotas
curl https://supercine-tv.net/wp-json/ | jq '.routes | keys | length'   # 158

# Planos
curl -X POST https://supercine-tv.net/wp-json/auth/plans \
  -H 'Content-Type: application/json' -d '{}'

# Login com código inválido
curl -X POST https://supercine-tv.net/wp-json/auth/login \
  -H 'Content-Type: application/json' \
  -d '{"code":"INVALID","device":"test"}'

# Embed de um filme
curl 'https://supercine-tv.net/embed-api/?imdb=tt2250912&type=movies'

# Decodificar o primeiro server
SERVER=$(curl -s 'https://supercine-tv.net/embed-api/?imdb=tt2250912&type=movies' \
  | grep -oE 'data-server="[^"]+"' | head -1 | cut -d'"' -f2)
curl "https://supercine-tv.net/embed-api/?action=embed&url=$SERVER"

# Categorias WordPress
curl 'https://supercine-tv.net/wp-json/wp/v2/categories?per_page=100' | jq '.[] | {slug, count}'
```
