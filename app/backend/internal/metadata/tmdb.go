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

	"github.com/HolyArrs/holyarrs/app/backend/internal/core/outbound"
	"github.com/HolyArrs/holyarrs/app/backend/internal/core/startup"
)

// ErrNoAccess means TMDB or the metadata server refused the credential, or none is set.
var ErrNoAccess = errors.New("the metadata server refused this install; searches and new artwork are unavailable")

// ImageBase is where TMDB serves pictures.
const ImageBase = "https://image.tmdb.org/t/p"

// Attribution is the notice TMDB asks apps that use its API to show.
const Attribution = "This product uses the TMDB API but is not endorsed or certified by TMDB."

// EnvTokenFile names a file holding the TMDB read access token (for the lab).
const EnvTokenFile = "HOLYARRS_TMDB_TOKEN_FILE"

// TMDB looks movies up on The Movie Database with a v4 read access token,
// sent as a Bearer header and never in a URL.
//
// Answers are kept in memory for TTL. With Disk set, every answer but a
// search is kept there too for tmdbKeep, so a restart reads it back: an
// answer older than TTL is served at once and asked again in the
// background, and a 404 is remembered for tmdbMissTTL. A failed or held
// request serves the old copy when there is one.
//
// A fetch made ahead of a person (Ahead) carries X-Fetch: ahead, is spaced
// at least AheadGap apart, is not sent while the server asked us to wait,
// and has its own breaker, so warm-ups never hold back a person's page.
type TMDB struct {
	BaseURL string                                    // default https://api.themoviedb.org/3
	Access  func(ctx context.Context) (string, error) // the read access token, read on each request; never logged
	HTTP    *http.Client
	Timeout time.Duration // per request, default 10s
	Gap     time.Duration // least time between requests, default 50ms
	// AheadGap is the least time between fetch-ahead requests, default
	// 250ms (the server's per-install budget leaves room for pages).
	AheadGap time.Duration
	TTL      time.Duration // cache time, default 6h
	// Images is where pictures are served (ImageBase when empty).
	Images string
	// Disk keeps answers across restarts (lookup_cache); nil keeps memory only.
	Disk Cache

	mu         sync.Mutex
	last       time.Time
	lastAhead  time.Time
	cache      map[string]cached
	refreshing map[string]bool
	hold       serviceHold
}

type cached struct {
	body []byte
	at   time.Time
	miss bool // a 404
}

const maxCache = 1000

const (
	// tmdbKeep is how long an answer on disk may be served stale.
	tmdbKeep = 7 * 24 * time.Hour
	// tmdbMissTTL is how long a 404 is remembered.
	tmdbMissTTL = time.Hour
	// tmdbRefreshWait bounds one background refresh.
	tmdbRefreshWait = 30 * time.Second
)

// tmdbDisk is one answer as kept in lookup_cache.
type tmdbDisk struct {
	At   time.Time       `json:"at"`
	Miss bool            `json:"miss,omitempty"`
	Body json.RawMessage `json:"body,omitempty"`
}

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

