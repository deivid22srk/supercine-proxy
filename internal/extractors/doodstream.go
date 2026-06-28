package extractors

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"regexp"
	"strings"

	"github.com/deivid22srk/supercine-proxy/internal/types"
)

// DoodStream extractor — ported from tv.supercine.supercine.sites.DoodStream.
//
// Flow:
//  1. GET the /e/<id> page (replace /d/ with /e/ if present).
//  2. Regex out the /pass_md5/... path.
//  3. GET {base}/pass_md5/... with Referer to obtain a 10-char random salt.
//  4. Final URL = md5_response + randomStr(10) + "?token=" + last URL segment.
type DoodStream struct{}

func NewDoodStream() *DoodStream { return &DoodStream{} }

func (d *DoodStream) Name() string { return "doodstream" }

func (d *DoodStream) Match(u string) bool {
	return matchAny(u, `(?i).+(doodstream|dood|vidply|do7go)\.(com|watch|to|so|la|ws|sh|pm|re|li)/.+`)
}

var (
	doodPassRe = regexp.MustCompile(`/pass_md5/[^']+`)
)

func (d *DoodStream) Extract(ctx context.Context, rawURL string) (*types.ExtractorResult, error) {
	// Normalize: /d/ -> /e/
	pageURL := strings.Replace(rawURL, "/d/", "/e/", 1)

	client := httpClient()

	// Step 1: fetch the embed page.
	req, err := newGet(ctx, pageURL, pageURL)
	if err != nil {
		return nil, err
	}
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

	// Step 2: locate /pass_md5/...
	match := doodPassRe.FindString(bodyStr)
	if match == "" {
		return nil, fmt.Errorf("pass_md5 não encontrado na página do DoodStream")
	}

	// Build absolute URL.
	base, err := getBaseURL(pageURL)
	if err != nil {
		return nil, err
	}
	passURL := base + match

	// Step 3: GET the pass_md5 endpoint to get the hash.
	req2, _ := newGet(ctx, passURL, pageURL)
	resp2, err := client.Do(req2)
	if err != nil {
		return nil, err
	}
	defer resp2.Body.Close()
	hash, err := io.ReadAll(resp2.Body)
	if err != nil {
		return nil, err
	}

	// Step 4: assemble final URL: hash + randomStr(10) + "?token=" + last segment.
	segments := strings.Split(passURL, "/")
	lastSeg := segments[len(segments)-1]
	token := randomString(10)
	finalURL := string(hash) + token + "?token=" + lastSeg

	return &types.ExtractorResult{
		Hoster:     d.Name(),
		URL:        rawURL,
		StatusCode: resp.StatusCode,
		Videos: []types.Jmodel{
			{URL: finalURL, Quality: "Normal"},
		},
	}, nil
}

// getBaseURL extracts scheme://host from a URL.
func getBaseURL(rawURL string) (string, error) {
	u, err := url.Parse(rawURL)
	if err != nil {
		return "", err
	}
	return u.Scheme + "://" + u.Host, nil
}

// randomString returns a hex-encoded random ASCII string of given length
// (matching DoodStream.randomStr which uses [A-Za-z0-9]).
func randomString(n int) string {
	b := make([]byte, n)
	_, _ = rand.Read(b)
	const charset = "ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz0123456789"
	out := make([]byte, n)
	for i, v := range b {
		out[i] = charset[int(v)%len(charset)]
	}
	return string(out)
}

// Unused but kept for parity with the APK (used for debug).
func encodeHex(b []byte) string { return hex.EncodeToString(b) }

// Ensure http variable is referenced (used by DoodStream).
var _ = http.MethodGet
