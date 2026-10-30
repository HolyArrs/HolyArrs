package api

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"net/http"
	"sort"
	"strings"
	"sync"
	"time"
	"unicode"

	"github.com/CoderLuii/holyarrs/internal/domain"
	"github.com/CoderLuii/holyarrs/internal/library"
	"github.com/CoderLuii/holyarrs/internal/metadata"
	"github.com/CoderLuii/holyarrs/internal/parse"
)

// Matching torrents HolyArrs did not send (Sonarr's and Radarr's "match
// first, then add"): each name is parsed, the title looked up once, and a
// sure match is added to the Library (when missing) and linked by info
// hash. A link lives in client_links only: it is never a download record,
// never marks anything as on disk, and adding never searches (the title is
// added with keep_up_to_date off and search_on_add off). Unsure matches wait
// in the Confirm match list with their looked-up choices.

// Link states.
const (
	linkLinked   = "linked"
	linkUnsure   = "unsure"
	linkNotMedia = "not_media"
	linkSkipped  = "skipped"
)

// matchRetry is how long a failed lookup waits before it is tried again.
var matchRetry = 30 * time.Minute

// Candidate is one looked-up choice for a torrent.
type Candidate struct {
	ID     string `json:"id"`   // "tmdb:603" (movie) or "tmdb:1399" / "tvmaze:82" (show)
	Kind   string `json:"kind"` // movie or show
	Name   string `json:"name"`
	Year   int    `json:"year,omitempty"`
	Poster string `json:"poster,omitempty"`
	Score  int    `json:"score"` // 0..100 name likeness
}

// matchWant is one outside torrent waiting for a match.
type matchWant struct {
	downloader int64
	hash, name string
	category   string
}

// clientMatch runs the matching one lookup at a time in the background.
type clientMatch struct {
	mu      sync.Mutex
	queue   []matchWant
	queued  map[clientKey]bool
	failed  map[clientKey]time.Time
	running bool
	// Hooks; nil means the real lookups and adds (tests swap them).
	searchMovies func(ctx context.Context, name string, year int) ([]metadata.Movie, error)
	searchShows  func(ctx context.Context, name string) ([]metadata.Show, error)
	addMovie     func(ctx context.Context, id string) (int64, error)
	addShow      func(ctx context.Context, id string) (int64, error)
	// done is told after each torrent (tests wait on it).
	done func()
}

// matchQueue hands rows with no link yet to the background matcher.
func (a *API) matchQueue(ctx context.Context, rows []DownloadRow) {
	if len(rows) == 0 {
		return
	}
	known := a.linkStates(ctx, rows)
	m := &a.match
	m.mu.Lock()
	if m.queued == nil {
		m.queued, m.failed = map[clientKey]bool{}, map[clientKey]time.Time{}
	}
	for _, r := range rows {
		k := clientKey{r.DownloaderID, strings.ToLower(r.Hash)}
		if !r.Outside || r.TitleID != 0 || known[k] != "" || m.queued[k] {
			continue
		}
		if t, ok := m.failed[k]; ok && time.Since(t) < matchRetry {
			continue
		}
		m.queued[k] = true
		m.queue = append(m.queue, matchWant{r.DownloaderID, k.hash, r.Name, r.Category})
	}
	start := !m.running && len(m.queue) > 0
	if start {
		m.running = true
	}
	m.mu.Unlock()
	if start {
		go a.matchLoop()
	}
}

func (a *API) matchLoop() {
	m := &a.match
	for {
		m.mu.Lock()
		if len(m.queue) == 0 {
			m.running = false
			m.mu.Unlock()
			return
		}
		w := m.queue[0]
		m.queue = m.queue[1:]
		m.mu.Unlock()
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
		err := a.matchOne(ctx, w)
		cancel()
		k := clientKey{w.downloader, w.hash}
		m.mu.Lock()
		delete(m.queued, k)
		if err != nil {
			log.Printf("api: match %q: %v", w.name, err)
			m.failed[k] = time.Now()
		} else {
			delete(m.failed, k)
		}
		done := m.done
		m.mu.Unlock()
		if done != nil {
			done()
		}
	}
}

