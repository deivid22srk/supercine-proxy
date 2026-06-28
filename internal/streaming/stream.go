// Package streaming — video stream proxy.
//
// The hoster CDNs (MixDrop's mxcontent.net, StreamWish's premilkyway.com,
// VidHide's cdn-centaurus.com, etc.) reject browser requests that don't
// carry the hoster's own Origin/Referer. When a user opens the streaming
// UI at https://supercine-proxy.onrender.com and clicks play, the browser
// tries to fetch the video stream with Origin: https://supercine-proxy.onrender.com,
// which the CDN rejects with 403.
//
// The /v1/stream endpoint solves this by acting as a transparent proxy:
// the browser fetches from /v1/stream?url=<video_url> on the same origin
// as the UI, and the server forwards the request to the CDN with the
// correct Origin/Referer headers for that hoster. The response is
// streamed back to the browser with permissive CORS headers.
//
// For HLS (m3u8) playlists, the endpoint also rewrites relative segment
// URLs in the playlist to point back at /v1/stream so the browser can
// fetch the segments through the proxy too.

package streaming

import (
        "context"
        "fmt"
        "io"
        "net/http"
        "net/url"
        "regexp"
        "strings"
        "time"
)

// streamUserAgent is the browser UA we send to hoster CDNs.
const streamUserAgent = "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/137.0.0.0 Safari/537.36"

// handleStream proxies a video URL through this server so the browser
// can play it without CORS/Origin issues.
//
//   GET /v1/stream?url=<video_url>
//
// The `url` query parameter must be a URL returned by /v1/resolve or
// /v1/resolveEpisode. The server fetches the URL with the correct
// Origin/Referer headers for the hoster CDN and streams the response
// back. Range requests are supported for seeking in mp4 files.
//
// For m3u8 playlists, relative segment URLs are rewritten to route
// through this proxy endpoint.
func (h *Handler) handleStream(w http.ResponseWriter, r *http.Request) {
        targetURL := strings.TrimSpace(r.URL.Query().Get("url"))
        if targetURL == "" {
                writeJSON(w, http.StatusBadRequest, map[string]string{
                        "error": "parâmetro 'url' é obrigatório",
                })
                return
        }
        // Validate the URL — only allow http/https to prevent SSRF.
        parsed, err := url.Parse(targetURL)
        if err != nil || (parsed.Scheme != "http" && parsed.Scheme != "https") {
                writeJSON(w, http.StatusBadRequest, map[string]string{
                        "error": "URL inválida — deve ser http ou https",
                })
                return
        }

        ctx, cancel := context.WithTimeout(r.Context(), 60*time.Second)
        defer cancel()

        // Build the upstream request, forwarding the Range header so the
        // browser can seek in mp4 files.
        upReq, err := http.NewRequestWithContext(ctx, http.MethodGet, targetURL, nil)
        if err != nil {
                writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
                return
        }
        // Forward the browser's Range header for seeking support.
        if rng := r.Header.Get("Range"); rng != "" {
                upReq.Header.Set("Range", rng)
        }
        upReq.Header.Set("User-Agent", streamUserAgent)
        upReq.Header.Set("Accept", "*/*")
        upReq.Header.Set("Accept-Language", "pt-BR,pt;q=0.9,en;q=0.8")
        // Set the Origin and Referer to match the hoster's page so the CDN
        // accepts the request. This mirrors what verifyURL does.
        if origin := originForCDNURL(targetURL); origin != "" {
                upReq.Header.Set("Origin", origin)
                upReq.Header.Set("Referer", origin+"/")
        } else {
                upReq.Header.Set("Referer", "https://supercine-tv.net/")
        }

        client := &http.Client{
                Timeout: 0, // no timeout — we stream
                CheckRedirect: func(req *http.Request, via []*http.Request) error {
                        // Keep our headers on redirects.
                        req.Header.Set("User-Agent", streamUserAgent)
                        return nil
                },
        }
        resp, err := client.Do(upReq)
        if err != nil {
                writeJSON(w, http.StatusBadGateway, map[string]string{
                        "error":  "falha ao buscar o stream do CDN",
                        "detail": err.Error(),
                })
                return
        }
        defer resp.Body.Close()

        // If the CDN returned an error, forward it to the browser with a
        // clear message so the user knows the hoster is down (not the proxy).
        if resp.StatusCode >= 400 {
                body, _ := io.ReadAll(io.LimitReader(resp.Body, 512))
                w.Header().Set("Content-Type", "application/json")
                w.WriteHeader(resp.StatusCode)
                _, _ = w.Write([]byte(fmt.Sprintf(
                        `{"error":"CDN retornou HTTP %d","detail":%q,"url":%q}`,
                        resp.StatusCode, string(body), targetURL,
                )))
                return
        }

        // Copy the CDN's response headers, but override CORS to be permissive
        // so the browser (same-origin) can access the stream.
        contentType := resp.Header.Get("Content-Type")
        w.Header().Set("Content-Type", contentType)
        if cl := resp.Header.Get("Content-Length"); cl != "" {
                w.Header().Set("Content-Length", cl)
        }
        if cr := resp.Header.Get("Content-Range"); cr != "" {
                w.Header().Set("Content-Range", cr)
        }
        // Accept-Ranges lets the browser seek in mp4 files.
        w.Header().Set("Accept-Ranges", "bytes")
        // Permissive CORS — the UI is same-origin but we set this anyway
        // in case the page is embedded in an iframe or accessed from a
        // different host.
        w.Header().Set("Access-Control-Allow-Origin", "*")
        w.Header().Set("Access-Control-Allow-Methods", "GET, HEAD, OPTIONS")
        w.Header().Set("Access-Control-Allow-Headers", "Range")
        w.Header().Set("Access-Control-Expose-Headers", "Content-Length, Content-Range, Accept-Ranges")
        // Allow the browser to cache segments for replay.
        w.Header().Set("Cache-Control", "public, max-age=3600")

        w.WriteHeader(resp.StatusCode)

        // For HLS m3u8 playlists, rewrite relative segment URLs to route
        // through this proxy so the browser can fetch them without CORS
        // issues. hls.js would otherwise try to fetch the segments directly
        // from the CDN, which would fail with 403.
        if strings.Contains(contentType, "mpegurl") || strings.HasSuffix(parsed.Path, ".m3u8") {
                body, err := io.ReadAll(resp.Body)
                if err != nil {
                        return
                }
                rewritten := rewriteM3U8(string(body), targetURL)
                _, _ = w.Write([]byte(rewritten))
                return
        }

        // For non-HLS (mp4, mkv, etc.), stream the body through.
        _, _ = io.Copy(w, resp.Body)
}

