package metadata

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/HolyArrs/holyarrs/app/backend/internal/core/outbound"
	"github.com/HolyArrs/holyarrs/app/backend/internal/core/startup"
)

// Errors from show lookups.
var (
	// ErrShowNotFound means the source has no show with that id, or cannot
	// look up that kind of id.
	ErrShowNotFound = errors.New("no such show")
	// ErrNoTVDBKey means TVDB is not used: no key of the person's own is
	// set, or TVDB refused it.
	ErrNoTVDBKey = errors.New("TVDB needs a TVDB key, and none is set; use TMDB or TVMaze for this show")
	// ErrUnknownSource names a details source HolyArrs does not have.
	ErrUnknownSource = errors.New("show details source must be tmdb or tvmaze")
)

// Show is one looked-up show. Episodes are empty in search answers.
type Show struct {
	Source           string            `json:"source"` // the source that answered: tmdb, tvmaze, tvdb
	Name             string            `json:"name"`
	Year             int               `json:"year,omitempty"`
	Overview         string            `json:"overview,omitempty"`
	OriginalLanguage string            `json:"original_language,omitempty"`
	Status           string            `json:"status"` // announced, running, ended
	Network          string            `json:"network,omitempty"`
	AirTime          string            `json:"air_time,omitempty"`  // local HH:MM, "" when unknown
	TimeZone         string            `json:"time_zone,omitempty"` // IANA, "" when unknown
	Runtime          int               `json:"runtime,omitempty"`   // usual episode minutes
	IDs              map[string]string `json:"ids"`                 // tmdb, tvdb, tvmaze, imdb
	Poster           string            `json:"poster,omitempty"`
	Backdrop         string            `json:"backdrop,omitempty"`
	Logo             string            `json:"logo,omitempty"`
	Genres           []string          `json:"genres,omitempty"`
	Rating           string            `json:"rating,omitempty"` // US content rating
	Score            float64           `json:"score,omitempty"`  // members' average out of 10, one decimal
	Votes            int               `json:"votes,omitempty"`  // how many members voted
	People           []string          `json:"people,omitempty"`
	OtherNames       []string          `json:"other_names,omitempty"` // original, alternative and translated names
	Seasons          []Season          `json:"seasons,omitempty"`
	Episodes         []Episode         `json:"episodes,omitempty"`
	// AltStale says the release-name numbers could not be read this time
	// (the metadata server failed), so stored ones are kept.
	AltStale bool `json:"-"`
}

// Season is one season of a looked-up show; 0 holds the specials.
type Season struct {
	Number int    `json:"number"`
	Name   string `json:"name,omitempty"`
}

// Episode is one episode in the source's aired numbering. AirDay is the
// local day. AirDate is the exact air time in UTC when Timed; when only the
// day is known it is the end of that day (so the episode never counts as
// aired early) and Timed is false. AirDate is nil when even the day is
// unknown.
type Episode struct {
	Season   int        `json:"season"`
	Number   int        `json:"number"`
	Absolute int        `json:"absolute,omitempty"` // 0 when the source gives none
	Name     string     `json:"name,omitempty"`
	AirDate  *time.Time `json:"air_date,omitempty"`
	AirDay   string     `json:"air_day,omitempty"`
	Timed    bool       `json:"timed,omitempty"`
	Runtime  int        `json:"runtime,omitempty"`
	ID       string     `json:"id,omitempty"`    // the source's episode id
	Still    string     `json:"still,omitempty"` // still picture URL (w780), "" when none
	// AltSeason and AltEpisode are how release names number the episode
	// when that differs (an anime part TMDB keeps inside one season); 0 when
	// they agree or nothing is known.
	AltSeason  int `json:"alt_season,omitempty"`
	AltEpisode int `json:"alt_episode,omitempty"`
}

// ShowProvider finds shows. Show takes "<kind>:<id>": the source's own id
// kind, plus whatever other id kinds it can look up (tvdb, imdb, tmdb).
type ShowProvider interface {
	SearchShows(ctx context.Context, query string) ([]Show, error)
	Show(ctx context.Context, id string) (Show, error)
}

