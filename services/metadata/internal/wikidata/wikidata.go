// Package wikidata answers awards, people facts, cross-walk IDs and the
// Wikipedia summary for a title, person or artist. Wikidata (CC0) is read
// with the action API (haswbstatement search, then wbgetentities, labels
// batched 50 at a time; an artist is found by its MusicBrainz link first,
// see artist.go); the summary comes from Wikipedia's REST API
// (/page/summary) and carries CC BY-SA 4.0 attribution. Answers are kept
// in their own SQLite file (14 days when found, a day when not). A weekly
// job asks the query service once for every film and series that won a
// major award (Academy Awards, Golden Globes, Emmys, BAFTA) and keeps the
// TMDB IDs for the "Award winners" collection.
package wikidata

import (
	"context"
	"crypto/md5"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"regexp"
	"slices"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"
)

// Default endpoints.
const (
	APIURL       = "https://www.wikidata.org/w/api.php"
	SPARQLURL    = "https://query.wikidata.org/sparql"
	WikipediaURL = "https://en.wikipedia.org/api/rest_v1"
	License      = "CC BY-SA 4.0"
	LicenseURL   = "https://creativecommons.org/licenses/by-sa/4.0/"
)

// ErrNotFound: Wikidata has no item for the asked ID.
var ErrNotFound = errors.New("wikidata: not found")

// Options configure the service.
type Options struct {
	// HTTP is used for every call (budgeted by the caller; default 15 s).
	HTTP                            *http.Client
	UserAgent                       string
	APIURL, SPARQLURL, WikipediaURL string
	// TVmazeURL is TVmaze's person search (tests point it at a fake).
	TVmazeURL string
	// Fresh is how long a found answer is kept (default 14 days, 7–30);
	// Miss how long a "not found" is kept (default a day).
	Fresh, Miss time.Duration
	// ArtistFresh is how long an artist's answer (its Wikipedia bio) is
	// kept (default 30 days; bios change slowly).
	ArtistFresh time.Duration
	// Every is how often the award-winner list is rebuilt (default 7 days).
	Every time.Duration
	Now   func() time.Time
	// ArtistMB reads an artist's stored MusicBrainz answer (no call); false
	// when none is stored. nil: artists go by Wikidata's own MusicBrainz
	// statement only.
	ArtistMB func(ctx context.Context, mbid string) (MBArtist, bool)
}

// Award is one award received or nomination.
type Award struct {
	QID     string `json:"qid"`
	Name    string `json:"name"`
	Family  string `json:"family,omitempty"`
	Won     bool   `json:"won"`
	Year    int    `json:"year,omitempty"`
	ForWork string `json:"for_work,omitempty"`
}

// Tally counts wins and nominations of one award family (oscar, emmy,
// golden_globe, bafta, grammy).
type Tally struct {
	Family      string `json:"family"`
	Wins        int    `json:"wins"`
	Nominations int    `json:"nominations"`
}

// IDs are the cross-walk IDs present on the item.
type IDs struct {
	IMDb        string `json:"imdb,omitempty"`
	TVDB        string `json:"tvdb,omitempty"`
	MusicBrainz string `json:"musicbrainz,omitempty"`
	Discogs     string `json:"discogs,omitempty"`
	AniList     string `json:"anilist,omitempty"`
}

// Summary is Wikipedia's short summary with its attribution.
type Summary struct {
	Title       string `json:"title"`
	Extract     string `json:"extract"`
	URL         string `json:"url"`
	License     string `json:"license"`
	LicenseURL  string `json:"license_url"`
	Attribution string `json:"attribution"`
}

