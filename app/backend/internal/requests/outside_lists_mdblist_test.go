package requests

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"slices"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/HolyArrs/holyarrs/app/backend/internal/core/database"
	domain "github.com/HolyArrs/holyarrs/app/backend/internal/core/model"
	"github.com/HolyArrs/holyarrs/app/backend/internal/metadata"
)

// The only key the fake MDBList API takes.
const fakeMDBListKey = "mdblist-key-for-tests"

// fakeMDBList counts what reached the public site and the API.
type fakeMDBList struct {
	site, api atomic.Int32
	queries   []string // the API queries, in order
	broken    bool     // answer 500 everywhere
}

// mdbItem is one list item as both MDBList answers name it; tvdbKey is the
// field name of the TVDB id (tvdbid on the site, tvdb_id in the API).
func mdbItems(tvdbKey string, items ...map[string]any) []map[string]any {
	out := []map[string]any{}
	for _, it := range items {
		x := map[string]any{}
		for k, v := range it {
			if k == "tvdb" {
				k = tvdbKey
			}
			x[k] = v
		}
		out = append(out, x)
	}
	return out
}

// The lab lists. lab/mixed holds two lab movies (one by IMDb id only), a
// show, and a movie MDBList knows no TMDB id for.
var (
	mdbMixed = []map[string]any{
		{"id": 900002, "imdb_id": "tt9000020", "tvdb": 0, "mediatype": "movie", "title": "Paper Harbour"},
		{"id": 0, "imdb_id": "tt9000030", "tvdb": 0, "mediatype": "movie", "title": "Quiet Orchard"},
		{"id": 700001, "imdb_id": "tt7000010", "tvdb": 810001, "mediatype": "show", "title": "Rookery Dial"},
		{"id": 900004, "imdb_id": "tt9000040", "tvdb": 123, "mediatype": "movie", "title": "Glass Meridian"},
	}
	mdbShows = []map[string]any{
		{"id": 700001, "imdb_id": "tt7000010", "tvdb": 810001, "mediatype": "show", "title": "Rookery Dial"},
		{"id": 700002, "imdb_id": "", "tvdb": 810002, "mediatype": "show", "title": "Bellwether Ferry"},
	}
	mdbNoIDs = []map[string]any{{"id": 0, "imdb_id": "", "tvdb": 0, "mediatype": "movie", "title": "Nobody Knows"}}
)

// newFakeMDBList serves /lists/<user>/<slug>/json (the site) and
// /api/lists/<user>/<slug>/items (the API, paged by limit/offset). It never
// reaches the real MDBList.
func newFakeMDBList(t *testing.T) (*fakeMDBList, *httptest.Server) {
	f := &fakeMDBList{}
	lists := map[string][]map[string]any{"lab/mixed": mdbMixed, "lab/shows": mdbShows, "lab/no-ids": mdbNoIDs}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if f.broken {
			w.WriteHeader(500)
			return
		}
		if site, ok := strings.CutPrefix(r.URL.Path, "/lists/"); ok {
			f.site.Add(1)
			if r.URL.Query().Has("apikey") {
				t.Error("the key went to the public site")
			}
			items, ok := lists[strings.TrimSuffix(site, "/json")]
			if !ok || !strings.HasSuffix(site, "/json") {
				w.WriteHeader(404)
				return
			}
			_ = json.NewEncoder(w).Encode(mdbItems("tvdbid", items...))
			return
		}
		api, ok := strings.CutPrefix(r.URL.Path, "/api/lists/")
		if !ok {
			w.WriteHeader(404)
			return
		}
		f.api.Add(1)
		f.queries = append(f.queries, r.URL.RawQuery)
		if r.URL.Query().Get("apikey") != fakeMDBListKey {
			w.WriteHeader(401)
			return
		}
		items, ok := lists[strings.TrimSuffix(api, "/items")]
		if !ok {
			w.WriteHeader(404)
			return
		}
		// The fake pages two items at a time whatever limit is asked, so a
		// list of four needs two reads.
		offset := 0
		_ = json.Unmarshal([]byte(r.URL.Query().Get("offset")), &offset)
		end := min(offset+2, len(items))
		page := mdbItems("tvdb_id", items[min(offset, len(items)):end]...)
		var movies, shows []map[string]any
		for _, it := range page {
			if it["mediatype"] == "show" {
				shows = append(shows, it)
			} else {
				movies = append(movies, it)
			}
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"movies": movies, "shows": shows,
			"pagination": map[string]any{"limit": 2, "offset": offset, "total": len(items), "has_more": end < len(items)}})
	}))
	t.Cleanup(srv.Close)
	return f, srv
}

// withMDBList points the lab at a fake MDBList, with key as the saved key.
func withMDBList(t *testing.T, e *env, key string) *fakeMDBList {
	f, srv := newFakeMDBList(t)
	e.svc.MDBListSite, e.svc.MDBListBase = srv.URL, srv.URL+"/api"
	e.svc.MDBListKey = func(context.Context) (string, error) { return key, nil }
	return f
}

