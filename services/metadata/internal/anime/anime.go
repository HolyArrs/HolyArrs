// Package anime maps anime IDs between AniDB, AniList, MyAnimeList, Kitsu,
// TVDB and TMDB, with the season and episode offsets that fix cour splits
// and absolute numbering. A daily job downloads two community lists from
// GitHub and rebuilds the mapping tables:
//
//   - Anime-Lists/anime-lists anime-list-master.xml: AniDB to TVDB/TMDB,
//     default seasons, episode offsets and per-episode remaps;
//   - Fribb/anime-lists anime-list-full.json: adds AniList, MAL and Kitsu
//     IDs (and TMDB/TVDB where the first list has none).
//
// Neither repository carries a licence, so the raw files are kept only for
// lookups and the API exposes derived ID and offset mappings, never the
// lists' names or other fields. AniDB itself is never called.
//
// AniList title variants (romaji, English, native) are read from AniList's
// public GraphQL API only for titles someone looks up, a few at a time, and
// cached for a month.
package anime

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"encoding/xml"
	"errors"
	"fmt"
	"io"
	"net/http"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	_ "modernc.org/sqlite"
)

// Default sources (raw GitHub, master branch).
const (
	MasterURL  = "https://raw.githubusercontent.com/Anime-Lists/anime-lists/master/anime-list-master.xml"
	FullURL    = "https://raw.githubusercontent.com/Fribb/anime-lists/master/anime-list-full.json"
	AniListURL = "https://graphql.anilist.co"
)

// ErrNotFound: no list maps the asked ID.
var ErrNotFound = errors.New("not found")

// ErrNoData: the lists have not been loaded yet.
var ErrNoData = errors.New("no data")

// Key kinds a lookup may use.
const (
	KeyTMDBTV    = "tmdb_tv"
	KeyTMDBMovie = "tmdb_movie"
	KeyTVDB      = "tvdb"
	KeyAniList   = "anilist"
	KeyMAL       = "mal"
)

// Options configure the service.
type Options struct {
	HTTP       *http.Client
	UserAgent  string
	MasterURL  string
	FullURL    string
	AniListURL string
	// Every is how often the lists are downloaded (default 24 h).
	Every time.Duration
	// TitleTTL is how long AniList titles are kept (default 24 hours).
	TitleTTL time.Duration
	// MaxTitles is how many AniList IDs one lookup may ask titles for
	// (default 8).
	MaxTitles int
	Now       func() time.Time
}

// EpisodeMap remaps episodes of one AniDB season onto a TVDB or TMDB season.
type EpisodeMap struct {
	AniDBSeason int  `json:"anidb_season"`
	TVDBSeason  *int `json:"tvdb_season,omitempty"`
	TMDBSeason  *int `json:"tmdb_season,omitempty"`
	// Start..End (AniDB episodes) shift by Offset when set.
	Start  int `json:"start,omitempty"`
	End    int `json:"end,omitempty"`
	Offset int `json:"offset,omitempty"`
	// Pairs are single-episode remaps: AniDB episode to target episodes
	// (0 means no equivalent).
	Pairs []Pair `json:"pairs,omitempty"`
}

// Pair maps one AniDB episode to one or more target episodes.
type Pair struct {
	AniDB int   `json:"anidb"`
	To    []int `json:"to"`
}

// Entry is one AniDB (or AniList-only) title with its IDs and offsets.
type Entry struct {
	AniDB      int64   `json:"anidb,omitempty"`
	AniList    int64   `json:"anilist,omitempty"`
	MAL        int64   `json:"mal,omitempty"`
	Kitsu      int64   `json:"kitsu,omitempty"`
	Type       string  `json:"type,omitempty"`
	IMDb       string  `json:"imdb,omitempty"`
	TVDB       int64   `json:"tvdb,omitempty"`
	TVDBSeason *int    `json:"tvdb_season,omitempty"`
	TVDBOffset int     `json:"tvdb_offset,omitempty"`
	TMDBTV     int64   `json:"tmdb_tv,omitempty"`
	TMDBSeason *int    `json:"tmdb_season,omitempty"`
	TMDBOffset int     `json:"tmdb_offset,omitempty"`
	TMDBMovies []int64 `json:"tmdb_movies,omitempty"`
	// Absolute: the TVDB/TMDB side numbers episodes absolutely (no season).
	Absolute bool         `json:"absolute,omitempty"`
	Episodes []EpisodeMap `json:"episodes,omitempty"`
}

