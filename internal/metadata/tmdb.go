package metadata

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"strconv"
	"strings"
	"sync"
	"time"
)

// ErrNoAccess means TMDB refused the token, or no token is set.
var ErrNoAccess = errors.New("TMDB refused access; check the TMDB read access token")

// ImageBase is where TMDB serves pictures.
const ImageBase = "https://image.tmdb.org/t/p"

// Attribution is the notice TMDB asks apps that use its API to show.
const Attribution = "This product uses the TMDB API but is not endorsed or certified by TMDB."

// EnvTokenFile names a file holding the TMDB read access token (for the lab).
const EnvTokenFile = "HOLYARRS_TMDB_TOKEN_FILE"

// TMDB looks movies up on The Movie Database with a v4 read access token,
// sent as a Bearer header and never in a URL.
type TMDB struct {
	BaseURL string                                    // default https://api.themoviedb.org/3
	Access  func(ctx context.Context) (string, error) // the read access token, read on each request; never logged
	HTTP    *http.Client
	Timeout time.Duration // per request, default 10s
	Gap     time.Duration // least time between requests, default 50ms
	TTL     time.Duration // cache time, default 6h
	// Images is where pictures are served (ImageBase when empty).
	Images string

	mu    sync.Mutex
	last  time.Time
	cache map[string]cached
}

type cached struct {
	body []byte
	at   time.Time
}

const maxCache = 1000

// TokenFile reads the token from a file on each call, so a changed file is
// picked up without a restart.
func TokenFile(path string) func(context.Context) (string, error) {
	return func(context.Context) (string, error) {
		b, err := os.ReadFile(path)
		if err != nil {
			return "", fmt.Errorf("reading the TMDB token file: %w", err)
		}
		return strings.TrimSpace(string(b)), nil
	}
}

func (t *TMDB) images() string {
	if t.Images == "" {
		return ImageBase
	}
	return strings.TrimRight(t.Images, "/")
}

func (t *TMDB) base() string {
	if t.BaseURL == "" {
		return "https://api.themoviedb.org/3"
	}
	return strings.TrimRight(t.BaseURL, "/")
}

// wait spaces requests by Gap.
func (t *TMDB) wait(ctx context.Context) error {
	gap := t.Gap
	if gap <= 0 {
		gap = 50 * time.Millisecond
	}
	t.mu.Lock()
	next := t.last.Add(gap)
	if now := time.Now(); next.Before(now) {
		next = now
	}
	t.last = next
	t.mu.Unlock()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-time.After(time.Until(next)):
		return nil
	}
}

func (t *TMDB) get(ctx context.Context, path string, q url.Values) ([]byte, error) {
	u := t.base() + path
	if len(q) > 0 {
		u += "?" + q.Encode()
	}
	ttl := t.TTL
	if ttl <= 0 {
		ttl = 6 * time.Hour
	}
	t.mu.Lock()
	if c, ok := t.cache[u]; ok && time.Since(c.at) < ttl && !noCache(ctx) {
		t.mu.Unlock()
		return c.body, nil
	}
	t.mu.Unlock()
	tok, err := t.Access(ctx)
	if err != nil {
		return nil, err
	}
	if tok == "" {
		return nil, ErrNoAccess
	}
	cl := t.HTTP
	if cl == nil {
		cl = http.DefaultClient
	}
	timeout := t.Timeout
	if timeout <= 0 {
		timeout = 10 * time.Second
	}
	for try := 0; ; try++ {
		if err := t.wait(ctx); err != nil {
			return nil, err
		}
		rctx, cancel := context.WithTimeout(ctx, timeout)
		req, _ := http.NewRequestWithContext(rctx, http.MethodGet, u, nil)
		req.Header.Set("Authorization", "Bearer "+tok)
		req.Header.Set("Accept", "application/json")
		resp, err := cl.Do(req)
		if err != nil {
			cancel()
			return nil, fmt.Errorf("TMDB: %w", stripURL(err))
		}
		body, err := io.ReadAll(io.LimitReader(resp.Body, 4<<20))
		resp.Body.Close()
		cancel()
		if err != nil {
			return nil, fmt.Errorf("TMDB: %w", stripURL(err))
		}
		switch {
		case resp.StatusCode == http.StatusOK:
			t.mu.Lock()
			if t.cache == nil || len(t.cache) >= maxCache {
				t.cache = map[string]cached{}
			}
			t.cache[u] = cached{body: body, at: time.Now()}
			t.mu.Unlock()
			return body, nil
		case resp.StatusCode == http.StatusNotFound:
			return nil, ErrNotFound
		case resp.StatusCode == http.StatusUnauthorized || resp.StatusCode == http.StatusForbidden:
			return nil, ErrNoAccess
		case resp.StatusCode == http.StatusTooManyRequests && try == 0:
			d, _ := strconv.Atoi(resp.Header.Get("Retry-After"))
			select {
			case <-ctx.Done():
				return nil, ctx.Err()
			case <-time.After(time.Duration(min(max(d, 0), 10)) * time.Second):
			}
		case resp.StatusCode == http.StatusTooManyRequests:
			return nil, errors.New("TMDB: rate limited; try again shortly")
		default:
			return nil, fmt.Errorf("TMDB answered %d", resp.StatusCode)
		}
	}
}

