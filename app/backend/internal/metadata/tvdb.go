package metadata

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"time"
)

// TVDB looks shows up on TVDB (API v4) with the person's own key. HolyArrs
// ships no TVDB key: without one, every call answers ErrNoTVDBKey. The key
// is sent only in the login body, never in a URL or a log; the session
// token it buys is kept in memory.
type TVDB struct {
	BaseURL string                                    // default https://api4.thetvdb.com/v4
	Key     func(ctx context.Context) (string, error) // the person's key, read on each login
	HTTP    *http.Client
	Timeout time.Duration
	Gap     time.Duration
	TTL     time.Duration

	once  sync.Once
	g     *getter
	mu    sync.Mutex
	token string
	keyAt string // the key the token was bought with
}

func (d *TVDB) getter() *getter {
	d.once.Do(func() { d.g = &getter{name: "TVDB", http: d.HTTP, timeout: d.Timeout, gap: d.Gap, ttl: d.TTL} })
	return d.g
}

func (d *TVDB) base() string {
	if d.BaseURL == "" {
		return "https://api4.thetvdb.com/v4"
	}
	return strings.TrimRight(d.BaseURL, "/")
}

// session returns a token, logging in when there is none for this key.
func (d *TVDB) session(ctx context.Context, fresh bool) (string, error) {
	if d.Key == nil {
		return "", ErrNoTVDBKey
	}
	key, err := d.Key(ctx)
	if err != nil {
		return "", err
	}
	if key == "" {
		return "", ErrNoTVDBKey
	}
	d.mu.Lock()
	if !fresh && d.token != "" && d.keyAt == key {
		t := d.token
		d.mu.Unlock()
		return t, nil
	}
	d.mu.Unlock()
	body, _ := json.Marshal(map[string]string{"apikey": key})
	b, err := d.getter().do(ctx, []string{d.base(), key}, func(ctx context.Context) (*http.Request, error) {
		req, err := http.NewRequestWithContext(ctx, http.MethodPost, d.base()+"/login", bytes.NewReader(body))
		if err == nil {
			req.Header.Set("Content-Type", "application/json")
		}
		return req, err
	})
	var es errStatus
	if errors.As(err, &es) {
		if es.code == http.StatusUnauthorized || es.code == http.StatusForbidden {
			return "", ErrNoTVDBKey
		}
		return "", fmt.Errorf("TVDB answered %d", es.code)
	}
	if err != nil {
		return "", err
	}
	var res struct {
		Data struct {
			Token string `json:"token"`
		} `json:"data"`
	}
	if err := json.Unmarshal(b, &res); err != nil || res.Data.Token == "" {
		return "", errors.New("TVDB: odd login answer")
	}
	d.mu.Lock()
	d.token, d.keyAt = res.Data.Token, key
	d.mu.Unlock()
	return res.Data.Token, nil
}

// errTVDBNotFound is TVDB's 404; only a show lookup turns it into
// ErrShowNotFound (a missing episode page does not mean the show is gone).
var errTVDBNotFound = errors.New("TVDB answered 404")

// get sends an authorised GET; a refused session logs in once more.
func (d *TVDB) get(ctx context.Context, path string, q url.Values) ([]byte, error) {
	u := d.base() + path
	if len(q) > 0 {
		u += "?" + q.Encode()
	}
	for try := 0; try < 2; try++ {
		tok, err := d.session(ctx, try > 0)
		if err != nil {
			return nil, err
		}
		d.mu.Lock()
		key := d.keyAt
		d.mu.Unlock()
		b, err := d.getter().do(ctx, []string{d.base(), key}, func(ctx context.Context) (*http.Request, error) {
			req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
			if err == nil {
				req.Header.Set("Authorization", "Bearer "+tok)
			}
			return req, err
		})
		var es errStatus
		if errors.As(err, &es) {
			switch es.code {
			case http.StatusUnauthorized:
				continue
			case http.StatusNotFound:
				return nil, errTVDBNotFound
			}
			return nil, fmt.Errorf("TVDB answered %d", es.code)
		}
		return b, err
	}
	return nil, ErrNoTVDBKey
}

// tvdbMaxPages caps the episode pages read for one show.
const tvdbMaxPages = 20

func tvdbStatus(s string) string {
	switch s {
	case "Ended":
		return "ended"
	case "Upcoming":
		return "announced"
	}
	return "running"
}

// SearchShows finds shows by name on TVDB.
func (d *TVDB) SearchShows(ctx context.Context, query string) ([]Show, error) {
	b, err := d.get(ctx, "/search", url.Values{"query": {query}, "type": {"series"}})
	if errors.Is(err, errTVDBNotFound) {
		return []Show{}, nil
	}
	if err != nil {
		return nil, err
	}
	var res struct {
		Data []struct {
			ID       string `json:"tvdb_id"`
			Name     string `json:"name"`
			Year     string `json:"year"`
			Overview string `json:"overview"`
			Language string `json:"primary_language"`
			Image    string `json:"image_url"`
		} `json:"data"`
	}
	if err := json.Unmarshal(b, &res); err != nil {
		return nil, fmt.Errorf("TVDB: odd answer: %w", err)
	}
	out := make([]Show, 0, len(res.Data))
	for _, r := range res.Data {
		out = append(out, Show{Source: "tvdb", Name: r.Name, Year: yearOf(r.Year), Overview: r.Overview, OriginalLanguage: lang(r.Language),
			Poster: r.Image, Status: "running", IDs: map[string]string{"tvdb": r.ID}})
	}
	return out, nil
}