// Entity is the answer for a title, person or artist.
type Entity struct {
	QID        string   `json:"qid"`
	Kind       string   `json:"kind"`
	ID         string   `json:"id"`
	IDs        IDs      `json:"ids"`
	Awards     []Award  `json:"awards"`
	Tallies    []Tally  `json:"tallies"`
	Born       string   `json:"born,omitempty"`
	Died       string   `json:"died,omitempty"`
	BirthPlace string   `json:"birth_place,omitempty"`
	DeathPlace string   `json:"death_place,omitempty"`
	Wikipedia  string   `json:"wikipedia,omitempty"`
	Summary    *Summary `json:"summary,omitempty"`
	// Image is a person's Wikimedia Commons photo (P18) as a 330 px thumbnail (one of the widths Commons serves); the app uses it only when TMDB has none.
	Image string `json:"image,omitempty"`
	// V is the person or artist answer's shape (shapeV); older answers are
	// fetched again when asked.
	V int `json:"v,omitempty"`
	// Match is how an artist's item was found: "musicbrainz" (MusicBrainz's
	// own Wikidata link), "statement" (the item's MusicBrainz id) or "name"
	// (a name search that passed the checks in artist.go).
	Match string `json:"match,omitempty"`
	// Country, Begin and Genres are an artist's facts from Wikidata alone
	// (P27/P495 as an ISO code, P569/P571, P136 labels); Facts is what the
	// artist route serves: MusicBrainz first, these only for its gaps.
	Country   string       `json:"wd_country,omitempty"`
	Begin     string       `json:"wd_begin,omitempty"`
	Genres    []string     `json:"wd_genres,omitempty"`
	Facts     *ArtistFacts `json:"facts,omitempty"`
	FetchedAt string       `json:"fetched_at"`
}

// Winner is one award-winning title.
type Winner struct {
	TMDB int64 `json:"tmdb"`
	Wins int   `json:"wins"`
}

// Stats are the Wikidata numbers in /v1/stats.
type Stats struct {
	Lookups     int64  `json:"lookups"`
	Failures    int64  `json:"failures"`
	WinnersAt   string `json:"winners_at,omitempty"`
	Movies      int    `json:"winner_movies"`
	Shows       int    `json:"winner_shows"`
	WinnerError string `json:"winner_error,omitempty"`
}

// Service keeps answers in SQLite and the winner lists in memory.
type Service struct {
	db   *sql.DB
	opts Options

	mu        sync.Mutex
	winners   map[string][]Winner
	winnersAt time.Time
	lookups   int64
	failures  int64
	lastErr   string
}

// Kinds the routes take, with their Wikidata property.
var props = map[string]string{"movie": "P4947", "tv": "P4983", "person": "P4985", "artist": "P434"}

// ValidKind reports a title kind (movie or tv).
func ValidKind(k string) bool { return k == "movie" || k == "tv" }

var mbid = regexp.MustCompile(`^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$`)

// ValidMBID reports a MusicBrainz ID.
func ValidMBID(s string) bool { return mbid.MatchString(s) }

func Open(path string, o Options) (*Service, error) {
	if o.HTTP == nil {
		o.HTTP = &http.Client{Timeout: 15 * time.Second}
	}
	if o.UserAgent == "" {
		o.UserAgent = "HolyArrsMetadata/0.2 (+https://github.com/CoderLuii/HolyArrs)"
	}
	if o.APIURL == "" {
		o.APIURL = APIURL
	}
	if o.SPARQLURL == "" {
		o.SPARQLURL = SPARQLURL
	}
	if o.WikipediaURL == "" {
		o.WikipediaURL = WikipediaURL
	}
	if o.Fresh < 7*24*time.Hour || o.Fresh > 30*24*time.Hour {
		o.Fresh = 14 * 24 * time.Hour
	}
	if o.Miss <= 0 {
		o.Miss = 24 * time.Hour
	}
	if o.ArtistFresh <= 0 {
		o.ArtistFresh = 30 * 24 * time.Hour
	}
	if o.Every <= 0 {
		o.Every = 7 * 24 * time.Hour
	}
	if o.Now == nil {
		o.Now = time.Now
	}
	db, err := sql.Open("sqlite", path)
	if err != nil {
		return nil, err
	}
	db.SetMaxOpenConns(1)
	if _, err = db.Exec(`PRAGMA journal_mode=WAL; PRAGMA busy_timeout=5000;
CREATE TABLE IF NOT EXISTS answers (k TEXT PRIMARY KEY, body BLOB, fetched_at INTEGER NOT NULL) WITHOUT ROWID;
CREATE TABLE IF NOT EXISTS meta (k TEXT PRIMARY KEY, v BLOB NOT NULL, at INTEGER NOT NULL) WITHOUT ROWID;`); err != nil {
		db.Close()
		return nil, err
	}
	s := &Service{db: db, opts: o, winners: map[string][]Winner{}}
	var body []byte
	var at int64
	if db.QueryRow(`SELECT v, at FROM meta WHERE k='winners'`).Scan(&body, &at) == nil {
		var w map[string][]Winner
		if json.Unmarshal(body, &w) == nil {
			s.winners, s.winnersAt = w, time.Unix(at, 0)
		}
	}
	return s, nil
}