// linkStates reads the stored link state of each row's torrent.
func (a *API) linkStates(ctx context.Context, rows []DownloadRow) map[clientKey]string {
	out := map[clientKey]string{}
	rs, err := a.s.Store.Read().QueryContext(ctx, "SELECT downloader_id, hash, state FROM client_links")
	if err != nil {
		return out
	}
	defer rs.Close()
	for rs.Next() {
		var k clientKey
		var st string
		if rs.Scan(&k.downloader, &k.hash, &st) == nil {
			out[k] = st
		}
	}
	return out
}

// guess is what a torrent name says.
type guess struct {
	kind     string // movie, show or "" (not media)
	title    string
	year     int
	seasons  []int
	episodes []int
	complete bool
}

// guessOf parses a torrent name, with the client category as a hint.
func guessOf(name, category string) guess {
	cat := strings.ToLower(category)
	tvHint := strings.Contains(cat, "tv") || strings.Contains(cat, "show") || strings.Contains(cat, "series") || strings.Contains(cat, "sonarr")
	movieHint := strings.Contains(cat, "movie") || strings.Contains(cat, "film") || strings.Contains(cat, "radarr")
	p := parse.Parse(name, parse.Hints{Show: tvHint})
	g := guess{title: strings.TrimSpace(p.Title.Value), year: p.Year.Value}
	if g.title == "" || p.IsMusic() || p.Has("executable") {
		return guess{}
	}
	switch {
	case p.IsShow():
		g.kind = "show"
		g.seasons, g.episodes = p.Show.Seasons, p.Show.Episodes
		g.complete = p.Kind.Value == parse.KindComplete
	case tvHint:
		g.kind = "show"
		g.complete = true
	case cat != "" && !movieHint:
		// Another category (linux, games, music...): not a movie or show.
	case movieHint || g.year > 0 || p.Resolution.Value > 0 || p.Codec.Value != "":
		g.kind = "movie"
	}
	return g
}

// matchFold folds a name for comparing: lower case letters and digits only, a
// leading "the" dropped and "&" read as "and".
func matchFold(s string) string {
	s = strings.ToLower(strings.ReplaceAll(s, "&", " and "))
	var b strings.Builder
	for _, r := range s {
		if unicode.IsLetter(r) || unicode.IsDigit(r) {
			b.WriteRune(r)
		} else {
			b.WriteRune(' ')
		}
	}
	f := strings.Fields(b.String())
	if len(f) > 1 && f[0] == "the" {
		f = f[1:]
	}
	return strings.Join(f, "")
}

// likeness scores two names 0..100 (edit distance on the folded names).
func likeness(a, b string) int {
	x, y := []rune(matchFold(a)), []rune(matchFold(b))
	if len(x) == 0 || len(y) == 0 {
		return 0
	}
	prev := make([]int, len(y)+1)
	cur := make([]int, len(y)+1)
	for j := range prev {
		prev[j] = j
	}
	for i := 1; i <= len(x); i++ {
		cur[0] = i
		for j := 1; j <= len(y); j++ {
			c := 1
			if x[i-1] == y[j-1] {
				c = 0
			}
			cur[j] = min(prev[j]+1, cur[j-1]+1, prev[j-1]+c)
		}
		prev, cur = cur, prev
	}
	return 100 - 100*prev[len(y)]/max(len(x), len(y))
}

// sureScore is the likeness a name needs to count as the same title.
const sureScore = 92

// pickSure returns the one sure candidate, or -1. A candidate is sure when
// its name matches; with a year in the name the year must match too (one
// off is allowed for a release late in the year). Two such candidates are
// not sure.
func pickSure(cs []Candidate, year int) int {
	hit := -1
	for i, c := range cs {
		if c.Score < sureScore {
			continue
		}
		if year > 0 && c.Year != 0 && (c.Year < year-1 || c.Year > year+1) {
			continue
		}
		if hit >= 0 {
			// Two alike: an exact year decides; otherwise unsure.
			if year > 0 && (cs[hit].Year == year) != (c.Year == year) {
				if c.Year == year {
					hit = i
				}
				continue
			}
			return -1
		}
		hit = i
	}
	return hit
}

