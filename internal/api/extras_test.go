package api

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"

	"github.com/CoderLuii/holyarrs/internal/auth"

	"github.com/CoderLuii/holyarrs/internal/metadata"
)

// route: GET /api/v1/extras/{kind}/{id}
func TestTitleExtrasFiltersScoresAndFallsBack(t *testing.T) {
	titleExtrasCache.seen = map[string]extrasEntry{}
	asked := 0
	srv := httptest.NewServer(New(Server{Version: "test",
		ExtraArt: func(_ context.Context, kind, id string) (metadata.ServerArt, error) {
			asked++
			if id == "1" {
				return metadata.ServerArt{Logo: "https://assets.fanart.tv/l.png"}, nil
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
	var v TitleExtras
	_ = json.Unmarshal(do(t, "GET", srv.URL+"/api/v1/extras/movie/1", "", 200), &v)
	if v.Logo != "https://assets.fanart.tv/l.png" || len(v.Scores) != 2 || v.Scores[0].Source != "imdb" || v.Scores[1].Source != "rt_audience" {
		t.Fatalf("%+v", v)
	}
	do(t, "GET", srv.URL+"/api/v1/extras/movie/1", "", 200)
	if asked != 1 {
		t.Fatalf("not cached: %d", asked)
	}
	v = TitleExtras{}
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
	srv := httptest.NewServer(New(Server{Version: "test"}).Handler())
	defer srv.Close()
	b := string(do(t, "GET", srv.URL+"/api/v1/extras/movie/9", "", 200))
	if b != "{\"scores\":[]}\n" {
		t.Fatalf("%q", b)
	}
}

// A Kid gets extras only for titles under their limit, even when the answer
// is already cached for someone else; music never passes a limit.
func TestTitleExtrasKidGate(t *testing.T) {
	titleExtrasCache.seen = map[string]extrasEntry{}
	md := animeTMDB(t)
	_, st := discoverRig(t, md)
	asked := 0
	a := New(Server{Store: st, Metadata: md, Version: "test",
		ExtraArt: func(_ context.Context, kind, id string) (metadata.ServerArt, error) {
			asked++
			return metadata.ServerArt{Logo: "https://assets.fanart.tv/l.png"}, nil
		}})
	call := func(who *auth.Who, kind, id string) int {
		r := httptest.NewRequest("GET", "/api/v1/extras/"+kind+"/"+id, nil)
		r.SetPathValue("kind", kind)
		r.SetPathValue("id", id)
		if who != nil {
			r = r.WithContext(auth.ContextWith(r.Context(), *who))
		}
		w := httptest.NewRecorder()
		a.titleExtras(w, r)
		return w.Code
	}
	kid := &auth.Who{UserID: 7, Role: auth.RoleMember, MaxRating: "TV-Y7"}
	owner := &auth.Who{UserID: 1, Role: auth.RoleOwner}
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
// A cold artist through the real metadata client: the server first answers
// 404 (or 429, its install limit), then 200. The first app answer is empty
// and never cached; the next ask gets the wide photo and the logo.
func TestTitleExtrasColdArtistThenArt(t *testing.T) {
	for _, first := range []int{404, 429, 503} {
		titleExtrasCache.seen = map[string]extrasEntry{}
		var n atomic.Int32
		md := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if n.Add(1) == 1 {
				w.Header().Set("Retry-After", "1")
				w.WriteHeader(first)
				return
			}
			_, _ = w.Write([]byte(`{"logo":"https://assets.fanart.tv/l.png","backgrounds":["https://assets.fanart.tv/b.jpg"]}`))
		}))
		svc, err := metadata.NewService(metadata.ServiceOptions{URL: md.URL, AllowLocal: true, HTTP: &http.Client{}, Access: func(context.Context) (string, error) { return "t", nil }})
		if err != nil {
			t.Fatal(err)
		}
		srv := httptest.NewServer(New(Server{Version: "test", ExtraArt: svc.Art}).Handler())
		const p = "/api/v1/extras/artist/c8b03190-306c-4120-bb0b-6f2ebfc06ea9"
		res, err := http.Get(srv.URL + p)
		if err != nil {
			t.Fatal(err)
		}
		var v TitleExtras
		_ = json.NewDecoder(res.Body).Decode(&v)
		res.Body.Close()
		if len(v.Backgrounds) != 0 || res.Header.Get("Cache-Control") != "no-store" {
			t.Fatalf("%d: cold answer %+v %q", first, v, res.Header.Get("Cache-Control"))
		}
		v = TitleExtras{}
		_ = json.Unmarshal(do(t, "GET", srv.URL+p, "", 200), &v)
		if v.Logo == "" || len(v.Backgrounds) != 1 {
			t.Fatalf("%d: second answer %+v", first, v)
		}
		srv.Close()
		md.Close()
	}
}

// The server's 202 (art fetch still running) is waited on inside the same
// answer: the first view already gets the art.
func TestTitleExtrasWaitsOnPendingArt(t *testing.T) {
	titleExtrasCache.seen = map[string]extrasEntry{}
	var n atomic.Int32
	md := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if n.Add(1) == 1 {
			w.Header().Set("Retry-After", "1")
			w.WriteHeader(202)
			return
		}
		_, _ = w.Write([]byte(`{"logo":"https://assets.fanart.tv/l.png","backgrounds":["https://assets.fanart.tv/b.jpg"]}`))
	}))
	defer md.Close()
	svc, _ := metadata.NewService(metadata.ServiceOptions{URL: md.URL, AllowLocal: true, HTTP: &http.Client{}, Access: func(context.Context) (string, error) { return "t", nil }})
	srv := httptest.NewServer(New(Server{Version: "test", ExtraArt: svc.Art}).Handler())
	defer srv.Close()
	var v TitleExtras
	_ = json.Unmarshal(do(t, "GET", srv.URL+"/api/v1/extras/artist/c8b03190-306c-4120-bb0b-6f2ebfc06ea9", "", 200), &v)
	if len(v.Backgrounds) != 1 {
		t.Fatalf("%+v", v)
	}
}