func (s *Service) Close() error { return s.db.Close() }

// Title answers a movie or tv title by TMDB ID.
func (s *Service) Title(ctx context.Context, kind string, tmdb int64) (Entity, error) {
	if !ValidKind(kind) || tmdb <= 0 {
		return Entity{}, ErrNotFound
	}
	return s.lookup(ctx, kind, strconv.FormatInt(tmdb, 10))
}

// Person answers a person by TMDB ID.
func (s *Service) Person(ctx context.Context, tmdb int64) (Entity, error) {
	if tmdb <= 0 {
		return Entity{}, ErrNotFound
	}
	return s.lookup(ctx, "person", strconv.FormatInt(tmdb, 10))
}

// Artist answers a music artist by MusicBrainz ID, with its facts:
// MusicBrainz's, and Wikidata's only where MusicBrainz has none.
func (s *Service) Artist(ctx context.Context, id string) (Entity, error) {
	if !ValidMBID(id) {
		return Entity{}, ErrNotFound
	}
	e, err := s.lookup(ctx, "artist", id)
	if err != nil {
		return e, err
	}
	mb, _ := s.artistMB(ctx, id)
	e.Facts = mergeFacts(mb, e)
	return e, nil
}

func (s *Service) lookup(ctx context.Context, kind, id string) (Entity, error) {
	key := kind + ":" + id
	now := s.opts.Now()
	var body []byte
	var at int64
	cached := s.db.QueryRowContext(ctx, `SELECT body, fetched_at FROM answers WHERE k=?`, key).Scan(&body, &at) == nil
	if cached {
		age := now.Sub(time.Unix(at, 0))
		if body == nil && age < s.opts.Miss {
			return Entity{}, ErrNotFound
		}
		var e Entity
		if body != nil && age < s.fresh(kind) && json.Unmarshal(body, &e) == nil && e.V >= shapeV[kind] {
			return e, nil
		}
	}
	s.mu.Lock()
	s.lookups++
	s.mu.Unlock()
	e, err := s.fetch(ctx, kind, id)
	if errors.Is(err, ErrNotFound) {
		_, _ = s.db.ExecContext(ctx, `INSERT OR REPLACE INTO answers VALUES (?, NULL, ?)`, key, now.Unix())
		return Entity{}, err
	}
	if err != nil {
		s.mu.Lock()
		s.failures++
		s.mu.Unlock()
		// Serve the old answer rather than nothing.
		var old Entity
		if cached && body != nil && json.Unmarshal(body, &old) == nil {
			return old, nil
		}
		return Entity{}, err
	}
	e.FetchedAt = now.UTC().Format(time.RFC3339)
	if b, err := json.Marshal(e); err == nil {
		_, _ = s.db.ExecContext(ctx, `INSERT OR REPLACE INTO answers VALUES (?, ?, ?)`, key, b, now.Unix())
	}
	return e, nil
}

// fresh is how long a found answer of kind is kept.
func (s *Service) fresh(kind string) time.Duration {
	if kind == "artist" {
		return s.opts.ArtistFresh
	}
	return s.opts.Fresh
}

// Stored reports whether a title's answer (kind movie or tv) is kept and
// still fresh, or known missing: asking for it makes no outside call.
func (s *Service) Stored(ctx context.Context, kind string, tmdb int64) bool {
	var body []byte
	var at int64
	if s.db.QueryRowContext(ctx, `SELECT body, fetched_at FROM answers WHERE k=?`, kind+":"+strconv.FormatInt(tmdb, 10)).Scan(&body, &at) != nil {
		return false
	}
	age := s.opts.Now().Sub(time.Unix(at, 0))
	if body == nil {
		return age < s.opts.Miss
	}
	return age < s.fresh(kind)
}