// candidatesFor looks the guess up once and scores the answers.
func (a *API) candidatesFor(ctx context.Context, g guess) ([]Candidate, error) {
	var cs []Candidate
	m := &a.match
	switch g.kind {
	case "movie":
		find := m.searchMovies
		if find == nil {
			if a.s.Metadata == nil {
				return nil, errors.New("no movie details source")
			}
			find = a.s.Metadata.Search
		}
		ms, err := find(ctx, g.title, g.year)
		if err != nil {
			return nil, err
		}
		if len(ms) == 0 && g.year > 0 { // the year in a name is sometimes wrong
			if ms, err = find(ctx, g.title, 0); err != nil {
				return nil, err
			}
		}
		for _, x := range ms {
			if x.IDs["tmdb"] == "" {
				continue
			}
			cs = append(cs, Candidate{ID: "tmdb:" + x.IDs["tmdb"], Kind: "movie", Name: x.Name, Year: x.Year, Poster: x.Poster, Score: likeness(g.title, x.Name)})
		}
	case "show":
		find := m.searchShows
		if find == nil {
			find = func(ctx context.Context, name string) ([]metadata.Show, error) {
				d, err := library.ReadShowDefaults(ctx, a.s.Store.Read())
				if err != nil {
					return nil, err
				}
				p, err := a.showSource(d.Source)
				if err != nil {
					return nil, err
				}
				return p.SearchShows(ctx, name)
			}
		}
		ss, err := find(ctx, g.title)
		if err != nil {
			return nil, err
		}
		for _, x := range ss {
			src := x.Source
			if src == "" || x.IDs[src] == "" {
				for _, k := range []string{"tmdb", "tvdb", "tvmaze"} {
					if x.IDs[k] != "" {
						src = k
						break
					}
				}
			}
			if x.IDs[src] == "" {
				continue
			}
			cs = append(cs, Candidate{ID: src + ":" + x.IDs[src], Kind: "show", Name: x.Name, Year: x.Year, Poster: x.Poster, Score: likeness(g.title, x.Name)})
		}
	}
	sort.SliceStable(cs, func(i, j int) bool { return cs[i].Score > cs[j].Score })
	if len(cs) > 6 {
		cs = cs[:6]
	}
	return cs, nil
}

// matchOne matches one torrent and stores the outcome.
func (a *API) matchOne(ctx context.Context, w matchWant) error {
	g := guessOf(w.name, w.category)
	if g.kind == "" {
		return a.saveLink(ctx, w, linkNotMedia, 0, nil, "", nil, "name")
	}
	cs, err := a.candidatesFor(ctx, g)
	if err != nil {
		return err
	}
	i := pickSure(cs, g.year)
	if i < 0 && g.kind == "show" && g.year == 0 {
		// A show name without a year: one alike candidate is enough.
		n := 0
		for j, c := range cs {
			if c.Score >= sureScore {
				n, i = n+1, j
			}
		}
		if n != 1 {
			i = -1
		}
	}
	if i < 0 {
		if len(cs) == 0 {
			return a.saveLink(ctx, w, linkUnsure, 0, nil, "", []Candidate{}, "no candidates")
		}
		return a.saveLink(ctx, w, linkUnsure, 0, nil, "", cs, "unsure")
	}
	return a.addAndLink(ctx, w, g, cs[i], cs, "auto")
}

// addAndLink adds the candidate (or reuses the Library's copy) and links the
// torrent to it and the episodes it names.
func (a *API) addAndLink(ctx context.Context, w matchWant, g guess, c Candidate, cs []Candidate, how string) error {
	titleID, err := a.ensureTitle(ctx, c)
	if err != nil {
		var fe *library.FolderError
		if errors.Is(err, library.ErrNoShowFolder) || errors.As(err, &fe) || errors.Is(err, library.ErrNotShowFolder) || isNoFolder(err) {
			// Adding needs a choice that has no default: ask instead.
			return a.saveLink(ctx, w, linkUnsure, 0, nil, "", cs, "no default folder")
		}
		return err
	}
	slots, covers := a.linkSlots(ctx, titleID, c.Kind, g)
	return a.saveLink(ctx, w, linkLinked, titleID, slots, covers, cs, how)
}

func isNoFolder(err error) bool {
	return err != nil && strings.Contains(strings.ToLower(err.Error()), "folder")
}