// wait spaces requests by Gap, and fetch-ahead requests by AheadGap too.
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
	if IsAhead(ctx) {
		ag := t.AheadGap
		if ag <= 0 {
			ag = 250 * time.Millisecond
		}
		if a := t.lastAhead.Add(max(ag, gap)); a.After(next) {
			next = a
		}
		t.lastAhead = next
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

// kept reads u's answer from memory, else from Disk when disk is set.
func (t *TMDB) kept(ctx context.Context, u string, disk bool) (cached, bool) {
	t.mu.Lock()
	c, ok := t.cache[u]
	t.mu.Unlock()
	if ok || !disk {
		return c, ok
	}
	b, ok, err := t.Disk.Get(ctx, "tmdb:"+u, time.Now())
	if err != nil || !ok {
		return cached{}, false
	}
	var d tmdbDisk
	if json.Unmarshal(b, &d) != nil || d.At.IsZero() || !d.Miss && len(d.Body) == 0 {
		return cached{}, false
	}
	c = cached{body: d.Body, at: d.At, miss: d.Miss}
	t.remember(u, c)
	return c, true
}

func (t *TMDB) remember(u string, c cached) {
	t.mu.Lock()
	// Low-memory mode keeps fewer answers here: the Disk copy answers the rest.
	if t.cache == nil || len(t.cache) >= startup.LowMemoryPick(maxCache, 150) {
		t.cache = map[string]cached{}
	}
	t.cache[u] = c
	t.mu.Unlock()
}

// keep stores a fetched answer (or a 404) in memory and on Disk.
func (t *TMDB) keep(ctx context.Context, u string, c cached, disk bool) {
	t.remember(u, c)
	if !disk {
		return
	}
	b, err := json.Marshal(tmdbDisk{At: c.at, Miss: c.miss, Body: c.body})
	if err != nil {
		return
	}
	until := c.at.Add(tmdbKeep)
	if c.miss {
		until = c.at.Add(tmdbMissTTL)
	}
	_ = t.Disk.Put(context.WithoutCancel(ctx), "tmdb:"+u, b, c.at, until)
}

// refresh asks for u again in the background (once at a time), as a
// fetch-ahead, and keeps the answer.
func (t *TMDB) refresh(u string, disk bool) {
	t.mu.Lock()
	if t.refreshing[u] {
		t.mu.Unlock()
		return
	}
	if t.refreshing == nil {
		t.refreshing = map[string]bool{}
	}
	t.refreshing[u] = true
	t.mu.Unlock()
	go func() {
		defer func() {
			t.mu.Lock()
			delete(t.refreshing, u)
			t.mu.Unlock()
		}()
		ctx, cancel := context.WithTimeout(Ahead(context.Background()), tmdbRefreshWait)
		defer cancel()
		body, err := t.fetch(ctx, u)
		switch {
		case err == nil:
			t.keep(ctx, u, cached{body: body, at: time.Now()}, disk)
		case errors.Is(err, ErrNotFound):
			t.keep(ctx, u, cached{at: time.Now(), miss: true}, disk)
		}
	}()
}

// tmdbStaleOK says whether err may be answered from an old copy: the
// service failed, was held or is rate limited (not a refusal or a 404).
func tmdbStaleOK(ctx context.Context, err error) bool {
	return err != nil && ctx.Err() == nil && !errors.Is(err, ErrNotFound) && !errors.Is(err, ErrNoAccess)
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
	// Searches hold the person's words: they stay in memory only.
	disk := t.Disk != nil && !strings.HasPrefix(path, "/search")
	var stale *cached
	if !noCache(ctx) {
		if c, ok := t.kept(ctx, u, disk); ok {
			switch {
			case c.miss && time.Since(c.at) < tmdbMissTTL:
				return nil, ErrNotFound
			case c.miss:
			case time.Since(c.at) < ttl:
				return c.body, nil
			default:
				stale = &c
			}
		}
	}
	if stale != nil && disk {
		t.refresh(u, disk)
		return stale.body, nil
	}
	body, err := t.fetch(ctx, u)
	switch {
	case err == nil:
		t.keep(ctx, u, cached{body: body, at: time.Now()}, disk)
	case errors.Is(err, ErrNotFound):
		t.keep(ctx, u, cached{at: time.Now(), miss: true}, disk)
	case stale != nil && tmdbStaleOK(ctx, err):
		return stale.body, nil
	}
	return body, err
}

// fetch makes one request (one retry after a 429).
func (t *TMDB) fetch(ctx context.Context, u string) ([]byte, error) {
	ahead := IsAhead(ctx)
	if ahead && t.hold.held(time.Now()) {
		return nil, ErrRateLimited
	}
	tok, err := t.Access(ctx)
	if err != nil {
		return nil, err
	}
	if tok == "" {
		return nil, ErrNoAccess
	}
	// Fetch-ahead and a person's pages fail apart: a warm-up that keeps
	// failing never opens the breaker a page call goes through.
	lane := "page"
	if ahead {
		lane = "ahead"
	}
	op, err := outbound.Begin(ctx, outbound.Target{Kind: "metadata_tmdb", Identity: []string{t.base(), tok, lane}})
	if err != nil {
		if errors.Is(err, outbound.ErrSuppressed) {
			return nil, fmt.Errorf("TMDB: %w (%w)", ErrServiceUnavailable, err)
		}
		return nil, err
	}
	outcome := outbound.Canceled
	defer func() { op.Finish(outcome) }()
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
		rctx, cancel := op.AttemptContext(ctx, timeout)
		req, _ := http.NewRequestWithContext(rctx, http.MethodGet, u, nil)
		req.Header.Set("Authorization", "Bearer "+tok)
		req.Header.Set("Accept", "application/json")
		aheadRequest(ctx, req)
		resp, err := cl.Do(req)
		if err != nil {
			outcome = outbound.HTTPOutcome(0, err, ctx)
			cancel()
			return nil, fmt.Errorf("TMDB: %w (%w)", ErrServiceUnavailable, stripURL(err))
		}
		body, err := io.ReadAll(io.LimitReader(resp.Body, 4<<20))
		if closeErr := resp.Body.Close(); err == nil {
			err = closeErr
		}
		cancel()
		if err != nil {
			outcome = outbound.HTTPOutcome(resp.StatusCode, err, ctx)
			return nil, fmt.Errorf("TMDB: %w", stripURL(err))
		}
		switch {
		case resp.StatusCode == http.StatusOK:
			outcome = outbound.Success
			return body, nil
		case resp.StatusCode == http.StatusNotFound:
			outcome = outbound.TerminalFailure
			return nil, ErrNotFound
		case resp.StatusCode == http.StatusUnauthorized || resp.StatusCode == http.StatusForbidden:
			outcome = outbound.TerminalFailure
			return nil, ErrNoAccess
		case resp.StatusCode == http.StatusTooManyRequests && try == 0 && !ahead:
			d, _ := strconv.Atoi(resp.Header.Get("Retry-After"))
			select {
			case <-ctx.Done():
				return nil, ctx.Err()
			case <-time.After(time.Duration(min(max(d, 0), 10)) * time.Second):
			}
		case resp.StatusCode == http.StatusTooManyRequests:
			// Fetch-ahead does not wait inside the call: it holds back instead.
			t.hold.set(resp.Header.Get("Retry-After"), body, time.Now())
			outcome = outbound.TransientFailure
			return nil, ErrRateLimited
		default:
			outcome = outbound.HTTPOutcome(resp.StatusCode, nil, ctx)
			if resp.StatusCode == http.StatusServiceUnavailable {
				t.hold.set(resp.Header.Get("Retry-After"), body, time.Now())
			}
			if resp.StatusCode >= 500 {
				return nil, fmt.Errorf("TMDB answered %d: %w", resp.StatusCode, ErrServiceUnavailable)
			}
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
	ID               int64  `json:"id"`
	Title            string `json:"title"`
	OriginalTitle    string `json:"original_title"`
	ReleaseDate      string `json:"release_date"`
	Overview         string `json:"overview"`
	OriginalLanguage string `json:"original_language"`
	Runtime          int    `json:"runtime"`
	PosterPath       string `json:"poster_path"`
	tmdbExtras
	IMDbID              string `json:"imdb_id"`
	BelongsToCollection *struct {
		ID   int64  `json:"id"`
		Name string `json:"name"`
	} `json:"belongs_to_collection"`
	ProductionCompanies []struct {
		Name string `json:"name"`
	} `json:"production_companies"`
	AltTitles *struct {
		Titles []struct {
			Title string `json:"title"`
		} `json:"titles"`
	} `json:"alternative_titles,omitempty"`
	Translations *struct {
		List []struct {
			Data struct {
				Title string `json:"title"`
			} `json:"data"`
		} `json:"translations"`
	} `json:"translations,omitempty"`
	ReleaseDates *struct {
		Results []struct {
			Country string `json:"iso_3166_1"`
			Dates   []struct {
				Date          string `json:"release_date"`
				Type          int    `json:"type"`
				Certification string `json:"certification"`
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

// certification is the US certification, preferring the cinema release.
func (m tmdbMovie) certification() string {
	if m.ReleaseDates == nil {
		return ""
	}
	best, bestType := "", 0
	for _, c := range m.ReleaseDates.Results {
		if c.Country != "US" {
			continue
		}
		for _, d := range c.Dates {
			if d.Certification == "" {
				continue
			}
			if best == "" || d.Type == relCinema || (bestType != relCinema && d.Type < bestType) {
				best, bestType = d.Certification, d.Type
			}
		}
	}
	return best
}

// tmdbExtras are the detail fields movies and shows share: wide picture,
// genres, logos and credits (the last two only with append_to_response).
type tmdbExtras struct {
	BackdropPath string `json:"backdrop_path"`
	Genres       []struct {
		Name string `json:"name"`
	} `json:"genres"`
	Images *struct {
		Logos []struct {
			FilePath string  `json:"file_path"`
			Language *string `json:"iso_639_1"`
		} `json:"logos"`
	} `json:"images"`
	Credits *struct {
		Cast []struct {
			Name string `json:"name"`
		} `json:"cast"`
		Crew []struct {
			Name string `json:"name"`
			Job  string `json:"job"`
		} `json:"crew"`
	} `json:"credits"`
	VoteAverage float64 `json:"vote_average"`
	VoteCount   int     `json:"vote_count"`
}

// score is the members' average to one decimal and its vote count.
func (x tmdbExtras) score() (float64, int) {
	return float64(int(x.VoteAverage*10+0.5)) / 10, x.VoteCount
}

// maxCast is how many cast names a Title keeps.
const maxCast = 10

// pictures returns the backdrop (w1280), the logo (w500 PNG: English
// first, then one with no language), the genre names and the people (top
// cast, then directors not already listed).
func (x tmdbExtras) pictures(images string) (backdrop, logo string, genres, people []string) {
	if x.BackdropPath != "" {
		backdrop = images + "/w1280" + x.BackdropPath
	}
	if x.Images != nil {
		var none string
		for _, l := range x.Images.Logos {
			if !strings.HasSuffix(strings.ToLower(l.FilePath), ".png") {
				continue
			}
			if l.Language != nil && *l.Language == "en" {
				logo = images + "/w500" + l.FilePath
				break
			}
			if l.Language == nil && none == "" {
				none = images + "/w500" + l.FilePath
			}
		}
		if logo == "" {
			logo = none
		}
	}
	for _, g := range x.Genres {
		if g.Name != "" {
			genres = append(genres, g.Name)
		}
	}
	if x.Credits != nil {
		seen := map[string]bool{}
		for _, c := range x.Credits.Cast {
			if len(people) == maxCast {
				break
			}
			if c.Name != "" && !seen[c.Name] {
				seen[c.Name] = true
				people = append(people, c.Name)
			}
		}
		for _, c := range x.Credits.Crew {
			if c.Job == "Director" && c.Name != "" && !seen[c.Name] {
				seen[c.Name] = true
				people = append(people, c.Name)
			}
		}
	}
	return backdrop, logo, genres, people
}

// directors returns the directors' names in credit order (empty without credits).
func (x tmdbExtras) directors() []string {
	var out []string
	if x.Credits == nil {
		return out
	}
	seen := map[string]bool{}
	for _, c := range x.Credits.Crew {
		if c.Job == "Director" && c.Name != "" && !seen[c.Name] {
			seen[c.Name] = true
			out = append(out, c.Name)
		}
	}
	return out
}

// parseDay reads a YYYY-MM-DD date or a full time, as UTC midnight of that day.
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
	out.Backdrop, out.Logo, out.Genres, out.People = m.tmdbExtras.pictures(images)
	out.Directors = m.tmdbExtras.directors()
	out.Score, out.Votes = m.tmdbExtras.score()
	for _, c := range m.ProductionCompanies {
		if c.Name != "" {
			out.Studio = c.Name
			break
		}
	}
	out.Rating = m.certification()
	out.OtherNames = append(out.OtherNames, m.OriginalTitle)
	if m.AltTitles != nil {
		for _, a := range m.AltTitles.Titles {
			out.OtherNames = append(out.OtherNames, a.Title)
		}
	}
	if m.Translations != nil {
		for _, tr := range m.Translations.List {
			out.OtherNames = append(out.OtherNames, tr.Data.Title)
		}
	}
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
		b, err := t.get(ctx, "/movie/"+strconv.FormatInt(n, 10), url.Values{"append_to_response": {"release_dates,images,credits,translations,alternative_titles"}, "include_image_language": {"en,null"}})
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
