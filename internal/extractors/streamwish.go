package extractors

import (
	"context"
	"fmt"
	"io"
	"regexp"
	"strings"

	"github.com/PuerkitoBio/goquery"
	"github.com/deivid22srk/supercine-proxy/internal/types"
)

// StreamWish extractor — ported from tv.supercine.supercine.sites.StreamWish.
//
// Flow:
//  1. GET the page.
//  2. Find every <script> block; if it contains "eval(function(p,a,c,k,e," run
//     JSUnpacker on it and then scan for video sources.
//  3. Also scan <source> and <video> tags for src/data-src attributes.
type StreamWish struct{}

func NewStreamWish() *StreamWish { return &StreamWish{} }

func (s *StreamWish) Name() string { return "streamwish" }

func (s *StreamWish) Match(u string) bool {
	return matchAny(u, `(?i).+(streamwish|asnwish|tlnwish|playerwish|tln-hg)\.(com|co|to|sx|bz|xyz|top)/.+`)
}

var (
	streamwishPackedRe = regexp.MustCompile(`eval\(function\(p,a,c,k,e,.*\)`)
	streamwishSourceRe = regexp.MustCompile(`["']?(file|hls|hls2|src|link|url|path)["']?\s*[:=]\s*["']([^"']+\.(mp4|m3u8|mkv)[^"']*)["']`)
	streamwishSourcesRe = regexp.MustCompile(`sources:\s*\[\s*\{\s*file:\s*["']([^"']+)["']`)
)

func (s *StreamWish) Extract(ctx context.Context, rawURL string) (*types.ExtractorResult, error) {
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

	// Walk all <script> tags
	doc.Find("script").Each(func(_ int, sel *goquery.Selection) {
		data := sel.Text()
		if strings.Contains(data, "eval(function(p,a,c,k,e,") {
			if unpacked := unpackJS(data); unpacked != "" {
				extractSourcesFromString(unpacked, &videos, seen)
			}
		}
		extractSourcesFromString(data, &videos, seen)
	})

	// Fallback: scan full body if no sources found
	if len(videos) == 0 {
		extractSourcesFromString(bodyStr, &videos, seen)
	}

	// Walk <source> and <video> tags for src/data-src
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
			Hoster:     s.Name(),
			URL:        rawURL,
			StatusCode: resp.StatusCode,
			Error:      "nenhuma fonte de vídeo encontrada",
		}, fmt.Errorf("nenhuma fonte encontrada")
	}

	return &types.ExtractorResult{
		Hoster:     s.Name(),
		URL:        rawURL,
		StatusCode: resp.StatusCode,
		Videos:     videos,
	}, nil
}

// extractSourcesFromString scans text for file:/hls:/src:/etc patterns.
func extractSourcesFromString(text string, videos *[]types.Jmodel, seen map[string]bool) {
	matches := streamwishSourceRe.FindAllStringSubmatch(text, -1)
	for _, m := range matches {
		raw := strings.ReplaceAll(m[2], `\/`, "/")
		addSource(raw, videos, seen)
	}
	matches = streamwishSourcesRe.FindAllStringSubmatch(text, -1)
	for _, m := range matches {
		raw := strings.ReplaceAll(m[1], `\/`, "/")
		addSource(raw, videos, seen)
	}
}

// addSource normalizes a URL (// -> https://) and deduplicates.
func addSource(raw string, videos *[]types.Jmodel, seen map[string]bool) {
	if raw == "" {
		return
	}
	if strings.HasPrefix(raw, "//") {
		raw = "https:" + raw
	}
	if seen[raw] {
		return
	}
	seen[raw] = true
	*videos = append(*videos, types.Jmodel{
		URL:     raw,
		Quality: qualityFor(raw),
	})
}