func tmdbIDs(es []entry) []string {
	out := []string{}
	for _, e := range es {
		out = append(out, e.tmdb+"|"+e.imdb+"|"+e.tvdb)
	}
	slices.Sort(out)
	return out
}

func TestMDBListPublicLink(t *testing.T) {
	e := setup(t)
	f := withMDBList(t, e, "")
	got, err := e.svc.fetchMDBList(context.Background(), "lab/mixed", "movie")
	if err != nil {
		t.Fatal(err)
	}
	// Movies keep no TVDB id, as for Trakt; the show is left out.
	if want := []string{"900002|tt9000020|", "900004|tt9000040|", "|tt9000030|"}; !slices.Equal(tmdbIDs(got), want) {
		t.Fatalf("entries %v, want %v", tmdbIDs(got), want)
	}
	if f.site.Load() != 1 || f.api.Load() != 0 {
		t.Fatalf("site %d api %d", f.site.Load(), f.api.Load())
	}
}

func TestMDBListAPIWithKey(t *testing.T) {
	e := setup(t)
	f := withMDBList(t, e, " "+fakeMDBListKey+" ")
	got, err := e.svc.fetchMDBList(context.Background(), "lab/mixed", "movie")
	if err != nil {
		t.Fatal(err)
	}
	if want := []string{"900002|tt9000020|", "900004|tt9000040|", "|tt9000030|"}; !slices.Equal(tmdbIDs(got), want) {
		t.Fatalf("entries %v, want %v", tmdbIDs(got), want)
	}
	if f.api.Load() != 2 || f.site.Load() != 0 {
		t.Fatalf("api %d site %d, want 2 API pages and no site read", f.api.Load(), f.site.Load())
	}
	if !strings.Contains(f.queries[0], "limit=1000") || !strings.Contains(f.queries[0], "offset=0") || !strings.Contains(f.queries[1], "offset=2") {
		t.Fatalf("queries %v", f.queries)
	}
	shows, err := e.svc.fetchMDBList(context.Background(), "lab/shows", "show")
	if err != nil {
		t.Fatal(err)
	}
	if want := []string{"700001|tt7000010|810001", "700002||810002"}; !slices.Equal(tmdbIDs(shows), want) {
		t.Fatalf("shows %v, want %v", tmdbIDs(shows), want)
	}
}

func TestMDBListKeyRefused(t *testing.T) {
	e := setup(t)
	f := withMDBList(t, e, "not-the-key")
	ctx := context.Background()
	l, err := AddOutsideList(ctx, e.s, OutsideListInput{Name: "Picks", Kind: ListMDBList, Ref: "lab/mixed"}, domain.System)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := e.svc.Sync(ctx, l.ID, domain.System); err == nil || !strings.Contains(err.Error(), "MDBList refused the key") {
		t.Fatalf("err %v", err)
	}
	got, _ := GetOutsideList(ctx, e.s.Read(), l.ID)
	if !strings.Contains(got.LastError, "MDBList refused the key") {
		t.Fatalf("last_error %q", got.LastError)
	}
	if f.site.Load() != 0 {
		t.Fatal("a refused key fell back to the public link")
	}
}

func TestMDBListKeyNeverInErrors(t *testing.T) {
	for _, mode := range []string{"500", "closed"} {
		t.Run(mode, func(t *testing.T) {
			e := setup(t)
			f, srv := newFakeMDBList(t)
			e.svc.MDBListSite, e.svc.MDBListBase = srv.URL, srv.URL+"/api"
			e.svc.MDBListKey = func(context.Context) (string, error) { return fakeMDBListKey, nil }
			if mode == "500" {
				f.broken = true
			} else {
				srv.Close()
			}
			ctx := context.Background()
			l, _ := AddOutsideList(ctx, e.s, OutsideListInput{Name: "Picks", Kind: ListMDBList, Ref: "lab/mixed"}, domain.System)
			_, err := e.svc.Sync(ctx, l.ID, domain.System)
			if err == nil || strings.Contains(err.Error(), fakeMDBListKey) {
				t.Fatalf("err %v", err)
			}
			got, _ := GetOutsideList(ctx, e.s.Read(), l.ID)
			if got.LastError == "" || strings.Contains(got.LastError, fakeMDBListKey) {
				t.Fatalf("last_error %q", got.LastError)
			}
			evs, _ := database.Select[domain.Event](ctx, e.s.ReadEvents(), "events", "kind = ?", domain.EventListSynced)
			for _, ev := range evs {
				if b, _ := json.Marshal(ev); strings.Contains(string(b), fakeMDBListKey) {
					t.Fatalf("event holds the key: %s", b)
				}
			}
		})
	}
}

