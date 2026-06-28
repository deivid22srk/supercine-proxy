# Análise do APK Supercine.tv

> APK alvo: `Supercine.tv_1.0.0_antisplit.apk` (102 MB)
> Source: `https://github.com/deivid22srk/Glm-Android/releases/download/1/Supercine.tv_1.0.0_antisplit.apk`
> Package: `tv.supercine`
> Version: `1.0.0` (versionCode 59)
> Autor (do path de build): `ianoliveira` (`file:///Users/ianoliveira/Documents/GitHub/supercine_app/`)

---

## 1. Stack técnica

| Camada | Tecnologia |
|---|---|
| UI | Flutter (Dart, compilado AOT para `libapp.so`) |
| Native bridge | Kotlin (canal `com.example/links`, método `extractLinks`) |
| Networking (Dart) | `dio` / `http` (comum no ecossistema Flutter) |
| Networking (Kotlin) | `AndroidNetworking` (com.androidnetworking) |
| HTML parsing | Jsoup (`org.jsoup`) |
| Player | VLC (`libvlc.so` + `libvlcjni.so`) + ExoPlayer (Media3) |
| State | MobX (`mobx` package) |
| DI | Darts Modular (`app_module.dart`) |
| Ads | Unity Ads, Google Mobile Ads, ByteDance Pangle |
| Push | OneSignal |
| APM | APMInsight (bytedance) |
| Anti-tamper | `pairipcore` (`libpglarmor.so`) + Play Integrity |

---

## 2. Estrutura de packages

```
tv.supercine/
├── MainActivity.kt          ← FlutterActivity + MethodChannel("com.example/links")
├── MainActivityKt.kt        ← isEmulator() check
├── BuildConfig.kt
├── Jmodel.kt                ← data class { url: String, quality: String }
├── RegexExpress.kt          ← regexes dos 8 hosters
├── ExtractorLinks.kt        ← dispatcher: URL → site.fetch()
├── NativeView.kt            ← player view nativo (VLC/ExoPlayer)
├── NativeViewFactory.kt
├── utils/
│   ├── Utils.kt             ← helpers (getDomainFromURL, B64Encode, tokenCaptcha)
│   └── JSUnpacker.kt        ← decoder para eval(function(p,a,c,k,e,d)...)
└── sites/
    ├── DoodStream.kt
    ├── StreamWish.kt
    ├── VidHide.kt
    ├── FileMoon.kt
    ├── FileLions.kt
    ├── MixDrop.kt
    ├── StreamTape.kt
    └── Voe.kt
```

O resto da app (UI, controllers, repositories, stores MobX) é Dart compilado dentro de `libapp.so` (6.6 MB). Strings de Dart são visíveis via `strings -n 4 libapp.so | grep ...`.

---

## 3. Fluxo de execução

```
[Flutter UI]
    │
    │ MethodChannel("com.example/links").invokeMethod("extractLinks", {url})
    ▼
[MainActivity.kt.configureFlutterEngine$lambda$0]
    println("URL recebida no nativo 🛑: " + url)
    ExtractorLinks(ctx).find(url)
    ▼
[ExtractorLinks.find]
    for each (regex, fetcher) in urlFetchers:
        if regex.matcher(url).find():
            fetcher.accept(url, onComplete)
            return
    println("URL rNao encontrada 🛑")  ← typo em "não"
    onComplete.onError()
    ▼
[<Site>.fetch]
    GET url com UA + Referer
    parse HTML (Jsoup)
    unpack JS (JSUnpacker) se necessário
    regex para file/hls/src/...
    onComplete.onTaskCompleted(ArrayList<Jmodel>, multiple_quality)
    ▼
[Flutter UI]
    recebe vidURL.get(0).getUrl() → abre no player nativo (VLC)
```

---

## 4. Detecção de emulador

`MainActivityKt.isEmulator()` é chamada na inicialização da Activity. Se retornar `true`:

```kotlin
System.out.print("emulator 💙")
throw IllegalStateException()
```

Ou seja: o app **crasha deliberadamente em emuladores**, com um `💙` no log. Provavelmente anti-pirataria / anti-análise dinâmica.

---

## 5. URLs hardcoded no APK

Encontradas via `strings libapp.so`:

| URL | Uso |
|---|---|
| `https://supercine-tv.net/wp-json/api/` | API REST principal |
| `https://supercine-tv.net/embed-api/` | Endpoint de embed do player |
| `https://t.me/supercinetv` | Canal Telegram |
| `https://instagram.com/megahdfilmes` | Instagram antigo (?) |
| `https://api.ipify.org` | Lookup de IP |
| `https://api.iplocation.net/` | Geolocation |
| `https://i3.ytimg.com/vi/` | Thumbnails YouTube |
| `https://doodstream.com/d/` | DoodStream deep-link |
| `https://dood.li/d/` | DoodStream mirror |
| `https://streamtape.com/v/` | StreamTape deep-link |
| `https://vidhidepre.com/d/` | VidHide mirror |
| `https://asnwish.com/d/` | StreamWish mirror |
| `https://sfastwish.com/` | StreamWish mirror |