// ShowSources are the show details sources by name.
type ShowSources map[string]ShowProvider

// ValidShowSource reports whether name is a show details source.
func ValidShowSource(name string) bool { return name == "tmdb" || name == "tvmaze" }

// Get returns the named source.
func (ss ShowSources) Get(name string) (ShowProvider, error) {
	if p, ok := ss[name]; ok && p != nil {
		return p, nil
	}
	return nil, ErrUnknownSource
}

// finish sorts the episodes and fills the season list from them when the
// source gave none.
func (s *Show) finish() {
	sort.SliceStable(s.Episodes, func(i, j int) bool {
		a, b := s.Episodes[i], s.Episodes[j]
		if a.Season != b.Season {
			return a.Season < b.Season
		}
		return a.Number < b.Number
	})
	have := map[int]bool{}
	for _, se := range s.Seasons {
		have[se.Number] = true
	}
	for _, e := range s.Episodes {
		if !have[e.Season] {
			have[e.Season] = true
			s.Seasons = append(s.Seasons, Season{Number: e.Season})
		}
	}
	sort.Slice(s.Seasons, func(i, j int) bool { return s.Seasons[i].Number < s.Seasons[j].Number })
}

// dayEnd is the air time used when only the day is known: the end of that
// day in UTC, so an episode never counts as aired before its day is over.
func dayEnd(day string) (*time.Time, string) {
	t, ok := parseDay(day)
	if !ok {
		return nil, ""
	}
	end := t.AddDate(0, 0, 1)
	return &end, t.Format("2006-01-02")
}

// langs maps the language names and three-letter codes the sources use to
// two-letter codes.
var langs = map[string]string{"english": "en", "eng": "en", "japanese": "ja", "jpn": "ja", "korean": "ko", "kor": "ko",
	"spanish": "es", "spa": "es", "french": "fr", "fra": "fr", "german": "de", "deu": "de", "italian": "it", "ita": "it",
	"portuguese": "pt", "por": "pt", "chinese": "zh", "zho": "zh", "dutch": "nl", "nld": "nl", "swedish": "sv", "swe": "sv",
	"danish": "da", "dan": "da", "norwegian": "no", "nor": "no", "hindi": "hi", "hin": "hi", "russian": "ru", "rus": "ru"}

func lang(v string) string {
	v = strings.ToLower(strings.TrimSpace(v))
	if len(v) == 2 {
		return v
	}
	return langs[v]
}

var tags = regexp.MustCompile(`<[^>]*>`)

func plain(html string) string { return strings.TrimSpace(tags.ReplaceAllString(html, "")) }

func yearOf(day string) int {
	if len(day) < 4 {
		return 0
	}
	y, _ := strconv.Atoi(day[:4])
	return y
}

// splitID splits "<kind>:<id>".
func splitID(id string) (kind, v string, ok bool) {
	kind, v, ok = strings.Cut(id, ":")
	if !ok || v == "" || strings.ContainsAny(v, "/?#%\\ ") {
		return "", "", false
	}
	return kind, v, true
}

// getter sends spaced, time-limited GET and POST requests and caches GET
// answers; TVMaze and TVDB use it.
type getter struct {
	name    string
	http    *http.Client
	timeout time.Duration
	gap     time.Duration
	ttl     time.Duration
	agent   string // User-Agent, when set

	mu    sync.Mutex
	last  time.Time
	cache map[string]cached
	swept time.Time // when expired answers were last dropped
}

// put keeps an answer (g.mu held). Expired answers are dropped at most once a
// minute, so they never sit until the cache is full; low-memory mode keeps
// fewer (a re-view past them asks again).
func (g *getter) put(key string, body []byte, ttl time.Duration, now time.Time) {
	if now.Sub(g.swept) >= time.Minute {
		for k, c := range g.cache {
			if now.Sub(c.at) >= ttl {
				delete(g.cache, k)
			}
		}
		g.swept = now
	}
	if g.cache == nil || len(g.cache) >= startup.LowMemoryPick(maxCache, 250) {
		g.cache = map[string]cached{}
	}
	g.cache[key] = cached{body: body, at: now}
}