func TestMDBListKeepsOnlyItsKind(t *testing.T) {
	e := setup(t)
	withMDBList(t, e, "")
	ctx := context.Background()
	l, _ := AddOutsideList(ctx, e.s, OutsideListInput{Name: "Shows only", Kind: ListMDBList, Ref: "lab/shows"}, domain.System)
	res, err := e.svc.Sync(ctx, l.ID, domain.System)
	if err != nil || res.Found != 0 || res.Failed != 0 {
		t.Fatalf("%+v %v", res, err)
	}
}

func TestMDBListItemWithoutIDs(t *testing.T) {
	e := setup(t)
	withMDBList(t, e, "")
	ctx := context.Background()
	l, _ := AddOutsideList(ctx, e.s, OutsideListInput{Name: "No ids", Kind: ListMDBList, Ref: "lab/no-ids"}, domain.System)
	res, err := e.svc.Sync(ctx, l.ID, domain.System)
	if err != nil || res.Found != 1 || res.Skipped != 1 || res.Failed != 0 {
		t.Fatalf("%+v %v", res, err)
	}
}

func TestMDBListOffSendsNothing(t *testing.T) {
	for _, key := range []string{"", fakeMDBListKey} {
		e := setup(t)
		f := withMDBList(t, e, key)
		e.svc.TMDBRoute = func(context.Context) (metadata.TMDBRoute, error) {
			return metadata.TMDBRoute{}, metadata.ErrSourceOff
		}
		if _, err := e.svc.fetchMDBList(context.Background(), "lab/mixed", "movie"); !errors.Is(err, ErrListsOff) {
			t.Fatalf("key %q: %v", key, err)
		}
		if f.site.Load()+f.api.Load() != 0 {
			t.Fatalf("key %q: a request went out while lookups are off", key)
		}
	}
}

func TestMDBListSyncAddsTitles(t *testing.T) {
	e := setup(t)
	withMDBList(t, e, "")
	ctx := context.Background()
	l, err := AddOutsideList(ctx, e.s, OutsideListInput{Name: "Picks", Kind: ListMDBList, Ref: "https://mdblist.com/lists/lab/mixed"}, domain.System)
	if err != nil || l.Ref != "lab/mixed" {
		t.Fatalf("%+v %v", l, err)
	}
	res, err := e.svc.Sync(ctx, l.ID, domain.System)
	if err != nil || res.Found != 3 || res.Added != 3 {
		t.Fatalf("%+v %v", res, err)
	}
	ts := titles(t, e.s)
	for _, n := range []string{"Paper Harbour", "Quiet Orchard", "Glass Meridian"} {
		if ts[n].WhyAdded != "list" {
			t.Fatalf("%s not added by the list: %+v", n, ts)
		}
	}
}

// A key that cannot be read (a damaged vault) is an error, never a quiet
// switch to the public link; only a missing key means the public link.
func TestMDBListKeyReadFailureIsAnError(t *testing.T) {
	e := setup(t)
	f := withMDBList(t, e, "")
	e.svc.MDBListKey = func(context.Context) (string, error) { return "", errors.New("vault: cannot open") }
	if _, err := e.svc.fetchMDBList(context.Background(), "lab/mixed", "movie"); err == nil {
		t.Fatal("a key read failure was ignored")
	}
	if f.site.Load()+f.api.Load() != 0 {
		t.Fatal("a request went out without the key")
	}
}

// With no key saved, a refusal from the public site is not about a key.
func TestMDBListSiteRefusalDoesNotBlameAKey(t *testing.T) {
	e := setup(t)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(403) }))
	t.Cleanup(srv.Close)
	e.svc.MDBListSite = srv.URL
	_, err := e.svc.fetchMDBList(context.Background(), "lab/mixed", "movie")
	if err == nil || strings.Contains(err.Error(), "key") || !strings.Contains(err.Error(), "MDBList refused the request") {
		t.Fatalf("err %v", err)
	}
}

// A redirect from the MDBList API to another host is not followed, so the
// key in the query never reaches that host.
func TestMDBListKeyNotSentOnRedirect(t *testing.T) {
	e := setup(t)
	var leaked atomic.Int32
	other := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.Contains(r.URL.RawQuery, fakeMDBListKey) {
			leaked.Add(1)
		}
		_, _ = w.Write([]byte(`{"movies":[],"shows":[]}`))
	}))
	t.Cleanup(other.Close)
	api := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, other.URL+r.URL.RequestURI(), http.StatusFound)
	}))
	t.Cleanup(api.Close)
	e.svc.MDBListSite, e.svc.MDBListBase = api.URL, api.URL+"/api"
	e.svc.MDBListKey = func(context.Context) (string, error) { return fakeMDBListKey, nil }
	if _, err := e.svc.fetchMDBList(context.Background(), "lab/mixed", "movie"); err == nil {
		t.Fatal("followed a redirect to another host")
	}
	if leaked.Load() != 0 {
		t.Fatal("the key went to another host")
	}
}
