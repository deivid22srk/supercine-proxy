# Amenic Plus Provider

This document describes the Amenic Plus provider (`internal/provider/amenic`),
analyzed from the `Amenic.Plus_1.7.3_antisplit.apk` Android app
(package `media6.app.amenic`).

## ⚠️ Cloudflare limitation

`amenic-file.com` is behind **Cloudflare's managed challenge** mode. This
means datacenter IPs (like Render's Oregon region, AWS, GCP, Azure) get
HTTP 403 with a JavaScript challenge that requires a real browser to solve.

The provider detects this and returns `provider.ErrProviderDown`, which
makes the provider registry fall back to the next provider (Supercine).
So enabling Amenic on Render is safe — it just won't contribute any
results until the proxy runs from a residential IP or a browser-based
environment.

## APK analysis findings

### App structure

The APK is a WebView-based app. The entry activity loads
`assets/www/home.html`, which fetches the main UI from
`https://amenic-file.com/main?<query>`. The player is loaded from
`assets/www/player.html`, which receives video URLs via base64-encoded
query parameters.

### Endpoints discovered

| Endpoint | Purpose |
|---|---|
| `GET https://amenic-file.com/main?v=1.7.3&r=<device_id>` | Home screen HTML (server-rendered) |
| `GET https://amenic-file.com/js/players.js?<query>` | Player data (JS file with video URLs) |
| `GET https://thumb.fvs.io/asset<path>` | Thumbnails (posters, backdrops) |

### Player data shape

From `assets/www/player.html`:

```javascript
// params.data is base64-decoded from the URL query string
var data = JSON.parse(atob(params.data));

// Direct video URLs (one per quality)
data.data[i].file   // e.g. "https://cdn.example.com/movie_720p.mp4"
data.data[i].label  // e.g. "720p", "1080p"

// Poster image
data.player.poster_file  // appended to https://thumb.fvs.io/asset

// Subtitles
data.captions[i].hash       // subtitle hash
data.captions[i].id         // subtitle ID
data.captions[i].language   // e.g. "Português"
data.captions[i].extension  // e.g. ".srt", ".vtt"
```

### Metadata source

The app uses **The Movie Database (TMDB) API** for title metadata
(posters, backdrops, synopses). It does **not** have its own catalog
API — the home screen is rendered server-side by `amenic-file.com/main`.

### User-Agent

The APK sends an Android WebView UA:
```
Mozilla/5.0 (Linux; Android 13; SM-S901B Build/TP1A.220624.014; wv) AppleWebKit/537.36 (KHTML, like Gecko) Version/4.0 Chrome/120.0.6099.230 Mobile Safari/537.36
```

We use this exact UA in the provider because Cloudflare's managed
challenge is more lenient with mobile UAs.

### Other strings found in the APK

```
Lmedia6/app/amenic/Main;
Lmedia6/app/amenic/PlayerActivity;
Lmedia6/app/amenic/PlayerVideo;
Lmedia6/app/amenic/PlayerWeb;
Lmedia6/app/amenic/Players;
Lmedia6/app/amenic/Download;
Lmedia6/app/amenic/ForceUpdate;
Lmedia6/app/amenic/Maintenance;
Lmedia6/app/amenic/NoInternet;

# JS injection (in classes3.dex)
javascript:var script = document.createElement('script');
  script.type = 'text/javascript';
  script.src = 'https://amenic-file.com/js/players.js?...';

# Home page load (in classes3.dex)
file:///android_asset/www/home.html?v=1.7.3&r=<android_id>
```

## Configuration

The provider is **disabled by default**. Enable it with the
`AMENIC_ENABLED=true` environment variable:

```bash
AMENIC_ENABLED=true go run ./cmd/server
```

### Environment variables

| Variable | Default | Description |
|---|---|---|
| `AMENIC_ENABLED` | `false` | Set to `true` to register the Amenic provider |
| `AMENIC_BASE` | `https://amenic-file.com` | Amenic file server root |
| `AMENIC_THUMB_BASE` | `https://thumb.fvs.io/asset` | Thumbnail CDN root |
| `AMENIC_APP_VERSION` | `1.7.3` | App version sent in `v` query param |
| `AMENIC_DEVICE_ID` | `supercine-proxy-amenic` | Device ID sent in `r` query param |

## Provider behavior

### Priority

The Amenic provider is registered with priority `200` (higher than
Supercine's `100`). This means:

- When the user calls `/v1/resolve?imdb=...&provider=amenic`, the
  registry tries Amenic first. If Amenic fails (Cloudflare block, no
  servers, etc.), the registry falls back to Supercine.
- When the user calls `/v1/resolve?imdb=...` (no provider specified),
  the registry tries providers in priority order: Supercine first
  (priority 100), then Amenic (priority 200). So Supercine remains the
  default.

### Health check

The `HealthCheck()` method does a HEAD request to `https://amenic-file.com/`.
Cloudflare's 403 (managed challenge) counts as "healthy" because it
means the host is reachable — the provider just can't fetch content
from datacenter IPs. Only HTTP 5xx and network errors count as down.

### Cloudflare detection

The `fetchURL()` method checks for Cloudflare's challenge response by
looking for these signatures in the response body:

- `"cloudflare"`
- `"Attention Required"`
- `"you have been blocked"`
- `"cf-mitigated"` header

When detected, the method returns `provider.ErrProviderDown` with a
clear message, so the registry can fall back to the next provider.

## Known limitations

1. **Cloudflare blocks datacenter IPs** — the provider only works from
   residential IPs or browser-based environments.
2. **No catalog API** — Amenic doesn't expose a catalog endpoint; the
   home screen is server-rendered HTML. The provider can only resolve
   IMDB IDs that the user already has (from search or popular lists).
3. **TV episode support is stubbed** — `ResolveEpisode()` returns
   `ErrUnavailable` because Amenic requires the title's internal ID,
   which we can't derive from the IMDB ID alone without scraping the
   home page.
4. **Subtitle support not implemented** — the player data includes
   subtitle URLs but the provider doesn't expose them yet.

## Future work

1. **Cloudflare bypass** — integrate a headless browser (e.g. Rod,
   Chromedp) to solve the managed challenge and cache the
   `cf_clearance` cookie.
2. **Home page scraping** — parse `amenic-file.com/main` to build a
   catalog endpoint that mirrors what the app shows.
3. **TV episode resolution** — once we can map IMDB IDs to Amenic
   title IDs, implement `ResolveEpisode()`.
4. **Subtitle proxying** — expose subtitle URLs through the proxy so
   the browser can fetch them without CORS issues.