func (g *getter) wait(ctx context.Context) error {
	gap := g.gap
	if gap <= 0 {
		gap = 250 * time.Millisecond
	}
	g.mu.Lock()
	next := g.last.Add(gap)
	if now := time.Now(); next.Before(now) {
		next = now
	}
	g.last = next
	g.mu.Unlock()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-time.After(time.Until(next)):
		return nil
	}
}

// errStatus carries a status code the caller maps to its own error.
type errStatus struct{ code int }

func (e errStatus) Error() string { return "answered " + strconv.Itoa(e.code) }

// do sends a request made by mk (called again on a retry) and returns the
// body of a 200 answer. A GET answer is cached by its URL unless ctx asks
// for fresh data. 429 is tried once more after Retry-After (at most 10 s).
func (g *getter) do(ctx context.Context, identity []string, mk func(ctx context.Context) (*http.Request, error)) ([]byte, error) {
	probe, err := mk(ctx)
	if err != nil {
		return nil, err
	}
	key := probe.URL.String()
	cacheable := probe.Method == http.MethodGet
	ttl := g.ttl
	if ttl <= 0 {
		ttl = 6 * time.Hour
	}
	if cacheable {
		g.mu.Lock()
		if c, ok := g.cache[key]; ok && time.Since(c.at) < ttl && !noCache(ctx) {
			g.mu.Unlock()
			return c.body, nil
		}
		g.mu.Unlock()
	}
	op, err := outbound.Begin(ctx, outbound.Target{Kind: "metadata_" + strings.ToLower(g.name), Identity: identity})
	if err != nil {
		return nil, err
	}
	outcome := outbound.Canceled
	defer func() { op.Finish(outcome) }()
	cl := g.http
	if cl == nil {
		cl = http.DefaultClient
	}
	timeout := g.timeout
	if timeout <= 0 {
		timeout = 10 * time.Second
	}
	for try := 0; ; try++ {
		if err := g.wait(ctx); err != nil {
			return nil, err
		}
		rctx, cancel := op.AttemptContext(ctx, timeout)
		req, err := mk(rctx)
		if err != nil {
			cancel()
			return nil, err
		}
		req.Header.Set("Accept", "application/json")
		if g.agent != "" {
			req.Header.Set("User-Agent", g.agent)
		}
		resp, err := cl.Do(req)
		if err != nil {
			outcome = outbound.HTTPOutcome(0, err, ctx)
			cancel()
			return nil, fmt.Errorf("%s: %w", g.name, stripURL(err))
		}
		body, err := io.ReadAll(io.LimitReader(resp.Body, 8<<20))
		if closeErr := resp.Body.Close(); err == nil {
			err = closeErr
		}
		cancel()
		if err != nil {
			outcome = outbound.HTTPOutcome(resp.StatusCode, err, ctx)
			return nil, fmt.Errorf("%s: %w", g.name, stripURL(err))
		}
		switch {
		case resp.StatusCode == http.StatusOK:
			outcome = outbound.Success
			if cacheable {
				g.mu.Lock()
				g.put(key, body, ttl, time.Now())
				g.mu.Unlock()
			}
			return body, nil
		case resp.StatusCode == http.StatusTooManyRequests && try == 0:
			d, _ := strconv.Atoi(resp.Header.Get("Retry-After"))
			select {
			case <-ctx.Done():
				return nil, ctx.Err()
			case <-time.After(time.Duration(min(max(d, 0), 10)) * time.Second):
			}
		case resp.StatusCode == http.StatusTooManyRequests:
			outcome = outbound.TransientFailure
			return nil, fmt.Errorf("%s: rate limited; try again shortly", g.name)
		default:
			outcome = outbound.HTTPOutcome(resp.StatusCode, nil, ctx)
			return nil, errStatus{resp.StatusCode}
		}
	}
}
