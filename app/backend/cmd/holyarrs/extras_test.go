package main

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"sync/atomic"
	"testing"

	"github.com/HolyArrs/holyarrs/app/backend/internal/core/webserver"
	"github.com/HolyArrs/holyarrs/app/backend/internal/library"
	"github.com/HolyArrs/holyarrs/app/backend/internal/metadata"
	"github.com/HolyArrs/holyarrs/app/backend/internal/music"
)

// route: GET /api/v1/extras/{kind}/{id}
func TestTitleExtrasFiltersScoresAndFallsBack(t *testing.T) {
	t.Parallel()
	asked := 0
	const img = "https://api.example.test/v1/img"
	srv := httptest.NewServer(buildHandlers(services{Version: "test", ImageBase: img,
		ExtraArt: func(_ context.Context, kind, id string) (metadata.ServerArt, error) {
			asked++
			if id == "1" {
				return metadata.ServerArt{Logo: img + "/title-logo/movie-1/1200.png", Backgrounds: []string{img + "/title-wide/movie-1/1600.webp?v=a1", "https://assets.fanart.tv/b.jpg", img + "/title-wide/movie-1/../x.jpg"}}, nil
			}
			return metadata.ServerArt{}, metadata.ErrNotFound
		},
		ExtraRatings: func(_ context.Context, kind, id string) (metadata.ServerRatings, error) {
			if id == "3" {
				return metadata.ServerRatings{}, metadata.ErrServiceUnavailable
			}
			return metadata.ServerRatings{Scores: []metadata.ServerScore{
				{Source: "imdb", Value: 8.7, Max: 10, Votes: 2000000},
				{Source: "rt_critics", Value: 83, Max: 100, Votes: 5},
				{Source: "metacritic", Value: 73, Max: 100},
				{Source: "rt_audience", Value: 85, Max: 100, Votes: 250000},
				{Source: "unknown", Value: 1, Max: 10, Votes: 1e9},
			}}, nil
		}}).Handler())
	defer srv.Close()
	var v metadata.TitleExtras
	_ = json.Unmarshal(do(t, "GET", srv.URL+"/api/v1/extras/movie/1", "", 200), &v)
	// The server's pictures come back as artwork cache links; others are dropped.
	if v.Logo != "/api/v1/artwork/img/title-logo/movie-1/1200.png" || len(v.Backgrounds) != 1 || v.Backgrounds[0] != "/api/v1/artwork/img/title-wide/movie-1/1600.webp?v=a1" {
		t.Fatalf("pictures %+v", v)
	}
	if len(v.Scores) != 2 || v.Scores[0].Source != "imdb" || v.Scores[1].Source != "rt_audience" {
		t.Fatalf("%+v", v)
	}
	do(t, "GET", srv.URL+"/api/v1/extras/movie/1", "", 200)
	if asked != 1 {
		t.Fatalf("not cached: %d", asked)
	}
	v = metadata.TitleExtras{}
	_ = json.Unmarshal(do(t, "GET", srv.URL+"/api/v1/extras/show/2", "", 200), &v)
	if v.Logo != "" || len(v.Scores) != 2 {
		t.Fatalf("show %+v", v)
	}
	// A server hiccup is not kept.
	do(t, "GET", srv.URL+"/api/v1/extras/movie/3", "", 200)
	do(t, "GET", srv.URL+"/api/v1/extras/movie/3", "", 200)
	if asked != 4 {
		t.Fatalf("hiccup cached: %d", asked)
	}
	for _, p := range []string{"/api/v1/extras/movie/01", "/api/v1/extras/album/1", "/api/v1/extras/artist/abc"} {
		do(t, "GET", srv.URL+p, "", 400)
	}
}

func TestTitleExtrasEmptyWhenOff(t *testing.T) {
	t.Parallel()
	srv := httptest.NewServer(buildHandlers(services{Version: "test"}).Handler())
	defer srv.Close()
	b := string(do(t, "GET", srv.URL+"/api/v1/extras/movie/9", "", 200))
	if b != "{\"scores\":[]}\n" {
		t.Fatalf("%q", b)
	}
}

