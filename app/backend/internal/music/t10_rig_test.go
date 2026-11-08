package music

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/cookiejar"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/HolyArrs/holyarrs/app/backend/internal/accounts"
	"github.com/HolyArrs/holyarrs/app/backend/internal/core/database"
	"github.com/HolyArrs/holyarrs/app/backend/internal/core/jobs"
	domain "github.com/HolyArrs/holyarrs/app/backend/internal/core/model"
	"github.com/HolyArrs/holyarrs/app/backend/internal/core/secrets"
	"github.com/HolyArrs/holyarrs/app/backend/internal/core/webserver"
	"github.com/HolyArrs/holyarrs/app/backend/internal/library"
	"github.com/HolyArrs/holyarrs/app/backend/internal/metadata"
	"github.com/HolyArrs/holyarrs/app/backend/internal/shows"
)

// fakeShowSource answers fixed shows by "<kind>:<id>".
type fakeShowSource struct {
	mu    sync.Mutex
	shows map[string]metadata.Show
	err   error
}

func (f *fakeShowSource) SearchShows(_ context.Context, q string) ([]metadata.Show, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.err != nil {
		return nil, f.err
	}
	seen := map[string]bool{}
	out := []metadata.Show{}
	for _, sh := range f.shows {
		if strings.Contains(strings.ToLower(sh.Name), strings.ToLower(q)) && !seen[sh.Name] {
			seen[sh.Name] = true
			out = append(out, sh)
		}
	}
	return out, nil
}

func (f *fakeShowSource) Show(_ context.Context, id string) (metadata.Show, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.err != nil {
		return metadata.Show{}, f.err
	}
	if sh, ok := f.shows[id]; ok {
		return sh, nil
	}
	return metadata.Show{}, metadata.ErrShowNotFound
}

func at(s string) *time.Time { t, _ := time.Parse(time.RFC3339, s); return &t }

// apiShow is a show with a finished first season and a second one far in
// the future, so the test does not depend on today's date.
func apiShow(source, id string) metadata.Show {
	return metadata.Show{Source: source, Name: "Lab Show", Year: 2025, Status: "running", Network: "Lab TV",
		IDs:     map[string]string{source: id, "tvdb": "200", "imdb": "tt0100"},
		Seasons: []metadata.Season{{Number: 0, Name: "Specials"}, {Number: 1, Name: "Season 1"}, {Number: 2, Name: "Season 2"}},
		Episodes: []metadata.Episode{
			{Season: 0, Number: 1, Name: "Behind the Lab", AirDate: at("2025-03-01T12:00:00Z"), Timed: true, AirDay: "2025-03-01"},
			{Season: 1, Number: 1, Name: "Pilot", AirDate: at("2025-01-06T02:00:00Z"), Timed: true, AirDay: "2025-01-05", ID: "e101"},
			{Season: 1, Number: 2, Name: "Second", AirDate: at("2025-01-13T02:00:00Z"), Timed: true, AirDay: "2025-01-12", ID: "e102"},
			{Season: 2, Number: 1, Name: "Return", AirDate: at("2099-01-05T02:00:00Z"), Timed: true, AirDay: "2099-01-04", ID: "e201"},
			{Season: 2, Number: 2, Name: "TBA", ID: "e202"},
		}}
}

// t10Env is the music SQL pin's world: a signed-in owner and member over
// the music routes and the show add route, with a movie, a show and an
// artist in the Library.
type t10Env struct {
	t     *testing.T
	srv   *httptest.Server
	store *database.Store
	api   *API
	lib   *library.Service
	owner *signedIn
}

