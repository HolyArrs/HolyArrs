package discover

import (
	"context"
	"github.com/HolyArrs/holyarrs/app/backend/internal/core/database"
	"github.com/HolyArrs/holyarrs/app/backend/internal/requests"
	"net/http"
	"slices"
	"sort"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/HolyArrs/holyarrs/app/backend/internal/core/webserver"
	"github.com/HolyArrs/holyarrs/app/backend/internal/mediafiles"
	"github.com/HolyArrs/holyarrs/app/backend/internal/music"

	"github.com/HolyArrs/holyarrs/app/backend/internal/core/parts"
	"github.com/HolyArrs/holyarrs/app/backend/internal/library"
	"github.com/HolyArrs/holyarrs/app/backend/internal/metadata"
)

// Live search (music plan 22 October 2026, Phase 4): the search field's dropdown.
// Typing never reaches MusicBrainz: the answer comes from the Library and
// the metadata server's suggest index, with TMDB movies, shows and people as
// the existing search finds them. Each outside source has suggestWait; a
// slow one loses its slots. MusicBrainz is used only by the full results page.
const (
	suggestWait     = 700 * time.Millisecond
	suggestRows     = 8
	suggestPerKind  = 3
	suggestWords    = 3
	suggestFromEach = 5
)

// SuggestRows, SuggestPerKind and SuggestWords are the dropdown's limits.
const (
	SuggestRows    = suggestRows
	SuggestPerKind = suggestPerKind
	SuggestWords   = suggestWords
)

// SuggestRow is one rich row of the dropdown.
type SuggestRow struct {
	Kind     string `json:"kind"` // movie, show, person, artist, album, song
	Name     string `json:"name"`
	Sub      string `json:"sub,omitempty"`
	Art      string `json:"art,omitempty"`
	State    string `json:"state,omitempty"` // have, missing, asked
	TitleID  int64  `json:"title_id,omitempty"`
	TMDB     string `json:"tmdb,omitempty"`
	PersonID string `json:"person_id,omitempty"`
	MBID     string `json:"mbid,omitempty"`
	Artist   string `json:"artist_mbid,omitempty"`
	score    float64
}

// suggestText is how well name matches q: 1 starts with it, 0.8 every word
// starts a word, 0.5 contains it, 0.3 the source matched it another way.
func suggestText(name, q string) float64 {
	n, f := mediafiles.Fold(name), mediafiles.Fold(q)
	switch {
	case strings.HasPrefix(n, f):
		return 1
	case mediafiles.FoldMatch(name, mediafiles.FoldWords(q)):
		return 0.8
	case strings.Contains(n, f):
		return 0.5
	}
	return 0.3
}

// suggestScore is the plan's ranking: 0.6·text + 0.25·P + 0.15·[have, missing or asked].
func suggestScore(text, p float64, state string) float64 {
	s := 0.6*text + 0.25*p
	if state != "" {
		s += 0.15
	}
	return s
}

// pickSuggest orders rows by score (then name) and keeps at most
// suggestPerKind of a kind and suggestRows in all. Completions are up to
// suggestWords distinct names that start with q, best first.
func pickSuggest(rows []SuggestRow, q string) ([]string, []SuggestRow) {
	sort.SliceStable(rows, func(i, j int) bool {
		if rows[i].score != rows[j].score {
			return rows[i].score > rows[j].score
		}
		return mediafiles.Fold(rows[i].Name) < mediafiles.Fold(rows[j].Name)
	})
	words := []string{}
	seen := map[string]bool{}
	f := mediafiles.Fold(q)
	for _, r := range rows {
		n := mediafiles.Fold(r.Name)
		if len(words) < suggestWords && strings.HasPrefix(n, f) && !seen[n] {
			seen[n] = true
			words = append(words, strings.ToLower(r.Name))
		}
	}
	out := []SuggestRow{}
	per := map[string]int{}
	dup := map[string]bool{}
	for _, r := range rows {
		k := r.Kind + "|" + mediafiles.Fold(r.Name) + "|" + mediafiles.Fold(r.Sub)
		if len(out) == suggestRows || per[r.Kind] == suggestPerKind || dup[k] {
			continue
		}
		dup[k] = true
		per[r.Kind]++
		out = append(out, r)
	}
	return words, out
}

