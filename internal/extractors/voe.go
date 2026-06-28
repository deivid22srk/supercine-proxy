package extractors

import (
	"context"
	"encoding/base64"
	"fmt"
	"io"
	"regexp"
	"strings"

	"github.com/PuerkitoBio/goquery"
	"github.com/deivid22srk/supercine-proxy/internal/types"
)

// Voe extractor — ported from tv.supercine.supercine.sites.Voe.
//
// Flow:
//  1. GET the page.
//  2. Find every <script> block; if it contains "sources =",
//     regex out the "hls":"<base64>" pair.
//  3. Base64-decode the value -> direct m3u8 URL.
type Voe struct{}

func NewVoe() *Voe { return &Voe{} }

func (v *Voe) Name() string { return "voe" }

func (v *Voe) Match(u string) bool {
	return matchAny(u, `(?i).+(voe|donaldlineelse|jamessoundcost)\.(com|co|to|sx|bz)/.+`)
}

var voeHLSRe = regexp.MustCompile(`["']hls["']:\s*["'](.*?)['"]`)

func (v *Voe) Extract(ctx context.Context, rawURL string) (*types.ExtractorResult, error) {
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
		if !strings.Contains(data, "sources =") {
			return
		}
		m := voeHLSRe.FindStringSubmatch(data)
		if len(m) < 2 {
			return
		}
		decoded, err := base64.StdEncoding.DecodeString(m[1])
		if err != nil {
			return
		}
		addSource(string(decoded), &videos, seen)
	})

	if len(videos) == 0 {
		return &types.ExtractorResult{
			Hoster:     v.Name(),
			URL:        rawURL,
			StatusCode: resp.StatusCode,
			Error:      "nenhum hls base64 encontrado",
		}, fmt.Errorf("Voe: nenhum hls encontrado em %s", rawURL)
	}

	return &types.ExtractorResult{
		Hoster:     v.Name(),
		URL:        rawURL,
		StatusCode: resp.StatusCode,
		Videos:     videos,
	}, nil
}
