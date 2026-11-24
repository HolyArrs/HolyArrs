package mediaservers

import (
	"context"
	"database/sql"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/HolyArrs/holyarrs/app/backend/internal/core/database"
	domain "github.com/HolyArrs/holyarrs/app/backend/internal/core/model"
	"github.com/HolyArrs/holyarrs/app/backend/internal/mediafiles"
)

// itemServer plays Jellyfin or Emby holding one movie (tmdb 329865) and
// one series (tvdb 100); reads counts the item list reads.
func itemServer(t *testing.T, reads *atomic.Int32) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("X-MediaBrowser-Token") != tok && !strings.Contains(r.Header.Get("Authorization"), tok) {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		switch {
		case r.URL.Path == "/System/Info":
			_, _ = w.Write([]byte(`{"Id":"srv-1"}`))
		case r.URL.Path == "/Items" && r.URL.Query().Get("IncludeItemTypes") == "Movie":
			reads.Add(1)
			_, _ = w.Write([]byte(`{"Items":[{"Id":"other","ProviderIds":{"Tmdb":"1"}},{"Id":"mv-9","ProviderIds":{"Tmdb":"329865"}}]}`))
		case r.URL.Path == "/Items" && r.URL.Query().Get("IncludeItemTypes") == "Series":
			reads.Add(1)
			_, _ = w.Write([]byte(`{"Items":[{"Id":"sr-7","ProviderIds":{"Tvdb":"100"}}]}`))
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	t.Cleanup(srv.Close)
	return srv
}

func setTitleIDs(t *testing.T, w *world, id int64, ids map[string]string) {
	t.Helper()
	err := w.s.Write(context.Background(), func(tx *sql.Tx) error {
		var ti mediafiles.Title
		if err := database.Get(context.Background(), tx, "titles", id, &ti); err != nil {
			return err
		}
		ti.SourceIDs = ids
		return database.Update(tx, "titles", &ti, domain.System)
	})
	if err != nil {
		t.Fatal(err)
	}
}

// A movie and a show open on Jellyfin's details page or Emby's item page,
// at the browser address, never at HolyArrs' own address for the server.
func TestWatchLinks(t *testing.T) {
	for _, kind := range []string{"jellyfin", "emby"} {
		t.Run(kind, func(t *testing.T) {
			w := newWorld(t)
			setTitleIDs(t, w, w.title, map[string]string{"tmdb": "329865"})
			show, _ := packShow(t, w)
			var reads atomic.Int32
			srv := itemServer(t, &reads)
			ctx := context.Background()
			if _, err := Add(ctx, w.s, w.v, Input{Name: "Home", Kind: kind, URL: srv.URL, Token: tok, WebURL: "https://media.example.org/"}, domain.System); err != nil {
				t.Fatal(err)
			}
			l := &Linker{Store: w.s, Vault: w.v}
			page := "https://media.example.org/web/#/details?id="
			if kind == "emby" {
				page = "https://media.example.org/web/index.html#!/item?id="
			}
			for id, item := range map[int64]string{w.title: "mv-9", show: "sr-7"} {
				got, err := l.WatchLinks(ctx, id)
				want := page + item + "&serverId=srv-1"
				if err != nil || len(got) != 1 || got[0].URL != want || got[0].Server != "Home" || got[0].Kind != kind {
					t.Fatalf("title %d: %+v %v, want %s", id, got, err, want)
				}
				if strings.Contains(got[0].URL, srv.URL) {
					t.Fatal("link holds HolyArrs' own address for the server")
				}
			}
			n := reads.Load()
			if _, err := l.WatchLinks(ctx, w.title); err != nil || reads.Load() != n {
				t.Fatalf("kept list read again: %d -> %d %v", n, reads.Load(), err)
			}
		})
	}
}

// No browser address (Jellyfin, Emby), a server that is down, or a title the server
// lacks: no link and no error. A missing Library title is an error.
func TestWatchLinksNone(t *testing.T) {
	w := newWorld(t)
	ctx := context.Background()
	var reads atomic.Int32
	srv := itemServer(t, &reads)
	l := &Linker{Store: w.s, Vault: w.v}
	setTitleIDs(t, w, w.title, map[string]string{"tmdb": "329865"})
	if _, err := Add(ctx, w.s, w.v, Input{Name: "No web", Kind: "jellyfin", URL: srv.URL, Token: tok}, domain.System); err != nil {
		t.Fatal(err)
	}
	if got, err := l.WatchLinks(ctx, w.title); err != nil || len(got) != 0 || reads.Load() != 0 {
		t.Fatalf("no browser address: %+v %v, %d reads", got, err, reads.Load())
	}
	down := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusBadGateway) }))
	t.Cleanup(down.Close)
	for _, in := range []Input{
		{Name: "Down", Kind: "jellyfin", URL: down.URL, Token: tok, WebURL: "http://down.example"},
		{Name: "Up", Kind: "jellyfin", URL: srv.URL, Token: tok, WebURL: "http://up.example"},
	} {
		if _, err := Add(ctx, w.s, w.v, in, domain.System); err != nil {
			t.Fatal(err)
		}
	}
	if got, err := l.WatchLinks(ctx, w.title); err != nil || len(got) != 1 || got[0].Server != "Up" {
		t.Fatalf("one server down: %+v %v", got, err)
	}
	setTitleIDs(t, w, w.title, map[string]string{"tmdb": "404404"})
	if got, err := l.WatchLinks(ctx, w.title); err != nil || len(got) != 0 {
		t.Fatalf("title the server lacks: %+v %v", got, err)
	}
	if _, err := l.WatchLinks(ctx, 999999); err == nil {
		t.Fatal("no error for a missing title")
	}
}

