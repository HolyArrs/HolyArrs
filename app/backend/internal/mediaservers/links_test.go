package mediaservers

import (
	"context"
	"database/sql"
	"fmt"
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

// itemServer plays Jellyfin or Emby with one movie (tmdb 329865) and one
// series (tvdb 100); lists counts the item list reads.
func itemServer(t *testing.T, kind string, lists *atomic.Int32) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		auth := r.Header.Get("X-MediaBrowser-Token")
		if kind == "jellyfin" && strings.Contains(r.Header.Get("Authorization"), fmt.Sprintf("Token=%q", tok)) {
			auth = tok
		}
		if auth != tok {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		switch {
		case r.URL.Path == "/System/Info":
			_, _ = w.Write([]byte(`{"Id":"srv-1"}`))
		case r.URL.Path == "/Items" && r.URL.Query().Get("IncludeItemTypes") == "Movie":
			lists.Add(1)
			_, _ = w.Write([]byte(`{"Items":[{"Id":"other","ProviderIds":{"Tmdb":"1"}},{"Id":"mv-9","ProviderIds":{"Tmdb":"329865","Imdb":"tt2543164"}}]}`))
		case r.URL.Path == "/Items" && r.URL.Query().Get("IncludeItemTypes") == "Series":
			lists.Add(1)
			_, _ = w.Write([]byte(`{"Items":[{"Id":"sr-7","ProviderIds":{"Tvdb":"100"}}]}`))
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	t.Cleanup(srv.Close)
	return srv
}

// setIDs gives the world's movie its ids.
func setIDs(t *testing.T, w *world, id int64, ids map[string]string) {
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

// A movie and a show open on the Jellyfin details page and the Emby item
// page, by the browser address, never by HolyArrs' own one.
func TestWatchLinks(t *testing.T) {
	for _, kind := range []string{"jellyfin", "emby"} {
		t.Run(kind, func(t *testing.T) {
			w := newWorld(t)
			setIDs(t, w, w.title, map[string]string{"tmdb": "329865"})
			show, _ := packShow(t, w)
			var lists atomic.Int32
			srv := itemServer(t, kind, &lists)
			ctx := context.Background()
			if _, err := Add(ctx, w.s, w.v, Input{Name: "Home", Kind: kind, URL: srv.URL, Token: tok, WebURL: "https://media.example.org/"}, domain.System); err != nil {
				t.Fatal(err)
			}
			l := &Linker{Store: w.s, Vault: w.v}
			want := map[int64]string{
				w.title: "https://media.example.org/web/#/details?id=mv-9&serverId=srv-1",
				show:    "https://media.example.org/web/#/details?id=sr-7&serverId=srv-1",
			}
			if kind == "emby" {
				want[w.title] = "https://media.example.org/web/index.html#!/item?id=mv-9&serverId=srv-1"
				want[show] = "https://media.example.org/web/index.html#!/item?id=sr-7&serverId=srv-1"
			}
			for id, u := range want {
				got, err := l.WatchLinks(ctx, id)
				if err != nil || len(got) != 1 || got[0].URL != u || got[0].Server != "Home" || got[0].Kind != kind {
					t.Fatalf("title %d: %+v %v, want %s", id, got, err, u)
				}
				if strings.Contains(got[0].URL, srv.URL) {
					t.Fatal("link holds HolyArrs' own address")
				}
			}
			// The lists are kept: asking again reads nothing.
			n := lists.Load()
			if _, err := l.WatchLinks(ctx, w.title); err != nil || lists.Load() != n {
				t.Fatalf("read the list again: %d -> %d %v", n, lists.Load(), err)
			}
		})
	}
}

// No browser address, a title the server lacks, or a server that is down
// gives no link and no error.
func TestWatchLinksNone(t *testing.T) {
	w := newWorld(t)
	ctx := context.Background()
	var lists atomic.Int32
	srv := itemServer(t, "jellyfin", &lists)
	if _, err := Add(ctx, w.s, w.v, Input{Name: "No web", Kind: "jellyfin", URL: srv.URL, Token: tok}, domain.System); err != nil {
		t.Fatal(err)
	}
	l := &Linker{Store: w.s, Vault: w.v}
	setIDs(t, w, w.title, map[string]string{"tmdb": "329865"})
	if got, err := l.WatchLinks(ctx, w.title); err != nil || len(got) != 0 || lists.Load() != 0 {
		t.Fatalf("without a browser address: %+v %v, %d reads", got, err, lists.Load())
	}

	down := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusBadGateway) }))
	t.Cleanup(down.Close)
	if _, err := Add(ctx, w.s, w.v, Input{Name: "Down", Kind: "jellyfin", URL: down.URL, Token: tok, WebURL: "http://down.example"}, domain.System); err != nil {
		t.Fatal(err)
	}
	if _, err := Add(ctx, w.s, w.v, Input{Name: "Up", Kind: "jellyfin", URL: srv.URL, Token: tok, WebURL: "http://up.example"}, domain.System); err != nil {
		t.Fatal(err)
	}
	if got, err := l.WatchLinks(ctx, w.title); err != nil || len(got) != 1 || got[0].Server != "Up" {
		t.Fatalf("one server down: %+v %v", got, err)
	}

	setIDs(t, w, w.title, map[string]string{"tmdb": "404404"})
	if got, err := l.WatchLinks(ctx, w.title); err != nil || len(got) != 0 {
		t.Fatalf("title the server lacks: %+v %v", got, err)
	}
	if _, err := l.WatchLinks(ctx, 999999); err == nil {
		t.Fatal("no error for a missing title")
	}
}