// Title is one AniList title's variants.
type Title struct {
	AniList int64  `json:"anilist"`
	Romaji  string `json:"romaji,omitempty"`
	English string `json:"english,omitempty"`
	Native  string `json:"native,omitempty"`
}

// Answer is the body of GET /v1/anime/map.
type Answer struct {
	Entries   []Entry `json:"entries"`
	Titles    []Title `json:"titles"`
	UpdatedAt string  `json:"updated_at"`
}

// Stats describe the loaded lists.
type Stats struct {
	Entries   int64  `json:"entries"`
	UpdatedAt string `json:"updated_at"`
	LastError string `json:"last_error,omitempty"`
}

// Service keeps the mappings in its own SQLite file.
type Service struct {
	db      *sql.DB
	opts    Options
	mu      sync.Mutex // one refresh at a time
	titleMu sync.Mutex // one AniList call at a time
	lastErr string
}

func Open(path string, o Options) (*Service, error) {
	if o.HTTP == nil {
		o.HTTP = &http.Client{Timeout: time.Minute}
	}
	if o.MasterURL == "" {
		o.MasterURL = MasterURL
	}
	if o.FullURL == "" {
		o.FullURL = FullURL
	}
	if o.AniListURL == "" {
		o.AniListURL = AniListURL
	}
	if o.Every <= 0 {
		o.Every = 24 * time.Hour
	}
	if o.TitleTTL <= 0 {
		o.TitleTTL = 24 * time.Hour
	}
	if o.MaxTitles <= 0 {
		o.MaxTitles = 8
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
CREATE TABLE IF NOT EXISTS entries (id INTEGER PRIMARY KEY, data TEXT NOT NULL);
CREATE TABLE IF NOT EXISTS keys (kind TEXT NOT NULL, ext INTEGER NOT NULL, entry INTEGER NOT NULL, PRIMARY KEY(kind, ext, entry)) WITHOUT ROWID;
CREATE TABLE IF NOT EXISTS titles (anilist INTEGER PRIMARY KEY, romaji TEXT NOT NULL, english TEXT NOT NULL, native TEXT NOT NULL, fetched_at INTEGER NOT NULL);
CREATE TABLE IF NOT EXISTS meta (k TEXT PRIMARY KEY, v TEXT NOT NULL) WITHOUT ROWID;`); err != nil {
		db.Close()
		return nil, err
	}
	return &Service{db: db, opts: o}, nil
}

func (s *Service) Close() error { return s.db.Close() }

// Run refreshes the lists now when they are missing or stale, then every
// Every; a failed download keeps the old tables and retries in an hour.
func (s *Service) Run(ctx context.Context) {
	for {
		wait := time.Hour
		if due := s.due(ctx); due > 0 {
			wait = due
		} else if err := s.Refresh(ctx); err == nil {
			wait = s.opts.Every
		}
		select {
		case <-ctx.Done():
			return
		case <-time.After(wait):
		}
	}
}

// due is how long until the next refresh (0: now).
func (s *Service) due(ctx context.Context) time.Duration {
	var v string
	if s.db.QueryRowContext(ctx, `SELECT v FROM meta WHERE k='updated_at'`).Scan(&v) != nil {
		return 0
	}
	t, err := time.Parse(time.RFC3339, v)
	if err != nil {
		return 0
	}
	return max(0, t.Add(s.opts.Every).Sub(s.opts.Now()))
}

func (s *Service) download(ctx context.Context, u string) ([]byte, error) {
	r, err := http.NewRequestWithContext(ctx, "GET", u, nil)
	if err != nil {
		return nil, err
	}
	if s.opts.UserAgent != "" {
		r.Header.Set("User-Agent", s.opts.UserAgent)
	}
	resp, err := s.opts.HTTP.Do(r)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		return nil, fmt.Errorf("%s: status %d", u, resp.StatusCode)
	}
	return io.ReadAll(io.LimitReader(resp.Body, 128<<20))
}

// Refresh downloads both lists and replaces the mapping tables.
func (s *Service) Refresh(ctx context.Context) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	err := s.refresh(ctx)
	if err != nil {
		s.lastErr = err.Error()
	} else {
		s.lastErr = ""
	}
	return err
}

func (s *Service) refresh(ctx context.Context) error {
	master, err := s.download(ctx, s.opts.MasterURL)
	if err != nil {
		return err
	}
	full, err := s.download(ctx, s.opts.FullURL)
	if err != nil {
		return err
	}
	entries, err := Merge(master, full)
	if err != nil {
		return err
	}
	if len(entries) < 1 {
		return errors.New("empty lists")
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if _, err = tx.Exec(`DELETE FROM entries; DELETE FROM keys;`); err != nil {
		return err
	}
	ins, err := tx.Prepare(`INSERT INTO entries(id, data) VALUES(?, ?)`)
	if err != nil {
		return err
	}
	defer ins.Close()
	key, err := tx.Prepare(`INSERT OR IGNORE INTO keys(kind, ext, entry) VALUES(?, ?, ?)`)
	if err != nil {
		return err
	}
	defer key.Close()
	for i, e := range entries {
		b, _ := json.Marshal(e)
		if _, err = ins.Exec(i+1, string(b)); err != nil {
			return err
		}
		add := func(kind string, id int64) error {
			if id <= 0 {
				return nil
			}
			_, err := key.Exec(kind, id, i+1)
			return err
		}
		ids := []struct {
			k  string
			id int64
		}{{KeyTMDBTV, e.TMDBTV}, {KeyTVDB, e.TVDB}, {KeyAniList, e.AniList}, {KeyMAL, e.MAL}}
		for _, m := range e.TMDBMovies {
			ids = append(ids, struct {
				k  string
				id int64
			}{KeyTMDBMovie, m})
		}
		for _, x := range ids {
			if err = add(x.k, x.id); err != nil {
				return err
			}
		}
	}
	if _, err = tx.Exec(`INSERT OR REPLACE INTO meta(k, v) VALUES('updated_at', ?)`, s.opts.Now().UTC().Format(time.RFC3339)); err != nil {
		return err
	}
	return tx.Commit()
}

// Stats reports the loaded lists.
func (s *Service) Stats() Stats {
	var st Stats
	s.db.QueryRow(`SELECT count(*) FROM entries`).Scan(&st.Entries)
	s.db.QueryRow(`SELECT v FROM meta WHERE k='updated_at'`).Scan(&st.UpdatedAt)
	s.mu.Lock()
	st.LastError = s.lastErr
	s.mu.Unlock()
	return st
}

// Lookup answers one ID of kind (KeyTMDBTV, KeyTMDBMovie, KeyTVDB,
// KeyAniList or KeyMAL) with every entry that shares its show, ordered by
// season, plus AniList titles for the entries (best effort).
func (s *Service) Lookup(ctx context.Context, kind string, id int64) (Answer, error) {
	var a Answer
	if s.db.QueryRowContext(ctx, `SELECT v FROM meta WHERE k='updated_at'`).Scan(&a.UpdatedAt) != nil {
		return a, ErrNoData
	}
	entries, err := s.entries(ctx, kind, id)
	if err != nil {
		return a, err
	}
	// An AniList or MAL hit widens to the whole TVDB/TMDB show, so the
	// app gets every season of it.
	if (kind == KeyAniList || kind == KeyMAL) && len(entries) == 1 {
		if e := entries[0]; e.TMDBTV > 0 {
			if more, err := s.entries(ctx, KeyTMDBTV, e.TMDBTV); err == nil {
				entries = more
			}
		} else if e.TVDB > 0 {
			if more, err := s.entries(ctx, KeyTVDB, e.TVDB); err == nil {
				entries = more
			}
		}
	}
	if len(entries) == 0 {
		return a, ErrNotFound
	}
	sort.SliceStable(entries, func(i, j int) bool {
		si, sj := season(entries[i], kind), season(entries[j], kind)
		if si != sj {
			return si < sj
		}
		return entries[i].AniDB < entries[j].AniDB
	})
	a.Entries = entries
	a.Titles = s.titles(ctx, entries)
	return a, nil
}

// season orders entries: specials (0) first, absolute ones last.
func season(e Entry, kind string) int {
	p := e.TMDBSeason
	if kind == KeyTVDB || p == nil {
		if e.TVDBSeason != nil {
			p = e.TVDBSeason
		}
	}
	if p == nil {
		return 1 << 20
	}
	return *p
}

func (s *Service) entries(ctx context.Context, kind string, id int64) ([]Entry, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT e.data FROM keys k JOIN entries e ON e.id = k.entry WHERE k.kind = ? AND k.ext = ? ORDER BY e.id LIMIT 200`, kind, id)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Entry
	for rows.Next() {
		var b string
		if err := rows.Scan(&b); err != nil {
			return nil, err
		}
		var e Entry
		if json.Unmarshal([]byte(b), &e) == nil {
			out = append(out, e)
		}
	}
	return out, rows.Err()
}