// ensureTitle finds the candidate in the Library or adds it the normal way,
// without searching and without keeping it up to date.
func (a *API) ensureTitle(ctx context.Context, c Candidate) (int64, error) {
	db := a.s.Store.Read()
	kind, v, _ := strings.Cut(c.ID, ":")
	if c.Kind == "movie" {
		if id, err := library.TitleByTMDB(ctx, db, v); err != nil || id != 0 {
			return id, err
		}
		if a.match.addMovie != nil {
			return a.match.addMovie(ctx, c.ID)
		}
		m, err := a.s.Metadata.Movie(ctx, c.ID)
		if err != nil {
			return 0, err
		}
		opt := library.AddOptions{}
		if err := a.kindDefaultsFor(ctx, "movie", nil, &opt.LibraryFolderID, &opt.PickRulesID, &opt.KeepUpToDate, &opt.SearchOnAdd); err != nil {
			return 0, err
		}
		off := false
		opt.KeepUpToDate, opt.SearchOnAdd, opt.WhyAdded = &off, false, "import"
		id, _, err := a.s.Library.Add(ctx, m, opt, domain.System)
		return id, err
	}
	if id, err := library.ShowByIDs(ctx, db, map[string]string{kind: v}); err != nil || id != 0 {
		return id, err
	}
	if a.match.addShow != nil {
		return a.match.addShow(ctx, c.ID)
	}
	opt := library.ShowAddOptions{}
	if err := a.kindDefaultsFor(ctx, "show", nil, &opt.LibraryFolderID, &opt.PickRulesID, &opt.KeepUpToDate, &opt.SearchOnAdd); err != nil {
		return 0, err
	}
	off := false
	opt.KeepUpToDate, opt.SearchOnAdd, opt.WhyAdded = &off, false, "import"
	id, _, err := a.s.Library.AddShow(ctx, c.ID, opt, domain.System)
	return id, err
}

// linkSlots finds the main Version's Slots the torrent covers and a short
// line naming them.
func (a *API) linkSlots(ctx context.Context, titleID int64, kind string, g guess) ([]int64, string) {
	db := a.s.Store.Read()
	q := `SELECT s.id, COALESCE(json_extract(i.numbers, '$.season'), 0), COALESCE(json_extract(i.numbers, '$.episode'), 0)
		FROM slots s JOIN items i ON i.id = s.item_id JOIN versions v ON v.id = s.version_id
		WHERE i.title_id = ? AND v.is_main = 1 ORDER BY 2, 3`
	rs, err := db.QueryContext(ctx, q, titleID)
	if err != nil {
		return nil, ""
	}
	defer rs.Close()
	var ids []int64
	for rs.Next() {
		var id int64
		var se, ep int
		if rs.Scan(&id, &se, &ep) != nil {
			continue
		}
		if kind == "show" && !coversEpisode(g, se, ep) {
			continue
		}
		ids = append(ids, id)
	}
	if kind != "show" {
		return ids, ""
	}
	line := coversLine(g)
	if len(ids) == 1 && len(g.episodes) == 1 {
		var name string
		_ = db.QueryRowContext(ctx, "SELECT COALESCE(i.name, '') FROM slots s JOIN items i ON i.id = s.item_id WHERE s.id = ?", ids[0]).Scan(&name)
		if name != "" {
			line += " · " + name
		}
	}
	return ids, line
}

func coversEpisode(g guess, se, ep int) bool {
	if g.complete || len(g.seasons) == 0 {
		return se > 0
	}
	in := false
	for _, s := range g.seasons {
		in = in || s == se
	}
	if !in {
		return false
	}
	if len(g.episodes) == 0 {
		return true
	}
	for _, e := range g.episodes {
		if e == ep {
			return true
		}
	}
	return false
}

// coversLine names what a show torrent holds: S01E02, S01E02-E03, Season 1,
// Seasons 1-3 or Complete.
func coversLine(g guess) string {
	switch {
	case g.complete || len(g.seasons) == 0:
		return "Complete"
	case len(g.seasons) > 1:
		return fmt.Sprintf("Seasons %d-%d", g.seasons[0], g.seasons[len(g.seasons)-1])
	case len(g.episodes) == 0:
		return fmt.Sprintf("Season %d", g.seasons[0])
	case len(g.episodes) == 1:
		return fmt.Sprintf("S%02dE%02d", g.seasons[0], g.episodes[0])
	}
	return fmt.Sprintf("S%02dE%02d-E%02d", g.seasons[0], g.episodes[0], g.episodes[len(g.episodes)-1])
}

