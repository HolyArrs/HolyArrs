package extras

import (
	"context"
	"encoding/json"
	"net/http"
	"regexp"
	"sort"
	"strconv"
	"time"
)

// Fanart answers fanart.tv artwork, cached 7 days. Off without Key.
type Fanart struct {
	Key string
	// ClientKey is a personal key sent with the project key (fanart.tv
	// serves newer images to it).
	ClientKey string
	URL       string
	UserAgent string
	HTTP      *http.Client
	Now       func() time.Time
	// TVDB maps a TMDB show id to its TVDB id (fanart.tv keys shows by
	// TVDB); nil takes the id as a TVDB id.
	TVDB func(ctx context.Context, tmdb string) (string, error)
	c    cache
	once bool
}

// Art is the chosen artwork for one title or artist ("" when none).
// Lists are best first, at most 5.
type Art struct {
	Kind        string   `json:"kind"`
	ID          string   `json:"id"`
	Logo        string   `json:"logo,omitempty"`
	ClearArt    string   `json:"clearart,omitempty"`
	Banner      string   `json:"banner,omitempty"`
	Thumb       string   `json:"thumb,omitempty"`
	Backgrounds []string `json:"backgrounds,omitempty"`
	Thumbs      []string `json:"thumbs,omitempty"`
	Source      string   `json:"source"`
}

type fanartImage struct {
	URL   string `json:"url"`
	Lang  string `json:"lang"`
	Likes string `json:"likes"`
	ID    string `json:"id"`
}

// Enabled reports whether the key is set.
func (f *Fanart) Enabled() bool { return f != nil && f.Key != "" }

// Status is the source state for /v1/stats.
func (f *Fanart) Status() Status {
	if f == nil {
		return Status{State: StateDisabledNoKey}
	}
	return f.c.status(f.Enabled())
}

func (f *Fanart) init() {
	if !f.once {
		f.once = true
		f.c.TTL, f.c.MissTTL, f.c.Max, f.c.Now = 7*24*time.Hour, 24*time.Hour, 50000, f.Now
	}
}

// Art answers the JSON body for kind (movie and tv: TMDB id, music:
// MusicBrainz artist id). ErrDisabled without a key, ErrNotFound when the
// source has nothing usable.
func (f *Fanart) Art(ctx context.Context, kind, id string) ([]byte, string, error) {
	if !f.Enabled() {
		return nil, "", ErrDisabled
	}
	f.c.mu.Lock()
	f.init()
	f.c.mu.Unlock()
	return f.c.get(ctx, kind+":"+id, func(ctx context.Context) ([]byte, error) { return f.fetch(ctx, kind, id) })
}

func (f *Fanart) fetch(ctx context.Context, kind, id string) ([]byte, error) {
	base := f.URL
	if base == "" {
		base = FanartURL
	}
	path := map[string]string{"movie": "movies", "tv": "tv", "music": "music"}[kind]
	upID := id
	if kind == "tv" && f.TVDB != nil {
		tvdb, err := f.TVDB(ctx, id)
		if err != nil {
			return nil, err
		}
		upID = tvdb
	}
	h := http.Header{}
	h.Set("api-key", f.Key)
	if f.ClientKey != "" {
		h.Set("client-key", f.ClientKey)
	}
	var raw map[string]json.RawMessage
	if err := getJSON(ctx, f.HTTP, base+"/"+path+"/"+upID, f.UserAgent, h, &raw); err != nil {
		return nil, err
	}
	list := func(names ...string) []string {
		var all []fanartImage
		for _, n := range names {
			var imgs []fanartImage
			if json.Unmarshal(raw[n], &imgs) == nil {
				all = append(all, imgs...)
			}
		}
		return pickFanart(all)
	}
	first := func(l []string) string {
		if len(l) > 0 {
			return l[0]
		}
		return ""
	}
	a := Art{Kind: kind, ID: id, Source: "fanart.tv"}
	switch kind {
	case "movie":
		a.Logo, a.ClearArt, a.Banner = first(list("hdmovielogo")), first(list("hdmovieclearart")), first(list("moviebanner"))
		a.Backgrounds = list("moviebackground")
	case "tv":
		a.Logo, a.ClearArt, a.Banner = first(list("hdtvlogo")), first(list("hdclearart")), first(list("tvbanner"))
		a.Backgrounds = list("showbackground")
	case "music":
		a.Logo = first(list("hdmusiclogo", "musiclogo"))
		a.Thumbs = list("artistthumb")
		a.Thumb = first(a.Thumbs)
		a.Backgrounds = list("artistbackground")
	}
	if a.Logo == "" && a.ClearArt == "" && a.Banner == "" && a.Thumb == "" && len(a.Backgrounds) == 0 {
		return nil, ErrNotFound
	}
	return json.Marshal(a)
}

// hexStamp reads the upload time fanart.tv puts in a file name
// (".../drake-4fd17958f1286.jpg": 8 hex digits of a Unix time).
var hexStamp = regexp.MustCompile(`-([0-9a-f]{8})[0-9a-f]*\.[a-z]+$`)

// fanartTime is when an image was uploaded: the hex time in its file name,
// else its numeric id (ids rise with upload time; 0 when neither is known).
func fanartTime(im fanartImage) int64 {
	if m := hexStamp.FindStringSubmatch(im.URL); m != nil {
		if t, err := strconv.ParseInt(m[1], 16, 64); err == nil {
			return t
		}
	}
	id, _ := strconv.ParseInt(im.ID, 10, 64)
	return id
}

// pickFanart orders images English first, then no language, then newest
// first (so the freshest photo leads), then most liked; other languages are dropped. At most 5, https only.
func pickFanart(imgs []fanartImage) []string {
	rank := func(l string) int {
		switch l {
		case "en":
			return 0
		case "", "00":
			return 1
		}
		return 9
	}
	sort.SliceStable(imgs, func(i, j int) bool {
		ri, rj := rank(imgs[i].Lang), rank(imgs[j].Lang)
		if ri != rj {
			return ri < rj
		}
		if ti, tj := fanartTime(imgs[i]), fanartTime(imgs[j]); ti != tj {
			return ti > tj
		}
		li, _ := strconv.Atoi(imgs[i].Likes)
		lj, _ := strconv.Atoi(imgs[j].Likes)
		return li > lj
	})
	var out []string
	for _, im := range imgs {
		if rank(im.Lang) > 1 || len(im.URL) < 9 || im.URL[:8] != "https://" {
			continue
		}
		out = append(out, im.URL)
		if len(out) == 5 {
			break
		}
	}
	return out
}