// titles reads cached AniList titles and fetches the missing ones in one
// call; any failure just leaves them out.
func (s *Service) titles(ctx context.Context, entries []Entry) []Title {
	var ids []int64
	seen := map[int64]bool{}
	for _, e := range entries {
		if e.AniList > 0 && !seen[e.AniList] && len(ids) < s.opts.MaxTitles {
			seen[e.AniList] = true
			ids = append(ids, e.AniList)
		}
	}
	out := []Title{}
	if len(ids) == 0 {
		return out
	}
	cutoff := s.opts.Now().Add(-s.opts.TitleTTL).Unix()
	have := map[int64]Title{}
	var missing []int64
	for _, id := range ids {
		var t Title
		var at int64
		if s.db.QueryRowContext(ctx, `SELECT romaji, english, native, fetched_at FROM titles WHERE anilist = ?`, id).Scan(&t.Romaji, &t.English, &t.Native, &at) == nil && at >= cutoff {
			t.AniList = id
			have[id] = t
		} else {
			missing = append(missing, id)
		}
	}
	if len(missing) > 0 {
		for _, t := range s.fetchTitles(ctx, missing) {
			have[t.AniList] = t
		}
	}
	for _, id := range ids {
		if t, ok := have[id]; ok && (t.Romaji != "" || t.English != "" || t.Native != "") {
			out = append(out, t)
		}
	}
	return out
}

