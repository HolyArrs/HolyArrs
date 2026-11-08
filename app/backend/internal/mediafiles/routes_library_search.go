package mediafiles

import (
	"errors"
	"net/http"
	"slices"
	"strings"
	"unicode"

	"golang.org/x/text/unicode/norm"

	"github.com/HolyArrs/holyarrs/app/backend/internal/core/database"
	"github.com/HolyArrs/holyarrs/app/backend/internal/core/webserver"
)

// musicArtPrefix is the music picture route (music module).
const musicArtPrefix = "/api/v1/music-trending/art/"

// AlbumName is a matched album handed to the music picture warmer.
type AlbumName struct{ MBID, Name, Artist string }

// librarySearchMax bounds each list of a Library search: Search shows one row.
const librarySearchMax = 24

// LibraryAlbum is an album of a Library artist that a search matched.
type LibraryAlbum struct {
	ID      int64  `json:"id"`
	TitleID int64  `json:"title_id"`
	Name    string `json:"name"`
	Artist  string `json:"artist"`
	Year    int    `json:"year,omitempty"`
	// Image is the album's cover through the music picture route.
	Image string `json:"image,omitempty"`
}

// LibraryPerson is a person credited in Library titles that a search matched.
type LibraryPerson struct {
	Name   string            `json:"name"`
	Titles []LibraryPersonIn `json:"titles"`
}

// LibraryPersonIn is one Library title a person is credited in.
type LibraryPersonIn struct {
	ID   int64  `json:"id"`
	Name string `json:"name"`
	Kind string `json:"kind"`
}

// librarySearch is Search's "In your Library" row: ?q=&kind=all|movie|show|music|people.
// It reads only the local database: titles by name, original name, aliases and
// year (accents and case folded), albums of Library artists and people
// credited in Library titles. Kid limits and parts that are off hide titles.
func (h *Handlers) librarySearch(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	term := strings.TrimSpace(q.Get("q"))
	kind := q.Get("kind")
	if kind == "" {
		kind = "all"
	}
	if !slices.Contains([]string{"all", "movie", "show", "music", "people"}, kind) {
		webserver.Fail(w, http.StatusBadRequest, "kind is all, movie, show, music or people")
		return
	}
	out := map[string]any{"titles": []any{}, "albums": []LibraryAlbum{}, "people": []LibraryPerson{}}
	if term == "" {
		webserver.WriteJSON(w, http.StatusOK, out)
		return
	}
	visible := func(t Title) bool {
		if h.KindShown != nil && !h.KindShown(t.Kind) {
			return false
		}
		return t.State != "removed" && h.TitleAllowed(r, t)
	}
	ctx := r.Context()
	if h.TitleBatch != nil {
		ctx = h.TitleBatch(ctx)
	}
	if kind != "people" {
		ts, err := SearchTitles(ctx, h.Store, term)
		if errors.Is(err, ErrTitleSearchQuery) {
			webserver.Fail(w, http.StatusBadRequest, err.Error())
			return
		}
		if err != nil {
			webserver.Internal(w, "api", err)
			return
		}
		want := map[string]string{"movie": "movie", "show": "show", "music": "artist"}[kind]
		views := []any{}
		for _, t := range ts {
			if len(views) == librarySearchMax {
				break
			}
			if (want != "" && t.Kind != want) || !visible(t) {
				continue
			}
			v, err := h.TitleView(ctx, t)
			if err != nil {
				webserver.Internal(w, "api", err)
				return
			}
			views = append(views, v)
		}
		out["titles"] = views
	}
	words := FoldWords(term)
	if len(words) == 0 {
		webserver.WriteJSON(w, http.StatusOK, out)
		return
	}
	if kind == "all" || kind == "music" {
		albums, err := h.libraryAlbums(r, words, visible)
		if err != nil {
			webserver.Internal(w, "api", err)
			return
		}
		out["albums"] = albums
	}
	if kind == "all" || kind == "people" {
		people, err := h.LibraryPeople(r, words, visible)
		if err != nil {
			webserver.Internal(w, "api", err)
			return
		}
		out["people"] = people
	}
	webserver.WriteJSON(w, http.StatusOK, out)
}