// Suggest serves the live search dropdown. Its funcs are read on each
// request, so a test may change what they return.
type Suggest struct {
	MusicSuggest  func() func(ctx context.Context, q string) ([]metadata.MusicSuggestion, error)
	Metadata      func() metadata.Provider
	Store         func() *database.Store
	SafeMode      func() bool
	Music         func() SuggestMusic
	ShowSource    func(name string) (metadata.ShowProvider, error)
	PartOn        func(parts.Part) bool
	KidMax        func(*http.Request) string
	UserID        func(*http.Request) int64
	TitleAllowed  func(*http.Request, mediafiles.Title) bool
	TitleView     func(ctx context.Context, t mediafiles.Title) (TitleView, error)
	LibraryPeople func(r *http.Request, words []string, visible func(mediafiles.Title) bool) ([]mediafiles.LibraryPerson, error)
	OpenAsk       func(ctx context.Context, tmdbID string, me int64) (*requests.AskBrief, error)
	OpenShowAsk   func(ctx context.Context, ids map[string]string, me int64) (*requests.AskBrief, error)
}

// SuggestMusic is what live search asks the music module.
type SuggestMusic interface {
	MusicOn() bool
	RememberNames([]music.NameKey)
	WarmNames([]music.NameKey)
	LibrarySnapshot(ctx context.Context, now time.Time) (*music.LibrarySnapshot, error)
	AskStatesOf(ctx context.Context, member int64) (*music.AskStates, error)
}

// Routes registers GET /api/v1/search/suggest.
func (h *Suggest) Routes(m *http.ServeMux) {
	m.HandleFunc("GET /api/v1/search/suggest", h.searchSuggest)
}

// searchSuggest answers GET /api/v1/search/suggest?q=&kind=all|movie|show|music|people.
func (h *Suggest) searchSuggest(w http.ResponseWriter, r *http.Request) {
	q := strings.TrimSpace(r.URL.Query().Get("q"))
	kind := r.URL.Query().Get("kind")
	if kind == "" {
		kind = "all"
	}
	if !slices.Contains([]string{"all", "movie", "show", "music", "people"}, kind) || len(q) > 100 {
		webserver.Fail(w, http.StatusBadRequest, "send ?q=<2 to 100 characters>&kind=all, movie, show, music or people")
		return
	}
	w.Header().Set("Cache-Control", "no-store")
	if utf8.RuneCountInString(q) < 2 {
		webserver.WriteJSON(w, http.StatusOK, map[string]any{"q": q, "completions": []string{}, "items": []SuggestRow{}})
		return
	}
	want := func(k string) bool { return kind == "all" || kind == k }
	on := func(k string) bool {
		p, ok := parts.ForTitleKind(k)
		return !ok || h.PartOn(p) || h.SafeMode()
	}
	kid := h.KidMax(r) != ""
	// Music has no ratings, so a Kid limit shows none of it (as Explore does).
	withMusic := want("music") && h.Music().MusicOn() && !kid
	ctx, cancel := context.WithTimeout(r.Context(), suggestWait)
	defer cancel()
	results := make(chan []SuggestRow, 4)
	jobs := 0
	run := func(f func(context.Context) []SuggestRow) {
		jobs++
		go func() { results <- f(ctx) }()
	}
	if withMusic && h.MusicSuggest() != nil {
		run(func(ctx context.Context) []SuggestRow { return h.suggestMusicServer(ctx, r, q) })
	}
	// TMDB is the existing search's source. A Kid's lookups stay on the full
	// results page, where their rating checks and lookup limit run.
	if !kid && h.Metadata() != nil {
		if want("movie") && on("movie") {
			run(func(ctx context.Context) []SuggestRow { return h.suggestMovies(ctx, r, q) })
		}
		if want("show") && on("show") {
			run(func(ctx context.Context) []SuggestRow { return h.suggestShows(ctx, r, q) })
		}
		if want("people") {
			run(func(ctx context.Context) []SuggestRow { return h.suggestPeople(ctx, q) })
		}
	}
	rows := h.suggestLocal(r, q, kind, withMusic)
wait:
	for i := 0; i < jobs; i++ {
		select {
		case got := <-results:
			rows = append(rows, got...)
		case <-ctx.Done():
			break wait
		}
	}
	words, items := pickSuggest(mergeSuggest(rows), q)
	h.CoverAlbums(items)
	webserver.WriteJSON(w, http.StatusOK, map[string]any{"q": q, "completions": words, "items": items})
}