func (s *Service) fetchTitles(ctx context.Context, ids []int64) []Title {
	s.titleMu.Lock()
	defer s.titleMu.Unlock()
	q, _ := json.Marshal(map[string]any{
		"query":     `query($ids:[Int]){Page(perPage:50){media(id_in:$ids,type:ANIME){id title{romaji english native}}}}`,
		"variables": map[string]any{"ids": ids},
	})
	ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	r, err := http.NewRequestWithContext(ctx, "POST", s.opts.AniListURL, bytes.NewReader(q))
	if err != nil {
		return nil
	}
	r.Header.Set("Content-Type", "application/json")
	r.Header.Set("Accept", "application/json")
	if s.opts.UserAgent != "" {
		r.Header.Set("User-Agent", s.opts.UserAgent)
	}
	resp, err := s.opts.HTTP.Do(r)
	if err != nil {
		return nil
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		return nil
	}
	var body struct {
		Data struct {
			Page struct {
				Media []struct {
					ID    int64 `json:"id"`
					Title struct {
						Romaji  string `json:"romaji"`
						English string `json:"english"`
						Native  string `json:"native"`
					} `json:"title"`
				} `json:"media"`
			} `json:"Page"`
		} `json:"data"`
	}
	if json.NewDecoder(io.LimitReader(resp.Body, 1<<20)).Decode(&body) != nil {
		return nil
	}
	now := s.opts.Now().Unix()
	got := map[int64]Title{}
	for _, m := range body.Data.Page.Media {
		got[m.ID] = Title{AniList: m.ID, Romaji: clean(m.Title.Romaji), English: clean(m.Title.English), Native: clean(m.Title.Native)}
	}
	var out []Title
	for _, id := range ids {
		t := got[id] // unknown IDs are cached empty, so they are not asked again
		t.AniList = id
		s.db.ExecContext(ctx, `INSERT OR REPLACE INTO titles(anilist, romaji, english, native, fetched_at) VALUES(?, ?, ?, ?, ?)`, id, t.Romaji, t.English, t.Native, now)
		out = append(out, t)
	}
	return out
}

func clean(s string) string {
	s = strings.TrimSpace(s)
	if len(s) > 300 {
		s = s[:300]
	}
	return s
}

// --- parsing ---

