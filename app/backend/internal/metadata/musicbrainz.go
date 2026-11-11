package metadata

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/HolyArrs/holyarrs/app/backend/internal/core/outbound"
)

// Errors from music lookups.
var (
	// ErrArtistNotFound means MusicBrainz has no artist (or album) with that id.
	ErrArtistNotFound = errors.New("no such artist")
	// ErrMusicBusy means MusicBrainz kept answering "slow down" after the
	// back-off; the lookup can be tried again later.
	ErrMusicBusy = errors.New("MusicBrainz is busy; try again in a minute")
	// ErrListCut comes with a Pressings list that is not the whole list
	// (more pages than one lookup reads): use what came, remove nothing.
	ErrListCut = errors.New("MusicBrainz listed more than one lookup reads")
	// ErrMusicUnavailable means MusicBrainz (or the mirror) gave no usable
	// answer: an unexpected status, a refused or failed connection, or an
	// answer too large or not JSON. Its words carry no status code.
	ErrMusicUnavailable = errors.New("MusicBrainz could not answer; try again later")
)

// MusicStatusError is an unexpected status from MusicBrainz. Its words are
// ErrMusicUnavailable's; Code is kept for logs.
type MusicStatusError struct{ Code int }

func (e *MusicStatusError) Error() string { return ErrMusicUnavailable.Error() }

func (e *MusicStatusError) Is(target error) bool { return target == ErrMusicUnavailable }

// maxAnswer is the largest MusicBrainz answer read (16 MB).
const maxAnswer = 16 << 20

// DefaultMusicBrainzURL is used when no base URL is set.
const DefaultMusicBrainzURL = "https://musicbrainz.org"

// Artist is one looked-up artist. Albums are empty in search answers.
type Artist struct {
	MBID           string  `json:"mbid"`
	Name           string  `json:"name"`
	SortName       string  `json:"sort_name,omitempty"`
	Type           string  `json:"type,omitempty"` // person, group, orchestra, choir, character, other
	Disambiguation string  `json:"disambiguation,omitempty"`
	Country        string  `json:"country,omitempty"`
	Status         string  `json:"status"`          // running, ended
	Score          int     `json:"score,omitempty"` // search match, 0 to 100
	Albums         []Album `json:"albums,omitempty"`
	// Popularity is the metadata server's listening figures; nil when it
	// has none (the page then shows nothing).
	Popularity *ArtistPopularity `json:"popularity,omitempty"`
	// Partial says Albums is not the whole list (more pages than one
	// lookup reads): use what came, remove nothing.
	Partial bool `json:"-"`
}

// Album is one album (a MusicBrainz release group).
type Album struct {
	MBID           string   `json:"mbid"`
	Name           string   `json:"name"`
	PrimaryType    string   `json:"type"`                      // album, ep, single, broadcast, other, ""
	SecondaryTypes []string `json:"secondary_types,omitempty"` // live, compilation, ...
	ReleaseDate    string   `json:"release_date,omitempty"`    // YYYY, YYYY-MM or YYYY-MM-DD
	Disambiguation string   `json:"disambiguation,omitempty"`
}

// Track is one track of a Pressing.
type Track struct {
	ID          string `json:"id"`
	RecordingID string `json:"recording_id"`
	Disc        int    `json:"disc"`
	Number      int    `json:"number"`   // place on its disc
	Position    int    `json:"position"` // place on the whole Pressing
	Name        string `json:"name"`
	LengthMS    int    `json:"length_ms,omitempty"`
}

// Pressing is one edition of an album (a MusicBrainz release).
type Pressing struct {
	MBID           string   `json:"mbid"`
	Name           string   `json:"name"`
	Status         string   `json:"status"` // official, promotion, bootleg, pseudo-release, ""
	Country        string   `json:"country,omitempty"`
	Date           string   `json:"date,omitempty"`
	Disambiguation string   `json:"disambiguation,omitempty"`
	Formats        []string `json:"formats"` // one per disc
	Tracks         []Track  `json:"tracks"`
}

