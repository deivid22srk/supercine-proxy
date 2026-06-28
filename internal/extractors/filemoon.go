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

// FileMoon extractor — ported from tv.supercine.supercine.sites.FileMoon.
// FileMoon serves a packed JS that, once unpacked, contains file:"...m3u8...".
type FileMoon struct{}

func NewFileMoon() *FileMoon { return &FileMoon{} }

func (f *FileMoon) Name() string { return "filemoon" }

func (f *FileMoon) Match(u string) bool {
	return matchAny(u, `(?i).+(filemoon|96ar|tlnmoons)\.(com|co|to|sx|bz|in|top)/.+`)
}

var (
	filemoonPackedRe = regexp.MustCompile(`eval\(function\(p,a,c,k,e,[rd]`)
	filemoonM3U8Re   = regexp.MustCompile(`file:"(.*?m3u8.*?)"`)
)

func (f *FileMoon) Extract(ctx context.Context, rawURL string) (*types.ExtractorResult, error) {
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
		if !filemoonPackedRe.MatchString(data) {
			return
		}
		unpacked := unpackJS(data)
		if unpacked == "" {
			return
		}
		m := filemoonM3U8Re.FindStringSubmatch(unpacked)
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