type xmlList struct {
	Anime []struct {
		AniDB         string `xml:"anidbid,attr"`
		TVDB          string `xml:"tvdbid,attr"`
		DefaultSeason string `xml:"defaulttvdbseason,attr"`
		EpisodeOffset string `xml:"episodeoffset,attr"`
		TMDBTV        string `xml:"tmdbtv,attr"`
		TMDBSeason    string `xml:"tmdbseason,attr"`
		TMDBOffset    string `xml:"tmdboffset,attr"`
		TMDBID        string `xml:"tmdbid,attr"`
		IMDb          string `xml:"imdbid,attr"`
		Mappings      []struct {
			AniDBSeason string `xml:"anidbseason,attr"`
			TVDBSeason  string `xml:"tvdbseason,attr"`
			TMDBSeason  string `xml:"tmdbseason,attr"`
			Start       string `xml:"start,attr"`
			End         string `xml:"end,attr"`
			Offset      string `xml:"offset,attr"`
			Text        string `xml:",chardata"`
		} `xml:"mapping-list>mapping"`
	} `xml:"anime"`
}

type fribbRow struct {
	Type    string          `json:"type"`
	AniDB   int64           `json:"anidb_id"`
	AniList int64           `json:"anilist_id"`
	MAL     int64           `json:"mal_id"`
	Kitsu   int64           `json:"kitsu_id"`
	IMDb    []string        `json:"imdb_id"`
	TMDB    json.RawMessage `json:"themoviedb_id"`
	TVDB    int64           `json:"tvdb_id"`
	Season  struct {
		TVDB *int `json:"tvdb"`
		TMDB *int `json:"tmdb"`
	} `json:"season"`
	Offset struct {
		TVDB int `json:"tvdb"`
		TMDB int `json:"tmdb"`
	} `json:"episode_offset"`
}

func num(s string) int64 {
	s = strings.TrimSpace(s)
	if i := strings.IndexByte(s, ','); i >= 0 {
		s = s[:i]
	}
	n, err := strconv.ParseInt(s, 10, 64)
	if err != nil || n < 0 {
		return 0
	}
	return n
}

func optInt(s string) *int {
	n, err := strconv.Atoi(strings.TrimSpace(s))
	if err != nil {
		return nil
	}
	return &n
}

func ids(s string) []int64 {
	var out []int64
	for _, p := range strings.Split(s, ",") {
		if n := num(p); n > 0 {
			out = append(out, n)
		}
	}
	return out
}

// pairs reads ";1-4;2-5+6;" into AniDB-to-target episode pairs.
func pairs(s string) []Pair {
	var out []Pair
	for _, p := range strings.Split(s, ";") {
		from, to, ok := strings.Cut(strings.TrimSpace(p), "-")
		a, err := strconv.Atoi(from)
		if !ok || err != nil {
			continue
		}
		var targets []int
		for _, t := range strings.Split(to, "+") {
			if n, err := strconv.Atoi(t); err == nil {
				targets = append(targets, n)
			}
		}
		if len(targets) > 0 {
			out = append(out, Pair{AniDB: a, To: targets})
		}
	}
	return out
}