// A title that just arrived is found once the kept list is older than the
// retry wait, without waiting for the list to expire.
func TestWatchLinksRereadsForNewTitle(t *testing.T) {
	old := indexRetry
	indexRetry = 0
	t.Cleanup(func() { indexRetry = old })
	w := newWorld(t)
	ctx := context.Background()
	var lists atomic.Int32
	srv := itemServer(t, "jellyfin", &lists)
	if _, err := Add(ctx, w.s, w.v, Input{Name: "Home", Kind: "jellyfin", URL: srv.URL, Token: tok, WebURL: "http://jf.example"}, domain.System); err != nil {
		t.Fatal(err)
	}
	l := &Linker{Store: w.s, Vault: w.v}
	setIDs(t, w, w.title, map[string]string{"tmdb": "404404"})
	_, _ = l.WatchLinks(ctx, w.title)
	time.Sleep(time.Millisecond)
	_, _ = l.WatchLinks(ctx, w.title)
	if lists.Load() != 2 {
		t.Fatalf("missing title read the list %d times, want 2", lists.Load())
	}
}

// The browser address is checked like the server address and kept without
// a trailing slash; empty clears it.
func TestWebURLInput(t *testing.T) {
	w := newWorld(t)
	ctx := context.Background()
	srv, _ := fakeServer(t, "jellyfin", "")
	if _, err := Add(ctx, w.s, w.v, Input{Name: "A", Kind: "jellyfin", URL: srv.URL, Token: tok, WebURL: "jellyfin.local"}, domain.System); err != ErrWebURL {
		t.Fatalf("bad address: %v", err)
	}
	m, err := Add(ctx, w.s, w.v, Input{Name: "A", Kind: "jellyfin", URL: srv.URL, Token: tok, WebURL: " http://jf.example:8096/ "}, domain.System)
	if err != nil || m.WebURL != "http://jf.example:8096" || ViewOf(m).WebURL != m.WebURL {
		t.Fatalf("%q %v", m.WebURL, err)
	}
	m, err = Change(ctx, w.s, w.v, m.ID, Input{Name: "A", URL: srv.URL}, domain.System)
	if err != nil || m.WebURL != "" {
		t.Fatalf("cleared: %q %v", m.WebURL, err)
	}
}