// A Kid gets extras only for titles under their limit, even when the answer
// is already cached for someone else; music never passes a limit.
func TestTitleExtrasKidGate(t *testing.T) {
	t.Parallel()
	md := animeTMDB(t)
	_, st := discoverRig(t, md)
	asked := 0
	a := buildHandlers(services{Store: st, Metadata: md, Version: "test",
		ExtraArt: func(_ context.Context, kind, id string) (metadata.ServerArt, error) {
			asked++
			return metadata.ServerArt{Logo: "https://api.example.test/v1/img/title-logo/movie-1/1200.png"}, nil
		}})
	call := func(who *webserver.Caller, kind, id string) int {
		r := httptest.NewRequest("GET", "/api/v1/extras/"+kind+"/"+id, nil)
		r.SetPathValue("kind", kind)
		r.SetPathValue("id", id)
		if who != nil {
			r = r.WithContext(webserver.ContextWith(r.Context(), *who))
		}
		w := httptest.NewRecorder()
		titleRoute(a, w, r)
		return w.Code
	}
	kid := &webserver.Caller{UserID: 7, Role: webserver.RoleMember, MaxRating: "TV-Y7"}
	owner := &webserver.Caller{UserID: 1, Role: webserver.RoleOwner}
	// The owner fills the cache for a title above the Kid's limit.
	if c := call(owner, "show", "9400"); c != 200 {
		t.Fatalf("owner %d", c)
	}
	for _, id := range []string{"9400", "9100", "404404"} {
		if c := call(kid, "show", id); c != 403 {
			t.Fatalf("kid %s: %d", id, c)
		}
	}
	if c := call(kid, "artist", "5b11f4ce-a62d-471e-81fc-a69a8278c7da"); c != 403 {
		t.Fatalf("kid artist %d", c)
	}
	if c := call(kid, "show", "9300"); c != 200 {
		t.Fatalf("kid under the limit %d", c)
	}
	if asked != 2 {
		t.Fatalf("server asked %d", asked)
	}
}

// route: GET /api/v1/extras/{kind}/{id}
// An artist's wide photo and logo come only from the metadata server's
// stored pictures, through the picture route. Not stored yet: the answer
// is empty and never cached; once stored, the next ask gets both. No
// outside address is ever passed on.
func TestTitleExtrasArtistFromServerOnly(t *testing.T) {
	t.Parallel()
	const id = "c8b03190-306c-4120-bb0b-6f2ebfc06ea9"
	var stored atomic.Bool
	pics := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if stored.Load() && (r.URL.Path == "/v1/img/wide/"+id+"/1920.webp" || r.URL.Path == "/v1/img/logo/"+id+"/1200.png") {
			w.Header().Set("Content-Type", "image/jpeg")
			_, _ = w.Write([]byte("\xff\xd8\xff art"))
			return
		}
		w.Header().Set("Cache-Control", "no-store")
		http.NotFound(w, r)
	}))
	defer pics.Close()
	_, st := discoverRig(t, newDiscoverTMDB(t).provider())
	art := &library.ArtworkCache{Dir: filepath.Join(t.TempDir(), "art"), Allowed: []string{pics.URL + "/"}, HTTP: pics.Client()}
	outside := func(context.Context, string, string) (metadata.ServerArt, error) {
		return metadata.ServerArt{Logo: pics.URL + "/v1/img/title-logo/movie-1/1200.png", Backgrounds: []string{pics.URL + "/v1/img/title-wide/movie-1/1600.webp"}}, nil
	}
	a := buildHandlers(services{Store: st, Version: "test", ExtraArt: outside, ImageBase: pics.URL + "/v1/img", Library: &library.Service{Store: st, Artwork: art}})
	srv := httptest.NewServer(a.Handler())
	defer srv.Close()
	const p = "/api/v1/extras/artist/" + id
	res, err := http.Get(srv.URL + p)
	if err != nil {
		t.Fatal(err)
	}
	var v metadata.TitleExtras
	_ = json.NewDecoder(res.Body).Decode(&v)
	res.Body.Close()
	if len(v.Backgrounds) != 0 || v.Logo != "" || res.Header.Get("Cache-Control") != "private, no-cache" {
		t.Fatalf("not stored yet: %+v %q", v, res.Header.Get("Cache-Control"))
	}
	stored.Store(true)
	a.mh().TestHooks().ForgetArtMisses() // the miss time has passed
	v = metadata.TitleExtras{}
	_ = json.Unmarshal(do(t, "GET", srv.URL+p, "", 200), &v)
	if v.Logo != music.ArtPrefix+"logo/"+id || len(v.Backgrounds) != 1 || v.Backgrounds[0] != music.ArtPrefix+"wide/"+id {
		t.Fatalf("stored: %+v", v)
	}
}

// titleRoute serves one request through the title-page routes.
func titleRoute(a *handlers, w http.ResponseWriter, r *http.Request) {
	m := http.NewServeMux()
	a.titles.Routes(m)
	m.ServeHTTP(w, r)
}
