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

// No browser address, Plex, a server that is down, or a title the server
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