// ArtistProvider finds artists, their albums and the albums' Pressings.
type ArtistProvider interface {
	SearchArtists(ctx context.Context, query string) ([]Artist, error)
	// Artist returns the artist with every album (release group).
	Artist(ctx context.Context, mbid string) (Artist, error)
	// Pressings returns every Pressing of an album with its tracks; with
	// ErrListCut the list is only the first part.
	Pressings(ctx context.Context, albumMBID string) ([]Pressing, error)
}

// AlbumArtistFinder names the main artist of an album (release group),
// for an album link that came without its artist.
type AlbumArtistFinder interface {
	AlbumArtist(ctx context.Context, albumMBID string) (string, error)
}

// AlbumArtist returns the first credited artist's id of an album.
func (m *MusicBrainz) AlbumArtist(ctx context.Context, albumMBID string) (string, error) {
	if !ValidMBID(albumMBID) {
		return "", ErrArtistNotFound
	}
	b, err := m.get(ctx, "/ws/2/release-group/"+strings.ToLower(albumMBID), url.Values{"inc": {"artist-credits"}})
	if err != nil {
		return "", err
	}
	return firstCreditID(b)
}

func firstCreditID(b []byte) (string, error) {
	var rg struct {
		Credit []struct {
			Artist struct {
				ID string `json:"id"`
			} `json:"artist"`
		} `json:"artist-credit"`
	}
	if err := json.Unmarshal(b, &rg); err != nil {
		return "", fmt.Errorf("MusicBrainz: odd answer: %w", err)
	}
	if len(rg.Credit) == 0 || !ValidMBID(rg.Credit[0].Artist.ID) {
		return "", ErrArtistNotFound
	}
	return strings.ToLower(rg.Credit[0].Artist.ID), nil
}

var mbidRe = regexp.MustCompile(`^[0-9a-fA-F]{8}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{12}$`)

// ValidMBID reports whether id looks like a MusicBrainz id.
func ValidMBID(id string) bool { return mbidRe.MatchString(id) }

// Cache keeps provider answers with an expiry (the lookup_cache table).
type Cache interface {
	Get(ctx context.Context, key string, now time.Time) ([]byte, bool, error)
	Put(ctx context.Context, key string, body []byte, now, expires time.Time) error
}

// MusicBrainz looks artists up on MusicBrainz (or a mirror at BaseURL) at
// no more than one request a second, with a cache.
type MusicBrainz struct {
	// BaseURL returns the server to ask; "" means DefaultMusicBrainzURL.
	BaseURL func(ctx context.Context) string
	// UserAgent names HolyArrs, its version and a contact, as MusicBrainz asks.
	UserAgent string
	HTTP      *http.Client
	// Client picks the client per request (the local-network rule reads a
	// setting); it wins over HTTP. Either way redirects are followed only
	// on the same host.
	Client  func(ctx context.Context) *http.Client
	Timeout time.Duration
	// Limiter is shared by every caller; nil makes one of 1 request a second.
	Limiter *Limiter
	// Cache keeps answers until TTL passes; nil keeps none.
	Cache Cache
	TTL   func(ctx context.Context) time.Duration
	// Tries is how many times a request is sent while the server says to
	// slow down (503 or 429); default 4.
	Tries int
	// Allowed, when set, says MusicBrainz may be asked now (the metadata
	// mode); false answers ErrSourceOff without waiting for the limiter.
	Allowed func() bool

	once sync.Once
}

func (m *MusicBrainz) limiter() *Limiter {
	m.once.Do(func() {
		if m.Limiter == nil {
			m.Limiter = &Limiter{}
		}
	})
	return m.Limiter
}

func (m *MusicBrainz) base(ctx context.Context) string {
	b := ""
	if m.BaseURL != nil {
		b = strings.TrimSpace(m.BaseURL(ctx))
	}
	if b == "" {
		b = DefaultMusicBrainzURL
	}
	return strings.TrimRight(b, "/")
}

func (m *MusicBrainz) ttl(ctx context.Context) time.Duration {
	if m.TTL != nil {
		if d := m.TTL(ctx); d > 0 {
			return d
		}
	}
	return 24 * time.Hour
}

