package api

import (
	"context"
	"net/http"
	"slices"
	"sort"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/HolyArrs/holyarrs/app/backend/internal/core/webserver"
	"github.com/HolyArrs/holyarrs/app/backend/internal/mediafiles"

	"github.com/HolyArrs/holyarrs/app/backend/internal/library"
	"github.com/HolyArrs/holyarrs/app/backend/internal/metadata"
	"github.com/HolyArrs/holyarrs/app/backend/internal/parts"
)

// Live search (music plan 2026-10-22, Phase 4): the search field's dropdown.
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
	n, f := fold(name), fold(q)
	switch {
	case strings.HasPrefix(n, f):
		return 1
	case foldMatch(name, foldWords(q)):
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
		return fold(rows[i].Name) < fold(rows[j].Name)
	})
	words := []string{}
	seen := map[string]bool{}
	f := fold(q)
	for _, r := range rows {
		n := fold(r.Name)
		if len(words) < suggestWords && strings.HasPrefix(n, f) && !seen[n] {
			seen[n] = true
			words = append(words, strings.ToLower(r.Name))
		}
	}
	out := []SuggestRow{}
	per := map[string]int{}
	dup := map[string]bool{}
	for _, r := range rows {
		k := r.Kind + "|" + fold(r.Name) + "|" + fold(r.Sub)
		if len(out) == suggestRows || per[r.Kind] == suggestPerKind || dup[k] {
			continue
		}
		dup[k] = true
		per[r.Kind]++
		out = append(out, r)
	}
	return words, out
}

// searchSuggest answers GET /api/v1/search/suggest?q=&kind=all|movie|show|music|people.
func (a *API) searchSuggest(w http.ResponseWriter, r *http.Request) {
	if !a.s.LiveSearch {
		webserver.Fail(w, http.StatusNotFound, "live search is off")
		return
	}
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
		return !ok || a.partIsOn(p) || a.s.Parts.SafeMode
	}
	kid := a.kidMax(r) != ""
	// Music has no ratings, so a Kid limit shows none of it (as Explore does).
	mp := a.musicProvider()
	music := want("music") && mp.On() && !kid
	ctx, cancel := context.WithTimeout(r.Context(), suggestWait)
	defer cancel()
	results := make(chan []SuggestRow, 4)
	jobs := 0
	run := func(f func(context.Context) []SuggestRow) {
		jobs++
		go func() { results <- f(ctx) }()
	}
	if music && a.s.MusicSuggest != nil {
		run(func(ctx context.Context) []SuggestRow { return mp.Suggest(ctx, r, q) })
	}
	// TMDB is the existing search's source. A Kid's lookups stay on the full
	// results page, where their rating checks and lookup limit run.
	if !kid && a.s.Metadata != nil {
		if want("movie") && on("movie") {
			run(func(ctx context.Context) []SuggestRow { return a.suggestMovies(ctx, r, q) })
		}
		if want("show") && on("show") {
			run(func(ctx context.Context) []SuggestRow { return a.suggestShows(ctx, r, q) })
		}
		if want("people") {
			run(func(ctx context.Context) []SuggestRow { return a.suggestPeople(ctx, q) })
		}
	}
	rows := a.suggestLocal(r, q, kind, music)
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
	a.coverSuggestAlbums(items)
	webserver.WriteJSON(w, http.StatusOK, map[string]any{"q": q, "completions": words, "items": items})
}

// coverSuggestAlbums points every album row without art at the music
// picture route, registers it there with its names and prefetches it, so a
// missing album gets a cover from the same chain as Explore's rows.
func (a *API) coverSuggestAlbums(items []SuggestRow) {
	var names []nameKey
	for i := range items {
		r := &items[i]
		if r.Kind != "album" || r.Art != "" || r.MBID == "" {
			continue
		}
		r.Art = musicArtPrefix + "album/" + r.MBID
		names = append(names, nameKey{kind: "album", mbid: r.MBID, name: r.Name, artist: r.Sub})
	}
	if len(names) > 0 {
		a.rememberMusicNames(names)
		a.warmMusicNames(names)
	}
}

