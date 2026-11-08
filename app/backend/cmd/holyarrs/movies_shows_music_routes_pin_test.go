package main

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/cookiejar"
	"net/http/httptest"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"
	"time"

	"github.com/HolyArrs/holyarrs/app/backend/internal/accounts"
	"github.com/HolyArrs/holyarrs/app/backend/internal/backups"
	"github.com/HolyArrs/holyarrs/app/backend/internal/core/database"
	"github.com/HolyArrs/holyarrs/app/backend/internal/core/jobs"
	domain "github.com/HolyArrs/holyarrs/app/backend/internal/core/model"
	"github.com/HolyArrs/holyarrs/app/backend/internal/core/secrets"
	"github.com/HolyArrs/holyarrs/app/backend/internal/core/webserver"
	"github.com/HolyArrs/holyarrs/app/backend/internal/library"
	"github.com/HolyArrs/holyarrs/app/backend/internal/metadata"
)

// Pins the status code, auth level and response shape of every movie
// collection, show and music route, through the real API with a real
// sign-in (owner, member, nobody). The route table itself is pinned by
// TestRouteTableUnchanged; this adds what a route answers.

type t10Client struct {
	c    *http.Client
	csrf string
}

type t10Env struct {
	t      *testing.T
	srv    *httptest.Server
	store  *database.Store
	api    *handlers
	lib    *library.Service
	owner  t10Client
	member t10Client
	anon   t10Client
}

func (e *t10Env) call(c t10Client, method, path, body string) (int, []byte) {
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

func t10NewClient() *http.Client {
	jar, _ := cookiejar.New(nil)
	return &http.Client{Jar: jar, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
}

func t10Setup(t *testing.T) *t10Env {
	t.Helper()
	dir := t.TempDir()
	s, err := database.Open(dir, database.Options{})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = s.Close() })
	v, err := secrets.Open(context.Background(), s, make([]byte, secrets.KeySize))
	if err != nil {
		t.Fatal(err)
	}
	au := accounts.New(s, accounts.Options{Vault: v})
	q := jobs.New(s, jobs.Options{})
	tmdb := &fakeShowSource{shows: map[string]metadata.Show{"tmdb:100": apiShow("tmdb", "100"), "tvdb:200": apiShow("tmdb", "100")}}
	lib := &library.Service{Store: s, Queue: q, Artists: apiMusic(), Metadata: metadata.Lab(),
		Shows: metadata.ShowSources{"tmdb": tmdb, "tvdb": &fakeShowSource{err: metadata.ErrNoTVDBKey}}}
	a := buildHandlers(services{Store: s, Queue: q, Library: lib, Metadata: &metadata.Static{}, Version: "test", Auth: au, Secrets: v,
		Live: webserver.NewLiveHub(webserver.LiveOptions{}), Backups: &backups.Manager{Store: s, Dir: dir, Version: "test"}, Restart: func() {}})
	srv := httptest.NewServer(routes(a, config{}))
	t.Cleanup(srv.Close)
	e := &t10Env{t: t, srv: srv, store: s, api: a, lib: lib, anon: t10Client{c: t10NewClient()}}
	csrfOf := func(b []byte) string {
		var x struct {
			CSRF string `json:"csrf"`
			Path string `json:"path"`
		}
		_ = json.Unmarshal(b, &x)
		return x.CSRF + x.Path
	}
	e.owner = t10Client{c: t10NewClient()}
	st, b := e.call(e.owner, "POST", "/api/v1/auth/setup", `{"name":"luis","password":"a long test passphrase"}`)
	if st != 201 {
		t.Fatalf("setup %d %s", st, b)
	}
	e.owner.csrf = csrfOf(b)
	st, b = e.call(e.owner, "POST", "/api/v1/users/invites", `{}`)
	if st != 201 {
		t.Fatalf("invite %d %s", st, b)
	}
	link := csrfOf(b)
	e.member = t10Client{c: t10NewClient()}
	st, b = e.call(e.member, "POST", "/api/v1/auth/invites/"+strings.TrimPrefix(link, "/invite/"), `{"name":"ana","password":"a long test passphrase"}`)
	if st != 201 {
		t.Fatalf("join %d %s", st, b)
	}
	e.member.csrf = csrfOf(b)
	for _, f := range []struct{ kind, label string }{{"movie", "Movies"}, {"show", "Shows"}, {"music", "Music"}} {
		st, b = e.call(e.owner, "POST", "/api/v1/library-folders", fmt.Sprintf(`{"path":%q,"kind":%q,"label":%q}`, t.TempDir(), f.kind, f.label))
		if st != 201 {
			t.Fatalf("folder %s %d %s", f.kind, st, b)
		}
	}
	return e
}