// coverSuggestAlbums points every album row without art at the music
// picture route, registers it there with its names and prefetches it, so a
// missing album gets a cover from the same chain as Explore's rows.
func (h *Suggest) CoverAlbums(items []SuggestRow) {
	var names []music.NameKey
	for i := range items {
		r := &items[i]
		if r.Kind != "album" || r.Art != "" || r.MBID == "" {
			continue
		}
		r.Art = music.ArtPrefix + "album/" + r.MBID
		names = append(names, music.NewNameKey("album", r.MBID, r.Name, r.Sub))
	}
	if len(names) > 0 {
		h.Music().RememberNames(names)
		h.Music().WarmNames(names)
	}
}

// mergeSuggest drops an outside row that a Library row already shows (same
// title, or the same artist or album by name).
func mergeSuggest(rows []SuggestRow) []SuggestRow {
	named := map[string]bool{}
	for _, r := range rows {
		if r.TitleID > 0 && (r.Kind == "artist" || r.Kind == "album") {
			named[r.Kind+"|"+mediafiles.Fold(r.Name)+"|"+mediafiles.Fold(r.Sub)] = true
		}
	}
	out := []SuggestRow{}
	seen := map[string]bool{}
	for _, r := range rows {
		k := r.Kind + "|" + mediafiles.Fold(r.Name) + "|" + mediafiles.Fold(r.Sub)
		if r.Kind == "movie" || r.Kind == "show" {
			if r.TitleID > 0 {
				k = r.Kind + "#" + strconv.FormatInt(r.TitleID, 10)
			} else {
				k = r.Kind + "@" + r.TMDB
			}
		} else if r.TitleID == 0 && named[k] {
			continue
		}
		if seen[k] {
			continue
		}
		seen[k] = true
		out = append(out, r)
	}
	return out
}