// UserAgentFor is the User-Agent HolyArrs sends to MusicBrainz.
func UserAgentFor(version string) string {
	if version == "" {
		version = "dev"
	}
	return "HolyArrs/" + version + " ( https://github.com/HolyArrs/HolyArrs )"
}

// sameHost follows a redirect only on the host first asked, at most 5 times.
func sameHost(req *http.Request, via []*http.Request) error {
	if len(via) >= 5 || len(via) == 0 || req.URL.Host != via[0].URL.Host {
		return http.ErrUseLastResponse
	}
	return nil
}

// get fetches path?q (fmt=json added) and returns the body of a 200 answer
// that is JSON. Answers are cached by server, path and query, so a changed
// base URL never reads another server's answers.
func (m *MusicBrainz) get(ctx context.Context, path string, q url.Values) ([]byte, error) {
	if q == nil {
		q = url.Values{}
	}
	q.Set("fmt", "json")
	base := m.base(ctx)
	host := base
	if bu, err := url.Parse(base); err == nil && bu.Host != "" {
		host = bu.Host + strings.TrimRight(bu.Path, "/")
	}
	key := "musicbrainz:" + host + path + "?" + q.Encode()
	l := m.limiter()
	if m.Cache != nil && !noCache(ctx) {
		if b, ok, err := m.Cache.Get(ctx, key, l.now()); err == nil && ok {
			return b, nil
		}
	}
	if m.Allowed != nil && !m.Allowed() {
		// Kept answers above still answer; nothing new is asked.
		return nil, ErrSourceOff
	}
	u := base + path + "?" + q.Encode()
	var cl http.Client
	switch {
	case m.Client != nil:
		cl = *m.Client(ctx)
	case m.HTTP != nil:
		cl = *m.HTTP
	}
	cl.CheckRedirect = sameHost
	timeout := m.Timeout
	if timeout <= 0 {
		timeout = 20 * time.Second
	}
	tries := m.Tries
	if tries <= 0 {
		tries = 4
	}
	ua := m.UserAgent
	if ua == "" {
		ua = UserAgentFor("")
	}
	op, err := outbound.Begin(ctx, outbound.Target{Kind: "metadata_musicbrainz", Identity: []string{base, ua}})
	if err != nil {
		return nil, err
	}
	outcome := outbound.Canceled
	defer func() { op.Finish(outcome) }()
	for try := 0; ; try++ {
		if err := l.Wait(ctx); err != nil {
			return nil, err
		}
		rctx, cancel := op.AttemptContext(ctx, timeout)
		req, err := http.NewRequestWithContext(rctx, http.MethodGet, u, nil)
		if err != nil {
			cancel()
			return nil, err
		}
		req.Header.Set("Accept", "application/json")
		req.Header.Set("User-Agent", ua)
		resp, err := cl.Do(req)
		if err != nil {
			outcome = outbound.HTTPOutcome(0, err, ctx)
			cancel()
			return nil, fmt.Errorf("%w (%w)", ErrMusicUnavailable, stripURL(err))
		}
		body, err := io.ReadAll(io.LimitReader(resp.Body, maxAnswer+1))
		if closeErr := resp.Body.Close(); err == nil {
			err = closeErr
		}
		cancel()
		if err != nil {
			outcome = outbound.HTTPOutcome(resp.StatusCode, err, ctx)
			return nil, fmt.Errorf("%w (%w)", ErrMusicUnavailable, stripURL(err))
		}
		switch resp.StatusCode {
		case http.StatusOK:
			if len(body) > maxAnswer {
				outcome = outbound.TerminalFailure
				return nil, fmt.Errorf("%w (an answer over 16 MB)", ErrMusicUnavailable)
			}
			if !json.Valid(body) {
				outcome = outbound.TerminalFailure
				return nil, fmt.Errorf("%w (an answer that is not JSON)", ErrMusicUnavailable)
			}
			outcome = outbound.Success
			if m.Cache != nil {
				now := l.now()
				_ = m.Cache.Put(ctx, key, body, now, now.Add(m.ttl(ctx)))
			}
			return body, nil
		case http.StatusNotFound:
			outcome = outbound.TerminalFailure
			return nil, ErrArtistNotFound
		case http.StatusServiceUnavailable, http.StatusTooManyRequests:
			// Back off: Retry-After when given (at most a minute), else 2, 4,
			// 8 seconds; every caller of the shared limiter waits too.
			wait := time.Duration(2<<try) * time.Second
			if s, err := strconv.Atoi(strings.TrimSpace(resp.Header.Get("Retry-After"))); err == nil && s >= 0 {
				wait = time.Duration(min(s, 60)) * time.Second
			}
			l.Hold(wait)
			if try+1 >= tries {
				outcome = outbound.TransientFailure
				return nil, ErrMusicBusy
			}
		default:
			outcome = outbound.HTTPOutcome(resp.StatusCode, nil, ctx)
			return nil, &MusicStatusError{Code: resp.StatusCode}
		}
	}
}