// t10Shape reduces JSON to its field names and value kinds, so a golden
// freezes the record fields without freezing ids, times or counts.
func t10Shape(v any) any {
	switch x := v.(type) {
	case map[string]any:
		out := map[string]any{}
		for k, val := range x {
			out[k] = t10Shape(val)
		}
		return out
	case []any:
		if len(x) == 0 {
			return []any{}
		}
		return []any{t10Shape(x[0])}
	case nil:
		return "null"
	case float64:
		return "number"
	case string:
		return "string"
	case bool:
		return "bool"
	}
	return fmt.Sprintf("%T", v)
}

type t10Route struct {
	name, method, path, body string
}

func t10Golden(t *testing.T, name string, got []byte) {
	t.Helper()
	p := filepath.Join("testdata", name)
	if os.Getenv("PIN_UPDATE") == "1" {
		if err := os.MkdirAll("testdata", 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, append(got, '\n'), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	want, err := os.ReadFile(p)
	if err != nil {
		t.Fatalf("golden %s missing: %v", p, err)
	}
	if string(want) != string(got)+"\n" {
		t.Fatalf("golden %s differs, got:\n%s", p, got)
	}
}

// t10Seed adds a movie in a collection, a show and an artist, and returns
// the ids the route table needs.
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
	st, b := e.call(e.owner, "POST", "/api/v1/shows", `{"id":"tmdb:100"}`)
	if st != 201 {
		t.Fatalf("add show %d %s", st, b)
	}
	var sh addedShow
	_ = json.Unmarshal(b, &sh)
	show = sh.Title.ID
	st, b = e.call(e.owner, "GET", fmt.Sprintf("/api/v1/titles/%d/episodes", show), "")
	if st != 200 {
		t.Fatalf("episodes %d %s", st, b)
	}
	var eps struct {
		Episodes []struct {
			ID int64 `json:"id"`
		} `json:"episodes"`
	}
	_ = json.Unmarshal(b, &eps)
	if len(eps.Episodes) == 0 {
		t.Fatalf("no episodes %s", b)
	}
	episode = eps.Episodes[0].ID
	st, b = e.call(e.owner, "POST", "/api/v1/artists", `{"id":"mbid:`+apiArtist+`"}`)
	if st != 201 {
		t.Fatalf("add artist %d %s", st, b)
	}
	var ar struct {
		Title struct {
			ID int64 `json:"id"`
		} `json:"title"`
	}
	_ = json.Unmarshal(b, &ar)
	artist = ar.Title.ID
	st, b = e.call(e.owner, "GET", fmt.Sprintf("/api/v1/titles/%d/albums", artist), "")
	if st != 200 {
		t.Fatalf("albums %d %s", st, b)
	}
	var al struct {
		Albums []struct {
			ID int64 `json:"id"`
		} `json:"albums"`
	}
	_ = json.Unmarshal(b, &al)
	if len(al.Albums) == 0 {
		t.Fatalf("no albums %s", b)
	}
	album = al.Albums[0].ID
	return
}

func TestT10PinMoviesShowsMusicRoutes(t *testing.T) {
	e := t10Setup(t)
	collection, _, show, episode, artist, album := t10Seed(t, e)
	routes := []t10Route{
		{"collections list", "GET", "/api/v1/collections", ""},
		{"collection detail", "GET", fmt.Sprintf("/api/v1/collections/%d", collection), ""},
		{"collection missing", "GET", "/api/v1/collections/99999", ""},
		{"collection preview", "POST", fmt.Sprintf("/api/v1/collections/%d/remove/preview", collection), `{}`},
		{"collection remove unconfirmed", "POST", fmt.Sprintf("/api/v1/collections/%d/remove", collection), `{}`},
		{"shows lookup", "GET", "/api/v1/shows/lookup?term=lab", ""},
		{"shows lookup no term", "GET", "/api/v1/shows/lookup", ""},
		{"show seasons", "GET", fmt.Sprintf("/api/v1/titles/%d/seasons", show), ""},
		{"show season set", "PUT", fmt.Sprintf("/api/v1/titles/%d/seasons/1", show), `{"keep_up_to_date":true}`},
		{"show episodes", "GET", fmt.Sprintf("/api/v1/titles/%d/episodes", show), ""},
		{"show episode set", "PUT", fmt.Sprintf("/api/v1/episodes/%d", episode), `{"keep_up_to_date":true}`},
		{"show settings get", "GET", "/api/v1/settings/shows", ""},
		{"show settings put", "PUT", "/api/v1/settings/shows", `{}`},
		{"anime", "GET", "/api/v1/metadata/show/100/anime", ""},
		{"artists lookup", "GET", "/api/v1/artists/lookup?term=lab", ""},
		{"artist albums", "GET", fmt.Sprintf("/api/v1/titles/%d/albums", artist), ""},
		{"album", "GET", fmt.Sprintf("/api/v1/albums/%d", album), ""},
		{"album tracks", "GET", fmt.Sprintf("/api/v1/albums/%d/tracks", album), ""},
		{"album set", "PUT", fmt.Sprintf("/api/v1/albums/%d", album), `{}`},
		{"artist set", "PUT", fmt.Sprintf("/api/v1/titles/%d/artist", artist), `{}`},
		{"music settings get", "GET", "/api/v1/settings/music", ""},
		{"music settings put", "PUT", "/api/v1/settings/music", `{}`},
		{"music artist page", "GET", "/api/v1/music/artists/" + apiArtist, ""},
		{"music album page", "GET", "/api/v1/music/albums/" + apiAlbum, ""},
		{"music browse tiles", "GET", "/api/v1/music/browse-tiles", ""},
		{"music genre", "GET", "/api/v1/music/genre/rock", ""},
		{"music trending", "GET", "/api/v1/music-trending", ""},
		{"explore music", "GET", "/api/v1/explore/music", ""},
		{"explore music rising", "GET", "/api/v1/explore/music/rising", ""},
		{"explore new music", "GET", "/api/v1/explore/new/music", ""},
	}
	shapes := map[string]any{}
	for _, r := range routes {
		pattern := r.method + " " + r.path
		// Public id-free form of the pattern for the perm table lookup.
		need := t10Need(pattern)
		if need == "" {
			t.Errorf("%s: no route in accounts.RoutePerms", pattern)
			continue
		}
		if st, b := e.call(e.anon, r.method, r.path, r.body); st != http.StatusUnauthorized {
			t.Errorf("%s anonymous: %d %s", r.name, st, b)
		}
		mst, mb := e.call(e.member, r.method, r.path, r.body)
		if need == "signed_in" {
			if mst == http.StatusForbidden || mst == http.StatusUnauthorized {
				t.Errorf("%s member: %d %s", r.name, mst, mb)
			}
		} else if mst != http.StatusForbidden {
			t.Errorf("%s member: %d %s (needs %s)", r.name, mst, mb, need)
		}
		ost, ob := e.call(e.owner, r.method, r.path, r.body)
		entry := map[string]any{"need": need, "status": ost}
		var v any
		if json.Unmarshal(ob, &v) == nil {
			entry["shape"] = t10Shape(v)
		}
		shapes[r.name] = entry
	}
	keys := make([]string, 0, len(shapes))
	for k := range shapes {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	b, err := json.MarshalIndent(shapes, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	t10Golden(t, "movies_shows_music_routes_pin.golden", b)
}

// t10Need finds the permission of a concrete request path by matching it
// against the route patterns in accounts.RoutePerms.
func t10Need(pattern string) string {
	method, path, _ := strings.Cut(pattern, " ")
	path, _, _ = strings.Cut(path, "?")
	segs := strings.Split(path, "/")
	for p, need := range accounts.RoutePerms {
		pm, pp, _ := strings.Cut(p, " ")
		if pm != method {
			continue
		}
		ps := strings.Split(pp, "/")
		if len(ps) != len(segs) {
			continue
		}
		ok := true
		for i := range ps {
			if strings.HasPrefix(ps[i], "{") {
				continue
			}
			if ps[i] != segs[i] {
				ok = false
				break
			}
		}
		if ok {
			return string(need)
		}
	}
	return ""
}