// suggestLocal reads the Library: titles (Kid limits and parts apply),
// album parts of Library artists, people credited in Library titles.
func (h *Suggest) suggestLocal(r *http.Request, q, kind string, withMusic bool) []SuggestRow {
	ctx := WithLookupMemo(r.Context())
	out := []SuggestRow{}
	visible := func(t mediafiles.Title) bool {
		if p, ok := parts.ForTitleKind(t.Kind); ok && !h.PartOn(p) && !h.SafeMode() {
			return false
		}
		return t.State != "removed" && h.TitleAllowed(r, t)
	}
	scopeOf := map[string]string{"movie": "movie", "show": "show", "artist": "music"}
	if kind != "people" {
		if ts, err := mediafiles.SearchTitles(ctx, h.Store(), q); err == nil {
			n := 0
			for _, t := range ts {
				if n == 3*suggestFromEach {
					break
				}
				sc, ok := scopeOf[t.Kind]
				if !ok || !visible(t) || (t.Kind == "artist" && !withMusic) || (kind != "all" && sc != kind) {
					continue
				}
				v, err := h.TitleView(ctx, t)
				if err != nil {
					continue
				}
				n++
				row := SuggestRow{Kind: t.Kind, Name: t.Name, TitleID: t.ID, State: "have"}
				if len(v.Artwork) > 0 {
					row.Art = v.Artwork[0]
				}
				if t.Kind == "artist" {
					row.Sub = "Artist"
					row.MBID = strings.ToLower(t.SourceIDs["musicbrainz"])
				} else {
					row.Sub = map[string]string{"movie": "Movie", "show": "Show"}[t.Kind]
					if t.Year != nil && *t.Year > 0 {
						row.Sub += " · " + strconv.Itoa(*t.Year)
					}
					row.TMDB = t.SourceIDs["tmdb"]
				}
				row.score = suggestScore(suggestText(t.Name, q), 0.5, row.State)
				out = append(out, row)
			}
		}
	}
	words := mediafiles.FoldWords(q)
	if len(words) == 0 {
		return out
	}
	if withMusic {
		if lib, err := h.Music().LibrarySnapshot(ctx, time.Now()); err == nil && lib != nil {
			names := map[int64]music.LibArtist{}
			for _, ar := range lib.Artists {
				names[ar.ID] = ar
			}
			n := 0
			for _, al := range lib.Albums {
				if n == suggestFromEach {
					break
				}
				ar, ok := names[al.TitleID]
				if !ok || !mediafiles.FoldMatch(al.Name, words) {
					continue
				}
				n++
				row := SuggestRow{Kind: "album", Name: al.Name, Sub: ar.Name, TitleID: al.TitleID, MBID: al.MBID, Artist: ar.MBID, State: "missing"}
				if lib.HadParts[al.PartID] {
					row.State = "have"
				}
				row.score = suggestScore(suggestText(al.Name, q), 0.5, row.State)
				out = append(out, row)
			}
		}
	}
	if kind == "all" || kind == "people" {
		if ps, err := h.LibraryPeople(r, words, visible); err == nil {
			for i, p := range ps {
				if i == suggestFromEach {
					break
				}
				row := SuggestRow{Kind: "person", Name: p.Name, Sub: "Person"}
				if len(p.Titles) > 0 {
					row.Sub += " · " + p.Titles[0].Name
					row.TitleID = p.Titles[0].ID
				}
				row.score = suggestScore(suggestText(p.Name, q), 0.4, "")
				out = append(out, row)
			}
		}
	}
	return out
}

// suggestMusicServer reads the server's suggest index and marks what the
// Library has (by name) and what the household asked for (by id).
func (h *Suggest) suggestMusicServer(ctx context.Context, r *http.Request, q string) []SuggestRow {
	items, err := h.MusicSuggest()(ctx, q)
	if err != nil {
		return nil
	}
	lib, _ := h.Music().LibrarySnapshot(ctx, time.Now())
	acts, _ := h.Music().AskStatesOf(ctx, h.UserID(r))
	artists := map[string]music.LibArtist{}
	albums := map[string]music.LibAlbum{}
	if lib != nil {
		byID := map[int64]string{}
		for _, ar := range lib.Artists {
			artists[mediafiles.Fold(ar.Name)] = ar
			byID[ar.ID] = ar.Name
		}
		for _, al := range lib.Albums {
			albums[mediafiles.Fold(al.Name)+"|"+mediafiles.Fold(byID[al.TitleID])] = al
		}
	}
	out := []SuggestRow{}
	for _, it := range items {
		row := SuggestRow{Kind: it.Kind, Name: it.Name, Art: it.Cover, MBID: strings.ToLower(it.MBID), Artist: strings.ToLower(it.ArtistMBID)}
		switch it.Kind {
		case "artist":
			row.Sub = "Artist"
			if ar, ok := artists[mediafiles.Fold(it.Name)]; ok {
				row.TitleID, row.MBID, row.State = ar.ID, ar.MBID, "missing"
				if ar.HadAny {
					row.State = "have"
				}
				if ar.Image != "" {
					row.Art = ar.Image
				}
			} else if acts != nil && row.MBID != "" && acts.Artists[row.MBID] != "" {
				row.State = "asked"
			}
		case "album":
			row.Sub = it.Artist
			if al, ok := albums[mediafiles.Fold(it.Name)+"|"+mediafiles.Fold(it.Artist)]; ok {
				row.TitleID, row.State = al.TitleID, "missing"
				if lib.HadParts[al.PartID] {
					row.State = "have"
				}
				if row.MBID == "" {
					row.MBID = al.MBID
				}
			} else if acts != nil && row.MBID != "" && acts.Albums[row.MBID] != "" {
				row.State = "asked"
			}
		case "song":
			row.Sub = it.Artist
		default:
			continue
		}
		row.score = suggestScore(suggestText(it.Name, q), it.P, row.State)
		out = append(out, row)
	}
	return out
}