// The same Jellyfin added twice gives one link, not two: the title page
// keys its links by URL, and a duplicate key crashes it
// (each_key_duplicate, #717).
func TestWatchLinksDuplicateServer(t *testing.T) {
	w := newWorld(t)
	ctx := context.Background()
	var reads atomic.Int32
	srv := itemServer(t, &reads)
	setTitleIDs(t, w, w.title, map[string]string{"tmdb": "329865"})
	for _, name := range []string{"Home", "Home again"} {
		if _, err := Add(ctx, w.s, w.v, Input{Name: name, Kind: "jellyfin", URL: srv.URL, Token: tok, WebURL: "https://media.example.org/"}, domain.System); err != nil {
			t.Fatal(err)
		}
	}
	l := &Linker{Store: w.s, Vault: w.v}
	got, err := l.WatchLinks(ctx, w.title)
	if err != nil || len(got) != 1 {
		t.Fatalf("duplicate server: %+v %v, want 1 link", got, err)
	}
	if want := "https://media.example.org/web/#/details?id=mv-9&serverId=srv-1"; got[0].URL != want {
		t.Fatalf("duplicate server: %s, want %s", got[0].URL, want)
	}
}

// A title the kept list lacks reads the list again once the retry wait is
// over, so a title that just arrived gets its button soon.
func TestWatchLinksRereadsForNewTitle(t *testing.T) {
	old := indexRetry
	indexRetry = 0
	t.Cleanup(func() { indexRetry = old })
	w := newWorld(t)
	ctx := context.Background()
	var reads atomic.Int32
	srv := itemServer(t, &reads)
	if _, err := Add(ctx, w.s, w.v, Input{Name: "Home", Kind: "jellyfin", URL: srv.URL, Token: tok, WebURL: "http://jf.example"}, domain.System); err != nil {
		t.Fatal(err)
	}
	l := &Linker{Store: w.s, Vault: w.v}
	setTitleIDs(t, w, w.title, map[string]string{"tmdb": "404404"})
	_, _ = l.WatchLinks(ctx, w.title)
	time.Sleep(time.Millisecond)
	_, _ = l.WatchLinks(ctx, w.title)
	if reads.Load() != 2 {
		t.Fatalf("missing title read the list %d times, want 2", reads.Load())
	}
}

// The browser address takes http and https only, loses its trailing
// slashes, and empty clears it.
func TestWebURLInput(t *testing.T) {
	w := newWorld(t)
	ctx := context.Background()
	srv, _ := fakeServer(t, "jellyfin", "")
	for _, bad := range []string{"jellyfin.local", "javascript:alert(1)", "ftp://jf.example", "http://user:pw@jf.example", "data:text/html,x", "//jf.example"} {
		if _, err := Add(ctx, w.s, w.v, Input{Name: "A", Kind: "jellyfin", URL: srv.URL, Token: tok, WebURL: bad}, domain.System); err != ErrWebURL {
			t.Fatalf("%q: %v", bad, err)
		}
	}
	m, err := Add(ctx, w.s, w.v, Input{Name: "A", Kind: "jellyfin", URL: srv.URL, Token: tok, WebURL: " HTTPS://jf.example:8096// "}, domain.System)
	if err != nil || m.WebURL != "HTTPS://jf.example:8096" || ViewOf(m).WebURL != m.WebURL {
		t.Fatalf("%q %v", m.WebURL, err)
	}
	m, err = Change(ctx, w.s, w.v, m.ID, Input{Name: "A", URL: srv.URL}, domain.System)
	if err != nil || m.WebURL != "" {
		t.Fatalf("cleared: %q %v", m.WebURL, err)
	}
}

