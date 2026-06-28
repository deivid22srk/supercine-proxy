package extractors

import (
        "context"
        "fmt"
        "io"
        "strings"

        "github.com/PuerkitoBio/goquery"
        "github.com/deivid22srk/supercine-proxy/internal/types"
)

// VidHide extractor — ported from tv.supercine.supercine.sites.VidHide.
// VidHide and StreamWish use identical scrapers in the APK; we keep them as
// separate types so the registry can dispatch by URL.
type VidHide struct{}

func NewVidHide() *VidHide { return &VidHide{} }

func (v *VidHide) Name() string { return "vidhide" }

func (v *VidHide) Match(u string) bool {
        return matchAny(u, `(?i).+(vidhide|vidhidevip|tlnhide|megahide|niikaplayerr|tln-earn)\.(com|co|to|sx|bz|live|online|in|site|xyz|shop|top)/.+`)
}

func (v *VidHide) Extract(ctx context.Context, rawURL string) (*types.ExtractorResult, error) {
        client := httpClient()
        req, _ := newGet(ctx, rawURL, rawURL)
        resp, err := client.Do(req)
        if err != nil {
                return nil, err
        }
        defer resp.Body.Close()
        body, err := io.ReadAll(resp.Body)
        if err != nil {
                return nil, err
        }
        bodyStr := string(body)

        doc, err := goquery.NewDocumentFromReader(strings.NewReader(bodyStr))
        if err != nil {
                return nil, fmt.Errorf("falha ao parsear HTML: %w", err)
        }

        videos := []types.Jmodel{}
        seen := map[string]bool{}

        doc.Find("script").Each(func(_ int, sel *goquery.Selection) {
                data := sel.Text()
                if strings.Contains(data, "eval(function(p,a,c,k,e,") {
                        if unpacked := unpackJS(data); unpacked != "" {
                                extractSourcesFromString(unpacked, &videos, seen)
                        }
                }
                extractSourcesFromString(data, &videos, seen)
        })

        if len(videos) == 0 {
                extractSourcesFromString(bodyStr, &videos, seen)
        }

        doc.Find("source, video").Each(func(_ int, sel *goquery.Selection) {
                src, _ := sel.Attr("src")
                if src == "" {
                        src, _ = sel.Attr("data-src")
                }
                if src != "" && (strings.Contains(src, ".mp4") || strings.Contains(src, ".m3u8")) {
                        addSource(src, &videos, seen)
                }
        })

        if len(videos) == 0 {
                return &types.ExtractorResult{
                        Hoster:     v.Name(),
                        URL:        rawURL,
                        StatusCode: resp.StatusCode,
                        Error:      "nenhuma fonte de vídeo encontrada",
                }, fmt.Errorf("nenhuma fonte encontrada em %s", rawURL)
        }

        return &types.ExtractorResult{
                Hoster:     v.Name(),
                URL:        rawURL,
                StatusCode: resp.StatusCode,
                Videos:     videos,
        }, nil
}
