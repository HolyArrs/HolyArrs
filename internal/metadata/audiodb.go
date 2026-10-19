package metadata

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"
)

// AudioDBBase is TheAudioDB's API.
const AudioDBBase = "https://www.theaudiodb.com/api/v1/json"

// AudioDB finds an artist's pictures in TheAudioDB: by MusicBrainz artist
// id, then by name. HolyArrs ships no TheAudioDB key (its shared test key
// is for development only): with no key, or a nil *AudioDB, nothing is
// asked and nothing found. Calls are at least Gap apart (2 s by default,
// the free tier's 30 a minute); a 429 waits Retry-After once.
type AudioDB struct {
	BaseURL string // default AudioDBBase
	// Key is the person's own key; empty (or nil) turns TheAudioDB off.
	Key       func(ctx context.Context) (string, error)
	HTTP      *http.Client
	Timeout   time.Duration
	Gap       time.Duration
	TTL       time.Duration
	UserAgent string

	once sync.Once
	g    *getter
}

// AudioDBPictures are an artist's picture URLs: the square thumb, the wide
// fanart and the logo; each may be "".
type AudioDBPictures struct {
	Thumb, Fanart, Logo string
}

func (a *AudioDB) gap() time.Duration {
	if a.Gap > 0 {
		return a.Gap
	}
	return 2 * time.Second
}

func (a *AudioDB) getter() *getter {
	a.once.Do(func() {
		ttl := a.TTL
		if ttl <= 0 {
			ttl = 24 * time.Hour
		}
		a.g = &getter{name: "TheAudioDB", http: a.HTTP, timeout: a.Timeout, gap: a.gap(), ttl: ttl, agent: a.UserAgent}
	})
	return a.g
}

func (a *AudioDB) base() string {
	if a.BaseURL == "" {
		return AudioDBBase
	}
	return strings.TrimRight(a.BaseURL, "/")
}

func (a *AudioDB) key(ctx context.Context) string {
	if a.Key != nil {
		if k, err := a.Key(ctx); err == nil && strings.TrimSpace(k) != "" {
			return strings.TrimSpace(k)
		}
	}
	return ""
}

type audioDBArtist struct {
	Name   string `json:"strArtist"`
	MBID   string `json:"strMusicBrainzID"`
	Thumb  string `json:"strArtistThumb"`
	Fanart string `json:"strArtistFanart"`
	Logo   string `json:"strArtistLogo"`
}

func (a *AudioDB) ask(ctx context.Context, key, page string, q url.Values) ([]audioDBArtist, error) {
	u := a.base() + "/" + url.PathEscape(key) + "/" + page + "?" + q.Encode()
	b, err := a.getter().do(ctx, []string{a.base()}, func(ctx context.Context) (*http.Request, error) {
		return http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
	})
	if err != nil {
		return nil, fmt.Errorf("TheAudioDB: %w", err)
	}
	var out struct {
		Artists []audioDBArtist `json:"artists"`
	}
	if err := json.Unmarshal(b, &out); err != nil {
		return nil, fmt.Errorf("TheAudioDB: %w", err)
	}
	return out.Artists, nil
}

// On says whether a key is set, so TheAudioDB is asked.
func (a *AudioDB) On(ctx context.Context) bool { return a != nil && a.key(ctx) != "" }

// Pictures finds the artist by MusicBrainz id, and only with no id match
// by name (an exact name first, then a loose one; an entry carrying the
// same MusicBrainz id wins).
func (a *AudioDB) Pictures(ctx context.Context, mbid, name string) (AudioDBPictures, error) {
	mbid, name = strings.TrimSpace(mbid), strings.TrimSpace(name)
	if a == nil {
		return AudioDBPictures{}, nil
	}
	key := a.key(ctx)
	if key == "" {
		return AudioDBPictures{}, nil
	}
	if mbid != "" {
		as, err := a.ask(ctx, key, "artist-mb.php", url.Values{"i": {mbid}})
		if err != nil {
			return AudioDBPictures{}, err
		}
		for _, x := range as {
			if x.Thumb != "" || x.Fanart != "" || x.Logo != "" {
				return AudioDBPictures{Thumb: x.Thumb, Fanart: x.Fanart, Logo: x.Logo}, nil
			}
		}
	}
	if name == "" {
		return AudioDBPictures{}, nil
	}
	as, err := a.ask(ctx, key, "search.php", url.Values{"s": {name}})
	if err != nil {
		return AudioDBPictures{}, err
	}
	for _, x := range as {
		if mbid != "" && strings.EqualFold(x.MBID, mbid) {
			return AudioDBPictures{Thumb: x.Thumb, Fanart: x.Fanart, Logo: x.Logo}, nil
		}
	}
	for _, loose := range []bool{false, true} {
		for _, x := range as {
			if sameArtistName(x.Name, name, loose) {
				return AudioDBPictures{Thumb: x.Thumb, Fanart: x.Fanart, Logo: x.Logo}, nil
			}
		}
	}
	return AudioDBPictures{}, nil
}
