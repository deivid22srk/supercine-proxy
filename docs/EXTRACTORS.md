# Extractors — porta dos scrapers Java/Kotlin do APK para Go

Cada extractor é uma porta direta do código original em `tv.supercine.supercine.sites.*`. Esta doc explica o fluxo interno de cada um e como testá-los isoladamente.

---

## 1. Registry

O `extractors.Registry` mantém a lista dos 8 extractors e despacha por URL:

```go
reg := extractors.NewRegistry()
result, err := reg.Dispatch(ctx, "https://mixdrop.ps/e/abc123")
```

Ele itera pela lista chamando `Match(url)` (que usa a mesma regex do `RegexExpress.kt` do APK) e usa o primeiro match.

A ordem do registro segue a ordem do `LinkedHashMap` no `ExtractorLinks.java`:

1. `filelions`
2. `filemoon`
3. `streamwish`
4. `vidhide`
5. `doodstream`
6. `streamtape`
7. `mixdrop`
8. `voe`

---

## 2. DoodStream

**Arquivo:** `internal/extractors/doodstream.go`
**Original:** `tv.supercine.supercine.sites.DoodStream`

### Regex de URL

```
.+(doodstream|dood|vidply|do7go)\.(com|watch|to|so|la|ws|sh|pm|re|li)\/.+
```

### Fluxo

```
1. Normaliza /d/ -> /e/ (página de embed)
2. GET https://doodstream.com/e/<id>
3. Regex no HTML: /pass_md5/[^']+
4. GET https://doodstream.com/pass_md5/... -> 32-char hash
5. Final URL = hash + randomStr(10) + "?token=" + lastSegment(pass_md5_url)
```

O `randomStr(10)` usa `SecureRandom` no Java — em Go usamos `crypto/rand` sobre o charset `[A-Za-z0-9]`.

---

## 3. StreamWish

**Arquivo:** `internal/extractors/streamwish.go`
**Original:** `tv.supercine.supercine.sites.StreamWish`

### Regex de URL

```
.+(streamwish|asnwish|tlnwish|playerwish|tln-hg)\.(com|co|to|sx|bz|xyz|top)\/.+
```

### Fluxo

```
1. GET a URL original
2. Jsoup.parse(html)
3. Para cada <script>:
   - Se contém "eval(function(p,a,c,k,e,":
     - Unpack via JSUnpacker
     - Regex por file/hls/hls2/src/link/url/path = "...mp4|m3u8|mkv"
   - Em qualquer caso, também tenta regex no próprio <script> text
4. Se nada encontrado, regex no body inteiro
5. Walk <source>, <video> tags: src / data-src
6. Dedup + adiciona à lista
```

### Regex de sources

```
["']?(file|hls|hls2|src|link|url|path)["']?\s*[:=]\s*["']([^"']+\.(mp4|m3u8|mkv)[^"']*)["']
sources:\s*\[\s*\{\s*file:\s*["']([^"']+)["']
```

### Quality label

- `.mp4` → `MP4 Video`
- `.m3u8` → `HLS (m3u8)`
- outro → `Normal`

---

## 4. VidHide

**Arquivo:** `internal/extractors/vidhide.go`
**Original:** `tv.supercine.supercine.sites.VidHide`

### Regex de URL

```
.+(vidhide|vidhidevip|tlnhide|megahide|niikaplayerr|tln-earn)\.(com|co|to|sx|bz|live|online|in|site|xyz|shop|top)\/.+
```

### Fluxo

Idêntico ao StreamWish. No APK, VidHide e StreamWish são cópias uma da outra. Mantivemos essa estrutura para facilitar diffs futuros caso um deles mude.

---

## 5. FileMoon

**Arquivo:** `internal/extractors/filemoon.go`
**Original:** `tv.supercine.supercine.sites.FileMoon`

### Regex de URL

```
.+(filemoon|96ar|tlnmoons)\.(com|co|to|sx|bz|in|top)\/.+
```

### Fluxo

```
1. GET a URL
2. Jsoup.parse(html)
3. Para cada <script type="text/javascript">:
   - Se contém eval(function(p,a,c,k,e,[rd]  (note [rd] em vez de ,)
     - JSUnpacker.unpack()
     - Regex: file:"(.*?m3u8.*?)"
4. Adiciona URL m3u8 como "Normal"
```

---

## 6. FileLions

**Arquivo:** `internal/extractors/filelions.go`
**Original:** `tv.supercine.supercine.sites.FileLions`

### Regex de URL

```
.+(filelions)\.(live|online|to|sx|bz|in)\/.+
```

### Fluxo

Idêntico ao FileMoon (mesmo padrão de packed JS + `file:"...m3u8..."`).

---

## 7. MixDrop

**Arquivo:** `internal/extractors/mixdrop.go`
**Original:** `tv.supercine.supercine.sites.MixDrop`

### Regex de URL

```
.+(mixdrop)\.(com|co|to|sx|bz|ag|ch|pw|net|si|ms|ps)\/.+
```

### Fluxo

```
1. Normaliza /f/ -> /e/ (página de embed)
2. GET a página
3. Regex: eval\(function\(p,a,c,k,e,d\)(.*?)split
   - Para cada match, monta o bloco completo: "eval(function(p,a,c,k,e,d)<g1>split('|'),0,{}))"
   - Filtra apenas os blocos que contêm "MDCore"
   - JSUnpacker.unpack()
   - Regex: wurl="(.*?)";
   - Retorna "https:" + wurl
```

---

## 8. StreamTape

**Arquivo:** `internal/extractors/streamtape.go`
**Original:** `tv.supercine.supercine.sites.StreamTape`