// saveLink writes (or replaces) the torrent's one link row.
func (a *API) saveLink(ctx context.Context, w matchWant, state string, titleID int64, slots []int64, covers string, cs []Candidate, how string) error {
	if slots == nil {
		slots = []int64{}
	}
	sj, _ := json.Marshal(slots)
	var cj []byte
	if cs != nil {
		cj, _ = json.Marshal(cs)
	}
	var tid any
	if titleID != 0 {
		tid = titleID
	}
	return a.s.Store.Write(ctx, func(tx *sql.Tx) error {
		if cj == nil { // keep the choices already stored
			_ = tx.QueryRow("SELECT candidates FROM client_links WHERE downloader_id = ? AND hash = ?", w.downloader, w.hash).Scan(&cj)
			if cj == nil {
				cj = []byte("[]")
			}
		}
		_, err := tx.Exec(`INSERT INTO client_links (downloader_id, hash, name, state, title_id, slot_ids, covers, candidates, how, matched_at)
			VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
			ON CONFLICT (downloader_id, hash) DO UPDATE SET name = excluded.name, state = excluded.state, title_id = excluded.title_id,
			slot_ids = excluded.slot_ids, covers = excluded.covers, candidates = excluded.candidates, how = excluded.how, matched_at = excluded.matched_at`,
			w.downloader, w.hash, w.name, state, tid, string(sj), covers, string(cj), how, time.Now().UTC().Format(time.RFC3339))
		return err
	})
}

// linkedTitles names outside rows from their links: the Library title,
// its kind and the episodes the torrent holds. They stay outside rows.
func (a *API) linkedTitles(ctx context.Context, downloaderID int64, rows []DownloadRow) {
	db := a.s.Store.Read()
	rs, err := db.QueryContext(ctx, `SELECT l.hash, t.id, t.name, t.kind, t.year, l.covers FROM client_links l JOIN titles t ON t.id = l.title_id
		WHERE l.downloader_id = ? AND l.state = 'linked'`, downloaderID)
	if err != nil {
		return
	}
	defer rs.Close()
	type hit struct {
		id     int64
		name   string
		kind   string
		year   *int
		covers string
	}
	by := map[string]hit{}
	for rs.Next() {
		var h hit
		var hash string
		if rs.Scan(&hash, &h.id, &h.name, &h.kind, &h.year, &h.covers) == nil {
			by[hash] = h
		}
	}
	for i := range rows {
		r := &rows[i]
		if !r.Outside || r.TitleID != 0 {
			continue
		}
		if h, ok := by[strings.ToLower(r.Hash)]; ok {
			r.TitleID, r.Title, r.Kind, r.Year, r.Covers, r.Linked = h.id, h.name, h.kind, h.year, h.covers, true
		}
	}
}

// MatchView is one torrent in the Confirm match list.
type MatchView struct {
	ID           int64       `json:"id"`
	DownloaderID int64       `json:"downloader_id"`
	Hash         string      `json:"hash"`
	Name         string      `json:"name"`
	State        string      `json:"state"`
	TitleID      int64       `json:"title_id,omitempty"`
	Covers       string      `json:"covers,omitempty"`
	How          string      `json:"how,omitempty"`
	Candidates   []Candidate `json:"candidates"`
	MatchedAt    string      `json:"matched_at"`
}

func (a *API) clientMatchRoutes(m *http.ServeMux) {
	m.HandleFunc("GET /api/v1/downloads/matches", a.listMatches)
	m.HandleFunc("POST /api/v1/downloads/matches/run", a.runMatches)
	m.HandleFunc("POST /api/v1/downloads/matches/{id}/pick", a.pickMatch)
}

func (a *API) readMatches(ctx context.Context, where string, args ...any) ([]MatchView, error) {
	rs, err := a.s.Store.Read().QueryContext(ctx, `SELECT id, downloader_id, hash, name, state, COALESCE(title_id, 0), covers, how, candidates, matched_at
		FROM client_links `+where, args...)
	if err != nil {
		return nil, err
	}
	defer rs.Close()
	out := []MatchView{}
	for rs.Next() {
		var v MatchView
		var cj string
		if err := rs.Scan(&v.ID, &v.DownloaderID, &v.Hash, &v.Name, &v.State, &v.TitleID, &v.Covers, &v.How, &cj, &v.MatchedAt); err != nil {
			return nil, err
		}
		v.Candidates = []Candidate{}
		_ = json.Unmarshal([]byte(cj), &v.Candidates)
		out = append(out, v)
	}
	return out, rs.Err()
}