type mbArtist struct {
	ID             string `json:"id"`
	Name           string `json:"name"`
	SortName       string `json:"sort-name"`
	Type           string `json:"type"`
	Disambiguation string `json:"disambiguation"`
	Country        string `json:"country"`
	Score          int    `json:"score"`
	LifeSpan       struct {
		Ended bool `json:"ended"`
	} `json:"life-span"`
}

func (a mbArtist) artist() Artist {
	out := Artist{MBID: a.ID, Name: a.Name, SortName: a.SortName, Type: strings.ToLower(a.Type), Disambiguation: a.Disambiguation,
		Country: a.Country, Score: a.Score, Status: "running"}
	if a.LifeSpan.Ended {
		out.Status = "ended"
	}
	return out
}

// luceneSpecial are the characters the MusicBrainz search syntax gives a meaning.
var luceneSpecial = regexp.MustCompile(`([+\-&|!(){}\[\]^"~*?:\\/])`)

// SearchArtists finds artists by name.
func (m *MusicBrainz) SearchArtists(ctx context.Context, query string) ([]Artist, error) {
	q := strings.TrimSpace(query)
	if q == "" {
		return []Artist{}, nil
	}
	b, err := m.get(ctx, "/ws/2/artist", url.Values{"query": {luceneSpecial.ReplaceAllString(q, `\$1`)}, "limit": {"25"}})
	if errors.Is(err, ErrArtistNotFound) {
		return []Artist{}, nil
	}
	if err != nil {
		return nil, err
	}
	var res struct {
		Artists []mbArtist `json:"artists"`
	}
	if err := json.Unmarshal(b, &res); err != nil {
		return nil, fmt.Errorf("MusicBrainz: odd answer: %w", err)
	}
	out := make([]Artist, 0, len(res.Artists))
	for _, a := range res.Artists {
		if ValidMBID(a.ID) {
			out = append(out, a.artist())
		}
	}
	return out, nil
}

// pageSize is how many rows each browse request asks for; maxPages caps
// how many pages one lookup reads (MusicBrainz allows 100 a page).
const (
	pageSize = 100
	maxPages = 20
)

// Artist returns one artist with all its albums.
func (m *MusicBrainz) Artist(ctx context.Context, mbid string) (Artist, error) {
	if !ValidMBID(mbid) {
		return Artist{}, ErrArtistNotFound
	}
	mbid = strings.ToLower(mbid)
	b, err := m.get(ctx, "/ws/2/artist/"+mbid, nil)
	if err != nil {
		return Artist{}, err
	}
	var a mbArtist
	if err := json.Unmarshal(b, &a); err != nil || a.ID == "" {
		return Artist{}, fmt.Errorf("MusicBrainz: odd answer")
	}
	out := a.artist()
	out.Albums = []Album{}
	offset := 0
	for page := 0; page < maxPages; page++ {
		b, err := m.get(ctx, "/ws/2/release-group", url.Values{"artist": {mbid}, "limit": {strconv.Itoa(pageSize)}, "offset": {strconv.Itoa(offset)}})
		if err != nil {
			return Artist{}, err
		}
		var res struct {
			Count  int `json:"release-group-count"`
			Groups []struct {
				ID             string   `json:"id"`
				Title          string   `json:"title"`
				PrimaryType    string   `json:"primary-type"`
				SecondaryTypes []string `json:"secondary-types"`
				FirstRelease   string   `json:"first-release-date"`
				Disambiguation string   `json:"disambiguation"`
			} `json:"release-groups"`
		}
		if err := json.Unmarshal(b, &res); err != nil {
			return Artist{}, fmt.Errorf("MusicBrainz: odd answer: %w", err)
		}
		for _, g := range res.Groups {
			if !ValidMBID(g.ID) {
				continue
			}
			al := Album{MBID: strings.ToLower(g.ID), Name: g.Title, PrimaryType: strings.ToLower(g.PrimaryType), ReleaseDate: g.FirstRelease,
				Disambiguation: g.Disambiguation, SecondaryTypes: []string{}}
			for _, s := range g.SecondaryTypes {
				al.SecondaryTypes = append(al.SecondaryTypes, strings.ToLower(s))
			}
			out.Albums = append(out.Albums, al)
		}
		offset += len(res.Groups)
		if len(res.Groups) == 0 || offset >= res.Count {
			return out, nil
		}
	}
	out.Partial = true
	return out, nil
}

