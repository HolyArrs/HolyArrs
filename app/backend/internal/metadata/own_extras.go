package metadata

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"
)

// OwnExtras reads outside ratings from MDBList and title art from fanart.tv
// with the owner's own keys (Your own keys mode). Without a key each answers
// ErrNotFound, the same as a title the server has nothing for.
type OwnExtras struct {
	MDBListBase string // default https://api.mdblist.com
	FanartBase  string // default https://webservice.fanart.tv/v3
	MDBListKey  func(ctx context.Context) (string, error)
	FanartKey   func(ctx context.Context) (string, error)
	HTTP        *http.Client

	once sync.Once
	g    *getter
}

func (o *OwnExtras) getter() *getter {
	o.once.Do(func() {
		o.g = &getter{name: "Extras", http: o.HTTP, timeout: 8 * time.Second, gap: 250 * time.Millisecond, ttl: 12 * time.Hour}
	})
	return o.g
}

func ownBase(v, def string) string {
	if v == "" {
		return def
	}
	return strings.TrimRight(v, "/")
}

func ownKey(ctx context.Context, read func(context.Context) (string, error)) string {
	if read == nil {
		return ""
	}
	k, err := read(ctx)
	if err != nil {
		return ""
	}
	return strings.TrimSpace(k)
}

// get reads one answer; the key is only in the request, never in an error.
func (o *OwnExtras) get(ctx context.Context, u string, head map[string]string) ([]byte, error) {
	host := u
	if pu, err := url.Parse(u); err == nil {
		host = pu.Host
	}
	b, err := o.getter().do(ctx, []string{host}, func(ctx context.Context) (*http.Request, error) {
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
		if err == nil {
			req.Header.Set("Accept", "application/json")
			for k, v := range head {
				req.Header.Set(k, v)
			}
		}
		return req, err
	})
	var es errStatus
	switch {
	case errors.As(err, &es) && es.code == http.StatusNotFound:
		return nil, ErrNotFound
	case errors.As(err, &es) && (es.code == http.StatusUnauthorized || es.code == http.StatusForbidden):
		return nil, ErrNoAccess
	case errors.As(err, &es):
		return nil, ErrServiceUnavailable
	case err != nil:
		var ue *url.Error
		if errors.As(err, &ue) {
			err = ue.Err
		}
		return nil, err
	}
	return b, nil
}

// mdblistSources maps MDBList's rating names to the ones the title page
// shows, with each score's top value.
var mdblistSources = map[string]struct {
	name string
	max  float64
}{
	"imdb": {"imdb", 10}, "tomatoes": {"rt_critics", 100}, "popcorn": {"rt_audience", 100},
	"metacritic": {"metacritic", 100}, "letterboxd": {"letterboxd", 5}, "trakt": {"trakt", 100},
}

// Ratings reads a TMDB movie's or show's outside ratings from MDBList.
func (o *OwnExtras) Ratings(ctx context.Context, kind, id string) (ServerRatings, error) {
	key := ownKey(ctx, o.MDBListKey)
	if key == "" || (kind != "movie" && kind != "show") || !serviceID("tmdb", id) {
		return ServerRatings{}, ErrNotFound
	}
	u := ownBase(o.MDBListBase, "https://api.mdblist.com") + "/tmdb/" + kind + "/" + id + "?" + url.Values{"apikey": {key}}.Encode()
	b, err := o.get(ctx, u, nil)
	if err != nil {
		return ServerRatings{}, err
	}
	var raw struct {
		IDs struct {
			IMDb string `json:"imdb"`
		} `json:"ids"`
		Ratings []struct {
			Source string   `json:"source"`
			Value  *float64 `json:"value"`
			Votes  *int64   `json:"votes"`
		} `json:"ratings"`
	}
	if json.Unmarshal(b, &raw) != nil {
		return ServerRatings{}, ErrServiceInvalid
	}
	out := ServerRatings{}
	if serviceExternalID(raw.IDs.IMDb) {
		out.IMDbID = raw.IDs.IMDb
	}
	for _, r := range raw.Ratings {
		src, ok := mdblistSources[r.Source]
		if !ok || r.Value == nil || *r.Value <= 0 || *r.Value > src.max {
			continue
		}
		s := ServerScore{Source: src.name, Value: *r.Value, Max: src.max}
		if r.Votes != nil && *r.Votes > 0 {
			s.Votes = *r.Votes
		}
		out.Scores = append(out.Scores, s)
	}
	if len(out.Scores) == 0 {
		return out, ErrNotFound
	}
	return out, nil
}