func (s *Service) get(ctx context.Context, u string, accept string, out any) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
	if err != nil {
		return err
	}
	req.Header.Set("User-Agent", s.opts.UserAgent)
	req.Header.Set("Accept", accept)
	resp, err := s.opts.HTTP.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode == http.StatusNotFound {
		return ErrNotFound
	}
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("wikidata: status %d", resp.StatusCode)
	}
	return json.NewDecoder(io.LimitReader(resp.Body, 16<<20)).Decode(out)
}

func (s *Service) api(ctx context.Context, v url.Values, out any) error {
	v.Set("format", "json")
	v.Set("formatversion", "2")
	v.Set("maxlag", "5")
	var raw json.RawMessage
	if err := s.get(ctx, s.opts.APIURL+"?"+v.Encode(), "application/json", &raw); err != nil {
		return err
	}
	// An API error (maxlag when Wikidata is busy, and the like) is a failure to try again, never "not found".
	var e struct {
		Error *struct {
			Code string `json:"code"`
		} `json:"error"`
	}
	if json.Unmarshal(raw, &e) == nil && e.Error != nil {
		return fmt.Errorf("wikidata: api error %s", e.Error.Code)
	}
	return json.Unmarshal(raw, out)
}

// Wikidata's JSON, only the parts read here.
type snak struct {
	Snaktype  string `json:"snaktype"`
	Datavalue struct {
		Value json.RawMessage `json:"value"`
	} `json:"datavalue"`
}
type claim struct {
	Mainsnak   snak              `json:"mainsnak"`
	Rank       string            `json:"rank"`
	Qualifiers map[string][]snak `json:"qualifiers"`
}
type item struct {
	ID        string             `json:"id"`
	Missing   *string            `json:"missing"`
	Claims    map[string][]claim `json:"claims"`
	Sitelinks map[string]struct {
		Title string `json:"title"`
	} `json:"sitelinks"`
	Labels map[string]struct {
		Value string `json:"value"`
	} `json:"labels"`
}

func (n snak) str() string {
	var v string
	if n.Snaktype == "value" && json.Unmarshal(n.Datavalue.Value, &v) == nil {
		return v
	}
	return ""
}

func (n snak) qid() string {
	var v struct {
		ID string `json:"id"`
	}
	if n.Snaktype == "value" && json.Unmarshal(n.Datavalue.Value, &v) == nil {
		return v.ID
	}
	return ""
}

// date reads a time value as YYYY-MM-DD, YYYY-MM or YYYY by its precision.
func (n snak) date() string {
	var v struct {
		Time      string `json:"time"`
		Precision int    `json:"precision"`
	}
	if n.Snaktype != "value" || json.Unmarshal(n.Datavalue.Value, &v) != nil || len(v.Time) < 11 || v.Time[0] != '+' {
		return ""
	}
	d := v.Time[1:11]
	switch {
	case v.Precision >= 11:
		return d
	case v.Precision == 10:
		return d[:7]
	case v.Precision == 9:
		return d[:4]
	}
	return ""
}

// best is the first preferred, else normal, value of a property.
func best(cs []claim) snak {
	for _, rank := range []string{"preferred", "normal"} {
		for _, c := range cs {
			if c.Rank == rank && c.Mainsnak.Snaktype == "value" {
				return c.Mainsnak
			}
		}
	}
	return snak{}
}

// byStatement finds the item that carries the ID as its kind's property.
func (s *Service) byStatement(ctx context.Context, kind, id string) (string, error) {
	var found struct {
		Query struct {
			Search []struct {
				Title string `json:"title"`
			} `json:"search"`
		} `json:"query"`
	}
	if err := s.api(ctx, url.Values{"action": {"query"}, "list": {"search"}, "srlimit": {"1"}, "srsearch": {"haswbstatement:" + props[kind] + "=" + id}}, &found); err != nil {
		return "", err
	}
	if len(found.Query.Search) == 0 || !strings.HasPrefix(found.Query.Search[0].Title, "Q") {
		return "", ErrNotFound
	}
	return found.Query.Search[0].Title, nil
}

