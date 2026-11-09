package metadata

import (
	"net/http"
	"strings"
	"sync"
	"time"
	"unicode"

	"golang.org/x/text/runes"
	"golang.org/x/text/transform"
	"golang.org/x/text/unicode/norm"
)

// DeezerBase is Deezer's public API, which needs no key.
const DeezerBase = "https://api.deezer.com"

// ArtistImages reads Deezer's public artist profiles (top songs, explicit
// marks, album labels) and Wikipedia bios; it never fetches pictures. It is
// optional: a nil *ArtistImages finds nothing. Answers are kept for TTL (24
// hours by default), misses too.
type ArtistImages struct {
	BaseURL string // default DeezerBase
	HTTP    *http.Client
	Timeout time.Duration
	Gap     time.Duration // least time between requests, default 250ms
	TTL     time.Duration
	// UserAgent names HolyArrs to Deezer (UserAgentFor).
	UserAgent string
	// WikiBase is Wikipedia's REST address for artist bios (default WikiBioBase).
	WikiBase string

	once sync.Once
	g    *getter
}

func (d *ArtistImages) getter() *getter {
	d.once.Do(func() {
		ttl := d.TTL
		if ttl <= 0 {
			ttl = 24 * time.Hour
		}
		d.g = &getter{name: "Deezer", http: d.HTTP, timeout: d.Timeout, gap: d.Gap, ttl: ttl, agent: d.UserAgent}
	})
	return d.g
}

func (d *ArtistImages) base() string {
	if d.BaseURL == "" {
		return DeezerBase
	}
	return strings.TrimRight(d.BaseURL, "/")
}

func sameArtistName(a, b string, loose bool) bool {
	if !loose {
		return strings.EqualFold(strings.TrimSpace(a), strings.TrimSpace(b))
	}
	n := normalName(a)
	return n != "" && n == normalName(b)
}

// normalName folds a name for loose matching: lower case, no accents, only
// letters and digits, and no leading "the".
func normalName(s string) string {
	s = strings.ToLower(strings.TrimSpace(s))
	if t, _, err := transform.String(transform.Chain(norm.NFD, runes.Remove(runes.In(unicode.Mn)), norm.NFC), s); err == nil {
		s = t
	}
	s = strings.TrimPrefix(s, "the ")
	var b strings.Builder
	for _, r := range s {
		if unicode.IsLetter(r) || unicode.IsDigit(r) {
			b.WriteRune(r)
		}
	}
	return b.String()
}