// stripURL drops the URL from a transport error; it holds the search words.
func stripURL(err error) error {
	var ue *url.Error
	if errors.As(err, &ue) {
		return ue.Err
	}
	return err
}

type tmdbMovie struct {
	ID                  int64  `json:"id"`
	Title               string `json:"title"`
	ReleaseDate         string `json:"release_date"`
	Overview            string `json:"overview"`
	OriginalLanguage    string `json:"original_language"`
	Runtime             int    `json:"runtime"`
	PosterPath          string `json:"poster_path"`
	IMDbID              string `json:"imdb_id"`
	BelongsToCollection *struct {
		ID   int64  `json:"id"`
		Name string `json:"name"`
	} `json:"belongs_to_collection"`
	ReleaseDates *struct {
		Results []struct {
			Country string `json:"iso_3166_1"`
			Dates   []struct {
				Date string `json:"release_date"`
				Type int    `json:"type"`
			} `json:"release_dates"`
		} `json:"results"`
	} `json:"release_dates,omitempty"`
}

// TMDB release types: 1 premiere, 2 limited cinema, 3 cinema, 4 digital,
// 5 physical, 6 TV.
const (
	relLimited  = 2
	relCinema   = 3
	relDigital  = 4
	relPhysical = 5
)

// dates picks the cinema, digital and physical dates: the United States
// dates when it has them, else the earliest date of that type anywhere.
// A cinema date falls back to a limited cinema run, then to the main
// release date.
func (m tmdbMovie) dates() (cinema, digital, physical *time.Time) {
	us, anywhere := map[int]time.Time{}, map[int]time.Time{}
	keep := func(into map[int]time.Time, typ int, t time.Time) {
		if cur, seen := into[typ]; !seen || t.Before(cur) {
			into[typ] = t
		}
	}
	if m.ReleaseDates != nil {
		for _, c := range m.ReleaseDates.Results {
			for _, d := range c.Dates {
				t, ok := parseDay(d.Date)
				if !ok {
					continue
				}
				keep(anywhere, d.Type, t)
				if c.Country == "US" {
					keep(us, d.Type, t)
				}
			}
		}
	}
	pick := func(typ int) *time.Time {
		if t, ok := us[typ]; ok {
			return &t
		}
		if t, ok := anywhere[typ]; ok {
			return &t
		}
		return nil
	}
	cinema, digital, physical = pick(relCinema), pick(relDigital), pick(relPhysical)
	if cinema == nil {
		cinema = pick(relLimited)
	}
	if cinema == nil {
		if t, ok := parseDay(m.ReleaseDate); ok {
			cinema = &t
		}
	}
	return cinema, digital, physical
}

// parseDay reads "2024-03-01" or a full time, as UTC midnight of that day.
func parseDay(s string) (time.Time, bool) {
	if len(s) < 10 {
		return time.Time{}, false
	}
	t, err := time.Parse("2006-01-02", s[:10])
	return t, err == nil
}

func (m tmdbMovie) movie(images string) Movie {
	return m.movieWithCollection(images, false)
}