func (s *Service) fetch(ctx context.Context, kind, id string) (Entity, error) {
	qid, match := "", ""
	var err error
	if kind == "artist" {
		qid, match, err = s.findArtist(ctx, id)
	} else {
		qid, err = s.byStatement(ctx, kind, id)
	}
	if err != nil {
		return Entity{}, err
	}
	var got struct {
		Entities map[string]item `json:"entities"`
	}
	if err := s.api(ctx, url.Values{"action": {"wbgetentities"}, "ids": {qid}, "props": {"claims|sitelinks"}, "sitefilter": {"enwiki"}}, &got); err != nil {
		return Entity{}, err
	}
	it, ok := got.Entities[qid]
	if !ok || it.Missing != nil {
		return Entity{}, ErrNotFound
	}
	e := Entity{QID: qid, Kind: kind, ID: id, Awards: []Award{}, Tallies: []Tally{}, Match: match, V: shapeV[kind]}
	e.IDs = IDs{IMDb: best(it.Claims["P345"]).str(), TVDB: best(it.Claims["P4835"]).str(), MusicBrainz: best(it.Claims["P434"]).str(), Discogs: best(it.Claims["P1953"]).str(), AniList: best(it.Claims["P8729"]).str()}
	if kind == "person" || kind == "artist" {
		e.Born = best(it.Claims["P569"]).date()
		e.Died = best(it.Claims["P570"]).date()
	}
	if kind == "person" {
		e.Image = CommonsThumb(best(it.Claims["P18"]).str(), 330)
	}
	need := map[string]bool{}
	var genres []string
	if kind == "artist" {
		if e.Country, err = s.countryOf(ctx, it); err != nil {
			return Entity{}, err
		}
		e.Begin = artistBegin(it)
		for _, c := range it.Claims["P136"] {
			if g := c.Mainsnak.qid(); g != "" && c.Rank != "deprecated" {
				genres = append(genres, g)
				need[g] = true
			}
		}
	}
	for _, p := range []string{"P166", "P1411"} {
		for _, c := range it.Claims[p] {
			if c.Rank == "deprecated" {
				continue
			}
			a := Award{QID: c.Mainsnak.qid(), Won: p == "P166"}
			if a.QID == "" {
				continue
			}
			if y := best(asClaims(c.Qualifiers["P585"])).date(); len(y) >= 4 {
				a.Year, _ = strconv.Atoi(y[:4])
			}
			a.ForWork = best(asClaims(c.Qualifiers["P1686"])).qid()
			need[a.QID] = true
			if a.ForWork != "" {
				need[a.ForWork] = true
			}
			e.Awards = append(e.Awards, a)
		}
	}
	birth, death := "", ""
	if kind == "person" || kind == "artist" {
		birth, death = best(it.Claims["P19"]).qid(), best(it.Claims["P20"]).qid()
		need[birth], need[death] = birth != "", death != ""
	}
	labels, err := s.labels(ctx, need)
	if err != nil {
		return Entity{}, err
	}
	e.BirthPlace, e.DeathPlace = labels[birth], labels[death]
	for _, g := range genres {
		if l := labels[g]; l != "" && !slices.Contains(e.Genres, l) {
			e.Genres = append(e.Genres, l)
		}
	}
	out := e.Awards[:0]
	for _, a := range e.Awards {
		a.Name = labels[a.QID]
		if a.Name == "" {
			continue
		}
		a.Family = Family(a.Name)
		a.ForWork = labels[a.ForWork]
		out = append(out, a)
	}
	e.Awards = out
	sort.SliceStable(e.Awards, func(i, j int) bool {
		if e.Awards[i].Won != e.Awards[j].Won {
			return e.Awards[i].Won
		}
		return e.Awards[i].Year > e.Awards[j].Year
	})
	e.Tallies = tally(e.Awards)
	if link, ok := it.Sitelinks["enwiki"]; ok && link.Title != "" {
		e.Wikipedia = link.Title
		if sum, err := s.summary(ctx, link.Title); err == nil {
			e.Summary = &sum
		}
	}
	return e, nil
}

func asClaims(ns []snak) []claim {
	out := make([]claim, len(ns))
	for i, n := range ns {
		out[i] = claim{Mainsnak: n, Rank: "normal"}
	}
	return out
}

