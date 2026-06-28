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

// FileLions extractor — ported from tv.supercine.supercine.sites.FileLions.
// Identical scraping pattern to FileMoon (packed JS -> file:"...m3u8...").
type FileLions struct{}

func NewFileLions() *FileLions { return &FileLions{} }

func (f *FileLions) Name() string { return "filelions" }

func (f *FileLions) Match(u string) bool {
	return matchAny(u, `(?i).+(filelions)\.(live|online|to|sx|bz|in)/.+`)
}

var (
	filelionsPackedRe = regexp.MustCompile(`eval\(function\(p,a,c,k,e,[rd]`)
	filelionsM3U8Re   = regexp.MustCompile(`file:"(.*?m3u8.*?)"`)
)

func (f *FileLions) Extract(ctx context.Context, rawURL string) (*types.ExtractorResult, error) {
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

	doc.Find("script[type=text/javascript]").Each(func(_ int, sel *goquery.Selection) {
		data := sel.Text()
		if !filelionsPackedRe.MatchString(data) {
			return
		}
		unpacked := unpackJS(data)
		if unpacked == "" {
			return
		}
		m := filelionsM3U8Re.FindStringSubmatch(unpacked)
		if len(m) >= 2 {
			addSource(m[1], &videos, seen)
		}
	})

	if len(videos) == 0 {
		return &types.ExtractorResult{
			Hoster:     f.Name(),
			URL:        rawURL,
			StatusCode: resp.StatusCode,
			Error:      "nenhum m3u8 encontrado após unpack",
		}, fmt.Errorf("nenhuma fonte m3u8 encontrada em %s", rawURL)
	}

	return &types.ExtractorResult{
		Hoster:     f.Name(),
		URL:        rawURL,
		StatusCode: resp.StatusCode,
		Videos:     videos,
	}, nil
}