// Pressings returns every Pressing of an album with its discs and tracks.
func (m *MusicBrainz) Pressings(ctx context.Context, albumMBID string) ([]Pressing, error) {
	if !ValidMBID(albumMBID) {
		return nil, ErrArtistNotFound
	}
	albumMBID = strings.ToLower(albumMBID)
	out := []Pressing{}
	offset := 0
	for page := 0; page < maxPages; page++ {
		b, err := m.get(ctx, "/ws/2/release", url.Values{"release-group": {albumMBID}, "inc": {"media recordings"},
			"limit": {strconv.Itoa(pageSize)}, "offset": {strconv.Itoa(offset)}})
		if err != nil {
			return nil, err
		}
		var res struct {
			Count    int `json:"release-count"`
			Releases []struct {
				ID             string `json:"id"`
				Title          string `json:"title"`
				Status         string `json:"status"`
				Country        string `json:"country"`
				Date           string `json:"date"`
				Disambiguation string `json:"disambiguation"`
				Media          []struct {
					Position int    `json:"position"`
					Format   string `json:"format"`
					Tracks   []struct {
						ID        string `json:"id"`
						Position  int    `json:"position"`
						Title     string `json:"title"`
						Length    *int   `json:"length"`
						Recording struct {
							ID     string `json:"id"`
							Length *int   `json:"length"`
						} `json:"recording"`
					} `json:"tracks"`
				} `json:"media"`
			} `json:"releases"`
		}
		if err := json.Unmarshal(b, &res); err != nil {
			return nil, fmt.Errorf("MusicBrainz: odd answer: %w", err)
		}
		for _, r := range res.Releases {
			if !ValidMBID(r.ID) {
				continue
			}
			p := Pressing{MBID: strings.ToLower(r.ID), Name: r.Title, Status: strings.ToLower(r.Status), Country: r.Country, Date: r.Date,
				Disambiguation: r.Disambiguation, Formats: []string{}, Tracks: []Track{}}
			pos := 0
			for i, md := range r.Media {
				disc := md.Position
				if disc <= 0 {
					disc = i + 1
				}
				p.Formats = append(p.Formats, md.Format)
				for j, tr := range md.Tracks {
					pos++
					n := tr.Position
					if n <= 0 {
						n = j + 1
					}
					t := Track{ID: strings.ToLower(tr.ID), RecordingID: strings.ToLower(tr.Recording.ID), Disc: disc, Number: n, Position: pos, Name: tr.Title}
					if tr.Length != nil {
						t.LengthMS = *tr.Length
					} else if tr.Recording.Length != nil {
						t.LengthMS = *tr.Recording.Length
					}
					p.Tracks = append(p.Tracks, t)
				}
			}
			out = append(out, p)
		}
		offset += len(res.Releases)
		if len(res.Releases) == 0 || offset >= res.Count {
			return out, nil
		}
	}
	return out, ErrListCut
}

// ArtistLink is an artist another artist worked with: a member, a band
// they were in, or a collaborator.
type ArtistLink struct {
	MBID string `json:"mbid"`
	Name string `json:"name"`
	Type string `json:"type"` // member of band, collaboration, ...
}

