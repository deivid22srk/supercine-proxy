package extractors

import (
	"fmt"
	"math"
	"regexp"
	"strconv"
	"strings"
)

// JSUnpacker is a Go port of tv.supercine.supercine.utils.JSUnpacker.
// It decodes p.a.c.k.e.r. packed JavaScript (`eval(function(p,a,c,k,e,d) ...`)
// without needing a JS runtime, by replaying the same string substitution
// the original packer performed.
//
// The decoder is regex-driven and matches the original Java behavior 1:1,
// including the same fallback radix of 36 and the same 62/95-char alphabets.

var (
	// packedBodyRe captures the four payload pieces inside the
	//   }('body', radix, count, 'words|split|by|pipe'.split('|'))
	// segment of a packed eval string.
	packedBodyRe = regexp.MustCompile(`\}\s*\('(.*)',\s*(.*?),\s*(\d+),\s*'(.*?)'\.split\('\|'\)`)
	packedDetectRe = regexp.MustCompile(`eval\(function\(p,a,c,k,e,(?:r|d)`)
	tokenRe       = regexp.MustCompile(`\b\w+\b`)
)

// unpackJS decodes a p.a.c.k.e.r. block and returns the unpacked source.
// Returns empty string if the input is not a valid packed block.
func unpackJS(packed string) string {
	if !packedDetectRe.MatchString(strings.ReplaceAll(packed, " ", "")) {
		return ""
	}
	m := packedBodyRe.FindStringSubmatch(packed)
	if len(m) != 5 {
		return ""
	}
	body := strings.ReplaceAll(m[1], `\'`, `'`)
	radixStr := m[2]
	countStr := m[3]
	words := strings.Split(m[4], "|")

	radix, err := strconv.Atoi(radixStr)
	if err != nil {
		radix = 36
	}
	count, err := strconv.Atoi(countStr)
	if err != nil {
		count = 0
	}
	if len(words) != count {
		// "Unknown p.a.c.k.e.r. encoding" — keep going anyway, the original
		// throws but we want best-effort results.
		return ""
	}

	unbase := newUnbase(radix)
	if unbase == nil {
		return ""
	}

	// Replay token replacement, tracking an offset just like the Java version.
	out := []byte(body)
	offset := 0
	for _, m := range tokenRe.FindAllIndex([]byte(body), -1) {
		start, end := m[0], m[1]
		tok := body[start:end]
		idx := unbase.unbase(tok)
		var replacement string
		if idx >= 0 && idx < len(words) {
			replacement = words[idx]
		}
		if replacement == "" {
			continue
		}
		// apply offset
		s := start + offset
		e := end + offset
		if s < 0 || e > len(out) {
			continue
		}
		out = append(out[:s], append([]byte(replacement), out[e:]...)...)
		offset += len(replacement) - (end - start)
	}
	return string(out)
}

// unbase converts a token to its integer index using the p.a.c.k.e.r. alphabet
// for the given radix. Mirrors JSUnpacker.Unbase.
type unbase struct {
	radix     int
	alphabet  string
	dict      map[string]int
}

func newUnbase(radix int) *unbase {
	u := &unbase{radix: radix}
	const alpha62 = "0123456789abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ"
	const alpha95 = " !\"#$%&'()*+,-./0123456789:;<=>?@ABCDEFGHIJKLMNOPQRSTUVWXYZ[\\]^_`abcdefghijklmnopqrstuvwxyz{|}~"
	switch {
	case radix > 36 && radix < 62:
		u.alphabet = alpha62[:radix]
	case radix == 62:
		u.alphabet = alpha62
	case radix > 62 && radix < 95:
		u.alphabet = alpha95[:radix]
	case radix == 95:
		u.alphabet = alpha95
	default:
		// radix <= 36: native strconv handles it
		return u
	}
	u.dict = make(map[string]int, len(u.alphabet))
	for i, ch := range u.alphabet {
		u.dict[string(ch)] = i
	}
	return u
}

func (u *unbase) unbase(token string) int {
	if u.alphabet == "" {
		n, err := strconv.ParseInt(token, u.radix, 64)
		if err != nil {
			return -1
		}
		return int(n)
	}
	// Reverse string, then for each char: result += radix^i * dict[char]
	rev := reverse(token)
	result := 0
	for i, ch := range rev {
		v, ok := u.dict[string(ch)]
		if !ok {
			return -1
		}
		result += int(float64(v) * math.Pow(float64(u.radix), float64(i)))
	}
	return result
}

func reverse(s string) string {
	r := []rune(s)
	for i, j := 0, len(r)-1; i < j; i, j = i+1, j-1 {
		r[i], r[j] = r[j], r[i]
	}
	return string(r)
}

// Ensure fmt is referenced for future error messages.
var _ = fmt.Sprintf