// labels reads English labels, 50 items per call.
func (s *Service) labels(ctx context.Context, need map[string]bool) (map[string]string, error) {
	var ids []string
	for id, ok := range need {
		if ok && id != "" {
			ids = append(ids, id)
		}
	}
	sort.Strings(ids)
	out := map[string]string{}
	for len(ids) > 0 {
		n := min(50, len(ids))
		var got struct {
			Entities map[string]item `json:"entities"`
		}
		if err := s.api(ctx, url.Values{"action": {"wbgetentities"}, "ids": {strings.Join(ids[:n], "|")}, "props": {"labels"}, "languages": {"en"}, "languagefallback": {"1"}}, &got); err != nil {
			return nil, err
		}
		for id, it := range got.Entities {
			if l, ok := it.Labels["en"]; ok {
				out[id] = l.Value
			}
		}
		ids = ids[n:]
	}
	return out, nil
}

// Family names the major award an award belongs to, by its label.
func Family(name string) string {
	n := strings.ToLower(name)
	switch {
	case strings.HasPrefix(n, "academy award"):
		return "oscar"
	case strings.Contains(n, "emmy"):
		return "emmy"
	case strings.HasPrefix(n, "golden globe"):
		return "golden_globe"
	case strings.HasPrefix(n, "bafta") || strings.HasPrefix(n, "british academy"):
		return "bafta"
	case strings.HasPrefix(n, "grammy"):
		return "grammy"
	}
	return ""
}

var familyOrder = map[string]int{"oscar": 0, "emmy": 1, "golden_globe": 2, "bafta": 3, "grammy": 4}

func tally(as []Award) []Tally {
	m := map[string]*Tally{}
	for _, a := range as {
		if a.Family == "" {
			continue
		}
		t := m[a.Family]
		if t == nil {
			t = &Tally{Family: a.Family}
			m[a.Family] = t
		}
		if a.Won {
			t.Wins++
		} else {
			t.Nominations++
		}
	}
	out := []Tally{}
	for _, t := range m {
		out = append(out, *t)
	}
	sort.Slice(out, func(i, j int) bool { return familyOrder[out[i].Family] < familyOrder[out[j].Family] })
	return out
}

func (s *Service) summary(ctx context.Context, title string) (Summary, error) {
	var got struct {
		Title       string `json:"title"`
		Extract     string `json:"extract"`
		ContentURLs struct {
			Desktop struct {
				Page string `json:"page"`
			} `json:"desktop"`
		} `json:"content_urls"`
	}
	path := url.PathEscape(strings.ReplaceAll(title, " ", "_"))
	if err := s.get(ctx, s.opts.WikipediaURL+"/page/summary/"+path, "application/json", &got); err != nil {
		return Summary{}, err
	}
	page := got.ContentURLs.Desktop.Page
	if !WikipediaPage(page) {
		page = "https://en.wikipedia.org/wiki/" + path
	}
	if got.Extract == "" {
		return Summary{}, ErrNotFound
	}
	return Summary{Title: got.Title, Extract: got.Extract, URL: page, License: License, LicenseURL: LicenseURL, Attribution: "From Wikipedia, " + License}, nil
}

// WikipediaPage reports an https link on a *.wikipedia.org host.
func WikipediaPage(raw string) bool {
	u, err := url.Parse(raw)
	if err != nil {
		return false
	}
	h := strings.ToLower(u.Hostname())
	return u.Scheme == "https" && u.User == nil && (h == "wikipedia.org" || strings.HasSuffix(h, ".wikipedia.org"))
}

// winnersQuery lists films (P4947) or series (P4983) that received an
// award of a major family (Academy Awards, Golden Globes, Emmys, Primetime
// Emmys, BAFTA Film Awards), with the number of such wins.
const winnersQuery = `SELECT ?tmdb (COUNT(DISTINCT ?award) AS ?wins) WHERE {
  VALUES ?family { wd:Q19020 wd:Q1011547 wd:Q123737 wd:Q1044427 wd:Q732997 }
  ?award (wdt:P31|wdt:P279|wdt:P361) ?family .
  ?item wdt:P166 ?award .
  ?item wdt:%s ?tmdb .
} GROUP BY ?tmdb`