// plexItemServer plays Plex holding one movie (tmdb 329865, new agent) and
// one show (old agent, tvdb 100) with machine id mach-1.
func plexItemServer(t *testing.T, reads *atomic.Int32) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("X-Plex-Token") != tok {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		switch r.URL.Path {
		case "/identity":
			_, _ = w.Write([]byte(`<MediaContainer size="0" machineIdentifier="mach-1" version="1.41"/>`))
		case "/library/sections":
			_, _ = w.Write([]byte(`<MediaContainer><Directory key="1" type="movie" title="Movies"/><Directory key="2" type="show" title="TV"/></MediaContainer>`))
		case "/library/sections/1/all":
			reads.Add(1)
			_, _ = w.Write([]byte(`<MediaContainer><Video ratingKey="5" guid="plex://movie/x"><Guid id="tmdb://1"/></Video><Video ratingKey="42" guid="plex://movie/y"><Guid id="imdb://tt9"/><Guid id="tmdb://329865"/></Video></MediaContainer>`))
		case "/library/sections/2/all":
			reads.Add(1)
			_, _ = w.Write([]byte(`<MediaContainer><Directory ratingKey="77" guid="com.plexapp.agents.thetvdb://100?lang=en"/></MediaContainer>`))
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	t.Cleanup(srv.Close)
	return srv
}

// Plex needs no browser address: a movie and a show open on Plex's details
// page at app.plex.tv, or at the server's own web app when it has one.
func TestWatchLinksPlex(t *testing.T) {
	w := newWorld(t)
	setTitleIDs(t, w, w.title, map[string]string{"tmdb": "329865"})
	show, _ := packShow(t, w)
	var reads atomic.Int32
	srv := plexItemServer(t, &reads)
	ctx := context.Background()
	m, err := Add(ctx, w.s, w.v, Input{Name: "Home", Kind: "plex", URL: srv.URL, Token: tok}, domain.System)
	if err != nil {
		t.Fatal(err)
	}
	l := &Linker{Store: w.s, Vault: w.v}
	for id, key := range map[int64]string{w.title: "42", show: "77"} {
		got, err := l.WatchLinks(ctx, id)
		want := "https://app.plex.tv/desktop#!/server/mach-1/details?key=%2Flibrary%2Fmetadata%2F" + key
		if err != nil || len(got) != 1 || got[0].URL != want || got[0].Kind != "plex" {
			t.Fatalf("title %d: %+v %v, want %s", id, got, err, want)
		}
	}
	n := reads.Load()
	if _, err := l.WatchLinks(ctx, w.title); err != nil || reads.Load() != n {
		t.Fatalf("kept list read again: %d -> %d %v", n, reads.Load(), err)
	}
	if _, err := Change(ctx, w.s, w.v, m.ID, Input{Name: "Home", URL: srv.URL, WebURL: "http://plex.example:32400"}, domain.System); err != nil {
		t.Fatal(err)
	}
	got, err := l.WatchLinks(ctx, w.title)
	if want := "http://plex.example:32400/web/index.html#!/server/mach-1/details?key=%2Flibrary%2Fmetadata%2F42"; err != nil || len(got) != 1 || got[0].URL != want {
		t.Fatalf("own web app: %+v %v", got, err)
	}
	setTitleIDs(t, w, w.title, map[string]string{"tmdb": "404404"})
	if got, err := l.WatchLinks(ctx, w.title); err != nil || len(got) != 0 {
		t.Fatalf("title Plex lacks: %+v %v", got, err)
	}
}

// With no browser address, Jellyfin and Emby open at the connection address
// unless only HolyArrs can reach it (a Docker name, localhost).
func TestBrowserAddress(t *testing.T) {
	for in, want := range map[string]string{
		"http://192.168.1.10:8096/":      "http://192.168.1.10:8096",
		"https://jelly.example.org":      "https://jelly.example.org",
		"http://[fd00::5]:8096":          "http://[fd00::5]:8096",
		"http://nas.local:8096/jellyfin": "http://nas.local:8096/jellyfin",
		"http://jellyfin:8096":           "",
		"http://localhost:8096":          "",
		"http://emby.localhost:8096":     "",
		"http://127.0.0.1:8096":          "",
		"http://0.0.0.0:8096":            "",
		"":                               "",
	} {
		if got := browserAddress(domain.MediaServer{Kind: "jellyfin", URL: in}); got != want {
			t.Errorf("%q: got %q, want %q", in, got, want)
		}
	}
	if got := browserAddress(domain.MediaServer{URL: "http://jellyfin:8096", WebURL: "https://w.example"}); got != "https://w.example" {
		t.Errorf("browser address not used first: %q", got)
	}
}