func (e *t10Env) call(c *signedIn, method, path, body string) (int, []byte) {
	e.t.Helper()
	req, _ := http.NewRequest(method, e.srv.URL+path, strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	if c.csrf != "" {
		req.Header.Set(accounts.CSRFHeader, c.csrf)
	}
	resp, err := c.c.Do(req)
	if err != nil {
		e.t.Fatal(err)
	}
	defer resp.Body.Close()
	b, _ := io.ReadAll(resp.Body)
	return resp.StatusCode, b
}

func t10Setup(t *testing.T) *t10Env {
	t.Helper()
	s, err := database.Open(t.TempDir(), database.Options{})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = s.Close() })
	v, err := secrets.Open(context.Background(), s, make([]byte, secrets.KeySize))
	if err != nil {
		t.Fatal(err)
	}
	q := jobs.New(s, jobs.Options{})
	tmdb := &fakeShowSource{shows: map[string]metadata.Show{"tmdb:100": apiShow("tmdb", "100"), "tvdb:200": apiShow("tmdb", "100")}}
	lib := &library.Service{Store: s, Queue: q, Artists: apiMusic(), Metadata: metadata.Lab(),
		Shows: metadata.ShowSources{"tmdb": tmdb, "tvdb": &fakeShowSource{err: metadata.ErrNoTVDBKey}}}
	a := New(Server{Store: s, Queue: q, Library: lib, Metadata: &metadata.Static{}, Version: "test",
		Auth: accounts.New(s, accounts.Options{Vault: v}), Live: webserver.NewLiveHub(webserver.LiveOptions{})})
	a.extra = func(m *http.ServeMux) {
		(&shows.AddHandlers{Store: s, Lib: lib, Fail: rigLibraryError,
			KidLimited: func(http.ResponseWriter, *http.Request) bool { return false },
			KidShows:   func(_ *http.Request, l []metadata.Show) []metadata.Show { return l },
			OpenAsk:    func(*http.Request, map[string]string) (any, error) { return nil, nil },
			Defaults:   func(context.Context, map[string]json.RawMessage, *shows.ShowAddOptions) error { return nil },
			Follow:     func(r *http.Request, id int64) error { return a.s.Asks.FollowAdded(r.Context(), a.Person(r), id) },
			Added:      a.TitleView}).Routes(m)
	}
	srv := httptest.NewServer(a.Handler())
	t.Cleanup(srv.Close)
	e := &t10Env{t: t, srv: srv, store: s, api: a, lib: lib}
	read := func(b []byte) (string, string) {
		var x struct {
			CSRF string `json:"csrf"`
			Path string `json:"path"`
		}
		_ = json.Unmarshal(b, &x)
		return x.CSRF, x.Path
	}
	browser := func() *signedIn {
		jar, _ := cookiejar.New(nil)
		return &signedIn{t: t, base: srv.URL, c: &http.Client{Jar: jar}}
	}
	e.owner = browser()
	e.owner.csrf, _ = read(e.owner.call("POST", "/api/v1/auth/setup", `{"name":"luis","password":"a long test passphrase"}`, 201))
	_, link := read(e.owner.call("POST", "/api/v1/users/invites", `{}`, 201))
	member := browser()
	member.call("POST", "/api/v1/auth/invites/"+strings.TrimPrefix(link, "/invite/"), `{"name":"ana","password":"a long test passphrase"}`, 201)
	for _, f := range []struct{ kind, label string }{{"movie", "Movies"}, {"show", "Shows"}, {"music", "Music"}} {
		e.owner.call("POST", "/api/v1/library-folders", fmt.Sprintf(`{"path":%q,"kind":%q,"label":%q}`, t.TempDir(), f.kind, f.label), 201)
	}
	return e
}

// t10Seed adds a movie in a collection, a show and an artist.
func t10Seed(t *testing.T, e *t10Env) (collection, movie, show, episode, artist, album int64) {
	t.Helper()
	ctx := context.Background()
	movie, err := library.AddMovie(ctx, e.store, metadata.Movie{Name: "The Fellowship of the Ring", Year: 2001, IDs: map[string]string{"tmdb": "120"}}, library.AddOptions{}, nil, time.Now().UTC(), domain.System)
	if err != nil {
		t.Fatal(err)
	}
	err = e.store.Write(ctx, func(tx *sql.Tx) error {
		res, err := tx.Exec("INSERT INTO movie_collections(source_id, name) VALUES ('tmdb:119', 'The Lord of the Rings Collection')")
		if err != nil {
			return err
		}
		collection, _ = res.LastInsertId()
		_, err = tx.Exec("INSERT INTO movie_collection_titles(collection_id, title_id) VALUES (?, ?)", collection, movie)
		return err
	})
	if err != nil {
		t.Fatal(err)
	}
	var added struct {
		Title struct {
			ID int64 `json:"id"`
		} `json:"title"`
	}
	_ = json.Unmarshal(e.owner.call("POST", "/api/v1/shows", `{"id":"tmdb:100"}`, 201), &added)
	show = added.Title.ID
	_ = e.store.Read().QueryRow("SELECT id FROM items WHERE title_id = ? AND kind = 'episode' ORDER BY id LIMIT 1", show).Scan(&episode)
	_ = json.Unmarshal(e.owner.call("POST", "/api/v1/artists", `{"id":"mbid:`+apiArtist+`"}`, 201), &added)
	artist = added.Title.ID
	var al struct {
		Albums []struct {
			ID int64 `json:"id"`
		} `json:"albums"`
	}
	_ = json.Unmarshal(e.owner.call("GET", fmt.Sprintf("/api/v1/titles/%d/albums", artist), "", 200), &al)
	if len(al.Albums) == 0 {
		t.Fatal("no albums")
	}
	album = al.Albums[0].ID
	return
}