### Regex de URL

```
.+(streamtape|streamadblockplus|stapewithadblock|shavetape|tapenoads|tapeantiads)\.(com|to|sx|bz|beauty|cash)\/.+
```

### Fluxo

```
1. Normaliza /e/ -> /v/ (página de vídeo em vez de embed)
2. GET a página
3. Se body contém "norobot":
   Regex: ById\('.+robot.+?=.*(["']//[^;+]+).*'(.*?)'
   Senão:
   Regex: ById\('?robot.+?=.*(["']//[^;+]+).*'(.*?)'
4. Match retorna 2 grupos
   group1 = URL parcial (com possíveis aspas a remover)
   group2 = sufixo codificado
5. finalURL = "https:" + group1 + group2[3:] + "&stream=1"
6. GET (sem seguir redirect!) -> captura header Location
7. Retorna Location como URL final
```

O passo 6 é crítico: no Kotlin é `setInstanceFollowRedirects(false)` para capturar o 302 manualmente. Em Go, usamos `CheckRedirect: func(...) error { return http.ErrUseLastResponse }`.

---

## 9. Voe

**Arquivo:** `internal/extractors/voe.go`
**Original:** `tv.supercine.supercine.sites.Voe`

### Regex de URL

```
.+(voe|donaldlineelse|jamessoundcost)\.(com|co|to|sx|bz)\/.+
```

### Fluxo

```
1. GET a URL
2. Jsoup.parse(html)
3. Para cada <script>:
   - Se contém "sources =":
     - Regex: ["']hls["']:\s*["'](.*?)['"]
     - Base64 decode do match
     - Adiciona como URL "Normal"
```

O base64 decode usa `android.util.Base64.decode(str, 0)` no Kotlin — em Go é o `base64.StdEncoding.DecodeString` padrão.

---

## 10. JSUnpacker

**Arquivo:** `internal/extractors/jsunpacker.go`
**Original:** `tv.supercine.supercine.utils.JSUnpacker`

Decodifica packed JavaScript no formato `eval(function(p,a,c,k,e,d){...}('body', radix, count, 'words|split|by|pipe'.split('|'),0,{}))`.

### Algoritmo

1. Detecta o padrão `eval(function(p,a,c,k,e,(r|d)` (com espaço removido).
2. Captura 4 grupos via regex: `body`, `radix`, `count`, `words[]`.
3. Para cada token `\b\w+\b` no body, calcula seu índice:
   - Se `radix <= 36`: `strconv.ParseInt(token, radix, 64)` (nativo Go).
   - Se `radix > 36`: usa um dicionário com alfabeto apropriado (62 ou 95 chars).
4. Se `índice < len(words)`, substitui o token por `words[índice]`.
5. Mantém um offset acumulado para offsets de substituições subsequentes.

### Alfabetos

```go
const alpha62 = "0123456789abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ"
const alpha95 = " !\"#$%&'()*+,-./0123456789:;<=>?@ABCDEFGHIJKLMNOPQRSTUVWXYZ[\\]^_`abcdefghijklmnopqrstuvwxyz{|}~"
```

### Como testar isoladamente

```go
package main

import (
    "fmt"
    "github.com/deivid22srk/supercine-proxy/internal/extractors"
    _ = extractors.UnpackJS
)

func main() {
    packed := `eval(function(p,a,c,k,e,d){e=function(c){return c.toString(36)};if(!''.replace(/^/,String)){while(c--){d[c.toString(36)]=k[c]||c.toString(36)}k=[function(e){return d[e]}]}e=function(){return'\\w+'};c=1};while(c--){if(k[c]){p=p.replace(new RegExp('\\b'+e(c)+'\\b','g'),k[c])}}return p}('console.log("hello");',1,1,'hello'.split('|'),0,{}))`
    fmt.Println(extractors.UnpackJS(packed))
}
```

> Nota: `UnpackJS` é exportado para testes, mas os extractors usam `unpackJS` (lowercase) internamente.

---

## 11. CLI de teste

O exemplo `examples/extract_url.go` permite testar um único hoster URL via CLI:

```bash
# Testar um hoster diretamente
go run ./examples/extract_url.go https://mixdrop.ps/e/mkqwgplli4okq7

# Output:
# {
#   "hoster": "mixdrop",
#   "url": "https://mixdrop.ps/e/mkqwgplli4okq7",
#   "videos": [
#     {
#       "url": "https://30xplewoo.mxcontent.net/v2/mkqwgplli4okq7.mp4?s=...&e=...",
#       "quality": "Normal"
#     }
#   ],
#   "took": "437ms"
# }
```

---

## 12. Limitações e diferenças vs o APK

- **Sem execução de JS**: o `JSUnpacker` cobre apenas o caso `p.a.c.k.e.r.` clássico. Se um hoster mudar para outro packer (e.g. Obfuscator.io), será necessário estender.
- **Sem anti-bot bypass**: alguns hosters podem exigir cookies / tokens CSRF. O extractor não resolve isso — apenas usa UA + Referer como no APK.
- **Sem retry/backoff**: o APK não tem, e nós também não. Erros de rede retornam erro direto.
- **Headers**: o APK usa apenas `User-Agent` e `Referer`. Mantivemos o mesmo conjunto, adicionando `Accept` e `Accept-Language` por boa prática.
- **Qualidade**: o APK detecta qualidade só pelo extensão (.mp4, .m3u8). Mantivemos o mesmo comportamento — se o hoster servir 480p/720p/1080p separadamente, todos serão retornados como entries separadas sem ordem garantida.