---

## 6. MethodChannel nativo

Canal: `com.example/links`
Método: `extractLinks`
Argumento: `{ "url": "https://hoster.com/e/abc123" }`
Retorno: a primeira URL direta (mp4/m3u8) encontrada — apenas `vidURL.get(0).getUrl()`, ignora qualidades adicionais.

---

## 7. RegexExpress (todos os 8 hosters)

Cada hoster é identificado por uma regex case-insensitive. O `ExtractorLinks.find()` itera pela lista e usa o primeiro match.

```kotlin
filelions  = ".+(filelions)\\.(live|online|to|sx|bz|in)\\/.+"
filemoon   = ".+(filemoon|96ar|tlnmoons)\\.(com|co|to|sx|bz|in|top)\\/.+"
streamwish = ".+(streamwish|asnwish|tlnwish|playerwish|tln-hg)\\.(com|co|to|sx|bz|xyz|top)\\/.+"
vidhide    = ".+(vidhide|vidhidevip|tlnhide|megahide|niikaplayerr|tln-earn)\\.(com|co|to|sx|bz|live|online|in|site|xyz|shop|top)\\/.+"
doodstream = ".+(doodstream|dood|vidply|do7go)\\.(com|watch|to|so|la|ws|sh|pm|re|li)\\/.+"
streamtape = ".+(streamtape|streamadblockplus|stapewithadblock|shavetape|tapenoads|tapeantiads)\\.(com|to|sx|bz|beauty|cash)\\/.+"
mixdrop    = ".+(mixdrop)\\.(com|co|to|sx|bz|ag|ch|pw|net|si|ms|ps)\\/.+"
voe        = ".+(voe|donaldlineelse|jamessoundcost)\\.(com|co|to|sx|bz)\\/.+"
```

---

## 8. Player nativo

- `NativeView` registra a view factory `"playerViewTag"` no Flutter.
- Internamente usa **libVLC** (videolan) + **ExoPlayer** (Media3) como fallback.
- Há um player alternativo baseado em `InAppWebView` com iframe do YouTube para trailers.

---

## 9. Endpoints da API (descobertos)

Veja [`UPSTREAM_API.md`](UPSTREAM_API.md) para a spec completa. Resumo:

| Namespace | Endpoint | Método | Descrição |
|---|---|---|---|
| `api` | `/api/<type>` | GET | Lista conteúdo por tipo (filmes/series/animes) |
| `api` | `/api/add` | GET | (?) Adiciona conteúdo — requer params não-documentados |
| `auth` | `/auth/login` | POST | Ativação por código+device |
| `auth` | `/auth/plans` | POST | Lista planos de assinatura |
| `auth` | `/auth/checkout` | POST | Inicia checkout PIX |
| `auth` | `/auth/checkout-status` | POST | Status do pagamento PIX |
| `auth` | `/auth/history` | POST | Histórico de pedidos do device |
| `auth` | `/auth/logout` | POST | Desloga device |
| `inbox` | `/inbox/report` | POST | (?) Reporta conteúdo/link quebrado |
| `site` | `/site/extractor` | GET | (?) Extractor server-side — não suporta os hosters do APK |
| (HTML) | `/embed-api/?imdb=...&type=...` | GET | Página HTML com `<server-selector>` |

---

## 10. Como rodar jadx para reproduzir

```bash
# Baixar jadx
curl -L https://github.com/skylot/jadx/releases/download/v1.5.0/jadx-1.5.0.zip -o jadx.zip
unzip jadx.zip -d jadx

# Decompile apenas o código (sem resources)
./jadx/bin/jadx -d supercine_decompiled --no-res --log-level ERROR supercine.apk

# Strings do Flutter (libapp.so)
unzip supercine.apk "lib/arm64-v8a/libapp.so" -d extracted/
strings -n 6 extracted/lib/arm64-v8a/libapp.so | grep -E "https?://" | sort -u
```

---

## 11. Limitações da análise

- O código Dart é compilado AOT em `libapp.so`. Vemos apenas strings, não fluxo de controle. Para entender a UI/estado seria necessário um disassembler ARM64 + conhecimento do formato snapshot do Dart.
- O APK usa `pairipcore` (anti-tampering) e provavelmente checa integridade do `classes.dex`. Modificar o APK requer re-signing e bypass do pairip.
- A versão 1.0.0 é **bloqueada pelo servidor** — todos os endpoints `/api/*` retornam `{"status":"update", "url":"https://play.google.com/store/apps/details?id=tv.supercine"}`. A app força upgrade.

---

## 12. Mensagens engraçadas encontradas

Veja [`FUNNY_MESSAGES.md`](FUNNY_MESSAGES.md) para a lista completa com contexto.
