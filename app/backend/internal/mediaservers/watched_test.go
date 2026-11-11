package mediaservers

import (
	"context"
	"database/sql"
	"maps"
	"net/http"
	"net/http/httptest"
	"slices"
	"strconv"
	"strings"
	"testing"

	"github.com/HolyArrs/holyarrs/app/backend/internal/core/database"
	domain "github.com/HolyArrs/holyarrs/app/backend/internal/core/model"
	"github.com/HolyArrs/holyarrs/app/backend/internal/mediafiles"
)

// packShow adds a show (tvdb 100) with episodes S01E01-E03 and returns its
// id and the episode item ids.
func packShow(t *testing.T, w *world) (int64, []int64) {
	t.Helper()
	var title int64
	var eps []int64
	err := w.s.Write(context.Background(), func(tx *sql.Tx) error {
		ti := &mediafiles.Title{Kind: "show", GateDriver: "none", Name: "Severance", SortName: "severance", MetadataSource: "tvdb", NewPartsPolicy: "all",
			ReleaseGate: "released", WhyAdded: "by_hand", Status: "running", State: "active", SourceIDs: map[string]string{"tvdb": "100"}, Aliases: []string{}, MusicTypes: []string{}, Artwork: []string{}}
		if _, err := database.Insert(tx, "titles", ti, domain.System); err != nil {
			return err
		}
		title = ti.ID
		for e := 1; e <= 3; e++ {
			s, ep := 1, e
			it := &mediafiles.Item{TitleID: ti.ID, Kind: "episode", Numbers: mediafiles.Numbers{Season: &s, Episode: &ep}, KeepUpToDate: true}
			if _, err := database.Insert(tx, "items", it, domain.System); err != nil {
				return err
			}
			eps = append(eps, it.ID)
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	return title, eps
}

type advanced map[int64]map[int64]bool

func (a advanced) fn(_ context.Context, id int64, watched map[int64]bool, _ domain.Actor) error {
	a[id] = maps.Clone(watched)
	return nil
}

func plexWatchServer(t *testing.T, paths *[]string) *httptest.Server {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("X-Plex-Token") != tok || r.Method != http.MethodGet {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		*paths = append(*paths, r.URL.Path)
		switch r.URL.Path {
		case "/library/sections":
			_, _ = w.Write([]byte(`<MediaContainer><Directory key="1" type="movie" title="Movies"/><Directory key="2" type="show" title="TV"/></MediaContainer>`))
		case "/library/sections/2/all":
			_, _ = w.Write([]byte(plexPage(r, `<Directory ratingKey="9" guid="com.plexapp.agents.thetvdb://555?lang=en"/>`,
				`<Directory ratingKey="7" guid="plex://show/abc"><Guid id="imdb://tt1"/><Guid id="tvdb://100"/></Directory>`)))
		case "/status/sessions/history/all":
			// Another account watched S01E02; S02E05 belongs to another show.
			_, _ = w.Write([]byte(plexPage(r, `<Video type="episode" accountID="2" grandparentKey="/library/metadata/9" parentIndex="1" index="1"/>`,
				`<Video type="episode" accountID="2" grandparentKey="/library/metadata/7" parentIndex="1" index="2"/>`)))
		case "/library/metadata/7/allLeaves":
			_, _ = w.Write([]byte(`<MediaContainer><Video parentIndex="1" index="1" viewCount="2"/><Video parentIndex="1" index="2"/><Video parentIndex="1" index="3" viewCount="1"/></MediaContainer>`))
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	t.Cleanup(srv.Close)
	return srv
}

// plexPage answers one page of entries, as X-Plex-Container-Start and -Size ask.
func plexPage(r *http.Request, entries ...string) string {
	q := r.URL.Query()
	start, _ := strconv.Atoi(q.Get("X-Plex-Container-Start"))
	size, err := strconv.Atoi(q.Get("X-Plex-Container-Size"))
	if err != nil {
		size = len(entries)
	}
	start = min(start, len(entries))
	return "<MediaContainer>" + strings.Join(entries[start:min(start+size, len(entries))], "") + "</MediaContainer>"
}

// smallPages makes every list call ask for one entry at a time.
func smallPages(t *testing.T) {
	old := watchPage
	watchPage = 1
	t.Cleanup(func() { watchPage = old })
}

func TestAdvancePacksPlex(t *testing.T) {
	smallPages(t)
	w := newWorld(t)
	ctx := context.Background()
	title, eps := packShow(t, w)
	var paths []string
	srv := plexWatchServer(t, &paths)
	if _, err := Add(ctx, w.s, w.v, Input{Name: "Plex", Kind: "plex", URL: srv.URL, Token: tok}, domain.System); err != nil {
		t.Fatal(err)
	}
	got := advanced{}
	if err := (&Linker{Store: w.s, Vault: w.v}).AdvancePacks(ctx, []int64{title}, got.fn); err != nil {
		t.Fatal(err)
	}
	if !maps.Equal(got[title], map[int64]bool{eps[0]: true, eps[1]: true, eps[2]: true}) {
		t.Fatalf("watched %v, episodes %v", got, eps)
	}
}

func TestAdvancePacksJellyfinAnyUser(t *testing.T) {
	smallPages(t)
	w := newWorld(t)
	ctx := context.Background()
	title, eps := packShow(t, w)
	var paths []string
	srv := httptest.NewServer(http.HandlerFunc(func(rw http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			rw.WriteHeader(http.StatusMethodNotAllowed)
			return
		}
		paths = append(paths, r.URL.Path)
		q := r.URL.Query()
		switch r.URL.Path {
		case "/Items":
			all := []string{`{"Id":"s2","ProviderIds":{"Tvdb":"200"}}`, `{"Id":"s1","ProviderIds":{"Tvdb":"100"}}`}
			start, _ := strconv.Atoi(q.Get("StartIndex"))
			limit, err := strconv.Atoi(q.Get("Limit"))
			if err != nil {
				limit = len(all)
			}
			start = min(start, len(all))
			_, _ = rw.Write([]byte(`{"Items":[` + strings.Join(all[start:min(start+limit, len(all))], ",") + `]}`))
		case "/Users":
			_, _ = rw.Write([]byte(`[{"Id":"u1"},{"Id":"u2"}]`))
		case "/Users/u1/Items", "/Users/u2/Items":
			if q.Get("ParentId") != "s1" || q.Get("IsPlayed") != "true" {
				rw.WriteHeader(http.StatusBadRequest)
				return
			}
			if r.URL.Path == "/Users/u1/Items" {
				_, _ = rw.Write([]byte(`{"Items":[{"ParentIndexNumber":1,"IndexNumber":1}]}`))
			} else {
				_, _ = rw.Write([]byte(`{"Items":[{"ParentIndexNumber":1,"IndexNumber":2},{"ParentIndexNumber":2,"IndexNumber":9}]}`))
			}
		default:
			rw.WriteHeader(http.StatusNotFound)
		}
	}))
	t.Cleanup(srv.Close)
	if _, err := Add(ctx, w.s, w.v, Input{Name: "Jellyfin", Kind: "jellyfin", URL: srv.URL, Token: tok}, domain.System); err != nil {
		t.Fatal(err)
	}
	got := advanced{}
	if err := (&Linker{Store: w.s, Vault: w.v}).AdvancePacks(ctx, []int64{title}, got.fn); err != nil {
		t.Fatal(err)
	}
	if !maps.Equal(got[title], map[int64]bool{eps[0]: true, eps[1]: true}) {
		t.Fatalf("watched %v, episodes %v", got, eps)
	}
	if !slices.Equal(paths, []string{"/Items", "/Items", "/Items", "/Users", "/Users/u1/Items", "/Users/u2/Items"}) {
		t.Fatalf("calls %v", paths)
	}
}

// A failing server is skipped and the others still count; with no titles
// nothing is called.
func TestAdvancePacksSkipsAFailingServer(t *testing.T) {
	w := newWorld(t)
	ctx := context.Background()
	title, eps := packShow(t, w)
	bad := httptest.NewServer(http.HandlerFunc(func(rw http.ResponseWriter, r *http.Request) { rw.WriteHeader(http.StatusInternalServerError) }))
	t.Cleanup(bad.Close)
	var paths []string
	srv := plexWatchServer(t, &paths)
	for _, in := range []Input{{Name: "A broken", Kind: "jellyfin", URL: bad.URL, Token: tok}, {Name: "Plex", Kind: "plex", URL: srv.URL, Token: tok}} {
		if _, err := Add(ctx, w.s, w.v, in, domain.System); err != nil {
			t.Fatal(err)
		}
	}
	l := &Linker{Store: w.s, Vault: w.v}
	got := advanced{}
	if err := l.AdvancePacks(ctx, nil, got.fn); err != nil || len(paths) != 0 || len(got) != 0 {
		t.Fatalf("no titles: %v %v %v", err, paths, got)
	}
	if err := l.AdvancePacks(ctx, []int64{title}, got.fn); err != nil {
		t.Fatal(err)
	}
	if !maps.Equal(got[title], map[int64]bool{eps[0]: true, eps[1]: true, eps[2]: true}) {
		t.Fatalf("watched %v", got)
	}
}
