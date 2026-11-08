package library

import (
	"context"
	"database/sql"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/HolyArrs/holyarrs/app/backend/internal/core/database"
	domain "github.com/HolyArrs/holyarrs/app/backend/internal/core/model"
)

func addDownloader(t *testing.T, s *database.Store, state string, ours ...string) {
	t.Helper()
	pm := []domain.PathMap{}
	for _, o := range ours {
		pm = append(pm, domain.PathMap{Theirs: "/downloads", Ours: o})
	}
	err := s.Write(context.Background(), func(tx *sql.Tx) error {
		dr := domain.Downloader{Name: "Q-" + state, Adapter: "qbittorrent", Protocol: "torrent", URL: "http://127.0.0.1:9", Category: "holyarrs",
			Priority: 1, PathMap: pm, AfterImport: "leave", State: state}
		_, err := database.Insert(tx, "downloaders", &dr, domain.System)
		return err
	})
	if err != nil {
		t.Fatal(err)
	}
}

// TestCheckFoldersJobUpdatesHealth: the folder check reads the Downloader
// folders through DownloadPaths, tests every Library folder against them and
// stores the health; a folder that disappears is no longer usable.
func TestCheckFoldersJobUpdatesHealth(t *testing.T) {
	s := open(t)
	ctx := context.Background()
	root := t.TempDir()
	libDir := filepath.Join(root, "movies")
	saveDir := filepath.Join(root, "save")
	mapped := filepath.Join(root, "mapped")
	off := filepath.Join(root, "off-downloader")
	for _, d := range []string{libDir, saveDir, mapped, off} {
		if err := os.MkdirAll(d, 0o755); err != nil {
			t.Fatal(err)
		}
	}
	addDownloader(t, s, "ok", mapped, saveDir) // the save path again: listed once
	addDownloader(t, s, "disabled", off)       // a paused Downloader's folder is not tested against

	paths, err := DownloadPaths(ctx, s.Read(), saveDir)
	if err != nil {
		t.Fatal(err)
	}
	if !slices.Equal(paths, []string{saveDir, mapped}) {
		t.Fatalf("download paths %v, want the save path then the mapped folder, once each, without the disabled one", paths)
	}
	lf, err := AddFolder(ctx, s, FolderInput{Path: libDir, Kind: "movie", Label: "Movies"}, paths, domain.System)
	if err != nil {
		t.Fatal(err)
	}
	svc, q := folderCheckQueue(t, s, saveDir)
	if got := svc.DownloadFolders(ctx); !slices.Equal(got, paths) {
		t.Fatalf("DownloadFolders %v, want %v", got, paths)
	}
	runFolderCheck(t, s, q, runNow)
	got := storedFolder(t, s, lf.ID)
	if got.Health != "ok" || got.HealthCheckedAt == nil {
		t.Fatalf("after the check: %+v", got)
	}
	if es, _, _ := s.Events(ctx, database.EventFilter{Kinds: []string{domain.EventFolderHealth}}); len(es) > 1 {
		t.Fatalf("an unchanged health wrote %d events", len(es))
	}

	// The folder goes away: the next check says so and records an Event.
	if err := os.RemoveAll(libDir); err != nil {
		t.Fatal(err)
	}
	runFolderCheck(t, s, q, runNow)
	got = storedFolder(t, s, lf.ID)
	if got.Health == "ok" || got.Health == "no_hardlink" || got.HealthReason == "" {
		t.Fatalf("a folder that is gone is %q (%q)", got.Health, got.HealthReason)
	}
	es, _, _ := s.Events(ctx, database.EventFilter{Kinds: []string{domain.EventFolderHealth}})
	if len(es) == 0 {
		t.Fatal("no folder health Event for the change")
	}
}

// TestCachePosterStoresPictureAndKeepsOldOnFailure: a poster is fetched into
// the cache and named on the title; no cache, no URL or a failed fetch leaves
// the title as it was.
func TestCachePosterStoresPictureAndKeepsOldOnFailure(t *testing.T) {
	s := open(t)
	ctx := context.Background()
	addFolder(t, s)
	id, err := AddMovie(ctx, s, dune, AddOptions{}, nil, now, domain.System)
	if err != nil {
		t.Fatal(err)
	}
	pics := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/missing.jpg" {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "image/jpeg")
		_, _ = w.Write([]byte("\xff\xd8\xff\xe0 poster"))
	}))
	t.Cleanup(pics.Close)
	svc := &Service{Store: s, Now: func() time.Time { return now }}
	svc.CachePoster(ctx, id, pics.URL+"/p.jpg", domain.System) // no cache: nothing happens
	if a := artworkOf(t, s, id); len(a) != 0 {
		t.Fatalf("poster stored without a cache: %v", a)
	}
	svc.Artwork = &ArtworkCache{Dir: filepath.Join(t.TempDir(), "art"), Allowed: []string{pics.URL + "/"}, HTTP: &http.Client{}}
	svc.CachePoster(ctx, id, "", domain.System)
	if a := artworkOf(t, s, id); len(a) != 0 {
		t.Fatalf("poster stored without a URL: %v", a)
	}
	svc.CachePoster(ctx, id, pics.URL+"/p.jpg", domain.System)
	a := artworkOf(t, s, id)
	if len(a) != 1 || !strings.HasPrefix(a[0], "poster:") {
		t.Fatalf("artwork after a fetch: %v", a)
	}
	svc.CachePoster(ctx, id, pics.URL+"/missing.jpg", domain.System)
	if b := artworkOf(t, s, id); !slices.Equal(a, b) {
		t.Fatalf("a failed fetch changed the artwork: %v -> %v", a, b)
	}
}
