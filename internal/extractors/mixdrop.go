package extractors

import (
        "context"
        "fmt"
        "io"
        "net/url"
        "regexp"
        "strings"

        "github.com/deivid22srk/supercine-proxy/internal/types"
)

// MixDrop extractor — ported from tv.supercine.supercine.sites.MixDrop.
//
// Flow:
//  1. Replace /f/ with /e/ if present.
//  2. Strip query parameters from the URL. Supercine sometimes appends
//     subtitle parameters (sub1=...&sub1_label=...) to the MixDrop URL,
//     but MixDrop's server rejects requests with these parameters
//     (HTTP 400). The subtitle info isn't needed for extracting the
//     video URL, so we drop the query string entirely.
//  3. GET the page.
//  4. Find the eval(function(p,a,c,k,e,d)...split('|'),0,{})) block that
//     contains "MDCore" and unpack it; then locate wurl="..."; prepend https:.
type MixDrop struct{}

func NewMixDrop() *MixDrop { return &MixDrop{} }

func (m *MixDrop) Name() string { return "mixdrop" }

func (m *MixDrop) Match(u string) bool {
        return matchAny(u, `(?i).+(mixdrop)\.(com|co|to|sx|bz|ag|ch|pw|net|si|ms|ps)/.+`)
}

var (
        mixdropBlockRe = regexp.MustCompile(`eval\(function\(p,a,c,k,e,d\)(.*?)split`)
        mixdropWurlRe  = regexp.MustCompile(`wurl="(.*?)";`)
)

func (m *MixDrop) Extract(ctx context.Context, rawURL string) (*types.ExtractorResult, error) {
        pageURL := strings.Replace(rawURL, "/f/", "/e/", 1)

        // Strip query parameters. Supercine sometimes appends subtitle
        // parameters (sub1=...&sub1_label=...) to the MixDrop URL, but
        // MixDrop's server rejects requests with these parameters (HTTP 400).
        // The subtitle info isn't needed for extracting the video URL.
        if u, err := url.Parse(pageURL); err == nil && u.RawQuery != "" {
                u.RawQuery = ""
                pageURL = u.String()
        }

        client := httpClient()
        req, _ := newGet(ctx, pageURL, pageURL)
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

        var finalURL string
        matches := mixdropBlockRe.FindAllStringSubmatch(bodyStr, -1)
        for _, mt := range matches {
                block := "eval(function(p,a,c,k,e,d)" + mt[1] + "split('|'),0,{}))"
                if !strings.Contains(block, "MDCore") {
                        continue
                }
                unpacked := unpackJS(block)
                if unpacked == "" {
                        continue
                }
                wm := mixdropWurlRe.FindStringSubmatch(unpacked)
                if len(wm) >= 2 {
                        finalURL = "https:" + wm[1]
                        break
                }
        }

        if finalURL == "" {
                return &types.ExtractorResult{
                        Hoster:     m.Name(),
                        URL:        rawURL,
                        StatusCode: resp.StatusCode,
                        Error:      "wurl não encontrado após unpack do MDCore",
                }, fmt.Errorf("MixDrop: wurl não encontrado em %s", rawURL)
        }

        videos := []types.Jmodel{
                {URL: finalURL, Quality: "Normal"},
        }

        return &types.ExtractorResult{
                Hoster:     m.Name(),
                URL:        rawURL,
                StatusCode: resp.StatusCode,
                Videos:     videos,
        }, nil
}
