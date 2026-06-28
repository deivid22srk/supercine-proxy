package extractors

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"regexp"
	"strings"

	"github.com/deivid22srk/supercine-proxy/internal/types"
)

// StreamTape extractor — ported from tv.supercine.supercine.sites.StreamTape.
//
// Flow:
//  1. Replace /e/ with /v/ if present.
//  2. GET the page.
//  3. The page contains a robot/anti-robot div with a getLink string built
//     from two pieces. Two regex variants cover "norobot" and regular pages.
//  4. Build the stream link: https:{group1}{group2[3:]}&stream=1
//  5. Issue a non-redirect GET; capture the Location header -> final URL.
type StreamTape struct{}

func NewStreamTape() *StreamTape { return &StreamTape{} }

func (s *StreamTape) Name() string { return "streamtape" }

func (s *StreamTape) Match(u string) bool {
	return matchAny(u, `(?i).+(streamtape|streamadblockplus|stapewithadblock|shavetape|tapenoads|tapeantiads)\.(com|to|sx|bz|beauty|cash)/.+`)
}

var (
	stapeNoRobotRe = regexp.MustCompile(`ById\('.+robot.+?=.*(["']//[^;+]+).*'(.*?)'`)
	stapeRobotRe   = regexp.MustCompile(`ById\('?robot.+?=.*(["']//[^;+]+).*'(.*?)'`)
)

func (s *StreamTape) Extract(ctx context.Context, rawURL string) (*types.ExtractorResult, error) {
	pageURL := strings.Replace(rawURL, "/e/", "/v/", 1)

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

	var match []string
	if strings.Contains(bodyStr, "norobot") {
		match = stapeNoRobotRe.FindStringSubmatch(bodyStr)
	} else {
		match = stapeRobotRe.FindStringSubmatch(bodyStr)
	}
	if len(match) < 3 {
		return nil, fmt.Errorf("StreamTape: padrão robot não encontrado em %s", rawURL)
	}
	group1 := strings.ReplaceAll(match[1], "'", "")
	group2 := match[2]
	if len(group2) < 3 {
		return nil, fmt.Errorf("StreamTape: group2 muito curto")
	}
	streamURL := "https:" + group1 + group2[3:] + "&stream=1"
	streamURL = strings.ReplaceAll(streamURL, " ", "")

	// Non-redirect GET to capture Location header.
	finalURL, err := fetchRedirectLocation(ctx, streamURL, pageURL)
	if err != nil {
		return nil, err
	}

	videos := []types.Jmodel{
		{URL: finalURL, Quality: "Normal"},
	}

	return &types.ExtractorResult{
		Hoster:     s.Name(),
		URL:        rawURL,
		StatusCode: resp.StatusCode,
		Videos:     videos,
	}, nil
}

// fetchRedirectLocation does a manual HEAD/GET without following redirects,
// exactly like the APK does (setInstanceFollowRedirects(false)).
func fetchRedirectLocation(ctx context.Context, target, referer string) (string, error) {
	noRedirectClient := &http.Client{
		Timeout: 20 * 1000000000, // 20s
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			return http.ErrUseLastResponse
		},
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, target, nil)
	if err != nil {
		return "", err
	}
	req.Header.Set("User-Agent", UserAgent)
	req.Header.Set("Referer", referer)
	resp, err := noRedirectClient.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	loc := resp.Header.Get("Location")
	if loc == "" {
		return "", fmt.Errorf("StreamTape: nenhum cabeçalho Location retornado")
	}
	u, err := url.Parse(loc)
	if err != nil {
		return loc, nil
	}
	return u.String(), nil
}
