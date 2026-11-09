package metadata

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"
)

// TVMaze looks shows up on TVMaze's public API. It needs no key.
type TVMaze struct {
	BaseURL string // default https://api.tvmaze.com
	HTTP    *http.Client
	Timeout time.Duration
	Gap     time.Duration // least time between requests, default 250ms (TVMaze allows about 20 per 10 s)
	TTL     time.Duration

	once sync.Once
	g    *getter
}

func (m *TVMaze) getter() *getter {
	m.once.Do(func() { m.g = &getter{name: "TVMaze", http: m.HTTP, timeout: m.Timeout, gap: m.Gap, ttl: m.TTL} })
	return m.g
}

func (m *TVMaze) base() string {
	if m.BaseURL == "" {
		return "https://api.tvmaze.com"
	}
	return strings.TrimRight(m.BaseURL, "/")
}

func (m *TVMaze) get(ctx context.Context, path string, q url.Values) ([]byte, error) {
	u := m.base() + path
	if len(q) > 0 {
		u += "?" + q.Encode()
	}
	b, err := m.getter().do(ctx, []string{m.base()}, func(ctx context.Context) (*http.Request, error) {
		return http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
	})
	var es errStatus
	if errors.As(err, &es) {
		if es.code == http.StatusNotFound {
			return nil, ErrShowNotFound
		}
		return nil, fmt.Errorf("TVMaze answered %d", es.code)
	}
	return b, err
}

type mazeShow struct {
	ID             int64  `json:"id"`
	Name           string `json:"name"`
	Premiered      string `json:"premiered"`
	Summary        string `json:"summary"`
	Language       string `json:"language"`
	Status         string `json:"status"`
	AverageRuntime int    `json:"averageRuntime"`
	Schedule       struct {
		Time string `json:"time"`
	} `json:"schedule"`
	Network *struct {
		Name    string `json:"name"`
		Country *struct {
			TimeZone string `json:"timezone"`
		} `json:"country"`
	} `json:"network"`
	WebChannel *struct {
		Name string `json:"name"`
	} `json:"webChannel"`
	Externals struct {
		TVDB *int64 `json:"thetvdb"`
		IMDb string `json:"imdb"`
	} `json:"externals"`
	Image *struct {
		Original string `json:"original"`
	} `json:"image"`
}

// mazeAir reads one TVMaze episode's air day and time. The airstamp is the
// exact air instant only when TVMaze also knows the local air time (an
// empty airtime means TVMaze filled the stamp in itself); a missing
// airtime field is read as known, since the stamp is then all there is.
// Without a known time the episode gets its day's end and timed false.
func mazeAir(day string, airtime, stamp *string) (string, *time.Time, bool) {
	airDay := ""
	if t, ok := parseDay(day); ok {
		airDay = t.Format("2006-01-02")
	}
	if stamp != nil && (airtime == nil || strings.TrimSpace(*airtime) != "") {
		if t, err := time.Parse(time.RFC3339, *stamp); err == nil {
			u := t.UTC()
			return airDay, &u, true
		}
	}
	if airDay == "" {
		return "", nil, false
	}
	end, _ := dayEnd(airDay)
	return airDay, end, false
}

func mazeStatus(s string) string {
	switch s {
	case "Ended":
		return "ended"
	case "In Development":
		return "announced"
	}
	return "running"
}

func (s mazeShow) show() Show {
	out := Show{Source: "tvmaze", Name: s.Name, Overview: plain(s.Summary), OriginalLanguage: lang(s.Language), Year: yearOf(s.Premiered),
		Status: mazeStatus(s.Status), Runtime: s.AverageRuntime, AirTime: s.Schedule.Time,
		IDs: map[string]string{"tvmaze": strconv.FormatInt(s.ID, 10)}}
	if s.Network != nil {
		out.Network = s.Network.Name
		if s.Network.Country != nil {
			out.TimeZone = s.Network.Country.TimeZone
		}
	} else if s.WebChannel != nil {
		out.Network = s.WebChannel.Name
	}
	if s.Externals.TVDB != nil && *s.Externals.TVDB > 0 {
		out.IDs["tvdb"] = strconv.FormatInt(*s.Externals.TVDB, 10)
	}
	if s.Externals.IMDb != "" {
		out.IDs["imdb"] = s.Externals.IMDb
	}
	if s.Image != nil {
		out.Poster = s.Image.Original
	}
	return out
}