// Refresh rebuilds the award-winner lists: one query-service call per
// kind, a week apart.
func (s *Service) Refresh(ctx context.Context) error {
	w := map[string][]Winner{}
	var err error
	for _, kind := range []string{"movie", "tv"} {
		var got struct {
			Results struct {
				Bindings []map[string]struct {
					Value string `json:"value"`
				} `json:"bindings"`
			} `json:"results"`
		}
		q := fmt.Sprintf(winnersQuery, props[kind])
		if err = s.get(ctx, s.opts.SPARQLURL+"?"+url.Values{"query": {q}}.Encode(), "application/sparql-results+json", &got); err != nil {
			break
		}
		seen := map[int64]bool{}
		list := []Winner{}
		for _, b := range got.Results.Bindings {
			id, e1 := strconv.ParseInt(b["tmdb"].Value, 10, 64)
			wins, _ := strconv.Atoi(b["wins"].Value)
			if e1 != nil || id <= 0 || seen[id] {
				continue
			}
			seen[id] = true
			list = append(list, Winner{TMDB: id, Wins: wins})
		}
		sort.SliceStable(list, func(i, j int) bool {
			if list[i].Wins != list[j].Wins {
				return list[i].Wins > list[j].Wins
			}
			return list[i].TMDB < list[j].TMDB
		})
		w[kind] = list
	}
	if err == nil && len(w["movie"]) == 0 {
		err = errors.New("wikidata: empty winner list")
	}
	now := s.opts.Now()
	s.mu.Lock()
	defer s.mu.Unlock()
	if err != nil {
		s.lastErr = err.Error()
		return err
	}
	b, _ := json.Marshal(w)
	_, _ = s.db.ExecContext(ctx, `INSERT OR REPLACE INTO meta VALUES ('winners', ?, ?)`, b, now.Unix())
	s.winners, s.winnersAt, s.lastErr = w, now, ""
	return nil
}

// Winners is the cached award-winner list for a kind, most wins first.
func (s *Service) Winners(kind string) ([]Winner, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	w, ok := s.winners[kind]
	return w, ok && len(w) > 0
}

// Stats reports lookups and the winner lists.
func (s *Service) Stats() Stats {
	s.mu.Lock()
	defer s.mu.Unlock()
	st := Stats{Lookups: s.lookups, Failures: s.failures, Movies: len(s.winners["movie"]), Shows: len(s.winners["tv"]), WinnerError: s.lastErr}
	if !s.winnersAt.IsZero() {
		st.WinnersAt = s.winnersAt.UTC().Format(time.RFC3339)
	}
	return st
}

// Run rebuilds the winner lists when older than Every, retrying failures
// after an hour.
func (s *Service) Run(ctx context.Context) {
	for {
		s.mu.Lock()
		due := s.opts.Now().Sub(s.winnersAt) >= s.opts.Every
		s.mu.Unlock()
		wait := time.Hour
		if due {
			c, cancel := context.WithTimeout(ctx, 2*time.Minute)
			if s.Refresh(c) == nil {
				wait = s.opts.Every
			}
			cancel()
		}
		select {
		case <-ctx.Done():
			return
		case <-time.After(wait):
		}
	}
}

// shapeV is each kind's current answer shape: a person's 1 has the
// Commons photo, an artist's 1 the MusicBrainz-first item and its facts.
var shapeV = map[string]int{"person": 1, "artist": 1}

// CommonsThumb is the upload.wikimedia.org thumbnail of a Commons file at width w
// (the path uses the MD5 of the file name with spaces as underscores); "" for no file.
func CommonsThumb(file string, w int) string {
	name := strings.ReplaceAll(strings.TrimSpace(file), " ", "_")
	if name == "" {
		return ""
	}
	sum := fmt.Sprintf("%x", md5.Sum([]byte(name)))
	esc := url.PathEscape(name)
	thumb := fmt.Sprintf("%dpx-%s", w, esc)
	if strings.HasSuffix(strings.ToLower(name), ".svg") {
		thumb += ".png"
	}
	return fmt.Sprintf("https://upload.wikimedia.org/wikipedia/commons/thumb/%s/%s/%s/%s", sum[:1], sum[:2], esc, thumb)
}