func (m tmdbMovie) movieWithCollection(images string, collectionKnown bool) Movie {
	out := Movie{Name: m.Title, Overview: m.Overview, OriginalLanguage: m.OriginalLanguage, Runtime: m.Runtime,
		IDs: map[string]string{"tmdb": strconv.FormatInt(m.ID, 10)}, CollectionKnown: collectionKnown}
	if len(m.ReleaseDate) >= 4 {
		out.Year, _ = strconv.Atoi(m.ReleaseDate[:4])
	}
	if m.IMDbID != "" {
		out.IDs["imdb"] = m.IMDbID
	}
	if m.PosterPath != "" {
		out.Poster = images + "/w500" + m.PosterPath
	}
	if collectionKnown && m.BelongsToCollection != nil {
		out.Collection = &MovieCollection{ID: strconv.FormatInt(m.BelongsToCollection.ID, 10), Name: m.BelongsToCollection.Name}
	}
	out.CinemaDate, out.DigitalDate, out.PhysicalDate = m.dates()
	return out
}

// Search finds movies by name, and by year when year is not 0.
func (t *TMDB) Search(ctx context.Context, query string, year int) ([]Movie, error) {
	q := url.Values{"query": {query}, "include_adult": {"false"}}
	if year > 0 {
		q.Set("year", strconv.Itoa(year))
	}
	b, err := t.get(ctx, "/search/movie", q)
	if err != nil {
		return nil, err
	}
	var res struct {
		Results []tmdbMovie `json:"results"`
	}
	if err := json.Unmarshal(b, &res); err != nil {
		return nil, fmt.Errorf("TMDB: odd answer: %w", err)
	}
	out := make([]Movie, 0, len(res.Results))
	for _, m := range res.Results {
		out = append(out, m.movie(t.images()))
	}
	return out, nil
}

// Movie takes "tmdb:<id>" or "imdb:<tt id>".
func (t *TMDB) Movie(ctx context.Context, id string) (Movie, error) {
	src, v, ok := strings.Cut(id, ":")
	if !ok || v == "" {
		return Movie{}, ErrNotFound
	}
	switch src {
	case "imdb":
		if !strings.HasPrefix(v, "tt") || strings.ContainsAny(v, "/?#") {
			return Movie{}, ErrNotFound
		}
		b, err := t.get(ctx, "/find/"+url.PathEscape(v), url.Values{"external_source": {"imdb_id"}})
		if err != nil {
			return Movie{}, err
		}
		var res struct {
			Movies []tmdbMovie `json:"movie_results"`
		}
		if err := json.Unmarshal(b, &res); err != nil || len(res.Movies) == 0 {
			return Movie{}, ErrNotFound
		}
		return t.Movie(ctx, "tmdb:"+strconv.FormatInt(res.Movies[0].ID, 10))
	case "tmdb":
		n, err := strconv.ParseInt(v, 10, 64)
		if err != nil || n <= 0 {
			return Movie{}, ErrNotFound
		}
		b, err := t.get(ctx, "/movie/"+strconv.FormatInt(n, 10), url.Values{"append_to_response": {"release_dates"}})
		if err != nil {
			return Movie{}, err
		}
		var m tmdbMovie
		if err := json.Unmarshal(b, &m); err != nil {
			return Movie{}, fmt.Errorf("TMDB: odd answer: %w", err)
		}
		return m.movieWithCollection(t.images(), true), nil
	}
	return Movie{}, ErrNotFound
}

// maxChangePages caps how many pages of the changed list are read.
const maxChangePages = 50

// Changed lists the movies TMDB changed since since (TMDB keeps 14 days).
func (t *TMDB) Changed(ctx context.Context, since time.Time) ([]string, error) {
	var out []string
	start := since.UTC().Format("2006-01-02")
	end := time.Now().UTC().Format("2006-01-02")
	ctx = NoCache(ctx)
	for page := 1; page <= maxChangePages; page++ {
		b, err := t.get(ctx, "/movie/changes", url.Values{"start_date": {start}, "end_date": {end}, "page": {strconv.Itoa(page)}})
		if err != nil {
			return nil, err
		}
		var res struct {
			Results []struct {
				ID int64 `json:"id"`
			} `json:"results"`
			Page       int `json:"page"`
			TotalPages int `json:"total_pages"`
		}
		if err := json.Unmarshal(b, &res); err != nil {
			return nil, fmt.Errorf("TMDB: odd answer: %w", err)
		}
		for _, r := range res.Results {
			out = append(out, "tmdb:"+strconv.FormatInt(r.ID, 10))
		}
		if page >= res.TotalPages {
			break
		}
	}
	return out, nil
}