// rewriteM3U8 rewrites relative URLs in an HLS playlist to route through
// the /v1/stream proxy endpoint. Both standalone segment URLs (lines
// that don't start with '#') and URI="..." attributes inside HLS tags
// (e.g. #EXT-X-MEDIA, #EXT-X-I-FRAME-STREAM-INF) are rewritten so hls.js
// fetches every sub-resource through the proxy.
//
// Without rewriting the URI="..." attributes, hls.js would try to fetch
// audio variant playlists and I-frame playlists directly from the CDN,
// which would fail with 403 because the browser's Origin doesn't match
// the hoster.
func rewriteM3U8(playlist, baseURL string) string {
        base, err := url.Parse(baseURL)
        if err != nil {
                return playlist
        }
        lines := strings.Split(playlist, "\n")
        for i, line := range lines {
                trimmed := strings.TrimSpace(line)
                if trimmed == "" {
                        continue
                }
                // Case 1: standalone segment/playlist URL (line doesn't start with '#').
                if !strings.HasPrefix(trimmed, "#") {
                        segURL, err := url.Parse(trimmed)
                        if err != nil {
                                continue
                        }
                        absURL := base.ResolveReference(segURL).String()
                        lines[i] = "/v1/stream?url=" + url.QueryEscape(absURL)
                        continue
                }
                // Case 2: URI="..." attribute inside an HLS tag. These appear in
                // #EXT-X-MEDIA, #EXT-X-I-FRAME-STREAM-INF, #EXT-X-SESSION-DATA,
                // and a few other tags. We rewrite every URI="..." that points
                // to a .m3u8 or media segment.
                lines[i] = rewriteURIAttributes(line, base)
        }
        return strings.Join(lines, "\n")
}

// uriAttrRe matches URI="..." attributes inside HLS tags. We capture the
// opening quote style (" or ') so we can preserve it when rewriting.
var uriAttrRe = regexp.MustCompile(`URI="([^"]+)"`)

// rewriteURIAttributes rewrites every URI="..." attribute in an HLS tag
// line to route through /v1/stream. Relative URLs are resolved against
// the playlist's base URL.
func rewriteURIAttributes(line string, base *url.URL) string {
        return uriAttrRe.ReplaceAllStringFunc(line, func(match string) string {
                sub := uriAttrRe.FindStringSubmatch(match)
                if len(sub) < 2 {
                        return match
                }
                relURL := sub[1]
                // Only rewrite http/https/relative URLs. Leave data: and blob:
                // URIs alone.
                if strings.HasPrefix(relURL, "data:") || strings.HasPrefix(relURL, "blob:") {
                        return match
                }
                segURL, err := url.Parse(relURL)
                if err != nil {
                        return match
                }
                absURL := base.ResolveReference(segURL).String()
                return `URI="/v1/stream?url=` + url.QueryEscape(absURL) + `"`
        })
}

// originForCDNURL returns the Origin header value to send to a CDN based
// on the video URL's host. This is the same mapping used by
// SupercineProvider.verifyURL — kept in sync so the stream proxy and
// the verification use the same Origin.
func originForCDNURL(videoURL string) string {
        low := strings.ToLower(videoURL)
        switch {
        case strings.Contains(low, "mxcontent.net"):
                return "https://mixdrop.ps"
        case strings.Contains(low, "cdn-centaurus.com"):
                return "https://tln-hg.top"
        case strings.Contains(low, "premilkyway.com"):
                return "https://tln-hg.top"
        case strings.Contains(low, "dramiyos-cdn.com"):
                return "https://tln-earn.top"
        }
        return ""
}