// FanartPictureHost is where fanart.tv serves its pictures.
const FanartPictureHost = "https://assets.fanart.tv/"

// Art reads a TMDB movie's logo, banner, thumb and wide backgrounds from
// fanart.tv (fanart.tv lists shows by TheTVDB id only, so shows get none).
// Only English or language-free pictures on fanart.tv's own host are kept.
func (o *OwnExtras) Art(ctx context.Context, kind, id string) (ServerArt, error) {
	key := ownKey(ctx, o.FanartKey)
	if key == "" || kind != "movie" || !serviceID("tmdb", id) {
		return ServerArt{}, ErrNotFound
	}
	b, err := o.get(ctx, ownBase(o.FanartBase, "https://webservice.fanart.tv/v3")+"/movies/"+id, map[string]string{"api-key": key})
	if err != nil {
		return ServerArt{}, err
	}
	type pic struct {
		URL  string `json:"url"`
		Lang string `json:"lang"`
	}
	var raw struct {
		Logos       []pic `json:"hdmovielogo"`
		Banners     []pic `json:"moviebanner"`
		Thumbs      []pic `json:"moviethumb"`
		Backgrounds []pic `json:"moviebackground"`
	}
	if json.Unmarshal(b, &raw) != nil {
		return ServerArt{}, ErrServiceInvalid
	}
	first := func(ps []pic) string {
		for _, p := range ps {
			if FanartPicture(p.URL) && (p.Lang == "en" || p.Lang == "" || p.Lang == "00") {
				return p.URL
			}
		}
		return ""
	}
	out := ServerArt{Logo: first(raw.Logos), Banner: first(raw.Banners), Thumb: first(raw.Thumbs)}
	for _, p := range raw.Backgrounds {
		if FanartPicture(p.URL) && len(out.Backgrounds) < 8 {
			out.Backgrounds = append(out.Backgrounds, p.URL)
		}
	}
	if out.Logo == "" && out.Banner == "" && out.Thumb == "" && len(out.Backgrounds) == 0 {
		return out, ErrNotFound
	}
	return out, nil
}

// FanartPicture says u is a plain picture address on fanart.tv's own host.
func FanartPicture(u string) bool {
	rest, ok := strings.CutPrefix(u, FanartPictureHost)
	if !ok || rest == "" || len(u) > 300 || strings.ContainsAny(rest, "?#\\ \"'<>") || strings.Contains(rest, "..") {
		return false
	}
	low := strings.ToLower(rest)
	return strings.HasSuffix(low, ".png") || strings.HasSuffix(low, ".jpg") || strings.HasSuffix(low, ".jpeg") || strings.HasSuffix(low, ".webp")
}

// ModeArt picks the server's stored art, or fanart.tv with the owner's key,
// as the mode says.
func ModeArt(m *Modes, server, own func(ctx context.Context, kind, id string) (ServerArt, error)) func(ctx context.Context, kind, id string) (ServerArt, error) {
	return func(ctx context.Context, kind, id string) (ServerArt, error) {
		switch m.Current() {
		case ModeServer:
			return server(ctx, kind, id)
		case ModeOwn:
			return own(ctx, kind, id)
		}
		return ServerArt{}, ErrSourceOff
	}
}

// ModeRatings picks the server's ratings, or MDBList with the owner's key,
// as the mode says.
func ModeRatings(m *Modes, server, own func(ctx context.Context, kind, id string) (ServerRatings, error)) func(ctx context.Context, kind, id string) (ServerRatings, error) {
	return func(ctx context.Context, kind, id string) (ServerRatings, error) {
		switch m.Current() {
		case ModeServer:
			return server(ctx, kind, id)
		case ModeOwn:
			return own(ctx, kind, id)
		}
		return ServerRatings{}, ErrSourceOff
	}
}