// Show takes "tvdb:<id>", or "imdb:<tt id>" or "tmdb:<id>" looked up by
// TVDB's remote id search, and returns the show in aired order with
// TVDB's absolute numbers.
func (d *TVDB) Show(ctx context.Context, id string) (Show, error) {
	kind, v, ok := splitID(id)
	if !ok {
		return Show{}, ErrShowNotFound
	}
	switch kind {
	case "imdb", "tmdb":
		b, err := d.get(ctx, "/search/remoteid/"+url.PathEscape(v), nil)
		if errors.Is(err, errTVDBNotFound) {
			return Show{}, ErrShowNotFound
		}
		if err != nil {
			return Show{}, err
		}
		var res struct {
			Data []struct {
				Series *struct {
					ID int64 `json:"id"`
				} `json:"series"`
			} `json:"data"`
		}
		if err := json.Unmarshal(b, &res); err != nil {
			return Show{}, ErrShowNotFound
		}
		for _, r := range res.Data {
			if r.Series != nil && r.Series.ID > 0 {
				return d.Show(ctx, "tvdb:"+strconv.FormatInt(r.Series.ID, 10))
			}
		}
		return Show{}, ErrShowNotFound
	case "tvdb":
	default:
		return Show{}, ErrShowNotFound
	}
	if n, err := strconv.ParseInt(v, 10, 64); err != nil || n <= 0 {
		return Show{}, ErrShowNotFound
	}
	b, err := d.get(ctx, "/series/"+v+"/extended", url.Values{"short": {"true"}})
	if errors.Is(err, errTVDBNotFound) {
		return Show{}, ErrShowNotFound
	}
	if err != nil {
		return Show{}, err
	}
	var res struct {
		Data struct {
			ID         int64  `json:"id"`
			Name       string `json:"name"`
			Year       string `json:"year"`
			FirstAired string `json:"firstAired"`
			Overview   string `json:"overview"`
			Language   string `json:"originalLanguage"`
			Status     *struct {
				Name string `json:"name"`
			} `json:"status"`
			AirsTime string `json:"airsTime"`
			Runtime  int    `json:"averageRuntime"`
			Network  *struct {
				Name string `json:"name"`
			} `json:"originalNetwork"`
			Image     string `json:"image"`
			RemoteIDs []struct {
				ID         string `json:"id"`
				SourceName string `json:"sourceName"`
			} `json:"remoteIds"`
		} `json:"data"`
	}
	if err := json.Unmarshal(b, &res); err != nil || res.Data.ID == 0 {
		return Show{}, errors.New("TVDB: odd answer")
	}
	sd := res.Data
	out := Show{Source: "tvdb", Name: sd.Name, Year: yearOf(sd.Year), Overview: sd.Overview, OriginalLanguage: lang(sd.Language),
		Status: "running", AirTime: sd.AirsTime, Runtime: sd.Runtime, Poster: sd.Image, IDs: map[string]string{"tvdb": strconv.FormatInt(sd.ID, 10)}}
	if out.Year == 0 {
		out.Year = yearOf(sd.FirstAired)
	}
	if sd.Status != nil {
		out.Status = tvdbStatus(sd.Status.Name)
	}
	if sd.Network != nil {
		out.Network = sd.Network.Name
	}
	for _, r := range sd.RemoteIDs {
		switch r.SourceName {
		case "IMDB":
			out.IDs["imdb"] = r.ID
		case "TheMovieDB.com":
			out.IDs["tmdb"] = r.ID
		case "TV Maze":
			out.IDs["tvmaze"] = r.ID
		}
	}
	for page := 0; ; page++ {
		if page == tvdbMaxPages {
			// A cut-short list would make a refresh delete real episodes.
			return Show{}, errors.New("TVDB list too long: more than " + strconv.Itoa(tvdbMaxPages) + " pages of episodes")
		}
		b, err := d.get(ctx, "/series/"+v+"/episodes/default", url.Values{"page": {strconv.Itoa(page)}})
		if err != nil {
			return Show{}, fmt.Errorf("TVDB episodes: %w", err)
		}
		var eps struct {
			Data struct {
				Episodes []struct {
					ID       int64   `json:"id"`
					Season   int     `json:"seasonNumber"`
					Number   int     `json:"number"`
					Absolute int     `json:"absoluteNumber"`
					Name     string  `json:"name"`
					Aired    *string `json:"aired"`
					Runtime  int     `json:"runtime"`
				} `json:"episodes"`
			} `json:"data"`
			Links struct {
				Next *string `json:"next"`
			} `json:"links"`
		}
		if err := json.Unmarshal(b, &eps); err != nil {
			return Show{}, fmt.Errorf("TVDB: odd answer: %w", err)
		}
		for _, e := range eps.Data.Episodes {
			ep := Episode{Season: e.Season, Number: e.Number, Absolute: e.Absolute, Name: e.Name, Runtime: e.Runtime, ID: strconv.FormatInt(e.ID, 10)}
			if e.Season == 0 {
				ep.Absolute = 0
			}
			if e.Aired != nil {
				ep.AirDate, ep.AirDay = dayEnd(*e.Aired)
			}
			out.Episodes = append(out.Episodes, ep)
		}
		if eps.Links.Next == nil || *eps.Links.Next == "" {
			break
		}
	}
	out.finish()
	return out, nil
}