// mergeSuggest drops an outside row that a Library row already shows (same
// title, or the same artist or album by name).
func mergeSuggest(rows []SuggestRow) []SuggestRow {
	named := map[string]bool{}
	for _, r := range rows {
		if r.TitleID > 0 && (r.Kind == "artist" || r.Kind == "album") {
			named[r.Kind+"|"+fold(r.Name)+"|"+fold(r.Sub)] = true
		}
	}
	out := []SuggestRow{}
	seen := map[string]bool{}
	for _, r := range rows {
		k := r.Kind + "|" + fold(r.Name) + "|" + fold(r.Sub)
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
func (a *API) suggestLocal(r *http.Request, q, kind string, music bool) []SuggestRow {
	ctx := withLookupMemo(r.Context())
	out := []SuggestRow{}
	visible := func(t mediafiles.Title) bool {
		if p, ok := parts.ForTitleKind(t.Kind); ok && !a.partIsOn(p) && !a.s.Parts.SafeMode {
			return false
		}
		return t.State != "removed" && a.titleAllowed(r, t)
	}
	scopeOf := map[string]string{"movie": "movie", "show": "show", "artist": "music"}
	if kind != "people" {
		if ts, err := mediafiles.SearchTitles(ctx, a.s.Store, q); err == nil {
			n := 0
			for _, t := range ts {
				if n == 3*suggestFromEach {
					break
				}
				sc, ok := scopeOf[t.Kind]
				if !ok || !visible(t) || (t.Kind == "artist" && !music) || (kind != "all" && sc != kind) {
					continue
				}
				v, err := a.titleView(ctx, t)
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
	words := foldWords(q)
	if len(words) == 0 {
		return out
	}
	if music {
		if lib, err := a.musicLibrary(ctx, time.Now()); err == nil {
			names := map[int64]musicLibArtist{}
			for _, ar := range lib.artists {
				names[ar.id] = ar
			}
			n := 0
			for _, al := range lib.albums {
				if n == suggestFromEach {
					break
				}
				ar, ok := names[al.titleID]
				if !ok || !foldMatch(al.name, words) {
					continue
				}
				n++
				row := SuggestRow{Kind: "album", Name: al.name, Sub: ar.name, TitleID: al.titleID, MBID: al.mbid, Artist: ar.mbid, State: "missing"}
				if lib.hadParts[al.partID] {
					row.State = "have"
				}
				row.score = suggestScore(suggestText(al.name, q), 0.5, row.State)
				out = append(out, row)
			}
		}
	}
	if kind == "all" || kind == "people" {
		if ps, err := a.libraryPeople(r, words, visible); err == nil {
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
func (a *API) suggestMusicServer(ctx context.Context, r *http.Request, q string) []SuggestRow {
	items, err := a.s.MusicSuggest(ctx, q)
	if err != nil {
		return nil
	}
	lib, _ := a.musicLibrary(ctx, time.Now())
	acts, _ := a.musicActsOf(ctx, a.person(r).UserID)
	artists := map[string]musicLibArtist{}
	albums := map[string]musicLibAlbum{}
	if lib != nil {
		byID := map[int64]string{}
		for _, ar := range lib.artists {
			artists[fold(ar.name)] = ar
			byID[ar.id] = ar.name
		}
		for _, al := range lib.albums {
			albums[fold(al.name)+"|"+fold(byID[al.titleID])] = al
		}
	}
	out := []SuggestRow{}
	for _, it := range items {
		row := SuggestRow{Kind: it.Kind, Name: it.Name, Art: it.Cover, MBID: strings.ToLower(it.MBID), Artist: strings.ToLower(it.ArtistMBID)}
		switch it.Kind {
		case "artist":
			row.Sub = "Artist"
			if ar, ok := artists[fold(it.Name)]; ok {
				row.TitleID, row.MBID, row.State = ar.id, ar.mbid, "missing"
				if ar.hadAny {
					row.State = "have"
				}
				if ar.image != "" {
					row.Art = ar.image
				}
			} else if acts != nil && row.MBID != "" && acts.artistState[row.MBID] != "" {
				row.State = "asked"
			}
		case "album":
			row.Sub = it.Artist
			if al, ok := albums[fold(it.Name)+"|"+fold(it.Artist)]; ok {
				row.TitleID, row.State = al.titleID, "missing"
				if lib.hadParts[al.partID] {
					row.State = "have"
				}
				if row.MBID == "" {
					row.MBID = al.mbid
				}
			} else if acts != nil && row.MBID != "" && acts.albumState[row.MBID] != "" {
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

func (a *API) suggestMovies(ctx context.Context, r *http.Request, q string) []SuggestRow {
	ms, err := a.s.Metadata.Search(ctx, q, 0)
	if err != nil {
		return nil
	}
	if len(ms) > suggestFromEach {
		ms = ms[:suggestFromEach]
	}
	db := a.s.Store.Read()
	out := []SuggestRow{}
	for i, m := range ms {
		id := m.IDs["tmdb"]
		if id == "" {
			continue
		}
		row := SuggestRow{Kind: "movie", Name: m.Name, Sub: "Movie", Art: m.Poster, TMDB: id}
		if m.Year > 0 {
			row.Sub += " · " + strconv.Itoa(m.Year)
		}
		if tid, err := library.TitleByTMDB(ctx, db, id); err == nil && tid > 0 {
			row.TitleID, row.State = tid, "have"
		} else if ask, err := a.openAsk(ctx, id, a.person(r).UserID); err == nil && ask != nil {
			row.State = "asked"
		}
		row.score = suggestScore(suggestText(m.Name, q), rankP(i, len(ms)), row.State)
		out = append(out, row)
	}
	return out
}

func (a *API) suggestShows(ctx context.Context, r *http.Request, q string) []SuggestRow {
	d, err := library.ReadShowDefaults(ctx, a.s.Store.Read())
	if err != nil {
		return nil
	}
	p, err := a.showSource(d.Source)
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
	db := a.s.Store.Read()
	out := []SuggestRow{}
	for i, sh := range shows {
		row := SuggestRow{Kind: "show", Name: sh.Name, Sub: "Show", Art: sh.Poster, TMDB: sh.IDs["tmdb"]}
		if sh.Year > 0 {
			row.Sub += " · " + strconv.Itoa(sh.Year)
		}
		if tid, err := library.ShowByIDs(ctx, db, sh.IDs); err == nil && tid > 0 {
			row.TitleID, row.State = tid, "have"
		} else if ask, err := a.openShowAsk(ctx, sh.IDs, a.person(r).UserID); err == nil && ask != nil {
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

func (a *API) suggestPeople(ctx context.Context, q string) []SuggestRow {
	p, ok := metadata.PeopleProviderFor(ctx, a.s.Metadata)
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