// ArtistGenres returns an artist's MusicBrainz genres, most voted first, at
// most five (lower case, as MusicBrainz names them).
func (m *MusicBrainz) ArtistGenres(ctx context.Context, mbid string) ([]string, error) {
	g, _, err := m.ArtistGenresLinks(ctx, mbid)
	return g, err
}

// ArtistGenresLinks returns an artist's genres and the artists it worked
// with (members, bands, collaborations), in one lookup.
func (m *MusicBrainz) ArtistGenresLinks(ctx context.Context, mbid string) ([]string, []ArtistLink, error) {
	if !ValidMBID(mbid) {
		return nil, nil, ErrArtistNotFound
	}
	b, err := m.get(ctx, "/ws/2/artist/"+strings.ToLower(mbid), url.Values{"inc": {"genres artist-rels"}})
	if err != nil {
		return nil, nil, err
	}
	var a struct {
		Genres []struct {
			Name  string `json:"name"`
			Count int    `json:"count"`
		} `json:"genres"`
		Relations []struct {
			Type   string `json:"type"`
			Artist struct {
				ID   string `json:"id"`
				Name string `json:"name"`
			} `json:"artist"`
		} `json:"relations"`
	}
	if err := json.Unmarshal(b, &a); err != nil {
		return nil, nil, fmt.Errorf("MusicBrainz: odd answer")
	}
	sort.SliceStable(a.Genres, func(i, j int) bool { return a.Genres[i].Count > a.Genres[j].Count })
	out := []string{}
	for _, g := range a.Genres {
		if n := strings.ToLower(strings.TrimSpace(g.Name)); n != "" && g.Count > 0 && len(out) < 5 {
			out = append(out, n)
		}
	}
	links := []ArtistLink{}
	seen := map[string]bool{strings.ToLower(mbid): true}
	for _, r := range a.Relations {
		id := strings.ToLower(r.Artist.ID)
		switch r.Type {
		case "member of band", "collaboration", "is person", "supporting musician", "subgroup", "founder", "artistic director":
		default:
			continue
		}
		if ValidMBID(id) && r.Artist.Name != "" && !seen[id] {
			seen[id] = true
			links = append(links, ArtistLink{MBID: id, Name: r.Artist.Name, Type: r.Type})
		}
	}
	return out, links, nil
}

// SoundtrackFor finds soundtrack albums (release groups) named like a
// movie or show; year 0 is any year.
func (m *MusicBrainz) SoundtrackFor(ctx context.Context, title string, year int) ([]Album, error) {
	q := strings.TrimSpace(title)
	if q == "" {
		return []Album{}, nil
	}
	query := `releasegroup:"` + luceneSpecial.ReplaceAllString(q, `\$1`) + `" AND secondarytype:soundtrack`
	b, err := m.get(ctx, "/ws/2/release-group", url.Values{"query": {query}, "limit": {"5"}})
	if errors.Is(err, ErrArtistNotFound) {
		return []Album{}, nil
	}
	if err != nil {
		return nil, err
	}
	var res struct {
		Groups []struct {
			ID    string `json:"id"`
			Title string `json:"title"`
			Score int    `json:"score"`
			First string `json:"first-release-date"`
		} `json:"release-groups"`
	}
	if err := json.Unmarshal(b, &res); err != nil {
		return nil, fmt.Errorf("MusicBrainz: odd answer")
	}
	out := []Album{}
	low := strings.ToLower(q)
	for _, g := range res.Groups {
		if !ValidMBID(g.ID) || g.Score < 80 || !strings.Contains(strings.ToLower(g.Title), low) {
			continue
		}
		if year > 0 && len(g.First) >= 4 {
			if y, err := strconv.Atoi(g.First[:4]); err != nil || y < year-1 || y > year+1 {
				continue
			}
		}
		out = append(out, Album{MBID: strings.ToLower(g.ID), Name: g.Title, PrimaryType: "album", SecondaryTypes: []string{"soundtrack"}, ReleaseDate: g.First})
	}
	return out, nil
}