// SearchShows finds shows by name on TVMaze.
func (m *TVMaze) SearchShows(ctx context.Context, query string) ([]Show, error) {
	b, err := m.get(ctx, "/search/shows", url.Values{"q": {query}})
	if errors.Is(err, ErrShowNotFound) {
		return []Show{}, nil
	}
	if err != nil {
		return nil, err
	}
	var res []struct {
		Show mazeShow `json:"show"`
	}
	if err := json.Unmarshal(b, &res); err != nil {
		return nil, fmt.Errorf("TVMaze: odd answer: %w", err)
	}
	out := make([]Show, 0, len(res))
	for _, r := range res {
		out = append(out, r.Show.show())
	}
	return out, nil
}

// Show takes "tvmaze:<id>", or "tvdb:<id>" or "imdb:<tt id>" looked up on
// TVMaze, and returns the show with its episodes and specials. TVMaze puts
// specials inside the season they aired in, without a number; HolyArrs
// moves them to season 0 and numbers them in air order.
func (m *TVMaze) Show(ctx context.Context, id string) (Show, error) {
	kind, v, ok := splitID(id)
	if !ok {
		return Show{}, ErrShowNotFound
	}
	var b []byte
	var err error
	switch kind {
	case "tvmaze":
		if n, err := strconv.ParseInt(v, 10, 64); err != nil || n <= 0 {
			return Show{}, ErrShowNotFound
		}
		b, err = m.get(ctx, "/shows/"+v, nil)
	case "tvdb":
		b, err = m.get(ctx, "/lookup/shows", url.Values{"thetvdb": {v}})
	case "imdb":
		b, err = m.get(ctx, "/lookup/shows", url.Values{"imdb": {v}})
	default:
		return Show{}, ErrShowNotFound
	}
	if err != nil {
		return Show{}, err
	}
	var s mazeShow
	if err := json.Unmarshal(b, &s); err != nil || s.ID == 0 {
		return Show{}, fmt.Errorf("TVMaze: odd answer")
	}
	out := s.show()
	b, err = m.get(ctx, "/shows/"+strconv.FormatInt(s.ID, 10)+"/episodes", url.Values{"specials": {"1"}})
	if err != nil {
		return Show{}, err
	}
	var eps []struct {
		ID       int64   `json:"id"`
		Season   int     `json:"season"`
		Number   *int    `json:"number"`
		Name     string  `json:"name"`
		AirDay   string  `json:"airdate"`
		AirTime  *string `json:"airtime"`
		AirStamp *string `json:"airstamp"`
		Runtime  int     `json:"runtime"`
	}
	if err := json.Unmarshal(b, &eps); err != nil {
		return Show{}, fmt.Errorf("TVMaze: odd answer: %w", err)
	}
	var specials []Episode
	for _, e := range eps {
		ep := Episode{Season: e.Season, Name: e.Name, Runtime: e.Runtime, ID: strconv.FormatInt(e.ID, 10)}
		ep.AirDay, ep.AirDate, ep.Timed = mazeAir(e.AirDay, e.AirTime, e.AirStamp)
		if e.Number == nil {
			ep.Season = 0
			specials = append(specials, ep)
			continue
		}
		ep.Number = *e.Number
		out.Episodes = append(out.Episodes, ep)
	}
	sort.SliceStable(specials, func(i, j int) bool { return specials[i].AirDay < specials[j].AirDay })
	for i := range specials {
		specials[i].Number = i + 1
	}
	out.Episodes = append(out.Episodes, specials...)
	out.finish()
	return out, nil
}