func (h *Handlers) libraryAlbums(r *http.Request, words []string, visible func(Title) bool) ([]LibraryAlbum, error) {
	pre, args := FoldPrefilter("name", words)
	ps, err := database.Select[Part](r.Context(), h.Store.Read(), "parts", "kind = 'album' AND dropped_at IS NULL"+pre+" ORDER BY name, id", args...)
	if err != nil {
		return nil, err
	}
	artists := map[int64]*Title{}
	out := []LibraryAlbum{}
	var names []AlbumName
	for _, p := range ps {
		if len(out) == librarySearchMax {
			break
		}
		if !FoldMatch(p.Name, words) {
			continue
		}
		t, seen := artists[p.TitleID]
		if !seen {
			var x Title
			if err := database.Get(r.Context(), h.Store.Read(), "titles", p.TitleID, &x); err == nil && visible(x) {
				t = &x
			}
			artists[p.TitleID] = t
		}
		if t == nil {
			continue
		}
		al := LibraryAlbum{ID: p.ID, TitleID: p.TitleID, Name: p.Name, Artist: t.Name}
		if p.ReleaseDate != nil {
			al.Year = p.ReleaseDate.Year()
		}
		if rg := strings.ToLower(p.SourceIDs["musicbrainz"]); rg != "" {
			al.Image = musicArtPrefix + "album/" + rg
			names = append(names, AlbumName{MBID: rg, Name: p.Name, Artist: t.Name})
		}
		out = append(out, al)
	}
	// The covers come from the same chain and prefetch as Explore's rows.
	if h.AlbumNames != nil {
		h.AlbumNames(names)
	}
	return out, nil
}

// LibraryPeople are the people credited in visible Library titles whose
// names hold every folded word.
func (h *Handlers) LibraryPeople(r *http.Request, words []string, visible func(Title) bool) ([]LibraryPerson, error) {
	pre, args := FoldPrefilter("people", words)
	ts, err := database.Select[Title](r.Context(), h.Store.Read(), "titles", "state != 'removed' AND people != '[]'"+pre+" ORDER BY sort_name, id", args...)
	if err != nil {
		return nil, err
	}
	index := map[string]int{}
	out := []LibraryPerson{}
	for _, t := range ts {
		if !visible(t) {
			continue
		}
		for _, name := range t.People {
			if !FoldMatch(name, words) {
				continue
			}
			i, ok := index[name]
			if !ok {
				if len(out) == librarySearchMax {
					continue
				}
				i = len(out)
				index[name] = i
				out = append(out, LibraryPerson{Name: name, Titles: []LibraryPersonIn{}})
			}
			out[i].Titles = append(out[i].Titles, LibraryPersonIn{ID: t.ID, Name: t.Name, Kind: t.Kind})
		}
	}
	return out, nil
}

// Fold lowercases s and drops its accents ("Amélie" → "amelie").
func Fold(s string) string {
	var b strings.Builder
	for _, c := range norm.NFD.String(s) {
		if !unicode.Is(unicode.Mn, c) {
			b.WriteRune(unicode.ToLower(c))
		}
	}
	return b.String()
}

// FoldWords are the folded words of a query.
func FoldWords(s string) []string {
	return strings.FieldsFunc(Fold(s), func(c rune) bool { return !unicode.IsLetter(c) && !unicode.IsNumber(c) })
}

// FoldPrefilter narrows a scan in SQL without changing what foldMatch
// keeps: a row whose column is plain ASCII must hold every ASCII query word
// (a word prefix is a substring of the lowercased text); rows with other
// characters (accents) are left to foldMatch.
func FoldPrefilter(col string, words []string) (string, []any) {
	var b strings.Builder
	var args []any
	for _, w := range words {
		if strings.IndexFunc(w, func(c rune) bool { return !(c >= 'a' && c <= 'z' || c >= '0' && c <= '9') }) >= 0 {
			continue
		}
		b.WriteString(" AND instr(lower(" + col + "), ?) > 0")
		args = append(args, w)
	}
	if b.Len() == 0 {
		return "", nil
	}
	return " AND (" + col + " GLOB '*[^ -~]*' OR (1 = 1" + b.String() + "))", args
}

// FoldMatch: every query word starts a word of s, accents and case folded.
func FoldMatch(s string, words []string) bool {
	have := FoldWords(s)
	for _, w := range words {
		if !slices.ContainsFunc(have, func(h string) bool { return strings.HasPrefix(h, w) }) {
			return false
		}
	}
	return true
}