// Merge parses both lists into entries: one per AniDB title (the master
// list's seasons and offsets win; Fribb's list adds AniList, MAL and Kitsu
// and fills gaps), plus Fribb rows without an AniDB ID that carry a TMDB or
// TVDB ID.
func Merge(masterXML, fullJSON []byte) ([]Entry, error) {
	var x xmlList
	if err := xml.Unmarshal(masterXML, &x); err != nil {
		return nil, fmt.Errorf("master list: %w", err)
	}
	var rows []fribbRow
	if err := json.Unmarshal(fullJSON, &rows); err != nil {
		return nil, fmt.Errorf("full list: %w", err)
	}
	byAniDB := map[int64]*Entry{}
	var order []int64
	for _, a := range x.Anime {
		id := num(a.AniDB)
		if id == 0 || byAniDB[id] != nil {
			continue
		}
		e := &Entry{AniDB: id, TVDB: num(a.TVDB), TMDBTV: num(a.TMDBTV), TMDBMovies: ids(a.TMDBID)}
		if imdb := strings.TrimSpace(strings.Split(a.IMDb, ",")[0]); strings.HasPrefix(imdb, "tt") {
			e.IMDb = imdb
		}
		if strings.TrimSpace(a.DefaultSeason) == "a" || strings.TrimSpace(a.TMDBSeason) == "a" {
			e.Absolute = true
		}
		if e.TVDB > 0 {
			e.TVDBSeason = optInt(a.DefaultSeason)
		}
		if e.TMDBTV > 0 {
			e.TMDBSeason = optInt(a.TMDBSeason)
		}
		if o := optInt(a.EpisodeOffset); o != nil {
			e.TVDBOffset = *o
		}
		if o := optInt(a.TMDBOffset); o != nil {
			e.TMDBOffset = *o
		}
		for _, m := range a.Mappings {
			as := optInt(m.AniDBSeason)
			if as == nil {
				continue
			}
			em := EpisodeMap{AniDBSeason: *as, TVDBSeason: optInt(m.TVDBSeason), TMDBSeason: optInt(m.TMDBSeason), Pairs: pairs(m.Text)}
			if v := optInt(m.Start); v != nil {
				em.Start = *v
			}
			if v := optInt(m.End); v != nil {
				em.End = *v
			}
			if v := optInt(m.Offset); v != nil {
				em.Offset = *v
			}
			if em.TVDBSeason == nil && em.TMDBSeason == nil {
				continue
			}
			e.Episodes = append(e.Episodes, em)
		}
		byAniDB[id] = e
		order = append(order, id)
	}
	var extra []Entry
	for _, r := range rows {
		var tmdb struct {
			TV    json.RawMessage `json:"tv"`
			Movie json.RawMessage `json:"movie"`
		}
		json.Unmarshal(r.TMDB, &tmdb)
		tv := firstID(tmdb.TV)
		movies := allIDs(tmdb.Movie)
		e := byAniDB[r.AniDB]
		if r.AniDB <= 0 || e == nil {
			if tv == 0 && r.TVDB == 0 && len(movies) == 0 {
				continue
			}
			n := Entry{AniDB: max(0, r.AniDB), TVDB: r.TVDB, TMDBTV: tv, TMDBMovies: movies, TVDBOffset: r.Offset.TVDB, TMDBOffset: r.Offset.TMDB}
			if r.TVDB > 0 {
				n.TVDBSeason = r.Season.TVDB
			}
			if tv > 0 {
				n.TMDBSeason = r.Season.TMDB
			}
			e = &n
			if r.AniDB > 0 {
				byAniDB[r.AniDB] = e
				order = append(order, r.AniDB)
			} else {
				extra = append(extra, n)
				e = &extra[len(extra)-1]
			}
		}
		if e.AniList == 0 {
			e.AniList = max(0, r.AniList)
		}
		if e.MAL == 0 {
			e.MAL = max(0, r.MAL)
		}
		if e.Kitsu == 0 {
			e.Kitsu = max(0, r.Kitsu)
		}
		if e.Type == "" {
			e.Type = r.Type
		}
		if e.IMDb == "" && len(r.IMDb) > 0 && strings.HasPrefix(r.IMDb[0], "tt") {
			e.IMDb = r.IMDb[0]
		}
		if e.TVDB == 0 && r.TVDB > 0 {
			e.TVDB, e.TVDBSeason, e.TVDBOffset = r.TVDB, r.Season.TVDB, r.Offset.TVDB
		}
		if e.TMDBTV == 0 && tv > 0 {
			e.TMDBTV, e.TMDBSeason, e.TMDBOffset = tv, r.Season.TMDB, r.Offset.TMDB
		}
		if len(e.TMDBMovies) == 0 {
			e.TMDBMovies = movies
		}
	}
	out := make([]Entry, 0, len(order)+len(extra))
	for _, id := range order {
		out = append(out, *byAniDB[id])
	}
	return append(out, extra...), nil
}

func allIDs(raw json.RawMessage) []int64 {
	if len(raw) == 0 {
		return nil
	}
	var one int64
	if json.Unmarshal(raw, &one) == nil {
		if one > 0 {
			return []int64{one}
		}
		return nil
	}
	var many []int64
	json.Unmarshal(raw, &many)
	var out []int64
	for _, n := range many {
		if n > 0 {
			out = append(out, n)
		}
	}
	return out
}

func firstID(raw json.RawMessage) int64 {
	if l := allIDs(raw); len(l) > 0 {
		return l[0]
	}
	return 0
}