// listMatches is the Confirm match list, with how many torrents are in
// each state.
func (a *API) listMatches(w http.ResponseWriter, r *http.Request) {
	unsure, err := a.readMatches(r.Context(), "WHERE state = 'unsure' ORDER BY name")
	if err != nil {
		internal(w, err)
		return
	}
	counts := map[string]int{linkLinked: 0, linkUnsure: 0, linkNotMedia: 0, linkSkipped: 0}
	rs, err := a.s.Store.Read().QueryContext(r.Context(), "SELECT state, COUNT(*) FROM client_links GROUP BY state")
	if err != nil {
		internal(w, err)
		return
	}
	defer rs.Close()
	for rs.Next() {
		var st string
		var n int
		if rs.Scan(&st, &n) == nil {
			counts[st] = n
		}
	}
	a.match.mu.Lock()
	pending := len(a.match.queue)
	if a.match.running {
		pending++
	}
	a.match.mu.Unlock()
	writeJSON(w, http.StatusOK, map[string]any{"matches": unsure, "counts": counts, "pending": pending})
}

// runMatches (Match now) reads every torrent Downloader and matches the
// outside torrents with no link yet, failed lookups included.
func (a *API) runMatches(w http.ResponseWriter, r *http.Request) {
	a.match.mu.Lock()
	a.match.failed = map[clientKey]time.Time{}
	a.match.mu.Unlock()
	rows := a.clientRows(r.Context(), map[clientKey]bool{})
	n := 0
	for _, x := range rows {
		if x.Outside && x.TitleID == 0 {
			n++
		}
	}
	writeJSON(w, http.StatusAccepted, map[string]any{"ok": true, "checked": n})
}

const pickHelp = `send {"candidate": "<id from the list>"}, {"not_media": true} or {"skip": true}`

// pickMatch answers one Confirm match entry: a candidate (added and linked
// the same way as a sure match), not media, or skip.
func (a *API) pickMatch(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r)
	if !ok {
		return
	}
	var in struct {
		Candidate string `json:"candidate"`
		NotMedia  bool   `json:"not_media"`
		Skip      bool   `json:"skip"`
	}
	if !decode(w, r, &in, pickHelp) {
		return
	}
	vs, err := a.readMatches(r.Context(), "WHERE id = ?", id)
	if err != nil {
		internal(w, err)
		return
	}
	if len(vs) == 0 {
		fail(w, http.StatusNotFound, "no such match")
		return
	}
	v := vs[0]
	want := matchWant{downloader: v.DownloaderID, hash: v.Hash, name: v.Name}
	switch {
	case in.NotMedia:
		err = a.saveLink(r.Context(), want, linkNotMedia, 0, nil, "", nil, "by hand")
	case in.Skip:
		err = a.saveLink(r.Context(), want, linkSkipped, 0, nil, "", nil, "by hand")
	case in.Candidate != "":
		var c *Candidate
		for i := range v.Candidates {
			if v.Candidates[i].ID == in.Candidate {
				c = &v.Candidates[i]
			}
		}
		if c == nil {
			fail(w, http.StatusBadRequest, "that candidate is not in this torrent's list")
			return
		}
		g := guessOf(v.Name, "")
		if c.Kind == "show" && g.kind != "show" {
			g = guess{kind: "show", complete: true}
		}
		err = a.addAndLink(r.Context(), want, g, *c, nil, "by hand")
	default:
		fail(w, http.StatusBadRequest, pickHelp)
		return
	}
	if err != nil {
		libraryError(w, err)
		return
	}
	vs, err = a.readMatches(r.Context(), "WHERE id = ?", id)
	if err != nil || len(vs) == 0 {
		internal(w, err)
		return
	}
	writeJSON(w, http.StatusOK, vs[0])
}

// outsideHash says the hash is not tied to any HolyArrs download record in
// that Downloader.
func (a *API) outsideHash(ctx context.Context, downloaderID int64, hash string) bool {
	var n int
	_ = a.s.Store.Read().QueryRowContext(ctx, "SELECT COUNT(*) FROM downloads WHERE downloader_id = ? AND lower(external_id) = ?",
		downloaderID, strings.ToLower(hash)).Scan(&n)
	return n == 0
}