// rankP is a source's own order as a 0–1 popularity: the first 1, then down.
func rankP(i, n int) float64 {
	if n <= 1 {
		return 1
	}
	return 1 - float64(i)/float64(n)
}

func (h *Suggest) suggestMovies(ctx context.Context, r *http.Request, q string) []SuggestRow {
	ms, err := h.Metadata().Search(ctx, q, 0)
	if err != nil {
		return nil
	}
	if len(ms) > suggestFromEach {
		ms = ms[:suggestFromEach]
	}
	db := h.Store().Read()
	rules, _ := hideRulesFor(ctx, h.Store())
	out := []SuggestRow{}
	for i, m := range ms {
		id := m.IDs["tmdb"]
		if id == "" || rules.hides("movie", id, m.Genres) {
			continue
		}
		row := SuggestRow{Kind: "movie", Name: m.Name, Sub: "Movie", Art: m.Poster, TMDB: id}
		if m.Year > 0 {
			row.Sub += " · " + strconv.Itoa(m.Year)
		}
		if tid, err := library.TitleByTMDB(ctx, db, id); err == nil && tid > 0 {
			row.TitleID, row.State = tid, "have"
		} else if ask, err := h.OpenAsk(ctx, id, h.UserID(r)); err == nil && ask != nil {
			row.State = "asked"
		}
		row.score = suggestScore(suggestText(m.Name, q), rankP(i, len(ms)), row.State)
		out = append(out, row)
	}
	return out
}

func (h *Suggest) suggestShows(ctx context.Context, r *http.Request, q string) []SuggestRow {
	d, err := library.ReadShowDefaults(ctx, h.Store().Read())
	if err != nil {
		return nil
	}
	p, err := h.ShowSource(d.Source)
	if err != nil {
		return nil
	}
	shows, err := p.SearchShows(ctx, q)
	if err != nil {
		return nil
	}
	if len(shows) > suggestFromEach {
		shows = shows[:suggestFromEach]
	}
	db := h.Store().Read()
	rules, _ := hideRulesFor(ctx, h.Store())
	out := []SuggestRow{}
	for i, sh := range shows {
		if id := sh.IDs["tmdb"]; id != "" && rules.hides("show", id, sh.Genres) {
			continue
		}
		row := SuggestRow{Kind: "show", Name: sh.Name, Sub: "Show", Art: sh.Poster, TMDB: sh.IDs["tmdb"]}
		if sh.Year > 0 {
			row.Sub += " · " + strconv.Itoa(sh.Year)
		}
		if tid, err := library.ShowByIDs(ctx, db, sh.IDs); err == nil && tid > 0 {
			row.TitleID, row.State = tid, "have"
		} else if ask, err := h.OpenShowAsk(ctx, sh.IDs, h.UserID(r)); err == nil && ask != nil {
			row.State = "asked"
		}
		if row.TitleID == 0 && row.TMDB == "" {
			continue
		}
		row.score = suggestScore(suggestText(sh.Name, q), rankP(i, len(shows)), row.State)
		out = append(out, row)
	}
	return out
}

func (h *Suggest) suggestPeople(ctx context.Context, q string) []SuggestRow {
	p, ok := metadata.PeopleProviderFor(ctx, h.Metadata())
	if !ok {
		return nil
	}
	res, err := p.SearchPeople(ctx, q, 1)
	if err != nil {
		return nil
	}
	ps := res.People
	if len(ps) > suggestFromEach {
		ps = ps[:suggestFromEach]
	}
	out := []SuggestRow{}
	for i, pe := range ps {
		if pe.Adult {
			continue
		}
		row := SuggestRow{Kind: "person", Name: pe.Name, Sub: "Person", Art: pe.Profile, PersonID: pe.ID}
		if pe.Department != "" {
			row.Sub += " · " + pe.Department
		}
		row.score = suggestScore(suggestText(pe.Name, q), rankP(i, len(ps)), "")
		out = append(out, row)
	}
	return out
}
